package manager

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelledQueueRetainsWriterUntilAttemptDrain(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "next", nil)
	drained := make(chan struct{})
	// Model the exact Stop boundary: terminal disposition is already visible,
	// but the old runner and its event consumer have not completed cleanup.
	e.mgr.mu.Lock()
	e.mgr.pipelines["slow-stopping"] = &entry{info: PipelineInfo{FWUID: "slow-stopping", Status: StatusStopped}, done: drained}
	e.mgr.taskQueueActive = true
	e.mgr.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	returned := make(chan error, 1)
	go func() {
		err := e.mgr.waitTaskQueueRun(ctx, "slow-stopping")
		e.mgr.mu.Lock()
		e.mgr.taskQueueActive = false
		e.mgr.mu.Unlock()
		returned <- err
	}()
	select {
	case err := <-returned:
		close(drained)
		t.Fatalf("queue released ownership before attempt drain: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := e.mgr.Start(context.Background(), "next"); err == nil {
		close(drained)
		t.Fatal("competing Start entered while cancelled writer drains")
	}
	if err := e.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		close(drained)
		t.Fatal("competing queue entered while cancelled writer drains")
	}
	close(drained)
	select {
	case err := <-returned:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("drained cancellation did not return")
	}
}
