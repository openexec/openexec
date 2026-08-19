package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const repositoryEvidenceTestToken = "openexec-evidence-token-longer-than-thirty-two"

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
	for _, token := range []string{"", "wrong-token"} {
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

func TestRepositoryEvidenceTokenAlsoProtectsLegacyGraphReads(t *testing.T) {
	fixture := newGraphQueryFixtureWithToken(t, repositoryEvidenceTestToken)
	for _, target := range []string{
		"/api/v1/repository-graph/symbols?q=Target",
		"/api/v1/repository-context?symbols=Target",
	} {
		response := graphRequest(t, fixture, target, fixture.identity.CheckoutID)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("legacy read %s without bearer = %d, body=%s", target, response.Code, response.Body.String())
		}
	}
	response := graphRequestWithToken(t, fixture, "/api/v1/repository-graph/symbols?q=Target", fixture.identity.CheckoutID, repositoryEvidenceTestToken)
	if response.Code != http.StatusOK {
		t.Fatalf("legacy read with bearer = %d, body=%s", response.Code, response.Body.String())
	}
}
