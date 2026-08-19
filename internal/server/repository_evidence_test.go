package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
