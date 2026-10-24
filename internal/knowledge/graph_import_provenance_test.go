package knowledge

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Invariant 2 of docs/KNOWLEDGE_V3_PLAN.md: derived edges are never presented
// as compiler_exact. A Python import is a lexical guess the extractor rewrites
// to a file path; matching that path to a module proves the file exists, not
// that a compiler resolved the import.
func TestPythonImportProvenanceStaysLexicalThroughPublicReads(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "app/__init__.py", "")
	writeTestFile(t, root, "app/core.py", "def compute():\n    return 1\n")
	writeTestFile(t, root, "app/service.py", "from app.core import compute\nimport missing_module\n\ndef run():\n    return compute()\n")
	writeTestFile(t, root, "tests/test_core.py", "from app.core import compute\n\ndef test_compute():\n    assert compute() == 1\n")
	// Go call edges are true compiler positives and must stay exact beside the
	// Python correction; the Go import of an external package keeps its label.
	writeTestFile(t, root, "go.mod", "module sample\n\ngo 1.21\n")
	writeTestFile(t, root, "lib/a.go", "package lib\n\nimport \"fmt\"\n\nfunc A() string { return fmt.Sprint(\"a\") }\n")
	writeTestFile(t, root, "lib/b.go", "package lib\n\nfunc B() string { return A() }\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanRepository(ctx, root); err != nil {
		store.Close()
		t.Fatal(err)
	}
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	assertPythonImportProvenance(t, store, identity, "scan")
	assertGoExactPositives(t, store, identity, "scan")

	// Reopen: the labels are persisted, not held in memory.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { store.Close() }()
	identity, err = store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	assertPythonImportProvenance(t, store, identity, "reopen")

	// Incremental refresh carries tests/test_core.py forward from stored edges
	// rather than re-extracting it, and re-derives its tested_by edge.
	writeTestFile(t, root, "app/service.py", "from app.core import compute\nimport missing_module\n\ndef run():\n    return compute() + 1\n")
	refresh, err := store.RefreshRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if !refresh.Changed || refresh.Invalidation.FullScan {
		t.Fatalf("expected an incremental refresh, got %#v", refresh.Invalidation)
	}
	assertPythonImportProvenance(t, store, identity, "incremental refresh")
	assertGoExactPositives(t, store, identity, "incremental refresh")

	// A graph stored by the previous extractor holds the false label. Its
	// version forces a full re-extraction on the next read instead of carrying
	// the stored compiler_exact forward.
	generation, err := store.activeGeneration(ctx, identity.WorktreeID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE graph_edges SET resolution_status = 'compiler_exact' WHERE generation_id = ? AND edge_type IN ('imports','tested_by') AND resolution_status = 'static_lexical'`, generation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE graph_generations SET extractor_version = 'repository-graph-v2.3' WHERE id = ?`, generation.ID); err != nil {
		t.Fatal(err)
	}
	assertPythonImportProvenance(t, store, identity, "previous extractor version")

	// Existing refusals are unchanged: a foreign identity has no graph here, and
	// a drifted graph is refused when reads may not rebuild it.
	foreign := RepositoryIdentity{RepositoryID: "foreign-repository", CheckoutID: "foreign-checkout", WorktreeID: "foreign-worktree", RootPath: t.TempDir()}
	var missing *MissingGraphError
	if _, err := store.FindModuleDependencies(ctx, foreign, "app/service.py", false, 1, DefaultGraphLimits()); !errors.As(err, &missing) {
		t.Fatalf("foreign identity was not refused as missing: %v", err)
	}
	if _, err := store.ImpactAnalysis(ctx, foreign, []string{"anything"}, 1, DefaultGraphLimits()); !errors.As(err, &missing) {
		t.Fatalf("foreign identity impact was not refused as missing: %v", err)
	}
	store.SetRefreshOnRead(false)
	writeTestFile(t, root, "app/core.py", "def compute():\n    return 2\n")
	if _, err := store.FindModuleDependencies(ctx, identity, "app/service.py", false, 1, DefaultGraphLimits()); !IsStaleGraph(err) {
		t.Fatalf("drifted graph was not refused as stale: %v", err)
	}
}

func assertPythonImportProvenance(t *testing.T, store *Store, identity RepositoryIdentity, stage string) {
	t.Helper()
	ctx := context.Background()
	dependencies, err := store.FindModuleDependencies(ctx, identity, "app/service.py", false, 1, DefaultGraphLimits())
	if err != nil {
		t.Fatalf("%s: dependencies: %v", stage, err)
	}
	bySpecifier := map[string]ResolutionStatus{}
	for _, edge := range dependencies.Result.Edges {
		bySpecifier[edge.Metadata["specifier"]] = edge.Resolution
	}
	if bySpecifier["app/core.py"] != ResolutionStaticLexical {
		t.Fatalf("%s: resolved Python import read back as %q, want static_lexical: %#v", stage, bySpecifier["app/core.py"], dependencies.Result.Edges)
	}
	if bySpecifier["missing_module"] != ResolutionUnresolved {
		t.Fatalf("%s: missing Python import read back as %q, want unresolved: %#v", stage, bySpecifier["missing_module"], dependencies.Result.Edges)
	}
	for _, method := range dependencies.Resolution.Methods {
		if method == ResolutionCompilerExact {
			t.Fatalf("%s: Python dependencies claim compiler_exact: %#v", stage, dependencies.Resolution)
		}
	}

	resolved, err := store.ResolveGraphSymbol(ctx, identity, "compute", "app/core.py", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("%s: resolve compute: %#v %v", stage, resolved, err)
	}
	impact, err := store.ImpactAnalysis(ctx, identity, []string{resolved.Result.Candidate.Symbol.ID}, 1, DefaultGraphLimits())
	if err != nil {
		t.Fatalf("%s: impact: %v", stage, err)
	}
	foundTest := false
	for _, test := range impact.Result.RelatedTests {
		if test.FilePath == "tests/test_core.py" {
			foundTest = true
		}
	}
	if !foundTest {
		t.Fatalf("%s: test module importing app/core.py was not related: %#v", stage, impact.Result.RelatedTests)
	}
	lexical := false
	for _, method := range impact.Resolution.Methods {
		if method == ResolutionCompilerExact {
			t.Fatalf("%s: Python impact (imports and tested_by) claims compiler_exact: %#v", stage, impact.Resolution)
		}
		lexical = lexical || method == ResolutionStaticLexical
	}
	if !lexical {
		t.Fatalf("%s: Python impact lost its static_lexical evidence: %#v", stage, impact.Resolution)
	}
}

func assertGoExactPositives(t *testing.T, store *Store, identity RepositoryIdentity, stage string) {
	t.Helper()
	ctx := context.Background()
	dependencies, err := store.FindModuleDependencies(ctx, identity, "lib/a.go", false, 1, DefaultGraphLimits())
	if err != nil {
		t.Fatalf("%s: Go dependencies: %v", stage, err)
	}
	if len(dependencies.Result.Edges) != 1 || dependencies.Result.Edges[0].Resolution != ResolutionUnresolved {
		t.Fatalf("%s: Go external import changed label: %#v", stage, dependencies.Result.Edges)
	}
	caller, err := store.ResolveGraphSymbol(ctx, identity, "B", "lib/b.go", "function", 20)
	if err != nil || caller.Result.Candidate == nil {
		t.Fatalf("%s: resolve B: %#v %v", stage, caller, err)
	}
	calls, err := store.FindSymbolRelationships(ctx, identity, caller.Result.Candidate.Symbol.ID, false, 1, []string{"calls"}, DefaultGraphLimits())
	if err != nil {
		t.Fatalf("%s: Go calls: %v", stage, err)
	}
	if len(calls.Result.Edges) != 1 || calls.Result.Edges[0].Resolution != ResolutionCompilerExact {
		t.Fatalf("%s: Go typed call is no longer compiler_exact: %#v", stage, calls.Result.Edges)
	}
}

// The TypeScript compiler resolves imports itself, so its import edges stay
// compiler_exact. The tested_by edge derived from such an import is a
// test-file naming convention applied to that fact, so it is recorded below
// compiler_exact.
func TestTypeScriptCompilerImportStaysExactAndDerivedTestEdgeDoesNot(t *testing.T) {
	if _, err := findCompatibleNode(context.Background()); err != nil {
		t.Skipf("Node.js 18+ unavailable: %v", err)
	}
	compilerPackage, err := filepath.Abs(filepath.Join("..", "..", "ui", "node_modules", "typescript"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(compilerPackage, "lib", "typescript.js")); err != nil {
		t.Skip("repository TypeScript compiler is not installed")
	}
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compilerPackage, filepath.Join(root, "node_modules", "typescript")); err != nil {
		t.Skipf("cannot link TypeScript compiler: %v", err)
	}
	writeTestFile(t, root, "ui/tsconfig.json", `{"compilerOptions":{"strict":true,"target":"ES2022","module":"ESNext","moduleResolution":"Bundler"},"include":["src","tests"]}`)
	writeTestFile(t, root, "ui/src/dep.ts", "export function dependency(): number { return 1 }\n")
	writeTestFile(t, root, "ui/src/main.ts", "import { dependency } from './dep'\nexport const main = (): number => dependency()\n")
	writeTestFile(t, root, "ui/tests/dep.test.ts", "import { dependency } from '../src/dep'\nexport const check = (): boolean => dependency() === 1\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	scan, err := store.ScanRepository(ctx, root)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if scan.Generation.Capabilities["typescript.imports"] != "compiler_exact" {
		store.Close()
		t.Fatalf("TypeScript compiler did not resolve imports: %#v %v", scan.Generation.Capabilities, scan.Limitations)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range []string{"ui/src/main.ts", "ui/tests/dep.test.ts"} {
		dependencies, err := store.FindModuleDependencies(ctx, identity, module, false, 1, DefaultGraphLimits())
		if err != nil {
			t.Fatal(err)
		}
		if len(dependencies.Result.Edges) != 1 || dependencies.Result.Edges[0].Resolution != ResolutionCompilerExact {
			t.Fatalf("compiler-resolved import from %s is no longer compiler_exact: %#v", module, dependencies.Result.Edges)
		}
	}
	generation, err := store.activeGeneration(ctx, identity.WorktreeID)
	if err != nil {
		t.Fatal(err)
	}
	var derived string
	if err := store.db.QueryRowContext(ctx, `SELECT e.resolution_status FROM graph_edges e JOIN graph_nodes n ON n.id = e.from_node_id WHERE e.generation_id = ? AND e.edge_type = 'tested_by' AND e.source_file_path = 'ui/tests/dep.test.ts' AND n.qualified_name = 'ui/src/dep.ts'`, generation.ID).Scan(&derived); err != nil {
		t.Fatal(err)
	}
	if derived != string(ResolutionConfigurationDerived) {
		t.Fatalf("tested_by derived from a compiler import read back as %q, want configuration_derived", derived)
	}
}

func TestResolveImportNodeKeepsTheImportsOwnEvidence(t *testing.T) {
	modules := map[string]string{"pkg/mod.py": "node-mod", "ui/dep.ts": "node-dep"}
	file := ExtractedFile{Path: "ui/main.ts"}
	cases := []struct {
		name   string
		target string
		own    ResolutionStatus
		node   string
		want   ResolutionStatus
	}{
		{"lexical exact path stays lexical", "pkg/mod.py", ResolutionStaticLexical, "node-mod", ResolutionStaticLexical},
		{"compiler exact path stays exact", "ui/dep.ts", ResolutionCompilerExact, "node-dep", ResolutionCompilerExact},
		{"empty label is never exact", "pkg/mod.py", "", "node-mod", ResolutionStaticLexical},
		{"stored unresolved label is a lexical path match", "pkg/mod.py", ResolutionUnresolved, "node-mod", ResolutionStaticLexical},
		{"relative specifier is lexical", "./dep", ResolutionCompilerExact, "node-dep", ResolutionStaticLexical},
		{"near-miss path does not match", "pkg/mod", ResolutionStaticLexical, "", ResolutionUnresolved},
	}
	for _, tc := range cases {
		node, got := resolveImportNode(file, ExtractedImport{Target: tc.target, Resolution: tc.own}, modules)
		if node != tc.node || got != tc.want {
			t.Errorf("%s: got (%q, %q), want (%q, %q)", tc.name, node, got, tc.node, tc.want)
		}
	}
	for own, want := range map[ResolutionStatus]ResolutionStatus{
		ResolutionCompilerExact:        ResolutionConfigurationDerived,
		ResolutionASTExact:             ResolutionConfigurationDerived,
		ResolutionConfigurationDerived: ResolutionConfigurationDerived,
		ResolutionStaticLexical:        ResolutionStaticLexical,
		ResolutionHeuristic:            ResolutionHeuristic,
		"":                             ResolutionStaticLexical,
	} {
		if got := derivedTestResolution(own); got != want {
			t.Errorf("derived tested_by from %q = %q, want %q", own, got, want)
		}
	}
}
