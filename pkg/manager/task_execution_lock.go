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
func (m *Manager) reconcileInterruptedTasks(ctx context.Context, rel *release.Manager, storyIDs []string) error {
	tasks, err := rel.TasksInStories(ctx, storyIDs)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.Status != release.TaskStatusInProgress {
			continue
		}
		_, err := m.state.GetDB().ExecContext(ctx, `UPDATE tasks SET status='pending'
			WHERE id=? AND status='in_progress' AND attempt_count=?`, task.ID, task.AttemptCount)
		if err != nil {
			return fmt.Errorf("reconcile interrupted task %s: %w", task.ID, err)
		}
	}
	return nil
}
