package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// PlanReview is the result already specified by StoryReviewPrompt. It is
// evidence about a plan, not owner approval or authority to execute effects.
type PlanReview struct {
	Approved        bool            `json:"approved"`
	Assessment      string          `json:"assessment"`
	KeyIssues       json.RawMessage `json:"key_issues,omitempty"`
	RefactoringPlan json.RawMessage `json:"refactoring_plan,omitempty"`
}

// ReviewPlan uses the existing independent architecture-review prompt. The
// caller supplies a fresh reviewer; no executor transcript is carried over.
func (p *Planner) ReviewPlan(ctx context.Context, intent string, plan *ProjectPlan) (*PlanReview, error) {
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	response, err := p.provider.Complete(ctx, fmt.Sprintf(StoryReviewPrompt, intent, data))
	if err != nil {
		return nil, fmt.Errorf("plan review: %w", err)
	}
	// Missing approval is not a rejection that a fix loop can blindly retry;
	// it is malformed evidence. Never interpret prose or an empty object as pass.
	var wire struct {
		Approved *bool `json:"approved"`
		PlanReview
	}
	if err := json.Unmarshal([]byte(response), &wire); err != nil {
		return nil, fmt.Errorf("invalid plan review: %w", err)
	}
	if wire.Approved == nil || strings.TrimSpace(wire.Assessment) == "" {
		return nil, fmt.Errorf("plan review requires explicit approved and assessment")
	}
	wire.PlanReview.Approved = *wire.Approved
	if issues := LintPlanVerification(plan); len(issues) != 0 {
		wire.PlanReview.Approved = false
		wire.Assessment += fmt.Sprintf("; verification lint refused: %v", issues)
	}
	if issues := LintHumanBoundaries(plan); len(issues) != 0 {
		wire.PlanReview.Approved = false
		wire.Assessment += fmt.Sprintf("; human boundary lint refused: %v", issues)
	}
	return &wire.PlanReview, nil
}

// RefinePlan reuses the existing repair prompt. The changed plan still needs
// review before import; a successful model response is not approval.
func (p *Planner) RefinePlan(ctx context.Context, intent string, plan *ProjectPlan, review *PlanReview) (*ProjectPlan, error) {
	if review == nil || review.Approved {
		return nil, fmt.Errorf("plan refinement requires rejected review evidence")
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	findings, err := json.Marshal(review)
	if err != nil {
		return nil, err
	}
	response, err := p.provider.Complete(ctx, fmt.Sprintf(StoryFixPrompt, intent, data, findings))
	if err != nil {
		return nil, err
	}
	refined, err := p.parseResponse(response)
	if err != nil {
		return nil, err
	}
	if err := preserveHumanBoundaries(plan, refined); err != nil {
		return nil, err
	}
	carryGoals(plan, refined)
	return refined, nil
}

// carryGoals keeps the accepted goals with the refined stories. The fix
// prompt asks the model for stories only, so a refined plan normally has no
// goals of its own while its stories still cite the original goal ids;
// importing that plan would reference goals that were never persisted.
// Refinement revises stories and tasks, never the goals they serve.
func carryGoals(original, refined *ProjectPlan) {
	if original == nil || refined == nil {
		return
	}
	if refined.SchemaVersion == "" {
		refined.SchemaVersion = original.SchemaVersion
	}
	present := make(map[string]bool, len(refined.Goals))
	for _, g := range refined.Goals {
		present[g.ID] = true
	}
	if len(refined.Goals) == 0 {
		refined.Goals = append([]Goal(nil), original.Goals...)
		for _, g := range original.Goals {
			present[g.ID] = true
		}
	}
	// The fix prompt's story shape has no goal_id, so a refined story can come
	// back without the goal its original served; it still serves it.
	served := make(map[string]string, len(original.Stories))
	for _, s := range original.Stories {
		served[s.ID] = s.GoalID
	}
	for i := range refined.Stories {
		if refined.Stories[i].GoalID == "" {
			refined.Stories[i].GoalID = served[refined.Stories[i].ID]
		}
	}
	for _, s := range refined.Stories {
		if s.GoalID == "" || present[s.GoalID] {
			continue
		}
		for _, g := range original.Goals {
			if g.ID == s.GoalID {
				refined.Goals = append(refined.Goals, g)
				present[g.ID] = true
				break
			}
		}
	}
}
