package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

func TestTriageLight_SingleTaskAndOperatorApprovalWaivesReview(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	// No completer => risk defaults to low; operator session so approval is allowed.
	svc := NewService(store, Options{PlanStore: ps, OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusCandidate,
		Title: "Bump a constant", RawText: "Change MAX to 40.",
	})

	res, err := svc.TriageLight(ctx, "C-1", "operator")
	if err != nil {
		t.Fatalf("TriageLight: %v", err)
	}
	if len(res.StoryIDs) != 1 {
		t.Fatalf("expected 1 story, got %d", len(res.StoryIDs))
	}
	ch, _ := store.GetChangeRecord(ctx, "C-1")
	if !ch.Light {
		t.Fatalf("change should be marked light")
	}
	if ch.Status != governance.ChangeStatusPlanReady {
		t.Fatalf("expected plan_ready, got %s", ch.Status)
	}

	// Operator approval must succeed WITHOUT any AI review on record (waived).
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err != nil {
		t.Fatalf("light-lane operator approval should succeed without AI review, got %v", err)
	}
	ch, _ = store.GetChangeRecord(ctx, "C-1")
	if ch.Status != governance.ChangeStatusApprovedForAI {
		t.Fatalf("expected approved_for_ai, got %s", ch.Status)
	}
	// The waiver must be visible in the audit trail.
	events, _, _ := svc.History(ctx, "C-1")
	var sawWaiver bool
	for _, e := range events {
		if e.Decision == governance.DecisionApproved && strings.Contains(e.Comment, "AI review waived") {
			sawWaiver = true
		}
	}
	if !sawWaiver {
		t.Fatalf("approval event should record the AI-review waiver")
	}
}

func TestTriage_RetriageSupersedesPriorDecomposition(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	svc := NewService(store, Options{PlanStore: ps})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusCandidate, Title: "Trivial", RawText: "do x",
	})

	if _, err := svc.TriageLight(ctx, "C-1", "operator"); err != nil {
		t.Fatalf("first quickplan: %v", err)
	}

	// Re-triage (e.g. after a revision). The change must end up owning ONLY the
	// new decomposition — the prior one is superseded, not accumulated.
	second, err := svc.TriageLight(ctx, "C-1", "operator")
	if err != nil {
		t.Fatalf("re-quickplan: %v", err)
	}
	links, _ := store.ListChangeStories(ctx, "C-1")
	if len(links) != 1 {
		t.Fatalf("re-triage must leave exactly 1 linked story, got %d: %v", len(links), links)
	}
	// The backlog must not accumulate the prior story: exactly one story total.
	if len(ps.stories) != 1 {
		t.Fatalf("backlog accumulated stories: want 1, got %d", len(ps.stories))
	}
	if ps.GetStory(second.StoryIDs[0]) == nil {
		t.Fatalf("new story %s should exist", second.StoryIDs[0])
	}
}

func TestTriageLight_RefusesHighRisk(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	// Completer classifies the change as critical → light lane must refuse.
	svc := NewService(store, Options{
		PlanStore: ps,
		Completer: fakeCompleter{reply: "kind: security\nrisk: critical\n"},
	})
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusCandidate,
		Title: "Rework auth", RawText: "Replace the auth system.",
	})

	if _, err := svc.TriageLight(ctx, "C-1", "operator"); err == nil || !strings.Contains(err.Error(), "lightweight lane is only for trivial") {
		t.Fatalf("expected high-risk refusal, got %v", err)
	}
}
