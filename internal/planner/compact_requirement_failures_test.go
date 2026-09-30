package planner

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCompactRequirementFailureBoundaries(t *testing.T) {
	ctx := context.Background()
	p := New(&mockProvider{err: context.Canceled})
	if _, err := p.GeneratePlan(ctx, "intent", nil); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := p.GenerateCompactPlan(ctx, "intent"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	valid := &ProjectPlan{Stories: []Story{{ID: "S", Title: "Story"}}}
	rejected := &PlanReview{Assessment: "repair"}
	if _, err := p.RefinePlan(ctx, "intent", valid, rejected); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, plan := range []*ProjectPlan{nil, {Stories: []Story{{Title: "missing ID"}}}, {Stories: []Story{{ID: "missing title"}}}} {
		if err := plan.Validate(); err == nil {
			t.Fatal("invalid plan accepted")
		}
		if _, err := p.ReviewPlan(ctx, "intent", plan); err == nil {
			t.Fatal("invalid plan reviewed")
		}
		if _, err := p.RefinePlan(ctx, "intent", plan, rejected); err == nil {
			t.Fatal("invalid plan refined")
		}
	}
	if _, err := p.RefinePlan(ctx, "intent", valid, &PlanReview{KeyIssues: json.RawMessage(`invalid`)}); err == nil {
		t.Fatal("malformed findings accepted")
	}
	p = New(&mockProvider{response: `{"stories":[{"id":"S","requirement_id":42}]}`})
	if _, err := p.GenerateCompactPlan(ctx, "intent"); err == nil || !strings.Contains(err.Error(), "requirement_id") {
		t.Fatal("scalar diagnostic lost", err)
	}
	if _, err := p.RefinePlan(ctx, "intent", valid, rejected); err == nil || !strings.Contains(err.Error(), "requirement_id") {
		t.Fatal("refinement decode failure lost", err)
	}
	carryGoals(nil, valid)
	carryGoals(valid, nil)
}
