package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestNoRefreshOnReadRefusesInsteadOfScanning covers the contract a
// symbols-only MCP server depends on.
//
// The V2.1 read gate normally repairs a drifted generation, which on a large
// repository means a full extraction — so a lookup advertised as cheaper than
// grep could block for minutes. With refresh disabled the read must report the
// drift as an explicit stale refusal instead, and must not rebuild.
func TestNoRefreshOnReadRefusesInsteadOfScanning(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package sample\nfunc A() string { return \"one\" }\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	identity, _ := store.EnsureRepositoryIdentity(ctx, root, "")

	// A current graph still answers.
	store.SetRefreshOnRead(false)
	if _, err := store.FindGraphSymbols(ctx, identity, "A", "", "", 1, 10); err != nil {
		t.Fatalf("current graph refused a read: %v", err)
	}

	// Drift the worktree so the manifest no longer matches.
	writeTestFile(t, root, "b.go", "package sample\nfunc B() string { return A() }\n")

	_, err = store.FindGraphSymbols(ctx, identity, "A", "", "", 1, 10)
	if err == nil {
		t.Fatal("drifted graph answered instead of refusing")
	}
	if !IsStaleGraph(err) {
		t.Fatalf("expected a stale-graph refusal, got %T: %v", err, err)
	}

	// The refusal must be side-effect free: the stored generation is untouched,
	// so the new file is still absent from the graph rather than indexed.
	store.SetRefreshOnRead(true)
	generation, gerr := store.activeGeneration(ctx, identity.WorktreeID)
	if gerr != nil {
		t.Fatalf("active generation: %v", gerr)
	}
	if generation.Status == GraphStale {
		t.Error("non-refreshing read downgraded the stored generation status")
	}
}
