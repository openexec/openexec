package manager

import (
	"context"
	"encoding/json"
	"github.com/openexec/openexec/internal/planner"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func identityFixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/reviewed-identity/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReviewedIdentityLifecycle(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "retained"))
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	firstReq := replayRequest()
	first, err := e.mgr.Plan(ctx, firstReq)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE tasks SET status='done',attempt_count=2,completed_at='2026-01-01T00:00:00Z',git_branch='retained',git_commits='["retained-commit"]',metadata='{"mode":"afk","evidence":"retained"}'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE stories SET status='done'`); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`SELECT * FROM goals WHERE id IN ('G-001','G-002') ORDER BY id`,
		`SELECT * FROM stories WHERE id IN ('US-001','US-005') ORDER BY id`,
		`SELECT * FROM tasks WHERE id IN ('T-US-001-001','T-US-005-001') ORDER BY id`,
		`SELECT * FROM run_steps WHERE agent='reviewed-plan-import' ORDER BY id`,
	}
	var firstStep string
	if err := e.mgr.state.GetDB().QueryRow(`SELECT id FROM run_steps WHERE agent='reviewed-plan-import'`).Scan(&firstStep); err != nil {
		t.Fatal(err)
	}
	before := []string{}
	for _, q := range queries {
		before = append(before, identitySnapshot(t, e.mgr, q))
	}
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "subsequent"))
	req := firstReq
	req.RequestID += "-wave2"
	second, err := e.mgr.Plan(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	// A persisted ID is never renamed: the restated stories stay as they
	// were imported, and the new tasks the wave put into them follow in
	// continuation stories with IDs nothing persisted holds.
	p := second.Plan
	if len(p.Stories) != 4 || p.Stories[0].ID != "US-001" || p.Stories[2].ID != "US-005" {
		t.Fatalf("persisted stories renamed: %+v", p.Stories)
	}
	if !slices.Contains(p.Continues, "US-001") || !slices.Contains(p.Continues, "T-US-001-001") || !slices.Contains(p.Continues, "US-005") {
		t.Fatalf("continues = %v", p.Continues)
	}
	c1, c2 := p.Stories[1], p.Stories[3]
	for i, c := range []planner.Story{c1, c2} {
		retained := []string{"US-001", "US-005"}[i]
		if c.ID == retained || c.GoalID != "G-002" || !slices.Contains(c.DependsOn, retained) || len(c.Tasks) != 1 || c.Tasks[0].ID != "T-"+c.ID+"-001" {
			t.Fatalf("inconsistent continuation: %+v", c)
		}
	}
	if !reflect.DeepEqual(c2.Tasks[0].DependsOn, []string{c1.Tasks[0].ID}) {
		t.Fatal("dependencies not rewritten")
	}
	for i, q := range queries[:3] {
		if identitySnapshot(t, e.mgr, q) != before[i] {
			t.Fatalf("retained table changed: %s", q)
		}
	}
	artifact, err := os.ReadFile(second.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted planner.ProjectPlan
	if err := json.Unmarshal(artifact, &persisted); err != nil || !reflect.DeepEqual(&persisted, p) {
		t.Fatalf("artifact differs from import: %v", err)
	}
	goals, stories, tasks := reviewedPlanRows(&persisted)
	if err := e.rel.ValidatePlanIdentities(ctx, goals, stories, tasks); err != nil {
		t.Fatal(err)
	}
	all := []string{}
	for _, table := range []string{"goals", "stories", "tasks", "run_steps"} {
		all = append(all, identitySnapshot(t, e.mgr, "SELECT * FROM "+table+" ORDER BY id"))
	}
	e.mgr.Close()
	e.closeState()
	fresh := freshQueueManager(t, e)
	fresh.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("replay generated"); return "", nil })
	fresh.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("replay reviewed"); return "", nil })
	for i, r := range []PlanRequest{firstReq, req} {
		got, err := fresh.Plan(ctx, r)
		if err != nil {
			t.Fatal(err)
		}
		want := []*PlanResult{first, second}[i]
		if got.ArtifactHash != want.ArtifactHash || !reflect.DeepEqual(got.Plan, want.Plan) {
			t.Fatal("replay changed exact plan")
		}
	}
	for i, table := range []string{"goals", "stories", "tasks", "run_steps"} {
		if identitySnapshot(t, fresh, "SELECT * FROM "+table+" ORDER BY id") != all[i] {
			t.Fatalf("reopen replay changed %s", table)
		}
	}
	// Preparing the already allocated content is independently idempotent.
	if err := fresh.preparePlanIDs(&persisted); err != nil {
		t.Fatal(err)
	}
	// After import, rows that now match their persisted content need no
	// listing in Continues; every ID and all content stay as reviewed.
	want := *p
	want.Continues, persisted.Continues = nil, nil
	if !reflect.DeepEqual(&persisted, &want) {
		t.Fatal("exact allocated plan moved on preparation")
	}
	// A distinct request with exact content must not duplicate retained rows.
	fresh.cfg.PlanGenerator = fixedPlanCompletion(string(artifact))
	fresh.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	third := req
	third.RequestID += "-exact"
	if _, err := fresh.Plan(ctx, third); err != nil {
		t.Fatal(err)
	}
	for i, table := range []string{"goals", "stories", "tasks"} {
		if identitySnapshot(t, fresh, "SELECT * FROM "+table+" ORDER BY id") != all[i] {
			t.Fatalf("exact new request duplicated %s", table)
		}
	}
	if identitySnapshot(t, fresh, `SELECT * FROM run_steps WHERE id=?`, firstStep) != before[3] {
		t.Fatal("initial receipt changed during subsequent waves")
	}
	// Receipt bytes and hashes are binding even after a real reopen.
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	var step, run, digest, receipt string
	if err := fresh.state.GetDB().QueryRow(`SELECT id,run_id,inputs_hash,metadata FROM run_steps WHERE agent='reviewed-plan-import' ORDER BY id LIMIT 1`).Scan(&step, &run, &digest, &receipt); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		d, r, u := digest, receipt, run
		switch i {
		case 0:
			d += "changed"
		case 1:
			r += " "
		case 2:
			u += "changed"
		}
		if err := rel.ImportReviewedPlan(ctx, goals, stories, tasks, step, u, d, r); err == nil {
			t.Fatal("mismatched receipt accepted")
		}
	}
}

// Snapshot every SQL column, including timestamps, approvals, commits and metadata.
func identitySnapshot(t *testing.T, m *Manager, query string, args ...interface{}) string {
	t.Helper()
	rows, err := m.state.GetDB().Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	result := [][]interface{}{}
	for rows.Next() {
		v := make([]interface{}, len(cols))
		ptr := make([]interface{}, len(cols))
		for i := range v {
			ptr[i] = &v[i]
		}
		if err := rows.Scan(ptr...); err != nil {
			t.Fatal(err)
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReviewedIdentityChangedGoalAndTask(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "retained"))
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	req := replayRequest()
	if _, err := e.mgr.Plan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var p planner.ProjectPlan
	if err := json.Unmarshal([]byte(identityFixture(t, "retained")), &p); err != nil {
		t.Fatal(err)
	}
	p.Goals[0].Description = "Same title changed goal"
	p.Goals[0].SuccessCriteria = "Complete US-001 and T-US-001-001"
	p.Stories[0].Tasks[0].Description = "Changed task content alone"
	raw, _ := json.Marshal(p)
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(string(raw))
	req.RequestID += "-goal"
	result, err := e.mgr.Plan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	// The changed restatement is the persisted goal, story and task.
	if result.Plan.Goals[0].ID != "G-001" || result.Plan.Goals[0].Description == "Same title changed goal" ||
		result.Plan.Stories[0].ID != "US-001" || result.Plan.Stories[0].Tasks[0].Description == "Changed task content alone" {
		t.Fatalf("persisted identity renamed or rewritten: %+v", result.Plan)
	}
	// A task-only change does not rename the task either.
	raw, _ = json.Marshal(result.Plan)
	var next planner.ProjectPlan
	if err := json.Unmarshal(raw, &next); err != nil {
		t.Fatal(err)
	}
	next.Stories[0].Tasks[0].TechnicalStrategy = "A new strategy"
	if err := e.mgr.preparePlanIDs(&next); err != nil {
		t.Fatal(err)
	}
	if next.Stories[0].ID != "US-001" || next.Stories[0].Tasks[0].ID != "T-US-001-001" || next.Stories[0].Tasks[0].TechnicalStrategy != "" {
		t.Fatalf("task change renamed or rewrote the persisted task: %+v", next.Stories[0])
	}
	goals, stories, tasks := reviewedPlanRows(&next)
	if err := e.rel.ValidatePlanIdentities(context.Background(), goals, stories, tasks); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedIdentityDuplicateRefusal(t *testing.T) {
	e := newSchedulerTestEnv(t)
	for _, raw := range []string{
		`{"goals":[{"id":"G-001"},{"id":"G-001"}]}`,
		`{"stories":[{"id":"US-001"},{"id":"US-001"}]}`,
		`{"stories":[{"id":"US-001","tasks":[{"id":"T-1"},{"id":"T-1"}]}]}`,
		`{"goals":[{"id":""}]}`,
	} {
		var p planner.ProjectPlan
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			t.Fatal(err)
		}
		if err := e.mgr.preparePlanIDs(&p); err == nil {
			t.Fatal("ambiguous identities accepted")
		}
	}
	if err := uniquePlanIDs(nil); err == nil {
		t.Fatal("nil accepted")
	}
}

func TestReviewedIdentityClosedStoreRefusesPreparation(t *testing.T) {
	e := newSchedulerTestEnv(t)
	e.closeState()
	if err := e.mgr.preparePlanIDs(&planner.ProjectPlan{}); err == nil {
		t.Fatal("closed store accepted")
	}
}

func TestReviewedIdentityPostReviewConflictAtomicRetry(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "retained"))
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	req := replayRequest()
	if _, err := e.mgr.Plan(ctx, req); err != nil {
		t.Fatal(err)
	}
	receipt := identitySnapshot(t, e.mgr, `SELECT * FROM run_steps WHERE agent='reviewed-plan-import' ORDER BY id`)
	stories := identitySnapshot(t, e.mgr, `SELECT * FROM stories ORDER BY id`)
	tasks := identitySnapshot(t, e.mgr, `SELECT * FROM tasks ORDER BY id`)
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(identityFixture(t, "subsequent"))
	e.mgr.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) {
		if _, err := e.mgr.state.GetDB().Exec(`UPDATE goals SET description='concurrent change' WHERE id='G-002'`); err != nil {
			t.Fatal(err)
		}
		return replayReviewFixture, nil
	})
	req.RequestID += "-conflict"
	if _, err := e.mgr.Plan(ctx, req); err == nil || !strings.Contains(err.Error(), "reviewed goals G-002 conflicts with retained content") {
		t.Fatalf("genuine conflict not refused: %v", err)
	}
	if identitySnapshot(t, e.mgr, `SELECT * FROM stories ORDER BY id`) != stories || identitySnapshot(t, e.mgr, `SELECT * FROM tasks ORDER BY id`) != tasks || identitySnapshot(t, e.mgr, `SELECT * FROM run_steps WHERE agent='reviewed-plan-import' ORDER BY id`) != receipt {
		t.Fatal("conflict partially imported or changed receipt")
	}
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE goals SET description='Verify' WHERE id='G-002'`); err != nil {
		t.Fatal(err)
	}
	e.mgr.Close()
	e.closeState()
	fresh := freshQueueManager(t, e)
	fresh.cfg.PlanGenerator = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("retry regenerated"); return "", nil })
	fresh.cfg.PlanReviewer = planCompletionFunc(func(context.Context, string) (string, error) { t.Fatal("retry rereviewed"); return "", nil })
	if _, err := fresh.Plan(ctx, req); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedRowsCanonicalConversion(t *testing.T) {
	for _, strategy := range []string{"", " \t\n", "  preserve spacing\n"} {
		p := &planner.ProjectPlan{Goals: []planner.Goal{{ID: "G-001", Title: "Goal", Description: "description", SuccessCriteria: "success", VerificationMethod: "method"}}, Stories: []planner.Story{
			{ID: "US-001", GoalID: "G-001", Title: "Story", Contract: "contract", Tasks: []planner.Task{{ID: "T-1", Description: "description", TechnicalStrategy: strategy}, {ID: "T-2", Mode: "hitl", DecisionReason: "owner decision", DecisionRef: "accepted-goal"}}},
			{ID: "US-005", Title: "Second"},
		}}
		goals, stories, tasks := reviewedPlanRows(p)
		want := "description"
		if strings.TrimSpace(strategy) != "" {
			want += "\n\nTechnical strategy:\n" + strategy
		}
		if goals[0].SuccessCriteria != "success" || goals[0].VerificationMethod != "method" || stories[0].Contract != "contract" || stories[1].Priority != 1 || stories[0].StoryType != "feature" || !reflect.DeepEqual(stories[0].Tasks, []string{"T-1", "T-2"}) {
			t.Fatal("canonical Goal/story conversion changed")
		}
		if tasks[0].Description != want || tasks[0].MaxAttempts != 3 || tasks[1].Priority != 1 || tasks[0].StoryID != "US-001" || tasks[0].Metadata["mode"] != nil || tasks[1].Metadata["mode"] != "hitl" || tasks[1].Metadata["decision_ref"] != "accepted-goal" {
			t.Fatalf("canonical task conversion changed: %+v %+v", tasks[0], tasks[1])
		}
	}
}
