package state

// UnifiedSchema consolidates all state-related tables into one schema.
// This supports Sessions, Runs, Steps, Messages, ToolCalls, AuditLogs, and Release state.
const UnifiedSchema = `
-- 1. PERSISTENT SESSIONS (Conversational Context)
CREATE TABLE IF NOT EXISTS sessions (
	id TEXT PRIMARY KEY,
	project_path TEXT NOT NULL,
	provider TEXT NOT NULL,
	model TEXT NOT NULL,
	title TEXT NOT NULL DEFAULT '',
	parent_session_id TEXT DEFAULT NULL,
	fork_point_message_id TEXT DEFAULT NULL,
	status TEXT NOT NULL DEFAULT 'active',
	metadata TEXT DEFAULT '{}',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (parent_session_id) REFERENCES sessions(id) ON DELETE SET NULL
);

-- 2. MESSAGES (History within a Session)
CREATE TABLE IF NOT EXISTS messages (
	id TEXT PRIMARY KEY,
	session_id TEXT NOT NULL,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	tokens_input INTEGER DEFAULT 0,
	tokens_output INTEGER DEFAULT 0,
	tokens_cached INTEGER DEFAULT 0,
	cost_usd REAL DEFAULT 0.0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- 3. TOOL CALLS (Interactions within a Message)
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
	idempotency_key TEXT DEFAULT NULL, -- sha256(tool+args+version) for resume
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (message_id) REFERENCES messages(id) ON DELETE CASCADE,
	FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- 4a. RUN SPECS (Immutable Execution Specifications)
-- RunSpecs capture the deterministic inputs for a run, enabling replay and caching.
CREATE TABLE IF NOT EXISTS run_specs (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    intent TEXT NOT NULL,
    context_hash TEXT NOT NULL, -- Hash of context files used
    prompt_hash TEXT NOT NULL, -- Hash of the composed prompt
    model TEXT NOT NULL,
    mode TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
);

-- 4b. RUNS (Deterministic Execution Instances)
-- A Run represents one execution of a task or a conversational intent.
-- It can be linked to a Session for context and history.
CREATE TABLE IF NOT EXISTS runs (
    id TEXT PRIMARY KEY,
    session_id TEXT,
    task_id TEXT, -- Optional link to a specific task in the release graph
    spec_id TEXT, -- Link to immutable RunSpec for deterministic replay
    project_path TEXT NOT NULL,
    mode TEXT NOT NULL DEFAULT 'workspace-write', -- suggest, workspace-write, danger-full-access
    status TEXT NOT NULL DEFAULT 'starting', -- starting, running, paused, complete, error, stopped
    error_message TEXT,
    metadata TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME DEFAULT NULL,
    completed_at DATETIME DEFAULT NULL,
    worktree_path TEXT DEFAULT NULL, -- Path to git worktree for parallel runs
    FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE SET NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE SET NULL,
    FOREIGN KEY (spec_id) REFERENCES run_specs(id) ON DELETE SET NULL
);

-- 5. RUN STEPS (Individual iterations of a Run)
CREATE TABLE IF NOT EXISTS run_steps (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    trace_id TEXT,
    phase TEXT NOT NULL, -- intake, plan, execute, verify, finalize
    agent TEXT,
    iteration INTEGER NOT NULL,
    status TEXT NOT NULL,
    inputs_hash TEXT, -- Content-hash of inputs (briefing + context)
    outputs_hash TEXT, -- Content-hash of outputs (patches + logs)
    cache_key TEXT, -- Stable hash for deterministic replay (computed from semantic content)
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME DEFAULT NULL,
    metadata TEXT DEFAULT '{}',
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

-- 6. ARTIFACTS (Content-addressed storage pointers)
CREATE TABLE IF NOT EXISTS artifacts (
    hash TEXT PRIMARY KEY,
    type TEXT NOT NULL, -- patch, context_bundle, test_log, summary
    path TEXT NOT NULL, -- Local path under .openexec/artifacts/
    size INTEGER NOT NULL,
    metadata TEXT DEFAULT '{}',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 6b. RUN CHECKPOINTS (Resume support)
CREATE TABLE IF NOT EXISTS run_checkpoints (
    id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    phase TEXT NOT NULL,
    iteration INTEGER NOT NULL,
    timestamp TEXT NOT NULL,
    artifacts TEXT DEFAULT '{}', -- JSON map of hash -> path
    message_history TEXT DEFAULT '[]', -- JSON array of messages for resume
    tool_call_log TEXT DEFAULT '[]', -- JSON array of completed tool calls
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

-- 7. AUDIT LOGS (Immutable event trail)
CREATE TABLE IF NOT EXISTS audit_entries (
	id TEXT PRIMARY KEY NOT NULL,
    run_id TEXT,
    step_id TEXT,
	session_id TEXT,
	timestamp TEXT NOT NULL,
	event_type TEXT NOT NULL,
	severity TEXT NOT NULL DEFAULT 'info',
	actor_id TEXT NOT NULL,
	actor_type TEXT NOT NULL,
	project_path TEXT,
	provider TEXT,
	model TEXT,
	tokens_input INTEGER,
	tokens_output INTEGER,
	tokens_cached INTEGER,
	cost_usd REAL,
	duration_ms INTEGER,
	success INTEGER,
	error_message TEXT,
	metadata TEXT,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL,
    FOREIGN KEY (step_id) REFERENCES run_steps(id) ON DELETE SET NULL
);

-- 8. RELEASE STATE (Goals, Stories, Tasks)
-- [Reusing schemas from internal/release/schema.go but integrated]

CREATE TABLE IF NOT EXISTS goals (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	description TEXT DEFAULT '',
	success_criteria TEXT DEFAULT '',
	verification_method TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS stories (
	id TEXT PRIMARY KEY,
	epic_id TEXT DEFAULT NULL,
	goal_id TEXT DEFAULT NULL,
	title TEXT NOT NULL,
	description TEXT DEFAULT '',
	role TEXT DEFAULT '',
	want TEXT DEFAULT '',
	benefit TEXT DEFAULT '',
	acceptance_criteria TEXT DEFAULT '[]',
	verification_script TEXT DEFAULT '',
	contract TEXT DEFAULT '',
	tasks TEXT DEFAULT '[]',
	depends_on TEXT DEFAULT '[]',
	story_type TEXT DEFAULT 'feature',
	priority INTEGER DEFAULT 0,
	git_branch TEXT DEFAULT '',
	git_base_branch TEXT DEFAULT '',
	git_merged_to TEXT DEFAULT '',
	git_merge_commit TEXT DEFAULT '',
	git_merged_at DATETIME DEFAULT NULL,
	git_commit_count INTEGER DEFAULT 0,
	approval_status TEXT DEFAULT '',
	approval_approved_by TEXT DEFAULT '',
	approval_approved_at DATETIME DEFAULT NULL,
	approval_comments TEXT DEFAULT '',
	approval_rejection_reason TEXT DEFAULT '',
	approval_review_cycle INTEGER DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME DEFAULT NULL,
	completed_at DATETIME DEFAULT NULL,
	FOREIGN KEY (goal_id) REFERENCES goals(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	story_id TEXT NOT NULL,
	title TEXT NOT NULL,
	description TEXT DEFAULT '',
	verification_script TEXT DEFAULT '',
	depends_on TEXT DEFAULT '[]',
	task_type TEXT DEFAULT '',
	priority INTEGER DEFAULT 0,
	assigned_agent TEXT DEFAULT '',
	attempt_count INTEGER DEFAULT 0,
	max_attempts INTEGER DEFAULT 3,
	git_commits TEXT DEFAULT '[]',
	git_branch TEXT DEFAULT '',
	git_pr_number INTEGER DEFAULT NULL,
	git_pr_url TEXT DEFAULT '',
	approval_status TEXT DEFAULT '',
	approval_approved_by TEXT DEFAULT '',
	approval_approved_at DATETIME DEFAULT NULL,
	approval_comments TEXT DEFAULT '',
	approval_rejection_reason TEXT DEFAULT '',
	approval_review_cycle INTEGER DEFAULT 0,
	needs_review INTEGER DEFAULT 0,
	review_notes TEXT DEFAULT '',
	status TEXT NOT NULL DEFAULT 'pending',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME DEFAULT NULL,
	completed_at DATETIME DEFAULT NULL,
	error_message TEXT DEFAULT '',
	metadata TEXT DEFAULT '{}',
	FOREIGN KEY (story_id) REFERENCES stories(id) ON DELETE CASCADE
);

-- 9. KNOWLEDGE BASE (Symbols, Env, API Docs, PRD)
CREATE TABLE IF NOT EXISTS symbols (
    name TEXT PRIMARY KEY,
    kind TEXT,
    file_path TEXT,
    start_line INTEGER,
    end_line INTEGER,
    purpose TEXT,
    input_params TEXT,
    output_params TEXT,
    signature TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS environments (
    env TEXT PRIMARY KEY,
    runtime_type TEXT,
    auth_steps TEXT,
    deploy_steps TEXT,
    topology TEXT,
    instructions TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS api_docs (
    path TEXT,
    method TEXT,
    request_schema TEXT,
    response_schema TEXT,
    description TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (path, method)
);

CREATE TABLE IF NOT EXISTS policies (
    key TEXT PRIMARY KEY,
    value TEXT,
    description TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS prd_specs (
    section TEXT,
    key TEXT,
    content TEXT,
    metadata TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (section, key)
);

CREATE TABLE IF NOT EXISTS task_queue (
    id TEXT PRIMARY KEY,
    type TEXT,
    status TEXT,      -- pending, running, completed, failed
    payload TEXT,     -- JSON input for the agent
    error_log TEXT,
    metadata TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id);
CREATE INDEX IF NOT EXISTS idx_runs_session ON runs(session_id);
CREATE INDEX IF NOT EXISTS idx_runs_task ON runs(task_id);
CREATE INDEX IF NOT EXISTS idx_run_steps_run ON run_steps(run_id);
CREATE INDEX IF NOT EXISTS idx_audit_run ON audit_entries(run_id);
CREATE INDEX IF NOT EXISTS idx_audit_session ON audit_entries(session_id);
CREATE INDEX IF NOT EXISTS idx_run_checkpoints_run ON run_checkpoints(run_id);

-- Composite indexes for pagination queries (high-volume tables)
CREATE INDEX IF NOT EXISTS idx_runs_project_created ON runs(project_path, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_runs_status_created ON runs(status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_run_steps_run_started ON run_steps(run_id, started_at ASC);
CREATE INDEX IF NOT EXISTS idx_checkpoints_run_timestamp ON run_checkpoints(run_id, timestamp DESC);

-- Unique indexes to prevent duplicate writes under concurrency
CREATE UNIQUE INDEX IF NOT EXISTS idx_run_steps_unique ON run_steps(run_id, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_calls_idempotency_unique ON tool_calls(idempotency_key) WHERE idempotency_key IS NOT NULL;
`
