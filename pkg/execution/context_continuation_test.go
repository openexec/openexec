package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/openexec/openexec/pkg/agent"
)

// opaqueExecutor hides the executor's directory-listing guarantee, as any
// executor that cannot vouch for its listings does.
type opaqueExecutor struct{ inner ToolExecutor }

func (o opaqueExecutor) ValidateAccess(dir string, s Sandbox, w []string) error {
	return o.inner.ValidateAccess(dir, s, w)
}
func (o opaqueExecutor) ExecuteTool(ctx context.Context, r ToolRequest) (string, error) {
	return o.inner.ExecuteTool(ctx, r)
}

// Every response call gets the same ID, so no call/result pair is unambiguous.
type sameIDNativeAdapter struct{ *agent.OpenAIProvider }

func (a *sameIDNativeAdapter) CompleteBounded(ctx context.Context, req agent.Request, input, output int) (*agent.Response, error) {
	response, err := a.OpenAIProvider.CompleteBounded(ctx, req, input, output)
	if err == nil && response != nil {
		for i := range response.Content {
			if response.Content[i].Type == agent.ContentTypeToolUse {
				response.Content[i].ToolUseID = "same"
			}
		}
	}
	return response, err
}

func toolTranscript(tool string, id string, output string, errText string) []agent.Message {
	result := agent.NewToolResultMessage(id, output, nil)
	if errText != "" {
		result = agent.NewToolResultMessage(id, output, errors.New(errText))
	}
	return []agent.Message{
		{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: tool, ToolInput: json.RawMessage(`{"text":"needle"}`)}}},
		result,
	}
}

func bigOutput(label string) string { return label + " " + strings.Repeat("evidence ", 80) }

// The latest listing always stays in view, so each case is followed by one: only
// the guard under test decides whether the older result may be omitted.
func TestCompactionOmitsOnlyKnownUnambiguousDirectoryListings(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		err     string
		utf8bad bool
		omitted bool
	}{
		{"directory listing", "list_directory", "", false, true},
		{"complete search carrying file text", "search_files", "", false, false},
		{"failed tool diagnostic", "list_directory", "boom", false, false},
		{"file read (AGENTS.md instructions)", "read_file", "", false, false},
		{"unknown tool", "mystery", "", false, false},
		{"output that is not valid UTF-8", "list_directory", "", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
			output := bigOutput("needle match")
			if c.utf8bad {
				output += "\xff\xfe"
			}
			messages = append(messages, toolTranscript(c.tool, "a", output, c.err)...)
			messages = append(messages, toolTranscript("list_directory", "b", bigOutput("latest"), "")...)
			messages = append(messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "next"}}})
			index := priorResults{}
			compacted, changed := index.compactWith(agent.Request{System: "owner authority", Messages: messages}, true)
			got := compacted.Messages[2].Content[0].ToolOutput
			if changed != c.omitted || (strings.Contains(got, "needle match")) == c.omitted {
				t.Fatalf("omitted=%v want %v: %q", changed, c.omitted, got)
			}
			if c.omitted && (len(index) != 1 || !strings.Contains(got, recallTool)) {
				t.Fatalf("omission is not retrievable: %q %v", got, index)
			}
			if !c.omitted && len(index) != 0 {
				t.Fatalf("retained something it did not omit: %v", index)
			}
			if compacted.System != "owner authority" || messages[2].Content[0].ToolOutput == got && c.omitted {
				t.Fatal("authority or retained history was altered")
			}
		})
	}
}

func TestCompactionDoesNotOmitAmbiguousAssociation(t *testing.T) {
	messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
	// Two calls with one ID in the same turn: the results cannot be told apart.
	messages = append(messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{
		{Type: agent.ContentTypeToolUse, ToolUseID: "x", ToolName: "list_directory"},
		{Type: agent.ContentTypeToolUse, ToolUseID: "x", ToolName: "list_directory"},
	}})
	for _, text := range []string{"first", "second"} {
		messages = append(messages, agent.NewToolResultMessage("x", bigOutput(text), nil))
	}
	messages = append(messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "next"}}})
	if _, changed := (priorResults{}).compactWith(agent.Request{Messages: messages}, true); changed {
		t.Fatal("omitted a result whose call could not be identified")
	}
}

func TestUntrustedDirectoryListingsAreNeverOmitted(t *testing.T) {
	messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
	for _, id := range []string{"a", "b"} {
		messages = append(messages, toolTranscript("list_directory", id, bigOutput("names"), "")...)
	}
	if _, changed := (priorResults{}).compactWith(agent.Request{Messages: messages}, false); changed {
		t.Fatal("directory listing from an untrusted executor was omitted")
	}
	if _, changed := (priorResults{}).compactWith(agent.Request{Messages: messages}, true); !changed {
		t.Fatal("trusted directory listings stopped being omitted")
	}
}

// A reused ID, and identical output at two positions, each recall their own
// content; the newest listing stays verbatim.
func TestReusedIDsAndIdenticalOutputsRecallTheirOwnContent(t *testing.T) {
	outputs := []string{bigOutput("alpha"), bigOutput("beta"), bigOutput("alpha"), bigOutput("gamma")}
	messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
	for _, output := range outputs {
		messages = append(messages, toolTranscript("list_directory", "reused", output, "")...)
	}
	index := priorResults{}
	compacted, changed := index.compactWith(agent.Request{Messages: messages}, true)
	if !changed || len(index) != 2 {
		t.Fatalf("expected alpha and beta retained once each: changed=%v ids=%d", changed, len(index))
	}
	idPattern := regexp.MustCompile(`retained as ([0-9a-f]{64})`)
	for n, output := range outputs[:3] {
		stub := compacted.Messages[2+2*n].Content[0].ToolOutput
		match := idPattern.FindStringSubmatch(stub)
		if match == nil || index[match[1]] != output {
			t.Fatalf("position %d does not recall its own result: %q", n, stub)
		}
	}
	if compacted.Messages[8].Content[0].ToolOutput != outputs[3] {
		t.Fatal("the newest listing was omitted")
	}
}

func TestOmittedResultIsRetrievableByteForByteAcrossPages(t *testing.T) {
	// Multibyte runes straddle every 4096-byte page boundary.
	output := "x" + strings.Repeat("ä€𐍈", 2500)
	id := "abc"
	index := priorResults{id: output}
	var rebuilt strings.Builder
	offset, pages := 0, 0
	for offset < len(output) {
		page, err := index.read(json.RawMessage(fmt.Sprintf(`{"id":%q,"offset":%d}`, id, offset)))
		if err != nil {
			t.Fatal(err)
		}
		var decoded struct {
			Content    string
			NextOffset int
			TotalBytes int
		}
		if err := json.Unmarshal([]byte(page), &decoded); err != nil || !utf8.ValidString(decoded.Content) || decoded.NextOffset <= offset || decoded.TotalBytes != len(output) {
			t.Fatalf("bad page at %d: %v %s", offset, err, page)
		}
		rebuilt.WriteString(decoded.Content)
		offset, pages = decoded.NextOffset, pages+1
	}
	if rebuilt.String() != output || pages < 3 {
		t.Fatalf("not byte-for-byte across pages: pages=%d equal=%v", pages, rebuilt.String() == output)
	}
}

func TestRecallRejectsUnsafeRequests(t *testing.T) {
	index := priorResults{"known": "aé"}
	for _, bad := range []string{`not json`, `{}`, `{"id":"unknown","offset":0}`, `{"id":"known","offset":-1}`, `{"id":"known","offset":99}`, `{"id":"known","offset":2}`} {
		if _, err := index.read(json.RawMessage(bad)); err == nil {
			t.Fatalf("accepted %s", bad)
		}
	}
	if _, err := index.read(json.RawMessage(`{"id":"known","offset":3}`)); err != nil {
		t.Fatalf("the end offset is valid: %v", err)
	}
}

func TestRecallCoverageNeedsAContiguousRead(t *testing.T) {
	meter := &boundedAPIAdapter{prior: priorResults{"id": strings.Repeat("a", 9000)}}
	meter.noteRecall(recalledPage{"id", 8192, 9000, 9000}) // only the last page
	if meter.unreadOmissions(false) != 1 {
		t.Fatal("skipping ahead counted as reading the gap")
	}
	meter.noteRecall(recalledPage{"id", 0, 4096, 9000})
	meter.noteRecall(recalledPage{"id", 4096, 8192, 9000})
	if meter.unreadOmissions(false) != 1 || !strings.Contains(meter.omissionLimitation(false), "not read back") {
		t.Fatal("a partial read must still be reported")
	}
	meter.noteRecall(recalledPage{"id", 8192, 9000, 9000})
	if meter.unreadOmissions(false) != 0 || meter.omissionLimitation(false) != "" {
		t.Fatal("a complete read was still reported as a limitation")
	}
}

const answerCode = "K7-4Q9-ZX"

type continuationOptions struct {
	hideDirs     bool
	ids          string // "", "reused" or "same"
	contextLimit int64
	cancelOn     string // "recall" or "continuation"
	recallEarly  bool
	badRecall    string // "id" or "offset": the model asks for a page that does not exist
	maxSteps     int
}

type continuationRun struct {
	result   Result
	err      error
	requests *[]nativeWireRequest
	root     string
	recalls  int
	ids      map[string]int
}

func runContinuation(t *testing.T, o continuationOptions) continuationRun {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Never deploy without review.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	// Directory listings are the only omittable results: names alone, no file
	// text. The answer code is a file name in the first directory, so it can
	// only reach the final answer through a recalled page of the omitted listing.
	for i := 0; i < 24; i++ {
		dir := filepath.Join(root, fmt.Sprintf("ev-%02d", i))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		names := []string{}
		if i == 0 {
			names = append(names, "answer-code="+answerCode)
		}
		for j := 0; j < 60; j++ {
			names = append(names, fmt.Sprintf("file-%02d-%02d-%s", i, j, strings.Repeat(string(rune('a'+i%26)), 80)))
		}
		for _, name := range names {
			if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var seen *[]nativeWireRequest
	stubID := regexp.MustCompile(`retained as ([0-9a-f]{64})`)
	var server = func(n int) map[string]any {
		if o.cancelOn == "continuation" && stubID.MatchString(requestText((*seen)[n-1])) {
			cancel()
		}
		// Native servers report the true prompt size; approximate it.
		prompt := min(len(requestText((*seen)[n-1]))/4+200, 30000)
		call := func(name string, args map[string]any) map[string]any {
			return map[string]any{"function": map[string]any{"name": name, "arguments": args}}
		}
		reply := func(calls ...map[string]any) map[string]any {
			return map[string]any{"done": true, "prompt_eval_count": prompt, "eval_count": 100, "message": map[string]any{"tool_calls": calls}}
		}
		text := func(s string) map[string]any {
			return map[string]any{"done": true, "prompt_eval_count": prompt, "eval_count": 100, "message": map[string]any{"content": s}}
		}
		messages := (*seen)[n-1].Messages
		last := messages[len(messages)-1]
		assistants := 0
		for _, m := range messages {
			if m.Role == "assistant" {
				assistants++
			}
		}
		if strings.Contains(messages[0].Content, finalSynthesisInstruction) {
			return text("Synthesis of the evidence gathered so far.")
		}
		// The code is only ever taken from recalled pages: the verbatim listing
		// that carried it were omitted from the context long before.
		recalled := ""
		for _, m := range messages {
			var p struct {
				ID      string
				Content string
			}
			if m.Role == "tool" && json.Unmarshal([]byte(m.Content), &p) == nil && p.ID != "" {
				recalled += p.Content
			}
		}
		code := regexp.MustCompile(`answer-code=([A-Z0-9-]+)`).FindStringSubmatch(recalled)
		var page struct {
			ID         string
			NextOffset int
			TotalBytes int
		}
		if last.Role == "tool" && json.Unmarshal([]byte(last.Content), &page) == nil && page.ID != "" {
			if page.NextOffset < page.TotalBytes {
				return reply(call(recallTool, map[string]any{"id": page.ID, "offset": page.NextOffset}))
			}
			if code == nil {
				return text("The recalled evidence holds no answer code.")
			}
			return reply(call("write_file", map[string]any{"path": "answer.txt", "content": "answer-code=" + code[1] + "\n"}))
		}
		if _, err := os.Stat(filepath.Join(root, "answer.txt")); err == nil && code != nil {
			// Independent verification: read the written file back through the
			// tool and compare it with the code recalled from the omitted result.
			if strings.HasPrefix(last.Content, "answer-code=") {
				if strings.TrimSpace(last.Content) == "answer-code="+code[1] {
					return text("The original task is verified: " + code[1])
				}
				return text("Verification failed: the answer does not match the evidence.")
			}
			return reply(call("read_file", map[string]any{"path": "answer.txt"}))
		}
		if o.recallEarly && n == 1 {
			return reply(call(recallTool, map[string]any{"id": "0123", "offset": 0}))
		}
		for _, m := range messages {
			if m.Role == "tool" && strings.Contains(m.Content, "Prior directory listing retained") {
				id, offset := stubID.FindStringSubmatch(m.Content)[1], 0
				switch o.badRecall {
				case "id":
					id = strings.Repeat("0", 64)
				case "offset":
					offset = 1 << 30
				}
				return reply(call(recallTool, map[string]any{"id": id, "offset": offset}))
			}
		}
		var calls []map[string]any
		if assistants == 0 {
			calls = append(calls, call("read_file", map[string]any{"path": "AGENTS.md"}))
		}
		for i := 0; i < 4 && assistants*4+i < 24; i++ {
			calls = append(calls, call("list_directory", map[string]any{"path": fmt.Sprintf("ev-%02d", assistants*4+i)}))
		}
		if len(calls) == 0 {
			return text("listed everything")
		}
		return reply(calls...)
	}
	srv, requests := nativeTemplateServer(t, server)
	seen = requests
	var inner agent.ProviderAdapter = &reusedIDNativeAdapter{newNativeProvider(t, srv.URL)}
	if o.ids == "same" {
		inner = &sameIDNativeAdapter{newNativeProvider(t, srv.URL)}
	}
	var executor ToolExecutor = NewWorkspaceToolExecutor()
	if o.hideDirs {
		executor = opaqueExecutor{executor}
	}
	sandbox := Sandbox{Mode: SandboxWorkspaceWrite}
	steps := o.maxSteps
	if steps == 0 {
		steps = 40
	}
	p, err := NewAPIProvider(APIProviderConfig{Adapter: inner, ToolExecutor: executor, Tools: WorkspaceTools(sandbox), MaxSteps: steps})
	if err != nil {
		t.Fatal(err)
	}
	limit := o.contextLimit
	if limit == 0 {
		limit = 32768
	}
	run := continuationRun{requests: requests, root: root, ids: map[string]int{}}
	run.result, run.err = p.Execute(ctx, Request{ID: "continuation", WorkingDir: root, Prompt: "Inspect the evidence and advance the original task.", System: "Owner: never deploy without review.", Model: "local", Sandbox: sandbox, WritableRoots: []string{root}, TokenBudget: 250000, ContextTokenLimit: limit}, func(e Event) error {
		if e.Type == EventToolStarted && e.ToolName == recallTool {
			run.recalls++
			if o.cancelOn == "recall" {
				cancel()
			}
		}
		if e.Type == EventToolCompleted {
			run.ids[e.CallID]++
		}
		return nil
	})
	return run
}

func requestText(r nativeWireRequest) string {
	var all strings.Builder
	for _, m := range r.Messages {
		all.WriteString(m.Content + "\n")
	}
	return all.String()
}

// The vertical proof: real workspace directory listings, real native admission, a small
// context. The run is compacted, recalls the omitted evidence through the real
// recall tool across pages, advances the task, and ends with a stated limit.
func TestNativeTaskContinuesAfterBoundedCompactionAndRealRecall(t *testing.T) {
	run := runContinuation(t, continuationOptions{})
	if run.err != nil || run.result.Outcome != OutcomeSucceeded {
		t.Fatalf("run did not finish: %+v %v", run.result, run.err)
	}
	// Checked outside the scripted endpoint: the file holds the code that only
	// the omitted, recalled result contained, and the run reports it verified.
	data, err := os.ReadFile(filepath.Join(run.root, "answer.txt"))
	if err != nil || string(data) != "answer-code="+answerCode+"\n" || !strings.Contains(run.result.FinalText, "The original task is verified: "+answerCode) {
		t.Fatalf("original task was not advanced: %q %v", data, err)
	}
	if run.recalls < 2 {
		t.Fatalf("expected a multi-page recall, got %d", run.recalls)
	}
	if run.ids["reused-0"] < 2 {
		t.Fatalf("expected reused tool-call IDs: %v", run.ids)
	}
	requests := *run.requests
	stubbed := -1
	for i, r := range requests {
		if strings.Contains(requestText(r), "Prior directory listing retained") {
			stubbed = i
			break
		}
	}
	if stubbed < 1 {
		t.Fatal("the context never required compaction")
	}
	for i, r := range requests[stubbed:] {
		text := requestText(r)
		if !strings.Contains(text, "Never deploy without review") || !strings.Contains(r.Messages[0].Content, "never deploy without review") {
			t.Fatalf("request %d lost instructions", stubbed+i)
		}
	}
	last := requestText(requests[len(requests)-1])
	if !strings.Contains(last, "file-00") {
		t.Fatal("recalled evidence is not in the final request")
	}
	if !strings.Contains(run.result.FinalText, "Limitation:") || !strings.Contains(run.result.FinalText, "not read back") {
		t.Fatalf("answer relied on omitted evidence that was never recalled without saying so: %q", run.result.FinalText)
	}
}

// Without the executor's guarantee nothing may be omitted, so the same run is refused
// before dispatch rather than guessed at.
func TestListingsFromAnExecutorThatCannotVouchAreNeverOmitted(t *testing.T) {
	run := runContinuation(t, continuationOptions{hideDirs: true})
	var overflow *agent.ContextOverflowError
	if !errors.As(run.err, &overflow) || overflow.Dispatched || run.result.Outcome != OutcomeFailed {
		t.Fatalf("expected a pre-dispatch refusal: %+v %v", run.result, run.err)
	}
	for _, r := range *run.requests {
		if strings.Contains(requestText(r), "Prior directory listing retained") {
			t.Fatal("a result was omitted without the executor's guarantee")
		}
	}
	if _, err := os.Stat(filepath.Join(run.root, "answer.txt")); err == nil {
		t.Fatal("unsafe continuation advanced the task")
	}
}

func TestAmbiguousCallIDsFailTruthfullyWithoutUnsafeDispatch(t *testing.T) {
	run := runContinuation(t, continuationOptions{ids: "same"})
	// The provider already refuses a turn whose call IDs cannot be paired,
	// before any further dispatch; compaction also never omits such a pair
	// (TestCompactionDoesNotOmitAmbiguousAssociation).
	if run.err == nil || run.result.Outcome != OutcomeFailed || !strings.Contains(run.err.Error(), "no matching call") || len(*run.requests) != 1 {
		t.Fatalf("expected a pre-dispatch refusal after one request: %+v %v", run.result, run.err)
	}
}

func TestInsufficientContextAfterCompactionFailsBeforeDispatch(t *testing.T) {
	run := runContinuation(t, continuationOptions{contextLimit: 4096})
	var overflow *agent.ContextOverflowError
	if !errors.As(run.err, &overflow) || overflow.Dispatched || run.result.Outcome != OutcomeFailed {
		t.Fatalf("expected a pre-dispatch refusal: %+v %v", run.result, run.err)
	}
	if len(*run.requests) != 1 {
		t.Fatalf("an oversized request was dispatched: %d requests", len(*run.requests))
	}
}

// An invalid, unknown or unavailable recall ends the run: the model must not
// carry on as if it had read evidence it was refused.
func TestUnsafeRecallFailsTruthfullyWithoutAnotherDispatch(t *testing.T) {
	for _, c := range []struct {
		name string
		o    continuationOptions
	}{
		{"unavailable before any omission", continuationOptions{recallEarly: true}},
		{"unknown id", continuationOptions{badRecall: "id"}},
		{"invalid offset", continuationOptions{badRecall: "offset"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			run := runContinuation(t, c.o)
			if run.err == nil || run.result.Outcome != OutcomeFailed || !strings.Contains(run.err.Error(), "recall") || !strings.Contains(run.result.FinalText, "recall") {
				t.Fatalf("expected a truthful failure: %+v %v", run.result, run.err)
			}
			requests := *run.requests
			if strings.Contains(requestText(requests[len(requests)-1]), "unknown result") || strings.Contains(requestText(requests[len(requests)-1]), "recall unavailable") {
				t.Fatal("the recall failure was fed back to the model and dispatched")
			}
			if _, err := os.Stat(filepath.Join(run.root, "answer.txt")); err == nil {
				t.Fatal("the task advanced past a refused recall")
			}
			if run.recalls != 1 {
				t.Fatalf("expected exactly the refused recall, got %d", run.recalls)
			}
		})
	}
}

// Final synthesis is never compacted (its stubs would point at a recall tool it
// cannot call). A run that ends on the tool-round limit with a transcript that
// no longer fits is therefore refused before dispatch, not answered from stubs.
func TestMaxRoundSynthesisIsRefusedNotAnsweredFromStubs(t *testing.T) {
	run := runContinuation(t, continuationOptions{maxSteps: 7})
	var overflow *agent.ContextOverflowError
	if !errors.As(run.err, &overflow) || overflow.Dispatched || run.result.Outcome != OutcomeFailed {
		t.Fatalf("expected a pre-dispatch refusal: %+v %v", run.result, run.err)
	}
	for _, r := range *run.requests {
		if strings.Contains(r.Messages[0].Content, finalSynthesisInstruction) {
			t.Fatal("a synthesis request was dispatched")
		}
	}
	if strings.Contains(run.result.FinalText, "verified") {
		t.Fatalf("a claim was made without the evidence: %q", run.result.FinalText)
	}
}

func TestOmissionLimitationCountsEverythingWhenRecalledPagesAreOutOfView(t *testing.T) {
	meter := &boundedAPIAdapter{prior: priorResults{"id": "abc"}}
	meter.noteRecall(recalledPage{"id", 0, 3, 3})
	if meter.omissionLimitation(false) != "" {
		t.Fatal("a fully read result in view is not a limitation")
	}
	if !strings.Contains(meter.omissionLimitation(true), "Limitation:") {
		t.Fatal("a rebuilt request holds no recalled page; the read must not count")
	}
}

func TestStopHaltsContinuationAndRecall(t *testing.T) {
	for _, point := range []string{"continuation", "recall"} {
		t.Run(point, func(t *testing.T) {
			run := runContinuation(t, continuationOptions{cancelOn: point})
			if !errors.Is(run.err, context.Canceled) {
				t.Fatalf("stop was not honoured: %+v %v", run.result, run.err)
			}
			if _, err := os.Stat(filepath.Join(run.root, "answer.txt")); err == nil {
				t.Fatal("work continued after Stop")
			}
			if point == "recall" && (run.result.Outcome != OutcomeCancelled || run.recalls != 1) {
				t.Fatalf("recall continued after Stop: %+v recalls=%d", run.result, run.recalls)
			}
			for _, r := range *run.requests {
				if strings.Contains(requestText(r), `"nextOffset"`) && point == "recall" {
					t.Fatal("a request was dispatched carrying a recalled page after Stop")
				}
			}
		})
	}
}
