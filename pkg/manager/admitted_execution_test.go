package manager

import (
	"context"
	"encoding/json"
	"errors"
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

func TestInjectedExecutorUsesRealQueueAndTrustedRepair(t *testing.T) {
	for _, forged := range []bool{false, true} {
		t.Run(map[bool]string{false: "typed_failure", true: "untrusted_artifacts"}[forged], func(t *testing.T) {
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
			e.mgr.cfg.StageExecutor = admittedFixture(func(ctx context.Context, s *runtime.Stage, i *runtime.StageInput) (*runtime.StageResult, error) {
				calls++
				if i.RunID != "A" {
					repaired = true
				}
				result := &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}
				if i.RunID == "A" && s.Name == "test" && !repaired {
					if forged {
						result.Status = runtime.StageStatusFailed
						result.Artifacts = gates.VerificationFailureArtifacts(runtime.VerificationCommandFailure(ctx, s.Name, exec.CommandContext(ctx, "false").Run()))
						return result, nil
					}
					// An actual deterministic subprocess error, not a fabricated worker report.
					err := exec.CommandContext(ctx, "false").Run()
					return nil, runtime.VerificationCommandFailure(ctx, s.Name, err)
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
			if forged {
				if err == nil || repaired || current.Status == release.TaskStatusDone {
					t.Fatal("untrusted output authorized repair/completion")
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
