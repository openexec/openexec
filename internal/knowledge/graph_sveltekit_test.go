package knowledge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// svelteKitFixture lays out the files a SvelteKit project has after
// `svelte-kit sync`: a root tsconfig that extends the generated
// .svelte-kit/tsconfig.json, whose include list names the generated
// ambient.d.ts, which in turn references the @sveltejs/kit types declaring
// the $app/* modules. The kit package is a minimal stand-in with the same
// shape; the TypeScript compiler is the repository's real one.
type svelteKitFixture struct {
	generatedConfig bool
	ambientTypes    bool
	git             bool
	// outsideInclude makes the project tsconfig include declarations from a
	// directory beside the repository, which must never enter the program.
	outsideInclude bool
	// declarationSymlink replaces a generated declaration path with a
	// symlink whose target would satisfy $app/environment: "file-outside",
	// "file-archive", "dir-outside" or "dir-archive".
	declarationSymlink string
}

func writeSvelteKitFixture(t *testing.T, fixture svelteKitFixture) string {
	t.Helper()
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
	base := t.TempDir()
	root := filepath.Join(base, "repo")
	// Outside the repository: satisfies $app/environment if it were loaded.
	writeTestFile(t, base, "outside/kit.d.ts", "declare module '$app/environment' {\n\texport const browser: boolean;\n}\n")
	writeTestFile(t, base, "outside/types/src/routes/$types.d.ts", "declare module '$app/environment' {\n\texport const browser: boolean;\n}\n")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if fixture.git {
		if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
			t.Skipf("git unavailable: %v %s", err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(compilerPackage, filepath.Join(root, "node_modules", "typescript")); err != nil {
		t.Skipf("cannot link TypeScript compiler: %v", err)
	}
	writeTestFile(t, root, ".gitignore", "node_modules/\n.svelte-kit/\n")
	writeTestFile(t, root, "package.json", `{"name":"fixture","type":"module","devDependencies":{"@sveltejs/kit":"^2.0.0"}}`)
	writeTestFile(t, root, "node_modules/@sveltejs/kit/package.json", `{"name":"@sveltejs/kit","version":"2.0.0","types":"types/index.d.ts"}`)
	writeTestFile(t, root, "node_modules/@sveltejs/kit/types/index.d.ts", "declare module '$app/environment' {\n\texport const browser: boolean;\n\texport const dev: boolean;\n}\n")
	projectConfig := `{"extends":"./.svelte-kit/tsconfig.json","compilerOptions":{"allowJs":true,"checkJs":true,"strict":true,"skipLibCheck":true,"moduleResolution":"bundler"}}`
	if fixture.outsideInclude {
		projectConfig = `{"extends":"./.svelte-kit/tsconfig.json","compilerOptions":{"allowJs":true,"checkJs":true,"strict":true,"skipLibCheck":true,"moduleResolution":"bundler"},"include":[".svelte-kit/ambient.d.ts",".svelte-kit/non-ambient.d.ts","src/**/*.ts","../outside/*.d.ts"]}`
	}
	writeTestFile(t, root, "tsconfig.json", projectConfig)
	if fixture.generatedConfig {
		writeTestFile(t, root, ".svelte-kit/tsconfig.json", `{
	"compilerOptions": {
		"paths": {"$lib": ["../src/lib"], "$lib/*": ["../src/lib/*"]},
		"rootDirs": ["..", "./types"],
		"verbatimModuleSyntax": true,
		"isolatedModules": true,
		"lib": ["esnext", "DOM", "DOM.Iterable"],
		"moduleResolution": "bundler",
		"module": "esnext",
		"noEmit": true,
		"target": "esnext"
	},
	"include": ["ambient.d.ts", "non-ambient.d.ts", "./types/**/$types.d.ts", "../vite.config.ts", "../src/**/*.js", "../src/**/*.ts", "../src/**/*.svelte"],
	"exclude": ["../node_modules/**", "../src/service-worker.js", "../src/service-worker/**/*.js", "../src/service-worker.ts", "../src/service-worker/**/*.ts", "../src/service-worker.d.ts", "../src/service-worker/**/*.d.ts"]
}`)
		if fixture.ambientTypes {
			writeTestFile(t, root, ".svelte-kit/ambient.d.ts", "// this file is generated — do not edit it\n\n/// <reference types=\"@sveltejs/kit\" />\n")
			writeTestFile(t, root, ".svelte-kit/non-ambient.d.ts", "// this file is generated — do not edit it\n\n/// <reference types=\"@sveltejs/kit\" />\n\nexport {};\n")
		}
		linkTarget := map[string]string{
			"file-outside": filepath.Join(base, "outside", "kit.d.ts"),
			"file-archive": filepath.Join(root, "_archive", "kit.d.ts"),
			"dir-outside":  filepath.Join(base, "outside", "types"),
			"dir-archive":  filepath.Join(root, "_archive", "types"),
		}[fixture.declarationSymlink]
		if fixture.declarationSymlink != "" {
			if linkTarget == "" {
				t.Fatalf("unknown declaration symlink %q", fixture.declarationSymlink)
			}
			// The archived targets satisfy $app/environment on their own.
			writeTestFile(t, root, "_archive/kit.d.ts", "declare module '$app/environment' {\n\texport const browser: boolean;\n}\n")
			writeTestFile(t, root, "_archive/types/src/routes/$types.d.ts", "declare module '$app/environment' {\n\texport const browser: boolean;\n}\n")
			// A file symlink at ambient.d.ts, or a directory symlink at the
			// generated types/ tree the configuration includes.
			link := filepath.Join(root, ".svelte-kit", "ambient.d.ts")
			if strings.HasPrefix(fixture.declarationSymlink, "dir-") {
				link = filepath.Join(root, ".svelte-kit", "types")
			}
			if err := os.Symlink(linkTarget, link); err != nil {
				t.Skipf("cannot create declaration symlink: %v", err)
			}
		}
	}
	writeTestFile(t, root, "src/lib/util.ts", "export function helper(): number {\n\treturn 1;\n}\n")
	writeTestFile(t, root, "src/routes/+page.ts", "import { browser } from '$app/environment';\nimport { helper } from '$lib/util';\n\nexport const load = (): number => (browser ? helper() : 0);\n")
	// An archived owner tree inside the configuration's include glob. Its
	// declaration would satisfy $app/environment on its own, so admitting it to
	// the compiler program would hide a missing or stale generated config.
	writeTestFile(t, root, "src/_archive/kit.d.ts", "declare module '$app/environment' {\n\texport const browser: boolean;\n}\n")
	writeTestFile(t, root, "_archive/retired.ts", "import { dev } from '$app/environment';\nexport function retiredArchiveSymbol(): boolean {\n\treturn dev;\n}\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if fixture.git {
		if out, err := exec.Command("git", "-C", root, "add", "-A").CombinedOutput(); err != nil {
			t.Fatalf("git add: %v %s", err, out)
		}
	}
	return root
}

func scanSvelteKitFixture(t *testing.T, root string) (*Store, ScanResult) {
	t.Helper()
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	result, err := store.ScanRepository(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Capabilities["typescript.definitions"] != "compiler_exact" {
		t.Fatalf("compiler-backed capability was not published: %#v %v", result.Generation.Capabilities, result.Limitations)
	}
	return store, result
}

func appEnvironmentLimitations(result ScanResult) []string {
	var found []string
	for _, limitation := range result.Limitations {
		if strings.Contains(limitation, "$app/environment") {
			found = append(found, limitation)
		}
	}
	return found
}

func assertNoArchivedGraphState(t *testing.T, store *Store, result ScanResult) {
	t.Helper()
	if state := archivedGraphState(t, store, result); state != "" {
		t.Fatal(state)
	}
}

func archivedGraphState(t *testing.T, store *Store, result ScanResult) string {
	t.Helper()
	var inputs, nodes, occurrences int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_scan_inputs WHERE generation_id = ? AND file_path LIKE '%\_archive%' ESCAPE '\'`, result.Generation.ID).Scan(&inputs); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM graph_nodes WHERE generation_id = ? AND (display_name LIKE '%\_archive%' ESCAPE '\' OR qualified_name LIKE '%\_archive%' ESCAPE '\' OR metadata LIKE '%\_archive%' ESCAPE '\' OR display_name = 'retiredArchiveSymbol')`, result.Generation.ID).Scan(&nodes); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM symbol_occurrences WHERE generation_id = ? AND file_path LIKE '%\_archive%' ESCAPE '\'`, result.Generation.ID).Scan(&occurrences); err != nil {
		t.Fatal(err)
	}
	if inputs != 0 || nodes != 0 || occurrences != 0 {
		return fmt.Sprintf("_archive reached the persisted graph: inputs=%d nodes=%d occurrences=%d", inputs, nodes, occurrences)
	}
	for _, limitation := range result.Limitations {
		if strings.Contains(limitation, "_archive") {
			return "_archive source was analysed: " + limitation
		}
	}
	return ""
}

func TestScanResolvesSvelteKitGeneratedConfiguration(t *testing.T) {
	for _, git := range []bool{true, false} {
		name := "walk"
		if git {
			name = "git"
		}
		t.Run(name, func(t *testing.T) {
			root := writeSvelteKitFixture(t, svelteKitFixture{generatedConfig: true, ambientTypes: true, git: git})
			store, result := scanSvelteKitFixture(t, root)
			if found := appEnvironmentLimitations(result); len(found) != 0 {
				t.Fatalf("$app/environment did not resolve through the generated SvelteKit configuration: %v", found)
			}
			for _, limitation := range result.Limitations {
				if strings.Contains(limitation, "$lib") || strings.Contains(limitation, "compiler omitted") || strings.Contains(limitation, ".svelte-kit") {
					t.Fatalf("generated SvelteKit configuration was not applied: %s", limitation)
				}
			}
			if result.Generation.Status != GraphCurrent {
				t.Fatalf("generation status = %s: %v", result.Generation.Status, result.Limitations)
			}
			identity, err := store.EnsureRepositoryIdentity(context.Background(), root, "")
			if err != nil {
				t.Fatal(err)
			}
			dependencies, err := store.FindModuleDependencies(context.Background(), identity, "src/routes/+page.ts", false, 1, DefaultGraphLimits())
			if err != nil {
				t.Fatal(err)
			}
			resolvedLib := false
			for _, node := range dependencies.Result.Nodes {
				if node.QualifiedName == "src/lib/util.ts" {
					resolvedLib = true
				}
			}
			if !resolvedLib {
				t.Fatalf("$lib import did not resolve through generated paths: %#v", dependencies.Result)
			}
			// Generated declarations are compiler context, never graph source.
			var generated int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM symbol_occurrences WHERE generation_id = ? AND (file_path LIKE '.svelte-kit/%' OR file_path LIKE 'node_modules/%')`, result.Generation.ID).Scan(&generated); err != nil {
				t.Fatal(err)
			}
			if generated != 0 {
				t.Fatalf("generated or dependency declarations became graph symbols: %d", generated)
			}
			assertNoArchivedGraphState(t, store, result)
		})
	}
}

func TestScanReportsMissingSvelteKitGeneratedConfiguration(t *testing.T) {
	root := writeSvelteKitFixture(t, svelteKitFixture{git: true})
	store, result := scanSvelteKitFixture(t, root)
	if len(appEnvironmentLimitations(result)) == 0 {
		t.Fatalf("missing generated config did not leave $app/environment unresolved: %v", result.Limitations)
	}
	missingConfig := false
	for _, limitation := range result.Limitations {
		if strings.Contains(limitation, ".svelte-kit/tsconfig.json") {
			missingConfig = true
		}
	}
	if !missingConfig {
		t.Fatalf("missing generated config was not diagnosed: %v", result.Limitations)
	}
	assertNoArchivedGraphState(t, store, result)
}

func TestScanReportsStaleSvelteKitGeneratedDeclarations(t *testing.T) {
	root := writeSvelteKitFixture(t, svelteKitFixture{generatedConfig: true, git: true})
	store, result := scanSvelteKitFixture(t, root)
	if len(appEnvironmentLimitations(result)) == 0 {
		t.Fatalf("generated config without its declarations did not leave $app/environment unresolved: %v", result.Limitations)
	}
	assertNoArchivedGraphState(t, store, result)
}

func TestScanIgnoresSvelteKitDeclarationsOutsideRepository(t *testing.T) {
	root := writeSvelteKitFixture(t, svelteKitFixture{generatedConfig: true, git: true, outsideInclude: true})
	store, result := scanSvelteKitFixture(t, root)
	if len(appEnvironmentLimitations(result)) == 0 {
		t.Fatalf("a declaration outside the repository satisfied $app/environment: %v", result.Limitations)
	}
	assertNoArchivedGraphState(t, store, result)
}

// symlinkedDeclarationCases are the generated declaration paths whose targets
// leave the authorized boundary through a file or a parent-directory symlink.
var symlinkedDeclarationCases = []string{"file-outside", "file-archive", "dir-outside", "dir-archive"}

func TestScanRefusesSymlinkedSvelteKitDeclarationsLeavingBoundary(t *testing.T) {
	for _, link := range symlinkedDeclarationCases {
		t.Run(link, func(t *testing.T) {
			root := writeSvelteKitFixture(t, svelteKitFixture{generatedConfig: true, git: true, declarationSymlink: link})
			store, result := scanSvelteKitFixture(t, root)
			if len(appEnvironmentLimitations(result)) == 0 {
				t.Fatalf("a symlinked declaration leaving the boundary satisfied $app/environment: %v", result.Limitations)
			}
			assertNoArchivedGraphState(t, store, result)
		})
	}
}

// mutateTypeScriptExtractor replaces exactly one site in the embedded
// extractor for the rest of the test. Requiring a single match fails closed if
// the extractor changes, so an unrelated failure is never taken as evidence.
func mutateTypeScriptExtractor(t *testing.T, before, after string) {
	t.Helper()
	original := typeScriptCompilerScript
	if count := strings.Count(string(original), before); count != 1 {
		t.Fatalf("expected one extractor mutation site, found %d: %q", count, before)
	}
	typeScriptCompilerScript = []byte(strings.Replace(string(original), before, after, 1))
	t.Cleanup(func() { typeScriptCompilerScript = original })
}

// Causal controls: each removes one guard this change relies on and proves the
// regression above would then fail.
func TestSvelteKitScanGuardsAreCausal(t *testing.T) {
	t.Run("configured declarations loaded", func(t *testing.T) {
		mutateTypeScriptExtractor(t, "rootNames: [...new Set([...group.files, ...group.declarations])]", "rootNames: group.files")
		root := writeSvelteKitFixture(t, svelteKitFixture{generatedConfig: true, ambientTypes: true, git: true})
		_, result := scanSvelteKitFixture(t, root)
		if len(appEnvironmentLimitations(result)) == 0 {
			t.Fatalf("without generated declarations $app/environment still resolved: %v", result.Limitations)
		}
	})
	// Each boundary check is removed in turn; every fixture that crosses
	// that boundary must then resolve $app/environment through the refused
	// declaration, hiding the stale generated config.
	boundaries := []struct {
		name, before, after string
		fixtures            []svelteKitFixture
	}{
		{
			name:   "outside repository refused",
			before: "\n  if (relative === \"\" || relative.startsWith(\"..\") || path.isAbsolute(relative)) return false;",
			after:  "",
			fixtures: []svelteKitFixture{
				{generatedConfig: true, git: true, outsideInclude: true},
				{generatedConfig: true, git: true, declarationSymlink: "file-outside"},
				{generatedConfig: true, git: true, declarationSymlink: "dir-outside"},
			},
		},
		{
			name:   "excluded directory refused",
			before: "return !relative.split(path.sep).some((part) => excludedDirectories.has(part));",
			after:  "return true;",
			fixtures: []svelteKitFixture{
				{generatedConfig: true, git: true},
				{generatedConfig: true, git: true, declarationSymlink: "file-archive"},
				{generatedConfig: true, git: true, declarationSymlink: "dir-archive"},
			},
		},
	}
	for _, boundary := range boundaries {
		for _, fixture := range boundary.fixtures {
			t.Run(fmt.Sprintf("%s outside=%v symlink=%q", boundary.name, fixture.outsideInclude, fixture.declarationSymlink), func(t *testing.T) {
				mutateTypeScriptExtractor(t, boundary.before, boundary.after)
				root := writeSvelteKitFixture(t, fixture)
				_, result := scanSvelteKitFixture(t, root)
				if found := appEnvironmentLimitations(result); len(found) != 0 {
					t.Fatalf("refused declaration did not mask the stale generated config: %v", found)
				}
			})
		}
	}
	for _, git := range []bool{true, false} {
		t.Run(fmt.Sprintf("archive excluded git=%v", git), func(t *testing.T) {
			if !graphIgnoredDirectories["_archive"] {
				t.Fatal("_archive is no longer a graph-ignored directory")
			}
			delete(graphIgnoredDirectories, "_archive")
			t.Cleanup(func() { graphIgnoredDirectories["_archive"] = true })
			root := writeSvelteKitFixture(t, svelteKitFixture{generatedConfig: true, ambientTypes: true, git: git})
			store, result := scanSvelteKitFixture(t, root)
			if archivedGraphState(t, store, result) == "" {
				t.Fatal("removing the _archive exclusion left no archived graph state; the regression cannot detect it")
			}
		})
	}
}
