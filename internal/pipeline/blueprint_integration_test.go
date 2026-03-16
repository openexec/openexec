package pipeline

import (
	"context"
	"testing"
)

// SpyGateRunner implements a mock quality gate runner.
type SpyGateRunner struct {
	OnRunAll func() error
}

func (s *SpyGateRunner) RunAll(ctx context.Context) error {
	if s.OnRunAll != nil {
		return s.OnRunAll()
	}
	return nil
}

// TestBlueprintExecutor_InvokesQualityGates proves that when the blueprint
// executor runs a stage, it calls the quality gate runner.
//
// This test currently FAILS because the wiring does not exist.
// It will pass once Stage 3 is complete.
func TestBlueprintExecutor_InvokesQualityGates(t *testing.T) {
	gatesCalled := false
	spyRunner := &SpyGateRunner{
		OnRunAll: func() error {
			gatesCalled = true
			return nil
		},
	}

	// We create a pipeline and we want to ensure it uses our spyRunner.
	// Since Pipeline currently doesn't accept a GateRunner, we expect
	// this to require modifications to the New() function or a new
	// WithGateRunner option.
	
	// For Stage 2, we will try to use a hypothetical WithGateRunner option.
	// This will fail to compile initially.
	
	cfg := Config{
		FWUID:           "test-gate-wiring",
		WorkDir:         t.TempDir(),
		BlueprintID:     "quick_fix",
		TaskDescription: "Fix something",
		// Mock a successful execution
		CommandName: "echo",
		CommandArgs: []string{"phase-complete"},
	}

	p, events := New(cfg, WithGateRunner(spyRunner))

	// Drain events
	go func() {
		for range events {
		}
	}()

	ctx := context.Background()
	_ = p.Run(ctx)

	if !gatesCalled {
		t.Fatal("quality gates were not invoked during blueprint execution — this is the gap being fixed")
	}
}

// TestBlueprintExecutor_EmptyStage_StillRunsGates is the regression test
// for the original bug: a stage with no commands auto-succeeded without
// invoking quality gates.
func TestBlueprintExecutor_EmptyStage_StillRunsGates(t *testing.T) {
	gatesCalled := false
	spyRunner := &SpyGateRunner{
		OnRunAll: func() error {
			gatesCalled = true
			return nil
		},
	}

	cfg := Config{
		FWUID:           "test-empty-stage",
		WorkDir:         t.TempDir(),
		BlueprintID:     "quick_fix",
		TaskDescription: "Verify empty stage gating",
		// Override to successfully mock execution for all stages
		CommandName: "echo",
		CommandArgs: []string{"phase-complete"},
	}

	p, events := New(cfg, WithGateRunner(spyRunner))

	// Drain events
	go func() {
		for range events {
		}
	}()

	err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !gatesCalled {
		t.Fatal("an empty-command stage bypassed quality gates — original bug has regressed")
	}
}
