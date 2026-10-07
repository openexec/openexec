package manager

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

func freshQueueManager(t *testing.T, e *schedulerTestEnv) *Manager {
	t.Helper()
	db, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	config := e.mgr.cfg
	config.StateStore = db
	m, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

func retainInterruptedQueueTask(t *testing.T, e *schedulerTestEnv, id, story string) *release.Task {
	t.Helper()
	if err := e.rel.CreateTask(&release.Task{ID: id, Title: id, Description: "Resume the retained bounded fixture", StoryID: story, Status: release.TaskStatusInProgress, AttemptCount: 1, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	task, err := e.rel.TaskSnapshot(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	pr := 71
	task.Git = &release.TaskGitInfo{Branch: "feature/retained", PRNumber: &pr, Commits: []string{"retained-commit"}}
	task.Metadata = map[string]interface{}{"retained_evidence": "fixture-observation"}
	if err := e.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestFreshTaskQueueReconcilesOnlyScopedInterruptedWork(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createStory(t, e.rel, "outside", nil)
	before := retainInterruptedQueueTask(t, e, "resume", "S")
	outside := retainInterruptedQueueTask(t, e, "outside-task", "outside")
	// No provider/process remains. Only persisted attempt/task/candidate state
	// is available to the newly constructed manager and reopened SQLite store.
	e.mgr.Close()
	fresh := freshQueueManager(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	after, err := rel.TaskSnapshot(ctx, "resume")
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != release.TaskStatusDone || after.AttemptCount != before.AttemptCount+1 || after.MaxAttempts != before.MaxAttempts || !reflect.DeepEqual(after.Git, before.Git) || !reflect.DeepEqual(after.Metadata, before.Metadata) {
		t.Fatal("restart reset or lost retained work", after)
	}
	info, err := fresh.Status("resume")
	if err != nil || info.Status != StatusComplete {
		t.Fatal("resumed task did not run actual pipeline", err)
	}
	untouched, err := rel.TaskSnapshot(ctx, "outside-task")
	if err != nil || !reflect.DeepEqual(untouched, outside) {
		t.Fatal("reconciliation widened beyond scope", untouched, err)
	}
}

func TestLiveWorkspaceOwnerPreventsFreshQueueReconciliation(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	before := retainInterruptedQueueTask(t, e, "owned-task", "S")
	lock, err := e.mgr.lockTaskExecution(true)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	fresh := freshQueueManager(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("fresh queue displaced live workspace owner")
	}
	if err := fresh.Start(ctx, "owned-task"); err == nil {
		t.Fatal("public Start displaced live workspace owner")
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	after, err := rel.TaskSnapshot(ctx, "owned-task")
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatal("blocked restart reset another manager's task", after, err)
	}
	if _, err := fresh.Status("owned-task"); err == nil {
		t.Fatal("blocked manager launched provider")
	}
}

// A failed task used to stay failed for good, so every later queue on the
// same stories answered "no executable work" without running anything. The
// attempt after a diagnosis, a delivered repair or the owner's resume did
// nothing until someone reset the ledger by hand.
func TestFreshTaskQueueReopensFailedTaskWithAttemptsLeft(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createStory(t, e.rel, "outside", nil)
	for _, task := range []*release.Task{
		{ID: "retry", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 3},
		{ID: "spent", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 3, MaxAttempts: 3},
		{ID: "human", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 3, Metadata: map[string]interface{}{"mode": release.TaskModeHITL}},
		{ID: "outside-task", StoryID: "outside", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 3},
	} {
		task.Title, task.Description = task.ID, "Resume the retained bounded fixture"
		if err := e.rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	e.mgr.Close()
	fresh := freshQueueManager(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var boundary *TaskQueueBoundary
	if err := fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); !errors.As(err, &boundary) {
		t.Fatalf("want the spent and human tasks retained: %v", err)
	}
	want := []TaskBoundary{
		{TaskID: "human", Status: release.TaskStatusFailed, Kind: BoundaryFailed},
		{TaskID: "spent", Status: release.TaskStatusFailed, Kind: BoundaryFailed},
	}
	if !reflect.DeepEqual(boundary.Tasks, want) {
		t.Fatalf("boundary = %+v", boundary.Tasks)
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	retried, err := rel.TaskSnapshot(ctx, "retry")
	if err != nil || retried.Status != release.TaskStatusDone || retried.AttemptCount != 2 {
		t.Fatalf("failed task with attempts left did not run again: %+v, %v", retried, err)
	}
	outside, err := rel.TaskSnapshot(ctx, "outside-task")
	if err != nil || outside.Status != release.TaskStatusFailed || outside.AttemptCount != 1 {
		t.Fatalf("reopening widened beyond scope: %+v, %v", outside, err)
	}
}

func TestFailedRepairTaskUsesItsOwnAttemptsNotRepairCreation(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	for _, task := range []*release.Task{
		{ID: "original", StoryID: "S", Status: release.TaskStatusDone, AttemptCount: 1, MaxAttempts: 3},
		// Fixes that stopped without a failed-check receipt: one with an
		// attempt left runs again, a spent one is a boundary. A fix that
		// failed its check is closed and fixed again (failure_repair_test).
		{ID: "repair-retry", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 2,
			Metadata: map[string]interface{}{"repair_of": "original"}},
		{ID: "repair-spent", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 2, MaxAttempts: 2,
			Metadata: map[string]interface{}{"repair_of": "original"}},
	} {
		task.Title, task.Description = task.ID, "Resume the retained bounded fixture"
		if err := e.rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	e.mgr.Close()
	fresh := freshQueueManager(t, e)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var boundary *TaskQueueBoundary
	if err := fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); !errors.As(err, &boundary) {
		t.Fatalf("want only the spent fix retained: %v", err)
	}
	if want := []TaskBoundary{{TaskID: "repair-spent", Status: release.TaskStatusFailed, Kind: BoundaryFailed}}; !reflect.DeepEqual(boundary.Tasks, want) {
		t.Fatalf("boundary = %+v", boundary.Tasks)
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	retried, err := rel.TaskSnapshot(ctx, "repair-retry")
	if err != nil || retried.Status != release.TaskStatusDone || retried.AttemptCount != 2 {
		t.Fatalf("failed repair with attempts left did not run again: %+v, %v", retried, err)
	}
	tasks, err := rel.TasksInStories(ctx, []string{"S"})
	if err != nil || len(tasks) != 3 {
		t.Fatalf("a repair of a repair was created: %d tasks, %v", len(tasks), err)
	}
}

// A failed task closed by its fix stays closed: tasks are immutable, and its
// fix is the work that runs. Reopening it because it had attempts left ran
// the closed task again beside its fix, and its next stop was refused as
// "already closed with fix", failing the whole queue (agent-console Voice
// Goal fb9feb61, T-US-008-001 and repair-45e07bbb, 2026-10-07).
func TestFreshTaskQueueNeverReopensATaskClosedByItsFix(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	for _, task := range []*release.Task{
		{ID: "original", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 3,
			Metadata: map[string]interface{}{"fixed_by": "fix-1"}},
		{ID: "fix-1", StoryID: "S", Status: release.TaskStatusFailed, AttemptCount: 1, MaxAttempts: 2,
			Metadata: map[string]interface{}{"repair_of": "original", "fix_of": "original", "fixed_by": "fix-2"}},
		{ID: "fix-2", StoryID: "S", Status: release.TaskStatusPending, MaxAttempts: 2,
			Metadata: map[string]interface{}{"repair_of": "original", "fix_of": "fix-1"}},
	} {
		task.Title, task.Description = task.ID, "Resume the retained bounded fixture"
		if err := e.rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
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
	for _, id := range []string{"original", "fix-1"} {
		closed, err := rel.TaskSnapshot(ctx, id)
		if err != nil || closed.Status != release.TaskStatusFailed || closed.AttemptCount != 1 {
			t.Fatalf("task %s closed by its fix was reopened: %+v, %v", id, closed, err)
		}
	}
	fix, err := rel.TaskSnapshot(ctx, "fix-2")
	if err != nil || fix.Status != release.TaskStatusDone || fix.AttemptCount != 1 {
		t.Fatalf("the open fix did not run: %+v, %v", fix, err)
	}
}
