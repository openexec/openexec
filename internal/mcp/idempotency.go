// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements idempotency key generation and checking for tool calls.
package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// IdempotencyKeyVersion is incremented when the key generation algorithm changes.
// This ensures old keys don't collide with new ones after algorithm updates.
const IdempotencyKeyVersion = "1"

// GenerateIdempotencyKey creates a deterministic key for a tool call.
// The key is: sha256(version + run_id + tool_name + normalized_args + tool_registry_version)
// This enables skip-on-resume for identical tool invocations within the same run.
// Keys are scoped per-run to avoid global deduplication across different executions.
func GenerateIdempotencyKey(runID, toolName string, args map[string]interface{}) string {
	// Normalize args by sorting keys and re-serializing
	normalizedArgs := normalizeArgs(args)

	// Build the key input (per-run scoped)
	input := IdempotencyKeyVersion + ":" + runID + ":" + toolName + ":" + normalizedArgs + ":" + ToolRegistryVersion

	// Hash it
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// GenerateGlobalIdempotencyKey creates a key without run_id for global deduplication.
// Use sparingly - typically for operations that are truly global (e.g., one-time migrations).
func GenerateGlobalIdempotencyKey(toolName string, args map[string]interface{}) string {
	normalizedArgs := normalizeArgs(args)
	input := IdempotencyKeyVersion + ":global:" + toolName + ":" + normalizedArgs + ":" + ToolRegistryVersion
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}

// normalizeArgs creates a deterministic string representation of arguments.
// Keys are sorted alphabetically to ensure consistent ordering.
func normalizeArgs(args map[string]interface{}) string {
	if args == nil || len(args) == 0 {
		return "{}"
	}

	// Get sorted keys
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build ordered map
	ordered := make([]interface{}, 0, len(keys)*2)
	for _, k := range keys {
		ordered = append(ordered, k, normalizeValue(args[k]))
	}

	// Create a stable JSON representation
	result := make(map[string]interface{})
	for i := 0; i < len(ordered); i += 2 {
		result[ordered[i].(string)] = ordered[i+1]
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "{}"
	}
	return string(data)
}

// normalizeValue recursively normalizes nested maps.
func normalizeValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		return normalizeArgsToMap(val)
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = normalizeValue(item)
		}
		return result
	default:
		return v
	}
}

// normalizeArgsToMap creates a normalized map with sorted keys.
func normalizeArgsToMap(args map[string]interface{}) map[string]interface{} {
	if args == nil {
		return nil
	}

	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make(map[string]interface{})
	for _, k := range keys {
		result[k] = normalizeValue(args[k])
	}
	return result
}

// IdempotencyChecker provides an interface for checking if a tool call was already applied.
type IdempotencyChecker interface {
	// WasApplied returns true if a tool call with this key was successfully completed.
	WasApplied(key string) (bool, error)
	// MarkApplied records that a tool call with this key completed successfully.
	MarkApplied(key string, toolName string, output string) error
}

// ToolCallResult contains the result of checking idempotency.
type ToolCallResult struct {
	Key         string // The idempotency key
	WasSkipped  bool   // True if the call was skipped due to prior application
	PriorOutput string // The output from the prior application (if skipped)
}
