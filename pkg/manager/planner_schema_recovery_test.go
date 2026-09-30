package manager

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestReviewedPlanSchemaCorrection(t *testing.T) {
	for _, scenario := range []string{"approved", "rejected", "malformed", "interrupted", "boundary-removed", "remaining-budget"} {
		t.Run(scenario, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			e.mgr.cfg.MaxReviewCycles = 2
			if scenario == "remaining-budget" {
				e.mgr.cfg.MaxReviewCycles = 3
			}
			original := strings.Replace(replayPlanFixture, `"mode":"afk"`, `"mode":"hitl","decision_reason":"Owner must accept exact candidate"`, 1)
			malformed := strings.Replace(original, `"goal_id":"G-1"`, `"goal_id":"G-1","requirement_id":["REQ-001","REQ-002"]`, 1)
			corrected := strings.Replace(original, "Verify the running editing journey", "Verify the running editing journey after reload", 1)
			if scenario == "boundary-removed" {
				corrected = replayPlanFixture
			}
			calls, reviews := 0, 0
			e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
				calls++
				switch calls {
				case 1:
					return original, nil
				case 2:
					return malformed, nil
				case 3:
					for _, evidence := range []string{replayRequest().Intent, "Missing reload evidence", "SCHEMA CORRECTION", "requirement_id of type string", malformed, "Owner must accept exact candidate"} {
						if !strings.Contains(prompt, evidence) {
							t.Fatalf("missing correction evidence %q", evidence)
						}
					}
					var raw string
					if err := e.mgr.state.GetDB().QueryRow(`SELECT metadata FROM run_steps WHERE agent='reviewed-planner'`).Scan(&raw); err != nil {
						t.Fatal(err)
					}
					var retained retainedPlanRequest
					if err := json.Unmarshal([]byte(raw), &retained); err != nil {
						t.Fatal(err)
					}
					if retained.RefinementAttempts != 1 || !strings.Contains(retained.SchemaCorrection, malformed) {
						t.Fatal("dispatch occurred before durable reservation")
					}
					if scenario == "interrupted" || scenario == "remaining-budget" {
						return "", context.Canceled
					}
					if scenario == "malformed" {
						return malformed, nil
					}
					if scenario == "boundary-removed" {
						return strings.Replace(corrected, `"id":"T-1"`, `"id":"removed"`, 1), nil
					}
					return corrected, nil
				case 4:
					if scenario == "remaining-budget" {
						return malformed, nil
					}
					fallthrough
				default:
					t.Fatal("unbounded correction")
					return "", nil
				}
			})
			e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
				reviews++
				if reviews == 1 || scenario == "rejected" {
					return rejectedReplayReview, nil
				}
				if !strings.Contains(prompt, "after reload") {
					t.Fatal("review did not see correction")
				}
				return replayReviewFixture, nil
			})
			result, err := e.mgr.Plan(context.Background(), replayRequest())
			switch scenario {
			case "approved":
				if err != nil || !result.Valid {
					t.Fatalf("correction failed: %+v %v", result, err)
				}
			case "rejected":
				if err != nil || result.Valid {
					t.Fatalf("rejection bypassed: %+v %v", result, err)
				}
			case "interrupted", "remaining-budget":
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("want cancellation: %v", err)
				}
			default:
				if err == nil || result != nil {
					t.Fatalf("invalid correction escaped: %+v %v", result, err)
				}
			}
			if calls != 3 {
				t.Fatalf("calls=%d", calls)
			}
			wantReviews := 1
			if scenario == "approved" || scenario == "rejected" {
				wantReviews = 2
			}
			if reviews != wantReviews {
				t.Fatalf("reviews=%d", reviews)
			}
			e.mgr.Close()
			e.closeState()
			fresh := freshQueueManager(t, e)
			fresh.cfg.MaxReviewCycles = 100
			again, err := fresh.Plan(context.Background(), replayRequest())
			wantCalls := 3
			if scenario == "remaining-budget" {
				wantCalls = 4
				if err == nil || !strings.Contains(err.Error(), "retained schema correction budget exhausted") {
					t.Fatalf("correction refunded: %v", err)
				}
				again, err = fresh.Plan(context.Background(), replayRequest())
			}
			if err != nil {
				t.Fatal(err)
			}
			if again.Valid != (scenario == "approved") || calls != wantCalls || reviews != wantReviews {
				t.Fatal("restart refunded budget or changed approval")
			}
			var metadata string
			if err := fresh.state.GetDB().QueryRow(`SELECT metadata FROM run_steps WHERE agent='reviewed-planner'`).Scan(&metadata); err != nil {
				t.Fatal(err)
			}
			var receipt retainedPlanRequest
			if err := json.Unmarshal([]byte(metadata), &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.RefinementAttempts != wantCalls-2 || !strings.Contains(receipt.SchemaCorrection, malformed) || receipt.ReviewLimit != wantCalls-1 {
				t.Fatalf("reopened accounting/evidence changed: %+v", receipt)
			}
			var tasks, imports int
			if err := fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil {
				t.Fatal(err)
			}
			if err := fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM run_steps WHERE agent='reviewed-plan-import'`).Scan(&imports); err != nil {
				t.Fatal(err)
			}
			if scenario == "approved" {
				if tasks != 2 || imports != 1 {
					t.Fatalf("persisted tasks/imports=%d/%d", tasks, imports)
				}
				if again.Plan.Stories[0].Tasks[0].Mode != "hitl" || len(again.Plan.Goals) != 1 {
					t.Fatal("lost retained authority or goal")
				}
			} else if tasks != 0 || imports != 0 {
				t.Fatal("unapproved/partial plan imported")
			}
		})
	}
}
