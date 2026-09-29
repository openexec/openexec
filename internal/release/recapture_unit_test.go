package release

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRecaptureUnitEligibilityPreservesLedger(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	m := &Manager{store: s}
	task, err := s.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := m.RecaptureEligible(ctx, task)
	if err != nil || !eligible {
		t.Fatalf("failed task ineligible: %v %v", eligible, err)
	}
	saved, err := s.GetTask(ctx, "task")
	if err != nil || saved.Status != TaskStatusFailed || saved.AttemptCount != 1 {
		t.Fatalf("selection wrote ledger: %+v %v", saved, err)
	}
	task.DependsOn = []string{"missing"}
	if err := s.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	eligible, err = m.RecaptureEligible(ctx, task)
	if err != nil || eligible {
		t.Fatalf("missing dependency accepted: %v %v", eligible, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RecaptureEligible(ctx, task); err == nil {
		t.Fatal("unreadable ledger accepted")
	}
}

func TestRecaptureUnitSelectionRefusalsAndOrdering(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	// A missing or completed story cannot become runnable through recapture.
	if err := s.CreateStory(ctx, &Story{ID: "absent", Status: StoryStatusDone}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateTask(ctx, &Task{ID: "orphan", StoryID: "absent", Status: TaskStatusPending, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"early", "late"} {
		if err := s.CreateTask(ctx, &Task{ID: id, StoryID: "story", Status: TaskStatusPending, MaxAttempts: 3, CreatedAt: time.Unix(int64(i+1), 0),
			Metadata: map[string]interface{}{"repair_of": "task", "failure_evidence": "unbound"}}); err != nil {
			t.Fatal(err)
		}
	}
	ready, err := runnableTasks(ctx, s, []string{"story", "absent"}, "")
	if err != nil || len(ready) != 2 || ready[0].ID != "early" || ready[1].ID != "late" {
		t.Fatalf("selection order/authority: %+v %v", ready, err)
	}
	// Fail ListStories after successful ListTasks, proving partial reads refuse.
	broken := &recaptureStoryReadFailure{Store: s}
	if _, err := runnableTasks(ctx, broken, []string{"story"}, "task"); err == nil {
		t.Fatal("partial ledger accepted")
	}
}

type recaptureStoryReadFailure struct{ Store }

func (s *recaptureStoryReadFailure) ListStories(context.Context) ([]*Story, error) {
	return nil, fmt.Errorf("unreadable stories")
}
