package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"

	_ "modernc.org/sqlite"
)

// setupTestUsageServer creates a UsageServer with in-memory databases for testing.
func setupTestUsageServer(t *testing.T) (*UsageServer, *audit.AuditLogger, *session.SQLiteRepository) {
	t.Helper()

	// Create in-memory audit database
	auditDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open audit db: %v", err)
	}

	// Initialize audit schema
	if _, err := auditDB.Exec(audit.Schema); err != nil {
		t.Fatalf("failed to init audit schema: %v", err)
	}
	if _, err := auditDB.Exec(audit.EntrySchema); err != nil {
		t.Fatalf("failed to init entry schema: %v", err)
	}

	// Create audit logger using temp file (NewLogger requires file path)
	tmpDir := t.TempDir()
	auditLogger, err := audit.NewLogger(filepath.Join(tmpDir, "audit.db"))
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}

	// Create in-memory session database
	sessionDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open session db: %v", err)
	}

	sessionRepo, err := session.NewSQLiteRepository(sessionDB)
	if err != nil {
		t.Fatalf("failed to create session repo: %v", err)
	}

	// Create usage server
	srv := NewUsageServer(UsageServerConfig{
		AuditLogger: auditLogger,
		SessionRepo: sessionRepo,
		Addr:        ":0",
	})

	t.Cleanup(func() {
		auditLogger.Close()
		sessionRepo.Close()
	})

	return srv, auditLogger, sessionRepo
}

// seedTestAuditData creates test audit entries.
func seedTestAuditData(t *testing.T, logger *audit.AuditLogger) {
	t.Helper()
	ctx := context.Background()

	// Create some LLM response events
	for i := 0; i < 5; i++ {
		entry, err := audit.NewEntry(audit.EventLLMResponseReceived, "test-user", "system")
		if err != nil {
			t.Fatalf("failed to create audit entry: %v", err)
		}
		entry.WithProvider("openai", "gpt-4").
			WithTokens(100+int64(i*10), 50+int64(i*5)).
			WithCost(0.01 + float64(i)*0.005).
			WithDuration(1000 + int64(i*100)).
			WithSuccess(true)
		built, _ := entry.Build()
		if err := logger.Log(ctx, built); err != nil {
			t.Fatalf("failed to log entry: %v", err)
		}
	}

	// Create some tool call events
	for i := 0; i < 3; i++ {
		entry, _ := audit.NewEntry(audit.EventToolCallRequested, "test-user", "system")
		built, _ := entry.Build()
		logger.Log(ctx, built)
	}

	for i := 0; i < 2; i++ {
		entry, _ := audit.NewEntry(audit.EventToolCallCompleted, "test-user", "system")
		built, _ := entry.Build()
		logger.Log(ctx, built)
	}
}

// seedTestSessionData creates test session and message data.
func seedTestSessionData(t *testing.T, repo *session.SQLiteRepository) string {
	t.Helper()
	ctx := context.Background()

	// Create a session
	sess, err := session.NewSession("/test/project", "openai", "gpt-4")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}
	sess.Title = "Test Session"

	if err := repo.CreateSession(ctx, sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Create some messages with token usage
	for i := 0; i < 3; i++ {
		role := session.RoleUser
		if i%2 == 1 {
			role = session.RoleAssistant
		}
		msg, _ := session.NewMessage(sess.ID, role, "Test message")
		msg.SetTokenUsage(100+i*10, 50+i*5, 0.01+float64(i)*0.005)
		repo.CreateMessage(ctx, msg)
	}

	return sess.ID
}

func TestHandleGetUsageSummary(t *testing.T) {
	srv, logger, _ := setupTestUsageServer(t)
	seedTestAuditData(t, logger)

	req := httptest.NewRequest("GET", "/api/usage/summary", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response UsageSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.TotalRequests != 5 {
		t.Errorf("TotalRequests = %d, want 5", response.TotalRequests)
	}

	if response.TotalTokensInput <= 0 {
		t.Errorf("TotalTokensInput = %d, want > 0", response.TotalTokensInput)
	}

	if response.TotalCostUSD <= 0 {
		t.Errorf("TotalCostUSD = %f, want > 0", response.TotalCostUSD)
	}
}

func TestHandleGetUsageSummaryWithTimeFilter(t *testing.T) {
	srv, logger, _ := setupTestUsageServer(t)
	seedTestAuditData(t, logger)

	// Query with time filter (should include all data since it was just created)
	// Use URL-encoded timestamp to avoid issues with + in query string
	since := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/usage/summary?since="+since, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response UsageSummaryResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// The response should still have data (all data was created recently)
	if response.TotalRequests != 5 {
		t.Errorf("TotalRequests = %d, want 5", response.TotalRequests)
	}

	// Period should be set when filter is applied
	if response.Period == nil {
		// Debug: check if filter was parsed correctly
		t.Logf("since parameter: %s", since)
		t.Error("expected Period to be set when filter is applied")
	} else if response.Period.Since.IsZero() {
		t.Error("expected Period.Since to be non-zero")
	}
}

func TestHandleGetProviderUsage(t *testing.T) {
	srv, _, repo := setupTestUsageServer(t)
	seedTestSessionData(t, repo)

	req := httptest.NewRequest("GET", "/api/usage/providers", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response ProviderUsageResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Providers) != 1 {
		t.Errorf("len(Providers) = %d, want 1", len(response.Providers))
	}

	if response.Total == nil {
		t.Error("Total should not be nil")
	}

	if response.Total.SessionCount != 1 {
		t.Errorf("Total.SessionCount = %d, want 1", response.Total.SessionCount)
	}
}

func TestHandleGetSessionUsage(t *testing.T) {
	srv, _, repo := setupTestUsageServer(t)
	sessionID := seedTestSessionData(t, repo)

	req := httptest.NewRequest("GET", "/api/usage/sessions/"+sessionID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response SessionUsageResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.SessionID != sessionID {
		t.Errorf("SessionID = %s, want %s", response.SessionID, sessionID)
	}

	if response.MessageCount != 3 {
		t.Errorf("MessageCount = %d, want 3", response.MessageCount)
	}

	if response.TotalTokensInput <= 0 {
		t.Errorf("TotalTokensInput = %d, want > 0", response.TotalTokensInput)
	}
}

func TestHandleGetSessionUsageMissingID(t *testing.T) {
	srv, _, _ := setupTestUsageServer(t)

	// Note: The router should handle this, but test the handler behavior
	req := httptest.NewRequest("GET", "/api/usage/sessions/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	// Should return 404 as the route doesn't match without a session ID
	// or 400 if it hits the handler with empty ID
	if rec.Code == http.StatusOK {
		t.Errorf("expected non-200 status for missing session ID")
	}
}

func TestHandleGetToolCallStats(t *testing.T) {
	srv, logger, _ := setupTestUsageServer(t)
	seedTestAuditData(t, logger)

	req := httptest.NewRequest("GET", "/api/usage/tools", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response ToolCallStatsResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.TotalRequested != 3 {
		t.Errorf("TotalRequested = %d, want 3", response.TotalRequested)
	}

	if response.TotalCompleted != 2 {
		t.Errorf("TotalCompleted = %d, want 2", response.TotalCompleted)
	}
}

func TestHandleGetAuditLogs(t *testing.T) {
	srv, logger, _ := setupTestUsageServer(t)
	seedTestAuditData(t, logger)

	req := httptest.NewRequest("GET", "/api/usage/audit-logs?limit=10", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response AuditLogResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// We created 5 LLM events + 3 requested + 2 completed = 10 total
	if response.TotalCount != 10 {
		t.Errorf("TotalCount = %d, want 10", response.TotalCount)
	}

	if len(response.Entries) > 10 {
		t.Errorf("len(Entries) = %d, want <= 10", len(response.Entries))
	}
}

func TestHandleGetAuditLogsWithEventTypeFilter(t *testing.T) {
	srv, logger, _ := setupTestUsageServer(t)
	seedTestAuditData(t, logger)

	req := httptest.NewRequest("GET", "/api/usage/audit-logs?event_type=llm.response_received", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response AuditLogResponse
	json.NewDecoder(rec.Body).Decode(&response)

	if response.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5 (only LLM response events)", response.TotalCount)
	}

	for _, entry := range response.Entries {
		if entry.EventType != "llm.response_received" {
			t.Errorf("EventType = %s, want llm.response_received", entry.EventType)
		}
	}
}

func TestHandleGetAuditLogsPagination(t *testing.T) {
	srv, logger, _ := setupTestUsageServer(t)
	seedTestAuditData(t, logger)

	req := httptest.NewRequest("GET", "/api/usage/audit-logs?limit=3&offset=0", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response AuditLogResponse
	json.NewDecoder(rec.Body).Decode(&response)

	if len(response.Entries) != 3 {
		t.Errorf("len(Entries) = %d, want 3", len(response.Entries))
	}

	if !response.HasMore {
		t.Error("HasMore = false, want true")
	}
}

func TestHandleGetCostByModel(t *testing.T) {
	srv, _, repo := setupTestUsageServer(t)
	seedTestSessionData(t, repo)

	req := httptest.NewRequest("GET", "/api/usage/cost-by-model", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response CostByModelResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Models) != 1 {
		t.Errorf("len(Models) = %d, want 1", len(response.Models))
	}

	if response.TotalCostUSD <= 0 {
		t.Errorf("TotalCostUSD = %f, want > 0", response.TotalCostUSD)
	}

	// Percentage should be 100 since there's only one model
	if len(response.Models) > 0 && response.Models[0].PercentageOfTotal != 100 {
		t.Errorf("PercentageOfTotal = %f, want 100", response.Models[0].PercentageOfTotal)
	}
}

func TestParseTimeFilter(t *testing.T) {
	tests := []struct {
		name        string
		queryString string
		wantSince   bool
		wantUntil   bool
	}{
		{
			name:        "no filters",
			queryString: "",
			wantSince:   false,
			wantUntil:   false,
		},
		{
			name:        "since only",
			queryString: "since=2024-01-01T00:00:00Z",
			wantSince:   true,
			wantUntil:   false,
		},
		{
			name:        "until only",
			queryString: "until=2024-12-31T23:59:59Z",
			wantSince:   false,
			wantUntil:   true,
		},
		{
			name:        "both filters",
			queryString: "since=2024-01-01T00:00:00Z&until=2024-12-31T23:59:59Z",
			wantSince:   true,
			wantUntil:   true,
		},
		{
			name:        "invalid format ignored",
			queryString: "since=invalid&until=also-invalid",
			wantSince:   false,
			wantUntil:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test?"+tt.queryString, nil)
			filter := parseTimeFilter(req)

			gotSince := !filter.Since.IsZero()
			gotUntil := !filter.Until.IsZero()

			if gotSince != tt.wantSince {
				t.Errorf("Since: got %v, want %v", gotSince, tt.wantSince)
			}
			if gotUntil != tt.wantUntil {
				t.Errorf("Until: got %v, want %v", gotUntil, tt.wantUntil)
			}
		})
	}
}

func TestParseAuditFilter(t *testing.T) {
	tests := []struct {
		name          string
		queryString   string
		wantLimit     int
		wantOffset    int
		wantEventType int
	}{
		{
			name:        "default limit",
			queryString: "",
			wantLimit:   100,
			wantOffset:  0,
		},
		{
			name:        "custom limit",
			queryString: "limit=50",
			wantLimit:   50,
			wantOffset:  0,
		},
		{
			name:        "max limit enforced",
			queryString: "limit=5000",
			wantLimit:   1000,
			wantOffset:  0,
		},
		{
			name:        "with offset",
			queryString: "limit=10&offset=20",
			wantLimit:   10,
			wantOffset:  20,
		},
		{
			name:          "with event types",
			queryString:   "event_type=llm.response_received&event_type=tool_call.completed",
			wantLimit:     100,
			wantOffset:    0,
			wantEventType: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test?"+tt.queryString, nil)
			filter := parseAuditFilter(req)

			if filter.Limit != tt.wantLimit {
				t.Errorf("Limit: got %d, want %d", filter.Limit, tt.wantLimit)
			}
			if filter.Offset != tt.wantOffset {
				t.Errorf("Offset: got %d, want %d", filter.Offset, tt.wantOffset)
			}
			if len(filter.EventTypes) != tt.wantEventType {
				t.Errorf("EventTypes: got %d, want %d", len(filter.EventTypes), tt.wantEventType)
			}
		})
	}
}

func TestRegisterUsageRoutes(t *testing.T) {
	// Create dependencies
	tmpDir := t.TempDir()
	auditLogger, err := audit.NewLogger(filepath.Join(tmpDir, "audit.db"))
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	sessionDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open session db: %v", err)
	}
	sessionRepo, err := session.NewSQLiteRepository(sessionDB)
	if err != nil {
		t.Fatalf("failed to create session repo: %v", err)
	}
	defer sessionRepo.Close()

	// Create a mux and register routes
	mux := http.NewServeMux()
	RegisterUsageRoutes(mux, auditLogger, sessionRepo)

	// Test that routes are registered
	routes := []struct {
		method string
		path   string
	}{
		{"GET", "/api/usage/summary"},
		{"GET", "/api/usage/providers"},
		{"GET", "/api/usage/tools"},
		{"GET", "/api/usage/audit-logs"},
		{"GET", "/api/usage/cost-by-model"},
	}

	for _, route := range routes {
		req := httptest.NewRequest(route.method, route.path, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		// Should return 200 OK for valid routes (even with no data)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s: status = %d, want %d", route.method, route.path, rec.Code, http.StatusOK)
		}
	}
}
