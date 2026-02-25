package telegram

import (
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/protocol"
)

// Status emojis for notifications
const (
	// EmojiSuccess is displayed for successful task completion.
	EmojiSuccess = "✅"

	// EmojiFailure is displayed for failed tasks.
	EmojiFailure = "❌"

	// EmojyCancelled is displayed for cancelled tasks.
	EmojiCancelled = "🚫"

	// EmojiTimeout is displayed for timed out tasks.
	EmojiTimeout = "⏰"

	// EmojiClock is displayed alongside duration information.
	EmojiClock = "⏱️"

	// EmojiProject is displayed alongside project information.
	EmojiProject = "📁"

	// EmojiTask is displayed alongside task information.
	EmojiTask = "📋"
)

// NotificationFormatter formats task completion events for Telegram notifications.
type NotificationFormatter struct {
	// includeEmojis controls whether emojis are included in output.
	includeEmojis bool

	// maxMessageLength is the maximum length for notification messages.
	maxMessageLength int
}

// FormattedNotification represents a formatted notification ready for delivery.
type FormattedNotification struct {
	// Title is the main notification title/header.
	Title string

	// Body is the full notification content.
	Body string

	// IsSuccess indicates whether this is a success notification.
	IsSuccess bool

	// Summary is a one-line summary of the notification.
	Summary string
}

// NewNotificationFormatter creates a new NotificationFormatter with default settings.
func NewNotificationFormatter() *NotificationFormatter {
	return &NotificationFormatter{
		includeEmojis:    true,
		maxMessageLength: MaxTelegramMessageLength,
	}
}

// SetIncludeEmojis enables or disables emoji inclusion in notifications.
func (f *NotificationFormatter) SetIncludeEmojis(include bool) {
	f.includeEmojis = include
}

// SetMaxMessageLength sets the maximum message length.
func (f *NotificationFormatter) SetMaxMessageLength(length int) {
	if length > 0 {
		f.maxMessageLength = length
	}
}

// Format formats a TaskCompleteEvent into a notification message.
func (f *NotificationFormatter) Format(event *protocol.TaskCompleteEvent) *FormattedNotification {
	if event == nil {
		return &FormattedNotification{
			Title:   "Notification",
			Body:    "No event data available.",
			Summary: "No event data",
		}
	}

	var sb strings.Builder

	// Build title with status emoji
	title := f.formatTitle(event)

	// Build body
	sb.WriteString(title)
	sb.WriteString("\n\n")

	// Task ID
	if f.includeEmojis {
		sb.WriteString(fmt.Sprintf("%s Task: %s\n", EmojiTask, event.TaskID))
	} else {
		sb.WriteString(fmt.Sprintf("Task: %s\n", event.TaskID))
	}

	// Project ID (if present)
	if event.ProjectID != "" {
		if f.includeEmojis {
			sb.WriteString(fmt.Sprintf("%s Project: %s\n", EmojiProject, event.ProjectID))
		} else {
			sb.WriteString(fmt.Sprintf("Project: %s\n", event.ProjectID))
		}
	}

	// Duration
	if event.Duration > 0 {
		durationStr := f.formatDuration(event.Duration)
		if f.includeEmojis {
			sb.WriteString(fmt.Sprintf("%s Duration: %s\n", EmojiClock, durationStr))
		} else {
			sb.WriteString(fmt.Sprintf("Duration: %s\n", durationStr))
		}
	}

	// Message (if present)
	if event.Message != "" {
		sb.WriteString(fmt.Sprintf("\n%s", event.Message))
	}

	// Error details (for failures)
	if event.Error != "" && event.IsFailure() {
		sb.WriteString(fmt.Sprintf("\n\nError: %s", event.Error))
	}

	// Exit code (if present and non-zero)
	if event.ExitCode != nil && *event.ExitCode != 0 {
		sb.WriteString(fmt.Sprintf("\nExit code: %d", *event.ExitCode))
	}

	body := sb.String()

	// Truncate if necessary
	if len(body) > f.maxMessageLength {
		body = body[:f.maxMessageLength-50] + "\n\n... (truncated)"
	}

	return &FormattedNotification{
		Title:     title,
		Body:      body,
		IsSuccess: event.IsSuccess(),
		Summary:   f.formatSummary(event),
	}
}

// formatTitle creates the notification title with appropriate emoji.
func (f *NotificationFormatter) formatTitle(event *protocol.TaskCompleteEvent) string {
	var emoji, statusText string

	switch event.Status {
	case protocol.TaskCompleteStatusSuccess:
		emoji = EmojiSuccess
		statusText = "Task Completed Successfully"
	case protocol.TaskCompleteStatusFailure:
		emoji = EmojiFailure
		statusText = "Task Failed"
	case protocol.TaskCompleteStatusCancelled:
		emoji = EmojiCancelled
		statusText = "Task Cancelled"
	case protocol.TaskCompleteStatusTimeout:
		emoji = EmojiTimeout
		statusText = "Task Timed Out"
	default:
		emoji = EmojiTask
		statusText = "Task Status Update"
	}

	if f.includeEmojis {
		return fmt.Sprintf("%s %s", emoji, statusText)
	}
	return statusText
}

// formatSummary creates a one-line summary of the notification.
func (f *NotificationFormatter) formatSummary(event *protocol.TaskCompleteEvent) string {
	var emoji string
	if f.includeEmojis {
		switch event.Status {
		case protocol.TaskCompleteStatusSuccess:
			emoji = EmojiSuccess + " "
		case protocol.TaskCompleteStatusFailure:
			emoji = EmojiFailure + " "
		case protocol.TaskCompleteStatusCancelled:
			emoji = EmojiCancelled + " "
		case protocol.TaskCompleteStatusTimeout:
			emoji = EmojiTimeout + " "
		}
	}

	taskPart := event.TaskID
	if len(taskPart) > 20 {
		taskPart = taskPart[:17] + "..."
	}

	durationPart := ""
	if event.Duration > 0 {
		durationPart = fmt.Sprintf(" (%s)", f.formatDuration(event.Duration))
	}

	return fmt.Sprintf("%s%s: %s%s", emoji, taskPart, event.Status, durationPart)
}

// formatDuration formats a duration in seconds to a human-readable string.
func (f *NotificationFormatter) formatDuration(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))

	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", seconds)
	}

	if d < time.Hour {
		mins := int(d.Minutes())
		secs := int(d.Seconds()) % 60
		if secs == 0 {
			return fmt.Sprintf("%dm", mins)
		}
		return fmt.Sprintf("%dm %ds", mins, secs)
	}

	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if mins == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, mins)
}

// FormatSuccess creates a success notification message from basic parameters.
func (f *NotificationFormatter) FormatSuccess(taskID, projectID, message string, durationSeconds float64) *FormattedNotification {
	event := protocol.NewTaskCompleteEventWithDetails(
		taskID,
		protocol.TaskCompleteStatusSuccess,
		message,
		projectID,
		durationSeconds,
	)
	return f.Format(event)
}

// FormatFailure creates a failure notification message from basic parameters.
func (f *NotificationFormatter) FormatFailure(taskID, projectID, message, errorDetail string, durationSeconds float64) *FormattedNotification {
	event := protocol.NewTaskCompleteEventWithDetails(
		taskID,
		protocol.TaskCompleteStatusFailure,
		message,
		projectID,
		durationSeconds,
	)
	event.Error = errorDetail
	return f.Format(event)
}

// FormatCancelled creates a cancelled notification message from basic parameters.
func (f *NotificationFormatter) FormatCancelled(taskID, projectID, message string, durationSeconds float64) *FormattedNotification {
	event := protocol.NewTaskCompleteEventWithDetails(
		taskID,
		protocol.TaskCompleteStatusCancelled,
		message,
		projectID,
		durationSeconds,
	)
	return f.Format(event)
}

// FormatTimeout creates a timeout notification message from basic parameters.
func (f *NotificationFormatter) FormatTimeout(taskID, projectID, message string, durationSeconds float64) *FormattedNotification {
	event := protocol.NewTaskCompleteEventWithDetails(
		taskID,
		protocol.TaskCompleteStatusTimeout,
		message,
		projectID,
		durationSeconds,
	)
	return f.Format(event)
}

// GetStatusEmoji returns the appropriate emoji for a given task status.
func GetStatusEmoji(status protocol.TaskCompleteStatus) string {
	switch status {
	case protocol.TaskCompleteStatusSuccess:
		return EmojiSuccess
	case protocol.TaskCompleteStatusFailure:
		return EmojiFailure
	case protocol.TaskCompleteStatusCancelled:
		return EmojiCancelled
	case protocol.TaskCompleteStatusTimeout:
		return EmojiTimeout
	default:
		return EmojiTask
	}
}

// FormatDurationString formats a duration in seconds to a human-readable string.
// This is a convenience function that uses the default formatting.
func FormatDurationString(seconds float64) string {
	f := NewNotificationFormatter()
	return f.formatDuration(seconds)
}
