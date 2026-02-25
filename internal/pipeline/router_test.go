package pipeline

import (
	"testing"

	"github.com/openexec/openexec/internal/loop"
)

func TestHandleSignalPhaseComplete(t *testing.T) {
	event := loop.Event{
		Type:       loop.EventSignalReceived,
		SignalType: "phase-complete",
		Text:       "All tests passing",
	}
	result := HandleSignal(event)
	if result.Action != ActionPhaseComplete {
		t.Errorf("expected ActionPhaseComplete, got %d", result.Action)
	}
	if result.Reason != "All tests passing" {
		t.Errorf("reason = %q, want %q", result.Reason, "All tests passing")
	}
}

func TestHandleSignalRoute(t *testing.T) {
	event := loop.Event{
		Type:         loop.EventSignalReceived,
		SignalType:   "route",
		SignalTarget: "spark",
		Text:         "Needs rework",
	}
	result := HandleSignal(event)
	if result.Action != ActionRoute {
		t.Errorf("expected ActionRoute, got %d", result.Action)
	}
	if result.RouteTarget != "spark" {
		t.Errorf("target = %q, want %q", result.RouteTarget, "spark")
	}
}

func TestHandleSignalBlocked(t *testing.T) {
	event := loop.Event{
		Type:       loop.EventSignalReceived,
		SignalType: "blocked",
		Text:       "Missing credentials",
	}
	result := HandleSignal(event)
	if result.Action != ActionPause {
		t.Errorf("expected ActionPause, got %d", result.Action)
	}
}

func TestHandleSignalDecisionPoint(t *testing.T) {
	event := loop.Event{
		Type:       loop.EventSignalReceived,
		SignalType: "decision-point",
		Text:       "Need operator input",
	}
	result := HandleSignal(event)
	if result.Action != ActionPause {
		t.Errorf("expected ActionPause, got %d", result.Action)
	}
}

func TestHandleSignalProgress(t *testing.T) {
	event := loop.Event{
		Type:       loop.EventSignalReceived,
		SignalType: "progress",
		Text:       "Step done",
	}
	result := HandleSignal(event)
	if result.Action != ActionNone {
		t.Errorf("expected ActionNone, got %d", result.Action)
	}
}

func TestHandleSignalPlanningMismatch(t *testing.T) {
	event := loop.Event{
		Type:       loop.EventSignalReceived,
		SignalType: "planning-mismatch",
		Text:       "Plan diverged",
	}
	result := HandleSignal(event)
	if result.Action != ActionNone {
		t.Errorf("expected ActionNone, got %d", result.Action)
	}
}

func TestHandleSignalScopeDiscovery(t *testing.T) {
	event := loop.Event{
		Type:       loop.EventSignalReceived,
		SignalType: "scope-discovery",
		Text:       "Found extra work",
	}
	result := HandleSignal(event)
	if result.Action != ActionNone {
		t.Errorf("expected ActionNone, got %d", result.Action)
	}
}

func TestHandleSignalNonSignalEvent(t *testing.T) {
	event := loop.Event{
		Type: loop.EventAssistantText,
		Text: "Hello",
	}
	result := HandleSignal(event)
	if result.Action != ActionNone {
		t.Errorf("expected ActionNone for non-signal event, got %d", result.Action)
	}
}

func TestHandleTerminationPhaseComplete(t *testing.T) {
	events := []loop.Event{
		{Type: loop.EventIterationStart},
		{Type: loop.EventAssistantText, Text: "working..."},
		{Type: loop.EventSignalReceived, SignalType: "phase-complete", Text: "Done"},
		{Type: loop.EventComplete},
	}
	result := HandleTermination(events)
	if !result.PhaseCompleted {
		t.Error("expected PhaseCompleted")
	}
	if result.RouteTarget != "" {
		t.Errorf("unexpected RouteTarget: %q", result.RouteTarget)
	}
	if result.Blocked {
		t.Error("unexpected Blocked")
	}
}

func TestHandleTerminationRoute(t *testing.T) {
	events := []loop.Event{
		{Type: loop.EventSignalReceived, SignalType: "route", SignalTarget: "hon", Text: "Approved"},
		{Type: loop.EventComplete},
	}
	result := HandleTermination(events)
	if !result.PhaseCompleted {
		t.Error("expected PhaseCompleted for route")
	}
	if result.RouteTarget != "hon" {
		t.Errorf("RouteTarget = %q, want %q", result.RouteTarget, "hon")
	}
}

func TestHandleTerminationBlocked(t *testing.T) {
	events := []loop.Event{
		{Type: loop.EventSignalReceived, SignalType: "blocked", Text: "Missing API key"},
	}
	result := HandleTermination(events)
	if result.PhaseCompleted {
		t.Error("unexpected PhaseCompleted for blocked")
	}
	if !result.Blocked {
		t.Error("expected Blocked")
	}
}

func TestHandleTerminationNoSignals(t *testing.T) {
	events := []loop.Event{
		{Type: loop.EventIterationStart},
		{Type: loop.EventAssistantText, Text: "working..."},
		{Type: loop.EventMaxIterationsReached},
	}
	result := HandleTermination(events)
	if result.PhaseCompleted {
		t.Error("unexpected PhaseCompleted with no signals")
	}
	if result.Blocked {
		t.Error("unexpected Blocked with no signals")
	}
}
