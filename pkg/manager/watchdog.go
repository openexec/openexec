package manager

import (
	"context"
	"log"
	"time"
)

// Watchdog monitors active pipelines for stalls and failures.
type Watchdog struct {
	manager *Manager

	// StallThreshold is the duration after which a pipeline is considered stalled.
	StallThreshold time.Duration

	// CheckInterval is how often the watchdog scans pipelines.
	CheckInterval time.Duration
}

// NewWatchdog creates a new Watchdog for the given manager.
func NewWatchdog(m *Manager) *Watchdog {
	w := &Watchdog{
		manager:        m,
		StallThreshold: 5 * time.Minute,
		CheckInterval:  30 * time.Second,
	}
	// Set before Run's goroutine starts. Writing these afterwards raced the
	// goroutine that reads them on every tick.
	if m.cfg.WatchdogStallThreshold > 0 {
		w.StallThreshold = m.cfg.WatchdogStallThreshold
	}
	if m.cfg.WatchdogCheckInterval > 0 {
		w.CheckInterval = m.cfg.WatchdogCheckInterval
	}
	return w
}

// Run starts the watchdog monitoring loop.
func (w *Watchdog) Run(ctx context.Context) {
	log.Printf("[Watchdog] Started monitoring with stall threshold %v", w.StallThreshold)

	ticker := time.NewTicker(w.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.checkPipelines()
		}
	}
}

func (w *Watchdog) checkPipelines() {
	w.manager.mu.RLock()
	pipelines := make([]string, 0, len(w.manager.pipelines))
	for id := range w.manager.pipelines {
		pipelines = append(pipelines, id)
	}
	w.manager.mu.RUnlock()

	for _, id := range pipelines {
		w.checkPipeline(id)
	}
}

func (w *Watchdog) checkPipeline(id string) {
	w.checkPipelineWithClock(id, time.Now)
}

// Keep observation and the locked eligibility recheck on the same clock.
// Production uses wall time; native activity probes can sample exact boundaries
// without mistaking subprocess startup or host scheduling delays for silence.
func (w *Watchdog) checkPipelineWithClock(id string, now func() time.Time) {
	defer func() {
		if r := recover(); r != nil {
			// Pipeline may have been cleaned up during check — ignore
		}
	}()

	info, err := w.manager.Status(id)
	if err != nil {
		return
	}

	// Skip terminal states
	if !info.SilenceMonitoring || info.PendingChecks > 0 || isTerminal(info.Status) || info.Status == StatusPaused || info.StageType == "deterministic" {
		return
	}

	// 1. Detect Stalls (No activity for X minutes)
	if !info.LastActivity.IsZero() && now().Sub(info.LastActivity) > w.StallThreshold {
		log.Printf("[Watchdog] [%s] STALL DETECTED: No activity for %v (PID: %d)", id, now().Sub(info.LastActivity), info.CurrentPID)
		w.remediateStallWithClock(id, info, now)
		return
	}

	// 2. Detect Endless Loops (High iteration count without completion)
	// If iteration > 20 and no progress, it might be stuck
	if info.Iteration > 20 {
		// This is a candidate for remediation if needed
	}
}

func (w *Watchdog) remediateStall(id string, info PipelineInfo) {
	w.remediateStallWithClock(id, info, time.Now)
}

func (w *Watchdog) remediateStallWithClock(id string, info PipelineInfo, now func() time.Time) {
	log.Printf("[Watchdog] [%s] remediating stall...", id)

	// Cancel through the native attempt context. The task queue owns resumption;
	// a detached restart could override an explicit Stop or the caller's authority.
	_ = w.manager.stopWithCauseWithClock(id, ErrWatchdogStall, w.StallThreshold, now)
}
