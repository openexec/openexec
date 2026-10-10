package knowledge_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
)

// graphReadEntryPoints is the V2.1 coverage matrix: every public graph
// resolve/read. Each must pass the freshness gate before touching pointers, so
// a new entry point belongs here too.
func graphReadEntryPoints(root string) map[string]func(context.Context, *knowledge.Store, knowledge.RepositoryIdentity) error {
	limits := knowledge.DefaultGraphLimits()
	return map[string]func(context.Context, *knowledge.Store, knowledge.RepositoryIdentity) error{
		"state": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.CurrentRepositoryState(ctx, id)
			return err
		},
		"resolve": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.ResolveGraphSymbol(ctx, id, "Run", "", "", 20)
			return err
		},
		"find": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.FindGraphSymbols(ctx, id, "Run", "", "", 1, 10)
			return err
		},
		"detail": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.GraphSymbolDetail(ctx, id, "missing")
			return err
		},
		"source": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			reader, err := repository.NewRootedReader(root, id.RepositoryID, id.WorktreeID, limits.MaxBytes)
			if err != nil {
				return err
			}
			_, err = s.ReadGraphSymbol(ctx, id, "missing", reader)
			return err
		},
		"relations": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.FindSymbolRelationships(ctx, id, "missing", true, 1, nil, limits)
			return err
		},
		"dependencies": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.FindModuleDependencies(ctx, id, "missing", false, 1, limits)
			return err
		},
		"impact": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.ImpactAnalysis(ctx, id, []string{"missing"}, 1, limits)
			return err
		},
		"changed_impact": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity) error {
			_, err := s.ChangedImpactAnalysis(ctx, id, knowledge.ChangedImpactRequest{Files: []string{"a.go"}}, knowledge.DefaultChangedImpactLimits())
			return err
		},
	}
}

func scannedFixture(t *testing.T) (string, *knowledge.Store, knowledge.RepositoryIdentity) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "a.go", "package sample\n\nfunc Run(value string) string { return value }\n")
	store, err := knowledge.NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	return root, store, identity
}

// TestEveryGraphReadRefusesDriftWithoutRefresh: with refresh disabled each
// entry point must refuse a drifted worktree explicitly and mutate nothing.
func TestEveryGraphReadRefusesDriftWithoutRefresh(t *testing.T) {
	for name := range graphReadEntryPoints("") {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root, store, identity := scannedFixture(t)
			before, err := store.CurrentRepositoryState(ctx, identity)
			if err != nil {
				t.Fatal(err)
			}
			store.SetRefreshOnRead(false)
			writeFile(t, root, "a.go", "package sample\n\n// edited\nfunc Run(value string) string { return value }\n")

			err = graphReadEntryPoints(root)[name](ctx, store, identity)
			if !knowledge.IsStaleGraph(err) {
				t.Fatalf("drifted read answered or failed opaquely: %v", err)
			}
			// Restoring the bytes must make the untouched generation current
			// again: a refusal that rewrote state would leave it stale.
			writeFile(t, root, "a.go", "package sample\n\nfunc Run(value string) string { return value }\n")
			after, err := store.CurrentRepositoryState(ctx, identity)
			if err != nil {
				t.Fatalf("refusal mutated stored state: %v", err)
			}
			if after.GraphVersion != before.GraphVersion {
				t.Fatalf("refusal promoted a generation: %s -> %s", before.GraphVersion, after.GraphVersion)
			}
		})
	}
}

// TestEveryGraphReadRefreshesDrift: with refresh enabled each entry point must
// itself promote a generation built from the edited worktree before answering.
func TestEveryGraphReadRefreshesDrift(t *testing.T) {
	for name := range graphReadEntryPoints("") {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root, store, identity := scannedFixture(t)
			before, err := store.CurrentRepositoryState(ctx, identity)
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, root, "b.go", "package sample\n\nfunc Other() string { return Run(\"x\") }\n")

			err = graphReadEntryPoints(root)[name](ctx, store, identity)
			if knowledge.IsStaleGraph(err) {
				t.Fatalf("refresh-enabled read refused instead of refreshing: %v", err)
			}
			// Probe without refresh: only a generation the entry point already
			// promoted can answer.
			store.SetRefreshOnRead(false)
			after, err := store.CurrentRepositoryState(ctx, identity)
			if err != nil {
				t.Fatalf("entry point did not refresh the graph: %v", err)
			}
			if after.GraphVersion == before.GraphVersion || after.Freshness != knowledge.FreshnessCurrent {
				t.Fatalf("entry point answered from the old generation: before=%s after=%#v", before.GraphVersion, after)
			}
		})
	}
}

// editingReader simulates a write landing between the freshness gate and the
// source read — the window the gate's lock cannot cover.
type editingReader struct {
	inner knowledge.RepositoryReader
	edit  func()
}

func (r editingReader) ReadRange(ctx context.Context, request knowledge.SourceReadRequest) (knowledge.SourceReadResult, error) {
	r.edit()
	return r.inner.ReadRange(ctx, request)
}

func TestReadGraphSymbolRefusesConcurrentEditAsStale(t *testing.T) {
	edits := map[string]func(root string) error{
		"edited": func(root string) error {
			return os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\n\nfunc Run(value string) string { return value + \"?\" }\n"), 0o600)
		},
		"deleted": func(root string) error { return os.Remove(filepath.Join(root, "a.go")) },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			root, store, identity := scannedFixture(t)
			resolved, err := store.ResolveGraphSymbol(ctx, identity, "Run", "a.go", "function", 20)
			if err != nil || resolved.Result.Candidate == nil {
				t.Fatalf("resolve: %#v %v", resolved, err)
			}
			inner, err := repository.NewRootedReader(root, identity.RepositoryID, identity.WorktreeID, knowledge.DefaultGraphLimits().MaxBytes)
			if err != nil {
				t.Fatal(err)
			}
			reader := editingReader{inner: inner, edit: func() {
				if err := edit(root); err != nil {
					panic(fmt.Sprintf("edit fixture: %v", err))
				}
			}}
			result, err := store.ReadGraphSymbol(ctx, identity, resolved.Result.Candidate.Symbol.ID, reader)
			if !knowledge.IsStaleGraph(err) {
				t.Fatalf("concurrent %s returned %#v, %v; want stale refusal", name, result.Result, err)
			}
		})
	}
}
