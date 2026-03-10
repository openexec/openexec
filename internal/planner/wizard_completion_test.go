package planner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWizard_GeneratesIntentMD verifies that RenderIntent produces
// INTENT.md content when wizard completes.
func TestWizard_GeneratesIntentMD(t *testing.T) {
	// GIVEN a complete wizard state (IsComplete=true)
	state := IntentState{
		ProjectName:      "MyApp",
		Flow:             "greenfield",
		AppType:          "web",
		ProblemStatement: "Solve X",
		PrimaryGoals:     []Goal{{ID: "G-001", Description: "First goal", SuccessCriteria: "Test passes", VerificationMethod: "Unit test"}},
		Constraints:      []Constraint{{ID: "C-001", Description: "Constraint"}},
		Entities:         []Entity{{Name: "User", DataSource: "postgres"}},
		SuccessMetric:    "100% test coverage",
	}

	// Verify state is ready
	if !state.IsReady() {
		t.Fatal("State should be ready for this test")
	}

	// WHEN RenderIntentMD is called
	md := state.RenderIntentMD()

	// THEN INTENT.md content is generated
	if md == "" {
		t.Fatal("RenderIntentMD returned empty string")
	}

	// AND it contains the expected header
	if !strings.Contains(md, "# Intent: MyApp") {
		t.Errorf("Missing project header, got:\n%s", md)
	}
}

// TestWizard_IntentMDContainsRequiredSections verifies that generated
// INTENT.md has all required sections.
func TestWizard_IntentMDContainsRequiredSections(t *testing.T) {
	// GIVEN a complete IntentState
	state := IntentState{
		ProjectName:      "SectionTestApp",
		Flow:             "greenfield",
		AppType:          "api",
		ProblemStatement: "Build an API service",
		PrimaryGoals: []Goal{
			{ID: "G-001", Description: "RESTful endpoints", SuccessCriteria: "All endpoints respond", VerificationMethod: "curl tests"},
		},
		Constraints: []Constraint{
			{ID: "C-001", Description: "Must use JSON"},
			{ID: "C-002", Description: "Must have auth"},
		},
		Entities: []Entity{
			{Name: "User", DataSource: "postgres"},
		},
		SuccessMetric: "API uptime > 99%",
	}

	// WHEN RenderIntentMD is called
	md := state.RenderIntentMD()

	// THEN file has Goals section
	if !strings.Contains(md, "## Goals") {
		t.Error("Missing Goals section")
	}

	// AND file has Requirements section
	if !strings.Contains(md, "## Requirements") {
		t.Error("Missing Requirements section")
	}

	// AND file has Constraints section
	if !strings.Contains(md, "## Constraints") {
		t.Error("Missing Constraints section")
	}

	// AND file has Data Source Mapping
	if !strings.Contains(md, "REQ-002: Data Source Mapping") {
		t.Error("Missing Data Source Mapping section")
	}
}

// TestWizard_IntentMDMatchesState verifies that the generated INTENT.md
// accurately reflects the state values.
func TestWizard_IntentMDMatchesState(t *testing.T) {
	// GIVEN a specific IntentState
	state := IntentState{
		ProjectName:      "ContentMatchApp",
		Flow:             "greenfield",
		AppType:          "desktop",
		Platforms:        []string{"linux", "windows", "macos"},
		ProblemStatement: "Cross-platform file manager",
		PrimaryGoals: []Goal{
			{ID: "G-001", Description: "File browsing", SuccessCriteria: "Can navigate directories", VerificationMethod: "E2E test"},
			{ID: "G-002", Description: "File operations", SuccessCriteria: "Can copy/move/delete", VerificationMethod: "Integration test"},
		},
		Constraints: []Constraint{
			{ID: "C-001", Description: "No admin rights required"},
			{ID: "C-002", Description: "Under 50MB binary size"},
		},
		Entities: []Entity{
			{Name: "File", DataSource: "filesystem"},
			{Name: "Config", DataSource: "local-json"},
		},
		SuccessMetric: "Users can manage files on all platforms",
	}

	// WHEN RenderIntentMD is called
	md := state.RenderIntentMD()

	// THEN project name appears in header
	if !strings.Contains(md, "# Intent: ContentMatchApp") {
		t.Error("Missing project name in header")
	}

	// AND problem statement is in Goals
	if !strings.Contains(md, "Cross-platform file manager") {
		t.Error("Missing problem statement")
	}

	// AND all goals are rendered
	if !strings.Contains(md, "G-001: File browsing") {
		t.Error("Missing G-001 goal")
	}
	if !strings.Contains(md, "G-002: File operations") {
		t.Error("Missing G-002 goal")
	}

	// AND success criteria are rendered
	if !strings.Contains(md, "Success Criteria: Can navigate directories") {
		t.Error("Missing G-001 success criteria")
	}
	if !strings.Contains(md, "Success Criteria: Can copy/move/delete") {
		t.Error("Missing G-002 success criteria")
	}

	// AND verification methods are rendered
	if !strings.Contains(md, "Verification: E2E test") {
		t.Error("Missing G-001 verification method")
	}
	if !strings.Contains(md, "Verification: Integration test") {
		t.Error("Missing G-002 verification method")
	}

	// AND app type is in Requirements
	if !strings.Contains(md, "Shape: desktop") {
		t.Error("Missing app type")
	}

	// AND platforms are joined
	if !strings.Contains(md, "linux, windows, macos") {
		t.Error("Missing platforms")
	}

	// AND constraints are rendered
	if !strings.Contains(md, "C-001: No admin rights required") {
		t.Error("Missing C-001 constraint")
	}
	if !strings.Contains(md, "C-002: Under 50MB binary size") {
		t.Error("Missing C-002 constraint")
	}

	// AND entities with data sources are rendered
	if !strings.Contains(md, "File: Source of Truth: filesystem") {
		t.Error("Missing File entity")
	}
	if !strings.Contains(md, "Config: Source of Truth: local-json") {
		t.Error("Missing Config entity")
	}

	// AND global success metric is rendered
	if !strings.Contains(md, "Global Success Metric: Users can manage files on all platforms") {
		t.Error("Missing global success metric")
	}
}

// TestWizard_IntentMDWrittenToDisk verifies the full flow of generating
// and writing INTENT.md to disk.
func TestWizard_IntentMDWrittenToDisk(t *testing.T) {
	// GIVEN a wizard session that reaches IsComplete=true
	tmpDir := t.TempDir()
	intentPath := filepath.Join(tmpDir, "INTENT.md")

	state := IntentState{
		ProjectName:      "DiskWriteApp",
		Flow:             "greenfield",
		AppType:          "web",
		ProblemStatement: "Testing file write",
		PrimaryGoals:     []Goal{{ID: "G-001", Description: "Test", SuccessCriteria: "Pass", VerificationMethod: "Check"}},
		Constraints:      []Constraint{{ID: "C-001", Description: "Simple"}},
		Entities:         []Entity{{Name: "Data", DataSource: "postgres"}},
	}

	// WHEN the wizard renders and writes INTENT.md
	md := state.RenderIntentMD()
	if err := os.WriteFile(intentPath, []byte(md), 0644); err != nil {
		t.Fatalf("Failed to write INTENT.md: %v", err)
	}

	// THEN INTENT.md exists on disk
	if _, err := os.Stat(intentPath); os.IsNotExist(err) {
		t.Fatal("INTENT.md was not created")
	}

	// AND its content matches what was rendered
	content, err := os.ReadFile(intentPath)
	if err != nil {
		t.Fatalf("Failed to read INTENT.md: %v", err)
	}

	if string(content) != md {
		t.Error("INTENT.md content does not match rendered markdown")
	}

	// AND it contains expected content
	if !strings.Contains(string(content), "# Intent: DiskWriteApp") {
		t.Error("Written file missing expected header")
	}
}

// TestWizard_RenderIntentFromJSON verifies the Planner.RenderIntent method
// that takes JSON state and produces markdown.
func TestWizard_RenderIntentFromJSON(t *testing.T) {
	// GIVEN a Planner and valid state JSON
	p := New(nil) // Provider not needed for RenderIntent
	ctx := context.Background()

	stateJSON := `{
		"project_name": "JSONRenderApp",
		"flow": "greenfield",
		"app_type": "cli",
		"problem_statement": "CLI tool",
		"primary_goals": [{"id": "G-001", "description": "Fast execution", "success_criteria": "< 1s", "verification_method": "Benchmark"}],
		"constraints": [{"id": "C-001", "description": "No deps"}],
		"entities": [{"name": "Command", "data_source": "args"}]
	}`

	// WHEN RenderIntent is called with state JSON
	md, err := p.RenderIntent(ctx, stateJSON)

	// THEN markdown is generated without error
	if err != nil {
		t.Fatalf("RenderIntent failed: %v", err)
	}

	// AND it contains expected content
	if !strings.Contains(md, "# Intent: JSONRenderApp") {
		t.Error("Missing project header")
	}
	if !strings.Contains(md, "G-001: Fast execution") {
		t.Error("Missing goal")
	}
	if !strings.Contains(md, "C-001: No deps") {
		t.Error("Missing constraint")
	}
}

// TestWizard_RenderIntentInvalidJSON verifies error handling for
// malformed JSON input.
func TestWizard_RenderIntentInvalidJSON(t *testing.T) {
	// GIVEN a Planner and invalid JSON
	p := New(nil)
	ctx := context.Background()
	invalidJSON := `{"project_name": broken}`

	// WHEN RenderIntent is called
	_, err := p.RenderIntent(ctx, invalidJSON)

	// THEN error is returned
	if err == nil {
		t.Error("RenderIntent should return error for invalid JSON")
	}
}
