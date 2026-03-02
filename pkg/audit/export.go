// Package audit provides types and utilities for tracking all system events.
package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ExportFormat represents the output format for audit log exports.
type ExportFormat string

const (
	// FormatJSON exports audit entries as JSON.
	FormatJSON ExportFormat = "json"
	// FormatJSONLines exports audit entries as newline-delimited JSON (NDJSON).
	FormatJSONLines ExportFormat = "jsonl"
	// FormatCSV exports audit entries as CSV.
	FormatCSV ExportFormat = "csv"
)

// ValidExportFormats contains all valid export format values.
var ValidExportFormats = []ExportFormat{FormatJSON, FormatJSONLines, FormatCSV}

// IsValid checks if the export format is a valid format value.
func (f ExportFormat) IsValid() bool {
	for _, valid := range ValidExportFormats {
		if f == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the export format.
func (f ExportFormat) String() string {
	return string(f)
}

// ExportOptions configures the export operation.
type ExportOptions struct {
	// Format specifies the output format (json, jsonl, csv).
	Format ExportFormat

	// Filter specifies which entries to include.
	Filter *QueryFilter

	// IncludeMetadata includes the metadata field in the export.
	IncludeMetadata bool

	// PrettyPrint enables indented JSON output (only for FormatJSON).
	PrettyPrint bool
}

// DefaultExportOptions returns export options with sensible defaults.
func DefaultExportOptions() *ExportOptions {
	return &ExportOptions{
		Format:          FormatJSON,
		Filter:          &QueryFilter{},
		IncludeMetadata: true,
		PrettyPrint:     false,
	}
}

// ExportEntry represents a flattened audit entry for export.
// It converts sql.Null* types to their underlying values for cleaner output.
type ExportEntry struct {
	ID           string   `json:"id"`
	Timestamp    string   `json:"timestamp"`
	EventType    string   `json:"event_type"`
	Severity     string   `json:"severity"`
	SessionID    string   `json:"session_id,omitempty"`
	MessageID    string   `json:"message_id,omitempty"`
	ToolCallID   string   `json:"tool_call_id,omitempty"`
	ActorID      string   `json:"actor_id"`
	ActorType    string   `json:"actor_type"`
	ProjectPath  string   `json:"project_path,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	Model        string   `json:"model,omitempty"`
	TokensInput  *int64   `json:"tokens_input,omitempty"`
	TokensOutput *int64   `json:"tokens_output,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	DurationMs   *int64   `json:"duration_ms,omitempty"`
	Success      *bool    `json:"success,omitempty"`
	ErrorMessage string   `json:"error_message,omitempty"`
	Metadata     string   `json:"metadata,omitempty"`
	Iteration    *int64   `json:"iteration,omitempty"`
	IPAddress    string   `json:"ip_address,omitempty"`
	UserAgent    string   `json:"user_agent,omitempty"`
	CreatedAt    string   `json:"created_at"`
}

// entryToExportEntry converts an Entry to an ExportEntry for cleaner export.
func entryToExportEntry(e *Entry, includeMetadata bool) *ExportEntry {
	export := &ExportEntry{
		ID:        e.ID,
		Timestamp: e.Timestamp.Format(time.RFC3339Nano),
		EventType: string(e.EventType),
		Severity:  string(e.Severity),
		ActorID:   e.ActorID,
		ActorType: e.ActorType,
		CreatedAt: e.CreatedAt.Format(time.RFC3339Nano),
	}

	if e.SessionID.Valid {
		export.SessionID = e.SessionID.String
	}
	if e.MessageID.Valid {
		export.MessageID = e.MessageID.String
	}
	if e.ToolCallID.Valid {
		export.ToolCallID = e.ToolCallID.String
	}
	if e.ProjectPath.Valid {
		export.ProjectPath = e.ProjectPath.String
	}
	if e.Provider.Valid {
		export.Provider = e.Provider.String
	}
	if e.Model.Valid {
		export.Model = e.Model.String
	}
	if e.TokensInput.Valid {
		val := e.TokensInput.Int64
		export.TokensInput = &val
	}
	if e.TokensOutput.Valid {
		val := e.TokensOutput.Int64
		export.TokensOutput = &val
	}
	if e.CostUSD.Valid {
		val := e.CostUSD.Float64
		export.CostUSD = &val
	}
	if e.DurationMs.Valid {
		val := e.DurationMs.Int64
		export.DurationMs = &val
	}
	if e.Success.Valid {
		val := e.Success.Bool
		export.Success = &val
	}
	if e.ErrorMessage.Valid {
		export.ErrorMessage = e.ErrorMessage.String
	}
	if includeMetadata && e.Metadata.Valid {
		export.Metadata = e.Metadata.String
	}
	if e.Iteration.Valid {
		val := e.Iteration.Int64
		export.Iteration = &val
	}
	if e.IPAddress.Valid {
		export.IPAddress = e.IPAddress.String
	}
	if e.UserAgent.Valid {
		export.UserAgent = e.UserAgent.String
	}

	return export
}

// ExportResult contains the result of an export operation.
type ExportResult struct {
	// BytesWritten is the total number of bytes written.
	BytesWritten int64
	// EntriesExported is the number of audit entries exported.
	EntriesExported int64
	// Format is the export format used.
	Format ExportFormat
}

// Exporter provides audit log export functionality.
type Exporter struct {
	logger *AuditLogger
}

// NewExporter creates a new Exporter using the given AuditLogger.
func NewExporter(logger *AuditLogger) *Exporter {
	return &Exporter{logger: logger}
}

// Export writes audit entries to the given writer in the specified format.
func (e *Exporter) Export(ctx context.Context, w io.Writer, opts *ExportOptions) (*ExportResult, error) {
	if opts == nil {
		opts = DefaultExportOptions()
	}

	if !opts.Format.IsValid() {
		return nil, fmt.Errorf("invalid export format: %s", opts.Format)
	}

	// Query entries (without limit to get all matching entries)
	filter := opts.Filter
	if filter == nil {
		filter = &QueryFilter{}
	}

	// Get all matching entries - we need to handle pagination ourselves
	// for large datasets
	allEntries, err := e.getAllEntries(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query entries: %w", err)
	}

	var bytesWritten int64

	switch opts.Format {
	case FormatJSON:
		bytesWritten, err = e.exportJSON(w, allEntries, opts)
	case FormatJSONLines:
		bytesWritten, err = e.exportJSONLines(w, allEntries, opts)
	case FormatCSV:
		bytesWritten, err = e.exportCSV(w, allEntries, opts)
	default:
		return nil, fmt.Errorf("unsupported export format: %s", opts.Format)
	}

	if err != nil {
		return nil, err
	}

	return &ExportResult{
		BytesWritten:    bytesWritten,
		EntriesExported: int64(len(allEntries)),
		Format:          opts.Format,
	}, nil
}

// getAllEntries retrieves all entries matching the filter using pagination.
func (e *Exporter) getAllEntries(ctx context.Context, filter *QueryFilter) ([]*Entry, error) {
	// If a limit is set, use it directly
	if filter.Limit > 0 {
		result, err := e.logger.Query(ctx, filter)
		if err != nil {
			return nil, err
		}
		return result.Entries, nil
	}

	// Otherwise, paginate through all entries
	const pageSize = 1000
	var allEntries []*Entry
	offset := 0

	for {
		pageFilter := &QueryFilter{
			EventTypes:  filter.EventTypes,
			Severities:  filter.Severities,
			SessionID:   filter.SessionID,
			ActorID:     filter.ActorID,
			ProjectPath: filter.ProjectPath,
			Since:       filter.Since,
			Until:       filter.Until,
			Limit:       pageSize,
			Offset:      offset,
		}

		result, err := e.logger.Query(ctx, pageFilter)
		if err != nil {
			return nil, err
		}

		allEntries = append(allEntries, result.Entries...)

		if !result.HasMore || len(result.Entries) < pageSize {
			break
		}

		offset += pageSize
	}

	return allEntries, nil
}

// exportJSON exports entries as a JSON array.
func (e *Exporter) exportJSON(w io.Writer, entries []*Entry, opts *ExportOptions) (int64, error) {
	exportEntries := make([]*ExportEntry, len(entries))
	for i, entry := range entries {
		exportEntries[i] = entryToExportEntry(entry, opts.IncludeMetadata)
	}

	var data []byte
	var err error

	if opts.PrettyPrint {
		data, err = json.MarshalIndent(exportEntries, "", "  ")
	} else {
		data, err = json.Marshal(exportEntries)
	}

	if err != nil {
		return 0, fmt.Errorf("failed to marshal JSON: %w", err)
	}

	// Add trailing newline
	data = append(data, '\n')

	n, err := w.Write(data)
	return int64(n), err
}

// exportJSONLines exports entries as newline-delimited JSON (NDJSON).
func (e *Exporter) exportJSONLines(w io.Writer, entries []*Entry, opts *ExportOptions) (int64, error) {
	var totalWritten int64

	for _, entry := range entries {
		exportEntry := entryToExportEntry(entry, opts.IncludeMetadata)

		data, err := json.Marshal(exportEntry)
		if err != nil {
			return totalWritten, fmt.Errorf("failed to marshal entry %s: %w", entry.ID, err)
		}

		// Add newline
		data = append(data, '\n')

		n, err := w.Write(data)
		totalWritten += int64(n)
		if err != nil {
			return totalWritten, fmt.Errorf("failed to write entry: %w", err)
		}
	}

	return totalWritten, nil
}

// csvHeaders returns the CSV column headers.
func csvHeaders(includeMetadata bool) []string {
	headers := []string{
		"id",
		"timestamp",
		"event_type",
		"severity",
		"session_id",
		"message_id",
		"tool_call_id",
		"actor_id",
		"actor_type",
		"project_path",
		"provider",
		"model",
		"tokens_input",
		"tokens_output",
		"cost_usd",
		"duration_ms",
		"success",
		"error_message",
		"iteration",
		"ip_address",
		"user_agent",
		"created_at",
	}

	if includeMetadata {
		headers = append(headers, "metadata")
	}

	return headers
}

// entryToCSVRow converts an ExportEntry to a CSV row.
func entryToCSVRow(e *ExportEntry, includeMetadata bool) []string {
	row := []string{
		e.ID,
		e.Timestamp,
		e.EventType,
		e.Severity,
		e.SessionID,
		e.MessageID,
		e.ToolCallID,
		e.ActorID,
		e.ActorType,
		e.ProjectPath,
		e.Provider,
		e.Model,
		formatInt64Ptr(e.TokensInput),
		formatInt64Ptr(e.TokensOutput),
		formatFloat64Ptr(e.CostUSD),
		formatInt64Ptr(e.DurationMs),
		formatBoolPtr(e.Success),
		e.ErrorMessage,
		formatInt64Ptr(e.Iteration),
		e.IPAddress,
		e.UserAgent,
		e.CreatedAt,
	}

	if includeMetadata {
		row = append(row, e.Metadata)
	}

	return row
}

// formatInt64Ptr formats an int64 pointer to string.
func formatInt64Ptr(v *int64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%d", *v)
}

// formatFloat64Ptr formats a float64 pointer to string.
func formatFloat64Ptr(v *float64) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%f", *v)
}

// formatBoolPtr formats a bool pointer to string.
func formatBoolPtr(v *bool) string {
	if v == nil {
		return ""
	}
	if *v {
		return "true"
	}
	return "false"
}

// exportCSV exports entries as CSV.
func (e *Exporter) exportCSV(w io.Writer, entries []*Entry, opts *ExportOptions) (int64, error) {
	// Use a counting writer to track bytes written
	cw := &countingWriter{w: w}
	csvWriter := csv.NewWriter(cw)

	// Write header
	if err := csvWriter.Write(csvHeaders(opts.IncludeMetadata)); err != nil {
		return cw.count, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write data rows
	for _, entry := range entries {
		exportEntry := entryToExportEntry(entry, opts.IncludeMetadata)
		row := entryToCSVRow(exportEntry, opts.IncludeMetadata)

		if err := csvWriter.Write(row); err != nil {
			return cw.count, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return cw.count, fmt.Errorf("CSV flush error: %w", err)
	}

	return cw.count, nil
}

// countingWriter wraps an io.Writer and counts bytes written.
type countingWriter struct {
	w     io.Writer
	count int64
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.count += int64(n)
	return n, err
}

// ExportToFile is a convenience method that exports audit entries to a file.
// The format is determined by the file extension if not specified in options.
func (e *Exporter) ExportToFile(ctx context.Context, w io.Writer, filename string, opts *ExportOptions) (*ExportResult, error) {
	if opts == nil {
		opts = DefaultExportOptions()
		// Detect format from filename for default options
		opts.Format = detectFormatFromFilename(filename)
	} else if opts.Format == "" {
		// If format not set in provided options, try to detect from filename
		opts.Format = detectFormatFromFilename(filename)
	}

	return e.Export(ctx, w, opts)
}

// detectFormatFromFilename attempts to detect the export format from a filename.
func detectFormatFromFilename(filename string) ExportFormat {
	// Check file extension from longest to shortest
	if len(filename) >= 7 && filename[len(filename)-7:] == ".ndjson" {
		return FormatJSONLines
	}
	if len(filename) >= 6 && filename[len(filename)-6:] == ".jsonl" {
		return FormatJSONLines
	}
	if len(filename) >= 5 && filename[len(filename)-5:] == ".json" {
		return FormatJSON
	}
	if len(filename) >= 4 && filename[len(filename)-4:] == ".csv" {
		return FormatCSV
	}
	// Default to JSON
	return FormatJSON
}

// ExportStats exports aggregated usage and tool call statistics.
type ExportStats struct {
	ExportedAt    string         `json:"exported_at"`
	TimeRange     *TimeRange     `json:"time_range,omitempty"`
	UsageStats    *UsageStats    `json:"usage_stats,omitempty"`
	ToolCallStats *ToolCallStats `json:"tool_call_stats,omitempty"`
	EntryCounts   *EntryCounts   `json:"entry_counts,omitempty"`
}

// TimeRange represents the time range of exported data.
type TimeRange struct {
	Since string `json:"since,omitempty"`
	Until string `json:"until,omitempty"`
}

// EntryCounts contains counts of entries by event type and severity.
type EntryCounts struct {
	Total      int64            `json:"total"`
	ByEvent    map[string]int64 `json:"by_event,omitempty"`
	BySeverity map[string]int64 `json:"by_severity,omitempty"`
}

// ExportStatsOptions configures the stats export operation.
type ExportStatsOptions struct {
	// Filter specifies which entries to include in stats.
	Filter *QueryFilter
	// IncludeUsageStats includes LLM usage statistics.
	IncludeUsageStats bool
	// IncludeToolCallStats includes tool call statistics.
	IncludeToolCallStats bool
	// IncludeEntryCounts includes entry counts by event type and severity.
	IncludeEntryCounts bool
	// PrettyPrint enables indented JSON output.
	PrettyPrint bool
}

// DefaultExportStatsOptions returns stats export options with sensible defaults.
func DefaultExportStatsOptions() *ExportStatsOptions {
	return &ExportStatsOptions{
		Filter:               &QueryFilter{},
		IncludeUsageStats:    true,
		IncludeToolCallStats: true,
		IncludeEntryCounts:   true,
		PrettyPrint:          true,
	}
}

// ExportStatistics exports aggregated statistics as JSON.
func (e *Exporter) ExportStatistics(ctx context.Context, w io.Writer, opts *ExportStatsOptions) (*ExportResult, error) {
	if opts == nil {
		opts = DefaultExportStatsOptions()
	}

	filter := opts.Filter
	if filter == nil {
		filter = &QueryFilter{}
	}

	stats := &ExportStats{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	// Set time range if specified in filter
	if !filter.Since.IsZero() || !filter.Until.IsZero() {
		stats.TimeRange = &TimeRange{}
		if !filter.Since.IsZero() {
			stats.TimeRange.Since = filter.Since.Format(time.RFC3339)
		}
		if !filter.Until.IsZero() {
			stats.TimeRange.Until = filter.Until.Format(time.RFC3339)
		}
	}

	// Get usage stats
	if opts.IncludeUsageStats {
		usageStats, err := e.logger.GetUsageStats(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to get usage stats: %w", err)
		}
		stats.UsageStats = usageStats
	}

	// Get tool call stats
	if opts.IncludeToolCallStats {
		toolCallStats, err := e.logger.GetToolCallStats(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to get tool call stats: %w", err)
		}
		stats.ToolCallStats = toolCallStats
	}

	// Get entry counts
	if opts.IncludeEntryCounts {
		entryCounts, err := e.getEntryCounts(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("failed to get entry counts: %w", err)
		}
		stats.EntryCounts = entryCounts
	}

	var data []byte
	var err error

	if opts.PrettyPrint {
		data, err = json.MarshalIndent(stats, "", "  ")
	} else {
		data, err = json.Marshal(stats)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to marshal stats: %w", err)
	}

	// Add trailing newline
	data = append(data, '\n')

	n, err := w.Write(data)
	if err != nil {
		return nil, fmt.Errorf("failed to write stats: %w", err)
	}

	return &ExportResult{
		BytesWritten:    int64(n),
		EntriesExported: 0, // Stats export doesn't count entries
		Format:          FormatJSON,
	}, nil
}

// getEntryCounts queries the database for entry counts by event type and severity.
func (e *Exporter) getEntryCounts(ctx context.Context, filter *QueryFilter) (*EntryCounts, error) {
	// Get all entries to count them
	allEntries, err := e.getAllEntries(ctx, filter)
	if err != nil {
		return nil, err
	}

	counts := &EntryCounts{
		Total:      int64(len(allEntries)),
		ByEvent:    make(map[string]int64),
		BySeverity: make(map[string]int64),
	}

	for _, entry := range allEntries {
		counts.ByEvent[string(entry.EventType)]++
		counts.BySeverity[string(entry.Severity)]++
	}

	return counts, nil
}
