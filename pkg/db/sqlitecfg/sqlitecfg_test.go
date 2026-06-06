package sqlitecfg

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestDSNAppliesPragmas asserts the RUNTIME pragma state, not the DSN string:
// this is the regression guard for the silent-pragma bug (mattn-style DSN
// params were ignored by the modernc driver, leaving WAL/FK/busy_timeout off
// in production). If the driver or its DSN syntax ever changes, this fails.
func TestDSNAppliesPragmas(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", DSN(dbPath))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want wal — multi-process concurrency is broken without it", journalMode)
	}

	var foreignKeys int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}

	var busyTimeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000 — concurrent writers fail instantly without it", busyTimeout)
	}
}
