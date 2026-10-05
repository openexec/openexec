package manager

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// These fixtures enter through real project loading and JSON bootstrap, then
// reopen SQLite before calling the implementation-owned native recapture loop.
func newProtectedRecapture(t *testing.T, format string) *recaptureFixture {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join("testdata", "recapture-compatibility", format)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dir, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := project.LoadProjectConfig(dir)
	if err != nil || cfg.Name != "recapture-"+format || cfg.ProjectDir != dir {
		t.Fatalf("protected config: %+v %v", cfg, err)
	}
	store, err := state.NewStore(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	closeState := sync.OnceFunc(func() { store.Close() })
	t.Cleanup(closeState)
	mgr, err := New(Config{WorkDir: dir, StateStore: store, AgentsFS: os.DirFS(filepath.Join("..", "..", "internal", "pipeline", "testdata")), MaxRetries: 1, TaskTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mgr.Close() })
	rel, err := mgr.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	f := &recaptureFixture{env: &schedulerTestEnv{mgr: mgr, rel: rel, dir: dir, closeState: closeState}}
	mgr.cfg.StageExecutor = f
	task, err := rel.TaskSnapshot(context.Background(), "A")
	if err != nil || task.AttemptCount != 1 || task.MaxAttempts != 3 || !strings.Contains(task.VerificationScript, "PROTECTED_RECAPTURE") {
		t.Fatalf("bootstrap lost task: %+v %v", task, err)
	}
	if err := store.CreateRun(context.Background(), "A", "", "", dir, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(recaptureReceipt("verify", 2))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RecordTaskFailureStep(context.Background(), state.RunStepData{ID: "legacy", RunID: "A", Phase: "verify", Agent: "deterministic-verification", Status: "failed", Metadata: string(data)}, 1); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	return f
}

func TestRecaptureCompatibility(t *testing.T) {
	for _, format := range []string{"openexec", "uaos", "tasks_json"} {
		t.Run(format, func(t *testing.T) {
			for _, outcome := range []string{"failure", "success", "unresolved"} {
				t.Run(outcome, func(t *testing.T) {
					f := newProtectedRecapture(t, format)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					task, err := f.env.rel.TaskSnapshot(ctx, "A")
					if err != nil {
						t.Fatal(err)
					}
					if outcome == "success" {
						task.VerificationScript = "printf 'PROTECTED_SUCCESS\\n'"
						f.mode = "success"
					}
					if outcome == "unresolved" {
						task.VerificationScript = ""
					}
					if err := f.env.rel.UpdateTask(task); err != nil {
						t.Fatal(err)
					}
					f.restart(t)
					err = f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
					if (err == nil) != (outcome == "success") {
						t.Fatalf("queue outcome %s: %v", outcome, err)
					}
					f.restart(t)
					task, err = f.env.rel.TaskSnapshot(ctx, "A")
					if err != nil {
						t.Fatal(err)
					}
					settings, err := f.env.rel.TaskSnapshot(ctx, "Settings")
					if err != nil {
						t.Fatal(err)
					}
					tasks, err := f.env.rel.TasksInStories(ctx, []string{"S"})
					if err != nil {
						t.Fatal(err)
					}
					switch outcome {
					case "unresolved":
						f.assertTerminal(t, "unresolved", 1)
						if len(tasks) != 2 || f.calls != 0 {
							t.Fatal("unresolved command dispatched or invented repair")
						}
						if err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
							t.Fatal("terminal restart accepted")
						}
						f.restart(t)
						f.assertTerminal(t, "unresolved", 1)
						if f.calls != 0 {
							t.Fatal("terminal restart dispatched")
						}
					case "success":
						if task.Status != release.TaskStatusDone || settings.Status != release.TaskStatusDone || task.Metadata["verification_failure_evidence"] != nil || len(tasks) != 2 || f.calls != 1 {
							t.Fatalf("success did not converge: %+v %+v calls=%d", task, settings, f.calls)
						}
					case "failure":
						// The failed task is closed with its fix; its dependent waits for the fix.
						if task.Status != release.TaskStatusFailed || task.Metadata["fixed_by"] == nil || task.AttemptCount != 2 || settings.Status != release.TaskStatusPending || settings.AttemptCount != 0 || f.calls != 1 || len(tasks) != 3 {
							t.Fatalf("repair disposition: %+v %+v calls=%d tasks=%d", task, settings, f.calls, len(tasks))
						}
						id, _ := task.Metadata["verification_failure_evidence"].(string)
						if id == "" || id == "legacy" {
							t.Fatal("fresh receipt absent")
						}
						step, err := f.env.mgr.state.GetRunStep(ctx, id)
						if err != nil || step == nil {
							t.Fatal("receipt lost", err)
						}
						var refs map[string]string
						if err := json.Unmarshal([]byte(step.Metadata), &refs); err != nil {
							t.Fatal(err)
						}
						usable := false
						for hash := range refs {
							if ev, err := runtime.ReadCommandEvidence(f.env.dir, hash); err == nil {
								usable = usable || (ev.ExitCode == 2 && ev.Cwd == f.env.dir && len(ev.Argv) == 3 && ev.Argv[2] == task.VerificationScript && strings.Contains(ev.Stderr, "PROTECTED_RECAPTURE"))
							}
						}
						if !usable {
							t.Fatal("original command and diagnostics lost after reopen")
						}
						repairs := 0
						for _, repair := range tasks {
							if repair.Metadata["repair_of"] == "A" {
								repairs++
								if !strings.Contains(repair.Description, id) {
									t.Fatal("repair lost fresh evidence")
								}
							}
						}
						if repairs != 1 {
							t.Fatalf("repair count=%d", repairs)
						}
					}
					// A populated ledger must not reimport stale JSON or reset attempts.
					if format == "tasks_json" {
						if err := os.WriteFile(filepath.Join(f.env.dir, ".openexec", "tasks.json"), []byte(`{"tasks":[]}`), 0600); err != nil {
							t.Fatal(err)
						}
					}
					f.restart(t)
					persisted, err := f.env.rel.TaskSnapshot(ctx, "A")
					if err != nil || persisted.AttemptCount != task.AttemptCount || persisted.Status != task.Status {
						t.Fatalf("stale JSON replaced canonical state: %+v %v", persisted, err)
					}
					cfg, err := project.LoadProjectConfig(f.env.dir)
					if err != nil || cfg.Name != "recapture-"+format {
						t.Fatal("config lost after recapture", err)
					}
				})
			}
		})
	}
}
