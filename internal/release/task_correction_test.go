package release

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	persisted, err = CorrectionForTask(task)
	if err != nil || persisted.Outcome != "refused" || !persisted.Consumed {
		t.Fatal("failure disposition not persisted", err)
	}
	if err := fresh.AdmitTaskCorrection(ctx, c); err == nil {
		t.Fatal("failure renewed authority")
	}
	data, _ := json.Marshal(map[string]interface{}{"task": task})
	t.Logf("RELOADED_CORRECTION %s", data)
}

func TestCorrectionInvalidAuthorityAndClosedStore(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	c := TaskCorrection{DecisionRef: "owner", TaskID: "task", EvidenceID: "receipt", CandidatePath: "/candidate", CandidateDigest: "digest", Branch: "feature/retained", PlanID: "plan", StateHash: "hash"}
	for _, mode := range []string{"consumed", "outcome", "fresh", "reason", "missing"} {
		bad := c
		switch mode {
		case "consumed":
			bad.Consumed = true
		case "outcome":
			bad.Outcome = "running"
		case "fresh":
			bad.FreshEvidenceID = "forged"
		case "reason":
			bad.Reason = "forged"
		case "missing":
			bad.DecisionRef = ""
		}
		if err := s.AuthorizeTaskCorrection(ctx, bad); err == nil {
			t.Fatal("invalid authority persisted", mode)
		}
		if mode != "missing" {
			if err := s.AdmitTaskCorrection(ctx, bad); !errors.Is(err, ErrInvalidData) {
				t.Fatalf("invalid disposition must be refused before admission SQL: %s: %v", mode, err)
			}
		}
	}
	task, err := s.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	if task.Metadata["task_correction"] != nil {
		t.Fatal("invalid authority left state")
	}
	if err := s.FinishTaskCorrection(ctx, c, true); err == nil {
		t.Fatal("completion without an accepted validation plan succeeded")
	}
	unchanged, err := s.GetTask(ctx, "task")
	if err != nil || unchanged.Status != task.Status || unchanged.AttemptCount != task.AttemptCount {
		t.Fatal("rejected completion changed history", unchanged, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishTaskCorrection(ctx, c, true); err == nil {
		t.Fatal("closed store accepted finish")
	}
	if err := s.AuthorizeTaskCorrection(ctx, c); err == nil {
		t.Fatal("closed store accepted authorization")
	}
}

func TestCorrectionTaskMoveRoundTrip(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GitEnabled = false
	dir := testDir(t)
	m, err := NewManager(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, id := range []string{"old", "new"} {
		if err := m.CreateStory(&Story{ID: id, Title: id, Status: StoryStatusPending}); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"move", "stay"} {
		if err := m.CreateTask(&Task{ID: id, Title: id, StoryID: "old", Status: TaskStatusPending}); err != nil {
			t.Fatal(err)
		}
	}
	task, err := m.TaskSnapshot(context.Background(), "move")
	if err != nil {
		t.Fatal(err)
	}
	task.StoryID = "new"
	if err := m.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := m.Load(); err != nil {
		t.Fatal(err)
	}
	got, err := m.TaskSnapshot(context.Background(), "move")
	if err != nil || got.StoryID != "new" {
		t.Fatal("task move lost", got, err)
	}
	old := m.GetStory("old")
	next := m.GetStory("new")
	if len(old.Tasks) != 1 || old.Tasks[0] != "stay" || len(next.Tasks) != 1 || next.Tasks[0] != "move" {
		t.Fatal("story membership lost", old, next)
	}
	if err := m.UpdateTask(&Task{ID: "absent"}); err == nil {
		t.Fatal("missing task update accepted")
	}
}

func TestCorrectionRepairRollbackAndHistory(t *testing.T) {
	for _, mode := range []string{"missing", "metadata", "dependencies", "story_tasks", "completed_story", "inherited_branch", "insert_failure", "update_failure", "closed"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := repairFixture(t)
			ctx := context.Background()
			id := "task"
			query := ""
			switch mode {
			case "missing":
				id = "absent"
			case "metadata":
				query = `UPDATE tasks SET metadata='broken' WHERE id='task'`
			case "dependencies":
				query = `UPDATE tasks SET depends_on='broken' WHERE id='task'`
			case "story_tasks":
				query = `UPDATE stories SET tasks='broken'`
			case "completed_story":
				query = `UPDATE stories SET status='done'`
			case "inherited_branch":
				query = `UPDATE tasks SET git_branch='' WHERE id='task'`
			case "insert_failure":
				query = `CREATE TRIGGER refuse_repair BEFORE INSERT ON tasks BEGIN SELECT RAISE(ABORT,'insert refused'); END`
			case "update_failure":
				query = `CREATE TRIGGER refuse_task BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'update refused'); END`
			case "closed":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if query != "" {
				if _, err := s.db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			repair, err := s.CreateFailureRepair(ctx, id, "receipt", "diagnosis")
			if mode == "inherited_branch" {
				if err != nil || repair.Git.Branch != "feature/retained" {
					t.Fatal("candidate branch lost", repair, err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid repair admitted", mode)
			}
			if mode != "closed" {
				var n int
				if err := s.db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&n); err != nil || n != 1 {
					t.Fatal("partial repair persisted", n, err)
				}
			}
		})
	}
}

func TestCorrectionCreationMetadataAndBulkRollback(t *testing.T) {
	s, _ := repairFixture(t)
	ctx := context.Background()
	pr := 42
	for _, bulk := range []bool{false, true} {
		id := "single"
		if bulk {
			id = "bulk"
		}
		task := &Task{ID: id, StoryID: "story", Title: id, Status: TaskStatusPending, NeedsReview: true, Git: &TaskGitInfo{Branch: "feature/retained", PRNumber: &pr}, Metadata: map[string]interface{}{"invalid": make(chan int)}}
		var err error
		if bulk {
			err = s.BulkCreateTasks(ctx, []*Task{{}, task})
		} else {
			err = s.CreateTask(ctx, task)
		}
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.GetTask(ctx, id)
		if err != nil || !got.NeedsReview || got.Git.PRNumber == nil || *got.Git.PRNumber != pr {
			t.Fatal("candidate fields lost", got, err)
		}
		if got.Metadata["task_correction"] != nil || got.Metadata["invalid"] != nil {
			t.Fatal("invalid metadata persisted", got.Metadata)
		}
		if err := s.UpdateTask(ctx, task); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BulkCreateTasks(ctx, []*Task{{ID: "rollback", StoryID: "story", Status: TaskStatusPending}, {ID: "bad", StoryID: "absent", Status: TaskStatusPending}}); err == nil {
		t.Fatal("foreign story admitted")
	}
	if _, err := s.GetTask(ctx, "rollback"); err == nil {
		t.Fatal("partial batch committed")
	}
	if err := s.CreateTask(ctx, &Task{ID: "bad", StoryID: "absent", Status: TaskStatusPending}); err == nil {
		t.Fatal("invalid single task admitted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.BulkCreateTasks(ctx, nil); err == nil {
		t.Fatal("closed store accepted bulk")
	}
}

func TestCorrectionEligibilityRefusesLostStory(t *testing.T) {
	for _, mode := range []string{"missing", "branch", "closed"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := repairFixture(t)
			ctx := context.Background()
			m, err := NewManagerWithDB(t.TempDir(), &Config{}, s.db)
			if err != nil {
				t.Fatal(err)
			}
			defer m.Close()
			task, err := s.GetTask(ctx, "task")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "missing":
				task.StoryID = "absent"
			case "branch":
				task.Git.Branch = "replacement"
			case "closed":
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if eligible, err := m.CorrectionEligible(ctx, task); err == nil || eligible {
				t.Fatal("invalid story admitted", eligible, err)
			}
		})
	}
}
