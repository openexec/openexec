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

func TestReviewedPlanRestartResumesRefinementWithinRetainedLimit(t *testing.T) {
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
	if result.Valid || refinements != 1 {
		t.Fatal("retained review bound lost")
	}
	if _, err := fresh.Plan(context.Background(), replayRequest()); err != nil {
		t.Fatal(err)
	}
	if refinements != 1 {
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
