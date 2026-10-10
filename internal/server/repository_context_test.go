package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/pkg/db/state"
)

func TestRepositoryContextAPIRefreshesAndSurvivesRestart(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package sample\nfunc Main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(root, ".openexec", "openexec.db")
	stateStore, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{StateStore: stateStore, ProjectsDir: root}
	scanRequest := httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/scan", nil)
	identityStore, err := knowledge.NewStoreWithDB(stateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := identityStore.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	scanRequest.Header.Set("X-OpenExec-Checkout-ID", identity.CheckoutID)
	scanResponse := httptest.NewRecorder()
	server.handleRepositoryGraphScan(scanResponse, scanRequest)
	if scanResponse.Code != http.StatusOK {
		t.Fatalf("scan failed: %d %s", scanResponse.Code, scanResponse.Body.String())
	}
	first := requestRepositoryContext(t, server, "/api/v1/repository-context?symbols=Main&task_id=task&run_id=run")
	if first.SchemaVersion != 1 || first.Freshness != knowledge.FreshnessCurrent || len(first.ResolvedSymbols) != 1 {
		t.Fatalf("unexpected first projection: %#v", first)
	}
	if err := stateStore.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	server.StateStore = reopened
	second := requestRepositoryContext(t, server, "/api/v1/repository-context?symbols=Main&task_id=task&run_id=run")
	if second.OpenExecReference.ResourceVersion != first.OpenExecReference.ResourceVersion || second.RepositoryID != first.RepositoryID || second.GraphVersion != first.GraphVersion {
		t.Fatalf("projection did not round-trip restart: first=%#v second=%#v", first, second)
	}
	if _, err := os.Stat(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
}

// The repository-context route must surface the graph's stale refusal the way
// the graph routes do — 409 with the refused generation — never as a 500 that
// a consumer cannot tell apart from a broken server.
func TestRepositoryContextAPIAnswersStaleRefusalWith409(t *testing.T) {
	cases := map[string]func(t *testing.T, fixture graphQueryFixture){
		"refresh_disabled": func(t *testing.T, fixture graphQueryFixture) {
			store, err := knowledge.NewStoreWithDB(fixture.server.StateStore.GetDB())
			if err != nil {
				t.Fatal(err)
			}
			store.SetRefreshOnRead(false)
			fixture.server.KnowledgeStore = store
			if err := os.WriteFile(filepath.Join(fixture.server.ProjectsDir, "main.go"), []byte("package sample\n\n// edited\nfunc Target() {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"refresh_unsafe": func(t *testing.T, fixture graphQueryFixture) {
			outside := filepath.Join(t.TempDir(), "outside.go")
			if err := os.WriteFile(outside, []byte("package outside\nfunc Secret() {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(fixture.server.ProjectsDir, "escape.go")); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		},
	}
	for name, drift := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newGraphQueryFixture(t)
			before := graphRequest(t, fixture, "/api/v1/repository-context?symbols=Target", fixture.identity.CheckoutID)
			var current knowledge.RepositoryContextProjection
			if before.Code != http.StatusOK || json.Unmarshal(before.Body.Bytes(), &current) != nil {
				t.Fatalf("fresh context status = %d, body=%s", before.Code, before.Body.String())
			}
			drift(t, fixture)
			response := graphRequest(t, fixture, "/api/v1/repository-context?symbols=Target", fixture.identity.CheckoutID)
			if response.Code != http.StatusConflict {
				t.Fatalf("stale context status = %d, body=%s", response.Code, response.Body.String())
			}
			var refusal struct {
				Error      string                    `json:"error"`
				Freshness  string                    `json:"freshness"`
				Generation knowledge.RepositoryState `json:"generation"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil {
				t.Fatal(err)
			}
			if refusal.Freshness != string(knowledge.FreshnessStale) || refusal.Generation.GraphVersion != current.GraphVersion || refusal.Error == "" {
				t.Fatalf("refusal lost its generation: %#v", refusal)
			}
		})
	}
}

func requestRepositoryContext(t *testing.T, server *Server, target string) knowledge.RepositoryContextProjection {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	response := httptest.NewRecorder()
	server.handleRepositoryContext(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("repository context failed: %d %s", response.Code, response.Body.String())
	}
	var projection knowledge.RepositoryContextProjection
	if err := json.Unmarshal(response.Body.Bytes(), &projection); err != nil {
		t.Fatal(err)
	}
	return projection
}
