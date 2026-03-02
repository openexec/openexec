package audit

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewLogger(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	if logger.db == nil {
		t.Error("NewLogger() db should not be nil")
	}
}

func TestNewLogger_InvalidPath(t *testing.T) {
	// Try to create a logger with an invalid path
	_, err := NewLogger("/nonexistent/path/audit.db")
	if err == nil {
		t.Error("NewLogger() should fail with invalid path")
	}
}

func TestAuditLogger_Log(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	builder, err := NewEntry(EventSessionCreated, "user-123", "user")
	if err != nil {
		t.Fatalf("NewEntry() error = %v", err)
	}

	entry, err := builder.
		WithSession("session-456").
		WithProject("/path/to/project").
		WithSeverity(SeverityInfo).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if err := logger.Log(ctx, entry); err != nil {
		t.Errorf("Log() error = %v", err)
	}

	// Verify entry was stored
	retrieved, err := logger.GetByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if retrieved.ID != entry.ID {
		t.Errorf("Retrieved entry ID = %v, want %v", retrieved.ID, entry.ID)
	}
	if retrieved.EventType != EventSessionCreated {
		t.Errorf("Retrieved entry EventType = %v, want %v", retrieved.EventType, EventSessionCreated)
	}
	if retrieved.ActorID != "user-123" {
		t.Errorf("Retrieved entry ActorID = %v, want %v", retrieved.ActorID, "user-123")
	}
	if !retrieved.SessionID.Valid || retrieved.SessionID.String != "session-456" {
		t.Errorf("Retrieved entry SessionID = %v, want session-456", retrieved.SessionID)
	}
}

func TestAuditLogger_Log_NilEntry(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()
	err = logger.Log(ctx, nil)
	if err == nil {
		t.Error("Log() should fail with nil entry")
	}
}

func TestAuditLogger_LogEvent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	err = logger.LogEvent(ctx, EventSystemStartup, "system", "system")
	if err != nil {
		t.Errorf("LogEvent() error = %v", err)
	}

	// Verify entry was stored
	result, err := logger.Query(ctx, &QueryFilter{EventTypes: []EventType{EventSystemStartup}})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}

	if len(result.Entries) != 1 {
		t.Errorf("Query() returned %d entries, want 1", len(result.Entries))
	}
}

func TestAuditLogger_LogEvent_InvalidEventType(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	err = logger.LogEvent(ctx, EventType("invalid.event"), "system", "system")
	if err == nil {
		t.Error("LogEvent() should fail with invalid event type")
	}
}

func TestAuditLogger_Query_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	result, err := logger.Query(ctx, &QueryFilter{})
	if err != nil {
		t.Errorf("Query() error = %v", err)
	}

	if result.TotalCount != 0 {
		t.Errorf("Query() TotalCount = %d, want 0", result.TotalCount)
	}
	if len(result.Entries) != 0 {
		t.Errorf("Query() Entries = %d, want 0", len(result.Entries))
	}
}

func TestAuditLogger_Query_WithFilters(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create multiple entries
	events := []struct {
		eventType EventType
		actorID   string
		sessionID string
	}{
		{EventSessionCreated, "user-1", "session-1"},
		{EventSessionCreated, "user-2", "session-2"},
		{EventToolCallRequested, "user-1", "session-1"},
		{EventLLMRequestSent, "user-1", "session-1"},
	}

	for _, e := range events {
		builder, _ := NewEntry(e.eventType, e.actorID, "user")
		entry, _ := builder.WithSession(e.sessionID).Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	// Test filter by event type
	result, err := logger.Query(ctx, &QueryFilter{
		EventTypes: []EventType{EventSessionCreated},
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("Query(EventSessionCreated) returned %d entries, want 2", len(result.Entries))
	}

	// Test filter by session ID
	result, err = logger.Query(ctx, &QueryFilter{
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 3 {
		t.Errorf("Query(session-1) returned %d entries, want 3", len(result.Entries))
	}

	// Test filter by actor ID
	result, err = logger.Query(ctx, &QueryFilter{
		ActorID: "user-2",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 1 {
		t.Errorf("Query(user-2) returned %d entries, want 1", len(result.Entries))
	}
}

func TestAuditLogger_Query_Pagination(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create 10 entries
	for i := 0; i < 10; i++ {
		builder, _ := NewEntry(EventSessionCreated, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	// Test pagination
	result, err := logger.Query(ctx, &QueryFilter{
		Limit: 3,
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 3 {
		t.Errorf("Query(Limit=3) returned %d entries, want 3", len(result.Entries))
	}
	if result.TotalCount != 10 {
		t.Errorf("Query(Limit=3) TotalCount = %d, want 10", result.TotalCount)
	}
	if !result.HasMore {
		t.Error("Query(Limit=3) HasMore = false, want true")
	}

	// Test offset
	result, err = logger.Query(ctx, &QueryFilter{
		Limit:  3,
		Offset: 8,
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("Query(Limit=3,Offset=8) returned %d entries, want 2", len(result.Entries))
	}
	if result.HasMore {
		t.Error("Query(Limit=3,Offset=8) HasMore = true, want false")
	}
}

func TestAuditLogger_Query_TimeRange(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create entries with specific timestamps
	now := time.Now().UTC()
	times := []time.Time{
		now.Add(-2 * time.Hour),
		now.Add(-1 * time.Hour),
		now,
	}

	for _, ts := range times {
		builder, _ := NewEntry(EventSessionCreated, "user", "user")
		entry, _ := builder.WithTimestamp(ts).Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	// Query entries from the last 90 minutes
	result, err := logger.Query(ctx, &QueryFilter{
		Since: now.Add(-90 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("Query(Since=-90m) returned %d entries, want 2", len(result.Entries))
	}
}

func TestAuditLogger_GetByID_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	_, err = logger.GetByID(ctx, "nonexistent-id")
	if err != ErrAuditEntryNotFound {
		t.Errorf("GetByID() error = %v, want ErrAuditEntryNotFound", err)
	}
}

func TestAuditLogger_GetUsageStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create LLM response entries
	entries := []struct {
		provider     string
		model        string
		tokensInput  int64
		tokensOutput int64
		costUSD      float64
		durationMs   int64
		success      bool
	}{
		{"openai", "gpt-4", 100, 50, 0.01, 500, true},
		{"openai", "gpt-4", 200, 100, 0.02, 600, true},
		{"anthropic", "claude-3", 150, 75, 0.015, 550, true},
		{"anthropic", "claude-3", 50, 25, 0.005, 200, false}, // Failed request
	}

	for _, e := range entries {
		eventType := EventLLMResponseReceived
		if !e.success {
			eventType = EventLLMError
		}

		builder, _ := NewEntry(eventType, "system", "system")
		entry, _ := builder.
			WithProvider(e.provider, e.model).
			WithTokens(e.tokensInput, e.tokensOutput).
			WithCost(e.costUSD).
			WithDuration(e.durationMs).
			WithSuccess(e.success).
			Build()

		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	stats, err := logger.GetUsageStats(ctx, &QueryFilter{})
	if err != nil {
		t.Fatalf("GetUsageStats() error = %v", err)
	}

	if stats.TotalRequests != 4 {
		t.Errorf("UsageStats.TotalRequests = %d, want 4", stats.TotalRequests)
	}
	if stats.TotalTokensInput != 500 {
		t.Errorf("UsageStats.TotalTokensInput = %d, want 500", stats.TotalTokensInput)
	}
	if stats.TotalTokensOutput != 250 {
		t.Errorf("UsageStats.TotalTokensOutput = %d, want 250", stats.TotalTokensOutput)
	}
	// Note: Floating point comparison with some tolerance
	if stats.TotalCostUSD < 0.049 || stats.TotalCostUSD > 0.051 {
		t.Errorf("UsageStats.TotalCostUSD = %f, want ~0.05", stats.TotalCostUSD)
	}

	// Check per-provider stats
	if len(stats.ByProvider) != 2 {
		t.Errorf("UsageStats.ByProvider has %d providers, want 2", len(stats.ByProvider))
	}
	if openai, ok := stats.ByProvider["openai"]; ok {
		if openai.TotalRequests != 2 {
			t.Errorf("OpenAI TotalRequests = %d, want 2", openai.TotalRequests)
		}
	} else {
		t.Error("UsageStats.ByProvider missing 'openai'")
	}
}

func TestAuditLogger_GetToolCallStats(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create tool call events
	events := []EventType{
		EventToolCallRequested,
		EventToolCallRequested,
		EventToolCallRequested,
		EventToolCallApproved,
		EventToolCallApproved,
		EventToolCallRejected,
		EventToolCallAutoApproved,
		EventToolCallCompleted,
		EventToolCallCompleted,
		EventToolCallFailed,
	}

	for _, et := range events {
		builder, _ := NewEntry(et, "user", "user")
		entry, _ := builder.Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	stats, err := logger.GetToolCallStats(ctx, &QueryFilter{})
	if err != nil {
		t.Fatalf("GetToolCallStats() error = %v", err)
	}

	if stats.TotalRequested != 3 {
		t.Errorf("ToolCallStats.TotalRequested = %d, want 3", stats.TotalRequested)
	}
	if stats.TotalApproved != 2 {
		t.Errorf("ToolCallStats.TotalApproved = %d, want 2", stats.TotalApproved)
	}
	if stats.TotalRejected != 1 {
		t.Errorf("ToolCallStats.TotalRejected = %d, want 1", stats.TotalRejected)
	}
	if stats.TotalAutoApproved != 1 {
		t.Errorf("ToolCallStats.TotalAutoApproved = %d, want 1", stats.TotalAutoApproved)
	}
	if stats.TotalCompleted != 2 {
		t.Errorf("ToolCallStats.TotalCompleted = %d, want 2", stats.TotalCompleted)
	}
	if stats.TotalFailed != 1 {
		t.Errorf("ToolCallStats.TotalFailed = %d, want 1", stats.TotalFailed)
	}
}

func TestAuditLogger_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	// Close the logger
	if err := logger.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Double close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("Close() second call error = %v", err)
	}

	// Operations should fail after close
	ctx := context.Background()
	err = logger.LogEvent(ctx, EventSessionCreated, "user", "user")
	if err == nil {
		t.Error("LogEvent() should fail after Close()")
	}
}

func TestAuditLogger_Log_AllFields(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	metadata := map[string]interface{}{
		"tool_name": "read_file",
		"file_path": "/path/to/file.txt",
	}

	builder, _ := NewEntry(EventToolCallCompleted, "user-123", "user")
	entry, _ := builder.
		WithSeverity(SeverityDebug).
		WithSession("session-456").
		WithMessage("message-789").
		WithToolCall("toolcall-012").
		WithProject("/path/to/project").
		WithProvider("openai", "gpt-4").
		WithTokens(100, 50).
		WithCost(0.005).
		WithDuration(150).
		WithSuccess(true).
		WithIteration(3).
		WithClientInfo("127.0.0.1", "Mozilla/5.0").
		WithMetadata(metadata).
		Build()

	if err := logger.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	// Retrieve and verify all fields
	retrieved, err := logger.GetByID(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}

	if retrieved.Severity != SeverityDebug {
		t.Errorf("Severity = %v, want %v", retrieved.Severity, SeverityDebug)
	}
	if !retrieved.SessionID.Valid || retrieved.SessionID.String != "session-456" {
		t.Errorf("SessionID = %v, want session-456", retrieved.SessionID)
	}
	if !retrieved.MessageID.Valid || retrieved.MessageID.String != "message-789" {
		t.Errorf("MessageID = %v, want message-789", retrieved.MessageID)
	}
	if !retrieved.ToolCallID.Valid || retrieved.ToolCallID.String != "toolcall-012" {
		t.Errorf("ToolCallID = %v, want toolcall-012", retrieved.ToolCallID)
	}
	if !retrieved.Provider.Valid || retrieved.Provider.String != "openai" {
		t.Errorf("Provider = %v, want openai", retrieved.Provider)
	}
	if !retrieved.Model.Valid || retrieved.Model.String != "gpt-4" {
		t.Errorf("Model = %v, want gpt-4", retrieved.Model)
	}
	if !retrieved.TokensInput.Valid || retrieved.TokensInput.Int64 != 100 {
		t.Errorf("TokensInput = %v, want 100", retrieved.TokensInput)
	}
	if !retrieved.TokensOutput.Valid || retrieved.TokensOutput.Int64 != 50 {
		t.Errorf("TokensOutput = %v, want 50", retrieved.TokensOutput)
	}
	if !retrieved.CostUSD.Valid || retrieved.CostUSD.Float64 != 0.005 {
		t.Errorf("CostUSD = %v, want 0.005", retrieved.CostUSD)
	}
	if !retrieved.DurationMs.Valid || retrieved.DurationMs.Int64 != 150 {
		t.Errorf("DurationMs = %v, want 150", retrieved.DurationMs)
	}
	if !retrieved.Success.Valid || !retrieved.Success.Bool {
		t.Errorf("Success = %v, want true", retrieved.Success)
	}
	if !retrieved.Iteration.Valid || retrieved.Iteration.Int64 != 3 {
		t.Errorf("Iteration = %v, want 3", retrieved.Iteration)
	}
	if !retrieved.IPAddress.Valid || retrieved.IPAddress.String != "127.0.0.1" {
		t.Errorf("IPAddress = %v, want 127.0.0.1", retrieved.IPAddress)
	}
	if !retrieved.UserAgent.Valid || retrieved.UserAgent.String != "Mozilla/5.0" {
		t.Errorf("UserAgent = %v, want Mozilla/5.0", retrieved.UserAgent)
	}
	if !retrieved.Metadata.Valid {
		t.Error("Metadata should be valid")
	}

	// Verify metadata can be parsed
	var parsedMetadata map[string]interface{}
	if err := retrieved.GetMetadata(&parsedMetadata); err != nil {
		t.Errorf("GetMetadata() error = %v", err)
	}
	if parsedMetadata["tool_name"] != "read_file" {
		t.Errorf("Metadata tool_name = %v, want read_file", parsedMetadata["tool_name"])
	}
}

func TestAuditLogger_Query_SeverityFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create entries with different severities
	severities := []Severity{SeverityDebug, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical}
	for _, s := range severities {
		builder, _ := NewEntry(EventSystemError, "system", "system")
		entry, _ := builder.WithSeverity(s).Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	// Filter by severity
	result, err := logger.Query(ctx, &QueryFilter{
		Severities: []Severity{SeverityError, SeverityCritical},
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("Query(severity=error,critical) returned %d entries, want 2", len(result.Entries))
	}
}

func TestAuditLogger_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Run concurrent writes and reads
	done := make(chan bool)
	errCh := make(chan error, 20)

	// Writers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				builder, _ := NewEntry(EventSessionCreated, "user", "user")
				entry, _ := builder.Build()
				if err := logger.Log(ctx, entry); err != nil {
					errCh <- err
				}
			}
			done <- true
		}()
	}

	// Readers
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				_, err := logger.Query(ctx, &QueryFilter{Limit: 10})
				if err != nil {
					errCh <- err
				}
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 20; i++ {
		<-done
	}

	close(errCh)
	for err := range errCh {
		t.Errorf("Concurrent access error: %v", err)
	}
}

func TestAuditLogger_DatabasePersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	// Create logger and add entry
	logger1, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}

	ctx := context.Background()
	builder, _ := NewEntry(EventSessionCreated, "user-persistent", "user")
	entry, _ := builder.Build()
	entryID := entry.ID

	if err := logger1.Log(ctx, entry); err != nil {
		t.Fatalf("Log() error = %v", err)
	}
	logger1.Close()

	// Reopen database and verify entry exists
	logger2, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() second open error = %v", err)
	}
	defer logger2.Close()

	retrieved, err := logger2.GetByID(ctx, entryID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if retrieved.ActorID != "user-persistent" {
		t.Errorf("Retrieved ActorID = %v, want user-persistent", retrieved.ActorID)
	}
}

func TestAuditLogger_ProjectPathFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_audit.db")

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	ctx := context.Background()

	// Create entries for different projects
	projects := []string{"/project/a", "/project/b", "/project/a"}
	for _, p := range projects {
		builder, _ := NewEntry(EventSessionCreated, "user", "user")
		entry, _ := builder.WithProject(p).Build()
		if err := logger.Log(ctx, entry); err != nil {
			t.Fatalf("Log() error = %v", err)
		}
	}

	result, err := logger.Query(ctx, &QueryFilter{
		ProjectPath: "/project/a",
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("Query(project=/project/a) returned %d entries, want 2", len(result.Entries))
	}
}

// Ensure database file is created
func TestAuditLogger_DatabaseFileCreation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "test_audit.db")

	// Create subdirectory first
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	logger, err := NewLogger(dbPath)
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	// Verify file was created
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("Database file was not created")
	}
}
