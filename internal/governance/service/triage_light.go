package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/validation"
	"github.com/openexec/openexec/pkg/runtime"
)

// TriageLight is the lightweight lane: it prepares a TRIVIAL change for approval
// without running the planner. Instead of decomposing intent into a full
// goal/story/task tree, it builds a single story with a single task whose
// description IS the change's intent, so a one-line fix does not incur an
// 8-story plan and a round of planner-output review. The change is marked Light
// so a human operator may approve it without the AI review its risk tier would
// otherwise require (the operator is the reviewer).
//
// It refuses high/critical risk: a change the classifier deems dangerous must go
// through full triage + review, never the fast path. Requires a PlanStore (to
// persist the single task). The Completer is optional — used only to classify
// risk; without it the change defaults to low.
func (s *Service) TriageLight(ctx context.Context, changeID, actor string) (*DeepTriageResult, error) {
	if s.planStore == nil {
		return nil, errMissingPlanStore
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return nil, err
	}
	if err := validation.ValidateTriageable(ch); err != nil {
		return nil, err
	}

	// Classify risk (best-effort). The light lane is for trivial low/medium work;
	// high/critical must not skip the review gate.
	risk := governance.RiskLow
	kind := ch.Kind
	if s.completer != nil {
		if cls, cErr := ai.ClassifyIntent(ctx, s.completer, ch.Title, intentBody(ch)); cErr == nil {
			if cls.Risk != "" {
				risk = cls.Risk
			}
			if cls.Kind != "" {
				kind = cls.Kind
			}
		}
	}
	if risk == governance.RiskHigh || risk == governance.RiskCritical {
		return nil, fmt.Errorf("change %s classifies as %s risk; the lightweight lane is only for trivial low/medium changes — run full triage instead: openexec governance work triage %s --deep", changeID, risk, changeID)
	}

	// One story, one task, straight from the intent. RemapPlanIDs (inside
	// importPlanForChange) relocates US-001/T-US-001-001 to free ids if the
	// backlog already uses them, so this never collides with prior triage.
	plan := &runtime.ProjectPlan{
		Stories: []runtime.PlanStory{{
			ID:                 "US-001",
			Title:              ch.Title,
			Description:        intentBody(ch),
			AcceptanceCriteria: []string{"Implement the change exactly as specified in the issue; no scope beyond it."},
			Tasks: []runtime.PlanTask{{
				ID:          "T-US-001-001",
				Title:       ch.Title,
				Description: intentBody(ch),
			}},
		}},
	}
	storyIDs, err := s.importPlanForChange(ctx, ch, plan)
	if err != nil {
		return nil, err
	}

	ch.Kind = kind
	ch.Risk = risk
	ch.Light = true
	if ch.Summary == "" {
		ch.Summary = ch.Title
	}
	ch.AcceptanceCriteria = aggregateAcceptance(plan)
	// The light lane's verification is human review of the draft PR (the operator
	// is the reviewer). Record that as the plan so the approval gate — which
	// requires a non-empty verification plan — is satisfied honestly, rather than
	// with a synthetic pass-through script.
	ch.VerificationPlan = "Human verification: confirm the change satisfies the issue's acceptance criteria on the draft PR before merge (lightweight lane)."
	if raw, mErr := json.Marshal(plan); mErr == nil {
		ch.Plan = string(raw)
	}
	ch.ProposalVersion++
	ch.ApprovedVersion = 0
	ch.Status = governance.ChangeStatusPlanReady
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           actor,
		ActorType:       governance.ActorTypeAI,
		Decision:        governance.DecisionProposed,
		Comment:         "Lightweight lane: single-task plan (no deep decomposition); AI review waived at approval",
	}
	if err := s.store.TransitionChange(ctx, ch, ev); err != nil {
		return nil, fmt.Errorf("persist light-triaged change %q: %w", changeID, err)
	}

	s.mirrorGitHubLabel(ctx, ch)
	return &DeepTriageResult{Plan: plan, StoryIDs: storyIDs}, nil
}

// lightApprovalWaivesReview reports whether a change may be operator-approved
// without the AI review its risk tier requires: only light-lane changes, and
// never at high/critical risk. ApproveChange already requires an operator
// session, so this only relaxes the AI-review requirement, not the human gate.
func (s *Service) lightApprovalWaivesReview(ch *governance.ChangeRecord) bool {
	return ch != nil && ch.Light &&
		ch.Risk != governance.RiskHigh && ch.Risk != governance.RiskCritical
}
