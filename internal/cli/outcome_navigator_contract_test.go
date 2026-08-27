//go:build outcome_navigator_contract

package cli

import (
	"errors"
	"testing"

	"github.com/openexec/openexec/pkg/execution"
)

func TestOutcomeNavigatorContractTerminalProtocol(t *testing.T) {
	valid := executionTerminalReducer{terminals: []execution.Event{{
		Type: execution.EventInconclusive, Reason: execution.ReasonMaxTurns,
	}}}
	result := execution.Result{Outcome: execution.OutcomeInconclusive, Reason: execution.ReasonMaxTurns}
	if event := valid.reduce(&result, nil); event.Type != execution.EventInconclusive || event.Reason != execution.ReasonMaxTurns {
		t.Fatalf("valid terminal = %+v, result=%+v", event, result)
	}

	tests := []struct {
		name      string
		terminals []execution.Event
		result    execution.Result
		err       error
	}{
		{name: "missing terminal", result: execution.Result{Outcome: execution.OutcomeSucceeded}},
		{name: "duplicate terminal", terminals: []execution.Event{{Type: execution.EventCompleted}, {Type: execution.EventCompleted}}, result: execution.Result{Outcome: execution.OutcomeSucceeded}},
		{name: "contradictory terminal", terminals: []execution.Event{{Type: execution.EventCompleted}}, result: execution.Result{Outcome: execution.OutcomeInconclusive, Reason: execution.ReasonMaxTurns}},
		{name: "invalid reason", terminals: []execution.Event{{Type: execution.EventInconclusive, Reason: "invented"}}, result: execution.Result{Outcome: execution.OutcomeInconclusive, Reason: "invented"}},
		{name: "success with error", terminals: []execution.Event{{Type: execution.EventCompleted}}, result: execution.Result{Outcome: execution.OutcomeSucceeded}, err: errors.New("late failure")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := test.result
			event := (executionTerminalReducer{terminals: test.terminals}).reduce(&result, test.err)
			if result.Outcome != execution.OutcomeInconclusive || result.Reason != execution.ReasonProtocolError ||
				event.Type != execution.EventInconclusive || event.Reason != execution.ReasonProtocolError {
				t.Fatalf("event=%+v result=%+v", event, result)
			}
		})
	}
}
