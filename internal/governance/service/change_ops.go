package service

import (
	"context"
	"fmt"
	"strings"

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
	if err := s.guardHumanAuthority(authority); err != nil {
		return nil, err
	}
	_ = actor // attribution is by review authority, not a free-form actor
	// Record the review against the authority ID so RequiredReviews can later
	// verify that the specific required review (e.g. security) actually occurred.
	out, err := ai.ReviewPlan(ctx, s.completer, s.store, ch, authority, authority.ID)
	if err != nil {
		return nil, err
	}
	// When the reviewer requests changes on a GitHub-sourced change, post the
	// specific concerns on the issue so the human answers where the work lives
	// (the clarification loop). Best-effort: the review is already recorded in
	// the audit trail; a gh failure must never undo it.
	if out != nil && out.Decision == ai.ReviewerRequestChanges {
		s.postGitHubClarification(ctx, changeID, out)
	}
	return out, nil
}

// postGitHubClarification posts the reviewer's blocking concerns as a comment on
// the source GitHub issue and applies the changes-requested label. Best-effort
// and GitHub-only: a non-github change or a gh failure is silently skipped — the
// concerns already live in the decision history regardless.
func (s *Service) postGitHubClarification(ctx context.Context, changeID string, out *ai.ReviewerOutput) {
	ch, err := s.getChange(ctx, changeID)
	if err != nil || ch == nil || s.runner == nil || ch.SourceType != governance.SourceGitHubIssue {
		return
	}
	repo, number, ok := repoFromIssueURL(ch.SourceURL)
	if !ok {
		return
	}
	_ = github.SyncLabels(ctx, s.runner, repo, number, ch.Status)
	_ = github.PostComment(ctx, s.runner, repo, number, renderClarificationComment(ch, out))
}

// renderClarificationComment turns a reviewer's blocking output into a specific,
// answerable clarification comment for the issue.
func renderClarificationComment(ch *governance.ChangeRecord, out *ai.ReviewerOutput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**OpenExec governance** — change `%s` needs your input before it can proceed (status: **%s**).\n\n", ch.ID, ch.Status)
	b.WriteString("The plan review flagged the following for a human to resolve:\n\n")
	for _, c := range out.Concerns {
		fmt.Fprintf(&b, "- %s\n", c)
	}
	if len(out.MissingAcceptanceCriteria) > 0 {
		b.WriteString("\n**Missing acceptance criteria:**\n")
		for _, m := range out.MissingAcceptanceCriteria {
			fmt.Fprintf(&b, "- %s\n", m)
		}
	}
	if len(out.MissingTests) > 0 {
		b.WriteString("\n**Missing tests:**\n")
		for _, m := range out.MissingTests {
			fmt.Fprintf(&b, "- %s\n", m)
		}
	}
	if out.RecommendedPolicy != "" {
		fmt.Fprintf(&b, "\n**Recommended policy:** %s\n", out.RecommendedPolicy)
	}
	b.WriteString("\n_Reply here with the decisions, then re-run the review — or comment_ `/openexec revise` _once the plan is updated._")
	return b.String()
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
	// The lightweight lane lets a human operator approve a trivial low/medium
	// change without the AI review its tier requires — the operator is the
	// reviewer. Every other change still needs its required reviews on record.
	if !s.lightApprovalWaivesReview(ch) {
		if err := s.ensureRequiredReviews(ctx, ch); err != nil {
			return err
		}
	}
	// A critical-tier change needs an explicit, current human risk-acceptance on
	// record before it can be approved.
	if err := s.ensureRiskAccepted(ctx, ch); err != nil {
		return err
	}
	if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusApprovedForAI); err != nil {
		return err
	}

	ch.ApprovedVersion = ch.ProposalVersion
	ch.Status = governance.ChangeStatusApprovedForAI

	approvalComment := fmt.Sprintf("Plan v%d approved by %s%s", ch.ProposalVersion, authority.Name, s.operatorSuffix())
	if s.lightApprovalWaivesReview(ch) {
		approvalComment += " (lightweight lane: operator approval, AI review waived)"
	}
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authority.ID,
		ActorType:       authority.Type,
		Decision:        governance.DecisionApproved,
		Comment:         approvalComment,
	}
	if err := s.store.TransitionChange(ctx, ch, ev); err != nil {
		return fmt.Errorf("approve change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// RejectChange rejects a change's plan (terminal). It requires an authority
// holding the request_changes permission (reviewers/PMs), a legal transition to
// rejected, and records a rejected DecisionEvent with the reason.
func (s *Service) RejectChange(ctx context.Context, changeID, authorityID, reason string) error {
	return s.terminateChange(ctx, changeID, authorityID, reason,
		governance.ChangeStatusRejected, governance.DecisionRejected, "rejected")
}

// DeferChange defers a change's plan (terminal for this release). Same authority
// requirement as RejectChange.
func (s *Service) DeferChange(ctx context.Context, changeID, authorityID, reason string) error {
	return s.terminateChange(ctx, changeID, authorityID, reason,
		governance.ChangeStatusDeferred, governance.DecisionDeferred, "deferred")
}

// terminateChange is the shared body of RejectChange/DeferChange: authorize,
// validate the transition, write status, record the decision, mirror to GitHub.
func (s *Service) terminateChange(ctx context.Context, changeID, authorityID, reason, targetStatus, decision, verb string) error {
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	authority, err := s.getAuthority(ctx, authorityID)
	if err != nil {
		return err
	}
	if err := s.guardHumanAuthority(authority); err != nil {
		return err
	}
	if !authorityHasPerm(authority, governance.PermRequestChanges) {
		return fmt.Errorf("%s refused: authority %q lacks the request_changes permission", verb, authority.Name)
	}
	if err := validation.ValidateChangeTransition(ch.Status, targetStatus); err != nil {
		return err
	}
	ch.Status = targetStatus
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authority.ID,
		ActorType:       authority.Type,
		Decision:        decision,
		Comment:         fmt.Sprintf("%s by %s%s: %s", verb, authority.Name, s.operatorSuffix(), reason),
	}
	if err := s.store.TransitionChange(ctx, ch, ev); err != nil {
		return fmt.Errorf("%s change %q: %w", verb, changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// authorityHasPerm reports whether the authority holds the given permission.
func authorityHasPerm(a *governance.ReviewAuthority, perm string) bool {
	for _, p := range a.Permissions {
		if p == perm {
			return true
		}
	}
	return false
}

// RequestRevision moves a plan_ready change back to changes_requested with a
// human's note. Gated on the request_changes permission. Unlike ReviewPlan it
// needs no AI completer — the human's comment IS the revision request.
func (s *Service) RequestRevision(ctx context.Context, changeID, authorityID, comment string) error {
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	authority, err := s.getAuthority(ctx, authorityID)
	if err != nil {
		return err
	}
	if err := s.guardHumanAuthority(authority); err != nil {
		return err
	}
	if !authorityHasPerm(authority, governance.PermRequestChanges) {
		return fmt.Errorf("revise refused: authority %q lacks the request_changes permission", authority.Name)
	}
	if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusChangesRequested); err != nil {
		return err
	}
	ch.Status = governance.ChangeStatusChangesRequested
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authority.ID,
		ActorType:       authority.Type,
		Decision:        governance.DecisionChangesRequested,
		Comment:         fmt.Sprintf("Revision requested by %s%s: %s", authority.Name, s.operatorSuffix(), comment),
	}
	if err := s.store.TransitionChange(ctx, ch, ev); err != nil {
		return fmt.Errorf("request revision on change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}
