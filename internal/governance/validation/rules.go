package validation

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/governance"
)

// Sentinel errors for the structural/state validation rules. Each wrapped
// error message explains what is wrong and what is needed to proceed.
var (
	// ErrNoItems is returned when approving a release that has no items.
	ErrNoItems = errors.New("governance: release has no items")
	// ErrMissingAcceptanceCriteria is returned when approving a change that
	// has no acceptance criteria.
	ErrMissingAcceptanceCriteria = errors.New("governance: change is missing acceptance criteria")
	// ErrMissingVerificationPlan is returned when approving a change that has
	// no verification plan.
	ErrMissingVerificationPlan = errors.New("governance: change is missing a verification plan")
	// ErrReleaseNotImplementable is returned when implementation is attempted
	// while the release is not in an implementable state.
	ErrReleaseNotImplementable = errors.New("governance: release is not in an implementable state")
	// ErrChangeNotApproved is returned when implementation is attempted on a
	// change that has not been approved for AI execution.
	ErrChangeNotApproved = errors.New("governance: change is not approved for implementation")
	// ErrChangeNotTriageable is returned when a triage/plan write is attempted
	// on a change whose status does not permit planning.
	ErrChangeNotTriageable = errors.New("governance: change is not in a triageable state")
	// ErrReviewMissing is returned when approving a change for which policy
	// requires an AI or security review that has not been recorded.
	ErrReviewMissing = errors.New("governance: required review has not been recorded")
	// ErrStaleApproval is returned when a change's approved plan version no
	// longer matches its current proposal version.
	ErrStaleApproval = errors.New("governance: change approval is stale (proposal revised since approval)")
	// ErrAlreadyClaimed is returned when work is actively claimed by a
	// different executor.
	ErrAlreadyClaimed = errors.New("governance: work is actively claimed by another executor")
	// ErrNoPRNoEvidence is returned when marking ready-for-test without a PR
	// URL or configured manual evidence.
	ErrNoPRNoEvidence = errors.New("governance: ready-for-test requires a PR URL or manual evidence")
	// ErrNoVerificationEvidence is returned when marking done without any
	// verification evidence record.
	ErrNoVerificationEvidence = errors.New("governance: cannot mark done without verification evidence")
	// ErrMissingCustomerComms is returned when closing customer-facing work
	// without a customer/support communication artifact.
	ErrMissingCustomerComms = errors.New("governance: customer-facing work requires a customer or support communication artifact")
	// ErrSeparationOfDuties is returned when one actor performed every gated
	// role (propose, approve, mark done) on medium+ risk work.
	ErrSeparationOfDuties = errors.New("governance: separation of duties violated")
)

// ValidateApproveRelease rejects approval of a release that has zero items.
func ValidateApproveRelease(rel *governance.GovernanceRelease, items []*governance.ReleaseItem) error {
	if len(items) == 0 {
		return fmt.Errorf("%w: %s has no items; attach at least one change before approving", ErrNoItems, releaseID(rel))
	}
	return nil
}

// ValidateApproveChange rejects approval of a change that is missing acceptance
// criteria or a verification plan. Both are required before a plan can be
// approved for AI execution.
func ValidateApproveChange(ch *governance.ChangeRecord) error {
	if strings.TrimSpace(ch.AcceptanceCriteria) == "" {
		return fmt.Errorf("%w: add acceptance criteria to %s before approving", ErrMissingAcceptanceCriteria, changeID(ch))
	}
	if strings.TrimSpace(ch.VerificationPlan) == "" {
		return fmt.Errorf("%w: add a verification plan to %s before approving", ErrMissingVerificationPlan, changeID(ch))
	}
	return nil
}

// ValidateImplementable rejects implementation work unless the release is
// approved (or already implementing), the change itself is approved for AI (or
// already implementing — a renewal), and the change's approved plan version
// still matches its current proposal version (the stale-approval guard).
//
// The change-status gate is the primary safeguard against picking up unapproved
// work by ID: without it, a candidate change (ProposalVersion==ApprovedVersion==0)
// in an approved release would pass the version check and be claimable directly,
// bypassing approved_for_ai. ListApprovedWork already filters on status; this
// makes the same guarantee for direct claim-by-ID via CLI/MCP.
func ValidateImplementable(rel *governance.GovernanceRelease, ch *governance.ChangeRecord) error {
	if rel.Status != governance.ReleaseStatusApproved && rel.Status != governance.ReleaseStatusImplementing {
		return fmt.Errorf(
			"%w: %s is %q; must be %q or %q to implement",
			ErrReleaseNotImplementable, releaseID(rel), rel.Status,
			governance.ReleaseStatusApproved, governance.ReleaseStatusImplementing,
		)
	}
	if ch.Status != governance.ChangeStatusApprovedForAI && ch.Status != governance.ChangeStatusImplementing {
		return fmt.Errorf(
			"%w: %s is %q; must be %q before it can be implemented",
			ErrChangeNotApproved, changeID(ch), ch.Status, governance.ChangeStatusApprovedForAI,
		)
	}
	if ch.ApprovedVersion != ch.ProposalVersion {
		return fmt.Errorf(
			"%w: %s approved version %d != current proposal version %d; re-approve before implementing",
			ErrStaleApproval, changeID(ch), ch.ApprovedVersion, ch.ProposalVersion,
		)
	}
	return nil
}

// triageableStatuses are the change statuses from which a (re-)triage is legal:
// before a plan is locked in for execution. Triaging a change in flight
// (implementing/pr_open) or terminal (done/rejected/deferred) or already
// approved would rewrite scope behind an executor or resurrect closed work.
var triageableStatuses = map[string]bool{
	governance.ChangeStatusCandidate:        true,
	governance.ChangeStatusPlanned:          true,
	governance.ChangeStatusPlanReady:        true,
	governance.ChangeStatusChangesRequested: true,
}

// ValidateTriageable rejects a triage/plan write on a change whose current
// status is not one from which planning is legal. This guards the direct
// status write in ai.Triage (which does not go through the transition table
// because re-triage is a plan_ready -> plan_ready self-loop).
func ValidateTriageable(ch *governance.ChangeRecord) error {
	if triageableStatuses[ch.Status] {
		return nil
	}
	return fmt.Errorf(
		"%w: %s is %q; triage is only allowed from candidate, planned, plan_ready, or changes_requested",
		ErrChangeNotTriageable, changeID(ch), ch.Status,
	)
}

// ValidateClaim rejects a claim when the work is actively claimed by a
// different agent. A claim is active when ClaimExpiresAt is set and in the
// future. An expired lease, an unclaimed record, or a record already claimed by
// the same agent (re-claim/renewal) all pass.
func ValidateClaim(ch *governance.ChangeRecord, agent string, now time.Time) error {
	if ch.ClaimedBy == "" || ch.ClaimedBy == agent {
		return nil
	}
	if ch.ClaimExpiresAt != nil && ch.ClaimExpiresAt.After(now) {
		return fmt.Errorf(
			"%w: %s is claimed by %q until %s; cannot be claimed by %q",
			ErrAlreadyClaimed, changeID(ch), ch.ClaimedBy, ch.ClaimExpiresAt.Format(time.RFC3339), agent,
		)
	}
	return nil
}

// ValidateReadyForTest requires either a recorded PR URL or configured manual
// evidence before a change can move to ready-for-test.
func ValidateReadyForTest(ch *governance.ChangeRecord, hasManualEvidence bool) error {
	if strings.TrimSpace(ch.PRURL) != "" || hasManualEvidence {
		return nil
	}
	return fmt.Errorf(
		"%w: record a PR URL or manual evidence for %s before ready-for-test",
		ErrNoPRNoEvidence, changeID(ch),
	)
}

// verificationEvidenceKinds are the evidence kinds that count as proof a change
// was verified correct. Deploy and monitoring evidence document a rollout but
// do not by themselves verify the change, so they do not satisfy "mark done".
//
// Judgment call: the plan says "verification evidence" without enumerating
// kinds; this set treats test, CI, review, and manual evidence as verifying.
var verificationEvidenceKinds = map[string]bool{
	governance.EvidenceKindTest:   true,
	governance.EvidenceKindCI:     true,
	governance.EvidenceKindReview: true,
	governance.EvidenceKindManual: true,
}

// ValidateMarkDone requires at least one verification evidence record before a
// change can be marked done.
func ValidateMarkDone(evidence []*governance.Evidence) error {
	for _, e := range evidence {
		if e != nil && verificationEvidenceKinds[e.Kind] {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: attach test, CI, review, or manual evidence before marking done",
		ErrNoVerificationEvidence,
	)
}

// ValidateCloseCustomerFacing requires a customer- or support-targeted
// communication artifact before closing customer-facing work. Non
// customer-facing work passes with no artifact requirement.
func ValidateCloseCustomerFacing(comms []*governance.CommunicationArtifact, customerFacing bool) error {
	if !customerFacing {
		return nil
	}
	for _, c := range comms {
		if c == nil {
			continue
		}
		if c.Audience == governance.AudienceCustomer || c.Audience == governance.AudienceSupport {
			return nil
		}
	}
	return fmt.Errorf(
		"%w: generate a customer or support communication artifact before closing",
		ErrMissingCustomerComms,
	)
}

// ValidateScopeAddInvalidatesApproval reports whether adding scope to a release
// must invalidate its approval. It returns true when the release is currently
// approved: an approved release whose scope grows must be re-approved, because
// the approval covered the prior item set only. The caller is expected to act
// on a true result by resetting the release toward re-approval. It returns
// false for any non-approved release (nothing to invalidate).
func ValidateScopeAddInvalidatesApproval(rel *governance.GovernanceRelease) (mustReapprove bool) {
	return rel.Status == governance.ReleaseStatusApproved
}

// ValidateSeparationOfDuties enforces, for medium/high/critical risk work, that
// no single actor performed every gated role on a change. A violation occurs
// when the same actor appears as the proposer AND an approver AND the one who
// marked the work done.
//
// Judgment call: the decision-event model has no explicit "implemented" event,
// so "implement/mark done" is represented by the marked_done decision. The rule
// therefore checks the intersection of proposers, approvers, and done-markers.
// Low-risk work is exempt and always passes. The ch argument scopes the call
// (history is expected to already be the change's events).
func ValidateSeparationOfDuties(ch *governance.ChangeRecord, history []*governance.DecisionEvent, risk string) error {
	if !isGatedRisk(risk) {
		return nil
	}
	proposers := map[string]bool{}
	approvers := map[string]bool{}
	doneMarkers := map[string]bool{}
	for _, ev := range history {
		if ev == nil {
			continue
		}
		switch ev.Decision {
		case governance.DecisionProposed:
			proposers[ev.Actor] = true
		case governance.DecisionApproved:
			approvers[ev.Actor] = true
		case governance.DecisionMarkedDone:
			doneMarkers[ev.Actor] = true
		}
	}
	for actor := range proposers {
		if approvers[actor] && doneMarkers[actor] {
			return fmt.Errorf(
				"%w: actor %q proposed, approved, and marked done %s (%s risk requires distinct actors)",
				ErrSeparationOfDuties, actor, changeID(ch), risk,
			)
		}
	}
	return nil
}

// isGatedRisk reports whether a risk tier requires separation of duties.
func isGatedRisk(risk string) bool {
	switch risk {
	case governance.RiskMedium, governance.RiskHigh, governance.RiskCritical:
		return true
	default:
		return false
	}
}

// releaseID renders a release identifier for error messages, tolerating nil.
func releaseID(rel *governance.GovernanceRelease) string {
	if rel == nil || rel.ID == "" {
		return "release"
	}
	return "release " + rel.ID
}

// changeID renders a change identifier for error messages, tolerating nil.
func changeID(ch *governance.ChangeRecord) string {
	if ch == nil || ch.ID == "" {
		return "change"
	}
	return "change " + ch.ID
}
