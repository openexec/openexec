package loop

import "testing"

func TestTrackerPhaseComplete(t *testing.T) {
	tracker := NewSignalTracker(3)

	if tracker.PhaseComplete() {
		t.Error("PhaseComplete should be false initially")
	}

	tracker.RecordSignal("phase-complete", 1)

	if !tracker.PhaseComplete() {
		t.Error("PhaseComplete should be true after phase-complete signal")
	}
}

func TestTrackerProgressResetsThrasching(t *testing.T) {
	tracker := NewSignalTracker(3)

	// Simulate 2 iterations without progress.
	if tracker.CheckThrashing(1) {
		t.Error("should not thrash at iteration 1 with threshold 3")
	}
	if tracker.CheckThrashing(2) {
		t.Error("should not thrash at iteration 2 with threshold 3")
	}

	// Record progress at iteration 2.
	tracker.RecordSignal("progress", 2)

	// 2 more iterations without progress (3 and 4), threshold is 3 from iter 2.
	if tracker.CheckThrashing(3) {
		t.Error("should not thrash at iteration 3 (1 since progress)")
	}
	if tracker.CheckThrashing(4) {
		t.Error("should not thrash at iteration 4 (2 since progress)")
	}

	// Iteration 5 — 3 iterations since progress at iter 2.
	if !tracker.CheckThrashing(5) {
		t.Error("should thrash at iteration 5 (3 since progress at 2)")
	}
}

func TestTrackerThrashingAtThreshold(t *testing.T) {
	tracker := NewSignalTracker(3)

	// No signals at all. lastProgressIter = 0.
	if tracker.CheckThrashing(1) {
		t.Error("iteration 1: should not thrash")
	}
	if tracker.CheckThrashing(2) {
		t.Error("iteration 2: should not thrash")
	}
	if !tracker.CheckThrashing(3) {
		t.Error("iteration 3: should thrash (3 iterations with no progress)")
	}
}

func TestTrackerThrashingDisabled(t *testing.T) {
	tracker := NewSignalTracker(0)

	// Even after many iterations, should never report thrashing.
	for i := 1; i <= 100; i++ {
		if tracker.CheckThrashing(i) {
			t.Fatalf("should never thrash with threshold 0, failed at iteration %d", i)
		}
	}
}

func TestTrackerReset(t *testing.T) {
	tracker := NewSignalTracker(3)

	tracker.RecordSignal("phase-complete", 1)
	if !tracker.PhaseComplete() {
		t.Fatal("expected phase-complete true")
	}

	tracker.Reset()
	if tracker.PhaseComplete() {
		t.Error("PhaseComplete should be false after Reset")
	}

	// Progress tracking should be preserved (lastProgressIter still 1).
	if tracker.CheckThrashing(2) {
		t.Error("should not thrash at iteration 2 (progress was at 1)")
	}
}

func TestTrackerRouteSignalSetsPhaseComplete(t *testing.T) {
	tracker := NewSignalTracker(3)

	if tracker.PhaseComplete() {
		t.Error("PhaseComplete should be false initially")
	}

	tracker.RecordSignal("route", 1)

	if !tracker.PhaseComplete() {
		t.Error("PhaseComplete should be true after route signal")
	}

	// Route should also update progress tracking.
	if tracker.CheckThrashing(2) {
		t.Error("should not thrash at iteration 2 (route at 1)")
	}
}

func TestTrackerPhaseCompleteAlsoResetsProgress(t *testing.T) {
	tracker := NewSignalTracker(3)

	// Simulate phase-complete at iteration 5.
	tracker.RecordSignal("phase-complete", 5)

	// Check that progress tracking was updated.
	if tracker.CheckThrashing(6) {
		t.Error("should not thrash at 6 (phase-complete at 5)")
	}
	if tracker.CheckThrashing(7) {
		t.Error("should not thrash at 7 (phase-complete at 5)")
	}
	if !tracker.CheckThrashing(8) {
		t.Error("should thrash at 8 (3 since phase-complete at 5)")
	}
}
