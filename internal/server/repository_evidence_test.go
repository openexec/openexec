package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/pkg/db/state"
)

const repositoryEvidenceTestToken = "openexec-evidence-token-longer-than-thirty-two"
const repositoryGraphTestToken = "openexec-graph-token-distinct-from-evidence"

func evidenceRequest(t *testing.T, fixture graphQueryFixture, target, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.server.Mux.ServeHTTP(response, request)
	return response
}

func TestRepositoryEvidenceProfileRequiresIndependentTokenAndPreservesProvenance(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	fixture.server.repositoryEvidenceToken = repositoryEvidenceTestToken
	fixture.server.registerRepositoryEvidenceRoutes()
	for _, token := range []string{"", "wrong-token", repositoryGraphTestToken} {
		response := evidenceRequest(t, fixture, "/api/v1/external-evidence/symbols?q=Target", token)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("token %q status = %d, body=%s", token, response.Code, response.Body.String())
		}
	}
	response := evidenceRequest(t, fixture, "/api/v1/external-evidence/symbols?q=Target", repositoryEvidenceTestToken)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"freshness":"current"`) ||
		!strings.Contains(response.Body.String(), `"graph_version"`) {
		t.Fatalf("evidence response = %d %s", response.Code, response.Body.String())
	}
}

type evidenceBody struct {
	Reason     string                    `json:"reason"`
	Provenance knowledge.RepositoryState `json:"provenance"`
	Generation knowledge.RepositoryState `json:"generation"`
	Result     struct {
		Candidates []knowledge.SymbolCandidate `json:"candidates"`
		knowledge.SymbolSource
	} `json:"result"`
}

func decodeEvidence(t *testing.T, response *httptest.ResponseRecorder) evidenceBody {
	t.Helper()
	var body evidenceBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return body
}

func evidenceFixture(t *testing.T) graphQueryFixture {
	t.Helper()
	fixture := newGraphQueryFixture(t)
	fixture.server.repositoryEvidenceToken = repositoryEvidenceTestToken
	fixture.server.registerRepositoryEvidenceRoutes()
	return fixture
}

func evidenceTargetID(t *testing.T, fixture graphQueryFixture) (string, evidenceBody) {
	t.Helper()
	response := evidenceRequest(t, fixture, "/api/v1/external-evidence/symbols?q=Target", repositoryEvidenceTestToken)
	body := decodeEvidence(t, response)
	if response.Code != http.StatusOK || len(body.Result.Candidates) != 1 {
		t.Fatalf("target search = %d %s", response.Code, response.Body.String())
	}
	return body.Result.Candidates[0].Symbol.ID, body
}

// Phase 1B exit of the Agent Console external advisory plan: mutate the
// repository after graph publication and prove the external read refreshes,
// citing the answering generation in response-body provenance.
func TestRepositoryEvidenceReadAfterEditRefreshesAndCitesAnsweringGeneration(t *testing.T) {
	fixture := evidenceFixture(t)
	id, before := evidenceTargetID(t, fixture)
	if before.Provenance.GraphVersion == "" || before.Provenance.Freshness != knowledge.FreshnessCurrent ||
		before.Provenance.CheckoutID != fixture.identity.CheckoutID || before.Provenance != before.Generation {
		t.Fatalf("search provenance = %#v, generation = %#v", before.Provenance, before.Generation)
	}
	updated := "package sample\n\n\n// edited after publication\nfunc Target() { println(\"edited\") }\nfunc Caller() { Target() }\nfunc Run() {}\n"
	if err := os.WriteFile(filepath.Join(fixture.server.ProjectsDir, "main.go"), []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	response := evidenceRequest(t, fixture, "/api/v1/external-evidence/symbols/"+id+"/source", repositoryEvidenceTestToken)
	after := decodeEvidence(t, response)
	if response.Code != http.StatusOK || after.Provenance.Freshness != knowledge.FreshnessCurrent ||
		after.Provenance.GraphVersion == "" || after.Provenance.GraphVersion == before.Provenance.GraphVersion ||
		after.Result.Source.Content != "func Target() { println(\"edited\") }" {
		t.Fatalf("edited source read = %d %s", response.Code, response.Body.String())
	}
}

func TestRepositoryEvidenceRefusesStaleGraphWhenRefreshIsDisabled(t *testing.T) {
	fixture := evidenceFixture(t)
	store, err := knowledge.NewStoreWithDB(fixture.server.StateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	store.SetRefreshOnRead(false)
	fixture.server.KnowledgeStore = store
	id, before := evidenceTargetID(t, fixture)
	if err := os.WriteFile(filepath.Join(fixture.server.ProjectsDir, "main.go"), []byte("package sample\n\nfunc Target() { println(\"edited\") }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{
		"/api/v1/external-evidence/symbols?q=Target",
		"/api/v1/external-evidence/symbols/" + id + "/source",
	} {
		response := evidenceRequest(t, fixture, target, repositoryEvidenceTestToken)
		body := decodeEvidence(t, response)
		if response.Code != http.StatusConflict || body.Reason != "graph_stale" ||
			body.Provenance.Freshness != knowledge.FreshnessStale ||
			body.Provenance.GraphVersion != before.Provenance.GraphVersion ||
			len(body.Result.Candidates) != 0 || body.Result.Source.Content != "" {
			t.Fatalf("stale %s = %d %s", target, response.Code, response.Body.String())
		}
	}
}

func TestRepositoryEvidenceDistinguishesMissingGraphFromMissingSymbol(t *testing.T) {
	fixture := evidenceFixture(t)
	unknown := evidenceRequest(t, fixture, "/api/v1/external-evidence/symbols/no-such-symbol", repositoryEvidenceTestToken)
	if body := decodeEvidence(t, unknown); unknown.Code != http.StatusNotFound || body.Reason != "not_found" {
		t.Fatalf("unknown symbol = %d %s", unknown.Code, unknown.Body.String())
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package sample\nfunc Target() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stateStore, err := state.NewStore(filepath.Join(t.TempDir(), "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stateStore.Close() })
	graphStore, err := knowledge.NewStoreWithDB(stateStore.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := graphStore.EnsureRepositoryIdentity(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	unscanned := graphQueryFixture{server: &Server{
		StateStore: stateStore, KnowledgeStore: graphStore, ProjectsDir: root, Mux: http.NewServeMux(),
		repositoryEvidenceToken: repositoryEvidenceTestToken,
	}, identity: identity}
	unscanned.server.registerRepositoryEvidenceRoutes()
	for _, target := range []string{
		"/api/v1/external-evidence/symbols?q=Target",
		"/api/v1/external-evidence/symbols/any/source",
	} {
		response := evidenceRequest(t, unscanned, target, repositoryEvidenceTestToken)
		body := decodeEvidence(t, response)
		if response.Code != http.StatusNotFound || body.Reason != "graph_missing" ||
			body.Provenance.Freshness != knowledge.FreshnessMissing || body.Provenance.GraphVersion != "" ||
			body.Provenance.CheckoutID != identity.CheckoutID {
			t.Fatalf("missing graph %s = %d %s", target, response.Code, response.Body.String())
		}
	}
}

func TestRepositoryEvidenceProfileHasNoValidationOrMutationRoute(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	fixture.server.repositoryEvidenceToken = repositoryEvidenceTestToken
	fixture.server.registerRepositoryEvidenceRoutes()
	for _, target := range []string{
		"/api/v1/external-evidence/impact/changed",
		"/api/v1/external-evidence/validation-plans/propose",
		"/api/v1/external-evidence/validation-runs",
	} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
		request.Header.Set("Authorization", "Bearer "+repositoryEvidenceTestToken)
		request.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
		response := httptest.NewRecorder()
		fixture.server.Mux.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("external evidence mutation %s = %d, want 404", target, response.Code)
		}
	}
}

func TestRepositoryGraphReadsRequireTheIndependentGraphToken(t *testing.T) {
	fixture := newGraphQueryFixtureWithToken(t, repositoryGraphTestToken)
	for _, target := range []string{
		"/api/v1/repository-graph/symbols?q=Target",
		"/api/v1/repository-context?symbols=Target",
	} {
		response := graphRequestWithToken(t, fixture, target, fixture.identity.CheckoutID, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("legacy read %s without bearer = %d, body=%s", target, response.Code, response.Body.String())
		}
	}
	evidenceTokenResponse := graphRequestWithToken(t, fixture, "/api/v1/repository-graph/symbols?q=Target", fixture.identity.CheckoutID, repositoryEvidenceTestToken)
	if evidenceTokenResponse.Code != http.StatusUnauthorized {
		t.Fatalf("evidence token authorized graph read = %d, body=%s", evidenceTokenResponse.Code, evidenceTokenResponse.Body.String())
	}
	response := graphRequestWithToken(t, fixture, "/api/v1/repository-graph/symbols?q=Target", fixture.identity.CheckoutID, repositoryGraphTestToken)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy read with bearer = %d, body=%s", response.Code, response.Body.String())
	}
}

func TestRepositoryGraphRoutesFailClosedWithoutConfiguredCredential(t *testing.T) {
	fixture := newGraphQueryFixtureWithToken(t, "")
	for _, request := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/api/v1/repository-context", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/repository-graph/symbols?q=Target", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/impact/changed", strings.NewReader(`{"files":["main.go"]}`)),
		httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/scan", nil),
		httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/validation-plans/propose", strings.NewReader(`{"task_id":"task","files":["main.go"]}`)),
	} {
		request.Header.Set("Authorization", "Bearer ")
		request.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
		response := httptest.NewRecorder()
		fixture.server.Mux.ServeHTTP(response, request)
		if response.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s = %d, want 503; body=%s", request.Method, request.URL.Path, response.Code, response.Body.String())
		}
	}
}

func TestRepositoryGraphMutationsRequireBearerAndCheckoutAuthority(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	for _, target := range []string{
		"/api/v1/repository-graph/scan",
		"/api/v1/repository-graph/impact/changed",
		"/api/v1/repository-graph/validation-plans/propose",
	} {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(`{}`))
		request.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
		response := httptest.NewRecorder()
		fixture.server.Mux.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("mutation %s without bearer = %d, want 401; body=%s", target, response.Code, response.Body.String())
		}
	}

	scan := httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/scan", nil)
	scan.Header.Set("Authorization", "Bearer "+fixture.token)
	scanResponse := httptest.NewRecorder()
	fixture.server.Mux.ServeHTTP(scanResponse, scan)
	if scanResponse.Code != http.StatusForbidden {
		t.Fatalf("scan without checkout authority = %d, want 403; body=%s", scanResponse.Code, scanResponse.Body.String())
	}

	authorizedScan := httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/scan", nil)
	authorizedScan.Header.Set("Authorization", "Bearer "+fixture.token)
	authorizedScan.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
	authorizedScanResponse := httptest.NewRecorder()
	fixture.server.Mux.ServeHTTP(authorizedScanResponse, authorizedScan)
	if authorizedScanResponse.Code != http.StatusOK {
		t.Fatalf("authorized scan = %d, want 200; body=%s", authorizedScanResponse.Code, authorizedScanResponse.Body.String())
	}
}

func TestServerListenAddressDefaultsToLoopback(t *testing.T) {
	if got := serverListenAddress("", 8765); got != "127.0.0.1:8765" {
		t.Fatalf("default listen address = %q", got)
	}
	if got := serverListenAddress("0.0.0.0", 8765); got != "0.0.0.0:8765" {
		t.Fatalf("explicit listen address = %q", got)
	}
}

func TestServerRejectsSharedEvidenceAndGraphCredential(t *testing.T) {
	_, err := New(Config{
		ProjectsDir:             t.TempDir(),
		DataDir:                 t.TempDir(),
		SkipPreflight:           true,
		RepositoryEvidenceToken: repositoryEvidenceTestToken,
		RepositoryGraphToken:    repositoryEvidenceTestToken,
	})
	if err == nil || !strings.Contains(err.Error(), "OPENEXEC_REPOSITORY_EVIDENCE_TOKEN") ||
		!strings.Contains(err.Error(), "OPENEXEC_REPOSITORY_GRAPH_TOKEN") {
		t.Fatalf("New equal-token error = %v", err)
	}
}

func TestDCPAndKnowledgeRoutesRequireGraphCredentialAndCheckout(t *testing.T) {
	server := NewTestServer(t).Server
	requests := []*http.Request{
		httptest.NewRequest(http.MethodPost, "/api/v1/dcp/query", strings.NewReader(`{"query":"help"}`)),
		httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/symbols", nil),
		httptest.NewRequest(http.MethodGet, "/api/v1/knowledge/envs", nil),
	}
	for _, request := range requests {
		response := httptest.NewRecorder()
		server.Mux.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s without bearer = %d, want 401; body=%s", request.URL.Path, response.Code, response.Body.String())
		}

		request = request.Clone(request.Context())
		request.Header.Set("Authorization", "Bearer wrong-token")
		response = httptest.NewRecorder()
		server.Mux.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s with wrong bearer = %d, want 401; body=%s", request.URL.Path, response.Code, response.Body.String())
		}

		request = request.Clone(request.Context())
		request.Header.Set("Authorization", "Bearer "+repositoryGraphTestToken)
		response = httptest.NewRecorder()
		server.Mux.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s without checkout = %d, want 403; body=%s", request.URL.Path, response.Code, response.Body.String())
		}
	}
}
