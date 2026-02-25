package axontui

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/loop"
)

func TestLogViewer_AppendAndView(t *testing.T) {
	lv := NewLogViewer("FWU-001", nil)
	lv.SetSize(80, 20)

	lv.AppendEvent(loop.Event{Type: loop.EventPhaseStart, Phase: "TD", Agent: "clario"})
	lv.AppendEvent(loop.Event{Type: loop.EventIterationStart, Iteration: 1})
	lv.AppendEvent(loop.Event{Type: loop.EventAssistantText, Text: "I'll create the file..."})
	lv.AppendEvent(loop.Event{Type: loop.EventToolStart, Tool: "Write", ToolInput: map[string]interface{}{"file_path": "hello.go"}})
	lv.AppendEvent(loop.Event{Type: loop.EventToolResult, Text: "File created successfully"})
	lv.AppendEvent(loop.Event{Type: loop.EventSignalReceived, SignalType: "progress", Text: "File created"})
	lv.AppendEvent(loop.Event{Type: loop.EventPipelineComplete})

	view := lv.View()

	checks := []string{"Phase TD", "clario", "iteration 1", "I'll create the file",
		"Write", "hello.go", "PROGRESS", "Pipeline complete"}

	for _, check := range checks {
		if !strings.Contains(view, check) {
			t.Errorf("expected %q in view, got:\n%s", check, view)
		}
	}

	// Should have streaming cursor (auto-scroll on).
	if !strings.Contains(view, "▌") {
		t.Error("expected streaming cursor")
	}
}

func TestLogViewer_Scrolling(t *testing.T) {
	lv := NewLogViewer("FWU-001", nil)
	lv.SetSize(80, 5)

	// Add more lines than viewport height.
	for i := 0; i < 20; i++ {
		lv.AppendEvent(loop.Event{Type: loop.EventAssistantText, Text: "line"})
	}

	if !lv.autoScroll {
		t.Error("should start with auto-scroll on")
	}

	// Scroll up disables auto-scroll.
	lv.ScrollUp(3)
	if lv.autoScroll {
		t.Error("scroll up should disable auto-scroll")
	}

	// Go to bottom re-enables it.
	lv.GoToBottom()
	if !lv.autoScroll {
		t.Error("go to bottom should re-enable auto-scroll")
	}
	if lv.offset != 0 {
		t.Errorf("expected offset 0, got %d", lv.offset)
	}
}

func TestLogViewer_BufferLimit(t *testing.T) {
	lv := NewLogViewer("FWU-001", nil)
	lv.SetSize(80, 20)

	for i := 0; i < 11000; i++ {
		lv.AppendEvent(loop.Event{Type: loop.EventAssistantText, Text: "line"})
	}

	if len(lv.lines) > 6000 {
		t.Errorf("expected buffer trimmed, got %d lines", len(lv.lines))
	}
}

func TestRenderEvent_AllTypes(t *testing.T) {
	tests := []struct {
		name    string
		event   loop.Event
		wantSub string
	}{
		{"phase start", loop.Event{Type: loop.EventPhaseStart, Phase: "IM", Agent: "spark"}, "Phase IM"},
		{"iteration", loop.Event{Type: loop.EventIterationStart, Iteration: 3}, "iteration 3"},
		{"text", loop.Event{Type: loop.EventAssistantText, Text: "hello"}, "hello"},
		{"tool start", loop.Event{Type: loop.EventToolStart, Tool: "Read"}, "Read"},
		{"tool result", loop.Event{Type: loop.EventToolResult, Text: "ok"}, "ok"},
		{"signal progress", loop.Event{Type: loop.EventSignalReceived, SignalType: "progress", Text: "done"}, "PROGRESS"},
		{"signal route", loop.Event{Type: loop.EventSignalReceived, SignalType: "route", SignalTarget: "spark"}, "ROUTE"},
		{"route decision", loop.Event{Type: loop.EventRouteDecision, RouteTarget: "hon", Text: "approved"}, "ROUTE"},
		{"error", loop.Event{Type: loop.EventError, ErrText: "something failed"}, "ERROR"},
		{"operator", loop.Event{Type: loop.EventOperatorAttention, Text: "blocked"}, "OPERATOR"},
		{"retrying", loop.Event{Type: loop.EventRetrying, Iteration: 2}, "Retrying"},
		{"thrashing", loop.Event{Type: loop.EventThrashingDetected}, "Thrashing"},
		{"pipeline complete", loop.Event{Type: loop.EventPipelineComplete}, "Pipeline complete"},
		{"paused", loop.Event{Type: loop.EventPaused}, "Paused"},
		{"max iter", loop.Event{Type: loop.EventMaxIterationsReached}, "Max iterations"},
		{"complete", loop.Event{Type: loop.EventComplete}, "Complete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := renderEvent(tt.event)
			if !strings.Contains(result, tt.wantSub) {
				t.Errorf("expected %q in render, got: %s", tt.wantSub, result)
			}
		})
	}
}
