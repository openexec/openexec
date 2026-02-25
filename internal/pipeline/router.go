package pipeline

import "github.com/openexec/openexec/internal/loop"

// SignalAction describes what the pipeline should do in response to a signal.
type SignalAction int

const (
	ActionNone          SignalAction = iota
	ActionPhaseComplete              // phase finished normally
	ActionRoute                      // Blade routing decision
	ActionPause                      // needs operator attention
	ActionReplan                     // trigger re-planning workflow
)

// SignalResult is the outcome of interpreting a signal event.
type SignalResult struct {
	Action      SignalAction
	RouteTarget string // populated for ActionRoute
	Reason      string
}

// HandleSignal inspects a signal_received event and returns the pipeline action.
// Non-signal events return ActionNone.
func HandleSignal(event loop.Event) SignalResult {
	if event.Type != loop.EventSignalReceived {
		return SignalResult{Action: ActionNone}
	}

	switch event.SignalType {
	case "phase-complete":
		return SignalResult{
			Action: ActionPhaseComplete,
			Reason: event.Text,
		}
	case "route":
		return SignalResult{
			Action:      ActionRoute,
			RouteTarget: event.SignalTarget,
			Reason:      event.Text,
		}
	case "planning-mismatch", "scope-discovery":
		return SignalResult{
			Action: ActionReplan,
			Reason: event.Text,
		}
	case "blocked", "decision-point":
		return SignalResult{
			Action: ActionPause,
			Reason: event.Text,
		}
	default:
		// progress → no pipeline action
		return SignalResult{Action: ActionNone}
	}
}

// TerminationResult describes how a phase ended based on collected events.
type TerminationResult struct {
	PhaseCompleted bool
	RouteTarget    string // non-empty if ended via route signal
	Blocked        bool   // true if ended via blocked/decision-point
	Reason         string
}

// HandleTermination examines a slice of events to determine how a loop terminated.
func HandleTermination(events []loop.Event) TerminationResult {
	var result TerminationResult

	for _, e := range events {
		if e.Type != loop.EventSignalReceived {
			continue
		}
		switch e.SignalType {
		case "phase-complete":
			result.PhaseCompleted = true
			result.Reason = e.Text
		case "route":
			result.PhaseCompleted = true
			result.RouteTarget = e.SignalTarget
			result.Reason = e.Text
		case "blocked", "decision-point":
			result.Blocked = true
			result.Reason = e.Text
		}
	}

	return result
}
