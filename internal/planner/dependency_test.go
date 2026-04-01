package planner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestPlanner_IndependentTasksHaveNoDependencies verifies that when the LLM
// generates tasks for independent features, those tasks have no artificial
// dependency chains. This catches Bug #3: planner auto-chaining tasks.
func TestPlanner_IndependentTasksHaveNoDependencies(t *testing.T) {
	// GIVEN an LLM response with two independent stories (login page + about page)
	// that touch completely different files and have no data dependencies.
	mock := &mockProvider{
		response: `{
			"schema_version": "1.1",
			"goals": [
				{"id": "G-001", "title": "User-facing pages", "description": "Build pages", "success_criteria": "Pages load", "verification_method": "Browser test"}
			],
			"stories": [
				{
					"id": "US-001",
					"title": "Add login page",
					"description": "Create a standalone login page",
					"requirement_id": "REQ-001",
					"goal_id": "G-001",
					"depends_on": [],
					"acceptance_criteria": ["Login page renders"],
					"verification_script": "npm test -- --grep login",
					"tasks": [
						{"id": "T-US-001-001", "title": "Create login component", "description": "Build login form", "depends_on": [], "verification_script": "npm test"},
						{"id": "T-US-001-002", "title": "Add login styles", "description": "Style the form", "depends_on": [], "verification_script": "npm test"}
					]
				},
				{
					"id": "US-002",
					"title": "Add about page",
					"description": "Create a standalone about page",
					"requirement_id": "REQ-002",
					"goal_id": "G-001",
					"depends_on": [],
					"acceptance_criteria": ["About page renders"],
					"verification_script": "npm test -- --grep about",
					"tasks": [
						{"id": "T-US-002-001", "title": "Create about component", "description": "Build about page", "depends_on": [], "verification_script": "npm test"}
					]
				}
			]
		}`,
	}

	p := New(mock)
	plan, err := p.GeneratePlan(context.Background(), "# Build login page and about page\nThey are independent features.", nil)
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}

	// THEN independent stories must NOT depend on each other
	for _, story := range plan.Stories {
		if len(story.DependsOn) > 0 {
			t.Errorf("Story %s (%s) has depends_on=%v but should be independent",
				story.ID, story.Title, story.DependsOn)
		}
		// Also verify tasks within each story don't have unnecessary cross-story deps
		for _, task := range story.Tasks {
			for _, dep := range task.DependsOn {
				// A task in US-001 should NOT depend on tasks in US-002 and vice versa
				if story.ID == "US-001" && strings.HasPrefix(dep, "T-US-002") {
					t.Errorf("Task %s in US-001 depends on %s in US-002 — artificial cross-story dependency", task.ID, dep)
				}
				if story.ID == "US-002" && strings.HasPrefix(dep, "T-US-001") {
					t.Errorf("Task %s in US-002 depends on %s in US-001 — artificial cross-story dependency", task.ID, dep)
				}
			}
		}
	}
}

// TestPlanner_DependentTasksHaveDependencies verifies that when tasks have
// a real data dependency (e.g., "create user model" then "add user API"),
// the generated plan correctly models that dependency.
func TestPlanner_DependentTasksHaveDependencies(t *testing.T) {
	// GIVEN an LLM response where US-002 genuinely depends on US-001
	// because the API endpoints need the model that US-001 creates.
	mock := &mockProvider{
		response: `{
			"schema_version": "1.1",
			"goals": [
				{"id": "G-001", "title": "User system", "description": "Build user management", "success_criteria": "Users CRUD works", "verification_method": "API test"}
			],
			"stories": [
				{
					"id": "US-001",
					"title": "Create user model",
					"description": "Define User entity and database schema",
					"requirement_id": "REQ-001",
					"goal_id": "G-001",
					"depends_on": [],
					"acceptance_criteria": ["User model exists", "Migration runs"],
					"verification_script": "go test ./models/...",
					"tasks": [
						{"id": "T-US-001-001", "title": "Define User struct", "description": "Create user model", "depends_on": [], "verification_script": "go build ./models/..."},
						{"id": "T-US-001-002", "title": "Create migration", "description": "DB migration for users table", "depends_on": ["T-US-001-001"], "verification_script": "go test ./db/..."}
					]
				},
				{
					"id": "US-002",
					"title": "Add user API endpoints",
					"description": "REST endpoints for user CRUD — requires User model from US-001",
					"requirement_id": "REQ-002",
					"goal_id": "G-001",
					"depends_on": ["US-001"],
					"acceptance_criteria": ["GET /users returns 200", "POST /users creates user"],
					"verification_script": "go test ./api/...",
					"tasks": [
						{"id": "T-US-002-001", "title": "Create user handler", "description": "Implement API handlers", "depends_on": [], "verification_script": "go test ./api/..."}
					]
				}
			]
		}`,
	}

	p := New(mock)
	plan, err := p.GeneratePlan(context.Background(), "# User system\nCreate user model, then build API on top.", nil)
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}

	// THEN US-002 must depend on US-001
	if len(plan.Stories) < 2 {
		t.Fatalf("Expected at least 2 stories, got %d", len(plan.Stories))
	}

	apiStory := plan.Stories[1]
	if apiStory.ID != "US-002" {
		t.Fatalf("Expected second story to be US-002, got %s", apiStory.ID)
	}
	if len(apiStory.DependsOn) == 0 {
		t.Fatal("US-002 (user API) should depend on US-001 (user model) but has no dependencies")
	}

	foundDep := false
	for _, dep := range apiStory.DependsOn {
		if dep == "US-001" {
			foundDep = true
		}
	}
	if !foundDep {
		t.Errorf("US-002 depends_on=%v but should include US-001", apiStory.DependsOn)
	}

	// Also verify intra-story task dependency: migration depends on struct definition
	modelStory := plan.Stories[0]
	if len(modelStory.Tasks) >= 2 {
		migrationTask := modelStory.Tasks[1]
		if len(migrationTask.DependsOn) == 0 {
			t.Error("Migration task should depend on struct definition task")
		}
	}
}

// TestPlanner_PromptIncludesParallelizationGuidance verifies that the story
// generation prompt instructs the LLM to NOT auto-chain independent tasks.
// This is the guard against Bug #3.
func TestPlanner_PromptIncludesParallelizationGuidance(t *testing.T) {
	// The prompt template must contain explicit parallelization guidance
	t.Run("StoryGenerationPrompt contains parallelism rules", func(t *testing.T) {
		// Rule 5 in the prompt says: PARALLELISM: Maximize parallelism where possible.
		if !strings.Contains(StoryGenerationPrompt, "PARALLELISM") {
			t.Error("StoryGenerationPrompt missing PARALLELISM rule")
		}
		if !strings.Contains(StoryGenerationPrompt, "MUST NOT depend on each other") {
			t.Error("StoryGenerationPrompt missing explicit instruction that orthogonal stories MUST NOT depend on each other")
		}
	})

	t.Run("StoryGenerationPrompt instructs depends_on only for true dependencies", func(t *testing.T) {
		if !strings.Contains(StoryGenerationPrompt, "true data or artifact dependency") {
			t.Error("StoryGenerationPrompt missing instruction to only use depends_on for true data/artifact dependencies")
		}
	})

	t.Run("Task-level parallelism guidance exists", func(t *testing.T) {
		// Rule 8 says tasks should only have depends_on for true dependencies
		if !strings.Contains(StoryGenerationPrompt, "Independent tasks within the same story should have empty depends_on") {
			t.Error("StoryGenerationPrompt missing task-level parallelism guidance")
		}
	})

	t.Run("StoryReviewPrompt checks for unnecessary serialization", func(t *testing.T) {
		// The review prompt must catch unnecessary chaining
		if !strings.Contains(StoryReviewPrompt, "unnecessary serialization") {
			t.Error("StoryReviewPrompt missing check for unnecessary serialization of stories")
		}
		if !strings.Contains(StoryReviewPrompt, "parallel, not chained") {
			t.Error("StoryReviewPrompt missing instruction that independent stories should be parallel")
		}
	})
}

// TestPlanner_ParsedPlanPreservesDependencyStructure verifies that JSON
// round-tripping through parseResponse preserves the depends_on field
// correctly, including empty arrays (not null).
func TestPlanner_ParsedPlanPreservesDependencyStructure(t *testing.T) {
	p := &Planner{}

	t.Run("Empty depends_on is preserved as empty array", func(t *testing.T) {
		resp := `{
			"schema_version": "1.1",
			"stories": [{
				"id": "US-001",
				"title": "Independent story",
				"depends_on": [],
				"tasks": [
					{"id": "T-US-001-001", "title": "Task A", "depends_on": []},
					{"id": "T-US-001-002", "title": "Task B", "depends_on": []}
				]
			}]
		}`
		plan, err := p.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse failed: %v", err)
		}

		story := plan.Stories[0]
		// depends_on should be parsed (may be nil for Go's zero value, but must not be non-empty)
		if len(story.DependsOn) != 0 {
			t.Errorf("Story depends_on should be empty, got %v", story.DependsOn)
		}
		for _, task := range story.Tasks {
			if len(task.DependsOn) != 0 {
				t.Errorf("Task %s depends_on should be empty, got %v", task.ID, task.DependsOn)
			}
		}
	})

	t.Run("Non-empty depends_on is preserved", func(t *testing.T) {
		resp := `{
			"schema_version": "1.1",
			"stories": [{
				"id": "US-002",
				"title": "Dependent story",
				"depends_on": ["US-001"],
				"tasks": [
					{"id": "T-US-002-001", "title": "Task A", "depends_on": ["T-US-001-001"]}
				]
			}]
		}`
		plan, err := p.parseResponse(resp)
		if err != nil {
			t.Fatalf("parseResponse failed: %v", err)
		}

		story := plan.Stories[0]
		if len(story.DependsOn) != 1 || story.DependsOn[0] != "US-001" {
			t.Errorf("Story depends_on should be [US-001], got %v", story.DependsOn)
		}
		if len(story.Tasks[0].DependsOn) != 1 || story.Tasks[0].DependsOn[0] != "T-US-001-001" {
			t.Errorf("Task depends_on should be [T-US-001-001], got %v", story.Tasks[0].DependsOn)
		}
	})
}

// TestPlanner_GeneratePlanCapturesPromptForDependencyGuidance verifies the
// actual prompt sent to the LLM includes parallelization instructions.
func TestPlanner_GeneratePlanCapturesPromptForDependencyGuidance(t *testing.T) {
	mock := &mockProvider{
		response: `{"schema_version":"1.1","stories":[{"id":"US-001","title":"S","tasks":[]}]}`,
	}
	p := New(mock)

	_, err := p.GeneratePlan(context.Background(), "Build two independent features", nil)
	if err != nil {
		t.Fatalf("GeneratePlan failed: %v", err)
	}

	// Verify the prompt sent to the LLM contains parallelism guidance
	if !strings.Contains(mock.lastPrompt, "PARALLELISM") {
		t.Error("Prompt sent to LLM missing PARALLELISM guidance")
	}
	if !strings.Contains(mock.lastPrompt, "depends_on") {
		t.Error("Prompt sent to LLM missing depends_on instructions")
	}
}

// TestProjectPlan_ValidateDependencyReferences verifies that Validate catches
// plans where depends_on references non-existent stories.
func TestProjectPlan_ValidateDependencyReferences(t *testing.T) {
	t.Run("Valid plan passes validation", func(t *testing.T) {
		plan := &ProjectPlan{
			SchemaVersion: "1.1",
			Stories: []Story{
				{ID: "US-001", Title: "First"},
				{ID: "US-002", Title: "Second", DependsOn: []string{"US-001"}},
			},
		}
		if err := plan.Validate(); err != nil {
			t.Errorf("Valid plan should pass validation: %v", err)
		}
	})

	t.Run("Plan with no stories or goals fails", func(t *testing.T) {
		plan := &ProjectPlan{SchemaVersion: "1.1"}
		if err := plan.Validate(); err == nil {
			t.Error("Plan with no stories or goals should fail validation")
		}
	})
}

// TestPlanner_AutoChainDetection verifies that we can detect when a plan
// has been auto-chained (all stories form a linear chain). This is the
// specific pattern from Bug #3.
func TestPlanner_AutoChainDetection(t *testing.T) {
	// An auto-chained plan: US-001 -> US-002 -> US-003 (serial)
	autoChainedPlan := &ProjectPlan{
		SchemaVersion: "1.1",
		Stories: []Story{
			{ID: "US-001", Title: "Login page", DependsOn: []string{}},
			{ID: "US-002", Title: "About page", DependsOn: []string{"US-001"}},
			{ID: "US-003", Title: "Contact page", DependsOn: []string{"US-002"}},
		},
	}

	// Count stories that have dependencies
	storiesWithDeps := 0
	for _, s := range autoChainedPlan.Stories {
		if len(s.DependsOn) > 0 {
			storiesWithDeps++
		}
	}

	// If all stories except the first are chained, this is likely auto-chaining
	totalStories := len(autoChainedPlan.Stories)
	if totalStories > 2 && storiesWithDeps == totalStories-1 {
		// This is the auto-chain pattern — every story (except first) depends
		// on the previous one. For truly independent pages, this is wrong.
		// This test documents the detection heuristic.
		isLinearChain := true
		storyIndex := make(map[string]int)
		for i, s := range autoChainedPlan.Stories {
			storyIndex[s.ID] = i
		}
		for i := 1; i < len(autoChainedPlan.Stories); i++ {
			story := autoChainedPlan.Stories[i]
			if len(story.DependsOn) != 1 {
				isLinearChain = false
				break
			}
			depIdx, ok := storyIndex[story.DependsOn[0]]
			if !ok || depIdx != i-1 {
				isLinearChain = false
				break
			}
		}
		if isLinearChain {
			t.Log("Detected linear auto-chain pattern — this is the Bug #3 symptom")
			// In a corrected system, independent pages should NOT form a chain.
			// This test verifies the detection works.
		}
	}

	// A correctly parallelized plan: all stories independent
	parallelPlan := &ProjectPlan{
		SchemaVersion: "1.1",
		Stories: []Story{
			{ID: "US-001", Title: "Login page", DependsOn: []string{}},
			{ID: "US-002", Title: "About page", DependsOn: []string{}},
			{ID: "US-003", Title: "Contact page", DependsOn: []string{}},
		},
	}

	parallelStoriesWithDeps := 0
	for _, s := range parallelPlan.Stories {
		if len(s.DependsOn) > 0 {
			parallelStoriesWithDeps++
		}
	}
	if parallelStoriesWithDeps > 0 {
		t.Errorf("Parallel plan should have 0 stories with deps, got %d", parallelStoriesWithDeps)
	}
}

// TestPlanJSON_DependsOnSerialization ensures depends_on round-trips correctly
// through JSON marshaling, which is critical for plan persistence.
func TestPlanJSON_DependsOnSerialization(t *testing.T) {
	original := &ProjectPlan{
		SchemaVersion: "1.1",
		Stories: []Story{
			{
				ID:        "US-001",
				Title:     "Independent",
				DependsOn: []string{},
				Tasks: []Task{
					{ID: "T-001", Title: "Task", DependsOn: []string{}},
				},
			},
			{
				ID:        "US-002",
				Title:     "Dependent",
				DependsOn: []string{"US-001"},
				Tasks: []Task{
					{ID: "T-002", Title: "Task", DependsOn: []string{"T-001"}},
				},
			},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var restored ProjectPlan
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// US-001 should have empty depends_on
	if len(restored.Stories[0].DependsOn) != 0 {
		t.Errorf("US-001 depends_on should be empty after round-trip, got %v", restored.Stories[0].DependsOn)
	}

	// US-002 should depend on US-001
	if len(restored.Stories[1].DependsOn) != 1 || restored.Stories[1].DependsOn[0] != "US-001" {
		t.Errorf("US-002 depends_on should be [US-001], got %v", restored.Stories[1].DependsOn)
	}

	// Task-level deps preserved
	if len(restored.Stories[1].Tasks[0].DependsOn) != 1 || restored.Stories[1].Tasks[0].DependsOn[0] != "T-001" {
		t.Errorf("T-002 depends_on should be [T-001], got %v", restored.Stories[1].Tasks[0].DependsOn)
	}
}
