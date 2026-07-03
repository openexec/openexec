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

// decisionMerged is the service-local audit token for a merge (the governance
// Decision* vocabulary has no merge verb).
const decisionMerged = "merged"

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

	if err := github.MergePR(ctx, s.runner, repo, number, method); err != nil {
		return err
	}

	// A merged change is done: move it there if the transition is legal.
	if validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusDone) == nil {
		ch.Status = governance.ChangeStatusDone
		_ = s.store.UpdateChangeRecord(ctx, ch)
	}
	actorType := governance.ActorTypeSystem
	if a, aErr := s.getAuthority(ctx, authorityID); aErr == nil {
		actorType = a.Type
	}
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authorityID,
		ActorType:       actorType,
		Decision:        decisionMerged,
		Comment:         fmt.Sprintf("Merged PR %s via %s", ch.PRURL, mergeAuthLabel(s.operatorSession)),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
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
	// Path 2: auto-merge — ONLY if policy opts this tier in AND the operability
	// review clears it AND verification evidence exists. All three must hold; any
	// one missing fails closed. Operability is the hard SRE gate: a change that
	// can't be cleanly rolled back, needs a destructive DB migration, or is high
	// deploy risk can NEVER auto-merge even when its risk tier is opted in — a
	// human operator must own that deploy.
	if s.evaluator.CanAutoMerge(ch) {
		op, _ := s.ChangeOperability(ctx, ch.ID)
		if !op.AutoMergeSafe() {
			return false, "auto-merge blocked by the operability review (rollback safety / DB migration / deploy risk not cleared); a human operator must merge this deploy"
		}
		evidence, _ := s.store.ListEvidence(ctx, ch.ID)
		if validation.ValidateMarkDone(evidence) == nil {
			return true, ""
		}
		return false, fmt.Sprintf("auto-merge is policy-allowed and operability-clear for %s-risk work but no verification evidence is recorded", riskOrUnknown(ch.Risk))
	}
	return false, fmt.Sprintf(
		"auto-merge is not allowed for %s-risk work; merge requires a human operator session (OPENEXEC_OPERATOR_SESSION=1) with an approve authority",
		riskOrUnknown(ch.Risk),
	)
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
