package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryIdentityAndLegacyMigration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "service.go"), []byte("package service\n\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.SetSymbol(&SymbolRecord{Name: "Run", Kind: "function", FilePath: filepath.Join(root, "service.go"), StartLine: 3, EndLine: 3, Signature: "func Run()"}); err != nil {
		t.Fatal(err)
	}

	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if identity != again {
		t.Fatalf("identity changed across resolution: %#v != %#v", identity, again)
	}

	generationID, err := store.MigrateLegacySymbols(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := store.MigrateLegacySymbols(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if generationID == "" || generationID != secondID {
		t.Fatalf("legacy migration is not idempotent: %q != %q", generationID, secondID)
	}

	resolved, err := store.ResolveGraphSymbol(ctx, identity, "Run", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Result.Status != "resolved" || resolved.Result.Candidate == nil {
		t.Fatalf("legacy pointer did not resolve: %#v", resolved.Result)
	}
	if got := resolved.Result.Candidate.Occurrence.FilePath; got != "service.go" {
		t.Fatalf("expected repository-relative path, got %q", got)
	}
	if resolved.Generation.Freshness != FreshnessPartial {
		t.Fatalf("legacy graph must disclose partial freshness, got %q", resolved.Generation.Freshness)
	}
	legacy, err := store.GetSymbol("Run")
	if err != nil || legacy == nil {
		t.Fatalf("legacy pointer was not retained: %#v %v", legacy, err)
	}
}

func TestResolveGraphSymbolRefusesAmbiguity(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	manifest := ScanManifest{ManifestHash: "m", WorktreeStateHash: "w", ConfigurationDigest: "c"}
	generation, err := store.BeginGeneration(ctx, identity, manifest, map[string]string{"definitions": "ast_exact"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index, file := range []string{"a.go", "b.go"} {
		symbolID := stableID("sym", identity.RepositoryID, file, "Run")
		nodeID := stableID("node", generation.ID, symbolID)
		if _, err := store.db.Exec(`INSERT INTO repository_symbols (id, repository_id, language, kind, display_name, qualified_name) VALUES (?, ?, 'go', 'method', 'Run', ?)`, symbolID, identity.RepositoryID, file+".Run"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`INSERT INTO graph_nodes (id, generation_id, repository_id, node_type, language, display_name, qualified_name) VALUES (?, ?, ?, 'symbol', 'go', 'Run', ?)`, nodeID, generation.ID, identity.RepositoryID, file+".Run"); err != nil {
			t.Fatal(err)
		}
		if _, err := store.db.Exec(`INSERT INTO symbol_occurrences (symbol_id, generation_id, node_id, file_path, start_line, end_line, start_byte, end_byte, file_content_hash, source_range_hash, resolution_status) VALUES (?, ?, ?, ?, ?, ?, 0, 1, 'file', 'range', 'ast_exact')`, symbolID, generation.ID, nodeID, file, index+1, index+1); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.PromoteGeneration(ctx, generation.ID); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveGraphSymbol(ctx, identity, "Run", "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Result.Status != "ambiguous" || resolved.Result.Candidate != nil || len(resolved.Result.Candidates) != 2 {
		t.Fatalf("ambiguous name was silently selected: %#v", resolved.Result)
	}
}

func TestPromoteGenerationKeepsOneCurrent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.BeginGeneration(ctx, identity, ScanManifest{ManifestHash: "one", WorktreeStateHash: "one", ConfigurationDigest: "c"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteGeneration(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := store.BeginGeneration(ctx, identity, ScanManifest{ManifestHash: "two", WorktreeStateHash: "two", ConfigurationDigest: "c"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PromoteGeneration(ctx, second.ID); err != nil {
		t.Fatal(err)
	}
	var current, superseded int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status = 'current'`, identity.WorktreeID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE id = ? AND status = 'superseded'`, first.ID).Scan(&superseded); err != nil {
		t.Fatal(err)
	}
	if current != 1 || superseded != 1 {
		t.Fatalf("promotion contract failed: current=%d superseded=%d", current, superseded)
	}
}
