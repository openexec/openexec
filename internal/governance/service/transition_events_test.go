package service

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// hasDecision reports whether any event in the store carries the given decision
// token (searching the whole trail, including release-level events).
func hasDecision(t *testing.T, store governance.Store, decision string) bool {
	t.Helper()
	evs, err := store.ListAllDecisionEvents(context.Background())
	if err != nil {
		t.Fatalf("list all events: %v", err)
	}
	for _, e := range evs {
		if e.Decision == decision {
			return true
		}
	}
	return false
}

// TestTransitions_LeaveDecisionEvents proves the audit trail is continuous: the
// system-driven status advances that previously changed status silently now each
// record a decision event.
func TestTransitions_LeaveDecisionEvents(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	// RecordPR: implementing -> pr_open.
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusImplementing, Risk: governance.RiskLow,
	})
	if err := svc.RecordPR(ctx, "C-1", "https://github.com/org/repo/pull/7", "feat/x"); err != nil {
		t.Fatalf("RecordPR: %v", err)
	}
	if !hasDecision(t, store, decisionPROpened) {
		t.Fatalf("RecordPR left no pr_opened event")
	}

	// ReadyForTest: pr_open -> ready_for_test (PR URL satisfies the gate).
	if err := svc.ReadyForTest(ctx, "C-1"); err != nil {
		t.Fatalf("ReadyForTest: %v", err)
	}
	if !hasDecision(t, store, decisionReadyForTest) {
		t.Fatalf("ReadyForTest left no ready_for_test event")
	}

	// StartRelease: approved -> implementing.
	seedRelease(t, store, &governance.GovernanceRelease{
		ID: "R-1", Status: governance.ReleaseStatusApproved,
	})
	if err := svc.StartRelease(ctx, "R-1"); err != nil {
		t.Fatalf("StartRelease: %v", err)
	}
	if !hasDecision(t, store, decisionReleaseStarted) {
		t.Fatalf("StartRelease left no release_started event")
	}

	// PlanRelease: a fresh draft release -> planned.
	seedRelease(t, store, &governance.GovernanceRelease{
		ID: "R-2", Status: governance.ReleaseStatusDraft,
	})
	if err := svc.PlanRelease(ctx, "R-2"); err != nil {
		t.Fatalf("PlanRelease: %v", err)
	}
	if !hasDecision(t, store, decisionReleasePlanned) {
		t.Fatalf("PlanRelease left no release_planned event")
	}

	// The whole trail must still verify as an intact hash chain.
	ok, reason, _, err := store.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("audit chain broken after transitions: %s", reason)
	}
}
