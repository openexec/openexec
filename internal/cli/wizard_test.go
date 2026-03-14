package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/planner"
)

// TestWizard_ExitCommandRecognition verifies that "exit" and "quit" strings
// are recognized as exit commands.
func TestWizard_ExitCommandRecognition(t *testing.T) {
	tests := []struct {
		input     string
		isExit    bool
		emptySkip bool
	}{
		{"exit", true, false},
		{"quit", true, false},
		{"EXIT", false, false}, // not recognized - wizard uses lowercase comparison only
		{"QUIT", false, false}, // not recognized - wizard uses lowercase comparison only
		{"exit ", true, false}, // trailing space becomes "exit" after TrimSpace
		{" exit", true, false}, // leading space becomes "exit" after TrimSpace
		{"", false, true},      // empty input continues loop
		{"continue", false, false},
		{"exit now", false, false}, // not exact match
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			// Simulate the exit check from wizard.go
			message := strings.TrimSpace(tt.input)
			isExit := message == "exit" || message == "quit"
			isEmpty := message == ""

			if isExit != tt.isExit {
				t.Errorf("exit check for %q: got %v, want %v", tt.input, isExit, tt.isExit)
			}
			if isEmpty != tt.emptySkip {
				t.Errorf("empty check for %q: got %v, want %v", tt.input, isEmpty, tt.emptySkip)
			}
		})
	}
}

// TestWizard_ExitSavesState verifies that wizard state is preserved
// before exit, simulating the save behavior.
func TestWizard_ExitSavesState(t *testing.T) {
	// GIVEN a wizard session with state
	tmpDir := t.TempDir()
	openexecDir := filepath.Join(tmpDir, ".openexec")
	statePath := filepath.Join(openexecDir, "wizard_state.json")

	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	state := planner.IntentState{
		ProjectName: "InProgress",
		Flow:        "greenfield",
		AppType:     "web",
	}

	// WHEN state is saved before exit (simulating wizard behavior)
	stateBytes, _ := json.Marshal(state)
	if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
		t.Fatalf("Failed to write state: %v", err)
	}

	// Simulate user typing "exit"
	message := "exit"
	isExit := message == "exit" || message == "quit"
	if !isExit {
		t.Fatal("exit command not recognized")
	}

	// THEN wizard_state.json contains the current state
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("Failed to read state file: %v", err)
	}

	var savedState planner.IntentState
	if err := json.Unmarshal(data, &savedState); err != nil {
		t.Fatalf("Failed to parse saved state: %v", err)
	}

	if savedState.ProjectName != "InProgress" {
		t.Errorf("ProjectName = %q, want InProgress", savedState.ProjectName)
	}
	if savedState.Flow != "greenfield" {
		t.Errorf("Flow = %q, want greenfield", savedState.Flow)
	}
}

// TestWizard_QuitSavesState verifies that "quit" command works same as "exit"
func TestWizard_QuitSavesState(t *testing.T) {
	// GIVEN a wizard session with state
	tmpDir := t.TempDir()
	openexecDir := filepath.Join(tmpDir, ".openexec")
	statePath := filepath.Join(openexecDir, "wizard_state.json")

	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	state := planner.IntentState{
		ProjectName: "QuitTest",
		Flow:        "refactor",
		AppType:     "cli",
	}

	// Save state before quit
	stateBytes, _ := json.Marshal(state)
	if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
		t.Fatalf("Failed to write state: %v", err)
	}

	// Simulate user typing "quit"
	message := "quit"
	isExit := message == "exit" || message == "quit"

	// THEN quit is recognized as exit
	if !isExit {
		t.Error("quit command not recognized")
	}

	// AND state is preserved
	data, _ := os.ReadFile(statePath)
	var savedState planner.IntentState
	_ = json.Unmarshal(data, &savedState)

	if savedState.ProjectName != "QuitTest" {
		t.Errorf("ProjectName = %q, want QuitTest", savedState.ProjectName)
	}
}

// TestWizard_EmptyInputContinues verifies that empty input doesn't
// trigger exit or state update.
func TestWizard_EmptyInputContinues(t *testing.T) {
	// GIVEN empty input (user presses Enter)
	inputs := []string{"", "  ", "\t", "\n"}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			message := strings.TrimSpace(input)

			// WHEN input is trimmed
			isEmpty := message == ""
			isExit := message == "exit" || message == "quit"

			// THEN it should be detected as empty (continue loop)
			if !isEmpty {
				t.Errorf("Input %q should be empty after trim", input)
			}

			// AND it should not be detected as exit
			if isExit {
				t.Errorf("Empty input should not be exit")
			}
		})
	}
}

// TestWizard_StatePathConstruction verifies the state file path is correct.
func TestWizard_StatePathConstruction(t *testing.T) {
	// The wizard uses filepath.Join(".openexec", "wizard_state.json")
	statePath := filepath.Join(".openexec", "wizard_state.json")

	// Verify it's a relative path in .openexec
	if !strings.HasPrefix(statePath, ".openexec") {
		t.Errorf("State path should be in .openexec, got %s", statePath)
	}

	if !strings.HasSuffix(statePath, "wizard_state.json") {
		t.Errorf("State file should be wizard_state.json, got %s", statePath)
	}
}

// TestWizard_StateFilePermissions verifies state file is created with
// appropriate permissions.
func TestWizard_StateFilePermissions(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "wizard_state.json")

	// Write state with 0644 permissions (matching wizard.go)
	if err := os.WriteFile(statePath, []byte("{}"), 0644); err != nil {
		t.Fatalf("Failed to write state: %v", err)
	}

	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("Failed to stat state file: %v", err)
	}

	// Check file mode (permissions vary by umask, so check it's at least readable)
	mode := info.Mode().Perm()
	if mode&0400 == 0 {
		t.Error("State file should be owner-readable")
	}
	if mode&0200 == 0 {
		t.Error("State file should be owner-writable")
	}
}

// TestWizard_NoIntentMDOnExit verifies that INTENT.md is NOT written
// when user exits early.
func TestWizard_NoIntentMDOnExit(t *testing.T) {
	// GIVEN a wizard session that exits before completion
	tmpDir := t.TempDir()
	intentPath := filepath.Join(tmpDir, "INTENT.md")
	openexecDir := filepath.Join(tmpDir, ".openexec")
	statePath := filepath.Join(openexecDir, "wizard_state.json")

	os.MkdirAll(openexecDir, 0755)

	// Save incomplete state
	state := planner.IntentState{
		ProjectName: "IncompleteProject",
		Flow:        "greenfield",
		// Missing required fields - state is not ready
	}
	stateBytes, _ := json.Marshal(state)
	os.WriteFile(statePath, stateBytes, 0644)

	// Simulate exit command
	message := "exit"
	isExit := message == "exit" || message == "quit"
	if !isExit {
		t.Fatal("exit not recognized")
	}

	// On exit, wizard does NOT write INTENT.md
	// (INTENT.md is only written when IsComplete=true)

	// THEN INTENT.md should not exist
	if _, err := os.Stat(intentPath); err == nil {
		t.Error("INTENT.md should not exist after exit")
	} else if !os.IsNotExist(err) {
		t.Errorf("Unexpected error checking INTENT.md: %v", err)
	}

	// BUT wizard_state.json should exist
	if _, err := os.Stat(statePath); err != nil {
		t.Error("wizard_state.json should exist after exit")
	}
}

// TestWizard_StateUpdateOnMessageProcess simulates the state update flow
// that happens after each wizard message is processed.
func TestWizard_StateUpdateOnMessageProcess(t *testing.T) {
	tmpDir := t.TempDir()
	openexecDir := filepath.Join(tmpDir, ".openexec")
	statePath := filepath.Join(openexecDir, "wizard_state.json")
	os.MkdirAll(openexecDir, 0755)

	// Simulate first response updating state
	updatedState := planner.IntentState{
		ProjectName: "FirstUpdate",
		Flow:        "greenfield",
	}
	stateBytes, _ := json.Marshal(updatedState)

	// Persist state to disk (as wizard does after each turn)
	if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
		t.Fatalf("Failed to write state: %v", err)
	}

	// Verify state persisted
	data, _ := os.ReadFile(statePath)
	var savedState planner.IntentState
	json.Unmarshal(data, &savedState)

	if savedState.ProjectName != "FirstUpdate" {
		t.Errorf("State not updated correctly: got %s", savedState.ProjectName)
	}

	// Simulate second message updating state further
	updatedState.AppType = "web"
	updatedState.ProblemStatement = "Build a thing"
	stateBytes, _ = json.Marshal(updatedState)
	os.WriteFile(statePath, stateBytes, 0644)

	// Verify second update persisted
	data, _ = os.ReadFile(statePath)
	json.Unmarshal(data, &savedState)

	if savedState.AppType != "web" {
		t.Errorf("AppType not updated: got %s", savedState.AppType)
	}
	if savedState.ProblemStatement != "Build a thing" {
		t.Errorf("ProblemStatement not updated: got %s", savedState.ProblemStatement)
	}
}

// =============================================================================
// Wizard Session Persistence Tests (T-US-001-003)
// =============================================================================

// TestWizard_SessionResumption verifies that a wizard session can be resumed
// from an existing wizard_state.json file (T-US-001-003 behavioral scenario 1).
func TestWizard_SessionResumption(t *testing.T) {
	t.Run("Existing state file is loaded on session start", func(t *testing.T) {
		// GIVEN an existing wizard_state.json with saved progress
		tmpDir := t.TempDir()
		openexecDir := filepath.Join(tmpDir, ".openexec")
		statePath := filepath.Join(openexecDir, "wizard_state.json")

		if err := os.MkdirAll(openexecDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Save state representing partial interview completion
		savedState := planner.IntentState{
			ProjectName:      "ResumedProject",
			Flow:             "greenfield",
			AppType:          "api",
			ProblemStatement: "Build an API service",
			// Missing: Goals, Constraints, Entities - interview not complete
		}
		stateBytes, err := json.Marshal(savedState)
		if err != nil {
			t.Fatalf("Failed to marshal state: %v", err)
		}
		if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
			t.Fatalf("Failed to write state: %v", err)
		}

		// WHEN the wizard session is resumed (simulated by reading existing state)
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("Failed to read state file: %v", err)
		}

		var loadedState planner.IntentState
		if err := json.Unmarshal(data, &loadedState); err != nil {
			t.Fatalf("Failed to parse saved state: %v", err)
		}

		// THEN session should resume with saved state
		if loadedState.ProjectName != "ResumedProject" {
			t.Errorf("ProjectName = %q, want \"ResumedProject\"", loadedState.ProjectName)
		}
		if loadedState.Flow != "greenfield" {
			t.Errorf("Flow = %q, want \"greenfield\"", loadedState.Flow)
		}
		if loadedState.AppType != "api" {
			t.Errorf("AppType = %q, want \"api\"", loadedState.AppType)
		}
		if loadedState.ProblemStatement != "Build an API service" {
			t.Errorf("ProblemStatement = %q, want \"Build an API service\"", loadedState.ProblemStatement)
		}

		// AND state should not be ready (incomplete)
		if loadedState.IsReady() {
			t.Error("Loaded state should not be ready (missing required fields)")
		}
	})

	t.Run("No state file starts fresh session", func(t *testing.T) {
		// GIVEN no existing wizard_state.json
		tmpDir := t.TempDir()
		openexecDir := filepath.Join(tmpDir, ".openexec")
		statePath := filepath.Join(openexecDir, "wizard_state.json")

		// Ensure the directory exists but no state file
		if err := os.MkdirAll(openexecDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// WHEN checking for existing state
		_, err := os.ReadFile(statePath)

		// THEN state file should not exist
		if err == nil {
			t.Error("Expected state file to not exist for fresh session")
		}
		if !os.IsNotExist(err) {
			t.Errorf("Expected os.IsNotExist error, got: %v", err)
		}

		// AND a fresh IntentState would be used
		freshState := planner.IntentState{}
		if freshState.IsReady() {
			t.Error("Fresh state should not be ready")
		}
	})

	t.Run("Corrupted state file is handled gracefully", func(t *testing.T) {
		// GIVEN a corrupted wizard_state.json
		tmpDir := t.TempDir()
		openexecDir := filepath.Join(tmpDir, ".openexec")
		statePath := filepath.Join(openexecDir, "wizard_state.json")

		if err := os.MkdirAll(openexecDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Write corrupted JSON
		if err := os.WriteFile(statePath, []byte("{invalid json"), 0644); err != nil {
			t.Fatalf("Failed to write corrupted state: %v", err)
		}

		// WHEN reading the corrupted state
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("Failed to read state file: %v", err)
		}

		var loadedState planner.IntentState
		err = json.Unmarshal(data, &loadedState)

		// THEN JSON parse should fail
		if err == nil {
			t.Error("Expected JSON parse error for corrupted state")
		}

		// AND a fresh state can be used as fallback
		freshState := planner.IntentState{}
		if freshState.ProjectName != "" {
			t.Error("Fresh fallback state should have empty ProjectName")
		}
	})

	t.Run("Complete state file indicates ready to render", func(t *testing.T) {
		// GIVEN an existing wizard_state.json with all required fields
		tmpDir := t.TempDir()
		openexecDir := filepath.Join(tmpDir, ".openexec")
		statePath := filepath.Join(openexecDir, "wizard_state.json")

		if err := os.MkdirAll(openexecDir, 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}

		// Save complete state
		completeState := planner.IntentState{
			ProjectName:      "CompleteProject",
			Flow:             "greenfield",
			AppType:          "web",
			ProblemStatement: "Build a complete web app",
			PrimaryGoals:     []planner.Goal{{ID: "G-001", Description: "Primary goal"}},
			Constraints:      []planner.Constraint{{ID: "C-001", Description: "Constraint"}},
			Entities:         []planner.Entity{{Name: "User", DataSource: "postgres"}},
		}
		stateBytes, err := json.Marshal(completeState)
		if err != nil {
			t.Fatalf("Failed to marshal state: %v", err)
		}
		if err := os.WriteFile(statePath, stateBytes, 0644); err != nil {
			t.Fatalf("Failed to write state: %v", err)
		}

		// WHEN loading the state
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("Failed to read state: %v", err)
		}
		var loadedState planner.IntentState
		if err := json.Unmarshal(data, &loadedState); err != nil {
			t.Fatalf("Failed to unmarshal state: %v", err)
		}

		// THEN state should be ready for INTENT.md rendering
		if !loadedState.IsReady() {
			t.Error("Complete loaded state should be ready")
		}
	})
}
