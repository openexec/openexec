package release

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"
)

func TestCorrectionRefusalReopenAndDecisionHistory(t *testing.T) {
	s, path := repairFixture(t)
	ctx := context.Background()
	if _, err := s.db.Exec("UPDATE tasks SET attempt_count=3,metadata=json_set(metadata,'$.verification_failure_evidence','receipt') WHERE id='task'"); err != nil {
		t.Fatal(err)
	}
	c := TaskCorrection{DecisionRef: "first", TaskID: "task", EvidenceID: "receipt", CandidatePath: "/candidate", CandidateDigest: "digest", Branch: "feature/retained", PlanID: "plan", StateHash: "hash"}
	if err := s.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	task, err := s.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RefuseTaskCorrection(ctx, task, "candidate mismatch"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s, err = NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	task, err = s.GetTask(ctx, "task")
	got, e := CorrectionForTask(task)
	if err != nil || e != nil || !CorrectionRefused(task) || got.Reason != "candidate mismatch" || task.Status != "failed" || task.AttemptCount != 3 || task.MaxAttempts != 3 || task.Metadata["verification_failure_evidence"] != "receipt" {
		t.Fatal(task, got, err, e)
	}
	if err := s.AuthorizeTaskCorrection(ctx, c); err == nil {
		t.Fatal("replay accepted")
	}
	next := c
	next.DecisionRef = "second"
	if err := s.AuthorizeTaskCorrection(ctx, next); err != nil {
		t.Fatal(err)
	}
	// Ordinary worker updates cannot remove or forge decision history.
	task, err = s.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	task.Metadata = map[string]interface{}{"verification_failure_evidence": "receipt"}
	if err := s.UpdateTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	task, err = s.GetTask(ctx, "task")
	if err != nil {
		t.Fatal(err)
	}
	history := task.Metadata["task_correction_history"].([]interface{})
	if len(history) != 1 || history[0].(map[string]interface{})["decision_ref"] != "first" {
		t.Fatal("history lost", history)
	}
	if err := s.RefuseTaskCorrection(ctx, task, "second refusal"); err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeTaskCorrection(ctx, c); err == nil {
		t.Fatal("historical decision reused")
	}
	third := next
	third.DecisionRef = "third"
	if err := s.AuthorizeTaskCorrection(ctx, third); err != nil {
		t.Fatal(err)
	}
	if err := s.AdmitTaskCorrection(ctx, third); err != nil {
		t.Fatal(err)
	}
	if err := s.FailTaskCorrection(ctx, third, "refused", "admitted failure"); err != nil {
		t.Fatal(err)
	}
	// Even forcibly returning a spent task to failed cannot renew an admission.
	if _, err := s.db.Exec("UPDATE tasks SET status='failed' WHERE id='task'"); err != nil {
		t.Fatal(err)
	}
	fourth := third
	fourth.DecisionRef = "fourth"
	if err := s.AuthorizeTaskCorrection(ctx, fourth); err == nil {
		t.Fatal("admitted refusal replaced")
	}
}

func TestCorrectionAdmissionRefusalRace(t *testing.T) {
	for i := 0; i < 8; i++ {
		s, _ := repairFixture(t)
		ctx := context.Background()
		if _, err := s.db.Exec("UPDATE tasks SET attempt_count=3,metadata=json_set(metadata,'$.verification_failure_evidence','receipt') WHERE id='task'"); err != nil {
			t.Fatal(err)
		}
		c := TaskCorrection{DecisionRef: "race", TaskID: "task", EvidenceID: "receipt", CandidatePath: "/candidate", CandidateDigest: "digest", Branch: "feature/retained", PlanID: "plan", StateHash: "hash"}
		if err := s.AuthorizeTaskCorrection(ctx, c); err != nil {
			t.Fatal(err)
		}
		task, err := s.GetTask(ctx, "task")
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; results <- s.AdmitTaskCorrection(ctx, c) }()
		go func() { defer wg.Done(); <-start; results <- s.RefuseTaskCorrection(ctx, task, "drift") }()
		close(start)
		wg.Wait()
		close(results)
		successes := 0
		for err := range results {
			if err == nil {
				successes++
			}
		}
		if successes != 1 {
			t.Fatalf("race winners=%d", successes)
		}
		after, err := s.GetTask(ctx, "task")
		if err != nil || after.AttemptCount != 3 {
			t.Fatal(after, err)
		}
		if err := s.AdmitTaskCorrection(ctx, c); err == nil {
			t.Fatal("double admission")
		}
	}
}

func TestCorrectionRefusalReplacementGuards(t *testing.T) {
	for _, mode := range []string{"fresh", "stale_snapshot", "write_failure", "closed", "concurrent_authorization"} {
		t.Run(mode, func(t *testing.T) {
			s, _ := repairFixture(t)
			ctx := context.Background()
			if _, err := s.db.Exec("UPDATE tasks SET attempt_count=3,metadata=json_set(metadata,'$.verification_failure_evidence','receipt') WHERE id='task'"); err != nil {
				t.Fatal(err)
			}
			c := TaskCorrection{DecisionRef: "first", TaskID: "task", EvidenceID: "receipt", CandidatePath: "/candidate", CandidateDigest: "digest", Branch: "feature/retained", PlanID: "plan", StateHash: "hash"}
			if err := s.AuthorizeTaskCorrection(ctx, c); err != nil {
				t.Fatal(err)
			}
			task, err := s.GetTask(ctx, "task")
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "stale_snapshot":
				if _, err := s.db.Exec("UPDATE tasks SET metadata=json_set(metadata,'$.task_correction.decision_ref','newer') WHERE id='task'"); err != nil {
					t.Fatal(err)
				}
			case "write_failure":
				if _, err := s.db.Exec("CREATE TRIGGER fail_refusal BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'store failure'); END"); err != nil {
					t.Fatal(err)
				}
			case "closed":
				s.Close()
			default:
				if err := s.RefuseTaskCorrection(ctx, task, "invalid"); err != nil {
					t.Fatal(err)
				}
				next := c
				next.DecisionRef = "next"
				if mode == "fresh" {
					if _, err := s.db.Exec("UPDATE tasks SET metadata=json_set(metadata,'$.task_correction.fresh_evidence_id','fresh') WHERE id='task'"); err != nil {
						t.Fatal(err)
					}
					if err := s.AuthorizeTaskCorrection(ctx, next); err == nil {
						t.Fatal("fresh evidence refunded")
					}
				} else {
					results := make(chan error, 2)
					var wg sync.WaitGroup
					for i := 0; i < 2; i++ {
						wg.Add(1)
						go func() { defer wg.Done(); results <- s.AuthorizeTaskCorrection(ctx, next) }()
					}
					wg.Wait()
					close(results)
					wins := 0
					for err := range results {
						if err == nil {
							wins++
						}
					}
					if wins != 1 {
						t.Fatal("replacement race", wins)
					}
					got, err := s.GetTask(ctx, "task")
					if err != nil {
						t.Fatal(err)
					}
					data, _ := json.Marshal(got.Metadata["task_correction_history"])
					var history []TaskCorrection
					if err := json.Unmarshal(data, &history); err != nil || len(history) != 1 || history[0].DecisionRef != "first" {
						t.Fatal(string(data), err)
					}
				}
				return
			}
			if err := s.RefuseTaskCorrection(ctx, task, "invalid"); err == nil {
				t.Fatal("unsafe refusal succeeded")
			}
		})
	}
}
