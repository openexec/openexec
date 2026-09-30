package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

type admittedFixture func(context.Context, *runtime.Stage, *runtime.StageInput) (*runtime.StageResult, error)

func (f admittedFixture) Execute(c context.Context, s *runtime.Stage, i *runtime.StageInput) (*runtime.StageResult, error) {
	return f(c, s, i)
}

func TestCancelledInjectedTaskRemainsResumable(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.mgr.cfg.StageExecutor = admittedFixture(func(c context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		if s.Name == "implement" {
			cancel()
			return nil, c.Err()
		}
		return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted}, nil
	})
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation not preserved", err)
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "A")
	if err != nil || task.Status != release.TaskStatusInProgress || task.AttemptCount != 1 {
		t.Fatal("cancellation poisoned retained work", task, err)
	}
	e.mgr.Close()
	cfg := e.mgr.cfg
	cfg.StageExecutor = admittedFixture(func(_ context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		if s.Name == "implement" {
			if err := os.WriteFile(filepath.Join(e.dir, "resumed.txt"), []byte("useful work"), 0600); err != nil {
				return nil, err
			}
		}
		return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted}, nil
	})
	fresh, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	task, err = e.rel.TaskSnapshot(context.Background(), "A")
	if err != nil || task.Status != release.TaskStatusDone || task.AttemptCount != 2 {
		t.Fatal("same native task did not resume", task, err)
	}
	if raw, err := os.ReadFile(filepath.Join(e.dir, "resumed.txt")); err != nil || string(raw) != "useful work" {
		t.Fatal("resumed task produced no work", err)
	}
}

func TestInjectedExecutorUsesRealQueueAndTrustedRepair(t *testing.T) {
	for _, mode := range []string{"typed_failure", "untrusted_artifacts", "legacy_receipt"} {
		t.Run(mode, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			marker := filepath.Join(e.dir, "host-command-ran")
			// Neither the legacy provider nor project-configured commands may execute.
			e.mgr.cfg.CommandName = filepath.Join(e.dir, "nonexistent-provider")
			config, _ := json.Marshal(map[string]any{"execution": map[string]any{"lint_commands": []string{"touch " + marker}, "test_commands": []string{"touch " + marker}}})
			if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), config, 0600); err != nil {
				t.Fatal(err)
			}
			repaired := false
			calls := 0
			legacyChecks := 0
			e.mgr.cfg.StageExecutor = admittedFixture(func(ctx context.Context, s *runtime.Stage, i *runtime.StageInput) (*runtime.StageResult, error) {
				calls++
				if i.RunID != "A" {
					repaired = true
				}
				result := &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}
				if i.RunID == "A" && s.Name == "test" && !repaired {
					if mode == "untrusted_artifacts" {
						result.Status = runtime.StageStatusFailed
						result.Artifacts = gates.VerificationFailureArtifacts(runtime.VerificationCommandFailure(ctx, s.Name, exec.CommandContext(ctx, "false").Run()))
						return result, nil
					}
					// An actual deterministic subprocess error, not a fabricated worker report.
					cmd := exec.CommandContext(ctx, "false")
					cmd.Dir = e.dir
					err := cmd.Run()
					failure := runtime.VerificationCommandFailure(ctx, s.Name, err)
					if mode == "legacy_receipt" {
						legacyChecks++
						if legacyChecks > 1 && len(s.Commands) != 0 {
							return nil, fmt.Errorf("named recapture acquired host commands")
						}
						return nil, failure
					}
					// Silent checks still need exact command evidence. A bare exit
					// classification now takes the bounded legacy recapture path.
					if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1 {
						return nil, fmt.Errorf("fixture did not exit 1: %v", err)
					}
					hash, path, captureErr := runtime.RetainCommandEvidence(e.dir, runtime.CommandEvidence{
						Argv: cmd.Args, Cwd: cmd.Dir, ExitCode: cmd.ProcessState.ExitCode(),
					})
					if captureErr != nil {
						return nil, captureErr
					}
					result.Status = runtime.StageStatusFailed
					result.Artifacts = map[string]string{hash: path}
					return result, failure
				}
				return result, nil
			})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
			if calls == 0 {
				t.Fatal("injected executor not called")
			}
			if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatal("host command executed", statErr)
			}
			current, _ := e.rel.TaskSnapshot(ctx, "A")
			if mode == "untrusted_artifacts" {
				if err == nil || repaired || current.Status == release.TaskStatusDone {
					t.Fatal("untrusted output authorized repair/completion")
				}
				return
			}
			if mode == "legacy_receipt" {
				tasks, listErr := e.rel.TasksInStories(ctx, []string{"S"})
				if err == nil || repaired || current.Status != release.TaskStatusNeedsReview || current.AttemptCount != current.MaxAttempts || legacyChecks != current.MaxAttempts || current.Metadata["recapture_outcome"] != "exhausted" || listErr != nil || len(tasks) != 1 {
					t.Fatalf("diagnostic-free named recapture escaped attempt bound: %v %#v repaired=%v tasks=%d listErr=%v", err, current, repaired, len(tasks), listErr)
				}
				return
			}
			if err != nil || !repaired || current.Status != release.TaskStatusDone || current.AttemptCount != 2 {
				t.Fatalf("real queue did not repair/resume: %v %#v repaired=%v", err, current, repaired)
			}
			tasks, err := e.rel.TasksInStories(ctx, []string{"S"})
			if err != nil || len(tasks) != 2 {
				t.Fatal("repair duplicated", len(tasks), err)
			}
		})
	}
}

func TestInjectedManagerDoesNotReapForeignExecutionBeforeQueueLock(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	if err := e.mgr.state.CreateRun(ctx, "foreign-live", "", "", e.dir, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.state.UpdateRunStatus(ctx, "foreign-live", "running", ""); err != nil {
		t.Fatal(err)
	}
	cfg := e.mgr.cfg
	cfg.StageExecutor = admittedFixture(func(context.Context, *runtime.Stage, *runtime.StageInput) (*runtime.StageResult, error) {
		t.Fatal("constructor dispatched work")
		return nil, nil
	})
	replacement, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	run, err := e.mgr.state.GetRun(ctx, "foreign-live")
	if err != nil || run.Status != "running" {
		t.Fatalf("constructor rewrote foreign execution: %#v %v", run, err)
	}
}
