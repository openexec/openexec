package router

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// =============================================================================
// API Surface Tests (T-US-001-002 Section 7.1)
// These tests verify the documented API contracts for Router and Intent.
// =============================================================================

// TestIntent_JSONSerialization verifies AC-1: Intent struct JSON serialization
// Contract: Intent must serialize to JSON with tool_name, args, confidence fields
func TestIntent_JSONSerialization(t *testing.T) {
	t.Run("Intent with all fields serializes correctly", func(t *testing.T) {
		// Arrange
		intent := &Intent{
			ToolName:   "deploy",
			Args:       map[string]interface{}{"env": "prod", "action": "push"},
			Confidence: 0.95,
		}

		// Act
		data, err := json.Marshal(intent)

		// Assert
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		// Verify JSON field names match documented contract
		jsonStr := string(data)
		if !strings.Contains(jsonStr, `"tool_name"`) {
			t.Error("JSON should contain 'tool_name' field (snake_case)")
		}
		if !strings.Contains(jsonStr, `"args"`) {
			t.Error("JSON should contain 'args' field")
		}
		if !strings.Contains(jsonStr, `"confidence"`) {
			t.Error("JSON should contain 'confidence' field")
		}

		// Verify round-trip
		var decoded Intent
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if decoded.ToolName != intent.ToolName {
			t.Errorf("ToolName mismatch: got %q, want %q", decoded.ToolName, intent.ToolName)
		}
		if decoded.Confidence != intent.Confidence {
			t.Errorf("Confidence mismatch: got %f, want %f", decoded.Confidence, intent.Confidence)
		}
	})

	t.Run("Intent with nested args serializes correctly", func(t *testing.T) {
		// Arrange
		intent := &Intent{
			ToolName: "complex_tool",
			Args: map[string]interface{}{
				"query":  "test query",
				"config": map[string]interface{}{"timeout": 30, "retry": true},
				"tags":   []interface{}{"alpha", "beta"},
			},
			Confidence: 0.8,
		}

		// Act
		data, err := json.Marshal(intent)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		// Assert: Verify round-trip preserves structure
		var decoded Intent
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		// Verify nested map
		config, ok := decoded.Args["config"].(map[string]interface{})
		if !ok {
			t.Fatal("Args['config'] should be a map")
		}
		if config["timeout"].(float64) != 30 {
			t.Errorf("config.timeout mismatch: got %v", config["timeout"])
		}

		// Verify array
		tags, ok := decoded.Args["tags"].([]interface{})
		if !ok {
			t.Fatal("Args['tags'] should be an array")
		}
		if len(tags) != 2 {
			t.Errorf("tags length mismatch: got %d, want 2", len(tags))
		}
	})

	t.Run("Fallback intent for general_chat has query arg", func(t *testing.T) {
		// Contract: For general_chat, Args must contain "query" key
		intent := &Intent{
			ToolName:   GeneralChatTool,
			Args:       map[string]interface{}{"query": "hello world"},
			Confidence: FallbackConfidence,
		}

		data, err := json.Marshal(intent)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		var decoded Intent
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}

		query, ok := decoded.Args["query"].(string)
		if !ok || query == "" {
			t.Error("general_chat intent must have non-empty 'query' arg")
		}
	})
}

// TestRouter_FallbackOnError verifies AC-2: Router returns fallback on error
// Contract: ParseIntent SHOULD return (fallback, nil) on errors, not (nil, error)
func TestRouter_FallbackOnError(t *testing.T) {
	ctx := context.Background()

	t.Run("Model unavailable returns fallback not error", func(t *testing.T) {
		// Arrange: Router with non-existent model
		r := NewBitNetRouter("/tmp/definitely-not-a-model.bin")
		// skipAvailabilityCheck is false, so CheckAvailability will fail

		// Act
		intent, err := r.ParseIntent(ctx, "test query")

		// Assert: Contract says SHOULD return (fallback, nil), not (nil, error)
		if err != nil {
			t.Fatalf("ParseIntent should return fallback, not error: %v", err)
		}
		if intent == nil {
			t.Fatal("ParseIntent should never return (nil, nil)")
		}
		if intent.ToolName != GeneralChatTool {
			t.Errorf("Fallback tool should be %q, got %q", GeneralChatTool, intent.ToolName)
		}
		if intent.Confidence < FallbackConfidence {
			t.Errorf("Fallback confidence should be >= %f, got %f", FallbackConfidence, intent.Confidence)
		}
	})

	t.Run("Inference error returns fallback not error", func(t *testing.T) {
		// This tests the path where runLocalInference returns an error
		// With skipAvailabilityCheck=false and missing model, we simulate inference failure
		r := NewBitNetRouter("/nonexistent/path/model.bin")

		intent, err := r.ParseIntent(ctx, "any query here")

		if err != nil {
			t.Fatalf("ParseIntent should fallback on inference error, not return error: %v", err)
		}
		if intent.ToolName != GeneralChatTool {
			t.Errorf("Should fallback to %q, got %q", GeneralChatTool, intent.ToolName)
		}
	})
}

// TestRouter_ConfidenceSemantics verifies documented confidence ranges
func TestRouter_ConfidenceSemantics(t *testing.T) {
	ctx := context.Background()

	t.Run("High confidence (0.9+) for surgical tool match", func(t *testing.T) {
		r := NewBitNetRouter("/mock/model")
		r.SetSkipAvailabilityCheck(true)
		r.RegisterTool("deploy", "Deploy to server", "{}")

		// Query with keyword that triggers deploy tool
		intent, err := r.ParseIntent(ctx, "please deploy to production now")

		if err != nil {
			t.Fatalf("ParseIntent failed: %v", err)
		}
		if intent.ToolName != "deploy" {
			t.Errorf("Expected deploy tool, got %q", intent.ToolName)
		}
		// Contract: 0.9+ for high confidence surgical match
		if intent.Confidence < 0.9 {
			t.Errorf("Surgical tool match should have confidence >= 0.9, got %f", intent.Confidence)
		}
	})

	t.Run("Medium confidence (0.5) for intentional fallback", func(t *testing.T) {
		r := NewBitNetRouter("/mock/model")
		r.SetSkipAvailabilityCheck(true)

		// Query that doesn't match any registered tool
		intent, err := r.ParseIntent(ctx, "random gibberish xyz123")

		if err != nil {
			t.Fatalf("ParseIntent failed: %v", err)
		}
		// Contract: 0.5 for intentional fallback
		if intent.Confidence != FallbackConfidence {
			t.Errorf("Fallback should have confidence exactly %f, got %f", FallbackConfidence, intent.Confidence)
		}
	})

	t.Run("LowConfidenceThreshold is 0.2", func(t *testing.T) {
		// Verify the documented constant
		if LowConfidenceThreshold != 0.2 {
			t.Errorf("LowConfidenceThreshold should be 0.2, got %f", LowConfidenceThreshold)
		}
	})

	t.Run("FallbackConfidence is 0.5", func(t *testing.T) {
		// Verify the documented constant
		if FallbackConfidence != 0.5 {
			t.Errorf("FallbackConfidence should be 0.5, got %f", FallbackConfidence)
		}
	})

	t.Run("GeneralChatTool is 'general_chat'", func(t *testing.T) {
		// Verify the documented constant
		if GeneralChatTool != "general_chat" {
			t.Errorf("GeneralChatTool should be 'general_chat', got %q", GeneralChatTool)
		}
	})
}

// TestRouter_RegisterTool verifies tool registration contract
func TestRouter_RegisterTool(t *testing.T) {
	t.Run("RegisterTool stores tool info", func(t *testing.T) {
		r := NewBitNetRouter("/mock/model")

		err := r.RegisterTool("test_tool", "Test description", `{"type": "object"}`)

		if err != nil {
			t.Fatalf("RegisterTool should not return error: %v", err)
		}

		// Verify tool is stored
		if _, ok := r.tools["test_tool"]; !ok {
			t.Error("Tool should be registered in tools map")
		}
	})

	t.Run("Multiple tools can be registered", func(t *testing.T) {
		r := NewBitNetRouter("/mock/model")

		r.RegisterTool("tool1", "First tool", "{}")
		r.RegisterTool("tool2", "Second tool", "{}")
		r.RegisterTool("tool3", "Third tool", "{}")

		if len(r.tools) != 3 {
			t.Errorf("Expected 3 tools registered, got %d", len(r.tools))
		}
	})
}

// =============================================================================
// Edge Case Tests (T-US-001-002 Section 7.3)
// =============================================================================

// TestRouter_EmptyQuery verifies handling of empty queries
func TestRouter_EmptyQuery(t *testing.T) {
	ctx := context.Background()
	r := NewBitNetRouter("/mock/model")
	r.SetSkipAvailabilityCheck(true)

	t.Run("Empty query falls back to general_chat", func(t *testing.T) {
		intent, err := r.ParseIntent(ctx, "")

		if err != nil {
			t.Fatalf("Empty query should not error: %v", err)
		}
		if intent.ToolName != GeneralChatTool {
			t.Errorf("Empty query should fall back to %q, got %q", GeneralChatTool, intent.ToolName)
		}
	})

	t.Run("Whitespace-only query falls back to general_chat", func(t *testing.T) {
		intent, err := r.ParseIntent(ctx, "   \t\n  ")

		if err != nil {
			t.Fatalf("Whitespace query should not error: %v", err)
		}
		if intent.ToolName != GeneralChatTool {
			t.Errorf("Whitespace query should fall back to %q, got %q", GeneralChatTool, intent.ToolName)
		}
	})
}

// TestRouter_VeryLongQuery verifies handling of very long queries
func TestRouter_VeryLongQuery(t *testing.T) {
	ctx := context.Background()
	r := NewBitNetRouter("/mock/model")
	r.SetSkipAvailabilityCheck(true)

	t.Run("10KB query is processed without error", func(t *testing.T) {
		// Create a 10KB query
		longQuery := strings.Repeat("a", 10*1024)

		intent, err := r.ParseIntent(ctx, longQuery)

		// Should not error - may be truncated by sanitizer but processed normally
		if err != nil {
			t.Fatalf("Long query should not cause error: %v", err)
		}
		if intent == nil {
			t.Fatal("Long query should return valid intent")
		}
	})

	t.Run("Long query with keywords still matches tool", func(t *testing.T) {
		r.RegisterTool("deploy", "Deploy to server", "{}")

		// Long query but contains deploy keyword
		longQuery := strings.Repeat("context ", 1000) + "please deploy to production"

		intent, err := r.ParseIntent(ctx, longQuery)

		if err != nil {
			t.Fatalf("Long query with keyword should not error: %v", err)
		}
		// The simulator should still find the keyword
		if intent.ToolName != "deploy" {
			t.Errorf("Long query with 'deploy' keyword should match deploy tool, got %q", intent.ToolName)
		}
	})
}

// TestRouter_ConcurrentQueries verifies thread-safe execution
func TestRouter_ConcurrentQueries(t *testing.T) {
	ctx := context.Background()
	r := NewBitNetRouter("/mock/model")
	r.SetSkipAvailabilityCheck(true)
	r.RegisterTool("deploy", "Deploy to server", "{}")
	r.RegisterTool("read_symbol", "Read code symbol", "{}")

	t.Run("Concurrent queries execute safely", func(t *testing.T) {
		const numGoroutines = 50
		const queriesPerGoroutine = 10

		var wg sync.WaitGroup
		errors := make(chan error, numGoroutines*queriesPerGoroutine)

		queries := []string{
			"deploy to production",
			"read symbol Execute",
			"random query",
			"what is the weather",
			"help me",
		}

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for j := 0; j < queriesPerGoroutine; j++ {
					query := queries[(workerID+j)%len(queries)]
					intent, err := r.ParseIntent(ctx, query)
					if err != nil {
						errors <- err
						return
					}
					if intent == nil {
						errors <- fmt.Errorf("nil intent for query %q", query)
						return
					}
				}
			}(i)
		}

		wg.Wait()
		close(errors)

		var errCount int
		for err := range errors {
			t.Errorf("Concurrent error: %v", err)
			errCount++
		}
		if errCount > 0 {
			t.Errorf("Got %d errors during concurrent execution", errCount)
		}
	})
}
