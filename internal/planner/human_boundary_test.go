package planner

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func boundaryPlan() *ProjectPlan {
	return &ProjectPlan{Stories: []Story{{ID: "S", Title: "Deliver", Tasks: []Task{
		{ID: "prepare", Mode: TaskModeAFK},
		{ID: "decide", Mode: TaskModeHITL, DecisionReason: "Owner must grant production access", DecisionRef: "decision:access", DependsOn: []string{"prepare"}},
		{ID: "apply", Mode: TaskModeAFK, DependsOn: []string{"decide"}},
		{ID: "independent", Mode: TaskModeAFK},
	}}}}
}

func TestHumanBoundaryCompactionKeepsPreparationAndDependencies(t *testing.T) {
	plan := boundaryPlan()
	before, _ := json.Marshal(plan)
	EnforceFastTrack(plan, "surgical", "greenfield")
	after, _ := json.Marshal(plan)
	if string(before) != string(after) {
		t.Fatal("compaction changed boundary or independent preparation")
	}
}

func TestHumanBoundaryReviewRequiresConcreteReason(t *testing.T) {
	for _, reason := range []string{"", "   ", "Owner must choose the data retention period"} {
		plan := boundaryPlan()
		plan.Stories[0].Tasks[1].DecisionReason = reason
		review, err := New(&mockProvider{response: `{"approved":true,"assessment":"reviewed"}`}).ReviewPlan(context.Background(), "intent", plan)
		if err != nil {
			t.Fatal(err)
		}
		if review.Approved != (strings.TrimSpace(reason) != "") {
			t.Fatalf("unjustified approval: %+v", review)
		}
	}
}

func TestHumanBoundaryRefinementCannotRemoveOrAuthorizeRetainedWork(t *testing.T) {
	// Rewording or detaching a retained boundary is undone; removing retained
	// work is refused. Either way the owner is asked exactly what was planned.
	for _, change := range []string{"mode", "delete", "edge", "reference", "action", "unchanged", "legacy", "independent"} {
		t.Run(change, func(t *testing.T) {
			original, next := boundaryPlan(), boundaryPlan()
			switch change {
			case "mode":
				next.Stories[0].Tasks[1].Mode = TaskModeAFK
			case "delete":
				next.Stories[0].Tasks = next.Stories[0].Tasks[2:]
			case "edge":
				next.Stories[0].Tasks[2].DependsOn = nil
			case "reference":
				next.Stories[0].Tasks[1].DecisionRef = "unrelated"
			case "action":
				next.Stories[0].Tasks[1].Description = "Perform a different effect"
			case "independent":
				next.Stories[0].Tasks = next.Stories[0].Tasks[:3]
			case "legacy":
				original.Stories[0].Tasks[1].DecisionReason = ""
				original.Stories[0].Tasks[1].DecisionRef = ""
				next.Stories[0].Tasks[1] = original.Stories[0].Tasks[1]
			}
			raw, _ := json.Marshal(next)
			got, err := New(&mockProvider{response: string(raw)}).RefinePlan(context.Background(), "intent", original, &PlanReview{Assessment: "improve verification"})
			if change == "delete" {
				if err == nil {
					t.Fatalf("refinement removed retained work: %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tasks := got.Stories[0].Tasks
			if !reflect.DeepEqual(tasks[1], original.Stories[0].Tasks[1]) {
				t.Fatalf("boundary not restored: %+v", tasks[1])
			}
			if !reflect.DeepEqual(tasks[2].DependsOn, []string{"decide"}) {
				t.Fatalf("edge not restored: %v", tasks[2].DependsOn)
			}
		})
	}
}

func TestHumanBoundaryMetadataRoundTrip(t *testing.T) {
	task := boundaryPlan().Stories[0].Tasks[1]
	raw, _ := json.Marshal(task)
	var restored Task
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	want := map[string]interface{}{"mode": "hitl", "decision_reason": task.DecisionReason, "decision_ref": task.DecisionRef}
	if !reflect.DeepEqual(restored.ExecutionMetadata(), want) {
		t.Fatal("decision metadata lost")
	}
}

func TestHumanBoundaryRuleSharedAcrossPlannerPrompts(t *testing.T) {
	for _, prompt := range []string{StoryGenerationPrompt, CompactStoryGenerationPrompt, StoryReviewPrompt, StoryFixPrompt} {
		if !strings.Contains(prompt, HumanBoundaryRule) {
			t.Fatal("planner path lacks shared rule")
		}
	}
}

func TestHumanBoundaryRefinementMayMergeWorkBeyondTheBoundary(t *testing.T) {
	// Owner acceptance at the end of a plan is connected to nearly every task.
	// A review asking to merge an implementation task with its verification
	// must be satisfiable; only the boundary and its direct neighbours stay.
	chain := func() *ProjectPlan {
		return &ProjectPlan{Stories: []Story{{ID: "S", Title: "Deliver", Tasks: []Task{
			{ID: "implement", Mode: TaskModeAFK},
			{ID: "verify", Mode: TaskModeAFK, DependsOn: []string{"implement"}},
			{ID: "integrate", Mode: TaskModeAFK, DependsOn: []string{"verify"}},
			{ID: "accept", Mode: TaskModeHITL, DecisionReason: "Owner accepts the delivered settings", DependsOn: []string{"integrate"}},
		}}}}
	}
	original, next := chain(), chain()
	next.Stories[0].Tasks = []Task{
		{ID: "implement", Mode: TaskModeAFK},
		{ID: "integrate", Mode: TaskModeAFK, DependsOn: []string{"implement"}},
		original.Stories[0].Tasks[3],
	}
	raw, _ := json.Marshal(next)
	got, err := New(&mockProvider{response: string(raw)}).RefinePlan(context.Background(), "intent", original, &PlanReview{Assessment: "merge implementation and verification"})
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("refined plan invalid: %v", err)
	}
	if deps := got.Stories[0].Tasks[1].DependsOn; !reflect.DeepEqual(deps, []string{"implement"}) {
		t.Fatalf("edge to merged-away work restored: %v", deps)
	}

	// Removing the work the owner decides on is still refused.
	next = chain()
	next.Stories[0].Tasks = []Task{next.Stories[0].Tasks[0], next.Stories[0].Tasks[1], {ID: "accept", Mode: TaskModeHITL, DecisionReason: "Owner accepts the delivered settings", DependsOn: []string{"verify"}}}
	raw, _ = json.Marshal(next)
	if got, err := New(&mockProvider{response: string(raw)}).RefinePlan(context.Background(), "intent", chain(), &PlanReview{Assessment: "drop integration"}); err == nil {
		t.Fatalf("refinement removed the boundary's direct dependency: %+v", got)
	}
}
