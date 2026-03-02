package context

// Schema defines the context-related table structures for SQLite.
// These tables support automatic context gathering, caching, and token budget management
// for AI agent sessions.
const Schema = `
-- context_items stores gathered context data from the project workspace.
-- Each item represents a piece of context (git status, CLAUDE.md, etc.) that can be
-- injected into the conversation.
CREATE TABLE IF NOT EXISTS context_items (
	id TEXT PRIMARY KEY NOT NULL,
	session_id TEXT DEFAULT NULL,
	type TEXT NOT NULL,
	source TEXT NOT NULL,
	content TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	token_count INTEGER NOT NULL DEFAULT 0,
	priority INTEGER NOT NULL DEFAULT 50,
	metadata TEXT DEFAULT NULL,
	is_stale INTEGER NOT NULL DEFAULT 0,
	gathered_at TEXT NOT NULL,
	expires_at TEXT DEFAULT NULL,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now')),
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- gatherer_configs stores configuration for context gatherers.
-- Each gatherer is responsible for collecting a specific type of context.
CREATE TABLE IF NOT EXISTS gatherer_configs (
	id TEXT PRIMARY KEY NOT NULL,
	project_path TEXT DEFAULT NULL,
	type TEXT NOT NULL,
	name TEXT NOT NULL,
	description TEXT DEFAULT NULL,
	priority INTEGER NOT NULL DEFAULT 50,
	max_tokens INTEGER NOT NULL DEFAULT 4000,
	refresh_interval_seconds INTEGER NOT NULL DEFAULT 300,
	command TEXT DEFAULT NULL,
	file_paths TEXT DEFAULT NULL,
	file_patterns TEXT DEFAULT NULL,
	options TEXT DEFAULT NULL,
	is_enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- context_budgets stores token budget configurations for context injection.
-- Budgets control how many tokens can be used for context vs conversation.
CREATE TABLE IF NOT EXISTS context_budgets (
	id TEXT PRIMARY KEY NOT NULL,
	project_path TEXT DEFAULT NULL,
	total_token_budget INTEGER NOT NULL DEFAULT 128000,
	reserved_for_system_prompt INTEGER NOT NULL DEFAULT 2000,
	reserved_for_conversation INTEGER NOT NULL DEFAULT 32000,
	max_per_type TEXT DEFAULT NULL,
	min_priority_to_include INTEGER NOT NULL DEFAULT 10,
	is_default INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- gatherer_executions records the execution history of context gatherers.
-- Used for debugging, performance monitoring, and audit.
CREATE TABLE IF NOT EXISTS gatherer_executions (
	id TEXT PRIMARY KEY NOT NULL,
	gatherer_id TEXT NOT NULL,
	session_id TEXT DEFAULT NULL,
	status TEXT NOT NULL DEFAULT 'running',
	context_item_id TEXT DEFAULT NULL,
	tokens_gathered INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	error TEXT DEFAULT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT DEFAULT NULL,
	FOREIGN KEY (gatherer_id) REFERENCES gatherer_configs(id) ON DELETE CASCADE,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL,
	FOREIGN KEY (context_item_id) REFERENCES context_items(id) ON DELETE SET NULL
);

-- Indexes for efficient querying

-- context_items indexes
CREATE INDEX IF NOT EXISTS idx_context_items_session_id ON context_items(session_id);
CREATE INDEX IF NOT EXISTS idx_context_items_type ON context_items(type);
CREATE INDEX IF NOT EXISTS idx_context_items_priority ON context_items(priority DESC);
CREATE INDEX IF NOT EXISTS idx_context_items_gathered_at ON context_items(gathered_at);
CREATE INDEX IF NOT EXISTS idx_context_items_is_stale ON context_items(is_stale);
CREATE INDEX IF NOT EXISTS idx_context_items_content_hash ON context_items(content_hash);

-- Composite index for session context lookup with priority ordering
CREATE INDEX IF NOT EXISTS idx_context_items_session_priority ON context_items(session_id, priority DESC);

-- Composite index for type-based queries within session
CREATE INDEX IF NOT EXISTS idx_context_items_session_type ON context_items(session_id, type);

-- gatherer_configs indexes
CREATE INDEX IF NOT EXISTS idx_gatherer_configs_project_path ON gatherer_configs(project_path);
CREATE INDEX IF NOT EXISTS idx_gatherer_configs_type ON gatherer_configs(type);
CREATE INDEX IF NOT EXISTS idx_gatherer_configs_is_enabled ON gatherer_configs(is_enabled);

-- context_budgets indexes
CREATE INDEX IF NOT EXISTS idx_context_budgets_project_path ON context_budgets(project_path);
CREATE INDEX IF NOT EXISTS idx_context_budgets_is_default ON context_budgets(is_default);

-- gatherer_executions indexes
CREATE INDEX IF NOT EXISTS idx_gatherer_executions_gatherer_id ON gatherer_executions(gatherer_id);
CREATE INDEX IF NOT EXISTS idx_gatherer_executions_session_id ON gatherer_executions(session_id);
CREATE INDEX IF NOT EXISTS idx_gatherer_executions_status ON gatherer_executions(status);
CREATE INDEX IF NOT EXISTS idx_gatherer_executions_started_at ON gatherer_executions(started_at);

-- Composite index for recent executions by gatherer
CREATE INDEX IF NOT EXISTS idx_gatherer_executions_gatherer_time ON gatherer_executions(gatherer_id, started_at DESC);
`

// SeedSQL contains SQL to seed default gatherer configurations.
// This is run after schema creation to ensure default gatherers are available.
const SeedSQL = `
-- Seed default gatherer configurations if not present

-- Project Instructions gatherer (CLAUDE.md)
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	file_paths, is_enabled, created_at, updated_at
) VALUES (
	'default-project-instructions',
	'project_instructions',
	'Project Instructions',
	'Reads CLAUDE.md and similar instruction files',
	100,
	8000,
	300,
	'["CLAUDE.md", ".claude/CLAUDE.md", "INSTRUCTIONS.md", ".github/INSTRUCTIONS.md"]',
	1,
	datetime('now'),
	datetime('now')
);

-- Git Status gatherer
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	is_enabled, created_at, updated_at
) VALUES (
	'default-git-status',
	'git_status',
	'Git Status',
	'Gathers current git repository status',
	75,
	2000,
	30,
	1,
	datetime('now'),
	datetime('now')
);

-- Environment Info gatherer
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	is_enabled, created_at, updated_at
) VALUES (
	'default-environment',
	'environment',
	'Environment Info',
	'Collects OS, platform, and runtime information',
	75,
	500,
	3600,
	1,
	datetime('now'),
	datetime('now')
);

-- Package Info gatherer
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	file_paths, is_enabled, created_at, updated_at
) VALUES (
	'default-package-info',
	'package_info',
	'Package Info',
	'Reads package.json, go.mod, requirements.txt, etc.',
	50,
	2000,
	300,
	'["package.json", "go.mod", "requirements.txt", "Cargo.toml", "pom.xml"]',
	1,
	datetime('now'),
	datetime('now')
);

-- Directory Structure gatherer
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	options, is_enabled, created_at, updated_at
) VALUES (
	'default-directory-structure',
	'directory_structure',
	'Directory Structure',
	'Generates project directory tree',
	50,
	3000,
	120,
	'{"max_depth": 4, "exclude": ["node_modules", ".git", "__pycache__", "vendor"]}',
	1,
	datetime('now'),
	datetime('now')
);

-- Recent Files gatherer
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	options, is_enabled, created_at, updated_at
) VALUES (
	'default-recent-files',
	'recent_files',
	'Recent Files',
	'Lists recently modified files',
	50,
	1000,
	60,
	'{"max_files": 20, "max_age_hours": 24}',
	1,
	datetime('now'),
	datetime('now')
);

-- Git Diff gatherer (disabled by default)
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	is_enabled, created_at, updated_at
) VALUES (
	'default-git-diff',
	'git_diff',
	'Git Diff',
	'Shows unstaged changes in the repository',
	25,
	4000,
	60,
	0,
	datetime('now'),
	datetime('now')
);

-- Git Log gatherer (disabled by default)
INSERT OR IGNORE INTO gatherer_configs (
	id, type, name, description, priority, max_tokens, refresh_interval_seconds,
	options, is_enabled, created_at, updated_at
) VALUES (
	'default-git-log',
	'git_log',
	'Git Log',
	'Shows recent commit history',
	25,
	2000,
	120,
	'{"max_commits": 10}',
	0,
	datetime('now'),
	datetime('now')
);

-- Default context budget
INSERT OR IGNORE INTO context_budgets (
	id, total_token_budget, reserved_for_system_prompt, reserved_for_conversation,
	min_priority_to_include, is_default, created_at, updated_at
) VALUES (
	'default-budget',
	128000,
	2000,
	32000,
	10,
	1,
	datetime('now'),
	datetime('now')
);
`

// CleanupSQL contains SQL to clean up expired and stale context data.
// This should be run periodically to keep the database clean.
const CleanupSQL = `
-- Mark context items as stale if they've exceeded their expiration
UPDATE context_items
SET is_stale = 1, updated_at = datetime('now')
WHERE expires_at IS NOT NULL
  AND datetime(expires_at) < datetime('now')
  AND is_stale = 0;

-- Delete context items that have been stale for more than 24 hours
DELETE FROM context_items
WHERE is_stale = 1
  AND datetime(updated_at) < datetime('now', '-1 day');

-- Delete old gatherer executions (keep last 7 days)
DELETE FROM gatherer_executions
WHERE datetime(started_at) < datetime('now', '-7 days');

-- Delete orphaned context items (no session and older than 1 hour)
DELETE FROM context_items
WHERE session_id IS NULL
  AND datetime(gathered_at) < datetime('now', '-1 hour');
`

// MigrationSQL contains SQL to migrate from older schema versions.
// This is used for schema evolution and backward compatibility.
const MigrationSQL = `
-- Migration v1: Add content_hash column if it doesn't exist
-- Note: SQLite doesn't support IF NOT EXISTS for columns, so this is a no-op placeholder
-- Actual migrations should be handled via proper migration tooling

-- Migration v2: Add min_priority_to_include to budgets
-- Handled by CREATE TABLE IF NOT EXISTS with default value
`
