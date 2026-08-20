package knowledge

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	statepkg "github.com/openexec/openexec/pkg/db/state"
)

func TestRepositoryContextIsVersionedLossyProjection(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "dep.ts", "export function dep(): number { return 1 }\n")
	writeTestFile(t, root, "main.ts", "import { dep } from './dep'\nexport const main = () => dep()\n")
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
	report := &statepkg.CompletionReport{CanComplete: false, Verified: []statepkg.CompletionClaim{{Criterion: "Selected unit tests pass"}}, NotVerified: []statepkg.CompletionClaim{{Criterion: "Full suite passes", Status: "not_run"}}}
	projection, err := store.BuildRepositoryContext(ctx, identity, []string{"main"}, "task", "run", "plan", report)
	if err != nil {
		t.Fatal(err)
	}
	if projection.SchemaVersion != 1 || projection.SourceSystem != "openexec" || projection.Freshness != FreshnessCurrent || projection.OpenExecReference.ResourceVersion == "" {
		t.Fatalf("projection identity is incomplete: %#v", projection)
	}
	withoutSymbols, err := store.BuildRepositoryContext(ctx, identity, nil, "task", "run", "plan", report)
	if err != nil {
		t.Fatal(err)
	}
	if withoutSymbols.OpenExecReference.ResourceVersion == projection.OpenExecReference.ResourceVersion {
		t.Fatal("different projection payloads reused one resource version")
	}
	overviewProjection, err := store.BuildRepositoryContext(ctx, identity, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if overviewProjection.ResolvedSymbols == nil || overviewProjection.ModuleDependencies == nil || overviewProjection.Limitations == nil || overviewProjection.ValidationSummary.Verified == nil || overviewProjection.ValidationSummary.NotVerified == nil {
		t.Fatalf("overview projection contains nil collections: %#v", overviewProjection)
	}
	if len(overviewProjection.ValidationSummary.NotVerified) != 1 || !strings.Contains(overviewProjection.ValidationSummary.NotVerified[0], "unevaluated") || overviewProjection.ValidationSummary.CanComplete {
		t.Fatalf("missing validation report was presented as empty evidence: %#v", overviewProjection.ValidationSummary)
	}
	if len(overviewProjection.ResolvedSymbols) == 0 {
		t.Fatalf("default projection omitted repository symbols: %#v", overviewProjection)
	}
	if len(overviewProjection.ModuleDependencies) != 1 || overviewProjection.ModuleDependencies[0].To != "dep.ts" {
		t.Fatalf("default projection omitted module dependency: %#v", overviewProjection.ModuleDependencies)
	}
	if len(projection.ResolvedSymbols) != 1 || projection.ResolvedSymbols[0].SafeLocation != "main.ts:2" {
		t.Fatalf("safe symbol projection is wrong: %#v", projection.ResolvedSymbols)
	}
	if len(projection.ModuleDependencies) != 1 || projection.ModuleDependencies[0].To != "dep.ts" {
		t.Fatalf("module dependency projection is wrong: %#v", projection.ModuleDependencies)
	}
	if len(projection.ValidationSummary.Verified) != 1 || len(projection.ValidationSummary.NotVerified) != 1 || projection.ValidationSummary.CanComplete {
		t.Fatalf("claim projection is wrong: %#v", projection.ValidationSummary)
	}
	if projection.Provenance.Worktree.StateHash == "" || len(projection.Provenance.Extractors) == 0 {
		t.Fatalf("projection omitted provenance: %#v", projection.Provenance)
	}
	if scope := projection.Selections["resolved_symbols"]; scope.Scope == "" || scope.Returned != 1 || scope.Truncated {
		t.Fatalf("explicit symbol selection scope is wrong: %#v", scope)
	}
	// The Agent Console read model must not carry source bytes or exact byte
	// ranges; it receives only a safe line locator and stable OpenExec reference.
	if projection.ResolvedSymbols[0].SafeLocation == filepath.Join(root, "main.ts") {
		t.Fatal("projection exposed an authoritative absolute source location")
	}
}

func TestRepositoryContextDefaultOverviewIsBoundedAndCarriesGraphLimitations(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var source strings.Builder
	source.WriteString("package sample\n\n")
	for i := 0; i < defaultRepositoryContextSymbolLimit+5; i++ {
		fmt.Fprintf(&source, "func Exported%02d() {}\n", i)
	}
	writeTestFile(t, root, "sample.go", source.String())
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	result, err := store.ScanRepository(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, `UPDATE graph_generations SET limitations = ? WHERE id = ?`, `["fixture graph limitation"]`, result.Generation.ID); err != nil {
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
	if got := len(projection.ResolvedSymbols); got != defaultRepositoryContextSymbolLimit {
		t.Fatalf("default projection has %d symbols, want %d", got, defaultRepositoryContextSymbolLimit)
	}
	joined := strings.Join(projection.Limitations, "\n")
	if !strings.Contains(joined, "fixture graph limitation") {
		t.Fatalf("projection omitted graph limitation: %#v", projection.Limitations)
	}
	if !strings.Contains(joined, "bounded to 50 representative symbols") {
		t.Fatalf("projection omitted bounded-overview limitation: %#v", projection.Limitations)
	}
	scope := projection.Selections["resolved_symbols"]
	if !scope.Truncated || scope.Returned != defaultRepositoryContextSymbolLimit || scope.Total != defaultRepositoryContextSymbolLimit+5 || scope.TruncatedCount != 5 {
		t.Fatalf("symbol truncation is not exact: %#v", scope)
	}
	for _, name := range []string{"module_dependencies", "dead_code_candidates", "hotspots", "module_sketch"} {
		if projection.Selections[name].Scope == "" {
			t.Fatalf("bounded list %s has no selection scope: %#v", name, projection.Selections)
		}
	}
}

func TestRepositoryContextReportsCommitAndDirtyWorktreeCount(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, ".gitignore", ".openexec/\n")
	writeTestFile(t, root, "main.go", "package sample\nfunc Main() {}\n")
	for _, args := range [][]string{{"init"}, {"add", "."}, {"-c", "user.name=OpenExec Test", "-c", "user.email=test@openexec.local", "commit", "-m", "fixture"}} {
		command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.ScanRepository(ctx, root); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "main.go", "package sample\n\nfunc Main() {}\n")
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	projection, err := store.BuildRepositoryContext(ctx, identity, nil, "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Provenance.BaseCommit == "" || projection.Provenance.Worktree.State != "dirty" || projection.Provenance.Worktree.DirtyCount != 1 {
		t.Fatalf("dirty provenance = %#v", projection.Provenance)
	}
}
