package audit

// Schema defines the audit table structure for SQLite with encrypted append-only support.
const Schema = `
CREATE TABLE IF NOT EXISTS audit_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	iteration INTEGER,
	data TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

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
