package execution

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// The offline structure reader for a request trace written under
// RequestTraceEnv. It is the operator command for a private trace:
//
//	go test ./pkg/execution/ -run TestTraceStructureReader -v -count=1 -args -trace-structure=/path/to/trace.json
//
// It reads the file, prints one line per request/response entry (tool counts,
// tool_choice, response block types, stop reason, text length), the event
// order, the outcome, and an offline replay verdict through traceReplayAdapter.
// It never prints message text, tool arguments, tool output, system prompt or
// any other content, and it contacts no endpoint: the replay serves the
// RECORDED responses back, so it shows whether OpenExec's loop re-derives the
// same request sequence, not how any model or endpoint behaves.
var traceStructurePath = flag.String("trace-structure", "", "path of a request trace to summarize structurally")

// traceStructure returns the content-free report lines for a decoded trace.
func traceStructure(trace map[string]any) []string {
	var out []string
	entries := asList(trace["entries"])
	for i, e := range entries {
		entry := asMap(e)
		request, wire, response := entryRequest(e), asMap(entry["wire"]), asMap(entry["response"])
		blocks := map[string]int{}
		textChars := 0
		for _, b := range asList(response["content"]) {
			block := asMap(b)
			blocks[asString(block["type"])]++
			textChars += len([]rune(asString(block["text"])))
		}
		wireChoice := "null"
		if c, ok := wire["tool_choice"].(string); ok {
			wireChoice = c
		}
		out = append(out, fmt.Sprintf("entry %d: requestTools=%d wireTools=%d requestToolChoice=%v wireToolChoice=%s roles=%s blocks=%s textChars=%d stopReason=%v",
			i+1, len(asList(request["tools"])), len(asList(wire["tools"])), request["tool_choice"], wireChoice,
			roles(request), countsString(blocks), textChars, response["stop_reason"]))
	}
	counts := map[string]int{}
	var order []string
	for _, e := range asList(trace["events"]) {
		ty := asString(asMap(e)["type"])
		counts[ty]++
		order = append(order, ty)
	}
	tail := order
	if len(tail) > 6 {
		tail = tail[len(tail)-6:]
	}
	out = append(out,
		fmt.Sprintf("events=%d byType=%s terminalEventTypes=%s", len(order), countsString(counts), strings.Join(tail, ",")),
		fmt.Sprintf("outcome=%v reason=%v finalTextChars=%d incomplete=%v droppedEntries=%v redactionChangedContent=%v imagesOmitted=%v",
			trace["outcome"], trace["reason"], len([]rune(asString(trace["finalText"]))), trace["incomplete"],
			trace["droppedEntries"], trace["redactionChangedContent"], trace["imagesOmitted"]))
	return out
}

func countsString(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s:%d", k, counts[k]))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// divergenceAdapter is traceReplayAdapter without the testing hooks, so the
// reader can report WHICH top-level fields of a request differ (names only)
// instead of failing the test.
type divergenceAdapter struct {
	fakeAPIAdapter
	entries []any
	next    int
	diverge string
}

func (a *divergenceAdapter) Complete(_ context.Context, request agent.Request) (*agent.Response, error) {
	if a.next >= len(a.entries) {
		return nil, fmt.Errorf("replay asked for request %d, trace has %d", a.next+1, len(a.entries))
	}
	entry := asMap(a.entries[a.next])
	a.next++
	raw, _ := json.Marshal(request)
	var tree map[string]any
	_ = json.Unmarshal(raw, &tree)
	got, _ := sanitizeCapture(tree, 1<<30)
	want := asMap(entry["request"])
	if !reflect.DeepEqual(got, want) {
		var fields []string
		for k := range got {
			if !reflect.DeepEqual(got[k], want[k]) {
				fields = append(fields, k)
			}
		}
		for k := range want {
			if _, ok := got[k]; !ok {
				fields = append(fields, k)
			}
		}
		sort.Strings(fields)
		a.diverge = fmt.Sprintf("request %d differs in fields %v", a.next, fields)
		return nil, fmt.Errorf("%s", a.diverge)
	}
	raw, _ = json.Marshal(entry["response"])
	var response agent.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	return &response, nil
}

// traceReplayVerdict re-runs the real loop over the recorded responses and
// returns a content-free verdict line.
func traceReplayVerdict(trace map[string]any) string {
	entries := asList(trace["entries"])
	if len(entries) == 0 {
		return "replay: no entries"
	}
	first := entryRequest(entries[0])
	raw, _ := json.Marshal(first)
	var recorded agent.Request
	if err := json.Unmarshal(raw, &recorded); err != nil || len(recorded.Messages) == 0 {
		return "replay: first request not decodable"
	}
	prompt := ""
	for _, b := range recorded.Messages[0].Content {
		prompt += b.Text
	}
	outputs := map[string]string{}
	for _, e := range asList(trace["events"]) {
		if event := asMap(e); event["type"] == EventToolCompleted {
			outputs[asString(event["call_id"])] = asString(event["text"])
		}
	}
	adapter := &divergenceAdapter{entries: entries}
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, ToolExecutor: &recordedTools{outputs: outputs}, Tools: recorded.Tools})
	if err != nil {
		return "replay: provider not constructible"
	}
	var types []string
	result, err := provider.Execute(context.Background(), Request{ID: "trace-structure", WorkingDir: os.TempDir(), Prompt: prompt,
		Model: recorded.Model, System: recorded.System, Sandbox: Sandbox{Mode: "read-only"}},
		func(event Event) error { types = append(types, event.Type); return nil })
	if err != nil {
		return fmt.Sprintf("replay: stopped after %d of %d requests: %s", adapter.next, len(entries), adapter.diverge)
	}
	return fmt.Sprintf("replay: requests=%d of %d outcome=%s reason=%s finalTextChars=%d eventsMatchTrace=%v",
		adapter.next, len(entries), result.Outcome, result.Reason, len([]rune(result.FinalText)),
		len(types) == len(asList(trace["events"])))
}

// TestTraceStructureReader is skipped unless -trace-structure is given.
func TestTraceStructureReader(t *testing.T) {
	if *traceStructurePath == "" {
		t.Skip("pass -args -trace-structure=<trace.json> to summarize a private trace")
	}
	raw, err := os.ReadFile(*traceStructurePath)
	if err != nil {
		t.Fatalf("cannot read the trace: %v", err)
	}
	var trace map[string]any
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatalf("the file is not a request trace: %v", err)
	}
	for _, line := range append(traceStructure(trace), traceReplayVerdict(trace)) {
		fmt.Println(line)
	}
}

// The reader's own contract, on authored surrogate data: the report states the
// 7a722 structure, reports the replay, and leaks no content (near miss: a
// distinctive prompt, system and tool-output marker must not appear).
func TestTraceStructureReportIsContentFree(t *testing.T) {
	const marker = "CONTENT-MARKER-MUST-NOT-APPEAR"
	server := sequence7a722Server(t, true)
	path := filepath.Join(t.TempDir(), "traces", "trace.json")
	t.Setenv(RequestTraceEnv, path)
	req := fixtureRequest(t)
	req.Prompt, req.System = "triage "+marker, "system "+marker
	var events []Event
	if _, err := sequence7a722Provider(t, compatible(t, server.URL), &fixtureTools{}).Execute(context.Background(), req,
		func(e Event) error { events = append(events, e); return nil }); err != nil {
		t.Fatal(err)
	}
	trace := readTrace(t, path)
	t.Setenv(RequestTraceEnv, "")

	lines := traceStructure(trace)
	if len(lines) != 18+2 {
		t.Fatalf("report has %d lines", len(lines))
	}
	if !strings.Contains(lines[16], "requestToolChoice=none") || !strings.Contains(lines[16], "blocks={tool_use:1}") || !strings.Contains(lines[16], "wireTools=2") {
		t.Fatalf("entry 17: %s", lines[16])
	}
	if !strings.Contains(lines[17], "requestTools=0 wireTools=0") || !strings.Contains(lines[17], "wireToolChoice=null") || !strings.Contains(lines[17], "blocks={text:1} textChars=6021") {
		t.Fatalf("entry 18: %s", lines[17])
	}
	if !strings.Contains(lines[18], "tool.completed:23") || !strings.HasSuffix(lines[18][strings.Index(lines[18], "terminalEventTypes="):], "assistant.delta,execution.inconclusive") {
		t.Fatalf("events line: %s", lines[18])
	}
	verdict := traceReplayVerdict(trace)
	if !strings.Contains(verdict, "requests=18 of 18 outcome=inconclusive reason=max_turns finalTextChars=6021 eventsMatchTrace=true") {
		t.Fatalf("verdict: %s", verdict)
	}
	for _, line := range append(lines, verdict) {
		if strings.Contains(line, marker) || strings.Contains(line, "f1.txt") || strings.Contains(line, "evidence line") {
			t.Fatalf("content reached the report: %s", line)
		}
	}

	// Near miss: a trace whose request 5 was edited is reported as a divergence
	// naming the field, not as a replay success, and still without content.
	entryRequest(asList(trace["entries"])[4])["system"] = "edited " + marker
	diverged := traceReplayVerdict(trace)
	if !strings.Contains(diverged, "stopped after 5 of 18 requests: request 5 differs in fields [system]") || strings.Contains(diverged, marker) {
		t.Fatalf("diverged verdict: %s", diverged)
	}
}
