package telegram

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/protocol"
)

// Telegram message size limits
const (
	// MaxTelegramMessageLength is the maximum length for a Telegram text message.
	// Telegram's actual limit is 4096 characters, but we use a lower value
	// to account for formatting overhead and provide a buffer.
	MaxTelegramMessageLength = 4000

	// LogFileThreshold is the minimum size (in bytes) at which logs are sent as a file.
	// If the formatted log content exceeds this, it will be sent as a document attachment.
	LogFileThreshold = 3500
)

// LogOutputMode represents how log content should be delivered.
type LogOutputMode int

const (
	// LogOutputText indicates logs should be sent as a text message.
	LogOutputText LogOutputMode = iota

	// LogOutputFile indicates logs should be sent as a file attachment.
	LogOutputFile
)

// FormattedLogs represents the result of formatting log entries.
type FormattedLogs struct {
	// Mode indicates whether this should be sent as text or file.
	Mode LogOutputMode

	// TextContent contains the formatted text for text mode.
	// In file mode, this contains a brief summary message.
	TextContent string

	// FileContent contains the full log content for file attachments.
	// Only populated when Mode is LogOutputFile.
	FileContent []byte

	// FileName is the suggested filename for file attachments.
	// Only populated when Mode is LogOutputFile.
	FileName string

	// EntryCount is the number of log entries formatted.
	EntryCount int

	// Truncated indicates if entries were truncated for text output.
	Truncated bool
}

// LogFormatter formats log entries for Telegram delivery.
type LogFormatter struct {
	// maxTextLength is the maximum length for text messages.
	maxTextLength int

	// fileThreshold is the size threshold for switching to file mode.
	fileThreshold int
}

// NewLogFormatter creates a new LogFormatter with default settings.
func NewLogFormatter() *LogFormatter {
	return &LogFormatter{
		maxTextLength: MaxTelegramMessageLength,
		fileThreshold: LogFileThreshold,
	}
}

// SetMaxTextLength sets the maximum text message length.
func (f *LogFormatter) SetMaxTextLength(length int) {
	if length > 0 {
		f.maxTextLength = length
	}
}

// SetFileThreshold sets the threshold for switching to file mode.
func (f *LogFormatter) SetFileThreshold(threshold int) {
	if threshold > 0 {
		f.fileThreshold = threshold
	}
}

// Format formats a LogsCommandResult for Telegram delivery.
// It automatically chooses between text and file modes based on content size.
func (f *LogFormatter) Format(result *LogsCommandResult) *FormattedLogs {
	if result == nil {
		return &FormattedLogs{
			Mode:        LogOutputText,
			TextContent: "No log data available.",
		}
	}

	// Handle error case
	if result.Error != "" {
		return f.formatError(result)
	}

	// Handle empty entries
	if len(result.Entries) == 0 {
		return f.formatEmpty(result)
	}

	// Format full content to determine size
	fullContent := f.formatFullContent(result)
	contentSize := len(fullContent)

	// Decide output mode based on content size
	if contentSize > f.fileThreshold {
		return f.formatAsFile(result, fullContent)
	}

	return f.formatAsText(result, fullContent)
}

// formatError formats an error result.
func (f *LogFormatter) formatError(result *LogsCommandResult) *FormattedLogs {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Logs for task: %s\n", result.TaskID))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	sb.WriteString(fmt.Sprintf("\nError: %s", result.Error))

	return &FormattedLogs{
		Mode:        LogOutputText,
		TextContent: sb.String(),
	}
}

// formatEmpty formats a result with no entries.
func (f *LogFormatter) formatEmpty(result *LogsCommandResult) *FormattedLogs {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Logs for task: %s\n", result.TaskID))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	sb.WriteString("\nNo log entries found.")

	return &FormattedLogs{
		Mode:        LogOutputText,
		TextContent: sb.String(),
	}
}

// formatFullContent formats all log entries as plain text.
func (f *LogFormatter) formatFullContent(result *LogsCommandResult) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Logs for task: %s\n", result.TaskID))

	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	sb.WriteString(fmt.Sprintf("\n%d log entries:\n", len(result.Entries)))
	sb.WriteString(strings.Repeat("-", 50))
	sb.WriteString("\n")

	for _, entry := range result.Entries {
		sb.WriteString(f.formatEntry(entry))
		sb.WriteString("\n")
	}

	if result.HasMore {
		sb.WriteString("\n... (more entries available)")
	}

	return sb.String()
}

// formatEntry formats a single log entry.
func (f *LogFormatter) formatEntry(entry protocol.LogEntry) string {
	// Format timestamp (show full timestamp for file, short for messages)
	timestamp := entry.Timestamp
	if len(timestamp) > 19 {
		// Extract date and time: YYYY-MM-DD HH:MM:SS
		timestamp = timestamp[:10] + " " + timestamp[11:19]
	}

	// Level indicator
	level := strings.ToUpper(string(entry.Level))
	if level == "" {
		level = "INFO"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, level, entry.Message)
}

// formatAsText formats the result as a text message.
func (f *LogFormatter) formatAsText(result *LogsCommandResult, fullContent string) *FormattedLogs {
	// Check if we need to truncate
	if len(fullContent) <= f.maxTextLength {
		return &FormattedLogs{
			Mode:        LogOutputText,
			TextContent: fullContent,
			EntryCount:  len(result.Entries),
		}
	}

	// Truncate and add indicator
	truncated := fullContent[:f.maxTextLength-50]
	// Find last newline to avoid cutting mid-line
	lastNewline := strings.LastIndex(truncated, "\n")
	if lastNewline > f.maxTextLength/2 {
		truncated = truncated[:lastNewline]
	}
	truncated += "\n\n... (output truncated, use file mode for full logs)"

	return &FormattedLogs{
		Mode:        LogOutputText,
		TextContent: truncated,
		EntryCount:  len(result.Entries),
		Truncated:   true,
	}
}

// formatAsFile formats the result as a file attachment.
func (f *LogFormatter) formatAsFile(result *LogsCommandResult, fullContent string) *FormattedLogs {
	// Create detailed file content
	var fileBuffer bytes.Buffer

	// Header
	fileBuffer.WriteString("=" + strings.Repeat("=", 59) + "\n")
	fileBuffer.WriteString(fmt.Sprintf("  TASK LOGS: %s\n", result.TaskID))
	if result.ProjectID != "" {
		fileBuffer.WriteString(fmt.Sprintf("  PROJECT: %s\n", result.ProjectID))
	}
	fileBuffer.WriteString(fmt.Sprintf("  ENTRIES: %d\n", len(result.Entries)))
	fileBuffer.WriteString("=" + strings.Repeat("=", 59) + "\n\n")

	// Log entries with detailed formatting
	for i, entry := range result.Entries {
		fileBuffer.WriteString(fmt.Sprintf("--- Entry %d ---\n", i+1))
		fileBuffer.WriteString(fmt.Sprintf("Timestamp: %s\n", entry.Timestamp))
		fileBuffer.WriteString(fmt.Sprintf("Level:     %s\n", strings.ToUpper(string(entry.Level))))
		if entry.Source != "" {
			fileBuffer.WriteString(fmt.Sprintf("Source:    %s\n", entry.Source))
		}
		fileBuffer.WriteString(fmt.Sprintf("Message:\n%s\n\n", entry.Message))
	}

	if result.HasMore {
		fileBuffer.WriteString("\n[Note: More entries available. Request with pagination to see all.]\n")
	}

	// Summary message for chat
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Logs for task: %s\n", result.TaskID))
	if result.ProjectID != "" {
		summary.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}
	summary.WriteString(fmt.Sprintf("\n%d log entries (attached as file due to size)", len(result.Entries)))

	// Generate filename
	fileName := fmt.Sprintf("logs_%s.txt", sanitizeFileName(result.TaskID))

	return &FormattedLogs{
		Mode:        LogOutputFile,
		TextContent: summary.String(),
		FileContent: fileBuffer.Bytes(),
		FileName:    fileName,
		EntryCount:  len(result.Entries),
	}
}

// FormatAsTextBlock formats logs as a code/preformatted text block for Telegram.
// This is useful when you want to display logs in a monospace format within
// a regular message.
func (f *LogFormatter) FormatAsTextBlock(result *LogsCommandResult) *FormattedLogs {
	if result == nil || result.Error != "" || len(result.Entries) == 0 {
		return f.Format(result)
	}

	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Logs for task: %s\n", result.TaskID))
	if result.ProjectID != "" {
		sb.WriteString(fmt.Sprintf("Project: %s\n", result.ProjectID))
	}

	sb.WriteString("\n```\n")

	entriesContent := f.formatEntriesOnly(result.Entries)

	// Check if we need to truncate within the code block
	maxBlockSize := f.maxTextLength - len(sb.String()) - 10 // Reserve space for closing
	if len(entriesContent) > maxBlockSize {
		entriesContent = entriesContent[:maxBlockSize-50]
		lastNewline := strings.LastIndex(entriesContent, "\n")
		if lastNewline > maxBlockSize/2 {
			entriesContent = entriesContent[:lastNewline]
		}
		entriesContent += "\n... (truncated)"
	}

	sb.WriteString(entriesContent)
	sb.WriteString("\n```")

	if result.HasMore {
		sb.WriteString("\n\n... (more entries available)")
	}

	return &FormattedLogs{
		Mode:        LogOutputText,
		TextContent: sb.String(),
		EntryCount:  len(result.Entries),
		Truncated:   len(entriesContent) < len(f.formatEntriesOnly(result.Entries)),
	}
}

// formatEntriesOnly formats just the log entries without header.
func (f *LogFormatter) formatEntriesOnly(entries []protocol.LogEntry) string {
	var sb strings.Builder

	for _, entry := range entries {
		timestamp := entry.Timestamp
		if len(timestamp) > 19 {
			timestamp = timestamp[11:19] // HH:MM:SS only
		}

		levelIcon := getLevelIcon(entry.Level)
		sb.WriteString(fmt.Sprintf("%s [%s] %s\n", levelIcon, timestamp, entry.Message))
	}

	return sb.String()
}

// getLevelIcon returns an icon character for the log level.
func getLevelIcon(level protocol.LogLevel) string {
	switch level {
	case protocol.LogLevelDebug:
		return "D"
	case protocol.LogLevelInfo:
		return "I"
	case protocol.LogLevelWarn:
		return "W"
	case protocol.LogLevelError:
		return "E"
	default:
		return "-"
	}
}

// sanitizeFileName removes or replaces characters that are invalid in filenames.
func sanitizeFileName(name string) string {
	// Replace common invalid characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	sanitized := replacer.Replace(name)

	// Limit length
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}

	return sanitized
}
