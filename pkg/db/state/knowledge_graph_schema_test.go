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
	if _, err := db.Exec(`CREATE TABLE symbols (name TEXT PRIMARY KEY, file_path TEXT); INSERT INTO symbols (name, file_path) VALUES ('Run', 'run.go')`); err != nil {
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
	for _, table := range []string{"repositories", "graph_generations", "repository_symbols", "symbol_occurrences", "validation_plan_revisions", "completion_claims"} {
		var found string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found); err != nil {
			t.Errorf("expected additive table %s: %v", table, err)
		}
	}
}
