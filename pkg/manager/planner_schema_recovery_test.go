package manager

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestReviewedPlanSchemaCorrection(t *testing.T) {
	for _, scenario := range []string{"approved", "rejected", "malformed", "interrupted", "boundary-removed", "remaining-budget", "missing-approval", "malformed-approval", "legacy-hitl", "invalid-boundary"} {
		t.Run(scenario, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			e.mgr.cfg.MaxReviewCycles = 2
			if scenario == "remaining-budget" {
				e.mgr.cfg.MaxReviewCycles = 3
			}
			original := strings.Replace(replayPlanFixture, `"mode":"afk"`, `"mode":"hitl","decision_reason":"Owner must accept exact candidate"`, 1)
			if scenario == "legacy-hitl" {
				original = strings.Replace(original, `,"decision_reason":"Owner must accept exact candidate"`, "", 1)
			}
			malformed := strings.Replace(original, `"goal_id":"G-1"`, `"goal_id":"G-1","requirement_id":["REQ-001","REQ-002"]`, 1)
			corrected := strings.Replace(original, "Verify the running editing journey", "Verify the running editing journey after reload", 1)
			corrected = strings.Replace(corrected, `"goal_id":"G-1"`, `"goal_id":"G-1","requirement_id":"REQ-001"`, 1)
			if scenario == "invalid-boundary" {
				corrected = strings.Replace(corrected, `"mode":"afk"`, `"mode":"unknown"`, 1)
			}
			diagnostic := "json: cannot unmarshal array into Go struct field Story.stories.requirement_id of type string"
			retainedDiagnostic := "failed to parse LLM response as JSON: " + diagnostic + "\nResponse was: " + malformed
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
					for _, evidence := range []string{replayRequest().Intent, "Missing reload evidence", "SCHEMA CORRECTION", diagnostic, malformed} {
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
					planEvidence, err := json.Marshal(retained.Result.Plan)
					if err != nil || !strings.Contains(prompt, string(planEvidence)) {
						t.Fatal("missing retained original plan")
					}
					if retained.RefinementAttempts != 1 || retained.SchemaCorrection != retainedDiagnostic {
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
				if scenario == "missing-approval" {
					return `{"assessment":"No explicit decision"}`, nil
				}
				if scenario == "malformed-approval" {
					return "{", nil
				}
				if !strings.Contains(prompt, `"requirement_id":"REQ-001"`) {
					t.Fatal("review lost scalar requirement")
				}
				return replayReviewFixture, nil
			})
			result, err := e.mgr.Plan(context.Background(), replayRequest())
			// One review, one fix: the corrected plan is imported without a
			// second review, but still has to pass the deterministic checks.
			switch scenario {
			case "approved", "rejected", "missing-approval", "malformed-approval":
				if err != nil || !result.Valid {
					t.Fatalf("correction failed: %+v %v", result, err)
				}
			case "legacy-hitl", "invalid-boundary":
				if err == nil || !strings.Contains(err.Error(), "human boundary lint refused") {
					t.Fatalf("an invalid boundary was imported after its fix: %+v %v", result, err)
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
			imported := scenario == "approved" || scenario == "rejected" || scenario == "missing-approval" || scenario == "malformed-approval"
			if reviews != wantReviews {
				t.Fatalf("reviews=%d", reviews)
			}
			if imported {
				if _, err := e.mgr.state.GetDB().Exec(`UPDATE tasks SET status='in_progress',attempt_count=2 WHERE id='T-1'`); err != nil {
					t.Fatal(err)
				}
			}
			e.mgr.Close()
			e.closeState()
			fresh := freshQueueManager(t, e)
			fresh.cfg.MaxReviewCycles = 100
			if scenario == "missing-approval" || scenario == "malformed-approval" {
				// A later explicit rejection may finish the pending review, never import it.
				fresh.cfg.PlanReviewer = fixedPlanCompletion(rejectedReplayReview)
			}
			again, err := fresh.Plan(context.Background(), replayRequest())
			if scenario == "legacy-hitl" || scenario == "invalid-boundary" {
				// The refusal holds across a restart, and nothing was imported.
				var tasks int
				fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks)
				if err == nil || !strings.Contains(err.Error(), "human boundary lint refused") || tasks != 0 {
					t.Fatalf("an invalid boundary was imported after a restart: %v tasks=%d", err, tasks)
				}
				return
			}
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
			if again.Valid != imported || calls != wantCalls || reviews != wantReviews {
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
			if receipt.RefinementAttempts != wantCalls-2 || receipt.SchemaCorrection != retainedDiagnostic || receipt.ReviewLimit != wantCalls-1 {
				t.Fatalf("reopened accounting/evidence changed: %+v", receipt)
			}
			var tasks, imports int
			if err := fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks); err != nil {
				t.Fatal(err)
			}
			if err := fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM run_steps WHERE agent='reviewed-plan-import'`).Scan(&imports); err != nil {
				t.Fatal(err)
			}
			if imported {
				var status string
				var attempts, history int
				if err := fresh.state.GetDB().QueryRow(`SELECT status,attempt_count FROM tasks WHERE id='T-1'`).Scan(&status, &attempts); err != nil {
					t.Fatal(err)
				}
				if status != "in_progress" || attempts != 2 {
					t.Fatal("import replay reset task progress")
				}
				if err := fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM run_steps WHERE agent='plan-review-history'`).Scan(&history); err != nil {
					t.Fatal(err)
				}
				if history != 2 {
					t.Fatalf("lost independent review history: %d", history)
				}
				if tasks != 2 || imports != 1 {
					t.Fatalf("persisted tasks/imports=%d/%d", tasks, imports)
				}
				if again.Plan.Stories[0].RequirementID != "REQ-001" {
					t.Fatal("scalar mapping lost on reopen")
				}
				if again.Plan.Stories[0].Tasks[0].Mode != "hitl" || len(again.Plan.Goals) != 1 {
					t.Fatal("lost retained authority or goal")
				}
			} else if tasks != 0 || imports != 0 {
				t.Fatal("unapproved/partial plan imported")
			}
			if scenario == "legacy-hitl" {
				task := again.Plan.Stories[0].Tasks[0]
				if task.Mode != "hitl" || task.DecisionReason != "" {
					t.Fatal("legacy boundary silently changed")
				}
			}
		})
	}
}
