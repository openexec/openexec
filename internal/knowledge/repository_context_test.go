package knowledge

import (
	"context"
	"os"
	"path/filepath"
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
	if len(projection.ResolvedSymbols) != 1 || projection.ResolvedSymbols[0].SafeLocation != "main.ts:2" {
		t.Fatalf("safe symbol projection is wrong: %#v", projection.ResolvedSymbols)
	}
	if len(projection.ModuleDependencies) != 1 || projection.ModuleDependencies[0].To != "dep.ts" {
		t.Fatalf("module dependency projection is wrong: %#v", projection.ModuleDependencies)
	}
	if len(projection.ValidationSummary.Verified) != 1 || len(projection.ValidationSummary.NotVerified) != 1 || projection.ValidationSummary.CanComplete {
		t.Fatalf("claim projection is wrong: %#v", projection.ValidationSummary)
	}
	// The Agent Console read model must not carry source bytes or exact byte
	// ranges; it receives only a safe line locator and stable OpenExec reference.
	if projection.ResolvedSymbols[0].SafeLocation == filepath.Join(root, "main.ts") {
		t.Fatal("projection exposed an authoritative absolute source location")
	}
}
