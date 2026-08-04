package knowledge

import (
	"context"
	"os"
	"path/filepath"
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
		dead[candidate.DisplayName] = true
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
		dead[candidate.DisplayName] = true
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
