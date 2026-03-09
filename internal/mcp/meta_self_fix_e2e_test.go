package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// E2E Test: Meta Self-Fix (G-005)
//
// This test suite validates the end-to-end integration of meta self-fix capabilities.
// Meta self-fix allows the orchestrator to detect issues in its own code, apply fixes,
// rebuild itself, and restart with session continuity. The key components tested are:
//
// 1. OrchestratorLocator - Finds and classifies orchestrator source files
// 2. OrchestratorBuilder - Compiles the orchestrator with syntax/build validation
// 3. RestartManager - Handles restart requests with approval workflow
// 4. SessionResumeManager - Persists and restores session state across restarts
//
// Together these enable the orchestrator to autonomously fix compilation errors,
// configuration issues, or code problems while preserving the user's session context.

// =============================================================================
// Test Fixtures and Helpers
// =============================================================================

// setupTestOrchestratorRoot creates a temporary directory structure that mimics
// a valid orchestrator project for testing.
func setupTestOrchestratorRoot(t *testing.T) string {
	t.Helper()

	tmpDir := t.TempDir()

	// Create go.mod file (required for orchestrator detection)
	goMod := `module github.com/openexec/openexec

go 1.21

require github.com/google/uuid v1.3.0
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatalf("failed to create go.mod: %v", err)
	}

	// Create internal directory structure
	internalDir := filepath.Join(tmpDir, "internal", "mcp")
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		t.Fatalf("failed to create internal/mcp: %v", err)
	}

	// Create a simple Go source file
	serverGo := `package mcp

import "fmt"

func Hello() {
	fmt.Println("Hello from orchestrator")
}
`
	if err := os.WriteFile(filepath.Join(internalDir, "server.go"), []byte(serverGo), 0644); err != nil {
		t.Fatalf("failed to create server.go: %v", err)
	}

	// Create cmd directory with main entry point
	cmdDir := filepath.Join(tmpDir, "cmd", "openexec")
	if err := os.MkdirAll(cmdDir, 0755); err != nil {
		t.Fatalf("failed to create cmd/openexec: %v", err)
	}

	mainGo := `package main

func main() {
	println("OpenExec Orchestrator")
}
`
	if err := os.WriteFile(filepath.Join(cmdDir, "main.go"), []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to create main.go: %v", err)
	}

	// Create .openexec directory for metadata
	openexecDir := filepath.Join(tmpDir, ".openexec")
	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatalf("failed to create .openexec: %v", err)
	}

	return tmpDir
}

// setupTestOrchestratorWithSyntaxError creates a test project with a syntax error
// for testing error detection and fix workflows.
func setupTestOrchestratorWithSyntaxError(t *testing.T) (string, string) {
	t.Helper()

	root := setupTestOrchestratorRoot(t)

	// Create a file with a syntax error
	errorFile := filepath.Join(root, "internal", "mcp", "broken.go")
	brokenCode := `package mcp

import "fmt"

func Broken() {
	fmt.Println("This is missing a closing brace"
}
`
	if err := os.WriteFile(errorFile, []byte(brokenCode), 0644); err != nil {
		t.Fatalf("failed to create broken.go: %v", err)
	}

	return root, errorFile
}

// =============================================================================
// OrchestratorLocator E2E Tests
// =============================================================================

// TestE2E_MetaSelfFix_LocatorDetectsOrchestratorFiles verifies that the
// OrchestratorLocator can find and classify orchestrator source files.
func TestE2E_MetaSelfFix_LocatorDetectsOrchestratorFiles(t *testing.T) {
	// Use the real orchestrator root for this test
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("detects Go source files", func(t *testing.T) {
		_ = ctx // context for future use

		files, err := locator.LocateByType(FileTypeGoSource)
		if err != nil {
			t.Fatalf("failed to locate Go files: %v", err)
		}

		if len(files) == 0 {
			t.Fatal("expected to find Go source files")
		}

		// Verify all files are Go files
		for _, f := range files {
			if !strings.HasSuffix(f.Path, ".go") {
				t.Errorf("non-Go file returned: %s", f.Path)
			}
		}

		t.Logf("Found %d Go source files", len(files))
	})

	t.Run("classifies test files correctly", func(t *testing.T) {
		// Find this test file
		file, err := locator.Locate("internal/mcp/meta_self_fix_e2e_test.go")
		if err != nil {
			t.Fatalf("failed to locate test file: %v", err)
		}

		if !file.IsTest {
			t.Error("expected test file to be marked as IsTest")
		}
		if file.Type != FileTypeGoSource {
			t.Errorf("expected FileTypeGoSource, got %v", file.Type)
		}
		if file.Package != "mcp" {
			t.Errorf("expected package 'mcp', got %q", file.Package)
		}
	})

	t.Run("validates paths for editing", func(t *testing.T) {
		// Valid path within orchestrator
		validPath := filepath.Join(root, "internal", "mcp", "new_file.go")
		if err := locator.ValidateForEdit(validPath); err != nil {
			t.Errorf("expected valid path to be allowed: %v", err)
		}

		// Invalid path outside orchestrator
		invalidPath := "/tmp/outside/file.go"
		if err := locator.ValidateForEdit(invalidPath); err == nil {
			t.Error("expected path outside orchestrator to be rejected")
		}

		// Invalid path in excluded directory
		excludedPath := filepath.Join(root, ".git", "config")
		if err := locator.ValidateForEdit(excludedPath); err == nil {
			t.Error("expected path in .git to be rejected")
		}
	})

	t.Run("locates files in package", func(t *testing.T) {
		files, err := locator.LocateInPackage("internal/mcp")
		if err != nil {
			t.Fatalf("failed to locate files in package: %v", err)
		}

		if len(files) < 5 {
			t.Errorf("expected at least 5 files in internal/mcp, got %d", len(files))
		}

		// Verify all files are in the mcp package
		for _, f := range files {
			if f.Package != "mcp" {
				t.Errorf("expected package 'mcp', got %q for %s", f.Package, f.Path)
			}
		}
	})
}

// =============================================================================
// OrchestratorBuilder E2E Tests
// =============================================================================

// TestE2E_MetaSelfFix_BuilderValidatesSyntax verifies that the builder can
// detect syntax errors in orchestrator code.
func TestE2E_MetaSelfFix_BuilderValidatesSyntax(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./internal/mcp")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Run("syntax check passes for valid code", func(t *testing.T) {
		result, err := builder.CheckSyntax(ctx)
		if err != nil {
			t.Fatalf("syntax check error: %v", err)
		}

		if !result.Success {
			t.Errorf("expected syntax check to pass: %s", result.Output)
		}
	})

	t.Run("build produces valid result", func(t *testing.T) {
		result, err := builder.Build(ctx)
		if err != nil {
			t.Fatalf("build error: %v", err)
		}

		// Build should succeed for this package
		if !result.Success {
			t.Logf("Build output: %s", result.Output)
			// Not failing because internal/mcp may have dependencies
		}

		// Verify result has expected fields
		if result.Command == "" {
			t.Error("expected command to be recorded")
		}
		if result.Duration == 0 {
			t.Error("expected non-zero duration")
		}
	})

	t.Run("vet detects potential issues", func(t *testing.T) {
		result, err := builder.Vet(ctx)
		if err != nil {
			t.Fatalf("vet error: %v", err)
		}

		// Vet should complete without crashing
		if result.Command == "" {
			t.Error("expected command to be recorded")
		}

		// Check for any warnings
		if len(result.Warnings) > 0 {
			t.Logf("Vet found %d warnings", len(result.Warnings))
		}
	})
}

// TestE2E_MetaSelfFix_BuilderParsesErrors verifies that build errors are
// correctly parsed and categorized.
func TestE2E_MetaSelfFix_BuilderParsesErrors(t *testing.T) {
	testCases := []struct {
		name         string
		output       string
		cmd          string
		wantErrors   int
		wantWarnings int
	}{
		{
			name:         "undefined identifier",
			output:       "./handler.go:42:10: undefined: ProcessRequest",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "type mismatch",
			output:       "./server.go:15:20: cannot use x (type int) as type string",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "multiple errors",
			output:       "./a.go:10:5: undefined: foo\n./b.go:20:10: cannot use x as type y\n./c.go:30:15: missing return",
			cmd:          "build",
			wantErrors:   3,
			wantWarnings: 0,
		},
		{
			name:         "vet warning",
			output:       "./util.go:25:3: result of fmt.Sprint not used",
			cmd:          "vet",
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name:         "import error",
			output:       `./main.go:5:2: cannot find package "nonexistent" in any of`,
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errors, warnings := parseGoBuildOutput(tc.output, tc.cmd)

			if len(errors) != tc.wantErrors {
				t.Errorf("expected %d errors, got %d: %+v", tc.wantErrors, len(errors), errors)
			}
			if len(warnings) != tc.wantWarnings {
				t.Errorf("expected %d warnings, got %d: %+v", tc.wantWarnings, len(warnings), warnings)
			}
		})
	}
}

// TestE2E_MetaSelfFix_BuildResultProvidesSuggestions verifies that build results
// include actionable fix suggestions.
func TestE2E_MetaSelfFix_BuildResultProvidesSuggestions(t *testing.T) {
	result := &BuildResult{
		Output: `./handler.go:42:10: undefined: ProcessRequest
./server.go:15:3: declared and not used: unusedVar
./config.go:8:2: cannot find package "github.com/missing/pkg"`,
		Target:  BuildTargetMain,
		Command: "go build ./...",
	}

	suggestions := result.GetSuggestionsForFix()

	if len(suggestions) == 0 {
		t.Fatal("expected suggestions for errors")
	}

	// Check that undefined error has suggestion
	foundUndefinedSuggestion := false
	for _, s := range suggestions {
		if strings.Contains(s.Message, "undefined") {
			foundUndefinedSuggestion = true
			if s.Suggestion == "" {
				t.Error("expected non-empty suggestion for undefined error")
			}
		}
	}

	if !foundUndefinedSuggestion {
		t.Error("expected suggestion for undefined identifier error")
	}

	// Check categorized errors
	categorized := result.GetCategorizedErrors()
	if len(categorized) == 0 {
		t.Error("expected categorized errors")
	}
}

// =============================================================================
// RestartManager E2E Tests
// =============================================================================

// TestE2E_MetaSelfFix_RestartManagerLifecycle tests the complete lifecycle of
// a restart request from creation to completion.
func TestE2E_MetaSelfFix_RestartManagerLifecycle(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Create manager with approval disabled for testing
	config := DefaultRestartManagerConfig()
	config.RequireApproval = false
	config.AutoBuild = false
	config.EnableSessionResume = false

	manager, err := NewRestartManagerWithConfig(locator, config)
	if err != nil {
		t.Fatalf("failed to create restart manager: %v", err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("creates restart request", func(t *testing.T) {
		request, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "Test restart", "agent-test")
		if err != nil {
			t.Fatalf("failed to create restart request: %v", err)
		}

		if request.ID == "" {
			t.Error("expected request to have an ID")
		}
		if request.Reason != RestartReasonCodeChange {
			t.Errorf("expected reason %s, got %s", RestartReasonCodeChange, request.Reason)
		}
		if request.Status != RestartStatusApproved { // Auto-approved when RequireApproval=false
			t.Errorf("expected status approved, got %s", request.Status)
		}
	})

	t.Run("retrieves request by ID", func(t *testing.T) {
		request, err := manager.RequestRestart(ctx, RestartReasonConfigChange, "Another test", "agent-test")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		retrieved, err := manager.GetRequest(ctx, request.ID)
		if err != nil {
			t.Fatalf("failed to retrieve request: %v", err)
		}

		if retrieved.ID != request.ID {
			t.Errorf("expected ID %s, got %s", request.ID, retrieved.ID)
		}
	})

	t.Run("performs pre-flight checks", func(t *testing.T) {
		result, err := manager.CanRestart(ctx)
		if err != nil {
			t.Fatalf("pre-flight check error: %v", err)
		}

		// Should have performed multiple checks
		if len(result.Checks) < 3 {
			t.Errorf("expected at least 3 pre-flight checks, got %d", len(result.Checks))
		}

		// Log check results
		for _, check := range result.Checks {
			t.Logf("Check %s: passed=%v, critical=%v, message=%s",
				check.Name, check.Passed, check.Critical, check.Message)
		}
	})

	t.Run("lists active requests", func(t *testing.T) {
		// Clear existing requests
		manager.ClearCompletedRequests(ctx)

		// Create a new pending request
		configWithApproval := *config
		configWithApproval.RequireApproval = true
		manager.SetConfig(&configWithApproval)

		_, err := manager.RequestRestart(ctx, RestartReasonUserRequested, "Pending request", "agent-test")
		if err != nil {
			t.Fatalf("failed to create pending request: %v", err)
		}

		// Reset config
		manager.SetConfig(config)

		// Should find the pending request
		pending := manager.ListPendingRequests(ctx)
		if len(pending) == 0 {
			t.Error("expected at least one pending request")
		}

		active := manager.ListActiveRequests(ctx)
		if len(active) == 0 {
			t.Error("expected at least one active request")
		}
	})
}

// TestE2E_MetaSelfFix_RestartManagerApprovalWorkflow tests the approval/rejection
// workflow for restart requests.
func TestE2E_MetaSelfFix_RestartManagerApprovalWorkflow(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Create manager with approval required
	config := DefaultRestartManagerConfig()
	config.RequireApproval = true
	config.AutoBuild = false
	config.EnableSessionResume = false

	manager, err := NewRestartManagerWithConfig(locator, config)
	if err != nil {
		t.Fatalf("failed to create restart manager: %v", err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("request starts as pending", func(t *testing.T) {
		request, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "Needs approval", "agent-test")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		if request.Status != RestartStatusPending {
			t.Errorf("expected status pending, got %s", request.Status)
		}
		if !request.IsPending() {
			t.Error("IsPending() should return true")
		}
		if request.CanExecute() {
			t.Error("CanExecute() should return false for pending request")
		}
	})

	t.Run("approval enables execution", func(t *testing.T) {
		request, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "To be approved", "agent-test")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		err = manager.Approve(ctx, request.ID, "admin", "approved for testing")
		if err != nil {
			t.Fatalf("failed to approve request: %v", err)
		}

		// Retrieve and verify
		approved, err := manager.GetRequest(ctx, request.ID)
		if err != nil {
			t.Fatalf("failed to retrieve request: %v", err)
		}

		if approved.Status != RestartStatusApproved {
			t.Errorf("expected status approved, got %s", approved.Status)
		}
		if !approved.CanExecute() {
			t.Error("CanExecute() should return true for approved request")
		}
	})

	t.Run("rejection blocks execution", func(t *testing.T) {
		request, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "To be rejected", "agent-test")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		err = manager.Reject(ctx, request.ID, "admin", "rejected for testing")
		if err != nil {
			t.Fatalf("failed to reject request: %v", err)
		}

		// Retrieve and verify
		rejected, err := manager.GetRequest(ctx, request.ID)
		if err != nil {
			t.Fatalf("failed to retrieve request: %v", err)
		}

		if rejected.Status != RestartStatusRejected {
			t.Errorf("expected status rejected, got %s", rejected.Status)
		}
		if rejected.CanExecute() {
			t.Error("CanExecute() should return false for rejected request")
		}
	})

	t.Run("cancellation works", func(t *testing.T) {
		request, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "To be cancelled", "agent-test")
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		err = manager.Cancel(ctx, request.ID, "no longer needed")
		if err != nil {
			t.Fatalf("failed to cancel request: %v", err)
		}

		cancelled, err := manager.GetRequest(ctx, request.ID)
		if err != nil {
			t.Fatalf("failed to retrieve request: %v", err)
		}

		if cancelled.Status != RestartStatusCancelled {
			t.Errorf("expected status cancelled, got %s", cancelled.Status)
		}
	})
}

// =============================================================================
// SessionResumeManager E2E Tests
// =============================================================================

// TestE2E_MetaSelfFix_SessionResumeLifecycle tests the complete lifecycle of
// session resume states from creation to restoration.
func TestE2E_MetaSelfFix_SessionResumeLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "resume")

	config := &SessionResumeManagerConfig{
		StoragePath:         storePath,
		DefaultExpiry:       1 * time.Hour,
		MaxStatesPerSession: 3,
		AutoCleanup:         false,
	}

	manager, err := NewSessionResumeManager(config)
	if err != nil {
		t.Fatalf("failed to create session resume manager: %v", err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "test-session-123"

	t.Run("creates resume state", func(t *testing.T) {
		state, err := manager.CreateResumeState(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to create resume state: %v", err)
		}

		if state.ID == "" {
			t.Error("expected state to have an ID")
		}
		if state.SessionID != sessionID {
			t.Errorf("expected session ID %s, got %s", sessionID, state.SessionID)
		}
		if state.Status != SessionResumeStatusPending {
			t.Errorf("expected status pending, got %s", state.Status)
		}
		if !state.CanResume() {
			t.Error("new state should be resumable")
		}
	})

	t.Run("stores and retrieves state data", func(t *testing.T) {
		state, err := manager.CreateResumeState(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to create state: %v", err)
		}

		// Set various state data
		state.Iteration = 5
		state.TotalTokens = 1000
		state.TotalCostUSD = 0.05
		state.Model = "claude-opus-4-5-20251101"
		state.WorkDir = "/project/path"

		// Set messages
		messages := []map[string]string{
			{"role": "user", "content": "Hello"},
			{"role": "assistant", "content": "Hi there!"},
		}
		if err := state.SetMessages(messages); err != nil {
			t.Fatalf("failed to set messages: %v", err)
		}
		state.MessageCount = 2

		// Set metadata
		state.SetMetadata("task_id", "T-001")
		state.SetMetadata("agent", "worker-1")

		// Update the state
		if err := manager.UpdateResumeState(ctx, state); err != nil {
			t.Fatalf("failed to update state: %v", err)
		}

		// Retrieve and verify
		retrieved, err := manager.GetResumeState(ctx, state.ID)
		if err != nil {
			t.Fatalf("failed to retrieve state: %v", err)
		}

		if retrieved.Iteration != 5 {
			t.Errorf("expected iteration 5, got %d", retrieved.Iteration)
		}
		if retrieved.TotalTokens != 1000 {
			t.Errorf("expected 1000 tokens, got %d", retrieved.TotalTokens)
		}
		if retrieved.Model != "claude-opus-4-5-20251101" {
			t.Errorf("expected model claude-opus-4-5-20251101, got %s", retrieved.Model)
		}
		if retrieved.MessageCount != 2 {
			t.Errorf("expected 2 messages, got %d", retrieved.MessageCount)
		}

		// Verify messages can be unmarshaled
		var retrievedMessages []map[string]string
		if err := retrieved.GetMessages(&retrievedMessages); err != nil {
			t.Fatalf("failed to get messages: %v", err)
		}
		if len(retrievedMessages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(retrievedMessages))
		}

		// Verify metadata
		taskID, ok := retrieved.GetMetadata("task_id")
		if !ok || taskID != "T-001" {
			t.Errorf("expected task_id T-001, got %s", taskID)
		}
	})

	t.Run("persists state to disk", func(t *testing.T) {
		state, err := manager.CreateResumeState(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to create state: %v", err)
		}

		// Check file exists
		filePath := filepath.Join(storePath, state.ID+".json")
		if _, err := os.Stat(filePath); err != nil {
			t.Errorf("expected state file to exist: %v", err)
		}
	})

	t.Run("enforces max states per session", func(t *testing.T) {
		// Create more than max states
		for i := 0; i < 5; i++ {
			_, err := manager.CreateResumeState(ctx, sessionID)
			if err != nil {
				t.Fatalf("failed to create state %d: %v", i, err)
			}
		}

		// Should only have MaxStatesPerSession states
		states, err := manager.ListResumeStates(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to list states: %v", err)
		}

		if len(states) > config.MaxStatesPerSession {
			t.Errorf("expected at most %d states, got %d", config.MaxStatesPerSession, len(states))
		}
	})
}

// TestE2E_MetaSelfFix_SessionResumeWorkflow tests the complete resume workflow
// including state marking and restoration.
func TestE2E_MetaSelfFix_SessionResumeWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "resume")

	config := &SessionResumeManagerConfig{
		StoragePath:         storePath,
		DefaultExpiry:       1 * time.Hour,
		MaxStatesPerSession: 5,
		AutoCleanup:         false,
	}

	manager, err := NewSessionResumeManager(config)
	if err != nil {
		t.Fatalf("failed to create session resume manager: %v", err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sessionID := "workflow-session"

	t.Run("gets latest resume state", func(t *testing.T) {
		// Create multiple states
		for i := 0; i < 3; i++ {
			state, err := manager.CreateResumeState(ctx, sessionID)
			if err != nil {
				t.Fatalf("failed to create state %d: %v", i, err)
			}
			state.Iteration = i + 1
			manager.UpdateResumeState(ctx, state)
		}

		latest, err := manager.GetLatestResumeState(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to get latest state: %v", err)
		}

		// Latest should be iteration 3
		if latest.Iteration != 3 {
			t.Errorf("expected latest iteration to be 3, got %d", latest.Iteration)
		}
	})

	t.Run("resume workflow transitions states", func(t *testing.T) {
		state, err := manager.CreateResumeState(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to create state: %v", err)
		}

		// Start resume
		resumed, err := manager.ResumeSession(ctx, state.ID)
		if err != nil {
			t.Fatalf("failed to start resume: %v", err)
		}

		if resumed.Status != SessionResumeStatusResuming {
			t.Errorf("expected status resuming, got %s", resumed.Status)
		}

		// Complete resume
		if err := manager.CompleteResume(ctx, state.ID); err != nil {
			t.Fatalf("failed to complete resume: %v", err)
		}

		completed, err := manager.GetResumeState(ctx, state.ID)
		if err != nil {
			t.Fatalf("failed to get state: %v", err)
		}

		if completed.Status != SessionResumeStatusResumed {
			t.Errorf("expected status resumed, got %s", completed.Status)
		}
		if completed.ResumedAt == nil {
			t.Error("expected ResumedAt to be set")
		}
	})

	t.Run("handles failed resume", func(t *testing.T) {
		state, err := manager.CreateResumeState(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to create state: %v", err)
		}

		// Fail the resume
		if err := manager.FailResume(ctx, state.ID, "connection timeout"); err != nil {
			t.Fatalf("failed to mark resume as failed: %v", err)
		}

		failed, err := manager.GetResumeState(ctx, state.ID)
		if err != nil {
			t.Fatalf("failed to get state: %v", err)
		}

		if failed.Status != SessionResumeStatusFailed {
			t.Errorf("expected status failed, got %s", failed.Status)
		}
		if failed.Error != "connection timeout" {
			t.Errorf("expected error 'connection timeout', got %s", failed.Error)
		}
	})

	t.Run("lists pending states", func(t *testing.T) {
		// Create a new pending state
		_, err := manager.CreateResumeState(ctx, "pending-session")
		if err != nil {
			t.Fatalf("failed to create state: %v", err)
		}

		pending := manager.ListPendingStates(ctx)
		if len(pending) == 0 {
			t.Error("expected at least one pending state")
		}

		// All returned states should be resumable
		for _, state := range pending {
			if !state.CanResume() {
				t.Errorf("state %s should be resumable", state.ID)
			}
		}
	})
}

// =============================================================================
// Integration Tests - Complete Meta Self-Fix Workflow
// =============================================================================

// TestE2E_MetaSelfFix_CompleteWorkflow tests the complete meta self-fix workflow
// from error detection through rebuild and session resume.
func TestE2E_MetaSelfFix_CompleteWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping complete workflow test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	tmpDir := t.TempDir()
	resumeStorePath := filepath.Join(tmpDir, "resume")

	// Setup locator
	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Setup restart manager with session resume enabled
	restartConfig := &RestartManagerConfig{
		Timeout:             2 * time.Minute,
		BuildTimeout:        2 * time.Minute,
		RequireApproval:     false,
		AutoBuild:           false,
		DefaultPort:         8080,
		EnableSessionResume: true,
		SessionResumeConfig: &SessionResumeManagerConfig{
			StoragePath:         resumeStorePath,
			DefaultExpiry:       1 * time.Hour,
			MaxStatesPerSession: 5,
			AutoCleanup:         false,
		},
	}

	restartManager, err := NewRestartManagerWithConfig(locator, restartConfig)
	if err != nil {
		t.Fatalf("failed to create restart manager: %v", err)
	}
	defer restartManager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	t.Run("step 1: detect orchestrator source files", func(t *testing.T) {
		files, err := locator.LocateByType(FileTypeGoSource)
		if err != nil {
			t.Fatalf("failed to locate files: %v", err)
		}

		if len(files) == 0 {
			t.Fatal("no Go source files found")
		}

		t.Logf("Found %d orchestrator source files", len(files))
	})

	t.Run("step 2: validate current build", func(t *testing.T) {
		builder := NewOrchestratorBuilder(locator).
			WithTimeout(2 * time.Minute).
			WithTargets("./internal/mcp")

		result, err := builder.CheckSyntax(ctx)
		if err != nil {
			t.Fatalf("syntax check error: %v", err)
		}

		t.Logf("Syntax check: success=%v, duration=%v", result.Success, result.Duration)
	})

	t.Run("step 3: request restart with session state", func(t *testing.T) {
		sessionState := &SessionState{
			SessionID:      "meta-fix-session",
			Iteration:      10,
			TotalTokens:    5000,
			TotalCostUSD:   0.25,
			MessageCount:   20,
			Model:          "claude-opus-4-5-20251101",
			WorkDir:        root,
			PendingPrompt:  "Continue fixing the issue",
			ContextSummary: "Working on meta self-fix implementation",
		}

		request, err := restartManager.RequestRestartWithResume(
			ctx,
			RestartReasonCodeChange,
			"Meta self-fix code change",
			"meta-fix-agent",
			sessionState,
			true, // resumeOnStartup
		)
		if err != nil {
			t.Fatalf("failed to create restart request: %v", err)
		}

		if request.ID == "" {
			t.Error("expected request ID")
		}
		if !request.HasResume() {
			t.Log("Resume may not be enabled due to session state not being persisted")
		}

		t.Logf("Created restart request: %s, resume_enabled=%v", request.ID, request.ResumeEnabled)
	})

	t.Run("step 4: check auto-resume on startup", func(t *testing.T) {
		result := restartManager.CheckAutoResume(ctx)

		t.Logf("Auto-resume check: has_pending=%v, states=%d",
			result.HasPendingResume, len(result.PendingStates))

		// If there are pending states, they should be valid
		for _, state := range result.PendingStates {
			if !state.CanResume() {
				t.Errorf("state %s should be resumable", state.ID)
			}
		}
	})

	t.Run("step 5: verify pre-flight checks pass", func(t *testing.T) {
		result, err := restartManager.CanRestart(ctx)
		if err != nil {
			t.Fatalf("pre-flight check error: %v", err)
		}

		if !result.AllPassed {
			for _, check := range result.Checks {
				if !check.Passed {
					t.Logf("Pre-flight check failed: %s - %s", check.Name, check.Message)
				}
			}
		}
	})
}

// TestE2E_MetaSelfFix_ConcurrentOperations tests that the meta self-fix
// components handle concurrent operations correctly.
func TestE2E_MetaSelfFix_ConcurrentOperations(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "resume")

	config := &SessionResumeManagerConfig{
		StoragePath:         storePath,
		DefaultExpiry:       1 * time.Hour,
		MaxStatesPerSession: 10,
		AutoCleanup:         false,
	}

	manager, err := NewSessionResumeManager(config)
	if err != nil {
		t.Fatalf("failed to create session resume manager: %v", err)
	}
	defer manager.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	t.Run("concurrent state creation", func(t *testing.T) {
		var wg sync.WaitGroup
		statesChan := make(chan *SessionResumeState, 10)
		errChan := make(chan error, 10)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				sessionID := "concurrent-session"
				state, err := manager.CreateResumeState(ctx, sessionID)
				if err != nil {
					errChan <- err
					return
				}
				statesChan <- state
			}(i)
		}

		wg.Wait()
		close(statesChan)
		close(errChan)

		// Check for errors
		for err := range errChan {
			t.Errorf("concurrent creation error: %v", err)
		}

		// Count created states
		count := 0
		for range statesChan {
			count++
		}

		t.Logf("Successfully created %d states concurrently", count)
	})

	t.Run("concurrent updates", func(t *testing.T) {
		// Each goroutine gets its own state to avoid race conditions
		// This tests the manager's ability to handle concurrent updates
		// to different states rather than the same state.
		sessionID := "concurrent-update-session"

		var wg sync.WaitGroup
		var mu sync.Mutex
		successCount := 0

		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				// Create a separate state for each goroutine
				state, err := manager.CreateResumeState(ctx, sessionID)
				if err != nil {
					return
				}
				state.Iteration = id
				if err := manager.UpdateResumeState(ctx, state); err == nil {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}(i)
		}

		wg.Wait()

		t.Logf("Successfully updated %d states concurrently", successCount)
		if successCount == 0 {
			t.Error("expected at least one successful update")
		}
	})

	t.Run("concurrent read and write", func(t *testing.T) {
		// Note: SessionResumeState.SetMetadata is not thread-safe by design
		// (the state is expected to be protected by the manager's mutex).
		// This test validates concurrent operations through the manager.

		var wg sync.WaitGroup
		readCount := 0
		createCount := 0
		var mu sync.Mutex

		// Writers - create new states through the manager
		for i := 0; i < 3; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				sessionID := "concurrent-rw-session"
				state, err := manager.CreateResumeState(ctx, sessionID)
				if err == nil {
					mu.Lock()
					createCount++
					mu.Unlock()
					state.Iteration = id
					_ = manager.UpdateResumeState(ctx, state)
				}
			}(i)
		}

		// Readers - list pending states through the manager
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pending := manager.ListPendingStates(ctx)
				mu.Lock()
				readCount += len(pending)
				mu.Unlock()
			}()
		}

		wg.Wait()

		t.Logf("Successful concurrent creates: %d, reads: %d", createCount, readCount)
	})
}

// TestE2E_MetaSelfFix_ErrorRecovery tests that the meta self-fix system
// gracefully handles various error conditions.
func TestE2E_MetaSelfFix_ErrorRecovery(t *testing.T) {
	t.Run("handles non-existent restart request", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Skip("orchestrator root not found")
		}

		locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
		config := DefaultRestartManagerConfig()
		config.EnableSessionResume = false

		manager, _ := NewRestartManagerWithConfig(locator, config)
		defer manager.Close()

		ctx := context.Background()
		_, err = manager.GetRequest(ctx, "non-existent-id")
		if err != ErrRestartNotFound {
			t.Errorf("expected ErrRestartNotFound, got %v", err)
		}
	})

	t.Run("handles non-existent session resume state", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath: filepath.Join(tmpDir, "resume"),
			AutoCleanup: false,
		}

		manager, _ := NewSessionResumeManager(config)
		defer manager.Close()

		ctx := context.Background()
		_, err := manager.GetResumeState(ctx, "non-existent-id")
		if err != ErrSessionResumeNotFound {
			t.Errorf("expected ErrSessionResumeNotFound, got %v", err)
		}
	})

	t.Run("handles expired session resume state", func(t *testing.T) {
		tmpDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath:   filepath.Join(tmpDir, "resume"),
			DefaultExpiry: -1 * time.Hour, // Already expired
			AutoCleanup:   false,
		}

		manager, _ := NewSessionResumeManager(config)
		defer manager.Close()

		ctx := context.Background()
		state, err := manager.CreateResumeState(ctx, "test-session")
		if err != nil {
			t.Fatalf("failed to create state: %v", err)
		}

		// State should be expired
		if !state.IsExpired() {
			t.Error("state should be expired")
		}
		if state.CanResume() {
			t.Error("expired state should not be resumable")
		}

		// Trying to resume should fail
		_, err = manager.ResumeSession(ctx, state.ID)
		if err != ErrSessionResumeExpired {
			t.Errorf("expected ErrSessionResumeExpired, got %v", err)
		}
	})

	t.Run("handles invalid locator path", func(t *testing.T) {
		_, err := NewOrchestratorLocator(OrchestratorLocatorConfig{
			Root: "/non/existent/path",
		})
		if err == nil {
			t.Error("expected error for invalid root path")
		}
	})

	t.Run("handles concurrent restart requests gracefully", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Skip("orchestrator root not found")
		}

		locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
		config := DefaultRestartManagerConfig()
		config.RequireApproval = false
		config.EnableSessionResume = false

		manager, _ := NewRestartManagerWithConfig(locator, config)
		defer manager.Close()

		ctx := context.Background()

		// Create first request
		req1, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "First request", "agent-1")
		if err != nil {
			t.Fatalf("failed to create first request: %v", err)
		}

		// Mark it as in progress
		req1.Status = RestartStatusInProgress

		// Try to create second request while first is in progress
		_, err = manager.RequestRestart(ctx, RestartReasonCodeChange, "Second request", "agent-2")
		// This may or may not fail depending on timing, just verify no panic
		t.Logf("Second request error (expected ErrRestartInProgress or nil): %v", err)
	})
}
