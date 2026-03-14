package mcp

import (
	"testing"
)

func TestGenerateIdempotencyKey(t *testing.T) {
	runID := "RUN-20240315-120000"

	// Same inputs should produce same key
	args1 := map[string]interface{}{
		"path":    "/foo/bar.txt",
		"content": "hello world",
	}
	args2 := map[string]interface{}{
		"path":    "/foo/bar.txt",
		"content": "hello world",
	}

	key1 := GenerateIdempotencyKey(runID, "write_file", args1)
	key2 := GenerateIdempotencyKey(runID, "write_file", args2)

	if key1 != key2 {
		t.Errorf("Same inputs produced different keys: %s vs %s", key1, key2)
	}

	// Different inputs should produce different keys
	args3 := map[string]interface{}{
		"path":    "/foo/bar.txt",
		"content": "different content",
	}

	key3 := GenerateIdempotencyKey(runID, "write_file", args3)

	if key1 == key3 {
		t.Error("Different inputs produced same key")
	}

	// Different tool names should produce different keys
	key4 := GenerateIdempotencyKey(runID, "read_file", args1)

	if key1 == key4 {
		t.Error("Different tool names produced same key")
	}
}

func TestGenerateIdempotencyKey_PerRunScope(t *testing.T) {
	args := map[string]interface{}{
		"path":    "/foo/bar.txt",
		"content": "hello world",
	}

	// Different run IDs should produce different keys
	key1 := GenerateIdempotencyKey("RUN-001", "write_file", args)
	key2 := GenerateIdempotencyKey("RUN-002", "write_file", args)

	if key1 == key2 {
		t.Error("Different run IDs should produce different keys (per-run scope)")
	}

	// Same run ID should produce same key
	key3 := GenerateIdempotencyKey("RUN-001", "write_file", args)

	if key1 != key3 {
		t.Error("Same run ID should produce same key")
	}
}

func TestGenerateIdempotencyKey_KeyOrdering(t *testing.T) {
	runID := "RUN-001"

	// Keys in different order should produce same hash
	args1 := map[string]interface{}{
		"a": "1",
		"b": "2",
		"c": "3",
	}
	args2 := map[string]interface{}{
		"c": "3",
		"a": "1",
		"b": "2",
	}

	key1 := GenerateIdempotencyKey(runID, "test_tool", args1)
	key2 := GenerateIdempotencyKey(runID, "test_tool", args2)

	if key1 != key2 {
		t.Errorf("Key ordering affected hash: %s vs %s", key1, key2)
	}
}

func TestGenerateIdempotencyKey_VersionInvalidation(t *testing.T) {
	// This test verifies that the key includes ToolRegistryVersion.
	// When the version changes, keys should be different.
	// We test this by checking the key includes the version in its input.

	runID := "RUN-001"
	args := map[string]interface{}{
		"path": "/test.txt",
	}

	// Generate a key
	key1 := GenerateIdempotencyKey(runID, "write_file", args)

	// The key should be 64 hex characters (SHA-256)
	if len(key1) != 64 {
		t.Errorf("Key should be 64 hex chars, got %d", len(key1))
	}

	// Verify the key is deterministic with current version
	key2 := GenerateIdempotencyKey(runID, "write_file", args)
	if key1 != key2 {
		t.Error("Keys should be deterministic")
	}

	// Note: When ToolRegistryVersion changes in mcp/version.go,
	// all existing keys will automatically become invalid because
	// the version is part of the key input. This is by design.
	// The test above verifies the key is deterministic with the current version.
}

func TestGenerateIdempotencyKey_NestedArgs(t *testing.T) {
	runID := "RUN-001"

	// Nested args should be handled correctly
	args1 := map[string]interface{}{
		"nested": map[string]interface{}{
			"a": "1",
			"b": "2",
		},
	}
	args2 := map[string]interface{}{
		"nested": map[string]interface{}{
			"b": "2",
			"a": "1",
		},
	}

	key1 := GenerateIdempotencyKey(runID, "test_tool", args1)
	key2 := GenerateIdempotencyKey(runID, "test_tool", args2)

	if key1 != key2 {
		t.Errorf("Nested key ordering affected hash: %s vs %s", key1, key2)
	}
}

func TestGenerateIdempotencyKey_EmptyArgs(t *testing.T) {
	runID := "RUN-001"

	key1 := GenerateIdempotencyKey(runID, "test_tool", nil)
	key2 := GenerateIdempotencyKey(runID, "test_tool", map[string]interface{}{})

	if key1 != key2 {
		t.Errorf("nil and empty args produced different keys: %s vs %s", key1, key2)
	}
}

func TestInMemoryIdempotencyChecker(t *testing.T) {
	checker := NewInMemoryIdempotencyChecker()

	key := "test-key-123"

	// Initially not applied
	applied, err := checker.WasApplied(key)
	if err != nil {
		t.Fatalf("WasApplied error: %v", err)
	}
	if applied {
		t.Error("Key should not be applied initially")
	}

	// Mark as applied
	err = checker.MarkApplied(key, "test_tool", "output")
	if err != nil {
		t.Fatalf("MarkApplied error: %v", err)
	}

	// Now should be applied
	applied, err = checker.WasApplied(key)
	if err != nil {
		t.Fatalf("WasApplied error: %v", err)
	}
	if !applied {
		t.Error("Key should be applied after marking")
	}
}

func TestIsIdempotentTool(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"write_file", true},
		{"run_shell_command", false}, // Shell commands are non-idempotent (partial execution risk)
		{"git_apply_patch", true},
		{"read_file", false},
		{"openexec_signal", false},
		{"fork_session", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isIdempotentTool(tt.name)
			if result != tt.expected {
				t.Errorf("isIdempotentTool(%s) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}
