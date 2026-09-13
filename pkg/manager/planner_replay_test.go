package manager

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const replayPlanFixture = `{"goals":[{"id":"G-1","title":"Edit","description":"Validated edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Implement edit","description":"Implement and verify editing","mode":"afk"},{"id":"T-2","title":"Verify edit","description":"Verify the running editing journey","mode":"afk","depends_on":["T-1"]}]}]}`
const replayReviewFixture = `{"approved":true,"assessment":"Required verification represented"}`

func replayRequest() PlanRequest {
	return PlanRequest{RequestID: "goal-3-ready-3-gap-A", Intent: "Deliver editing with tested persistence.", NoValidate: true, Review: true, AutoImport: true}
}

func TestReviewedPlanReplaySurvivesCancelledReviewAndCompletedImport(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generated := 0
	e.mgr.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { generated++; return replayPlanFixture, nil })
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { cancel(); return "", ctx.Err() })
	req := replayRequest()
	if _, err := e.mgr.Plan(ctx, req); !errors.Is(err, context.Canceled) {
		t.Fatalf("want interrupted review: %v", err)
	}
	changed := req
	changed.Intent += " Changed before review."
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		t.Fatal("changed request reached reviewer")
		return "", nil
	})
	if _, err := e.mgr.Plan(context.Background(), changed); err == nil {
		t.Fatal("changed generated request accepted")
	}
	fresh := &Manager{cfg: e.mgr.cfg, state: e.mgr.state}
	fresh.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	result, err := fresh.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || generated != 1 {
		t.Fatalf("regenerated or invalid: count=%d result=%+v", generated, result)
	}
	db := e.mgr.state.GetDB()
	if _, err := db.Exec(`UPDATE tasks SET status='in_progress',attempt_count=2,git_branch='retained-candidate',git_pr_number=42 WHERE id='T-1'`); err != nil {
		t.Fatal(err)
	}
	fresh = &Manager{cfg: fresh.cfg, state: e.mgr.state}
	fresh.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("replay regenerated"); return "", nil })
	fresh.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("replay reviewed again"); return "", nil })
	replayed, err := fresh.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ArtifactHash != result.ArtifactHash {
		t.Fatal("plan identity changed")
	}
	var count, attempts, pr int
	var status, branch string
	if err := db.QueryRow(`SELECT COUNT(*) FROM tasks`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("duplicate tasks %d %v", count, err)
	}
	if err := db.QueryRow(`SELECT status,attempt_count,git_branch,git_pr_number FROM tasks WHERE id='T-1'`).Scan(&status, &attempts, &branch, &pr); err != nil {
		t.Fatal(err)
	}
	if status != "in_progress" || attempts != 2 || branch != "retained-candidate" || pr != 42 {
		t.Fatal("replay rewrote retained progress")
	}
	req.Intent += " Changed outcome."
	if _, err := fresh.Plan(context.Background(), req); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("changed input reused request: %v", err)
	}
}

func TestReviewedPlanReplayAtomicImportFailure(t *testing.T) {
	e := newSchedulerTestEnv(t)
	calls := 0
	provider := planCompletionFunc(func(context.Context, string) (string, error) { calls++; return replayPlanFixture, nil })
	e.mgr.cfg.PlanGenerator = provider
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	db := e.mgr.state.GetDB()
	if _, err := db.Exec(`CREATE TRIGGER refuse_second_task BEFORE INSERT ON tasks WHEN NEW.id='T-2' BEGIN SELECT RAISE(ABORT,'injected interruption'); END`); err != nil {
		t.Fatal(err)
	}
	req := replayRequest()
	if _, err := e.mgr.Plan(context.Background(), req); err == nil {
		t.Fatal("injected import failure ignored")
	}
	for _, table := range []string{"goals", "stories", "tasks"} {
		var n int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("partial %s import: %d %v", table, n, err)
		}
	}
	if _, err := db.Exec(`DROP TRIGGER refuse_second_task`); err != nil {
		t.Fatal(err)
	}
	fresh := &Manager{cfg: e.mgr.cfg, state: e.mgr.state}
	fresh.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("retained review rerun"); return "", nil })
	if _, err := fresh.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("regenerated %d times", calls)
	}
}

func TestReviewedPlanReplayRefusesConflictAndMissingAdapters(t *testing.T) {
	e := newSchedulerTestEnv(t)
	req := replayRequest()
	if _, err := e.mgr.Plan(context.Background(), req); err == nil {
		t.Fatal("native fallback permitted")
	}
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(replayPlanFixture)
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		_, err := e.mgr.state.GetDB().Exec(`INSERT INTO goals(id,title,description) VALUES('G-1','Conflicting retained goal','Do not overwrite')`)
		if err != nil {
			t.Fatal(err)
		}
		return replayReviewFixture, nil
	})
	if _, err := e.mgr.Plan(context.Background(), req); err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("retained conflict not refused: %v", err)
	}
	var title string
	if err := e.mgr.state.GetDB().QueryRow(`SELECT title FROM goals WHERE id='G-1'`).Scan(&title); err != nil || title != "Conflicting retained goal" {
		t.Fatal("retained goal modified")
	}
}
