package manager

import (
	"context"
	"errors"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
)

func createQueueTask(t *testing.T, e *schedulerTestEnv, id string, deps []string) {
	t.Helper()
	if err := e.rel.CreateTask(&release.Task{ID: id, Title: id, Description: "Implement the bounded fixture task", StoryID: "S", Status: release.TaskStatusPending, MaxAttempts: 3, DependsOn: deps}); err != nil {
		t.Fatal(err)
	}
}

func TestTaskOrientedQueueRunsActualPipelineAndDependencies(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	createQueueTask(t, e, "B", []string{"A"})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B"} {
		task, err := e.rel.TaskSnapshot(ctx, id)
		if err != nil || task.Status != release.TaskStatusDone {
			t.Fatal("task not durably completed", id, err)
		}
		info, err := e.mgr.Status(id)
		if err != nil || info.Status != StatusComplete {
			t.Fatal("actual pipeline not exercised", id, err)
		}
	}
}

func TestTaskOrientedQueueRetainsBlockedWorkAndScope(t *testing.T) {
	for _, mode := range []string{"hitl", "dependency", "scope", "unknown_scope", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			task := e.rel.GetTask("A")
			if mode == "hitl" {
				task.Metadata = map[string]interface{}{"mode": release.TaskModeHITL}
			}
			if mode == "dependency" {
				task.DependsOn = []string{"missing"}
			}
			if err := e.rel.UpdateTask(task); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			options := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
			if mode == "scope" {
				options.StoryIDs = nil
			}
			if mode == "unknown_scope" {
				options.StoryIDs = append(options.StoryIDs, "missing")
			}
			if mode == "cancelled" {
				cancel()
			}
			err := e.mgr.ExecuteTasks(ctx, options)
			if err == nil {
				t.Fatal("blocked task reported complete")
			}
			if mode == "hitl" && !strings.Contains(err.Error(), "no executable work") {
				t.Fatal(err)
			}
			current, _ := e.rel.TaskSnapshot(context.Background(), "A")
			if current.Status != release.TaskStatusPending {
				t.Fatal("blocked task changed status")
			}
			if _, err := e.mgr.Status("A"); err == nil {
				t.Fatal("blocked task dispatched")
			}
		})
	}
}

func TestTaskOrientedQueueCompletesCrossStoryDependencies(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createStory(t, e.rel, "next", []string{"S"})
	createQueueTask(t, e, "A", nil)
	if err := e.rel.CreateTask(&release.Task{ID: "B", Title: "B", StoryID: "next", Status: release.TaskStatusPending, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S", "next"}}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B"} {
		task, _ := e.rel.TaskSnapshot(ctx, id)
		if task.Status != release.TaskStatusDone || task.AttemptCount != 1 {
			t.Fatal("task did not run exactly once", task)
		}
	}
}

func TestTaskOrientedQueueFailureDoesNotCascadeOrInventRepair(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	createQueueTask(t, e, "B", []string{"A"})
	if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), []byte(`{"execution":{"lint_commands":["missing_verifier_command_98312"],"test_commands":["true"]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("failed gate completed scope")
	}
	a, _ := e.rel.TaskSnapshot(context.Background(), "A")
	b, _ := e.rel.TaskSnapshot(context.Background(), "B")
	// A missing verifier stops every attempt the same way: A is closed with a
	// provider fix, the fix stops for the same reason, and the chain ends
	// there. B, which waits on A, never runs.
	if a.Status != release.TaskStatusFailed || a.AttemptCount != 1 || a.Metadata["fixed_by"] == nil || b.Status != release.TaskStatusPending || b.AttemptCount != 0 {
		t.Fatal("failure lost accounting or cascaded", a, b)
	}
	fix, _ := e.rel.TaskSnapshot(context.Background(), a.Metadata["fixed_by"].(string))
	if fix == nil || fix.Status != release.TaskStatusFailed || fix.Metadata["failure_kind"] != "provider" || fix.Metadata["fixed_by"] != nil ||
		!strings.Contains(fix.Description, "missing_verifier_command_98312") {
		t.Fatalf("the stop is not a provider fix carrying its reason: %+v", fix)
	}
	tasks, _ := e.rel.TasksInStories(context.Background(), []string{"S"})
	if len(tasks) != 3 {
		t.Fatalf("a repeated stop extended the chain: %d tasks", len(tasks))
	}
}

func TestTaskOrientedQueueExcludesCompetingStart(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	e.mgr.mu.Lock()
	e.mgr.taskQueueActive = true
	e.mgr.mu.Unlock()
	defer func() { e.mgr.mu.Lock(); e.mgr.taskQueueActive = false; e.mgr.mu.Unlock(); _ = e.mgr.Stop("A") }()
	if err := e.mgr.Start(context.Background(), "A"); err == nil {
		t.Fatal("unowned Start bypassed sequential queue")
	}
	if err := e.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("second task queue started")
	}
}

func TestTaskOrientedQueueDiscoversTaskAddedAfterFirstDispatch(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for {
		if _, err := e.mgr.Status("A"); err == nil {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("first pipeline did not start")
		case <-time.After(time.Millisecond):
		}
	}
	createQueueTask(t, e, "dynamic", []string{"A"})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	current, err := e.rel.TaskSnapshot(ctx, "dynamic")
	if err != nil || current.Status != release.TaskStatusDone {
		t.Fatal("queue snapshot omitted newly created work", current, err)
	}
	if _, err := e.mgr.Status("dynamic"); err != nil {
		t.Fatal("dynamic task did not execute actual pipeline", err)
	}
}

// A task whose attempt stops is closed and its provider fix runs next in the
// same queue, told why the attempt stopped, instead of ending the run.
func TestTaskOrientedQueueFixesAStoppedAttemptInTheSameRun(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	count := filepath.Join(e.dir, "lint-count")
	// The first run of the verifier finds something; the second finds it gone.
	lint := `n=$(cat ` + count + ` 2>/dev/null || echo 0); n=$((n+1)); echo $n > ` + count + `; ` +
		`[ $n -ge 2 ] && exit 0; echo "found defect $n" >&2; exit 127`
	cfg, _ := json.Marshal(map[string]any{"execution": map[string]any{"lint_commands": []string{lint}, "test_commands": []string{"true"}}})
	if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), cfg, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatalf("a stop with attempts left ended the run: %v", err)
	}
	a, _ := e.rel.TaskSnapshot(context.Background(), "A")
	fixID, _ := a.Metadata["fixed_by"].(string)
	fix, _ := e.rel.TaskSnapshot(context.Background(), fixID)
	if a.Status != release.TaskStatusFailed || a.AttemptCount != 1 || fix == nil || fix.Status != release.TaskStatusDone || fix.Metadata["failure_kind"] != "provider" {
		t.Fatalf("want A closed and its provider fix done in one run: %+v %+v", a, fix)
	}
}

func TestAProviderFixIsToldWhyTheAttemptStopped(t *testing.T) {
	original := &release.Task{ID: "A", Title: "Move the header", Description: "Move the header", VerificationScript: "make check"}
	got := providerFixDescription(original, original, "provider-stop-x", "ten assertions expect the previous header")
	for _, want := range []string{"Fix task for A", "Move the header", "ten assertions expect the previous header", "remove the cause", "make check", "out of scope"} {
		if !strings.Contains(got, want) {
			t.Fatalf("provider fix description lacks %q: %q", want, got)
		}
	}
	for evidence, kind := range map[string]string{"provider-stop-1": "provider", "console-repair:2": "finding", "verification-failure-3": "check"} {
		if got := release.FailureKind(evidence); got != kind {
			t.Fatalf("FailureKind(%s) = %s, want %s", evidence, got, kind)
		}
	}
}

// A queue asked to yield stops between tasks: the task that finished stays
// done, the next is not started and spends no attempt.
func TestTaskOrientedQueueYieldsAtATaskBoundary(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	createQueueTask(t, e, "B", []string{"A"})
	asked := 0
	yield := func() bool { asked++; return asked > 1 } // after A, before B
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}, Yield: yield})
	if !errors.Is(err, ErrQueueYielded) {
		t.Fatalf("want a yield, got %v", err)
	}
	a, _ := e.rel.TaskSnapshot(context.Background(), "A")
	b, _ := e.rel.TaskSnapshot(context.Background(), "B")
	if a.Status != release.TaskStatusDone || b.Status != release.TaskStatusPending || b.AttemptCount != 0 {
		t.Fatalf("yield lost work or spent an attempt: A=%s B=%s/%d", a.Status, b.Status, b.AttemptCount)
	}
	// And the queue resumes where it yielded.
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatalf("resume after yield: %v", err)
	}
	b, _ = e.rel.TaskSnapshot(context.Background(), "B")
	if b.Status != release.TaskStatusDone || b.AttemptCount != 1 {
		t.Fatalf("B did not run once after the yield: %s/%d", b.Status, b.AttemptCount)
	}
}
