package manager

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const rejectedReplayReview = `{"approved":false,"assessment":"Missing reload evidence","key_issues":["Verify persistence after reload"]}`

func TestReviewedPlanRefinesRejectedReviewAndImportsOnce(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated, reviewed := 0, 0
	refined := strings.Replace(replayPlanFixture, "Verify the running editing journey", "Verify the running editing journey and persistence after reload", 1)
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		generated++
		if generated == 1 {
			return replayPlanFixture, nil
		}
		if !strings.Contains(prompt, "Missing reload evidence") || !strings.Contains(prompt, "Verify persistence after reload") {
			t.Fatal("refinement omitted retained findings")
		}
		return refined, nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		reviewed++
		if reviewed == 1 {
			return rejectedReplayReview, nil
		}
		if !strings.Contains(prompt, "persistence after reload") {
			t.Fatal("refined plan not independently reviewed")
		}
		return replayReviewFixture, nil
	})
	result, err := e.mgr.Plan(context.Background(), replayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || generated != 2 || reviewed != 2 {
		t.Fatalf("did not close ordinary review loop: %+v %d/%d", result, generated, reviewed)
	}
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); err != nil {
		t.Fatal(err)
	}
	if generated != 2 || reviewed != 2 {
		t.Fatal("completed refinement repeated")
	}
	var reviews, imports, tasks int
	db := e.mgr.state.GetDB()
	db.QueryRow(`SELECT COUNT(*) FROM run_steps WHERE agent='plan-review-history'`).Scan(&reviews)
	db.QueryRow(`SELECT COUNT(*) FROM run_steps WHERE agent='reviewed-plan-import'`).Scan(&imports)
	db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks)
	if reviews != 2 || imports != 1 || tasks != 2 {
		t.Fatalf("history/import count %d/%d/%d", reviews, imports, tasks)
	}
}

func TestReviewedPlanRestartDoesNotRefundInterruptedRefinement(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
		generated++
		if generated == 1 {
			return replayPlanFixture, nil
		}
		return "", context.Canceled
	})
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(rejectedReplayReview)
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected refinement interruption: %v", err)
	}
	fresh := &Manager{cfg: e.mgr.cfg, state: e.mgr.state}
	fresh.cfg.MaxReviewCycles = 100 // A changed process configuration cannot extend this request.
	refinements := 0
	fresh.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, prompt string) (string, error) {
		refinements++
		if !strings.Contains(prompt, "Missing reload evidence") {
			t.Fatal("restart regenerated rather than refined")
		}
		return strings.Replace(replayPlanFixture, "Verify the running editing journey", "Verify retained reload result", 1), nil
	})
	result, err := fresh.Plan(context.Background(), replayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid || refinements != 0 {
		t.Fatal("retained review bound lost")
	}
	if _, err := fresh.Plan(context.Background(), replayRequest()); err != nil {
		t.Fatal(err)
	}
	if refinements != 0 {
		t.Fatal("exhausted request refined again")
	}
	var tasks int
	fresh.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&tasks)
	if tasks != 0 {
		t.Fatal("rejected plan imported")
	}
}

func TestReviewedPlanRefinementConflictRefusesBeforeRereview(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated, reviews := 0, 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) {
		generated++
		if generated == 1 {
			return replayPlanFixture, nil
		}
		if _, err := e.mgr.state.GetDB().Exec(`INSERT INTO goals(id,title,description) VALUES('G-1','Edit','Different retained purpose')`); err != nil {
			t.Fatal(err)
		}
		return strings.Replace(replayPlanFixture, "Verify the running editing journey", "Verify reload evidence", 1), nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		reviews++
		if reviews > 1 {
			t.Fatal("conflicting plan reached rereview")
		}
		return rejectedReplayReview, nil
	})
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("conflict not refused: %v", err)
	}
	var stories int
	e.mgr.state.GetDB().QueryRow(`SELECT COUNT(*) FROM stories`).Scan(&stories)
	if stories != 0 {
		t.Fatal("pre-review validation persisted work")
	}
}

// The production failure of 2026-09-19: the fix prompt asks for stories only,
// the model answers with a bare array whose stories still cite the original
// goal, and the reviewed import died with "FOREIGN KEY constraint failed (787)".
func TestReviewedPlanRefinementKeepsCitedGoalsFromABareStoryArray(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 2
	generated, reviewed := 0, 0
	refined := `[{"id":"US-001","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-US-001-001","title":"Implement edit","description":"Implement and verify editing","mode":"afk"},{"id":"T-US-001-002","title":"Verify edit","description":"Verify the running editing journey and persistence after reload","mode":"afk","depends_on":["T-US-001-001"]}]}]`
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, _ string) (string, error) {
		generated++
		if generated == 1 {
			return replayPlanFixture, nil
		}
		return refined, nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, _ string) (string, error) {
		reviewed++
		if reviewed == 1 {
			return rejectedReplayReview, nil
		}
		return replayReviewFixture, nil
	})
	result, err := e.mgr.Plan(context.Background(), replayRequest())
	if err != nil {
		t.Fatalf("refined plan import failed: %v", err)
	}
	if !result.Valid || generated != 2 || reviewed != 2 || len(result.Plan.Goals) != 1 || result.Plan.Goals[0].ID != "G-1" {
		t.Fatalf("refined plan lost its goal: %+v %d/%d", result.Plan.Goals, generated, reviewed)
	}
	var goals, stories int
	db := e.mgr.state.GetDB()
	db.QueryRow(`SELECT COUNT(*) FROM goals WHERE id='G-1'`).Scan(&goals)
	db.QueryRow(`SELECT COUNT(*) FROM stories WHERE goal_id='G-1'`).Scan(&stories)
	if goals != 1 || stories != 1 {
		t.Fatalf("goal/story persisted %d/%d", goals, stories)
	}
}

func TestReviewedPlanImportNamesADanglingGoalReference(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 1
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(_ context.Context, _ string) (string, error) {
		return `{"goals":[{"id":"G-1","title":"Edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-404","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Implement edit","description":"Implement","mode":"afk"}]}]}`, nil
	})
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(_ context.Context, _ string) (string, error) { return replayReviewFixture, nil })
	_, err := e.mgr.Plan(context.Background(), replayRequest())
	if err == nil || !strings.Contains(err.Error(), "US-1 references goal G-404") || strings.Contains(err.Error(), "787") {
		t.Fatalf("dangling goal not named: %v", err)
	}
}

// A refusal replayed after the planner changed is the same refusal forever:
// Agent Console Goal b5b5611a got three "fresh plans" in three seconds after
// the planner fix it waited for went live. A refused receipt from another
// build is planned again; an approved one, and one from this build, replay.
func TestReviewedPlanRefusalIsReplannedByAChangedPlanner(t *testing.T) {
	build := "old"
	defer func(previous func() string) { plannerBuild = previous }(plannerBuild)
	plannerBuild = func() string { return build }
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.MaxReviewCycles = 1
	generated := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { generated++; return replayPlanFixture, nil })
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(rejectedReplayReview)
	refused, err := e.mgr.Plan(context.Background(), replayRequest())
	if err != nil || refused.Valid {
		t.Fatalf("want a refused plan: %+v %v", refused, err)
	}
	if _, err := e.mgr.Plan(context.Background(), replayRequest()); err != nil || generated != 1 {
		t.Fatalf("same build re-planned a refusal: generated=%d %v", generated, err)
	}
	build = "fixed"
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	approved, err := e.mgr.Plan(context.Background(), replayRequest())
	if err != nil {
		t.Fatal(err)
	}
	if !approved.Valid || generated != 2 {
		t.Fatalf("changed planner replayed the refusal: generated=%d %+v", generated, approved)
	}
	build = "later"
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("approved plan regenerated"); return "", nil })
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("approved plan reviewed again"); return "", nil })
	replayed, err := e.mgr.Plan(context.Background(), replayRequest())
	if err != nil || replayed.ArtifactHash != approved.ArtifactHash {
		t.Fatalf("approved receipt not replayed: %+v %v", replayed, err)
	}
}
