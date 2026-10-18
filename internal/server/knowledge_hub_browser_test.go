//go:build d5browser

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
)

// Run through verify-d5.py: real coordinator, route registration, scoped auth,
// SQLite reads and compiled UI. No browser response interception or providers.
func TestKnowledgeHubBrowser(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir()) // Legacy knowledge handlers open the current directory.
	store, err := knowledge.NewStore(".")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetSymbol(&knowledge.SymbolRecord{Name: "LiveKnowledgeExample", Kind: "function", FilePath: "example.go", StartLine: 1, EndLine: 3}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEnvironment(&knowledge.EnvironmentRecord{Env: "dev", RuntimeType: "local", Topology: "localhost"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	// Close before serving: the production handlers must reopen persisted data.
	fixture := NewTestServer(t)
	defer fixture.Server.StateStore.Close()
	if fixture.Server.Coordinator == nil {
		t.Fatal("coordinator missing")
	}
	assets := http.FileServer(http.Dir(filepath.Join(root, "ui/dist")))
	live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			fixture.Server.Mux.ServeHTTP(w, r)
			return
		}
		assets.ServeHTTP(w, r)
	}))
	defer live.Close()
	cmd := exec.Command("node", "node_modules/@playwright/test/cli.js", "test", "--config=playwright.d5-live.config.ts")
	cmd.Dir = filepath.Join(root, "ui")
	cmd.Env = append(os.Environ(), "D5_BASE_URL="+live.URL, "D5_TOKEN="+repositoryGraphTestToken, "D5_CHECKOUT="+fixture.checkoutID)
	output, err := cmd.CombinedOutput()
	t.Logf("coordinator=true; isolated SQLite; scoped bearer/checkout auth; server=%s\n%s", live.URL, output)
	if err != nil {
		t.Fatal(err)
	}
}
