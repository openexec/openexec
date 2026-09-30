package runtime_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/runtime"
)

type schemaSequence func(context.Context, string) (string, error)

func (f schemaSequence) Complete(ctx context.Context, prompt string) (string, error) {
	return f(ctx, prompt)
}

func TestPlannerSchemaRecoveryPublicRuntime(t *testing.T) {
	original := &runtime.ProjectPlan{Goals: []runtime.PlanGoal{{ID: "G-001", Title: "Accepted goal"}}}
	if err := json.Unmarshal(discoveryFixture(t, "scalar"), &original.Stories); err != nil {
		t.Fatal(err)
	}
	// Explicitly authored correction fixture preserves both requirement identities
	// by separating the multi-requirement story. No production coercion is used.
	correction := *original
	correction.Stories = append([]runtime.PlanStory(nil), original.Stories...)
	correction.Stories = append(correction.Stories, runtime.PlanStory{ID: "US-003", Title: "Verify second requirement", GoalID: "G-001", RequirementID: "REQ-002"})
	corrected, err := json.Marshal(correction.Stories)
	if err != nil {
		t.Fatal(err)
	}
	malformed := string(discoveryFixture(t, "incident-array"))
	calls := 0
	p := runtime.NewPlanner(schemaSequence(func(_ context.Context, prompt string) (string, error) {
		calls++
		if calls == 1 {
			return malformed, nil
		}
		if calls != 2 {
			t.Fatal("unbounded correction")
		}
		for _, part := range []string{malformed, "Original intent", "Reviewer evidence", "Story.requirement_id of type string", "SCHEMA CORRECTION"} {
			if !strings.Contains(prompt, part) {
				t.Fatalf("missing %q", part)
			}
		}
		return string(corrected), nil
	}))
	plan, err := p.RefinePlan(context.Background(), "Original intent", original, &runtime.PlanReview{Assessment: "Reviewer evidence"})
	if err != nil || calls != 2 || !reflect.DeepEqual(plan.Goals, original.Goals) || !reflect.DeepEqual(plan.Stories, correction.Stories) {
		t.Fatalf("failed correction: %+v %v calls=%d", plan, err, calls)
	}
}
