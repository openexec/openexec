package release

import (
	"context"
	"database/sql"
	"fmt"
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

func TestFailureClosesTaskAndItsFixRunsFirstAcrossRestart(t *testing.T) {
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
	// The fix runs first, even before higher-priority work; the closed
	// task is not runnable.
	if err != nil || len(ready) != 2 || ready[0].ID != repair.ID || ready[1].ID != "peer" {
		t.Fatal("fix does not run first", ready, err)
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
	// Closed, not reopened: the failed task keeps everything it had, and
	// only its closing record names the fix.
	retained, _ := reopened.GetTask(ctx, "task")
	if retained.Status != TaskStatusFailed || retained.Metadata["fixed_by"] != repair.ID || retained.ErrorMessage != original.ErrorMessage ||
		!reflect.DeepEqual(retained.Git, original.Git) || retained.AttemptCount != original.AttemptCount || !reflect.DeepEqual(retained.DependsOn, original.DependsOn) {
		t.Fatal("failed task not closed as it failed", retained)
	}
	if again.Metadata["repair_of"] != "task" || again.Metadata["fix_of"] != "task" || again.TaskType != "fix" {
		t.Fatal("fix does not name its original", again.Metadata)
	}
	again.Status = TaskStatusDone
	if err := reopened.UpdateTask(ctx, again); err != nil {
		t.Fatal(err)
	}
	ready, err = RunnableTasks(ctx, reopened, []string{"story"})
	if err != nil || len(ready) != 1 || ready[0].ID != "peer" {
		t.Fatal("closed task ran again after its fix", ready, err)
	}
	retained, _ = reopened.GetTask(ctx, "task")
	tasks, _ := reopened.ListTasks(ctx)
	byID := map[string]*Task{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	if retained.Status != TaskStatusFailed || !Delivered(retained, byID) {
		t.Fatal("closed task rewritten, or not delivered by its passed fix", retained)
	}
}

// A fix that fails its check is closed like any task and the next fix
// follows for the same original task, until the original's budget is spent.
// Work that waits on the original runs once the chain ends in a passed fix;
// no task in the chain is ever changed after it closed.
func TestFixChainClosesFailedFixesAndDeliversThroughTheLastFix(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	if err := s.CreateTask(ctx, &Task{ID: "after", StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, DependsOn: []string{"task"}}); err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateFailureRepair(ctx, "task", "evidence:1", "fix one")
	if err != nil {
		t.Fatal(err)
	}
	first.Status = TaskStatusFailed
	if err := s.UpdateTask(ctx, first); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateFailureRepair(ctx, first.ID, "evidence:2", "fix two")
	if err != nil {
		t.Fatal(err)
	}
	if second.Metadata["repair_of"] != "task" || second.Metadata["fix_of"] != first.ID {
		t.Fatal("next fix lost its original", second.Metadata)
	}
	closedFirst, _ := s.GetTask(ctx, first.ID)
	if closedFirst.Status != TaskStatusFailed || closedFirst.Metadata["fixed_by"] != second.ID {
		t.Fatal("failed fix not closed with the next fix", closedFirst)
	}
	ready, err := RunnableTasks(ctx, s, []string{"story"})
	if err != nil || len(ready) != 1 || ready[0].ID != second.ID {
		t.Fatal("dependent ran before the chain passed, or next fix not first", ready, err)
	}
	second.Status = TaskStatusDone
	if err := s.UpdateTask(ctx, second); err != nil {
		t.Fatal(err)
	}
	ready, err = RunnableTasks(ctx, s, []string{"story"})
	if err != nil || len(ready) != 1 || ready[0].ID != "after" {
		t.Fatal("dependent did not run after the chain passed", ready, err)
	}
	original, _ := s.GetTask(ctx, "task")
	closedFirst, _ = s.GetTask(ctx, first.ID)
	if original.Status != TaskStatusFailed || closedFirst.Status != TaskStatusFailed {
		t.Fatal("a closed task in the chain was rewritten")
	}
	// The budget is the original's max_attempts (3): a third failed fix
	// gets one more, a fourth is refused.
	second.Status = TaskStatusFailed
	if err := s.UpdateTask(ctx, second); err != nil {
		t.Fatal(err)
	}
	third, err := s.CreateFailureRepair(ctx, second.ID, "evidence:3", "fix three")
	if err != nil {
		t.Fatal(err)
	}
	third.Status = TaskStatusFailed
	if err := s.UpdateTask(ctx, third); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFailureRepair(ctx, third.ID, "evidence:4", "fix four"); err == nil {
		t.Fatal("fix chain outran the original task's budget")
	}
	if _, err := s.CreateFailureRepair(ctx, first.ID, "evidence:5", "fix again"); err == nil {
		t.Fatal("a task already closed with a fix was fixed twice")
	}
}

func TestFailureRepairGuardsAndTransactionRollback(t *testing.T) {
	for _, mode := range []string{"in_progress", "pending", "limit", "hitl", "branch", "missing_evidence", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := repairFixture(t)
			ctx := context.Background()
			task, _ := s.GetTask(ctx, "task")
			switch mode {
			case "in_progress":
				task.Status = TaskStatusInProgress
			case "pending":
				task.Status = TaskStatusPending
			case "limit":
				task.MaxAttempts = 0
			case "hitl":
				task.Metadata["mode"] = TaskModeHITL
				task.Metadata["decision_reason"] = "Owner must grant access"
				task.Metadata["decision_ref"] = "decision:access"
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
				if err != nil || repair.ExecutionMode() != TaskModeHITL || repair.Metadata["decision_reason"] != task.Metadata["decision_reason"] || repair.Metadata["decision_ref"] != task.Metadata["decision_ref"] {
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

func TestFailureRepairConcurrentReplayCreatesOneFix(t *testing.T) {
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
	if count != 2 || len(task.DependsOn) != 0 || len(story.Tasks) != 2 || task.Metadata["fixed_by"] != story.Tasks[1] {
		t.Fatal("duplicate fix, or closed task rewritten", task, story, count)
	}
}

// A finding on delivered work is a new fix task; the done task it names is
// never changed, and the fix runs before other work.
func TestFixOfDoneTaskLeavesItUnchanged(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	task, _ := s.GetTask(ctx, "task")
	task.Status = TaskStatusDone
	if err := s.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	before, _ := s.GetTask(ctx, "task")
	if err := s.CreateTask(ctx, &Task{ID: "peer", StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, Priority: -1}); err != nil {
		t.Fatal(err)
	}
	var fixes []string
	for i := 1; i <= 4; i++ { // more findings than the failure budget
		fix, err := s.CreateFailureRepair(ctx, "task", fmt.Sprintf("review-finding:%d", i), fmt.Sprintf("finding %d", i))
		if err != nil {
			t.Fatal(err)
		}
		fixes = append(fixes, fix.ID)
	}
	after, _ := s.GetTask(ctx, "task")
	if !reflect.DeepEqual(after, before) {
		t.Fatal("done task changed by a finding", after)
	}
	ready, err := RunnableTasks(ctx, s, []string{"story"})
	if err != nil || len(ready) != 5 || ready[4].ID != "peer" {
		t.Fatal("fixes of findings do not run first", ready, err)
	}
}
