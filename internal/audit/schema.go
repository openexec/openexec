package audit

// Schema defines the audit table structure for SQLite with encrypted append-only support.
const Schema = `
-- Legacy audit_logs table for backward compatibility
CREATE TABLE IF NOT EXISTS audit_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	iteration INTEGER,
	data TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Legacy encrypted_audit_logs table for backward compatibility
CREATE TABLE IF NOT EXISTS encrypted_audit_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	iteration INTEGER,
	encrypted_data BLOB NOT NULL,
	iv BLOB NOT NULL,
	nonce BLOB,
	hash TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_iteration ON audit_logs(iteration);

CREATE INDEX IF NOT EXISTS idx_encrypted_audit_event_type ON encrypted_audit_logs(event_type);
CREATE INDEX IF NOT EXISTS idx_encrypted_audit_timestamp ON encrypted_audit_logs(timestamp);
CREATE INDEX IF NOT EXISTS idx_encrypted_audit_iteration ON encrypted_audit_logs(iteration);
`

// EntrySchema defines the comprehensive audit entry table structure.
const EntrySchema = `
-- audit_entries: comprehensive audit logging table
CREATE TABLE IF NOT EXISTS audit_entries (
	id TEXT PRIMARY KEY NOT NULL,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'info',
	session_id TEXT,
	message_id TEXT,
	tool_call_id TEXT,
	actor_id TEXT NOT NULL,
	actor_type TEXT NOT NULL,
	project_path TEXT,
	provider TEXT,
	model TEXT,
	tokens_input INTEGER,
	tokens_output INTEGER,
	cost_usd REAL,
	duration_ms INTEGER,
	success INTEGER,
	error_message TEXT,
	metadata TEXT,
	iteration INTEGER,
	ip_address TEXT,
	user_agent TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_audit_entries_timestamp ON audit_entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_audit_entries_event_type ON audit_entries(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_entries_severity ON audit_entries(severity);
CREATE INDEX IF NOT EXISTS idx_audit_entries_session_id ON audit_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_audit_entries_actor_id ON audit_entries(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_entries_project_path ON audit_entries(project_path);
CREATE INDEX IF NOT EXISTS idx_audit_entries_provider ON audit_entries(provider);

-- Composite index for time-range queries with event type
CREATE INDEX IF NOT EXISTS idx_audit_entries_event_time ON audit_entries(event_type, timestamp);

-- Composite index for session-based queries
CREATE INDEX IF NOT EXISTS idx_audit_entries_session_time ON audit_entries(session_id, timestamp);

-- encrypted_audit_entries: encrypted version of audit_entries for sensitive data
CREATE TABLE IF NOT EXISTS encrypted_audit_entries (
	id TEXT PRIMARY KEY NOT NULL,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'info',
	session_id TEXT,
	actor_id TEXT NOT NULL,
	actor_type TEXT NOT NULL,
	encrypted_data BLOB NOT NULL,
	iv BLOB NOT NULL,
	hash TEXT NOT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_encrypted_entries_timestamp ON encrypted_audit_entries(timestamp);
CREATE INDEX IF NOT EXISTS idx_encrypted_entries_event_type ON encrypted_audit_entries(event_type);
CREATE INDEX IF NOT EXISTS idx_encrypted_entries_session_id ON encrypted_audit_entries(session_id);
`

// MigrationSQL contains SQL to migrate from legacy tables to the new schema.
const MigrationSQL = `
-- Migration from legacy audit_logs to audit_entries
-- This is a data migration that preserves existing audit data
INSERT OR IGNORE INTO audit_entries (
	id,
	timestamp,
	event_type,
	severity,
	actor_id,
	actor_type,
	iteration,
	metadata,
	created_at
)
SELECT
	'legacy-' || id,
	timestamp,
	event_type,
	'info',
	'system',
	'system',
	iteration,
	data,
	created_at
FROM audit_logs
WHERE NOT EXISTS (
	SELECT 1 FROM audit_entries WHERE audit_entries.id = 'legacy-' || audit_logs.id
);
`
