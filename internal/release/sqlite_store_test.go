package release

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// testDB creates a temporary SQLite database for testing.
func testDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "release-store-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "state.db")
	db, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on")
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to open database: %v", err)
	}

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return db, cleanup
}

// TestSQLiteStore_CreateRelease tests release creation.
func TestSQLiteStore_CreateRelease(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("creates release with valid data", func(t *testing.T) {
		release := &Release{
			Name:        "Test Release",
			Version:     "1.0.0",
			Description: "A test release",
			Status:      ReleaseStatusDraft,
			Stories:     []string{},
			CreatedAt:   time.Now(),
		}

		err := store.CreateRelease(ctx, release)
		if err != nil {
			t.Fatalf("CreateRelease failed: %v", err)
		}

		// Verify it was created
		got, err := store.GetRelease(ctx)
		if err != nil {
			t.Fatalf("GetRelease failed: %v", err)
		}
		if got.Name != release.Name {
			t.Errorf("got name %q, want %q", got.Name, release.Name)
		}
		if got.Version != release.Version {
			t.Errorf("got version %q, want %q", got.Version, release.Version)
		}
	})

	t.Run("returns error for duplicate release", func(t *testing.T) {
		release := &Release{
			Name:      "Duplicate",
			Version:   "2.0.0",
			Status:    ReleaseStatusDraft,
			Stories:   []string{},
			CreatedAt: time.Now(),
		}

		err := store.CreateRelease(ctx, release)
		if err != ErrReleaseAlreadyExist {
			t.Errorf("expected ErrReleaseAlreadyExist, got %v", err)
		}
	})
}

// TestSQLiteStore_GetRelease tests release retrieval.
func TestSQLiteStore_GetRelease(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("returns nil when no release exists", func(t *testing.T) {
		release, err := store.GetRelease(ctx)
		if err != nil {
			t.Fatalf("GetRelease should not error on empty: %v", err)
		}
		if release != nil {
			t.Errorf("expected nil release, got %+v", release)
		}
	})

	t.Run("returns release when exists", func(t *testing.T) {
		release := &Release{
			Name:        "Existing Release",
			Version:     "1.0.0",
			Description: "Test",
			Status:      ReleaseStatusDraft,
			Stories:     []string{"US-001", "US-002"},
			Git: &ReleaseGitInfo{
				Branch:     "release/1.0.0",
				BaseBranch: "main",
			},
			CreatedAt: time.Now(),
		}

		if err := store.CreateRelease(ctx, release); err != nil {
			t.Fatalf("CreateRelease failed: %v", err)
		}

		got, err := store.GetRelease(ctx)
		if err != nil {
			t.Fatalf("GetRelease failed: %v", err)
		}
		if got == nil {
			t.Fatal("expected release, got nil")
		}
		if got.Name != release.Name {
			t.Errorf("got name %q, want %q", got.Name, release.Name)
		}
		if len(got.Stories) != 2 {
			t.Errorf("got %d stories, want 2", len(got.Stories))
		}
		if got.Git == nil || got.Git.Branch != "release/1.0.0" {
			t.Errorf("git info not preserved")
		}
	})
}

// TestSQLiteStore_CreateGoal tests goal creation.
func TestSQLiteStore_CreateGoal(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("creates goal with valid data", func(t *testing.T) {
		goal := &Goal{
			ID:                 "G-001",
			Title:              "Test Goal",
			Description:        "A test goal",
			SuccessCriteria:    "Tests pass",
			VerificationMethod: "Run tests",
		}

		err := store.CreateGoal(ctx, goal)
		if err != nil {
			t.Fatalf("CreateGoal failed: %v", err)
		}

		got, err := store.GetGoal(ctx, "G-001")
		if err != nil {
			t.Fatalf("GetGoal failed: %v", err)
		}
		if got.Title != goal.Title {
			t.Errorf("got title %q, want %q", got.Title, goal.Title)
		}
	})

	t.Run("returns error for duplicate ID", func(t *testing.T) {
		goal := &Goal{
			ID:    "G-001",
			Title: "Duplicate",
		}

		err := store.CreateGoal(ctx, goal)
		if err != ErrGoalAlreadyExist {
			t.Errorf("expected ErrGoalAlreadyExist, got %v", err)
		}
	})

	t.Run("returns error for empty ID", func(t *testing.T) {
		goal := &Goal{
			ID:    "",
			Title: "No ID",
		}

		err := store.CreateGoal(ctx, goal)
		if err != ErrInvalidData {
			t.Errorf("expected ErrInvalidData, got %v", err)
		}
	})
}

// TestSQLiteStore_CreateStory tests story creation.
func TestSQLiteStore_CreateStory(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create a goal first
	goal := &Goal{
		ID:    "G-001",
		Title: "Test Goal",
	}
	if err := store.CreateGoal(ctx, goal); err != nil {
		t.Fatalf("CreateGoal failed: %v", err)
	}

	t.Run("creates story with valid data", func(t *testing.T) {
		story := &Story{
			ID:                 "US-001",
			GoalID:             "G-001",
			Title:              "Test Story",
			Description:        "A test story",
			AcceptanceCriteria: []string{"Criteria 1", "Criteria 2"},
			Status:             StoryStatusPending,
			StoryType:          StoryTypeFeature,
			Tasks:              []string{},
			DependsOn:          []string{},
			CreatedAt:          time.Now(),
		}

		err := store.CreateStory(ctx, story)
		if err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		got, err := store.GetStory(ctx, "US-001")
		if err != nil {
			t.Fatalf("GetStory failed: %v", err)
		}
		if got.Title != story.Title {
			t.Errorf("got title %q, want %q", got.Title, story.Title)
		}
		if len(got.AcceptanceCriteria) != 2 {
			t.Errorf("got %d acceptance criteria, want 2", len(got.AcceptanceCriteria))
		}
	})

	t.Run("creates story without goal reference", func(t *testing.T) {
		story := &Story{
			ID:        "US-002",
			Title:     "Standalone Story",
			Status:    StoryStatusPending,
			StoryType: StoryTypeChore,
			Tasks:     []string{},
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}

		err := store.CreateStory(ctx, story)
		if err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}
	})
}

// TestSQLiteStore_CreateTask tests task creation.
func TestSQLiteStore_CreateTask(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create story first
	story := &Story{
		ID:        "US-001",
		Title:     "Test Story",
		Status:    StoryStatusPending,
		StoryType: StoryTypeFeature,
		Tasks:     []string{},
		DependsOn: []string{},
		CreatedAt: time.Now(),
	}
	if err := store.CreateStory(ctx, story); err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	t.Run("creates task with valid data", func(t *testing.T) {
		task := &Task{
			ID:          "T-US-001-001",
			StoryID:     "US-001",
			Title:       "Test Task",
			Description: "A test task",
			Status:      TaskStatusPending,
			DependsOn:   []string{},
			CreatedAt:   time.Now(),
		}

		err := store.CreateTask(ctx, task)
		if err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		got, err := store.GetTask(ctx, "T-US-001-001")
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if got.Title != task.Title {
			t.Errorf("got title %q, want %q", got.Title, task.Title)
		}
		if got.StoryID != task.StoryID {
			t.Errorf("got storyID %q, want %q", got.StoryID, task.StoryID)
		}
	})

	t.Run("cascades delete when story is deleted", func(t *testing.T) {
		// Create another story and task
		story2 := &Story{
			ID:        "US-002",
			Title:     "Story to Delete",
			Status:    StoryStatusPending,
			StoryType: StoryTypeFeature,
			Tasks:     []string{},
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := store.CreateStory(ctx, story2); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		task2 := &Task{
			ID:        "T-US-002-001",
			StoryID:   "US-002",
			Title:     "Task to be cascaded",
			Status:    TaskStatusPending,
			DependsOn: []string{},
			CreatedAt: time.Now(),
		}
		if err := store.CreateTask(ctx, task2); err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		// Delete the story
		if err := store.DeleteStory(ctx, "US-002"); err != nil {
			t.Fatalf("DeleteStory failed: %v", err)
		}

		// Task should be gone
		_, err := store.GetTask(ctx, "T-US-002-001")
		if err != ErrTaskNotFound {
			t.Errorf("expected ErrTaskNotFound after cascade, got %v", err)
		}
	})
}

// TestSQLiteStore_UpdateTask tests task updates.
func TestSQLiteStore_UpdateTask(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create story and task
	story := &Story{
		ID:        "US-001",
		Title:     "Test Story",
		Status:    StoryStatusPending,
		StoryType: StoryTypeFeature,
		Tasks:     []string{},
		DependsOn: []string{},
		CreatedAt: time.Now(),
	}
	if err := store.CreateStory(ctx, story); err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	task := &Task{
		ID:        "T-001",
		StoryID:   "US-001",
		Title:     "Original Title",
		Status:    TaskStatusPending,
		DependsOn: []string{},
		CreatedAt: time.Now(),
	}
	if err := store.CreateTask(ctx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	t.Run("updates status", func(t *testing.T) {
		task.Status = TaskStatusInProgress
		now := time.Now()
		task.StartedAt = &now

		err := store.UpdateTask(ctx, task)
		if err != nil {
			t.Fatalf("UpdateTask failed: %v", err)
		}

		got, err := store.GetTask(ctx, "T-001")
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if got.Status != TaskStatusInProgress {
			t.Errorf("got status %q, want %q", got.Status, TaskStatusInProgress)
		}
	})

	t.Run("updates dependencies", func(t *testing.T) {
		task.DependsOn = []string{"T-002", "T-003"}

		err := store.UpdateTask(ctx, task)
		if err != nil {
			t.Fatalf("UpdateTask failed: %v", err)
		}

		got, err := store.GetTask(ctx, "T-001")
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if len(got.DependsOn) != 2 {
			t.Errorf("got %d dependencies, want 2", len(got.DependsOn))
		}
	})

	t.Run("returns error for non-existent task", func(t *testing.T) {
		nonExistent := &Task{
			ID:      "T-NOPE",
			StoryID: "US-001",
			Title:   "Non-existent",
			Status:  TaskStatusPending,
		}

		err := store.UpdateTask(ctx, nonExistent)
		if err != ErrTaskNotFound {
			t.Errorf("expected ErrTaskNotFound, got %v", err)
		}
	})
}

// TestSQLiteStore_ListTasksByStory tests listing tasks by story.
func TestSQLiteStore_ListTasksByStory(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Create story
	story := &Story{
		ID:        "US-001",
		Title:     "Test Story",
		Status:    StoryStatusPending,
		StoryType: StoryTypeFeature,
		Tasks:     []string{},
		DependsOn: []string{},
		CreatedAt: time.Now(),
	}
	if err := store.CreateStory(ctx, story); err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	t.Run("returns empty slice for no tasks", func(t *testing.T) {
		tasks, err := store.ListTasksByStory(ctx, "US-001")
		if err != nil {
			t.Fatalf("ListTasksByStory failed: %v", err)
		}
		if tasks == nil {
			t.Error("expected empty slice, got nil")
		}
		if len(tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(tasks))
		}
	})

	t.Run("returns all tasks for story ordered by created_at", func(t *testing.T) {
		// Create tasks in specific order
		for i, title := range []string{"First", "Second", "Third"} {
			task := &Task{
				ID:        "T-00" + string(rune('1'+i)),
				StoryID:   "US-001",
				Title:     title,
				Status:    TaskStatusPending,
				DependsOn: []string{},
				CreatedAt: time.Now().Add(time.Duration(i) * time.Second),
			}
			if err := store.CreateTask(ctx, task); err != nil {
				t.Fatalf("CreateTask failed: %v", err)
			}
		}

		tasks, err := store.ListTasksByStory(ctx, "US-001")
		if err != nil {
			t.Fatalf("ListTasksByStory failed: %v", err)
		}
		if len(tasks) != 3 {
			t.Errorf("expected 3 tasks, got %d", len(tasks))
		}
		// Verify order
		if tasks[0].Title != "First" {
			t.Errorf("expected first task 'First', got %q", tasks[0].Title)
		}
	})
}

// TestSQLiteStore_BulkOperations tests bulk creation for bootstrap.
func TestSQLiteStore_BulkOperations(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("bulk creates goals", func(t *testing.T) {
		goals := []*Goal{
			{ID: "G-001", Title: "Goal 1"},
			{ID: "G-002", Title: "Goal 2"},
			{ID: "G-003", Title: "Goal 3"},
		}

		err := store.BulkCreateGoals(ctx, goals)
		if err != nil {
			t.Fatalf("BulkCreateGoals failed: %v", err)
		}

		count, err := store.CountGoals(ctx)
		if err != nil {
			t.Fatalf("CountGoals failed: %v", err)
		}
		if count != 3 {
			t.Errorf("expected 3 goals, got %d", count)
		}
	})

	t.Run("bulk creates stories", func(t *testing.T) {
		stories := []*Story{
			{ID: "US-001", GoalID: "G-001", Title: "Story 1", Status: StoryStatusPending, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
			{ID: "US-002", GoalID: "G-002", Title: "Story 2", Status: StoryStatusPending, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
		}

		err := store.BulkCreateStories(ctx, stories)
		if err != nil {
			t.Fatalf("BulkCreateStories failed: %v", err)
		}

		count, err := store.CountStories(ctx)
		if err != nil {
			t.Fatalf("CountStories failed: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 stories, got %d", count)
		}
	})

	t.Run("bulk creates tasks", func(t *testing.T) {
		tasks := []*Task{
			{ID: "T-001", StoryID: "US-001", Title: "Task 1", Status: TaskStatusPending, DependsOn: []string{}, CreatedAt: time.Now()},
			{ID: "T-002", StoryID: "US-001", Title: "Task 2", Status: TaskStatusPending, DependsOn: []string{}, CreatedAt: time.Now()},
			{ID: "T-003", StoryID: "US-002", Title: "Task 3", Status: TaskStatusPending, DependsOn: []string{}, CreatedAt: time.Now()},
		}

		err := store.BulkCreateTasks(ctx, tasks)
		if err != nil {
			t.Fatalf("BulkCreateTasks failed: %v", err)
		}

		count, err := store.CountTasks(ctx)
		if err != nil {
			t.Fatalf("CountTasks failed: %v", err)
		}
		if count != 3 {
			t.Errorf("expected 3 tasks, got %d", count)
		}
	})
}

// TestSQLiteStore_CountOperations tests count operations for bootstrap check.
func TestSQLiteStore_CountOperations(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("counts return zero for empty tables", func(t *testing.T) {
		goalCount, err := store.CountGoals(ctx)
		if err != nil {
			t.Fatalf("CountGoals failed: %v", err)
		}
		if goalCount != 0 {
			t.Errorf("expected 0 goals, got %d", goalCount)
		}

		storyCount, err := store.CountStories(ctx)
		if err != nil {
			t.Fatalf("CountStories failed: %v", err)
		}
		if storyCount != 0 {
			t.Errorf("expected 0 stories, got %d", storyCount)
		}

		taskCount, err := store.CountTasks(ctx)
		if err != nil {
			t.Fatalf("CountTasks failed: %v", err)
		}
		if taskCount != 0 {
			t.Errorf("expected 0 tasks, got %d", taskCount)
		}
	})
}

// TestSQLiteStore_GitInfoPreservation tests that git info is preserved.
func TestSQLiteStore_GitInfoPreservation(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("preserves task git info", func(t *testing.T) {
		story := &Story{
			ID:        "US-001",
			Title:     "Test Story",
			Status:    StoryStatusPending,
			StoryType: StoryTypeFeature,
			Tasks:     []string{},
			DependsOn: []string{},
			Git: &StoryGitInfo{
				Branch:     "feature/US-001",
				BaseBranch: "main",
			},
			CreatedAt: time.Now(),
		}
		if err := store.CreateStory(ctx, story); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		task := &Task{
			ID:        "T-001",
			StoryID:   "US-001",
			Title:     "Task with Git",
			Status:    TaskStatusPending,
			DependsOn: []string{},
			Git: &TaskGitInfo{
				Commits: []string{"abc123", "def456"},
				Branch:  "feature/US-001",
			},
			CreatedAt: time.Now(),
		}
		if err := store.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask failed: %v", err)
		}

		got, err := store.GetTask(ctx, "T-001")
		if err != nil {
			t.Fatalf("GetTask failed: %v", err)
		}
		if got.Git == nil {
			t.Fatal("expected git info, got nil")
		}
		if len(got.Git.Commits) != 2 {
			t.Errorf("expected 2 commits, got %d", len(got.Git.Commits))
		}
		if got.Git.Branch != "feature/US-001" {
			t.Errorf("expected branch 'feature/US-001', got %q", got.Git.Branch)
		}
	})
}

// TestSQLiteStore_ApprovalInfoPreservation tests that approval info is preserved.
func TestSQLiteStore_ApprovalInfoPreservation(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	t.Run("preserves story approval info", func(t *testing.T) {
		now := time.Now()
		story := &Story{
			ID:        "US-001",
			Title:     "Test Story",
			Status:    StoryStatusApproved,
			StoryType: StoryTypeFeature,
			Tasks:     []string{},
			DependsOn: []string{},
			Approval: &ApprovalInfo{
				Status:      ApprovalApproved,
				ApprovedBy:  "reviewer@example.com",
				ApprovedAt:  &now,
				Comments:    "LGTM",
				ReviewCycle: 1,
			},
			CreatedAt: time.Now(),
		}
		if err := store.CreateStory(ctx, story); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}

		got, err := store.GetStory(ctx, "US-001")
		if err != nil {
			t.Fatalf("GetStory failed: %v", err)
		}
		if got.Approval == nil {
			t.Fatal("expected approval info, got nil")
		}
		if got.Approval.Status != ApprovalApproved {
			t.Errorf("expected approval status %q, got %q", ApprovalApproved, got.Approval.Status)
		}
		if got.Approval.ApprovedBy != "reviewer@example.com" {
			t.Errorf("expected approved by 'reviewer@example.com', got %q", got.Approval.ApprovedBy)
		}
	})
}

// TestSQLiteStore_ListMethods tests various list methods.
func TestSQLiteStore_ListMethods(t *testing.T) {
	db, cleanup := testDB(t)
	defer cleanup()

	store, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Setup data
	goals := []*Goal{
		{ID: "G-001", Title: "Goal 1"},
		{ID: "G-002", Title: "Goal 2"},
	}
	for _, g := range goals {
		if err := store.CreateGoal(ctx, g); err != nil {
			t.Fatalf("CreateGoal failed: %v", err)
		}
	}

	stories := []*Story{
		{ID: "US-001", GoalID: "G-001", Title: "Story 1", Status: StoryStatusPending, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
		{ID: "US-002", GoalID: "G-001", Title: "Story 2", Status: StoryStatusInProgress, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
		{ID: "US-003", GoalID: "G-002", Title: "Story 3", Status: StoryStatusPending, StoryType: StoryTypeFeature, Tasks: []string{}, DependsOn: []string{}, CreatedAt: time.Now()},
	}
	for _, s := range stories {
		if err := store.CreateStory(ctx, s); err != nil {
			t.Fatalf("CreateStory failed: %v", err)
		}
	}

	t.Run("ListGoals returns all goals", func(t *testing.T) {
		got, err := store.ListGoals(ctx)
		if err != nil {
			t.Fatalf("ListGoals failed: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 goals, got %d", len(got))
		}
	})

	t.Run("ListStories returns all stories", func(t *testing.T) {
		got, err := store.ListStories(ctx)
		if err != nil {
			t.Fatalf("ListStories failed: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("expected 3 stories, got %d", len(got))
		}
	})

	t.Run("ListStoriesByGoal filters by goal", func(t *testing.T) {
		got, err := store.ListStoriesByGoal(ctx, "G-001")
		if err != nil {
			t.Fatalf("ListStoriesByGoal failed: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 stories for G-001, got %d", len(got))
		}
	})

	t.Run("ListStoriesByStatus filters by status", func(t *testing.T) {
		got, err := store.ListStoriesByStatus(ctx, StoryStatusPending)
		if err != nil {
			t.Fatalf("ListStoriesByStatus failed: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("expected 2 pending stories, got %d", len(got))
		}
	})
}

// Ensure SQLiteStore implements Store interface.
var _ Store = (*SQLiteStore)(nil)
