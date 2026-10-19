package pipeline

import (
	"context"
	"github.com/openexec/openexec/internal/loop"
	"sync"

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
	result, err := a.executor.Execute(ctx, stage, input)
	a.evidence.receipt = gates.VerificationFailureArtifacts(err)
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

// activityStageExecutor scopes reporting to one Execute call. Late provider
// callbacks cannot refresh or exempt a successor stage.
type activityStageExecutor struct {
	executor blueprint.StageExecutor
	emit     func(loop.Event)
}

func (a activityStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	scoped := *input
	var mu sync.Mutex
	active := true
	scoped.OnExecutionActivity = func(kind string) {
		mu.Lock()
		defer mu.Unlock()
		if active && ctx.Err() == nil {
			a.emit(loop.Event{Type: loop.EventExecutionActivity, FWUID: input.RunID, StageName: stage.Name, ExecutionActivity: kind})
		}
	}
	defer func() { mu.Lock(); active = false; mu.Unlock() }()
	return a.executor.Execute(ctx, stage, &scoped)
}
