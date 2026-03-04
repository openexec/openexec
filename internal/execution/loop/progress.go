package loop

import "sync"

// SignalTracker accumulates agent signals during loop execution.
// Thread-safe — called from the parser goroutine, read from the loop goroutine.
type SignalTracker struct {
	mu               sync.Mutex
	phaseComplete    bool
	lastProgressIter int
	thrashThreshold  int
}

// NewSignalTracker creates a tracker. thrashThreshold is the number of iterations
// without a progress/phase-complete signal before CheckThrashing returns true.
// 0 disables thrashing detection.
func NewSignalTracker(thrashThreshold int) *SignalTracker {
	return &SignalTracker{
		thrashThreshold: thrashThreshold,
	}
}

// RecordSignal records a signal from the agent. Called by the parser goroutine.
func (t *SignalTracker) RecordSignal(signalType string, iteration int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch signalType {
	case "phase-complete", "route":
		t.phaseComplete = true
		t.lastProgressIter = iteration
	case "progress":
		t.lastProgressIter = iteration
	}
}

// PhaseComplete returns true if a phase-complete signal was received.
func (t *SignalTracker) PhaseComplete() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.phaseComplete
}

// CheckThrashing returns true if the thrash threshold has been exceeded:
// more than thrashThreshold iterations have passed since the last progress
// or phase-complete signal. Returns false if thrashing detection is disabled (threshold 0).
func (t *SignalTracker) CheckThrashing(iteration int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.thrashThreshold <= 0 {
		return false
	}
	return iteration-t.lastProgressIter >= t.thrashThreshold
}

// Reset clears the phase-complete flag for the next iteration.
// Does NOT reset progress tracking (thrashing detection spans iterations).
func (t *SignalTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.phaseComplete = false
}
