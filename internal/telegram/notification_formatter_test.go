package telegram

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/protocol"
)

func TestNewNotificationFormatter(t *testing.T) {
	formatter := NewNotificationFormatter()

	if formatter == nil {
		t.Fatal("Expected non-nil formatter")
	}

	if !formatter.includeEmojis {
		t.Error("Expected includeEmojis to be true by default")
	}

	if formatter.maxMessageLength != MaxTelegramMessageLength {
		t.Errorf("Expected maxMessageLength %d, got %d", MaxTelegramMessageLength, formatter.maxMessageLength)
	}
}

func TestSetIncludeEmojis(t *testing.T) {
	formatter := NewNotificationFormatter()

	formatter.SetIncludeEmojis(false)
	if formatter.includeEmojis {
		t.Error("Expected includeEmojis to be false")
	}

	formatter.SetIncludeEmojis(true)
	if !formatter.includeEmojis {
		t.Error("Expected includeEmojis to be true")
	}
}

func TestSetMaxMessageLength(t *testing.T) {
	formatter := NewNotificationFormatter()

	formatter.SetMaxMessageLength(1000)
	if formatter.maxMessageLength != 1000 {
		t.Errorf("Expected maxMessageLength 1000, got %d", formatter.maxMessageLength)
	}

	// Invalid values should be ignored
	formatter.SetMaxMessageLength(0)
	if formatter.maxMessageLength != 1000 {
		t.Errorf("Expected maxMessageLength to remain 1000, got %d", formatter.maxMessageLength)
	}

	formatter.SetMaxMessageLength(-1)
	if formatter.maxMessageLength != 1000 {
		t.Errorf("Expected maxMessageLength to remain 1000, got %d", formatter.maxMessageLength)
	}
}

func TestFormatNilEvent(t *testing.T) {
	formatter := NewNotificationFormatter()

	result := formatter.Format(nil)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Body == "" {
		t.Error("Expected non-empty body")
	}

	if !strings.Contains(result.Body, "No event data") {
		t.Errorf("Expected 'No event data' in body, got: %s", result.Body)
	}
}

func TestFormatSuccessEvent(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEventWithDetails(
		"task-123",
		protocol.TaskCompleteStatusSuccess,
		"Build completed",
		"my-project",
		45.5,
	)

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if !result.IsSuccess {
		t.Error("Expected IsSuccess to be true")
	}

	// Check for success emoji
	if !strings.Contains(result.Title, EmojiSuccess) {
		t.Errorf("Expected success emoji in title, got: %s", result.Title)
	}

	// Check for task ID
	if !strings.Contains(result.Body, "task-123") {
		t.Errorf("Expected task ID in body, got: %s", result.Body)
	}

	// Check for project ID
	if !strings.Contains(result.Body, "my-project") {
		t.Errorf("Expected project ID in body, got: %s", result.Body)
	}

	// Check for duration
	if !strings.Contains(result.Body, "45.5s") {
		t.Errorf("Expected duration in body, got: %s", result.Body)
	}

	// Check for message
	if !strings.Contains(result.Body, "Build completed") {
		t.Errorf("Expected message in body, got: %s", result.Body)
	}
}

func TestFormatFailureEvent(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEventWithDetails(
		"task-456",
		protocol.TaskCompleteStatusFailure,
		"Tests failed",
		"test-project",
		120.0,
	)
	event.Error = "3 tests failed"
	exitCode := 1
	event.ExitCode = &exitCode

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.IsSuccess {
		t.Error("Expected IsSuccess to be false")
	}

	// Check for failure emoji
	if !strings.Contains(result.Title, EmojiFailure) {
		t.Errorf("Expected failure emoji in title, got: %s", result.Title)
	}

	// Check for error details
	if !strings.Contains(result.Body, "3 tests failed") {
		t.Errorf("Expected error details in body, got: %s", result.Body)
	}

	// Check for exit code
	if !strings.Contains(result.Body, "Exit code: 1") {
		t.Errorf("Expected exit code in body, got: %s", result.Body)
	}

	// Check for duration (2 minutes)
	if !strings.Contains(result.Body, "2m") {
		t.Errorf("Expected duration in body, got: %s", result.Body)
	}
}

func TestFormatCancelledEvent(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"task-789",
		protocol.TaskCompleteStatusCancelled,
		"User cancelled the task",
	)

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check for cancelled emoji
	if !strings.Contains(result.Title, EmojiCancelled) {
		t.Errorf("Expected cancelled emoji in title, got: %s", result.Title)
	}

	if !strings.Contains(result.Title, "Cancelled") {
		t.Errorf("Expected 'Cancelled' in title, got: %s", result.Title)
	}
}

func TestFormatTimeoutEvent(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEventWithDetails(
		"task-timeout",
		protocol.TaskCompleteStatusTimeout,
		"Task exceeded time limit",
		"slow-project",
		3600.0, // 1 hour
	)

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check for timeout emoji
	if !strings.Contains(result.Title, EmojiTimeout) {
		t.Errorf("Expected timeout emoji in title, got: %s", result.Title)
	}

	if !strings.Contains(result.Title, "Timed Out") {
		t.Errorf("Expected 'Timed Out' in title, got: %s", result.Title)
	}

	// Check for duration (1 hour)
	if !strings.Contains(result.Body, "1h") {
		t.Errorf("Expected duration in body, got: %s", result.Body)
	}
}

func TestFormatWithoutEmojis(t *testing.T) {
	formatter := NewNotificationFormatter()
	formatter.SetIncludeEmojis(false)

	event := protocol.NewTaskCompleteEventWithDetails(
		"task-no-emoji",
		protocol.TaskCompleteStatusSuccess,
		"Done",
		"project-x",
		10.0,
	)

	result := formatter.Format(event)

	// Should not contain emojis
	if strings.Contains(result.Body, EmojiSuccess) {
		t.Errorf("Expected no success emoji, got: %s", result.Body)
	}

	if strings.Contains(result.Body, EmojiTask) {
		t.Errorf("Expected no task emoji, got: %s", result.Body)
	}

	if strings.Contains(result.Body, EmojiProject) {
		t.Errorf("Expected no project emoji, got: %s", result.Body)
	}

	if strings.Contains(result.Body, EmojiClock) {
		t.Errorf("Expected no clock emoji, got: %s", result.Body)
	}

	// But should still contain the text
	if !strings.Contains(result.Title, "Task Completed Successfully") {
		t.Errorf("Expected status text in title, got: %s", result.Title)
	}
}

func TestFormatWithoutProjectID(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"task-no-project",
		protocol.TaskCompleteStatusSuccess,
		"Completed",
	)

	result := formatter.Format(event)

	if strings.Contains(result.Body, "Project:") {
		t.Errorf("Should not include Project line, got: %s", result.Body)
	}
}

func TestFormatWithoutDuration(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"task-no-duration",
		protocol.TaskCompleteStatusSuccess,
		"Completed",
	)
	// Duration is 0 by default

	result := formatter.Format(event)

	if strings.Contains(result.Body, "Duration:") {
		t.Errorf("Should not include Duration line, got: %s", result.Body)
	}
}

func TestFormatDuration(t *testing.T) {
	formatter := NewNotificationFormatter()

	tests := []struct {
		seconds  float64
		expected string
	}{
		{0.5, "500ms"},
		{0.001, "1ms"},
		{1.0, "1.0s"},
		{1.5, "1.5s"},
		{30.0, "30.0s"},
		{59.9, "59.9s"},
		{60.0, "1m"},
		{90.0, "1m 30s"},
		{120.0, "2m"},
		{125.0, "2m 5s"},
		{3600.0, "1h"},
		{3660.0, "1h 1m"},
		{7200.0, "2h"},
		{7320.0, "2h 2m"},
	}

	for _, tt := range tests {
		result := formatter.formatDuration(tt.seconds)
		if result != tt.expected {
			t.Errorf("formatDuration(%.1f) = %q, want %q", tt.seconds, result, tt.expected)
		}
	}
}

func TestFormatSummary(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEventWithDetails(
		"task-123",
		protocol.TaskCompleteStatusSuccess,
		"Done",
		"project",
		45.0,
	)

	result := formatter.Format(event)

	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	if !strings.Contains(result.Summary, "task-123") {
		t.Errorf("Expected task ID in summary, got: %s", result.Summary)
	}

	if !strings.Contains(result.Summary, "success") {
		t.Errorf("Expected status in summary, got: %s", result.Summary)
	}

	if !strings.Contains(result.Summary, "45.0s") {
		t.Errorf("Expected duration in summary, got: %s", result.Summary)
	}
}

func TestFormatSummaryLongTaskID(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"very-long-task-id-that-exceeds-twenty-characters",
		protocol.TaskCompleteStatusSuccess,
		"Done",
	)

	result := formatter.Format(event)

	// Task ID should be truncated in summary
	if len(result.Summary) > 100 {
		t.Errorf("Summary too long: %s", result.Summary)
	}

	if !strings.Contains(result.Summary, "...") {
		t.Errorf("Expected truncation indicator in summary, got: %s", result.Summary)
	}
}

func TestFormatSuccessHelper(t *testing.T) {
	formatter := NewNotificationFormatter()

	result := formatter.FormatSuccess("task-1", "proj-1", "Build passed", 30.0)

	if !result.IsSuccess {
		t.Error("Expected IsSuccess to be true")
	}

	if !strings.Contains(result.Title, EmojiSuccess) {
		t.Errorf("Expected success emoji in title, got: %s", result.Title)
	}
}

func TestFormatFailureHelper(t *testing.T) {
	formatter := NewNotificationFormatter()

	result := formatter.FormatFailure("task-2", "proj-2", "Build failed", "Compilation error", 15.0)

	if result.IsSuccess {
		t.Error("Expected IsSuccess to be false")
	}

	if !strings.Contains(result.Title, EmojiFailure) {
		t.Errorf("Expected failure emoji in title, got: %s", result.Title)
	}

	if !strings.Contains(result.Body, "Compilation error") {
		t.Errorf("Expected error details in body, got: %s", result.Body)
	}
}

func TestFormatCancelledHelper(t *testing.T) {
	formatter := NewNotificationFormatter()

	result := formatter.FormatCancelled("task-3", "proj-3", "User cancelled", 5.0)

	if result.IsSuccess {
		t.Error("Expected IsSuccess to be false")
	}

	if !strings.Contains(result.Title, EmojiCancelled) {
		t.Errorf("Expected cancelled emoji in title, got: %s", result.Title)
	}
}

func TestFormatTimeoutHelper(t *testing.T) {
	formatter := NewNotificationFormatter()

	result := formatter.FormatTimeout("task-4", "proj-4", "Timed out", 600.0)

	if result.IsSuccess {
		t.Error("Expected IsSuccess to be false")
	}

	if !strings.Contains(result.Title, EmojiTimeout) {
		t.Errorf("Expected timeout emoji in title, got: %s", result.Title)
	}
}

func TestGetStatusEmoji(t *testing.T) {
	tests := []struct {
		status   protocol.TaskCompleteStatus
		expected string
	}{
		{protocol.TaskCompleteStatusSuccess, EmojiSuccess},
		{protocol.TaskCompleteStatusFailure, EmojiFailure},
		{protocol.TaskCompleteStatusCancelled, EmojiCancelled},
		{protocol.TaskCompleteStatusTimeout, EmojiTimeout},
		{protocol.TaskCompleteStatus("unknown"), EmojiTask},
	}

	for _, tt := range tests {
		result := GetStatusEmoji(tt.status)
		if result != tt.expected {
			t.Errorf("GetStatusEmoji(%q) = %q, want %q", tt.status, result, tt.expected)
		}
	}
}

func TestFormatDurationString(t *testing.T) {
	result := FormatDurationString(90.0)
	if result != "1m 30s" {
		t.Errorf("FormatDurationString(90.0) = %q, want %q", result, "1m 30s")
	}
}

func TestFormattedNotificationStructure(t *testing.T) {
	fn := FormattedNotification{
		Title:     "Test Title",
		Body:      "Test Body",
		IsSuccess: true,
		Summary:   "Test Summary",
	}

	if fn.Title != "Test Title" {
		t.Error("Title not set correctly")
	}
	if fn.Body != "Test Body" {
		t.Error("Body not set correctly")
	}
	if !fn.IsSuccess {
		t.Error("IsSuccess not set correctly")
	}
	if fn.Summary != "Test Summary" {
		t.Error("Summary not set correctly")
	}
}

func TestEmojiConstants(t *testing.T) {
	// Verify emoji constants are non-empty and distinct
	emojis := []string{EmojiSuccess, EmojiFailure, EmojiCancelled, EmojiTimeout, EmojiClock, EmojiProject, EmojiTask}

	for i, emoji := range emojis {
		if emoji == "" {
			t.Errorf("Emoji constant %d is empty", i)
		}
	}

	// Check that key status emojis are distinct
	if EmojiSuccess == EmojiFailure {
		t.Error("Success and Failure emojis should be different")
	}
	if EmojiCancelled == EmojiTimeout {
		t.Error("Cancelled and Timeout emojis should be different")
	}
}

func TestNotificationFormatTruncation(t *testing.T) {
	formatter := NewNotificationFormatter()
	formatter.SetMaxMessageLength(150)

	event := protocol.NewTaskCompleteEventWithDetails(
		"task-truncate",
		protocol.TaskCompleteStatusFailure,
		"This is a very long message that will definitely need to be truncated because it exceeds our maximum message length limit",
		"long-project-name-here",
		999.0,
	)
	event.Error = "This is also a very long error message that adds to the total length"

	result := formatter.Format(event)

	if len(result.Body) > 200 {
		t.Errorf("Body should be truncated, length: %d", len(result.Body))
	}

	if !strings.Contains(result.Body, "truncated") {
		t.Errorf("Expected truncation notice in body, got: %s", result.Body)
	}
}

func TestFormatWithZeroExitCode(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"task-exit-0",
		protocol.TaskCompleteStatusSuccess,
		"Completed",
	)
	exitCode := 0
	event.ExitCode = &exitCode

	result := formatter.Format(event)

	// Should not show exit code when it's 0
	if strings.Contains(result.Body, "Exit code:") {
		t.Errorf("Should not include exit code for 0, got: %s", result.Body)
	}
}

func TestFormatWithNonZeroExitCode(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"task-exit-1",
		protocol.TaskCompleteStatusFailure,
		"Failed",
	)
	exitCode := 127
	event.ExitCode = &exitCode

	result := formatter.Format(event)

	if !strings.Contains(result.Body, "Exit code: 127") {
		t.Errorf("Expected exit code in body, got: %s", result.Body)
	}
}

func TestFormatUnknownStatus(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := &protocol.TaskCompleteEvent{
		TaskID:  "task-unknown",
		Status:  protocol.TaskCompleteStatus("unknown"),
		Message: "Unknown status",
	}

	result := formatter.Format(event)

	// Should use task emoji for unknown status
	if !strings.Contains(result.Title, EmojiTask) {
		t.Errorf("Expected task emoji for unknown status, got: %s", result.Title)
	}

	if !strings.Contains(result.Title, "Status Update") {
		t.Errorf("Expected 'Status Update' in title, got: %s", result.Title)
	}
}

func TestFormatSummaryWithoutDuration(t *testing.T) {
	formatter := NewNotificationFormatter()

	event := protocol.NewTaskCompleteEvent(
		"task-no-dur",
		protocol.TaskCompleteStatusSuccess,
		"Done",
	)

	result := formatter.Format(event)

	// Summary should not have parentheses for duration
	if strings.Contains(result.Summary, "(") {
		t.Errorf("Summary should not have duration parentheses, got: %s", result.Summary)
	}
}

func TestFormatSummaryWithoutEmojis(t *testing.T) {
	formatter := NewNotificationFormatter()
	formatter.SetIncludeEmojis(false)

	event := protocol.NewTaskCompleteEvent(
		"task-summary",
		protocol.TaskCompleteStatusSuccess,
		"Done",
	)

	result := formatter.Format(event)

	// Summary should not start with emoji
	if strings.HasPrefix(result.Summary, EmojiSuccess) {
		t.Errorf("Summary should not start with emoji, got: %s", result.Summary)
	}
}
