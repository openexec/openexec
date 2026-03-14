// Package release provides release management with git integration and optional approval workflows.
package release

// StateSchema defines the state-related table structures for SQLite.
// These tables support releases, goals, stories, and tasks with their relationships.
// This schema replaces JSON-based persistence for these entities.
const StateSchema = `
-- Releases table stores the current release metadata.
-- Only one release is typically active at a time (id='current').
CREATE TABLE IF NOT EXISTS releases (
	id TEXT PRIMARY KEY DEFAULT 'current',
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	description TEXT DEFAULT '',
	status TEXT NOT NULL DEFAULT 'draft',
	stories TEXT DEFAULT '[]',
	git_branch TEXT DEFAULT '',
	git_base_branch TEXT DEFAULT '',
	git_tag TEXT DEFAULT '',
	git_merge_commit TEXT DEFAULT '',
	git_head_commit TEXT DEFAULT '',
	approval_status TEXT DEFAULT '',
	approval_approved_by TEXT DEFAULT '',
	approval_approved_at DATETIME DEFAULT NULL,
	approval_comments TEXT DEFAULT '',
	approval_rejection_reason TEXT DEFAULT '',
	approval_review_cycle INTEGER DEFAULT 0,
	deployed_to TEXT DEFAULT '[]',
	changelog TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	started_at DATETIME DEFAULT NULL,
	completed_at DATETIME DEFAULT NULL,
	deployed_at DATETIME DEFAULT NULL
);

-- Goals table stores high-level project objectives.
CREATE TABLE IF NOT EXISTS goals (
	id TEXT PRIMARY KEY,
	title TEXT NOT NULL DEFAULT '',
	description TEXT DEFAULT '',
	success_criteria TEXT DEFAULT '',
	verification_method TEXT DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Stories table stores user stories with optional relationships to goals.
-- goal_id is optional; stories can exist without a goal reference.
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
	completed_at DATETIME DEFAULT NULL
);

-- Tasks table stores work units with relationships to stories.
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

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_stories_goal_id ON stories(goal_id);
CREATE INDEX IF NOT EXISTS idx_stories_status ON stories(status);
CREATE INDEX IF NOT EXISTS idx_stories_priority ON stories(priority);

CREATE INDEX IF NOT EXISTS idx_tasks_story_id ON tasks(story_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_priority ON tasks(priority);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_agent ON tasks(assigned_agent);
`
