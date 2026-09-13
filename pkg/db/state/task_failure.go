package state

import (
	"context"
	"fmt"
)

// RecordTaskFailureStep atomically binds an observed failure to the task attempt
// that produced it. A restart can see neither half or both halves, never a
// receipt without its task disposition. It does not authenticate the evidence;
// the caller must be the trusted executor, not a worker-supplied artifact.
func (s *Store) RecordTaskFailureStep(ctx context.Context, step RunStepData, attempt int) error {
	if step.ID == "" || step.RunID == "" || step.Status != "failed" || attempt < 1 {
		return fmt.Errorf("invalid task failure binding")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE tasks SET status='failed',
		metadata=json_set(COALESCE(NULLIF(CAST(metadata AS TEXT),'null'),'{}'),'$.verification_failure_evidence',?)
		WHERE id=? AND status='in_progress' AND attempt_count=?`, step.ID, step.RunID, attempt)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil || n != 1 {
		return fmt.Errorf("task attempt no longer owns failure disposition")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO run_steps
		(id,run_id,trace_id,phase,agent,iteration,status,inputs_hash,metadata)
		VALUES(?,?,?,?,?,?,?,?,?)`, step.ID, step.RunID, step.TraceID, step.Phase,
		step.Agent, step.Iteration, step.Status, step.InputsHash, step.Metadata)
	if err != nil {
		return err
	}
	return tx.Commit()
}
