package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/evidence"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

var errRecaptureWaiting = errors.New("verification recapture waiting for dependencies")

// An exit classification/error string is not a diagnostic. Legacy receipts
// cannot create a repair merely by repeating the fact that the check failed.
func (m *Manager) diagnosticFreeReceipt(artifacts map[string]string) bool {
	if strings.TrimSpace(artifacts["stage_output"]) != "" || strings.TrimSpace(artifacts["stage_diagnostics"]) != "" {
		return false
	}
	// The Console's WithOutput adapter retains its joined argv in the trusted
	// receipt. A silent normal exit still has a reproducible command; it must
	// not be treated as a legacy classification-only failure needing recapture.
	var checks []gates.CheckFailure
	if gates.ValidateVerificationFailureArtifacts(artifacts) &&
		json.Unmarshal([]byte(artifacts[gates.VerificationFailureReceiptKey]), &checks) == nil {
		complete := true
		for _, check := range checks {
			complete = complete && strings.TrimSpace(check.Command) != ""
		}
		if complete {
			return false
		}
	}
	// Modern capture also supplies usable context for silent checks (for
	// example test -f): exact argv/cwd and the observed exit survive privately.
	for hash, path := range artifacts {
		if len(hash) != 64 {
			continue
		}
		ref, err := m.state.GetArtifact(context.Background(), hash)
		if err != nil || ref == nil || ref.Path != path || path != evidence.Path(m.cfg.WorkDir, hash) {
			continue
		}
		ev, err := runtime.ReadCommandEvidence(m.cfg.WorkDir, hash)
		if err == nil && len(ev.Argv) > 0 && ev.Cwd != "" && ev.ExitCode > 0 && ev.ExitCode < 126 {
			return false
		}
	}
	return true
}

// A missing legacy phase can be recovered only from one validated check.
func recapturePhase(phase string, artifacts map[string]string) (string, error) {
	var checks []gates.CheckFailure
	if !gates.ValidateVerificationFailureArtifacts(artifacts) || json.Unmarshal([]byte(artifacts[gates.VerificationFailureReceiptKey]), &checks) != nil || len(checks) != 1 {
		return "", fmt.Errorf("original verification identity is ambiguous")
	}
	if phase == "" {
		phase = checks[0].Gate
	}
	if checks[0].Gate != phase {
		return "", fmt.Errorf("original verification identity is ambiguous")
	}
	return phase, nil
}

// Prefer registered historical shell evidence or the task's verify script.
// An empty command for lint/test requests the current named check definition
// at the execution boundary; it makes no claim about historical argv.
func (m *Manager) resolveRecaptureCommand(task *release.Task, phase string, artifacts map[string]string) (string, error) {
	phase, err := recapturePhase(phase, artifacts)
	if err != nil {
		return "", err
	}
	command := ""
	for hash, path := range artifacts {
		if len(hash) != 64 {
			continue
		}
		registered, err := m.state.GetArtifact(context.Background(), hash)
		if err != nil || registered == nil || registered.Path != path || path != evidence.Path(m.cfg.WorkDir, hash) {
			return "", fmt.Errorf("original verification reference unavailable")
		}
		ev, err := runtime.ReadCommandEvidence(m.cfg.WorkDir, hash)
		if err != nil {
			return "", fmt.Errorf("original verification reference unreadable")
		}
		if filepath.Clean(ev.Cwd) != filepath.Clean(m.cfg.WorkDir) || len(ev.Argv) != 3 || (ev.Argv[0] != "sh" && ev.Argv[0] != "/bin/sh") || ev.Argv[1] != "-c" || strings.TrimSpace(ev.Argv[2]) == "" {
			return "", fmt.Errorf("original verification command cannot be replayed in this candidate")
		}
		if command != "" && command != ev.Argv[2] {
			return "", fmt.Errorf("original verification identity is ambiguous")
		}
		command = ev.Argv[2]
	}
	if command == "" && (phase == "verify" || phase == "verification") {
		command = task.VerificationScript
	}
	if command == "" && (phase == "lint" || phase == "test") {
		return "", nil
	}
	if strings.TrimSpace(command) == "" {
		return "", fmt.Errorf("original verification command unresolved")
	}
	return command, nil
}

// Called inside the same queue writer lock as repair insertion. Reuse native
// attempts, pipeline admission/Stop, deterministic execution and receipt
// persistence; no host fallback, retry budget or evidence-only task is added.
func (m *Manager) recaptureTaskFailure(ctx context.Context, taskID, evidenceID, phase string, artifacts map[string]string) error {
	m.mu.RLock()
	owned := m.taskQueueActive
	m.mu.RUnlock()
	if !owned {
		return fmt.Errorf("diagnostic-free verification requires the native task queue")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return err
	}
	task, err := rel.TaskSnapshot(ctx, taskID)
	if err != nil {
		return err
	}
	if task.Status != release.TaskStatusFailed || task.Metadata["verification_failure_evidence"] != evidenceID {
		return fmt.Errorf("recapture no longer owns failed receipt")
	}
	ownedAttempt := task.AttemptCount
	finish := func(outcome, status string) error {
		// Re-fetch: the native event consumer may have atomically replaced the
		// evidence binding. Never restore the legacy binding over a new receipt.
		current, err := rel.TaskSnapshot(context.Background(), taskID)
		if err != nil {
			return err
		}
		if current.AttemptCount != ownedAttempt || (current.Status != release.TaskStatusFailed && current.Status != release.TaskStatusInProgress) {
			return fmt.Errorf("recapture attempt no longer owns disposition")
		}
		current.Metadata["recapture_outcome"] = outcome
		current.Status = status
		if outcome == "success" {
			delete(current.Metadata, "verification_failure_evidence")
		}
		metadata, err := json.Marshal(current.Metadata)
		if err != nil {
			return err
		}
		result, err := m.state.GetDB().ExecContext(context.Background(), `UPDATE tasks SET status=?, metadata=? WHERE id=? AND attempt_count=? AND status IN ('failed','in_progress')`, status, string(metadata), taskID, ownedAttempt)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return fmt.Errorf("recapture attempt no longer owns disposition")
		}
		return nil
	}
	terminal := func(outcome string, cause error) error {
		if err := finish(outcome, release.TaskStatusNeedsReview); err != nil {
			return err
		}
		return fmt.Errorf("verification recapture %s: %w", outcome, cause)
	}
	if task.ExecutionMode() == release.TaskModeHITL || task.NeedsReview {
		return terminal("refused", fmt.Errorf("task requires existing review boundary"))
	}
	if task.MaxAttempts <= 0 || task.AttemptCount >= task.MaxAttempts {
		return terminal("exhausted", fmt.Errorf("task attempt limit reached"))
	}
	phase, err = recapturePhase(phase, artifacts)
	if err != nil {
		return terminal("unresolved", err)
	}
	command, err := m.resolveRecaptureCommand(task, phase, artifacts)
	if err != nil {
		return terminal("unresolved", err)
	}
	// Run the ordinary dependency predicate before claiming an attempt. Failed
	// status is the sole substitution; Settings and all other work stay waiting.
	eligible, err := rel.RecaptureEligible(ctx, task)
	if err != nil {
		return err
	}
	if !eligible {
		return errRecaptureWaiting
	}
	result, err := m.state.GetDB().ExecContext(ctx, `UPDATE tasks SET status='in_progress', attempt_count=attempt_count+1,
 metadata=json_set(metadata,'$.recapture_outcome','running')
 WHERE id=? AND status='failed' AND attempt_count=? AND attempt_count < max_attempts
 AND json_extract(metadata,'$.verification_failure_evidence')=?`, taskID, task.AttemptCount, evidenceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return fmt.Errorf("recapture attempt claim refused")
	}
	ownedAttempt++
	timeout := 5 * time.Minute
	if m.cfg.TaskTimeout > 0 && m.cfg.TaskTimeout < timeout {
		timeout = m.cfg.TaskTimeout
	}
	stage := &blueprint.Stage{Name: phase, Type: runtime.StageTypeDeterministic, Timeout: timeout}
	if command != "" {
		stage.Commands = []string{command}
	}
	err = m.start(ctx, taskID, true, WithBlueprint("standard_task"), func(cfg *pipeline.Config) { cfg.RecaptureStage = stage })
	if err != nil {
		return terminal("refused", err)
	}
	err = m.waitTaskQueueRun(ctx, taskID)
	if ctx.Err() != nil {
		return terminal("cancelled", ctx.Err())
	}
	if err == nil {
		// A successful recheck does not prove the entire original task complete.
		// Resume it through normal completion validation and dependency selection.
		return finish("success", release.TaskStatusPending)
	}
	info, statusErr := m.Status(taskID)
	if statusErr != nil || info.FailureEvidenceID == "" {
		return terminal("refused", err)
	}
	if err := finish("failed", release.TaskStatusFailed); err != nil {
		return err
	}
	if ownedAttempt >= task.MaxAttempts {
		return terminal("exhausted", fmt.Errorf("task attempt limit reached"))
	}
	// The next native queue iteration resolves the newly retained receipt. If
	// diagnostics are still absent it spends another existing task attempt.
	return nil
}
