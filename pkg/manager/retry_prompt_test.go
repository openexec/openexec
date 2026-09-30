package manager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

// Exercise actual queue dispatch: a failed execution leaves useful work, the
// next executor receives the correction request, and completion survives reopen.
func TestRetryDispatchReceivesCorrectionRequest(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	task := e.rel.GetTask("A")
	original := task.Description
	task.VerificationScript = "test -f repaired.txt"
	if err := e.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(e.dir, "completed-work.txt")
	fixed := filepath.Join(e.dir, "repaired.txt")
	attempts := 0
	reason := "generated configuration refers to a missing file"
	e.mgr.cfg.StageExecutor = admittedFixture(func(_ context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
		if stage.Name == "implement" {
			attempts++
			if attempts == 1 {
				if input.TaskDescription != original {
					t.Fatal("first attempt received retry instructions")
				}
				if err := os.WriteFile(marker, []byte("keep this"), 0600); err != nil {
					return nil, err
				}
				return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
			}
			for _, want := range []string{original, reason, "Could you fix this?", "Use your judgment", "preserve completed work", "scope and authority", task.VerificationScript} {
				if !strings.Contains(input.TaskDescription, want) {
					t.Fatalf("retry executor did not receive %q: %s", want, input.TaskDescription)
				}
			}
			if raw, err := os.ReadFile(marker); err != nil || string(raw) != "keep this" {
				t.Fatalf("completed work lost: %v", err)
			}
			if err := os.WriteFile(fixed, []byte("corrected"), 0600); err != nil {
				return nil, err
			}
		}
		if stage.Name == "lint" && attempts == 1 {
			return nil, errors.New(reason)
		}
		if stage.Name == "test" {
			if _, err := os.Stat(fixed); err != nil {
				return nil, err
			}
		}
		return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("got %d implementation attempts", attempts)
	}
	e.mgr.Close()
	fresh := freshQueueManager(t, e)
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	saved, err := rel.TaskSnapshot(ctx, "A")
	if err != nil || saved.Status != release.TaskStatusDone || saved.AttemptCount != 2 || saved.Description != original {
		t.Fatalf("reopened task lost original task or completion: %+v, %v", saved, err)
	}
	if stop, _ := saved.Metadata[previousAttemptStop].(string); !strings.Contains(stop, reason) {
		t.Fatal("failure evidence did not persist")
	}
}
