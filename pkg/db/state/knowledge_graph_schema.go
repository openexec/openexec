package state

import (
	"database/sql"
	"fmt"
)

// KnowledgeGraphSchemaVersion is the current additive repository graph schema.
const KnowledgeGraphSchemaVersion = 1

// KnowledgeGraphSchema extends the legacy symbols table without replacing it.
// The legacy table remains readable until a separately approved deprecation.
const KnowledgeGraphSchema = `
CREATE TABLE IF NOT EXISTS repositories (
    id TEXT PRIMARY KEY,
    persisted_uuid TEXT NOT NULL UNIQUE,
    discovery_remote TEXT NOT NULL DEFAULT '',
    forked_from_repository_id TEXT DEFAULT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (forked_from_repository_id) REFERENCES repositories(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS repository_aliases (
    repository_id TEXT NOT NULL,
    alias_type TEXT NOT NULL,
    alias_value TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (repository_id, alias_type, alias_value),
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS checkouts (
    id TEXT PRIMARY KEY,
    repository_id TEXT NOT NULL,
    root_path TEXT NOT NULL UNIQUE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    removed_at DATETIME DEFAULT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS worktrees (
    id TEXT PRIMARY KEY,
    repository_id TEXT NOT NULL,
    checkout_id TEXT NOT NULL,
    root_path TEXT NOT NULL UNIQUE,
    git_dir TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    removed_at DATETIME DEFAULT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
    FOREIGN KEY (checkout_id) REFERENCES checkouts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS graph_generations (
    id TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL,
    repository_id TEXT NOT NULL,
    checkout_id TEXT NOT NULL,
    worktree_id TEXT NOT NULL,
    base_commit TEXT NOT NULL DEFAULT '',
    worktree_state_hash TEXT NOT NULL,
    configuration_digest TEXT NOT NULL,
    extractor_version TEXT NOT NULL,
    manifest_hash TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('building','current','stale','partial','inconsistent','failed','incompatible','superseded')),
    capabilities TEXT NOT NULL DEFAULT '{}',
    limitations TEXT NOT NULL DEFAULT '[]',
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME DEFAULT NULL,
    promoted_at DATETIME DEFAULT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
    FOREIGN KEY (checkout_id) REFERENCES checkouts(id) ON DELETE CASCADE,
    FOREIGN KEY (worktree_id) REFERENCES worktrees(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS graph_scan_inputs (
    generation_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    input_kind TEXT NOT NULL,
    size INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    symlink_target TEXT NOT NULL DEFAULT '',
    included INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (generation_id, file_path),
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS graph_scan_errors (
    id TEXT PRIMARY KEY,
    generation_id TEXT NOT NULL,
    file_path TEXT NOT NULL DEFAULT '',
    error_kind TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS graph_nodes (
    id TEXT PRIMARY KEY,
    generation_id TEXT NOT NULL,
    repository_id TEXT NOT NULL,
    node_type TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL,
    qualified_name TEXT NOT NULL DEFAULT '',
    metadata TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE CASCADE,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS repository_symbols (
    id TEXT PRIMARY KEY,
    repository_id TEXT NOT NULL,
    language TEXT NOT NULL,
    kind TEXT NOT NULL,
    display_name TEXT NOT NULL,
    qualified_name TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    retired_at DATETIME DEFAULT NULL,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS symbol_occurrences (
    symbol_id TEXT NOT NULL,
    generation_id TEXT NOT NULL,
    node_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    start_line INTEGER NOT NULL,
    end_line INTEGER NOT NULL,
    start_byte INTEGER NOT NULL,
    end_byte INTEGER NOT NULL,
    signature TEXT NOT NULL DEFAULT '',
    file_content_hash TEXT NOT NULL,
    source_range_hash TEXT NOT NULL,
    exported INTEGER NOT NULL DEFAULT 0,
    resolution_status TEXT NOT NULL,
    PRIMARY KEY (symbol_id, generation_id),
    FOREIGN KEY (symbol_id) REFERENCES repository_symbols(id) ON DELETE CASCADE,
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES graph_nodes(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS symbol_lineage (
    id TEXT PRIMARY KEY,
    repository_id TEXT NOT NULL,
    symbol_id TEXT NOT NULL,
    previous_symbol_id TEXT DEFAULT NULL,
    continuity_status TEXT NOT NULL CHECK (continuity_status IN ('preserved','moved','renamed','split','merged','ambiguous','new')),
    resolution_method TEXT NOT NULL CHECK (resolution_method IN ('exact','structural','heuristic','reviewed')),
    generation_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (repository_id) REFERENCES repositories(id) ON DELETE CASCADE,
    FOREIGN KEY (symbol_id) REFERENCES repository_symbols(id) ON DELETE CASCADE,
    FOREIGN KEY (previous_symbol_id) REFERENCES repository_symbols(id) ON DELETE SET NULL,
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS graph_edges (
    id TEXT PRIMARY KEY,
    generation_id TEXT NOT NULL,
    from_node_id TEXT NOT NULL,
    to_node_id TEXT NOT NULL,
    edge_type TEXT NOT NULL,
    resolution_status TEXT NOT NULL,
    source_file_path TEXT NOT NULL DEFAULT '',
    source_start_byte INTEGER NOT NULL DEFAULT 0,
    source_end_byte INTEGER NOT NULL DEFAULT 0,
    metadata TEXT NOT NULL DEFAULT '{}',
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE CASCADE,
    FOREIGN KEY (from_node_id) REFERENCES graph_nodes(id) ON DELETE CASCADE,
    FOREIGN KEY (to_node_id) REFERENCES graph_nodes(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS task_graph_bindings (
    task_id TEXT NOT NULL,
    binding_kind TEXT NOT NULL CHECK (binding_kind IN ('planning','validation')),
    generation_id TEXT NOT NULL,
    base_commit TEXT NOT NULL DEFAULT '',
    worktree_state_hash TEXT NOT NULL,
    patch_hash TEXT NOT NULL DEFAULT '',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (task_id, binding_kind, generation_id),
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS validation_plan_revisions (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    run_id TEXT DEFAULT NULL,
    revision INTEGER NOT NULL,
    generation_id TEXT NOT NULL,
    worktree_state_hash TEXT NOT NULL,
    patch_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('proposed','accepted','superseded')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    accepted_at DATETIME DEFAULT NULL,
    UNIQUE (task_id, revision),
    FOREIGN KEY (generation_id) REFERENCES graph_generations(id) ON DELETE RESTRICT
);

CREATE TABLE IF NOT EXISTS validation_items (
    id TEXT PRIMARY KEY,
    plan_revision_id TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('graph','blueprint','policy','user','agent')),
    disposition TEXT NOT NULL CHECK (disposition IN ('suggested','accepted','rejected')),
    requirement TEXT NOT NULL CHECK (requirement IN ('optional','required','blocking')),
    criterion TEXT NOT NULL,
    command_argv TEXT NOT NULL DEFAULT '[]',
    scope TEXT NOT NULL DEFAULT '',
    graph_paths TEXT NOT NULL DEFAULT '[]',
    limitations TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (plan_revision_id) REFERENCES validation_plan_revisions(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS validation_evidence_links (
    validation_item_id TEXT NOT NULL,
    run_id TEXT NOT NULL,
    run_step_id TEXT NOT NULL,
    artifact_hash TEXT DEFAULT NULL,
    worktree_state_hash TEXT NOT NULL,
    patch_hash TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('passed','failed','inconclusive','not_run','unavailable')),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (validation_item_id, run_step_id),
    FOREIGN KEY (validation_item_id) REFERENCES validation_items(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS completion_claims (
    id TEXT PRIMARY KEY,
    validation_item_id TEXT NOT NULL,
    predicate TEXT NOT NULL,
    scope TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('supported','unsupported','inconclusive','not_run','unavailable')),
    repository_state_hash TEXT NOT NULL,
    evidence_artifact_ids TEXT NOT NULL DEFAULT '[]',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (validation_item_id) REFERENCES validation_items(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_checkouts_repository ON checkouts(repository_id);
CREATE INDEX IF NOT EXISTS idx_worktrees_repository ON worktrees(repository_id);
CREATE INDEX IF NOT EXISTS idx_generations_worktree_status ON graph_generations(worktree_id, status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_one_current_generation ON graph_generations(worktree_id) WHERE status = 'current';
CREATE INDEX IF NOT EXISTS idx_nodes_generation_type ON graph_nodes(generation_id, node_type);
CREATE INDEX IF NOT EXISTS idx_symbols_repository_name ON repository_symbols(repository_id, display_name);
CREATE INDEX IF NOT EXISTS idx_occurrences_generation_file ON symbol_occurrences(generation_id, file_path);
CREATE INDEX IF NOT EXISTS idx_edges_generation_from ON graph_edges(generation_id, from_node_id, edge_type);
CREATE INDEX IF NOT EXISTS idx_edges_generation_to ON graph_edges(generation_id, to_node_id, edge_type);
CREATE INDEX IF NOT EXISTS idx_validation_items_plan ON validation_items(plan_revision_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_completion_claim_identity ON completion_claims(validation_item_id, predicate, repository_state_hash);
`

// EnsureKnowledgeGraphSchema applies only additive graph schema changes.
func EnsureKnowledgeGraphSchema(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("knowledge graph schema: nil database")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin knowledge graph migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(KnowledgeGraphSchema); err != nil {
		return fmt.Errorf("apply knowledge graph schema: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit knowledge graph migration: %w", err)
	}
	return nil
}
