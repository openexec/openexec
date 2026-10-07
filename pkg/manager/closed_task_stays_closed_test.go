package manager

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
)

// A task closed with its fix is finished: a restart never reopens it, whether
// its status reads failed (attempts left) or in progress (interrupted). It
// used to be reopened, ran again beside its own fix and, stopped once more,
// was refused a second fix, which ended the run (voice-output Goal fb9feb61,
// 10-07 14:46). The fix task still runs.
func TestATaskClosedWithItsFixIsNeverReopened(t *testing.T) {
	for _, status := range []string{release.TaskStatusFailed, release.TaskStatusInProgress} {
		t.Run(status, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			if err := e.rel.CreateTask(&release.Task{ID: "closed", Title: "closed", Description: "Stopped once and closed with its fix", StoryID: "S", Status: status, AttemptCount: 1, MaxAttempts: 3}); err != nil {
				t.Fatal(err)
			}
			closed, _ := e.rel.TaskSnapshot(context.Background(), "closed")
			closed.Metadata = map[string]interface{}{"fixed_by": "closed-fix", "failure_kind": "provider"}
			if err := e.rel.UpdateTask(closed); err != nil {
				t.Fatal(err)
			}
			if err := e.rel.CreateTask(&release.Task{ID: "closed-fix", Title: "fix", Description: "The fix for the stopped task", StoryID: "S", Status: release.TaskStatusPending, MaxAttempts: 3}); err != nil {
				t.Fatal(err)
			}
			fix, _ := e.rel.TaskSnapshot(context.Background(), "closed-fix")
			fix.Metadata = map[string]interface{}{"repair_of": "closed", "fix_of": "closed"}
			if err := e.rel.UpdateTask(fix); err != nil {
				t.Fatal(err)
			}
			e.mgr.Close()
			fresh := freshQueueManager(t, e)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
			rel, err := fresh.GetInternalReleaseManager()
			if err != nil {
				t.Fatal(err)
			}
			after, _ := rel.TaskSnapshot(ctx, "closed")
			if after.Status != release.TaskStatusFailed || after.AttemptCount != 1 {
				t.Fatalf("a task closed with its fix was reopened or run again: status=%s attempts=%d", after.Status, after.AttemptCount)
			}
			if done, _ := rel.TaskSnapshot(ctx, "closed-fix"); done.Status != release.TaskStatusDone {
				t.Fatalf("the fix did not run: %s", done.Status)
			}
		})
	}
}

// A stop on a task already closed with its fix is handled, not refused.
func TestAProviderStopOnAClosedTaskIsHandled(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	if err := e.rel.CreateTask(&release.Task{ID: "closed", Title: "closed", Description: "Closed with its fix", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	closed, _ := e.rel.TaskSnapshot(context.Background(), "closed")
	closed.Metadata = map[string]interface{}{"fixed_by": "closed-fix"}
	if err := e.rel.UpdateTask(closed); err != nil {
		t.Fatal(err)
	}
	fixed, err := e.mgr.fixProviderStop(context.Background(), e.rel, "closed", "implement", 1, "provider error: connection reset")
	if err != nil || !fixed {
		t.Fatalf("a stop on a closed task was not handled: fixed=%v err=%v", fixed, err)
	}
}

// A stale pending task closed with its fix is not runnable.
func TestRunnableTasksSkipATaskClosedWithItsFix(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	if err := e.rel.CreateTask(&release.Task{ID: "stale", Title: "stale", Description: "Pending but closed with its fix", StoryID: "S", Status: release.TaskStatusPending, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	stale, _ := e.rel.TaskSnapshot(context.Background(), "stale")
	stale.Metadata = map[string]interface{}{"fixed_by": "its-fix"}
	if err := e.rel.UpdateTask(stale); err != nil {
		t.Fatal(err)
	}
	ready, err := e.rel.RunnableTasks(context.Background(), []string{"S"})
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range ready {
		if task.ID == "stale" {
			t.Fatal("a task closed with its fix is runnable")
		}
	}
}
