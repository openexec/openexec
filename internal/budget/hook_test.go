package budget

import (
	"context"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/loop"
)

func TestNewLoopHook(t *testing.T) {
	// With nil monitor
	hook := NewLoopHook(nil)
	if hook == nil {
		t.Fatal("NewLoopHook(nil) returned nil")
	}
	if hook.sessionSpend == nil {
		t.Error("sessionSpend map should be initialized")
	}

	// With monitor
	cfg := DefaultConfig()
	cfg.Enabled = true
	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	hook = NewLoopHook(monitor)
	if hook == nil {
		t.Fatal("NewLoopHook() returned nil")
	}
	if hook.monitor == nil {
		t.Error("monitor should be set")
	}
}

func TestLoopHookOnCostUpdated(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		SessionBudgetUSD:  10,
		DailyBudgetUSD:    25,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		AlertCooldown:     time.Millisecond,
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	hook := NewLoopHook(monitor)
	ctx := context.Background()

	// First update
	status, err := hook.OnCostUpdated(ctx, "session-1", 5.0)
	if err != nil {
		t.Errorf("OnCostUpdated() error = %v", err)
	}
	if status == nil {
		t.Fatal("OnCostUpdated() returned nil status")
	}
	if status.SessionSpentUSD != 5.0 {
		t.Errorf("SessionSpentUSD = %v, want 5.0", status.SessionSpentUSD)
	}

	// Second update for same session
	status, err = hook.OnCostUpdated(ctx, "session-1", 8.0)
	if err != nil {
		t.Errorf("OnCostUpdated() error = %v", err)
	}

	// Check totals are correctly accumulated
	if hook.GetTotalSpend() != 8.0 {
		t.Errorf("GetTotalSpend() = %v, want 8.0", hook.GetTotalSpend())
	}
	if hook.GetSessionSpend("session-1") != 8.0 {
		t.Errorf("GetSessionSpend() = %v, want 8.0", hook.GetSessionSpend("session-1"))
	}

	// Update for different session
	_, err = hook.OnCostUpdated(ctx, "session-2", 3.0)
	if err != nil {
		t.Errorf("OnCostUpdated() error = %v", err)
	}

	// Total should include both sessions
	if hook.GetTotalSpend() != 11.0 {
		t.Errorf("GetTotalSpend() = %v, want 11.0", hook.GetTotalSpend())
	}
	if hook.GetDailySpend() != 11.0 {
		t.Errorf("GetDailySpend() = %v, want 11.0", hook.GetDailySpend())
	}
}

func TestLoopHookOnCostUpdatedExceeded(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		SessionBudgetUSD:  10,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		BlockOnExceed:     true,
		AlertCooldown:     time.Millisecond,
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	hook := NewLoopHook(monitor)
	ctx := context.Background()

	// Exceed session budget
	status, err := hook.OnCostUpdated(ctx, "session-1", 10.0)
	if err == nil {
		t.Error("OnCostUpdated() should return error when budget exceeded")
	}
	if !status.IsBlocked {
		t.Error("Status should be blocked when budget exceeded")
	}
}

func TestLoopHookOnCostUpdatedWithNilMonitor(t *testing.T) {
	hook := NewLoopHook(nil)
	ctx := context.Background()

	status, err := hook.OnCostUpdated(ctx, "session-1", 5.0)
	if err != nil {
		t.Errorf("OnCostUpdated() error = %v", err)
	}
	if status == nil {
		t.Fatal("OnCostUpdated() returned nil status")
	}
	if status.SessionSpentUSD != 5.0 {
		t.Errorf("SessionSpentUSD = %v, want 5.0", status.SessionSpentUSD)
	}
}

func TestLoopHookHandleEvent(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		AlertCooldown:     time.Millisecond,
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	hook := NewLoopHook(monitor)
	ctx := context.Background()

	// Create a cost event
	event := &loop.LoopEvent{
		Type:      loop.CostUpdated,
		Kind:      loop.EventKindCost,
		SessionID: "session-1",
		Cost: &loop.CostInfo{
			SessionTotal: 50.0,
		},
	}

	err = hook.HandleEvent(ctx, event)
	if err != nil {
		t.Errorf("HandleEvent() error = %v", err)
	}

	// Check that spending was tracked
	if hook.GetSessionSpend("session-1") != 50.0 {
		t.Errorf("GetSessionSpend() = %v, want 50.0", hook.GetSessionSpend("session-1"))
	}
}

func TestLoopHookHandleEventNonCost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	monitor, _ := NewMonitor(cfg, nil)
	hook := NewLoopHook(monitor)
	ctx := context.Background()

	// Non-cost events should be ignored
	event := &loop.LoopEvent{
		Type:      loop.IterationStart,
		Kind:      loop.EventKindIteration,
		SessionID: "session-1",
	}

	err := hook.HandleEvent(ctx, event)
	if err != nil {
		t.Errorf("HandleEvent() error = %v for non-cost event", err)
	}

	// No spending should be tracked
	if hook.GetSessionSpend("session-1") != 0 {
		t.Errorf("GetSessionSpend() = %v, want 0", hook.GetSessionSpend("session-1"))
	}
}

func TestLoopHookResetSession(t *testing.T) {
	hook := NewLoopHook(nil)
	ctx := context.Background()

	// Add some spending
	_, _ = hook.OnCostUpdated(ctx, "session-1", 5.0)
	_, _ = hook.OnCostUpdated(ctx, "session-2", 3.0)

	// Verify spending is tracked
	if hook.GetSessionSpend("session-1") != 5.0 {
		t.Errorf("GetSessionSpend() = %v, want 5.0", hook.GetSessionSpend("session-1"))
	}

	// Reset session-1
	hook.ResetSession("session-1")

	// session-1 should be reset, session-2 unchanged
	if hook.GetSessionSpend("session-1") != 0 {
		t.Errorf("GetSessionSpend() = %v, want 0 after reset", hook.GetSessionSpend("session-1"))
	}
	if hook.GetSessionSpend("session-2") != 3.0 {
		t.Errorf("GetSessionSpend() = %v, want 3.0", hook.GetSessionSpend("session-2"))
	}
}

func TestLoopHookEventHandlerFunc(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		AlertCooldown:     time.Millisecond,
	}

	monitor, _ := NewMonitor(cfg, nil)
	hook := NewLoopHook(monitor)

	// Get the event handler function
	handlerFunc := hook.EventHandlerFunc()
	if handlerFunc == nil {
		t.Fatal("EventHandlerFunc() returned nil")
	}

	// Use it with an event
	event := &loop.LoopEvent{
		Type:      loop.CostUpdated,
		Kind:      loop.EventKindCost,
		SessionID: "session-1",
		Cost: &loop.CostInfo{
			SessionTotal: 25.0,
		},
	}

	// Should not panic
	handlerFunc(event)

	// Check spending was tracked
	if hook.GetSessionSpend("session-1") != 25.0 {
		t.Errorf("GetSessionSpend() = %v, want 25.0", hook.GetSessionSpend("session-1"))
	}
}

func TestLoopHookCreateEventCallback(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		AlertCooldown:     time.Millisecond,
	}

	monitor, _ := NewMonitor(cfg, nil)
	hook := NewLoopHook(monitor)

	var existingCallbackCalled bool
	existingCallback := func(e *loop.LoopEvent) {
		existingCallbackCalled = true
	}

	// Create wrapped callback
	wrappedCallback := hook.CreateEventCallback(existingCallback)
	if wrappedCallback == nil {
		t.Fatal("CreateEventCallback() returned nil")
	}

	// Use it with an event
	event := &loop.LoopEvent{
		Type:      loop.CostUpdated,
		Kind:      loop.EventKindCost,
		SessionID: "session-1",
		Cost: &loop.CostInfo{
			SessionTotal: 30.0,
		},
	}

	wrappedCallback(event)

	// Both existing callback and budget tracking should have run
	if !existingCallbackCalled {
		t.Error("Existing callback was not called")
	}
	if hook.GetSessionSpend("session-1") != 30.0 {
		t.Errorf("GetSessionSpend() = %v, want 30.0", hook.GetSessionSpend("session-1"))
	}
}

func TestLoopHookCreateEventCallbackNilExisting(t *testing.T) {
	hook := NewLoopHook(nil)

	// Create wrapped callback with nil existing
	wrappedCallback := hook.CreateEventCallback(nil)
	if wrappedCallback == nil {
		t.Fatal("CreateEventCallback() returned nil")
	}

	// Should not panic
	event := &loop.LoopEvent{
		Type:      loop.CostUpdated,
		Kind:      loop.EventKindCost,
		SessionID: "session-1",
		Cost: &loop.CostInfo{
			SessionTotal: 10.0,
		},
	}

	wrappedCallback(event)
}
