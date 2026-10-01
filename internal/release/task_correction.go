package release

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// TaskCorrection is explicit trusted-caller authority for one verification pass
// of a retained candidate. It grants no implementation or external effects.
// Consumed decisions remain in history; replay and restart never reset them.
type TaskCorrection struct {
	DecisionRef     string `json:"decision_ref"`
	TaskID          string `json:"task_id"`
	EvidenceID      string `json:"evidence_id"`
	CandidatePath   string `json:"candidate_path"`
	CandidateDigest string `json:"candidate_digest"`
	Branch          string `json:"branch"`
	PlanID          string `json:"plan_id"`
	StateHash       string `json:"state_hash"`
	Consumed        bool   `json:"consumed"`
	Outcome         string `json:"outcome"`
	FreshEvidenceID string `json:"fresh_evidence_id,omitempty"`
	Reason          string `json:"reason,omitempty"`
}

func CorrectionForTask(task *Task) (TaskCorrection, error) {
	var c TaskCorrection
	data, err := json.Marshal(task.Metadata["task_correction"])
	if err == nil {
		err = json.Unmarshal(data, &c)
	}
	if err != nil || strings.TrimSpace(c.DecisionRef) == "" || c.TaskID != task.ID || c.EvidenceID == "" || c.CandidatePath == "" || c.CandidateDigest == "" || c.Branch == "" || c.PlanID == "" || c.StateHash == "" {
		return c, fmt.Errorf("explicit candidate-bound correction authority required")
	}
	return c, nil
}

func correctionChanged(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("correction authority, candidate or allowance no longer matches")
	}
	return nil
}

// AuthorizeTaskCorrection persists exactly one explicit decision. The caller
// authenticates that decision; task metadata/receipts never manufacture it.
func (s *SQLiteStore) AuthorizeTaskCorrection(ctx context.Context, c TaskCorrection) error {
	if c.Consumed || c.Outcome != "" || c.FreshEvidenceID != "" || c.Reason != "" {
		return ErrInvalidData
	}
	if _, err := CorrectionForTask(&Task{ID: c.TaskID, Metadata: map[string]interface{}{"task_correction": c}}); err != nil {
		return err
	}
	data, _ := json.Marshal(c)
	return correctionChanged(s.db.ExecContext(ctx, `UPDATE tasks SET metadata=json_set(
 CASE WHEN json_extract(metadata,'$.task_correction') IS NULL THEN metadata
 ELSE json_insert(json_set(metadata,'$.task_correction_history',
 COALESCE(json_extract(metadata,'$.task_correction_history'),json('[]'))),
 '$.task_correction_history[#]',json_extract(metadata,'$.task_correction')) END,
 '$.task_correction',json(?))
 WHERE id=? AND status='failed' AND max_attempts>0 AND attempt_count=max_attempts
 AND git_branch=? AND json_extract(metadata,'$.verification_failure_evidence')=?
 AND (json_extract(metadata,'$.task_correction') IS NULL OR (
 json_extract(metadata,'$.task_correction.outcome')='pre_admission_refused'
 AND json_extract(metadata,'$.task_correction.consumed')=1
 AND COALESCE(json_extract(metadata,'$.task_correction.fresh_evidence_id'),'')=''
 AND COALESCE(json_extract(metadata,'$.task_correction.decision_ref'),'')!=?))
 AND NOT EXISTS (SELECT 1 FROM json_each(metadata,'$.task_correction_history')
 WHERE json_extract(value,'$.decision_ref')=?)`, string(data), c.TaskID, c.Branch, c.EvidenceID, c.DecisionRef, c.DecisionRef))
}

// CorrectionRefused recognizes a durable pre-admission disposition even when
// the retained authority itself is malformed.
func CorrectionRefused(task *Task) bool {
	data, _ := json.Marshal(task.Metadata["task_correction"])
	var c struct {
		Consumed bool   `json:"consumed"`
		Outcome  string `json:"outcome"`
	}
	return json.Unmarshal(data, &c) == nil && c.Consumed && c.Outcome == "pre_admission_refused"
}

// RefuseTaskCorrection consumes only the exact observed, never-admitted record.
// Operational read/write errors do not masquerade as a recorded refusal.
func (s *SQLiteStore) RefuseTaskCorrection(ctx context.Context, task *Task, reason string) error {
	var raw string
	if err := s.db.QueryRowContext(ctx, "SELECT metadata FROM tasks WHERE id=?", task.ID).Scan(&raw); err != nil {
		return err
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return err
	}
	observed, err := json.Marshal(task.Metadata["task_correction"])
	if err != nil {
		return err
	}
	retained, err := json.Marshal(metadata["task_correction"])
	if err != nil {
		return err
	}
	if !bytes.Equal(observed, retained) || metadata["task_correction"] == nil {
		return fmt.Errorf("correction authority changed before refusal")
	}
	record, ok := metadata["task_correction"].(map[string]interface{})
	if !ok {
		record = map[string]interface{}{"retained_record": metadata["task_correction"]}
	}
	// A malformed unconsumed record can be refused, but never refund anything
	// that may already have run or produced fresh evidence.
	if record["consumed"] == true || (record["outcome"] != nil && record["outcome"] != "") {
		return fmt.Errorf("correction already admitted or consumed")
	}
	record["consumed"], record["outcome"], record["reason"] = true, "pre_admission_refused", reason
	branch := ""
	if task.Git != nil {
		branch = task.Git.Branch
	}
	evidence, _ := task.Metadata["verification_failure_evidence"].(string)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return correctionChanged(s.db.ExecContext(ctx, `UPDATE tasks SET
 metadata=json_set(metadata,'$.task_correction',json(?))
 WHERE id=? AND status='failed' AND max_attempts>0 AND attempt_count=max_attempts
 AND metadata=? AND git_branch=?
 AND COALESCE(json_extract(metadata,'$.verification_failure_evidence'),'')=?`, string(data), task.ID, raw, branch, evidence))
}

func (s *SQLiteStore) AdmitTaskCorrection(ctx context.Context, c TaskCorrection) error {
	if c.Consumed || c.Outcome != "" || c.FreshEvidenceID != "" || c.Reason != "" {
		return ErrInvalidData
	}
	if _, err := CorrectionForTask(&Task{ID: c.TaskID, Metadata: map[string]interface{}{"task_correction": c}}); err != nil {
		return err
	}
	data, _ := json.Marshal(c)
	return correctionChanged(s.db.ExecContext(ctx, `UPDATE tasks SET status='in_progress',
 metadata=json_set(metadata,'$.task_correction.consumed',json('true'),'$.task_correction.outcome','running')
 WHERE id=? AND status='failed' AND attempt_count=max_attempts AND max_attempts>0 AND needs_review=0
 AND COALESCE(json_extract(metadata,'$.mode'),'afk')!='hitl'
 AND git_branch=? AND json_extract(metadata,'$.verification_failure_evidence')=?
 AND json_extract(metadata,'$.task_correction')=json(?)
 AND json_extract(metadata,'$.task_correction.consumed')=0`, c.TaskID, c.Branch, c.EvidenceID, string(data)))
}

// FinishTaskCorrection atomically checks existing completion obligations and
// updates the consumed disposition. Failed/stopped checks retain review work.
func (s *SQLiteStore) FinishTaskCorrection(ctx context.Context, c TaskCorrection, success bool) error {
	return s.finishTaskCorrection(ctx, c, success, "refused", "")
}

// FailTaskCorrection retains the terminal reason without minting another attempt.
func (s *SQLiteStore) FailTaskCorrection(ctx context.Context, c TaskCorrection, outcome, reason string) error {
	return s.finishTaskCorrection(ctx, c, false, outcome, reason)
}

func (s *SQLiteStore) finishTaskCorrection(ctx context.Context, c TaskCorrection, success bool, failureOutcome, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	status, outcome := TaskStatusNeedsReview, failureOutcome
	if success {
		var plan, hash string
		if err := tx.QueryRowContext(ctx, `SELECT id,worktree_state_hash FROM validation_plan_revisions WHERE task_id=? AND status='accepted' ORDER BY revision DESC LIMIT 1`, c.TaskID).Scan(&plan, &hash); err != nil {
			return err
		}
		if plan != c.PlanID || hash != c.StateHash {
			return fmt.Errorf("correction validation binding changed")
		}
		if err := canCompleteTask(ctx, tx, c.TaskID); err != nil {
			return err
		}
		var missing int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM validation_items i WHERE i.plan_revision_id=?
        AND i.disposition='accepted' AND i.requirement IN ('required','blocking') AND NOT EXISTS (
            SELECT 1 FROM validation_evidence_links l JOIN run_steps r ON r.id=l.run_step_id
            WHERE l.validation_item_id=i.id AND l.status='passed' AND l.worktree_state_hash=?
            AND r.run_id=? AND r.status='completed' AND r.agent='deterministic-verification'
            AND r.inputs_hash=? AND json_extract(r.metadata,'$.correction_decision')=?
            AND json_extract(r.metadata,'$.validation_item_id')=i.id
            AND json_extract(r.metadata,'$.plan_id')=?)`, c.PlanID, c.StateHash, c.TaskID, c.CandidateDigest, c.DecisionRef, c.PlanID).Scan(&missing); err != nil {
			return err
		}
		if missing != 0 {
			return fmt.Errorf("correction requires fresh evidence for every required check")
		}
		status, outcome = TaskStatusDone, "completed"
	}
	if err := correctionChanged(tx.ExecContext(ctx, `UPDATE tasks SET status=?, completed_at=CASE WHEN ?='done' THEN datetime('now') ELSE completed_at END,
 metadata=json_set(metadata,'$.task_correction.outcome',?,'$.task_correction.reason',?) WHERE id=? AND (status='in_progress' OR (?='needs_review' AND status='failed'))
 AND attempt_count=max_attempts AND git_branch=?
 AND json_extract(metadata,'$.task_correction.decision_ref')=?
 AND json_extract(metadata,'$.task_correction.candidate_digest')=?
 AND json_extract(metadata,'$.task_correction.consumed')=1
 AND json_extract(metadata,'$.task_correction.outcome')='running'`, status, status, outcome, reason, c.TaskID, status, c.Branch, c.DecisionRef, c.CandidateDigest)); err != nil {
		return err
	}
	return tx.Commit()
}

// CorrectionEligible reuses the native dependency/story/mode predicate. Only
// selection's exhausted counter is substituted, never the persisted history.
func (m *Manager) CorrectionEligible(ctx context.Context, task *Task) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	story, err := m.store.GetStory(ctx, task.StoryID)
	if err != nil {
		return false, err
	}
	if story.Git != nil && story.Git.Branch != "" && (task.Git == nil || task.Git.Branch != story.Git.Branch) {
		return false, fmt.Errorf("%w: task and story candidate branches disagree", ErrInvalidData)
	}
	tasks, err := selectRunnableTasks(ctx, m.store, []string{task.StoryID}, task.ID, true)
	if err != nil {
		return false, err
	}
	for _, next := range tasks {
		if next.ID == task.ID {
			return !task.NeedsReview, nil
		}
	}
	return false, nil
}
