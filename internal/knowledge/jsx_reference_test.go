package knowledge

import (
	"context"
	"os"
	"path/filepath"
	"sort"
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

// The architecture must survive a layout where different relationships
// separate at different depths. One global depth cannot: stopping at the first
// depth that produced any edge reported tests -> app and left the
// services -> repositories relationship — the one that carries the design —
// collapsed into a single area.
func TestModuleSketchNamesEachRelationshipWhereItDiverges(t *testing.T) {
	root := t.TempDir()
	// Shallow: src/components -> src/lib, separating at segment 2.
	writeTestFile(t, root, "src/components/Card.ts", "import { fmt } from '../lib/fmt'\nexport const Card = () => fmt()\n")
	writeTestFile(t, root, "src/lib/fmt.ts", "export function fmt() { return 1 }\n")
	// Deep: app/services -> app/repositories, separating at segment 4.
	writeTestFile(t, root, "code/backend/app/services/order.py", "from code.backend.app.repositories.order import fetch\n\ndef place():\n    return fetch()\n")
	writeTestFile(t, root, "code/backend/app/repositories/order.py", "def fetch():\n    return 1\n")
	// Mixed, in the same repository: tests -> app separates at segment 3, and
	// must not decide the granularity of the pair above.
	writeTestFile(t, root, "code/backend/tests/test_order.py", "from code.backend.app.services.order import place\n\ndef test_place():\n    assert place()\n")
	// Deeper than four segments, to prove nothing is capped.
	writeTestFile(t, root, "code/backend/app/domain/pricing/rules.py", "from code.backend.app.domain.tax.vat import rate\n\ndef price():\n    return rate()\n")
	writeTestFile(t, root, "code/backend/app/domain/tax/vat.py", "def rate():\n    return 24\n")

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
	flows := map[string]bool{}
	for _, flow := range projection.ModuleSketch {
		flows[flow.From+" -> "+flow.To] = true
		if flow.From == flow.To {
			t.Errorf("an area was reported as depending on itself: %#v", flow)
		}
	}
	for _, wanted := range []string{
		"src/components -> src/lib",
		"code/backend/app/services -> code/backend/app/repositories",
		"code/backend/tests -> code/backend/app",
		"code/backend/app/domain/pricing -> code/backend/app/domain/tax",
	} {
		if !flows[wanted] {
			t.Errorf("missing relationship %q; got %v", wanted, keysOf(flows))
		}
	}
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
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

// A migration's upgrade and downgrade have no caller by construction: the
// migration runner finds them by convention. Offering them for deletion is a
// wrong answer, not a cautious one — Siivous listed 856 of them.
func TestRunnerInvokedEntryPointsAreNotCleanupCandidates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "alembic/versions/0cae_add_users.py",
		"def upgrade():\n    pass\n\ndef downgrade():\n    pass\n")
	writeTestFile(t, root, "migrations/0002_add_index.py", "def upgrade():\n    pass\n")
	writeTestFile(t, root, "tests/conftest.py", "def pytest_configure():\n    pass\n")
	writeTestFile(t, root, "app/orphan.py", "def really_unused():\n    pass\n")
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
	for _, candidate := range projection.DeadCodeCandidates {
		if strings.Contains(candidate.SafeLocation, "alembic") ||
			strings.Contains(candidate.SafeLocation, "migrations/") ||
			strings.Contains(candidate.SafeLocation, "conftest") {
			t.Errorf("a runner-invoked entry point was offered for deletion: %#v", candidate)
		}
	}
	// And the check must still find code nothing invokes, or it has been
	// emptied rather than corrected.
	var found bool
	for _, candidate := range projection.DeadCodeCandidates {
		if trailingName(candidate.DisplayName) == "really_unused" {
			found = true
		}
	}
	if !found {
		t.Errorf("genuinely unused code was not reported: %v", projection.DeadCodeCandidates)
	}
	// The listing and its total come from one exclusion, and the test has to
	// hold both to that: a divergence would otherwise report "15 of 856" with
	// fifteen correct rows and a count that still includes what was excluded.
	scope := projection.Selections["dead_code_candidates"]
	if scope.Total != len(projection.DeadCodeCandidates) {
		t.Errorf("count %d disagrees with the %d rows it summarises", scope.Total, len(projection.DeadCodeCandidates))
	}
}

// FastAPI, Celery and pytest call functions the code never calls: something
// holds a reference the source does not show. Which one needs framework
// knowledge this extractor lacks, so anything decorated is treated as used —
// being wrong here costs working code.
func TestDecoratedDeclarationsAreNotCleanupCandidates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "app/api/admin.py",
		"from fastapi import APIRouter\n\nrouter = APIRouter()\n\n\n@router.get(\"/\")\nasync def admin_root():\n    return {}\n")
	// Arguments spanning lines: the decorator is not on the line above.
	writeTestFile(t, root, "app/api/users.py",
		"from fastapi import APIRouter\n\nrouter = APIRouter()\n\n\n@router.get(\n    \"/users\",\n    response_model=dict,\n)\nasync def get_user():\n    return {}\n")
	// A stack whose nearest decorator is bare must not hide the one above it.
	writeTestFile(t, root, "app/api/stacked.py",
		"from functools import cache\nfrom fastapi import APIRouter\n\nrouter = APIRouter()\n\n\n@router.get(\"/stacked\")\n@cache\nasync def stacked_handler():\n    return {}\n")
	// An imported bare decorator registers just as thoroughly as a dotted one.
	writeTestFile(t, root, "tests_support/fixtures.py",
		"from pytest import fixture\n\n\n@fixture\ndef database():\n    return 1\n")
	writeTestFile(t, root, "app/model.py", "def genuinely_unused():\n    return 2\n")

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
	for _, decorated := range []string{"admin_root", "get_user", "stacked_handler", "database"} {
		if dead[decorated] {
			t.Errorf("%s is decorated but was offered for deletion", decorated)
		}
	}
	if !dead["genuinely_unused"] {
		t.Errorf("undecorated unused code was not reported: %v", dead)
	}
	scope := projection.Selections["dead_code_candidates"]
	if scope.Total != len(projection.DeadCodeCandidates) {
		t.Errorf("count %d disagrees with the %d rows it summarises", scope.Total, len(projection.DeadCodeCandidates))
	}
}

// A CSS import becomes an external node named ./App.css, which is not a
// repository-relative module path. Publishing it made the consumer reject the
// whole projection, so one stylesheet cost seven repositories their context —
// and dropping it silently would trade that for quiet data loss.
func TestProjectionOmitsUnpublishableDependenciesAndSaysSo(t *testing.T) {
	for _, value := range []string{"./App.css", "../styles/main.scss", "/abs/path.ts", ".."} {
		if repositoryRelativeModule(value) {
			t.Errorf("repositoryRelativeModule(%q) accepted a path the consumer rejects", value)
		}
	}
	// Bare package names are accepted by both sides; the filter is about
	// unresolved relative paths, not about external packages.
	for _, value := range []string{"react", "@eslint/js"} {
		if !repositoryRelativeModule(value) {
			t.Errorf("repositoryRelativeModule(%q) rejected a target the consumer accepts", value)
		}
	}
	root := t.TempDir()
	writeTestFile(t, root, "src/App.css", "body { color: red }\n")
	writeTestFile(t, root, "src/lib/fmt.ts", "export function fmt() { return 1 }\n")
	writeTestFile(t, root, "src/App.tsx", "import './App.css'\nimport { fmt } from './lib/fmt'\nexport const App = () => fmt()\n")
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
	var keptReal, keptCSS bool
	for _, dependency := range projection.ModuleDependencies {
		if !repositoryRelativeModule(dependency.To) || !repositoryRelativeModule(dependency.From) {
			t.Errorf("published a dependency the consumer rejects: %#v", dependency)
		}
		if strings.HasSuffix(dependency.To, ".css") {
			keptCSS = true
		}
		if strings.Contains(dependency.To, "lib/fmt") {
			keptReal = true
		}
	}
	if keptCSS {
		t.Error("the unresolved stylesheet target was published")
	}
	// An empty dependency list would satisfy the assertion above; it must not.
	if !keptReal {
		t.Errorf("the real dependency was dropped with the unpublishable one: %#v", projection.ModuleDependencies)
	}
	// The count is exact and scoped: it describes this bounded projection, not
	// a repository-wide total, and the wording has to say so.
	var disclosed bool
	for _, limitation := range projection.Limitations {
		if strings.Contains(limitation, "dependency target") && strings.Contains(limitation, "omitted") {
			disclosed = true
			if !strings.Contains(limitation, "bounded projection") {
				t.Errorf("omission disclosed without its scope: %q", limitation)
			}
			if !strings.HasPrefix(limitation, "1 dependency target ") {
				t.Errorf("expected exactly one omission (the stylesheet), got %q", limitation)
			}
		}
	}
	if !disclosed {
		t.Errorf("dropped targets were not disclosed: %v", projection.Limitations)
	}
}
