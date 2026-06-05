package planner

import "testing"

func noExisting() ExistingLookup {
	return ExistingLookup{
		GoalTitle:  func(string) (string, bool) { return "", false },
		StoryTitle: func(string) (string, bool) { return "", false },
		TaskExists: func(string) bool { return false },
	}
}

func TestRemapPlanIDs_FreshProjectUntouched(t *testing.T) {
	plan := &ProjectPlan{
		Goals: []Goal{{ID: "G-001", Title: "Goal"}},
		Stories: []Story{{
			ID: "US-001", Title: "Story", GoalID: "G-001",
			Tasks: []Task{{ID: "T-US-001-001", Title: "Task"}},
		}},
	}
	if n := RemapPlanIDs(plan, noExisting()); n != 0 {
		t.Fatalf("fresh project: expected 0 remapped, got %d", n)
	}
	if plan.Stories[0].ID != "US-001" || plan.Stories[0].Tasks[0].ID != "T-US-001-001" {
		t.Fatal("fresh project IDs must be untouched")
	}
}

func TestRemapPlanIDs_IdempotentReimport(t *testing.T) {
	// Same ID AND same title = same item: leave alone (importer skips it).
	plan := &ProjectPlan{
		Stories: []Story{{ID: "US-001", Title: "Existing story"}},
	}
	look := noExisting()
	look.StoryTitle = func(id string) (string, bool) {
		if id == "US-001" {
			return "Existing story", true
		}
		return "", false
	}
	if n := RemapPlanIDs(plan, look); n != 0 {
		t.Fatalf("identical re-import: expected 0 remapped, got %d", n)
	}
	if plan.Stories[0].ID != "US-001" {
		t.Fatal("identical story must keep its ID")
	}
}

func TestRemapPlanIDs_ReplanAppendsToBacklog(t *testing.T) {
	// Existing backlog: G-001, US-001..US-002 (different titles), tasks taken.
	existingStories := map[string]string{
		"US-001": "Old: initial build story",
		"US-002": "Old: another done story",
	}
	existingGoals := map[string]string{"G-001": "Old goal"}
	existingTasks := map[string]bool{"T-US-001-001": true, "T-US-002-001": true}

	look := ExistingLookup{
		GoalTitle: func(id string) (string, bool) {
			t, ok := existingGoals[id]
			return t, ok
		},
		StoryTitle: func(id string) (string, bool) {
			t, ok := existingStories[id]
			return t, ok
		},
		TaskExists: func(id string) bool { return existingTasks[id] },
	}

	// Incoming refactor plan reuses the low IDs with new content.
	plan := &ProjectPlan{
		Goals: []Goal{{ID: "G-001", Title: "Refactor to plugin architecture"}},
		Stories: []Story{
			{
				ID: "US-001", Title: "Extract plugin interfaces", GoalID: "G-001",
				Tasks: []Task{
					{ID: "T-US-001-001", Title: "Define interfaces"},
					{ID: "T-US-001-002", Title: "Migrate first module", DependsOn: []string{"T-US-001-001"}},
				},
			},
			{
				ID: "US-002", Title: "Port remaining modules", GoalID: "G-001",
				DependsOn: []string{"US-001"},
				Tasks:     []Task{{ID: "T-US-002-001", Title: "Port"}},
			},
		},
	}

	n := RemapPlanIDs(plan, look)
	if n == 0 {
		t.Fatal("replan with colliding IDs must remap")
	}

	// Goal moved off the taken ID; stories reference the new goal ID.
	if plan.Goals[0].ID == "G-001" {
		t.Fatal("colliding goal must be remapped")
	}
	if plan.Stories[0].GoalID != plan.Goals[0].ID {
		t.Fatalf("story goal_id %q must follow remapped goal %q", plan.Stories[0].GoalID, plan.Goals[0].ID)
	}

	// Stories moved to free IDs (US-003+) and cross-references follow.
	s1, s2 := plan.Stories[0], plan.Stories[1]
	if s1.ID == "US-001" || s2.ID == "US-002" {
		t.Fatalf("colliding stories must be remapped, got %s/%s", s1.ID, s2.ID)
	}
	if s2.DependsOn[0] != s1.ID {
		t.Fatalf("story dependency must follow remap: %q != %q", s2.DependsOn[0], s1.ID)
	}

	// Task IDs follow their story; intra-story dependencies follow tasks.
	if s1.Tasks[0].ID == "T-US-001-001" {
		t.Fatal("task ID must follow its remapped story")
	}
	if s1.Tasks[1].DependsOn[0] != s1.Tasks[0].ID {
		t.Fatalf("task dependency must follow remap: %q != %q", s1.Tasks[1].DependsOn[0], s1.Tasks[0].ID)
	}

	// No remapped ID collides with the existing backlog.
	if _, exists := existingStories[s1.ID]; exists {
		t.Fatalf("remapped story ID %s still collides", s1.ID)
	}
	if existingTasks[s1.Tasks[0].ID] {
		t.Fatalf("remapped task ID %s still collides", s1.Tasks[0].ID)
	}
}

func TestNextFreeTaskID(t *testing.T) {
	taken := map[string]bool{"T-US-003-001": true, "T-US-003-002": true}
	got := nextFreeTaskID("T-US-003-001", func(id string) bool { return taken[id] })
	if got != "T-US-003-003" {
		t.Fatalf("expected T-US-003-003, got %s", got)
	}
}
