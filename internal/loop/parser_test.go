package loop

import (
	"strings"
	"testing"
)

func collectEvents(input string, iteration int) []Event {
	ch := make(chan Event, 64)
	p := NewParser(ch, iteration)
	p.Parse(strings.NewReader(input))
	close(ch)
	var events []Event
	for e := range ch {
		events = append(events, e)
	}
	return events
}

func TestParseCompleteSession(t *testing.T) {
	input := `{"type":"system","subtype":"init","session_id":"abc123"}
{"type":"assistant","message":{"content":[{"type":"text","text":"I'll create a file."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Write","input":{"file_path":"hello.go","content":"package main"}}]}}
{"type":"tool_result","tool_use_id":"tu1","content":"File created"}
{"type":"result","result":{"content":[{"type":"text","text":"Done."}]}}
`
	events := collectEvents(input, 1)

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d: %+v", len(events), events)
	}

	// Event 0: assistant text
	if events[0].Type != EventAssistantText {
		t.Errorf("event[0] type = %q, want %q", events[0].Type, EventAssistantText)
	}
	if events[0].Text != "I'll create a file." {
		t.Errorf("event[0] text = %q", events[0].Text)
	}
	if events[0].Iteration != 1 {
		t.Errorf("event[0] iteration = %d, want 1", events[0].Iteration)
	}

	// Event 1: tool start
	if events[1].Type != EventToolStart {
		t.Errorf("event[1] type = %q, want %q", events[1].Type, EventToolStart)
	}
	if events[1].Tool != "Write" {
		t.Errorf("event[1] tool = %q", events[1].Tool)
	}
	if events[1].ToolInput["file_path"] != "hello.go" {
		t.Errorf("event[1] tool_input = %v", events[1].ToolInput)
	}

	// Event 2: tool result
	if events[2].Type != EventToolResult {
		t.Errorf("event[2] type = %q, want %q", events[2].Type, EventToolResult)
	}
	if events[2].Text != "File created" {
		t.Errorf("event[2] text = %q", events[2].Text)
	}
}

func TestParseMixedContent(t *testing.T) {
	// A single assistant message with both text and tool_use in content array.
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me write that."},{"type":"tool_use","id":"tu1","name":"Read","input":{"file_path":"main.go"}}]}}
`
	events := collectEvents(input, 2)

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d: %+v", len(events), events)
	}
	if events[0].Type != EventAssistantText {
		t.Errorf("event[0] type = %q", events[0].Type)
	}
	if events[1].Type != EventToolStart {
		t.Errorf("event[1] type = %q", events[1].Type)
	}
	if events[1].Tool != "Read" {
		t.Errorf("event[1] tool = %q", events[1].Tool)
	}
	if events[0].Iteration != 2 || events[1].Iteration != 2 {
		t.Error("expected iteration 2 on both events")
	}
}

func TestParseMalformedJSON(t *testing.T) {
	input := `not json at all
{"type":"assistant","message":{"content":[{"type":"text","text":"ok"}]}}
{broken
`
	events := collectEvents(input, 1)

	if len(events) != 1 {
		t.Fatalf("expected 1 event (skip malformed), got %d: %+v", len(events), events)
	}
	if events[0].Text != "ok" {
		t.Errorf("event text = %q", events[0].Text)
	}
}

func TestParseEmptyInput(t *testing.T) {
	events := collectEvents("", 1)
	if len(events) != 0 {
		t.Fatalf("expected 0 events, got %d", len(events))
	}
}

func TestParseToolResultStructuredContent(t *testing.T) {
	// tool_result with array content instead of string.
	input := `{"type":"tool_result","tool_use_id":"tu1","content":[{"type":"text","text":"structured"}]}
`
	events := collectEvents(input, 1)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolResult {
		t.Errorf("type = %q", events[0].Type)
	}
	// Structured content is stringified.
	if events[0].Text == "" {
		t.Error("expected non-empty text for structured content")
	}
}

func TestParseBlankLinesSkipped(t *testing.T) {
	input := "\n\n{\"type\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"hi\"}]}}\n\n"
	events := collectEvents(input, 1)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
}

func TestParseUnrecognizedType(t *testing.T) {
	input := `{"type":"unknown_future_type","data":"something"}
{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}
`
	events := collectEvents(input, 1)

	if len(events) != 1 {
		t.Fatalf("expected 1 event (skip unknown), got %d", len(events))
	}
}

// --- Signal detection tests (V2) ---

func TestParseAxonSignalMCPPrefixed(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__axon_signal","input":{"type":"phase-complete","reason":"All tests passing"}}]}}
{"type":"tool_result","tool_use_id":"tu1","content":"Signal received: phase-complete"}
`
	events := collectEvents(input, 1)

	if len(events) != 2 {
		t.Fatalf("expected 2 events (signal + tool_result), got %d: %+v", len(events), events)
	}

	sig := events[0]
	if sig.Type != EventSignalReceived {
		t.Errorf("event[0] type = %q, want %q", sig.Type, EventSignalReceived)
	}
	if sig.SignalType != "phase-complete" {
		t.Errorf("signal_type = %q, want 'phase-complete'", sig.SignalType)
	}
	if sig.Text != "All tests passing" {
		t.Errorf("text (reason) = %q", sig.Text)
	}
	if sig.Iteration != 1 {
		t.Errorf("iteration = %d, want 1", sig.Iteration)
	}
}

func TestParseAxonSignalDirect(t *testing.T) {
	// Tool name without MCP prefix (edge case).
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"axon_signal","input":{"type":"progress","reason":"Step done"}}]}}
`
	events := collectEvents(input, 2)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventSignalReceived {
		t.Errorf("type = %q", events[0].Type)
	}
	if events[0].SignalType != "progress" {
		t.Errorf("signal_type = %q", events[0].SignalType)
	}
}

func TestParseNonSignalToolStillWorks(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Write","input":{"file_path":"test.go"}}]}}
`
	events := collectEvents(input, 1)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != EventToolStart {
		t.Errorf("type = %q, want %q", events[0].Type, EventToolStart)
	}
	if events[0].Tool != "Write" {
		t.Errorf("tool = %q", events[0].Tool)
	}
}

func TestParseMixedToolsAndSignal(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Working on it."}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"Write","input":{"file_path":"test.go"}}]}}
{"type":"tool_result","tool_use_id":"tu1","content":"File created"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu2","name":"mcp__axon-signal__axon_signal","input":{"type":"phase-complete","reason":"Done"}}]}}
{"type":"tool_result","tool_use_id":"tu2","content":"Signal received: phase-complete"}
`
	events := collectEvents(input, 1)

	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d: %+v", len(events), events)
	}

	if events[0].Type != EventAssistantText {
		t.Errorf("[0] type = %q", events[0].Type)
	}
	if events[1].Type != EventToolStart {
		t.Errorf("[1] type = %q", events[1].Type)
	}
	if events[2].Type != EventToolResult {
		t.Errorf("[2] type = %q", events[2].Type)
	}
	if events[3].Type != EventSignalReceived {
		t.Errorf("[3] type = %q", events[3].Type)
	}
	if events[4].Type != EventToolResult {
		t.Errorf("[4] type = %q", events[4].Type)
	}
}

func TestParseSignalWithTarget(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__axon_signal","input":{"type":"route","target":"spark","reason":"Test failures found"}}]}}
`
	events := collectEvents(input, 1)

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].SignalType != "route" {
		t.Errorf("signal_type = %q", events[0].SignalType)
	}
	if events[0].SignalTarget != "spark" {
		t.Errorf("signal_target = %q", events[0].SignalTarget)
	}
	if events[0].Text != "Test failures found" {
		t.Errorf("text = %q", events[0].Text)
	}
}

func TestParseSignalUpdatesTracker(t *testing.T) {
	tracker := NewSignalTracker(3)
	ch := make(chan Event, 64)
	p := NewParser(ch, 1)
	p.tracker = tracker

	input := `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"tu1","name":"mcp__axon-signal__axon_signal","input":{"type":"phase-complete","reason":"Done"}}]}}
`
	p.Parse(strings.NewReader(input))
	close(ch)

	if !tracker.PhaseComplete() {
		t.Error("expected tracker.PhaseComplete() = true")
	}
}
