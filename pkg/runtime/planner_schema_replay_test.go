package runtime_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/runtime"
)

// Exercise only the exported Console-facing seam with separate fixed providers.
func TestPlannerSchemaReplayJourney(t *testing.T) {
	for _, scenario := range []string{"approved", "exhausted", "removed-boundary", "legacy-hitl", "missing-approval", "rejected"} {
		t.Run(scenario, func(t *testing.T) {
			initial := `{"goals":[{"id":"G-1","title":"Edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-1","requirement_id":"REQ-001","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Owner acceptance","description":"Accept exact candidate","mode":"hitl","decision_reason":"Owner acceptance required"},{"id":"T-2","title":"Verify reload","description":"Verify persisted edit","mode":"afk","depends_on":["T-1"]}]}]}`
			if scenario == "legacy-hitl" {
				initial = strings.Replace(initial, `,"decision_reason":"Owner acceptance required"`, "", 1)
			}
			malformed := strings.Replace(initial, `"requirement_id":"REQ-001"`, `"requirement_id":["REQ-001"]`, 1)
			corrected := strings.Replace(initial, "Verify persisted edit", "Verify persisted edit after reload", 1)
			if scenario == "exhausted" {
				corrected = malformed
			}
			if scenario == "removed-boundary" {
				corrected = strings.Replace(corrected, `"id":"T-1"`, `"id":"removed"`, 1)
				corrected = strings.Replace(corrected, "Owner acceptance required", "Different decision", 1)
			}
			ctx := context.Background()
			calls, reviews := 0, 0
			var originalEvidence string
			generator := runtime.NewPlanner(schemaSequence(func(_ context.Context, prompt string) (string, error) {
				calls++
				switch calls {
				case 1:
					return initial, nil
				case 2:
					return malformed, nil
				case 3:
					for _, evidence := range []string{
						"Deliver editing", originalEvidence, "Reload evidence missing", "Verify reload",
						"json: cannot unmarshal array into Go struct field Story.stories.requirement_id of type string",
						malformed, "SCHEMA CORRECTION (one attempt)",
					} {
						if !strings.Contains(prompt, evidence) {
							t.Fatalf("missing evidence %q", evidence)
						}
					}
					return corrected, nil
				default:
					t.Fatal("correction exceeded bound")
					return "", nil
				}
			}))
			reviewer := runtime.NewPlanner(schemaSequence(func(_ context.Context, prompt string) (string, error) {
				reviews++
				if reviews == 1 {
					return `{"approved":false,"assessment":"Reload evidence missing","key_issues":["Verify reload"]}`, nil
				}
				if !strings.Contains(prompt, "after reload") || strings.Contains(prompt, "SCHEMA CORRECTION") {
					t.Fatal("independent reviewer did not receive just the corrected plan")
				}
				if scenario == "missing-approval" {
					return `{"assessment":"Looks complete"}`, nil
				}
				if scenario == "rejected" {
					return `{"approved":false,"assessment":"Still incomplete"}`, nil
				}
				return `{"approved":true,"assessment":"Verified"}`, nil
			}))
			original, err := generator.GeneratePlan(ctx, "Deliver editing")
			if err != nil {
				t.Fatal(err)
			}
			if err := original.Validate(); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			originalEvidence = string(raw)
			rejection, err := reviewer.ReviewPlan(ctx, "Deliver editing", original)
			if err != nil || rejection.Approved {
				t.Fatalf("initial rejection: %+v %v", rejection, err)
			}
			plan, err := generator.RefinePlan(ctx, "Deliver editing", original, rejection)
			if calls != 3 {
				t.Fatalf("completion count %d", calls)
			}
			if scenario == "exhausted" || scenario == "removed-boundary" {
				if err == nil || plan != nil || reviews != 1 {
					t.Fatalf("invalid refinement escaped: %+v %v", plan, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := plan.Validate(); err != nil {
				t.Fatal(err)
			}
			if plan.Stories[0].RequirementID != "REQ-001" || plan.Stories[0].Tasks[0].Mode != "hitl" {
				t.Fatal("correction changed mapping or boundary")
			}
			approval, err := reviewer.ReviewPlan(ctx, "Deliver editing", plan)
			if scenario == "missing-approval" {
				if err == nil || approval != nil {
					t.Fatal("implicit approval accepted")
				}
			} else if err != nil || approval.Approved != (scenario == "approved") {
				t.Fatalf("approval or boundary validation bypassed: %+v %v", approval, err)
			}
			if reviews != 2 {
				t.Fatalf("review count %d", reviews)
			}
			if scenario == "legacy-hitl" && plan.Stories[0].Tasks[0].DecisionReason != "" {
				t.Fatal("legacy reason invented")
			}
		})
	}
}
