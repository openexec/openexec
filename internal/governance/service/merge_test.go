package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/policy"
)

// failMergeRunner authorizes everything but fails the actual `pr merge` call.
type failMergeRunner struct{}

func (failMergeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "pr" && args[1] == "merge" {
		return nil, errors.New("gh merge failed")
	}
	return []byte("{}"), nil
}

func TestMerge_FailureRecordsAuthorizedAndFailed(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{Runner: failMergeRunner{}, OperatorSession: true})
	ctx := context.Background()
	mergeChange(t, store, "C-1", governance.RiskHigh)

	if err := svc.MergeChange(ctx, "C-1", "pm", "squash"); err == nil {
		t.Fatalf("expected the merge to fail")
	}
	// The trail must show the authorization AND the failure (external side effect
	// is never silent), and the change must not have advanced to done.
	evs, _ := store.ListDecisionEvents(ctx, "C-1")
	var sawAuth, sawFail, sawMerged bool
	for _, e := range evs {
		switch e.Decision {
		case decisionMergeAuthorized:
			sawAuth = true
		case decisionMergeFailed:
			sawFail = true
		case decisionMerged:
			sawMerged = true
		}
	}
	if !sawAuth || !sawFail || sawMerged {
		t.Fatalf("want authorized+failed, no merged; got auth=%v fail=%v merged=%v", sawAuth, sawFail, sawMerged)
	}
	if ch, _ := store.GetChangeRecord(ctx, "C-1"); ch.Status == governance.ChangeStatusDone {
		t.Fatalf("a failed merge must not mark the change done")
	}
}

// setOperability seeds a stored operability report for a change.
func setOperability(t *testing.T, store governance.Store, changeID, rollback, dbmig, risk string) {
	t.Helper()
	raw, _ := json.Marshal(map[string]string{"rollback_safe": rollback, "db_migration": dbmig, "deploy_risk": risk})
	if err := store.SetChangeOperability(context.Background(), changeID, string(raw)); err != nil {
		t.Fatalf("set operability: %v", err)
	}
}

// seedTrustedEvidence records externally-verified (github) CI evidence directly
// via the store, exactly as the connector's SyncGitHubChecks would — the manual
// RecordEvidence path now refuses a trusted source, so tests must seed it here.
func seedTrustedEvidence(t *testing.T, store governance.Store, changeID string) {
	t.Helper()
	if err := store.CreateEvidence(context.Background(), &governance.Evidence{
		ID:       newID(),
		ChangeID: changeID,
		Kind:     governance.EvidenceKindCI,
		Source:   governance.EvidenceSourceGitHub,
		Summary:  "GitHub checks green (test seed)",
	}); err != nil {
		t.Fatalf("seed trusted evidence: %v", err)
	}
}

// mergeRunner records whether a merge was actually invoked.
type mergeRunner struct{ merged bool }

func (r *mergeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "pr" && args[1] == "merge" {
		r.merged = true
	}
	return []byte("{}"), nil
}

func mergeChange(t *testing.T, store governance.Store, id, risk string) {
	t.Helper()
	seedChange(t, store, &governance.ChangeRecord{
		ID: id, Status: governance.ChangeStatusReadyForTest, Risk: risk,
		PRURL:      "https://github.com/org/repo/pull/42",
		SourceType: governance.SourceManual, SourceID: id,
	})
}

// markDone advances a change to done (as MarkDone would), for auto-merge tests.
func markDone(t *testing.T, store governance.Store, id string) {
	t.Helper()
	ch, err := store.GetChangeRecord(context.Background(), id)
	if err != nil {
		t.Fatalf("get change: %v", err)
	}
	ch.Status = governance.ChangeStatusDone
	if err := store.UpdateChangeRecord(context.Background(), ch); err != nil {
		t.Fatalf("mark done: %v", err)
	}
}

func TestMerge_RefusedByDefault(t *testing.T) {
	store := newTestStore(t)
	r := &mergeRunner{}
	// No operator session; DefaultPolicy => no tier auto-merges.
	svc := NewService(store, Options{Runner: r})
	ctx := context.Background()
	mergeChange(t, store, "C-1", governance.RiskLow)

	err := svc.MergeChange(ctx, "C-1", "pm", "squash")
	if err == nil || !strings.Contains(err.Error(), "auto-merge is not allowed") {
		t.Fatalf("expected default refusal, got %v", err)
	}
	if r.merged {
		t.Fatalf("PR must NOT have been merged")
	}
}

func TestMerge_AllowedByOperatorWithApprover(t *testing.T) {
	store := newTestStore(t)
	r := &mergeRunner{}
	svc := NewService(store, Options{Runner: r, OperatorSession: true})
	ctx := context.Background()
	mergeChange(t, store, "C-1", governance.RiskHigh) // high risk

	// pm is human + approve + critical limit → may authorize a high-risk merge.
	if err := svc.MergeChange(ctx, "C-1", "pm", "squash"); err != nil {
		t.Fatalf("expected operator merge to succeed, got %v", err)
	}
	if !r.merged {
		t.Fatalf("expected the PR to be merged")
	}
	// An AI/verifier authority may not authorize a high-risk merge even in an
	// operator session.
	mergeChange(t, store, "C-2", governance.RiskHigh)
	if err := svc.MergeChange(ctx, "C-2", "ci_verifier", "squash"); err == nil {
		t.Fatalf("expected refusal: ci_verifier cannot approve high-risk merge")
	}
}

func TestMerge_AutoMergeOnlyWithPolicyAndEvidence(t *testing.T) {
	store := newTestStore(t)
	r := &mergeRunner{}
	// Policy opts LOW risk into auto-merge.
	p := policy.DefaultPolicy()
	low := p.RiskTiers[governance.RiskLow]
	low.AutoMergeAllowed = true
	p.RiskTiers[governance.RiskLow] = low
	svc := NewService(store, Options{Runner: r, Policy: p}) // NO operator session
	ctx := context.Background()

	// Low-risk change, auto-merge policy on, but neither operability cleared nor
	// evidence recorded → refused (fails closed).
	mergeChange(t, store, "C-1", governance.RiskLow)
	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err == nil {
		t.Fatalf("expected refusal with no operability/evidence")
	}
	if r.merged {
		t.Fatalf("must not merge without operability + evidence")
	}

	// Move to done + record EXTERNALLY-verified (github) evidence, but operability
	// not yet cleared → still blocked (operability is a hard gate).
	markDone(t, store, "C-1")
	seedTrustedEvidence(t, store, "C-1")
	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err == nil || !strings.Contains(err.Error(), "operability") {
		t.Fatalf("expected operability block without a cleared review, got %v", err)
	}
	// Clear operability → auto-merge now allowed (policy + done + github evidence + operability).
	setOperability(t, store, "C-1", "yes", "none", "low")
	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err != nil {
		t.Fatalf("expected auto-merge, got %v", err)
	}
	if !r.merged {
		t.Fatalf("expected the PR to be merged")
	}

	// A HIGH-risk change is still refused (policy only opted low in).
	mergeChange(t, store, "C-2", governance.RiskHigh)
	markDone(t, store, "C-2")
	seedTrustedEvidence(t, store, "C-2")
	if err := svc.MergeChange(ctx, "C-2", "", "squash"); err == nil {
		t.Fatalf("expected high-risk auto-merge to remain refused")
	}
}

func TestMerge_AutoMergeRequiresDoneAndExternalEvidence(t *testing.T) {
	store := newTestStore(t)
	r := &mergeRunner{}
	p := policy.DefaultPolicy()
	low := p.RiskTiers[governance.RiskLow]
	low.AutoMergeAllowed = true
	p.RiskTiers[governance.RiskLow] = low
	svc := NewService(store, Options{Runner: r, Policy: p}) // no operator session
	ctx := context.Background()

	// Operability clear, evidence present, but status is only ready_for_test (not
	// done) → auto-merge refused (the full MarkDone gate hasn't run).
	mergeChange(t, store, "C-1", governance.RiskLow)
	setOperability(t, store, "C-1", "yes", "none", "low")
	seedTrustedEvidence(t, store, "C-1")
	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err == nil || !strings.Contains(err.Error(), "done") {
		t.Fatalf("expected refusal because change is not done, got %v", err)
	}

	// Done + operability clear, but only AGENT-self-reported evidence → refused.
	markDone(t, store, "C-1")
	// (C-1 already has a github evidence row above; use a fresh change with only agent evidence)
	mergeChange(t, store, "C-2", governance.RiskLow)
	markDone(t, store, "C-2")
	setOperability(t, store, "C-2", "yes", "none", "low")
	_ = svc.RecordEvidence(ctx, "C-2", governance.EvidenceKindTest, governance.EvidenceSourceAgent, "self-reported", "", "")
	if err := svc.MergeChange(ctx, "C-2", "", "squash"); err == nil || !strings.Contains(err.Error(), "externally-verified") {
		t.Fatalf("expected refusal on agent-self-reported evidence, got %v", err)
	}
	if r.merged {
		t.Fatalf("neither change should have merged")
	}
}

func TestMerge_OperabilityBlocksAutoMergeEvenWithEvidence(t *testing.T) {
	store := newTestStore(t)
	r := &mergeRunner{}
	p := policy.DefaultPolicy()
	low := p.RiskTiers[governance.RiskLow]
	low.AutoMergeAllowed = true
	p.RiskTiers[governance.RiskLow] = low
	svc := NewService(store, Options{Runner: r, Policy: p}) // no operator session
	ctx := context.Background()

	mergeChange(t, store, "C-1", governance.RiskLow)
	markDone(t, store, "C-1")
	seedTrustedEvidence(t, store, "C-1")
	// Policy + done + external evidence present, but a DESTRUCTIVE migration →
	// operability blocks auto-merge; a human operator must own this deploy.
	setOperability(t, store, "C-1", "no", "destructive", "high")

	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err == nil || !strings.Contains(err.Error(), "operability") {
		t.Fatalf("expected operability to block a destructive-migration auto-merge, got %v", err)
	}
	if r.merged {
		t.Fatalf("a destructive-migration change must NOT auto-merge")
	}
}
