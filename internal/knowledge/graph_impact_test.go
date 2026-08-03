package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestImpactAnalysisExplainsCallersTestsAndValidationScope(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "service.go", "package sample\nfunc A() string { return \"a\" }\n")
	writeTestFile(t, root, "caller.go", "package sample\nfunc B() string { return A() }\n")
	writeTestFile(t, root, "outer.go", "package sample\nfunc C() string { return B() }\n")
	writeTestFile(t, root, "service_test.go", "package sample\nimport \"testing\"\nfunc TestA(t *testing.T) { if A() == \"\" { t.Fatal(\"empty\") } }\n")
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
	resolved, err := store.ResolveGraphSymbol(ctx, identity, "A", "service.go", "function", 20)
	if err != nil || resolved.Result.Candidate == nil {
		t.Fatalf("resolve A: %#v %v", resolved, err)
	}
	impact, err := store.ImpactAnalysis(ctx, identity, []string{resolved.Result.Candidate.Symbol.ID}, 2, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if impact.Generation.Freshness != FreshnessCurrent || impact.Resolution.Status != "bounded" {
		t.Fatalf("impact did not declare current bounded scope: %#v", impact)
	}
	foundCaller := false
	foundTransitiveCaller := false
	for _, caller := range impact.Result.DirectCallers {
		if caller.Node.DisplayName == "B" && len(caller.Path) == 1 && caller.Path[0].EdgeType == "calls" {
			foundCaller = true
		}
	}
	for _, caller := range impact.Result.AffectedCallers {
		if caller.Node.DisplayName == "C" && len(caller.Path) == 2 {
			foundTransitiveCaller = true
		}
	}
	if !foundCaller || !foundTransitiveCaller {
		t.Fatalf("bounded caller paths were not explained: %#v", impact.Result.DirectCallers)
	}
	if len(impact.Result.RelatedTests) != 1 || impact.Result.RelatedTests[0].FilePath != "service_test.go" {
		t.Fatalf("structural test link missing: %#v", impact.Result.RelatedTests)
	}
	if len(impact.Result.ValidationRecommendations) != 1 || len(impact.Result.ValidationRecommendations[0].TestFiles) != 1 {
		t.Fatalf("validation recommendation missing: %#v", impact.Result.ValidationRecommendations)
	}
	if len(impact.Result.ValidationRecommendations[0].GraphPaths) == 0 {
		t.Fatal("validation recommendation has no explaining graph path")
	}

	writeTestFile(t, root, "service.go", "package sample\nfunc A() string { return \"changed\" }\n")
	stale, err := store.ImpactAnalysis(ctx, identity, []string{resolved.Result.Candidate.Symbol.ID}, 2, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if stale.Generation.Freshness != FreshnessStale || stale.Resolution.Status != "unavailable" || len(stale.Result.DirectCallers) != 0 {
		t.Fatalf("stale graph produced impact conclusions: %#v", stale)
	}
}
