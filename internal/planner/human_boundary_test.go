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

func TestHumanBoundaryRefinementMayRenumberTheBoundaryStory(t *testing.T) {
	// Review c7e97959 on fotoyks: the repair plan reused story ids already
	// committed on the branch, review asked for fresh ids, and refinement
	// returned owner acceptance unchanged as US-019. That is not removal.
	plan := func(story string, reason string) *ProjectPlan {
		id := func(n string) string { return "T-" + story + "-" + n }
		return &ProjectPlan{Stories: []Story{
			{ID: "US-010", Title: "Build", Tasks: []Task{{ID: "T-US-010-001", Mode: TaskModeAFK}}},
			{ID: story, Title: "Goal Validation", DependsOn: []string{"US-010"}, Tasks: []Task{
				{ID: id("001"), Mode: TaskModeAFK},
				{ID: id("002"), Mode: TaskModeAFK, DependsOn: []string{id("001")}},
				{ID: id("003"), Mode: TaskModeHITL, DecisionReason: reason, Description: "Owner accepts", DependsOn: []string{id("001"), id("002")}},
			}},
		}}
	}
	const reason = "Accepting the result is the owner's decision"
	refine := func(next *ProjectPlan) (*ProjectPlan, error) {
		raw, _ := json.Marshal(next)
		return New(&mockProvider{response: string(raw)}).RefinePlan(context.Background(), "intent", plan("US-009", reason), &PlanReview{Assessment: "story ids collide with earlier commits"})
	}

	next := plan("US-019", reason)
	next.Stories[1].Tasks[2].Description = "Reworded"
	next.Stories[1].Tasks[2].DependsOn = []string{"T-US-019-002"}
	got, err := refine(next)
	if err != nil {
		t.Fatal(err)
	}
	accept := got.Stories[1].Tasks[2]
	if accept.ID != "T-US-019-003" || accept.Description != "Owner accepts" {
		t.Fatalf("renumbered boundary not kept as planned: %+v", accept)
	}
	if !reflect.DeepEqual(accept.DependsOn, []string{"T-US-019-002", "T-US-019-001"}) {
		t.Fatalf("edge not restored under the new ids: %v", accept.DependsOn)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("refined plan invalid: %v", err)
	}

	// A renumbered story whose decision changed is a different boundary.
	if got, err := refine(plan("US-019", "Owner approves a production release")); err == nil {
		t.Fatalf("changed decision accepted as a renumbering: %+v", got)
	}
	// Two candidates for the same decision is a guess, not a renumbering.
	twice := plan("US-019", reason)
	twice.Stories[0].Tasks = append(twice.Stories[0].Tasks, Task{ID: "T-US-010-002", Mode: TaskModeHITL, DecisionReason: reason})
	if got, err := refine(twice); err == nil {
		t.Fatalf("ambiguous renumbering accepted: %+v", got)
	}
	// Goal 10a38d05 on fotoyks, after the fix above: refinement renumbered the
	// story and its preparation but kept the boundary's own id, T-US-009-003,
	// inside US-019.
	kept := plan("US-019", reason)
	kept.Stories[1].Tasks[2].ID = "T-US-009-003"
	got, err = refine(kept)
	if err != nil {
		t.Fatalf("boundary kept under its own id in a renumbered story refused: %v", err)
	}
	accept = got.Stories[1].Tasks[2]
	if accept.ID != "T-US-009-003" || accept.Description != "Owner accepts" || !reflect.DeepEqual(accept.DependsOn, []string{"T-US-019-001", "T-US-019-002"}) {
		t.Fatalf("moved boundary not kept as planned: %+v", accept)
	}
	// Moving it is still not dropping the work it decides on.
	keptDropped := plan("US-019", reason)
	keptDropped.Stories[1].Tasks = keptDropped.Stories[1].Tasks[2:]
	keptDropped.Stories[1].Tasks[0].ID, keptDropped.Stories[1].Tasks[0].DependsOn = "T-US-009-003", nil
	if got, err := refine(keptDropped); err == nil {
		t.Fatalf("moved boundary dropped its dependencies: %+v", got)
	}
	// Renumbering must not drop the work the owner decides on.
	dropped := plan("US-019", reason)
	dropped.Stories[1].Tasks = dropped.Stories[1].Tasks[1:]
	dropped.Stories[1].Tasks[0].DependsOn = nil
	if got, err := refine(dropped); err == nil {
		t.Fatalf("renumbering dropped the boundary's dependency: %+v", got)
	}
}

func TestRequirementIdentityRuleSharedAcrossPlannerPrompts(t *testing.T) {
	// Without it the planner invents REQ aliases for the intent's condition
	// ids and the reviewer rejects every refinement for a mapping the story
	// format cannot carry.
	for name, prompt := range map[string]string{"generation": StoryGenerationPrompt, "review": StoryReviewPrompt, "fix": StoryFixPrompt} {
		if !strings.Contains(prompt, RequirementIdentityRule) {
			t.Fatalf("%s prompt lacks the requirement identity rule", name)
		}
	}
	if strings.Contains(StoryReviewPrompt, "Each REQ-XXX in the intent") {
		t.Fatal("review still requires REQ-XXX identifiers the intent may not use")
	}
}
