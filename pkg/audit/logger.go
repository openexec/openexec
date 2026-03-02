// Package audit provides types and utilities for tracking all system events.
package audit

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Logger defines the interface for audit logging operations.
type Logger interface {
	// Log writes an audit entry to the store.
	Log(ctx context.Context, entry *Entry) error

	// LogEvent is a convenience method for logging with minimal setup.
	LogEvent(ctx context.Context, eventType EventType, actorID, actorType string) error

	// Query retrieves audit entries matching the given filter.
	Query(ctx context.Context, filter *QueryFilter) (*QueryResult, error)

	// GetUsageStats returns aggregated usage statistics for the given filter.
	GetUsageStats(ctx context.Context, filter *QueryFilter) (*UsageStats, error)

	// GetToolCallStats returns aggregated tool call statistics for the given filter.
	GetToolCallStats(ctx context.Context, filter *QueryFilter) (*ToolCallStats, error)

	// Close releases resources held by the logger.
	Close() error
}

// AuditLogger implements the Logger interface with SQLite storage.
type AuditLogger struct {
	db     *sql.DB
	mu     sync.RWMutex
	closed bool
}

// NewLogger creates a new AuditLogger with the given database path.
func NewLogger(dbPath string) (*AuditLogger, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open audit database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping audit database: %w", err)
	}

	logger := &AuditLogger{db: db}

	// Initialize schema
	if err := logger.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return logger, nil
}

// initSchema creates the audit tables if they don't exist.
func (l *AuditLogger) initSchema() error {
	// First run legacy schema for backward compatibility
	if _, err := l.db.Exec(Schema); err != nil {
		return fmt.Errorf("failed to initialize legacy schema: %w", err)
	}

	// Then run the new entry schema
	if _, err := l.db.Exec(EntrySchema); err != nil {
		return fmt.Errorf("failed to initialize entry schema: %w", err)
	}

	return nil
}

// Log writes an audit entry to the store.
func (l *AuditLogger) Log(ctx context.Context, entry *Entry) error {
	if entry == nil {
		return fmt.Errorf("%w: entry cannot be nil", ErrInvalidAuditEntry)
	}

	if err := entry.Validate(); err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return fmt.Errorf("audit logger is closed")
	}

	query := `
		INSERT INTO audit_entries (
			id, timestamp, event_type, severity, session_id, message_id, tool_call_id,
			actor_id, actor_type, project_path, provider, model, tokens_input, tokens_output,
			cost_usd, duration_ms, success, error_message, metadata, iteration, ip_address, user_agent, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := l.db.ExecContext(ctx, query,
		entry.ID,
		entry.Timestamp.Format(time.RFC3339Nano),
		string(entry.EventType),
		string(entry.Severity),
		nullString(entry.SessionID),
		nullString(entry.MessageID),
		nullString(entry.ToolCallID),
		entry.ActorID,
		entry.ActorType,
		nullString(entry.ProjectPath),
		nullString(entry.Provider),
		nullString(entry.Model),
		nullInt64(entry.TokensInput),
		nullInt64(entry.TokensOutput),
		nullFloat64(entry.CostUSD),
		nullInt64(entry.DurationMs),
		nullBool(entry.Success),
		nullString(entry.ErrorMessage),
		nullString(entry.Metadata),
		nullInt64(entry.Iteration),
		nullString(entry.IPAddress),
		nullString(entry.UserAgent),
		entry.CreatedAt.Format(time.RFC3339Nano),
	)

	if err != nil {
		return fmt.Errorf("failed to insert audit entry: %w", err)
	}

	return nil
}

// LogEvent is a convenience method for logging with minimal setup.
func (l *AuditLogger) LogEvent(ctx context.Context, eventType EventType, actorID, actorType string) error {
	builder, err := NewEntry(eventType, actorID, actorType)
	if err != nil {
		return err
	}

	entry, err := builder.Build()
	if err != nil {
		return err
	}

	return l.Log(ctx, entry)
}

// Query retrieves audit entries matching the given filter.
func (l *AuditLogger) Query(ctx context.Context, filter *QueryFilter) (*QueryResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, fmt.Errorf("audit logger is closed")
	}

	// Build the WHERE clause
	var whereClauses []string
	var args []interface{}

	if len(filter.EventTypes) > 0 {
		placeholders := ""
		for i, et := range filter.EventTypes {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, string(et))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("event_type IN (%s)", placeholders))
	}

	if len(filter.Severities) > 0 {
		placeholders := ""
		for i, s := range filter.Severities {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, string(s))
		}
		whereClauses = append(whereClauses, fmt.Sprintf("severity IN (%s)", placeholders))
	}

	if filter.SessionID != "" {
		whereClauses = append(whereClauses, "session_id = ?")
		args = append(args, filter.SessionID)
	}

	if filter.ActorID != "" {
		whereClauses = append(whereClauses, "actor_id = ?")
		args = append(args, filter.ActorID)
	}

	if filter.ProjectPath != "" {
		whereClauses = append(whereClauses, "project_path = ?")
		args = append(args, filter.ProjectPath)
	}

	if !filter.Since.IsZero() {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, filter.Since.Format(time.RFC3339Nano))
	}

	if !filter.Until.IsZero() {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, filter.Until.Format(time.RFC3339Nano))
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE "
		for i, clause := range whereClauses {
			if i > 0 {
				whereSQL += " AND "
			}
			whereSQL += clause
		}
	}

	// Count total matching entries
	countQuery := "SELECT COUNT(*) FROM audit_entries " + whereSQL
	var totalCount int64
	if err := l.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("failed to count audit entries: %w", err)
	}

	// Build the main query with pagination
	query := `
		SELECT id, timestamp, event_type, severity, session_id, message_id, tool_call_id,
			actor_id, actor_type, project_path, provider, model, tokens_input, tokens_output,
			cost_usd, duration_ms, success, error_message, metadata, iteration, ip_address, user_agent, created_at
		FROM audit_entries
	` + whereSQL + " ORDER BY timestamp DESC"

	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	rows, err := l.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit entries: %w", err)
	}
	defer rows.Close()

	var entries []*Entry
	for rows.Next() {
		entry := &Entry{}
		var timestamp, createdAt string

		err := rows.Scan(
			&entry.ID,
			&timestamp,
			&entry.EventType,
			&entry.Severity,
			&entry.SessionID,
			&entry.MessageID,
			&entry.ToolCallID,
			&entry.ActorID,
			&entry.ActorType,
			&entry.ProjectPath,
			&entry.Provider,
			&entry.Model,
			&entry.TokensInput,
			&entry.TokensOutput,
			&entry.CostUSD,
			&entry.DurationMs,
			&entry.Success,
			&entry.ErrorMessage,
			&entry.Metadata,
			&entry.Iteration,
			&entry.IPAddress,
			&entry.UserAgent,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan audit entry: %w", err)
		}

		entry.Timestamp, _ = time.Parse(time.RFC3339Nano, timestamp)
		entry.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit entries: %w", err)
	}

	hasMore := false
	if filter.Limit > 0 {
		hasMore = int64(filter.Offset+len(entries)) < totalCount
	}

	return &QueryResult{
		Entries:    entries,
		TotalCount: totalCount,
		HasMore:    hasMore,
	}, nil
}

// GetUsageStats returns aggregated usage statistics for the given filter.
func (l *AuditLogger) GetUsageStats(ctx context.Context, filter *QueryFilter) (*UsageStats, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, fmt.Errorf("audit logger is closed")
	}

	// Build WHERE clause for LLM events
	var whereClauses []string
	var args []interface{}

	// Only include LLM response events for stats
	whereClauses = append(whereClauses, "event_type IN (?, ?)")
	args = append(args, string(EventLLMResponseReceived), string(EventLLMError))

	if filter.SessionID != "" {
		whereClauses = append(whereClauses, "session_id = ?")
		args = append(args, filter.SessionID)
	}

	if filter.ProjectPath != "" {
		whereClauses = append(whereClauses, "project_path = ?")
		args = append(args, filter.ProjectPath)
	}

	if !filter.Since.IsZero() {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, filter.Since.Format(time.RFC3339Nano))
	}

	if !filter.Until.IsZero() {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, filter.Until.Format(time.RFC3339Nano))
	}

	whereSQL := "WHERE " + whereClauses[0]
	for i := 1; i < len(whereClauses); i++ {
		whereSQL += " AND " + whereClauses[i]
	}

	// Aggregate query
	query := `
		SELECT
			COALESCE(SUM(tokens_input), 0) as total_tokens_input,
			COALESCE(SUM(tokens_output), 0) as total_tokens_output,
			COALESCE(SUM(cost_usd), 0) as total_cost_usd,
			COUNT(*) as total_requests,
			COALESCE(SUM(CASE WHEN success = 1 THEN 1 ELSE 0 END), 0) as successful_requests,
			COALESCE(SUM(CASE WHEN success = 0 THEN 1 ELSE 0 END), 0) as failed_requests,
			COALESCE(AVG(duration_ms), 0) as average_duration_ms
		FROM audit_entries
	` + whereSQL

	stats := &UsageStats{ByProvider: make(map[string]*ProviderStats)}

	err := l.db.QueryRowContext(ctx, query, args...).Scan(
		&stats.TotalTokensInput,
		&stats.TotalTokensOutput,
		&stats.TotalCostUSD,
		&stats.TotalRequests,
		&stats.SuccessfulRequests,
		&stats.FailedRequests,
		&stats.AverageDurationMs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage stats: %w", err)
	}

	// Get per-provider stats
	providerQuery := `
		SELECT
			provider,
			COALESCE(SUM(tokens_input), 0) as total_tokens_input,
			COALESCE(SUM(tokens_output), 0) as total_tokens_output,
			COALESCE(SUM(cost_usd), 0) as total_cost_usd,
			COUNT(*) as total_requests
		FROM audit_entries
	` + whereSQL + `
		AND provider IS NOT NULL
		GROUP BY provider
	`

	rows, err := l.db.QueryContext(ctx, providerQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		ps := &ProviderStats{}
		if err := rows.Scan(&ps.Provider, &ps.TotalTokensInput, &ps.TotalTokensOutput, &ps.TotalCostUSD, &ps.TotalRequests); err != nil {
			return nil, fmt.Errorf("failed to scan provider stats: %w", err)
		}
		stats.ByProvider[ps.Provider] = ps
	}

	return stats, nil
}

// GetToolCallStats returns aggregated tool call statistics for the given filter.
func (l *AuditLogger) GetToolCallStats(ctx context.Context, filter *QueryFilter) (*ToolCallStats, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, fmt.Errorf("audit logger is closed")
	}

	// Build WHERE clause for tool call events
	var whereClauses []string
	var args []interface{}

	// Include all tool call event types
	toolCallEvents := []EventType{
		EventToolCallRequested,
		EventToolCallApproved,
		EventToolCallRejected,
		EventToolCallAutoApproved,
		EventToolCallStarted,
		EventToolCallCompleted,
		EventToolCallFailed,
		EventToolCallCancelled,
	}

	placeholders := ""
	for i, et := range toolCallEvents {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
		args = append(args, string(et))
	}
	whereClauses = append(whereClauses, fmt.Sprintf("event_type IN (%s)", placeholders))

	if filter.SessionID != "" {
		whereClauses = append(whereClauses, "session_id = ?")
		args = append(args, filter.SessionID)
	}

	if filter.ProjectPath != "" {
		whereClauses = append(whereClauses, "project_path = ?")
		args = append(args, filter.ProjectPath)
	}

	if !filter.Since.IsZero() {
		whereClauses = append(whereClauses, "timestamp >= ?")
		args = append(args, filter.Since.Format(time.RFC3339Nano))
	}

	if !filter.Until.IsZero() {
		whereClauses = append(whereClauses, "timestamp <= ?")
		args = append(args, filter.Until.Format(time.RFC3339Nano))
	}

	whereSQL := "WHERE " + whereClauses[0]
	for i := 1; i < len(whereClauses); i++ {
		whereSQL += " AND " + whereClauses[i]
	}

	// Aggregate query
	query := `
		SELECT
			COALESCE(SUM(CASE WHEN event_type = ? THEN 1 ELSE 0 END), 0) as total_requested,
			COALESCE(SUM(CASE WHEN event_type = ? THEN 1 ELSE 0 END), 0) as total_approved,
			COALESCE(SUM(CASE WHEN event_type = ? THEN 1 ELSE 0 END), 0) as total_rejected,
			COALESCE(SUM(CASE WHEN event_type = ? THEN 1 ELSE 0 END), 0) as total_auto_approved,
			COALESCE(SUM(CASE WHEN event_type = ? THEN 1 ELSE 0 END), 0) as total_completed,
			COALESCE(SUM(CASE WHEN event_type = ? THEN 1 ELSE 0 END), 0) as total_failed
		FROM audit_entries
	` + whereSQL

	// Add event type args for the aggregation
	statsArgs := append([]interface{}{
		string(EventToolCallRequested),
		string(EventToolCallApproved),
		string(EventToolCallRejected),
		string(EventToolCallAutoApproved),
		string(EventToolCallCompleted),
		string(EventToolCallFailed),
	}, args...)

	stats := &ToolCallStats{ByTool: make(map[string]int64)}

	err := l.db.QueryRowContext(ctx, query, statsArgs...).Scan(
		&stats.TotalRequested,
		&stats.TotalApproved,
		&stats.TotalRejected,
		&stats.TotalAutoApproved,
		&stats.TotalCompleted,
		&stats.TotalFailed,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool call stats: %w", err)
	}

	return stats, nil
}

// GetByID retrieves a single audit entry by its ID.
func (l *AuditLogger) GetByID(ctx context.Context, id string) (*Entry, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, fmt.Errorf("audit logger is closed")
	}

	query := `
		SELECT id, timestamp, event_type, severity, session_id, message_id, tool_call_id,
			actor_id, actor_type, project_path, provider, model, tokens_input, tokens_output,
			cost_usd, duration_ms, success, error_message, metadata, iteration, ip_address, user_agent, created_at
		FROM audit_entries
		WHERE id = ?
	`

	entry := &Entry{}
	var timestamp, createdAt string

	err := l.db.QueryRowContext(ctx, query, id).Scan(
		&entry.ID,
		&timestamp,
		&entry.EventType,
		&entry.Severity,
		&entry.SessionID,
		&entry.MessageID,
		&entry.ToolCallID,
		&entry.ActorID,
		&entry.ActorType,
		&entry.ProjectPath,
		&entry.Provider,
		&entry.Model,
		&entry.TokensInput,
		&entry.TokensOutput,
		&entry.CostUSD,
		&entry.DurationMs,
		&entry.Success,
		&entry.ErrorMessage,
		&entry.Metadata,
		&entry.Iteration,
		&entry.IPAddress,
		&entry.UserAgent,
		&createdAt,
	)

	if err == sql.ErrNoRows {
		return nil, ErrAuditEntryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get audit entry: %w", err)
	}

	entry.Timestamp, _ = time.Parse(time.RFC3339Nano, timestamp)
	entry.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)

	return entry, nil
}

// Close releases resources held by the logger.
func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}

	l.closed = true
	if l.db != nil {
		return l.db.Close()
	}
	return nil
}

// Helper functions for handling nullable values

func nullString(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullInt64(ni sql.NullInt64) interface{} {
	if ni.Valid {
		return ni.Int64
	}
	return nil
}

func nullFloat64(nf sql.NullFloat64) interface{} {
	if nf.Valid {
		return nf.Float64
	}
	return nil
}

func nullBool(nb sql.NullBool) interface{} {
	if nb.Valid {
		if nb.Bool {
			return 1
		}
		return 0
	}
	return nil
}

// Compile-time check that AuditLogger implements Logger
var _ Logger = (*AuditLogger)(nil)
