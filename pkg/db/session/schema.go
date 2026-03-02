// Package session provides SQLite-based session persistence for the conversational AI orchestrator.
package session

// Schema defines the session-related table structures for SQLite.
// These tables support multi-project chat sessions with provider/model tracking,
// message history, tool call audit, and session forking capabilities.
const Schema = `
-- Sessions table stores top-level chat session metadata.
-- Each session is bound to a project workspace and tracks provider/model selection.
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	project_path TEXT NOT NULL,
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	parent_session_id TEXT DEFAULT NULL,
	fork_point_message_id TEXT DEFAULT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (parent_session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

-- Messages table stores the conversation history for each session.
-- Supports both user and assistant messages with token usage tracking.
CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	tokens_input INTEGER DEFAULT 0,
	tokens_output INTEGER DEFAULT 0,
	cost_usd REAL DEFAULT 0.0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Tool calls table stores MCP tool invocations for audit and replay.
-- Each tool call is associated with an assistant message.
CREATE TABLE IF NOT EXISTS tool_calls (
	id TEXT PRIMARY KEY,
	message_id TEXT NOT NULL,
	session_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	tool_input TEXT NOT NULL,
	tool_output TEXT DEFAULT NULL,
	status TEXT NOT NULL DEFAULT 'pending',
	approval_status TEXT DEFAULT NULL,
	approved_by TEXT DEFAULT NULL,
	approved_at DATETIME DEFAULT NULL,
	started_at DATETIME DEFAULT NULL,
	completed_at DATETIME DEFAULT NULL,
	error TEXT DEFAULT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Session summaries table stores compressed conversation summaries.
-- Used for context window management when history exceeds token limits.
CREATE TABLE IF NOT EXISTS session_summaries (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	summary_text TEXT NOT NULL,
	messages_summarized INTEGER NOT NULL,
	tokens_saved INTEGER NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_sessions_project_path ON sessions(project_path);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_parent ON sessions(parent_session_id);

CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);
CREATE INDEX IF NOT EXISTS idx_messages_role ON messages(role);

CREATE INDEX IF NOT EXISTS idx_tool_calls_session_id ON tool_calls(session_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_message_id ON tool_calls(message_id);
CREATE INDEX IF NOT EXISTS idx_tool_calls_status ON tool_calls(status);
CREATE INDEX IF NOT EXISTS idx_tool_calls_tool_name ON tool_calls(tool_name);

CREATE INDEX IF NOT EXISTS idx_session_summaries_session_id ON session_summaries(session_id);
`
