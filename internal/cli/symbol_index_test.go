package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/mcp"
)

// TestServableGraphRequiresAGeneration proves the gate accepts a workspace only
// once a graph exists, and that probing never writes.
func TestServableGraphRequiresAGeneration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSample(t, root)

	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if hasServableGraph(root) {
		t.Error("a database with no generation was treated as servable")
	}
	if _, err := store.ScanRepository(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if !hasServableGraph(root) {
		t.Error("a scanned workspace was not treated as servable")
	}
	alias := filepath.Join(t.TempDir(), "workspace-alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	if !hasServableGraph(alias) {
		t.Error("a canonical scanned workspace was not servable through its path alias")
	}
}

func writeSample(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module sample\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc Sample() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSymbolIndexRootUsesResolvedWorkspace is a regression test for a bug unit
// tests could not see: the composition root originally probed for .openexec
// under the process working directory, while the MCP server scopes itself to
// WORKSPACE_ROOT when set. A client spawning `openexec mcp-serve` from an
// unrelated cwd — Agent Console does exactly that — got no symbol tools even
// though the server was correctly scoped to a workspace that had a graph.
func TestSymbolIndexRootUsesResolvedWorkspace(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	// An initialized-but-never-scanned workspace must not be selected: opening
	// a store there would create a database and insert identity rows on a path
	// that claims to be read-only.
	if got := symbolIndexRoot([]string{project}); got != "" {
		t.Errorf("workspace with no graph database selected: got %q", got)
	}

	// A real, valid database that another feature created, holding no graph.
	// A file-existence gate passes this and then answers "no graph" to every
	// lookup; only a generation check rejects it.
	store, err := knowledge.NewStore(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if got := symbolIndexRoot([]string{project}); got != "" {
		t.Errorf("workspace with a database but no graph generation selected: got %q", got)
	}

	if got := symbolIndexRoot([]string{t.TempDir()}); got != "" {
		t.Errorf("workspace without .openexec selected: got %q", got)
	}
	for _, roots := range [][]string{nil, {}, {""}} {
		if got := symbolIndexRoot(roots); got != "" {
			t.Errorf("empty roots %v selected %q", roots, got)
		}
	}
}

// TestResolveServeRootPrecedence: one root feeds the symbol index, project
// modules, the infra allowlist and the approvals database. If any of them
// resolved differently, --workspace would be a security boundary with a hole
// in it — the flag must win, then WORKSPACE_ROOT, then cwd.
func TestResolveServeRootPrecedence(t *testing.T) {
	flagRoot, envRoot := t.TempDir(), t.TempDir()

	t.Setenv("WORKSPACE_ROOT", envRoot)
	if got := resolveServeRoot(flagRoot); got != flagRoot {
		t.Errorf("flag must outrank WORKSPACE_ROOT: got %q", got)
	}
	if got := resolveServeRoot(""); got != envRoot {
		t.Errorf("WORKSPACE_ROOT must be used when no flag is given: got %q", got)
	}

	t.Setenv("WORKSPACE_ROOT", "")
	cwd, _ := os.Getwd()
	if got := resolveServeRoot(""); got != cwd {
		t.Errorf("working directory is the last resort: got %q, want %q", got, cwd)
	}
}

// TestValidateServeModeFailsClosed: NewToolBroker normalizes an unknown mode
// to auto-edit, which permits git_apply_patch. A misspelled security flag must
// be an error, not a silent widening.
func TestValidateServeModeFailsClosed(t *testing.T) {
	for _, valid := range []string{"", "suggest", "auto-edit", "danger-full-access"} {
		if err := validateServeMode(valid); err != nil {
			t.Errorf("valid mode %q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"sugest", "read-only", "plan", "SUGGEST", "danger"} {
		if err := validateServeMode(invalid); err == nil {
			t.Errorf("invalid mode %q accepted — it would become auto-edit", invalid)
		}
	}
}

// TestWorkspaceRootOverridesWorkDir pins the server contract the wiring above
// depends on: WORKSPACE_ROOT wins over the working directory, so resolving the
// graph root from cwd would disagree with the server's own path scoping.
func TestWorkspaceRootOverridesWorkDir(t *testing.T) {
	project := t.TempDir()
	t.Setenv("WORKSPACE_ROOT", project)

	srv, err := mcp.NewServerWithConfig(strings.NewReader(""), &strings.Builder{},
		mcp.ServerConfig{WorkDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewServerWithConfig: %v", err)
	}
	roots := srv.WorkspaceRoots()
	if len(roots) == 0 || roots[0] != project {
		t.Fatalf("server root = %v, want %q — the symbol wiring resolves the graph from this", roots, project)
	}
}
