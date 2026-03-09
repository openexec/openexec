package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/protocol"
)

// Alert emoji constants for different severity levels and types.
const (
	// EmojiAlertCritical is displayed for critical alerts.
	EmojiAlertCritical = "🚨"

	// EmojiAlertWarning is displayed for warning alerts.
	EmojiAlertWarning = "⚠️"

	// EmojiAlertInfo is displayed for informational alerts.
	EmojiAlertInfo = "ℹ️"

	// EmojiLongRunning is displayed for long-running task alerts.
	EmojiLongRunning = "⏰"

	// EmojiStalled is displayed for stalled task alerts.
	EmojiStalled = "🔄"

	// EmojiResourceLimit is displayed for resource limit alerts.
	EmojiResourceLimit = "📊"

	// EmojiError is displayed for error alerts.
	EmojiError = "❌"
)

// AlertFormatter formats alert events for Telegram notifications.
type AlertFormatter struct {
	// includeEmojis controls whether emojis are included in output.
	includeEmojis bool

	// maxMessageLength is the maximum length for notification messages.
	maxMessageLength int
}

// FormattedAlert represents a formatted alert ready for delivery.
type FormattedAlert struct {
	// Title is the main alert title/header.
	Title string

	// Body is the full alert content.
	Body string

	// Severity indicates the alert severity level.
	Severity protocol.AlertSeverity

	// Summary is a one-line summary of the alert.
	Summary string

	// TaskID is the associated task ID (if any).
	TaskID string

	// ProjectID is the associated project ID (if any).
	ProjectID string
}

// NewAlertFormatter creates a new AlertFormatter with default settings.
func NewAlertFormatter() *AlertFormatter {
	return &AlertFormatter{
		includeEmojis:    true,
		maxMessageLength: MaxTelegramMessageLength,
	}
}

// SetIncludeEmojis enables or disables emoji inclusion in alerts.
func (f *AlertFormatter) SetIncludeEmojis(include bool) {
	f.includeEmojis = include
}

// SetMaxMessageLength sets the maximum message length.
func (f *AlertFormatter) SetMaxMessageLength(length int) {
	if length > 0 {
		f.maxMessageLength = length
	}
}

// Format formats an AlertEvent into a notification message.
func (f *AlertFormatter) Format(event *protocol.AlertEvent) *FormattedAlert {
	if event == nil {
		return &FormattedAlert{
			Title:   "Alert",
			Body:    "No alert data available.",
			Summary: "No alert data",
		}
	}

	var sb strings.Builder

	// Build title with severity emoji
	title := f.formatTitle(event)

	// Build body
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Alert details
	if event.TaskID != "" {
		if f.includeEmojis {
			sb.WriteString(fmt.Sprintf("%s Task: %s\n", EmojiTask, event.TaskID))
		} else {
			sb.WriteString(fmt.Sprintf("Task: %s\n", event.TaskID))
		}
	}

	if event.ProjectID != "" {
		if f.includeEmojis {
			sb.WriteString(fmt.Sprintf("%s Project: %s\n", EmojiProject, event.ProjectID))
		} else {
			sb.WriteString(fmt.Sprintf("Project: %s\n", event.ProjectID))
		}
	}

	// Duration (for long-running alerts)
	if event.Duration > 0 {
		durationStr := FormatDuration(event.Duration)
		if f.includeEmojis {
			sb.WriteString(fmt.Sprintf("%s Duration: %s\n", EmojiClock, durationStr))
		} else {
			sb.WriteString(fmt.Sprintf("Duration: %s\n", durationStr))
		}
	}

	// Threshold information
	if event.Threshold > 0 {
		thresholdStr := FormatDuration(event.Threshold)
		sb.WriteString(fmt.Sprintf("Threshold: %s\n", thresholdStr))
	}

	// Message (if present)
	if event.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s", event.Message))
	}

	body := sb.String()

	// Truncate if necessary
	if len(body) > f.maxMessageLength {
		body = body[:f.maxMessageLength-50] + "\n\n... (truncated)"
	}

	return &FormattedAlert{
		Title:     title,
		Body:      body,
		Severity:  event.Severity,
		Summary:   f.formatSummary(event),
		TaskID:    event.TaskID,
		ProjectID: event.ProjectID,
	}
}

// formatTitle creates the alert title with appropriate emoji.
func (f *AlertFormatter) formatTitle(event *protocol.AlertEvent) string {
	var severityEmoji, typeEmoji, statusText string

	// Severity emoji
	switch event.Severity {
	case protocol.AlertSeverityCritical:
		severityEmoji = EmojiAlertCritical
	case protocol.AlertSeverityWarning:
		severityEmoji = EmojiAlertWarning
	case protocol.AlertSeverityInfo:
		severityEmoji = EmojiAlertInfo
	default:
		severityEmoji = EmojiAlertInfo
	}

	// Type emoji and text
	switch event.AlertType {
	case protocol.AlertTypeLongRunning:
		typeEmoji = EmojiLongRunning
		statusText = "Long Running Task"
	case protocol.AlertTypeStalled:
		typeEmoji = EmojiStalled
		statusText = "Task Stalled"
	case protocol.AlertTypeResourceLimit:
		typeEmoji = EmojiResourceLimit
		statusText = "Resource Limit"
	case protocol.AlertTypeError:
		typeEmoji = EmojiError
		statusText = "Error Alert"
	default:
		typeEmoji = severityEmoji
		statusText = "Alert"
	}

	// Use custom title if provided
	if event.Title != "" {
		statusText = event.Title
	}

	if f.includeEmojis {
		return fmt.Sprintf("%s %s %s", severityEmoji, typeEmoji, statusText)
	}
	return statusText
}

// formatSummary creates a one-line summary of the alert.
func (f *AlertFormatter) formatSummary(event *protocol.AlertEvent) string {
	var emoji string
	if f.includeEmojis {
		switch event.Severity {
		case protocol.AlertSeverityCritical:
			emoji = EmojiAlertCritical + " "
		case protocol.AlertSeverityWarning:
			emoji = EmojiAlertWarning + " "
		case protocol.AlertSeverityInfo:
			emoji = EmojiAlertInfo + " "
		}
	}

	taskPart := event.TaskID
	if len(taskPart) > 20 {
		taskPart = taskPart[:17] + "..."
	}

	if taskPart != "" {
		return fmt.Sprintf("%s%s: %s (%s)", emoji, event.AlertType, taskPart, event.Severity)
	}
	return fmt.Sprintf("%s%s (%s)", emoji, event.AlertType, event.Severity)
}

// FormatLongRunning creates an alert notification for a long-running task.
func (f *AlertFormatter) FormatLongRunning(taskID, projectID string, durationSeconds, thresholdSeconds float64) *FormattedAlert {
	event := protocol.NewLongRunningAlertEvent(
		fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		taskID,
		projectID,
		durationSeconds,
		thresholdSeconds,
	)
	return f.Format(event)
}

// FormatError creates an alert notification for an error condition.
func (f *AlertFormatter) FormatError(taskID, projectID, title, message string, severity protocol.AlertSeverity) *FormattedAlert {
	event := protocol.NewAlertEvent(
		fmt.Sprintf("alert_%d", time.Now().UnixNano()),
		protocol.AlertTypeError,
		severity,
		title,
	)
	event.TaskID = taskID
	event.ProjectID = projectID
	event.Message = message
	return f.Format(event)
}

// GetSeverityEmoji returns the appropriate emoji for a given alert severity.
func GetSeverityEmoji(severity protocol.AlertSeverity) string {
	switch severity {
	case protocol.AlertSeverityCritical:
		return EmojiAlertCritical
	case protocol.AlertSeverityWarning:
		return EmojiAlertWarning
	case protocol.AlertSeverityInfo:
		return EmojiAlertInfo
	default:
		return EmojiAlertInfo
	}
}

// GetAlertTypeEmoji returns the appropriate emoji for a given alert type.
func GetAlertTypeEmoji(alertType protocol.AlertType) string {
	switch alertType {
	case protocol.AlertTypeLongRunning:
		return EmojiLongRunning
	case protocol.AlertTypeStalled:
		return EmojiStalled
	case protocol.AlertTypeResourceLimit:
		return EmojiResourceLimit
	case protocol.AlertTypeError:
		return EmojiError
	default:
		return EmojiAlertInfo
	}
}
