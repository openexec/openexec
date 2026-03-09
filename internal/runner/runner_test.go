package runner

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// =============================================================================
// Test Seeds for Runner Resolution
// Implements T-US-002-004: CLI Integration Testing
// =============================================================================

// TestResolve_ClaudeModels verifies Claude model variants map to claude CLI.
func TestResolve_ClaudeModels(t *testing.T) {
	// Skip if claude not in PATH (CI environments may not have it)
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not in PATH, skipping integration test")
	}

	testCases := []struct {
		model string
	}{
		{"claude"},
		{"claude-3"},
		{"sonnet"},
		{"claude-3-sonnet"},
		{"opus"},
		{"claude-opus-4"},
		{"haiku"},
		{"claude-3-haiku"},
		{"CLAUDE"}, // case insensitive
		{"Sonnet"},
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			cmd, args, err := Resolve(tc.model, "", nil)
			if err != nil {
				t.Fatalf("Resolve(%q) failed: %v", tc.model, err)
			}

			if !strings.Contains(cmd, "claude") {
				t.Errorf("expected claude executable, got %q", cmd)
			}

			// Verify default args are included
			if len(args) == 0 {
				t.Error("expected non-empty default args")
			}

			// Check for key default args
			argsStr := strings.Join(args, " ")
			if !strings.Contains(argsStr, "--dangerously-skip-permissions") {
				t.Error("missing --dangerously-skip-permissions in default args")
			}
			if !strings.Contains(argsStr, "--output-format") {
				t.Error("missing --output-format in default args")
			}
		})
	}
}

// TestResolve_OpenAIModels verifies OpenAI model variants map to codex CLI.
func TestResolve_OpenAIModels(t *testing.T) {
	// Skip if codex not in PATH
	if _, err := exec.LookPath("codex"); err != nil {
		t.Skip("codex CLI not in PATH, skipping integration test")
	}

	testCases := []struct {
		model string
	}{
		{"gpt-4"},
		{"gpt-4-turbo"},
		{"gpt-3.5-turbo"},
		{"codex"},
		{"openai"},
		{"GPT-4"}, // case insensitive
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			cmd, args, err := Resolve(tc.model, "", nil)
			if err != nil {
				t.Fatalf("Resolve(%q) failed: %v", tc.model, err)
			}

			if !strings.Contains(cmd, "codex") {
				t.Errorf("expected codex executable, got %q", cmd)
			}

			// Verify args include prompt flag
			argsStr := strings.Join(args, " ")
			if !strings.Contains(argsStr, "--prompt") {
				t.Error("missing --prompt in codex args")
			}
		})
	}
}

// TestResolve_GeminiModels verifies Gemini models map to gemini CLI.
func TestResolve_GeminiModels(t *testing.T) {
	// Skip if gemini not in PATH
	if _, err := exec.LookPath("gemini"); err != nil {
		t.Skip("gemini CLI not in PATH, skipping integration test")
	}

	testCases := []struct {
		model string
	}{
		{"gemini"},
		{"gemini-pro"},
		{"gemini-1.5-pro"},
		{"GEMINI"}, // case insensitive
	}

	for _, tc := range testCases {
		t.Run(tc.model, func(t *testing.T) {
			cmd, args, err := Resolve(tc.model, "", nil)
			if err != nil {
				t.Fatalf("Resolve(%q) failed: %v", tc.model, err)
			}

			if !strings.Contains(cmd, "gemini") {
				t.Errorf("expected gemini executable, got %q", cmd)
			}

			// Verify args include yolo flag
			argsStr := strings.Join(args, " ")
			if !strings.Contains(argsStr, "--yolo") {
				t.Error("missing --yolo in gemini args")
			}
		})
	}
}

// TestResolve_UnknownModelFallback verifies unknown models fall back to claude.
func TestResolve_UnknownModelFallback(t *testing.T) {
	// Skip if claude not in PATH
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not in PATH, skipping integration test")
	}

	testCases := []string{
		"mistral-7b",
		"llama-2",
		"phi-3",
		"unknown-model",
		"custom-local",
	}

	for _, model := range testCases {
		t.Run(model, func(t *testing.T) {
			cmd, _, err := Resolve(model, "", nil)
			if err != nil {
				t.Fatalf("Resolve(%q) failed: %v", model, err)
			}

			// Unknown models should fall back to claude
			if !strings.Contains(cmd, "claude") {
				t.Errorf("expected claude fallback for unknown model %q, got %q", model, cmd)
			}
		})
	}
}

// TestResolve_EmptyModelFallback verifies empty string falls back to claude.
func TestResolve_EmptyModelFallback(t *testing.T) {
	// Skip if claude not in PATH
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not in PATH, skipping integration test")
	}

	cmd, args, err := Resolve("", "", nil)
	if err != nil {
		t.Fatalf("Resolve('') failed: %v", err)
	}

	if !strings.Contains(cmd, "claude") {
		t.Errorf("expected claude for empty model, got %q", cmd)
	}

	if len(args) == 0 {
		t.Error("expected default args for empty model")
	}
}

// TestResolve_OverrideCmd verifies override command takes precedence.
func TestResolve_OverrideCmd(t *testing.T) {
	// Create a mock executable in temp dir
	tmpDir := t.TempDir()
	mockExec := tmpDir + "/mock-cli"

	// Create empty executable file
	if err := os.WriteFile(mockExec, []byte("#!/bin/sh\necho ok"), 0755); err != nil {
		t.Fatalf("failed to create mock executable: %v", err)
	}

	overrideArgs := []string{"--custom-flag", "value"}

	cmd, args, err := Resolve("claude", mockExec, overrideArgs)
	if err != nil {
		t.Fatalf("Resolve with override failed: %v", err)
	}

	if cmd != mockExec {
		t.Errorf("expected override path %q, got %q", mockExec, cmd)
	}

	if len(args) != len(overrideArgs) {
		t.Errorf("expected override args %v, got %v", overrideArgs, args)
	}
}

// TestResolve_PathNotFound verifies error when executable not in PATH.
func TestResolve_PathNotFound(t *testing.T) {
	// Use a model that doesn't exist
	_, _, err := Resolve("", "nonexistent-cli-that-does-not-exist-12345", nil)

	if err == nil {
		t.Fatal("expected error for nonexistent CLI, got nil")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

// TestResolve_DefaultArgsNotMutated verifies default args are copied, not shared.
func TestResolve_DefaultArgsNotMutated(t *testing.T) {
	// Skip if claude not in PATH
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude CLI not in PATH, skipping integration test")
	}

	// Get args twice
	_, args1, err := Resolve("claude", "", nil)
	if err != nil {
		t.Fatalf("first Resolve failed: %v", err)
	}

	_, args2, err := Resolve("claude", "", nil)
	if err != nil {
		t.Fatalf("second Resolve failed: %v", err)
	}

	// Mutate first slice
	if len(args1) > 0 {
		args1[0] = "mutated"
	}

	// Verify second slice is not affected
	if len(args2) > 0 && args2[0] == "mutated" {
		t.Error("default args were shared between calls, not copied")
	}
}

// TestClaudeDefaultArgs verifies the exported default args constants.
func TestClaudeDefaultArgs(t *testing.T) {
	if len(ClaudeDefaultArgs) < 4 {
		t.Errorf("expected at least 4 default claude args, got %d", len(ClaudeDefaultArgs))
	}

	expectedFlags := []string{
		"--dangerously-skip-permissions",
		"--output-format",
		"--verbose",
		"--max-turns",
	}

	argsStr := strings.Join(ClaudeDefaultArgs, " ")
	for _, flag := range expectedFlags {
		if !strings.Contains(argsStr, flag) {
			t.Errorf("missing expected flag %q in ClaudeDefaultArgs", flag)
		}
	}
}

// TestCodexDefaultArgs verifies the exported codex default args.
func TestCodexDefaultArgs(t *testing.T) {
	if len(CodexDefaultArgs) < 2 {
		t.Errorf("expected at least 2 default codex args, got %d", len(CodexDefaultArgs))
	}

	argsStr := strings.Join(CodexDefaultArgs, " ")
	if !strings.Contains(argsStr, "--prompt") {
		t.Error("missing --prompt in CodexDefaultArgs")
	}
}

// TestGeminiDefaultArgs verifies the exported gemini default args.
func TestGeminiDefaultArgs(t *testing.T) {
	if len(GeminiDefaultArgs) < 2 {
		t.Errorf("expected at least 2 default gemini args, got %d", len(GeminiDefaultArgs))
	}

	argsStr := strings.Join(GeminiDefaultArgs, " ")
	if !strings.Contains(argsStr, "--yolo") {
		t.Error("missing --yolo in GeminiDefaultArgs")
	}
}
