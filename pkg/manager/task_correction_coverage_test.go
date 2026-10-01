package manager

import (
	"context"
	"github.com/openexec/openexec/internal/loop"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/state"
)

// These are candidate identity contracts, not synthetic coverage increments:
// changing bytes/modes/deletions must change identity, while escaping or
// ambiguous candidates must refuse before any correction authority is spent.
func TestCorrectionCandidateInputs(t *testing.T) {
	for _, mode := range []string{"missing", "not_git", "subdirectory", "detached", "unborn", "deleted", "internal_link", "external_link", "broken_link", "directory_link", "mode", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			f, _, _ := correctionFixture(t)
			m := f.env.mgr
			ctx := context.Background()
			_, _, before, err := m.CorrectionCandidate(ctx)
			if err != nil {
				t.Fatal(err)
			}
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", args...)
				cmd.Dir = f.env.dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, out)
				}
			}
			file := filepath.Join(f.env.dir, "corrected.go")
			wantError := true
			switch mode {
			case "missing":
				m.cfg.WorkDir = filepath.Join(f.env.dir, "missing")
			case "not_git":
				m.cfg.WorkDir = t.TempDir()
			case "subdirectory":
				m.cfg.WorkDir = filepath.Join(f.env.dir, "subdir")
				if err := os.Mkdir(m.cfg.WorkDir, 0755); err != nil {
					t.Fatal(err)
				}
			case "detached":
				git("checkout", "--detach", "HEAD")
			case "unborn":
				git("checkout", "--orphan", "unborn")
			case "deleted":
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
				wantError = false
			case "internal_link":
				if err := os.Symlink("corrected.go", filepath.Join(f.env.dir, "link")); err != nil {
					t.Fatal(err)
				}
				wantError = false
			case "external_link":
				if err := os.Symlink(t.TempDir(), filepath.Join(f.env.dir, "link")); err != nil {
					t.Fatal(err)
				}
			case "broken_link":
				if err := os.Symlink("absent", filepath.Join(f.env.dir, "link")); err != nil {
					t.Fatal(err)
				}
			case "directory_link":
				if err := os.Symlink(".", filepath.Join(f.env.dir, "link")); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if err := os.Chmod(file, 0755); err != nil {
					t.Fatal(err)
				}
				wantError = false
			case "bytes":
				if err := os.WriteFile(file, []byte("package changed\n"), 0644); err != nil {
					t.Fatal(err)
				}
				wantError = false
			}
			_, _, after, err := m.CorrectionCandidate(ctx)
			if wantError {
				if err == nil {
					t.Fatal("invalid candidate accepted")
				}
			} else if err != nil || before == after {
				t.Fatalf("changed candidate identity lost: %s %v", after, err)
			}
		})
	}
}

func TestCorrectionPlanRefusalCoverage(t *testing.T) {
	for _, mode := range []string{"missing_plan", "wrong_hash", "unsupported", "empty", "not_accepted", "named", "optional"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			db := f.env.mgr.state.GetDB()
			var query string
			switch mode {
			case "missing_plan":
				c.PlanID = "absent"
			case "wrong_hash":
				c.StateHash = "wrong"
			case "unsupported":
				query = `UPDATE validation_items SET command_argv='["unsupported"]'`
			case "empty":
				query = `DELETE FROM validation_items`
			case "not_accepted":
				query = `UPDATE validation_plan_revisions SET status='proposed'`
			case "named":
				name, command, err := correctionCheck(state.ValidationItem{Scope: "lint"})
				if err != nil || name != "lint" || command != "" {
					t.Fatal(name, command, err)
				}
				return
			case "optional":
				query = `UPDATE validation_items SET requirement='optional'`
			}
			if query != "" {
				if _, err := db.Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.env.mgr.correctionPlan(context.Background(), c); err == nil {
				t.Fatal("invalid plan accepted")
			}
			if mode == "wrong_hash" {
				if err := f.env.mgr.checkCorrectionCandidate(context.Background(), c); err == nil || !strings.Contains(err.Error(), "validation state mismatch") {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestCorrectionQueueAdmissionErrors(t *testing.T) {
	for _, mode := range []string{"parallel", "active_queue", "active_pipeline", "missing_task_wait", "missing_task_retry", "invalid_authority", "consumed", "missing_receipt"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			m := f.env.mgr
			ctx := context.Background()
			opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
			var err error
			switch mode {
			case "parallel":
				opts.MaxParallel = 2
				err = m.ExecuteTasks(ctx, opts)
			case "active_queue":
				m.taskQueueActive = true
				err = m.ExecuteTasks(ctx, opts)
				m.taskQueueActive = false
			case "active_pipeline":
				m.pipelines["other"] = &entry{info: PipelineInfo{Status: StatusRunning}}
				err = m.ExecuteTasks(ctx, opts)
				delete(m.pipelines, "other")
			case "missing_task_wait":
				err = m.waitTaskQueueRun(ctx, "absent")
			case "missing_task_retry":
				_, err = retryWithStopReason(ctx, f.env.rel, "absent", "reason")
			default:
				task, e := f.env.rel.TaskSnapshot(ctx, "A")
				if e != nil {
					t.Fatal(e)
				}
				switch mode {
				case "invalid_authority":
					c.DecisionRef = ""
				case "consumed":
					c.Consumed = true
				case "missing_receipt":
					c.EvidenceID = "absent"
				}
				task.Metadata["task_correction"] = c
				err = m.reconcileTaskCorrection(ctx, task)
			}
			if err == nil {
				t.Fatal("invalid admission accepted", mode)
			}
			task, e := f.env.rel.TaskSnapshot(ctx, "A")
			if e != nil || task.AttemptCount != 3 || task.Status != "failed" {
				t.Fatal("refusal changed history", task, e)
			}
		})
	}
}

func TestCorrectionLegacyHumanBoundary(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "held", nil)
	task, err := e.rel.TaskSnapshot(context.Background(), "held")
	if err != nil {
		t.Fatal(err)
	}
	task.Metadata = map[string]interface{}{"mode": "hitl"}
	if err := e.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.ExecuteTasks(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.mgr.Status("held"); err == nil {
		t.Fatal("human work dispatched")
	}
	got, err := e.rel.TaskSnapshot(context.Background(), "held")
	if err != nil || got.AttemptCount != 0 || got.Status != "pending" {
		t.Fatal("human boundary lost", got, err)
	}
}

func TestCorrectionEvidenceWriteFailures(t *testing.T) {
	for _, mode := range []string{"step", "link", "disposition"} {
		t.Run(mode, func(t *testing.T) {
			f, c, repair := correctionFixture(t)
			ctx := context.Background()
			if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
				t.Fatal(err)
			}
			query := `CREATE TRIGGER refuse_proof BEFORE INSERT ON run_steps WHEN NEW.id LIKE 'correction-%' BEGIN SELECT RAISE(ABORT,'proof refused'); END`
			if mode == "link" {
				query = `CREATE TRIGGER refuse_proof BEFORE INSERT ON validation_evidence_links BEGIN SELECT RAISE(ABORT,'link refused'); END`
			}
			if mode == "disposition" {
				query = `CREATE TRIGGER refuse_proof BEFORE UPDATE ON tasks WHEN NEW.status='done' BEGIN SELECT RAISE(ABORT,'completion refused'); END`
			}
			if _, err := f.env.mgr.state.GetDB().Exec(query); err != nil {
				t.Fatal(err)
			}
			if err := boundaryRun(f); err == nil {
				t.Fatal("missing evidence completed task")
			}
			assertCorrectionReload(t, f, c, repair, false)
			if err := boundaryRun(f); err == nil {
				t.Fatal("failed evidence silently renewed allowance")
			}
		})
	}
}

func TestCorrectionRefusesLostStoreAndWriter(t *testing.T) {
	for _, mode := range []string{"writer", "candidate", "receipt_store", "failure_store"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			m := f.env.mgr
			ctx := context.Background()
			switch mode {
			case "writer":
				lock, err := m.lockTaskExecution(true)
				if err != nil {
					t.Fatal(err)
				}
				defer lock.Close()
				if err := m.AuthorizeTaskCorrection(ctx, c); err == nil {
					t.Fatal("competing writer admitted")
				}
			case "candidate":
				m.cfg.WorkDir = t.TempDir()
				if err := m.checkCorrectionCandidate(ctx, c); err == nil {
					t.Fatal("non-git candidate accepted")
				}
			case "receipt_store":
				task, err := f.env.rel.TaskSnapshot(ctx, "A")
				if err != nil {
					t.Fatal(err)
				}
				task.Metadata["task_correction"] = c
				f.env.closeState()
				if err := m.reconcileTaskCorrection(ctx, task); err == nil {
					t.Fatal("unreadable receipt admitted")
				}
			case "failure_store":
				step := state.RunStepData{ID: "failure", RunID: "A", Status: "failed"}
				if err := m.state.RecordTaskFailureStep(ctx, state.RunStepData{}, 0); err == nil {
					t.Fatal("invalid binding persisted")
				}
				if _, err := m.state.GetDB().Exec(`UPDATE tasks SET status='in_progress' WHERE id='A'`); err != nil {
					t.Fatal(err)
				}
				if _, err := m.state.GetDB().Exec(`CREATE TRIGGER refuse_failure BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'failure binding refused'); END`); err != nil {
					t.Fatal(err)
				}
				if err := m.state.RecordTaskFailureStep(ctx, step, 3); err == nil {
					t.Fatal("failure persistence bypassed")
				}
				var status string
				if err := m.state.GetDB().QueryRow(`SELECT status FROM tasks WHERE id='A'`).Scan(&status); err != nil || status != "in_progress" {
					t.Fatal("failed binding changed task", status, err)
				}
				if step, err := m.state.GetRunStep(ctx, "failure"); err != nil || step != nil {
					t.Fatal("failed binding persisted receipt", step, err)
				}
				if _, err := m.state.GetDB().Exec(`DROP TRIGGER refuse_failure`); err != nil {
					t.Fatal(err)
				}
				f.env.closeState()
				if err := m.state.RecordTaskFailureStep(ctx, step, 3); err == nil {
					t.Fatal("closed store accepted failure")
				}
			}
		})
	}
}

func TestCorrectionLateEventStatus(t *testing.T) {
	for _, test := range []struct {
		event  loop.Event
		stage  string
		status PipelineStatus
		reason string
	}{
		{loop.Event{Type: loop.EventPlanningMismatch, Text: "changed plan"}, "", StatusPaused, "Planning Mismatch: changed plan"},
		{loop.Event{Type: loop.EventStageRetry, StageName: "verify", Attempt: 2}, "verify:retry", StatusRunning, ""},
		{loop.Event{Type: loop.EventCheckpointCreated, StageName: "verify"}, "verify:checkpoint", StatusRunning, ""},
	} {
		info := PipelineInfo{Status: StatusRunning}
		updateInfo(&info, test.event)
		if info.Status != test.status || info.Stage != test.stage || info.Error != test.reason {
			t.Fatal("event disposition lost", info)
		}
		stopped := PipelineInfo{Status: StatusStopped}
		updateInfo(&stopped, test.event)
		if stopped.Status != StatusStopped || stopped.Stage != "" || stopped.Error != "" {
			t.Fatal("late event undid Stop", stopped)
		}
	}
}

func TestCorrectionOptionalChecksAndTimeout(t *testing.T) {
	f, c, repair := correctionFixture(t)
	f.env.mgr.cfg.TaskTimeout = time.Second
	if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE validation_items SET requirement='optional' WHERE id='second'`); err != nil {
		t.Fatal(err)
	}
	if err := f.env.mgr.AuthorizeTaskCorrection(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := boundaryRun(f); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("optional check dispatched: %d checks", f.calls)
	}
	assertCorrectionReload(t, f, c, repair, true)
}
