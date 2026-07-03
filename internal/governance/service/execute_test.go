package service

import (
	"context"
	"errors"
	"testing"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/release"
)

func releaseStory(id string, tasks []string) *release.Story {
	return &release.Story{ID: id, Title: id, Status: release.StoryStatusPending, Tasks: tasks}
}

// fakeExecutor records dispatched task ids.
type fakeExecutor struct {
	ran     []string
	failOn  string
	failErr error
}

func (f *fakeExecutor) RunTask(_ context.Context, taskID, _ string) error {
	if taskID == f.failOn {
		return f.failErr
	}
	f.ran = append(f.ran, taskID)
	return nil
}

// setupApprovedChangeWithTasks builds an approved change in an approved release
// that owns one story with two tasks, ready for the execute hop.
func setupApprovedChangeWithTasks(t *testing.T, store governance.Store, ps *fakePlanStore) {
	t.Helper()
	ctx := context.Background()
	seedRelease(t, store, &governance.GovernanceRelease{
		ID: "R-1", Status: governance.ReleaseStatusApproved, ApprovedForAI: true,
	})
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", ReleaseID: "R-1", Status: governance.ChangeStatusApprovedForAI,
		ProposalVersion: 1, ApprovedVersion: 1,
		SourceType: governance.SourceManual, SourceID: "C-1",
	})
	ps.stories["US-1"] = releaseStory("US-1", []string{"T-1", "T-2"})
	if err := store.LinkChangeStory(ctx, "C-1", "US-1"); err != nil {
		t.Fatalf("link: %v", err)
	}
}

func TestExecuteChange_DispatchesApprovedTasks(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	exec := &fakeExecutor{}
	svc := NewService(store, Options{PlanStore: ps, Executor: exec})
	ctx := context.Background()

	setupApprovedChangeWithTasks(t, store, ps)

	rep, err := svc.ExecuteChange(ctx, "C-1", "codex", "workspace-write")
	if err != nil {
		t.Fatalf("ExecuteChange: %v", err)
	}
	if len(rep.DispatchedTasks) != 2 {
		t.Fatalf("expected 2 dispatched tasks, got %+v", rep.DispatchedTasks)
	}
	if len(exec.ran) != 2 || exec.ran[0] != "T-1" || exec.ran[1] != "T-2" {
		t.Fatalf("executor should have run T-1,T-2 in order, got %+v", exec.ran)
	}
	// The change was claimed and advanced to implementing.
	ch, _ := store.GetChangeRecord(ctx, "C-1")
	if ch.Status != governance.ChangeStatusImplementing {
		t.Fatalf("expected implementing after execute, got %q", ch.Status)
	}
	if ch.ClaimedBy != "codex" {
		t.Fatalf("expected claim by codex, got %q", ch.ClaimedBy)
	}
}

func TestExecuteChange_RefusesUnapprovedWork(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	svc := NewService(store, Options{PlanStore: ps, Executor: &fakeExecutor{}})
	ctx := context.Background()

	// Release approved, but the change itself is only plan_ready (not approved).
	seedRelease(t, store, &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusApproved})
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", ReleaseID: "R-1", Status: governance.ChangeStatusPlanReady,
		SourceType: governance.SourceManual, SourceID: "C-1",
	})
	ps.stories["US-1"] = releaseStory("US-1", []string{"T-1"})
	_ = store.LinkChangeStory(ctx, "C-1", "US-1")

	if _, err := svc.ExecuteChange(ctx, "C-1", "codex", ""); err == nil {
		t.Fatalf("expected execute to refuse an unapproved change")
	}
}

func TestExecuteChange_RequiresExecutor(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{PlanStore: newFakePlanStore()}) // no Executor
	if _, err := svc.ExecuteChange(context.Background(), "C-1", "a", ""); !errors.Is(err, errMissingExecutor) {
		t.Fatalf("expected errMissingExecutor, got %v", err)
	}
}
