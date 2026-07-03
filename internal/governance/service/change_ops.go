package service

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/connectors/github"
	"github.com/openexec/openexec/internal/governance/policy"
	"github.com/openexec/openexec/internal/governance/validation"
)

// securityAuthorityID is the seeded authority whose recorded review satisfies a
// required security review. First-slice: a single named security reviewer; a
// richer model would tag authorities as security-capable.
const securityAuthorityID = "security_ai"

// reviewDecisions are the decision kinds that count as "a review happened".
var reviewDecisions = map[string]bool{
	governance.DecisionReviewed:            true,
	governance.DecisionRecommendedApproval: true,
	governance.DecisionChangesRequested:    true,
}

// ensureRequiredReviews refuses approval when the policy tier for the change's
// risk requires an AI or security review that is not present in decision
// history. Reviews are attributed to the reviewing authority's ID by ReviewPlan.
func (s *Service) ensureRequiredReviews(ctx context.Context, ch *governance.ChangeRecord) error {
	required := s.evaluator.RequiredReviews(ch)
	if len(required) == 0 {
		return nil
	}
	history, err := s.store.ListDecisionEvents(ctx, ch.ID)
	if err != nil {
		return fmt.Errorf("list decision history for change %q: %w", ch.ID, err)
	}
	var haveAIReview, haveSecurityReview bool
	for _, ev := range history {
		if ev == nil || !reviewDecisions[ev.Decision] {
			continue
		}
		if ev.ActorType == governance.ActorTypeAI {
			haveAIReview = true
		}
		if ev.Actor == securityAuthorityID {
			haveSecurityReview = true
		}
	}
	for _, r := range required {
		switch r {
		case policy.ReviewAIReview:
			if !haveAIReview {
				return fmt.Errorf("%w: change %s requires an AI review before approval (run review-plan)", validation.ErrReviewMissing, ch.ID)
			}
		case policy.ReviewSecurityReview:
			if !haveSecurityReview {
				return fmt.Errorf("%w: change %s requires a security review (review-plan --reviewer security_ai) before approval", validation.ErrReviewMissing, ch.ID)
			}
		}
	}
	return nil
}

// ImportGitHubIssue creates or refreshes a change record from a GitHub issue.
// It requires a configured Runner (the gh CLI). Import is idempotent: an
// existing record for the same (project, issue) is updated, not duplicated.
func (s *Service) ImportGitHubIssue(ctx context.Context, projectID, repo string, number int) (*governance.ChangeRecord, error) {
	if s.runner == nil {
		return nil, errMissingRunner
	}
	ch, err := github.ImportIssue(ctx, s.runner, s.store, projectID, repo, number)
	if err != nil {
		return nil, fmt.Errorf("import github issue %d from %s: %w", number, repo, err)
	}
	return ch, nil
}

// Triage runs the planner AI over a change and applies the resulting plan. It
// requires a configured Completer. The heavy lifting (prompt, parse, version
// bump, approval invalidation, plan_ready status, proposed DecisionEvent) lives
// in ai.Triage; this method loads the change and delegates so the audit trail is
// recorded exactly once, in the shared path.
func (s *Service) Triage(ctx context.Context, changeID, repoContext, actor string) (*ai.PlannerOutput, error) {
	if s.completer == nil {
		return nil, errMissingCompleter
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return nil, err
	}
	// Gate the plan write on a legal source status: triaging a change that is in
	// flight (implementing/pr_open), approved, or terminal (done/rejected/
	// deferred) would rewrite scope behind an executor or resurrect closed work.
	if err := validation.ValidateTriageable(ch); err != nil {
		return nil, err
	}
	out, err := ai.Triage(ctx, s.completer, s.store, ch, repoContext, actor)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ReviewPlan runs the reviewer AI over a change's current plan and records its
// assessment as a DecisionEvent — without ever granting approval. It requires a
// configured Completer. The authority gates whether a recommendation can be
// emitted (see ai.ReviewPlan); a reviewer that cannot recommend is downgraded to
// a plain review note.
func (s *Service) ReviewPlan(ctx context.Context, changeID, authorityID, actor string) (*ai.ReviewerOutput, error) {
	if s.completer == nil {
		return nil, errMissingCompleter
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return nil, err
	}
	authority, err := s.getAuthority(ctx, authorityID)
	if err != nil {
		return nil, err
	}
	_ = actor // attribution is by review authority, not a free-form actor
	// Record the review against the authority ID so RequiredReviews can later
	// verify that the specific required review (e.g. security) actually occurred.
	out, err := ai.ReviewPlan(ctx, s.completer, s.store, ch, authority, authority.ID)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ApproveChange approves a change's plan for AI execution. The full gate runs in
// order:
//  1. structural: acceptance criteria + verification plan present (ValidateApproveChange);
//  2. authority/policy: the authority may approve this risk tier (evaluator.CanApprove);
//  3. transition: the change may legally move to approved_for_ai.
//
// On success ApprovedVersion is pinned to the current ProposalVersion (so any
// later plan revision makes the approval stale), the status becomes
// approved_for_ai, and an approved DecisionEvent is recorded against the
// approving authority.
func (s *Service) ApproveChange(ctx context.Context, changeID, authorityID string) error {
	if !s.operatorSession {
		return errNotOperator
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	authority, err := s.getAuthority(ctx, authorityID)
	if err != nil {
		return err
	}

	if err := validation.ValidateApproveChange(ch); err != nil {
		return err
	}
	if ok, reason := s.evaluator.CanApprove(authority, ch); !ok {
		return fmt.Errorf("approval refused: %s", reason)
	}
	if err := s.ensureRequiredReviews(ctx, ch); err != nil {
		return err
	}
	if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusApprovedForAI); err != nil {
		return err
	}

	ch.ApprovedVersion = ch.ProposalVersion
	ch.Status = governance.ChangeStatusApprovedForAI
	if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
		return fmt.Errorf("approve change %q: %w", changeID, err)
	}

	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authority.ID,
		ActorType:       authority.Type,
		Decision:        governance.DecisionApproved,
		Comment:         fmt.Sprintf("Plan v%d approved by %s", ch.ProposalVersion, authority.Name),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return fmt.Errorf("record approval for change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}
