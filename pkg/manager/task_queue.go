package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/openexec/openexec/internal/release"
)

func (m *Manager) executeTaskQueue(ctx context.Context, opts RunOptions) error {
	if len(opts.StoryIDs) == 0 {
		return fmt.Errorf("task-oriented execution requires explicit story scope")
	}
	if opts.MaxParallel > 1 {
		return fmt.Errorf("task-oriented execution is sequential")
	}
	m.mu.Lock()
	if m.taskQueueActive {
		m.mu.Unlock()
		return fmt.Errorf("task-oriented queue already active")
	}
	for _, e := range m.pipelines {
		if !entryFinished(e) {
			m.mu.Unlock()
			return fmt.Errorf("existing pipeline owns execution")
		}
	}
	m.taskQueueActive = true
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.taskQueueActive = false; m.mu.Unlock() }()
	lock, err := m.lockTaskExecution(true)
	if err != nil {
		return err
	}
	defer lock.Close()
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return err
	}
	if _, err := rel.TasksInStories(ctx, opts.StoryIDs); err != nil {
		return fmt.Errorf("invalid task scope: %w", err)
	}
	if err := m.reconcileInterruptedTasks(ctx, rel, opts.StoryIDs); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// A crash after receipt persistence but before repair insertion must not
		// require the owner to recreate that disposition. Repair insertion itself
		// is idempotent and keeps all work in the same existing task ledger.
		retained, err := rel.TasksInStories(ctx, opts.StoryIDs)
		if err != nil {
			return err
		}
		for _, task := range retained {
			if task.Status != release.TaskStatusFailed {
				continue
			}
			id, _ := task.Metadata["verification_failure_evidence"].(string)
			if id != "" {
				if err := m.repairTaskFromRetainedFailure(ctx, task.ID, id); err != nil {
					return err
				}
			}
		}
		ready, err := rel.RunnableTasks(ctx, opts.StoryIDs)
		if err != nil {
			return err
		}
		if len(ready) == 0 {
			current, err := rel.TasksInStories(ctx, opts.StoryIDs)
			if err != nil {
				return err
			}
			if len(current) == 0 {
				return fmt.Errorf("scoped stories have no tasks; completion is unproven")
			}
			for _, task := range current {
				if task.Status != release.TaskStatusDone {
					return fmt.Errorf("no executable work: task %s remains %s", task.ID, task.Status)
				}
			}
			return nil // Task scope complete, not a Goal/Ready verdict.
		}
		task := ready[0]
		attempt := *task
		attempt.Metadata = make(map[string]interface{}, len(task.Metadata))
		for key, value := range task.Metadata {
			attempt.Metadata[key] = value
		}
		// The prior receipt remains in run_steps and its repair task. It must
		// not classify a later provider failure as the same failed check.
		delete(attempt.Metadata, "verification_failure_evidence")
		attempt.AttemptCount++
		attempt.Status = release.TaskStatusInProgress
		if err := rel.UpdateTask(&attempt); err != nil {
			return fmt.Errorf("record task attempt: %w", err)
		}
		options := []StartOption{WithBlueprint("standard_task"), WithTaskDescription(task.Description)}
		if opts.IsStudy {
			options = append(options, WithIsStudy(true))
		}
		if opts.Mode != "" {
			options = append(options, WithExecMode(opts.Mode))
		}
		if err := m.start(ctx, task.ID, true, options...); err != nil {
			if saveErr := rel.SetTaskStatus(task.ID, release.TaskStatusFailed); saveErr != nil {
				return fmt.Errorf("start refused and disposition failed: %w", saveErr)
			}
			return fmt.Errorf("task %s start refused: %w", task.ID, err)
		}
		if err := m.waitTaskQueueRun(ctx, task.ID); err != nil {
			if info, statusErr := m.Status(task.ID); statusErr == nil && info.Status == StatusError {
				if saveErr := rel.SetTaskStatus(task.ID, release.TaskStatusFailed); saveErr != nil {
					return fmt.Errorf("task failed and failure disposition could not persist: %w", saveErr)
				}
				if info.FailureEvidenceID != "" && ctx.Err() == nil {
					if repairErr := m.repairTaskFromRetainedFailure(ctx, task.ID, info.FailureEvidenceID); repairErr != nil {
						return fmt.Errorf("repair task refused: %w", repairErr)
					}
					continue
				}
			}
			return err
		}
		// Normal completion still passes the release manager's accepted
		// validation obligations; a pipeline exit cannot bypass them.
		current, err := rel.TaskSnapshot(ctx, task.ID)
		if err != nil {
			return err
		}
		if current.Status != release.TaskStatusDone {
			if current.Status == release.TaskStatusFailed || current.Status == "error" {
				return fmt.Errorf("pipeline ended but task %s remains failed", task.ID)
			}
			if err := rel.SetTaskStatus(task.ID, release.TaskStatusDone); err != nil {
				return fmt.Errorf("task completion evidence refused: %w", err)
			}
		}
	}
}

func (m *Manager) waitTaskQueueRun(ctx context.Context, id string) error {
	m.mu.RLock()
	var attemptDone <-chan struct{}
	var attemptNumber int
	if attempt := m.pipelines[id]; attempt != nil {
		attemptDone = attempt.done
		attemptNumber = attempt.taskAttempt
	}
	m.mu.RUnlock()
	cancelAndDrain := func() error {
		_ = m.Stop(id)
		// Stop records a terminal disposition before runner cleanup and event
		// persistence finish. Cancellation must not release the queue's writer
		// ownership while that exact attempt can still write.
		if attemptDone != nil {
			<-attemptDone
		}
		return ctx.Err()
	}
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return cancelAndDrain()
		}
		info, err := m.Status(id)
		if err != nil {
			return err
		}
		if isTerminal(info.Status) {
			m.mu.RLock()
			done := m.pipelines[id].done
			m.mu.RUnlock()
			if done != nil {
				select {
				case <-done:
				case <-ctx.Done():
					return cancelAndDrain()
				}
				info, err = m.Status(id)
				if err != nil {
					return err
				}
			}
		}
		switch info.Status {
		case StatusComplete:
			return nil
		case StatusStopped:
			return fmt.Errorf("task %s stopped; remaining work retained", id)
		case StatusPaused:
			_ = m.Stop(id)
			if attemptDone != nil {
				<-attemptDone
			}
			rel, err := m.GetInternalReleaseManager()
			if err != nil {
				return err
			}
			task, err := rel.TaskSnapshot(context.Background(), id)
			if err != nil {
				return err
			}
			if task.Status == release.TaskStatusInProgress {
				// Do not overwrite an owner stop or a newer task disposition that
				// arrived while the old attempt was draining.
				if _, err := m.state.GetDB().ExecContext(context.Background(), `UPDATE tasks SET status='needs_review' WHERE id=? AND status='in_progress' AND attempt_count=?`, id, attemptNumber); err != nil {
					return err
				}
			}
			return fmt.Errorf("task %s requires an existing boundary decision; remaining work retained", id)
		case StatusError:
			return fmt.Errorf("task %s failed; no typed deterministic failure receipt is available for automatic repair", id)
		}
		select {
		case <-ctx.Done():
			return cancelAndDrain()
		case <-ticker.C:
		}
	}
}
