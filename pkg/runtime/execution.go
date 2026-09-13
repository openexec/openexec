package runtime

import (
	"context"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

// StageExecutor is an admitted execution boundary, not another task scheduler.
type StageExecutor = blueprint.StageExecutor
type Stage = blueprint.Stage
type StageInput = blueprint.StageInput
type StageResult = blueprint.StageResult

const (
	StageStatusCompleted   = types.StageStatusCompleted
	StageStatusFailed      = types.StageStatusFailed
	StageTypeDeterministic = types.StageTypeDeterministic
)

// VerificationCommandFailure classifies an actually observed command error.
// Cancellation, launch and transport failures cannot become repair evidence.
func VerificationCommandFailure(ctx context.Context, name string, err error) error {
	return gates.NewCommandFailure(ctx, name, err)
}
