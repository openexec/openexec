package manager

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"

	"github.com/openexec/openexec/internal/planner"
)

func continuePlanJSON(t *testing.T, plan planner.ProjectPlan) string {
	t.Helper()
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// A Goal's re-plan listed its pending owner boundary again in its own words.
// The remapper took the reworded story for new work and renamed the boundary
// (T-US-008-002 → T-US-017-002 → T-US-022-002); plan review refused every
// rename and the Goal re-planned eight times without converging.
func TestReplanNeverRenamesPersistedWork(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	first := planner.ProjectPlan{
		Goals: []planner.Goal{{ID: "G-001", Title: "Observers", Description: "Observe V1-V6"}},
		Stories: []planner.Story{
			{ID: "US-001", GoalID: "G-001", Title: "Map observer inputs", Tasks: []planner.Task{
				{ID: "T-US-001-001", Title: "Map inputs", Description: "Map them", VerificationScript: "true"},
			}},
			{ID: "US-002", GoalID: "G-001", Title: "Goal Validation: full check, owner acceptance", DependsOn: []string{"US-001"}, Tasks: []planner.Task{
				{ID: "T-US-002-001", Title: "Run make check", Description: "Check it", VerificationScript: "make check"},
				{ID: "T-US-002-002", Title: "Owner accepts merging the observer repairs", Description: "Ask the owner", Mode: planner.TaskModeHITL, DecisionReason: "Merging needs the owner", DependsOn: []string{"T-US-002-001"}},
			}},
		},
	}
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(continuePlanJSON(t, first))
	e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
	req := replayRequest()
	if _, err := e.mgr.Plan(ctx, req); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE tasks SET status='done' WHERE id IN ('T-US-001-001','T-US-002-001')`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.state.GetDB().Exec(`UPDATE stories SET status='done' WHERE id='US-001'`); err != nil {
		t.Fatal(err)
	}
	retained := identitySnapshot(t, e.mgr, `SELECT * FROM tasks WHERE id IN ('T-US-002-001','T-US-002-002') ORDER BY id`)
	retainedStory := identitySnapshot(t, e.mgr, `SELECT * FROM stories WHERE id='US-002'`)

	// The re-plan: new work, the finished story restated, and the pending
	// boundary re-listed under its own IDs with new wording, a new edge and a
	// new task in its story.
	second := planner.ProjectPlan{
		Goals: []planner.Goal{{ID: "G-001", Title: "Observers, closed loop", Description: "Observe V1-V6 in production"}},
		Stories: []planner.Story{
			{ID: "US-001", GoalID: "G-001", Title: "Map observer inputs again", Tasks: []planner.Task{
				{ID: "T-US-001-001", Title: "Map production inputs", Description: "Map them again", VerificationScript: "true"},
			}},
			{ID: "US-003", GoalID: "G-001", Title: "V2 counts a gap verdict", Tasks: []planner.Task{
				{ID: "T-US-003-001", Title: "Count gap verdicts", Description: "Count them in US-001's map", VerificationScript: "true"},
			}},
			{ID: "US-002", GoalID: "G-001", Title: "Owner acceptance of the candidate", DependsOn: []string{"US-003"}, Tasks: []planner.Task{
				{ID: "T-US-002-002", Title: "Owner decides on the exact candidate", Description: "Ask", Mode: planner.TaskModeHITL, DecisionReason: "The owner merges", DependsOn: []string{"T-US-002-001", "T-US-003-001"}},
				{ID: "T-US-002-003", Title: "Record the decision", Description: "After T-US-002-002", VerificationScript: "true", DependsOn: []string{"T-US-002-002"}},
			}},
		},
	}
	e.mgr.cfg.PlanGenerator = fixedPlanCompletion(continuePlanJSON(t, second))
	next := req
	next.RequestID += "-replan"
	result, err := e.mgr.Plan(ctx, next)
	if err != nil {
		t.Fatal(err)
	}
	p := result.Plan
	byID := map[string]planner.Story{}
	hitl := []string{}
	for _, s := range p.Stories {
		byID[s.ID] = s
		for _, task := range s.Tasks {
			if task.Mode == planner.TaskModeHITL {
				hitl = append(hitl, task.ID)
			}
		}
	}
	kept, ok := byID["US-002"]
	if !ok || kept.Title != "Goal Validation: full check, owner acceptance" || len(kept.Tasks) != 2 {
		t.Fatalf("pending story not continued as persisted: %+v", kept)
	}
	if !reflect.DeepEqual(hitl, []string{"T-US-002-002"}) || kept.Tasks[1].DecisionReason != "Merging needs the owner" {
		t.Fatalf("owner boundary renamed or reworded: %v %+v", hitl, kept.Tasks[1])
	}
	if !reflect.DeepEqual(p.Continues, []string{"G-001", "T-US-001-001", "T-US-002-001", "T-US-002-002", "US-001", "US-002"}) {
		t.Fatalf("continues = %v", p.Continues)
	}
	// The new task follows the boundary in a story of its own.
	var cont planner.Story
	for _, s := range p.Stories {
		if slices.Contains(s.DependsOn, "US-002") {
			cont = s
		}
	}
	if cont.ID == "" || cont.ID == "US-002" || len(cont.Tasks) != 1 || cont.Tasks[0].ID != "T-"+cont.ID+"-001" || cont.Tasks[0].Title != "Record the decision" {
		t.Fatalf("new task not moved to a continuation story: %+v", cont)
	}
	if !reflect.DeepEqual(cont.Tasks[0].DependsOn, []string{"T-US-002-002"}) {
		t.Fatalf("continuation lost its edge to the retained boundary: %v", cont.Tasks[0].DependsOn)
	}
	// Finished work restated in new words is the finished work: never renamed,
	// never re-created as a pending duplicate.
	if s, ok := byID["US-001"]; !ok || s.Title != "Map observer inputs" || s.Tasks[0].ID != "T-US-001-001" {
		t.Fatalf("finished story renamed or rewritten: %+v", s)
	}
	if identitySnapshot(t, e.mgr, `SELECT * FROM tasks WHERE id IN ('T-US-002-001','T-US-002-002') ORDER BY id`) != retained ||
		identitySnapshot(t, e.mgr, `SELECT * FROM stories WHERE id='US-002'`) != retainedStory {
		t.Fatal("retained rows rewritten")
	}
	var n int
	if err := e.mgr.state.GetDB().QueryRow(`SELECT COUNT(*) FROM tasks WHERE id=?`, cont.Tasks[0].ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("continuation task not imported: %v", err)
	}
	// Preparing the allocated plan again changes nothing (reviewed import
	// refuses a plan that moves after review).
	again := *p
	raw, _ := json.Marshal(p)
	if err := json.Unmarshal(raw, &again); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.preparePlanIDs(&again); err != nil {
		t.Fatal(err)
	}
	// Rows that now match their persisted content exactly need no listing
	// in Continues; every ID and all content stay as reviewed.
	want := *p
	want.Continues, again.Continues = nil, nil
	if !reflect.DeepEqual(&again, &want) {
		t.Fatal("continued plan moved on preparation")
	}
}
