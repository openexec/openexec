package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A React component is used by being rendered, never by being called. The
// extractor only looked for `Name(`, so every component reached through JSX had
// no inbound edge and the dead-code review listed the entire component tree —
// including the application's own entry component.
func TestJSXRenderedComponentsAreNotDeadCode(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/components/Layout.tsx",
		"export function Layout() { return <div>shell</div> }\n")
	writeTestFile(t, root, "src/components/Unused.tsx",
		"export function Unused() { return <div>nobody renders me</div> }\n")
	writeTestFile(t, root, "src/App.tsx",
		"import { Layout } from \"./components/Layout\"\nexport function App() { return <Layout /> }\n")
	writeTestFile(t, root, "src/main.tsx",
		"import { App } from \"./App\"\nexport function boot() { return <App /> }\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ScanRepository(ctx, root); err != nil {
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
	dead := map[string]bool{}
	for _, candidate := range projection.DeadCodeCandidates {
		dead[trailingName(candidate.DisplayName)] = true
	}
	for _, rendered := range []string{"App", "Layout"} {
		if dead[rendered] {
			t.Errorf("%s is rendered as JSX but was reported as dead code", rendered)
		}
	}
	// The check must still find real dead code, or it has been defanged rather
	// than fixed: a component nobody renders is exactly what it exists to find.
	if !dead["Unused"] {
		t.Errorf("a component nobody renders was not flagged: %v", dead)
	}
}

// The Go extractor had the same hole as the TypeScript one: only calls were
// recorded, so a type used in a field and a constant read in an expression had
// no inbound edge and were offered for deletion.
func TestGoTypesAndConstantsAreNotDeadCode(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "model.go", "package sample\n\ntype Config struct{ Name string }\n\nconst Retries = 3\n\ntype Unused struct{}\n")
	writeTestFile(t, root, "use.go", "package sample\n\nfunc Run(config Config) int { return Retries }\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ScanRepository(ctx, root); err != nil {
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
	dead := map[string]bool{}
	for _, candidate := range projection.DeadCodeCandidates {
		dead[trailingName(candidate.DisplayName)] = true
	}
	for _, used := range []string{"Config", "Retries"} {
		if dead[used] {
			t.Errorf("%s is used but was reported as dead code", used)
		}
	}
	if !dead["Unused"] {
		t.Errorf("a type nobody mentions was not flagged: %v", dead)
	}
}

// A name shared by several symbols used to be dropped, which made every one of
// them look unreferenced. Ambiguity means "which one is unknown", not "none of
// them" — and for deletion review the safe reading is that all are alive.
func TestAmbiguousReferencesKeepEveryCandidateAlive(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "alpha/store.go", "package alpha\n\ntype Store struct{}\n")
	writeTestFile(t, root, "beta/store.go", "package beta\n\ntype Store struct{}\n")
	writeTestFile(t, root, "use.go", "package sample\n\nfunc Use(s Store) {}\n")
	writeTestFile(t, root, "lonely/lonely.go", "package lonely\n\ntype NeverMentioned struct{}\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ScanRepository(ctx, root); err != nil {
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
	dead := map[string]bool{}
	for _, candidate := range projection.DeadCodeCandidates {
		dead[trailingName(candidate.DisplayName)] = true
	}
	if dead["Store"] {
		t.Error("an ambiguously referenced symbol was offered for deletion")
	}
	if !dead["NeverMentioned"] {
		t.Errorf("a symbol nobody mentions was not flagged: %v", dead)
	}
	// Ranking must stay precise: a guess about which Store was meant would
	// promote it up the "touch with care" list on evidence that does not exist.
	for _, hotspot := range projection.Hotspots {
		if trailingName(hotspot.DisplayName) == "Store" {
			t.Error("an ambiguous use was counted as a hotspot ranking signal")
		}
	}
}

// The panel's most prominent sentence is "X is the highest-ranked hotspot".
// Counting bare mentions made that sentence report whichever common word
// appeared most — "post", "count", "title" — and counting one-letter names
// made it report `t`. Both are true numbers and useless conclusions.
func TestHotspotRankingIgnoresBareMentionsAndTrivialNames(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "core.go", "package sample\n\nfunc Important() {}\n\nfunc t() {}\n\ntype count struct{}\n")
	// count is only ever mentioned as a field name and a local; Important is
	// genuinely called; t is called a great many times but says nothing.
	body := "package sample\n\nfunc Driver() {\n\tImportant()\n"
	for i := 0; i < 30; i++ {
		body += "\tt()\n"
	}
	for i := 0; i < 40; i++ {
		body += "\tvar count int\n\t_ = count\n"
	}
	body += "}\n"
	writeTestFile(t, root, "driver.go", body)
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ScanRepository(ctx, root); err != nil {
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
	for _, hotspot := range projection.Hotspots {
		if trailingName(hotspot.DisplayName) == "t" {
			t.Error("a one-letter name headlined the hotspot ranking")
		}
		if trailingName(hotspot.DisplayName) == "count" {
			t.Error("a symbol reached the ranking on bare mentions alone")
		}
	}
	// The check must still rank something, or it has been emptied rather than
	// cleaned: a genuinely called symbol has to survive.
	var ranked bool
	for _, hotspot := range projection.Hotspots {
		if trailingName(hotspot.DisplayName) == "Important" {
			ranked = true
		}
	}
	if !ranked {
		t.Errorf("a called symbol did not rank at all: %#v", projection.Hotspots)
	}
}

// trailingName takes the last segment of a qualified identity. Insights name a
// symbol by what distinguishes it (internal/logging/logger.Logger.Error), and
// these fixtures care only which symbol was meant.
func trailingName(identity string) string {
	if index := strings.LastIndex(identity, "."); index >= 0 {
		return identity[index+1:]
	}
	return identity
}

// A repository laid out as code/backend/app/... collapsed to a single area at
// the fixed two-segment depth, so every import pointed at itself and the
// architecture came out empty — 732 module imports, no flows, and a flowchart
// export that produced nothing.
func TestModuleSketchFindsFlowsInADeepLayout(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "code/backend/app/services/order.py", "from code.backend.app.repositories.order import fetch\n\ndef place():\n    return fetch()\n")
	writeTestFile(t, root, "code/backend/app/repositories/order.py", "def fetch():\n    return 1\n")
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.ScanRepository(ctx, root); err != nil {
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
	if len(projection.ModuleSketch) == 0 {
		t.Fatalf("a repository whose areas differ below the second segment reported no architecture")
	}
	// The areas must still be distinguishable, not the whole repository as one.
	for _, flow := range projection.ModuleSketch {
		if flow.From == flow.To {
			t.Errorf("an area was reported as depending on itself: %#v", flow)
		}
	}
}
