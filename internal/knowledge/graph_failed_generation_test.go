package knowledge

import (
	"context"
	"testing"
)

// failedNewestGeneration leaves a checkout with no current generation and a
// newest generation that a cancelled scan failed after storing its files. It
// is the state a cancelled extractor-migration read left on 24 October 2026:
// the previous generation labelled stale, the cancelled build "failed" with
// the very manifest the repository still has.
func failedNewestGeneration(t *testing.T) (*Store, string, RepositoryIdentity, GraphGeneration) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\n\nfunc main() { helper() }\n")
	writeTestFile(t, root, "helper.go", "package main\n\nfunc helper() {}\n")
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	previous, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE graph_generations SET status = 'stale' WHERE id = ?`, previous.Generation.ID); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	if _, err := store.scanRepository(cancelled, root, cancel); err == nil {
		t.Fatal("cancelled scan unexpectedly succeeded")
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	failed, err := store.activeGeneration(ctx, identity.WorktreeID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != GraphFailed || failed.ExtractorVersion != ExtractorVersion {
		t.Fatalf("active generation is %s/%s, want the cancelled scan's failed generation", failed.Status, failed.ExtractorVersion)
	}
	return store, root, identity, failed
}

// A failed generation is never the graph. The read rebuilds and answers from
// a promoted generation instead of serving the cancelled build.
func TestReadNeverAnswersFromFailedGeneration(t *testing.T) {
	store, _, identity, failed := failedNewestGeneration(t)
	generation, state, err := store.freshGeneration(context.Background(), identity)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if generation.ID == failed.ID || generation.Status != GraphCurrent || state.Freshness != FreshnessCurrent {
		t.Fatalf("read answered %s (%s, %s), want a newly promoted generation, not failed %s", generation.ID, generation.Status, state.Freshness, failed.ID)
	}
}

// Incremental refresh carries unchanged files forward only from a generation
// that was complete. From a failed one it re-extracts everything; from a
// stale one — the near miss — it still reparses only what changed.
func TestRefreshNeverCarriesFilesFromIncompleteGeneration(t *testing.T) {
	t.Run("failed previous", func(t *testing.T) {
		store, root, _, _ := failedNewestGeneration(t)
		writeTestFile(t, root, "helper.go", "package main\n\nfunc helper() { other() }\n\nfunc other() {}\n")
		refresh, err := store.RefreshRepository(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		if !refresh.Invalidation.FullScan || refresh.Generation.Status != GraphCurrent {
			t.Fatalf("refresh from a failed generation = %+v (%s), want a full scan", refresh.Invalidation, refresh.Generation.Status)
		}
	})
	t.Run("stale previous", func(t *testing.T) {
		ctx := context.Background()
		root := t.TempDir()
		writeTestFile(t, root, "main.go", "package main\n\nfunc main() { helper() }\n")
		writeTestFile(t, root, "helper.go", "package main\n\nfunc helper() {}\n")
		store, err := NewStore(root)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		previous, err := store.ScanRepository(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`UPDATE graph_generations SET status = 'stale' WHERE id = ?`, previous.Generation.ID); err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, root, "helper.go", "package main\n\nfunc helper() { other() }\n\nfunc other() {}\n")
		refresh, err := store.RefreshRepository(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		if refresh.Invalidation.FullScan || refresh.Generation.Status != GraphCurrent {
			t.Fatalf("refresh from a stale generation = %+v (%s), want an incremental refresh", refresh.Invalidation, refresh.Generation.Status)
		}
	})
}
