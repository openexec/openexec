package telegram

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/protocol"
)

func TestNewLogFormatter(t *testing.T) {
	formatter := NewLogFormatter()

	if formatter == nil {
		t.Fatal("Expected non-nil formatter")
	}

	if formatter.maxTextLength != MaxTelegramMessageLength {
		t.Errorf("Expected maxTextLength %d, got %d", MaxTelegramMessageLength, formatter.maxTextLength)
	}

	if formatter.fileThreshold != LogFileThreshold {
		t.Errorf("Expected fileThreshold %d, got %d", LogFileThreshold, formatter.fileThreshold)
	}
}

func TestLogFormatterSetMaxTextLength(t *testing.T) {
	formatter := NewLogFormatter()

	formatter.SetMaxTextLength(1000)
	if formatter.maxTextLength != 1000 {
		t.Errorf("Expected maxTextLength 1000, got %d", formatter.maxTextLength)
	}

	// Invalid value should be ignored
	formatter.SetMaxTextLength(-1)
	if formatter.maxTextLength != 1000 {
		t.Errorf("Expected maxTextLength to remain 1000, got %d", formatter.maxTextLength)
	}

	formatter.SetMaxTextLength(0)
	if formatter.maxTextLength != 1000 {
		t.Errorf("Expected maxTextLength to remain 1000, got %d", formatter.maxTextLength)
	}
}

func TestLogFormatterSetFileThreshold(t *testing.T) {
	formatter := NewLogFormatter()

	formatter.SetFileThreshold(500)
	if formatter.fileThreshold != 500 {
		t.Errorf("Expected fileThreshold 500, got %d", formatter.fileThreshold)
	}

	// Invalid value should be ignored
	formatter.SetFileThreshold(-1)
	if formatter.fileThreshold != 500 {
		t.Errorf("Expected fileThreshold to remain 500, got %d", formatter.fileThreshold)
	}

	formatter.SetFileThreshold(0)
	if formatter.fileThreshold != 500 {
		t.Errorf("Expected fileThreshold to remain 500, got %d", formatter.fileThreshold)
	}
}

func TestFormatNilResult(t *testing.T) {
	formatter := NewLogFormatter()

	result := formatter.Format(nil)

	if result.Mode != LogOutputText {
		t.Errorf("Expected LogOutputText mode, got %d", result.Mode)
	}

	if !strings.Contains(result.TextContent, "No log data available") {
		t.Errorf("Expected 'No log data available' in text, got: %s", result.TextContent)
	}
}

func TestFormatErrorResult(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:    "task-123",
		ProjectID: "project-a",
		Error:     "Something went wrong",
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputText {
		t.Errorf("Expected LogOutputText mode, got %d", result.Mode)
	}

	if !strings.Contains(result.TextContent, "task-123") {
		t.Errorf("Expected task ID in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "project-a") {
		t.Errorf("Expected project ID in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "Something went wrong") {
		t.Errorf("Expected error message in text, got: %s", result.TextContent)
	}
}

func TestFormatEmptyEntries(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:  "task-456",
		Entries: []protocol.LogEntry{},
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputText {
		t.Errorf("Expected LogOutputText mode, got %d", result.Mode)
	}

	if !strings.Contains(result.TextContent, "No log entries found") {
		t.Errorf("Expected 'No log entries found' in text, got: %s", result.TextContent)
	}
}

func TestFormatSmallPayloadAsText(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:    "task-789",
		ProjectID: "my-project",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Task started",
			},
			{
				Timestamp: "2024-01-15T10:30:05Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Task completed",
			},
		},
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputText {
		t.Errorf("Expected LogOutputText mode, got %d", result.Mode)
	}

	if result.EntryCount != 2 {
		t.Errorf("Expected EntryCount 2, got %d", result.EntryCount)
	}

	if !strings.Contains(result.TextContent, "task-789") {
		t.Errorf("Expected task ID in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "my-project") {
		t.Errorf("Expected project ID in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "Task started") {
		t.Errorf("Expected log message in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "Task completed") {
		t.Errorf("Expected log message in text, got: %s", result.TextContent)
	}
}

func TestFormatLargePayloadAsFile(t *testing.T) {
	formatter := NewLogFormatter()
	formatter.SetFileThreshold(100) // Set low threshold for testing

	// Create a result with content that exceeds threshold
	input := &LogsCommandResult{
		TaskID:    "task-large",
		ProjectID: "big-project",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "This is a long log message that will exceed our threshold",
			},
			{
				Timestamp: "2024-01-15T10:30:01Z",
				Level:     protocol.LogLevelWarn,
				Message:   "Another long message to push us over the limit for file mode",
			},
		},
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputFile {
		t.Errorf("Expected LogOutputFile mode, got %d", result.Mode)
	}

	if result.EntryCount != 2 {
		t.Errorf("Expected EntryCount 2, got %d", result.EntryCount)
	}

	if len(result.FileContent) == 0 {
		t.Error("Expected non-empty FileContent")
	}

	if result.FileName == "" {
		t.Error("Expected non-empty FileName")
	}

	if !strings.Contains(result.FileName, "task-large") {
		t.Errorf("Expected task ID in filename, got: %s", result.FileName)
	}

	if !strings.HasSuffix(result.FileName, ".txt") {
		t.Errorf("Expected .txt extension, got: %s", result.FileName)
	}

	// Check summary message
	if !strings.Contains(result.TextContent, "attached as file") {
		t.Errorf("Expected file attachment notice in text, got: %s", result.TextContent)
	}

	// Check file content
	fileStr := string(result.FileContent)
	if !strings.Contains(fileStr, "TASK LOGS: task-large") {
		t.Errorf("Expected task header in file, got: %s", fileStr)
	}

	if !strings.Contains(fileStr, "PROJECT: big-project") {
		t.Errorf("Expected project in file, got: %s", fileStr)
	}
}

func TestFormatWithHasMore(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:  "task-paginated",
		HasMore: true,
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "First entry",
			},
		},
	}

	result := formatter.Format(input)

	if !strings.Contains(result.TextContent, "more entries available") {
		t.Errorf("Expected 'more entries available' in text, got: %s", result.TextContent)
	}
}

func TestFormatTruncation(t *testing.T) {
	formatter := NewLogFormatter()
	formatter.SetMaxTextLength(200)
	formatter.SetFileThreshold(5000) // High threshold to force text mode

	// Create long content that needs truncation
	input := &LogsCommandResult{
		TaskID: "task-truncate",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "This is a very long log message that will need to be truncated because it exceeds our maximum text length limit",
			},
			{
				Timestamp: "2024-01-15T10:30:01Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Another long message to ensure we exceed the limit",
			},
		},
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputText {
		t.Errorf("Expected LogOutputText mode, got %d", result.Mode)
	}

	if !result.Truncated {
		t.Error("Expected Truncated to be true")
	}

	if !strings.Contains(result.TextContent, "truncated") {
		t.Errorf("Expected truncation notice in text, got: %s", result.TextContent)
	}

	if len(result.TextContent) > 250 {
		t.Errorf("Expected text to be truncated, length: %d", len(result.TextContent))
	}
}

func TestFormatAllLogLevels(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID: "task-levels",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelDebug,
				Message:   "Debug message",
			},
			{
				Timestamp: "2024-01-15T10:30:01Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Info message",
			},
			{
				Timestamp: "2024-01-15T10:30:02Z",
				Level:     protocol.LogLevelWarn,
				Message:   "Warning message",
			},
			{
				Timestamp: "2024-01-15T10:30:03Z",
				Level:     protocol.LogLevelError,
				Message:   "Error message",
			},
		},
	}

	result := formatter.Format(input)

	if !strings.Contains(result.TextContent, "DEBUG") {
		t.Errorf("Expected DEBUG level in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "INFO") {
		t.Errorf("Expected INFO level in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "WARN") {
		t.Errorf("Expected WARN level in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "ERROR") {
		t.Errorf("Expected ERROR level in text, got: %s", result.TextContent)
	}
}

func TestFormatAsTextBlock(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:    "task-block",
		ProjectID: "project-x",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Test entry",
			},
		},
	}

	result := formatter.FormatAsTextBlock(input)

	if result.Mode != LogOutputText {
		t.Errorf("Expected LogOutputText mode, got %d", result.Mode)
	}

	if !strings.Contains(result.TextContent, "```") {
		t.Errorf("Expected code block markers in text, got: %s", result.TextContent)
	}

	if !strings.Contains(result.TextContent, "Test entry") {
		t.Errorf("Expected log message in text, got: %s", result.TextContent)
	}
}

func TestFormatAsTextBlockWithError(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID: "task-err",
		Error:  "Failed to fetch logs",
	}

	result := formatter.FormatAsTextBlock(input)

	// Should fall back to regular format for errors
	if !strings.Contains(result.TextContent, "Failed to fetch logs") {
		t.Errorf("Expected error in text, got: %s", result.TextContent)
	}
}

func TestFormatAsTextBlockWithEmptyEntries(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:  "task-empty",
		Entries: []protocol.LogEntry{},
	}

	result := formatter.FormatAsTextBlock(input)

	// Should fall back to regular format for empty entries
	if !strings.Contains(result.TextContent, "No log entries found") {
		t.Errorf("Expected 'No log entries found' in text, got: %s", result.TextContent)
	}
}

func TestFormatAsTextBlockTruncation(t *testing.T) {
	formatter := NewLogFormatter()
	formatter.SetMaxTextLength(150)

	input := &LogsCommandResult{
		TaskID: "task-truncate-block",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "This is a very long message that will need truncation",
			},
			{
				Timestamp: "2024-01-15T10:30:01Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Another message to push over the limit",
			},
		},
	}

	result := formatter.FormatAsTextBlock(input)

	if !strings.Contains(result.TextContent, "truncated") {
		t.Errorf("Expected truncation notice in text, got: %s", result.TextContent)
	}

	// Should still have code block markers
	if !strings.Contains(result.TextContent, "```") {
		t.Errorf("Expected code block markers in text, got: %s", result.TextContent)
	}
}

func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple-task", "simple-task"},
		{"task/with/slashes", "task_with_slashes"},
		{"task:colon", "task_colon"},
		{"task*star", "task_star"},
		{"task?question", "task_question"},
		{"task\"quote", "task_quote"},
		{"task<less>greater", "task_less_greater"},
		{"task|pipe", "task_pipe"},
		{"task with spaces", "task_with_spaces"},
		{"task\\backslash", "task_backslash"},
		{"very-long-task-name-that-exceeds-fifty-characters-limit-here", "very-long-task-name-that-exceeds-fifty-characters-"},
	}

	for _, tt := range tests {
		result := sanitizeFileName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetLevelIcon(t *testing.T) {
	tests := []struct {
		level    protocol.LogLevel
		expected string
	}{
		{protocol.LogLevelDebug, "D"},
		{protocol.LogLevelInfo, "I"},
		{protocol.LogLevelWarn, "W"},
		{protocol.LogLevelError, "E"},
		{protocol.LogLevel("unknown"), "-"},
		{protocol.LogLevel(""), "-"},
	}

	for _, tt := range tests {
		result := getLevelIcon(tt.level)
		if result != tt.expected {
			t.Errorf("getLevelIcon(%q) = %q, want %q", tt.level, result, tt.expected)
		}
	}
}

func TestFormatEntryTimestampHandling(t *testing.T) {
	formatter := NewLogFormatter()

	// Test with full RFC3339 timestamp
	entry1 := protocol.LogEntry{
		Timestamp: "2024-01-15T10:30:45Z",
		Level:     protocol.LogLevelInfo,
		Message:   "Test",
	}

	result1 := formatter.formatEntry(entry1)
	if !strings.Contains(result1, "2024-01-15 10:30:45") {
		t.Errorf("Expected formatted timestamp in entry, got: %s", result1)
	}

	// Test with short timestamp
	entry2 := protocol.LogEntry{
		Timestamp: "10:30:45",
		Level:     protocol.LogLevelInfo,
		Message:   "Test",
	}

	result2 := formatter.formatEntry(entry2)
	if !strings.Contains(result2, "10:30:45") {
		t.Errorf("Expected short timestamp in entry, got: %s", result2)
	}

	// Test with empty level
	entry3 := protocol.LogEntry{
		Timestamp: "2024-01-15T10:30:45Z",
		Level:     "",
		Message:   "Test",
	}

	result3 := formatter.formatEntry(entry3)
	if !strings.Contains(result3, "INFO") {
		t.Errorf("Expected default INFO level, got: %s", result3)
	}
}

func TestFormatFileContentWithSource(t *testing.T) {
	formatter := NewLogFormatter()
	formatter.SetFileThreshold(100) // Low threshold to force file mode

	input := &LogsCommandResult{
		TaskID: "task-source",
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Message with source",
				Source:    "agent",
			},
		},
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputFile {
		t.Errorf("Expected LogOutputFile mode, got %d", result.Mode)
	}

	fileStr := string(result.FileContent)
	if !strings.Contains(fileStr, "Source:    agent") {
		t.Errorf("Expected source in file content, got: %s", fileStr)
	}
}

func TestFormatFileContentWithHasMore(t *testing.T) {
	formatter := NewLogFormatter()
	formatter.SetFileThreshold(100) // Low threshold to force file mode

	input := &LogsCommandResult{
		TaskID:  "task-more",
		HasMore: true,
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Entry content here to exceed threshold limit",
			},
		},
	}

	result := formatter.Format(input)

	if result.Mode != LogOutputFile {
		t.Errorf("Expected LogOutputFile mode, got %d", result.Mode)
	}

	fileStr := string(result.FileContent)
	if !strings.Contains(fileStr, "More entries available") {
		t.Errorf("Expected pagination note in file content, got: %s", fileStr)
	}
}

func TestLogOutputModeConstants(t *testing.T) {
	// Verify constant values are distinct
	if LogOutputText == LogOutputFile {
		t.Error("LogOutputText and LogOutputFile should be different")
	}
}

func TestFormattedLogsStructure(t *testing.T) {
	// Test that FormattedLogs has all expected fields
	fl := FormattedLogs{
		Mode:        LogOutputFile,
		TextContent: "summary",
		FileContent: []byte("file content"),
		FileName:    "test.txt",
		EntryCount:  5,
		Truncated:   true,
	}

	if fl.Mode != LogOutputFile {
		t.Error("Mode not set correctly")
	}
	if fl.TextContent != "summary" {
		t.Error("TextContent not set correctly")
	}
	if string(fl.FileContent) != "file content" {
		t.Error("FileContent not set correctly")
	}
	if fl.FileName != "test.txt" {
		t.Error("FileName not set correctly")
	}
	if fl.EntryCount != 5 {
		t.Error("EntryCount not set correctly")
	}
	if !fl.Truncated {
		t.Error("Truncated not set correctly")
	}
}

func TestFormatWithNoProjectID(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID:    "task-no-project",
		ProjectID: "", // Empty project ID
		Entries: []protocol.LogEntry{
			{
				Timestamp: "2024-01-15T10:30:00Z",
				Level:     protocol.LogLevelInfo,
				Message:   "Test message",
			},
		},
	}

	result := formatter.Format(input)

	// Should not contain "Project:" line
	if strings.Contains(result.TextContent, "Project:") {
		t.Errorf("Should not include Project line for empty project ID, got: %s", result.TextContent)
	}
}

func TestFormatErrorWithNoProjectID(t *testing.T) {
	formatter := NewLogFormatter()

	input := &LogsCommandResult{
		TaskID: "task-err",
		Error:  "Test error",
	}

	result := formatter.Format(input)

	if strings.Contains(result.TextContent, "Project:") {
		t.Errorf("Should not include Project line for empty project ID, got: %s", result.TextContent)
	}
}
