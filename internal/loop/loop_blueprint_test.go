package loop

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/types"
)

// mockStageExecutor is a simple stage executor for testing.
type mockStageExecutor struct {
	results map[string]*blueprint.StageResult
}

func (e *mockStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	result := blueprint.NewStageResult(stage.Name, 1)
	if r, ok := e.results[stage.Name]; ok {
		return r, nil
	}
	result.Complete("success")
	return result, nil
}

func TestLoop_BlueprintMode_Initialization(t *testing.T) {
	// Create a simple test blueprint
	bp := &blueprint.Blueprint{
		ID:   "test-blueprint",
		Name: "Test Blueprint",
		Stages: map[string]*blueprint.Stage{
			"start": {
				Name:      "start",
				Type:      types.StageTypeAgentic,
				OnSuccess: "complete",
			},
		},
		InitialStage: "start",
	}

	executor := &mockStageExecutor{
		results: map[string]*blueprint.StageResult{},
	}

	cfg := DefaultConfig()
	cfg.BlueprintEnabled = true
	cfg.Blueprint = bp
	cfg.BlueprintExecutor = executor

	l, events := New(cfg)

	// Check that blueprint engine was initialized
	if l.blueprintEngine == nil {
		t.Error("Expected blueprint engine to be initialized")
	}

	// Check that we can read events channel
	if events == nil {
		t.Error("Expected events channel to be initialized")
	}
}

func TestLoop_BlueprintMode_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BlueprintEnabled = false

	l, _ := New(cfg)

	// Blueprint engine should not be initialized when disabled
	if l.blueprintEngine != nil {
		t.Error("Expected blueprint engine to be nil when disabled")
	}
}

func TestLoop_BlueprintMode_DefaultBlueprint(t *testing.T) {
	executor := &mockStageExecutor{
		results: map[string]*blueprint.StageResult{},
	}

	cfg := DefaultConfig()
	cfg.BlueprintEnabled = true
	cfg.Blueprint = nil // Should use default
	cfg.BlueprintExecutor = executor

	l, _ := New(cfg)

	// Should use default blueprint
	if l.blueprintEngine == nil {
		t.Error("Expected blueprint engine to be initialized with default blueprint")
	}

	if l.blueprintEngine.GetBlueprint().ID != "standard_task" {
		t.Errorf("Expected default blueprint ID to be 'standard_task', got %q", l.blueprintEngine.GetBlueprint().ID)
	}
}

func TestLoop_BlueprintMode_SimpleExecution(t *testing.T) {
	// Create a simple two-stage blueprint
	bp := &blueprint.Blueprint{
		ID:   "test-simple",
		Name: "Test Simple",
		Stages: map[string]*blueprint.Stage{
			"stage1": {
				Name:      "stage1",
				Type:      types.StageTypeDeterministic,
				OnSuccess: "stage2",
			},
			"stage2": {
				Name:      "stage2",
				Type:      types.StageTypeAgentic,
				OnSuccess: "complete",
			},
		},
		InitialStage: "stage1",
	}

	result1 := blueprint.NewStageResult("stage1", 1)
	result1.Complete("stage1 done")

	result2 := blueprint.NewStageResult("stage2", 1)
	result2.Complete("stage2 done")

	executor := &mockStageExecutor{
		results: map[string]*blueprint.StageResult{
			"stage1": result1,
			"stage2": result2,
		},
	}

	cfg := DefaultConfig()
	cfg.BlueprintEnabled = true
	cfg.Blueprint = bp
	cfg.BlueprintExecutor = executor
	cfg.FwuID = "test-fwu"
	cfg.WorkDir = t.TempDir()

	l, events := New(cfg)

	// Run the loop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = l.Run(ctx)
	}()

	// Collect events
	var collectedEvents []Event
	for e := range events {
		collectedEvents = append(collectedEvents, e)
		if e.Type == EventComplete || e.Type == EventBlueprintComplete {
			break
		}
	}

	// Verify we got the expected events
	hasStage1Start := false
	hasStage1Complete := false
	hasStage2Start := false
	hasStage2Complete := false
	hasBlueprintStart := false
	hasBlueprintComplete := false

	for _, e := range collectedEvents {
		switch e.Type {
		case EventBlueprintStart:
			hasBlueprintStart = true
		case EventBlueprintComplete:
			hasBlueprintComplete = true
		case EventStageStart:
			if e.StageName == "stage1" {
				hasStage1Start = true
			}
			if e.StageName == "stage2" {
				hasStage2Start = true
			}
		case EventStageComplete:
			if e.StageName == "stage1" {
				hasStage1Complete = true
			}
			if e.StageName == "stage2" {
				hasStage2Complete = true
			}
		}
	}

	if !hasBlueprintStart {
		t.Error("Expected EventBlueprintStart event")
	}
	if !hasStage1Start {
		t.Error("Expected stage1 start event")
	}
	if !hasStage1Complete {
		t.Error("Expected stage1 complete event")
	}
	if !hasStage2Start {
		t.Error("Expected stage2 start event")
	}
	if !hasStage2Complete {
		t.Error("Expected stage2 complete event")
	}
	if !hasBlueprintComplete {
		t.Error("Expected EventBlueprintComplete event")
	}
}

func TestLoop_BlueprintMode_Callbacks(t *testing.T) {
	bp := &blueprint.Blueprint{
		ID:   "test-callbacks",
		Name: "Test Callbacks",
		Stages: map[string]*blueprint.Stage{
			"stage1": {
				Name:             "stage1",
				Type:             types.StageTypeDeterministic,
				OnSuccess:        "complete",
				CreateCheckpoint: true,
			},
		},
		InitialStage: "stage1",
	}

	result1 := blueprint.NewStageResult("stage1", 1)
	result1.Complete("stage1 done")

	executor := &mockStageExecutor{
		results: map[string]*blueprint.StageResult{
			"stage1": result1,
		},
	}

	var stageStartCalled bool
	var stageCompleteCalled bool
	var checkpointCalled bool
	var runCompleteCalled bool

	cfg := DefaultConfig()
	cfg.BlueprintEnabled = true
	cfg.Blueprint = bp
	cfg.BlueprintExecutor = executor
	cfg.FwuID = "test-fwu"
	cfg.WorkDir = t.TempDir()
	cfg.BlueprintCallbacks = &BlueprintCallbacks{
		OnStageStart: func(run *blueprint.Run, stage *blueprint.Stage) {
			stageStartCalled = true
		},
		OnStageComplete: func(run *blueprint.Run, result *blueprint.StageResult) {
			stageCompleteCalled = true
		},
		OnCheckpoint: func(run *blueprint.Run, stageName string) {
			checkpointCalled = true
		},
		OnRunComplete: func(run *blueprint.Run) {
			runCompleteCalled = true
		},
	}

	l, events := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = l.Run(ctx)
	}()

	// Drain events
	for e := range events {
		if e.Type == EventComplete || e.Type == EventBlueprintComplete {
			break
		}
	}

	if !stageStartCalled {
		t.Error("Expected OnStageStart callback to be called")
	}
	if !stageCompleteCalled {
		t.Error("Expected OnStageComplete callback to be called")
	}
	if !checkpointCalled {
		t.Error("Expected OnCheckpoint callback to be called")
	}
	if !runCompleteCalled {
		t.Error("Expected OnRunComplete callback to be called")
	}
}

func TestLoop_BlueprintMode_StageFailureWithRetry(t *testing.T) {
	bp := &blueprint.Blueprint{
		ID:   "test-retry",
		Name: "Test Retry",
		Stages: map[string]*blueprint.Stage{
			"stage1": {
				Name:       "stage1",
				Type:       types.StageTypeAgentic,
				MaxRetries: 2,
				OnSuccess:  "complete",
				OnFailure:  "stage1", // Retry same stage
			},
		},
		InitialStage: "stage1",
	}

	customExecutor := &mockRetryExecutor{
		callCount: 0,
	}

	cfg := DefaultConfig()
	cfg.BlueprintEnabled = true
	cfg.Blueprint = bp
	cfg.BlueprintExecutor = customExecutor
	cfg.FwuID = "test-fwu"
	cfg.WorkDir = t.TempDir()

	l, events := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		_ = l.Run(ctx)
	}()

	// Collect events
	hasRetry := false
	hasComplete := false

	for e := range events {
		if e.Type == EventStageRetry {
			hasRetry = true
		}
		if e.Type == EventComplete || e.Type == EventBlueprintComplete {
			hasComplete = true
			break
		}
	}

	if !hasRetry {
		t.Error("Expected at least one retry event")
	}
	if !hasComplete {
		t.Error("Expected completion event after retry")
	}
}

// mockRetryExecutor counts calls and fails on first call
type mockRetryExecutor struct {
	callCount int
}

func (e *mockRetryExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	e.callCount++
	result := blueprint.NewStageResult(stage.Name, e.callCount)

	if e.callCount == 1 {
		result.Fail("simulated failure")
	} else {
		result.Complete("success")
	}
	return result, nil
}
