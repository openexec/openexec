package manager

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
)

func TestTaskQueueHumanWaitRunsIndependentWorkAndSurvivesRestart(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "prepare", nil)
	createQueueTask(t, e, "human", []string{"prepare"})
	createQueueTask(t, e, "dependent", []string{"human"})
	createQueueTask(t, e, "independent", nil)
	human := *e.rel.GetTask("human")
	human.Metadata = map[string]interface{}{"mode": release.TaskModeHITL, "decision_reason": "Owner must grant access", "decision_ref": "request:retained"}
	if err := e.rel.UpdateTask(&human); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
	var before *TaskQueueBoundary
	if err := e.mgr.ExecuteTasks(ctx, opts); !errors.As(err, &before) {
		t.Fatalf("want typed boundary, got %v", err)
	}
	want := []TaskBoundary{
		{TaskID: "dependent", Status: "pending", Kind: BoundaryDependency},
		{TaskID: "human", Status: "pending", Kind: BoundaryHuman, DecisionReason: "Owner must grant access", DecisionRef: "request:retained"},
	}
	if !reflect.DeepEqual(before.Tasks, want) {
		t.Fatalf("boundary = %+v", before.Tasks)
	}
	for _, id := range []string{"prepare", "independent"} {
		task, err := e.rel.TaskSnapshot(ctx, id)
		if err != nil || task.Status != release.TaskStatusDone || task.AttemptCount != 1 {
			t.Fatalf("independent work did not complete: %+v, %v", task, err)
		}
	}
	e.mgr.Close()
	fresh := freshQueueManager(t, e)
	for i := 0; i < 2; i++ {
		var after *TaskQueueBoundary
		if err := fresh.ExecuteTasks(ctx, opts); !errors.As(err, &after) || !reflect.DeepEqual(after, before) {
			t.Fatalf("restart changed waiting evidence: %+v, %v", after, err)
		}
	}
	backlog, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"human", "dependent"} {
		task, err := backlog.TaskSnapshot(ctx, id)
		if err != nil || task.Status != release.TaskStatusPending || task.AttemptCount != 0 {
			t.Fatalf("waiting created a failed attempt: %+v, %v", task, err)
		}
	}
	tasks, err := backlog.TasksInStories(ctx, []string{"S"})
	if err != nil || len(tasks) != 4 {
		t.Fatalf("waiting created repair/duplicate work: %d, %v", len(tasks), err)
	}
}

func TestTaskQueueBoundaryKeepsLegacyHumanFailureAndLimitsDistinct(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	for _, task := range []*release.Task{
		{ID: "legacy-human", Status: release.TaskStatusPending, MaxAttempts: 3, Metadata: map[string]interface{}{"mode": release.TaskModeHITL}},
		{ID: "limit", Status: release.TaskStatusPending, MaxAttempts: 2, AttemptCount: 2},
		{ID: "failed", Status: release.TaskStatusFailed, MaxAttempts: 3, AttemptCount: 3},
		{ID: "review", Status: release.TaskStatusNeedsReview, MaxAttempts: 3},
	} {
		task.StoryID, task.Title = "S", task.ID
		if err := e.rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	var boundary *TaskQueueBoundary
	err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
	if !errors.As(err, &boundary) {
		t.Fatalf("want mixed native boundary: %v", err)
	}
	kinds := map[string]string{}
	for _, item := range boundary.Tasks {
		kinds[item.TaskID] = item.Kind
	}
	if !reflect.DeepEqual(kinds, map[string]string{"legacy-human": BoundaryHuman, "limit": BoundaryAttemptLimit, "failed": BoundaryAttemptLimit, "review": BoundaryReview}) {
		t.Fatalf("boundary kinds = %v", kinds)
	}
	boundary.Tasks[0].DecisionReason = "sensitive owner detail"
	if strings.Contains(boundary.Error(), "sensitive") {
		t.Fatal("ordinary error log exposes decision detail")
	}
}
