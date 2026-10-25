package manager

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// A check that really exited 1 is classified while the run is live, then the
// run is stopped before the stage returns. A stopped run is not a failed check:
// no receipt is persisted and no fix task is filed, and after reopening the
// database the same task resumes and is delivered without a repair.
func TestStoppedRunAfterClassifiedCheckFailureFilesNoRepair(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	classified := false
	e.mgr.cfg.StageExecutor = admittedFixture(func(c context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		if s.Name != "test" {
			return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
		}
		argv := []string{"/bin/sh", "-c", "printf 'FAIL TestX\\n'; exit 1"}
		output, err := exec.CommandContext(c, argv[0], argv[1:]...).CombinedOutput()
		failure := runtime.VerificationCommandFailureWithOutput(c, s.Name, err, strings.Join(argv, " "), string(output))
		classified = gates.VerificationFailureArtifacts(failure) != nil
		cancel()
		return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusFailed, Attempt: 1}, failure
	})
	if err := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("stopped run reported success")
	}
	if !classified {
		t.Fatal("fixture did not produce a live exit-1 receipt before the stop")
	}
	stopped, err := e.rel.TaskSnapshot(context.Background(), "A")
	if err != nil || stopped.Status == release.TaskStatusFailed || stopped.Metadata["verification_failure_evidence"] != nil || stopped.Metadata["fixed_by"] != nil {
		t.Fatalf("stopped run persisted repair evidence: %+v %v", stopped, err)
	}

	cfg := e.mgr.cfg
	e.mgr.Close()
	e.closeState()
	reopened, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cfg.StateStore = reopened
	cfg.StageExecutor = admittedFixture(func(_ context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
		return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
	})
	fresh, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if err := fresh.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := rel.TasksInStories(context.Background(), []string{"S"})
	if err != nil || len(tasks) != 1 {
		t.Fatalf("stopped run filed repair work: %d tasks %v", len(tasks), err)
	}
	resumed, err := rel.TaskSnapshot(context.Background(), "A")
	if err != nil || resumed.Status != release.TaskStatusDone || resumed.Metadata["fixed_by"] != nil {
		t.Fatalf("same task did not resume to delivery: %+v %v", resumed, err)
	}
}
