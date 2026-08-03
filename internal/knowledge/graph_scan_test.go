package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildScanManifestIncludesDirtyInputsAndConfiguration(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.ts", "export const main = () => 1\n")
	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"strict":true}}`)
	writeTestFile(t, root, "untracked.go", "package sample\nfunc Untracked() {}\n")
	writeTestFile(t, root, "node_modules/ignored.ts", "export const ignored = true\n")
	writeTestFile(t, root, ".openexec/private.go", "package hidden\n")

	manifest, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := make(map[string]string)
	for _, input := range manifest.Inputs {
		paths[input.FilePath] = input.InputKind
	}
	for _, expected := range []string{"src/main.ts", "tsconfig.json", "untracked.go"} {
		if paths[expected] == "" {
			t.Errorf("manifest omitted %s: %#v", expected, paths)
		}
	}
	for _, excluded := range []string{"node_modules/ignored.ts", ".openexec/private.go"} {
		if paths[excluded] != "" {
			t.Errorf("manifest included excluded input %s", excluded)
		}
	}
	if manifest.ManifestHash == "" || manifest.ConfigurationDigest == "" || manifest.WorktreeStateHash != manifest.ManifestHash {
		t.Fatalf("manifest identity is incomplete: %#v", manifest)
	}

	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"strict":false}}`)
	changed, err := BuildScanManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ConfigurationDigest == changed.ConfigurationDigest || manifest.ManifestHash == changed.ManifestHash {
		t.Fatal("configuration change did not invalidate the manifest")
	}
}

func TestBuildScanManifestRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildScanManifest(root); err == nil || !strings.Contains(err.Error(), "escapes repository") {
		t.Fatalf("expected symlink escape rejection, got %v", err)
	}
}

func TestScanRepositoryPromotesDeterministicGoAndTypeScriptGraph(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "go.mod", "module example.test/sample\n\ngo 1.25\n")
	writeTestFile(t, root, "service/service.go", "package service\n\nimport \"fmt\"\n\ntype Runner struct{}\nfunc (Runner) Run() string { return fmt.Sprint(\"ok\") }\n")
	writeTestFile(t, root, "ui/dep.ts", "export function dependency(): number { return 1 }\n")
	writeTestFile(t, root, "ui/main.ts", "import { dependency } from './dep'\nexport const main = () => dependency()\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	first, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation.Status != GraphCurrent || first.Files != 3 || first.Symbols < 4 {
		t.Fatalf("unexpected scan result: %#v", first)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.ResolveGraphSymbol(ctx, identity, "Run", "", "method", 20)
	if err != nil || run.Result.Candidate == nil {
		t.Fatalf("Go method was not resolved: %#v %v", run, err)
	}
	main, err := store.ResolveGraphSymbol(ctx, identity, "main", "ui/main.ts", "function", 20)
	if err != nil || main.Result.Candidate == nil {
		t.Fatalf("TypeScript arrow function was not resolved: %#v %v", main, err)
	}
	firstMainID := main.Result.Candidate.Symbol.ID

	second, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation.ID == first.Generation.ID || second.Generation.ManifestHash != first.Generation.ManifestHash {
		t.Fatalf("repeat scan is not deterministic: first=%#v second=%#v", first.Generation, second.Generation)
	}
	main, err = store.ResolveGraphSymbol(ctx, identity, "main", "ui/main.ts", "function", 20)
	if err != nil || main.Result.Candidate == nil || main.Result.Candidate.Symbol.ID != firstMainID {
		t.Fatalf("stable symbol identity was not preserved: %#v %v", main, err)
	}
	var current, importEdges int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status = 'current'`, identity.WorktreeID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE generation_id = ? AND edge_type = 'imports'`, second.Generation.ID).Scan(&importEdges); err != nil {
		t.Fatal(err)
	}
	if current != 1 || importEdges < 2 {
		t.Fatalf("graph publication missing current/import contracts: current=%d imports=%d", current, importEdges)
	}
	dependencies, err := store.FindModuleDependencies(ctx, identity, "ui/main.ts", false, 1, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if dependencies.Generation.Freshness != FreshnessCurrent || len(dependencies.Result.Edges) != 1 || len(dependencies.Result.Nodes) != 1 || dependencies.Result.Nodes[0].QualifiedName != "ui/dep.ts" {
		t.Fatalf("bounded module dependency query returned the wrong graph: %#v", dependencies)
	}
	writeTestFile(t, root, "ui/dep.ts", "export function dependency(): number { return 2 }\n")
	state, err := store.CurrentRepositoryState(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if state.Freshness != FreshnessStale {
		t.Fatalf("worktree change did not stale the graph: %#v", state)
	}
}

func TestScanRepositoryDoesNotPromoteMixedInputs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\nfunc Main() {}\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, err = store.scanRepository(ctx, root, func() {
		writeTestFile(t, root, "main.go", "package main\nfunc Main() { println(\"changed\") }\n")
	})
	if err == nil || !strings.Contains(err.Error(), "changed while graph generation was building") {
		t.Fatalf("expected inconsistent scan rejection, got %v", err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	var current, inconsistent int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status = 'current'`, identity.WorktreeID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status = 'inconsistent'`, identity.WorktreeID).Scan(&inconsistent); err != nil {
		t.Fatal(err)
	}
	if current != 0 || inconsistent != 1 {
		t.Fatalf("mixed generation became visible: current=%d inconsistent=%d", current, inconsistent)
	}
}

func TestScanRepositoryUsesTypeScriptCompilerWhenProjectProvidesIt(t *testing.T) {
	compilerPackage := filepath.Join("..", "..", "ui", "node_modules", "typescript")
	compilerPackage, err := filepath.Abs(compilerPackage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(compilerPackage, "lib", "typescript.js")); err != nil {
		t.Skip("repository TypeScript compiler is not installed")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compilerPackage, filepath.Join(root, "node_modules", "typescript")); err != nil {
		t.Skipf("cannot link TypeScript compiler: %v", err)
	}
	writeTestFile(t, root, "tsconfig.json", `{"compilerOptions":{"strict":true,"moduleResolution":"node"},"include":["src"]}`)
	writeTestFile(t, root, "src/dep.ts", "export function dependency(): number { return 1 }\n")
	writeTestFile(t, root, "src/main.ts", "import { dependency } from './dep'\nexport const main = (): number => {\n  return dependency()\n}\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := store.ScanRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Capabilities["typescript.definitions"] != "compiler_exact" {
		t.Fatalf("compiler-backed capability was not published: %#v", result.Generation.Capabilities)
	}
	identity, err := store.EnsureRepositoryIdentity(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveGraphSymbol(context.Background(), identity, "main", "src/main.ts", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("compiler symbol did not resolve: %#v %v", resolved, err)
	}
	occurrence := resolved.Result.Candidate.Occurrence
	if occurrence.Resolution != ResolutionCompilerExact || occurrence.EndLine < 4 {
		t.Fatalf("compiler range was not preserved: %#v", occurrence)
	}
}

func TestSymbolIdentityRecordsMoveRenameAndDeletionGap(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "a.go", "package sample\nfunc Run(value string) string { return value }\n")
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	identity, _ := store.EnsureRepositoryIdentity(ctx, root, "")
	first, err := store.ResolveGraphSymbol(ctx, identity, "Run", "a.go", "function", 20)
	if err != nil || first.Result.Candidate == nil {
		t.Fatalf("first symbol = %#v, %v", first, err)
	}
	stable := first.Result.Candidate.Symbol.ID

	if err := os.Remove(filepath.Join(root, "a.go")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "b.go", "package sample\nfunc Run(value string) string { return value }\n")
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	moved, _ := store.ResolveGraphSymbol(ctx, identity, "Run", "b.go", "function", 20)
	if moved.Result.Candidate == nil || moved.Result.Candidate.Symbol.ID != stable {
		t.Fatalf("move lost identity: %#v", moved)
	}
	assertLatestContinuity(t, store, stable, "moved")

	writeTestFile(t, root, "b.go", "package sample\nfunc Execute(value string) string { return value }\n")
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	renamed, _ := store.ResolveGraphSymbol(ctx, identity, "Execute", "b.go", "function", 20)
	if renamed.Result.Candidate == nil || renamed.Result.Candidate.Symbol.ID != stable {
		t.Fatalf("rename lost identity: %#v", renamed)
	}
	assertLatestContinuity(t, store, stable, "renamed")

	writeTestFile(t, root, "b.go", "package sample\n")
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "b.go", "package sample\nfunc Execute(value string) string { return value }\n")
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	recreated, _ := store.ResolveGraphSymbol(ctx, identity, "Execute", "b.go", "function", 20)
	if recreated.Result.Candidate == nil || recreated.Result.Candidate.Symbol.ID == stable {
		t.Fatalf("delete/recreate silently reused identity: %#v", recreated)
	}
}

func assertLatestContinuity(t *testing.T, store *Store, symbolID, expected string) {
	t.Helper()
	var status string
	if err := store.db.QueryRow(`SELECT continuity_status FROM symbol_lineage WHERE symbol_id = ? ORDER BY created_at DESC, rowid DESC LIMIT 1`, symbolID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != expected {
		t.Fatalf("continuity = %q, want %q", status, expected)
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
