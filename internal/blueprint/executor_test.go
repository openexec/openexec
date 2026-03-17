package blueprint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/types"
)

func TestDefaultExecutor_DeterministicStage_Success(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewDefaultExecutor(tmpDir)

	stage := &Stage{
		Name:     "test_stage",
		Type:     types.StageTypeDeterministic,
		Commands: []string{"echo hello", "echo world"},
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	result, err := executor.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != types.StageStatusCompleted {
		t.Errorf("Expected status %s, got %s", types.StageStatusCompleted, result.Status)
	}

	if result.Output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestDefaultExecutor_DeterministicStage_CommandFailure(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewDefaultExecutor(tmpDir)

	stage := &Stage{
		Name:     "failing_stage",
		Type:     types.StageTypeDeterministic,
		Commands: []string{"exit 1"},
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	result, err := executor.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute returned error (should return result with failed status): %v", err)
	}

	if result.Status != types.StageStatusFailed {
		t.Errorf("Expected status %s, got %s", types.StageStatusFailed, result.Status)
	}

	if result.Error == "" {
		t.Error("Expected error message for failed command")
	}
}

func TestDefaultExecutor_DeterministicStage_Timeout(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewDefaultExecutor(tmpDir)
	executor.Timeout = 100 * time.Millisecond

	stage := &Stage{
		Name:     "slow_stage",
		Type:     types.StageTypeDeterministic,
		Commands: []string{"sleep 10"},
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	result, err := executor.Execute(ctx, stage, input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != types.StageStatusFailed {
		t.Errorf("Expected status %s, got %s", types.StageStatusFailed, result.Status)
	}
}

func TestDefaultExecutor_DeterministicStage_NoCommands(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewDefaultExecutor(tmpDir)

	stage := &Stage{
		Name:     "empty_stage",
		Type:     types.StageTypeDeterministic,
		Commands: []string{},
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	result, err := executor.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != types.StageStatusCompleted {
		t.Errorf("Expected status %s, got %s", types.StageStatusCompleted, result.Status)
	}
}

func TestDefaultExecutor_DeterministicStage_MultipleCommands_StopsOnFailure(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a marker file that should be created by first command
	markerPath := filepath.Join(tmpDir, "marker.txt")

	executor := NewDefaultExecutor(tmpDir)

	stage := &Stage{
		Name: "multi_stage",
		Type: types.StageTypeDeterministic,
		Commands: []string{
			"echo marker > marker.txt",
			"exit 1",          // This should fail
			"echo never > never.txt", // This should not run
		},
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	result, err := executor.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	if result.Status != types.StageStatusFailed {
		t.Errorf("Expected status %s, got %s", types.StageStatusFailed, result.Status)
	}

	// First command should have run
	if _, err := os.Stat(markerPath); os.IsNotExist(err) {
		t.Error("First command should have created marker.txt")
	}

	// Third command should not have run
	neverPath := filepath.Join(tmpDir, "never.txt")
	if _, err := os.Stat(neverPath); !os.IsNotExist(err) {
		t.Error("Third command should not have run after failure")
	}
}

func TestDefaultExecutor_AgenticStage_NoRunner(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewDefaultExecutor(tmpDir)
	// AgenticRunner is nil by default

	stage := &Stage{
		Name: "agentic_stage",
		Type: types.StageTypeAgentic,
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	result, err := executor.Execute(context.Background(), stage, input)
	if err == nil {
		t.Error("Expected error when AgenticRunner is not configured")
	}

	if result.Status != types.StageStatusFailed {
		t.Errorf("Expected status %s, got %s", types.StageStatusFailed, result.Status)
	}
}

func TestDefaultExecutor_AgenticStage_WithSimpleRunner(t *testing.T) {
	tmpDir := t.TempDir()

	executor := NewDefaultExecutor(tmpDir)
	executor.AgenticRunner = &SimpleAgenticRunner{
		RunFunc: func(ctx context.Context, stage *Stage, input *StageInput) (string, map[string]string, error) {
			return "agentic output", map[string]string{"patch": "abc123"}, nil
		},
	}

	stage := &Stage{
		Name: "agentic_stage",
		Type: types.StageTypeAgentic,
	}

	input := NewStageInput("run-1", "implement feature X", tmpDir)

	result, err := executor.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Status != types.StageStatusCompleted {
		t.Errorf("Expected status %s, got %s", types.StageStatusCompleted, result.Status)
	}

	if result.Output != "agentic output" {
		t.Errorf("Expected output 'agentic output', got %q", result.Output)
	}

	if result.Artifacts["patch"] != "abc123" {
		t.Errorf("Expected artifact patch=abc123, got %v", result.Artifacts)
	}
}

func TestDefaultExecutor_Callbacks(t *testing.T) {
	tmpDir := t.TempDir()

	var startedCmds []string
	var completedCmds []string

	executor := NewDefaultExecutor(tmpDir)
	executor.OnCommandStart = func(stage *Stage, cmd string) {
		startedCmds = append(startedCmds, cmd)
	}
	executor.OnCommandComplete = func(stage *Stage, cmd string, output string, err error) {
		completedCmds = append(completedCmds, cmd)
	}

	stage := &Stage{
		Name:     "callback_stage",
		Type:     types.StageTypeDeterministic,
		Commands: []string{"echo one", "echo two"},
	}

	input := NewStageInput("run-1", "test task", tmpDir)

	_, err := executor.Execute(context.Background(), stage, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(startedCmds) != 2 {
		t.Errorf("Expected 2 started commands, got %d", len(startedCmds))
	}

	if len(completedCmds) != 2 {
		t.Errorf("Expected 2 completed commands, got %d", len(completedCmds))
	}
}

func TestBuildAgenticPrompt(t *testing.T) {
	stage := &Stage{
		Name:        "implement",
		Description: "Implement the feature",
		Prompt:      "Custom instructions here",
	}

	input := NewStageInput("run-1", "Add user authentication", "/project")
	input.PreviousStages = []*StageResult{
		{StageName: "gather_context", Status: types.StageStatusCompleted, Output: "Found 5 relevant files"},
	}
	input.ContextPack["src/auth.go"] = "contents..."

	prompt := buildAgenticPrompt(stage, input)

	// Check key elements
	if !containsString(prompt, "implement") {
		t.Error("Prompt should contain stage name")
	}
	if !containsString(prompt, "Add user authentication") {
		t.Error("Prompt should contain task description")
	}
	if !containsString(prompt, "gather_context") {
		t.Error("Prompt should contain previous stage info")
	}
	if !containsString(prompt, "src/auth.go") {
		t.Error("Prompt should contain context file paths")
	}
	if !containsString(prompt, "phase-complete") {
		t.Error("Prompt should instruct to emit phase-complete signal")
	}
}

func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || containsString(s[1:], substr)))
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"ab", 5, "ab"},
		{"", 5, ""},
	}

	for _, tc := range tests {
		got := truncate(tc.input, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
		}
	}
}
