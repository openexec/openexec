package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/validation"
	"github.com/openexec/openexec/pkg/runtime"
)

// DeepTriageResult is what TriageDeep returns: the generated plan and the ids of
// the release stories now linked to the change.
type DeepTriageResult struct {
	Plan     *runtime.ProjectPlan
	StoryIDs []string
}

// TriageDeep is the intent-extraction bridge: it runs the EXISTING planner over
// a change record's intent to produce a real decomposition (goals -> stories ->
// vertical-slice tasks), persists those into the release store, links the
// stories to the change, and aggregates their acceptance/verification onto the
// change so the governance approval gates operate on the real plan.
//
// This is the "internally it needs to do the stories and tasks" step: unlike the
// flat Triage (a prose plan), a human then reviews and approves an actual task
// breakdown — the same breakdown a Jira connector would later mirror as
// subtasks. It requires both a Completer (drives the planner) and a PlanStore
// (persists the stories/tasks). The planner's LLMProvider interface is
// identical to ai.Completer, so the injected completer drives it directly.
func (s *Service) TriageDeep(ctx context.Context, changeID, repoContext, actor string) (*DeepTriageResult, error) {
	if s.completer == nil {
		return nil, errMissingCompleter
	}
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

	p := runtime.NewPlanner(s.completer)
	plan, err := p.GeneratePlan(ctx, composeIntent(ch, repoContext))
	if err != nil {
		return nil, fmt.Errorf("deep triage planning for change %q: %w", changeID, err)
	}

	storyIDs, err := s.importPlanForChange(ctx, ch, plan)
	if err != nil {
		return nil, err
	}

	// Aggregate the decomposition onto the change so the existing approval gates
	// (which read AcceptanceCriteria / VerificationPlan) work, and store the full
	// plan JSON for traceability.
	ch.Summary = planSummary(plan, ch)
	ch.AcceptanceCriteria = aggregateAcceptance(plan)
	ch.VerificationPlan = aggregateVerification(plan)
	if raw, mErr := json.Marshal(plan); mErr == nil {
		ch.Plan = string(raw)
	}
	// Classify kind + risk from the intent (the planner does not). This is the
	// AI's assessment, subject to human review before approval. Best-effort: on
	// classifier failure, fall back to a conservative medium so a deep-triaged
	// change still requires human approval. ClassifyIntent already clamps an
	// unrecognized risk to medium, so it can never downgrade to auto-approvable.
	if cls, cErr := ai.ClassifyIntent(ctx, s.completer, ch.Title, intentBody(ch)); cErr == nil {
		if cls.Kind != "" {
			ch.Kind = cls.Kind
		}
		ch.Risk = cls.Risk
	} else if strings.TrimSpace(ch.Risk) == "" {
		ch.Risk = governance.RiskMedium
	}
	ch.ProposalVersion++
	ch.ApprovedVersion = 0 // a new proposal supersedes any prior approval
	ch.Status = governance.ChangeStatusPlanReady
	if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
		return nil, fmt.Errorf("persist deep-triaged change %q: %w", changeID, err)
	}

	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           actor,
		ActorType:       governance.ActorTypeAI,
		Decision:        governance.DecisionProposed,
		Comment:         fmt.Sprintf("Deep triage: decomposed into %d stor(ies)", len(storyIDs)),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return nil, fmt.Errorf("record deep-triage proposal for change %q: %w", changeID, err)
	}

	// File-level impact analysis (best-effort, review-only): when real repo
	// excerpts were supplied, ask the model which exact files the change touches
	// so a human reviews "affects this and that" before approval. A failure here
	// never fails triage — the decomposition already succeeded.
	if strings.TrimSpace(repoContext) != "" {
		if imp, iErr := ai.AnalyzeImpact(ctx, s.completer, composeIntent(ch, ""), repoContext); iErr == nil {
			if raw, mErr := json.Marshal(imp); mErr == nil {
				_ = s.store.SetChangeImpact(ctx, ch.ID, string(raw))
			}
		}
		// Operability / production-readiness review (rollback, DB migration,
		// deploy risk) — feeds both human review and the merge gate.
		if op, oErr := ai.AnalyzeOperability(ctx, s.completer, composeIntent(ch, ""), repoContext); oErr == nil {
			if raw, mErr := json.Marshal(op); mErr == nil {
				_ = s.store.SetChangeOperability(ctx, ch.ID, string(raw))
			}
		}
	}

	s.mirrorGitHubLabel(ctx, ch)
	return &DeepTriageResult{Plan: plan, StoryIDs: storyIDs}, nil
}

// ChangeImpact returns the stored file-level impact analysis for a change (nil
// if none was produced, e.g. triage ran without repo context).
func (s *Service) ChangeImpact(ctx context.Context, changeID string) (*ai.ImpactReport, error) {
	raw, err := s.store.GetChangeImpact(ctx, changeID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return nil, nil
	}
	rep := &ai.ImpactReport{}
	if err := json.Unmarshal([]byte(raw), rep); err != nil {
		return nil, fmt.Errorf("parse impact for change %q: %w", changeID, err)
	}
	return rep, nil
}

// ChangeOperability returns the stored operability / production-readiness report
// for a change (nil if none was produced).
func (s *Service) ChangeOperability(ctx context.Context, changeID string) (*ai.OperabilityReport, error) {
	raw, err := s.store.GetChangeOperability(ctx, changeID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" || raw == "{}" {
		return nil, nil
	}
	rep := &ai.OperabilityReport{}
	if err := json.Unmarshal([]byte(raw), rep); err != nil {
		return nil, fmt.Errorf("parse operability for change %q: %w", changeID, err)
	}
	return rep, nil
}

// ChangeStories returns the release stories a change owns (via deep triage),
// for review/display. Requires a PlanStore.
func (s *Service) ChangeStories(ctx context.Context, changeID string) ([]*runtime.Story, error) {
	if s.planStore == nil {
		return nil, errMissingPlanStore
	}
	ids, err := s.store.ListChangeStories(ctx, changeID)
	if err != nil {
		return nil, err
	}
	out := make([]*runtime.Story, 0, len(ids))
	for _, id := range ids {
		if st := s.planStore.GetStory(id); st != nil {
			out = append(out, st)
		}
	}
	return out, nil
}

// importPlanForChange persists a generated ProjectPlan's goals/stories/tasks
// into the release store and links each story to the change. It mirrors
// pkg/manager.importPlan (including RemapPlanIDs so a plan generated from
// US-001 does not collide with an existing backlog) but targets the injected
// PlanStore and records the change<->story links.
func (s *Service) importPlanForChange(ctx context.Context, ch *governance.ChangeRecord, plan *runtime.ProjectPlan) ([]string, error) {
	// Supersede any prior decomposition this change owns. Re-triage used to LINK
	// the new stories alongside the old ones, so a re-triaged change accumulated
	// duplicate stories and ExecuteChange would build both. Delete the old
	// stories (and their tasks) from the backlog and drop the links first, so the
	// change ends up owning ONLY the new plan — and so RemapPlanIDs below no
	// longer sees the just-removed ids as collisions.
	if err := s.supersedeChangeDecomposition(ctx, ch.ID); err != nil {
		return nil, err
	}

	runtime.RemapPlanIDs(plan, runtime.ExistingLookup{
		GoalTitle:  func(id string) (string, bool) { g := s.planStore.GetGoal(id); return titleOf(g), g != nil },
		StoryTitle: func(id string) (string, bool) { st := s.planStore.GetStory(id); return storyTitleOf(st), st != nil },
		TaskExists: func(id string) bool { return s.planStore.GetTask(id) != nil },
	})

	now := time.Now().UTC()
	goalExists := map[string]bool{}
	for _, g := range plan.Goals {
		goalExists[g.ID] = true
		if s.planStore.GetGoal(g.ID) != nil {
			continue
		}
		if err := s.planStore.CreateGoal(&runtime.Goal{
			ID: g.ID, Title: g.Title, Description: g.Description,
			SuccessCriteria: g.SuccessCriteria, VerificationMethod: g.VerificationMethod,
		}); err != nil {
			return nil, fmt.Errorf("import goal %s: %w", g.ID, err)
		}
	}

	storyIDs := make([]string, 0, len(plan.Stories))
	for i, st := range plan.Stories {
		goalID := st.GoalID
		if goalID != "" && !goalExists[goalID] && s.planStore.GetGoal(goalID) == nil {
			goalID = ""
		}
		if s.planStore.GetStory(st.ID) == nil {
			// Create the story with an EMPTY task list: the store's CreateTask
			// appends each task to its parent story, so pre-populating Tasks here
			// would double every task id in the story.
			if err := s.planStore.CreateStory(&runtime.Story{
				ID: st.ID, GoalID: goalID, Title: st.Title, Description: st.Description,
				AcceptanceCriteria: st.AcceptanceCriteria, VerificationScript: st.VerificationScript,
				DependsOn: st.DependsOn,
				StoryType: runtime.StoryTypeFeature, Priority: i,
				Status: runtime.StoryStatusPending, CreatedAt: now,
			}); err != nil {
				return nil, fmt.Errorf("import story %s: %w", st.ID, err)
			}
		}
		for j, t := range st.Tasks {
			if s.planStore.GetTask(t.ID) != nil {
				continue
			}
			// GOVERNANCE HOLD (the binding that makes approval mean something):
			// every triaged task is created hitl, so the ungoverned runtime
			// scheduler / `openexec run` / backlog tools hold it (and its
			// dependents) and NEVER auto-build unapproved work. Only the
			// governance ExecuteChange path un-holds a change's tasks (hitl->afk)
			// after the change is approved and claimed. Governance is the sole
			// un-holder — this is what turns the control plane into a control
			// system.
			task := &runtime.Task{
				ID: t.ID, Title: t.Title, Description: t.Description,
				VerificationScript: t.VerificationScript, StoryID: st.ID,
				DependsOn: t.DependsOn, Priority: j, MaxAttempts: 3,
				Status: runtime.TaskStatusPending, CreatedAt: now,
				Metadata: map[string]interface{}{"mode": runtime.TaskModeHITL},
			}
			if err := s.planStore.CreateTask(task); err != nil {
				return nil, fmt.Errorf("import task %s: %w", t.ID, err)
			}
		}
		if err := s.store.LinkChangeStory(ctx, ch.ID, st.ID); err != nil {
			return nil, err
		}
		storyIDs = append(storyIDs, st.ID)
	}
	return storyIDs, nil
}

// supersedeChangeDecomposition removes a change's currently-linked stories (and
// their tasks) from the backlog and clears the links, so a re-triage replaces
// the prior decomposition instead of accumulating it. A no-op on first triage.
func (s *Service) supersedeChangeDecomposition(ctx context.Context, changeID string) error {
	old, err := s.store.ListChangeStories(ctx, changeID)
	if err != nil {
		return fmt.Errorf("list prior stories for change %q: %w", changeID, err)
	}
	if len(old) == 0 {
		return nil
	}
	for _, sid := range old {
		if err := s.planStore.DeleteStory(sid); err != nil {
			return fmt.Errorf("supersede prior story %q of change %q: %w", sid, changeID, err)
		}
	}
	return s.store.ClearChangeStories(ctx, changeID)
}

// composeIntent builds the intent markdown fed to the planner from the change's
// title, raw text, and optional repo context.
func composeIntent(ch *governance.ChangeRecord, repoContext string) string {
	var b strings.Builder
	if ch.Title != "" {
		fmt.Fprintf(&b, "# %s\n\n", ch.Title)
	}
	if ch.RawText != "" {
		b.WriteString(ch.RawText)
		b.WriteString("\n\n")
	} else if ch.Summary != "" {
		b.WriteString(ch.Summary)
		b.WriteString("\n\n")
	}
	if repoContext != "" {
		b.WriteString("## Repository context\n\n")
		b.WriteString(repoContext)
		b.WriteString("\n")
	}
	return b.String()
}

// intentBody returns the free text used to classify a change (raw source text,
// else the summary).
func intentBody(ch *governance.ChangeRecord) string {
	if strings.TrimSpace(ch.RawText) != "" {
		return ch.RawText
	}
	return ch.Summary
}

func planSummary(plan *runtime.ProjectPlan, ch *governance.ChangeRecord) string {
	if len(plan.Goals) > 0 && plan.Goals[0].Description != "" {
		return plan.Goals[0].Description
	}
	if ch.Summary != "" {
		return ch.Summary
	}
	return ch.Title
}

func aggregateAcceptance(plan *runtime.ProjectPlan) string {
	var lines []string
	for _, st := range plan.Stories {
		for _, ac := range st.AcceptanceCriteria {
			lines = append(lines, fmt.Sprintf("[%s] %s", st.ID, ac))
		}
	}
	return strings.Join(lines, "\n")
}

func aggregateVerification(plan *runtime.ProjectPlan) string {
	var lines []string
	for _, st := range plan.Stories {
		if st.VerificationScript != "" {
			lines = append(lines, fmt.Sprintf("[%s] %s", st.ID, st.VerificationScript))
		}
		for _, t := range st.Tasks {
			if t.VerificationScript != "" {
				lines = append(lines, fmt.Sprintf("[%s] %s", t.ID, t.VerificationScript))
			}
		}
	}
	return strings.Join(lines, "\n")
}

func titleOf(g *runtime.Goal) string {
	if g == nil {
		return ""
	}
	return g.Title
}

func storyTitleOf(s *runtime.Story) string {
	if s == nil {
		return ""
	}
	return s.Title
}
