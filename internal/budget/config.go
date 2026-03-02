// Package budget provides budget monitoring and alerting functionality.
package budget

import (
	"errors"
	"time"
)

// Errors specific to budget configuration.
var (
	ErrBudgetNotConfigured = errors.New("budget is not configured")
	ErrInvalidBudgetConfig = errors.New("invalid budget configuration")
	ErrBudgetExceeded      = errors.New("budget limit exceeded")
)

// AlertThreshold represents a threshold level for budget alerts.
type AlertThreshold string

const (
	// ThresholdNone indicates no threshold has been crossed.
	ThresholdNone AlertThreshold = "none"

	// ThresholdWarning indicates the warning threshold has been crossed.
	ThresholdWarning AlertThreshold = "warning"

	// ThresholdCritical indicates the critical threshold has been crossed.
	ThresholdCritical AlertThreshold = "critical"

	// ThresholdExceeded indicates the budget has been exceeded.
	ThresholdExceeded AlertThreshold = "exceeded"
)

// Config defines budget limits and alert thresholds.
type Config struct {
	// Enabled controls whether budget monitoring is active.
	Enabled bool `json:"enabled" yaml:"enabled"`

	// TotalBudgetUSD is the overall spending limit in USD.
	// When this limit is reached, operations may be blocked.
	TotalBudgetUSD float64 `json:"total_budget_usd" yaml:"total_budget_usd"`

	// SessionBudgetUSD is the per-session spending limit in USD.
	// Each session will be blocked from continuing when it reaches this limit.
	SessionBudgetUSD float64 `json:"session_budget_usd" yaml:"session_budget_usd"`

	// DailyBudgetUSD is the daily spending limit in USD.
	// Spending is tracked per calendar day (UTC).
	DailyBudgetUSD float64 `json:"daily_budget_usd" yaml:"daily_budget_usd"`

	// WarningThreshold is the percentage of budget at which warnings are triggered.
	// Value should be between 0 and 1 (e.g., 0.8 = 80%).
	WarningThreshold float64 `json:"warning_threshold" yaml:"warning_threshold"`

	// CriticalThreshold is the percentage of budget at which critical alerts are triggered.
	// Value should be between 0 and 1 (e.g., 0.95 = 95%).
	CriticalThreshold float64 `json:"critical_threshold" yaml:"critical_threshold"`

	// BlockOnExceed controls whether to block operations when budget is exceeded.
	// If false, only alerts are sent but operations continue.
	BlockOnExceed bool `json:"block_on_exceed" yaml:"block_on_exceed"`

	// AlertCooldown is the minimum time between repeated alerts for the same threshold.
	AlertCooldown time.Duration `json:"alert_cooldown" yaml:"alert_cooldown"`

	// AlertChannels specifies which channels to send alerts to.
	// Supported values: "telegram", "console", "audit"
	AlertChannels []string `json:"alert_channels" yaml:"alert_channels"`
}

// DefaultConfig returns a sensible default budget configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:           false, // Disabled by default
		TotalBudgetUSD:    100.0, // $100 total budget
		SessionBudgetUSD:  10.0,  // $10 per session
		DailyBudgetUSD:    25.0,  // $25 per day
		WarningThreshold:  0.8,   // Alert at 80%
		CriticalThreshold: 0.95,  // Critical at 95%
		BlockOnExceed:     false, // Don't block by default
		AlertCooldown:     5 * time.Minute,
		AlertChannels:     []string{"console", "audit"},
	}
}

// Validate checks that the budget configuration is valid.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil // Skip validation if not enabled
	}

	// At least one budget limit must be set
	if c.TotalBudgetUSD <= 0 && c.SessionBudgetUSD <= 0 && c.DailyBudgetUSD <= 0 {
		return ErrInvalidBudgetConfig
	}

	// Threshold values must be valid percentages
	if c.WarningThreshold < 0 || c.WarningThreshold > 1 {
		return ErrInvalidBudgetConfig
	}
	if c.CriticalThreshold < 0 || c.CriticalThreshold > 1 {
		return ErrInvalidBudgetConfig
	}

	// Warning should be lower than critical
	if c.WarningThreshold >= c.CriticalThreshold {
		return ErrInvalidBudgetConfig
	}

	return nil
}

// GetWarningBudget returns the USD amount at which a warning should be triggered
// for the given budget limit.
func (c *Config) GetWarningBudget(budgetUSD float64) float64 {
	return budgetUSD * c.WarningThreshold
}

// GetCriticalBudget returns the USD amount at which a critical alert should be triggered
// for the given budget limit.
func (c *Config) GetCriticalBudget(budgetUSD float64) float64 {
	return budgetUSD * c.CriticalThreshold
}

// CheckThreshold determines which threshold level the current spending has reached.
func (c *Config) CheckThreshold(currentSpend, budgetLimit float64) AlertThreshold {
	if budgetLimit <= 0 {
		return ThresholdNone
	}

	percentage := currentSpend / budgetLimit

	if percentage >= 1.0 {
		return ThresholdExceeded
	}
	if percentage >= c.CriticalThreshold {
		return ThresholdCritical
	}
	if percentage >= c.WarningThreshold {
		return ThresholdWarning
	}

	return ThresholdNone
}

// Status represents the current budget status.
type Status struct {
	// TotalSpentUSD is the total amount spent across all sessions.
	TotalSpentUSD float64 `json:"total_spent_usd"`

	// TotalBudgetUSD is the configured total budget.
	TotalBudgetUSD float64 `json:"total_budget_usd"`

	// TotalPercentUsed is the percentage of total budget used.
	TotalPercentUsed float64 `json:"total_percent_used"`

	// TotalThreshold is the current threshold level for total budget.
	TotalThreshold AlertThreshold `json:"total_threshold"`

	// DailySpentUSD is the amount spent today (UTC).
	DailySpentUSD float64 `json:"daily_spent_usd"`

	// DailyBudgetUSD is the configured daily budget.
	DailyBudgetUSD float64 `json:"daily_budget_usd"`

	// DailyPercentUsed is the percentage of daily budget used.
	DailyPercentUsed float64 `json:"daily_percent_used"`

	// DailyThreshold is the current threshold level for daily budget.
	DailyThreshold AlertThreshold `json:"daily_threshold"`

	// SessionSpentUSD is the amount spent in the current session.
	SessionSpentUSD float64 `json:"session_spent_usd,omitempty"`

	// SessionBudgetUSD is the configured session budget.
	SessionBudgetUSD float64 `json:"session_budget_usd"`

	// SessionPercentUsed is the percentage of session budget used.
	SessionPercentUsed float64 `json:"session_percent_used,omitempty"`

	// SessionThreshold is the current threshold level for session budget.
	SessionThreshold AlertThreshold `json:"session_threshold,omitempty"`

	// IsBlocked indicates whether operations are blocked due to budget limits.
	IsBlocked bool `json:"is_blocked"`

	// BlockReason explains why operations are blocked (if IsBlocked is true).
	BlockReason string `json:"block_reason,omitempty"`

	// CheckedAt is when this status was computed.
	CheckedAt time.Time `json:"checked_at"`
}

// NewStatus creates a new budget status with calculated fields.
func NewStatus(config *Config, totalSpent, dailySpent, sessionSpent float64) *Status {
	status := &Status{
		TotalSpentUSD:    totalSpent,
		TotalBudgetUSD:   config.TotalBudgetUSD,
		DailySpentUSD:    dailySpent,
		DailyBudgetUSD:   config.DailyBudgetUSD,
		SessionSpentUSD:  sessionSpent,
		SessionBudgetUSD: config.SessionBudgetUSD,
		CheckedAt:        time.Now().UTC(),
	}

	// Calculate percentages
	if config.TotalBudgetUSD > 0 {
		status.TotalPercentUsed = (totalSpent / config.TotalBudgetUSD) * 100
		status.TotalThreshold = config.CheckThreshold(totalSpent, config.TotalBudgetUSD)
	}

	if config.DailyBudgetUSD > 0 {
		status.DailyPercentUsed = (dailySpent / config.DailyBudgetUSD) * 100
		status.DailyThreshold = config.CheckThreshold(dailySpent, config.DailyBudgetUSD)
	}

	if config.SessionBudgetUSD > 0 {
		status.SessionPercentUsed = (sessionSpent / config.SessionBudgetUSD) * 100
		status.SessionThreshold = config.CheckThreshold(sessionSpent, config.SessionBudgetUSD)
	}

	// Determine if blocked
	if config.BlockOnExceed {
		if status.TotalThreshold == ThresholdExceeded {
			status.IsBlocked = true
			status.BlockReason = "Total budget limit exceeded"
		} else if status.DailyThreshold == ThresholdExceeded {
			status.IsBlocked = true
			status.BlockReason = "Daily budget limit exceeded"
		} else if status.SessionThreshold == ThresholdExceeded {
			status.IsBlocked = true
			status.BlockReason = "Session budget limit exceeded"
		}
	}

	return status
}

// HighestThreshold returns the highest threshold level across all budget types.
func (s *Status) HighestThreshold() AlertThreshold {
	thresholds := []AlertThreshold{s.TotalThreshold, s.DailyThreshold, s.SessionThreshold}
	highest := ThresholdNone

	for _, t := range thresholds {
		if thresholdPriority(t) > thresholdPriority(highest) {
			highest = t
		}
	}

	return highest
}

// thresholdPriority returns a numeric priority for threshold comparison.
func thresholdPriority(t AlertThreshold) int {
	switch t {
	case ThresholdNone:
		return 0
	case ThresholdWarning:
		return 1
	case ThresholdCritical:
		return 2
	case ThresholdExceeded:
		return 3
	default:
		return 0
	}
}
