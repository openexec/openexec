package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Authored surrogate of the structural shape of native triage run 7a722
// (operator receipt, metadata only): 16 tool rounds of 2,1,1,1,2,2,1,2,1,1,2,2,
// 1,1,1,2 calls (23), a synthesis request (tools kept, tool_choice none) that
// the endpoint answered with ANOTHER tool call, then the recovery request
// (no tools) answered with a 6021-character text. No captured content is used.
// It shows what the loop does with that sequence; it says nothing about the
// configured endpoint, which was not reproduced.

var sequence7a722Rounds = []int{2, 1, 1, 1, 2, 2, 1, 2, 1, 1, 2, 2, 1, 1, 1, 2}

const sequence7a722AnswerLen = 6021

func sequence7a722Provider(t *testing.T, adapter agent.ProviderAdapter, tools ToolExecutor) *APIProvider {
	t.Helper()
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: tools,
		Tools: []agent.ToolDefinition{
			{Name: "read_file", Description: "Read a file", InputSchema: schema},
			{Name: "list_directory", Description: "List a directory", InputSchema: schema},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

// sequence7a722Server answers the scripted sequence. When recoveryAnswers is
// false the recovery request is also answered with a tool call.
func sequence7a722Server(t *testing.T, recoveryAnswers bool) *httptest.Server {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		n := int(calls.Add(1))
		toolCalls := 0
		switch {
		case n <= len(sequence7a722Rounds):
			toolCalls = sequence7a722Rounds[n-1]
		case n == len(sequence7a722Rounds)+1 || !recoveryAnswers:
			toolCalls = 1 // tool_choice none ignored by the endpoint
		}
		if toolCalls == 0 {
			body, _ := json.Marshal(strings.Repeat("x", sequence7a722AnswerLen))
			_, _ = fmt.Fprintf(w, `{"id":"r%d","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":%s},"finish_reason":"stop"}]}`, n, body)
			return
		}
		var parts []string
		for c := 0; c < toolCalls; c++ {
			parts = append(parts, fmt.Sprintf(`{"id":"call-%d-%d","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"f%d.txt\"}"}}`, n, c, n))
		}
		_, _ = fmt.Fprintf(w, `{"id":"r%d","model":"test-model","choices":[{"index":0,"message":{"role":"assistant","content":"","tool_calls":[%s]},"finish_reason":"tool_calls"}]}`, n, strings.Join(parts, ","))
	}))
	t.Cleanup(server.Close)
	return server
}

func runSequence7a722(t *testing.T, adapter agent.ProviderAdapter, tools ToolExecutor) (Result, []Event, error) {
	t.Helper()
	var events []Event
	result, err := sequence7a722Provider(t, adapter, tools).Execute(context.Background(), fixtureRequest(t),
		func(event Event) error { events = append(events, event); return nil })
	return result, events, err
}

func TestSequence7a722ShapeEndsMaxTurnsWithTheRecoveryTextAndReplaysOffline(t *testing.T) {
	server := sequence7a722Server(t, true)
	path := filepath.Join(t.TempDir(), "traces", "trace.json")
	t.Setenv(RequestTraceEnv, path)
	result, events, err := runSequence7a722(t, compatible(t, server.URL), &fixtureTools{})
	if err != nil || result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got := len([]rune(result.FinalText)); got != sequence7a722AnswerLen {
		t.Fatalf("final text = %d characters, want %d", got, sequence7a722AnswerLen)
	}
	trace := readTrace(t, path)
	t.Setenv(RequestTraceEnv, "")
	entries := asList(trace["entries"])
	if len(entries) != 18 {
		t.Fatalf("entries = %d, want 18", len(entries))
	}
	for i, e := range entries[:17] {
		req := entryRequest(e)
		want := "auto"
		if i == 16 {
			want = "none"
		}
		if len(asList(req["tools"])) != 2 || req["tool_choice"] != want {
			t.Fatalf("entry %d: tools=%d choice=%v", i+1, len(asList(req["tools"])), req["tool_choice"])
		}
	}
	// Entry 17 is the synthesis request: answered with a tool call that is not run.
	resp := asMap(asMap(entries[16])["response"])
	if len(asList(resp["content"])) != 1 || asString(asMap(asList(resp["content"])[0])["type"]) != "tool_use" {
		t.Fatalf("entry 17 response = %v", resp)
	}
	// Entry 18 is the recovery: no tools on the request or on the wire.
	recovery := asMap(entries[17])
	if len(asList(entryRequest(recovery)["tools"])) != 0 || entryRequest(recovery)["tool_choice"] != "none" || len(asList(asMap(recovery["wire"])["tools"])) != 0 {
		t.Fatalf("recovery request = %v", entryRequest(recovery)["tool_choice"])
	}
	counts := map[string]int{}
	var types []string
	for _, e := range asList(trace["events"]) {
		ty := asString(asMap(e)["type"])
		counts[ty]++
		types = append(types, ty)
	}
	if counts[EventToolProposed] != 23 || counts[EventToolStarted] != 23 || counts[EventToolCompleted] != 23 || counts[EventAssistantDelta] != 1 || counts[EventInconclusive] != 1 || counts[EventCompleted] != 0 {
		t.Fatalf("event counts = %v", counts)
	}
	if n := len(types); types[n-1] != EventInconclusive || types[n-2] != EventAssistantDelta || types[n-3] != EventToolCompleted {
		t.Fatalf("terminal events = %v", types[len(types)-4:])
	}

	// Offline replay through the existing consumer: the loop re-derives the
	// identical 18 requests and the same ending.
	outputs := map[string]string{}
	for _, e := range asList(trace["events"]) {
		if event := asMap(e); event["type"] == EventToolCompleted {
			outputs[asString(event["call_id"])] = asString(event["text"])
		}
	}
	replay := &traceReplayAdapter{t: t, entries: entries}
	replayed, replayEvents, err := runSequence7a722(t, replay, &recordedTools{outputs: outputs})
	if err != nil || replay.next != 18 || replayed.Outcome != result.Outcome || replayed.Reason != result.Reason || replayed.FinalText != result.FinalText {
		t.Fatalf("replay err=%v requests=%d result=%+v", err, replay.next, replayed)
	}
	if !reflect.DeepEqual(eventTypes(replayEvents), eventTypes(events)) {
		t.Fatalf("replayed events differ")
	}
}

// Near miss: when the recovery request is also answered with a tool call there
// is no answer, so the run must not end as the inconclusive-with-text case.
func TestSequence7a722ShapeWithoutARecoveryAnswerDoesNotEndWithText(t *testing.T) {
	server := sequence7a722Server(t, false)
	result, events, _ := runSequence7a722(t, compatible(t, server.URL), &fixtureTools{})
	for _, e := range events {
		if e.Type == EventAssistantDelta {
			t.Fatalf("an answer was emitted without a recovery answer: %q", e.Text)
		}
	}
	if strings.Count(result.FinalText, "x") == sequence7a722AnswerLen || result.Outcome == OutcomeSucceeded {
		t.Fatalf("unexpected result: %+v", result.Outcome)
	}
}
