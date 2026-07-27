package blueprint

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/types"
)

// A stage naming an action the registry does not have reached the same empty case
// as a stage with no commands, so a missing gate implementation read as a
// satisfied gate. The no-commands case itself is covered by
// TestDefaultExecutor_DeterministicStage_NoCommands.
func TestDeterministicStageWithUnregisteredActionFails(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewDefaultExecutor(tmpDir)
	stage := &Stage{Name: "verify", Type: types.StageTypeDeterministic, Action: "run_gates"}

	result, err := executor.Execute(context.Background(), stage, NewStageInput("run-1", "task", tmpDir))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status == types.StageStatusCompleted {
		t.Fatalf("a stage with an unavailable action passed: %q", result.Output)
	}
	if !strings.Contains(result.Error, "not registered") {
		t.Fatalf("error should name the missing action, got %q", result.Error)
	}
}

// Commands that do run still decide the outcome, so the change cannot be read as
// "deterministic stages always fail".
func TestDeterministicStageWithCommandsStillPasses(t *testing.T) {
	tmpDir := t.TempDir()
	executor := NewDefaultExecutor(tmpDir)
	stage := &Stage{Name: "lint", Type: types.StageTypeDeterministic, Commands: []string{"true"}}

	result, err := executor.Execute(context.Background(), stage, NewStageInput("run-1", "task", tmpDir))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != types.StageStatusCompleted {
		t.Fatalf("a stage with a passing command failed: %q", result.Error)
	}
}
