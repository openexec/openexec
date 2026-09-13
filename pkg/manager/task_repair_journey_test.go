package manager

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

// This is a controlled-provider integration journey, not native model or
// deployment evidence. Both task execution and failed checks run as processes
// through the ordinary manager/blueprint path against a temporary project.
func TestTaskQueueFailedCheckCreatesRepairAndResumesOriginalWork(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.RunnerCommand = e.mgr.cfg.CommandName
	e.mgr.cfg.CommandName = ""
	if err := os.WriteFile(filepath.Join(e.dir, ".task-loop-fixture"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	config := `{"execution":{"lint_commands":["true"],"test_commands":["test -f feature.txt"]}}`
	if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	createQueueTask(t, e, "B", []string{"A"})
	b := *e.rel.GetTask("B")
	b.Description = "Finish remaining original task B by persisting remaining.txt"
	if err := e.rel.UpdateTask(&b); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	for _, filename := range []string{"feature.txt", "remaining.txt"} {
		if data, err := os.ReadFile(filepath.Join(e.dir, filename)); err != nil || len(data) == 0 {
			t.Fatalf("useful fixture work absent: %s %v", filename, err)
		}
	}
	tasks, err := e.rel.TasksInStories(ctx, []string{"S"})
	if err != nil {
		t.Fatal(err)
	}
	var repairs int
	for _, task := range tasks {
		if task.Status != release.TaskStatusDone {
			t.Fatalf("unfinished task: %s %s", task.ID, task.Status)
		}
		if task.Metadata["repair_of"] == "A" {
			repairs++
			id, _ := task.Metadata["failure_evidence"].(string)
			receipt, err := e.mgr.state.GetRunStep(ctx, id)
			if err != nil || receipt == nil || receipt.RunID != "A" {
				t.Fatal("repair lacks retained failed-check receipt", err)
			}
		}
	}
	if repairs != 1 {
		t.Fatalf("wanted one repair, got %d", repairs)
	}
	a, err := e.rel.TaskSnapshot(ctx, "A")
	if err != nil || a.AttemptCount != 2 {
		t.Fatal("original task did not resume after repair", a, err)
	}
}

func TestTaskQueueRestartAfterFailureReceiptBeforeRepair(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.RunnerCommand = e.mgr.cfg.CommandName
	e.mgr.cfg.CommandName = ""
	if err := os.WriteFile(filepath.Join(e.dir, ".task-loop-fixture"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), []byte(`{"execution":{"lint_commands":["true"],"test_commands":["test -f feature.txt"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	a := *e.rel.GetTask("A")
	a.Status = release.TaskStatusInProgress
	a.AttemptCount = 1
	if err := e.rel.UpdateTask(&a); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	e.mgr.mu.Lock()
	e.mgr.taskQueueActive = true
	e.mgr.mu.Unlock()
	if err := e.mgr.start(ctx, "A", true, WithBlueprint("standard_task"), WithTaskDescription(a.Description)); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.waitTaskQueueRun(ctx, "A"); err == nil {
		t.Fatal("deliberate failed check unexpectedly passed")
	}
	failed, err := e.rel.TaskSnapshot(ctx, "A")
	if err != nil || failed.Status != release.TaskStatusFailed || failed.Metadata["verification_failure_evidence"] == nil {
		t.Fatal("failure cannot be reconstructed durably", failed, err)
	}
	// Replace both manager and database connection before the repair is created.
	// No prior native session, event stream or execution explanation is supplied.
	cfg := e.mgr.cfg
	e.mgr.Close()
	e.closeState()
	reopened, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cfg.StateStore = reopened
	fresh, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := rel.TaskSnapshot(ctx, "A")
	if err != nil || recovered.Status != release.TaskStatusDone || recovered.AttemptCount != 2 {
		t.Fatal("original did not resume after restart", recovered, err)
	}
	if _, err := os.Stat(filepath.Join(e.dir, "feature.txt")); err != nil {
		t.Fatal("repair action absent", err)
	}
}
