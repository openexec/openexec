package manager

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/state"
)

// writeGoFixture creates a tiny Go source file in dir with two functions
// the indexer should pick up via its Go AST provider.
func writeGoFixture(t *testing.T, dir string) {
	t.Helper()
	src := `package fixture

// Greet returns a friendly hello.
func Greet(name string) string {
	return "hello, " + name
}

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}
`
	path := filepath.Join(dir, "fixture.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

// TestSymbolIndexer_PopulatesSymbolsTable verifies the auto-indexing goroutine
// kicked off by Manager.New() actually walks the WorkDir and writes rows to
// the symbols table. Without the Layer 1 wiring this test would fail because
// the table would stay empty (the indexer was previously only invoked from
// the DCP coordinator path).
func TestSymbolIndexer_PopulatesSymbolsTable(t *testing.T) {
	tmpDir := t.TempDir()
	writeGoFixture(t, tmpDir)

	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stateStore.Close() })

	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })

	// Indexing runs in a background goroutine. Poll until at least one
	// symbol is present or the timeout fires.
	deadline := time.Now().Add(10 * time.Second)
	var count int
	for time.Now().Before(deadline) {
		_ = stateStore.GetDB().QueryRow(`SELECT COUNT(*) FROM symbols`).Scan(&count)
		if count > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if count == 0 {
		t.Fatalf("expected symbols table to be populated by background indexer, got 0 rows")
	}

	// Sanity: the two functions in our fixture must both be present.
	for _, name := range []string{"Greet", "Add"} {
		var path string
		err := stateStore.GetDB().QueryRow(`SELECT file_path FROM symbols WHERE name = ?`, name).Scan(&path)
		if err != nil {
			t.Errorf("expected symbol %q to be indexed: %v", name, err)
			continue
		}
		if filepath.Base(path) != "fixture.go" {
			t.Errorf("symbol %q indexed at unexpected path %q", name, path)
		}
	}
}

// TestSymbolIndexer_DisabledByFlag verifies that an explicit
// "symbol_indexing": false in the project config skips the indexer entirely.
// The symbols table stays at zero rows even though a fixture file exists.
func TestSymbolIndexer_DisabledByFlag(t *testing.T) {
	tmpDir := t.TempDir()
	writeGoFixture(t, tmpDir)

	// Materialise a .openexec/config.json with symbol_indexing disabled so
	// project.LoadProjectConfig will pick it up and IsSymbolIndexingEnabled
	// returns false.
	openexecDir := filepath.Join(tmpDir, ".openexec")
	if err := os.MkdirAll(openexecDir, 0755); err != nil {
		t.Fatalf("mkdir .openexec: %v", err)
	}
	cfg := `{"name":"test","execution":{"symbol_indexing":false}}`
	if err := os.WriteFile(filepath.Join(openexecDir, "config.json"), []byte(cfg), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	stateStore, err := state.NewStore(filepath.Join(tmpDir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stateStore.Close() })

	m, err := New(Config{WorkDir: tmpDir, StateStore: stateStore})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })

	// Give any (mistakenly spawned) indexer time to run, then assert empty.
	time.Sleep(500 * time.Millisecond)

	var count int
	if err := stateStore.GetDB().QueryRow(`SELECT COUNT(*) FROM symbols`).Scan(&count); err != nil {
		t.Fatalf("query symbols count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 symbols when indexing is disabled, got %d", count)
	}
}
