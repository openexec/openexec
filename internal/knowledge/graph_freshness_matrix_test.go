package knowledge_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/repository"
	"github.com/openexec/openexec/pkg/db/sqlitecfg"
)

// graphRead is one public graph resolve/read. It returns the graph version its
// answer was built from, so a successful answer can be held to the generation
// the read itself promoted.
type graphRead func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, symbolID string) (string, error)

// graphReadEntryPoints is the V2.1 coverage matrix: every public graph
// resolve/read, including the composite repository-context projection. Each
// must pass the freshness gate before touching pointers, so a new entry point
// belongs here too. Symbol-ID reads get a real symbol, so success is checked
// for the current graph version rather than reduced to a not-found.
func graphReadEntryPoints(root string) map[string]graphRead {
	limits := knowledge.DefaultGraphLimits()
	return map[string]graphRead{
		"state": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, _ string) (string, error) {
			state, err := s.CurrentRepositoryState(ctx, id)
			return state.GraphVersion, err
		},
		"resolve": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, _ string) (string, error) {
			result, err := s.ResolveGraphSymbol(ctx, id, "Run", "", "", 20)
			if err == nil && result.Result.Candidate == nil {
				err = fmt.Errorf("Run did not resolve: %s", result.Result.Status)
			}
			return result.Generation.GraphVersion, err
		},
		"find": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, _ string) (string, error) {
			result, err := s.FindGraphSymbols(ctx, id, "Run", "", "", 1, 10)
			return result.Generation.GraphVersion, err
		},
		"detail": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, symbolID string) (string, error) {
			result, err := s.GraphSymbolDetail(ctx, id, symbolID)
			return result.Generation.GraphVersion, err
		},
		"source": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, symbolID string) (string, error) {
			reader, err := repository.NewRootedReader(root, id.RepositoryID, id.WorktreeID, limits.MaxBytes)
			if err != nil {
				return "", err
			}
			result, err := s.ReadGraphSymbol(ctx, id, symbolID, reader)
			if err == nil && result.Result == nil {
				err = fmt.Errorf("source read returned no source")
			}
			return result.Generation.GraphVersion, err
		},
		"relations": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, symbolID string) (string, error) {
			result, err := s.FindSymbolRelationships(ctx, id, symbolID, true, 1, nil, limits)
			return result.Generation.GraphVersion, err
		},
		"dependencies": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, _ string) (string, error) {
			result, err := s.FindModuleDependencies(ctx, id, "a.go", false, 1, limits)
			return result.Generation.GraphVersion, err
		},
		"impact": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, symbolID string) (string, error) {
			result, err := s.ImpactAnalysis(ctx, id, []string{symbolID}, 1, limits)
			return result.Generation.GraphVersion, err
		},
		"changed_impact": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, _ string) (string, error) {
			result, err := s.ChangedImpactAnalysis(ctx, id, knowledge.ChangedImpactRequest{Files: []string{"a.go"}}, knowledge.DefaultChangedImpactLimits())
			return result.Generation.GraphVersion, err
		},
		"repository_context": func(ctx context.Context, s *knowledge.Store, id knowledge.RepositoryIdentity, _ string) (string, error) {
			projection, err := s.BuildRepositoryContext(ctx, id, []string{"Run"}, "", "", "", nil)
			if err == nil && (projection.Freshness != knowledge.FreshnessCurrent || len(projection.ResolvedSymbols) != 1) {
				err = fmt.Errorf("projection is not a current single-symbol answer: %#v", projection)
			}
			return projection.GraphVersion, err
		},
	}
}

const fixtureSource = "package sample\n\nfunc Run(value string) string { return value }\n"

type matrixFixture struct {
	root     string
	store    *knowledge.Store
	identity knowledge.RepositoryIdentity
	// db is a second connection to the store's database, standing in for
	// another process (CLI, HTTP server, MCP server) sharing it.
	db       *sql.DB
	symbolID string
}

func newMatrixFixture(t *testing.T) matrixFixture {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	writeFile(t, root, "a.go", fixtureSource)
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
	resolved, err := store.ResolveGraphSymbol(ctx, identity, "Run", "a.go", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("resolve fixture symbol: %#v %v", resolved, err)
	}
	db, err := sql.Open("sqlite", sqlitecfg.DSN(filepath.Join(root, ".openexec", "openexec.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return matrixFixture{root: root, store: store, identity: identity, db: db, symbolID: resolved.Result.Candidate.Symbol.ID}
}

// graphDrift is one way the stored generation can stop describing the
// repository: each of the three inputs the gate compares, plus a real
// configuration edit. restore must make the original generation valid again.
type graphDrift struct {
	apply, restore func(t *testing.T, f matrixFixture, generationID string)
}

func setGenerationColumn(t *testing.T, f matrixFixture, generationID, column, value string) string {
	t.Helper()
	var previous string
	if err := f.db.QueryRow(`SELECT `+column+` FROM graph_generations WHERE id = ?`, generationID).Scan(&previous); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`UPDATE graph_generations SET `+column+` = ? WHERE id = ?`, value, generationID); err != nil {
		t.Fatal(err)
	}
	return previous
}

func graphDrifts() map[string]graphDrift {
	var savedDigest, savedExtractor string
	return map[string]graphDrift{
		"source_edit": {
			apply: func(t *testing.T, f matrixFixture, _ string) {
				writeFile(t, f.root, "a.go", "package sample\n\n// edited: shifts Run down a line\nfunc Run(value string) string { return value }\n")
			},
			restore: func(t *testing.T, f matrixFixture, _ string) { writeFile(t, f.root, "a.go", fixtureSource) },
		},
		"configuration_edit": {
			apply: func(t *testing.T, f matrixFixture, _ string) {
				writeFile(t, f.root, "package.json", "{\"name\":\"sample\"}\n")
			},
			restore: func(t *testing.T, f matrixFixture, _ string) {
				if err := os.Remove(filepath.Join(f.root, "package.json")); err != nil {
					t.Fatal(err)
				}
			},
		},
		// The generation was recorded under a different configuration while the
		// manifest hash still matches: only the digest comparison can catch it.
		"configuration_digest": {
			apply: func(t *testing.T, f matrixFixture, id string) {
				savedDigest = setGenerationColumn(t, f, id, "configuration_digest", "sha256:other-configuration")
			},
			restore: func(t *testing.T, f matrixFixture, id string) {
				setGenerationColumn(t, f, id, "configuration_digest", savedDigest)
			},
		},
		// The generation was built by an older extractor over identical inputs.
		"extractor_version": {
			apply: func(t *testing.T, f matrixFixture, id string) {
				savedExtractor = setGenerationColumn(t, f, id, "extractor_version", "repository-graph-v0-previous")
			},
			restore: func(t *testing.T, f matrixFixture, id string) {
				setGenerationColumn(t, f, id, "extractor_version", savedExtractor)
			},
		},
	}
}

// TestEveryGraphReadRefusesDriftWithoutRefresh: with refresh disabled each
// entry point must refuse every kind of drift explicitly and mutate nothing.
func TestEveryGraphReadRefusesDriftWithoutRefresh(t *testing.T) {
	for driftName, drift := range graphDrifts() {
		for name := range graphReadEntryPoints("") {
			t.Run(driftName+"/"+name, func(t *testing.T) {
				ctx := context.Background()
				f := newMatrixFixture(t)
				before, err := f.store.CurrentRepositoryState(ctx, f.identity)
				if err != nil {
					t.Fatal(err)
				}
				f.store.SetRefreshOnRead(false)
				drift.apply(t, f, before.GraphVersion)

				_, err = graphReadEntryPoints(f.root)[name](ctx, f.store, f.identity, f.symbolID)
				if !knowledge.IsStaleGraph(err) {
					t.Fatalf("drifted read answered or failed opaquely: %v", err)
				}
				// Restoring the input must make the untouched generation current
				// again: a refusal that rewrote state would leave it stale.
				drift.restore(t, f, before.GraphVersion)
				after, err := f.store.CurrentRepositoryState(ctx, f.identity)
				if err != nil {
					t.Fatalf("refusal mutated stored state: %v", err)
				}
				if after.GraphVersion != before.GraphVersion || after.Freshness != knowledge.FreshnessCurrent {
					t.Fatalf("refusal changed the generation: %s -> %#v", before.GraphVersion, after)
				}
			})
		}
	}
}

// TestEveryGraphReadRefreshesDrift: with refresh enabled each entry point must
// itself promote a generation built from the drifted inputs and answer from it.
func TestEveryGraphReadRefreshesDrift(t *testing.T) {
	for driftName, drift := range graphDrifts() {
		for name := range graphReadEntryPoints("") {
			t.Run(driftName+"/"+name, func(t *testing.T) {
				ctx := context.Background()
				f := newMatrixFixture(t)
				before, err := f.store.CurrentRepositoryState(ctx, f.identity)
				if err != nil {
					t.Fatal(err)
				}
				drift.apply(t, f, before.GraphVersion)

				answered, err := graphReadEntryPoints(f.root)[name](ctx, f.store, f.identity, f.symbolID)
				if err != nil {
					t.Fatalf("refresh-enabled read did not answer: %v", err)
				}
				// Probe without refresh: only a generation the entry point already
				// promoted can answer.
				f.store.SetRefreshOnRead(false)
				after, err := f.store.CurrentRepositoryState(ctx, f.identity)
				if err != nil {
					t.Fatalf("entry point did not refresh the graph: %v", err)
				}
				if after.GraphVersion == before.GraphVersion || after.Freshness != knowledge.FreshnessCurrent {
					t.Fatalf("entry point left the old generation active: before=%s after=%#v", before.GraphVersion, after)
				}
				if answered != after.GraphVersion {
					t.Fatalf("answer carries graph %s, current graph is %s", answered, after.GraphVersion)
				}
				if after.ExtractorVersion != knowledge.ExtractorVersion {
					t.Fatalf("refreshed generation records extractor %q", after.ExtractorVersion)
				}
			})
		}
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
			f := newMatrixFixture(t)
			inner, err := repository.NewRootedReader(f.root, f.identity.RepositoryID, f.identity.WorktreeID, knowledge.DefaultGraphLimits().MaxBytes)
			if err != nil {
				t.Fatal(err)
			}
			reader := editingReader{inner: inner, edit: func() {
				if err := edit(f.root); err != nil {
					panic(fmt.Sprintf("edit fixture: %v", err))
				}
			}}
			result, err := f.store.ReadGraphSymbol(ctx, f.identity, f.symbolID, reader)
			if !knowledge.IsStaleGraph(err) {
				t.Fatalf("concurrent %s returned %#v, %v; want stale refusal", name, result.Result, err)
			}
		})
	}
}
