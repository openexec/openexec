package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
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
	data, err := json.Marshal(event.Artifacts)
	if err != nil {
		return ""
	}
	hash := sha256.Sum256([]byte(taskID + "\x00" + started + "\x00" + string(data)))
	id := "verification-failure-" + hex.EncodeToString(hash[:])
	ctx := context.Background()
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
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return err
	}
	diagnosis := fmt.Sprintf("Diagnose and repair the failed verification for task %s. Evidence: run step %s; checks: %s. Preserve the original task/candidate and accepted scope. Reproduce the failing check, determine its cause, repair it, and verify it; this receipt proves failure, not a particular code defect. Do not weaken the check or cross effect boundaries.", taskID, evidenceID, artifacts[gates.VerificationFailureReceiptKey])
	_, err = rel.CreateFailureRepair(ctx, taskID, evidenceID, diagnosis)
	return err
}
