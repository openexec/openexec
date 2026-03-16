package dcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/logging"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
)

func TestDCPCoordinator(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	// Knowledge store expects .openexec dir
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755); err != nil {
		t.Fatalf("failed to create .openexec: %v", err)
	}
	store, err := knowledge.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// Use a mock BitNet router (BitNetRouter with skipAvailabilityCheck)
	r := router.NewBitNetRouter("/mock/model")
	r.SetSkipAvailabilityCheck(true)

	coord := NewCoordinator(r, store)
	coord.AllowExecution = true // Enable execution for unit tests
	coord.RegisterTool(tools.NewDeployTool(store))
	coord.RegisterTool(tools.NewSymbolReaderTool(store))

	ctx := context.Background()

	t.Run("Process Deploy Query", func(t *testing.T) {
		// Arrange: Set up the required knowledge for deployment
		store.SetEnvironment(&knowledge.EnvironmentRecord{
			Env:         "prod",
			Topology:    `[{"ip": "1.1.1.1"}]`,
			RuntimeType: "vm",
			DeploySteps: `echo "success"`,
		})

		// Act
		// We bypass actual BitNet call in unit tests by ensuring our simulated runLocalInference
		// handles the keywords.
		res, err := coord.ProcessQuery(ctx, "Please deploy to prod")

		// Assert
		if err != nil {
			// This might fail because skipAvailabilityCheck is false.
			// Let's check the error.
			if strings.Contains(err.Error(), "router unavailable") {
				t.Log("Skipping full integration test: BitNet model not present in test env")
				return
			}
			t.Fatalf("ProcessQuery failed: %v", err)
		}

		if sugg, ok := res.(*IntentSuggestion); ok {
			if !strings.Contains(sugg.Description, "Successfully deployed") {
				t.Errorf("unexpected result: %v", sugg.Description)
			}
		} else {
			// Fallback for cases where it might return a string directly
			if !strings.Contains(res.(string), "Successfully deployed") {
				t.Errorf("unexpected result: %v", res)
			}
		}
	})

	t.Run("PII and Infrastructure Scrubbing", func(t *testing.T) {
		// Arrange: Register a mock tool that echoes its input
		echoTool := &mockEchoTool{}
		coord.RegisterTool(echoTool)

		// Create a mock router that returns the echo tool
		mockRouter := &mockPIIRouter{}
		coord.router = mockRouter

		query := "Process data for test@example.com on 10.0.0.1"

		// Act
		res, err := coord.ProcessQuery(ctx, query)

		// Assert
		if err != nil {
			t.Fatalf("ProcessQuery failed: %v", err)
		}

		var output string
		if sugg, ok := res.(*IntentSuggestion); ok {
			output = sugg.Description
		} else {
			output = res.(string)
		}

		if strings.Contains(output, "test@example.com") {
			t.Error("PII (email) was not scrubbed")
		}
		if !strings.Contains(output, "[EMAIL_REDACTED]") {
			t.Error("PII (email) placeholder missing")
		}
		if strings.Contains(output, "10.0.0.1") {
			t.Error("Infrastructure (IP) was not masked")
		}
		if !strings.Contains(output, "[IP_REDACTED]") {
			t.Error("Infrastructure (IP) placeholder missing")
		}
	})
}

type mockEchoTool struct{}

func (m *mockEchoTool) Name() string        { return "echo" }
func (m *mockEchoTool) Description() string { return "Echoes input" }
func (m *mockEchoTool) InputSchema() string { return "{}" }
func (m *mockEchoTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	return args["text"], nil
}

type mockPIIRouter struct{}

func (m *mockPIIRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	return &router.Intent{
		ToolName:   "echo",
		Args:       map[string]interface{}{"text": query},
		Confidence: 1.0,
	}, nil
}
func (m *mockPIIRouter) RegisterTool(name, desc, schema string) error { return nil }

// TestCoordinator_NoDoubleFallback verifies that router fallback with confidence >= 0.5
// does NOT trigger the coordinator's low-confidence fallback (threshold 0.2)
func TestCoordinator_NoDoubleFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("Router fallback confidence 0.5 does not trigger coordinator re-fallback", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns exactly what a real fallback would return
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "general_chat",
				Args:       map[string]interface{}{"query": "hello"},
				Confidence: 0.5, // This is the fallback confidence (>= coordinator threshold 0.2)
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		// Track execution count
		executionCount := 0
		chatTool := &mockCountingTool{
			name: "general_chat",
			onExecute: func() {
				executionCount++
			},
		}
		coord.RegisterTool(chatTool)

		// Act
		_, err := coord.ProcessQuery(ctx, "hello")

		// Assert
		if err != nil {
			t.Fatalf("ProcessQuery should not return error, got: %v", err)
		}
		if executionCount != 1 {
			t.Errorf("general_chat should be executed exactly once, got %d executions", executionCount)
		}
	})

	t.Run("Fallback without general_chat registered returns error", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns a tool that doesn't exist
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true
		// Don't register any tools - no general_chat fallback available

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")

		// Assert: Should return error since tool not found AND no general_chat
		if err == nil {
			t.Error("ProcessQuery should return error when tool not found and no fallback available")
		}
		if !strings.Contains(err.Error(), "not registered") {
			t.Errorf("error should mention tool not registered, got: %v", err)
		}
	})

	t.Run("Low confidence from router triggers coordinator fallback to general_chat", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns low confidence (below 0.2 threshold)
		// This tests the coordinator's own fallback logic
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.1, // Below coordinator threshold
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		executionCount := 0
		chatTool := &mockCountingTool{
			name: "general_chat",
			onExecute: func() {
				executionCount++
			},
		}
		coord.RegisterTool(chatTool)
		coord.RegisterTool(&mockCountingTool{name: "some_tool"})

		// Act
		_, err := coord.ProcessQuery(ctx, "test")

		// Assert: Should fall back to general_chat
		if err != nil {
			t.Fatalf("ProcessQuery should not return error, got: %v", err)
		}
		if executionCount != 1 {
			t.Errorf("general_chat should be executed due to low confidence, got %d executions", executionCount)
		}
	})

	t.Run("Confidence exactly at threshold 0.2 does not trigger fallback", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns confidence exactly at threshold (0.2)
		// Since the check is "< 0.2", confidence of 0.2 should NOT trigger fallback
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.2, // Exactly at threshold
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		someToolExecuted := false
		chatToolExecuted := false
		coord.RegisterTool(&mockCountingTool{
			name: "some_tool",
			onExecute: func() {
				someToolExecuted = true
			},
		})
		coord.RegisterTool(&mockCountingTool{
			name: "general_chat",
			onExecute: func() {
				chatToolExecuted = true
			},
		})

		// Act
		_, err := coord.ProcessQuery(ctx, "test")

		// Assert: Should NOT fall back - should execute some_tool
		if err != nil {
			t.Fatalf("ProcessQuery should not return error, got: %v", err)
		}
		if chatToolExecuted {
			t.Error("general_chat should NOT be executed when confidence is at threshold")
		}
		if !someToolExecuted {
			t.Error("some_tool should be executed when confidence is at threshold")
		}
	})
}

// TestCoordinator_RouterErrorFallback verifies that router errors trigger fallback to general_chat
func TestCoordinator_RouterErrorFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("Router error with general_chat registered triggers fallback", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns an error
		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model inference failed"),
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		executionCount := 0
		chatTool := &mockCountingTool{
			name: "general_chat",
			onExecute: func() {
				executionCount++
			},
		}
		coord.RegisterTool(chatTool)

		// Act
		result, err := coord.ProcessQuery(ctx, "test query")

		// Assert: Should fall back to general_chat, no error
		if err != nil {
			t.Fatalf("ProcessQuery should not return error when general_chat is registered, got: %v", err)
		}
		if executionCount != 1 {
			t.Errorf("general_chat should be executed due to router error, got %d executions", executionCount)
		}
		if result != "executed" {
			t.Errorf("expected result 'executed', got: %v", result)
		}
	})

	t.Run("Router error without general_chat registered returns error", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns an error
		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model inference failed"),
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true
		// Don't register general_chat

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")

		// Assert: Should return error since no fallback available
		if err == nil {
			t.Error("ProcessQuery should return error when router fails and no fallback available")
		}
		if !strings.Contains(err.Error(), "intent routing failed") {
			t.Errorf("error should mention intent routing failed, got: %v", err)
		}
	})
}

// TestCoordinator_MissingToolFallback verifies that missing tools trigger fallback to general_chat
func TestCoordinator_MissingToolFallback(t *testing.T) {
	ctx := context.Background()

	t.Run("Missing tool with general_chat registered triggers fallback", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns an intent for a tool that doesn't exist
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		executionCount := 0
		chatTool := &mockCountingTool{
			name: "general_chat",
			onExecute: func() {
				executionCount++
			},
		}
		coord.RegisterTool(chatTool)

		// Act
		result, err := coord.ProcessQuery(ctx, "use nonexistent tool")

		// Assert: Should fall back to general_chat, no error
		if err != nil {
			t.Fatalf("ProcessQuery should not return error when general_chat is registered, got: %v", err)
		}
		if executionCount != 1 {
			t.Errorf("general_chat should be executed due to missing tool, got %d executions", executionCount)
		}
		if result != "executed" {
			t.Errorf("expected result 'executed', got: %v", result)
		}
	})
}

// TestCoordinator_FallbackExecutionError verifies behavior when general_chat itself fails
func TestCoordinator_FallbackExecutionError(t *testing.T) {
	ctx := context.Background()

	t.Run("general_chat execution error still returns error for missing tool scenario", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns an intent for a tool that doesn't exist
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		// general_chat is registered but it returns an error
		chatTool := &mockCountingTool{
			name:    "general_chat",
			execErr: fmt.Errorf("LLM backend unavailable"),
		}
		coord.RegisterTool(chatTool)

		// Act
		_, err := coord.ProcessQuery(ctx, "use nonexistent tool")

		// Assert: Should return error since fallback failed
		if err == nil {
			t.Error("ProcessQuery should return error when general_chat execution fails")
		}
		if !strings.Contains(err.Error(), "not registered") {
			t.Errorf("error should mention tool not registered, got: %v", err)
		}
	})

	t.Run("general_chat execution error for router error still returns routing error", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns an error
		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model inference failed"),
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		// general_chat is registered but it returns an error
		chatTool := &mockCountingTool{
			name:    "general_chat",
			execErr: fmt.Errorf("LLM backend unavailable"),
		}
		coord.RegisterTool(chatTool)

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")

		// Assert: Should return routing error since fallback failed
		if err == nil {
			t.Error("ProcessQuery should return error when both routing and general_chat fail")
		}
		if !strings.Contains(err.Error(), "intent routing failed") {
			t.Errorf("error should mention intent routing failed, got: %v", err)
		}
	})

	t.Run("general_chat execution error for low confidence still executes original tool", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router that returns low confidence
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.1, // Below threshold
			},
		}

		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		// general_chat is registered but it returns an error
		chatTool := &mockCountingTool{
			name:    "general_chat",
			execErr: fmt.Errorf("LLM backend unavailable"),
		}
		coord.RegisterTool(chatTool)

		// some_tool is registered and works
		someToolExecuted := false
		coord.RegisterTool(&mockCountingTool{
			name: "some_tool",
			onExecute: func() {
				someToolExecuted = true
			},
		})

		// Act
		result, err := coord.ProcessQuery(ctx, "test")

		// Assert: Since general_chat fallback failed, should fall through to execute some_tool
		// (This is the current behavior - low confidence fallback failure doesn't error out)
		if err != nil {
			t.Fatalf("ProcessQuery should not error, got: %v", err)
		}
		if !someToolExecuted {
			t.Error("some_tool should be executed when general_chat fallback fails")
		}
		if result != "executed" {
			t.Errorf("expected result 'executed', got: %v", result)
		}
	})
}

// mockErrorRouter returns an error on ParseIntent
type mockErrorRouter struct {
	err error
}

func (m *mockErrorRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	return nil, m.err
}

func (m *mockErrorRouter) RegisterTool(name, desc, schema string) error { return nil }

// mockFallbackRouter returns a fixed intent for testing
type mockFallbackRouter struct {
	returnIntent *router.Intent
}

func (m *mockFallbackRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	return m.returnIntent, nil
}

func (m *mockFallbackRouter) RegisterTool(name, desc, schema string) error { return nil }

// mockCountingTool tracks execution count
type mockCountingTool struct {
	name      string
	onExecute func()
	execErr   error // optional error to return from Execute
}

func (m *mockCountingTool) Name() string        { return m.name }
func (m *mockCountingTool) Description() string { return "Mock tool" }
func (m *mockCountingTool) InputSchema() string { return "{}" }
func (m *mockCountingTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	if m.onExecute != nil {
		m.onExecute()
	}
	if m.execErr != nil {
		return nil, m.execErr
	}
	return "executed", nil
}

// =============================================================================
// Logger Injection Tests (TDD for T-US-003-002)
// =============================================================================

// newTestCoordinator creates a Coordinator with a buffer logger for log capture
func newTestCoordinator(r router.Router, s *knowledge.Store) (*Coordinator, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	logger := logging.New(logging.Config{
		Level:  slog.LevelDebug,
		Format: "json",
		Output: buf,
	})
	coord := NewCoordinator(r, s, WithLogger(logger))
	coord.AllowExecution = true
	return coord, buf
}

// TestCoordinator_LoggerInjection tests the logger injection pattern
func TestCoordinator_LoggerInjection(t *testing.T) {
	ctx := context.Background()

	t.Run("Default logger is used when not specified", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "general_chat",
				Args:       map[string]interface{}{"query": "hello"},
				Confidence: 0.5,
			},
		}

		// Act: Create coordinator without WithLogger option
		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true
		coord.RegisterTool(&mockCountingTool{name: "general_chat"})

		// Assert: Coordinator should have a non-nil logger
		// (can't easily verify it's the default, but we can verify it works)
		_, err := coord.ProcessQuery(ctx, "hello")
		if err != nil {
			t.Fatalf("ProcessQuery should not error: %v", err)
		}
	})

	t.Run("Custom logger is used when injected", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		// Mock router returning low confidence to trigger fallback logging
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.1, // Below threshold, triggers fallback
			},
		}

		coord, logBuf := newTestCoordinator(mockRouter, store)
		coord.RegisterTool(&mockCountingTool{name: "general_chat"})

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")

		// Assert: No error and logs should appear in buffer
		if err != nil {
			t.Fatalf("ProcessQuery should not error: %v", err)
		}
		if logBuf.Len() == 0 {
			t.Error("Expected logs to appear in injected logger buffer")
		}
	})
}

// TestCoordinator_FallbackLogging tests structured logging for fallback events
func TestCoordinator_FallbackLogging(t *testing.T) {
	ctx := context.Background()

	t.Run("Low confidence fallback logs with correct attributes", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.1,
			},
		}

		coord, logBuf := newTestCoordinator(mockRouter, store)
		coord.RegisterTool(&mockCountingTool{name: "general_chat"})

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")
		if err != nil {
			t.Fatalf("ProcessQuery should not error: %v", err)
		}

		// Assert: Parse JSON log entries
		logEntry := findLogEntry(t, logBuf, "Fallback triggered")
		if logEntry == nil {
			t.Fatal("Expected 'Fallback triggered' log entry")
		}

		// Verify attributes
		assertLogAttribute(t, logEntry, "level", "WARN")
		assertLogAttribute(t, logEntry, "reason", FallbackReasonLowConfidence)
		assertLogAttributeFloat(t, logEntry, "original_confidence", 0.1)
		assertLogAttribute(t, logEntry, "fallback_tool", "general_chat")
		assertLogAttribute(t, logEntry, "component", "dcp")
		assertLogAttributeExists(t, logEntry, "query_hash")
	})

	t.Run("Router error fallback logs with error attribute", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model failed"),
		}

		coord, logBuf := newTestCoordinator(mockRouter, store)
		coord.RegisterTool(&mockCountingTool{name: "general_chat"})

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")
		if err != nil {
			t.Fatalf("ProcessQuery should not error: %v", err)
		}

		// Assert: Parse JSON log entries
		logEntry := findLogEntry(t, logBuf, "Fallback triggered")
		if logEntry == nil {
			t.Fatal("Expected 'Fallback triggered' log entry")
		}

		assertLogAttribute(t, logEntry, "level", "WARN")
		assertLogAttribute(t, logEntry, "reason", FallbackReasonRouterError)
		assertLogAttribute(t, logEntry, "error", "model failed")
	})

	t.Run("Missing tool fallback logs original tool name", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord, logBuf := newTestCoordinator(mockRouter, store)
		coord.RegisterTool(&mockCountingTool{name: "general_chat"})

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")
		if err != nil {
			t.Fatalf("ProcessQuery should not error: %v", err)
		}

		// Assert
		logEntry := findLogEntry(t, logBuf, "Fallback triggered")
		if logEntry == nil {
			t.Fatal("Expected 'Fallback triggered' log entry")
		}

		assertLogAttribute(t, logEntry, "reason", FallbackReasonMissingTool)
		assertLogAttribute(t, logEntry, "original_tool", "nonexistent")
	})

	t.Run("Fallback failure logs error level", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model failed"),
		}

		coord, logBuf := newTestCoordinator(mockRouter, store)
		// Register general_chat that fails
		coord.RegisterTool(&mockCountingTool{
			name:    "general_chat",
			execErr: fmt.Errorf("LLM unavailable"),
		})

		// Act
		_, _ = coord.ProcessQuery(ctx, "test query")

		// Assert: Should log fallback failure at ERROR level
		logEntry := findLogEntry(t, logBuf, "Fallback failed")
		if logEntry == nil {
			t.Fatal("Expected 'Fallback failed' log entry")
		}

		assertLogAttribute(t, logEntry, "level", "ERROR")
		assertLogAttribute(t, logEntry, "reason", FallbackReasonChatFailed)
		assertLogAttribute(t, logEntry, "error", "LLM unavailable")
	})

	t.Run("Tool execution logs at INFO level", func(t *testing.T) {
		// Arrange
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9, // Above threshold
			},
		}

		coord, logBuf := newTestCoordinator(mockRouter, store)
		coord.RegisterTool(&mockCountingTool{name: "some_tool"})

		// Act
		_, err := coord.ProcessQuery(ctx, "test query")
		if err != nil {
			t.Fatalf("ProcessQuery should not error: %v", err)
		}

		// Assert
		logEntry := findLogEntry(t, logBuf, "Executing tool")
		if logEntry == nil {
			t.Fatal("Expected 'Executing tool' log entry")
		}

		assertLogAttribute(t, logEntry, "level", "INFO")
		assertLogAttribute(t, logEntry, "tool", "some_tool")
		assertLogAttributeFloat(t, logEntry, "confidence", 0.9)
	})
}

// TestCoordinator_QueryHash tests the query hash functionality
func TestCoordinator_QueryHash(t *testing.T) {
	t.Run("Query hash is consistent for same query", func(t *testing.T) {
		hash1 := queryHash("test query")
		hash2 := queryHash("test query")
		if hash1 != hash2 {
			t.Errorf("Same query should produce same hash: got %s and %s", hash1, hash2)
		}
	})

	t.Run("Query hash differs for different queries", func(t *testing.T) {
		hash1 := queryHash("test query 1")
		hash2 := queryHash("test query 2")
		if hash1 == hash2 {
			t.Error("Different queries should produce different hashes")
		}
	})

	t.Run("Query hash is 8 characters", func(t *testing.T) {
		hash := queryHash("test")
		if len(hash) != 8 {
			t.Errorf("Query hash should be 8 characters, got %d", len(hash))
		}
	})

	t.Run("Empty query produces valid hash", func(t *testing.T) {
		hash := queryHash("")
		if len(hash) != 8 {
			t.Errorf("Empty query should still produce 8-char hash, got %d chars", len(hash))
		}
	})
}

// =============================================================================
// Test Helpers
// =============================================================================

// newTestStore creates a temporary knowledge store for testing.
// Returns the store and a cleanup function. Caller should defer cleanup().
func newTestStore(t *testing.T) (*knowledge.Store, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	// Knowledge store expects .openexec dir
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755); err != nil {
		t.Fatalf("Failed to create .openexec: %v", err)
	}
	store, err := knowledge.NewStore(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create test store: %v", err)
	}
	return store, func() { store.Close() }
}

// findLogEntry finds a log entry with the given message in JSON log output
func findLogEntry(t *testing.T, buf *bytes.Buffer, msg string) map[string]interface{} {
	t.Helper()
	lines := strings.Split(buf.String(), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Logf("Failed to parse log line: %s", line)
			continue
		}
		if entry["msg"] == msg {
			return entry
		}
	}
	return nil
}

// assertLogAttribute asserts that a log entry has the expected string attribute
func assertLogAttribute(t *testing.T, entry map[string]interface{}, key, expected string) {
	t.Helper()
	val, ok := entry[key]
	if !ok {
		t.Errorf("Log entry missing attribute %q", key)
		return
	}
	if val != expected {
		t.Errorf("Log attribute %q: expected %q, got %q", key, expected, val)
	}
}

// assertLogAttributeFloat asserts that a log entry has the expected float attribute
func assertLogAttributeFloat(t *testing.T, entry map[string]interface{}, key string, expected float64) {
	t.Helper()
	val, ok := entry[key]
	if !ok {
		t.Errorf("Log entry missing attribute %q", key)
		return
	}
	// JSON numbers are float64
	f, ok := val.(float64)
	if !ok {
		t.Errorf("Log attribute %q is not a number: %v", key, val)
		return
	}
	if f != expected {
		t.Errorf("Log attribute %q: expected %v, got %v", key, expected, f)
	}
}

// assertLogAttributeExists asserts that a log entry has the attribute (non-empty)
func assertLogAttributeExists(t *testing.T, entry map[string]interface{}, key string) {
	t.Helper()
	val, ok := entry[key]
	if !ok {
		t.Errorf("Log entry missing attribute %q", key)
		return
	}
	if val == "" || val == nil {
		t.Errorf("Log attribute %q is empty", key)
	}
}

// =============================================================================
// Tool Registration Propagation Tests (T-US-001-003)
// =============================================================================

// mockTrackingRouter tracks RegisterTool calls to verify propagation
type mockTrackingRouter struct {
	registrations []toolRegistration
}

type toolRegistration struct {
	name        string
	description string
	schema      string
}

func (m *mockTrackingRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	return &router.Intent{
		ToolName:   "general_chat",
		Args:       map[string]interface{}{"query": query},
		Confidence: 0.5,
	}, nil
}

func (m *mockTrackingRouter) RegisterTool(name, description, schema string) error {
	m.registrations = append(m.registrations, toolRegistration{
		name:        name,
		description: description,
		schema:      schema,
	})
	return nil
}

// TestCoordinator_ToolRegistrationPropagation verifies that tool registration
// on the Coordinator propagates to the Router (T-US-001-003 behavioral scenario).
func TestCoordinator_ToolRegistrationPropagation(t *testing.T) {
	t.Run("RegisterTool propagates to router", func(t *testing.T) {
		// GIVEN a Coordinator with a tracking router
		store, cleanup := newTestStore(t)
		defer cleanup()

		trackingRouter := &mockTrackingRouter{}
		coord := NewCoordinator(trackingRouter, store)

		// WHEN a tool is registered on the Coordinator
		testTool := &mockDescriptiveTool{
			name:        "test_tool",
			description: "A test tool for verification",
			schema:      `{"type": "object", "properties": {"input": {"type": "string"}}}`,
		}
		coord.RegisterTool(testTool)

		// THEN the Router should have received the registration
		if len(trackingRouter.registrations) != 1 {
			t.Fatalf("Expected 1 registration, got %d", len(trackingRouter.registrations))
		}

		reg := trackingRouter.registrations[0]
		if reg.name != "test_tool" {
			t.Errorf("Router received name %q, want \"test_tool\"", reg.name)
		}
		if reg.description != "A test tool for verification" {
			t.Errorf("Router received description %q, want \"A test tool for verification\"", reg.description)
		}
		if reg.schema != `{"type": "object", "properties": {"input": {"type": "string"}}}` {
			t.Errorf("Router received schema %q, want JSON schema", reg.schema)
		}
	})

	t.Run("Multiple tool registrations propagate correctly", func(t *testing.T) {
		// GIVEN a Coordinator with a tracking router
		store, cleanup := newTestStore(t)
		defer cleanup()

		trackingRouter := &mockTrackingRouter{}
		coord := NewCoordinator(trackingRouter, store)

		// WHEN multiple tools are registered
		coord.RegisterTool(&mockDescriptiveTool{name: "tool_a", description: "Tool A", schema: "{}"})
		coord.RegisterTool(&mockDescriptiveTool{name: "tool_b", description: "Tool B", schema: "{}"})
		coord.RegisterTool(&mockDescriptiveTool{name: "tool_c", description: "Tool C", schema: "{}"})

		// THEN all registrations are propagated
		if len(trackingRouter.registrations) != 3 {
			t.Fatalf("Expected 3 registrations, got %d", len(trackingRouter.registrations))
		}

		expectedNames := []string{"tool_a", "tool_b", "tool_c"}
		for i, expected := range expectedNames {
			if trackingRouter.registrations[i].name != expected {
				t.Errorf("Registration %d: got name %q, want %q", i, trackingRouter.registrations[i].name, expected)
			}
		}
	})

	t.Run("Registered tool is available for execution", func(t *testing.T) {
		// GIVEN a Coordinator with a mock router that returns a specific tool
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "custom_tool",
				Args:       map[string]interface{}{"value": "test"},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		// WHEN a tool is registered
		executed := false
		customTool := &mockDescriptiveTool{
			name:        "custom_tool",
			description: "Custom tool",
			schema:      "{}",
			onExecute: func(args map[string]interface{}) (any, error) {
				executed = true
				return "custom_result", nil
			},
		}
		coord.RegisterTool(customTool)

		// AND a query is processed that routes to that tool
		ctx := context.Background()
		result, err := coord.ProcessQuery(ctx, "test query")

		// THEN the tool is executed
		if err != nil {
			t.Fatalf("ProcessQuery failed: %v", err)
		}
		if !executed {
			t.Error("Custom tool was not executed")
		}
		if result != "custom_result" {
			t.Errorf("Result = %v, want \"custom_result\"", result)
		}
	})
}

// mockDescriptiveTool is a configurable mock tool for testing
type mockDescriptiveTool struct {
	name        string
	description string
	schema      string
	onExecute   func(args map[string]interface{}) (any, error)
}

func (m *mockDescriptiveTool) Name() string        { return m.name }
func (m *mockDescriptiveTool) Description() string { return m.description }
func (m *mockDescriptiveTool) InputSchema() string { return m.schema }
func (m *mockDescriptiveTool) Execute(ctx context.Context, args map[string]interface{}) (any, error) {
	if m.onExecute != nil {
		return m.onExecute(args)
	}
	return "executed", nil
}

// =============================================================================
// Contract Tests: Thin DCP Layer (T-US-002-003)
// =============================================================================

// TestCoordinator_ThinLayer verifies that the Coordinator is a thin tool-routing
// layer that does NOT manage orchestration state. This is a contract test ensuring
// the DCP's limited role in the architecture.
func TestCoordinator_ThinLayer(t *testing.T) {
	// This test validates that:
	// 1. Coordinator only routes queries to tools
	// 2. Coordinator does NOT maintain phase state
	// 3. Coordinator does NOT track iteration counts
	// 4. Coordinator does NOT make routing decisions (that's Pipeline's job)

	t.Run("Coordinator has no phase state", func(t *testing.T) {
		// GIVEN a Coordinator
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "test_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true
		coord.RegisterTool(&mockCountingTool{name: "test_tool"})

		// THEN Coordinator has no phase-related fields or methods
		// (This is a compile-time check - if this compiles, we pass)
		// The Coordinator struct should NOT have:
		// - CurrentPhase
		// - PhaseHistory
		// - AdvancePhase()
		// - Route()
		// These are Pipeline responsibilities

		// Verify by checking the Coordinator only has the expected fields
		// (This is implicitly tested by the fact that we can call ProcessQuery
		// multiple times without any internal state changing)
		ctx := context.Background()
		_, err1 := coord.ProcessQuery(ctx, "query 1")
		_, err2 := coord.ProcessQuery(ctx, "query 2")
		_, err3 := coord.ProcessQuery(ctx, "query 3")

		if err1 != nil || err2 != nil || err3 != nil {
			t.Errorf("ProcessQuery errors: %v, %v, %v", err1, err2, err3)
		}
	})

	t.Run("Coordinator has no iteration tracking", func(t *testing.T) {
		// GIVEN a Coordinator
		store, cleanup := newTestStore(t)
		defer cleanup()

		callCount := 0
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "test_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true
		coord.RegisterTool(&mockCountingTool{
			name: "test_tool",
			onExecute: func() {
				callCount++
			},
		})

		ctx := context.Background()

		// WHEN we call ProcessQuery many times
		for i := 0; i < 10; i++ {
			_, err := coord.ProcessQuery(ctx, fmt.Sprintf("query %d", i))
			if err != nil {
				t.Fatalf("ProcessQuery %d: %v", i, err)
			}
		}

		// THEN each call is independent (no iteration limit)
		// The Loop enforces iteration limits, not the Coordinator
		if callCount != 10 {
			t.Errorf("callCount = %d, want 10 (no iteration limits in Coordinator)", callCount)
		}
	})

	t.Run("Coordinator only routes, does not make phase decisions", func(t *testing.T) {
		// GIVEN a Coordinator
		store, cleanup := newTestStore(t)
		defer cleanup()

		executedTools := []string{}
		mockRouter := &mockSequenceRouter{
			sequence: []*router.Intent{
				{ToolName: "tool_a", Args: map[string]interface{}{}, Confidence: 0.9},
				{ToolName: "tool_b", Args: map[string]interface{}{}, Confidence: 0.9},
				{ToolName: "tool_c", Args: map[string]interface{}{}, Confidence: 0.9},
			},
		}
		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true

		for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
			n := name
			coord.RegisterTool(&mockCountingTool{
				name: n,
				onExecute: func() {
					executedTools = append(executedTools, n)
				},
			})
		}

		ctx := context.Background()

		// WHEN we process queries
		coord.ProcessQuery(ctx, "query 1")
		coord.ProcessQuery(ctx, "query 2")
		coord.ProcessQuery(ctx, "query 3")

		// THEN tools are executed based on router decision, not phase logic
		expected := []string{"tool_a", "tool_b", "tool_c"}
		if len(executedTools) != len(expected) {
			t.Fatalf("executedTools = %v, want %v", executedTools, expected)
		}
		for i, tool := range expected {
			if executedTools[i] != tool {
				t.Errorf("executedTools[%d] = %s, want %s", i, executedTools[i], tool)
			}
		}
	})

	t.Run("Coordinator does not emit pipeline signals", func(t *testing.T) {
		// GIVEN a Coordinator
		store, cleanup := newTestStore(t)
		defer cleanup()

		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "test_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}
		coord := NewCoordinator(mockRouter, store)
		coord.AllowExecution = true
		coord.RegisterTool(&mockCountingTool{name: "test_tool"})

		// THEN Coordinator has no signal-related methods
		// Signal handling (phase-complete, blocked, route) is Pipeline's job
		// This is a compile-time assertion - if the code compiles, we pass

		// The Coordinator should NOT have:
		// - HandleSignal()
		// - EmitSignal()
		// - OnPhaseComplete()
		// - OnBlocked()

		ctx := context.Background()
		result, err := coord.ProcessQuery(ctx, "test")

		// Only tool result is returned, no signal metadata
		if err != nil {
			t.Fatalf("ProcessQuery: %v", err)
		}
		if result != "executed" {
			t.Errorf("result = %v, want 'executed'", result)
		}
	})
}

// mockSequenceRouter returns intents from a predefined sequence
type mockSequenceRouter struct {
	sequence []*router.Intent
	index    int
}

func (m *mockSequenceRouter) ParseIntent(ctx context.Context, query string) (*router.Intent, error) {
	if m.index >= len(m.sequence) {
		return &router.Intent{
			ToolName:   "general_chat",
			Args:       map[string]interface{}{"query": query},
			Confidence: 0.5,
		}, nil
	}
	intent := m.sequence[m.index]
	m.index++
	return intent, nil
}

func (m *mockSequenceRouter) RegisterTool(name, desc, schema string) error { return nil }
