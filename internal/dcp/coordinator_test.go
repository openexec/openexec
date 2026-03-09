package dcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
)

func TestDCPCoordinator(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	store, _ := knowledge.NewStore(tmpDir)
	defer store.Close()

	// Use a mock BitNet router (BitNetRouter with skipAvailabilityCheck)
	r := router.NewBitNetRouter("/mock/model")
	r.SetSkipAvailabilityCheck(true)

	coord := NewCoordinator(r, store)
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

		if !strings.Contains(res.(string), "Successfully deployed") {
			t.Errorf("unexpected result: %v", res)
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

		output := res.(string)
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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns exactly what a real fallback would return
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "general_chat",
				Args:       map[string]interface{}{"query": "hello"},
				Confidence: 0.5, // This is the fallback confidence (>= coordinator threshold 0.2)
			},
		}

		coord := NewCoordinator(mockRouter, store)

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns a tool that doesn't exist
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord := NewCoordinator(mockRouter, store)
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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns an error
		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model inference failed"),
		}

		coord := NewCoordinator(mockRouter, store)

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns an error
		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model inference failed"),
		}

		coord := NewCoordinator(mockRouter, store)
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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns an intent for a tool that doesn't exist
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord := NewCoordinator(mockRouter, store)

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns an intent for a tool that doesn't exist
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "nonexistent_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.9,
			},
		}

		coord := NewCoordinator(mockRouter, store)

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns an error
		mockRouter := &mockErrorRouter{
			err: fmt.Errorf("model inference failed"),
		}

		coord := NewCoordinator(mockRouter, store)

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
		tmpDir := t.TempDir()
		store, _ := knowledge.NewStore(tmpDir)
		defer store.Close()

		// Mock router that returns low confidence
		mockRouter := &mockFallbackRouter{
			returnIntent: &router.Intent{
				ToolName:   "some_tool",
				Args:       map[string]interface{}{},
				Confidence: 0.1, // Below threshold
			},
		}

		coord := NewCoordinator(mockRouter, store)

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
