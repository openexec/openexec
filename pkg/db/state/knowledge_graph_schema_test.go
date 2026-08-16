package state

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEnsureKnowledgeGraphSchemaIsAdditive(t *testing.T) {
	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE symbols (name TEXT PRIMARY KEY, file_path TEXT);
		INSERT INTO symbols (name, file_path) VALUES ('Run', 'run.go');
		CREATE TABLE validation_plan_revisions (
			id TEXT PRIMARY KEY, task_id TEXT NOT NULL, run_id TEXT, revision INTEGER NOT NULL,
			generation_id TEXT NOT NULL, worktree_state_hash TEXT NOT NULL, patch_hash TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL, created_at DATETIME, accepted_at DATETIME
		)`); err != nil {
		t.Fatal(err)
	}
	if err := EnsureKnowledgeGraphSchema(db); err != nil {
		t.Fatal(err)
	}
	if err := EnsureKnowledgeGraphSchema(db); err != nil {
		t.Fatalf("schema migration is not idempotent: %v", err)
	}
	var file string
	if err := db.QueryRow(`SELECT file_path FROM symbols WHERE name = 'Run'`).Scan(&file); err != nil {
		t.Fatalf("legacy symbol was lost: %v", err)
	}
	if file != "run.go" {
		t.Fatalf("legacy symbol changed: %q", file)
	}
	for _, table := range []string{"repositories", "graph_generations", "repository_symbols", "symbol_occurrences", "validation_plan_revisions", "completion_claims", "completion_reports"} {
		var found string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found); err != nil {
			t.Errorf("expected additive table %s: %v", table, err)
		}
	}
	for _, column := range []string{"impact_query", "impact_summary", "source_revision_id"} {
		rows, err := db.Query(`PRAGMA table_info(validation_plan_revisions)`)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				t.Fatal(err)
			}
			found = found || name == column
		}
		rows.Close()
		if !found {
			t.Errorf("expected migrated column validation_plan_revisions.%s", column)
		}
	}
}
