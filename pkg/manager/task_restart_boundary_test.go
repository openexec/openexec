package manager

import (
	"context"
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
