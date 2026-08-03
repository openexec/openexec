package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectionCarriesInsightsAndTotals(t *testing.T) {
	root := t.TempDir()
	code := "package demo\n\nfunc Used() int { return 1 }\n\nfunc Orphan() int { return 2 }\n\nfunc caller() int { return Used() + Used() }\n"
	if err := os.WriteFile(filepath.Join(root, "demo.go"), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.BuildRepositoryContext(ctx, identity, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Totals == nil || projection.Totals.Symbols == 0 || projection.Totals.Modules == 0 {
		t.Fatalf("totals missing: %#v", projection.Totals)
	}
	foundOrphan := false
	for _, candidate := range projection.DeadCodeCandidates {
		if candidate.DisplayName == "Orphan" {
			foundOrphan = true
		}
		if candidate.DisplayName == "Used" {
			t.Fatalf("called symbol reported dead: %#v", projection.DeadCodeCandidates)
		}
	}
	if !foundOrphan {
		t.Fatalf("Orphan not reported dead: %#v", projection.DeadCodeCandidates)
	}
	if len(projection.Hotspots) == 0 || projection.Hotspots[0].DisplayName != "Used" || projection.Hotspots[0].Inbound < 2 {
		t.Fatalf("hotspots = %#v", projection.Hotspots)
	}
	disclosed := false
	for _, limitation := range projection.Limitations {
		if len(limitation) > 0 && limitation[0] == 'd' {
			disclosed = true
		}
	}
	if !disclosed {
		t.Fatalf("heuristic limitation not disclosed: %#v", projection.Limitations)
	}
}
