package planner

import (
	"reflect"
	"testing"
)

func boundaryLedger() RetainedLedger {
	story := Story{ID: "US-008", GoalID: "G-005", Title: "Goal Validation", DependsOn: []string{"US-007"}, Tasks: []Task{
		{ID: "T-US-008-001", Title: "Run make check"},
		{ID: "T-US-008-002", Title: "Owner accepts merging", Mode: TaskModeHITL, DecisionReason: "The owner merges", DependsOn: []string{"T-US-008-001"}},
	}}
	ids := map[string]bool{"G-005": true, "US-007": true, "US-008": true, "T-US-007-001": true, "T-US-008-001": true, "T-US-008-002": true}
	return RetainedLedger{
		Goal:  func(id string) (Goal, bool) { return Goal{ID: id, Title: "Observers"}, id == "G-005" },
		Story: func(id string) (Story, bool) { return story, id == "US-008" },
		TaskStory: func(id string) (string, bool) {
			if len(id) > 8 && ids[id] {
				return id[2:8], true
			}
			return "", false
		},
		OpenBoundary: func(id string) bool { return id == "US-008" },
		Taken:        func(id string) bool { return ids[id] },
		Conflicts: func(p *ProjectPlan) map[string]bool {
			c := map[string]bool{}
			for _, s := range p.Stories {
				for _, t := range s.Tasks {
					c[t.ID] = ids[t.ID]
				}
			}
			return c
		},
	}
}

// The 07:09 re-plan put the boundary alone into a new story of its own; the
// story is that boundary's story, and references to it follow.
func TestContinueRetainedWorkRegroupedBoundary(t *testing.T) {
	plan := &ProjectPlan{Stories: []Story{
		{ID: "US-014", Title: "New work", Tasks: []Task{{ID: "T-US-014-001", Title: "Build"}}},
		{ID: "US-017", Title: "Owner acceptance", DependsOn: []string{"US-014"}, Tasks: []Task{
			{ID: "T-US-008-002", Title: "Owner decides", Mode: TaskModeHITL, DecisionReason: "Reworded"},
		}},
		{ID: "US-018", Title: "After acceptance", DependsOn: []string{"US-017"}, Tasks: []Task{{ID: "T-US-018-001", DependsOn: []string{"T-US-008-002"}}}},
	}}
	got := ContinueRetainedWork(plan, boundaryLedger())
	if !reflect.DeepEqual(got, []string{"T-US-008-001", "T-US-008-002", "US-008"}) {
		t.Fatalf("continues = %v", got)
	}
	ids := []string{}
	for _, s := range plan.Stories {
		ids = append(ids, s.ID)
	}
	if !reflect.DeepEqual(ids, []string{"US-014", "US-008", "US-018"}) {
		t.Fatalf("stories = %v", ids)
	}
	if b := plan.Stories[1].Tasks[1]; b.ID != "T-US-008-002" || b.DecisionReason != "The owner merges" {
		t.Fatalf("boundary = %+v", b)
	}
	if !reflect.DeepEqual(plan.Stories[2].DependsOn, []string{"US-008"}) || !reflect.DeepEqual(plan.Stories[2].Tasks[0].DependsOn, []string{"T-US-008-002"}) {
		t.Fatalf("references did not follow the boundary: %+v", plan.Stories[2])
	}
	if !reflect.DeepEqual(plan.Stories[1].DependsOn, []string{"US-007"}) {
		t.Fatalf("persisted story rewritten: %+v", plan.Stories[1])
	}
}

// Without an open boundary nothing is continued: a changed re-listing of
// automatic work keeps moving to new IDs.
func TestContinueRetainedWorkLeavesAutomaticWork(t *testing.T) {
	ledger := boundaryLedger()
	ledger.OpenBoundary = func(string) bool { return false }
	plan := &ProjectPlan{Stories: []Story{{ID: "US-008", Title: "Revised", Tasks: []Task{{ID: "T-US-008-001", Title: "Revised check"}}}}}
	if got := ContinueRetainedWork(plan, ledger); got != nil || plan.Stories[0].Title != "Revised" {
		t.Fatalf("continued %v: %+v", got, plan.Stories[0])
	}
}
