package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

// Unlike asynchronous activity telemetry, a receipt used to derive repair work
// must be committed before the scheduler can observe it. Reuse run_steps.
func (m *Manager) persistTaskVerificationFailure(taskID string, event loop.Event) string {
	if m.state == nil || event.Type != loop.EventBlueprintFailed || !gates.ValidateVerificationFailureArtifacts(event.Artifacts) {
		return ""
	}
	m.mu.RLock()
	e := m.pipelines[taskID]
	queueOwned := m.taskQueueActive
	started := ""
	attempt := 0
	if e != nil {
		started = e.info.StartedAt.UTC().Format(time.RFC3339Nano)
		attempt = e.taskAttempt
	}
	m.mu.RUnlock()
	if started == "" {
		return ""
	}
	retainedArtifacts := make(map[string]string, len(event.Artifacts)+2)
	for key, value := range event.Artifacts {
		retainedArtifacts[key] = value
	}
	retainedArtifacts["stage_output"] = event.Text
	retainedArtifacts["stage_error"] = event.ErrText
	if event.Result != nil {
		retainedArtifacts["stage_diagnostics"] = event.Result.Diagnostics
	}
	data, err := json.Marshal(retainedArtifacts)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256([]byte(taskID + "\x00" + started + "\x00" + string(data)))
	id := "verification-failure-" + hex.EncodeToString(hash[:])
	ctx := context.Background()
	// Retain references synchronously before publishing a repair-eligible step.
	// The payload remains at the executor's existing artifact location.
	for hash, path := range event.Artifacts {
		if hash == gates.VerificationFailureReceiptKey || hash == gates.VerificationFailureDigestKey || hash == "" || path == "" {
			continue
		}
		if err := m.state.RecordArtifact(ctx, hash, "test_log", path, 0); err != nil {
			return ""
		}
	}
	if queueOwned {
		err = m.state.RecordTaskFailureStep(ctx, state.RunStepData{
			ID: id, RunID: taskID, TraceID: event.TraceID, Phase: event.StageName,
			Agent: "deterministic-verification", Iteration: event.Attempt,
			Status: "failed", Metadata: string(data),
		}, attempt)
	} else {
		err = m.state.AddRunStepFull(ctx, id, taskID, event.TraceID, event.StageName, "deterministic-verification", event.Attempt, "failed", "", string(data))
	}
	if err != nil {
		return ""
	}
	retained, err := m.state.GetRunStep(ctx, id)
	if err != nil || retained == nil || retained.RunID != taskID || retained.Metadata != string(data) {
		return ""
	}
	return id
}

func (m *Manager) repairTaskFromRetainedFailure(ctx context.Context, taskID, evidenceID string) error {
	if evidenceID == "" || m.state == nil {
		return fmt.Errorf("no retained deterministic verification failure")
	}
	step, err := m.state.GetRunStep(ctx, evidenceID)
	if err != nil {
		return err
	}
	if step == nil || step.RunID != taskID || step.Status != "failed" || step.Agent.String != "deterministic-verification" {
		return fmt.Errorf("verification failure does not belong to the failed task")
	}
	var artifacts map[string]string
	if err := json.Unmarshal([]byte(step.Metadata), &artifacts); err != nil || !gates.ValidateVerificationFailureArtifacts(artifacts) {
		return fmt.Errorf("verification failure receipt invalid")
	}
	if m.diagnosticFreeReceipt(artifacts) {
		return m.recaptureTaskFailure(ctx, taskID, evidenceID, step.Phase, artifacts)
	}
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return err
	}
	failed, err := rel.TaskSnapshot(ctx, taskID)
	if err != nil {
		return err
	}
	original := failed
	if rootID, _ := failed.Metadata["repair_of"].(string); rootID != "" {
		if original, err = rel.TaskSnapshot(ctx, rootID); err != nil {
			return err
		}
	}
	_, err = rel.CreateFailureRepair(ctx, taskID, evidenceID, fixTaskDescription(original, failed, evidenceID, step.Metadata))
	return err
}

// fixTaskDescription is the whole scope of a fix task, for the stage that
// writes the fix and for the review that judges it: the original task, its
// code, how it failed, and that only the fix is to be delivered. Its review
// sees the original task, the original code and the fix together, so a
// finding outside the original task is out of scope, not more work.
func fixTaskDescription(original, failed *release.Task, evidenceID, evidence string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Fix task for %s %q. The original task is closed: it was implemented and failed its check. ", original.ID, original.Title)
	b.WriteString("Deliver only the fix that makes the original task pass. Do not redo the original task.\n\n")
	fmt.Fprintf(&b, "Original task:\n%s\n\n", strings.TrimSpace(original.Description))
	if script := strings.TrimSpace(original.VerificationScript); script != "" {
		fmt.Fprintf(&b, "Its check, which this fix must pass unchanged:\n%s\n\n", script)
	}
	if original.Git != nil && (len(original.Git.Commits) > 0 || original.Git.Branch != "") {
		fmt.Fprintf(&b, "Its code: branch %s, commits %s. Read them with git show before changing anything.\n\n",
			original.Git.Branch, strings.Join(original.Git.Commits, ", "))
	} else {
		b.WriteString("Its code is the original task's work in this candidate; read its changes before changing anything.\n\n")
	}
	if failed.ID != original.ID {
		fmt.Fprintf(&b, "An earlier fix, %s, also failed; this fix replaces it.\n\n", failed.ID)
	}
	fmt.Fprintf(&b, "How it failed (run step %s): %s\n\n", evidenceID, evidence)
	b.WriteString("Preserve the original task/candidate and accepted scope. Reproduce the failing check, find its cause, change only what that cause requires, and verify with the check above. ")
	b.WriteString("The review of this fix judges the original task, its code and this fix together. ")
	b.WriteString("Anything outside the original task is out of scope for this fix: record it as a finding for later, do not do it. ")
	b.WriteString("Note: this receipt proves failure, not a particular code defect. Do not weaken the check or cross effect boundaries.")
	return b.String()
}
