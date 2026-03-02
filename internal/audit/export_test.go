package audit

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExportFormat_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		format ExportFormat
		want   bool
	}{
		{"valid json", FormatJSON, true},
		{"valid jsonl", FormatJSONLines, true},
		{"valid csv", FormatCSV, true},
		{"invalid empty", ExportFormat(""), false},
		{"invalid unknown", ExportFormat("xml"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.format.IsValid(); got != tt.want {
				t.Errorf("ExportFormat.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExportFormat_String(t *testing.T) {
	if got := FormatJSON.String(); got != "json" {
		t.Errorf("FormatJSON.String() = %v, want json", got)
	}
	if got := FormatJSONLines.String(); got != "jsonl" {
		t.Errorf("FormatJSONLines.String() = %v, want jsonl", got)
	}
	if got := FormatCSV.String(); got != "csv" {
		t.Errorf("FormatCSV.String() = %v, want csv", got)
	}
}

func TestDefaultExportOptions(t *testing.T) {
	opts := DefaultExportOptions()

	if opts.Format != FormatJSON {
		t.Errorf("DefaultExportOptions().Format = %v, want %v", opts.Format, FormatJSON)
	}
	if opts.Filter == nil {
		t.Error("DefaultExportOptions().Filter should not be nil")
	}
	if !opts.IncludeMetadata {
		t.Error("DefaultExportOptions().IncludeMetadata should be true")
	}
	if opts.PrettyPrint {
		t.Error("DefaultExportOptions().PrettyPrint should be false")
	}
}

func TestNewExporter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	if exporter == nil {
		t.Error("NewExporter() should not return nil")
	}
	if exporter.logger != logger {
		t.Error("NewExporter().logger should be set to the provided logger")
	}
}

func TestExporter_Export_JSON_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	ctx := context.Background()

	var buf bytes.Buffer
	opts := &ExportOptions{Format: FormatJSON}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != 0 {
		t.Errorf("Export() EntriesExported = %d, want 0", result.EntriesExported)
	}
	if result.Format != FormatJSON {
		t.Errorf("Export() Format = %v, want %v", result.Format, FormatJSON)
	}

	// Verify output is valid JSON (empty array)
	var entries []ExportEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("JSON contains %d entries, want 0", len(entries))
	}
}

func TestExporter_Export_JSON_WithEntries(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create some test entries
	metadata := map[string]string{"key": "value"}

	builder, _ := NewEntry(EventSessionCreated, "user-123", "user")
	entry1, _ := builder.
		WithSession("session-456").
		WithProject("/path/to/project").
		WithMetadata(metadata).
		Build()
	if err := logger.Log(ctx, entry1); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	builder2, _ := NewEntry(EventLLMResponseReceived, "system", "system")
	entry2, _ := builder2.
		WithProvider("openai", "gpt-4").
		WithTokens(100, 50).
		WithCost(0.01).
		WithDuration(500).
		WithSuccess(true).
		Build()
	if err := logger.Log(ctx, entry2); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	exporter := NewExporter(logger)

	var buf bytes.Buffer
	opts := &ExportOptions{
		Format:          FormatJSON,
		IncludeMetadata: true,
	}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != 2 {
		t.Errorf("Export() EntriesExported = %d, want 2", result.EntriesExported)
	}
	if result.BytesWritten != int64(buf.Len()) {
		t.Errorf("Export() BytesWritten = %d, want %d", result.BytesWritten, buf.Len())
	}

	// Verify output is valid JSON
	var entries []ExportEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Errorf("Output is not valid JSON: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("JSON contains %d entries, want 2", len(entries))
	}

	// Check first entry has metadata
	foundSession := false
	for _, e := range entries {
		if e.EventType == "session.created" {
			foundSession = true
			if e.SessionID != "session-456" {
				t.Errorf("Entry SessionID = %v, want session-456", e.SessionID)
			}
			if e.Metadata == "" {
				t.Error("Entry Metadata should not be empty")
			}
		}
	}
	if !foundSession {
		t.Error("Did not find session.created entry in export")
	}
}

func TestExporter_Export_JSON_PrettyPrint(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	builder, _ := NewEntry(EventSessionCreated, "user", "user")
	entry, _ := builder.Build()
	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	exporter := NewExporter(logger)

	// Without pretty print
	var bufCompact bytes.Buffer
	opts := &ExportOptions{Format: FormatJSON, PrettyPrint: false}
	if _, err := exporter.Export(ctx, &bufCompact, opts); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	// With pretty print
	var bufPretty bytes.Buffer
	opts.PrettyPrint = true
	if _, err := exporter.Export(ctx, &bufPretty, opts); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	// Pretty printed should be longer (has indentation)
	if bufPretty.Len() <= bufCompact.Len() {
		t.Error("Pretty printed output should be longer than compact")
	}

	// Pretty printed should contain indentation
	if !strings.Contains(bufPretty.String(), "  ") {
		t.Error("Pretty printed output should contain indentation")
	}
}

func TestExporter_Export_JSONLines(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create some test entries
	for i := 0; i < 3; i++ {
		builder, _ := NewEntry(EventSessionCreated, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	exporter := NewExporter(logger)

	var buf bytes.Buffer
	opts := &ExportOptions{Format: FormatJSONLines}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != 3 {
		t.Errorf("Export() EntriesExported = %d, want 3", result.EntriesExported)
	}
	if result.Format != FormatJSONLines {
		t.Errorf("Export() Format = %v, want %v", result.Format, FormatJSONLines)
	}

	// Verify each line is valid JSON
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("Output has %d lines, want 3", len(lines))
	}

	for i, line := range lines {
		var entry ExportEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Errorf("Line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestExporter_Export_CSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create a test entry with all fields
	builder, _ := NewEntry(EventToolCallCompleted, "user-123", "user")
	entry, _ := builder.
		WithSession("session-456").
		WithMessage("message-789").
		WithToolCall("toolcall-012").
		WithProject("/path/to/project").
		WithProvider("openai", "gpt-4").
		WithTokens(100, 50).
		WithCost(0.005).
		WithDuration(150).
		WithSuccess(true).
		WithIteration(3).
		WithClientInfo("127.0.0.1", "Mozilla/5.0").
		Build()
	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	exporter := NewExporter(logger)

	var buf bytes.Buffer
	opts := &ExportOptions{
		Format:          FormatCSV,
		IncludeMetadata: true,
	}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != 1 {
		t.Errorf("Export() EntriesExported = %d, want 1", result.EntriesExported)
	}
	if result.Format != FormatCSV {
		t.Errorf("Export() Format = %v, want %v", result.Format, FormatCSV)
	}

	// Parse CSV
	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}

	// Should have header + 1 data row
	if len(records) != 2 {
		t.Errorf("CSV has %d rows, want 2", len(records))
	}

	// Check header
	header := records[0]
	if header[0] != "id" {
		t.Errorf("First header = %v, want id", header[0])
	}
	if header[len(header)-1] != "metadata" {
		t.Errorf("Last header = %v, want metadata", header[len(header)-1])
	}

	// Check data row
	dataRow := records[1]
	if dataRow[0] != entry.ID {
		t.Errorf("ID in CSV = %v, want %v", dataRow[0], entry.ID)
	}
	if dataRow[2] != "tool_call.completed" {
		t.Errorf("Event type in CSV = %v, want tool_call.completed", dataRow[2])
	}
}

func TestExporter_Export_CSV_NoMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	builder, _ := NewEntry(EventSessionCreated, "user", "user")
	entry, _ := builder.Build()
	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	exporter := NewExporter(logger)

	var buf bytes.Buffer
	opts := &ExportOptions{
		Format:          FormatCSV,
		IncludeMetadata: false,
	}

	if _, err := exporter.Export(ctx, &buf, opts); err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	// Parse CSV
	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error: %v", err)
	}

	// Check header doesn't have metadata
	header := records[0]
	if header[len(header)-1] == "metadata" {
		t.Error("Header should not include metadata column")
	}
	if header[len(header)-1] != "created_at" {
		t.Errorf("Last header = %v, want created_at", header[len(header)-1])
	}
}

func TestExporter_Export_WithFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create entries with different event types
	eventTypes := []EventType{EventSessionCreated, EventSessionCreated, EventToolCallRequested, EventLLMRequestSent}
	for _, et := range eventTypes {
		builder, _ := NewEntry(et, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	exporter := NewExporter(logger)

	// Export only session events
	var buf bytes.Buffer
	opts := &ExportOptions{
		Format: FormatJSON,
		Filter: &QueryFilter{
			EventTypes: []EventType{EventSessionCreated},
		},
	}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != 2 {
		t.Errorf("Export() EntriesExported = %d, want 2", result.EntriesExported)
	}

	// Verify all entries are session events
	var entries []ExportEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	for _, e := range entries {
		if e.EventType != "session.created" {
			t.Errorf("Entry has event type %v, want session.created", e.EventType)
		}
	}
}

func TestExporter_Export_WithLimit(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create 10 entries
	for i := 0; i < 10; i++ {
		builder, _ := NewEntry(EventSessionCreated, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	exporter := NewExporter(logger)

	// Export with limit
	var buf bytes.Buffer
	opts := &ExportOptions{
		Format: FormatJSON,
		Filter: &QueryFilter{
			Limit: 3,
		},
	}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != 3 {
		t.Errorf("Export() EntriesExported = %d, want 3", result.EntriesExported)
	}
}

func TestExporter_Export_InvalidFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	ctx := context.Background()

	var buf bytes.Buffer
	opts := &ExportOptions{Format: ExportFormat("invalid")}

	_, err = exporter.Export(ctx, &buf, opts)
	if err == nil {
		t.Error("Export() should fail with invalid format")
	}
}

func TestExporter_Export_NilOptions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	ctx := context.Background()

	var buf bytes.Buffer

	// Should use default options
	result, err := exporter.Export(ctx, &buf, nil)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.Format != FormatJSON {
		t.Errorf("Export() Format = %v, want %v (default)", result.Format, FormatJSON)
	}
}

func TestDetectFormatFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     ExportFormat
	}{
		{"audit.json", FormatJSON},
		{"audit.jsonl", FormatJSONLines},
		{"audit.ndjson", FormatJSONLines},
		{"audit.csv", FormatCSV},
		{"audit.txt", FormatJSON}, // default
		{"audit", FormatJSON},     // default
		{"", FormatJSON},          // default
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			if got := detectFormatFromFilename(tt.filename); got != tt.want {
				t.Errorf("detectFormatFromFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestExporter_ExportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	builder, _ := NewEntry(EventSessionCreated, "user", "user")
	entry, _ := builder.Build()
	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	exporter := NewExporter(logger)

	// Test auto-detection from filename
	var buf bytes.Buffer
	result, err := exporter.ExportToFile(ctx, &buf, "export.csv", nil)
	if err != nil {
		t.Fatalf("ExportToFile() error = %v", err)
	}

	if result.Format != FormatCSV {
		t.Errorf("ExportToFile() Format = %v, want %v", result.Format, FormatCSV)
	}

	// Should be CSV
	reader := csv.NewReader(&buf)
	if _, err := reader.ReadAll(); err != nil {
		t.Errorf("Output is not valid CSV: %v", err)
	}
}

func TestExporter_ExportToFile_WithOpts(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	ctx := context.Background()

	// Options should override filename detection
	var buf bytes.Buffer
	opts := &ExportOptions{Format: FormatJSONLines}
	result, err := exporter.ExportToFile(ctx, &buf, "export.csv", opts)
	if err != nil {
		t.Fatalf("ExportToFile() error = %v", err)
	}

	if result.Format != FormatJSONLines {
		t.Errorf("ExportToFile() Format = %v, want %v", result.Format, FormatJSONLines)
	}
}

func TestExporter_ExportStatistics(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create some LLM entries
	builder, _ := NewEntry(EventLLMResponseReceived, "system", "system")
	entry, _ := builder.
		WithProvider("openai", "gpt-4").
		WithTokens(100, 50).
		WithCost(0.01).
		WithSuccess(true).
		Build()
	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Create some tool call entries
	for _, et := range []EventType{EventToolCallRequested, EventToolCallApproved, EventToolCallCompleted} {
		builder, _ := NewEntry(et, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	exporter := NewExporter(logger)

	var buf bytes.Buffer
	opts := &ExportStatsOptions{
		IncludeUsageStats:    true,
		IncludeToolCallStats: true,
		IncludeEntryCounts:   true,
		PrettyPrint:          true,
	}

	result, err := exporter.ExportStatistics(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("ExportStatistics() error = %v", err)
	}

	if result.Format != FormatJSON {
		t.Errorf("ExportStatistics() Format = %v, want %v", result.Format, FormatJSON)
	}

	// Parse output
	var stats ExportStats
	if err := json.Unmarshal(buf.Bytes(), &stats); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if stats.ExportedAt == "" {
		t.Error("ExportStats.ExportedAt should not be empty")
	}

	if stats.UsageStats == nil {
		t.Error("ExportStats.UsageStats should not be nil")
	} else {
		if stats.UsageStats.TotalRequests != 1 {
			t.Errorf("UsageStats.TotalRequests = %d, want 1", stats.UsageStats.TotalRequests)
		}
	}

	if stats.ToolCallStats == nil {
		t.Error("ExportStats.ToolCallStats should not be nil")
	} else {
		if stats.ToolCallStats.TotalRequested != 1 {
			t.Errorf("ToolCallStats.TotalRequested = %d, want 1", stats.ToolCallStats.TotalRequested)
		}
		if stats.ToolCallStats.TotalApproved != 1 {
			t.Errorf("ToolCallStats.TotalApproved = %d, want 1", stats.ToolCallStats.TotalApproved)
		}
		if stats.ToolCallStats.TotalCompleted != 1 {
			t.Errorf("ToolCallStats.TotalCompleted = %d, want 1", stats.ToolCallStats.TotalCompleted)
		}
	}

	if stats.EntryCounts == nil {
		t.Error("ExportStats.EntryCounts should not be nil")
	} else {
		if stats.EntryCounts.Total != 4 {
			t.Errorf("EntryCounts.Total = %d, want 4", stats.EntryCounts.Total)
		}
	}
}

func TestExporter_ExportStatistics_NilOptions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	ctx := context.Background()

	var buf bytes.Buffer
	result, err := exporter.ExportStatistics(ctx, &buf, nil)
	if err != nil {
		t.Fatalf("ExportStatistics() error = %v", err)
	}

	if result.Format != FormatJSON {
		t.Errorf("ExportStatistics() Format = %v, want %v", result.Format, FormatJSON)
	}
}

func TestExporter_ExportStatistics_WithTimeRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	exporter := NewExporter(logger)
	ctx := context.Background()

	now := time.Now().UTC()
	var buf bytes.Buffer
	opts := &ExportStatsOptions{
		Filter: &QueryFilter{
			Since: now.Add(-24 * time.Hour),
			Until: now,
		},
		IncludeUsageStats:    false,
		IncludeToolCallStats: false,
		IncludeEntryCounts:   true,
	}

	if _, err := exporter.ExportStatistics(ctx, &buf, opts); err != nil {
		t.Fatalf("ExportStatistics() error = %v", err)
	}

	var stats ExportStats
	if err := json.Unmarshal(buf.Bytes(), &stats); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if stats.TimeRange == nil {
		t.Error("ExportStats.TimeRange should not be nil")
	}
	if stats.TimeRange.Since == "" {
		t.Error("TimeRange.Since should not be empty")
	}
	if stats.TimeRange.Until == "" {
		t.Error("TimeRange.Until should not be empty")
	}
	if stats.UsageStats != nil {
		t.Error("UsageStats should be nil when IncludeUsageStats is false")
	}
	if stats.ToolCallStats != nil {
		t.Error("ToolCallStats should be nil when IncludeToolCallStats is false")
	}
}

func TestEntryToExportEntry(t *testing.T) {
	now := time.Now().UTC()

	builder, _ := NewEntry(EventToolCallCompleted, "user-123", "user")
	entry, _ := builder.
		WithSeverity(SeverityDebug).
		WithSession("session-456").
		WithMessage("message-789").
		WithToolCall("toolcall-012").
		WithProject("/path/to/project").
		WithProvider("openai", "gpt-4").
		WithTokens(100, 50).
		WithCost(0.005).
		WithDuration(150).
		WithSuccess(true).
		WithIteration(3).
		WithClientInfo("127.0.0.1", "Mozilla/5.0").
		WithMetadata(map[string]string{"key": "value"}).
		WithTimestamp(now).
		Build()

	export := entryToExportEntry(entry, true)

	if export.ID != entry.ID {
		t.Errorf("ExportEntry.ID = %v, want %v", export.ID, entry.ID)
	}
	if export.EventType != "tool_call.completed" {
		t.Errorf("ExportEntry.EventType = %v, want tool_call.completed", export.EventType)
	}
	if export.Severity != "debug" {
		t.Errorf("ExportEntry.Severity = %v, want debug", export.Severity)
	}
	if export.SessionID != "session-456" {
		t.Errorf("ExportEntry.SessionID = %v, want session-456", export.SessionID)
	}
	if export.Provider != "openai" {
		t.Errorf("ExportEntry.Provider = %v, want openai", export.Provider)
	}
	if export.Model != "gpt-4" {
		t.Errorf("ExportEntry.Model = %v, want gpt-4", export.Model)
	}
	if export.TokensInput == nil || *export.TokensInput != 100 {
		t.Errorf("ExportEntry.TokensInput = %v, want 100", export.TokensInput)
	}
	if export.TokensOutput == nil || *export.TokensOutput != 50 {
		t.Errorf("ExportEntry.TokensOutput = %v, want 50", export.TokensOutput)
	}
	if export.CostUSD == nil || *export.CostUSD != 0.005 {
		t.Errorf("ExportEntry.CostUSD = %v, want 0.005", export.CostUSD)
	}
	if export.DurationMs == nil || *export.DurationMs != 150 {
		t.Errorf("ExportEntry.DurationMs = %v, want 150", export.DurationMs)
	}
	if export.Success == nil || *export.Success != true {
		t.Errorf("ExportEntry.Success = %v, want true", export.Success)
	}
	if export.Iteration == nil || *export.Iteration != 3 {
		t.Errorf("ExportEntry.Iteration = %v, want 3", export.Iteration)
	}
	if export.IPAddress != "127.0.0.1" {
		t.Errorf("ExportEntry.IPAddress = %v, want 127.0.0.1", export.IPAddress)
	}
	if export.UserAgent != "Mozilla/5.0" {
		t.Errorf("ExportEntry.UserAgent = %v, want Mozilla/5.0", export.UserAgent)
	}
	if export.Metadata == "" {
		t.Error("ExportEntry.Metadata should not be empty")
	}
}

func TestEntryToExportEntry_NoMetadata(t *testing.T) {
	builder, _ := NewEntry(EventSessionCreated, "user", "user")
	entry, _ := builder.
		WithMetadata(map[string]string{"key": "value"}).
		Build()

	export := entryToExportEntry(entry, false)

	if export.Metadata != "" {
		t.Error("ExportEntry.Metadata should be empty when includeMetadata is false")
	}
}

func TestEntryToExportEntry_EmptyOptionalFields(t *testing.T) {
	builder, _ := NewEntry(EventSessionCreated, "user", "user")
	entry, _ := builder.Build()

	export := entryToExportEntry(entry, true)

	if export.SessionID != "" {
		t.Errorf("ExportEntry.SessionID = %v, want empty", export.SessionID)
	}
	if export.Provider != "" {
		t.Errorf("ExportEntry.Provider = %v, want empty", export.Provider)
	}
	if export.TokensInput != nil {
		t.Errorf("ExportEntry.TokensInput = %v, want nil", export.TokensInput)
	}
	if export.Success != nil {
		t.Errorf("ExportEntry.Success = %v, want nil", export.Success)
	}
}

func TestFormatHelpers(t *testing.T) {
	// Test formatInt64Ptr
	var i64 int64 = 42
	if got := formatInt64Ptr(&i64); got != "42" {
		t.Errorf("formatInt64Ptr(&42) = %v, want 42", got)
	}
	if got := formatInt64Ptr(nil); got != "" {
		t.Errorf("formatInt64Ptr(nil) = %v, want empty", got)
	}

	// Test formatFloat64Ptr
	f64 := 0.005
	if got := formatFloat64Ptr(&f64); !strings.HasPrefix(got, "0.00") {
		t.Errorf("formatFloat64Ptr(&0.005) = %v, want 0.005000", got)
	}
	if got := formatFloat64Ptr(nil); got != "" {
		t.Errorf("formatFloat64Ptr(nil) = %v, want empty", got)
	}

	// Test formatBoolPtr
	bTrue := true
	bFalse := false
	if got := formatBoolPtr(&bTrue); got != "true" {
		t.Errorf("formatBoolPtr(&true) = %v, want true", got)
	}
	if got := formatBoolPtr(&bFalse); got != "false" {
		t.Errorf("formatBoolPtr(&false) = %v, want false", got)
	}
	if got := formatBoolPtr(nil); got != "" {
		t.Errorf("formatBoolPtr(nil) = %v, want empty", got)
	}
}

func TestCsvHeaders(t *testing.T) {
	headersWithMeta := csvHeaders(true)
	headersNoMeta := csvHeaders(false)

	if headersWithMeta[len(headersWithMeta)-1] != "metadata" {
		t.Error("Headers with metadata should end with 'metadata'")
	}
	if headersNoMeta[len(headersNoMeta)-1] == "metadata" {
		t.Error("Headers without metadata should not end with 'metadata'")
	}
	if headersNoMeta[len(headersNoMeta)-1] != "created_at" {
		t.Errorf("Headers without metadata should end with 'created_at', got %v", headersNoMeta[len(headersNoMeta)-1])
	}
}

func TestExporter_Export_LargeDataset(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create 1500 entries to test pagination (pageSize is 1000)
	const numEntries = 1500
	for i := 0; i < numEntries; i++ {
		builder, _ := NewEntry(EventSessionCreated, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	exporter := NewExporter(logger)

	var buf bytes.Buffer
	opts := &ExportOptions{Format: FormatJSON}

	result, err := exporter.Export(ctx, &buf, opts)
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if result.EntriesExported != numEntries {
		t.Errorf("Export() EntriesExported = %d, want %d", result.EntriesExported, numEntries)
	}

	// Verify all entries are in the output
	var entries []ExportEntry
	if err := json.Unmarshal(buf.Bytes(), &entries); err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	if len(entries) != numEntries {
		t.Errorf("JSON contains %d entries, want %d", len(entries), numEntries)
	}
}

func TestDefaultExportStatsOptions(t *testing.T) {
	opts := DefaultExportStatsOptions()

	if opts.Filter == nil {
		t.Error("DefaultExportStatsOptions().Filter should not be nil")
	}
	if !opts.IncludeUsageStats {
		t.Error("DefaultExportStatsOptions().IncludeUsageStats should be true")
	}
	if !opts.IncludeToolCallStats {
		t.Error("DefaultExportStatsOptions().IncludeToolCallStats should be true")
	}
	if !opts.IncludeEntryCounts {
		t.Error("DefaultExportStatsOptions().IncludeEntryCounts should be true")
	}
	if !opts.PrettyPrint {
		t.Error("DefaultExportStatsOptions().PrettyPrint should be true")
	}
}

func TestCountingWriter(t *testing.T) {
	var buf bytes.Buffer
	cw := &countingWriter{w: &buf}

	data := []byte("hello world")
	n, err := cw.Write(data)

	if err != nil {
		t.Errorf("countingWriter.Write() error = %v", err)
	}
	if n != len(data) {
		t.Errorf("countingWriter.Write() returned %d, want %d", n, len(data))
	}
	if cw.count != int64(len(data)) {
		t.Errorf("countingWriter.count = %d, want %d", cw.count, len(data))
	}
}
