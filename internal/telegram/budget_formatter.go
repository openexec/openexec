package telegram

import (
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/budget"
	"github.com/openexec/openexec/internal/protocol"
)

// Budget emoji constants.
const (
	// EmojiBudget is the budget/money emoji.
	EmojiBudget = "💰"

	// EmojiBudgetWarning is shown for budget warnings.
	EmojiBudgetWarning = "💸"

	// EmojiBudgetCritical is shown for critical budget alerts.
	EmojiBudgetCritical = "🚨"

	// EmojiBudgetExceeded is shown when budget is exceeded.
	EmojiBudgetExceeded = "🛑"

	// EmojiProgress is used for progress bars.
	EmojiProgress = "▓"

	// EmojiProgressEmpty is used for empty progress bars.
	EmojiProgressEmpty = "░"
)

// BudgetFormatter formats budget alerts and status for Telegram notifications.
type BudgetFormatter struct {
	// includeEmojis controls whether emojis are included in output.
	includeEmojis bool

	// maxMessageLength is the maximum length for notification messages.
	maxMessageLength int

	// progressBarWidth is the width of the progress bar in characters.
	progressBarWidth int
}

// FormattedBudgetAlert represents a formatted budget alert ready for delivery.
type FormattedBudgetAlert struct {
	// Title is the main alert title/header.
	Title string

	// Body is the full alert content.
	Body string

	// Summary is a one-line summary of the alert.
	Summary string

	// Severity indicates the alert severity level.
	Severity protocol.AlertSeverity

	// SessionID is the associated session ID (if any).
	SessionID string
}

// NewBudgetFormatter creates a new BudgetFormatter with default settings.
func NewBudgetFormatter() *BudgetFormatter {
	return &BudgetFormatter{
		includeEmojis:    true,
		maxMessageLength: MaxTelegramMessageLength,
		progressBarWidth: 20,
	}
}

// SetIncludeEmojis enables or disables emoji inclusion.
func (f *BudgetFormatter) SetIncludeEmojis(include bool) {
	f.includeEmojis = include
}

// SetMaxMessageLength sets the maximum message length.
func (f *BudgetFormatter) SetMaxMessageLength(length int) {
	if length > 0 {
		f.maxMessageLength = length
	}
}

// FormatAlert formats a budget Alert into a notification message.
func (f *BudgetFormatter) FormatAlert(alert *budget.Alert) *FormattedBudgetAlert {
	if alert == nil {
		return &FormattedBudgetAlert{
			Title:   "Budget Alert",
			Body:    "No alert data available.",
			Summary: "No alert data",
		}
	}

	var sb strings.Builder

	// Build title
	title := f.formatAlertTitle(alert)
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Budget details
	sb.WriteString(f.formatBudgetLine("Budget Type", string(alert.Type)))
	sb.WriteString(f.formatBudgetLine("Current Spend", fmt.Sprintf("$%.2f", alert.CurrentSpend)))
	sb.WriteString(f.formatBudgetLine("Budget Limit", fmt.Sprintf("$%.2f", alert.BudgetLimit)))

	// Progress bar
	sb.WriteString("\n")
	sb.WriteString(f.formatProgressBar(alert.PercentUsed))
	sb.WriteString(fmt.Sprintf(" %.1f%%\n", alert.PercentUsed))

	// Session info
	if alert.SessionID != "" {
		sb.WriteString(fmt.Sprintf("\nSession: %s", alert.SessionID))
	}

	// Alert message
	sb.WriteString(fmt.Sprintf("\n\n%s", alert.Message))

	body := sb.String()

	// Truncate if necessary
	if len(body) > f.maxMessageLength {
		body = body[:f.maxMessageLength-50] + "\n\n... (truncated)"
	}

	return &FormattedBudgetAlert{
		Title:     title,
		Body:      body,
		Summary:   f.formatAlertSummary(alert),
		Severity:  f.thresholdToSeverity(alert.Threshold),
		SessionID: alert.SessionID,
	}
}

// FormatStatus formats a budget Status into a notification message.
func (f *BudgetFormatter) FormatStatus(status *budget.Status) string {
	if status == nil {
		return "No budget status available."
	}

	var sb strings.Builder

	// Header
	if f.includeEmojis {
		sb.WriteString(fmt.Sprintf("%s *Budget Status*\n\n", EmojiBudget))
	} else {
		sb.WriteString("Budget Status\n\n")
	}

	// Total budget
	if status.TotalBudgetUSD > 0 {
		sb.WriteString("*Total Budget*\n")
		sb.WriteString(f.formatStatusLine(status.TotalSpentUSD, status.TotalBudgetUSD, status.TotalPercentUsed, status.TotalThreshold))
		sb.WriteString("\n\n")
	}

	// Daily budget
	if status.DailyBudgetUSD > 0 {
		sb.WriteString("*Daily Budget*\n")
		sb.WriteString(f.formatStatusLine(status.DailySpentUSD, status.DailyBudgetUSD, status.DailyPercentUsed, status.DailyThreshold))
		sb.WriteString("\n\n")
	}

	// Session budget
	if status.SessionBudgetUSD > 0 && status.SessionSpentUSD > 0 {
		sb.WriteString("*Session Budget*\n")
		sb.WriteString(f.formatStatusLine(status.SessionSpentUSD, status.SessionBudgetUSD, status.SessionPercentUsed, status.SessionThreshold))
		sb.WriteString("\n\n")
	}

	// Blocked status
	if status.IsBlocked {
		if f.includeEmojis {
			sb.WriteString(fmt.Sprintf("%s *BLOCKED*: %s\n", EmojiBudgetExceeded, status.BlockReason))
		} else {
			sb.WriteString(fmt.Sprintf("BLOCKED: %s\n", status.BlockReason))
		}
	}

	return sb.String()
}

// formatAlertTitle creates the alert title with appropriate emoji.
func (f *BudgetFormatter) formatAlertTitle(alert *budget.Alert) string {
	var emoji string
	var statusText string

	switch alert.Threshold {
	case budget.ThresholdWarning:
		emoji = EmojiBudgetWarning
		statusText = "Budget Warning"
	case budget.ThresholdCritical:
		emoji = EmojiBudgetCritical
		statusText = "Budget Critical"
	case budget.ThresholdExceeded:
		emoji = EmojiBudgetExceeded
		statusText = "Budget Exceeded"
	default:
		emoji = EmojiBudget
		statusText = "Budget Alert"
	}

	if f.includeEmojis {
		return fmt.Sprintf("%s %s", emoji, statusText)
	}
	return statusText
}

// formatAlertSummary creates a one-line summary of the alert.
func (f *BudgetFormatter) formatAlertSummary(alert *budget.Alert) string {
	var emoji string
	if f.includeEmojis {
		switch alert.Threshold {
		case budget.ThresholdWarning:
			emoji = EmojiBudgetWarning + " "
		case budget.ThresholdCritical:
			emoji = EmojiBudgetCritical + " "
		case budget.ThresholdExceeded:
			emoji = EmojiBudgetExceeded + " "
		}
	}

	return fmt.Sprintf("%s%s: $%.2f / $%.2f (%.1f%%)",
		emoji, alert.Type, alert.CurrentSpend, alert.BudgetLimit, alert.PercentUsed)
}

// formatBudgetLine formats a single line with label and value.
func (f *BudgetFormatter) formatBudgetLine(label, value string) string {
	return fmt.Sprintf("%s: %s\n", label, value)
}

// formatProgressBar creates a text-based progress bar.
func (f *BudgetFormatter) formatProgressBar(percentUsed float64) string {
	if !f.includeEmojis {
		// Simple ASCII progress bar
		filled := int((percentUsed / 100) * float64(f.progressBarWidth))
		if filled > f.progressBarWidth {
			filled = f.progressBarWidth
		}
		if filled < 0 {
			filled = 0
		}

		bar := strings.Repeat("#", filled) + strings.Repeat("-", f.progressBarWidth-filled)
		return fmt.Sprintf("[%s]", bar)
	}

	// Emoji progress bar
	filled := int((percentUsed / 100) * float64(f.progressBarWidth))
	if filled > f.progressBarWidth {
		filled = f.progressBarWidth
	}
	if filled < 0 {
		filled = 0
	}

	return strings.Repeat(EmojiProgress, filled) + strings.Repeat(EmojiProgressEmpty, f.progressBarWidth-filled)
}

// formatStatusLine formats spending info with progress bar.
func (f *BudgetFormatter) formatStatusLine(spent, limit, percentUsed float64, threshold budget.AlertThreshold) string {
	var sb strings.Builder

	// Threshold indicator
	if f.includeEmojis {
		switch threshold {
		case budget.ThresholdWarning:
			sb.WriteString(EmojiBudgetWarning + " ")
		case budget.ThresholdCritical:
			sb.WriteString(EmojiBudgetCritical + " ")
		case budget.ThresholdExceeded:
			sb.WriteString(EmojiBudgetExceeded + " ")
		}
	}

	// Spending info
	sb.WriteString(fmt.Sprintf("$%.2f / $%.2f\n", spent, limit))

	// Progress bar
	sb.WriteString(f.formatProgressBar(percentUsed))
	sb.WriteString(fmt.Sprintf(" %.1f%%", percentUsed))

	return sb.String()
}

// thresholdToSeverity converts a budget threshold to a protocol severity.
func (f *BudgetFormatter) thresholdToSeverity(threshold budget.AlertThreshold) protocol.AlertSeverity {
	switch threshold {
	case budget.ThresholdWarning:
		return protocol.AlertSeverityWarning
	case budget.ThresholdCritical, budget.ThresholdExceeded:
		return protocol.AlertSeverityCritical
	default:
		return protocol.AlertSeverityInfo
	}
}

// FormatBudgetAlert creates a formatted budget alert message.
// This is a convenience function for common use cases.
func FormatBudgetAlert(alert *budget.Alert) string {
	formatter := NewBudgetFormatter()
	formatted := formatter.FormatAlert(alert)
	return formatted.Body
}

// FormatBudgetStatus creates a formatted budget status message.
// This is a convenience function for common use cases.
func FormatBudgetStatus(status *budget.Status) string {
	formatter := NewBudgetFormatter()
	return formatter.FormatStatus(status)
}
