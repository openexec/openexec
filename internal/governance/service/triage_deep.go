package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/validation"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
)

// DeepTriageResult is what TriageDeep returns: the generated plan and the ids of
// the release stories now linked to the change.
type DeepTriageResult struct {
	Plan     *planner.ProjectPlan
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

	p := planner.New(s.completer)
	plan, err := p.GeneratePlan(ctx, composeIntent(ch, repoContext), nil)
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
	// The planner does not classify risk; default conservatively to medium so a
	// deep-triaged change requires human approval unless an operator lowers it.
	if strings.TrimSpace(ch.Risk) == "" {
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
	s.mirrorGitHubLabel(ctx, ch)

	return &DeepTriageResult{Plan: plan, StoryIDs: storyIDs}, nil
}

// ChangeStories returns the release stories a change owns (via deep triage),
// for review/display. Requires a PlanStore.
func (s *Service) ChangeStories(ctx context.Context, changeID string) ([]*release.Story, error) {
	if s.planStore == nil {
		return nil, errMissingPlanStore
	}
	ids, err := s.store.ListChangeStories(ctx, changeID)
	if err != nil {
		return nil, err
	}
	out := make([]*release.Story, 0, len(ids))
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
func (s *Service) importPlanForChange(ctx context.Context, ch *governance.ChangeRecord, plan *planner.ProjectPlan) ([]string, error) {
	planner.RemapPlanIDs(plan, planner.ExistingLookup{
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
		if err := s.planStore.CreateGoal(&release.Goal{
			ID: g.ID, Title: g.Title, Description: g.Description,
			SuccessCriteria: g.SuccessCriteria, VerificationMethod: g.VerificationMethod,
		}); err != nil {
			return nil, fmt.Errorf("import goal %s: %w", g.ID, err)
		}
	}

	storyIDs := make([]string, 0, len(plan.Stories))
	for i, st := range plan.Stories {
		taskIDs := make([]string, len(st.Tasks))
		for j, t := range st.Tasks {
			taskIDs[j] = t.ID
		}
		goalID := st.GoalID
		if goalID != "" && !goalExists[goalID] && s.planStore.GetGoal(goalID) == nil {
			goalID = ""
		}
		if s.planStore.GetStory(st.ID) == nil {
			if err := s.planStore.CreateStory(&release.Story{
				ID: st.ID, GoalID: goalID, Title: st.Title, Description: st.Description,
				AcceptanceCriteria: st.AcceptanceCriteria, VerificationScript: st.VerificationScript,
				Tasks: taskIDs, DependsOn: st.DependsOn,
				StoryType: release.StoryTypeFeature, Priority: i,
				Status: release.StoryStatusPending, CreatedAt: now,
			}); err != nil {
				return nil, fmt.Errorf("import story %s: %w", st.ID, err)
			}
		}
		for j, t := range st.Tasks {
			if s.planStore.GetTask(t.ID) != nil {
				continue
			}
			task := &release.Task{
				ID: t.ID, Title: t.Title, Description: t.Description,
				VerificationScript: t.VerificationScript, StoryID: st.ID,
				DependsOn: t.DependsOn, Priority: j, MaxAttempts: 3,
				Status: release.TaskStatusPending, CreatedAt: now,
			}
			if t.Mode == planner.TaskModeHITL {
				task.Metadata = map[string]interface{}{"mode": release.TaskModeHITL}
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

func planSummary(plan *planner.ProjectPlan, ch *governance.ChangeRecord) string {
	if len(plan.Goals) > 0 && plan.Goals[0].Description != "" {
		return plan.Goals[0].Description
	}
	if ch.Summary != "" {
		return ch.Summary
	}
	return ch.Title
}

func aggregateAcceptance(plan *planner.ProjectPlan) string {
	var lines []string
	for _, st := range plan.Stories {
		for _, ac := range st.AcceptanceCriteria {
			lines = append(lines, fmt.Sprintf("[%s] %s", st.ID, ac))
		}
	}
	return strings.Join(lines, "\n")
}

func aggregateVerification(plan *planner.ProjectPlan) string {
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

func titleOf(g *release.Goal) string {
	if g == nil {
		return ""
	}
	return g.Title
}

func storyTitleOf(s *release.Story) string {
	if s == nil {
		return ""
	}
	return s.Title
}
