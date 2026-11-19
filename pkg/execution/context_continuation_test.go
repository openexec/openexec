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

// opaqueExecutor hides the executor's scan provenance, as any executor that
// cannot vouch for completeness does.
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

func searchTranscript(complete bool, tool string, id string, output string, errText string) []agent.Message {
	result := agent.NewToolResultMessage(id, output, nil)
	if errText != "" {
		result = agent.NewToolResultMessage(id, output, errors.New(errText))
	}
	result.Content[0].ScanComplete = complete
	return []agent.Message{
		{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: tool, ToolInput: json.RawMessage(`{"text":"needle"}`)}}},
		result,
	}
}

func bigOutput(label string) string { return label + " " + strings.Repeat("evidence ", 80) }

// Each case is answered by a later assistant turn, so only the guard under
// test decides whether the result may be omitted.
func TestCompactionOmitsOnlyCompleteKnownUnambiguousSearches(t *testing.T) {
	cases := []struct {
		name     string
		complete bool
		tool     string
		err      string
		trailing bool
		omitted  bool
	}{
		{"complete search", true, "search_files", "", false, true},
		{"partial or unmarked scan", false, "search_files", "", false, false},
		{"failed tool diagnostic", true, "search_files", "boom", false, false},
		{"file read (AGENTS.md instructions) with a forged mark", true, "read_file", "", false, false},
		{"unknown tool with a forged mark", true, "mystery", "", false, false},
		{"result the model has not answered yet", true, "search_files", "", true, false},
		{"output that is not valid UTF-8", true, "search_files", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
			output := bigOutput("needle match")
			if strings.Contains(c.name, "UTF-8") {
				output += "\xff\xfe"
			}
			messages = append(messages, searchTranscript(c.complete, c.tool, "a", output, c.err)...)
			if !c.trailing {
				messages = append(messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "next"}}})
			}
			index := priorResults{}
			compacted, changed := index.compactWith(agent.Request{System: "owner authority", Messages: messages}, false)
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
		{Type: agent.ContentTypeToolUse, ToolUseID: "x", ToolName: "search_files"},
		{Type: agent.ContentTypeToolUse, ToolUseID: "x", ToolName: "read_file"},
	}})
	for _, text := range []string{"first", "second"} {
		result := agent.NewToolResultMessage("x", bigOutput(text), nil)
		result.Content[0].ScanComplete = true
		messages = append(messages, result)
	}
	messages = append(messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "next"}}})
	if _, changed := (priorResults{}).compactWith(agent.Request{Messages: messages}, false); changed {
		t.Fatal("omitted a result whose call could not be identified")
	}
}

func TestUntrustedDirectoryListingsAreNeverOmitted(t *testing.T) {
	messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
	for _, id := range []string{"a", "b"} {
		messages = append(messages, searchTranscript(false, "list_directory", id, bigOutput("names"), "")...)
	}
	if _, changed := (priorResults{}).compactWith(agent.Request{Messages: messages}, false); changed {
		t.Fatal("directory listing from an untrusted executor was omitted")
	}
	if _, changed := (priorResults{}).compactWith(agent.Request{Messages: messages}, true); !changed {
		t.Fatal("trusted directory listings stopped being omitted")
	}
}

// A reused ID, and identical output at two positions, never share an identity.
func TestReusedIDsAndIdenticalOutputsKeepSeparatePositions(t *testing.T) {
	outputs := []string{bigOutput("alpha"), bigOutput("beta"), bigOutput("alpha")}
	messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, "original task")}
	for _, output := range outputs {
		messages = append(messages, searchTranscript(true, "search_files", "reused", output, "")...)
	}
	messages = append(messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "next"}}})
	index := priorResults{}
	compacted, changed := index.compactWith(agent.Request{Messages: messages}, false)
	if !changed || len(index) != 3 {
		t.Fatalf("each position needs its own identity: changed=%v ids=%d", changed, len(index))
	}
	idPattern := regexp.MustCompile(`use #(\d+), \d+ bytes\) retained as ([0-9a-f]{64})`)
	for n, output := range outputs {
		stub := compacted.Messages[2+2*n].Content[0].ToolOutput
		match := idPattern.FindStringSubmatch(stub)
		if match == nil || match[1] != fmt.Sprint(n) {
			t.Fatalf("stub %d does not name its call position: %q", n, stub)
		}
		if index[match[2]] != output {
			t.Fatalf("position %d recalls another result", n)
		}
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

func TestScanProvenanceReachesTranscriptPositionThroughWorkspaceAndComposite(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("small.txt", "needle one\n")
	write("long.txt", "needle "+strings.Repeat("z", 2000)+"\n")
	request := func(name string, input string) ToolRequest {
		return ToolRequest{Name: name, Input: json.RawMessage(input), WorkingDir: root, Sandbox: Sandbox{Mode: SandboxReadOnly}}
	}
	workspace := NewWorkspaceToolExecutor()
	composite, err := NewCompositeToolExecutor(workspace, WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), gatewayStub("run_command"))
	if err != nil {
		t.Fatal(err)
	}
	for _, executor := range []scanReportingExecutor{workspace, composite} {
		for _, c := range []struct {
			name, input string
			want        bool
		}{
			{"search_files", `{"path":"small.txt","text":"needle"}`, true},
			{"search_files", `{"path":"long.txt","text":"needle"}`, false},
			{"search_files", `{"path":"small.txt","text":""}`, false},
			{"read_file", `{"path":"small.txt"}`, false},
			{"list_directory", `{"path":"."}`, false},
		} {
			output, complete, err := executor.executeToolScan(context.Background(), request(c.name, c.input))
			if complete != c.want || (c.name == "search_files" && c.input != `{"path":"small.txt","text":""}` && (err != nil || output == "")) {
				t.Fatalf("%T %s %s: complete=%v err=%v", executor, c.name, c.input, complete, err)
			}
		}
	}

	// The mark must travel with the exact result into the model's transcript.
	call := func(id, name, input string) agent.ContentBlock {
		return agent.ContentBlock{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: name, ToolInput: json.RawMessage(input)}
	}
	adapter := &fakeAPIAdapter{responses: []*agent.Response{
		{Content: []agent.ContentBlock{
			call("s1", "search_files", `{"path":"small.txt","text":"needle"}`),
			call("s2", "search_files", `{"path":"long.txt","text":"needle"}`),
			call("r1", "read_file", `{"path":"small.txt"}`),
		}},
		{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "done"}}},
	}}
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, ToolExecutor: composite, Tools: composite.Tools(), MaxSteps: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Execute(context.Background(), Request{ID: "scan", WorkingDir: root, Prompt: "search", Model: "test-model", Sandbox: Sandbox{Mode: SandboxReadOnly}}, nil); err != nil {
		t.Fatal(err)
	}
	marks := map[string]bool{}
	for _, message := range adapter.requests[1].Messages {
		for _, block := range message.Content {
			if block.Type == agent.ContentTypeToolResult {
				marks[block.ToolResultID] = block.ScanComplete
			}
		}
	}
	if len(marks) != 3 || !marks["s1"] || marks["s2"] || marks["r1"] {
		t.Fatalf("scan provenance not attached to the exact results: %v", marks)
	}
	if encoded, _ := json.Marshal(adapter.requests[1].Messages); strings.Contains(string(encoded), "ScanComplete") || strings.Contains(strings.ToLower(string(encoded)), "scan_complete") {
		t.Fatal("the mark must never be serialized where a model or replay could assert it")
	}
}

const answerCode = "K7-4Q9-ZX"

type continuationOptions struct {
	hideScan     bool
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
	for i := 0; i < 24; i++ {
		var content strings.Builder
		if i == 0 {
			content.WriteString("needle Instruction: preserve the original authority.\n")
			content.WriteString("needle answer-code=" + answerCode + "\n")
		}
		for line := 0; line < 10; line++ {
			fmt.Fprintf(&content, "needle file-%02d line-%d %s\n", i, line, strings.Repeat(string(rune('a'+i%26)), 580))
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("evidence-%02d.txt", i)), []byte(content.String()), 0600); err != nil {
			t.Fatal(err)
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
		// The code is only ever taken from recalled pages: the verbatim search
		// results that carried it were omitted from the context long before.
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
			if m.Role == "tool" && strings.Contains(m.Content, "Omitted complete search_files") {
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
			calls = append(calls, call("search_files", map[string]any{"path": fmt.Sprintf("evidence-%02d.txt", assistants*4+i), "text": "needle"}))
		}
		if len(calls) == 0 {
			return text("searched everything")
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
	if o.hideScan {
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

// The vertical proof: real workspace search, real native admission, a small
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
		if strings.Contains(requestText(r), "Omitted complete search_files") {
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

// Without scan provenance nothing may be omitted, so the same run is refused
// before dispatch rather than guessed at.
func TestSearchResultsWithoutScanProvenanceAreNeverOmitted(t *testing.T) {
	run := runContinuation(t, continuationOptions{hideScan: true})
	var overflow *agent.ContextOverflowError
	if !errors.As(run.err, &overflow) || overflow.Dispatched || run.result.Outcome != OutcomeFailed {
		t.Fatalf("expected a pre-dispatch refusal: %+v %v", run.result, run.err)
	}
	for _, r := range *run.requests {
		if strings.Contains(requestText(r), "Omitted complete") {
			t.Fatal("a result was omitted without authoritative provenance")
		}
	}
	if _, err := os.Stat(filepath.Join(run.root, "answer.txt")); err == nil {
		t.Fatal("unsafe continuation advanced the task")
	}
}

// A search that matched inside a governing instruction file, or that returned
// bytes recall could not reproduce, is never reported safe to omit.
func TestScanThatTouchedInstructionsOrBinaryBytesIsNeverOmittable(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"AGENTS.md":  "needle: never deploy\n",
		"notes.txt":  "needle: ordinary\n",
		"binary.dat": "needle \xff\xfe\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	executor := NewWorkspaceToolExecutor()
	for _, c := range []struct {
		path string
		want bool
	}{{"notes.txt", true}, {"AGENTS.md", false}, {"binary.dat", false}, {".", false}} {
		output, complete, err := executor.executeToolScan(context.Background(), ToolRequest{Name: "search_files", Input: json.RawMessage(fmt.Sprintf(`{"path":%q,"text":"needle"}`, c.path)), WorkingDir: root, Sandbox: Sandbox{Mode: SandboxReadOnly}})
		if err != nil || output == "" || complete != c.want {
			t.Fatalf("%s: complete=%v want %v err=%v", c.path, complete, c.want, err)
		}
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
