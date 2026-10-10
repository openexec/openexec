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

// VerificationCommandFailureWithOutput also records the command that ran and
// the tail of its output, which is what a repair needs to reproduce it.
func VerificationCommandFailureWithOutput(ctx context.Context, name string, err error, command, output string) error {
	return gates.NewCommandFailureWithOutput(ctx, name, err, command, output)
}

// VerificationCommandFailureWithEvidence attaches the reference returned by
// RetainCommandEvidence to an observed verification failure. Admitted executors
// must retain evidence before calling this function. References do not classify
// cancellation, launch or transport failures as repairable check failures.
func VerificationCommandFailureWithEvidence(ctx context.Context, name string, err error, hash, path string) error {
	return gates.CommandFailureWithEvidence(ctx, name, err, hash, path)
}

// Structured completion types cross the same admitted verification boundary.
type TerminalBinding = gates.TerminalBinding
type TerminalCompletion = gates.TerminalCompletion
type TerminalLoader = gates.TerminalLoader

// VerificationTerminalFailure consumes an immutable completion through a trusted
// loader owned by the admitting caller. Never implement the loader with worker
// output or an unauthenticated terminal record. The native pipeline independently
// matches the returned typed evidence to the executing task/stage/attempt.
func VerificationTerminalFailure(ctx context.Context, expected TerminalBinding, id string, load TerminalLoader) error {
	return gates.NewTerminalFailure(ctx, expected, id, load)
}
