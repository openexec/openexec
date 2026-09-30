package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

func TestRecaptureUnitResolutionAndEvidence(t *testing.T) {
	for _, mode := range []string{"script", "verification", "unknown", "invalid", "mismatch", "unregistered", "wrong-path", "legacy-path", "unreadable", "foreign", "argv", "empty-command", "ambiguous", "private", "silent", "bad-exit", "output", "diagnostics"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureFixture(t, "exit 0")
			task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			phase := "verify"
			if mode == "verification" {
				phase = mode
			}
			refs := recaptureReceipt(phase, 2)
			wantError, free := false, true
			switch mode {
			case "unknown":
				phase = "unknown"
				refs = recaptureReceipt(phase, 2)
				wantError = true
			case "invalid":
				refs["verification_failure_receipt"] = "invalid"
				wantError = true
			case "mismatch":
				phase = "test"
				wantError = true
			case "output":
				refs["stage_output"] = "observed diagnostic"
				free = false
			case "diagnostics":
				refs["stage_diagnostics"] = "observed diagnostic"
				free = false
			case "script", "verification":
			default:
				ev := runtime.CommandEvidence{Argv: []string{"sh", "-c", "exit 2"}, Cwd: f.env.dir, ExitCode: 2}
				if mode == "foreign" {
					ev.Cwd = "/foreign"
				}
				if mode == "empty-command" {
					ev.Argv[2] = ""
				}
				if mode == "argv" {
					ev.Argv = []string{"go", "test"}
				}
				if mode == "bad-exit" {
					ev.ExitCode = 126
				}
				hash, path, err := runtime.RetainCommandEvidence(f.env.dir, ev)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "legacy-path" {
					path = filepath.Join(f.env.dir, ".openexec-verification", hash+".json")
				}
				refs[hash] = path
				if mode != "unregistered" {
					if err := f.env.mgr.state.RecordArtifact(context.Background(), hash, "test_log", path, 0); err != nil {
						t.Fatal(err)
					}
				}
				free = mode == "unregistered" || mode == "wrong-path" || mode == "legacy-path" || mode == "unreadable" || mode == "bad-exit"
				wantError = mode == "unregistered" || mode == "wrong-path" || mode == "legacy-path" || mode == "unreadable" || mode == "foreign" || mode == "argv" || mode == "empty-command" || mode == "ambiguous"
				if mode == "wrong-path" {
					refs[hash] = "wrong"
				}
				if mode == "unreadable" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "ambiguous" {
					ev.Argv[2] = "exit 3"
					h, p, e := runtime.RetainCommandEvidence(f.env.dir, ev)
					if e != nil {
						t.Fatal(e)
					}
					if e := f.env.mgr.state.RecordArtifact(context.Background(), h, "test_log", p, 0); e != nil {
						t.Fatal(e)
					}
					refs[h] = p
				}
			}
			got, err := f.env.mgr.resolveRecaptureCommand(task, phase, refs)
			if (err != nil) != wantError {
				t.Fatalf("resolution %q: %v, want error %v", got, err, wantError)
			}
			if !wantError && strings.TrimSpace(got) == "" {
				t.Fatal("empty authority")
			}
			if actual := f.env.mgr.diagnosticFreeReceipt(refs); actual != free {
				t.Fatalf("diagnostic free=%v want %v", actual, free)
			}
		})
	}
}

func TestRecaptureUnitOwnershipAndEligibility(t *testing.T) {
	for _, mode := range []string{"no-owner", "cancelled", "missing", "stale", "review", "exhausted", "waiting"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureFixture(t, "exit 0")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.env.mgr.taskQueueActive = mode != "no-owner"
			task, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			id, evidence := "A", "legacy"
			switch mode {
			case "cancelled":
				cancel()
			case "missing":
				id = "absent"
			case "stale":
				evidence = "stale"
			case "review":
				task.NeedsReview = true
			case "exhausted":
				task.MaxAttempts = 1
			case "waiting":
				task.DependsOn = []string{"Settings"}
			}
			if mode != "cancelled" {
				if err := f.env.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
			}
			err = f.env.mgr.recaptureTaskFailure(ctx, id, evidence, "verify", recaptureReceipt("verify", 2))
			if err == nil {
				t.Fatal("invalid recapture accepted")
			}
			if mode == "waiting" && err != errRecaptureWaiting {
				t.Fatal(err)
			}
			if f.calls != 0 {
				t.Fatal("refused attempt dispatched")
			}
			f.restart(t)
			got, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil || got.AttemptCount != 1 {
				t.Fatalf("attempt consumed: %+v %v", got, err)
			}
			if mode == "review" {
				f.assertTerminal(t, "refused", 1)
			}
			if mode == "exhausted" {
				f.assertTerminal(t, "exhausted", 1)
			}
			if mode == "waiting" && got.Status != release.TaskStatusFailed {
				t.Fatal("waiting changed state")
			}
		})
	}
}

// SQLite triggers inject write failures at the actual durable ownership boundary.
func TestRecaptureUnitPersistenceRefusals(t *testing.T) {
	for _, mode := range []string{"claim-error", "claim-ignored", "finish-error", "finish-ignored", "start-refused", "release-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureFixture(t, "exit 0")
			m := f.env.mgr
			m.taskQueueActive = true
			m.cfg.TaskTimeout = time.Second
			query := ""
			switch mode {
			case "claim-error":
				query = "CREATE TRIGGER refusal BEFORE UPDATE OF attempt_count ON tasks BEGIN SELECT RAISE(ABORT, 'claim refused'); END"
			case "claim-ignored":
				query = "CREATE TRIGGER refusal BEFORE UPDATE OF attempt_count ON tasks BEGIN SELECT RAISE(IGNORE); END"
			case "finish-error":
				query = "CREATE TRIGGER refusal BEFORE UPDATE OF status ON tasks WHEN NEW.status='needs_review' BEGIN SELECT RAISE(ABORT, 'finish refused'); END"
			case "finish-ignored":
				query = "CREATE TRIGGER refusal BEFORE UPDATE OF status ON tasks WHEN NEW.status='needs_review' BEGIN SELECT RAISE(IGNORE); END"
			case "start-refused":
				m.pipelines["A"] = &entry{info: PipelineInfo{Status: StatusRunning}}
			case "release-unavailable":
				m.rel = nil
				f.env.closeState()
			}
			if query != "" {
				if _, err := m.state.GetDB().Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			if strings.HasPrefix(mode, "finish") {
				task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
				if err != nil {
					t.Fatal(err)
				}
				task.NeedsReview = true
				if err := f.env.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
			}
			err := m.recaptureTaskFailure(context.Background(), "A", "legacy", "verify", recaptureReceipt("verify", 2))
			if err == nil {
				t.Fatal("persistence refusal accepted")
			}
			if f.calls != 0 {
				t.Fatal("refusal dispatched command")
			}
			delete(m.pipelines, "A")
			if mode == "release-unavailable" {
				return
			}
			if query != "" {
				if _, err := m.state.GetDB().Exec("DROP TRIGGER refusal"); err != nil {
					t.Fatal(err)
				}
			}
			f.restart(t)
			task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			want := 1
			if mode == "start-refused" {
				want = 2
				f.assertTerminal(t, "refused", want)
			}
			if task.AttemptCount != want {
				t.Fatalf("attempt count %d want %d", task.AttemptCount, want)
			}
		})
	}
}

func TestRecaptureUnitQueueGuards(t *testing.T) {
	for _, mode := range []string{"parallel", "pipeline", "empty", "waiting", "options", "update-refused", "reconcile-refused"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureFixture(t, "exit 0")
			m := f.env.mgr
			opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
			switch mode {
			case "parallel":
				opts.MaxParallel = 2
			case "pipeline":
				m.pipelines["other"] = &entry{info: PipelineInfo{Status: StatusRunning}}
			case "empty":
				if _, err := m.state.GetDB().Exec("DELETE FROM tasks"); err != nil {
					t.Fatal(err)
				}
			case "waiting":
				task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
				if err != nil {
					t.Fatal(err)
				}
				task.DependsOn = []string{"Settings"}
				if err := f.env.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
			default:
				task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
				if err != nil {
					t.Fatal(err)
				}
				task.Status = release.TaskStatusPending
				if mode == "reconcile-refused" {
					task.Status = release.TaskStatusInProgress
				}
				task.Metadata = map[string]interface{}{}
				if err := f.env.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
				if mode == "options" {
					opts.IsStudy = true
					opts.Mode = "implement"
					f.mode = "success"
				} else {
					if _, err := m.state.GetDB().Exec("CREATE TRIGGER refusal BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT, 'write refused'); END"); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := m.ExecuteTasks(context.Background(), opts)
			delete(m.pipelines, "other")
			if mode == "options" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("queue guard accepted")
			}
		})
	}
}

func TestRecaptureUnitAttemptRoundTrip(t *testing.T) {
	for _, mode := range []string{"success", "failure", "exhausted", "refusal", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			command := "printf diagnostic; exit 2"
			if mode == "success" {
				command = "exit 0"
			}
			f := newRecaptureFixture(t, command)
			f.mode = mode
			// Model a process interrupted after its second claim. Reconciliation must
			// preserve that debit, leaving precisely one attempt after reopen.
			task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			task.AttemptCount = 2
			task.Status = release.TaskStatusInProgress
			task.Metadata["recapture_outcome"] = "running"
			if mode == "failure" {
				task.MaxAttempts = 4
			}
			if err := f.env.rel.UpdateTask(task); err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			if err := f.env.mgr.reconcileInterruptedTasks(context.Background(), f.env.rel, []string{"S"}); err != nil {
				t.Fatal(err)
			}
			f.env.mgr.taskQueueActive = true
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.cancel = cancel
			err = f.env.mgr.recaptureTaskFailure(ctx, "A", "legacy", "verify", recaptureReceipt("verify", 2))
			if (err == nil) != (mode == "success" || mode == "failure") {
				t.Fatalf("unexpected disposition: %v", err)
			}
			f.restart(t)
			got, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			if f.calls != 1 || got.AttemptCount != 3 {
				t.Fatalf("dispatch/debit: %d %+v", f.calls, got)
			}
			switch mode {
			case "success":
				if got.Status != release.TaskStatusPending || got.Metadata["verification_failure_evidence"] != nil {
					t.Fatal("success retained obsolete receipt", got)
				}
			case "failure":
				if got.Status != release.TaskStatusFailed || got.Metadata["verification_failure_evidence"] == "legacy" {
					t.Fatal("failure did not bind fresh evidence", got)
				}
			default:
				outcome := mode
				if mode == "refusal" {
					outcome = "refused"
				}
				if mode == "cancel" {
					outcome = "cancelled"
				}
				f.assertTerminal(t, outcome, 3)
				if err := f.env.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
					t.Fatal("terminal restart resumed")
				}
				if f.calls != 1 {
					t.Fatal("terminal restart refunded budget")
				}
			}
		})
	}
}

func TestRecaptureUnitInvalidReceipt(t *testing.T) {
	f := newRecaptureFixture(t, "exit 0")
	e := f.env
	ctx := context.Background()
	refs := recaptureReceipt("verify", 2)
	payload, err := json.Marshal(refs)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, run, status, agent, metadata string }{
		{"wrong-run", "A", "failed", "deterministic-verification", string(payload)},
		{"wrong-status", "A", "completed", "deterministic-verification", string(payload)},
		{"wrong-agent", "A", "failed", "worker", string(payload)},
		{"bad-json", "A", "failed", "deterministic-verification", "{"},
		{"forged", "A", "failed", "deterministic-verification", `{"verification_failure_digest":"forged"}`},
	} {
		if err := e.mgr.state.AddRunStepFull(ctx, tc.name, tc.run, "", "test", tc.agent, 1, tc.status, "", tc.metadata); err != nil {
			t.Fatal(err)
		}
		taskID := "A"
		if tc.name == "wrong-run" {
			taskID = "B"
		}
		if err := e.mgr.repairTaskFromRetainedFailure(ctx, taskID, tc.name); err == nil {
			t.Fatal("invalid evidence accepted", tc.name)
		}
	}
	if err := e.mgr.repairTaskFromRetainedFailure(ctx, "A", ""); err == nil {
		t.Fatal("empty evidence accepted")
	}
	if err := (&Manager{}).repairTaskFromRetainedFailure(ctx, "A", "id"); err == nil {
		t.Fatal("missing store")
	}
	if err := e.mgr.repairTaskFromRetainedFailure(ctx, "A", "missing"); err == nil {
		t.Fatal("missing evidence")
	}
	e.closeState()
	if err := e.mgr.repairTaskFromRetainedFailure(ctx, "A", "wrong-run"); err == nil {
		t.Fatal("unreadable store")
	}
}
