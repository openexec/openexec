package planner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDeliveryBoundarySharedAcrossPlanningPaths(t *testing.T) {
	for name, prompt := range map[string]string{"generation": StoryGenerationPrompt, "compact": CompactStoryGenerationPrompt, "review": StoryReviewPrompt, "refinement": StoryFixPrompt} {
		t.Run(name, func(t *testing.T) {
			if !strings.Contains(prompt, DeliveryBoundaryRule) {
				t.Fatal("planning path omits post-queue delivery boundary")
			}
			if name != "review" && !strings.Contains(prompt, `"contract"`) {
				t.Fatal("output cannot retain outstanding delivery obligation")
			}
		})
	}
}

func TestDeliveryContractSurvivesPlanningReviewAndRefinement(t *testing.T) {
	const response = `{"schema_version":"1.0.0","goals":[{"id":"G","title":"Correction and delivery"}],"stories":[{"id":"S","goal_id":"G","requirement_id":"D2","title":"Prepare correction","contract":"D2 outstanding: Agent Console default-branch merge after queue; owner controls retained","acceptance_criteria":["D1 native preparation verified","D2 remains outstanding until actual coordinator merge evidence"],"verification_script":"python3 scripts/verify-exhausted-task-records.py","tasks":[{"id":"A","title":"Prepare","mode":"afk","verification_script":"test -f prepared"}]}]}`
	provider := &mockProvider{response: response}
	p := New(provider)
	ctx := context.Background()
	original, err := p.GenerateCompactPlan(ctx, "Prepare D1, retain post-queue D2 under Console controls")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(provider.lastPrompt, DeliveryBoundaryRule) {
		t.Fatal("generation boundary absent")
	}
	wire, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var reopened ProjectPlan
	if err := json.Unmarshal(wire, &reopened); err != nil {
		t.Fatal(err)
	}
	refined, err := p.RefinePlan(ctx, "D2 outstanding", &reopened, &PlanReview{Assessment: "Keep D2 outside the native task queue"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(provider.lastPrompt, DeliveryBoundaryRule) {
		t.Fatal("refinement boundary absent")
	}
	provider.response = `{"approved":true,"assessment":"Preparation runnable, D2 explicitly outstanding"}`
	review, err := p.ReviewPlan(ctx, "D2 outstanding", refined)
	if err != nil || !review.Approved {
		t.Fatalf("review: %v %v", review, err)
	}
	if !strings.Contains(provider.lastPrompt, DeliveryBoundaryRule) {
		t.Fatal("review boundary absent")
	}
	after, err := json.Marshal(refined)
	if err != nil || string(after) != string(wire) {
		t.Fatal("delivery contract lost in round trip", err)
	}
	provider.response = `{"approved":false,"assessment":"Merge task wrongly blocks queue"}`
	review, err = p.ReviewPlan(ctx, "D2 outstanding", refined)
	if err != nil || review.Approved {
		t.Fatal("independent rejection overridden", err)
	}
}
