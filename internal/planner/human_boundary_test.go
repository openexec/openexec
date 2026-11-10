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

func TestHumanBoundaryRefinementMayCorrectTheBoundaryStrategy(t *testing.T) {
	// Review refused a retained acceptance task's strategy; the refinement
	// that corrects it must reach review instead of the original wording.
	original, next := boundaryPlan(), boundaryPlan()
	original.Stories[0].Tasks[1].TechnicalStrategy = "Use safe_commit for preparatory changes."
	next.Stories[0].Tasks[1].TechnicalStrategy = "Consume the owner's decision; make no commits."
	next.Stories[0].Tasks[1].Description = "Reworded"
	raw, _ := json.Marshal(next)
	got, err := New(&mockProvider{response: string(raw)}).RefinePlan(context.Background(), "intent", original, &PlanReview{Assessment: "remove safe_commit from the acceptance task"})
	if err != nil {
		t.Fatal(err)
	}
	task := got.Stories[0].Tasks[1]
	if task.TechnicalStrategy != "Consume the owner's decision; make no commits." {
		t.Fatalf("corrected strategy replaced by the refused one: %q", task.TechnicalStrategy)
	}
	if task.Description != original.Stories[0].Tasks[1].Description || task.DecisionReason != original.Stories[0].Tasks[1].DecisionReason {
		t.Fatalf("owner-facing boundary not restored: %+v", task)
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

func TestHumanBoundaryRuleForbidsReviewDeletingRetainedHumanTasks(t *testing.T) {
	for _, prompt := range []string{StoryReviewPrompt, StoryFixPrompt} {
		if !strings.Contains(prompt, "never to delete it") || !strings.Contains(prompt, "refinement keeps every retained HITL task") {
			t.Fatal("prompt lacks the no-deletion instruction for retained human tasks")
		}
	}
}

// Goal 09a204a4, run f22b8c26: human tasks moved to US-016 while the automatic
// neighbour stayed in US-009; the guard reported the neighbour removed.
func TestHumanBoundaryRefinementMayMoveHumanTasksOutOfASplitStory(t *testing.T) {
	const r3, r4 = "Owner runs the user study", "Owner records the study result"
	original := func() *ProjectPlan {
		return &ProjectPlan{Stories: []Story{
			{ID: "US-009", Title: "Study", Tasks: []Task{
				{ID: "T-US-009-001", Mode: TaskModeAFK},
				{ID: "T-US-009-002", Mode: TaskModeAFK, DependsOn: []string{"T-US-009-001", "T-US-009-003"}},
				{ID: "T-US-009-003", Mode: TaskModeHITL, DecisionReason: r3, DecisionRef: "decision:study", Description: "Run study", DependsOn: []string{"T-US-009-001"}},
				{ID: "T-US-009-004", Mode: TaskModeHITL, DecisionReason: r4, Description: "Record result", DependsOn: []string{"T-US-009-003"}},
			}},
			{ID: "US-013", Title: "Other", Tasks: []Task{
				{ID: "T-US-013-001", Mode: TaskModeAFK},
				{ID: "T-US-013-004", Mode: TaskModeHITL, DecisionReason: "Owner accepts", DependsOn: []string{"T-US-013-001"}},
			}},
		}}
	}
	split := func() *ProjectPlan {
		p := original()
		hum := append([]Task(nil), p.Stories[0].Tasks[2:]...)
		p.Stories[0].Tasks = p.Stories[0].Tasks[:2]
		p.Stories = append(p.Stories, Story{ID: "US-016", Title: "Human study", DependsOn: []string{"US-009"}, Tasks: hum})
		return p
	}
	refine := func(next *ProjectPlan) (*ProjectPlan, error) {
		raw, _ := json.Marshal(next)
		return New(&mockProvider{response: string(raw)}).RefinePlan(context.Background(), "intent", original(), &PlanReview{Assessment: "split the human work"})
	}

	next := split()
	// Reworded / detached metadata must be restored on the moved tasks.
	h := &next.Stories[2].Tasks[0]
	h.Description, h.DecisionRef = "x", ""
	next.Stories[2].Tasks[1].DependsOn = nil
	next.Stories[0].Tasks[1].DependsOn = []string{"T-US-009-001"}
	got, err := refine(next)
	if err != nil {
		t.Fatalf("split refused: %v", err)
	}
	if len(got.Stories[0].Tasks) != 2 || got.Stories[0].Tasks[1].ID != "T-US-009-002" {
		t.Fatalf("automatic neighbour not retained in US-009: %+v", got.Stories[0].Tasks)
	}
	if !reflect.DeepEqual(got.Stories[0].Tasks[1].DependsOn, []string{"T-US-009-001", "T-US-009-003"}) {
		t.Fatalf("neighbour dependency on human task lost: %v", got.Stories[0].Tasks[1].DependsOn)
	}
	want := original().Stories[0].Tasks[2:]
	if !reflect.DeepEqual(got.Stories[2].Tasks, want) {
		t.Fatalf("moved human tasks not restored:\n got %+v\nwant %+v", got.Stories[2].Tasks, want)
	}

	// Near miss: a new story tied only through a task edge is still a split.
	tied := split()
	tied.Stories[2].DependsOn = nil
	if _, err := refine(tied); err != nil {
		t.Fatalf("split tied by a task edge refused: %v", err)
	}

	// Real deletions and unsafe changes stay refused.
	cases := map[string]func(p *ProjectPlan){
		"delete human task":          func(p *ProjectPlan) { p.Stories[2].Tasks = p.Stories[2].Tasks[:1] },
		"delete automatic neighbour": func(p *ProjectPlan) { p.Stories[0].Tasks = p.Stories[0].Tasks[:1] },
		"delete T-US-013-004":        func(p *ProjectPlan) { p.Stories[1].Tasks = p.Stories[1].Tasks[:1] },
		"remove story with retained": func(p *ProjectPlan) { p.Stories = p.Stories[1:] },
		"ambiguous duplicate id": func(p *ProjectPlan) {
			p.Stories[0].Tasks = append(p.Stories[0].Tasks, p.Stories[2].Tasks[0])
		},
		"duplicate id inside one story": func(p *ProjectPlan) {
			p.Stories[2].Tasks = append(p.Stories[2].Tasks, p.Stories[2].Tasks[0])
		},
		"moved into unrelated existing story": func(p *ProjectPlan) {
			p.Stories[1].Tasks = append(p.Stories[1].Tasks, p.Stories[2].Tasks[1])
			p.Stories[2].Tasks = p.Stories[2].Tasks[:1]
		},
		"moved into unrelated new story": func(p *ProjectPlan) {
			p.Stories[2].DependsOn = nil
			p.Stories[2].ID = "US-017"
			p.Stories[2].Tasks[0].DependsOn = nil
			p.Stories[2].Tasks[1].DependsOn = nil
		},
		"changed identity with two candidates": func(p *ProjectPlan) {
			a, b := p.Stories[2].Tasks[0], p.Stories[2].Tasks[0]
			a.ID, b.ID = "T-US-016-001", "T-US-016-002"
			p.Stories[2].Tasks = []Task{a, b, p.Stories[2].Tasks[1]}
		},
		"changed identity": func(p *ProjectPlan) {
			p.Stories[2].Tasks[0].ID, p.Stories[2].Tasks[0].DecisionReason = "T-US-016-001", "Something else"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			p := split()
			mutate(p)
			if got, err := refine(p); err == nil {
				t.Fatalf("accepted: %+v", got)
			}
		})
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

func TestRepositoryScopeRuleSharedAcrossPlannerPrompts(t *testing.T) {
	// Without it a single-repository Goal is planned a task in another
	// repository, which no stage in this candidate can do or verify.
	for name, prompt := range map[string]string{"generation": StoryGenerationPrompt, "compact": CompactStoryGenerationPrompt, "review": StoryReviewPrompt, "fix": StoryFixPrompt} {
		if !strings.Contains(prompt, RepositoryScopeRule) {
			t.Fatalf("%s prompt lacks the repository scope rule", name)
		}
	}
}

func TestDeliveryEvidenceRuleSharedAcrossPlannerPrompts(t *testing.T) {
	// Without it the planner makes tasks finish on evidence only the
	// delivered, running candidate can produce, and review refuses every
	// fresh plan (openexec f5cae74f, run ff3c0c5e).
	for name, prompt := range map[string]string{"generation": StoryGenerationPrompt, "compact": CompactStoryGenerationPrompt, "review": StoryReviewPrompt, "fix": StoryFixPrompt} {
		if !strings.Contains(prompt, DeliveryEvidenceRule) {
			t.Fatalf("%s prompt lacks the delivery-ordered evidence rule", name)
		}
	}
	for _, want := range []string{"never a task's acceptance criterion", "Goal review after delivery", "Do not reject a plan for leaving"} {
		if !strings.Contains(DeliveryEvidenceRule, want) {
			t.Fatalf("delivery evidence rule lost %q", want)
		}
	}
}
