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
)

// layeredBrief is a brief cut by technical section, the shape that sent
// laskenta/Kanavat to the owner's first upload with nothing end to end.
const layeredBrief = `# Kanavat takeoff

Real example: drawings/G3022.pdf (owner-held, not committed).

## Data models
Equipment, drawing and takeoff models.

## Storage
Persist drawings and takeoffs.

## Accounts and tenancy
Per-customer accounts; drawings are private.

## Billing
Charge per takeoff.

## Production operations
Monitoring and backups.

## Upload and result
Upload a drawing and get an equipment takeoff.
`

func shapeStory(id, title, role, reason, example string, deps ...string) map[string]any {
	n := strings.TrimPrefix(id, "US-")
	s := map[string]any{
		"id": id, "title": title, "goal_id": "G-001", "role": role,
		"verification_script": "go test ./...", "depends_on": deps,
		"tasks": []map[string]any{{"id": "T-US-" + n + "-001", "title": title, "description": "Implement and verify " + title, "mode": "afk"}},
	}
	if reason != "" {
		s["role_reason"] = reason
	}
	if example != "" {
		s["real_example"] = example
	}
	return s
}

func shapeJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// layeredPlan is what a planner cutting by brief section returns: layers
// first, the end-to-end slice last, billing and ops ahead of it, no baseline.
func layeredPlan(t *testing.T, version string) string {
	plan := map[string]any{
		"goals": []map[string]any{{"id": "G-001", "title": "Takeoff", "description": "Equipment takeoff from a drawing"}},
		"stories": []map[string]any{
			shapeStory("US-001", "Data models", planner.RoleCore, "", ""),
			shapeStory("US-002", "Storage", planner.RoleCore, "", "", "US-001"),
			shapeStory("US-003", "Accounts and tenancy", planner.RoleInfrastructure, "Customers need accounts", "", "US-002"),
			shapeStory("US-004", "Billing", planner.RoleInfrastructure, "Charge per takeoff", "", "US-003"),
			shapeStory("US-005", "Upload and result", planner.RoleSkeleton, "", "drawings/G3022.pdf", "US-001", "US-002"),
			shapeStory("US-006", "Production operations", planner.RoleInfrastructure, "Monitoring and backups", "", "US-004"),
		},
	}
	if version != "" {
		plan["schema_version"] = version
	}
	return shapeJSON(t, plan)
}

// refinedStories is the fix: tenant isolation the private real example needs
// stays ahead as a stated prerequisite, the skeleton runs on G3022 next, and
// billing and ops follow it.
var refinedStories = []map[string]any{
	shapeStory("US-001", "Tenant isolation for private drawings", planner.RolePrerequisite, "The real example is private; the skeleton must not expose it", ""),
	shapeStory("US-002", "Upload G3022 and see an equipment list", planner.RoleSkeleton, "", "drawings/G3022.pdf (owner-held)", "US-001"),
	shapeStory("US-003", "Models and storage behind the working path", planner.RoleCore, "", "", "US-002"),
	shapeStory("US-004", "Billing", planner.RoleInfrastructure, "Charge once takeoffs are useful", "", "US-003"),
	shapeStory("US-005", "Production operations", planner.RoleInfrastructure, "Run the working path for customers", "", "US-002"),
}

func refinedPlan(t *testing.T) string {
	return shapeJSON(t, map[string]any{
		"baseline": map[string]any{"approach": "A 30-line script reading the PDF text layer", "result": "A useful equipment list in seconds", "bar": "Quantities per equipment type from the uploaded drawing, beyond the text layer", "verdict": "build_on_baseline", "source": "intent: owner's G3022 attempt"},
		"stories":  refinedStories,
	})
}

func storyCount(t *testing.T, m *Manager) int {
	t.Helper()
	var n int
	if err := m.state.GetDB().QueryRow(`SELECT COUNT(*) FROM stories`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

var outcomeShapeRefusals = []string{
	"outcome shape lint refused",
	"plan records no baseline",
	"US-001: first implementation story is core, not the walking skeleton",
	"US-005: skeleton depends on US-001",
	"US-004: infrastructure story does not depend on skeleton US-005",
	"US-006: infrastructure story does not depend on skeleton US-005",
}

// Criteria 1 and 3: the layered brief through manager.Plan with review and
// import is refused even though the reviewer approved it, whatever version
// the model wrote, and nothing is imported.
func TestLayeredBriefPlanRefusedAndNotImported(t *testing.T) {
	for _, version := range []string{"", "1.0.0", "1.1"} {
		t.Run("version="+version, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			if err := os.WriteFile(filepath.Join(e.dir, "INTENT.md"), []byte(layeredBrief), 0600); err != nil {
				t.Fatal(err)
			}
			e.mgr.cfg.PlanGenerator = fixedPlanCompletion(layeredPlan(t, version))
			e.mgr.cfg.PlanReviewer = fixedPlanCompletion(`{"approved":true,"assessment":"Covers every section of the brief"}`)
			result, err := e.mgr.Plan(context.Background(), PlanRequest{IntentFile: "INTENT.md", NoValidate: true, AutoImport: true, Review: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.Valid || result.Review == nil || result.Review.Approved {
				t.Fatalf("layered plan admitted: %+v", result.Review)
			}
			for _, want := range outcomeShapeRefusals {
				if !strings.Contains(result.Review.Assessment, want) {
					t.Fatalf("assessment lacks %q: %s", want, result.Review.Assessment)
				}
			}
			if result.Plan.SchemaVersion != planner.PlanSchemaVersion {
				t.Fatalf("model version %q kept", result.Plan.SchemaVersion)
			}
			if n := storyCount(t, e.mgr); n != 0 {
				t.Fatalf("refused plan imported %d stories", n)
			}
		})
	}
}

// Criterion 2: the same entry point with a durable request refines the
// refused plan; the fixed plan is skeleton-first, keeps the stated
// prerequisite ahead, and is imported in that order with its declared
// depends_on unchanged.
func TestLayeredBriefRefinedSkeletonFirstAndImported(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	var prompts []string
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		prompts = append(prompts, prompt)
		if len(prompts) == 1 {
			return layeredPlan(t, ""), nil
		}
		return refinedPlan(t), nil
	})
	reviews := 0
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		reviews++
		return `{"approved":true,"assessment":"Covers every section of the brief"}`, nil
	})
	req := PlanRequest{RequestID: "kanavat-intake", Intent: layeredBrief, NoValidate: true, Review: true, AutoImport: true}
	result, err := e.mgr.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || reviews != 1 || len(prompts) != 2 {
		t.Fatalf("valid=%v reviews=%d generations=%d", result.Valid, reviews, len(prompts))
	}
	// The fix was asked with the refusals in hand.
	for _, want := range outcomeShapeRefusals {
		if !strings.Contains(prompts[1], want) {
			t.Fatalf("fix prompt lacks %q", want)
		}
	}
	if !strings.Contains(prompts[0], planner.OutcomeShapeRule) || !strings.Contains(prompts[1], planner.OutcomeShapeRule) {
		t.Fatal("generation or fix prompt lacks the outcome shape rule")
	}
	if b := result.Plan.Baseline; b == nil || b.Verdict != planner.VerdictBuildOnBaseline {
		t.Fatalf("baseline lost: %+v", b)
	}
	rows, err := e.mgr.state.GetDB().Query(`SELECT id, depends_on FROM stories ORDER BY priority`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var id, deps string
		if err := rows.Scan(&id, &deps); err != nil {
			t.Fatal(err)
		}
		var parsed []string
		if err := json.Unmarshal([]byte(deps), &parsed); err != nil {
			t.Fatal(err)
		}
		got = append(got, id+"<-"+strings.Join(parsed, ","))
	}
	var want []string
	for _, s := range refinedStories {
		want = append(want, s["id"].(string)+"<-"+strings.Join(s["depends_on"].([]string), ","))
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("imported order/dependencies\n got %v\nwant %v", got, want)
	}
}

// Guard for the post-fix route: a fix that is still layered is refused, not
// imported, because DeterministicPlanIssues carries the outcome shape.
func TestRefinedPlanStillLayeredIsRefused(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	calls := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
		calls++
		if calls == 1 {
			return layeredPlan(t, ""), nil
		}
		var p map[string]any
		_ = json.Unmarshal([]byte(layeredPlan(t, "")), &p)
		p["baseline"] = map[string]any{"approach": "One chat", "result": "A list", "bar": "A takeoff", "verdict": "build", "source": "intent"}
		return shapeJSON(t, p), nil
	})
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(`{"approved":true,"assessment":"Covers every section"}`)
	req := PlanRequest{RequestID: "kanavat-still-layered", Intent: layeredBrief, NoValidate: true, Review: true, AutoImport: true}
	_, err := e.mgr.Plan(context.Background(), req)
	if err == nil || !strings.Contains(err.Error(), "still fails required checks") || !strings.Contains(err.Error(), "first implementation story is core, not the walking skeleton") {
		t.Fatalf("still-layered fix admitted: %v", err)
	}
	if strings.Contains(err.Error(), "plan records no baseline") {
		t.Fatal("baseline the fix supplied was not read")
	}
	if n := storyCount(t, e.mgr); n != 0 {
		t.Fatalf("refused fix imported %d stories", n)
	}
}

// Criterion 4: a plan retained under the earlier prompt and schema replays as
// it was judged: an approved receipt stays approved and a fixed plan awaiting
// its post-fix checks is not judged by the new shape. Neither re-plans nor
// re-reviews.
func TestLegacyRetainedPlanReplaysUnchanged(t *testing.T) {
	for _, stage := range []string{"approved", "fixed"} {
		t.Run(stage, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			e.mgr.cfg.PlanGenerator = fixedPlanCompletion(layeredPlan(t, "1.1"))
			e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { return "", errors.New("reviewer offline") })
			req := PlanRequest{RequestID: "legacy-" + stage, Intent: layeredBrief, NoValidate: true, Review: true, AutoImport: true}
			if _, err := e.mgr.Plan(context.Background(), req); err == nil {
				t.Fatal("offline reviewer accepted")
			}
			// Rewrite the receipt as the earlier build left it.
			db := e.mgr.state.GetDB()
			stepID := "plan-request-" + planDigest(req.RequestID) + "-review"
			var raw string
			if err := db.QueryRow(`SELECT metadata FROM run_steps WHERE id=?`, stepID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var retained retainedPlanRequest
			if err := json.Unmarshal([]byte(raw), &retained); err != nil {
				t.Fatal(err)
			}
			plan := retained.Result.Plan
			plan.SchemaVersion = "1.1"
			id, hash, path := e.mgr.writePlanArtifact(plan)
			retained.Result = &PlanResult{Plan: plan, PlanID: id, ArtifactHash: hash, ArtifactPath: path, PromptVersion: "1.0.0", PlannerBuild: "earlier-build"}
			const judged = "Approved under the earlier planner"
			if stage == "approved" {
				review := &planner.PlanReview{Approved: true, Assessment: judged}
				reviewRaw, _ := json.Marshal(struct {
					PlanDigest string
					Review     *planner.PlanReview
				}{hash, review})
				sum := sha256.Sum256(reviewRaw)
				retained.Result.Review, retained.Result.Valid = review, true
				retained.Result.ReviewArtifactPath = filepath.Join(filepath.Dir(path), hex.EncodeToString(sum[:])+".review.json")
				if err := e.mgr.writePlanEvidence(retained.Result.ReviewArtifactPath, reviewRaw, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				retained.RefinementAttempts = 1
			}
			encoded, _ := json.Marshal(retained)
			if _, err := db.Exec(`UPDATE run_steps SET metadata=? WHERE id=?`, string(encoded), stepID); err != nil {
				t.Fatal(err)
			}
			e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("legacy replay regenerated"); return "", nil })
			e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("legacy replay re-reviewed"); return "", nil })
			result, err := e.mgr.Plan(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Valid || result.Plan.SchemaVersion != "1.1" || result.ArtifactHash != hash {
				t.Fatalf("legacy judgment changed: valid=%v schema=%s", result.Valid, result.Plan.SchemaVersion)
			}
			if stage == "approved" && result.Review.Assessment != judged {
				t.Fatalf("legacy review rewritten: %s", result.Review.Assessment)
			}
			if n := storyCount(t, e.mgr); n != 6 {
				t.Fatalf("legacy plan imported %d stories, want 6", n)
			}
		})
	}
}
