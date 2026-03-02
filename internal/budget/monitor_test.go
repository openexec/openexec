package budget

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewMonitor(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "nil config uses default",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid config",
			config: &Config{
				Enabled:           true,
				TotalBudgetUSD:    100,
				WarningThreshold:  0.8,
				CriticalThreshold: 0.95,
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &Config{
				Enabled:           true,
				WarningThreshold:  0.95,
				CriticalThreshold: 0.8, // warning > critical
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor, err := NewMonitor(tt.config, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewMonitor() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && monitor == nil {
				t.Error("NewMonitor() returned nil monitor without error")
			}
		})
	}
}

func TestMonitorCheck(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		DailyBudgetUSD:    25,
		SessionBudgetUSD:  10,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		BlockOnExceed:     true,
		AlertCooldown:     time.Millisecond, // Short cooldown for testing
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	ctx := context.Background()

	tests := []struct {
		name         string
		totalSpent   float64
		dailySpent   float64
		sessionSpent float64
		wantErr      bool
		wantBlocked  bool
	}{
		{
			name:         "all under budget",
			totalSpent:   50,
			dailySpent:   10,
			sessionSpent: 5,
			wantErr:      false,
			wantBlocked:  false,
		},
		{
			name:         "total exceeded",
			totalSpent:   100,
			dailySpent:   10,
			sessionSpent: 5,
			wantErr:      true,
			wantBlocked:  true,
		},
		{
			name:         "daily exceeded",
			totalSpent:   50,
			dailySpent:   25,
			sessionSpent: 5,
			wantErr:      true,
			wantBlocked:  true,
		},
		{
			name:         "session exceeded",
			totalSpent:   50,
			dailySpent:   10,
			sessionSpent: 10,
			wantErr:      true,
			wantBlocked:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			monitor.ResetCooldowns() // Reset between tests

			status, err := monitor.Check(ctx, tt.totalSpent, tt.dailySpent, tt.sessionSpent, "test-session")

			if (err != nil) != tt.wantErr {
				t.Errorf("Check() error = %v, wantErr %v", err, tt.wantErr)
			}
			if status == nil {
				t.Fatal("Check() returned nil status")
			}
			if status.IsBlocked != tt.wantBlocked {
				t.Errorf("IsBlocked = %v, want %v", status.IsBlocked, tt.wantBlocked)
			}
		})
	}
}

func TestMonitorCheckDisabled(t *testing.T) {
	cfg := &Config{
		Enabled: false,
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	ctx := context.Background()

	// Even with high spending, should not error or block when disabled
	status, err := monitor.Check(ctx, 1000, 1000, 1000, "test-session")

	if err != nil {
		t.Errorf("Check() error = %v, want nil", err)
	}
	if status.IsBlocked {
		t.Error("Check() should not block when disabled")
	}
}

func TestMonitorAlertHandlers(t *testing.T) {
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

	var receivedAlerts []*Alert
	var mu sync.Mutex

	monitor.RegisterHandler(func(ctx context.Context, alert *Alert) {
		mu.Lock()
		receivedAlerts = append(receivedAlerts, alert)
		mu.Unlock()
	})

	ctx := context.Background()

	// Trigger a warning threshold
	_, err = monitor.Check(ctx, 85, 10, 5, "test-session")
	if err != nil {
		t.Errorf("Check() error = %v", err)
	}

	// Allow time for alert processing
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	alertCount := len(receivedAlerts)
	mu.Unlock()

	if alertCount == 0 {
		t.Error("Expected at least one alert to be triggered")
	}

	// Check alert properties
	if alertCount > 0 {
		mu.Lock()
		alert := receivedAlerts[0]
		mu.Unlock()

		if alert.Type != AlertTypeTotal {
			t.Errorf("Alert.Type = %v, want %v", alert.Type, AlertTypeTotal)
		}
		if alert.Threshold != ThresholdWarning {
			t.Errorf("Alert.Threshold = %v, want %v", alert.Threshold, ThresholdWarning)
		}
	}
}

func TestMonitorAlertCooldown(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		AlertCooldown:     100 * time.Millisecond, // 100ms cooldown
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	var alertCount int
	var mu sync.Mutex

	monitor.RegisterHandler(func(ctx context.Context, alert *Alert) {
		mu.Lock()
		alertCount++
		mu.Unlock()
	})

	ctx := context.Background()

	// First check should trigger alert
	_, _ = monitor.Check(ctx, 85, 10, 5, "test-session")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	firstCount := alertCount
	mu.Unlock()

	// Second check within cooldown should NOT trigger alert
	_, _ = monitor.Check(ctx, 86, 10, 5, "test-session")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	secondCount := alertCount
	mu.Unlock()

	if secondCount != firstCount {
		t.Errorf("Alert was triggered during cooldown period, got %d alerts, expected %d", secondCount, firstCount)
	}

	// Wait for cooldown to expire
	time.Sleep(100 * time.Millisecond)

	// Third check after cooldown should trigger alert
	_, _ = monitor.Check(ctx, 87, 10, 5, "test-session")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	thirdCount := alertCount
	mu.Unlock()

	if thirdCount <= secondCount {
		t.Errorf("Alert was not triggered after cooldown expired, got %d alerts", thirdCount)
	}
}

func TestMonitorCheckSession(t *testing.T) {
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

	ctx := context.Background()

	// Under session budget
	status, err := monitor.CheckSession(ctx, 5, "test-session")
	if err != nil {
		t.Errorf("CheckSession() error = %v", err)
	}
	if status.SessionPercentUsed != 50 {
		t.Errorf("SessionPercentUsed = %v, want 50", status.SessionPercentUsed)
	}

	// Over session budget
	status, err = monitor.CheckSession(ctx, 10, "test-session-2")
	if err == nil {
		t.Error("CheckSession() expected error for exceeded budget")
	}
	if !status.IsBlocked {
		t.Error("CheckSession() expected IsBlocked=true")
	}
}

func TestMonitorUpdateConfig(t *testing.T) {
	monitor, err := NewMonitor(DefaultConfig(), nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	// Update with valid config
	newConfig := &Config{
		Enabled:           true,
		TotalBudgetUSD:    200,
		WarningThreshold:  0.7,
		CriticalThreshold: 0.9,
	}

	err = monitor.UpdateConfig(newConfig)
	if err != nil {
		t.Errorf("UpdateConfig() error = %v", err)
	}

	// Verify config was updated
	cfg := monitor.Config()
	if cfg.TotalBudgetUSD != 200 {
		t.Errorf("TotalBudgetUSD = %v, want 200", cfg.TotalBudgetUSD)
	}

	// Update with invalid config
	invalidConfig := &Config{
		Enabled:           true,
		WarningThreshold:  0.9,
		CriticalThreshold: 0.7, // warning > critical
	}

	err = monitor.UpdateConfig(invalidConfig)
	if err == nil {
		t.Error("UpdateConfig() expected error for invalid config")
	}
}

func TestAlertToProtocolAlert(t *testing.T) {
	alert := &Alert{
		ID:           "test-alert-1",
		Type:         AlertTypeTotal,
		Threshold:    ThresholdWarning,
		SessionID:    "session-123",
		CurrentSpend: 85,
		BudgetLimit:  100,
		PercentUsed:  85,
		Message:      "Total budget warning: $85.00 / $100.00 (85.0% used)",
		CreatedAt:    time.Now(),
	}

	protoAlert := alert.ToProtocolAlert()

	if protoAlert == nil {
		t.Fatal("ToProtocolAlert() returned nil")
	}

	if protoAlert.AlertID != "test-alert-1" {
		t.Errorf("AlertID = %v, want test-alert-1", protoAlert.AlertID)
	}

	if protoAlert.Threshold != 100 {
		t.Errorf("Threshold = %v, want 100", protoAlert.Threshold)
	}

	if protoAlert.CurrentValue != 85 {
		t.Errorf("CurrentValue = %v, want 85", protoAlert.CurrentValue)
	}
}

func TestMonitorResetCooldowns(t *testing.T) {
	cfg := &Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
		AlertCooldown:     time.Hour, // Long cooldown
	}

	monitor, err := NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	var alertCount int
	var mu sync.Mutex

	monitor.RegisterHandler(func(ctx context.Context, alert *Alert) {
		mu.Lock()
		alertCount++
		mu.Unlock()
	})

	ctx := context.Background()

	// First check triggers alert
	_, _ = monitor.Check(ctx, 85, 10, 5, "test-session")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	firstCount := alertCount
	mu.Unlock()

	// Reset cooldowns
	monitor.ResetCooldowns()

	// Second check should now trigger alert
	_, _ = monitor.Check(ctx, 86, 10, 5, "test-session")
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	secondCount := alertCount
	mu.Unlock()

	if secondCount <= firstCount {
		t.Errorf("Alert not triggered after ResetCooldowns(), got %d alerts", secondCount)
	}
}
