package loop

import (
	"testing"
)

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
