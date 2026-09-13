package manager

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/release"
)

func TestPausedQueueDrainsBeforeReleasingWorkspaceAndRetainsBoundary(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	retainInterruptedQueueTask(t, e, "paused", "S")
	lock, err := e.mgr.lockTaskExecution(true)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	p, _ := pipeline.New(pipeline.Config{FWUID: "paused"})
	e.mgr.mu.Lock()
	e.mgr.taskQueueActive = true
	e.mgr.pipelines["paused"] = &entry{taskAttempt: 1, pipeline: p, info: PipelineInfo{FWUID: "paused", Status: StatusPaused}, done: done}
	e.mgr.mu.Unlock()
	returned := make(chan error, 1)
	go func() {
		err := e.mgr.waitTaskQueueRun(context.Background(), "paused")
		_ = lock.Close()
		e.mgr.mu.Lock()
		e.mgr.taskQueueActive = false
		e.mgr.mu.Unlock()
		returned <- err
	}()
	select {
	case err := <-returned:
		close(done)
		t.Fatalf("pause released live writer: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	fresh := freshQueueManager(t, e)
	if err := fresh.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		close(done)
		t.Fatal("another manager entered while paused writer drains")
	}
	close(done)
	select {
	case err := <-returned:
		if err == nil {
			t.Fatal("pause claimed completion")
		}
	case <-time.After(time.Second):
		t.Fatal("pause did not finish draining")
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "paused")
	if err != nil || task.Status != release.TaskStatusNeedsReview || task.AttemptCount != 1 {
		t.Fatal("owner boundary lost", task, err)
	}
	restarted := freshQueueManager(t, e)
	if err := restarted.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("restart auto-executed unresolved boundary")
	}
	if _, err := restarted.Status("paused"); err == nil {
		t.Fatal("unresolved boundary launched pipeline")
	}
}

func TestPausedTaskCannotOverwriteNewerAttemptDuringDrain(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	retainInterruptedQueueTask(t, e, "paused", "S")
	done := make(chan struct{})
	stopped := make(chan struct{})
	p, _ := pipeline.New(pipeline.Config{FWUID: "paused"})
	e.mgr.mu.Lock()
	e.mgr.pipelines["paused"] = &entry{taskAttempt: 1, pipeline: p, info: PipelineInfo{FWUID: "paused", Status: StatusPaused}, done: done, cancel: func() { close(stopped) }}
	e.mgr.mu.Unlock()
	returned := make(chan error, 1)
	go func() { returned <- e.mgr.waitTaskQueueRun(context.Background(), "paused") }()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		close(done)
		t.Fatal("pause did not stop attempt")
	}
	// An external state transition while cleanup drains must not be rewritten
	// by this older executor, even if its status is also in_progress.
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE tasks SET attempt_count=2 WHERE id='paused'`); err != nil {
		close(done)
		t.Fatal(err)
	}
	close(done)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("pause did not drain")
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "paused")
	if err != nil || task.Status != release.TaskStatusInProgress || task.AttemptCount != 2 {
		t.Fatal("old pause overwrote newer attempt", task, err)
	}
}

func TestStartRefusesTerminalButUndrainedAttempt(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	done := make(chan struct{})
	e.mgr.mu.Lock()
	e.mgr.pipelines["A"] = &entry{info: PipelineInfo{FWUID: "A", Status: StatusStopped}, done: done}
	e.mgr.mu.Unlock()
	if err := e.mgr.Start(context.Background(), "A"); err == nil {
		close(done)
		t.Fatal("terminal disposition bypassed live attempt drain")
	}
	close(done)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.mgr.Start(ctx, "A"); err != nil {
		t.Fatal("drained attempt could not restart", err)
	}
	if err := e.mgr.waitTaskQueueRun(ctx, "A"); err != nil {
		t.Fatal("replacement pipeline failed", err)
	}
}
