package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/connectors/github"
	"github.com/openexec/openexec/internal/governance/validation"
)

// Service-local audit tokens for a merge (the governance Decision* vocabulary
// has no merge verb). merge_authorized is recorded BEFORE the external gh call
// so a crash mid-merge still leaves a trace; merged / merge_failed record the
// outcome after.
const (
	decisionMerged          = "merged"
	decisionMergeAuthorized = "merge_authorized"
	decisionMergeFailed     = "merge_failed"
)

// mergeEvent builds a merge-related decision event for a change.
func (s *Service) mergeEvent(ch *governance.ChangeRecord, actor, actorType, decision, comment string) *governance.DecisionEvent {
	return &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           actor,
		ActorType:       actorType,
		Decision:        decision,
		Comment:         comment,
	}
}

// MergeChange merges a change's pull request — THE safety gate. It never merges
// unless explicitly authorized, one of:
//   - a human operator session (OPENEXEC_OPERATOR_SESSION=1) whose authority may
//     approve this risk tier; or
//   - an explicit policy opt-in for the change's risk tier AND recorded
//     verification evidence.
//
// By default nothing auto-merges (every tier's AutoMergeAllowed is false), so a
// change can never accidentally merge into a branch wired to CI/CD and trigger a
// production deploy. The agent/MCP plane exposes no merge tool at all.
func (s *Service) MergeChange(ctx context.Context, changeID, authorityID, method string) error {
	if s.runner == nil {
		return errMissingRunner
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(ch.PRURL) == "" {
		return fmt.Errorf("change %s has no PR to merge (record its PR first with work record-pr)", changeID)
	}
	repo, number, ok := repoFromPRURL(ch.PRURL)
	if !ok {
		return fmt.Errorf("cannot derive repo/PR number from PR URL %q", ch.PRURL)
	}

	authorized, reason := s.canMerge(ctx, ch, authorityID)
	if !authorized {
		return fmt.Errorf("merge refused: %s", reason)
	}

	actorType := governance.ActorTypeSystem
	if a, aErr := s.getAuthority(ctx, authorityID); aErr == nil {
		actorType = a.Type
	}

	// Record the authorization BEFORE the external merge, so even a crash between
	// the gh call and the outcome event leaves an audit trace that a merge was
	// authorized and attempted (no silent external side effect).
	if err := s.store.CreateDecisionEvent(ctx, s.mergeEvent(ch, authorityID, actorType, decisionMergeAuthorized,
		fmt.Sprintf("Merge authorized for PR %s via %s%s", ch.PRURL, mergeAuthLabel(s.operatorSession), s.operatorSuffix()))); err != nil {
		return fmt.Errorf("record merge authorization for change %q: %w", changeID, err)
	}

	if err := github.MergePR(ctx, s.runner, repo, number, method); err != nil {
		_ = s.store.CreateDecisionEvent(ctx, s.mergeEvent(ch, authorityID, actorType, decisionMergeFailed,
			fmt.Sprintf("Merge FAILED for PR %s: %v", ch.PRURL, err)))
		return err
	}

	// Success. A merged change is done: advance status + record the merge
	// atomically so the two never diverge. If the transition is illegal, record
	// the merge event on its own.
	merged := s.mergeEvent(ch, authorityID, actorType, decisionMerged,
		fmt.Sprintf("Merged PR %s via %s%s", ch.PRURL, mergeAuthLabel(s.operatorSession), s.operatorSuffix()))
	if validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusDone) == nil {
		ch.Status = governance.ChangeStatusDone
		if err := s.store.TransitionChange(ctx, ch, merged); err != nil {
			return fmt.Errorf("record merge for change %q: %w", changeID, err)
		}
	} else if err := s.store.CreateDecisionEvent(ctx, merged); err != nil {
		return fmt.Errorf("record merge for change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// canMerge is the merge authorization gate. It fails closed: the default return
// is refusal with an actionable reason.
func (s *Service) canMerge(ctx context.Context, ch *governance.ChangeRecord, authorityID string) (bool, string) {
	// Path 1: an explicit human operator with approve authority for this tier.
	if s.operatorSession {
		auth, err := s.getAuthority(ctx, authorityID)
		if err != nil {
			return false, err.Error()
		}
		if ok, reason := s.evaluator.CanApprove(auth, ch); !ok {
			return false, fmt.Sprintf("operator session present but %s", reason)
		}
		return true, ""
	}
	// Path 2: auto-merge (no human). This is the deploy hop, so it is gated the
	// hardest — all of the following must hold, each fails closed:
	//   a. policy opts this risk tier into auto-merge (default: no tier does);
	//   b. the change is DONE — so MarkDone's authority + separation-of-duties +
	//      evidence gate already ran (closes the "merge at pr_open" bypass);
	//   c. the operability review clears it (rollback-safe, non-destructive
	//      migration, low/medium deploy risk);
	//   d. verification evidence is EXTERNALLY observed (CI/GitHub), never
	//      agent-self-reported — an agent cannot manufacture a passing CI run.
	if s.evaluator.CanAutoMerge(ch) {
		if ch.Status != governance.ChangeStatusDone {
			return false, fmt.Sprintf("auto-merge requires the change to be done (fully verified); it is %q — a human operator must merge earlier states", ch.Status)
		}
		op, _ := s.ChangeOperability(ctx, ch.ID)
		if !op.AutoMergeSafe() {
			return false, "auto-merge blocked by the operability review (rollback safety / DB migration / deploy risk not cleared); a human operator must merge this deploy"
		}
		evidence, _ := s.store.ListEvidence(ctx, ch.ID)
		if !hasTrustedVerification(evidence) {
			return false, "auto-merge requires externally-verified evidence (CI/GitHub); agent-self-reported evidence does not qualify — a human operator must merge"
		}
		return true, ""
	}
	return false, fmt.Sprintf(
		"auto-merge is not allowed for %s-risk work; merge requires a human operator session (OPENEXEC_OPERATOR_SESSION=1) with an approve authority",
		riskOrUnknown(ch.Risk),
	)
}

// trustedVerificationSources are evidence sources an agent cannot manufacture:
// they come from an external system (a CI run, a GitHub check) rather than
// self-report. Agent/cli/human sources are all reachable by a shell-capable
// agent and so do not qualify for an UNattended auto-merge.
var trustedVerificationSources = map[string]bool{
	governance.EvidenceSourceGitHub:  true,
	governance.EvidenceSourceWebhook: true,
}

// verifyingEvidenceKinds are the kinds that attest a change is correct.
var verifyingEvidenceKinds = map[string]bool{
	governance.EvidenceKindTest:   true,
	governance.EvidenceKindCI:     true,
	governance.EvidenceKindReview: true,
}

// hasTrustedVerification reports whether any evidence is a verifying kind from an
// externally-observed source — the bar for auto-merge (auto-deploy).
func hasTrustedVerification(evidence []*governance.Evidence) bool {
	for _, e := range evidence {
		if e != nil && verifyingEvidenceKinds[e.Kind] && trustedVerificationSources[e.Source] {
			return true
		}
	}
	return false
}

func mergeAuthLabel(operator bool) string {
	if operator {
		return "human operator"
	}
	return "policy auto-merge"
}

func riskOrUnknown(r string) string {
	if strings.TrimSpace(r) == "" {
		return "unknown"
	}
	return r
}

// repoFromPRURL parses "https://github.com/<owner>/<repo>/pull/<n>".
func repoFromPRURL(u string) (repo string, number int, ok bool) {
	const marker = "github.com/"
	i := strings.Index(u, marker)
	if i < 0 {
		return "", 0, false
	}
	parts := strings.Split(strings.Trim(u[i+len(marker):], "/"), "/")
	if len(parts) < 4 || parts[2] != "pull" {
		return "", 0, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", 0, false
	}
	return parts[0] + "/" + parts[1], n, true
}
