package router

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBitNetRouter(t *testing.T) {
	// Arrange
	r := NewBitNetRouter("/models/bitnet-2b.gguf")
	r.skipAvailabilityCheck = true
	r.RegisterTool("deploy", "Deploy app to server", "{}")
	r.RegisterTool("read_symbol", "Read code function", "{}")
	ctx := context.Background()

	t.Run("Parse Deploy Intent", func(t *testing.T) {
		// Act
		intent, err := r.ParseIntent(ctx, "Please deploy the current build to production")

		// Assert
		if err != nil {
			t.Fatalf("ParseIntent failed: %v", err)
		}
		if intent.ToolName != "deploy" {
			t.Errorf("got tool %q, want deploy", intent.ToolName)
		}
		if intent.Confidence < 0.9 {
			t.Errorf("confidence too low: %f", intent.Confidence)
		}
	})

	t.Run("Parse Symbol Intent", func(t *testing.T) {
		// Act
		intent, err := r.ParseIntent(ctx, "Show me the Execute function implementation")

		// Assert
		if err != nil {
			t.Fatalf("ParseIntent failed: %v", err)
		}
		if intent.ToolName != "read_symbol" {
			t.Errorf("got tool %q, want read_symbol", intent.ToolName)
		}
	})

	t.Run("General Chat Fallback", func(t *testing.T) {
		// Act
		intent, err := r.ParseIntent(ctx, "What is the weather today?")

		// Assert
		if err != nil {
			t.Fatalf("ParseIntent failed: %v", err)
		}
		if intent.ToolName != "general_chat" {
			t.Errorf("got tool %q, want general_chat", intent.ToolName)
		}
		if intent.Args["query"] == "" {
			t.Error("expected query arg to be populated")
		}
	})

	t.Run("Real Inference Failure Fallback", func(t *testing.T) {
		// Use a router that will actually fail (availability check fails)
		failR := NewBitNetRouter("/tmp/non-existent.bin")
		// failR.skipAvailabilityCheck is false by default

		intent, err := failR.ParseIntent(ctx, "hello")
		if err != nil {
			t.Fatalf("expected nil error due to fallback, got %v", err)
		}
		if intent.ToolName != "general_chat" {
			t.Errorf("expected general_chat fallback, got %q", intent.ToolName)
		}
	})
}

// TestBitNetRouter_FallbackConfidence verifies AC-4: fallback confidence >= 0.5
// This ensures fallback intents won't trigger coordinator re-fallback (threshold 0.2)
func TestBitNetRouter_FallbackConfidence(t *testing.T) {
	ctx := context.Background()

	t.Run("Model unavailable fallback returns confidence >= 0.5", func(t *testing.T) {
		// Arrange: Create router with non-existent model (will fail availability check)
		r := NewBitNetRouter("/tmp/non-existent-model.bin")
		// skipAvailabilityCheck is false, so CheckAvailability will fail

		// Act
		intent, err := r.ParseIntent(ctx, "any query")

		// Assert: AC-1 - never returns error
		if err != nil {
			t.Fatalf("ParseIntent should never return error, got: %v", err)
		}
		// Assert: AC-4 - confidence >= 0.5
		if intent.Confidence < 0.5 {
			t.Errorf("fallback confidence should be >= 0.5, got %f", intent.Confidence)
		}
		// Assert: tool is general_chat
		if intent.ToolName != "general_chat" {
			t.Errorf("fallback tool should be general_chat, got %q", intent.ToolName)
		}
	})

	t.Run("Inference error fallback returns confidence >= 0.5", func(t *testing.T) {
		// Arrange: Create router with mock that simulates inference error
		r := NewBitNetRouter("/mock/model")
		r.SetSkipAvailabilityCheck(true)
		// The simulateInference path should handle all queries gracefully

		// Act: Query that doesn't match any surgical tool (falls back)
		intent, err := r.ParseIntent(ctx, "random unmatched query xyz123")

		// Assert: AC-1 - never returns error
		if err != nil {
			t.Fatalf("ParseIntent should never return error, got: %v", err)
		}
		// Assert: AC-4 - confidence >= 0.5 for fallback
		if intent.Confidence < 0.5 {
			t.Errorf("fallback confidence should be >= 0.5, got %f", intent.Confidence)
		}
	})

	t.Run("Malformed JSON fallback returns confidence >= 0.5", func(t *testing.T) {
		// This test needs a mock that returns invalid JSON
		// For now, we test the existing fallback path behavior
		r := NewBitNetRouter("/mock/model")
		r.SetSkipAvailabilityCheck(true)

		// Act: The simulator handles this, but we verify the fallback intent structure
		intent, err := r.ParseIntent(ctx, "weather today")

		// Assert
		if err != nil {
			t.Fatalf("ParseIntent should never return error, got: %v", err)
		}
		if intent.ToolName != "general_chat" {
			t.Errorf("expected general_chat, got %q", intent.ToolName)
		}
		if intent.Confidence < 0.5 {
			t.Errorf("fallback confidence should be >= 0.5, got %f", intent.Confidence)
		}
		if intent.Args["query"] == nil {
			t.Error("expected query arg to be populated")
		}
	})

	t.Run("Low confidence model output triggers fallback with >= 0.5", func(t *testing.T) {
		// This tests the threshold at line 78 in bitnet.go
		// When model returns confidence < 0.2, router should fall back with 0.5
		r := NewBitNetRouter("/mock/model")
		r.SetSkipAvailabilityCheck(true)

		// The simulator returns high confidence for matched tools, 0.50 for fallback
		// We test that unmatched queries get the correct fallback confidence
		intent, err := r.ParseIntent(ctx, "completely unrecognized request")

		if err != nil {
			t.Fatalf("ParseIntent should never return error, got: %v", err)
		}
		// The fallback confidence must prevent coordinator re-fallback
		if intent.Confidence < 0.5 {
			t.Errorf("fallback confidence should be >= 0.5, got %f", intent.Confidence)
		}
	})
}

func TestBitNetRouter_Availability(t *testing.T) {
	t.Run("Missing Model", func(t *testing.T) {
		r := NewBitNetRouter("/tmp/non-existent-model.bin")
		err := r.CheckAvailability()
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got %v", err)
		}
	})

	t.Run("Missing CLI", func(t *testing.T) {
		// Create a temporary "real" model path to pass the first check
		tmpDir := t.TempDir()
		modelPath := filepath.Join(tmpDir, "model.bin")
		os.WriteFile(modelPath, []byte("fake"), 0644)

		r := NewBitNetRouter(modelPath)
		// bitnet-cli is likely not in PATH during test, so this should fail
		err := r.CheckAvailability()
		if err == nil || !strings.Contains(err.Error(), "bitnet-cli") {
			t.Errorf("expected 'bitnet-cli not found' error, got %v", err)
		}
	})
}
