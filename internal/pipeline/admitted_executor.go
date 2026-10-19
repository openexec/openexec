package pipeline

import (
	"context"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/evidence"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

// Capture only typed execution-boundary evidence, never worker artifacts.
type admittedStageExecutor struct {
	executor blueprint.StageExecutor
	evidence *gateRunnerAction
}

func (a admittedStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	expected := gates.TerminalBinding{}
	if input != nil {
		expected = gates.TerminalBinding{TaskID: input.RunID, Stage: stage.Name, TaskAttempt: input.TaskAttempt, StageAttempt: input.StageAttempt, Source: "deterministic-runner"}
	}
	result, err := a.executor.Execute(ctx, stage, input)
	a.evidence.receipt = nil
	if ctx.Err() == nil {
		a.evidence.receipt = gates.VerificationFailureArtifactsForStage(err, expected)
	}
	if stage.Type != types.StageTypeDeterministic {
		return result, err
	}
	// Adapters retain raw command evidence privately before returning. Events and
	// repair descriptions receive only bounded, credential-redacted summaries.
	if result != nil {
		result.Output = evidence.Public(result.Output, nil)
		result.Diagnostics = evidence.Public(result.Diagnostics, nil)
		result.Error = evidence.Public(result.Error, nil)
	}
	if err != nil {
		err = publicExecutionError{err}
	}
	return result, err
}

// Preserve error identity/classification while keeping diagnostics out of public text.
type publicExecutionError struct{ error }

func (e publicExecutionError) Error() string { return evidence.Public(e.error.Error(), nil) }
func (e publicExecutionError) Unwrap() error { return e.error }
