package pipeline

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// helper: open an in-memory SQLite DB with the symbols table.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE symbols (
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
	)`)
	if err != nil {
		t.Fatalf("create symbols table: %v", err)
	}
	return db
}

func insertSymbol(t *testing.T, db *sql.DB, name, kind, filePath string, startLine, endLine int, signature string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO symbols (name, kind, file_path, start_line, end_line, signature) VALUES (?, ?, ?, ?, ?, ?)`,
		name, kind, filePath, startLine, endLine, signature,
	)
	if err != nil {
		t.Fatalf("insert symbol %s: %v", name, err)
	}
}

func TestPreResolve_ExtractsSymbolNames(t *testing.T) {
	names := extractCandidateNames("implement the Greet function and Add helper")

	has := func(want string) bool {
		for _, n := range names {
			if n == want {
				return true
			}
		}
		return false
	}

	if !has("Greet") {
		t.Errorf("expected candidate 'Greet', got %v", names)
	}
	if !has("Add") {
		t.Errorf("expected candidate 'Add', got %v", names)
	}
	// "implement", "the", "function", "and", "helper" are also valid identifiers >= 3 chars,
	// but the important thing is that the actual symbol names are present.
}

func TestPreResolve_QueriesSymbolTable(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a temp dir with a source file.
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "greet.py")
	content := `def Greet(name: str) -> str:
    """Say hello."""
    greeting = f"Hello, {name}!"
    return greeting

def Add(a: int, b: int) -> int:
    """Add two numbers."""
    return a + b
`
	if err := os.WriteFile(srcFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	insertSymbol(t, db, "Greet", "func", "greet.py", 1, 4, "def Greet(name: str) -> str:")
	insertSymbol(t, db, "Add", "func", "greet.py", 6, 8, "def Add(a: int, b: int) -> int:")

	pr := &PreResolver{}
	result := pr.Resolve(context.Background(), "implement the Greet function and Add helper", tmpDir, db)

	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "Greet") {
		t.Error("result should contain Greet")
	}
	if !strings.Contains(result, "Add") {
		t.Error("result should contain Add")
	}
	if !strings.Contains(result, "greet.py:1") {
		t.Errorf("result should contain file path with line number, got:\n%s", result)
	}
	if !strings.Contains(result, "greet.py:6") {
		t.Errorf("result should contain Add's file path with line number, got:\n%s", result)
	}
	if !strings.Contains(result, "Pre-Resolved Symbols") {
		t.Error("result should contain header")
	}
	if !strings.Contains(result, "```python") {
		t.Error("result should contain python code fence")
	}
}

func TestPreResolve_NoMatches(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	pr := &PreResolver{}
	result := pr.Resolve(context.Background(), "fix the layout spacing issue", t.TempDir(), db)

	if result != "" {
		t.Errorf("expected empty result for no matches, got: %s", result)
	}
}

func TestPreResolve_SignaturePlusFirstLines(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a source file with a long function (40 lines).
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "long.py")
	var lines []string
	lines = append(lines, "def LongFunc(x: int) -> int:")
	lines = append(lines, `    """A very long function."""`)
	for i := 2; i < 40; i++ {
		lines = append(lines, "    pass  # line "+strings.Repeat("x", i))
	}
	if err := os.WriteFile(srcFile, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}

	insertSymbol(t, db, "LongFunc", "func", "long.py", 1, 40, "def LongFunc(x: int) -> int:")

	pr := &PreResolver{SliceLines: 20}
	result := pr.Resolve(context.Background(), "refactor the LongFunc implementation", tmpDir, db)

	if result == "" {
		t.Fatal("expected non-empty result")
	}

	// Count lines inside the code fence.
	inFence := false
	codeLines := 0
	for _, line := range strings.Split(result, "\n") {
		if strings.HasPrefix(line, "```") {
			if inFence {
				break
			}
			inFence = true
			continue
		}
		if inFence {
			codeLines++
		}
	}

	if codeLines > 21 { // 20 lines + possible trailing empty line
		t.Errorf("expected at most ~20 code lines, got %d", codeLines)
	}
	if codeLines < 10 {
		t.Errorf("expected at least 10 code lines, got %d", codeLines)
	}

	// Verify the last lines of the 40-line function are NOT present.
	if strings.Contains(result, "# line "+strings.Repeat("x", 35)) {
		t.Error("result should NOT contain lines near the end of the 40-line function")
	}
}

func TestPreResolve_NoSymbolsTable(t *testing.T) {
	// DB without a symbols table — should fail gracefully.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	pr := &PreResolver{}
	result := pr.Resolve(context.Background(), "implement the Greet function", t.TempDir(), db)

	if result != "" {
		t.Errorf("expected empty result when symbols table is missing, got: %s", result)
	}
}

func TestPreResolve_NilDB(t *testing.T) {
	pr := &PreResolver{}
	result := pr.Resolve(context.Background(), "implement the Greet function", t.TempDir(), nil)
	if result != "" {
		t.Errorf("expected empty result for nil DB, got: %s", result)
	}
}
