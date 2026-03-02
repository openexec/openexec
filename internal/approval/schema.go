package approval

// Schema defines the approval-related table structures for SQLite.
// These tables support tool approval workflows including requests, policies, and decisions.
const Schema = `
-- Approval policies table stores the rules for when approval is required.
-- Policies can be global or project-specific, with priority ordering.
CREATE TABLE IF NOT EXISTS approval_policies (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL UNIQUE,
	description TEXT NOT NULL DEFAULT '',
	project_path TEXT DEFAULT NULL,
	mode TEXT NOT NULL DEFAULT 'risk_based',
	auto_approve_risk_level TEXT NOT NULL DEFAULT 'low',
	auto_approve_trusted_tools TEXT DEFAULT NULL,
	always_require_approval_tools TEXT DEFAULT NULL,
	timeout_seconds INTEGER NOT NULL DEFAULT 300,
	is_default INTEGER NOT NULL DEFAULT 0,
	priority INTEGER NOT NULL DEFAULT 100,
	is_active INTEGER NOT NULL DEFAULT 1,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Approval requests table stores pending and resolved approval requests.
-- Each request is linked to a tool call and tracks the approval lifecycle.
CREATE TABLE IF NOT EXISTS approval_requests (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	tool_call_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	tool_input TEXT NOT NULL,
	risk_level TEXT NOT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	policy_id TEXT DEFAULT NULL,
	requested_by TEXT NOT NULL,
	expires_at DATETIME DEFAULT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (policy_id) REFERENCES approval_policies(id) ON DELETE SET NULL
);

-- Approval decisions table records the final decision for each request.
-- This provides an audit trail of who approved/rejected each operation.
CREATE TABLE IF NOT EXISTS approval_decisions (
	id TEXT PRIMARY KEY,
	request_id TEXT NOT NULL UNIQUE,
	decision TEXT NOT NULL,
	decided_by TEXT NOT NULL,
	reason TEXT DEFAULT NULL,
	policy_applied TEXT DEFAULT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (request_id) REFERENCES approval_requests(id) ON DELETE CASCADE,
	FOREIGN KEY (policy_applied) REFERENCES approval_policies(id) ON DELETE SET NULL
);

-- Indexes for efficient querying

-- Policy indexes
CREATE INDEX IF NOT EXISTS idx_approval_policies_project_path ON approval_policies(project_path);
CREATE INDEX IF NOT EXISTS idx_approval_policies_is_active ON approval_policies(is_active);
CREATE INDEX IF NOT EXISTS idx_approval_policies_is_default ON approval_policies(is_default);
CREATE INDEX IF NOT EXISTS idx_approval_policies_priority ON approval_policies(priority);

-- Request indexes
CREATE INDEX IF NOT EXISTS idx_approval_requests_session_id ON approval_requests(session_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_tool_call_id ON approval_requests(tool_call_id);
CREATE INDEX IF NOT EXISTS idx_approval_requests_status ON approval_requests(status);
CREATE INDEX IF NOT EXISTS idx_approval_requests_risk_level ON approval_requests(risk_level);
CREATE INDEX IF NOT EXISTS idx_approval_requests_tool_name ON approval_requests(tool_name);
CREATE INDEX IF NOT EXISTS idx_approval_requests_expires_at ON approval_requests(expires_at);
CREATE INDEX IF NOT EXISTS idx_approval_requests_created_at ON approval_requests(created_at);
CREATE INDEX IF NOT EXISTS idx_approval_requests_session_status ON approval_requests(session_id, status);

-- Decision indexes
CREATE INDEX IF NOT EXISTS idx_approval_decisions_request_id ON approval_decisions(request_id);
CREATE INDEX IF NOT EXISTS idx_approval_decisions_decided_by ON approval_decisions(decided_by);
CREATE INDEX IF NOT EXISTS idx_approval_decisions_decision ON approval_decisions(decision);
CREATE INDEX IF NOT EXISTS idx_approval_decisions_created_at ON approval_decisions(created_at);
`

// SeedDefaultPolicy returns the SQL to insert a default approval policy.
const SeedDefaultPolicy = `
INSERT OR IGNORE INTO approval_policies (
	id,
	name,
	description,
	mode,
	auto_approve_risk_level,
	timeout_seconds,
	is_default,
	priority,
	is_active
) VALUES (
	'default-policy',
	'Default Policy',
	'Default approval policy that auto-approves low-risk operations',
	'risk_based',
	'low',
	300,
	1,
	1000,
	1
);
`
