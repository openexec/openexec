package budget

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/internal/loop"
)

// LoopHook integrates the budget monitor with the agent loop event system.
// It listens to cost events and triggers budget alerts.
type LoopHook struct {
	monitor *Monitor

	// Track per-session spending
	sessionSpend map[string]float64

	// Track daily spending
	dailySpend    float64
	dailyResetDay int // day of year when daily spending was last reset

	// Track total spending since start
	totalSpend float64

	mu sync.Mutex
}

// NewLoopHook creates a new loop hook for budget monitoring.
func NewLoopHook(monitor *Monitor) *LoopHook {
	if monitor == nil {
		return &LoopHook{
			sessionSpend: make(map[string]float64),
		}
	}
	return &LoopHook{
		monitor:      monitor,
		sessionSpend: make(map[string]float64),
	}
}

// HandleEvent processes loop events and triggers budget checks.
// This should be called from the agent loop's OnEvent callback.
func (h *LoopHook) HandleEvent(ctx context.Context, event *loop.LoopEvent) error {
	if event == nil || h.monitor == nil {
		return nil
	}

	// Only process cost events
	if event.Kind != loop.EventKindCost {
		return nil
	}

	// Extract cost info from event
	if event.Cost == nil {
		return nil
	}

	h.mu.Lock()

	// Check for daily reset
	today := time.Now().UTC().YearDay()
	if h.dailyResetDay != today {
		h.dailySpend = 0
		h.dailyResetDay = today
	}

	// Update session spending
	sessionID := event.SessionID
	oldSessionSpend := h.sessionSpend[sessionID]
	newSessionSpend := event.Cost.SessionTotal

	// Calculate incremental cost
	incrementalCost := newSessionSpend - oldSessionSpend
	if incrementalCost > 0 {
		h.sessionSpend[sessionID] = newSessionSpend
		h.dailySpend += incrementalCost
		h.totalSpend += incrementalCost
	}

	totalSpend := h.totalSpend
	dailySpend := h.dailySpend
	sessionSpend := h.sessionSpend[sessionID]

	h.mu.Unlock()

	// Run budget check
	_, err := h.monitor.Check(ctx, totalSpend, dailySpend, sessionSpend, sessionID)

	return err
}

// OnCostUpdated should be called when cost is updated in the agent loop.
// This is an alternative to HandleEvent for direct integration.
func (h *LoopHook) OnCostUpdated(ctx context.Context, sessionID string, sessionCost float64) (*Status, error) {
	h.mu.Lock()

	// Check for daily reset
	today := time.Now().UTC().YearDay()
	if h.dailyResetDay != today {
		h.dailySpend = 0
		h.dailyResetDay = today
	}

	// Update session spending
	oldSessionSpend := h.sessionSpend[sessionID]
	incrementalCost := sessionCost - oldSessionSpend

	if incrementalCost > 0 {
		h.sessionSpend[sessionID] = sessionCost
		h.dailySpend += incrementalCost
		h.totalSpend += incrementalCost
	}

	totalSpend := h.totalSpend
	dailySpend := h.dailySpend

	h.mu.Unlock()

	// If no monitor, return basic status without checking
	if h.monitor == nil {
		return &Status{
			SessionSpentUSD: sessionCost,
			TotalSpentUSD:   totalSpend,
			DailySpentUSD:   dailySpend,
			CheckedAt:       time.Now().UTC(),
		}, nil
	}

	return h.monitor.Check(ctx, totalSpend, dailySpend, sessionCost, sessionID)
}

// GetSessionSpend returns the current spending for a session.
func (h *LoopHook) GetSessionSpend(sessionID string) float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sessionSpend[sessionID]
}

// GetDailySpend returns the current daily spending.
func (h *LoopHook) GetDailySpend() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Check for daily reset
	today := time.Now().UTC().YearDay()
	if h.dailyResetDay != today {
		h.dailySpend = 0
		h.dailyResetDay = today
	}

	return h.dailySpend
}

// GetTotalSpend returns the total spending since the hook was created.
func (h *LoopHook) GetTotalSpend() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.totalSpend
}

// ResetSession clears the spending tracking for a specific session.
func (h *LoopHook) ResetSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.sessionSpend, sessionID)
}

// EventHandlerFunc returns a function suitable for use as the agent loop's OnEvent callback.
func (h *LoopHook) EventHandlerFunc() func(*loop.LoopEvent) {
	return func(event *loop.LoopEvent) {
		// Use a background context for event handling
		ctx := context.Background()
		_ = h.HandleEvent(ctx, event)
	}
}

// CreateEventCallback creates an event callback that wraps an existing callback
// while also performing budget checks.
func (h *LoopHook) CreateEventCallback(existingCallback func(*loop.LoopEvent)) func(*loop.LoopEvent) {
	return func(event *loop.LoopEvent) {
		// Call existing callback first
		if existingCallback != nil {
			existingCallback(event)
		}

		// Then do budget check
		ctx := context.Background()
		_ = h.HandleEvent(ctx, event)
	}
}

// InitFromAuditLogger initializes spending totals from the audit logger.
// This should be called at startup to restore accurate budget tracking.
func (h *LoopHook) InitFromAuditLogger(ctx context.Context, logger audit.Logger) error {
	if logger == nil {
		return nil
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Get total spending from all time
	totalStats, err := logger.GetUsageStats(ctx, &audit.QueryFilter{})
	if err != nil {
		return fmt.Errorf("failed to get total usage stats: %w", err)
	}
	if totalStats != nil {
		h.totalSpend = totalStats.TotalCostUSD
	}

	// Get daily spending
	today := time.Now().UTC()
	todayStart := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)

	dailyStats, err := logger.GetUsageStats(ctx, &audit.QueryFilter{
		Since: todayStart,
	})
	if err != nil {
		return fmt.Errorf("failed to get daily usage stats: %w", err)
	}
	if dailyStats != nil {
		h.dailySpend = dailyStats.TotalCostUSD
	}

	h.dailyResetDay = today.YearDay()

	return nil
}
