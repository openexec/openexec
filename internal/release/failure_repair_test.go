package release

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func repairFixture(t *testing.T) (*SQLiteStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	if err := s.CreateStory(ctx, &Story{ID: "story", Title: "accepted outcome", Tasks: []string{"task"}, Git: &StoryGitInfo{Branch: "feature/retained"}, Status: StoryStatusInProgress, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	pr := 17
	if err := s.CreateTask(ctx, &Task{ID: "task", StoryID: "story", Title: "deliver outcome", Status: TaskStatusFailed, AttemptCount: 1, MaxAttempts: 3, Git: &TaskGitInfo{Branch: "feature/retained", Commits: []string{"old-commit"}, PRNumber: &pr}, Metadata: map[string]interface{}{"retained": "original"}, ErrorMessage: "observed failure", CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	return s, path
}

func TestFailureRepairPersistsDependencyAndSameCandidateAcrossRestart(t *testing.T) {
	s, path := repairFixture(t)
	ctx := context.Background()
	original, _ := s.GetTask(ctx, "task")
	repair, err := s.CreateFailureRepair(ctx, "task", "evidence:1", "repair the demonstrated cause")
	if err != nil {
		t.Fatal(err)
	}
	if repair.StoryID != original.StoryID || repair.Git.Branch != original.Git.Branch || len(repair.Git.Commits) != 0 || *repair.Git.PRNumber != *original.Git.PRNumber || repair.Status != TaskStatusPending || repair.Approval != nil {
		t.Fatal("repair invented delivery/approval or candidate", repair)
	}
	if err := s.CreateTask(ctx, &Task{ID: "peer", StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, Priority: -1}); err != nil {
		t.Fatal(err)
	}
	ready, err := RunnableTasks(ctx, s, []string{"story"})
	if err != nil || len(ready) != 2 || ready[0].ID != repair.ID {
		t.Fatal("original did not wait on new repair", ready, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	again, err := reopened.CreateFailureRepair(ctx, "task", "evidence:1", "repair the demonstrated cause")
	if err != nil || again.ID != repair.ID {
		t.Fatal("retry duplicated repair", err)
	}
	if _, err := reopened.CreateFailureRepair(ctx, "task", "evidence:1", "different diagnosis"); err == nil {
		t.Fatal("same evidence authorized changed repair")
	}
	retained, _ := reopened.GetTask(ctx, "task")
	if retained.Status != TaskStatusPending || retained.ErrorMessage != original.ErrorMessage || !reflect.DeepEqual(retained.Git, original.Git) || retained.AttemptCount != original.AttemptCount {
		t.Fatal("original history rewritten", retained)
	}
	again.Status = TaskStatusDone
	if err := reopened.UpdateTask(ctx, again); err != nil {
		t.Fatal(err)
	}
	ready, err = RunnableTasks(ctx, reopened, []string{"story"})
	if err != nil || len(ready) != 2 || ready[1].ID != "task" {
		t.Fatal("original cannot resume after repair", err)
	}
	retained, _ = reopened.GetTask(ctx, "task")
	if retained.Status == TaskStatusDone {
		t.Fatal("repair falsely completed original")
	}
}

func TestFailureRepairGuardsAndTransactionRollback(t *testing.T) {
	for _, mode := range []string{"done", "pending", "limit", "hitl", "branch", "missing_evidence", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := repairFixture(t)
			ctx := context.Background()
			task, _ := s.GetTask(ctx, "task")
			switch mode {
			case "done":
				task.Status = TaskStatusDone
			case "pending":
				task.Status = TaskStatusPending
			case "limit":
				task.AttemptCount = task.MaxAttempts
			case "hitl":
				task.Metadata["mode"] = TaskModeHITL
			case "branch":
				task.Git.Branch = "other"
			}
			if err := s.UpdateTask(ctx, task); err != nil {
				t.Fatal(err)
			}
			if mode == "rollback" {
				if _, err := s.db.Exec(`CREATE TRIGGER refuse_story_repair BEFORE UPDATE ON stories BEGIN SELECT RAISE(ABORT,'fixture failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			evidence := "receipt"
			if mode == "missing_evidence" {
				evidence = ""
			}
			repair, err := s.CreateFailureRepair(ctx, "task", evidence, "diagnosed cause")
			if mode == "hitl" {
				if err != nil || repair.ExecutionMode() != TaskModeHITL {
					t.Fatal("HITL lost", err)
				}
				ready, err := RunnableTasks(ctx, s, []string{"story"})
				if err != nil || len(ready) != 0 {
					t.Fatal("HITL automatically selected")
				}
				return
			}
			if err == nil {
				t.Fatal("guard refused no work", mode)
			}
			got, _ := s.GetTask(ctx, "task")
			if !reflect.DeepEqual(got, task) {
				t.Fatal("failed transaction changed original")
			}
			count, _ := s.CountTasks(ctx)
			if count != 1 {
				t.Fatal("orphan repair persisted")
			}
		})
	}
}

func TestRunnableTasksHonorsScopeDependenciesPriorityAndMissingRecords(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	for _, task := range []*Task{{ID: "a", StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, Priority: 3}, {ID: "b", StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, Priority: 1}, {ID: "c", StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, DependsOn: []string{"missing"}}} {
		if err := s.CreateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	ready, err := RunnableTasks(ctx, s, []string{"story"})
	if err != nil || len(ready) != 2 || ready[0].ID != "b" || ready[1].ID != "a" {
		t.Fatal("dependency/priority lost", ready, err)
	}
	ready, err = RunnableTasks(ctx, s, nil)
	if err != nil || len(ready) != 0 {
		t.Fatal("empty scope widened")
	}
	story, _ := s.GetStory(ctx, "story")
	story.DependsOn = []string{"unknown-story"}
	if err := s.UpdateStory(ctx, story); err != nil {
		t.Fatal(err)
	}
	ready, err = RunnableTasks(ctx, s, []string{"story"})
	if err != nil || len(ready) != 0 {
		t.Fatal("story dependency ignored")
	}
}

func TestFailureRepairConcurrentReplayCreatesOneDependency(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.CreateFailureRepair(ctx, "task", "same-evidence", "same diagnosis")
			errors <- err
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	task, _ := s.GetTask(ctx, "task")
	story, _ := s.GetStory(ctx, "story")
	count, _ := s.CountTasks(ctx)
	if count != 2 || len(task.DependsOn) != 1 || len(story.Tasks) != 2 || task.DependsOn[0] != story.Tasks[1] {
		t.Fatal("duplicate repair or dependency", task, story, count)
	}
}
