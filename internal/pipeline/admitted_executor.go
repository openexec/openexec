package pipeline

import (
	"context"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
)

// Capture only typed execution-boundary evidence, never worker artifacts.
type admittedStageExecutor struct {
	executor blueprint.StageExecutor
	evidence *gateRunnerAction
}

func (a admittedStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	result, err := a.executor.Execute(ctx, stage, input)
	a.evidence.receipt = gates.VerificationFailureArtifacts(err)
	return result, err
}
