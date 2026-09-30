package manager

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// These fixtures put each public engine API inside the admitted executor seam,
// so the same real command must survive that boundary before persistence/repair.
type retentionMutationExecutor struct {
	fixture *retentionFixture
	single  bool
}

type retentionMutationRecorder struct {
	*retentionFixture
	failure error
}

func (r *retentionMutationRecorder) Execute(ctx context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
	result, err := r.retentionFixture.Execute(ctx, stage, input)
	r.failure = err
	return result, err
}

func (e *retentionMutationExecutor) Execute(ctx context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
	if stage.Name != "test" {
		return e.fixture.Execute(ctx, stage, input)
	}
	isolated := *stage
	isolated.OnSuccess, isolated.OnFailure, isolated.MaxRetries = "complete", "", 0
	stage = &isolated
	recorder := &retentionMutationRecorder{retentionFixture: e.fixture}
	bp := &blueprint.Blueprint{ID: "mutation", Name: "mutation", InitialStage: "test", Stages: map[string]*blueprint.Stage{"test": stage}}
	engine, err := blueprint.NewEngine(bp, recorder, nil)
	if err != nil {
		return nil, err
	}
	run, err := engine.StartRun(ctx, "mutation", input)
	if err != nil {
		return nil, err
	}
	if e.single {
		return engine.ExecuteStage(ctx, run, "test", input)
	}
	err = engine.Execute(ctx, run, input)
	// Execute returns a summary error; preserve the executor's typed receipt,
	// never reconstruct authority from text or discarded result artifacts.
	if recorder.failure != nil {
		err = recorder.failure
	}
	return run.GetLastResult(), err
}

func TestRetentionMutationJourney(t *testing.T) {
	for _, single := range []bool{true, false} {
		t.Run(map[bool]string{true: "ExecuteStage", false: "Execute"}[single], func(t *testing.T) { retentionMutationJourney(t, single) })
	}
}

func retentionMutationJourney(t *testing.T, single bool) {
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	task := *e.rel.GetTask("A")
	task.Status, task.AttemptCount = release.TaskStatusInProgress, 1
	if err := e.rel.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	f := newRetentionFixture(t, e.dir)
	e.mgr.cfg.StageExecutor = &retentionMutationExecutor{fixture: f, single: single}
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
