package gates

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// TerminalBinding identifies one deterministic invocation, including both native
// task resumption and stage retries. Source is the admitted runner identity.
type TerminalBinding struct {
	TaskID       string `json:"task_id"`
	Stage        string `json:"stage"`
	TaskAttempt  int    `json:"task_attempt"`
	StageAttempt int    `json:"stage_attempt"`
	Source       string `json:"source"`
}

func (b TerminalBinding) valid() bool {
	return strings.TrimSpace(b.TaskID) != "" && strings.TrimSpace(b.Stage) != "" && b.TaskAttempt > 0 && b.StageAttempt > 0 && b.Source == "deterministic-runner"
}

// TerminalCompletion is loaded from the admitting caller's trusted terminal
// store, never from model output. ExitCode is absent for non-process failures.
type TerminalCompletion struct {
	ID       string          `json:"id"`
	Binding  TerminalBinding `json:"binding"`
	Outcome  string          `json:"outcome"` // exited, cancelled, timed_out, launch_error, transport_error
	ExitCode *int            `json:"exit_code,omitempty"`
	// Command and Output are what the runner ran and the tail of what it
	// printed. A receipt without them is diagnostic-free: the queue re-runs
	// the check instead of opening a repair with nothing to reproduce.
	Command string `json:"command,omitempty"`
	Output  string `json:"output,omitempty"`
}

// TerminalLoader must authenticate the persisted producer, verify integrity and
// terminal status, and return the immutable completion for id. A worker-writable
// JSON file or self-declared source field alone is NOT a trusted implementation.
// Authorization/storage stay with the admitted executor; no host fallback occurs.
type TerminalLoader func(context.Context, string) (TerminalCompletion, error)

func NewTerminalFailure(ctx context.Context, expected TerminalBinding, id string, load TerminalLoader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !expected.valid() || strings.TrimSpace(id) == "" || load == nil {
		return errors.New("invalid terminal evidence request")
	}
	terminal, err := load(ctx, id)
	if err != nil {
		return fmt.Errorf("load trusted terminal: %v", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if terminal.ID != id || terminal.Binding != expected || terminal.Outcome != "exited" || terminal.ExitCode == nil {
		return errors.New("terminal evidence binding or outcome refused")
	}
	code := *terminal.ExitCode
	if code < 0 || code >= 126 {
		return errors.New("terminal exit refused")
	}
	if code == 0 {
		return nil
	}
	output := terminal.Output
	if len(output) > maxFailureOutput {
		output = "…" + output[len(output)-maxFailureOutput:]
	}
	return &verificationFailure{message: fmt.Sprintf("verification %s exited %d", expected.Stage, code), checks: []CheckFailure{{Gate: expected.Stage, ExitCode: code, Command: terminal.Command, Output: output, TerminalID: id, Binding: &expected}}}
}

// Match structured evidence again at the native admitted boundary. Legacy local
// exec.ExitError receipts have no terminal binding and retain their behavior.
func VerificationFailureArtifactsForStage(err error, expected TerminalBinding) map[string]string {
	artifacts := VerificationFailureArtifacts(err)
	if artifacts == nil {
		return nil
	}
	checks, ok := decodeChecks(artifacts)
	if !ok {
		return nil
	}
	for _, check := range checks {
		if check.Binding != nil && *check.Binding != expected {
			return nil
		}
	}
	return artifacts
}
