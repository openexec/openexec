package manager

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/internal/testutil/admittedevidence"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

func assertPublicSilentEvidence(t *testing.T, f *admittedevidence.Executor) {
	t.Helper()
	command, err := runtime.ReadCommandEvidence(f.Dir, f.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(command.Argv, f.Argv) || command.Cwd != f.Dir || command.ExitCode != 2 || command.Stdout != "" || command.Stderr != "" {
		t.Fatalf("silent command identity lost: %+v", command)
	}
}

func TestPublicSilentFailureReloadAndRepair(t *testing.T) {
	for _, gate := range []string{"lint", "test"} {
		t.Run(gate, func(t *testing.T) { testPublicSilentFailureReloadAndRepair(t, gate) })
	}
}

func testPublicSilentFailureReloadAndRepair(t *testing.T, gate string) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	task := *e.rel.GetTask("A")
	task.Status, task.AttemptCount = release.TaskStatusInProgress, 1
	if err := e.rel.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	f := &admittedevidence.Executor{Dir: e.dir, Gate: gate}
	e.mgr.cfg.StageExecutor = f
	e.mgr.mu.Lock()
	e.mgr.taskQueueActive = true
	e.mgr.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := e.mgr.start(ctx, "A", true, WithBlueprint("standard_task"), WithTaskDescription(task.Description)); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.waitTaskQueueRun(ctx, "A"); err == nil {
		t.Fatal("exit 2 accepted")
	}
	failed, err := e.rel.TaskSnapshot(ctx, "A")
	if err != nil || failed.Status != release.TaskStatusFailed {
		t.Fatalf("failed task: %+v %v", failed, err)
	}
	evidenceID, _ := failed.Metadata["verification_failure_evidence"].(string)
	if evidenceID == "" {
		t.Fatal("no persisted evidence binding")
	}

	// Close the producing manager/database before deriving any repair.
	cfg := e.mgr.cfg
	e.mgr.Close()
	e.closeState()
	reopened, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	cfg.StateStore = reopened
	fresh, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	step, err := reopened.GetRunStep(ctx, evidenceID)
	if err != nil || step == nil {
		t.Fatal("missing reloaded step", err)
	}
	if step.Status != "failed" || step.Phase != gate || step.Iteration != 1 {
		t.Fatalf("failed stage identity lost: %+v", step)
	}
	var artifacts map[string]string
	if err := json.Unmarshal([]byte(step.Metadata), &artifacts); err != nil {
		t.Fatal(err)
	}
	if artifacts["stage_output"] != "" || artifacts["stage_diagnostics"] != "" || f.Calls != 1 {
		t.Fatal("silent fixture produced diagnostics or repeated execution")
	}
	diagnosticFree := fresh.diagnosticFreeReceipt(artifacts)
	if artifacts[f.Hash] != f.Path || diagnosticFree {
		t.Fatalf("silent receipt lost attached evidence or became diagnostic-free (diagnosticFree=%t)", diagnosticFree)
	}
	artifact, err := reopened.GetArtifact(ctx, f.Hash)
	if err != nil || artifact == nil || artifact.Path != f.Path {
		t.Fatal("artifact reference unavailable", err)
	}
	assertPublicSilentEvidence(t, f)

	if err := fresh.repairTaskFromRetainedFailure(ctx, "A", evidenceID); err != nil {
		t.Fatal(err)
	}
	rel, err := fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := rel.TasksInStories(ctx, []string{"S"})
	if err != nil {
		t.Fatal(err)
	}
	var repairID string
	for _, candidate := range tasks {
		if candidate.Metadata["repair_of"] == "A" {
			repairID = candidate.ID
		}
	}
	if repairID == "" || len(tasks) != 2 {
		t.Fatal("one repair was not created")
	}
	repair, err := rel.TaskSnapshot(ctx, repairID)
	if err != nil || !strings.Contains(repair.Description, evidenceID) || !strings.Contains(repair.Description, f.Hash) ||
		!strings.Contains(repair.Description, f.Path) || !strings.Contains(repair.Description, "Reproduce the failing check") {
		t.Fatal("repair has no usable authoritative verification reference", err)
	}
	// The referenced artifact contains the exact executable argv and cwd,
	// including whitespace, and remains readable after repair persistence.
	assertPublicSilentEvidence(t, f)
	if err := fresh.repairTaskFromRetainedFailure(ctx, "A", "missing-evidence"); err == nil {
		t.Fatal("missing evidence authorized repair")
	}
}
