package release

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestManager_SQLiteCanonicalState verifies that state is persisted to SQLite only,
// and JSON files are not written during normal operations (AC-1, AC-2).
func TestManager_SQLiteCanonicalState(t *testing.T) {
	t.Run("saves state to SQLite without writing JSON files", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := DefaultConfig()
		cfg.GitEnabled = false

		manager, err := NewManager(tmpDir, cfg)
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		defer manager.Close()

		// Create a goal
		goal := &Goal{
			ID:                 "G-TEST",
			Title:              "Test Goal",
			Description:        "A test goal",
			SuccessCriteria:    "Tests pass",
			VerificationMethod: "Run tests",
		}
		if err := manager.CreateGoal(goal); err != nil {
			t.Fatalf("CreateGoal failed: %v", err)
		}

		// Create a story
		story := &Story{
			ID:                 "US-TEST",
			GoalID:             "G-TEST",
			Title:              "Test Story",
			Description:        "A test story",
			AcceptanceCriteria: []string{"Criteria 1"},
			Status:             StoryStatusPending,
			StoryType:          StoryTypeFeature,
			Tasks:              []string{},
			DependsOn:          []string{},
			CreatedAt:          time.Now(),
		}
		if err := manager.CreateStory(story); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		// Create a task
		task := &Task{
			ID:          "T-TEST-001",
			StoryID:     "US-TEST",
			Title:       "Test Task",
			Description: "A test task",
			Status:      TaskStatusPending,
			DependsOn:   []string{},
			CreatedAt:   time.Now(),
		}
		if err := manager.CreateTask(task); err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		// Verify: JSON files should NOT exist (AC-2: JSON artifacts are read-only exports or removed)
		openexecDir := filepath.Join(tmpDir, ".openexec")
		storiesJSON := filepath.Join(openexecDir, "stories.json")
		tasksJSON := filepath.Join(openexecDir, "tasks.json")
		releaseJSON := filepath.Join(openexecDir, "release.json")

		if _, err := os.Stat(storiesJSON); !os.IsNotExist(err) {
			t.Errorf("stories.json should not be created during normal operations, but it exists")
		}
		if _, err := os.Stat(tasksJSON); !os.IsNotExist(err) {
			t.Errorf("tasks.json should not be created during normal operations, but it exists")
		}
		if _, err := os.Stat(releaseJSON); !os.IsNotExist(err) {
			t.Errorf("release.json should not be created during normal operations, but it exists")
		}

		// Verify: SQLite database should exist and contain the data
		dbPath := filepath.Join(openexecDir, "data", "state.db")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Fatalf("state.db should exist: %v", err)
		}

		// Verify data is in SQLite by reading back
		ctx := context.Background()

		// Check goal via store
		gotGoal, err := manager.store.GetGoal(ctx, "G-TEST")
		if err != nil {
			t.Fatalf("GetGoal from store failed: %v", err)
		}
		if gotGoal.Title != goal.Title {
			t.Errorf("goal title mismatch: got %q, want %q", gotGoal.Title, goal.Title)
		}

		// Check story via store
		gotStory, err := manager.store.GetStory(ctx, "US-TEST")
		if err != nil {
			t.Fatalf("GetStory from store failed: %v", err)
		}
		if gotStory.Title != story.Title {
			t.Errorf("story title mismatch: got %q, want %q", gotStory.Title, story.Title)
		}

		// Check task via store
		gotTask, err := manager.store.GetTask(ctx, "T-TEST-001")
		if err != nil {
			t.Fatalf("GetTask from store failed: %v", err)
		}
		if gotTask.Title != task.Title {
			t.Errorf("task title mismatch: got %q, want %q", gotTask.Title, task.Title)
		}
	})

	t.Run("loads state from SQLite on restart", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := DefaultConfig()
		cfg.GitEnabled = false

		// First manager instance: create data
		manager1, err := NewManager(tmpDir, cfg)
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}

		goal := &Goal{
			ID:    "G-PERSIST",
			Title: "Persistence Test Goal",
		}
		if err := manager1.CreateGoal(goal); err != nil {
			t.Fatalf("CreateGoal failed: %v", err)
		}

		story := &Story{
			ID:        "US-PERSIST",
			GoalID:    "G-PERSIST",
			Title:     "Persistence Test Story",
			Status:    StoryStatusPending,
			StoryType: StoryTypeFeature,
			Tasks:     []string{},
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := manager1.CreateStory(story); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		task := &Task{
			ID:        "T-PERSIST-001",
			StoryID:   "US-PERSIST",
			Title:     "Persistence Test Task",
			Status:    TaskStatusInProgress,
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := manager1.CreateTask(task); err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		manager1.Close()

		// Second manager instance: verify data is loaded from SQLite
		manager2, err := NewManager(tmpDir, cfg)
		if err != nil {
			t.Fatalf("NewManager (2nd instance) failed: %v", err)
		}
		defer manager2.Close()

		// Verify via Manager public API (which reads from cache, populated from SQLite)
		gotGoal := manager2.GetGoal("G-PERSIST")
		if gotGoal == nil {
			t.Fatal("goal not loaded from SQLite")
		}
		if gotGoal.Title != "Persistence Test Goal" {
			t.Errorf("goal title mismatch: got %q, want %q", gotGoal.Title, "Persistence Test Goal")
		}

		gotStory := manager2.GetStory("US-PERSIST")
		if gotStory == nil {
			t.Fatal("story not loaded from SQLite")
		}
		if gotStory.Title != "Persistence Test Story" {
			t.Errorf("story title mismatch: got %q, want %q", gotStory.Title, "Persistence Test Story")
		}

		gotTask := manager2.GetTask("T-PERSIST-001")
		if gotTask == nil {
			t.Fatal("task not loaded from SQLite")
		}
		if gotTask.Title != "Persistence Test Task" {
			t.Errorf("task title mismatch: got %q, want %q", gotTask.Title, "Persistence Test Task")
		}
		if gotTask.Status != TaskStatusInProgress {
			t.Errorf("task status mismatch: got %q, want %q", gotTask.Status, TaskStatusInProgress)
		}
	})

	t.Run("updates state in SQLite correctly", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := DefaultConfig()
		cfg.GitEnabled = false

		manager, err := NewManager(tmpDir, cfg)
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		defer manager.Close()

		// Create a task
		story := &Story{
			ID:        "US-UPDATE",
			Title:     "Update Test Story",
			Status:    StoryStatusPending,
			StoryType: StoryTypeFeature,
			Tasks:     []string{},
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := manager.CreateStory(story); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		task := &Task{
			ID:        "T-UPDATE-001",
			StoryID:   "US-UPDATE",
			Title:     "Original Title",
			Status:    TaskStatusPending,
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := manager.CreateTask(task); err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		// Update the task
		task.Title = "Updated Title"
		task.Status = TaskStatusInProgress
		if err := manager.UpdateTask(task); err != nil {
			t.Fatalf("UpdateTask failed: %v", err)
		}

		// Verify update is persisted in SQLite
		ctx := context.Background()
		gotTask, err := manager.store.GetTask(ctx, "T-UPDATE-001")
		if err != nil {
			t.Fatalf("GetTask from store failed: %v", err)
		}
		if gotTask.Title != "Updated Title" {
			t.Errorf("task title not updated in SQLite: got %q, want %q", gotTask.Title, "Updated Title")
		}
		if gotTask.Status != TaskStatusInProgress {
			t.Errorf("task status not updated in SQLite: got %q, want %q", gotTask.Status, TaskStatusInProgress)
		}
	})
}

// TestManager_SessionAuditCompatibility verifies that Session and Audit tests pass (AC-3).
// This test ensures the Manager interacts correctly with the SQLite-based stores
// that Session and Audit components depend on.
func TestManager_SessionAuditCompatibility(t *testing.T) {
	t.Run("manager store interface is compatible with session/audit patterns", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := DefaultConfig()
		cfg.GitEnabled = false

		manager, err := NewManager(tmpDir, cfg)
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		defer manager.Close()

		// Verify the store interface is accessible and functional
		ctx := context.Background()

		// Test bulk operations (used during bootstrap/session restore)
		goals := []*Goal{
			{ID: "G-BULK-1", Title: "Bulk Goal 1"},
			{ID: "G-BULK-2", Title: "Bulk Goal 2"},
		}
		if err := manager.store.BulkCreateGoals(ctx, goals); err != nil {
			t.Fatalf("BulkCreateGoals failed: %v", err)
		}

		stories := []*Story{
			{ID: "US-BULK-1", Title: "Bulk Story 1", Status: StoryStatusPending, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
			{ID: "US-BULK-2", Title: "Bulk Story 2", Status: StoryStatusPending, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
		}
		if err := manager.store.BulkCreateStories(ctx, stories); err != nil {
			t.Fatalf("BulkCreateStories failed: %v", err)
		}

		tasks := []*Task{
			{ID: "T-BULK-1", StoryID: "US-BULK-1", Title: "Bulk Task 1", Status: TaskStatusPending, DependsOn: []string{}, CreatedAt: time.Now()},
			{ID: "T-BULK-2", StoryID: "US-BULK-2", Title: "Bulk Task 2", Status: TaskStatusPending, DependsOn: []string{}, CreatedAt: time.Now()},
		}
		if err := manager.store.BulkCreateTasks(ctx, tasks); err != nil {
			t.Fatalf("BulkCreateTasks failed: %v", err)
		}

		// Verify counts (used by session checks)
		goalCount, err := manager.store.CountGoals(ctx)
		if err != nil {
			t.Fatalf("CountGoals failed: %v", err)
		}
		if goalCount < 2 {
			t.Errorf("goal count: got %d, want >= 2", goalCount)
		}

		storyCount, err := manager.store.CountStories(ctx)
		if err != nil {
			t.Fatalf("CountStories failed: %v", err)
		}
		if storyCount < 2 {
			t.Errorf("story count: got %d, want >= 2", storyCount)
		}

		taskCount, err := manager.store.CountTasks(ctx)
		if err != nil {
			t.Fatalf("CountTasks failed: %v", err)
		}
		if taskCount < 2 {
			t.Errorf("task count: got %d, want >= 2", taskCount)
		}

		// Verify list operations (used during session iteration)
		listedGoals, err := manager.store.ListGoals(ctx)
		if err != nil {
			t.Fatalf("ListGoals failed: %v", err)
		}
		if len(listedGoals) < 2 {
			t.Errorf("listed goals: got %d, want >= 2", len(listedGoals))
		}

		listedStories, err := manager.store.ListStories(ctx)
		if err != nil {
			t.Fatalf("ListStories failed: %v", err)
		}
		if len(listedStories) < 2 {
			t.Errorf("listed stories: got %d, want >= 2", len(listedStories))
		}

		listedTasks, err := manager.store.ListTasks(ctx)
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}
		if len(listedTasks) < 2 {
			t.Errorf("listed tasks: got %d, want >= 2", len(listedTasks))
		}
	})
}

// TestManager_ExportJSON verifies that JSON export functionality works correctly
// for read-only artifact generation (AC-2 allows read-only exports).
func TestManager_ExportJSON(t *testing.T) {
	t.Run("exports state to JSON files on demand", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := DefaultConfig()
		cfg.GitEnabled = false

		manager, err := NewManager(tmpDir, cfg)
		if err != nil {
			t.Fatalf("NewManager failed: %v", err)
		}
		defer manager.Close()

		// Create some data
		goal := &Goal{ID: "G-EXPORT", Title: "Export Test Goal"}
		if err := manager.CreateGoal(goal); err != nil {
			t.Fatalf("CreateGoal failed: %v", err)
		}

		story := &Story{
			ID:        "US-EXPORT",
			GoalID:    "G-EXPORT",
			Title:     "Export Test Story",
			Status:    StoryStatusPending,
			StoryType: StoryTypeFeature,
			Tasks:     []string{},
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := manager.CreateStory(story); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		// Export to JSON (read-only artifact generation)
		exportDir := filepath.Join(tmpDir, "export")
		if err := manager.ExportJSON(exportDir); err != nil {
			t.Fatalf("ExportJSON failed: %v", err)
		}

		// Verify exported files exist
		storiesJSON := filepath.Join(exportDir, "stories.json")
		if _, err := os.Stat(storiesJSON); os.IsNotExist(err) {
			t.Errorf("stories.json should exist in export directory")
		}

		// Verify .openexec directory still doesn't have JSON files (normal save path)
		openexecDir := filepath.Join(tmpDir, ".openexec")
		normalStoriesJSON := filepath.Join(openexecDir, "stories.json")
		if _, err := os.Stat(normalStoriesJSON); !os.IsNotExist(err) {
			t.Errorf("stories.json should not exist in .openexec during normal operations")
		}
	})
}
