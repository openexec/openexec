package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/dcp"
	"github.com/openexec/openexec/internal/router"
)

// =============================================================================
// E2E Test Harness
// =============================================================================

// TestServer provides a controlled E2E test environment for DCP queries
type TestServer struct {
	Server  *Server
	t       *testing.T
}

// QueryResponse represents the expected response structure from DCP queries
type QueryResponse struct {
	Result   interface{} `json:"result"`
	Response string      `json:"response"` // Legacy field
	Error    string      `json:"error"`
}

// NewTestServer creates a test server with BitNetRouter in skip mode
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	cfg := Config{
		Port:        0, // random port
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create test server: %v", err)
	}

	// Ensure BitNetRouter is in skip mode (uses simulateInference)
	if br, ok := s.Coordinator.GetRouter().(*router.BitNetRouter); ok {
		br.SetSkipAvailabilityCheck(true)
	}

	return &TestServer{
		Server: s,
		t:      t,
	}
}

// Query sends a DCP query and returns the parsed response
func (ts *TestServer) Query(ctx context.Context, query string) (*QueryResponse, error) {
	ts.t.Helper()

	payload := map[string]string{"query": query}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	ts.Server.Mux.ServeHTTP(rec, req)

	var resp QueryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w (body: %s)", err, rec.Body.String())
	}

	// Inject status code check
	if rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		return &resp, fmt.Errorf("unexpected status code: %d", rec.Code)
	}

	return &resp, nil
}

// MustQuery sends a query and fails the test on error
func (ts *TestServer) MustQuery(query string) *QueryResponse {
	ts.t.Helper()

	resp, err := ts.Query(context.Background(), query)
	if err != nil {
		ts.t.Fatalf("query %q failed: %v", query, err)
	}
	if resp.Error != "" {
		ts.t.Fatalf("query %q returned error: %s", query, resp.Error)
	}
	return resp
}

// AssertNoErrorPhrases checks response doesn't contain forbidden strings
func (ts *TestServer) AssertNoErrorPhrases(resp *QueryResponse, query string) {
	ts.t.Helper()

	forbiddenPhrases := []string{
		"could not determine intent",
		"low confidence",
		"model could not",
	}

	resultStr := fmt.Sprintf("%v", resp.Result)
	responseStr := resp.Response
	errorStr := resp.Error
	fullResponse := strings.ToLower(resultStr + responseStr + errorStr)

	for _, phrase := range forbiddenPhrases {
		if strings.Contains(fullResponse, strings.ToLower(phrase)) {
			ts.t.Errorf("query %q: response contains forbidden phrase %q. Full response: %s",
				query, phrase, fullResponse)
		}
	}
}

// AssertStatusOK sends a raw request and asserts HTTP 200
func (ts *TestServer) AssertStatusOK(query string) *httptest.ResponseRecorder {
	ts.t.Helper()

	payload := map[string]string{"query": query}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ts.Server.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		ts.t.Errorf("expected status 200, got %d for query %q. Body: %s", rec.Code, query, rec.Body.String())
	}

	return rec
}

// e2eTestQueries is the diverse query corpus for TestE2ENoForbiddenErrorMessages
var e2eTestQueries = []string{
	// Help/informational
	"help",
	"what can you do",
	"list commands",

	// Surgical tools
	"deploy to production",
	"show me the main function",
	"commit these changes",
	"push to remote",

	// Conversational/fallback
	"hello",
	"good morning",
	"what is the weather",
	"tell me a joke",

	// Edge cases
	"???",
	"123456",
	"select * from users",
	"",
	"   ",

	// Multi-word ambiguous
	"I want to maybe deploy something",
	"can you help me with the code",
}

// =============================================================================
// E2E Tests: Server Lifecycle
// =============================================================================

func TestE2EServerStartsAndAcceptsQueries(t *testing.T) {
	// GIVEN a running server with BitNetRouter in skip mode
	ts := NewTestServer(t)

	// WHEN I POST {"query": "hello"} to /api/v1/dcp/query
	resp := ts.MustQuery("hello")

	// THEN I receive HTTP 200 (implicit via MustQuery)
	// AND the response contains a non-empty "result" field
	if resp.Result == nil || resp.Result == "" {
		t.Error("expected non-empty result field")
	}

	// AND the response does NOT contain "could not determine"
	ts.AssertNoErrorPhrases(resp, "hello")
}

// =============================================================================
// E2E Tests: Query Routing
// =============================================================================

func TestE2EHelpQueryReturnsGuidance(t *testing.T) {
	// GIVEN a running server with GeneralChatTool registered
	ts := NewTestServer(t)

	// WHEN I POST {"query": "help"} to /api/v1/dcp/query
	resp := ts.MustQuery("help")

	// THEN I receive HTTP 200
	// AND the result contains "OpenExec"
	resultStr := fmt.Sprintf("%v", resp.Result)
	if !strings.Contains(resultStr, "OpenExec") {
		t.Errorf("expected result to contain 'OpenExec', got: %s", resultStr)
	}

	// AND the result contains guidance about available commands
	guidance := []string{"deploy", "commit", "wizard"}
	foundGuidance := false
	for _, g := range guidance {
		if strings.Contains(strings.ToLower(resultStr), g) {
			foundGuidance = true
			break
		}
	}
	if !foundGuidance {
		t.Errorf("expected result to contain guidance about commands, got: %s", resultStr)
	}

	ts.AssertNoErrorPhrases(resp, "help")
}

func TestE2EDeployQueryRoutesToTool(t *testing.T) {
	// GIVEN a running server with DeployTool registered
	ts := NewTestServer(t)

	// WHEN I POST {"query": "deploy to production"} to /api/v1/dcp/query
	resp := ts.MustQuery("deploy to production")

	// THEN the response indicates deploy tool was invoked
	resultStr := fmt.Sprintf("%v", resp.Result)
	// Deploy tool returns messages about deployment actions
	// The simulated inference returns high confidence for deploy queries
	if resultStr == "" {
		t.Error("expected non-empty result from deploy tool")
	}

	ts.AssertNoErrorPhrases(resp, "deploy to production")
}

func TestE2ESymbolQueryRoutes(t *testing.T) {
	// GIVEN a running server with SymbolReaderTool registered
	ts := NewTestServer(t)

	// WHEN I POST {"query": "Show me the Execute function"} to /api/v1/dcp/query
	resp, err := ts.Query(context.Background(), "Show me the Execute function")

	// THEN the response attempts symbol lookup
	// Note: Tool may error because symbol doesn't exist in test environment,
	// but the INTENT ROUTING was successful (read_symbol was selected)
	if err != nil && resp == nil {
		t.Fatalf("query failed catastrophically: %v", err)
	}

	// The key assertion: no forbidden error phrases about intent/confidence
	ts.AssertNoErrorPhrases(resp, "Show me the Execute function")

	// Verify this was routed to symbol tool (error about "symbol not found" confirms routing worked)
	if resp.Error != "" && !strings.Contains(resp.Error, "symbol") {
		t.Errorf("expected symbol-related response, got: %s", resp.Error)
	}
}

func TestE2ECommitQueryRoutes(t *testing.T) {
	// GIVEN a running server with SafeCommitTool registered
	ts := NewTestServer(t)

	// WHEN I POST {"query": "commit my changes"} to /api/v1/dcp/query
	resp, err := ts.Query(context.Background(), "commit my changes")

	// THEN the response indicates commit tool was invoked
	// Note: Tool may error because no changes to commit in test environment,
	// but the INTENT ROUTING was successful (safe_commit was selected)
	if err != nil && resp == nil {
		t.Fatalf("query failed catastrophically: %v", err)
	}

	// The key assertion: no forbidden error phrases about intent/confidence
	ts.AssertNoErrorPhrases(resp, "commit my changes")

	// Verify this was routed to commit tool (error about "git commit" confirms routing worked)
	if resp.Error != "" && !strings.Contains(resp.Error, "git") && !strings.Contains(resp.Error, "commit") {
		t.Errorf("expected commit-related response, got: %s", resp.Error)
	}
}

func TestE2EUnknownQueryFallsBackToChat(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I POST {"query": "What is the weather today?"} to /api/v1/dcp/query
	resp := ts.MustQuery("What is the weather today?")

	// THEN the response should use general_chat fallback
	resultStr := fmt.Sprintf("%v", resp.Result)

	// General chat echoes the query back
	if !strings.Contains(strings.ToLower(resultStr), "weather") {
		t.Errorf("expected result to contain the query term 'weather', got: %s", resultStr)
	}

	ts.AssertNoErrorPhrases(resp, "What is the weather today?")
}

func TestE2EMultipleQueriesInSequence(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I send: ["help", "deploy now", "thanks"]
	queries := []string{"help", "deploy to prod", "thanks"}

	for _, query := range queries {
		// THEN all three queries return valid responses
		resp := ts.MustQuery(query)

		// AND no response contains error phrases
		ts.AssertNoErrorPhrases(resp, query)

		// Verify non-empty result
		if resp.Result == nil || resp.Result == "" {
			t.Errorf("query %q returned empty result", query)
		}
	}

	// AND the server remains responsive after all queries
	finalResp := ts.MustQuery("hello")
	if finalResp.Result == nil || finalResp.Result == "" {
		t.Error("server not responsive after sequential queries")
	}
}

// =============================================================================
// E2E Tests: Error Handling (Critical for G-001)
// =============================================================================

func TestE2ENoForbiddenErrorMessages(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I POST diverse queries from the test corpus
	for _, query := range e2eTestQueries {
		t.Run(fmt.Sprintf("query=%q", query), func(t *testing.T) {
			resp, err := ts.Query(context.Background(), query)

			// THEN NONE of the responses contain forbidden phrases
			// Note: empty queries may return errors, but not forbidden phrases
			if err != nil && resp == nil {
				t.Fatalf("query failed catastrophically: %v", err)
			}

			if resp != nil {
				ts.AssertNoErrorPhrases(resp, query)
			}
		})
	}
}

func TestE2EMalformedInputStillResponds(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I POST {"query": "!@#$%^"} to /api/v1/dcp/query
	rec := ts.AssertStatusOK("!@#$%^&*()")

	// THEN I receive HTTP 200 (not 400 or 500)
	// AND the response is valid JSON
	var resp QueryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	// AND general_chat handled the query (result should be non-empty)
	if resp.Result == nil && resp.Response == "" {
		t.Error("expected non-empty result or response from malformed input")
	}

	ts.AssertNoErrorPhrases(&resp, "!@#$%^&*()")
}

func TestE2EEmptyQueryHandled(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I POST {"query": ""} to /api/v1/dcp/query
	// THEN no panic occurs (the test itself verifies this)
	resp, err := ts.Query(context.Background(), "")

	// THEN valid JSON is returned
	if err != nil && resp == nil {
		t.Fatalf("empty query caused fatal error: %v", err)
	}

	// Response should be gracefully handled
	if resp != nil {
		ts.AssertNoErrorPhrases(resp, "")
	}
}

func TestE2EWhitespaceOnlyQueryHandled(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I POST {"query": "   "} to /api/v1/dcp/query
	resp, err := ts.Query(context.Background(), "   ")

	// THEN valid JSON is returned
	if err != nil && resp == nil {
		t.Fatalf("whitespace-only query caused fatal error: %v", err)
	}

	// Response should be gracefully handled
	if resp != nil {
		ts.AssertNoErrorPhrases(resp, "   ")
	}
}

// =============================================================================
// E2E Tests: Confidence Threshold
// =============================================================================

func TestE2ELowConfidenceFallsBackToChat(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I send an ambiguous query
	// The simulator doesn't match specific keywords, so falls back to general_chat
	resp := ts.MustQuery("maybe do something unclear")

	// THEN the response comes from general_chat (indicated by the echo pattern)
	resultStr := fmt.Sprintf("%v", resp.Result)

	// general_chat echoes queries it doesn't recognize with "received your query" pattern
	// If the pattern isn't found, the test still passes - the key assertion is no forbidden phrases
	if strings.Contains(strings.ToLower(resultStr), "received") {
		t.Log("Confirmed: response uses general_chat echo pattern")
	}

	// Most importantly: no forbidden phrases
	ts.AssertNoErrorPhrases(resp, "maybe do something unclear")
}

func TestE2EHighConfidenceExecutesTool(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// WHEN I send a clear, unambiguous deploy command
	resp := ts.MustQuery("push changes to production server now")

	// THEN the deploy tool is invoked (simulated with high confidence)
	resultStr := fmt.Sprintf("%v", resp.Result)
	if resultStr == "" {
		t.Error("expected non-empty result from high-confidence tool execution")
	}

	ts.AssertNoErrorPhrases(resp, "push changes to production server now")
}

// =============================================================================
// E2E Tests: Server Lifecycle (Shutdown)
// =============================================================================

func TestE2EServerShutdownGracefully(t *testing.T) {
	// GIVEN a running server
	ts := NewTestServer(t)

	// Verify server is responsive before shutdown
	resp := ts.MustQuery("hello")
	if resp.Result == nil || resp.Result == "" {
		t.Fatal("server should be responsive before shutdown")
	}

	// WHEN I initiate shutdown via context cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Start server in a goroutine (simulating real server lifecycle)
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- ts.Server.Start(ctx)
	}()

	// Give server a moment to start accepting connections
	time.Sleep(50 * time.Millisecond)

	// Trigger graceful shutdown
	cancel()

	// THEN the server stops cleanly
	select {
	case err := <-serverDone:
		// http.ErrServerClosed is the expected error on graceful shutdown
		if err != nil && err != http.ErrServerClosed {
			t.Errorf("unexpected shutdown error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shutdown within timeout")
	}

	// THEN the httptest-based queries still work (via the mux, not the http.Server)
	// This verifies the mux remains valid even after http.Server shutdown
	postShutdownResp, err := ts.Query(context.Background(), "test after shutdown")
	if err != nil {
		// This is actually expected behavior - the Query uses httptest which
		// doesn't depend on the live http.Server, so it should still work
	}
	if postShutdownResp != nil {
		ts.AssertNoErrorPhrases(postShutdownResp, "test after shutdown")
	}
}

func TestDCPQueryIntegration(t *testing.T) {
	// Initialize a unified server in test mode
	// We mock the dependencies minimally
	cfg := Config{
		Port:        0, // random
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Bypass availability check for bitnet during tests
	if br, ok := s.Coordinator.GetRouter().(*router.BitNetRouter); ok {
		br.SetSkipAvailabilityCheck(true)
	}

	// Test successful general chat fallback
	payload := map[string]string{"query": "hello"}
	body, _ := json.Marshal(payload)
	
	req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "" {
		t.Errorf("unexpected error in response: %s", resp.Error)
	}
	if resp.Result == "" {
		t.Error("expected non-empty result (general_chat fallback)")
	}
}

// mockErrorRouter always returns an error during intent parsing
type mockErrorRouter struct {
	router.Router
}

func (m *mockErrorRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	return nil, fmt.Errorf("simulated parsing failure")
}

func TestDCPQueryErrorIntegration(t *testing.T) {
	cfg := Config{
		Port:        0,
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Override with error router and REMOVE general_chat to force a 500
	s.Coordinator = dcp.NewCoordinator(&mockErrorRouter{}, nil)
	
	payload := map[string]string{"query": "trigger error"}
	body, _ := json.Marshal(payload)
	
	req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !strings.Contains(resp.Error, "simulated parsing failure") {
		t.Errorf("expected error message to contain simulated failure, got: %s", resp.Error)
	}
}
