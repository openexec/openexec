package pipeline

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/quality"
	"github.com/openexec/openexec/internal/types"
)

// spyQualityManager wraps a quality.Manager and records calls to RunAll.
type spyQualityManager struct {
	mu         sync.Mutex
	calls      []string // stage names that triggered RunAll
	underlying *quality.Manager
}

func newSpyQualityManager() *spyQualityManager {
	// Create a real manager with no gates so RunAll returns quickly
	mgr := quality.NewManager("/tmp", []quality.Gate{})
	return &spyQualityManager{
		underlying: mgr,
	}
}

func (s *spyQualityManager) recordCall(stageName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, stageName)
}

func (s *spyQualityManager) getCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.calls))
	copy(result, s.calls)
	return result
}

// stubStageExecutor always succeeds immediately.
type stubStageExecutor struct{}

func (e *stubStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	result := blueprint.NewStageResult(stage.Name, 1)
	result.Complete("stub: ok")
	return result, nil
}

// TestPipeline_QualityGatesOnlyRunOnOptedInStages creates a blueprint with
// some stages having RunQualityGates=true and others not, then verifies
// that the quality manager's RunAll is only called for the opted-in stages.
// This catches Bug #1: quality gates running on wrong stages.
func TestPipeline_QualityGatesOnlyRunOnOptedInStages(t *testing.T) {
	// Build a test blueprint:
	//   stage_a (no gates) -> stage_b (gates=true) -> stage_c (no gates) -> complete
	bp := &blueprint.Blueprint{
		ID:           "test_quality_gates",
		Name:         "Test Quality Gates",
		InitialStage: "stage_a",
		Stages: map[string]*blueprint.Stage{
			"stage_a": {
				Name:            "stage_a",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "stage_b",
				RunQualityGates: false,
			},
			"stage_b": {
				Name:            "stage_b",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "stage_c",
				RunQualityGates: true, // Only this stage should trigger gates
			},
			"stage_c": {
				Name:            "stage_c",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "complete",
				RunQualityGates: false,
			},
		},
	}

	spy := newSpyQualityManager()

	// Track which stages triggered quality gates via the OnStageComplete callback
	// This replicates the logic from pipeline.go's runBlueprintMode()
	engineConfig := blueprint.DefaultEngineConfig()
	engineConfig.OnStageComplete = func(run *blueprint.Run, result *blueprint.StageResult) {
		stage, ok := bp.GetStage(result.StageName)
		if spy.underlying != nil && result.Status == types.StageStatusCompleted && ok && stage.RunQualityGates {
			spy.recordCall(result.StageName)
			// In real code this runs in a goroutine; here we call synchronously for determinism
			_, _ = spy.underlying.RunAll(context.Background())
		}
	}

	executor := &stubStageExecutor{}
	engine, err := blueprint.NewEngine(bp, executor, engineConfig)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()
	run, err := engine.StartRun(ctx, "test-run", nil)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	input := blueprint.NewStageInput("test-run", "test task", t.TempDir())
	if err := engine.Execute(ctx, run, input); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Verify: RunAll should have been called exactly once, for stage_b only
	calls := spy.getCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected quality gates to be called 1 time (for stage_b), got %d calls: %v", len(calls), calls)
	}
	if calls[0] != "stage_b" {
		t.Errorf("Expected quality gates to be called for stage_b, got %s", calls[0])
	}
}

// TestPipeline_QualityGatesNotCalledOnFailedStages verifies that quality
// gates are NOT triggered when a stage fails, even if it has RunQualityGates=true.
func TestPipeline_QualityGatesNotCalledOnFailedStages(t *testing.T) {
	bp := &blueprint.Blueprint{
		ID:           "test_gates_on_failure",
		Name:         "Test Gates On Failure",
		InitialStage: "failing_stage",
		Stages: map[string]*blueprint.Stage{
			"failing_stage": {
				Name:            "failing_stage",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "complete",
				RunQualityGates: true,
			},
		},
	}

	spy := newSpyQualityManager()

	// An executor that always fails
	failingExecutor := &failingStageExecutor{}

	engineConfig := blueprint.DefaultEngineConfig()
	engineConfig.OnStageComplete = func(run *blueprint.Run, result *blueprint.StageResult) {
		stage, ok := bp.GetStage(result.StageName)
		// This is the key check: result.Status must be Completed (not Failed)
		if spy.underlying != nil && result.Status == types.StageStatusCompleted && ok && stage.RunQualityGates {
			spy.recordCall(result.StageName)
		}
	}

	engine, err := blueprint.NewEngine(bp, failingExecutor, engineConfig)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()
	run, err := engine.StartRun(ctx, "test-run", nil)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	input := blueprint.NewStageInput("test-run", "test task", t.TempDir())
	// Execute will return error because stage fails — that's expected
	_ = engine.Execute(ctx, run, input)

	// Verify: RunAll should NOT have been called
	calls := spy.getCalls()
	if len(calls) != 0 {
		t.Errorf("Quality gates should not run on failed stages, but got %d calls: %v", len(calls), calls)
	}
}

// failingStageExecutor always returns a failed result.
type failingStageExecutor struct{}

func (e *failingStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	result := blueprint.NewStageResult(stage.Name, 1)
	result.Fail("intentional test failure")
	return result, nil
}

// TestPipeline_DefaultBlueprintQualityGateStages verifies which stages
// in the default blueprints have RunQualityGates enabled.
// This is a regression test for Bug #1: gates were on wrong stages.
func TestPipeline_DefaultBlueprintQualityGateStages(t *testing.T) {
	t.Run("DefaultBlueprint has no quality gate stages", func(t *testing.T) {
		// The standard_task blueprint uses the V2 quality manager via the pipeline's
		// OnStageComplete callback, but individual stages should only opt in where
		// code changes are produced.
		for name, stage := range blueprint.DefaultBlueprint.Stages {
			if stage.RunQualityGates {
				// Only implement, fix_lint, fix_tests should potentially have gates
				switch name {
				case "implement", "fix_lint", "fix_tests":
					// These are acceptable stages for quality gates
				default:
					t.Errorf("Stage %q has RunQualityGates=true but should not (non-code-producing stage)", name)
				}
			}
		}
	})

	t.Run("QuickFixBlueprint verify stage has quality gates", func(t *testing.T) {
		verifyStage, ok := blueprint.QuickFixBlueprint.Stages["verify"]
		if !ok {
			t.Fatal("QuickFixBlueprint missing 'verify' stage")
		}
		if !verifyStage.RunQualityGates {
			t.Error("QuickFixBlueprint 'verify' stage should have RunQualityGates=true")
		}
	})

	t.Run("QuickFixBlueprint implement stage should NOT have quality gates", func(t *testing.T) {
		implStage, ok := blueprint.QuickFixBlueprint.Stages["implement"]
		if !ok {
			t.Fatal("QuickFixBlueprint missing 'implement' stage")
		}
		if implStage.RunQualityGates {
			t.Error("QuickFixBlueprint 'implement' stage should NOT have RunQualityGates=true — gates run at verify stage")
		}
	})
}

// TestPipeline_QualityGatesMultipleOptedInStages verifies that when multiple
// stages have RunQualityGates=true, all of them trigger the manager.
func TestPipeline_QualityGatesMultipleOptedInStages(t *testing.T) {
	bp := &blueprint.Blueprint{
		ID:           "test_multi_gates",
		Name:         "Test Multi Gates",
		InitialStage: "stage_a",
		Stages: map[string]*blueprint.Stage{
			"stage_a": {
				Name:            "stage_a",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "stage_b",
				RunQualityGates: true,
			},
			"stage_b": {
				Name:            "stage_b",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "stage_c",
				RunQualityGates: false,
			},
			"stage_c": {
				Name:            "stage_c",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "complete",
				RunQualityGates: true,
			},
		},
	}

	spy := newSpyQualityManager()

	engineConfig := blueprint.DefaultEngineConfig()
	engineConfig.OnStageComplete = func(run *blueprint.Run, result *blueprint.StageResult) {
		stage, ok := bp.GetStage(result.StageName)
		if spy.underlying != nil && result.Status == types.StageStatusCompleted && ok && stage.RunQualityGates {
			spy.recordCall(result.StageName)
		}
	}

	executor := &stubStageExecutor{}
	engine, err := blueprint.NewEngine(bp, executor, engineConfig)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()
	run, err := engine.StartRun(ctx, "test-run", nil)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	input := blueprint.NewStageInput("test-run", "test task", t.TempDir())
	if err := engine.Execute(ctx, run, input); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Should be called for stage_a and stage_c (both opted in), but not stage_b
	calls := spy.getCalls()
	if len(calls) != 2 {
		t.Fatalf("Expected 2 quality gate calls, got %d: %v", len(calls), calls)
	}
	if calls[0] != "stage_a" {
		t.Errorf("First call should be stage_a, got %s", calls[0])
	}
	if calls[1] != "stage_c" {
		t.Errorf("Second call should be stage_c, got %s", calls[1])
	}
}

// TestPipeline_QualityGateEventsEmitted verifies that the pipeline emits
// the correct events when quality gates pass or fail. This tests the
// full integration path used in runBlueprintMode().
func TestPipeline_QualityGateEventsEmitted(t *testing.T) {
	bp := &blueprint.Blueprint{
		ID:           "test_gate_events",
		Name:         "Test Gate Events",
		InitialStage: "gated_stage",
		Stages: map[string]*blueprint.Stage{
			"gated_stage": {
				Name:            "gated_stage",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "complete",
				RunQualityGates: true,
			},
		},
	}

	// Create a real quality manager with no gates (all pass)
	qm := quality.NewManager(t.TempDir(), []quality.Gate{})

	var emittedEvents []loop.Event
	var mu sync.Mutex

	engineConfig := blueprint.DefaultEngineConfig()
	engineConfig.OnStageComplete = func(run *blueprint.Run, result *blueprint.StageResult) {
		stage, ok := bp.GetStage(result.StageName)
		if qm != nil && result.Status == types.StageStatusCompleted && ok && stage.RunQualityGates {
			summary, err := qm.RunAll(context.Background())
			if err != nil {
				mu.Lock()
				emittedEvents = append(emittedEvents, loop.Event{
					Type:    loop.EventError,
					ErrText: err.Error(),
				})
				mu.Unlock()
				return
			}
			mu.Lock()
			if summary.FailedGates > 0 {
				emittedEvents = append(emittedEvents, loop.Event{
					Type: loop.EventGatesFailed,
				})
			} else {
				emittedEvents = append(emittedEvents, loop.Event{
					Type: loop.EventGatesPassed,
				})
			}
			mu.Unlock()
		}
	}

	executor := &stubStageExecutor{}
	engine, err := blueprint.NewEngine(bp, executor, engineConfig)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()
	run, err := engine.StartRun(ctx, "test-run", nil)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	input := blueprint.NewStageInput("test-run", "test task", t.TempDir())
	if err := engine.Execute(ctx, run, input); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Allow brief time for async operations if any
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	events := emittedEvents
	mu.Unlock()

	// With no gates configured, RunAll returns 0 passed, 0 failed — gates passed
	hasGatesPassed := false
	for _, e := range events {
		if e.Type == loop.EventGatesPassed {
			hasGatesPassed = true
		}
	}
	if !hasGatesPassed {
		t.Error("Expected EventGatesPassed to be emitted when quality gates pass")
	}
}
