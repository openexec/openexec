package prompt

import (
	"strings"
	"testing"
)

func TestSignalProtocol_NonEmpty(t *testing.T) {
	result := SignalProtocol()
	if result == "" {
		t.Fatal("SignalProtocol() returned empty string")
	}
}

func TestSignalProtocol_MentionsAllSignalTypes(t *testing.T) {
	result := SignalProtocol()
	signalTypes := []string{
		"progress",
		"phase-complete",
		"blocked",
		"decision-point",
		"planning-mismatch",
		"scope-discovery",
		"route",
	}
	for _, st := range signalTypes {
		if !strings.Contains(result, st) {
			t.Errorf("SignalProtocol() does not mention signal type %q", st)
		}
	}
}

func TestConsultProtocol_NonEmpty(t *testing.T) {
	result := ConsultProtocol()
	if result == "" {
		t.Fatal("ConsultProtocol() returned empty string")
	}
}

func TestConsultProtocol_MentionsAllTiers(t *testing.T) {
	result := ConsultProtocol()
	tiers := []string{
		"Tier 1",
		"Tier 2",
		"Tier 3",
		"Self-Resolve",
		"Subagent",
		"Escalation",
	}
	for _, tier := range tiers {
		if !strings.Contains(result, tier) {
			t.Errorf("ConsultProtocol() does not mention %q", tier)
		}
	}
}
