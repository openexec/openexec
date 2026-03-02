package budget

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/internal/protocol"
)

// AlertHandler is a callback function invoked when a budget alert is triggered.
type AlertHandler func(ctx context.Context, alert *Alert)

// Alert represents a budget alert event.
type Alert struct {
	// ID is the unique identifier for this alert.
	ID string `json:"id"`

	// Type indicates what budget limit triggered the alert.
	Type AlertType `json:"type"`

	// Threshold indicates which threshold was crossed.
	Threshold AlertThreshold `json:"threshold"`

	// SessionID is the session that triggered the alert (if applicable).
	SessionID string `json:"session_id,omitempty"`

	// CurrentSpend is the current spending amount.
	CurrentSpend float64 `json:"current_spend"`

	// BudgetLimit is the configured budget limit.
	BudgetLimit float64 `json:"budget_limit"`

	// PercentUsed is the percentage of budget consumed.
	PercentUsed float64 `json:"percent_used"`

	// Message is a human-readable description of the alert.
	Message string `json:"message"`

	// CreatedAt is when the alert was generated.
	CreatedAt time.Time `json:"created_at"`
}

// AlertType indicates the category of budget that triggered an alert.
type AlertType string

const (
	// AlertTypeTotal indicates the total budget triggered the alert.
	AlertTypeTotal AlertType = "total"

	// AlertTypeDaily indicates the daily budget triggered the alert.
	AlertTypeDaily AlertType = "daily"

	// AlertTypeSession indicates the session budget triggered the alert.
	AlertTypeSession AlertType = "session"
)

// ToProtocolAlert converts a budget Alert to a protocol.AlertEvent.
func (a *Alert) ToProtocolAlert() *protocol.AlertEvent {
	var severity protocol.AlertSeverity
	switch a.Threshold {
	case ThresholdWarning:
		severity = protocol.AlertSeverityWarning
	case ThresholdCritical, ThresholdExceeded:
		severity = protocol.AlertSeverityCritical
	default:
		severity = protocol.AlertSeverityInfo
	}

	event := protocol.NewAlertEvent(
		a.ID,
		protocol.AlertTypeResourceLimit,
		severity,
		a.Message,
	)
	event.TaskID = a.SessionID
	event.Threshold = a.BudgetLimit
	event.CurrentValue = a.CurrentSpend
	event.Metadata = map[string]interface{}{
		"alert_type":   string(a.Type),
		"threshold":    string(a.Threshold),
		"percent_used": a.PercentUsed,
	}
	return event
}

// Monitor tracks spending and triggers alerts when budget thresholds are crossed.
type Monitor struct {
	config *Config
	logger audit.Logger

	// handlers stores registered alert handlers
	handlers []AlertHandler

	// lastAlert tracks the last alert time per threshold to implement cooldown
	lastAlert map[string]time.Time

	mu sync.RWMutex
}

// NewMonitor creates a new budget monitor with the given configuration.
func NewMonitor(config *Config, logger audit.Logger) (*Monitor, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid budget config: %w", err)
	}

	return &Monitor{
		config:    config,
		logger:    logger,
		handlers:  make([]AlertHandler, 0),
		lastAlert: make(map[string]time.Time),
	}, nil
}

// Config returns the current budget configuration.
func (m *Monitor) Config() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.config
}

// UpdateConfig updates the budget configuration.
func (m *Monitor) UpdateConfig(config *Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("invalid budget config: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.config = config
	return nil
}

// RegisterHandler adds an alert handler to be called when alerts are triggered.
func (m *Monitor) RegisterHandler(handler AlertHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
}

// Check evaluates current spending against budget limits and triggers alerts.
// Returns the current budget status and any blocking error.
func (m *Monitor) Check(ctx context.Context, totalSpent, dailySpent, sessionSpent float64, sessionID string) (*Status, error) {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	if !config.Enabled {
		return &Status{
			TotalSpentUSD:   totalSpent,
			DailySpentUSD:   dailySpent,
			SessionSpentUSD: sessionSpent,
			CheckedAt:       time.Now().UTC(),
		}, nil
	}

	status := NewStatus(config, totalSpent, dailySpent, sessionSpent)

	// Check and alert for each budget type
	m.checkAndAlert(ctx, AlertTypeTotal, status.TotalThreshold, totalSpent, config.TotalBudgetUSD, status.TotalPercentUsed, sessionID)
	m.checkAndAlert(ctx, AlertTypeDaily, status.DailyThreshold, dailySpent, config.DailyBudgetUSD, status.DailyPercentUsed, sessionID)
	m.checkAndAlert(ctx, AlertTypeSession, status.SessionThreshold, sessionSpent, config.SessionBudgetUSD, status.SessionPercentUsed, sessionID)

	// Return error if blocked
	if status.IsBlocked {
		return status, ErrBudgetExceeded
	}

	return status, nil
}

// CheckSession checks only the session budget for a specific session.
func (m *Monitor) CheckSession(ctx context.Context, sessionSpent float64, sessionID string) (*Status, error) {
	m.mu.RLock()
	config := m.config
	m.mu.RUnlock()

	if !config.Enabled || config.SessionBudgetUSD <= 0 {
		return &Status{
			SessionSpentUSD:  sessionSpent,
			SessionBudgetUSD: config.SessionBudgetUSD,
			CheckedAt:        time.Now().UTC(),
		}, nil
	}

	threshold := config.CheckThreshold(sessionSpent, config.SessionBudgetUSD)
	percentUsed := (sessionSpent / config.SessionBudgetUSD) * 100

	status := &Status{
		SessionSpentUSD:    sessionSpent,
		SessionBudgetUSD:   config.SessionBudgetUSD,
		SessionPercentUsed: percentUsed,
		SessionThreshold:   threshold,
		CheckedAt:          time.Now().UTC(),
	}

	if config.BlockOnExceed && threshold == ThresholdExceeded {
		status.IsBlocked = true
		status.BlockReason = "Session budget limit exceeded"
	}

	m.checkAndAlert(ctx, AlertTypeSession, threshold, sessionSpent, config.SessionBudgetUSD, percentUsed, sessionID)

	if status.IsBlocked {
		return status, ErrBudgetExceeded
	}

	return status, nil
}

// checkAndAlert generates and dispatches an alert if threshold is crossed and cooldown has passed.
func (m *Monitor) checkAndAlert(ctx context.Context, alertType AlertType, threshold AlertThreshold, currentSpend, budgetLimit, percentUsed float64, sessionID string) {
	if threshold == ThresholdNone {
		return
	}

	// Generate unique key for this alert type + threshold combination
	alertKey := fmt.Sprintf("%s:%s:%s", alertType, threshold, sessionID)

	m.mu.Lock()
	lastTime, exists := m.lastAlert[alertKey]
	config := m.config

	// Check cooldown
	if exists && time.Since(lastTime) < config.AlertCooldown {
		m.mu.Unlock()
		return
	}

	// Update last alert time
	m.lastAlert[alertKey] = time.Now()
	handlers := make([]AlertHandler, len(m.handlers))
	copy(handlers, m.handlers)
	m.mu.Unlock()

	// Create alert
	alert := &Alert{
		ID:           fmt.Sprintf("budget_%s_%d", alertType, time.Now().UnixNano()),
		Type:         alertType,
		Threshold:    threshold,
		SessionID:    sessionID,
		CurrentSpend: currentSpend,
		BudgetLimit:  budgetLimit,
		PercentUsed:  percentUsed,
		Message:      m.formatAlertMessage(alertType, threshold, currentSpend, budgetLimit, percentUsed),
		CreatedAt:    time.Now().UTC(),
	}

	// Log to audit if logger is available
	if m.logger != nil {
		m.logBudgetAlert(ctx, alert)
	}

	// Call registered handlers
	for _, handler := range handlers {
		handler(ctx, alert)
	}
}

// formatAlertMessage creates a human-readable alert message.
func (m *Monitor) formatAlertMessage(alertType AlertType, threshold AlertThreshold, currentSpend, budgetLimit, percentUsed float64) string {
	var budgetName string
	switch alertType {
	case AlertTypeTotal:
		budgetName = "Total budget"
	case AlertTypeDaily:
		budgetName = "Daily budget"
	case AlertTypeSession:
		budgetName = "Session budget"
	default:
		budgetName = "Budget"
	}

	var severity string
	switch threshold {
	case ThresholdWarning:
		severity = "warning"
	case ThresholdCritical:
		severity = "critical"
	case ThresholdExceeded:
		severity = "exceeded"
	default:
		severity = "threshold"
	}

	return fmt.Sprintf("%s %s: $%.2f / $%.2f (%.1f%% used)",
		budgetName, severity, currentSpend, budgetLimit, percentUsed)
}

// logBudgetAlert logs an audit entry for the budget alert.
func (m *Monitor) logBudgetAlert(ctx context.Context, alert *Alert) {
	var eventType audit.EventType
	var severity audit.Severity

	switch alert.Threshold {
	case ThresholdWarning:
		eventType = audit.EventBudgetWarning
		severity = audit.SeverityWarning
	case ThresholdCritical, ThresholdExceeded:
		eventType = audit.EventBudgetExceeded
		severity = audit.SeverityCritical
	default:
		eventType = audit.EventBudgetWarning
		severity = audit.SeverityInfo
	}

	builder, err := audit.NewEntry(eventType, "system", "budget_monitor")
	if err != nil {
		return
	}

	entry, err := builder.
		WithSeverity(severity).
		WithSession(alert.SessionID).
		WithCost(alert.CurrentSpend).
		WithMetadata(map[string]interface{}{
			"budget_type":   string(alert.Type),
			"threshold":     string(alert.Threshold),
			"budget_limit":  alert.BudgetLimit,
			"percent_used":  alert.PercentUsed,
			"alert_message": alert.Message,
		}).
		Build()

	if err != nil {
		return
	}

	_ = m.logger.Log(ctx, entry)
}

// GetStatus returns the current budget status.
func (m *Monitor) GetStatus(ctx context.Context) (*Status, error) {
	m.mu.RLock()
	config := m.config
	logger := m.logger
	m.mu.RUnlock()

	if !config.Enabled {
		return &Status{
			TotalBudgetUSD:   config.TotalBudgetUSD,
			DailyBudgetUSD:   config.DailyBudgetUSD,
			SessionBudgetUSD: config.SessionBudgetUSD,
			CheckedAt:        time.Now().UTC(),
		}, nil
	}

	// Get usage stats from audit logger
	var totalSpent, dailySpent float64

	if logger != nil {
		// Total spending
		totalStats, err := logger.GetUsageStats(ctx, &audit.QueryFilter{})
		if err == nil && totalStats != nil {
			totalSpent = totalStats.TotalCostUSD
		}

		// Daily spending
		todayStart := time.Now().UTC().Truncate(24 * time.Hour)
		dailyStats, err := logger.GetUsageStats(ctx, &audit.QueryFilter{
			Since: todayStart,
		})
		if err == nil && dailyStats != nil {
			dailySpent = dailyStats.TotalCostUSD
		}
	}

	return NewStatus(config, totalSpent, dailySpent, 0), nil
}

// ResetCooldowns clears all alert cooldown timers.
func (m *Monitor) ResetCooldowns() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAlert = make(map[string]time.Time)
}
