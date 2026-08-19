package server

import (
	"bytes"
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

type graphQueryFixture struct {
	server   *Server
	identity knowledge.RepositoryIdentity
	token    string
}

func newGraphQueryFixture(t *testing.T) graphQueryFixture {
	return newGraphQueryFixtureWithToken(t, repositoryEvidenceTestToken)
}

func newGraphQueryFixtureWithToken(t *testing.T, evidenceToken string) graphQueryFixture {
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
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tests", "service.go"), []byte("package sample\nfunc TestService() { Target() }\n"), 0o600); err != nil {
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
	s := &Server{
		StateStore: stateStore, ProjectsDir: root, Mux: http.NewServeMux(),
		repositoryEvidenceToken: evidenceToken,
	}
	s.Mux.Handle("POST /api/v1/repository-graph/scan", s.repositoryGraphAuth(http.HandlerFunc(s.handleRepositoryGraphScan)))
	s.Mux.Handle("GET /api/v1/repository-context", s.repositoryGraphAuth(http.HandlerFunc(s.handleRepositoryContext)))
	s.registerRepositoryGraphQueryRoutes()
	return graphQueryFixture{server: s, identity: identity, token: evidenceToken}
}

func graphRequest(t *testing.T, fixture graphQueryFixture, target, checkoutID string) *httptest.ResponseRecorder {
	return graphRequestWithToken(t, fixture, target, checkoutID, fixture.token)
}

func graphRequestWithToken(t *testing.T, fixture graphQueryFixture, target, checkoutID, token string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	if checkoutID != "" {
		request.Header.Set("X-OpenExec-Checkout-ID", checkoutID)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	fixture.server.Mux.ServeHTTP(response, request)
	return response
}

func graphPostRequest(t *testing.T, fixture graphQueryFixture, target, checkoutID string, body any) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	if checkoutID != "" {
		request.Header.Set("X-OpenExec-Checkout-ID", checkoutID)
	}
	if fixture.token != "" {
		request.Header.Set("Authorization", "Bearer "+fixture.token)
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

func TestRepositoryGraphChangedImpactIsCheckoutAuthorizedAndBounded(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	request := knowledge.ChangedImpactRequest{Files: []string{"main.go"}, MaxDepth: 2}
	if response := graphPostRequest(t, fixture, "/api/v1/repository-graph/impact/changed", "", request); response.Code != http.StatusForbidden {
		t.Fatalf("missing checkout status = %d, body=%s", response.Code, response.Body.String())
	}
	response := graphPostRequest(t, fixture, "/api/v1/repository-graph/impact/changed", fixture.identity.CheckoutID, request)
	if response.Code != http.StatusOK {
		t.Fatalf("changed impact status = %d, body=%s", response.Code, response.Body.String())
	}
	var result knowledge.ChangedImpactResponse
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Generation.Freshness != knowledge.FreshnessCurrent || len(result.ChangedSymbols) != 3 || len(result.Propagation.DirectCallers) == 0 {
		t.Fatalf("changed impact response = %#v", result)
	}
	unsafe := graphPostRequest(t, fixture, "/api/v1/repository-graph/impact/changed", fixture.identity.CheckoutID,
		knowledge.ChangedImpactRequest{Files: []string{"../secret.go"}})
	if unsafe.Code != http.StatusUnprocessableEntity {
		t.Fatalf("unsafe changed impact status = %d, body=%s", unsafe.Code, unsafe.Body.String())
	}
	for _, body := range []string{`{"files":["main.go"],"unknown":true}`, `{"files":["main.go"]}{"files":["other.go"]}`} {
		malformedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/repository-graph/impact/changed", bytes.NewBufferString(body))
		malformedRequest.Header.Set("X-OpenExec-Checkout-ID", fixture.identity.CheckoutID)
		malformedRequest.Header.Set("Authorization", "Bearer "+fixture.token)
		malformedResponse := httptest.NewRecorder()
		fixture.server.Mux.ServeHTTP(malformedResponse, malformedRequest)
		if malformedResponse.Code != http.StatusBadRequest {
			t.Fatalf("malformed changed impact status = %d, body=%s", malformedResponse.Code, malformedResponse.Body.String())
		}
	}
}

func TestRepositoryGraphValidationLifecycleRoundTripsAndRefusesStaleAcceptance(t *testing.T) {
	fixture := newGraphQueryFixture(t)
	proposalResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/propose", fixture.identity.CheckoutID, validationPlanProposalRequest{
		TaskID: "console-task-one", Files: []string{"main.go"}, MaxDepth: 2,
	})
	if proposalResponse.Code != http.StatusCreated {
		t.Fatalf("proposal status = %d, body=%s", proposalResponse.Code, proposalResponse.Body.String())
	}
	var proposed state.ValidationPlanRevision
	if err := json.Unmarshal(proposalResponse.Body.Bytes(), &proposed); err != nil {
		t.Fatal(err)
	}
	if proposed.Status != "proposed" || proposed.ImpactQuery.Files[0] != "main.go" || len(proposed.ImpactSummary.ChangedSymbolIDs) != 3 || len(proposed.Items) == 0 {
		t.Fatalf("proposal = %#v", proposed)
	}

	read := graphRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+proposed.ID, fixture.identity.CheckoutID)
	if read.Code != http.StatusOK {
		t.Fatalf("read proposal status = %d, body=%s", read.Code, read.Body.String())
	}
	unauthorized := graphRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+proposed.ID, "different-checkout")
	if unauthorized.Code != http.StatusForbidden {
		t.Fatalf("cross-checkout plan read status = %d", unauthorized.Code)
	}

	missingDecisions := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+proposed.ID+"/accept", fixture.identity.CheckoutID, struct{}{})
	if missingDecisions.Code != http.StatusUnprocessableEntity {
		t.Fatalf("accept without explicit item policy status = %d, body=%s", missingDecisions.Code, missingDecisions.Body.String())
	}
	acceptRequest := validationPlanAcceptRequest{Items: []state.ValidationItemDecision{{ID: proposed.Items[0].ID, Disposition: "accepted", Requirement: "required"}}}
	acceptedResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+proposed.ID+"/accept", fixture.identity.CheckoutID, acceptRequest)
	if acceptedResponse.Code != http.StatusCreated {
		t.Fatalf("accept status = %d, body=%s", acceptedResponse.Code, acceptedResponse.Body.String())
	}
	var accepted state.ValidationPlanRevision
	if err := json.Unmarshal(acceptedResponse.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	var acceptedItem state.ValidationItem
	for _, item := range accepted.Items {
		if item.Disposition == "accepted" {
			acceptedItem = item
		}
	}
	if accepted.Status != "accepted" || accepted.Revision != 2 || acceptedItem.ID == "" || acceptedItem.Requirement != "required" {
		t.Fatalf("accepted plan = %#v", accepted)
	}
	retryResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+proposed.ID+"/accept", fixture.identity.CheckoutID, acceptRequest)
	var retried state.ValidationPlanRevision
	if retryResponse.Code != http.StatusCreated || json.Unmarshal(retryResponse.Body.Bytes(), &retried) != nil || retried.ID != accepted.ID {
		t.Fatalf("accept retry status = %d, body=%s", retryResponse.Code, retryResponse.Body.String())
	}

	runResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-runs", fixture.identity.CheckoutID, validationRunRequest{ID: "console-run-one", Mode: "workspace-write"})
	if runResponse.Code != http.StatusCreated {
		t.Fatalf("register run status = %d, body=%s", runResponse.Code, runResponse.Body.String())
	}
	stepResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-runs/console-run-one/steps", fixture.identity.CheckoutID, validationRunStepRequest{ID: "console-step-one", Phase: "verify", Iteration: 1, Status: "completed"})
	if stepResponse.Code != http.StatusCreated {
		t.Fatalf("register step status = %d, body=%s", stepResponse.Code, stepResponse.Body.String())
	}
	mismatchedEvidence := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+accepted.ID+"/evidence", fixture.identity.CheckoutID, validationEvidenceRequest{
		ValidationItemID: acceptedItem.ID, RunID: "console-run-one", RunStepID: "console-step-one", Status: "failed",
	})
	if mismatchedEvidence.Code != http.StatusUnprocessableEntity {
		t.Fatalf("mismatched evidence status = %d, body=%s", mismatchedEvidence.Code, mismatchedEvidence.Body.String())
	}
	evidenceResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+accepted.ID+"/evidence", fixture.identity.CheckoutID, validationEvidenceRequest{
		ValidationItemID: acceptedItem.ID, RunID: "console-run-one", RunStepID: "console-step-one", Status: "passed",
	})
	if evidenceResponse.Code != http.StatusCreated {
		t.Fatalf("link evidence status = %d, body=%s", evidenceResponse.Code, evidenceResponse.Body.String())
	}
	completionResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+accepted.ID+"/completion", fixture.identity.CheckoutID, struct{}{})
	if completionResponse.Code != http.StatusCreated {
		t.Fatalf("finalize completion status = %d, body=%s", completionResponse.Code, completionResponse.Body.String())
	}
	var completion state.CompletionReport
	if err := json.Unmarshal(completionResponse.Body.Bytes(), &completion); err != nil {
		t.Fatal(err)
	}
	if completion.ID == "" || !completion.CanComplete || len(completion.Verified) == 0 {
		t.Fatalf("completion report = %#v", completion)
	}
	completionRead := graphRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+accepted.ID+"/completion", fixture.identity.CheckoutID)
	if completionRead.Code != http.StatusOK || !strings.Contains(completionRead.Body.String(), completion.ID) {
		t.Fatalf("completion read status = %d, body=%s", completionRead.Code, completionRead.Body.String())
	}

	staleProposalResponse := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/propose", fixture.identity.CheckoutID, validationPlanProposalRequest{
		TaskID: "console-task-stale", Files: []string{"main.go"}, MaxDepth: 2,
	})
	if staleProposalResponse.Code != http.StatusCreated {
		t.Fatalf("stale proposal setup status = %d, body=%s", staleProposalResponse.Code, staleProposalResponse.Body.String())
	}
	var staleProposal state.ValidationPlanRevision
	if err := json.Unmarshal(staleProposalResponse.Body.Bytes(), &staleProposal); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture.server.ProjectsDir, "main.go"), []byte("package sample\nfunc Target() { println(\"changed\") }\nfunc Caller() { Target() }\nfunc Run() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleAccept := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+staleProposal.ID+"/accept", fixture.identity.CheckoutID, validationPlanAcceptRequest{Items: []state.ValidationItemDecision{}})
	if staleAccept.Code != http.StatusConflict || !strings.Contains(staleAccept.Body.String(), "stale") {
		t.Fatalf("stale accept status = %d, body=%s", staleAccept.Code, staleAccept.Body.String())
	}
	retryAfterMove := graphPostRequest(t, fixture, "/api/v1/repository-graph/validation-plans/"+proposed.ID+"/accept", fixture.identity.CheckoutID, acceptRequest)
	var retriedAfterMove state.ValidationPlanRevision
	if retryAfterMove.Code != http.StatusCreated || json.Unmarshal(retryAfterMove.Body.Bytes(), &retriedAfterMove) != nil || retriedAfterMove.ID != accepted.ID {
		t.Fatalf("accepted proposal retry after worktree movement status = %d, body=%s", retryAfterMove.Code, retryAfterMove.Body.String())
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
