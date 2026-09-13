package release

import (
	"context"
	"database/sql"
	"reflect"
	"testing"
)

func TestTaskLocalUpdateDoesNotReplayOtherManagerHistory(t *testing.T) {
	s, path := repairFixture(t)
	ctx := context.Background()
	if err := s.CreateTask(ctx, &Task{ID: "other", StoryID: "story", Title: "other task", Status: TaskStatusPending, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	one, err := NewManagerWithDB(t.TempDir(), &Config{}, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := one.Load(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	two, err := NewManagerWithDB(t.TempDir(), &Config{}, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := two.Load(); err != nil {
		t.Fatal(err)
	}
	other, err := two.TaskSnapshot(ctx, "other")
	if err != nil {
		t.Fatal(err)
	}
	other.Status = TaskStatusDone
	other.Metadata = map[string]interface{}{"evidence": "worker receipt"}
	if err := two.UpdateTask(other); err != nil {
		t.Fatal(err)
	}
	selected, err := one.TaskSnapshot(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	selected.Status = TaskStatusInProgress
	if err := one.UpdateTask(selected); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTask(ctx, "other")
	if err != nil || !reflect.DeepEqual(got, other) {
		t.Fatal("stale manager overwrote unrelated durable work", got, err)
	}
	// A worker's newer observation on the selected task must also survive a
	// later controller status-only update from its older cached task pointer.
	selected, err = two.TaskSnapshot(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	selected.Metadata["new_observation"] = "verified running revision"
	if err := two.UpdateTask(selected); err != nil {
		t.Fatal(err)
	}
	if err := one.SetTaskStatus("task", TaskStatusPending); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetTask(ctx, "task")
	if err != nil || got.Metadata["new_observation"] != "verified running revision" {
		t.Fatal("status-only update discarded latest evidence", got, err)
	}
}

func TestTaskLocalUpdateFailureDoesNotAdvanceCache(t *testing.T) {
	s, _ := repairFixture(t)
	m, err := NewManagerWithDB(t.TempDir(), &Config{}, s.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	before, err := m.TaskSnapshot(context.Background(), "task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_task_update BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'fixture refusal'); END`); err != nil {
		t.Fatal(err)
	}
	updated := *before
	updated.Status = TaskStatusInProgress
	if err := m.UpdateTask(&updated); err == nil {
		t.Fatal("expected task persistence failure")
	}
	if !reflect.DeepEqual(m.GetTask("task"), before) {
		t.Fatal("failed update changed cache")
	}
	got, err := s.GetTask(context.Background(), "task")
	if err != nil || !reflect.DeepEqual(got, before) {
		t.Fatal("failed update changed durable task")
	}
}
