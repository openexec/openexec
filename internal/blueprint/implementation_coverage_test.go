package blueprint

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestEngineUnitFailureAndSingleStage(t *testing.T) {
	ctx := context.Background()
	bp := &Blueprint{ID: "unit", Name: "unit", InitialStage: "test", Stages: map[string]*Stage{"test": {Name: "test", OnSuccess: "complete", OnFailure: "test", MaxRetries: 2}}}
	executor := NewMockExecutor()
	engine, err := NewEngine(bp, executor, &EngineConfig{MaxTotalRetries: 0})
	if err != nil {
		t.Fatal(err)
	}
	run, _ := engine.StartRun(ctx, "A", nil)
	input := NewStageInput("A", "unit", t.TempDir())
	run.IncrementRetries("test")
	result, err := engine.ExecuteStage(ctx, run, "test", input)
	if err != nil || result == nil || input.StageAttempt != 2 || len(run.Results) != 1 {
		t.Fatalf("single stage: result=%v error=%v input=%+v", result, err, input)
	}
	if _, err := engine.ExecuteStage(ctx, run, "missing", input); err == nil {
		t.Fatal("missing stage accepted")
	}
	executor.SetError("test", errors.New("executor failed"))
	if _, err := engine.ExecuteStage(ctx, run, "test", input); err == nil || len(run.Results) != 1 {
		t.Fatal("executor failure was recorded as success")
	}
	run, _ = engine.StartRun(ctx, "B", nil)
	if err := engine.Execute(ctx, run, input); err == nil || !strings.Contains(err.Error(), "maximum total retries") {
		t.Fatalf("retry limit: %v", err)
	}
	run.CurrentStage = "missing"
	if err := engine.Execute(ctx, run, input); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing stage: %v", err)
	}
}
