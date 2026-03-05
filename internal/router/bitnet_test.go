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

	t.Run("Low Confidence Failure", func(t *testing.T) {
		// Act
		_, err := r.ParseIntent(ctx, "What is the weather today?")

		// Assert
		if err == nil || !strings.Contains(err.Error(), "confidence") {
			t.Errorf("expected low confidence error, got %v", err)
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
