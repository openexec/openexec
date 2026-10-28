package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Everything below drives the real APIProvider and the real OpenAI-compatible
// adapter against a LOCAL scripted server. It is a FIXTURE: it shows what the
// loop sends, receives and records for a triage-shaped run (16 rounds of two
// tool calls, then the tool-less synthesis returning an 8971-character answer).
// It says nothing about how the configured endpoint or model behaves, and the
// 32/16/8971 numbers are the observed shape of native run 9fc124a8, not its content.

const (
	fixtureRounds    = 16
	fixtureCallsEach = 2
	fixtureAnswerLen = 8971
	fixtureSecret    = "sk-abcdefghijklmnopqrstuvwx"
	fixtureAPIKey    = "test-key-not-in-the-body"
)

// fixtureToolOutput is longer than the capture's 2000-character message bound
// and carries a secret-shaped string, so truncation and redaction both show.
var fixtureToolOutput = strings.Repeat("evidence line without secrets\n", 170) + "token=" + fixtureSecret + "\n" + strings.Repeat("tail\n", 100)

type fixtureTools struct{ recordingToolExecutor }

func (e *fixtureTools) ExecuteTool(ctx context.Context, request ToolRequest) (string, error) {
	_, _ = e.recordingToolExecutor.ExecuteTool(ctx, request)
	return fixtureToolOutput, nil
}

func fixtureAnswer() string {
	return string([]rune(strings.Repeat("Execution contract paragraph. ", 400))[:fixtureAnswerLen])
}

// fixtureServer scripts the triage-shaped sequence over HTTP and returns the
// Authorization headers it saw.
func fixtureServer(t *testing.T, answer string) (*httptest.Server, *[]string) {
	t.Helper()
	var calls atomic.Int32
	var auth []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		auth = append(auth, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		n := int(calls.Add(1))
		if n <= fixtureRounds {
			var parts []string
			for c := 0; c < fixtureCallsEach; c++ {
				parts = append(parts, fmt.Sprintf(`{"id":"call-%d-%d","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"f%d.txt\"}"}}`, n, c, n))
			}
			_, _ = fmt.Fprintf(w, `{"id":"r%d","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[%s]},"finish_reason":"tool_calls"}]}`, n, strings.Join(parts, ","))
			return
		}
		body, _ := json.Marshal(answer)
		_, _ = fmt.Fprintf(w, `{"id":"r%d","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, n, body)
	}))
	t.Cleanup(server.Close)
	return server, &auth
}

func fixtureProvider(t *testing.T, adapter agent.ProviderAdapter, tools ToolExecutor) *APIProvider {
	t.Helper()
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: tools, MaxSteps: fixtureRounds,
		Tools: []agent.ToolDefinition{{Name: "read_file", Description: "Read a file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func fixtureRequest(t *testing.T) Request {
	return Request{ID: "triage-fixture", WorkingDir: t.TempDir(), Prompt: "triage this task " + fixtureSecret, Model: "test-model",
		System: "standing context", Sandbox: Sandbox{Mode: "read-only"}, ReadableRoots: []string{"/srv/evidence"}}
}

func runFixture(t *testing.T, adapter agent.ProviderAdapter, tools ToolExecutor) (Result, []Event, error) {
	t.Helper()
	var events []Event
	result, err := fixtureProvider(t, adapter, tools).Execute(context.Background(), fixtureRequest(t),
		func(event Event) error { events = append(events, event); return nil })
	return result, events, err
}

func readTrace(t *testing.T, path string) map[string]any {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no trace written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("trace mode = %v, want 0600", info.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(path))
	if err != nil || dir.Mode().Perm() != 0o700 {
		t.Fatalf("trace directory = %v %v, want 0700", dir, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var trace map[string]any
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatal(err)
	}
	return trace
}

func asMap(v any) map[string]any        { m, _ := v.(map[string]any); return m }
func asList(v any) []any                { l, _ := v.([]any); return l }
func asString(v any) string             { s, _ := v.(string); return s }
func entryRequest(e any) map[string]any { return asMap(asMap(e)["request"]) }

func roles(request map[string]any) string {
	var out []string
	for _, message := range asList(request["messages"]) {
		out = append(out, asString(asMap(message)["role"]))
	}
	return strings.Join(out, ",")
}

// The sequence the existing capture cannot hold, recorded in full by the trace:
// every request with its roles, tool definitions, tool_choice and tool results,
// the response to each, the events in order and the caller's outcome. The same
// run through RequestCaptureEnv keeps one request, cut and without a response.
func TestFixtureTriageSequenceIsRecordedInFullByTheTraceNotTheCapture(t *testing.T) {
	server, auth := fixtureServer(t, fixtureAnswer())
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "traces", "run.json") // directory created by the writer
	capturePath := filepath.Join(dir, "capture.json")
	t.Setenv(RequestTraceEnv, tracePath)
	t.Setenv(RequestCaptureEnv, capturePath)

	result, events, err := runFixture(t, compatible(t, server.URL), &fixtureTools{})
	if err != nil || result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if !strings.Contains(result.FinalText, fixtureAnswer()) {
		t.Fatal("the synthesized answer is not in the caller's result")
	}

	trace := readTrace(t, tracePath)
	entries := asList(trace["entries"])
	if len(entries) != fixtureRounds+1 || trace["incomplete"] != false {
		t.Fatalf("entries = %d incomplete=%v", len(entries), trace["incomplete"])
	}
	// Roles grow by an assistant batch and its tool results each round.
	first, synthesis := entryRequest(entries[0]), entryRequest(entries[fixtureRounds])
	if roles(first) != "user" || first["tool_choice"] != "auto" || len(asList(first["tools"])) != 1 {
		t.Fatalf("first request: %v", first)
	}
	// The system prompt is a field of the loop's request, a message on the wire.
	wantRoles := "user" + strings.Repeat(",assistant"+strings.Repeat(",tool", fixtureCallsEach), fixtureRounds)
	// The synthesis request is the round-16 conversation plus nothing but the
	// budget instruction in the system turn; it keeps the tool definitions.
	if got := roles(synthesis); got != wantRoles || synthesis["tool_choice"] != "none" || len(asList(synthesis["tools"])) != 1 {
		t.Fatalf("synthesis request roles=%q choice=%v tools=%d", got, synthesis["tool_choice"], len(asList(synthesis["tools"])))
	}
	if !strings.Contains(asString(synthesis["system"]), "tool-call budget") {
		t.Fatalf("budget instruction missing from synthesis request")
	}
	// Tool results are untruncated; only the secret-shaped string is replaced.
	toolMessage := asMap(asList(synthesis["messages"])[2])
	body := fmt.Sprint(toolMessage)
	if !strings.Contains(body, strings.Repeat("evidence line without secrets\n", 170)) || strings.Contains(body, "characters cut") {
		t.Fatalf("tool result was cut in the trace")
	}
	// The adapter's own body is kept for the replay, with tools in the synthesis wire request.
	wire := asMap(asMap(entries[fixtureRounds])["wire"])
	if wire["tool_choice"] != "none" || len(asList(wire["tools"])) != 1 || len(asList(wire["messages"])) != len(asList(synthesis["messages"]))+1 {
		t.Fatalf("wire body: tool_choice=%v tools=%d", wire["tool_choice"], len(asList(wire["tools"])))
	}
	response := asMap(asMap(entries[fixtureRounds])["response"])
	text := asString(asMap(asList(response["content"])[0])["text"])
	if len([]rune(text)) != fixtureAnswerLen {
		t.Fatalf("recorded synthesis response is %d characters, want %d", len([]rune(text)), fixtureAnswerLen)
	}
	// Events, in order, with the terminal after the delta and no completed event.
	var types []string
	started, completed := 0, 0
	for _, e := range asList(trace["events"]) {
		ty := asString(asMap(e)["type"])
		types = append(types, ty)
		if ty == EventToolStarted {
			started++
		}
		if ty == EventToolCompleted {
			completed++
		}
	}
	if started != 32 || completed != 32 || len(types) != len(events) || types[len(types)-1] != EventInconclusive || types[len(types)-2] != EventAssistantDelta {
		t.Fatalf("events = %v", types)
	}
	if trace["outcome"] != OutcomeInconclusive || trace["reason"] != ReasonMaxTurns || len([]rune(asString(trace["finalText"]))) < fixtureAnswerLen {
		t.Fatalf("outcome fields = %v %v", trace["outcome"], trace["reason"])
	}
	// Redaction is reported, never silent.
	if trace["redactionChangedContent"] != true {
		t.Fatal("the trace replaced a secret and did not say so")
	}
	raw, _ := os.ReadFile(tracePath)
	for _, leak := range []string{fixtureSecret, fixtureAPIKey, "Authorization"} {
		if strings.Contains(string(raw), leak) {
			t.Fatalf("%q reached the trace", leak)
		}
	}
	if len(*auth) != fixtureRounds+1 || !strings.Contains((*auth)[0], fixtureAPIKey) {
		t.Fatalf("the fixture did not exercise authentication: %v", *auth)
	}

	// The existing capture, for the same run, cannot reconstruct any of it.
	capture := readCapture(t, capturePath)
	request := capture["request"].(map[string]any)
	if capture["truncated"] != true || len(asList(request["messages"])) != len(asList(synthesis["messages"]))+1 {
		t.Fatalf("capture = truncated:%v", capture["truncated"])
	}
	for _, key := range []string{"response", "entries", "events", "responses"} {
		if _, ok := capture[key]; ok {
			t.Fatalf("capture unexpectedly holds %q", key)
		}
	}
	if !strings.Contains(fmt.Sprint(asList(request["messages"])[3]), "characters cut") {
		t.Fatalf("capture kept the tool result whole; the regression no longer shows the loss")
	}
	// In THIS loop the synthesis request keeps its tool definitions, so the
	// empty tools array seen in the real 9fc capture is not explained by it.
	if len(asList(request["tools"])) != 1 {
		t.Fatalf("capture tools = %v", request["tools"])
	}
}

// normalized is a request or response as the trace stores it.
func normalized(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	out, _ := sanitizeCapture(tree, math.MaxInt)
	return out
}

// traceReplayAdapter is the round-trip consumer: it serves the responses a
// trace recorded and refuses any request that differs from the recorded one.
// It reads the trace file only; it is not api-replay.py and says nothing about
// that script's compatibility.
type traceReplayAdapter struct {
	fakeAPIAdapter
	t       *testing.T
	entries []any
	next    int
}

func (a *traceReplayAdapter) Complete(_ context.Context, request agent.Request) (*agent.Response, error) {
	a.t.Helper()
	if a.next >= len(a.entries) {
		return nil, fmt.Errorf("replay asked for request %d, trace has %d", a.next+1, len(a.entries))
	}
	entry := asMap(a.entries[a.next])
	a.next++
	if !reflect.DeepEqual(normalized(a.t, request), entry["request"]) {
		return nil, fmt.Errorf("request %d differs from the recorded request", a.next)
	}
	raw, _ := json.Marshal(entry["response"])
	var response agent.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// recordedTools answers each call with the output the trace recorded for it.
type recordedTools struct {
	recordingToolExecutor
	outputs map[string]string
}

func (e *recordedTools) ExecuteTool(ctx context.Context, request ToolRequest) (string, error) {
	_, _ = e.recordingToolExecutor.ExecuteTool(ctx, request)
	return e.outputs[request.CallID], nil
}

// A written trace is enough to run the loop again offline: the replay consumer
// gets the same requests, returns the recorded responses and ends in the same
// outcome with the same events. That is what "complete and not truncated" means.
func TestFixtureTraceRoundTripsThroughAReplayConsumer(t *testing.T) {
	server, _ := fixtureServer(t, fixtureAnswer())
	path := filepath.Join(t.TempDir(), "traces", "trace.json")
	t.Setenv(RequestTraceEnv, path)
	original, originalEvents, err := runFixture(t, compatible(t, server.URL), &fixtureTools{})
	if err != nil {
		t.Fatal(err)
	}
	trace := readTrace(t, path)
	t.Setenv(RequestTraceEnv, "")

	outputs := map[string]string{}
	for _, e := range asList(trace["events"]) {
		if event := asMap(e); event["type"] == EventToolCompleted {
			outputs[asString(event["call_id"])] = asString(event["text"])
		}
	}
	replay := &traceReplayAdapter{t: t, entries: asList(trace["entries"])}
	replayed, replayedEvents, err := runFixture(t, replay, &recordedTools{outputs: outputs})
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if replay.next != fixtureRounds+1 || replayed.Outcome != original.Outcome || replayed.Reason != original.Reason || replayed.FinalText != original.FinalText {
		t.Fatalf("replayed %+v after %d requests", replayed, replay.next)
	}
	if !reflect.DeepEqual(eventTypes(replayedEvents), eventTypes(originalEvents)) {
		t.Fatalf("events differ: %v vs %v", eventTypes(replayedEvents), eventTypes(originalEvents))
	}
}

// Near miss for the consumer: a trace edited so one request differs is refused,
// not silently replayed.
func TestFixtureTraceReplayRefusesADivergingRequest(t *testing.T) {
	server, _ := fixtureServer(t, fixtureAnswer())
	path := filepath.Join(t.TempDir(), "traces", "trace.json")
	t.Setenv(RequestTraceEnv, path)
	if _, _, err := runFixture(t, compatible(t, server.URL), &fixtureTools{}); err != nil {
		t.Fatal(err)
	}
	trace := readTrace(t, path)
	t.Setenv(RequestTraceEnv, "")
	entries := asList(trace["entries"])
	entryRequest(entries[3])["system"] = "a different system prompt"
	replay := &traceReplayAdapter{t: t, entries: entries}
	outputs := map[string]string{}
	for _, e := range asList(trace["events"]) {
		if event := asMap(e); event["type"] == EventToolCompleted {
			outputs[asString(event["call_id"])] = asString(event["text"])
		}
	}
	_, _, err := runFixture(t, replay, &recordedTools{outputs: outputs})
	if err == nil || !strings.Contains(err.Error(), "differs from the recorded request") {
		t.Fatalf("err = %v", err)
	}
}

// Controls: off by default, only for runs that end failed or inconclusive, an
// unwritable path changes nothing about the run, a run with nothing to redact
// says so, and the trace grants the tools nothing.
func TestRequestTraceControls(t *testing.T) {
	answer := fixtureAnswer()
	server, _ := fixtureServer(t, answer)
	dir := t.TempDir()

	// Baseline run, trace unset: nothing written, and the tracer is the no-op.
	t.Setenv(RequestTraceEnv, "")
	if newRunTrace() != nil {
		t.Fatal("an unset variable produced a tracer")
	}
	baseTools := &fixtureTools{}
	base, baseEvents, baseErr := runFixture(t, compatible(t, server.URL), baseTools)
	if baseErr != nil {
		t.Fatal(baseErr)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("files appeared without the variable: %v", entries)
	}

	// Unwritable: the parent is a regular file. The run is identical.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(RequestTraceEnv, filepath.Join(blocker, "sub", "trace.json"))
	server2, _ := fixtureServer(t, answer)
	tools := &fixtureTools{}
	result, events, err := runFixture(t, compatible(t, server2.URL), tools)
	if err != nil || result.Outcome != base.Outcome || result.Reason != base.Reason || result.FinalText != base.FinalText ||
		!reflect.DeepEqual(eventTypes(events), eventTypes(baseEvents)) {
		t.Fatalf("an unwritable trace changed the run: %+v %v", result, err)
	}
	// Tool grants are the request's, not the trace's.
	if len(tools.requests) != len(baseTools.requests) {
		t.Fatalf("tool calls %d vs %d", len(tools.requests), len(baseTools.requests))
	}
	for i := range tools.requests {
		a, b := tools.requests[i], baseTools.requests[i]
		a.WorkingDir, b.WorkingDir = "", "" // each run has its own temp dir
		if !reflect.DeepEqual(a, b) || a.Sandbox.Mode != "read-only" || !reflect.DeepEqual(a.ReadableRoots, []string{"/srv/evidence"}) || len(a.WritableRoots) != 0 {
			t.Fatalf("tool request %d changed under tracing: %+v vs %+v", i, a, b)
		}
	}

	// A succeeding run leaves no trace even when asked.
	quiet := filepath.Join(dir, "quiet.json")
	t.Setenv(RequestTraceEnv, quiet)
	ok := &fakeAPIAdapter{responses: []*agent.Response{{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "done"}}, StopReason: agent.StopReasonEnd}}}
	if r, _, err := runFixture(t, ok, &fixtureTools{}); err != nil || r.Outcome != OutcomeSucceeded {
		t.Fatalf("%+v %v", r, err)
	}
	if _, err := os.Stat(quiet); !os.IsNotExist(err) {
		t.Fatalf("a successful run left a trace: %v", err)
	}

	// Nothing secret-shaped in the conversation: the trace says it is unchanged.
	clean := filepath.Join(dir, "clean", "trace.json")
	t.Setenv(RequestTraceEnv, clean)
	provider := fixtureProvider(t, &fakeAPIAdapter{responses: append(roundsWithCalls(16, 16), draftResponse())}, &recordingToolExecutor{})
	if r, err := provider.Execute(context.Background(), Request{ID: "c", WorkingDir: dir, Prompt: "plain prompt", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}}, nil); err != nil || r.Outcome != OutcomeInconclusive {
		t.Fatalf("%+v %v", r, err)
	}
	trace := readTrace(t, clean)
	if trace["redactionChangedContent"] != false || len(asList(trace["entries"])) != 17 {
		t.Fatalf("clean trace: redactionChangedContent=%v entries=%d", trace["redactionChangedContent"], len(asList(trace["entries"])))
	}
	// A fake adapter keeps no wire body: the trace says nothing rather than inventing one.
	if _, ok := asMap(asList(trace["entries"])[0])["wire"]; ok {
		t.Fatal("a wire body appeared for an adapter that does not keep one")
	}
}

// Past the size bound later entries are dropped and the trace says so; the run
// is unaffected.
func TestRequestTraceSizeBoundIsReportedNotSilent(t *testing.T) {
	old := traceLimit
	traceLimit = 20 << 10
	defer func() { traceLimit = old }()
	server, _ := fixtureServer(t, fixtureAnswer())
	path := filepath.Join(t.TempDir(), "traces", "trace.json")
	t.Setenv(RequestTraceEnv, path)
	result, _, err := runFixture(t, compatible(t, server.URL), &fixtureTools{})
	if err != nil || result.Outcome != OutcomeInconclusive {
		t.Fatalf("%+v %v", result, err)
	}
	trace := readTrace(t, path)
	dropped := int(trace["droppedEntries"].(float64))
	if trace["incomplete"] != true || dropped == 0 || len(asList(trace["entries"]))+dropped != fixtureRounds+1 {
		t.Fatalf("incomplete=%v dropped=%d kept=%d", trace["incomplete"], dropped, len(asList(trace["entries"])))
	}
}

// Failed model requests are recorded too, with their error, so the request the
// endpoint refused is in the trace.
func TestRequestTraceKeepsTheRefusedRequest(t *testing.T) {
	adapter := &fakeAPIAdapter{responses: []*agent.Response{readCalls("a", 1)}}
	adapter.completeErrors = []error{nil, fmt.Errorf("endpoint refused: 500 for %s", fixtureSecret)}
	path := filepath.Join(t.TempDir(), "traces", "trace.json")
	t.Setenv(RequestTraceEnv, path)
	result, _, err := runFixture(t, adapter, &recordingToolExecutor{})
	if err == nil || result.Outcome != OutcomeFailed {
		t.Fatalf("%+v %v", result, err)
	}
	trace := readTrace(t, path)
	entries := asList(trace["entries"])
	if len(entries) != 2 || !strings.Contains(asString(asMap(entries[1])["error"]), "endpoint refused") || strings.Contains(asString(asMap(entries[1])["error"]), fixtureSecret) {
		t.Fatalf("entries = %v", entries)
	}
	if _, has := asMap(entries[1])["response"]; has {
		t.Fatal("a response was invented for a failed request")
	}
}
