package loop

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestLoop_ExitStrategyC003_CleanExit validates constraint C-003:
// Self-healing must not enter infinite loops.
//
// GIVEN a Loop with ThrashThreshold=3, MaxIterations=10
// AND a mock process that completes without sending signals
// WHEN the loop runs for 3 iterations
// THEN EventThrashingDetected is emitted
// AND loop.Run() returns nil (not an error)
func TestLoop_ExitStrategyC003_CleanExit(t *testing.T) {
	mockPath, err := filepath.Abs("testdata/mock_claude")
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}

	cfg := DefaultConfig()
	cfg.CommandName = mockPath
	cfg.CommandArgs = []string{"no-progress"} // Completes without progress signals
	cfg.ThrashThreshold = 3
	cfg.MaxIterations = 10

	l, events := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	var thrashingDetected bool
	var thrashingIteration int

	go func() {
		defer close(done)
		for e := range events {
			if e.Type == EventThrashingDetected {
				thrashingDetected = true
				thrashingIteration = e.Iteration
			}
		}
	}()

	err = l.Run(ctx)

	<-done

	// Loop should exit cleanly (nil error), not crash
	if err != nil {
		t.Errorf("expected nil error (clean exit), got: %v", err)
	}

	// EventThrashingDetected should have been emitted
	if !thrashingDetected {
		t.Error("expected EventThrashingDetected to be emitted")
	}

	// Should have exited after 3 iterations (thrashThreshold)
	if thrashingIteration != 3 {
		t.Errorf("expected thrashing detected at iteration 3, got: %d", thrashingIteration)
	}
}

// TestLoop_ExitStrategyC003_ProgressResets validates that progress signals
// reset the thrashing counter, allowing the loop to continue.
//
// GIVEN a Loop with ThrashThreshold=3, MaxIterations=10
// AND a mock process that sends progress signal on each iteration
// WHEN the loop runs
// THEN no EventThrashingDetected is emitted (before MaxIterations)
func TestLoop_ExitStrategyC003_ProgressResets(t *testing.T) {
	mockPath, err := filepath.Abs("testdata/mock_claude")
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}

	cfg := DefaultConfig()
	cfg.CommandName = mockPath
	cfg.CommandArgs = []string{"signal-progress"} // Sends progress signal
	cfg.ThrashThreshold = 3
	cfg.MaxIterations = 5

	l, events := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	var thrashingDetected bool
	var maxIterationsReached bool

	go func() {
		defer close(done)
		for e := range events {
			if e.Type == EventThrashingDetected {
				thrashingDetected = true
			}
			if e.Type == EventMaxIterationsReached {
				maxIterationsReached = true
			}
		}
	}()

	err = l.Run(ctx)

	<-done

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// With progress signals, should NOT have thrashing detected
	if thrashingDetected {
		t.Error("thrashing should NOT be detected when progress signals are sent")
	}

	// Should have reached max iterations instead
	if !maxIterationsReached {
		t.Error("expected MaxIterationsReached when progress resets thrashing counter")
	}
}

// TestLoop_ExitStrategyC003_DisabledWhenZero validates that thrashing detection
// can be disabled by setting ThrashThreshold to 0.
//
// GIVEN a Loop with ThrashThreshold=0, MaxIterations=5
// AND a mock process that completes without sending signals
// WHEN the loop runs
// THEN no EventThrashingDetected is emitted
// AND the loop runs until MaxIterations
func TestLoop_ExitStrategyC003_DisabledWhenZero(t *testing.T) {
	mockPath, err := filepath.Abs("testdata/mock_claude")
	if err != nil {
		t.Fatalf("failed to get abs path: %v", err)
	}

	cfg := DefaultConfig()
	cfg.CommandName = mockPath
	cfg.CommandArgs = []string{"no-progress"} // Completes without progress signals
	cfg.ThrashThreshold = 0                   // Disabled
	cfg.MaxIterations = 3

	l, events := New(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	var thrashingDetected bool
	var maxIterationsReached bool
	var finalIteration int

	go func() {
		defer close(done)
		for e := range events {
			if e.Type == EventThrashingDetected {
				thrashingDetected = true
			}
			if e.Type == EventMaxIterationsReached {
				maxIterationsReached = true
				finalIteration = e.Iteration
			}
		}
	}()

	err = l.Run(ctx)

	<-done

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Thrashing detection is disabled
	if thrashingDetected {
		t.Error("thrashing should NOT be detected when ThrashThreshold=0")
	}

	// Should have hit max iterations
	if !maxIterationsReached {
		t.Error("expected MaxIterationsReached when thrashing is disabled")
	}

	if finalIteration != 3 {
		t.Errorf("expected final iteration 3, got: %d", finalIteration)
	}
}

// TestSignalTracker_CheckThrashing directly tests the SignalTracker logic.
func TestSignalTracker_CheckThrashing(t *testing.T) {
	t.Run("returns true when threshold exceeded", func(t *testing.T) {
		tracker := NewSignalTracker(3)

		// No progress recorded, iteration 3 should trigger thrashing
		if !tracker.CheckThrashing(3) {
			t.Error("expected CheckThrashing(3) to return true with threshold 3 and no progress")
		}
	})

	t.Run("returns false when progress recorded", func(t *testing.T) {
		tracker := NewSignalTracker(3)

		// Record progress at iteration 2
		tracker.RecordSignal("progress", 2)

		// Iteration 4 (2 iterations since progress) should NOT trigger
		if tracker.CheckThrashing(4) {
			t.Error("expected CheckThrashing(4) to return false (only 2 iterations since progress)")
		}

		// Iteration 5 (3 iterations since progress) should trigger
		if !tracker.CheckThrashing(5) {
			t.Error("expected CheckThrashing(5) to return true (3 iterations since progress)")
		}
	})

	t.Run("returns false when threshold is zero", func(t *testing.T) {
		tracker := NewSignalTracker(0)

		// Even at very high iterations, should not trigger
		if tracker.CheckThrashing(100) {
			t.Error("expected CheckThrashing to return false when threshold is 0 (disabled)")
		}
	})

	t.Run("phase-complete also resets progress", func(t *testing.T) {
		tracker := NewSignalTracker(3)

		// Record phase-complete at iteration 5
		tracker.RecordSignal("phase-complete", 5)

		// Iteration 7 (2 iterations since phase-complete) should NOT trigger
		if tracker.CheckThrashing(7) {
			t.Error("expected CheckThrashing(7) to return false (phase-complete resets progress)")
		}
	})
}
