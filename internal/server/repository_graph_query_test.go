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

type graphQueryFixture struct {
	server   *Server
	identity knowledge.RepositoryIdentity
}

func newGraphQueryFixture(t *testing.T) graphQueryFixture {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package sample\n\nfunc Target() {}\nfunc Caller() { Target() }\nfunc Run() {}\n"
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.go"), []byte("package sample\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateStore, err := state.NewStore(filepath.Join(root, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stateStore.Close() })
	graphStore, err := knowledge.NewStoreWithDB(stateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graphStore.ScanRepository(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	identity, err := graphStore.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{StateStore: stateStore, ProjectsDir: root, Mux: http.NewServeMux()}
	s.registerRepositoryGraphQueryRoutes()
	return graphQueryFixture{server: s, identity: identity}
}

func graphRequest(t *testing.T, fixture graphQueryFixture, target, checkoutID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if checkoutID != "" {
		request.Header.Set("X-OpenExec-Checkout-ID", checkoutID)
	}
	response := httptest.NewRecorder()
	fixture.server.Mux.ServeHTTP(response, request)
	return response
}

func TestRepositoryGraphQueriesRequireCheckoutAuthorityAndPreserveAmbiguity(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	if response := graphRequest(t, fixture, "/api/v1/repository-graph/symbols?q=Run", ""); response.Code != http.StatusForbidden {
		t.Fatalf("missing checkout status = %d, body=%s", response.Code, response.Body.String())
	}
	response := graphRequest(t, fixture, "/api/v1/repository-graph/symbols?q=Run&page=1&page_size=1", fixture.identity.CheckoutID)
	if response.Code != http.StatusOK {
		t.Fatalf("search status = %d, body=%s", response.Code, response.Body.String())
	}
	var result knowledge.QueryEnvelope[knowledge.SymbolSearchResult]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Generation.Freshness != knowledge.FreshnessCurrent || result.Pagination == nil || result.Pagination.Total != 2 || len(result.Result.Candidates) != 1 || !result.Truncated {
		t.Fatalf("ambiguous paginated result = %#v", result)
	}
	// Search returns candidates only. An explicit opaque identity is required
	// before detail or source can be read.
	id := result.Result.Candidates[0].Symbol.ID
	detail := graphRequest(t, fixture, "/api/v1/repository-graph/symbols/"+id, fixture.identity.CheckoutID)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail status = %d, body=%s", detail.Code, detail.Body.String())
	}
	invalidCalls := graphRequest(t, fixture, "/api/v1/repository-graph/symbols/"+id+"/calls", fixture.identity.CheckoutID)
	if invalidCalls.Code != http.StatusBadRequest {
		t.Fatalf("direction-less calls status = %d", invalidCalls.Code)
	}
}

func TestRepositoryGraphSourceRefreshesBeforeRead(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	search := graphRequest(t, fixture, "/api/v1/repository-graph/symbols?q=Target", fixture.identity.CheckoutID)
	var found knowledge.QueryEnvelope[knowledge.SymbolSearchResult]
	if err := json.Unmarshal(search.Body.Bytes(), &found); err != nil {
		t.Fatal(err)
	}
	if len(found.Result.Candidates) != 1 {
		t.Fatalf("target candidates = %#v", found.Result.Candidates)
	}
	id := found.Result.Candidates[0].Symbol.ID
	updated := "package sample\n\n\n// moved in the dirty worktree\nfunc Target() { println(\"current\") }\nfunc Caller() { Target() }\nfunc Run() {}\n"
	if err := os.WriteFile(filepath.Join(fixture.server.ProjectsDir, "main.go"), []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	response := graphRequest(t, fixture, "/api/v1/repository-graph/symbols/"+id+"/source", fixture.identity.CheckoutID)
	if response.Code != http.StatusOK {
		t.Fatalf("source status = %d, body=%s", response.Code, response.Body.String())
	}
	var result knowledge.QueryEnvelope[*knowledge.SymbolSource]
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Generation.Freshness != knowledge.FreshnessCurrent || result.Result == nil || result.Result.Occurrence.StartLine != 5 || result.Result.Source.Content != "func Target() { println(\"current\") }" {
		t.Fatalf("source did not re-resolve current worktree: %#v", result)
	}
}

func TestRepositoryGraphRefusesStalePointersWhenRefreshIsUnsafe(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\nfunc Secret() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(fixture.server.ProjectsDir, "escape.go")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	response := graphRequest(t, fixture, "/api/v1/repository-graph/symbols?q=Target", fixture.identity.CheckoutID)
	if response.Code != http.StatusConflict {
		t.Fatalf("unsafe refresh status = %d, body=%s", response.Code, response.Body.String())
	}
	var refusal struct {
		Freshness string `json:"freshness"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &refusal); err != nil {
		t.Fatal(err)
	}
	if refusal.Freshness != string(knowledge.FreshnessStale) {
		t.Fatalf("freshness = %q, want stale", refusal.Freshness)
	}
}
