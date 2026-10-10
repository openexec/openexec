package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func contextFixture(t *testing.T) (string, *Store, RepositoryIdentity, string) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\n\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	scan, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	return root, store, identity, scan.Generation.ID
}

// An edit after the gate must not pull later per-item reads onto a newer
// graph. Before the single-gate rewrite, resolving "Run" re-entered the gate,
// refreshed to a new generation, and reported its symbols under the old
// GraphVersion with Freshness: current.
func TestRepositoryContextReadsOneGenerationAcrossAMidBuildEdit(t *testing.T) {
	ctx := context.Background()
	root, store, identity, gated := contextFixture(t)
	store.afterContextGate = func() {
		if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package sample\n\nfunc Run() {}\nfunc Fresh() {}\n"), 0o600); err != nil {
			t.Error(err)
		}
	}
	projection, err := store.BuildRepositoryContext(ctx, identity, []string{"Run", "Fresh"}, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if projection.GraphVersion != gated {
		t.Fatalf("projection labelled %s, gated generation was %s", projection.GraphVersion, gated)
	}
	if len(projection.ResolvedSymbols) != 1 || !strings.HasSuffix(projection.ResolvedSymbols[0].DisplayName, "Run") {
		t.Fatalf("projection mixed in symbols from a newer graph: %#v", projection.ResolvedSymbols)
	}
	if !containsLimitation(projection.Limitations, "Fresh: unresolved") {
		t.Fatalf("symbol absent from the gated graph not disclosed: %#v", projection.Limitations)
	}
	active, err := store.activeGeneration(ctx, identity.WorktreeID)
	if err != nil || active.ID != gated {
		t.Fatalf("building the projection refreshed the graph mid-read: active=%s err=%v", active.ID, err)
	}
}

// A scan promoting a newer generation while the projection is built — here
// from a second store on the same database, the cross-process case graphMu
// cannot serialize — makes "current" false, so the build must refuse as stale.
func TestRepositoryContextRefusesGenerationSupersededMidBuild(t *testing.T) {
	ctx := context.Background()
	root, store, identity, gated := contextFixture(t)
	other, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	store.afterContextGate = func() {
		if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("package sample\n\nfunc Other() {}\n"), 0o600); err != nil {
			t.Error(err)
		}
		if _, err := other.ScanRepository(ctx, root); err != nil {
			t.Error(err)
		}
	}
	_, err = store.BuildRepositoryContext(ctx, identity, []string{"Run"}, "", "", "", nil)
	if !IsStaleGraph(err) {
		t.Fatalf("projection over superseded generation %s answered: %v", gated, err)
	}
	if !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("stale refusal does not say why: %v", err)
	}
}

func containsLimitation(limitations []string, want string) bool {
	for _, limitation := range limitations {
		if limitation == want {
			return true
		}
	}
	return false
}
