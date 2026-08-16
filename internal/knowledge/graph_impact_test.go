package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	refreshed, err := store.ImpactAnalysis(ctx, identity, []string{resolved.Result.Candidate.Symbol.ID}, 2, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Generation.Freshness != FreshnessCurrent || refreshed.Resolution.Status != "bounded" || len(refreshed.Result.DirectCallers) == 0 {
		t.Fatalf("impact did not refresh before deriving conclusions: %#v", refreshed)
	}
}

func TestChangedImpactAnalysisResolvesFilesInOneNamedGeneration(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "service.go", "package sample\nfunc A() string { return \"a\" }\n")
	writeTestFile(t, root, "caller.go", "package sample\nfunc B() string { return A() }\n")
	writeTestFile(t, root, "service_test.go", "package sample\nimport \"testing\"\nfunc TestA(t *testing.T) { _ = A() }\n")
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
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.ChangedImpactAnalysis(ctx, identity, ChangedImpactRequest{
		Files: []string{"service.go", "missing.go", "service.go"}, MaxDepth: 2,
	}, DefaultGraphLimits())
	if err != nil {
		t.Fatal(err)
	}
	if result.Generation.Freshness != FreshnessCurrent || result.Provenance.GraphVersion == "" || result.Provenance.GraphVersion != result.Generation.GraphVersion {
		t.Fatalf("generation provenance = %#v", result)
	}
	if len(result.ChangedSymbols) != 1 || result.ChangedSymbols[0].DisplayName != "A" {
		t.Fatalf("changed symbols = %#v", result.ChangedSymbols)
	}
	if len(result.Propagation.DirectCallers) != 2 || len(result.Propagation.RelatedTests) != 1 {
		t.Fatalf("propagation = %#v", result.Propagation)
	}
	if len(result.UnresolvedFiles) != 1 || result.UnresolvedFiles[0] != "missing.go: no symbols in the current graph generation" {
		t.Fatalf("unresolved files = %#v", result.UnresolvedFiles)
	}
	if len(result.ValidationRecommendations) != 1 || result.Resolution.Status != "bounded" {
		t.Fatalf("validation result = %#v", result)
	}
}

func TestChangedImpactAnalysisRefusesUnsafeAndUnboundedRequests(t *testing.T) {
	store := &Store{}
	tooManyFiles := make([]string, MaxChangedImpactFiles+1)
	tooManySymbols := make([]string, MaxChangedImpactSymbols+1)
	for index := range tooManySymbols {
		tooManySymbols[index] = "symbol-" + strconv.Itoa(index)
	}
	tests := []struct {
		name    string
		request ChangedImpactRequest
	}{
		{name: "empty", request: ChangedImpactRequest{}},
		{name: "parent traversal", request: ChangedImpactRequest{Files: []string{"../secret.go"}}},
		{name: "absolute", request: ChangedImpactRequest{Files: []string{"/tmp/secret.go"}}},
		{name: "file cap", request: ChangedImpactRequest{Files: tooManyFiles}},
		{name: "symbol cap", request: ChangedImpactRequest{SymbolIDs: tooManySymbols}},
		{name: "depth cap", request: ChangedImpactRequest{Files: []string{"main.go"}, MaxDepth: MaxChangedImpactDepth + 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.ChangedImpactAnalysis(t.Context(), RepositoryIdentity{}, test.request, DefaultGraphLimits()); err == nil {
				t.Fatal("invalid request was accepted")
			} else if _, ok := err.(*ImpactRequestError); !ok {
				t.Fatalf("error = %T %v, want ImpactRequestError", err, err)
			}
		})
	}
	tightLimits := DefaultChangedImpactLimits()
	tightLimits.MaxDepth = 1
	if _, err := store.ChangedImpactAnalysis(t.Context(), RepositoryIdentity{}, ChangedImpactRequest{SymbolIDs: []string{"symbol"}, MaxDepth: 2}, tightLimits); err == nil {
		t.Fatal("caller traversal limit was silently widened")
	} else if _, ok := err.(*ImpactRequestError); !ok {
		t.Fatalf("tight-limit error = %T %v, want ImpactRequestError", err, err)
	}
}

func TestImpactDeduplicationKeepsShortestExplanation(t *testing.T) {
	impacts := map[string]ImpactNode{}
	keepShortestImpact(impacts, ImpactNode{Node: GraphNode{ID: "consumer"}, Path: []ImpactPath{{EdgeID: "outer"}, {EdgeID: "inner"}}})
	keepShortestImpact(impacts, ImpactNode{Node: GraphNode{ID: "consumer"}, Path: []ImpactPath{{EdgeID: "direct"}}})
	if got := impacts["consumer"].Path; len(got) != 1 || got[0].EdgeID != "direct" {
		t.Fatalf("impact path = %#v", got)
	}

	tests := map[string]RelatedTest{}
	keepShortestTest(tests, RelatedTest{FilePath: "consumer_test.go", Path: []ImpactPath{{EdgeID: "outer"}, {EdgeID: "inner"}}})
	keepShortestTest(tests, RelatedTest{FilePath: "consumer_test.go", Path: []ImpactPath{{EdgeID: "direct"}}})
	if got := tests["consumer_test.go"].Path; len(got) != 1 || got[0].EdgeID != "direct" {
		t.Fatalf("test path = %#v", got)
	}
}

func TestChangedImpactAnalysisEnumeratesTraversalBounds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeTestFile(t, root, "target.go", "package sample\nfunc Target() {}\n")
	writeTestFile(t, root, "callers.go", "package sample\nfunc CallerA() { Target() }\nfunc CallerB() { Target() }\nfunc CallerC() { Target() }\n")
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
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}

	nodeLimits := DefaultChangedImpactLimits()
	nodeLimits.MaxNodes = 1
	nodeResult, err := store.ChangedImpactAnalysis(ctx, identity, ChangedImpactRequest{Files: []string{"target.go"}, MaxDepth: 2}, nodeLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !nodeResult.Truncated || !containsText(nodeResult.Unresolved, "caller propagation incomplete") {
		t.Fatalf("node-bound result did not enumerate incomplete root: %#v", nodeResult)
	}

	edgeLimits := DefaultChangedImpactLimits()
	edgeLimits.MaxEdges = 1
	edgeResult, err := store.ChangedImpactAnalysis(ctx, identity, ChangedImpactRequest{Files: []string{"target.go"}, MaxDepth: 2}, edgeLimits)
	if err != nil {
		t.Fatal(err)
	}
	if !edgeResult.Truncated || !containsText(edgeResult.Unresolved, "exceeded the 1-edge adjacency bound") {
		t.Fatalf("edge-bound result hid truncated adjacency: %#v", edgeResult)
	}
}

func TestChangedImpactAnalysisBoundsFileExpansionWithoutRefusingResult(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	var source strings.Builder
	source.WriteString("package sample\n")
	for index := 0; index < MaxChangedImpactSymbols+5; index++ {
		source.WriteString("func Symbol" + strconv.Itoa(index) + "() {}\n")
	}
	writeTestFile(t, root, "large.go", source.String())
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
	identity, err := store.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.ChangedImpactAnalysis(ctx, identity, ChangedImpactRequest{Files: []string{"large.go"}, MaxDepth: 1}, DefaultChangedImpactLimits())
	if err != nil {
		t.Fatalf("file expansion was refused: %v", err)
	}
	if len(result.ChangedSymbols) != MaxChangedImpactSymbols || !result.Truncated {
		t.Fatalf("bounded file expansion = %d symbols, truncated=%v", len(result.ChangedSymbols), result.Truncated)
	}
	if !containsText(result.UnresolvedFiles, "selected 200 of 205 symbols") || !containsText(result.Limitations, "file-granularity over-approximation") {
		t.Fatalf("file expansion honesty signals missing: %#v", result)
	}
}

func containsText(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
