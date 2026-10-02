package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/pkg/runtime"
)

func TestTerminalBoundary(t *testing.T) {
	binding := runtime.TerminalBinding{TaskID: "A", Stage: "test", TaskAttempt: 1, StageAttempt: 1, Source: "deterministic-runner"}
	for _, code := range []int{0, 1, 2, 125, 126, 127, 128, 255, -1} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			terminal := runtime.TerminalCompletion{ID: "completion", Binding: binding, Outcome: "exited", ExitCode: &code}
			err := runtime.VerificationTerminalFailure(context.Background(), binding, terminal.ID, func(context.Context, string) (runtime.TerminalCompletion, error) { return terminal, nil })
			artifacts := gates.VerificationFailureArtifacts(err)
			if (artifacts != nil) != (code > 0 && code < 126) {
				t.Fatalf("code=%d err=%v artifacts=%v", code, err, artifacts)
			}
			if code != 0 && err == nil {
				t.Fatal("nonzero exit passed")
			}
			if code == 0 && err != nil {
				t.Fatal(err)
			}
			if artifacts != nil && !gates.ValidateVerificationFailureArtifacts(artifacts) {
				t.Fatal("invalid artifacts")
			}
		})
	}
	for _, name := range []string{"task", "stage", "task_attempt", "stage_attempt", "source", "id", "missing_exit", "cancelled", "timed_out", "launch_error", "transport_error", "missing", "tampered", "loader_typed_failure", "nil_loader", "empty_binding", "context_cancelled", "empty_id", "blank_task", "blank_stage", "zero_task_attempt", "negative_task_attempt", "zero_stage_attempt", "negative_stage_attempt", "context_timeout", "cancel_during_load", "unknown_outcome"} {
		t.Run(name, func(t *testing.T) {
			code := 1
			terminal := runtime.TerminalCompletion{ID: "completion", Binding: binding, Outcome: "exited", ExitCode: &code}
			expected := binding
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var loadErr error
			switch name {
			case "task":
				terminal.Binding.TaskID = "B"
			case "stage":
				terminal.Binding.Stage = "lint"
			case "task_attempt":
				terminal.Binding.TaskAttempt++
			case "stage_attempt":
				terminal.Binding.StageAttempt++
			case "source":
				terminal.Binding.Source = "worker"
			case "id":
				terminal.ID = "other"
			case "missing_exit":
				terminal.ExitCode = nil
			case "cancelled", "timed_out", "launch_error", "transport_error":
				terminal.Outcome = name
			case "missing", "tampered":
				loadErr = errors.New(name)
			case "loader_typed_failure":
				loadErr = runtime.VerificationCommandFailure(ctx, "test", exec.Command("false").Run())
			case "empty_binding":
				expected = runtime.TerminalBinding{}
			case "context_cancelled":
				cancel()
			case "blank_task":
				expected.TaskID = " "
			case "blank_stage":
				expected.Stage = " "
			case "zero_task_attempt":
				expected.TaskAttempt = 0
			case "negative_task_attempt":
				expected.TaskAttempt = -1
			case "zero_stage_attempt":
				expected.StageAttempt = 0
			case "negative_stage_attempt":
				expected.StageAttempt = -1
			case "unknown_outcome":
				terminal.Outcome = "running"
			case "context_timeout":
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer stop()
			}
			switch name {
			case "blank_task", "blank_stage", "zero_task_attempt", "negative_task_attempt", "zero_stage_attempt", "negative_stage_attempt", "empty_binding":
				terminal.Binding = expected // Matching invalid values must still be refused.
			}

			var loader runtime.TerminalLoader = func(context.Context, string) (runtime.TerminalCompletion, error) {
				if name == "cancel_during_load" {
					cancel()
				}
				return terminal, loadErr
			}
			if name == "nil_loader" {
				loader = nil
			}
			id := "completion"
			if name == "empty_id" {
				id = " "
				terminal.ID = id
			}
			err := runtime.VerificationTerminalFailure(ctx, expected, id, loader)
			if err == nil || gates.VerificationFailureArtifacts(err) != nil {
				t.Fatalf("refusal authorized repair: %v", err)
			}
		})
	}
}

func TestTerminalMixedErrorsAndNativeBinding(t *testing.T) {
	code := 1
	b := runtime.TerminalBinding{TaskID: "A", Stage: "test", TaskAttempt: 1, StageAttempt: 1, Source: "deterministic-runner"}
	err := runtime.VerificationTerminalFailure(context.Background(), b, "id", func(context.Context, string) (runtime.TerminalCompletion, error) {
		return runtime.TerminalCompletion{ID: "id", Binding: b, Outcome: "exited", ExitCode: &code}, nil
	})
	if gates.VerificationFailureArtifacts(fmt.Errorf("wrapped: %w", err)) == nil {
		t.Fatal("lost typed evidence")
	}
	if gates.VerificationFailureArtifacts(errors.Join(err, errors.New("transport"))) != nil {
		t.Fatal("mixed error accepted")
	}
	for _, field := range []string{"task", "stage", "task_attempt", "stage_attempt", "source"} {
		t.Run(field, func(t *testing.T) {
			mismatch := b
			switch field {
			case "task":
				mismatch.TaskID = "B"
			case "stage":
				mismatch.Stage = "lint"
			case "task_attempt":
				mismatch.TaskAttempt++
			case "stage_attempt":
				mismatch.StageAttempt++
			case "source":
				mismatch.Source = "worker"
			}
			if gates.VerificationFailureArtifactsForStage(err, mismatch) != nil {
				t.Fatal("cross-boundary binding accepted")
			}
		})
	}
	if gates.VerificationFailureArtifactsForStage(err, b) == nil {
		t.Fatal("matched binding refused")
	}
}

func TestLocalCommandFailureCompatibility(t *testing.T) {
	for _, code := range []int{0, 1, 125, 126, 127, 128, 255} {
		t.Run(fmt.Sprintf("exit_%d", code), func(t *testing.T) {
			observed := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
			if code != 0 {
				exit, ok := observed.(*exec.ExitError)
				if !ok || exit.ExitCode() != code {
					t.Fatalf("expected actual ExitError %d, got %v", code, observed)
				}
			}
			err := runtime.VerificationCommandFailure(context.Background(), "test", observed)
			if (gates.VerificationFailureArtifacts(err) != nil) != (code > 0 && code < 126) {
				t.Fatalf("local exit %d changed", code)
			}
		})
	}
	t.Run("forged_text", func(t *testing.T) {
		if gates.VerificationFailureArtifacts(runtime.VerificationCommandFailure(context.Background(), "test", errors.New("exit status 1"))) != nil {
			t.Fatal("text forged command failure")
		}
	})
	observed := exec.Command("sh", "-c", "exit 1").Run()
	if _, ok := observed.(*exec.ExitError); !ok {
		t.Fatalf("expected actual ExitError, got %v", observed)
	}
	for name, refusal := range map[string]error{
		"cancelled": context.Canceled,
		"timeout":   context.DeadlineExceeded,
		"launch":    &exec.Error{Name: "missing", Err: exec.ErrNotFound},
		"transport": errors.New("transport failure"),
	} {
		t.Run("mixed_"+name, func(t *testing.T) {
			typed := runtime.VerificationCommandFailure(context.Background(), "test", observed)
			for _, err := range []error{
				runtime.VerificationCommandFailure(context.Background(), "test", errors.Join(observed, refusal)),
				errors.Join(typed, refusal), errors.Join(refusal, typed),
				fmt.Errorf("wrapped: %w", errors.Join(typed, refusal)),
			} {
				if gates.VerificationFailureArtifacts(err) != nil {
					t.Fatal("mixed local failure authorized repair")
				}
			}
		})
	}
}
