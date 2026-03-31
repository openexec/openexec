package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToolResultCache(t *testing.T) {
	// Arrange
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, ".openexec"), 0755); err != nil {
		t.Fatalf("failed to create .openexec dir: %v", err)
	}

	cache, err := NewToolResultCache(tmpDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to create tool result cache: %v", err)
	}
	defer cache.Close()

	t.Run("Set and Get", func(t *testing.T) {
		toolName := "read_file"
		input := map[string]interface{}{
			"path": "/project/main.go",
		}
		result := []byte(`{"content": "package main"}`)
		executionMs := int64(100)

		// Act - Set
		err := cache.Set(toolName, input, result, executionMs)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Act - Get
		cachedResult, err := cache.Get(toolName, input)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Assert
		if cachedResult == nil {
			t.Fatal("expected cache hit, got nil")
		}
		if string(cachedResult) != string(result) {
			t.Errorf("expected %s, got %s", string(result), string(cachedResult))
		}
	})

	t.Run("GetWithMetadata", func(t *testing.T) {
		toolName := "grep"
		input := map[string]interface{}{
			"pattern": "func",
			"path":    "/project",
		}
		result := []byte(`{"matches": ["main.go:10", "utils.go:20"]}`)
		executionMs := int64(250)

		// Set
		err := cache.Set(toolName, input, result, executionMs)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Get with metadata
		entry, err := cache.GetWithMetadata(toolName, input)
		if err != nil {
			t.Fatalf("GetWithMetadata failed: %v", err)
		}

		if entry == nil {
			t.Fatal("expected cache entry, got nil")
		}
		if entry.ToolName != toolName {
			t.Errorf("expected tool name %s, got %s", toolName, entry.ToolName)
		}
		if entry.ExecutionMs != executionMs {
			t.Errorf("expected execution time %d, got %d", executionMs, entry.ExecutionMs)
		}
		if string(entry.Result) != string(result) {
			t.Errorf("expected result %s, got %s", string(result), string(entry.Result))
		}
	})

	t.Run("Cache Miss - Different Input", func(t *testing.T) {
		toolName := "read_file"
		input1 := map[string]interface{}{"path": "/project/file1.go"}
		input2 := map[string]interface{}{"path": "/project/file2.go"}
		result := []byte(`{"content": "package main"}`)

		// Set with input1
		err := cache.Set(toolName, input1, result, 100)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Get with input2 (different path)
		cachedResult, err := cache.Get(toolName, input2)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		// Assert - should be nil (different input)
		if cachedResult != nil {
			t.Error("expected cache miss for different input, got result")
		}
	})

	t.Run("Cache Miss - Non-existent Entry", func(t *testing.T) {
		input := map[string]interface{}{"path": "/nonexistent"}
		result, err := cache.Get("nonexistent_tool", input)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if result != nil {
			t.Error("expected nil for non-existent entry")
		}
	})

	t.Run("Cache Expiration", func(t *testing.T) {
		// Create cache with very short TTL
		shortCache, err := NewToolResultCache(tmpDir, 1*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to create short cache: %v", err)
		}
		defer shortCache.Close()

		toolName := "expiring_tool"
		input := map[string]interface{}{"data": "test"}
		result := []byte(`{"result": "expiring"}`)

		// Set
		err = shortCache.Set(toolName, input, result, 50)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		// Get - should be expired
		cachedResult, err := shortCache.Get(toolName, input)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if cachedResult != nil {
			t.Error("expected nil for expired entry")
		}
	})

	t.Run("Invalidate", func(t *testing.T) {
		toolName := "invalidate_tool"
		input := map[string]interface{}{"key": "value"}
		result := []byte(`{"result": "to be invalidated"}`)

		// Set
		err := cache.Set(toolName, input, result, 100)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		// Invalidate
		err = cache.Invalidate(toolName, input)
		if err != nil {
			t.Fatalf("Invalidate failed: %v", err)
		}

		// Get - should be nil
		cachedResult, err := cache.Get(toolName, input)
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if cachedResult != nil {
			t.Error("expected nil after invalidate")
		}
	})

	t.Run("InvalidateTool", func(t *testing.T) {
		toolName := "multi_invalidate_tool"
		
		// Set multiple entries for same tool
		for i := 0; i < 3; i++ {
			input := map[string]interface{}{"id": i}
			result := []byte(`{"result": i}`)
			
			err := cache.Set(toolName, input, result, 100)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		// Invalidate entire tool
		err := cache.InvalidateTool(toolName)
		if err != nil {
			t.Fatalf("InvalidateTool failed: %v", err)
		}

		// Verify all entries are gone
		for i := 0; i < 3; i++ {
			input := map[string]interface{}{"id": i}
			
			result, err := cache.Get(toolName, input)
			if err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if result != nil {
				t.Errorf("expected nil for input %d after tool invalidate", i)
			}
		}
	})

	t.Run("Stats", func(t *testing.T) {
		// Add some entries
		for i := 0; i < 5; i++ {
			toolName := filepath.Join("tool", string(rune('a'+i)))
			input := map[string]interface{}{"id": i}
			result := []byte(`{"result": i}`)
			
			err := cache.Set(toolName, input, result, 100)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		total, expired, err := cache.Stats()
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		if total < 5 {
			t.Errorf("expected at least 5 entries, got %d", total)
		}
		if expired != 0 {
			t.Errorf("expected 0 expired, got %d", expired)
		}
	})

	t.Run("Cleanup", func(t *testing.T) {
		// Create cache with short TTL
		cleanupCache, err := NewToolResultCache(tmpDir, 1*time.Millisecond)
		if err != nil {
			t.Fatalf("failed to create cleanup cache: %v", err)
		}
		defer cleanupCache.Close()

		// Add entries
		for i := 0; i < 3; i++ {
			toolName := filepath.Join("cleanup", string(rune('a'+i)))
			input := map[string]interface{}{"id": i}
			result := []byte(`{"result": i}`)
			
			err := cleanupCache.Set(toolName, input, result, 100)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}
		}

		// Wait for expiration
		time.Sleep(10 * time.Millisecond)

		// Cleanup
		err = cleanupCache.Cleanup()
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		// Verify entries are gone
		total, _, err := cleanupCache.Stats()
		if err != nil {
			t.Fatalf("Stats failed: %v", err)
		}

		if total != 0 {
			t.Errorf("expected 0 entries after cleanup, got %d", total)
		}
	})

	t.Run("Deterministic Hash", func(t *testing.T) {
		// Same input should produce same hash
		input1 := map[string]interface{}{
			"path": "/project/main.go",
			"offset": 0,
			"limit": 100,
		}
		input2 := map[string]interface{}{
			"path": "/project/main.go",
			"offset": 0,
			"limit": 100,
		}

		hash1 := computeInputHash(input1)
		hash2 := computeInputHash(input2)

		if hash1 != hash2 {
			t.Error("same input should produce same hash")
		}

		// Different input should produce different hash
		input3 := map[string]interface{}{
			"path": "/project/other.go",
			"offset": 0,
			"limit": 100,
		}

		hash3 := computeInputHash(input3)

		if hash1 == hash3 {
			t.Error("different input should produce different hash")
		}
	})
}

func TestShouldCache(t *testing.T) {
	tests := []struct {
		toolName string
		expected bool
	}{
		{"read_file", true},
		{"grep", true},
		{"glob", true},
		{"deploy", false},
		{"chat", false},
		{"time", false},
		{"random", false},
		{"network_fetch", false},
		{"", true}, // Unknown tools are cacheable by default
	}

	for _, tt := range tests {
		t.Run(tt.toolName, func(t *testing.T) {
			result := ShouldCache(tt.toolName)
			if result != tt.expected {
				t.Errorf("ShouldCache(%q) = %v, expected %v", tt.toolName, result, tt.expected)
			}
		})
	}
}
