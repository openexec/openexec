package knowledge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestRefreshRepositoryReparsesChangesAndMatchesFullScan(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package sample\nfunc A() string { return \"one\" }\n")
	writeTestFile(t, root, "b.go", "package sample\nfunc B() string { return A() }\n")
	writeTestFile(t, root, "old.go", "package sample\nfunc Old() {}\n")
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
	beforeB, err := store.ResolveGraphSymbol(ctx, identity, "B", "b.go", "function", 20)
	if err != nil || beforeB.Result.Candidate == nil {
		t.Fatal("B was not indexed")
	}
	stableB := beforeB.Result.Candidate.Symbol.ID
	writeTestFile(t, root, "a.go", "package sample\nfunc A() string { return \"two\" }\n")
	writeTestFile(t, root, "new.go", "package sample\nfunc New() {}\n")
	if err := os.Remove(filepath.Join(root, "old.go")); err != nil {
		t.Fatal(err)
	}
	refresh, err := store.RefreshRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !refresh.Changed || refresh.Invalidation.FullScan || refresh.Invalidation.Scope != InvalidationReverseDependencies {
		t.Fatalf("source edit used wrong invalidation: %#v", refresh.Invalidation)
	}
	if len(refresh.Invalidation.Added) != 1 || len(refresh.Invalidation.Modified) != 1 || len(refresh.Invalidation.Deleted) != 1 {
		t.Fatalf("changed files were not classified: %#v", refresh.Invalidation)
	}
	afterB, err := store.ResolveGraphSymbol(ctx, identity, "B", "b.go", "function", 20)
	if err != nil || afterB.Result.Candidate == nil || afterB.Result.Candidate.Symbol.ID != stableB {
		t.Fatalf("unchanged symbol identity was not carried forward: %#v %v", afterB, err)
	}
	old, err := store.ResolveGraphSymbol(ctx, identity, "Old", "", "", 20)
	if err != nil || old.Result.Status != "unresolved" {
		t.Fatalf("deleted symbol survived refresh: %#v %v", old, err)
	}

	oracleRoot := t.TempDir()
	for _, file := range []string{"a.go", "b.go", "new.go"} {
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, oracleRoot, file, string(data))
	}
	if err := os.MkdirAll(filepath.Join(oracleRoot, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	oracle, err := NewStore(oracleRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer oracle.Close()
	oracleScan, err := oracle.ScanRepository(ctx, oracleRoot)
	if err != nil {
		t.Fatal(err)
	}
	got := semanticGraph(t, store, refresh.Generation.ID)
	want := semanticGraph(t, oracle, oracleScan.Generation.ID)
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("incremental graph differs from full-scan oracle\n got: %v\nwant: %v", got, want)
	}
}

func TestRefreshRepositoryUsesFullScanForConfigurationChange(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "main.ts", "export const main = 1\n")
	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"strict":true}}`)
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
	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"strict":false}}`)
	refresh, err := store.RefreshRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !refresh.Invalidation.FullScan || refresh.Invalidation.Cause != CauseConfigChange || refresh.Invalidation.Scope != InvalidationRepository {
		t.Fatalf("configuration change did not force full scan: %#v", refresh.Invalidation)
	}
}

func TestRefreshRepositoryDoesNotPromotePartialCoverageIncrementally(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package sample\nfunc Main() {}\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	scan, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE graph_generations SET status = 'partial' WHERE id = ?`, scan.Generation.ID); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "main.go", "package sample\nfunc Main() { println(\"changed\") }\n")
	refresh, err := store.RefreshRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !refresh.Invalidation.FullScan || refresh.Invalidation.Scope != InvalidationRepository {
		t.Fatalf("partial generation was carried forward incrementally: %#v", refresh.Invalidation)
	}
	if refresh.Generation.Status != GraphCurrent {
		t.Fatalf("successful full extraction did not restore current status: %s", refresh.Generation.Status)
	}
}

func semanticGraph(t *testing.T, store *Store, generationID string) []string {
	t.Helper()
	var result []string
	rows, err := store.db.Query(`SELECT node_type, language, display_name, qualified_name FROM graph_nodes WHERE generation_id = ? AND node_type != 'repository' ORDER BY node_type, language, display_name, qualified_name`, generationID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var nodeType, language, display, qualified string
		if err := rows.Scan(&nodeType, &language, &display, &qualified); err != nil {
			t.Fatal(err)
		}
		result = append(result, "node:"+nodeType+":"+language+":"+display+":"+qualified)
	}
	rows.Close()
	edges, err := store.db.Query(`SELECT f.node_type, f.qualified_name, e.edge_type, t.node_type, t.qualified_name, e.resolution_status FROM graph_edges e JOIN graph_nodes f ON f.id = e.from_node_id JOIN graph_nodes t ON t.id = e.to_node_id WHERE e.generation_id = ? AND f.node_type != 'repository' ORDER BY f.node_type, f.qualified_name, e.edge_type, t.node_type, t.qualified_name, e.resolution_status`, generationID)
	if err != nil {
		t.Fatal(err)
	}
	defer edges.Close()
	for edges.Next() {
		var fromType, from, edgeType, toType, to, resolution string
		if err := edges.Scan(&fromType, &from, &edgeType, &toType, &to, &resolution); err != nil {
			t.Fatal(err)
		}
		result = append(result, "edge:"+fromType+":"+from+":"+edgeType+":"+toType+":"+to+":"+resolution)
	}
	sort.Strings(result)
	return result
}
