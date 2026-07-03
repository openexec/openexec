package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

func seedCriticalReviews(t *testing.T, store governance.Store, changeID string, version int) {
	t.Helper()
	seedDecision(t, store, &governance.DecisionEvent{
		ChangeID: changeID, Actor: "bugbot", ActorType: governance.ActorTypeAI,
		Decision: governance.DecisionRecommendedApproval, ProposalVersion: version,
	})
	seedDecision(t, store, &governance.DecisionEvent{
		ChangeID: changeID, Actor: "security_ai", ActorType: governance.ActorTypeAI,
		Decision: governance.DecisionRecommendedApproval, ProposalVersion: version,
	})
}

func TestApproveChange_CriticalRequiresRiskAcceptance(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusPlanReady, Risk: governance.RiskCritical,
		ProposalVersion: 1, AcceptanceCriteria: "ac", VerificationPlan: "vp",
	})
	seedCriticalReviews(t, store, "C-1", 1)

	// Reviews present, but no risk acceptance → refused.
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err == nil || !strings.Contains(err.Error(), "risk-acceptance") {
		t.Fatalf("expected risk-acceptance requirement, got %v", err)
	}
	// Accept risk (pm is human + holds risk_accept), then approval succeeds.
	if err := svc.AcceptRisk(ctx, "C-1", "pm", "accepted for pilot"); err != nil {
		t.Fatalf("AcceptRisk: %v", err)
	}
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err != nil {
		t.Fatalf("approve after risk-accept: %v", err)
	}
}

func TestAcceptRisk_RevisionInvalidatesAcceptance(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusPlanReady, Risk: governance.RiskCritical,
		ProposalVersion: 1, AcceptanceCriteria: "ac", VerificationPlan: "vp",
	})
	if err := svc.AcceptRisk(ctx, "C-1", "pm", "v1"); err != nil {
		t.Fatalf("AcceptRisk: %v", err)
	}

	// A plan revision bumps the proposal version; the v1 acceptance is now stale.
	ch, _ := store.GetChangeRecord(ctx, "C-1")
	ch.ProposalVersion = 2
	if err := store.UpdateChangeRecord(ctx, ch); err != nil {
		t.Fatalf("bump version: %v", err)
	}
	seedCriticalReviews(t, store, "C-1", 2)
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err == nil || !strings.Contains(err.Error(), "risk-acceptance") {
		t.Fatalf("stale (v1) acceptance must not satisfy v2 approval, got %v", err)
	}
	// Re-accept at v2 → approval succeeds.
	if err := svc.AcceptRisk(ctx, "C-1", "pm", "v2"); err != nil {
		t.Fatalf("re-accept: %v", err)
	}
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err != nil {
		t.Fatalf("approve after re-accept: %v", err)
	}
}

func TestAcceptRisk_RequiresOperatorSession(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{}) // no operator session
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusPlanReady, Risk: governance.RiskCritical})
	if err := svc.AcceptRisk(ctx, "C-1", "pm", "x"); err == nil {
		t.Fatalf("risk acceptance must require an operator session")
	}
}
