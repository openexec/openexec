package knowledge

import (
	"context"
	"sort"
	"strings"
	"testing"
)

// Deterministic disposable fixture (no provider, no network): a TypeScript
// import graph scanned by the real scanner, persisted, and read back through a
// reopened store so FindModuleDependencies runs against stored state only.
//
//	fan.ts   -> f1.ts, f2.ts, f3.ts          (three edges from one node)
//	pair.ts  -> p1.ts, p2.ts; p1.ts -> leaf.ts (spent budget, unread edge next)
//	flat.ts  -> q1.ts, q2.ts                 (spent budget, nothing unread)
//	x.ts    <-> y.ts                         (cycle)
//	f1.ts, f2.ts, f3.ts are all imported by fan.ts and shared.ts (dependants)
func scanModuleBoundsFixture(t *testing.T) (*Store, RepositoryIdentity) {
	t.Helper()
	ctx := context.Background()
	root := t.TempDir()
	leaf := func(name string) string { return "export function " + name + "(): number { return 1 }\n" }
	imports := func(body string, targets ...string) string {
		var out strings.Builder
		for _, target := range targets {
			out.WriteString("import { " + target + " } from './" + target + "'\n")
		}
		out.WriteString(body)
		return out.String()
	}
	writeTestFile(t, root, "f1.ts", leaf("f1"))
	writeTestFile(t, root, "f2.ts", leaf("f2"))
	writeTestFile(t, root, "f3.ts", leaf("f3"))
	writeTestFile(t, root, "fan.ts", imports("export const fan = () => f1() + f2() + f3()\n", "f1", "f2", "f3"))
	writeTestFile(t, root, "shared.ts", imports("export const shared = () => f1() + f2() + f3()\n", "f1", "f2", "f3"))
	writeTestFile(t, root, "leaf.ts", leaf("leaf"))
	writeTestFile(t, root, "p1.ts", imports("export const p1 = () => leaf()\n", "leaf"))
	writeTestFile(t, root, "p2.ts", leaf("p2"))
	writeTestFile(t, root, "pair.ts", imports("export const pair = () => p1() + p2()\n", "p1", "p2"))
	writeTestFile(t, root, "q1.ts", leaf("q1"))
	writeTestFile(t, root, "q2.ts", leaf("q2"))
	writeTestFile(t, root, "flat.ts", imports("export const flat = () => q1() + q2()\n", "q1", "q2"))
	writeTestFile(t, root, "x.ts", imports("export const x = (): number => y()\n", "y"))
	writeTestFile(t, root, "y.ts", imports("export const y = (): number => x()\n", "x"))

	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ScanRepository(ctx, root); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	identity, err := reopened.EnsureRepositoryIdentity(ctx, root, "")
	if err != nil {
		t.Fatal(err)
	}
	return reopened, identity
}

func moduleDeps(t *testing.T, store *Store, identity RepositoryIdentity, module string, reverse bool, depth int, limits GraphLimits) QueryEnvelope[ModuleDependencyResult] {
	t.Helper()
	result, err := store.FindModuleDependencies(context.Background(), identity, module, reverse, depth, limits)
	if err != nil {
		t.Fatalf("FindModuleDependencies(%s, reverse=%t, depth=%d, %+v): %v", module, reverse, depth, limits, err)
	}
	if result.Generation.Freshness != FreshnessCurrent {
		t.Fatalf("readback generation is not current: %#v", result.Generation)
	}
	return result
}

func edgeIDs(edges []GraphEdge) []string {
	ids := make([]string, 0, len(edges))
	for _, edge := range edges {
		ids = append(ids, edge.ID)
	}
	return ids
}

func hasDepthNote(limitations []string, requested, limit int) bool {
	want := depthLimitationNote(requested, limit)
	for _, limitation := range limitations {
		if limitation == want {
			return true
		}
	}
	return false
}

func edgeLimits(maxDepth, maxEdges int) GraphLimits {
	return GraphLimits{MaxDepth: maxDepth, MaxNodes: 50, MaxEdges: maxEdges, MaxBytes: 256 * 1024}
}

func TestModuleDependenciesEdgeLimitDisclosesOnlyOmittedEdges(t *testing.T) {
	store, identity := scanModuleBoundsFixture(t)

	full := moduleDeps(t, store, identity, "fan.ts", false, 1, edgeLimits(2, 10))
	if full.Truncated || len(full.Result.Edges) != 3 {
		t.Fatalf("unbounded fan-out = truncated %t, %d edges; want complete 3", full.Truncated, len(full.Result.Edges))
	}
	allIDs := edgeIDs(full.Result.Edges)
	if !sort.StringsAreSorted(allIDs) {
		t.Fatalf("edges are not in ID order: %v", allIDs)
	}
	for _, edge := range full.Result.Edges {
		if edge.Type != "imports" || edge.Metadata["specifier"] == "" {
			t.Fatalf("import edge lost its type or specifier metadata: %#v", edge)
		}
	}

	// One more reachable edge than the limit: the omitted edge is disclosed and
	// the kept edges are the first two in ID order.
	over := moduleDeps(t, store, identity, "fan.ts", false, 1, edgeLimits(2, 2))
	if !over.Truncated {
		t.Fatalf("fan.ts has 3 import edges but MaxEdges=2 returned Truncated=false: %v", edgeIDs(over.Result.Edges))
	}
	if got := edgeIDs(over.Result.Edges); len(got) != 2 || got[0] != allIDs[0] || got[1] != allIDs[1] {
		t.Fatalf("over-limit edges = %v, want first two by ID of %v", got, allIDs)
	}

	// Exactly at the limit and one below it: complete, nothing omitted.
	for _, maxEdges := range []int{3, 4} {
		result := moduleDeps(t, store, identity, "fan.ts", false, 1, edgeLimits(2, maxEdges))
		if result.Truncated || len(result.Result.Edges) != 3 {
			t.Fatalf("MaxEdges=%d with 3 reachable edges = truncated %t, %d edges; want complete", maxEdges, result.Truncated, len(result.Result.Edges))
		}
	}

	// Budget spent exactly at level 0; p1.ts still has an unread edge.
	pair := moduleDeps(t, store, identity, "pair.ts", false, 2, edgeLimits(2, 2))
	if !pair.Truncated || len(pair.Result.Edges) != 2 {
		t.Fatalf("spent budget with unread p1->leaf edge = truncated %t, %d edges; want truncated with 2", pair.Truncated, len(pair.Result.Edges))
	}
	pairFull := moduleDeps(t, store, identity, "pair.ts", false, 2, edgeLimits(2, 3))
	if pairFull.Truncated || len(pairFull.Result.Edges) != 3 {
		t.Fatalf("pair.ts at exactly 3 edges = truncated %t, %d edges; want complete", pairFull.Truncated, len(pairFull.Result.Edges))
	}

	// Budget spent exactly at level 0; q1.ts and q2.ts import nothing.
	flat := moduleDeps(t, store, identity, "flat.ts", false, 2, edgeLimits(2, 2))
	if flat.Truncated || len(flat.Result.Edges) != 2 {
		t.Fatalf("spent budget with no unread edge = truncated %t, %d edges; want complete 2", flat.Truncated, len(flat.Result.Edges))
	}

	// Reverse direction reads incoming import edges with the same bound.
	dependants := moduleDeps(t, store, identity, "f1.ts", true, 1, edgeLimits(2, 1))
	if !dependants.Truncated || len(dependants.Result.Edges) != 1 || dependants.Query.Type != "find_module_dependants" {
		t.Fatalf("f1.ts has 2 dependants but MaxEdges=1 = %#v", dependants)
	}
	dependantsFull := moduleDeps(t, store, identity, "f1.ts", true, 1, edgeLimits(2, 2))
	if dependantsFull.Truncated || len(dependantsFull.Result.Edges) != 2 {
		t.Fatalf("f1.ts dependants at exactly 2 edges = truncated %t, %d edges; want complete", dependantsFull.Truncated, len(dependantsFull.Result.Edges))
	}
}

func TestModuleDependenciesCycleCountsNoEdgeTwice(t *testing.T) {
	store, identity := scanModuleBoundsFixture(t)

	// x -> y -> x: both edges fit exactly; revisiting x must not be reported as
	// an omitted edge, however deep the walk is allowed to go.
	for _, depth := range []int{2, 5} {
		result := moduleDeps(t, store, identity, "x.ts", false, depth, edgeLimits(5, 2))
		if result.Truncated || len(result.Result.Edges) != 2 || len(result.Result.Nodes) != 1 {
			t.Fatalf("cycle depth=%d at exactly 2 edges = truncated %t, %d edges, %d nodes; want complete 2/1", depth, result.Truncated, len(result.Result.Edges), len(result.Result.Nodes))
		}
	}
	// With one edge of budget, y->x is a distinct unread edge and is disclosed.
	cut := moduleDeps(t, store, identity, "x.ts", false, 3, edgeLimits(5, 1))
	if !cut.Truncated || len(cut.Result.Edges) != 1 {
		t.Fatalf("cycle with y->x unread = truncated %t, %d edges; want truncated with 1", cut.Truncated, len(cut.Result.Edges))
	}
}

func TestModuleDependenciesDisclosesClampedDepth(t *testing.T) {
	store, identity := scanModuleBoundsFixture(t)

	clamped := moduleDeps(t, store, identity, "pair.ts", false, 3, edgeLimits(2, 50))
	if !clamped.Truncated || !hasDepthNote(clamped.Limitations, 3, 2) {
		t.Fatalf("depth 3 over MaxDepth 2 = truncated %t, limitations %q; want depth note and truncated", clamped.Truncated, clamped.Limitations)
	}
	// A module the graph does not contain is refused, not answered empty.
	if _, err := store.FindModuleDependencies(context.Background(), identity, "missing.ts", false, 99, edgeLimits(2, 50)); err == nil {
		t.Fatal("unknown module returned a result instead of an error")
	}
	if len(clamped.Result.Edges) != 3 {
		t.Fatalf("clamped walk should still return the depth-2 reach, got %d edges", len(clamped.Result.Edges))
	}

	atLimit := moduleDeps(t, store, identity, "pair.ts", false, 2, edgeLimits(2, 50))
	if atLimit.Truncated || hasDepthNote(atLimit.Limitations, 2, 2) || len(atLimit.Result.Edges) != 3 {
		t.Fatalf("depth equal to MaxDepth = truncated %t, limitations %q, %d edges; want complete without note", atLimit.Truncated, atLimit.Limitations, len(atLimit.Result.Edges))
	}
	for _, limitation := range atLimit.Limitations {
		if strings.Contains(limitation, "exceeds the traversal limit") {
			t.Fatalf("depth at the limit carried a depth note: %q", atLimit.Limitations)
		}
	}

	// Depth 0 and negative depth still mean depth 1: only pair.ts's own edges.
	for _, depth := range []int{0, -1} {
		result := moduleDeps(t, store, identity, "pair.ts", false, depth, edgeLimits(2, 50))
		if result.Truncated || len(result.Result.Edges) != 2 || len(result.Limitations) != len(atLimit.Limitations) {
			t.Fatalf("depth %d = truncated %t, %d edges, limitations %q; want depth-1 result", depth, result.Truncated, len(result.Result.Edges), result.Limitations)
		}
	}

	// Limits without a maximum depth fall back to the defaults (MaxDepth 2,
	// MaxEdges 400), so the caller's MaxEdges=1 is not applied.
	defaulted := moduleDeps(t, store, identity, "pair.ts", false, 5, GraphLimits{MaxEdges: 1})
	if !hasDepthNote(defaulted.Limitations, 5, DefaultGraphLimits().MaxDepth) || len(defaulted.Result.Edges) != 3 {
		t.Fatalf("malformed limits = limitations %q, %d edges; want default limits with depth note", defaulted.Limitations, len(defaulted.Result.Edges))
	}
}
