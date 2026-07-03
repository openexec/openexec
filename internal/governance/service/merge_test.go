package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/policy"
)

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

	// Low-risk change, auto-merge policy on, but NO evidence yet → refused.
	mergeChange(t, store, "C-1", governance.RiskLow)
	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err == nil || !strings.Contains(err.Error(), "no verification evidence") {
		t.Fatalf("expected refusal without evidence, got %v", err)
	}
	if r.merged {
		t.Fatalf("must not merge without evidence")
	}

	// Add verification evidence → auto-merge now allowed.
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindCI, governance.EvidenceSourceCLI, "ci green", "", ""); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if err := svc.MergeChange(ctx, "C-1", "", "squash"); err != nil {
		t.Fatalf("expected auto-merge with policy+evidence, got %v", err)
	}
	if !r.merged {
		t.Fatalf("expected the PR to be merged")
	}

	// A HIGH-risk change is still refused (policy only opted low in).
	mergeChange(t, store, "C-2", governance.RiskHigh)
	_ = svc.RecordEvidence(ctx, "C-2", governance.EvidenceKindCI, governance.EvidenceSourceCLI, "ci green", "", "")
	if err := svc.MergeChange(ctx, "C-2", "", "squash"); err == nil {
		t.Fatalf("expected high-risk auto-merge to remain refused")
	}
}
