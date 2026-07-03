package validation

import (
	"errors"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/governance"
)

func TestValidateReleaseTransition(t *testing.T) {
	cases := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		// Valid linear path.
		{"draft->planned", governance.ReleaseStatusDraft, governance.ReleaseStatusPlanned, false},
		{"planned->approved", governance.ReleaseStatusPlanned, governance.ReleaseStatusApproved, false},
		{"approved->implementing", governance.ReleaseStatusApproved, governance.ReleaseStatusImplementing, false},
		{"implementing->ready_for_test", governance.ReleaseStatusImplementing, governance.ReleaseStatusReadyForTest, false},
		{"ready_for_test->testing", governance.ReleaseStatusReadyForTest, governance.ReleaseStatusTesting, false},
		{"testing->ready_to_deploy", governance.ReleaseStatusTesting, governance.ReleaseStatusReadyToDeploy, false},
		{"ready_to_deploy->deployed", governance.ReleaseStatusReadyToDeploy, governance.ReleaseStatusDeployed, false},
		{"deployed->closed", governance.ReleaseStatusDeployed, governance.ReleaseStatusClosed, false},
		// Blocking from active states.
		{"draft->blocked", governance.ReleaseStatusDraft, governance.ReleaseStatusBlocked, false},
		{"deployed->blocked", governance.ReleaseStatusDeployed, governance.ReleaseStatusBlocked, false},
		// Resume from blocked into any active state.
		{"blocked->implementing", governance.ReleaseStatusBlocked, governance.ReleaseStatusImplementing, false},
		{"blocked->draft", governance.ReleaseStatusBlocked, governance.ReleaseStatusDraft, false},
		// Cancel only from draft/planned.
		{"draft->cancelled", governance.ReleaseStatusDraft, governance.ReleaseStatusCancelled, false},
		{"planned->cancelled", governance.ReleaseStatusPlanned, governance.ReleaseStatusCancelled, false},

		// Invalid transitions.
		{"draft->approved skip", governance.ReleaseStatusDraft, governance.ReleaseStatusApproved, true},
		{"approved->cancelled", governance.ReleaseStatusApproved, governance.ReleaseStatusCancelled, true},
		{"implementing->cancelled", governance.ReleaseStatusImplementing, governance.ReleaseStatusCancelled, true},
		{"blocked->closed", governance.ReleaseStatusBlocked, governance.ReleaseStatusClosed, true},
		{"blocked->cancelled", governance.ReleaseStatusBlocked, governance.ReleaseStatusCancelled, true},
		{"closed->anything", governance.ReleaseStatusClosed, governance.ReleaseStatusDeployed, true},
		{"cancelled->anything", governance.ReleaseStatusCancelled, governance.ReleaseStatusDraft, true},
		{"deployed->testing backward", governance.ReleaseStatusDeployed, governance.ReleaseStatusTesting, true},
		{"self transition", governance.ReleaseStatusDraft, governance.ReleaseStatusDraft, true},
		{"unknown from", "bogus", governance.ReleaseStatusDraft, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateReleaseTransition(tc.from, tc.to)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s->%s, got nil", tc.from, tc.to)
				}
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("expected ErrInvalidTransition, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error for %s->%s, got %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateChangeTransition(t *testing.T) {
	cases := []struct {
		name    string
		from    string
		to      string
		wantErr bool
	}{
		// Valid path.
		{"candidate->planned", governance.ChangeStatusCandidate, governance.ChangeStatusPlanned, false},
		{"planned->plan_ready", governance.ChangeStatusPlanned, governance.ChangeStatusPlanReady, false},
		{"plan_ready->changes_requested", governance.ChangeStatusPlanReady, governance.ChangeStatusChangesRequested, false},
		{"changes_requested->plan_ready", governance.ChangeStatusChangesRequested, governance.ChangeStatusPlanReady, false},
		{"plan_ready->approved_for_ai", governance.ChangeStatusPlanReady, governance.ChangeStatusApprovedForAI, false},
		{"approved_for_ai->implementing", governance.ChangeStatusApprovedForAI, governance.ChangeStatusImplementing, false},
		{"implementing->pr_open", governance.ChangeStatusImplementing, governance.ChangeStatusPROpen, false},
		{"pr_open->ready_for_test", governance.ChangeStatusPROpen, governance.ChangeStatusReadyForTest, false},
		{"ready_for_test->done", governance.ChangeStatusReadyForTest, governance.ChangeStatusDone, false},
		// Reject/defer edges.
		{"candidate->rejected", governance.ChangeStatusCandidate, governance.ChangeStatusRejected, false},
		{"planned->deferred", governance.ChangeStatusPlanned, governance.ChangeStatusDeferred, false},
		{"plan_ready->rejected", governance.ChangeStatusPlanReady, governance.ChangeStatusRejected, false},
		{"plan_ready->deferred", governance.ChangeStatusPlanReady, governance.ChangeStatusDeferred, false},

		// Invalid.
		{"candidate->plan_ready skip", governance.ChangeStatusCandidate, governance.ChangeStatusPlanReady, true},
		{"implementing->rejected", governance.ChangeStatusImplementing, governance.ChangeStatusRejected, true},
		{"approved_for_ai->deferred", governance.ChangeStatusApprovedForAI, governance.ChangeStatusDeferred, true},
		{"done->anything", governance.ChangeStatusDone, governance.ChangeStatusReadyForTest, true},
		{"rejected->anything", governance.ChangeStatusRejected, governance.ChangeStatusCandidate, true},
		{"deferred->anything", governance.ChangeStatusDeferred, governance.ChangeStatusCandidate, true},
		{"changes_requested->approved skip", governance.ChangeStatusChangesRequested, governance.ChangeStatusApprovedForAI, true},
		{"self transition", governance.ChangeStatusPlanned, governance.ChangeStatusPlanned, true},
		{"unknown from", "bogus", governance.ChangeStatusPlanned, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateChangeTransition(tc.from, tc.to)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s->%s, got nil", tc.from, tc.to)
				}
				if !errors.Is(err, ErrInvalidTransition) {
					t.Fatalf("expected ErrInvalidTransition, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("expected no error for %s->%s, got %v", tc.from, tc.to, err)
			}
		})
	}
}

func TestValidateApproveRelease(t *testing.T) {
	rel := &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusPlanned}
	if err := ValidateApproveRelease(rel, []*governance.ReleaseItem{{ReleaseID: "R-1", ChangeID: "C-1"}}); err != nil {
		t.Fatalf("expected pass with one item, got %v", err)
	}
	err := ValidateApproveRelease(rel, nil)
	if !errors.Is(err, ErrNoItems) {
		t.Fatalf("expected ErrNoItems, got %v", err)
	}
}

func TestValidateApproveChange(t *testing.T) {
	good := &governance.ChangeRecord{ID: "C-1", AcceptanceCriteria: "works", VerificationPlan: "go test"}
	if err := ValidateApproveChange(good); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	noAC := &governance.ChangeRecord{ID: "C-1", AcceptanceCriteria: "  ", VerificationPlan: "go test"}
	if err := ValidateApproveChange(noAC); !errors.Is(err, ErrMissingAcceptanceCriteria) {
		t.Fatalf("expected ErrMissingAcceptanceCriteria, got %v", err)
	}
	noVP := &governance.ChangeRecord{ID: "C-1", AcceptanceCriteria: "works", VerificationPlan: ""}
	if err := ValidateApproveChange(noVP); !errors.Is(err, ErrMissingVerificationPlan) {
		t.Fatalf("expected ErrMissingVerificationPlan, got %v", err)
	}
}

func TestValidateImplementable(t *testing.T) {
	ch := &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusApprovedForAI, ProposalVersion: 2, ApprovedVersion: 2}
	approved := &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusApproved}
	if err := ValidateImplementable(approved, ch); err != nil {
		t.Fatalf("expected pass (approved + fresh), got %v", err)
	}
	implementing := &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusImplementing}
	if err := ValidateImplementable(implementing, ch); err != nil {
		t.Fatalf("expected pass (implementing + fresh), got %v", err)
	}
	// Renewal: a change already implementing remains implementable.
	implCh := &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusImplementing, ProposalVersion: 2, ApprovedVersion: 2}
	if err := ValidateImplementable(implementing, implCh); err != nil {
		t.Fatalf("expected pass (implementing change renewal), got %v", err)
	}
	draft := &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusDraft}
	if err := ValidateImplementable(draft, ch); !errors.Is(err, ErrReleaseNotImplementable) {
		t.Fatalf("expected ErrReleaseNotImplementable, got %v", err)
	}
	// Status gate (direct-claim bypass guard): an unapproved change in an
	// approved release must be rejected even when versions match (0==0).
	candidate := &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusCandidate}
	if err := ValidateImplementable(approved, candidate); !errors.Is(err, ErrChangeNotApproved) {
		t.Fatalf("expected ErrChangeNotApproved for candidate change, got %v", err)
	}
	planReady := &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusPlanReady, ProposalVersion: 1}
	if err := ValidateImplementable(approved, planReady); !errors.Is(err, ErrChangeNotApproved) {
		t.Fatalf("expected ErrChangeNotApproved for plan_ready change, got %v", err)
	}
	stale := &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusApprovedForAI, ProposalVersion: 3, ApprovedVersion: 2}
	if err := ValidateImplementable(approved, stale); !errors.Is(err, ErrStaleApproval) {
		t.Fatalf("expected ErrStaleApproval, got %v", err)
	}
}

func TestValidateClaim(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	unclaimed := &governance.ChangeRecord{ID: "C-1"}
	if err := ValidateClaim(unclaimed, "codex", now); err != nil {
		t.Fatalf("expected pass for unclaimed, got %v", err)
	}
	sameAgent := &governance.ChangeRecord{ID: "C-1", ClaimedBy: "codex", ClaimExpiresAt: &future}
	if err := ValidateClaim(sameAgent, "codex", now); err != nil {
		t.Fatalf("expected pass for re-claim by same agent, got %v", err)
	}
	expired := &governance.ChangeRecord{ID: "C-1", ClaimedBy: "other", ClaimExpiresAt: &past}
	if err := ValidateClaim(expired, "codex", now); err != nil {
		t.Fatalf("expected pass for expired lease, got %v", err)
	}
	active := &governance.ChangeRecord{ID: "C-1", ClaimedBy: "other", ClaimExpiresAt: &future}
	if err := ValidateClaim(active, "codex", now); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed, got %v", err)
	}
}

func TestValidateReadyForTest(t *testing.T) {
	withPR := &governance.ChangeRecord{ID: "C-1", PRURL: "https://example/pr/1"}
	if err := ValidateReadyForTest(withPR, false); err != nil {
		t.Fatalf("expected pass with PR, got %v", err)
	}
	noPR := &governance.ChangeRecord{ID: "C-1"}
	if err := ValidateReadyForTest(noPR, true); err != nil {
		t.Fatalf("expected pass with manual evidence, got %v", err)
	}
	if err := ValidateReadyForTest(noPR, false); !errors.Is(err, ErrNoPRNoEvidence) {
		t.Fatalf("expected ErrNoPRNoEvidence, got %v", err)
	}
}

func TestValidateMarkDone(t *testing.T) {
	good := []*governance.Evidence{{Kind: governance.EvidenceKindTest}}
	if err := ValidateMarkDone(good); err != nil {
		t.Fatalf("expected pass with test evidence, got %v", err)
	}
	// Deploy-only evidence does not verify the change.
	deployOnly := []*governance.Evidence{{Kind: governance.EvidenceKindDeploy}}
	if err := ValidateMarkDone(deployOnly); !errors.Is(err, ErrNoVerificationEvidence) {
		t.Fatalf("expected ErrNoVerificationEvidence for deploy-only, got %v", err)
	}
	if err := ValidateMarkDone(nil); !errors.Is(err, ErrNoVerificationEvidence) {
		t.Fatalf("expected ErrNoVerificationEvidence for empty, got %v", err)
	}
}

func TestValidateCloseCustomerFacing(t *testing.T) {
	// Non customer-facing: no artifact required.
	if err := ValidateCloseCustomerFacing(nil, false); err != nil {
		t.Fatalf("expected pass for non customer-facing, got %v", err)
	}
	withCustomer := []*governance.CommunicationArtifact{{Audience: governance.AudienceCustomer}}
	if err := ValidateCloseCustomerFacing(withCustomer, true); err != nil {
		t.Fatalf("expected pass with customer artifact, got %v", err)
	}
	withSupport := []*governance.CommunicationArtifact{{Audience: governance.AudienceSupport}}
	if err := ValidateCloseCustomerFacing(withSupport, true); err != nil {
		t.Fatalf("expected pass with support artifact, got %v", err)
	}
	onlyPM := []*governance.CommunicationArtifact{{Audience: governance.AudiencePM}}
	if err := ValidateCloseCustomerFacing(onlyPM, true); !errors.Is(err, ErrMissingCustomerComms) {
		t.Fatalf("expected ErrMissingCustomerComms, got %v", err)
	}
}

func TestValidateScopeAddInvalidatesApproval(t *testing.T) {
	approved := &governance.GovernanceRelease{Status: governance.ReleaseStatusApproved}
	if !ValidateScopeAddInvalidatesApproval(approved) {
		t.Fatal("expected mustReapprove=true for approved release")
	}
	planned := &governance.GovernanceRelease{Status: governance.ReleaseStatusPlanned}
	if ValidateScopeAddInvalidatesApproval(planned) {
		t.Fatal("expected mustReapprove=false for planned release")
	}
}

func TestValidateSeparationOfDuties(t *testing.T) {
	ch := &governance.ChangeRecord{ID: "C-1"}

	violation := []*governance.DecisionEvent{
		{Actor: "alice", Decision: governance.DecisionProposed},
		{Actor: "alice", Decision: governance.DecisionApproved},
		{Actor: "alice", Decision: governance.DecisionMarkedDone},
	}
	if err := ValidateSeparationOfDuties(ch, violation, governance.RiskHigh); !errors.Is(err, ErrSeparationOfDuties) {
		t.Fatalf("expected ErrSeparationOfDuties for high risk, got %v", err)
	}
	// Low risk is exempt even with the same actor everywhere.
	if err := ValidateSeparationOfDuties(ch, violation, governance.RiskLow); err != nil {
		t.Fatalf("expected pass for low risk, got %v", err)
	}
	// Distinct actors pass for medium risk.
	distinct := []*governance.DecisionEvent{
		{Actor: "alice", Decision: governance.DecisionProposed},
		{Actor: "bob", Decision: governance.DecisionApproved},
		{Actor: "carol", Decision: governance.DecisionMarkedDone},
	}
	if err := ValidateSeparationOfDuties(ch, distinct, governance.RiskMedium); err != nil {
		t.Fatalf("expected pass for distinct actors, got %v", err)
	}
	// Same actor on two of three roles is not a violation (needs all three).
	twoRoles := []*governance.DecisionEvent{
		{Actor: "alice", Decision: governance.DecisionProposed},
		{Actor: "alice", Decision: governance.DecisionApproved},
		{Actor: "bob", Decision: governance.DecisionMarkedDone},
	}
	if err := ValidateSeparationOfDuties(ch, twoRoles, governance.RiskCritical); err != nil {
		t.Fatalf("expected pass when actor holds only two roles, got %v", err)
	}
}
