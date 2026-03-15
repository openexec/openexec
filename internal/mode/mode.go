// Package mode defines the three operational modes for OpenExec sessions.
// This follows the converged architecture pattern where:
// - Chat: Conversational, no side effects (read-only exploration)
// - Task: Scoped action producing artifacts (files/patches)
// - Run: Blueprint execution with full automation
package mode

import (
	"fmt"
	"time"
)

// Mode represents the operational mode of a session.
type Mode string

const (
	// ModeChat is conversational mode with no side effects.
	// The agent can read files, search, and answer questions but cannot modify anything.
	ModeChat Mode = "chat"

	// ModeTask is scoped action mode that produces artifacts.
	// The agent can read and write files, run commands, but within a bounded scope.
	ModeTask Mode = "task"

	// ModeRun is blueprint execution mode with full automation.
	// The agent follows a defined blueprint with stages and can work unattended.
	ModeRun Mode = "run"
)

// ValidModes contains all valid mode values.
var ValidModes = []Mode{ModeChat, ModeTask, ModeRun}

// IsValid checks if the mode is a valid mode value.
func (m Mode) IsValid() bool {
	for _, valid := range ValidModes {
		if m == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the mode.
func (m Mode) String() string {
	return string(m)
}

// Description returns a human-readable description of the mode.
func (m Mode) Description() string {
	switch m {
	case ModeChat:
		return "Conversational mode - read-only exploration, no side effects"
	case ModeTask:
		return "Task mode - scoped actions producing files and patches"
	case ModeRun:
		return "Run mode - blueprint execution with full automation"
	default:
		return "Unknown mode"
	}
}

// AllowsWrites returns true if the mode allows write operations.
func (m Mode) AllowsWrites() bool {
	return m == ModeTask || m == ModeRun
}

// AllowsExec returns true if the mode allows command execution.
func (m Mode) AllowsExec() bool {
	return m == ModeTask || m == ModeRun
}

// RequiresApproval returns true if write/exec operations require user approval in this mode.
func (m Mode) RequiresApproval() bool {
	// Task mode requires approval for each action
	// Run mode operates with pre-approved blueprint
	return m == ModeTask
}

// TransitionCondition describes why a mode transition occurred.
type TransitionCondition string

const (
	// ConditionUserApproved indicates the user explicitly approved the transition.
	ConditionUserApproved TransitionCondition = "user_approved"

	// ConditionInputsReady indicates all required inputs are available.
	ConditionInputsReady TransitionCondition = "inputs_ready"

	// ConditionCheckpoint indicates a checkpoint was reached in blueprint execution.
	ConditionCheckpoint TransitionCondition = "checkpoint"

	// ConditionCompleted indicates the task/run completed successfully.
	ConditionCompleted TransitionCondition = "completed"

	// ConditionFailed indicates the task/run failed.
	ConditionFailed TransitionCondition = "failed"

	// ConditionUserCancelled indicates the user cancelled the operation.
	ConditionUserCancelled TransitionCondition = "user_cancelled"

	// ConditionTimeout indicates an operation timed out.
	ConditionTimeout TransitionCondition = "timeout"
)

// Transition represents a mode transition event.
type Transition struct {
	// From is the previous mode.
	From Mode `json:"from"`

	// To is the new mode.
	To Mode `json:"to"`

	// Condition describes why the transition occurred.
	Condition TransitionCondition `json:"condition"`

	// Reason provides additional context for the transition.
	Reason string `json:"reason,omitempty"`

	// At is when the transition occurred.
	At time.Time `json:"at"`

	// TriggeredBy identifies what triggered the transition (user, system, blueprint).
	TriggeredBy string `json:"triggered_by,omitempty"`
}

// NewTransition creates a new mode transition.
func NewTransition(from, to Mode, condition TransitionCondition) *Transition {
	return &Transition{
		From:      from,
		To:        to,
		Condition: condition,
		At:        time.Now().UTC(),
	}
}

// WithReason adds a reason to the transition.
func (t *Transition) WithReason(reason string) *Transition {
	t.Reason = reason
	return t
}

// WithTriggeredBy sets who triggered the transition.
func (t *Transition) WithTriggeredBy(triggeredBy string) *Transition {
	t.TriggeredBy = triggeredBy
	return t
}

// ValidTransitions defines the allowed mode transitions.
// Key is the current mode, value is the list of modes it can transition to.
var ValidTransitions = map[Mode][]Mode{
	ModeChat: {ModeTask, ModeRun}, // Chat can escalate to Task or Run
	ModeTask: {ModeChat, ModeRun}, // Task can return to Chat or escalate to Run
	ModeRun:  {ModeChat, ModeTask}, // Run can return to Chat or Task (on checkpoint/completion)
}

// CanTransition checks if a transition from one mode to another is allowed.
func CanTransition(from, to Mode) bool {
	allowed, ok := ValidTransitions[from]
	if !ok {
		return false
	}
	for _, m := range allowed {
		if m == to {
			return true
		}
	}
	return false
}

// ValidateTransition checks if a mode transition is valid and returns an error if not.
func ValidateTransition(from, to Mode, condition TransitionCondition) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source mode: %s", from)
	}
	if !to.IsValid() {
		return fmt.Errorf("invalid target mode: %s", to)
	}
	if from == to {
		return fmt.Errorf("no-op transition: already in mode %s", from)
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid transition: %s -> %s not allowed", from, to)
	}

	// Validate condition for specific transitions
	switch {
	case from == ModeChat && to == ModeTask:
		if condition != ConditionUserApproved {
			return fmt.Errorf("chat -> task requires user_approved condition, got %s", condition)
		}
	case from == ModeChat && to == ModeRun:
		if condition != ConditionUserApproved && condition != ConditionInputsReady {
			return fmt.Errorf("chat -> run requires user_approved or inputs_ready condition, got %s", condition)
		}
	case from == ModeTask && to == ModeRun:
		if condition != ConditionInputsReady && condition != ConditionUserApproved {
			return fmt.Errorf("task -> run requires inputs_ready or user_approved condition, got %s", condition)
		}
	case from == ModeRun && (to == ModeChat || to == ModeTask):
		validConditions := []TransitionCondition{ConditionCheckpoint, ConditionCompleted, ConditionFailed, ConditionUserCancelled, ConditionTimeout}
		valid := false
		for _, c := range validConditions {
			if condition == c {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("run -> %s requires checkpoint, completed, failed, user_cancelled, or timeout condition, got %s", to, condition)
		}
	}

	return nil
}

// State tracks the current mode and transition history for a session.
type State struct {
	// Current is the current operational mode.
	Current Mode `json:"current"`

	// History is the list of mode transitions.
	History []Transition `json:"history,omitempty"`

	// EnteredAt is when the current mode was entered.
	EnteredAt time.Time `json:"entered_at"`
}

// NewState creates a new mode state starting in chat mode.
func NewState() *State {
	return &State{
		Current:   ModeChat,
		History:   make([]Transition, 0),
		EnteredAt: time.Now().UTC(),
	}
}

// TransitionTo transitions to a new mode if valid.
func (s *State) TransitionTo(to Mode, condition TransitionCondition, reason string) error {
	if err := ValidateTransition(s.Current, to, condition); err != nil {
		return err
	}

	transition := Transition{
		From:      s.Current,
		To:        to,
		Condition: condition,
		Reason:    reason,
		At:        time.Now().UTC(),
	}

	s.History = append(s.History, transition)
	s.Current = to
	s.EnteredAt = transition.At

	return nil
}

// LastTransition returns the most recent transition, or nil if no transitions have occurred.
func (s *State) LastTransition() *Transition {
	if len(s.History) == 0 {
		return nil
	}
	return &s.History[len(s.History)-1]
}

// TimeInCurrentMode returns how long the session has been in the current mode.
func (s *State) TimeInCurrentMode() time.Duration {
	return time.Since(s.EnteredAt)
}
