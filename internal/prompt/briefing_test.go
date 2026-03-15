package prompt

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
)

func fullBriefResponse() *release.BriefResponse {
	return &release.BriefResponse{
		FWU: release.FWU{
			ID:        "FWU-01.1.01",
			Name:      "Database infrastructure",
			Status:    "pending",
			Intent:    "Establish the SQLite database layer with GORM, WAL mode, and migration framework.",
			FeatureID: "F-01.1",
		},
		Boundaries: []release.Boundary{
			{ID: "BN:1", Scope: "in_scope", Description: "Pure Go SQLite driver setup and GORM integration"},
			{ID: "BN:2", Scope: "in_scope", Description: "Migration framework with versioned SQL files"},
			{ID: "BN:4", Scope: "out_of_scope", Description: "Table definitions for domain entities (owned by FWU-01.1.02)"},
		},
		Dependencies: []release.Dependency{
			{ID: "DP:1", DependencyType: "fwu", TargetFWUID: "FWU-01.1.02", Description: "Depends on database layer being available"},
		},
		DependencyStatus: []release.DependencyStatus{
			{DependencyID: "DP:1", TargetFWUID: "FWU-01.1.02", TargetFWUName: "Domain models", TargetStatus: "pending", Description: "Depends on database layer being available"},
		},
		DesignDecisions: []release.DesignDecision{
			{ID: "PD:1", Decision: "Which SQLite driver?", Resolution: "modernc.org/sqlite — pure Go, no CGO requirement", Rationale: "NFR14 mandates no CGO for deployment simplicity"},
		},
		InterfaceContracts: []release.InterfaceContract{
			{ID: "CT:1", Direction: "produces", CounterpartFWUID: "FWU-01.1.02", Description: "Database connection handle and migration runner"},
		},
		VerificationGates: []release.VerificationGate{
			{ID: "VG:1", Gate: "tests", Expectation: "Database opens with WAL mode and FK enforcement verified by test"},
			{ID: "VG:2", Gate: "quality", Expectation: "No CGO required — verified by build constraint"},
		},
		ReasoningChain: &release.ReasoningChain{
			Goals:      []release.ChainEntity{{ID: "G-01", Name: "Deliver MVP", Description: "Ship minimum viable product"}},
			CSFs:       []release.ChainEntity{{ID: "CSF-01", Name: "Data persistence", Description: "Reliable data storage"}},
			NCs:        []release.ChainEntity{{ID: "NC-01", Name: "SQLite support", Description: "Embedded database"}},
			SO:         &release.ChainEntity{ID: "SO-01", Name: "Storage layer", Description: "Database abstraction"},
			Capability: &release.ChainEntity{ID: "CAP-01", Name: "Persistence", Description: "Data persistence capability"},
			Epic:       &release.ChainEntity{ID: "E-01", Name: "Database setup", Description: "Database infrastructure epic"},
			Feature:    &release.ChainEntity{ID: "F-01.1", Name: "SQLite integration", Description: "SQLite database feature"},
		},
		PredecessorSpecs: []release.PredecessorSpec{
			{SourceFWUID: "FWU-01.1.01", SourceICID: "IC:1", EntityName: "SomeModel", EntityType: "model", ParentClass: "", CodeBlock: "type SomeModel struct { ... }"},
		},
		PriorICs: []release.PriorIC{
			{ICID: "IC:1", Attempt: 1, Status: "abandoned", PlanningVersion: 1},
		},
		PriorICCount: 1,
	}
}

func TestFormatBriefing_FullResponse(t *testing.T) {
	brief := fullBriefResponse()
	result := FormatBriefing(brief)

	// Verify header
	assertContains(t, result, "## FWU Briefing: FWU-01.1.01 — Database infrastructure")
	assertContains(t, result, "**Status:** pending")
	assertContains(t, result, "**Intent:** Establish the SQLite database layer")

	// Verify all sections present
	assertContains(t, result, "### Boundaries")
	assertContains(t, result, "**In scope:**")
	assertContains(t, result, "**Out of scope:**")
	assertContains(t, result, "### Dependencies")
	assertContains(t, result, "### Design Decisions")
	assertContains(t, result, "### Interface Contracts")
	assertContains(t, result, "### Verification Gates")
	assertContains(t, result, "### Reasoning Chain")
	assertContains(t, result, "### Predecessor Specs")
	assertContains(t, result, "### Prior Implementation Contexts")

	// Verify entity IDs in brackets
	assertContains(t, result, "[BN:1]")
	assertContains(t, result, "[BN:2]")
	assertContains(t, result, "[BN:4]")
	assertContains(t, result, "[DP:1]")
	assertContains(t, result, "[PD:1]")
	assertContains(t, result, "[CT:1]")
	assertContains(t, result, "[VG:1]")
	assertContains(t, result, "[VG:2]")

	// Verify dependency status
	assertContains(t, result, "Status: pending — FWU-01.1.02 \"Domain models\"")

	// Verify design decision formatting
	assertContains(t, result, "**Which SQLite driver?**")
	assertContains(t, result, "Rationale: NFR14")

	// Verify reasoning chain
	assertContains(t, result, "Goal: G-01 \"Deliver MVP\"")
	assertContains(t, result, "Feature: F-01.1 \"SQLite integration\"")
}

func TestFormatBriefing_EmptyBoundaries(t *testing.T) {
	brief := fullBriefResponse()
	brief.Boundaries = nil

	result := FormatBriefing(brief)

	assertNotContains(t, result, "### Boundaries")
	assertNotContains(t, result, "In scope")
	assertNotContains(t, result, "Out of scope")

	// Other sections should still be present
	assertContains(t, result, "### Dependencies")
	assertContains(t, result, "### Design Decisions")
}

func TestFormatBriefing_NilReasoningChain(t *testing.T) {
	brief := fullBriefResponse()
	brief.ReasoningChain = nil

	result := FormatBriefing(brief)

	assertNotContains(t, result, "### Reasoning Chain")

	// Other sections should still be present
	assertContains(t, result, "### Boundaries")
	assertContains(t, result, "### Dependencies")
}

func TestFormatBriefing_PredecessorSpecsWithCodeBlock(t *testing.T) {
	brief := &release.BriefResponse{
		FWU: release.FWU{ID: "FWU-01", Name: "Test", Status: "pending", Intent: "Test intent"},
		PredecessorSpecs: []release.PredecessorSpec{
			{
				SourceFWUID: "FWU-01.1.01",
				SourceICID:  "IC:1",
				EntityName:  "SomeModel",
				EntityType:  "model",
				ParentClass: "models",
				CodeBlock:   "type SomeModel struct { ID int }",
			},
			{
				SourceFWUID: "FWU-01.1.02",
				SourceICID:  "IC:2",
				EntityName:  "Helper",
				EntityType:  "function",
				CodeBlock:   "func Helper() error",
			},
		},
	}

	result := FormatBriefing(brief)

	assertContains(t, result, "### Predecessor Specs")
	assertContains(t, result, "From FWU-01.1.01 (IC:1):")
	assertContains(t, result, "SomeModel (model) in models: `type SomeModel struct { ID int }`")
	assertContains(t, result, "From FWU-01.1.02 (IC:2):")
	assertContains(t, result, "Helper (function): `func Helper() error`")
}

func TestFormatBriefing_PriorICsAttemptHistory(t *testing.T) {
	brief := &release.BriefResponse{
		FWU: release.FWU{ID: "FWU-01", Name: "Test", Status: "pending", Intent: "Test intent"},
		PriorICs: []release.PriorIC{
			{ICID: "IC:1", Attempt: 1, Status: "abandoned", PlanningVersion: 1},
			{ICID: "IC:2", Attempt: 2, Status: "rejected", PlanningVersion: 2},
		},
		PriorICCount: 2,
	}

	result := FormatBriefing(brief)

	assertContains(t, result, "### Prior Implementation Contexts")
	assertContains(t, result, "2 prior attempt(s):")
	assertContains(t, result, "IC:1 (attempt 1): abandoned, planning v1")
	assertContains(t, result, "IC:2 (attempt 2): rejected, planning v2")
}

func TestFormatBriefing_EmptyOptionalSections(t *testing.T) {
	brief := &release.BriefResponse{
		FWU: release.FWU{ID: "FWU-01", Name: "Minimal", Status: "active", Intent: "Minimal test"},
	}

	result := FormatBriefing(brief)

	assertContains(t, result, "## FWU Briefing: FWU-01 — Minimal")
	assertContains(t, result, "**Status:** active")
	assertContains(t, result, "**Intent:** Minimal test")

	// All optional sections should be omitted
	assertNotContains(t, result, "### Boundaries")
	assertNotContains(t, result, "### Dependencies")
	assertNotContains(t, result, "### Design Decisions")
	assertNotContains(t, result, "### Interface Contracts")
	assertNotContains(t, result, "### Verification Gates")
	assertNotContains(t, result, "### Reasoning Chain")
	assertNotContains(t, result, "### Predecessor Specs")
	assertNotContains(t, result, "### Prior Implementation Contexts")
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected output to contain %q, got:\n%s", substr, s)
	}
}

func assertNotContains(t *testing.T, s, substr string) {
	t.Helper()
	if strings.Contains(s, substr) {
		t.Errorf("expected output NOT to contain %q, got:\n%s", substr, s)
	}
}
