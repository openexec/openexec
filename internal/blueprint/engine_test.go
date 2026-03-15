package blueprint

import (
	"context"
	"errors"
	"testing"
	"time"
)

// MockExecutor is a test executor that can be configured to return specific results.
type MockExecutor struct {
	results   map[string]*StageResult
	errors    map[string]error
	callCount map[string]int
}

func NewMockExecutor() *MockExecutor {
	return &MockExecutor{
		results:   make(map[string]*StageResult),
		errors:    make(map[string]error),
		callCount: make(map[string]int),
	}
}

func (m *MockExecutor) SetResult(stageName string, result *StageResult) {
	m.results[stageName] = result
}

func (m *MockExecutor) SetError(stageName string, err error) {
	m.errors[stageName] = err
}

func (m *MockExecutor) Execute(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error) {
	m.callCount[stage.Name]++

	if err, ok := m.errors[stage.Name]; ok {
		return nil, err
	}

	if result, ok := m.results[stage.Name]; ok {
		return result, nil
	}

	// Default: return success
	result := NewStageResult(stage.Name, 1)
	result.Complete("success")
	return result, nil
}

func (m *MockExecutor) GetCallCount(stageName string) int {
	return m.callCount[stageName]
}

func TestBlueprint_Validate(t *testing.T) {
	tests := []struct {
		name      string
		blueprint *Blueprint
		wantErr   bool
	}{
		{
			name: "valid blueprint",
			blueprint: &Blueprint{
				ID:           "test",
				Name:         "Test",
				InitialStage: "start",
				Stages: map[string]*Stage{
					"start": {Name: "start", OnSuccess: "complete"},
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			blueprint: &Blueprint{
				Name:         "Test",
				InitialStage: "start",
				Stages: map[string]*Stage{
					"start": {Name: "start"},
				},
			},
			wantErr: true,
		},
		{
			name: "missing initial stage",
			blueprint: &Blueprint{
				ID:           "test",
				Name:         "Test",
				InitialStage: "nonexistent",
				Stages: map[string]*Stage{
					"start": {Name: "start"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid OnSuccess reference",
			blueprint: &Blueprint{
				ID:           "test",
				Name:         "Test",
				InitialStage: "start",
				Stages: map[string]*Stage{
					"start": {Name: "start", OnSuccess: "nonexistent"},
				},
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.blueprint.Validate()
			if (err != nil) != tc.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestEngine_Execute_SimpleFlow(t *testing.T) {
	// Simple two-stage blueprint
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {Name: "stage1", Type: StageTypeDeterministic, OnSuccess: "stage2"},
			"stage2": {Name: "stage2", Type: StageTypeDeterministic, OnSuccess: "complete"},
		},
	}

	executor := NewMockExecutor()
	engine, err := NewEngine(blueprint, executor, nil)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()
	input := NewStageInput("run1", "test task", "/tmp")

	run, err := engine.StartRun(ctx, "run1", input)
	if err != nil {
		t.Fatalf("StartRun failed: %v", err)
	}

	err = engine.Execute(ctx, run, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("run status = %s, want completed", run.Status)
	}

	if len(run.Results) != 2 {
		t.Errorf("results count = %d, want 2", len(run.Results))
	}

	if executor.GetCallCount("stage1") != 1 {
		t.Errorf("stage1 call count = %d, want 1", executor.GetCallCount("stage1"))
	}
	if executor.GetCallCount("stage2") != 1 {
		t.Errorf("stage2 call count = %d, want 1", executor.GetCallCount("stage2"))
	}
}

func TestEngine_Execute_WithRetry(t *testing.T) {
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {
				Name:       "stage1",
				Type:       StageTypeAgentic,
				MaxRetries: 2,
				OnSuccess:  "complete",
				OnFailure:  "stage1", // Retry same stage
			},
		},
	}

	failCount := 0

	// Custom executor that fails twice then succeeds
	customExecutor := &customMockExecutor{
		execute: func(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error) {
			failCount++
			result := NewStageResult(stage.Name, failCount)
			if failCount <= 2 {
				result.Fail("temporary failure")
			} else {
				result.Complete("success after retries")
			}
			return result, nil
		},
	}

	engine, err := NewEngine(blueprint, customExecutor, nil)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	ctx := context.Background()
	input := NewStageInput("run1", "test task", "/tmp")

	run, _ := engine.StartRun(ctx, "run1", input)
	err = engine.Execute(ctx, run, input)

	if err != nil {
		t.Fatalf("Execute should succeed after retries: %v", err)
	}

	if run.Status != RunStatusCompleted {
		t.Errorf("run status = %s, want completed", run.Status)
	}

	if failCount != 3 {
		t.Errorf("expected 3 attempts (2 failures + 1 success), got %d", failCount)
	}
}

func TestEngine_Execute_MaxRetriesExceeded(t *testing.T) {
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {
				Name:       "stage1",
				Type:       StageTypeAgentic,
				MaxRetries: 2,
				OnSuccess:  "complete",
				OnFailure:  "stage1",
			},
		},
	}

	// Always fail
	customExecutor := &customMockExecutor{
		execute: func(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error) {
			result := NewStageResult(stage.Name, 1)
			result.Fail("persistent failure")
			return result, nil
		},
	}

	engine, _ := NewEngine(blueprint, customExecutor, nil)
	ctx := context.Background()
	input := NewStageInput("run1", "test task", "/tmp")

	run, _ := engine.StartRun(ctx, "run1", input)
	err := engine.Execute(ctx, run, input)

	if err == nil {
		t.Error("expected error after max retries exceeded")
	}

	if run.Status != RunStatusFailed {
		t.Errorf("run status = %s, want failed", run.Status)
	}
}

func TestEngine_Execute_ContextCancellation(t *testing.T) {
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {Name: "stage1", OnSuccess: "stage2"},
			"stage2": {Name: "stage2", OnSuccess: "complete"},
		},
	}

	executor := NewMockExecutor()
	engine, _ := NewEngine(blueprint, executor, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	input := NewStageInput("run1", "test task", "/tmp")
	run, _ := engine.StartRun(ctx, "run1", input)
	err := engine.Execute(ctx, run, input)

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	if run.Status != RunStatusCancelled {
		t.Errorf("run status = %s, want cancelled", run.Status)
	}
}

func TestEngine_Callbacks(t *testing.T) {
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {Name: "stage1", OnSuccess: "complete", CreateCheckpoint: true},
		},
	}

	executor := NewMockExecutor()

	var stageCompleteCalled bool
	var checkpointCalled bool
	var runCompleteCalled bool

	config := &EngineConfig{
		MaxTotalRetries: 10,
		DefaultTimeout:  5 * time.Minute,
		OnStageComplete: func(run *Run, result *StageResult) {
			stageCompleteCalled = true
		},
		OnCheckpoint: func(run *Run, stageName string) {
			checkpointCalled = true
		},
		OnRunComplete: func(run *Run) {
			runCompleteCalled = true
		},
	}

	engine, _ := NewEngine(blueprint, executor, config)
	ctx := context.Background()
	input := NewStageInput("run1", "test task", "/tmp")

	run, _ := engine.StartRun(ctx, "run1", input)
	engine.Execute(ctx, run, input)

	if !stageCompleteCalled {
		t.Error("OnStageComplete callback not called")
	}
	if !checkpointCalled {
		t.Error("OnCheckpoint callback not called")
	}
	if !runCompleteCalled {
		t.Error("OnRunComplete callback not called")
	}
}

func TestEngine_PauseResume(t *testing.T) {
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {Name: "stage1", OnSuccess: "complete"},
		},
	}

	executor := NewMockExecutor()
	engine, _ := NewEngine(blueprint, executor, nil)

	ctx := context.Background()
	input := NewStageInput("run1", "test task", "/tmp")

	run, _ := engine.StartRun(ctx, "run1", input)

	// Pause
	err := engine.Pause("run1")
	if err != nil {
		t.Fatalf("Pause failed: %v", err)
	}
	if run.Status != RunStatusPaused {
		t.Errorf("status = %s, want paused", run.Status)
	}

	// Resume
	err = engine.Resume("run1")
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}
	if run.Status != RunStatusRunning {
		t.Errorf("status = %s, want running", run.Status)
	}

	// Cancel
	err = engine.Cancel("run1")
	if err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	if run.Status != RunStatusCancelled {
		t.Errorf("status = %s, want cancelled", run.Status)
	}
}

func TestStageResult_Complete(t *testing.T) {
	result := NewStageResult("test", 1)
	result.Complete("output data")

	if result.Status != StageStatusCompleted {
		t.Errorf("status = %s, want completed", result.Status)
	}
	if result.Output != "output data" {
		t.Errorf("output = %q, want %q", result.Output, "output data")
	}
	// Duration can be zero if completion is instant, just verify it's set
	if result.Duration < 0 {
		t.Error("duration should be non-negative")
	}
	if result.CompletedAt.IsZero() {
		t.Error("completedAt should be set")
	}
}

func TestStageResult_Fail(t *testing.T) {
	result := NewStageResult("test", 1)
	result.Fail("error message")

	if result.Status != StageStatusFailed {
		t.Errorf("status = %s, want failed", result.Status)
	}
	if result.Error != "error message" {
		t.Errorf("error = %q, want %q", result.Error, "error message")
	}
}

func TestStageInput_PreviousResults(t *testing.T) {
	input := NewStageInput("run1", "test", "/tmp")

	if input.GetLastResult() != nil {
		t.Error("expected nil for empty results")
	}

	r1 := NewStageResult("stage1", 1)
	r1.Complete("output1")
	input.AddPreviousResult(r1)

	if input.GetLastResult() != r1 {
		t.Error("expected r1 as last result")
	}

	r2 := NewStageResult("stage2", 1)
	r2.Fail("error")
	input.AddPreviousResult(r2)

	if !input.HasFailedStage() {
		t.Error("expected HasFailedStage to return true")
	}
}

func TestRun_Checkpoints(t *testing.T) {
	run := NewRun("run1", "blueprint1", "stage1")

	run.CurrentStage = "gather_context"
	run.AddCheckpoint()

	run.CurrentStage = "implement"
	run.AddCheckpoint()

	if len(run.Checkpoints) != 2 {
		t.Errorf("checkpoint count = %d, want 2", len(run.Checkpoints))
	}

	if run.Checkpoints[0] != "gather_context" {
		t.Errorf("checkpoint[0] = %s, want gather_context", run.Checkpoints[0])
	}
}

func TestDefaultBlueprint_Validate(t *testing.T) {
	if err := DefaultBlueprint.Validate(); err != nil {
		t.Errorf("DefaultBlueprint validation failed: %v", err)
	}
}

func TestQuickFixBlueprint_Validate(t *testing.T) {
	if err := QuickFixBlueprint.Validate(); err != nil {
		t.Errorf("QuickFixBlueprint validation failed: %v", err)
	}
}

// customMockExecutor allows custom execute logic.
type customMockExecutor struct {
	execute func(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error)
}

func (c *customMockExecutor) Execute(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error) {
	return c.execute(ctx, stage, input)
}

func TestEngine_Execute_StageError(t *testing.T) {
	blueprint := &Blueprint{
		ID:           "test",
		Name:         "Test",
		InitialStage: "stage1",
		Stages: map[string]*Stage{
			"stage1": {Name: "stage1", OnSuccess: "complete"},
		},
	}

	executor := NewMockExecutor()
	executor.SetError("stage1", errors.New("executor error"))

	engine, _ := NewEngine(blueprint, executor, nil)
	ctx := context.Background()
	input := NewStageInput("run1", "test task", "/tmp")

	run, _ := engine.StartRun(ctx, "run1", input)
	err := engine.Execute(ctx, run, input)

	if err == nil {
		t.Error("expected error from executor")
	}

	if run.Status != RunStatusFailed {
		t.Errorf("run status = %s, want failed", run.Status)
	}
}
