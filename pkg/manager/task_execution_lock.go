package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
	"github.com/openexec/openexec/internal/release"
)

// The queue owns the workspace exclusively. Legacy independent Starts retain
// shared access with each other, but cannot race a queue in another manager or
// controller process. Reuse the repository's existing OS-lock dependency; the
// lock is execution exclusion, not authority, task state, or a resource grant.
func (m *Manager) lockTaskExecution(exclusive bool) (*flock.Flock, error) {
	if exclusive {
		var databaseFile string
		if m.state == nil {
			return nil, fmt.Errorf("task-oriented execution requires durable task storage")
		}
		if err := m.state.GetDB().QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&databaseFile); err != nil || databaseFile == "" {
			return nil, fmt.Errorf("task-oriented execution requires durable task storage")
		}
	}
	dir := filepath.Join(m.cfg.WorkDir, ".openexec")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	l := flock.NewFlock(filepath.Join(dir, "execution.lock"))
	var ok bool
	var err error
	if exclusive {
		ok, err = l.TryLock()
	} else {
		ok, err = l.TryRLock()
	}
	if err != nil || !ok {
		_ = l.Close()
		return nil, fmt.Errorf("workspace execution already owned or unavailable: %v", err)
	}
	return l, nil
}

func entryFinished(e *entry) bool {
	if !isTerminal(e.info.Status) {
		return false
	}
	if e.done == nil { // Legacy/test entries without a launched pipeline.
		return true
	}
	select {
	case <-e.done:
		return true
	default:
		return false
	}
}

// Called only while holding exclusive execution ownership, never from New.
// The service supervisor must have terminated the previous process tree before
// restart. A live manager cannot be displaced by merely constructing another.
// Unknown interrupted work resumes with a new, counted task attempt; no prior
// resource consumption or completion evidence is refunded or fabricated.
//
// A failed task with attempts left is reopened the same way. A fresh queue is
// a fresh attempt: whoever started it (a diagnosis that changed the candidate,
// a delivered repair, the owner pressing resume) did so because something may
// have changed. Leaving the task failed answered every such attempt with "no
// executable work" before it ran anything, so each one needed a hand-edited
// ledger to mean anything. Bounded by the task's own max_attempts, reopened
// once per queue and never inside it; a failed check keeps its repair path.
// A repair task's failed check does not: repair creation refuses a repair of a
// repair, so its receipt sent it there only to fail the whole queue with
// "recursive repair creation is not authorized" while it had attempts left.
func (m *Manager) reconcileInterruptedTasks(ctx context.Context, rel *release.Manager, storyIDs []string) error {
	tasks, err := rel.TasksInStories(ctx, storyIDs)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		switch {
		case (task.Status == release.TaskStatusInProgress || task.Status == release.TaskStatusFailed) && task.Metadata["task_correction"] != nil:
			_, err = m.state.GetDB().ExecContext(ctx, `UPDATE tasks SET status='needs_review', metadata=json_set(metadata,'$.task_correction.outcome','interrupted') WHERE id=? AND status IN ('in_progress','failed') AND json_extract(metadata,'$.task_correction.consumed')=1 AND json_extract(metadata,'$.task_correction.outcome')='running'`, task.ID)
		case task.Status == release.TaskStatusInProgress && task.Metadata["recapture_outcome"] == "running":
			_, err = m.state.GetDB().ExecContext(ctx, `UPDATE tasks SET status='failed' WHERE id=? AND status='in_progress' AND attempt_count=?`, task.ID, task.AttemptCount)
		case task.Status == release.TaskStatusInProgress:
			_, err = m.state.GetDB().ExecContext(ctx, `UPDATE tasks SET status='pending'
				WHERE id=? AND status='in_progress' AND attempt_count=?`, task.ID, task.AttemptCount)
		case task.Status == release.TaskStatusFailed && task.AttemptCount < task.MaxAttempts &&
			(task.Metadata["verification_failure_evidence"] == nil || isRepairTask(task)) && task.ExecutionMode() != release.TaskModeHITL:
			_, err = m.state.GetDB().ExecContext(ctx, `UPDATE tasks SET status='pending'
				WHERE id=? AND status='failed' AND attempt_count=? AND attempt_count < max_attempts`, task.ID, task.AttemptCount)
		default:
			continue
		}
		if err != nil {
			return fmt.Errorf("reconcile retained task %s: %w", task.ID, err)
		}
	}
	return nil
}

// isRepairTask reports whether task was created to repair another task. It
// retries within its own max_attempts and is never itself repaired.
func isRepairTask(task *release.Task) bool {
	return task.Metadata["repair_of"] != nil
}
