package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

// fakeQwen is an OpenAI-compatible endpoint that behaves like the local Qwen
// where it fails: it enforces a context size (HTTP 400, "exceeds the available
// context size"), reads the prompt at a slow rate and abandons a request that
// would take longer than its header timeout (HTTP 504). Its model is scripted:
// it asks for `steps` tool rounds, then answers.
type fakeQwen struct {
	t              *testing.T
	contextTokens  int
	bytesPerToken  float64 // the endpoint's real tokenizer, independent of the harness estimate
	pps            float64 // prompt tokens per second (simulated); 0 = instant
	headerTimeout  float64 // simulated seconds
	steps          int
	writesHandover bool

	mu           sync.Mutex
	rounds       int
	overflows    int
	timeouts     int
	maxTokens    int
	maxBodyBytes int
	requests     []fakeRequest
}

type fakeRequest struct {
	tokens, bytes, messages, tools int
	firstUser                      string
	choice                         string
	names                          []string
}

func (f *fakeQwen) serve() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages   []map[string]any `json:"messages"`
			Tools      []map[string]any `json:"tools"`
			ToolChoice any              `json:"tool_choice"`
		}
		_ = json.Unmarshal(body, &req)
		tokens := int(float64(len(body)) / f.bytesPerToken)
		f.mu.Lock()
		defer f.mu.Unlock()
		record := fakeRequest{tokens: tokens, bytes: len(body), tools: len(req.Tools)}
		for _, m := range req.Messages {
			if m["role"] != "system" {
				record.messages++
			}
		}
		if len(req.Messages) > 0 {
			record.firstUser, _ = req.Messages[len(req.Messages)-len(req.Messages)]["content"].(string)
			for _, m := range req.Messages {
				if m["role"] == "user" {
					record.firstUser, _ = m["content"].(string)
					break
				}
			}
		}
		record.choice, _ = req.ToolChoice.(string)
		for _, tool := range req.Tools {
			if fn, ok := tool["function"].(map[string]any); ok {
				record.names = append(record.names, fmt.Sprint(fn["name"]))
			}
		}
		f.requests = append(f.requests, record)
		w.Header().Set("Content-Type", "application/json")
		if tokens > f.contextTokens {
			f.overflows++
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"error":{"message":"request (%d tokens) exceeds the available context size (%d tokens), try increasing it","type":"exceed_context_size_error"}}`, tokens, f.contextTokens)
			return
		}
		if f.pps > 0 && float64(tokens)/f.pps > f.headerTimeout {
			f.timeouts++
			w.WriteHeader(http.StatusGatewayTimeout)
			fmt.Fprint(w, `{"error":{"message":"upstream request timeout: no response headers within the header timeout"}}`)
			return
		}
		f.maxTokens, f.maxBodyBytes = max(f.maxTokens, tokens), max(f.maxBodyBytes, len(body))
		call := func(name, args string) {
			fmt.Fprintf(w, `{"id":"r","model":"qwen","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","content":"","tool_calls":[{"id":"c%d","type":"function","function":{"name":%q,"arguments":%q}}]}}],"usage":{"prompt_tokens":%d,"completion_tokens":10,"total_tokens":%d}}`, len(f.requests), name, args, tokens, tokens+10)
		}
		text := func(s string) {
			fmt.Fprintf(w, `{"id":"r","model":"qwen","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":%q}}],"usage":{"prompt_tokens":%d,"completion_tokens":10,"total_tokens":%d}}`, s, tokens, tokens+10)
		}
		has := func(name string) bool {
			for _, n := range record.names {
				if n == name {
					return true
				}
			}
			return false
		}
		last := req.Messages[len(req.Messages)-1]
		lastText, _ := last["content"].(string)
		switch {
		case record.choice == "none" || !has("inspect"):
			text("all done: final answer from the evidence")
		case f.writesHandover && has(updateHandoverTool) && last["role"] == "user" && strings.HasPrefix(lastText, "Context checkpoint:"):
			call(updateHandoverTool, fmt.Sprintf(`{"done":"inspected %d items","next_steps":"1. inspect item %d\n2. answer","decisions":"inspect in order, because items are independent"}`, f.rounds, f.rounds))
		case f.rounds >= f.steps:
			text("all done: every item inspected")
		default:
			f.rounds++
			call("inspect", fmt.Sprintf(`{"item":%d}`, f.rounds))
		}
	}))
}

type inspectTools struct {
	size  int
	mu    sync.Mutex
	calls int
}

func (e *inspectTools) ValidateAccess(string, Sandbox, []string) error { return nil }
func (e *inspectTools) ExecuteTool(_ context.Context, r ToolRequest) (string, error) {
	e.mu.Lock()
	e.calls++
	n := e.calls
	e.mu.Unlock()
	if r.Name == "run_command" {
		return "FAIL: TestX\nexit status 2", errors.New("command failed")
	}
	line := fmt.Sprintf("item %d: the quick brown fox jumps over the lazy dog\n", n)
	var b strings.Builder
	for b.Len() < e.size {
		b.WriteString(line)
	}
	return b.String() + "END-OF-ITEM-" + fmt.Sprint(n), nil
}

func inspectDefinitions() []agent.ToolDefinition {
	return []agent.ToolDefinition{
		{Name: "inspect", Description: "inspect one item", InputSchema: json.RawMessage(`{"type":"object","properties":{"item":{"type":"integer"}}}`)},
		{Name: "run_command", Description: "run a command", InputSchema: json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}}}`)},
	}
}

func newFakeProvider(t *testing.T, fake *fakeQwen, policy ContextPolicy, tools *inspectTools, maxSteps int) (*APIProvider, func()) {
	t.Helper()
	fake.t = t
	server := fake.serve()
	adapter, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{Name: "qwen", APIKey: "k", BaseURL: server.URL, Models: []string{"qwen"}, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Context: policy, Tools: inspectDefinitions(), ToolExecutor: tools, MaxSteps: maxSteps})
	if err != nil {
		t.Fatal(err)
	}
	return provider, server.Close
}

func runRequest(t *testing.T) (Request, string) {
	t.Helper()
	root := t.TempDir()
	capture := filepath.Join(root, "run-requests", "job-1.json")
	t.Setenv(RequestCaptureEnv, capture)
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	return Request{ID: "r1", WorkingDir: work, Model: "qwen", Prompt: "Inspect every item and report. TASK-CONTRACT-MARKER",
		System: "You are an implementer.", Sandbox: Sandbox{Mode: "read-only"}}, filepath.Join(root, "run-requests", "job-1.run")
}

func localPolicy() ContextPolicy {
	return ContextPolicy{ContextTokens: 32768}.WithDefaults(true)
}

// The scripted long run: 40 rounds of 40 KB each is 1.6 MB of tool output in a
// 32k-token endpoint. Without the change the chain overflows; with it the
// chain checkpoints, restarts from the handover and finishes.
func TestLongRunOverflowsWithoutDisciplineAndFinishesWithIt(t *testing.T) {
	t.Run("without", func(t *testing.T) {
		fake := &fakeQwen{contextTokens: 32768, bytesPerToken: 3, steps: 40}
		provider, closeServer := newFakeProvider(t, fake, ContextPolicy{ContextTokens: 32768, Disabled: true}, &inspectTools{size: 40 << 10}, 100)
		defer closeServer()
		request, _ := runRequest(t)
		result, err := provider.Execute(context.Background(), request, nil)
		if err == nil || result.Outcome != OutcomeFailed || fake.overflows == 0 || !strings.Contains(err.Error(), "context") {
			t.Fatalf("expected the unbounded chain to overflow: outcome=%s err=%v overflows=%d", result.Outcome, err, fake.overflows)
		}
	})
	t.Run("with", func(t *testing.T) {
		fake := &fakeQwen{contextTokens: 32768, bytesPerToken: 3, steps: 40, writesHandover: true}
		provider, closeServer := newFakeProvider(t, fake, localPolicy(), &inspectTools{size: 40 << 10}, 100)
		defer closeServer()
		request, dir := runRequest(t)
		var events []string
		result, err := provider.Execute(context.Background(), request, func(e Event) error {
			if e.Type == EventCheckpoint {
				events = append(events, e.Text)
			}
			return nil
		})
		if err != nil || result.Outcome != OutcomeSucceeded || !strings.Contains(result.FinalText, "every item inspected") {
			t.Fatalf("outcome=%s err=%v text=%q", result.Outcome, err, result.FinalText)
		}
		if fake.overflows != 0 || fake.timeouts != 0 {
			t.Fatalf("a request overflowed or timed out: %d/%d", fake.overflows, fake.timeouts)
		}
		restarts := 0
		for _, r := range fake.requests {
			if r.messages == 1 && strings.Contains(r.firstUser, "Context checkpoint") {
				restarts++
				// A restarted chain is built only from the handover: the task
				// is in it, no earlier tool result is.
				if !strings.Contains(r.firstUser, "TASK-CONTRACT-MARKER") || strings.Contains(r.firstUser, "END-OF-ITEM") {
					t.Fatalf("restarted chain is not task + handover only: %.300s", r.firstUser)
				}
			}
		}
		if restarts == 0 || len(events) == 0 {
			t.Fatalf("no chain restart happened: restarts=%d events=%v", restarts, events)
		}
		if fake.maxTokens > 32768*7/10 {
			t.Fatalf("a request of %d tokens passed the hard threshold", fake.maxTokens)
		}
		assertHandover(t, dir, "Context checkpoint")
		raw, _ := os.ReadFile(filepath.Join(dir, HandoverFile))
		if !regexp.MustCompile(`inspected \d+ items`).MatchString(string(raw)) || !strings.Contains(string(raw), "out-0001") {
			t.Fatalf("handover lacks the model's Done or the saved-output index:\n%s", raw)
		}
	})
}

func assertHandover(t *testing.T, dir, _ string) {
	t.Helper()
	info, err := os.Stat(filepath.Join(dir, HandoverFile))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("handover mode %v", info.Mode().Perm())
	}
	if d, _ := os.Stat(dir); d.Mode().Perm() != 0o700 {
		t.Fatalf("run dir mode %v", d.Mode().Perm())
	}
	raw, _ := os.ReadFile(filepath.Join(dir, HandoverFile))
	last := -1
	for _, section := range []string{"## Task", "## Constraints and do-not list", "## Done so far", "## Next steps", "## Where the information is", "## Decisions made and why", "## Open questions"} {
		at := strings.Index(string(raw), section)
		if at < 0 || at < last {
			t.Fatalf("section %q missing or out of order in:\n%s", section, raw)
		}
		last = at
	}
}

// A result of several megabytes never enters a request whole: it is a head, a
// tail and a pointer, and the full text is on disk and readable by range.
func TestHugeToolOutputNeverEntersARequestWhole(t *testing.T) {
	fake := &fakeQwen{contextTokens: 32768, bytesPerToken: 3, steps: 3}
	provider, closeServer := newFakeProvider(t, fake, localPolicy(), &inspectTools{size: 5 << 20}, 100)
	defer closeServer()
	request, dir := runRequest(t)
	result, err := provider.Execute(context.Background(), request, nil)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("outcome=%s err=%v", result.Outcome, err)
	}
	if fake.maxBodyBytes > 64<<10 {
		t.Fatalf("a request carried %d bytes of a 5 MB output", fake.maxBodyBytes)
	}
	for _, r := range fake.requests {
		if r.bytes > 64<<10 {
			t.Fatalf("request of %d bytes", r.bytes)
		}
	}
	saved, err := os.ReadFile(filepath.Join(dir, "out-0001.txt"))
	if err != nil || len(saved) < 5<<20 || !strings.HasSuffix(string(saved), "END-OF-ITEM-1") {
		t.Fatalf("full output not on disk: %d bytes err=%v", len(saved), err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "out-0001.txt")); info.Mode().Perm() != 0o600 {
		t.Fatalf("saved output mode %v", info.Mode().Perm())
	}
	state := &runState{dir: dir, policy: localPolicy(), data: handoverData{Outputs: []savedOutput{{ID: "out-0001"}}}}
	page, err := state.readSaved(json.RawMessage(`{"id":"out-0001","from_line":5,"to_line":7}`))
	if err != nil || !strings.HasPrefix(page, "out-0001 lines 5-7 of ") || !strings.Contains(page, "item 1:") {
		t.Fatalf("range read: %q %v", page, err)
	}
	if _, err := state.readSaved(json.RawMessage(`{"id":"../../etc/passwd","from_line":1}`)); err == nil {
		t.Fatal("read an id that is not a saved output")
	}
}

// The mechanical sections are the harness's own: a model that writes nothing
// still leaves files touched, commands with exit codes, test status and the
// saved-output index, and the empty sections are filled and flagged.
func TestHandoverHasMechanicalSectionsWhenTheModelWritesNothing(t *testing.T) {
	fake := &fakeQwen{contextTokens: 32768, bytesPerToken: 3, steps: 30, writesHandover: false}
	tools := &inspectTools{size: 20 << 10}
	provider, closeServer := newFakeProvider(t, fake, localPolicy(), tools, 100)
	defer closeServer()
	request, dir := runRequest(t)
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=a@b", "-c", "user.name=n", "commit", "-q", "--allow-empty", "-m", "base"}} {
		if out, err := exec.Command("git", append([]string{"-C", request.WorkingDir}, args...)...).CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v %s", err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(request.WorkingDir, "changed.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(request.WorkingDir, "seen-by-git-only.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := newRunState(request, localPolicy())
	state.recordCall("run_command", json.RawMessage(`{"command":"go test ./..."}`), "FAIL: TestX\nexit status 2", errors.New("command failed"))
	state.recordCall("write_file", json.RawMessage(`{"path":"changed.go"}`), "ok", nil)
	state.recordCall("read_file", json.RawMessage(`{"path":"pkg/a.go","start_line":120}`), "x", nil)
	state.data.Outputs = []savedOutput{{ID: "out-0001", Tool: "inspect", Bytes: 99, Lines: 3, Summary: "item 1"}}
	if err := state.save("checkpointed"); err != nil {
		t.Fatal(err)
	}
	_ = dir
	raw, _ := os.ReadFile(filepath.Join(state.dir, HandoverFile))
	text := string(raw)
	assertHandover(t, state.dir, "")
	for _, want := range []string{
		"seen-by-git-only.txt", // files touched, from git, never reported by a tool
		"`go test ./...`",      // command
		"-> exit 2",            // its exit code
		"Test status: `go test ./...` FAILED (exit 2)",
		"pkg/a.go (from line 120)", // where the information is
		"out-0001: inspect, 99 bytes, 3 lines",
		"FLAG: the model left this empty; the harness filled it from the log.",
		"FLAG: the model left this empty; do these in order",
		"Files written: changed.go",
		"None recorded by the model.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("handover lacks %q:\n%s", want, text)
		}
	}
	// And a whole run with a silent model finishes and leaves the same document.
	request2, dir2 := runRequest(t)
	result, err := provider.Execute(context.Background(), request2, nil)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("silent model: %s %v", result.Outcome, err)
	}
	raw2, _ := os.ReadFile(filepath.Join(dir2, HandoverFile))
	if !strings.Contains(string(raw2), "FLAG: the model left this empty") || !strings.Contains(string(raw2), "out-0001") {
		t.Fatalf("silent-model run left no mechanical handover:\n%s", raw2)
	}
}

// Prompt processing is slow: a request the endpoint could not read inside its
// header timeout is never dispatched.
func TestRequestThatCannotFinishInTimeIsNeverDispatched(t *testing.T) {
	// 100 prompt tokens/s, 100 s header timeout: at most 10000 tokens.
	fake := &fakeQwen{contextTokens: 131072, bytesPerToken: 3, pps: 100, headerTimeout: 100, steps: 30, writesHandover: true}
	policy := ContextPolicy{ContextTokens: 131072, PromptTokensPerSecond: 100, RequestTimeout: 100 * time.Second}.WithDefaults(true)
	provider, closeServer := newFakeProvider(t, fake, policy, &inspectTools{size: 30 << 10}, 100)
	defer closeServer()
	request, _ := runRequest(t)
	result, err := provider.Execute(context.Background(), request, nil)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("outcome=%s err=%v timeouts=%d", result.Outcome, err, fake.timeouts)
	}
	if fake.timeouts != 0 || fake.maxTokens > 8000 {
		t.Fatalf("a request the endpoint cannot read in time was sent: timeouts=%d max=%d", fake.timeouts, fake.maxTokens)
	}
	// The same run without a speed on file reaches the timeout, as in production.
	fake2 := &fakeQwen{contextTokens: 131072, bytesPerToken: 3, pps: 100, headerTimeout: 100, steps: 30, writesHandover: true}
	provider2, close2 := newFakeProvider(t, fake2, ContextPolicy{ContextTokens: 131072}.WithDefaults(true), &inspectTools{size: 30 << 10}, 100)
	defer close2()
	request2, _ := runRequest(t)
	_, _ = provider2.Execute(context.Background(), request2, nil)
	if fake2.timeouts == 0 {
		t.Fatal("fixture too small: without a prompt speed on file the endpoint never timed out, so the time bound proves nothing")
	}
}

// The guard once let an oversized request through (dispatched=true). Here the
// endpoint's tokenizer is denser than the harness assumes; the 400 it returns
// is a context checkpoint, not a failed pass.
func TestDispatchedOverflowRestartsFromTheHandover(t *testing.T) {
	fake := &fakeQwen{contextTokens: 9000, bytesPerToken: 1, steps: 12, writesHandover: true}
	provider, closeServer := newFakeProvider(t, fake, ContextPolicy{ContextTokens: 32768}.WithDefaults(true), &inspectTools{size: 3 << 10}, 100)
	defer closeServer()
	request, dir := runRequest(t)
	result, err := provider.Execute(context.Background(), request, nil)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("outcome=%s err=%v overflows=%d", result.Outcome, err, fake.overflows)
	}
	if fake.overflows == 0 {
		t.Fatal("fixture did not reach the endpoint's real limit")
	}
	assertHandover(t, dir, "")
}

// A chain that cannot restart (chain cap) ends the pass as a context
// checkpoint, never as a failure, and the next pass continues from the
// handover. Checkpoints and handover updates are not tool rounds.
func TestCheckpointPassesAreNotFailuresAndContinue(t *testing.T) {
	fake := &fakeQwen{contextTokens: 32768, bytesPerToken: 3, steps: 40, writesHandover: true}
	policy := localPolicy()
	policy.MaxChains = 1
	provider, closeServer := newFakeProvider(t, fake, policy, &inspectTools{size: 40 << 10}, 100)
	defer closeServer()
	request, dir := runRequest(t)
	var terminal Event
	result, err := provider.Execute(context.Background(), request, func(e Event) error {
		if e.Type == EventInconclusive {
			terminal = e
		}
		return nil
	})
	if err != nil || result.Outcome != OutcomeInconclusive || result.Reason != ReasonContextCheckpoint || terminal.Reason != ReasonContextCheckpoint {
		t.Fatalf("expected a clean context checkpoint, got outcome=%s reason=%s err=%v", result.Outcome, result.Reason, err)
	}
	if fake.overflows != 0 {
		t.Fatal("the checkpoint was reached by overflowing")
	}
	var saved handoverData
	raw, _ := os.ReadFile(filepath.Join(dir, handoverState))
	_ = json.Unmarshal(raw, &saved)
	if saved.Status != "checkpointed" {
		t.Fatalf("status %q", saved.Status)
	}
	// The next pass (same job, same run directory) resumes from it and finishes.
	roundsBefore, firstOfLaterPass := fake.rounds, len(fake.requests)
	for pass := 2; pass < 8; pass++ {
		result, err = provider.Execute(context.Background(), request, nil)
		if err != nil {
			t.Fatal(err)
		}
		if result.Outcome == OutcomeSucceeded {
			break
		}
		if result.Reason != ReasonContextCheckpoint {
			t.Fatalf("pass %d ended %s/%s", pass, result.Outcome, result.Reason)
		}
	}
	if result.Outcome != OutcomeSucceeded || fake.rounds <= roundsBefore {
		t.Fatalf("later passes did not continue: %s rounds %d -> %d", result.Outcome, roundsBefore, fake.rounds)
	}
	opening := fake.requests[firstOfLaterPass]
	if opening.messages != 1 || !strings.Contains(opening.firstUser, "Context checkpoint") || !strings.Contains(opening.firstUser, "pass 2") || !strings.Contains(opening.firstUser, "TASK-CONTRACT-MARKER") {
		t.Fatalf("the later pass did not open from the handover: %+v", opening)
	}
}

// The round allowance counts tool rounds only; chains and handover updates do
// not use it. A run over its rounds still synthesizes, from the handover and
// with no tools when its conversation had grown past the threshold.
func TestFinalSynthesisStillHappensFromTheHandoverWithNoTools(t *testing.T) {
	fake := &fakeQwen{contextTokens: 32768, bytesPerToken: 3, steps: 100, writesHandover: true}
	provider, closeServer := newFakeProvider(t, fake, localPolicy(), &inspectTools{size: 11 << 10}, 4)
	defer closeServer()
	request, _ := runRequest(t)
	result, err := provider.Execute(context.Background(), request, nil)
	if err != nil || result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("outcome=%s reason=%s err=%v", result.Outcome, result.Reason, err)
	}
	if fake.rounds != 4 {
		t.Fatalf("tool rounds = %d, want exactly the allowance of 4 (the handover update must not use it)", fake.rounds)
	}
	if !strings.Contains(result.FinalText, "all done: final answer") {
		t.Fatalf("no synthesis: %q", result.FinalText)
	}
	last := fake.requests[len(fake.requests)-1]
	if last.tools != 0 || last.messages != 1 || !strings.Contains(last.firstUser, "no tools are available") {
		t.Fatalf("last request: %+v", last)
	}
}

// Thresholds are the provider's: the same conversation checkpoints on a 32k
// window and not on a hosted 200k one.
func TestThresholdsComeFromTheProvider(t *testing.T) {
	small := ContextPolicy{ContextTokens: 32768}.WithDefaults(true)
	big := ContextPolicy{ContextTokens: 200000}.WithDefaults(false)
	if v, _ := small.admit(20000, 60000); v != admitHard {
		t.Fatal("20000 tokens in a 32k window should end the chain")
	}
	if v, _ := big.admit(20000, 60000); v != admitOK {
		t.Fatal("the same request should not checkpoint in a 200k window")
	}
	if v, _ := big.admit(110000, 300000); v != admitSoft {
		t.Fatal("55% of 200k is past the soft threshold")
	}
	custom := ContextPolicy{ContextTokens: 100000, SoftRatio: 0.3, HardRatio: 0.4}.WithDefaults(false)
	if v, _ := custom.admit(35000, 1000); v != admitSoft {
		t.Fatal("configured ratios not honoured")
	}
	if v, _ := custom.admit(10000, 150000); v != admitHard {
		t.Fatal("the byte bound is separate from the token estimate")
	}
	timed := ContextPolicy{ContextTokens: 131072, PromptTokensPerSecond: 100, RequestTimeout: 15 * time.Minute}.WithDefaults(true)
	if timed.timeTokens() != 72000 {
		t.Fatalf("time bound %d", timed.timeTokens())
	}
	if v, why := timed.admit(80000, 100000); v != admitHard || !strings.Contains(why, "inside the request timeout") {
		t.Fatalf("%v %s", v, why)
	}
}

func TestSizeFailureSeparatesSizeFromOtherFailures(t *testing.T) {
	policy := localPolicy()
	timeout := errors.New("HTTP error: Client.Timeout exceeded while awaiting headers (waited 15m0s of 15m0s request timeout; request body 1064217 bytes)")
	if !sizeFailure(timeout, 40000, policy) {
		t.Fatal("a header timeout on a large request is a size failure")
	}
	if sizeFailure(timeout, 500, policy) {
		t.Fatal("a header timeout on a tiny request is not hidden as a size failure")
	}
	if sizeFailure(errors.New("401 unauthorized"), 99999, policy) {
		t.Fatal("authentication is not size")
	}
	if !sizeFailure(&agent.ContextOverflowError{Dispatched: true}, 1, policy) || !sizeFailure(errors.New("input exceeds the context window of the model"), 1, policy) {
		t.Fatal("overflow not recognised")
	}
}

// The hard-token (native) path is where the guard let a request through
// (dispatched=true): the endpoint refused with HTTP 400 after the padded
// estimate admitted it. That refusal now restarts the chain from the handover.
func TestNativeRouteDispatchedOverflowRestartsFromTheHandover(t *testing.T) {
	for _, discipline := range []bool{false, true} {
		name := map[bool]string{false: "without", true: "with"}[discipline]
		t.Run(name, func(t *testing.T) {
			const endpointLimit = 90000 // bytes the real endpoint accepts
			var mu sync.Mutex
			rounds, dispatchedOverflows, restarts := 0, 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				body, _ := io.ReadAll(r.Body)
				mu.Lock()
				defer mu.Unlock()
				if len(body) > endpointLimit {
					dispatchedOverflows++
					w.WriteHeader(400)
					fmt.Fprint(w, `{"error":"the prompt exceeds the context size"}`)
					return
				}
				if strings.Contains(string(body), "Context checkpoint") && strings.Count(string(body), `"role":"tool"`) == 0 {
					restarts++
				}
				prompt := len(body) / 3
				if rounds >= 12 || strings.Contains(string(body), `"tools":[]`) {
					fmt.Fprintf(w, `{"done":true,"done_reason":"stop","prompt_eval_count":%d,"eval_count":10,"message":{"content":"all done"}}`, prompt)
					return
				}
				rounds++
				fmt.Fprintf(w, `{"done":true,"done_reason":"stop","prompt_eval_count":%d,"eval_count":10,"message":{"tool_calls":[{"function":{"name":"inspect","arguments":{"item":%d}}}]}}`, prompt, rounds)
			}))
			defer server.Close()
			adapter := newNativeProvider(t, server.URL)
			request, dir := runRequest(t)
			request.TokenBudget, request.ContextTokenLimit = 3_000_000, 65536
			policy := ContextPolicy{ContextTokens: 131072, Disabled: !discipline}.WithDefaults(true)
			provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Context: policy, Tools: inspectDefinitions(), ToolExecutor: &inspectTools{size: 11 << 10}, MaxSteps: 100})
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Execute(context.Background(), request, nil)
			if !discipline {
				var overflow *agent.ContextOverflowError
				if !errors.As(err, &overflow) || !overflow.Dispatched || result.Outcome != OutcomeFailed {
					t.Fatalf("expected the dispatched overflow to fail the pass: %v %s", err, result.Outcome)
				}
				return
			}
			if err != nil || result.Outcome != OutcomeSucceeded || result.FinalText != "all done" {
				t.Fatalf("outcome=%s err=%v text=%q overflows=%d", result.Outcome, err, result.FinalText, dispatchedOverflows)
			}
			if dispatchedOverflows == 0 || restarts == 0 {
				t.Fatalf("fixture never reached the endpoint's limit or never restarted: %d/%d", dispatchedOverflows, restarts)
			}
			assertHandover(t, dir, "")
		})
	}
}
