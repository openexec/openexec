package prompt

import (
	"strings"
	"testing"
)

func TestComposeValidAgentWorkflow(t *testing.T) {
	asm := NewAssembler("testdata")

	briefing := "## FWU Briefing: FWU-001 — Test Feature\n\n**Status:** in_progress\n**Intent:** Test intent"
	result, err := asm.Compose("clario", "technical-design", briefing)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	// Verify persona fields are present.
	if !strings.Contains(result, "**Role:**") {
		t.Error("prompt missing Role field")
	}
	if !strings.Contains(result, "**Identity:**") {
		t.Error("prompt missing Identity field")
	}
	if !strings.Contains(result, "**Communication Style:**") {
		t.Error("prompt missing Communication Style field")
	}
	if !strings.Contains(result, "**Principles:**") {
		t.Error("prompt missing Principles field")
	}

	// Verify workflow content is present.
	if !strings.Contains(result, "Execute full technical design workflow") {
		t.Error("prompt missing workflow instructions")
	}
	if !strings.Contains(result, "gap analysis") {
		t.Error("prompt missing workflow process")
	}

	// Verify briefing is present.
	if !strings.Contains(result, "FWU Briefing: FWU-001") {
		t.Error("prompt missing briefing")
	}

	// Verify protocols are present.
	if !strings.Contains(result, "OpenExec Signal Protocol") {
		t.Error("prompt missing signal protocol")
	}
	if !strings.Contains(result, "Consultation Protocol") {
		t.Error("prompt missing consult protocol")
	}
}

func TestComposeUnknownAgent(t *testing.T) {
	asm := NewAssembler("testdata")

	_, err := asm.Compose("nonexistent", "technical-design", "")
	if err == nil {
		t.Error("expected error for unknown agent, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should mention agent name, got: %v", err)
	}
}

func TestComposeUnknownWorkflow(t *testing.T) {
	asm := NewAssembler("testdata")

	_, err := asm.Compose("clario", "nonexistent-workflow", "")
	if err == nil {
		t.Error("expected error for unknown workflow, got nil")
	}
	if !strings.Contains(err.Error(), "nonexistent-workflow") {
		t.Errorf("error should mention workflow name, got: %v", err)
	}
}

func TestComposeEmptyBriefing(t *testing.T) {
	asm := NewAssembler("testdata")

	result, err := asm.Compose("clario", "technical-design", "")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	// Prompt should still be valid with protocols present.
	if !strings.Contains(result, "OpenExec Signal Protocol") {
		t.Error("prompt missing signal protocol with empty briefing")
	}
	if !strings.Contains(result, "Consultation Protocol") {
		t.Error("prompt missing consult protocol with empty briefing")
	}

	// Persona and workflow should still be present.
	if !strings.Contains(result, "## Persona") {
		t.Error("prompt missing persona section with empty briefing")
	}
	if !strings.Contains(result, "## Workflow") {
		t.Error("prompt missing workflow section with empty briefing")
	}
}

func TestComposeSectionOrdering(t *testing.T) {
	asm := NewAssembler("testdata")

	result, err := asm.Compose("clario", "technical-design", "## FWU Briefing: test")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	personaIdx := strings.Index(result, "## Persona")
	workflowIdx := strings.Index(result, "## Workflow")
	signalIdx := strings.Index(result, "## OpenExec Signal Protocol")
	consultIdx := strings.Index(result, "## Consultation Protocol")

	if personaIdx < 0 {
		t.Fatal("Persona section not found")
	}
	if workflowIdx < 0 {
		t.Fatal("Workflow section not found")
	}
	if signalIdx < 0 {
		t.Fatal("Signal Protocol section not found")
	}
	if consultIdx < 0 {
		t.Fatal("Consultation Protocol section not found")
	}

	if personaIdx >= workflowIdx {
		t.Errorf("Persona (at %d) should come before Workflow (at %d)", personaIdx, workflowIdx)
	}
	if workflowIdx >= signalIdx {
		t.Errorf("Workflow (at %d) should come before Signal Protocol (at %d)", workflowIdx, signalIdx)
	}
	if signalIdx >= consultIdx {
		t.Errorf("Signal Protocol (at %d) should come before Consultation Protocol (at %d)", signalIdx, consultIdx)
	}
}

func TestComposePersonaMergesBasePrinciples(t *testing.T) {
	asm := NewAssembler("testdata")

	result, err := asm.Compose("clario", "technical-design", "")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	// Base principles should be present (from _base.md).
	if !strings.Contains(result, "Team objectives matter more than ego") {
		t.Error("prompt missing base principles")
	}

	// Agent-specific principles should be present.
	if !strings.Contains(result, "I turn intent into technical clarity") {
		t.Error("prompt missing agent-specific principles")
	}
}

func TestComposeParameterizedWorkflow(t *testing.T) {
	asm := NewAssembler("testdata")

	result, err := asm.Compose("clario", "mutation-testing", "")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}

	// Params should be expanded (from clario manifest).
	if !strings.Contains(result, "Validate test quality for architecture") {
		t.Error("prompt missing expanded intent param")
	}
	if !strings.Contains(result, "mutation score meets standards") {
		t.Error("prompt missing expanded stopping_criteria param")
	}

	// No unexpanded placeholders.
	if strings.Contains(result, "{{") {
		t.Error("prompt still contains unexpanded placeholders")
	}
}

func TestComposeFeedbackLoopWorkflow(t *testing.T) {
	asm := NewAssembler("testdata")

	_, err := asm.Compose("clario", "feedback-loop", "")
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
}
