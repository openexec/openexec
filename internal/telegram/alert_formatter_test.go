package telegram

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/protocol"
)

func TestNewAlertFormatter(t *testing.T) {
	formatter := NewAlertFormatter()

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

func TestAlertFormatterSetIncludeEmojis(t *testing.T) {
	formatter := NewAlertFormatter()

	formatter.SetIncludeEmojis(false)
	if formatter.includeEmojis {
		t.Error("Expected includeEmojis to be false")
	}

	formatter.SetIncludeEmojis(true)
	if !formatter.includeEmojis {
		t.Error("Expected includeEmojis to be true")
	}
}

func TestAlertFormatterSetMaxMessageLength(t *testing.T) {
	formatter := NewAlertFormatter()

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

func TestAlertFormatterFormatNilEvent(t *testing.T) {
	formatter := NewAlertFormatter()

	result := formatter.Format(nil)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Body == "" {
		t.Error("Expected non-empty body")
	}

	if !strings.Contains(result.Body, "No alert data") {
		t.Errorf("Expected 'No alert data' in body, got: %s", result.Body)
	}
}

func TestAlertFormatterFormatLongRunningAlert(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewLongRunningAlertEvent(
		"alert-123",
		"task-456",
		"my-project",
		300.0, // 5 minutes
		180.0, // 3 minute threshold
	)

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check for long running emoji
	if !strings.Contains(result.Title, EmojiLongRunning) {
		t.Errorf("Expected long running emoji in title, got: %s", result.Title)
	}

	// Check for warning severity emoji
	if !strings.Contains(result.Title, EmojiAlertWarning) {
		t.Errorf("Expected warning emoji in title, got: %s", result.Title)
	}

	// Check for task ID
	if !strings.Contains(result.Body, "task-456") {
		t.Errorf("Expected task ID in body, got: %s", result.Body)
	}

	// Check for project ID
	if !strings.Contains(result.Body, "my-project") {
		t.Errorf("Expected project ID in body, got: %s", result.Body)
	}

	// Check for duration
	if !strings.Contains(result.Body, "5m") {
		t.Errorf("Expected duration in body, got: %s", result.Body)
	}

	// Check for threshold
	if !strings.Contains(result.Body, "3m") {
		t.Errorf("Expected threshold in body, got: %s", result.Body)
	}

	// Verify alert metadata
	if result.TaskID != "task-456" {
		t.Errorf("Expected TaskID task-456, got: %s", result.TaskID)
	}
	if result.ProjectID != "my-project" {
		t.Errorf("Expected ProjectID my-project, got: %s", result.ProjectID)
	}
}

func TestAlertFormatterFormatCriticalAlert(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-critical",
		protocol.AlertTypeError,
		protocol.AlertSeverityCritical,
		"System Error",
	)
	event.TaskID = "task-err"
	event.Message = "Critical system failure"

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check for critical emoji
	if !strings.Contains(result.Title, EmojiAlertCritical) {
		t.Errorf("Expected critical emoji in title, got: %s", result.Title)
	}

	// Check for error emoji
	if !strings.Contains(result.Title, EmojiError) {
		t.Errorf("Expected error emoji in title, got: %s", result.Title)
	}

	// Check for custom title
	if !strings.Contains(result.Title, "System Error") {
		t.Errorf("Expected custom title in title, got: %s", result.Title)
	}

	// Check for message
	if !strings.Contains(result.Body, "Critical system failure") {
		t.Errorf("Expected message in body, got: %s", result.Body)
	}

	// Verify severity
	if result.Severity != protocol.AlertSeverityCritical {
		t.Errorf("Expected severity critical, got: %s", result.Severity)
	}
}

func TestAlertFormatterFormatWarningAlert(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-warning",
		protocol.AlertTypeResourceLimit,
		protocol.AlertSeverityWarning,
		"Resource Warning",
	)
	event.CurrentValue = 85.0
	event.Threshold = 80.0

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check for warning emoji
	if !strings.Contains(result.Title, EmojiAlertWarning) {
		t.Errorf("Expected warning emoji in title, got: %s", result.Title)
	}

	// Check for resource limit emoji
	if !strings.Contains(result.Title, EmojiResourceLimit) {
		t.Errorf("Expected resource limit emoji in title, got: %s", result.Title)
	}
}

func TestAlertFormatterFormatInfoAlert(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-info",
		protocol.AlertTypeStalled,
		protocol.AlertSeverityInfo,
		"Task Stalled",
	)
	event.TaskID = "task-stalled"
	event.Duration = 60.0

	result := formatter.Format(event)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Check for info emoji
	if !strings.Contains(result.Title, EmojiAlertInfo) {
		t.Errorf("Expected info emoji in title, got: %s", result.Title)
	}

	// Check for stalled emoji
	if !strings.Contains(result.Title, EmojiStalled) {
		t.Errorf("Expected stalled emoji in title, got: %s", result.Title)
	}
}

func TestAlertFormatterFormatWithoutEmojis(t *testing.T) {
	formatter := NewAlertFormatter()
	formatter.SetIncludeEmojis(false)

	event := protocol.NewLongRunningAlertEvent(
		"alert-no-emoji",
		"task-123",
		"project-x",
		120.0,
		60.0,
	)

	result := formatter.Format(event)

	// Should not contain emojis
	if strings.Contains(result.Body, EmojiAlertWarning) {
		t.Errorf("Expected no warning emoji, got: %s", result.Body)
	}

	if strings.Contains(result.Body, EmojiTask) {
		t.Errorf("Expected no task emoji, got: %s", result.Body)
	}

	if strings.Contains(result.Body, EmojiProject) {
		t.Errorf("Expected no project emoji, got: %s", result.Body)
	}

	// But should still contain the text
	if !strings.Contains(result.Body, "Task:") {
		t.Errorf("Expected 'Task:' in body, got: %s", result.Body)
	}
}

func TestAlertFormatterFormatWithoutTaskID(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-no-task",
		protocol.AlertTypeResourceLimit,
		protocol.AlertSeverityWarning,
		"Resource Warning",
	)
	// No TaskID set

	result := formatter.Format(event)

	if strings.Contains(result.Body, "Task:") {
		t.Errorf("Should not include Task line, got: %s", result.Body)
	}
}

func TestAlertFormatterFormatWithoutProjectID(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-no-project",
		protocol.AlertTypeError,
		protocol.AlertSeverityCritical,
		"Error",
	)
	event.TaskID = "task-123"
	// No ProjectID set

	result := formatter.Format(event)

	if strings.Contains(result.Body, "Project:") {
		t.Errorf("Should not include Project line, got: %s", result.Body)
	}
}

func TestAlertFormatterFormatDuration(t *testing.T) {
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
		result := FormatDuration(tt.seconds)
		if result != tt.expected {
			t.Errorf("FormatDuration(%.1f) = %q, want %q", tt.seconds, result, tt.expected)
		}
	}
}

func TestAlertFormatterFormatSummary(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewLongRunningAlertEvent(
		"alert-summary",
		"task-123",
		"project",
		300.0,
		180.0,
	)

	result := formatter.Format(event)

	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	if !strings.Contains(result.Summary, "task-123") {
		t.Errorf("Expected task ID in summary, got: %s", result.Summary)
	}

	if !strings.Contains(result.Summary, "long_running") {
		t.Errorf("Expected alert type in summary, got: %s", result.Summary)
	}

	if !strings.Contains(result.Summary, "warning") {
		t.Errorf("Expected severity in summary, got: %s", result.Summary)
	}
}

func TestAlertFormatterFormatSummaryLongTaskID(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-long-id",
		protocol.AlertTypeError,
		protocol.AlertSeverityCritical,
		"Error",
	)
	event.TaskID = "very-long-task-id-that-exceeds-twenty-characters"

	result := formatter.Format(event)

	// Task ID should be truncated in summary
	if len(result.Summary) > 100 {
		t.Errorf("Summary too long: %s", result.Summary)
	}

	if !strings.Contains(result.Summary, "...") {
		t.Errorf("Expected truncation indicator in summary, got: %s", result.Summary)
	}
}

func TestAlertFormatterFormatSummaryWithoutTaskID(t *testing.T) {
	formatter := NewAlertFormatter()

	event := protocol.NewAlertEvent(
		"alert-no-task",
		protocol.AlertTypeResourceLimit,
		protocol.AlertSeverityInfo,
		"Info Alert",
	)
	// No TaskID

	result := formatter.Format(event)

	// Summary should still work without task ID
	if result.Summary == "" {
		t.Error("Expected non-empty summary")
	}

	if !strings.Contains(result.Summary, "resource_limit") {
		t.Errorf("Expected alert type in summary, got: %s", result.Summary)
	}
}

func TestAlertFormatterFormatLongRunningHelper(t *testing.T) {
	formatter := NewAlertFormatter()

	result := formatter.FormatLongRunning("task-1", "proj-1", 600.0, 300.0)

	if !strings.Contains(result.Title, EmojiLongRunning) {
		t.Errorf("Expected long running emoji in title, got: %s", result.Title)
	}

	if result.TaskID != "task-1" {
		t.Errorf("Expected TaskID task-1, got: %s", result.TaskID)
	}

	if result.ProjectID != "proj-1" {
		t.Errorf("Expected ProjectID proj-1, got: %s", result.ProjectID)
	}
}

func TestAlertFormatterFormatErrorHelper(t *testing.T) {
	formatter := NewAlertFormatter()

	result := formatter.FormatError("task-2", "proj-2", "Build Error", "Compilation failed", protocol.AlertSeverityCritical)

	if !strings.Contains(result.Title, EmojiError) {
		t.Errorf("Expected error emoji in title, got: %s", result.Title)
	}

	if !strings.Contains(result.Title, "Build Error") {
		t.Errorf("Expected title in title, got: %s", result.Title)
	}

	if !strings.Contains(result.Body, "Compilation failed") {
		t.Errorf("Expected message in body, got: %s", result.Body)
	}

	if result.Severity != protocol.AlertSeverityCritical {
		t.Errorf("Expected severity critical, got: %s", result.Severity)
	}
}

func TestGetSeverityEmoji(t *testing.T) {
	tests := []struct {
		severity protocol.AlertSeverity
		expected string
	}{
		{protocol.AlertSeverityCritical, EmojiAlertCritical},
		{protocol.AlertSeverityWarning, EmojiAlertWarning},
		{protocol.AlertSeverityInfo, EmojiAlertInfo},
		{protocol.AlertSeverity("unknown"), EmojiAlertInfo},
	}

	for _, tt := range tests {
		result := GetSeverityEmoji(tt.severity)
		if result != tt.expected {
			t.Errorf("GetSeverityEmoji(%q) = %q, want %q", tt.severity, result, tt.expected)
		}
	}
}

func TestGetAlertTypeEmoji(t *testing.T) {
	tests := []struct {
		alertType protocol.AlertType
		expected  string
	}{
		{protocol.AlertTypeLongRunning, EmojiLongRunning},
		{protocol.AlertTypeStalled, EmojiStalled},
		{protocol.AlertTypeResourceLimit, EmojiResourceLimit},
		{protocol.AlertTypeError, EmojiError},
		{protocol.AlertType("unknown"), EmojiAlertInfo},
	}

	for _, tt := range tests {
		result := GetAlertTypeEmoji(tt.alertType)
		if result != tt.expected {
			t.Errorf("GetAlertTypeEmoji(%q) = %q, want %q", tt.alertType, result, tt.expected)
		}
	}
}

func TestFormattedAlertStructure(t *testing.T) {
	fa := FormattedAlert{
		Title:     "Test Title",
		Body:      "Test Body",
		Severity:  protocol.AlertSeverityWarning,
		Summary:   "Test Summary",
		TaskID:    "task-123",
		ProjectID: "proj-456",
	}

	if fa.Title != "Test Title" {
		t.Error("Title not set correctly")
	}
	if fa.Body != "Test Body" {
		t.Error("Body not set correctly")
	}
	if fa.Severity != protocol.AlertSeverityWarning {
		t.Error("Severity not set correctly")
	}
	if fa.Summary != "Test Summary" {
		t.Error("Summary not set correctly")
	}
	if fa.TaskID != "task-123" {
		t.Error("TaskID not set correctly")
	}
	if fa.ProjectID != "proj-456" {
		t.Error("ProjectID not set correctly")
	}
}

func TestAlertEmojiConstants(t *testing.T) {
	// Verify emoji constants are non-empty
	emojis := []string{
		EmojiAlertCritical,
		EmojiAlertWarning,
		EmojiAlertInfo,
		EmojiLongRunning,
		EmojiStalled,
		EmojiResourceLimit,
		EmojiError,
	}

	for i, emoji := range emojis {
		if emoji == "" {
			t.Errorf("Emoji constant %d is empty", i)
		}
	}

	// Check that severity emojis are distinct
	if EmojiAlertCritical == EmojiAlertWarning {
		t.Error("Critical and Warning emojis should be different")
	}
	if EmojiAlertWarning == EmojiAlertInfo {
		t.Error("Warning and Info emojis should be different")
	}
}

func TestAlertFormatterTruncation(t *testing.T) {
	formatter := NewAlertFormatter()
	formatter.SetMaxMessageLength(150)

	event := protocol.NewAlertEvent(
		"alert-truncate",
		protocol.AlertTypeError,
		protocol.AlertSeverityCritical,
		"This is a very long title that should be included",
	)
	event.TaskID = "task-truncate"
	event.ProjectID = "project-truncate"
	event.Message = "This is a very long message that will definitely need to be truncated because it exceeds our maximum message length limit and keeps going on and on"

	result := formatter.Format(event)

	if len(result.Body) > 200 {
		t.Errorf("Body should be truncated, length: %d", len(result.Body))
	}

	if !strings.Contains(result.Body, "truncated") {
		t.Errorf("Expected truncation notice in body, got: %s", result.Body)
	}
}

func TestAlertFormatterDefaultTitle(t *testing.T) {
	formatter := NewAlertFormatter()

	// Test default titles for each alert type
	tests := []struct {
		alertType    protocol.AlertType
		expectedText string
	}{
		{protocol.AlertTypeLongRunning, "Long Running Task"},
		{protocol.AlertTypeStalled, "Task Stalled"},
		{protocol.AlertTypeResourceLimit, "Resource Limit"},
		{protocol.AlertTypeError, "Error Alert"},
	}

	for _, tt := range tests {
		event := protocol.NewAlertEvent(
			"alert-default-title",
			tt.alertType,
			protocol.AlertSeverityWarning,
			"", // Empty title to use default
		)

		result := formatter.Format(event)

		if !strings.Contains(result.Title, tt.expectedText) {
			t.Errorf("Expected default title '%s' for %s, got: %s", tt.expectedText, tt.alertType, result.Title)
		}
	}
}
