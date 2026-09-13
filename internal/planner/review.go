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
	return p.parseResponse(response)
}
