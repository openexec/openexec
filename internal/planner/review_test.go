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

func TestRefinedPlanKeepsTheGoalsItsStoriesCite(t *testing.T) {
	review := &PlanReview{Assessment: "stories not implementation-ready", KeyIssues: []byte(`[{"description":"coverage"}]`)}
	original := &ProjectPlan{SchemaVersion: "1.0", Goals: []Goal{{ID: "G-006", Title: "Verified repair"}, {ID: "G-007", Title: "Unused"}}, Stories: []Story{{ID: "US-001", Title: "Original", GoalID: "G-006"}}}
	cases := []struct {
		name     string
		response string
		want     []string
	}{
		{"bare story array keeps every original goal", `[{"id":"US-001","title":"Fixed","goal_id":"G-006"},{"id":"US-002","title":"More","goal_id":"G-006"}]`, []string{"G-006", "G-007"}},
		{"object without goals keeps every original goal", `{"stories":[{"id":"US-001","title":"Fixed","goal_id":"G-006"}]}`, []string{"G-006", "G-007"}},
		{"explicit goals are kept and cited originals are added", `{"goals":[{"id":"G-008","title":"New"}],"stories":[{"id":"US-001","title":"Fixed","goal_id":"G-006"}]}`, []string{"G-008", "G-006"}},
		{"unknown citation is left for import validation", `{"goals":[{"id":"G-008","title":"New"}],"stories":[{"id":"US-001","title":"Fixed","goal_id":"G-999"}]}`, []string{"G-008"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			refined, err := New(&mockProvider{response: tc.response}).RefinePlan(context.Background(), "intent", original, review)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, g := range refined.Goals {
				got = append(got, g.ID)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("goals = %v, want %v", got, tc.want)
			}
			if len(original.Goals) != 2 {
				t.Fatal("original plan mutated")
			}
		})
	}
}

// The fix prompt's story shape carries no goal_id, and the model returned
// every refined story without one. Saved as ”, each story then pointed at a
// goal no row has: "FOREIGN KEY constraint failed (787)".
func TestRefinedStoriesWithoutAGoalKeepTheGoalTheyServed(t *testing.T) {
	review := &PlanReview{Assessment: "stories not implementation-ready"}
	original := &ProjectPlan{Goals: []Goal{{ID: "G-006", Title: "Repair"}, {ID: "G-007", Title: "Deliver"}}, Stories: []Story{{ID: "US-014", Title: "Reconcile", GoalID: "G-007"}, {ID: "US-015", Title: "Repair", GoalID: "G-006"}}}
	refined, err := New(&mockProvider{response: `[{"id":"US-015","title":"Repair better"},{"id":"US-014","title":"Reconcile better"},{"id":"US-099","title":"New"}]`}).RefinePlan(context.Background(), "intent", original, review)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range refined.Stories {
		got[s.ID] = s.GoalID
	}
	if got["US-014"] != "G-007" || got["US-015"] != "G-006" || got["US-099"] != "" {
		t.Fatalf("story goals = %v", got)
	}
}
