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
	var current, importEdges, persistedEdges int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE worktree_id = ? AND status = 'current'`, identity.WorktreeID).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE generation_id = ? AND edge_type = 'imports'`, second.Generation.ID).Scan(&importEdges); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_edges WHERE generation_id = ?`, second.Generation.ID).Scan(&persistedEdges); err != nil {
		t.Fatal(err)
	}
	if current != 1 || importEdges < 2 {
		t.Fatalf("graph publication missing current/import contracts: current=%d imports=%d", current, importEdges)
	}
	if second.Edges != persistedEdges {
		t.Fatalf("reported edge count %d does not match persisted count %d", second.Edges, persistedEdges)
	}
	dependencies, err := store.FindModuleDependencies(ctx, identity, "ui/main.ts", false, 1, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if dependencies.Generation.Freshness != FreshnessCurrent || len(dependencies.Result.Edges) != 1 || len(dependencies.Result.Nodes) != 1 || dependencies.Result.Nodes[0].QualifiedName != "ui/dep.ts" {
		t.Fatalf("bounded module dependency query returned the wrong graph: %#v", dependencies)
	}
	writeTestFile(t, root, "ui/dep.ts", "export function dependency(): number { return 2 }\n")
	beforeVersion := dependencies.Generation.GraphVersion
	state, err := store.CurrentRepositoryState(ctx, identity)
	if err != nil {
		t.Fatal(err)
	}
	if state.Freshness != FreshnessCurrent || state.GraphVersion == beforeVersion {
		t.Fatalf("read-time freshness did not refresh the graph: %#v", state)
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
	if _, err := findCompatibleNode(context.Background()); err != nil {
		t.Skipf("Node.js 18+ unavailable: %v", err)
	}
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
	writeTestFile(t, root, "ui/tsconfig.json", `{"files":[],"references":[{"path":"./tsconfig.app.json"}]}`)
	writeTestFile(t, root, "ui/tsconfig.app.json", `{"compilerOptions":{"strict":true,"target":"ES2022","module":"ESNext","moduleResolution":"Bundler"},"include":["src"]}`)
	writeTestFile(t, root, "ui/src/dep.ts", "export function dependency(): number { return 1 }\n")
	writeTestFile(t, root, "ui/src/main.ts", "import { dependency } from './dep'\nexport const source = import.meta.url\nexport const main = (): number => {\n  return dependency()\n}\n")
	writeTestFile(t, root, "ui/src/vite-env.d.ts", "declare interface ImportMetaEnv { readonly PROD: boolean }\n")
	writeTestFile(t, root, "template/tsconfig.json", `{"extends":"./generated/tsconfig.json","compilerOptions":{"module":"ESNext","target":"ES2022"},"include":["src"]}`)
	writeTestFile(t, root, "template/src/example.ts", "export const example = 1\n")
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
	if result.Generation.Status != GraphCurrent || result.Files != 4 {
		t.Fatalf("nested-config/declaration scan was incomplete: %#v", result)
	}
	for _, limitation := range result.Limitations {
		if strings.Contains(limitation, "import.meta") || strings.Contains(limitation, "compiler omitted") || strings.Contains(limitation, "compiler extraction unavailable") {
			t.Fatalf("nested TypeScript configuration was not applied: %s", limitation)
		}
	}
	identity, err := store.EnsureRepositoryIdentity(context.Background(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := store.ResolveGraphSymbol(context.Background(), identity, "main", "ui/src/main.ts", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("compiler symbol did not resolve: %#v %v", resolved, err)
	}
	occurrence := resolved.Result.Candidate.Occurrence
	if occurrence.Resolution != ResolutionCompilerExact || occurrence.EndLine < 4 {
		t.Fatalf("compiler range was not preserved: %#v", occurrence)
	}
}

func TestCancelledScanPersistsFailedGeneration(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\nfunc main() {}\n")
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx, cancel := context.WithCancel(context.Background())
	_, err = store.scanRepository(ctx, root, cancel)
	if err == nil {
		t.Fatal("cancelled scan unexpectedly succeeded")
	}
	var building, failed int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE status = 'building'`).Scan(&building); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_generations WHERE status = 'failed'`).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if building != 0 || failed != 1 {
		t.Fatalf("cancelled generation state: building=%d failed=%d", building, failed)
	}
}

func TestScanOnlySweepsAbandonedGenerationForItsWorktree(t *testing.T) {
	ctx := context.Background()
	base := t.TempDir()
	storeRoot := filepath.Join(base, "state")
	if err := os.MkdirAll(storeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(storeRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	primaryRoot := filepath.Join(base, "primary")
	initGitFixture(t, primaryRoot, "")
	writeTestFile(t, primaryRoot, "main.go", "package main\nfunc main() {}\n")
	runGitFixture(t, primaryRoot, "add", "main.go")
	runGitFixture(t, primaryRoot, "-c", "user.name=OpenExec Test", "-c", "user.email=test@example.test", "commit", "-m", "source")
	linkedRoot := filepath.Join(base, "linked")
	runGitFixture(t, primaryRoot, "worktree", "add", "-b", "linked-scan", linkedRoot)

	primary, err := store.EnsureRepositoryIdentity(ctx, primaryRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	linked, err := store.EnsureRepositoryIdentity(ctx, linkedRoot, "")
	if err != nil {
		t.Fatal(err)
	}
	primaryManifest, err := BuildScanManifest(primaryRoot)
	if err != nil {
		t.Fatal(err)
	}
	linkedManifest, err := BuildScanManifest(linkedRoot)
	if err != nil {
		t.Fatal(err)
	}
	primaryBuilding, err := store.BeginGeneration(ctx, primary, primaryManifest, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	linkedBuilding, err := store.BeginGeneration(ctx, linked, linkedManifest, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.ScanRepository(ctx, primaryRoot); err != nil {
		t.Fatal(err)
	}
	var primaryStatus, linkedStatus string
	if err := store.db.QueryRow(`SELECT status FROM graph_generations WHERE id = ?`, primaryBuilding.ID).Scan(&primaryStatus); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT status FROM graph_generations WHERE id = ?`, linkedBuilding.ID).Scan(&linkedStatus); err != nil {
		t.Fatal(err)
	}
	if primaryStatus != string(GraphFailed) || linkedStatus != string(GraphBuilding) {
		t.Fatalf("abandoned sweep crossed worktrees: primary=%s linked=%s", primaryStatus, linkedStatus)
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
