package manager

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// A caller-owned fixture store, outside the worker directory. Its key and loader
// belong to admission, not the worker. Reopening checks authenticated bytes.
func persistTerminal(t *testing.T, terminal runtime.TerminalCompletion) (runtime.TerminalLoader, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terminal.json")
	raw, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	key := []byte("fixture-only-admission-key")
	sign := func(data []byte) []byte { h := hmac.New(sha256.New, key); h.Write(data); return h.Sum(nil) }
	signature := sign(raw)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return func(_ context.Context, id string) (runtime.TerminalCompletion, error) {
		data, err := os.ReadFile(path)
		if err != nil {
			return runtime.TerminalCompletion{}, err
		}
		if !hmac.Equal(signature, sign(data)) {
			return runtime.TerminalCompletion{}, errors.New("terminal integrity refused")
		}
		var reloaded runtime.TerminalCompletion
		if err := json.Unmarshal(data, &reloaded); err != nil {
			return reloaded, err
		}
		if reloaded.ID != id {
			return reloaded, errors.New("unknown completion")
		}
		return reloaded, nil
	}, path
}

func TestPersistedExitOneRecovery(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprintf("restart_%t", restart), func(t *testing.T) { exercisePersistedRecovery(t, restart, 1) })
	}
}

func TestPersistedRecoveryMatrix(t *testing.T) {
	for _, code := range []int{0, 1, 125} {
		for _, restart := range []bool{false, true} {
			t.Run(fmt.Sprintf("exit_%d/restart_%t", code, restart), func(t *testing.T) { exercisePersistedRecovery(t, restart, code) })
		}
	}
}

func exercisePersistedRecovery(t *testing.T, restart bool, failureCode int) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	createQueueTask(t, e, "B", []string{"A"})
	if failureCode == 0 {
		if err := os.WriteFile(filepath.Join(e.dir, "feature.txt"), []byte("repaired"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var order []string
	var terminalID string
	var binding runtime.TerminalBinding
	// All subprocesses have the admitted project directory as their working dir.
	e.mgr.cfg.StageExecutor = admittedFixture(func(ctx context.Context, s *runtime.Stage, i *runtime.StageInput) (*runtime.StageResult, error) {
		run := func(command string) error {
			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			cmd.Dir = e.dir
			return cmd.Run()
		}
		if s.Name == "implement" {
			order = append(order, i.RunID)
			if i.RunID != "A" && i.RunID != "B" {
				if err := run("printf repaired > feature.txt"); err != nil {
					return nil, err
				}
			}
			if i.RunID == "B" {
				if err := run("test -f feature.txt && printf remaining > remaining.txt"); err != nil {
					return nil, err
				}
			}
		}
		if s.Name == "test" {
			command := fmt.Sprintf("test -f feature.txt || exit %d", failureCode)
			commandErr := run(command)
			code := 0
			if commandErr != nil {
				exit, ok := commandErr.(*exec.ExitError)
				if !ok {
					return nil, commandErr
				}
				code = exit.ExitCode()
			}
			b := runtime.TerminalBinding{TaskID: i.RunID, Stage: s.Name, TaskAttempt: i.TaskAttempt, StageAttempt: i.StageAttempt, Source: "deterministic-runner"}
			id := fmt.Sprintf("%s-%d-%d", i.RunID, i.TaskAttempt, i.StageAttempt)
			terminal := runtime.TerminalCompletion{ID: id, Binding: b, Outcome: "exited", ExitCode: &code, Command: "sh -c " + command}
			loader, _ := persistTerminal(t, terminal)
			if code != 0 {
				terminalID = id
				binding = b
				t.Logf("actual persisted exit=%d binding=%+v", code, b)
			}
			if err := runtime.VerificationTerminalFailure(ctx, b, id, loader); err != nil {
				return nil, err
			}
		}
		return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: i.StageAttempt}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if restart {
		if failureCode == 0 {
			if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
				t.Fatal(err)
			}
		} else {
			a := *e.rel.GetTask("A")
			a.Status = release.TaskStatusInProgress
			a.AttemptCount = 1
			if err := e.rel.UpdateTask(&a); err != nil {
				t.Fatal(err)
			}
			e.mgr.mu.Lock()
			e.mgr.taskQueueActive = true
			e.mgr.mu.Unlock()
			if err := e.mgr.start(ctx, "A", true, WithBlueprint("standard_task"), WithTaskDescription(a.Description)); err != nil {
				t.Fatal(err)
			}
			if err := e.mgr.waitTaskQueueRun(ctx, "A"); (err != nil) != (failureCode != 0) {
				t.Fatalf("exit %d: %v", failureCode, err)
			}
			failed, err := e.rel.TaskSnapshot(ctx, "A")
			if err != nil || (failed.Metadata["verification_failure_evidence"] != nil) != (failureCode != 0) {
				t.Fatal("missing persisted failure before restart", err)
			}
		}
		cfg := e.mgr.cfg
		e.mgr.Close()
		e.closeState()
		reopened, err := state.NewStore(filepath.Join(e.dir, "state.db"))
		if err != nil {
			t.Fatal(err)
		}
		e.closeState = sync.OnceFunc(func() { _ = reopened.Close() })
		defer e.closeState()
		cfg.StateStore = reopened
		e.mgr, err = New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer e.mgr.Close()
	}
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	cfg := e.mgr.cfg
	e.mgr.Close()
	e.closeState()
	store, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cfg.StateStore = store
	fresh, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := rel.TasksInStories(ctx, []string{"S"})
	wantTasks := 3
	if failureCode == 0 {
		wantTasks = 2
	}
	if err != nil || len(tasks) != wantTasks {
		t.Fatalf("reopened tasks=%v err=%v", tasks, err)
	}
	repairID := ""
	for _, task := range tasks {
		if task.Status != release.TaskStatusDone {
			t.Fatalf("unfinished persisted task %+v", task)
		}
		wantAttempts := 2
		if failureCode == 0 {
			wantAttempts = 1
		}
		if task.ID == "A" && task.AttemptCount != wantAttempts {
			t.Fatal("A did not resume")
		}
		if task.Metadata["repair_of"] == "A" {
			repairID = task.ID
			id, _ := task.Metadata["failure_evidence"].(string)
			step, err := store.GetRunStep(ctx, id)
			if err != nil || step == nil || step.RunID != "A" || step.Status != "failed" {
				t.Fatalf("lost receipt %v %v", step, err)
			}
			var artifacts map[string]string
			if err := json.Unmarshal([]byte(step.Metadata), &artifacts); err != nil {
				t.Fatal(err)
			}
			if !gates.ValidateVerificationFailureArtifacts(artifacts) {
				t.Fatal("invalid reopened artifacts")
			}
			var checks []gates.CheckFailure
			if err := json.Unmarshal([]byte(artifacts[gates.VerificationFailureReceiptKey]), &checks); err != nil {
				t.Fatal(err)
			}
			if len(checks) != 1 || checks[0].ExitCode != failureCode || checks[0].TerminalID != terminalID || checks[0].Binding == nil || *checks[0].Binding != binding {
				t.Fatalf("lost terminal identity: %+v", checks)
			}
		}
	}
	wantOrder := []string{"A", repairID, "A", "B"}
	if failureCode == 0 {
		wantOrder = []string{"A", "B"}
	} else if repairID == "" {
		t.Fatal("missing repair")
	}
	if !reflect.DeepEqual(order, wantOrder) {
		t.Fatalf("wrong native order: %v repair=%s", order, repairID)
	}
	for file, want := range map[string]string{"feature.txt": "repaired", "remaining.txt": "remaining"} {
		raw, err := os.ReadFile(filepath.Join(e.dir, file))
		if err != nil || string(raw) != want {
			t.Fatalf("persisted work %s: %s %v", file, raw, err)
		}
	}
	// Re-deliver the same durable failure after restart. The existing repair
	// disposition must remain idempotent even though A has since completed.
	if repairID != "" {
		repair, err := rel.TaskSnapshot(ctx, repairID)
		if err != nil {
			t.Fatal(err)
		}
		evidenceID, _ := repair.Metadata["failure_evidence"].(string)
		for replay := 0; replay < 2; replay++ {
			if err := fresh.repairTaskFromRetainedFailure(ctx, "A", evidenceID); err != nil {
				t.Fatal(err)
			}
		}
	}
	before := len(order)
	if err := fresh.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	if len(order) != before {
		t.Fatal("restart duplicated completed work")
	}
	tasks, err = rel.TasksInStories(ctx, []string{"S"})
	if err != nil || len(tasks) != wantTasks {
		t.Fatalf("replay duplicated repair: %v %v", tasks, err)
	}
	for _, task := range tasks {
		if task.Status != release.TaskStatusDone {
			t.Fatalf("replay reopened task: %+v", task)
		}
		if task.Metadata["verification_failure_evidence"] != nil {
			t.Fatalf("completed task retains active failure: %+v", task)
		}
	}
	var receipts int
	if err := store.GetDB().QueryRow("SELECT count(*) FROM run_steps WHERE agent = 'deterministic-verification'").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	wantReceipts := 1
	if failureCode == 0 {
		wantReceipts = 0
	}
	if receipts != wantReceipts {
		t.Fatalf("reopened receipt count=%d want=%d", receipts, wantReceipts)
	}
	t.Logf("reopened exit=%d order=%v tasks=%d receipts=%d; replay dispatched no work", failureCode, order, len(tasks), receipts)
}

func TestPersistedTerminalRefusalsDoNotRepair(t *testing.T) {
	for _, mode := range []string{"tampered", "missing", "stale_task", "stale_stage", "stale_attempt", "stale_stage_attempt", "worker_source", "worker_artifacts", "mixed", "cancelled", "timeout", "launch", "transport", "exit126", "exit127", "exit128", "exit255", "exit_negative", "missing_exit", "empty_id", "wrong_id", "empty_task", "empty_stage", "zero_attempt", "negative_attempt", "zero_stage_attempt", "negative_stage_attempt", "nil_loader", "loader_typed_failure", "context_cancelled", "context_timeout", "mixed_cancelled", "mixed_timeout", "mixed_launch"} {
		t.Run(mode, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			e.mgr.cfg.StageExecutor = admittedFixture(func(ctx context.Context, s *runtime.Stage, i *runtime.StageInput) (*runtime.StageResult, error) {
				if s.Name != "test" {
					return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted}, nil
				}
				cmdErr := exec.CommandContext(ctx, "sh", "-c", "exit 1").Run()
				exit, ok := cmdErr.(*exec.ExitError)
				if !ok {
					t.Fatal(cmdErr)
				}
				code := exit.ExitCode()
				b := runtime.TerminalBinding{TaskID: i.RunID, Stage: s.Name, TaskAttempt: i.TaskAttempt, StageAttempt: i.StageAttempt, Source: "deterministic-runner"}
				switch mode {
				case "stale_task":
					b.TaskID = "other"
				case "stale_stage":
					b.Stage = "lint"
				case "stale_attempt":
					b.TaskAttempt++
				case "stale_stage_attempt":
					b.StageAttempt++
				case "worker_source":
					b.Source = "worker"
				case "exit126":
					code = 126
				case "exit127":
					code = 127
				case "exit128":
					code = 128
				case "exit255":
					code = 255
				case "exit_negative":
					code = -1
				case "empty_task":
					b.TaskID = " "
				case "empty_stage":
					b.Stage = " "
				case "zero_attempt":
					b.TaskAttempt = 0
				case "negative_attempt":
					b.TaskAttempt = -1
				case "zero_stage_attempt":
					b.StageAttempt = 0
				case "negative_stage_attempt":
					b.StageAttempt = -1
				}
				terminal := runtime.TerminalCompletion{ID: "failure", Binding: b, Outcome: "exited", ExitCode: &code}
				switch mode {
				case "cancelled", "timeout", "launch", "transport":
					terminal.Outcome = map[string]string{"cancelled": "cancelled", "timeout": "timed_out", "launch": "launch_error", "transport": "transport_error"}[mode]
				case "missing_exit":
					terminal.ExitCode = nil
				case "empty_id":
					terminal.ID = " "
				}
				loader, path := persistTerminal(t, terminal)
				if mode == "tampered" {
					if err := os.WriteFile(path, []byte(`{}`), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "missing" {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
				}
				id := terminal.ID
				switch mode {
				case "wrong_id":
					id = "unknown"
				case "nil_loader":
					loader = nil
				case "loader_typed_failure":
					loader = func(context.Context, string) (runtime.TerminalCompletion, error) {
						return terminal, runtime.VerificationCommandFailure(ctx, "test", cmdErr)
					}
				case "context_cancelled":
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				case "context_timeout":
					var cancel context.CancelFunc
					ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
					defer cancel()
				}
				err := runtime.VerificationTerminalFailure(ctx, b, id, loader)
				switch mode {
				case "mixed_cancelled":
					err = errors.Join(err, context.Canceled)
				case "mixed_timeout":
					err = errors.Join(err, context.DeadlineExceeded)
				case "mixed_launch":
					err = errors.Join(err, &exec.Error{Name: "missing", Err: exec.ErrNotFound})
				}
				if mode == "mixed" {
					err = errors.Join(err, errors.New("transport failure"))
				}
				if mode == "worker_artifacts" {
					return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusFailed, Artifacts: gates.VerificationFailureArtifacts(err)}, nil
				}
				return nil, err
			})
			if err := e.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
				t.Fatal("refusal completed")
			}
			cfg := e.mgr.cfg
			e.mgr.Close()
			e.closeState()
			store, err := state.NewStore(filepath.Join(e.dir, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			cfg.StateStore = store
			fresh, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			rel, err := fresh.GetInternalReleaseManager()
			if err != nil {
				t.Fatal(err)
			}
			tasks, err := rel.TasksInStories(context.Background(), []string{"S"})
			if err != nil || len(tasks) != 1 {
				t.Fatalf("unauthorized repair: %v %v", tasks, err)
			}
			// A stopped attempt may be retried in the same run within its own
			// attempts (#70); a refusal must never complete it or open a repair.
			if tasks[0].Status == release.TaskStatusDone || tasks[0].AttemptCount > tasks[0].MaxAttempts {
				t.Fatalf("refused task completed or retried past its attempts: %+v", tasks[0])
			}
			if tasks[0].Metadata["verification_failure_evidence"] != nil {
				t.Fatal("unauthorized receipt attached")
			}
			var count int
			// The receipt persistence path uses the deterministic-verification agent.
			if err := store.GetDB().QueryRow("SELECT count(*) FROM run_steps WHERE agent = 'deterministic-verification'").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("unauthorized receipt persisted")
			}
		})
	}
}
