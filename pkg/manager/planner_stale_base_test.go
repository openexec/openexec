package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
)

// staleBasePlan is replayPlanFixture with a `git diff <base>...HEAD`
// verification script owned by the story or by task T-2.
func staleBasePlan(t *testing.T, level, base string) string {
	t.Helper()
	var plan map[string]any
	if err := json.Unmarshal([]byte(replayPlanFixture), &plan); err != nil {
		t.Fatal(err)
	}
	script := "git diff --exit-code " + base + "...HEAD -- src"
	story := plan["stories"].([]any)[0].(map[string]any)
	if level == "story" {
		story["verification_script"] = script
	} else {
		story["tasks"].([]any)[1].(map[string]any)["verification_script"] = script
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func staleBaseOwner(level string) string {
	if level == "story" {
		return "story US-1:"
	}
	return "story US-1 task T-2:"
}

// importedTaskCount reopens the release manager on the same database, so it
// reports what persisted rather than the planning manager's cache.
func importedTaskCount(t *testing.T, e *schedulerTestEnv) int {
	t.Helper()
	rel, err := release.NewManagerWithDB(e.dir, release.DefaultConfig(), e.mgr.state.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if err := rel.Load(); err != nil {
		t.Fatal(err)
	}
	return len(rel.GetTasks())
}

// requireStaleRefusal accepts either refusal shape a Manager.Plan route has:
// an import error, or a review refusal returned as an invalid result.
func requireStaleRefusal(t *testing.T, result *PlanResult, err error, owner string) {
	t.Helper()
	diagnostic := ""
	switch {
	case err != nil:
		diagnostic = err.Error()
	case result != nil && !result.Valid:
		diagnostic = strings.Join(result.Issues, "\n")
	default:
		t.Fatalf("stale base ref plan was accepted: %+v", result)
	}
	if !strings.Contains(diagnostic, owner) || !strings.Contains(diagnostic, "origin/main") {
		t.Fatalf("refusal must name %q and the origin/ fix: %s", owner, diagnostic)
	}
}

func approvingReviewer() planCompletionFunc {
	return func(context.Context, string) (string, error) { return replayReviewFixture, nil }
}

func TestManagerPlan_RejectsStaleBaseRef(t *testing.T) {
	for _, level := range []string{"story", "task"} {
		owner := staleBaseOwner(level)
		for _, compact := range []bool{false, true} {
			path := map[bool]string{false: "full", true: "compact"}[compact]
			// native: no review, so importBoundPlan is the only gate.
			t.Run(path+"/native/"+level, func(t *testing.T) {
				for _, base := range []string{"main", "origin/main"} {
					e := newSchedulerTestEnv(t)
					if err := os.WriteFile(filepath.Join(e.dir, "INTENT.md"), []byte("Deliver editing."), 0600); err != nil {
						t.Fatal(err)
					}
					e.mgr.cfg.PlanGenerator = fixedPlanCompletion(staleBasePlan(t, level, base))
					result, err := e.mgr.Plan(context.Background(), PlanRequest{IntentFile: "INTENT.md", NoValidate: true, AutoImport: true, Compact: compact})
					if base == "main" {
						requireStaleRefusal(t, result, err, owner)
						if n := importedTaskCount(t, e); n != 0 {
							t.Fatalf("refused plan imported %d tasks", n)
						}
						continue
					}
					if err != nil || !result.Valid || importedTaskCount(t, e) < 2 {
						t.Fatalf("origin/main plan not persisted: %+v %v", result, err)
					}
				}
			})
			// reviewed: the reviewer approves; ReviewPlan must refuse anyway.
			t.Run(path+"/reviewed/"+level, func(t *testing.T) {
				for _, base := range []string{"main", "origin/main"} {
					e := newSchedulerTestEnv(t)
					e.mgr.cfg.MaxReviewCycles = 1
					e.mgr.cfg.PlanGenerator = fixedPlanCompletion(staleBasePlan(t, level, base))
					e.mgr.cfg.PlanReviewer = approvingReviewer()
					req := replayRequest()
					req.Compact = compact
					result, err := e.mgr.Plan(context.Background(), req)
					if base == "main" {
						requireStaleRefusal(t, result, err, owner)
						if n := importedTaskCount(t, e); n != 0 {
							t.Fatalf("refused plan imported %d tasks", n)
						}
						continue
					}
					if err != nil || !result.Valid || importedTaskCount(t, e) < 2 {
						t.Fatalf("origin/main plan not persisted: %+v %v", result, err)
					}
				}
			})
			// reviewed-direct: Review + AutoImport without a RequestID (route 3),
			// so ReviewPlan and importBoundPlan are the gates, not replay.
			t.Run(path+"/reviewed-direct/"+level, func(t *testing.T) {
				for _, base := range []string{"main", "origin/main"} {
					e := newSchedulerTestEnv(t)
					if err := os.WriteFile(filepath.Join(e.dir, "INTENT.md"), []byte("Deliver editing."), 0600); err != nil {
						t.Fatal(err)
					}
					e.mgr.cfg.PlanGenerator = fixedPlanCompletion(staleBasePlan(t, level, base))
					e.mgr.cfg.PlanReviewer = approvingReviewer()
					result, err := e.mgr.Plan(context.Background(), PlanRequest{IntentFile: "INTENT.md", NoValidate: true, Review: true, AutoImport: true, Compact: compact})
					if base == "main" {
						requireStaleRefusal(t, result, err, owner)
						if n := importedTaskCount(t, e); n != 0 {
							t.Fatalf("refused plan imported %d tasks", n)
						}
						continue
					}
					if err != nil || !result.Valid || importedTaskCount(t, e) < 2 {
						t.Fatalf("origin/main plan not persisted: %+v %v", result, err)
					}
				}
			})

			// refined: a rejected review is repaired into a stale plan; the gate
			// after refinement refuses it before identities are validated.
			t.Run(path+"/refined/"+level, func(t *testing.T) {
				for _, base := range []string{"main", "origin/main"} {
					e := newSchedulerTestEnv(t)
					e.mgr.cfg.MaxReviewCycles = 2
					generated, reviewed := 0, 0
					e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
						generated++
						if generated == 1 {
							return replayPlanFixture, nil
						}
						return staleBasePlan(t, level, base), nil
					})
					e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
						reviewed++
						if reviewed == 1 {
							return rejectedReplayReview, nil
						}
						return replayReviewFixture, nil
					})
					req := replayRequest()
					req.Compact = compact
					result, err := e.mgr.Plan(context.Background(), req)
					if base == "main" {
						if err == nil {
							t.Fatalf("stale refined plan was not refused: %+v", result)
						}
						requireStaleRefusal(t, nil, err, owner)
						if reviewed != 1 {
							t.Fatal("stale refined plan reached re-review")
						}
						if n := importedTaskCount(t, e); n != 0 {
							t.Fatalf("refused plan imported %d tasks", n)
						}
						continue
					}
					if err != nil || !result.Valid || reviewed != 1 || importedTaskCount(t, e) != 2 {
						t.Fatalf("origin/main refinement not persisted: %+v %v", result, err)
					}
				}
			})

			// retained: a receipt approved before the rule existed must be refused
			// on replay without rewriting the receipt or the plan artifact.
			t.Run(path+"/retained/"+level, func(t *testing.T) {
				for _, base := range []string{"main", "origin/main"} {
					e := newSchedulerTestEnv(t)
					e.mgr.cfg.PlanGenerator = fixedPlanCompletion(staleBasePlan(t, level, base))
					interrupted := errors.New("reviewer interrupted")
					e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { return "", interrupted })
					req := replayRequest()
					req.Compact = compact
					if _, err := e.mgr.Plan(context.Background(), req); !errors.Is(err, interrupted) {
						t.Fatalf("expected interrupted review: %v", err)
					}
					stepID := "plan-request-" + planDigest(req.RequestID) + "-review"
					artifact := retainApprovalWithoutReview(t, e, stepID)
					before := receiptMetadata(t, e, stepID)
					artifactBefore, err := os.ReadFile(artifact)
					if err != nil {
						t.Fatal(err)
					}
					e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
						t.Fatal("retained plan regenerated")
						return "", nil
					})
					e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
						t.Fatal("retained approval re-reviewed")
						return "", nil
					})
					result, err := e.mgr.Plan(context.Background(), req)
					if base == "main" {
						if err == nil {
							t.Fatalf("retained stale plan was not refused: %+v", result)
						}
						requireStaleRefusal(t, nil, err, owner)
						if n := importedTaskCount(t, e); n != 0 {
							t.Fatalf("refused plan imported %d tasks", n)
						}
						artifactAfter, err := os.ReadFile(artifact)
						if err != nil || string(artifactAfter) != string(artifactBefore) || receiptMetadata(t, e, stepID) != before {
							t.Fatal("refusal mutated the retained receipt or artifact")
						}
						continue
					}
					if err != nil || !result.Valid || importedTaskCount(t, e) != 2 {
						t.Fatalf("retained origin/main plan not persisted: %+v %v", result, err)
					}
				}
			})
		}
	}
}

func receiptMetadata(t *testing.T, e *schedulerTestEnv, stepID string) string {
	t.Helper()
	var raw string
	if err := e.mgr.state.GetDB().QueryRow(`SELECT metadata FROM run_steps WHERE id=?`, stepID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

// retainApprovalWithoutReview records an approval for the retained plan the
// way replay itself does, standing in for a receipt approved by a build that
// predates the stale-base review rule. It returns the plan artifact path.
func retainApprovalWithoutReview(t *testing.T, e *schedulerTestEnv, stepID string) string {
	t.Helper()
	oldRaw := receiptMetadata(t, e, stepID)
	var retained retainedPlanRequest
	if err := json.Unmarshal([]byte(oldRaw), &retained); err != nil || retained.Result == nil || retained.Result.Review != nil {
		t.Fatalf("expected an unreviewed retained plan: %v", err)
	}
	result := retained.Result
	result.Review = &planner.PlanReview{Approved: true, Assessment: "Approved before the stale-base rule"}
	result.Valid = true
	reviewRaw, _ := json.Marshal(struct {
		PlanDigest string
		Review     *planner.PlanReview
	}{result.ArtifactHash, result.Review})
	sum := sha256.Sum256(reviewRaw)
	result.ReviewArtifactPath = filepath.Join(filepath.Dir(result.ArtifactPath), hex.EncodeToString(sum[:])+".review.json")
	if err := e.mgr.writePlanEvidence(result.ReviewArtifactPath, reviewRaw, 0600); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(retained)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE run_steps SET metadata=?,status='completed' WHERE id=? AND metadata=?`, string(encoded), stepID, oldRaw); err != nil {
		t.Fatal(err)
	}
	return result.ArtifactPath
}
