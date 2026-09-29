package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

func assertRetentionEvidence(t *testing.T, f *retentionFixture, path string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var evidence retentionEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(evidence.Argv, f.argv) || evidence.Cwd != f.dir || evidence.ExitCode != 2 {
		t.Fatalf("execution identity lost: %+v", evidence)
	}
	for _, output := range []string{evidence.Stdout, evidence.Stderr} {
		if len(output) != retentionLimit || !strings.Contains(output, retentionMarker) {
			t.Fatalf("bounded diagnostic lost: length=%d", len(output))
		}
	}
}

func TestRetainedResultEngineBranches(t *testing.T) {
	for _, single := range []bool{true, false} {
		t.Run(map[bool]string{true: "ExecuteStage", false: "Execute"}[single], func(t *testing.T) {
			f := newRetentionFixture(t, t.TempDir())
			stage := &blueprint.Stage{Name: "test", Type: runtime.StageTypeDeterministic}
			bp := &blueprint.Blueprint{ID: "retention", Name: "retention", InitialStage: "test", Stages: map[string]*blueprint.Stage{"test": stage}}
			engine, err := blueprint.NewEngine(bp, f, nil)
			if err != nil {
				t.Fatal(err)
			}
			input := blueprint.NewStageInput("retention", "", f.dir)
			run, err := engine.StartRun(context.Background(), "retention", input)
			if err != nil {
				t.Fatal(err)
			}
			if single {
				result, execErr := engine.ExecuteStage(context.Background(), run, "test", input)
				if execErr == nil || result != f.last {
					t.Fatal("failed result/error pair lost")
				}
			} else {
				if err := engine.Execute(context.Background(), run, input); err == nil {
					t.Fatal("failure accepted")
				}
				if input.GetLastResult() != f.last || run.Status != blueprint.RunStatusFailed {
					t.Fatal("failed history lost")
				}
			}
			if len(run.Results) != 1 || run.GetLastResult() != f.last || f.last.Status != runtime.StageStatusFailed ||
				!strings.Contains(f.last.Output, retentionMarker) || !strings.Contains(f.last.Diagnostics, retentionMarker) ||
				f.last.Artifacts[f.hash] != f.path {
				t.Fatal("engine discarded executor evidence")
			}
			assertRetentionEvidence(t, f, f.path)
		})
	}
}

func TestRetainedResultAdmittedFailureReloadAndRepair(t *testing.T) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	task := *e.rel.GetTask("A")
	task.Status, task.AttemptCount = release.TaskStatusInProgress, 1
	if err := e.rel.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	f := newRetentionFixture(t, e.dir)
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
	if step.Status != "failed" || step.Phase != "test" || step.Iteration != 1 {
		t.Fatalf("failed stage identity lost: %+v", step)
	}
	var artifacts map[string]string
	if err := json.Unmarshal([]byte(step.Metadata), &artifacts); err != nil {
		t.Fatal(err)
	}
	if artifacts[f.hash] != f.path || !strings.Contains(artifacts["stage_output"], retentionMarker) ||
		!strings.Contains(artifacts["stage_diagnostics"], retentionMarker) || artifacts["stage_error"] == "" {
		t.Fatal("persisted stage evidence lost")
	}
	artifact, err := reopened.GetArtifact(ctx, f.hash)
	if err != nil || artifact == nil || artifact.Path != f.path {
		t.Fatal("artifact reference unavailable", err)
	}
	assertRetentionEvidence(t, f, artifact.Path)

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
	if err != nil || !strings.Contains(repair.Description, evidenceID) || !strings.Contains(repair.Description, f.hash) ||
		!strings.Contains(repair.Description, f.path) {
		t.Fatal("repair has no usable authoritative verification reference", err)
	}
	// The referenced artifact contains the exact executable argv and cwd,
	// including whitespace, and remains readable after repair persistence.
	assertRetentionEvidence(t, f, artifacts[f.hash])
	if err := fresh.repairTaskFromRetainedFailure(ctx, "A", "missing-evidence"); err == nil {
		t.Fatal("missing evidence authorized repair")
	}
}
