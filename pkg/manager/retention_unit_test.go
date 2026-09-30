package manager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

func TestRetentionUnitRepairContextRefusesInvalidEvidence(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	if err := e.mgr.state.CreateRun(ctx, "A", "", "", e.dir, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	f := newRetentionBoundaryFixture(t, e.dir, "")
	_, execErr := f.Execute(ctx, &runtime.Stage{Name: "test"}, nil)
	refs := gates.VerificationFailureArtifacts(execErr)
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

func TestRetentionUnitPersistReferencesAndRefusals(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	f := newRetentionBoundaryFixture(t, e.dir, "")
	_, execErr := f.Execute(ctx, &runtime.Stage{Name: "test"}, nil)
	refs := gates.VerificationFailureArtifacts(execErr)
	refs[f.hash] = f.path
	refs[""] = "ignored"
	refs["empty-path"] = ""
	event := loop.Event{Type: loop.EventBlueprintFailed, StageName: "test", Attempt: 1, Text: "output", ErrText: "failed", Artifacts: refs}
	if got := (&Manager{}).persistTaskVerificationFailure("A", event); got != "" {
		t.Fatal("missing store accepted")
	}
	if got := e.mgr.persistTaskVerificationFailure("A", event); got != "" {
		t.Fatal("missing pipeline accepted")
	}
	if err := e.mgr.state.CreateRun(ctx, "A", "", "", e.dir, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	e.mgr.pipelines["A"] = &entry{info: PipelineInfo{StartedAt: time.Now()}}
	id := e.mgr.persistTaskVerificationFailure("A", event)
	if id == "" {
		t.Fatal("reference not persisted")
	}
	step, err := e.mgr.state.GetRunStep(ctx, id)
	if err != nil || step == nil {
		t.Fatal(err)
	}
	var retained map[string]string
	if err := json.Unmarshal([]byte(step.Metadata), &retained); err != nil {
		t.Fatal(err)
	}
	if retained[f.hash] != f.path || retained["stage_output"] != "output" || retained["stage_error"] != "failed" {
		t.Fatal("evidence lost")
	}
	artifact, err := e.mgr.state.GetArtifact(ctx, f.hash)
	if err != nil || artifact == nil || artifact.Path != f.path {
		t.Fatal("reference missing", err)
	}
	if _, exists := refs["stage_output"]; exists {
		t.Fatal("caller map mutated")
	}
	for _, table := range []string{"artifacts", "run_steps"} {
		trigger := "refuse_" + table
		if _, err := e.mgr.state.GetDB().Exec("CREATE TRIGGER " + trigger + " BEFORE INSERT ON " + table + " BEGIN SELECT RAISE(ABORT,'storage refusal'); END"); err != nil {
			t.Fatal(err)
		}
		event.Text = table
		if got := e.mgr.persistTaskVerificationFailure("A", event); got != "" {
			t.Fatal("storage failure accepted", table)
		}
		if _, err := e.mgr.state.GetDB().Exec("DROP TRIGGER " + trigger); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRetentionUnitRepairDescriptionUsesPersistedContext(t *testing.T) {
	e := newSchedulerTestEnv(t)
	ctx := context.Background()
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	task := *e.rel.GetTask("A")
	task.Status = release.TaskStatusInProgress
	task.AttemptCount = 1
	if err := e.rel.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.state.CreateRun(ctx, "A", "", "", e.dir, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	e.mgr.taskQueueActive = true
	e.mgr.pipelines["A"] = &entry{taskAttempt: 1, info: PipelineInfo{StartedAt: time.Now()}}
	f := newRetentionBoundaryFixture(t, e.dir, "")
	result, execErr := f.Execute(ctx, &runtime.Stage{Name: "test"}, nil)
	refs := gates.VerificationFailureArtifacts(execErr)
	refs[f.hash] = f.path
	event := loop.Event{Type: loop.EventBlueprintFailed, StageName: "test", Attempt: 1, Text: result.Output, ErrText: "verification exited 2", Artifacts: refs, Result: &loop.StepResult{Diagnostics: result.Diagnostics}}
	id := e.mgr.persistTaskVerificationFailure("A", event)
	if id == "" {
		t.Fatal("no persisted failure")
	}
	// Mutating the original event must not alter repair context read from storage.
	event.Artifacts[f.hash] = "changed-after-persist"
	event.Text = "changed-after-persist"
	if err := e.mgr.repairTaskFromRetainedFailure(ctx, "A", id); err != nil {
		t.Fatal(err)
	}
	step, err := e.mgr.state.GetRunStep(ctx, id)
	if err != nil || step == nil {
		t.Fatal(err)
	}
	tasks, err := e.rel.TasksInStories(ctx, []string{"S"})
	if err != nil {
		t.Fatal(err)
	}
	repairs := 0
	for _, candidate := range tasks {
		if candidate.Metadata["repair_of"] != "A" {
			continue
		}
		repairs++
		saved, err := e.rel.TaskSnapshot(ctx, candidate.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{id, step.Metadata, "Preserve the original task/candidate and accepted scope", "Reproduce the failing check", "this receipt proves failure, not a particular code defect", "Do not weaken the check or cross effect boundaries"} {
			if !strings.Contains(saved.Description, required) {
				t.Fatalf("repair context missing %q", required)
			}
		}
		if strings.Contains(saved.Description, "changed-after-persist") || strings.Contains(saved.Description, "VALUE_SENTINEL") {
			t.Fatal("repair context used mutable or private diagnostics")
		}
	}
	if repairs != 1 {
		t.Fatal("expected exactly one repair", repairs)
	}
}
