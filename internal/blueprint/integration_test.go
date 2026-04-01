package blueprint

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/types"
)

// mockExecutor records which stages were executed and in what order.
// It also tracks quality gate invocations when used with a gate-aware wrapper.
type mockExecutor struct {
	stagesExecuted []string
	failStages     map[string]bool
	mu             sync.Mutex
}

func newMockExecutor() *mockExecutor {
	return &mockExecutor{
		failStages: make(map[string]bool),
	}
}

func (m *mockExecutor) Execute(_ context.Context, stage *Stage, _ *StageInput) (*StageResult, error) {
	m.mu.Lock()
	m.stagesExecuted = append(m.stagesExecuted, stage.Name)
	m.mu.Unlock()

	result := NewStageResult(stage.Name, 1)
	if m.failStages[stage.Name] {
		result.Fail("simulated failure")
		return result, nil
	}
	result.Complete("ok")
	return result, nil
}

func (m *mockExecutor) getStagesExecuted() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.stagesExecuted))
	copy(out, m.stagesExecuted)
	return out
}

// gateTrackingExecutor wraps a StageExecutor and records which stages triggered quality gate checks.
// It simulates the RunQualityGates field check that the real DefaultExecutor performs.
type gateTrackingExecutor struct {
	inner        *mockExecutor
	gatesInvoked []string
	mu           sync.Mutex
}

func newGateTrackingExecutor(inner *mockExecutor) *gateTrackingExecutor {
	return &gateTrackingExecutor{inner: inner}
}

func (g *gateTrackingExecutor) Execute(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error) {
	result, err := g.inner.Execute(ctx, stage, input)
	if err != nil {
		return result, err
	}

	// Simulate what DefaultExecutor does: check RunQualityGates and invoke gates
	if stage.RunQualityGates && result.Status == types.StageStatusCompleted {
		g.mu.Lock()
		g.gatesInvoked = append(g.gatesInvoked, stage.Name)
		g.mu.Unlock()
	}

	return result, err
}

func (g *gateTrackingExecutor) getGatesInvoked() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]string, len(g.gatesInvoked))
	copy(out, g.gatesInvoked)
	return out
}

// --- Stage ordering tests ---

func TestBlueprint_DefaultStageOrder(t *testing.T) {
	mock := newMockExecutor()
	engine, err := NewEngine(DefaultBlueprint, mock, DefaultEngineConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "test task", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "test task", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := mock.getStagesExecuted()
	want := []string{"gather_context", "implement", "lint", "test", "review"}

	if len(got) != len(want) {
		t.Fatalf("stage count mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stage[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("run status = %v, want %v", run.Status, RunStatusCompleted)
	}
}

func TestBlueprint_QuickFixStageOrder(t *testing.T) {
	mock := newMockExecutor()
	engine, err := NewEngine(QuickFixBlueprint, mock, DefaultEngineConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "quick fix", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "quick fix", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := mock.getStagesExecuted()
	want := []string{"implement", "verify"}

	if len(got) != len(want) {
		t.Fatalf("stage count mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stage[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("run status = %v, want %v", run.Status, RunStatusCompleted)
	}
}

func TestBlueprint_StageFailureGoesToOnFailure(t *testing.T) {
	// Build a simple blueprint: step_a -> step_b -> complete
	// step_a fails and has OnFailure = "step_recovery"
	// step_recovery succeeds and goes to step_b
	bp := &Blueprint{
		ID:   "fail-test",
		Name: "Failure Test",
		Stages: map[string]*Stage{
			"step_a": {
				Name:       "step_a",
				Type:       types.StageTypeDeterministic,
				OnSuccess:  "step_b",
				OnFailure:  "step_recovery",
				MaxRetries: 1,
			},
			"step_recovery": {
				Name:      "step_recovery",
				Type:      types.StageTypeDeterministic,
				OnSuccess: "step_b",
			},
			"step_b": {
				Name:      "step_b",
				Type:      types.StageTypeDeterministic,
				OnSuccess: "complete",
			},
		},
		InitialStage: "step_a",
	}

	mock := newMockExecutor()
	mock.failStages["step_a"] = true

	engine, err := NewEngine(bp, mock, DefaultEngineConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "fail test", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "fail test", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	got := mock.getStagesExecuted()
	// step_a fails -> goes to step_recovery -> step_b -> complete
	want := []string{"step_a", "step_recovery", "step_b"}

	if len(got) != len(want) {
		t.Fatalf("stage count mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("stage[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("run status = %v, want %v", run.Status, RunStatusCompleted)
	}
}

// --- Quality gates placement tests ---

func TestBlueprint_QualityGatesOnlyOnOptedInStages(t *testing.T) {
	// Blueprint with 3 stages; only "verify" has RunQualityGates: true
	bp := &Blueprint{
		ID:   "gate-test",
		Name: "Gate Test",
		Stages: map[string]*Stage{
			"build": {
				Name:            "build",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "verify",
				RunQualityGates: false,
			},
			"verify": {
				Name:            "verify",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "finalize",
				RunQualityGates: true,
			},
			"finalize": {
				Name:            "finalize",
				Type:            types.StageTypeDeterministic,
				OnSuccess:       "complete",
				RunQualityGates: false,
			},
		},
		InitialStage: "build",
	}

	mock := newMockExecutor()
	tracker := newGateTrackingExecutor(mock)

	engine, err := NewEngine(bp, tracker, DefaultEngineConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "gate test", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "gate test", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// All 3 stages should have executed
	executed := mock.getStagesExecuted()
	if len(executed) != 3 {
		t.Fatalf("expected 3 stages executed, got %v", executed)
	}

	// Quality gates should have been invoked ONLY for "verify"
	gated := tracker.getGatesInvoked()
	if len(gated) != 1 {
		t.Fatalf("expected gates invoked for 1 stage, got %v", gated)
	}
	if gated[0] != "verify" {
		t.Errorf("gates invoked for %q, want %q", gated[0], "verify")
	}
}

func TestBlueprint_QualityGatesNotCalledOnGatherContext(t *testing.T) {
	// Use the DefaultBlueprint — gather_context does not have RunQualityGates
	mock := newMockExecutor()
	tracker := newGateTrackingExecutor(mock)

	engine, err := NewEngine(DefaultBlueprint, tracker, DefaultEngineConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "test", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "test", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// No stage in DefaultBlueprint has RunQualityGates: true,
	// so gates should never be invoked.
	gated := tracker.getGatesInvoked()
	for _, name := range gated {
		if name == "gather_context" {
			t.Errorf("quality gates were invoked on gather_context, which should not happen")
		}
	}

	// Additionally verify that gather_context itself was executed (sanity check)
	executed := mock.getStagesExecuted()
	if len(executed) == 0 || executed[0] != "gather_context" {
		t.Errorf("expected gather_context as first stage, got %v", executed)
	}
}

// --- Callback tests ---

func TestBlueprint_OnStageStartCalledForEachStage(t *testing.T) {
	mock := newMockExecutor()

	var startedStages []string
	var mu sync.Mutex

	config := DefaultEngineConfig()
	config.OnStageStart = func(run *Run, stageName string) {
		mu.Lock()
		startedStages = append(startedStages, stageName)
		mu.Unlock()
	}

	engine, err := NewEngine(QuickFixBlueprint, mock, config)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "cb test", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "cb test", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	want := []string{"implement", "verify"}
	if len(startedStages) != len(want) {
		t.Fatalf("OnStageStart called %d times, want %d: %v", len(startedStages), len(want), startedStages)
	}
	for i := range want {
		if startedStages[i] != want[i] {
			t.Errorf("OnStageStart[%d] = %q, want %q", i, startedStages[i], want[i])
		}
	}
}

func TestBlueprint_OnStageCompleteCalledForEachStage(t *testing.T) {
	mock := newMockExecutor()

	var completedResults []*StageResult
	var mu sync.Mutex

	config := DefaultEngineConfig()
	config.OnStageComplete = func(run *Run, result *StageResult) {
		mu.Lock()
		completedResults = append(completedResults, result)
		mu.Unlock()
	}

	engine, err := NewEngine(QuickFixBlueprint, mock, config)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "cb test", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "cb test", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	want := []string{"implement", "verify"}
	if len(completedResults) != len(want) {
		t.Fatalf("OnStageComplete called %d times, want %d", len(completedResults), len(want))
	}
	for i, name := range want {
		if completedResults[i].StageName != name {
			t.Errorf("OnStageComplete[%d].StageName = %q, want %q", i, completedResults[i].StageName, name)
		}
		if completedResults[i].Status != types.StageStatusCompleted {
			t.Errorf("OnStageComplete[%d].Status = %v, want %v", i, completedResults[i].Status, types.StageStatusCompleted)
		}
	}
}

func TestBlueprint_OnRunCompleteCalledOnSuccess(t *testing.T) {
	mock := newMockExecutor()

	var completedRun *Run
	var mu sync.Mutex

	config := DefaultEngineConfig()
	config.OnRunComplete = func(run *Run) {
		mu.Lock()
		completedRun = run
		mu.Unlock()
	}

	engine, err := NewEngine(QuickFixBlueprint, mock, config)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	run, err := engine.StartRun(ctx, "run-1", NewStageInput("run-1", "cb test", "/tmp"))
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}

	err = engine.Execute(ctx, run, NewStageInput("run-1", "cb test", "/tmp"))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if completedRun == nil {
		t.Fatal("OnRunComplete was not called")
	}
	if completedRun.Status != RunStatusCompleted {
		t.Errorf("completed run status = %v, want %v", completedRun.Status, RunStatusCompleted)
	}
	if completedRun.ID != "run-1" {
		t.Errorf("completed run ID = %q, want %q", completedRun.ID, "run-1")
	}
}
