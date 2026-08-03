package knowledge_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
)

func writeFile(t *testing.T, root, rel, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveAndReadRefreshMovedRenamedAndLineShiftedSymbol(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "old.go", "package sample\n\nfunc Run(value string) string { return value }\n")
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.ResolveGraphSymbol(ctx, identity, "Run", "old.go", "function", 20)
	if err != nil || first.Result.Candidate == nil {
		t.Fatalf("initial resolve: %#v %v", first, err)
	}
	firstGeneration := first.Generation.GraphVersion

	if err := os.Remove(filepath.Join(root, "old.go")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "moved.go", "package sample\n\n// shifted\n\nfunc Execute(value string) string { return value + \"!\" }\n")
	resolved, err := store.ResolveGraphSymbol(ctx, identity, "Execute", "moved.go", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("fresh resolve: %#v %v", resolved, err)
	}
	if resolved.Generation.Freshness != knowledge.FreshnessCurrent || resolved.Generation.GraphVersion == firstGeneration {
		t.Fatalf("resolve did not promote a fresh generation: %#v", resolved.Generation)
	}
	if got := resolved.Result.Candidate.Occurrence.StartLine; got != 5 {
		t.Fatalf("line-shifted occurrence starts at %d, want 5", got)
	}
	reader, err := repository.NewRootedReader(root, identity.RepositoryID, identity.WorktreeID, knowledge.DefaultGraphLimits().MaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.ReadGraphSymbol(ctx, identity, resolved.Result.Candidate.Symbol.ID, reader)
	if err != nil {
		t.Fatal(err)
	}
	if source.Result == nil || source.Result.Source.Content != "func Execute(value string) string { return value + \"!\" }" {
		t.Fatalf("read returned stale source: %#v", source)
	}
}
