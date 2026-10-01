package release

import (
	"context"
	"database/sql"
	"sync"
	"testing"
)

func TestCorrectionAdmissionPersistence(t *testing.T) {
	s, path := repairFixture(t)
	ctx := context.Background()
	_, err := s.db.Exec(`UPDATE tasks SET attempt_count=3,metadata=json_set(metadata,'$.verification_failure_evidence','receipt') WHERE id='task'`)
	if err != nil {
		t.Fatal(err)
	}
	c := TaskCorrection{DecisionRef: "owner:correction", TaskID: "task", EvidenceID: "receipt", CandidatePath: "/retained", CandidateDigest: "exact-bytes", Branch: "feature/retained", PlanID: "accepted", StateHash: "state"}

	untrusted, err := s.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	untrusted.Metadata["task_correction"] = c
	if err := s.UpdateTask(ctx, untrusted); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"injected-single", "injected-bulk"} {
		injected := *untrusted
		injected.ID = id
		var err error
		if id == "injected-single" {
			err = s.CreateTask(ctx, &injected)
		} else {
			err = s.BulkCreateTasks(ctx, []*Task{&injected})
		}
		if err != nil {
			t.Fatal(err)
		}
		persisted, err := s.GetTask(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.Metadata["task_correction"] != nil {
			t.Fatal("ordinary creation injected correction authority")
		}
	}

	if err := s.AdmitTaskCorrection(ctx, c); err == nil {
		t.Fatal("absent authority admitted")
	}
	if err := s.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	bad := c
	bad.CandidateDigest = "replacement"
	if err := s.AdmitTaskCorrection(ctx, bad); err == nil {
		t.Fatal("mismatch admitted")
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.AdmitTaskCorrection(ctx, c) }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("admissions=%d", successes)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	task, err := fresh.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	got, err := CorrectionForTask(task)
	if err != nil || !got.Consumed || got.Outcome != "running" || task.AttemptCount != 3 || task.MaxAttempts != 3 || task.Git.Commits[0] != "old-commit" {
		t.Fatalf("reloaded admission: %+v %+v %v", task, got, err)
	}

	stale := *task
	stale.Metadata = map[string]interface{}{"task_correction": c}
	if err := fresh.UpdateTask(ctx, &stale); err != nil {
		t.Fatal(err)
	}

	stale.Metadata = nil
	if err := fresh.UpdateTask(ctx, &stale); err != nil {
		t.Fatal(err)
	}
	preserved, err := fresh.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := CorrectionForTask(preserved)
	if err != nil || !persisted.Consumed {
		t.Fatal("ordinary update refunded allowance", err)
	}
	if err := fresh.AdmitTaskCorrection(ctx, c); err == nil {
		t.Fatal("restart refunded allowance")
	}
	if err := fresh.AuthorizeTaskCorrection(ctx, c); err == nil {
		t.Fatal("replayed grant refunded allowance")
	}
	if err := fresh.FinishTaskCorrection(ctx, c, false); err != nil {
		t.Fatal(err)
	}
	task, err = fresh.GetTask(ctx, "task")
	if err != nil || task.Status != TaskStatusNeedsReview {
		t.Fatalf("disposition: %+v %v", task, err)
	}
	t.Logf("RELOADED_CORRECTION admission consumed=%v history=%d/%d candidate=%s", got.Consumed, task.AttemptCount, task.MaxAttempts, got.CandidateDigest)
}
