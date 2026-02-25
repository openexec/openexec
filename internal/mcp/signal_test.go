package mcp

import "testing"

func TestSignalValidateKnownTypes(t *testing.T) {
	for _, st := range []SignalType{
		SignalPhaseComplete, SignalBlocked, SignalDecisionPoint,
		SignalProgress, SignalPlanningMismatch, SignalScopeDiscovery, SignalRoute,
	} {
		s := Signal{Type: st}
		if err := s.Validate(); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", st, err)
		}
	}
}

func TestSignalValidateEmpty(t *testing.T) {
	s := Signal{}
	if err := s.Validate(); err == nil {
		t.Error("Validate empty type: want error, got nil")
	}
}

func TestSignalValidateUnknown(t *testing.T) {
	s := Signal{Type: "unknown-type"}
	if err := s.Validate(); err == nil {
		t.Error("Validate unknown type: want error, got nil")
	}
}
