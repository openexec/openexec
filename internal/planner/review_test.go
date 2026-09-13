package planner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReviewPlanUsesExistingDiscipline(t *testing.T) {
	provider := &mockProvider{response: `{"approved":true,"assessment":"Complete vertical slice with verification"}`}
	plan := &ProjectPlan{Stories: []Story{{ID: "US-1", Title: "Preserve existing records", VerificationScript: "go test ./..."}}}
	review, err := New(provider).ReviewPlan(context.Background(), "brownfield: preserve existing records", plan)
	if err != nil || !review.Approved {
		t.Fatalf("review = %#v, %v", review, err)
	}
	for _, required := range []string{"ORIGINAL INTENT:", "brownfield: preserve existing records", "Vertical Slicing", "Preserve existing records"} {
		if !strings.Contains(provider.lastPrompt, required) {
			t.Fatalf("existing review contract lost %q", required)
		}
	}
	plan.Stories[0].VerificationScript = "go test ./... || true"
	review, err = New(provider).ReviewPlan(context.Background(), "same intent", plan)
	if err != nil || review.Approved || !strings.Contains(review.Assessment, "verification lint refused") {
		t.Fatalf("model approval bypassed deterministic lint: %#v %v", review, err)
	}
}

func TestReviewPlanRefusesMissingOrMalformedEvidence(t *testing.T) {
	plan := &ProjectPlan{Stories: []Story{{ID: "US-1", Title: "Keep records"}}}
	for _, response := range []string{`{}`, `{"assessment":"fine"}`, `{"approved":true}`, `{"approved":null,"assessment":"fine"}`, `approved`, `{"approved":"yes","assessment":"fine"}`} {
		t.Run(response, func(t *testing.T) {
			if _, err := New(&mockProvider{response: response}).ReviewPlan(context.Background(), "intent", plan); err == nil {
				t.Fatal("accepted malformed review")
			}
		})
	}
	provider := &mockProvider{err: errors.New("provider unavailable")}
	if _, err := New(provider).ReviewPlan(context.Background(), "intent", plan); err == nil {
		t.Fatal("provider failure became review pass")
	}
}

func TestRejectedPlanRefinesThroughExistingPromptAndRequiresFreshReview(t *testing.T) {
	plan := &ProjectPlan{Stories: []Story{{ID: "US-1", Title: "Original"}}}
	reviewer := New(&mockProvider{response: `{"approved":false,"assessment":"Missing rollback verification","key_issues":[{"description":"rollback"}]}`})
	review, err := reviewer.ReviewPlan(context.Background(), "retain data", plan)
	if err != nil || review.Approved {
		t.Fatalf("review = %#v %v", review, err)
	}
	builder := &mockProvider{response: `{"stories":[{"id":"US-1","title":"Original with rollback verification"}]}`}
	fixed, err := New(builder).RefinePlan(context.Background(), "retain data", plan, review)
	if err != nil || fixed.Stories[0].ID != "US-1" || !strings.Contains(builder.lastPrompt, "Missing rollback verification") {
		t.Fatalf("refinement lost evidence/identity: %#v %v", fixed, err)
	}
	if review.Approved {
		t.Fatal("refinement modified previous review authority")
	}
	if _, err := New(builder).RefinePlan(context.Background(), "retain data", fixed, &PlanReview{Approved: true}); err == nil {
		t.Fatal("refining approved plan without findings")
	}
}
