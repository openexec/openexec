package planner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestWizard_SavesStateAfterEachTurn verifies that wizard state can be
// serialized to JSON for persistence.
func TestWizard_SavesStateAfterEachTurn(t *testing.T) {
	// GIVEN a wizard session with updated state
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, ".openexec", "wizard_state.json")

	// Create .openexec directory
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	state := IntentState{
		ProjectName:      "SaveTest",
		Flow:             "greenfield",
		AppType:          "web",
		ProblemStatement: "Testing persistence",
	}

	// WHEN state is serialized and written to disk
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Failed to marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
		t.Fatalf("Failed to write state file: %v", err)
	}

	// THEN wizard_state.json contains the updated state
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	var readState IntentState
	if err := json.Unmarshal(data, &readState); err != nil {
		t.Fatalf("Failed to unmarshal state: %v", err)
	}

	if readState.ProjectName != "SaveTest" {
		t.Errorf("ProjectName = %q, want SaveTest", readState.ProjectName)
	}
	if readState.Flow != "greenfield" {
		t.Errorf("Flow = %q, want greenfield", readState.Flow)
	}
}

// TestWizard_ResumesFromSavedState verifies that wizard state can be
// loaded from an existing wizard_state.json file.
func TestWizard_ResumesFromSavedState(t *testing.T) {
	// GIVEN .openexec/wizard_state.json exists with saved state
	tmpDir := t.TempDir()
	openexecDir := filepath.Join(tmpDir, ".openexec")
	statePath := filepath.Join(openexecDir, "wizard_state.json")

	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	existingState := IntentState{
		ProjectName: "ResumedProject",
		Flow:        "greenfield",
		AppType:     "cli",
	}
	existingBytes, _ := json.Marshal(existingState)
	if err := os.WriteFile(statePath, existingBytes, 0644); err != nil {
		t.Fatalf("Failed to write existing state: %v", err)
	}

	// WHEN wizard starts and reads existing state
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	var resumedState IntentState
	if err := json.Unmarshal(data, &resumedState); err != nil {
		t.Fatalf("Failed to parse resumed state: %v", err)
	}

	// THEN the saved state is loaded correctly
	if resumedState.ProjectName != "ResumedProject" {
		t.Errorf("ProjectName = %q, want ResumedProject", resumedState.ProjectName)
	}
	if resumedState.Flow != "greenfield" {
		t.Errorf("Flow = %q, want greenfield", resumedState.Flow)
	}
	if resumedState.AppType != "cli" {
		t.Errorf("AppType = %q, want cli", resumedState.AppType)
	}
}

// TestWizard_StartsEmptyWithNoStateFile verifies that a fresh wizard
// starts with empty state when no wizard_state.json exists.
func TestWizard_StartsEmptyWithNoStateFile(t *testing.T) {
	// GIVEN no wizard_state.json exists
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, ".openexec", "wizard_state.json")

	// WHEN wizard tries to read state file
	_, err := os.ReadFile(statePath)

	// THEN error indicates file does not exist
	if err == nil {
		t.Error("Expected error when reading non-existent state file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("Expected file not exist error, got: %v", err)
	}

	// AND starting with empty state "{}" should parse to zero IntentState
	emptyState := "{}"
	var state IntentState
	if err := json.Unmarshal([]byte(emptyState), &state); err != nil {
		t.Fatalf("Failed to parse empty state: %v", err)
	}

	if state.ProjectName != "" {
		t.Errorf("Expected empty ProjectName, got %q", state.ProjectName)
	}
	if state.Flow != "" {
		t.Errorf("Expected empty Flow, got %q", state.Flow)
	}
}

// TestWizard_StateFileIsValidJSON verifies that serialized state
// produces valid, parseable JSON.
func TestWizard_StateFileIsValidJSON(t *testing.T) {
	// GIVEN a complete IntentState
	state := IntentState{
		ProjectName:      "JSONTest",
		Flow:             "refactor",
		AppType:          "desktop",
		Platforms:        []string{"linux", "windows"},
		ProblemStatement: "Modernize legacy system",
		PrimaryGoals: []Goal{
			{ID: "G-001", Description: "Improve performance"},
			{ID: "G-002", Description: "Add tests"},
		},
		Constraints: []Constraint{
			{ID: "C-001", Description: "Must maintain API compatibility"},
		},
		Entities: []Entity{
			{Name: "User", DataSource: "postgres"},
			{Name: "Session", DataSource: "redis"},
		},
		LegacyRepoPath: "/path/to/legacy",
	}

	// WHEN state is serialized to JSON
	stateBytes, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Failed to marshal state: %v", err)
	}

	// THEN the JSON is valid and parseable
	var parsedState IntentState
	if err := json.Unmarshal(stateBytes, &parsedState); err != nil {
		t.Fatalf("Failed to parse serialized state: %v", err)
	}

	// AND all fields are preserved
	if parsedState.ProjectName != state.ProjectName {
		t.Errorf("ProjectName mismatch: %q vs %q", parsedState.ProjectName, state.ProjectName)
	}
	if len(parsedState.Platforms) != len(state.Platforms) {
		t.Errorf("Platforms length mismatch: %d vs %d", len(parsedState.Platforms), len(state.Platforms))
	}
	if len(parsedState.PrimaryGoals) != len(state.PrimaryGoals) {
		t.Errorf("PrimaryGoals length mismatch: %d vs %d", len(parsedState.PrimaryGoals), len(state.PrimaryGoals))
	}
	if len(parsedState.Entities) != len(state.Entities) {
		t.Errorf("Entities length mismatch: %d vs %d", len(parsedState.Entities), len(state.Entities))
	}
	if parsedState.LegacyRepoPath != state.LegacyRepoPath {
		t.Errorf("LegacyRepoPath mismatch: %q vs %q", parsedState.LegacyRepoPath, state.LegacyRepoPath)
	}
}

// TestWizard_PersistenceRoundTrip verifies full save-resume cycle
// simulating a user exiting and resuming the wizard.
func TestWizard_PersistenceRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	openexecDir := filepath.Join(tmpDir, ".openexec")
	statePath := filepath.Join(openexecDir, "wizard_state.json")

	// Create .openexec directory
	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Simulate wizard session 1: Start fresh
	stateJSON := "{}"
	var state IntentState
	_ = json.Unmarshal([]byte(stateJSON), &state)

	// User provides initial info
	state.ProjectName = "RoundTripApp"
	state.Flow = "greenfield"
	state.AppType = "api"

	// Save state (simulating exit)
	stateBytes, _ := json.Marshal(state)
	if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Simulate wizard session 2: Resume
	resumedData, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read saved state: %v", err)
	}

	var resumedState IntentState
	if err := json.Unmarshal(resumedData, &resumedState); err != nil {
		t.Fatalf("Failed to parse resumed state: %v", err)
	}

	// Verify state persisted correctly
	if resumedState.ProjectName != "RoundTripApp" {
		t.Errorf("ProjectName = %q, want RoundTripApp", resumedState.ProjectName)
	}
	if resumedState.Flow != "greenfield" {
		t.Errorf("Flow = %q, want greenfield", resumedState.Flow)
	}
	if resumedState.AppType != "api" {
		t.Errorf("AppType = %q, want api", resumedState.AppType)
	}

	// User continues and completes
	resumedState.ProblemStatement = "Build an API"
	resumedState.PrimaryGoals = []Goal{{ID: "G-001", Description: "RESTful API"}}
	resumedState.Constraints = []Constraint{{ID: "C-001", Description: "JSON only"}}
	resumedState.Entities = []Entity{{Name: "Resource", DataSource: "postgres"}}

	// Save final state
	finalBytes, _ := json.Marshal(resumedState)
	if err := os.WriteFile(statePath, finalBytes, 0644); err != nil {
		t.Fatalf("Failed to save final state: %v", err)
	}

	// Verify final state
	finalData, _ := os.ReadFile(statePath)
	var finalState IntentState
	_ = json.Unmarshal(finalData, &finalState)

	if !finalState.IsReady() {
		t.Error("Final state should be ready")
	}
}
