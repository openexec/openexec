// Package loop provides execution loop with stall detection and recovery.
// This file implements stall heuristics with provider timeouts and exponential backoff.
package loop

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// StallState represents the current stall detection state.
type StallState string

const (
	StateNormal      StallState = "normal"
	StateWarning     StallState = "warning"
	StateStalled     StallState = "stalled"
	StateRecovering  StallState = "recovering"
)

// StallConfig configures stall detection behavior.
type StallConfig struct {
	// NoOutputTimeout is how long to wait before considering the provider stalled.
	// Default: 60 seconds.
	NoOutputTimeout time.Duration

	// WarningThreshold is the time after which a warning is emitted.
	// Default: 30 seconds.
	WarningThreshold time.Duration

	// MaxStallAttempts is how many stall recoveries to attempt before failing.
	// Default: 3.
	MaxStallAttempts int

	// BaseBackoff is the initial backoff duration after a stall.
	// Default: 1 second.
	BaseBackoff time.Duration

	// MaxBackoff is the maximum backoff duration.
	// Default: 30 seconds.
	MaxBackoff time.Duration

	// BackoffMultiplier is the exponential backoff multiplier.
	// Default: 2.0.
	BackoffMultiplier float64

	// ProviderTimeout is the hard timeout for a single provider request.
	// Default: 5 minutes.
	ProviderTimeout time.Duration

	// IdleActivityThreshold is how often activity must be observed to consider not idle.
	// Default: 5 seconds.
	IdleActivityThreshold time.Duration
}

// DefaultStallConfig returns sensible default configuration.
func DefaultStallConfig() StallConfig {
	return StallConfig{
		NoOutputTimeout:       60 * time.Second,
		WarningThreshold:      30 * time.Second,
		MaxStallAttempts:      3,
		BaseBackoff:           1 * time.Second,
		MaxBackoff:            30 * time.Second,
		BackoffMultiplier:     2.0,
		ProviderTimeout:       5 * time.Minute,
		IdleActivityThreshold: 5 * time.Second,
	}
}

// StallDetector monitors execution for stalls and manages recovery.
type StallDetector struct {
	config      StallConfig
	mu          sync.RWMutex
	lastActivity time.Time
	state       StallState
	stallCount  int
	warningEmitted bool

	// Callbacks
	onWarning   func(duration time.Duration)
	onStall     func(count int, duration time.Duration)
	onRecovery  func(count int)
	onFatalStall func(count int)

	// Tracking
	operationStart time.Time
	operationCount atomic.Int64
}

// NewStallDetector creates a new stall detector with the given config.
func NewStallDetector(config StallConfig) *StallDetector {
	now := time.Now()
	return &StallDetector{
		config:       config,
		lastActivity: now,
		state:        StateNormal,
	}
}

// SetCallbacks configures event callbacks.
func (d *StallDetector) SetCallbacks(
	onWarning func(duration time.Duration),
	onStall func(count int, duration time.Duration),
	onRecovery func(count int),
	onFatalStall func(count int),
) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onWarning = onWarning
	d.onStall = onStall
	d.onRecovery = onRecovery
	d.onFatalStall = onFatalStall
}

// RecordActivity marks that activity was observed.
func (d *StallDetector) RecordActivity() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	d.lastActivity = now

	// If we were recovering, transition back to normal
	if d.state == StateRecovering || d.state == StateWarning {
		d.state = StateNormal
		d.warningEmitted = false
		if d.onRecovery != nil && d.stallCount > 0 {
			d.onRecovery(d.stallCount)
		}
	}
}

// StartOperation marks the beginning of a timed operation.
func (d *StallDetector) StartOperation() context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.operationStart = time.Now()
	d.operationCount.Add(1)

	// Create a context with the provider timeout
	ctx, _ := context.WithTimeout(context.Background(), d.config.ProviderTimeout)
	return ctx
}

// Check evaluates the current stall state and triggers callbacks.
// Returns the current state and whether action is needed.
func (d *StallDetector) Check() (StallState, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	idleDuration := time.Since(d.lastActivity)

	// Check for warning threshold
	if idleDuration >= d.config.WarningThreshold && !d.warningEmitted {
		d.state = StateWarning
		d.warningEmitted = true
		if d.onWarning != nil {
			d.onWarning(idleDuration)
		}
	}

	// Check for stall threshold
	if idleDuration >= d.config.NoOutputTimeout {
		d.state = StateStalled
		d.stallCount++

		if d.stallCount > d.config.MaxStallAttempts {
			if d.onFatalStall != nil {
				d.onFatalStall(d.stallCount)
			}
			return StateStalled, true
		}

		if d.onStall != nil {
			d.onStall(d.stallCount, idleDuration)
		}

		return StateStalled, true
	}

	return d.state, false
}

// GetBackoffDuration returns the current backoff duration based on stall count.
func (d *StallDetector) GetBackoffDuration() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.stallCount == 0 {
		return 0
	}

	backoff := float64(d.config.BaseBackoff) * math.Pow(d.config.BackoffMultiplier, float64(d.stallCount-1))
	if backoff > float64(d.config.MaxBackoff) {
		backoff = float64(d.config.MaxBackoff)
	}

	return time.Duration(backoff)
}

// Reset resets the stall detector state.
func (d *StallDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.state = StateNormal
	d.stallCount = 0
	d.warningEmitted = false
	d.lastActivity = time.Now()
}

// State returns the current stall state.
func (d *StallDetector) State() StallState {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// StallCount returns the number of stalls detected.
func (d *StallDetector) StallCount() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.stallCount
}

// IdleDuration returns how long since the last activity.
func (d *StallDetector) IdleDuration() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return time.Since(d.lastActivity)
}

// StallError is returned when a fatal stall is detected.
type StallError struct {
	StallCount int
	Duration   time.Duration
	Message    string
}

func (e *StallError) Error() string {
	return fmt.Sprintf("fatal stall detected: %s (stalls: %d, idle: %v)",
		e.Message, e.StallCount, e.Duration)
}

// ProviderStallWatcher wraps a stall detector for monitoring provider calls.
type ProviderStallWatcher struct {
	detector *StallDetector
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewProviderStallWatcher creates a watcher for provider stalls.
func NewProviderStallWatcher(config StallConfig) *ProviderStallWatcher {
	return &ProviderStallWatcher{
		detector: NewStallDetector(config),
		done:     make(chan struct{}),
	}
}

// Start begins monitoring for stalls in the background.
func (w *ProviderStallWatcher) Start(ctx context.Context, eventChan chan<- Event, iteration int) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.done = make(chan struct{})

	watchCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.mu.Unlock()

	// Set up callbacks to emit events
	w.detector.SetCallbacks(
		// On warning
		func(duration time.Duration) {
			select {
			case eventChan <- Event{
				Type:      EventProgress,
				Iteration: iteration,
				Text:      fmt.Sprintf("STALL_WARNING: No output for %v", duration),
			}:
			default:
			}
		},
		// On stall
		func(count int, duration time.Duration) {
			select {
			case eventChan <- Event{
				Type:      EventProgress,
				Iteration: iteration,
				Text:      fmt.Sprintf("STALL_DETECTED: No output for %v (attempt %d)", duration, count),
			}:
			default:
			}
		},
		// On recovery
		func(count int) {
			select {
			case eventChan <- Event{
				Type:      EventProgress,
				Iteration: iteration,
				Text:      fmt.Sprintf("STALL_RECOVERED: Resumed after %d stalls", count),
			}:
			default:
			}
		},
		// On fatal stall
		func(count int) {
			select {
			case eventChan <- Event{
				Type:      EventError,
				Iteration: iteration,
				Text:      fmt.Sprintf("FATAL_STALL: Max stall attempts (%d) exceeded", count),
			}:
			default:
			}
		},
	)

	// Background monitor
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-watchCtx.Done():
				close(w.done)
				return
			case <-ticker.C:
				state, actionNeeded := w.detector.Check()
				if actionNeeded && state == StateStalled {
					// Could trigger retry logic here
					backoff := w.detector.GetBackoffDuration()
					if backoff > 0 {
						time.Sleep(backoff)
					}
				}
			}
		}
	}()
}

// RecordActivity signals that activity was observed.
func (w *ProviderStallWatcher) RecordActivity() {
	w.detector.RecordActivity()
}

// Stop stops the stall watcher.
func (w *ProviderStallWatcher) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return
	}

	if w.cancel != nil {
		w.cancel()
	}
	<-w.done
	w.running = false
}

// GetState returns the current stall state.
func (w *ProviderStallWatcher) GetState() StallState {
	return w.detector.State()
}

// Reset resets the watcher state.
func (w *ProviderStallWatcher) Reset() {
	w.detector.Reset()
}
