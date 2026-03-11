package manager

import (
	"context"
	"log"
	"os"
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
	return &Watchdog{
		manager:        m,
		StallThreshold: 5 * time.Minute,
		CheckInterval:  30 * time.Second,
	}
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
	info, err := w.manager.Status(id)
	if err != nil {
		return
	}

	// Skip terminal states
	if isTerminal(info.Status) {
		return
	}

	// 1. Detect Stalls (No activity for X minutes)
	if !info.LastActivity.IsZero() && time.Since(info.LastActivity) > w.StallThreshold {
		log.Printf("[Watchdog] [%s] STALL DETECTED: No activity for %v (PID: %d)", id, time.Since(info.LastActivity), info.CurrentPID)
		w.remediateStall(id, info)
		return
	}

	// 2. Detect Endless Loops (High iteration count without completion)
	// If iteration > 20 and no progress, it might be stuck
	if info.Iteration > 20 {
		// This is a candidate for remediation if needed
	}
}

func (w *Watchdog) remediateStall(id string, info PipelineInfo) {
	log.Printf("[Watchdog] [%s] remediating stall...", id)

	// 1. Kill stuck process if PID exists
	if info.CurrentPID > 0 {
		log.Printf("[Watchdog] [%s] killing stuck process PID %d", id, info.CurrentPID)
		proc, err := os.FindProcess(info.CurrentPID)
		if err == nil {
			_ = proc.Kill()
		}
	}

	// 2. Stop the pipeline to clean up state
	_ = w.manager.Stop(id)

	// 3. AUTO-HEAL: Automatically restart the pipeline
	log.Printf("[Watchdog] [%s] stall remediated. Triggering auto-restart...", id)
	go func() {
		// Give it a moment to fully cleanup
		time.Sleep(2 * time.Second)
		if err := w.manager.Start(context.Background(), id); err != nil {
			log.Printf("[Watchdog] [%s] auto-restart failed: %v", id, err)
		} else {
			log.Printf("[Watchdog] [%s] ✨ Successfully auto-restarted pipeline", id)
		}
	}()
}
