package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

type terminalExecutor func(context.Context, *blueprint.Stage, *blueprint.StageInput) (*blueprint.StageResult, error)

func (f terminalExecutor) Execute(c context.Context, s *blueprint.Stage, i *blueprint.StageInput) (*blueprint.StageResult, error) {
	return f(c, s, i)
}

func TestAdmittedTerminalBindingAndReceiptReset(t *testing.T) {
	for _, mode := range []string{"matched", "mutated_input", "agentic", "cancelled", "success_clears_receipt", "exit_zero", "exit_125", "mixed_timeout", "forged_success"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := blueprint.NewStageInput("A", "task", t.TempDir())
			input.TaskAttempt = 1
			input.StageAttempt = 2
			stage := &blueprint.Stage{Name: "test", Type: types.StageTypeDeterministic}
			if mode == "agentic" {
				stage.Type = types.StageTypeAgentic
			}
			evidence := &gateRunnerAction{}
			wrapper := admittedStageExecutor{evidence: evidence, executor: terminalExecutor(func(ctx context.Context, _ *blueprint.Stage, i *blueprint.StageInput) (*blueprint.StageResult, error) {
				if mode == "mutated_input" {
					i.TaskAttempt++
				}
				code := 1
				if mode == "exit_zero" {
					code = 0
				}
				if mode == "exit_125" {
					code = 125
				}
				b := gates.TerminalBinding{TaskID: i.RunID, Stage: "test", TaskAttempt: i.TaskAttempt, StageAttempt: i.StageAttempt, Source: "deterministic-runner"}
				err := gates.NewTerminalFailure(ctx, b, "id", func(context.Context, string) (gates.TerminalCompletion, error) {
					return gates.TerminalCompletion{ID: "id", Binding: b, Outcome: "exited", ExitCode: &code}, nil
				})
				if mode == "cancelled" {
					cancel()
				}
				if mode == "mixed_timeout" {
					err = errors.Join(err, context.DeadlineExceeded)
				}
				if mode == "forged_success" {
					return &blueprint.StageResult{StageName: "test", Status: types.StageStatusCompleted, Artifacts: gates.VerificationFailureArtifacts(err)}, nil
				}
				if err == nil {
					return &blueprint.StageResult{StageName: "test", Status: types.StageStatusCompleted}, nil
				}
				return nil, err
			})}
			wrapper.Execute(ctx, stage, input)
			if mode == "success_clears_receipt" {
				if evidence.receipt == nil {
					t.Fatal("no initial receipt")
				}
				wrapper.executor = terminalExecutor(func(context.Context, *blueprint.Stage, *blueprint.StageInput) (*blueprint.StageResult, error) {
					return &blueprint.StageResult{StageName: "test", Status: types.StageStatusCompleted}, nil
				})
				wrapper.Execute(ctx, stage, input)
			}
			bp := &blueprint.Blueprint{Stages: map[string]*blueprint.Stage{"test": stage}}
			run := &blueprint.Run{Results: []*blueprint.StageResult{{StageName: "test", Status: types.StageStatusFailed}}}
			if (evidence.terminalEvidence(bp, run) != nil) != (mode == "matched" || mode == "exit_125") {
				t.Fatalf("wrong evidence for %s", mode)
			}
		})
	}
}

// A legacy caller may omit input; it must not authorize a bound terminal receipt.
func TestAdmittedTerminalWithoutInputRefusesReceipt(t *testing.T) {
	ctx := context.Background()
	binding := gates.TerminalBinding{TaskID: "A", Stage: "test", TaskAttempt: 1, StageAttempt: 1, Source: "deterministic-runner"}
	code := 1
	failure := gates.NewTerminalFailure(ctx, binding, "id", func(context.Context, string) (gates.TerminalCompletion, error) {
		return gates.TerminalCompletion{ID: "id", Binding: binding, Outcome: "exited", ExitCode: &code}, nil
	})
	receipt := &gateRunnerAction{}
	wrapper := admittedStageExecutor{evidence: receipt, executor: terminalExecutor(func(context.Context, *blueprint.Stage, *blueprint.StageInput) (*blueprint.StageResult, error) {
		return nil, failure
	})}
	_, err := wrapper.Execute(ctx, &blueprint.Stage{Name: "test", Type: types.StageTypeDeterministic}, nil)
	if err == nil || receipt.receipt != nil {
		t.Fatal("missing native binding authorized structured evidence")
	}
}
