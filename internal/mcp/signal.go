package mcp

import "fmt"

// SignalType identifies the kind of agent signal.
type SignalType string

const (
	SignalPhaseComplete    SignalType = "phase-complete"
	SignalBlocked          SignalType = "blocked"
	SignalDecisionPoint    SignalType = "decision-point"
	SignalProgress         SignalType = "progress"
	SignalPlanningMismatch SignalType = "planning-mismatch"
	SignalScopeDiscovery   SignalType = "scope-discovery"
	SignalRoute            SignalType = "route"
)

var validSignalTypes = map[SignalType]bool{
	SignalPhaseComplete:    true,
	SignalBlocked:          true,
	SignalDecisionPoint:    true,
	SignalProgress:         true,
	SignalPlanningMismatch: true,
	SignalScopeDiscovery:   true,
	SignalRoute:            true,
}

// Signal represents a structured signal from an agent via axon_signal tool.
type Signal struct {
	Type     SignalType             `json:"type"`
	Reason   string                 `json:"reason,omitempty"`
	Target   string                 `json:"target,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Validate checks that the signal has a known type.
func (s Signal) Validate() error {
	if s.Type == "" {
		return fmt.Errorf("signal type is required")
	}
	if !validSignalTypes[s.Type] {
		return fmt.Errorf("unknown signal type: %q", s.Type)
	}
	return nil
}
