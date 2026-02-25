package loop

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteAuditor records events to a SQLite database.
type SQLiteAuditor struct {
	db *sql.DB
}

// NewSQLiteAuditor creates a new SQLite-based auditor and initializes the schema.
func NewSQLiteAuditor(dbPath string) (*SQLiteAuditor, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Create table if not exists.
	schema := `
CREATE TABLE IF NOT EXISTS audit_events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp TEXT NOT NULL,
	type TEXT NOT NULL,
	iteration INTEGER,
	text TEXT,
	tool TEXT,
	tool_input TEXT,
	error TEXT,
	signal_type TEXT,
	signal_target TEXT,
	phase TEXT,
	fwu_id TEXT,
	agent TEXT,
	review_cycle INTEGER,
	route_target TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_events_timestamp ON audit_events(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_events_type ON audit_events(type);
CREATE INDEX IF NOT EXISTS idx_audit_events_iteration ON audit_events(iteration);
`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteAuditor{db: db}, nil
}

// Record inserts an event into the audit database.
func (a *SQLiteAuditor) Record(ctx context.Context, event Event) error {
	toolInputJSON := ""
	if event.ToolInput != nil {
		data, err := json.Marshal(event.ToolInput)
		if err != nil {
			return fmt.Errorf("marshal tool_input: %w", err)
		}
		toolInputJSON = string(data)
	}

	query := `
INSERT INTO audit_events (
	timestamp, type, iteration, text, tool, tool_input, error,
	signal_type, signal_target, phase, fwu_id, agent, review_cycle, route_target
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

	_, err := a.db.ExecContext(ctx,
		query,
		time.Now().UTC().Format(time.RFC3339Nano),
		string(event.Type),
		nullableInt(event.Iteration),
		nullableString(event.Text),
		nullableString(event.Tool),
		nullableString(toolInputJSON),
		nullableString(event.ErrText),
		nullableString(event.SignalType),
		nullableString(event.SignalTarget),
		nullableString(event.Phase),
		nullableString(event.FWUID),
		nullableString(event.Agent),
		nullableInt(event.ReviewCycle),
		nullableString(event.RouteTarget),
	)
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (a *SQLiteAuditor) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// nullableString returns a pointer to a string if the input is non-empty.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// nullableInt returns a pointer to an int if the input is non-zero.
func nullableInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}
