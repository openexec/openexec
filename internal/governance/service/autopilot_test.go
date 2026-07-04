package service

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/governance"
)

func TestNextActionable(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seed := func(id, status string, ageHours int) {
		seedChange(t, store, &governance.ChangeRecord{
			ID: id, ProjectID: "p", Title: id, Status: status,
			CreatedAt: base.Add(time.Duration(ageHours) * time.Hour),
		})
	}

	// Terminal + parked are never actionable; an actionable candidate exists.
	seed("CH-done", governance.ChangeStatusDone, 0)
	seed("CH-parked", governance.ChangeStatusChangesRequested, 1)
	seed("CH-planready", governance.ChangeStatusPlanReady, 2) // awaits approval → parked
	seed("CH-approved", governance.ChangeStatusApprovedForAI, 4)
	seed("CH-candidate", governance.ChangeStatusCandidate, 3) // older than approved

	ch, action, err := svc.NextActionable(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil || ch.ID != "CH-candidate" || action != "triage" {
		t.Fatalf("expected FIFO pick CH-candidate/triage, got %+v action=%q", ch, action)
	}

	// Single slot: an implementing change occupies it — nothing new starts.
	seed("CH-impl", governance.ChangeStatusImplementing, 9)
	ch, action, err = svc.NextActionable(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if ch == nil || ch.ID != "CH-impl" || action != "in-progress" {
		t.Fatalf("expected single-slot CH-impl/in-progress, got %+v action=%q", ch, action)
	}
}

func TestNextActionable_NothingToDo(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	seedChange(t, store, &governance.ChangeRecord{ID: "CH-1", ProjectID: "p", Status: governance.ChangeStatusChangesRequested})
	ch, _, err := svc.NextActionable(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if ch != nil {
		t.Fatalf("expected no actionable work (only a parked change), got %s", ch.ID)
	}
}
