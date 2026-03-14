//go:build integration

// Package cli provides CLI integration tests for the OpenExec chat command.
// These tests validate the G-001 goal fix at the CLI level by spawning actual
// binary processes and verifying no forbidden error messages appear.
//
// Run with: go test -tags=integration ./internal/cli/... -v
package cli

import (
	"context"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// forbiddenCLIErrorStrings defines error messages that must never appear in CLI output.
// These indicate intent routing failures that the G-001 fix should prevent.
// Mirrors the HTTP-level constant from internal/server/e2e_validation_test.go.
var forbiddenCLIErrorStrings = []string{
	"could not determine intent",
	"low confidence",
	"model could not determine",
}

// getProjectRoot finds the root directory of the openexec project.
// It walks up from the current working directory looking for go.mod.
func getProjectRoot() string {
	// Start from current working directory
	wd, err := os.Getwd()
	if err != nil {
		// Fall back to relative path from test location
		return filepath.Join("..", "..")
	}

	// Walk up until we find go.mod
	dir := wd
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fall back to assuming we're in internal/cli
	return filepath.Join(wd, "..", "..")
}

// TestCLIChatIntegrationGoalValidation validates G-001 at the CLI level.
// This is the primary integration test that proves the intent routing fix
// works when users actually run `openexec chat`.
//
// The test:
// 1. Builds the openexec binary
// 2. Starts the chat command (which auto-starts the server)
// 3. Sends test queries via stdin
// 4. Validates no forbidden error messages appear in output
func TestCLIChatIntegrationGoalValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI integration test in short mode")
	}

	projectRoot := getProjectRoot()
	binaryPath := filepath.Join(projectRoot, "openexec-test")

	// Build the binary
	t.Log("Building openexec binary...")
	buildCmd := exec.Command("go", "build", "-o", "openexec-test", "./cmd/openexec")
	buildCmd.Dir = projectRoot
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build openexec binary: %v\nOutput: %s", err, string(buildOut))
	}
	// Use t.Cleanup instead of defer to ensure cleanup runs AFTER parallel subtests complete
	t.Cleanup(func() {
		os.Remove(binaryPath)
	})

	t.Log("Binary built successfully, running test matrix...")

	// Test matrix - critical scenarios that cover different routing paths
	testCases := []struct {
		name  string
		input string
	}{
		// Help/informational - should route to GeneralChatTool
		{"help_query", "help\nexit\n"},

		// Gibberish - should fall back to general_chat, NOT error out
		{"gibberish_query", "asdf1234xyz\nexit\n"},

		// Keyword-based routing - should route to deploy tool
		{"deploy_keyword", "deploy to prod\nexit\n"},

		// Unicode - should handle UTF-8 gracefully
		{"unicode_input", "こんにちは\nexit\n"},

		// Empty-ish - should not crash or return forbidden errors
		{"whitespace_input", "   \nexit\n"},

		// Special characters - should be handled gracefully
		{"special_chars", "!@#$%^&*()\nexit\n"},

		// Multi-word ambiguous - should fall back gracefully
		{"ambiguous_query", "I want to maybe do something\nexit\n"},

		// Quick exit - should not crash
		{"quick_exit", "exit\n"},
	}

	for _, tc := range testCases {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Use a unique port per test to avoid conflicts (range: 18000-18999)
			port := 18000 + rand.Intn(1000)

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, binaryPath, "chat", "--port", strconv.Itoa(port))
			cmd.Dir = projectRoot
			cmd.Stdin = strings.NewReader(tc.input)

			output, err := cmd.CombinedOutput()
			outputStr := string(output)

			// Log output for debugging
			t.Logf("Output for %s (port %d):\n%s", tc.name, port, outputStr)

			// Handle exit conditions
			if err != nil {
				// Context timeout is a failure
				if ctx.Err() == context.DeadlineExceeded {
					t.Fatalf("timeout waiting for CLI to complete: %v", err)
				}

				// Check if it's an expected exit (exit code 0 or 1 for normal termination)
				if exitErr, ok := err.(*exec.ExitError); ok {
					// Non-zero exit is acceptable if no forbidden strings found
					t.Logf("CLI exited with code %d", exitErr.ExitCode())
				} else {
					// Unexpected error type
					t.Logf("CLI error (non-fatal): %v", err)
				}
			}

			// CRITICAL ASSERTION: Check for forbidden substrings
			outputLower := strings.ToLower(outputStr)
			for _, forbidden := range forbiddenCLIErrorStrings {
				if strings.Contains(outputLower, forbidden) {
					t.Errorf("CRITICAL G-001 VIOLATION: Found forbidden substring %q in CLI output:\n%s",
						forbidden, outputStr)
				}
			}

			// Verify we got some recognizable output (not just errors)
			// The chat command should show either:
			// - "OpenExec" banner
			// - "Agent:" response prefix
			// - Welcome message
			hasRecognizableOutput := strings.Contains(outputStr, "Agent:") ||
				strings.Contains(outputStr, "OpenExec") ||
				strings.Contains(outputStr, "Welcome") ||
				strings.Contains(outputStr, "Starting")

			if !hasRecognizableOutput && tc.name != "quick_exit" {
				t.Logf("Warning: No recognizable response patterns in output")
			}
		})
	}
}

// TestCLIChatProcessLifecycle validates clean process startup and shutdown.
// This ensures the CLI doesn't panic, hang, or produce stack traces.
func TestCLIChatProcessLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI integration test in short mode")
	}

	projectRoot := getProjectRoot()
	binaryPath := filepath.Join(projectRoot, "openexec-test")

	// Build if not exists (in case running this test independently)
	needsCleanup := false
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		buildCmd := exec.Command("go", "build", "-o", "openexec-test", "./cmd/openexec")
		buildCmd.Dir = projectRoot
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("failed to build: %v", err)
		}
		needsCleanup = true
	}
	if needsCleanup {
		t.Cleanup(func() {
			os.Remove(binaryPath)
		})
	}

	port := 18000 + rand.Intn(1000)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// GIVEN: The openexec binary is built
	// WHEN: User starts chat and immediately exits
	cmd := exec.CommandContext(ctx, binaryPath, "chat", "--port", strconv.Itoa(port))
	cmd.Dir = projectRoot
	cmd.Stdin = strings.NewReader("exit\n")

	output, err := cmd.CombinedOutput()
	outputStr := string(output)

	// THEN: Process exits without panic
	if strings.Contains(outputStr, "panic:") {
		t.Errorf("CLI panicked on exit:\n%s", outputStr)
	}
	if strings.Contains(outputStr, "runtime error:") {
		t.Errorf("CLI had runtime error:\n%s", outputStr)
	}
	if strings.Contains(outputStr, "goroutine") && strings.Contains(outputStr, "stack trace") {
		t.Errorf("CLI dumped stack trace:\n%s", outputStr)
	}

	// Check for timeout (indicates hang)
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("CLI hung on exit - did not terminate within timeout")
	}

	// Non-zero exit is acceptable for SIGTERM/interrupt scenarios
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Logf("CLI exit error (non-fatal): %v", err)
		}
	}

	t.Log("CLI process lifecycle test passed - clean startup and shutdown")
}

// TestCLIChatForbiddenErrorsComprehensive runs an extended matrix
// to ensure no forbidden errors appear under any input condition.
func TestCLIChatForbiddenErrorsComprehensive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI integration test in short mode")
	}

	projectRoot := getProjectRoot()
	binaryPath := filepath.Join(projectRoot, "openexec-test")

	// Build if not exists
	needsCleanup := false
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		buildCmd := exec.Command("go", "build", "-o", "openexec-test", "./cmd/openexec")
		buildCmd.Dir = projectRoot
		if err := buildCmd.Run(); err != nil {
			t.Fatalf("failed to build: %v", err)
		}
		needsCleanup = true
	}
	if needsCleanup {
		t.Cleanup(func() {
			os.Remove(binaryPath)
		})
	}

	// Extended input corpus - edge cases that might trigger routing failures
	edgeCaseInputs := []string{
		"",                       // empty
		"   ",                    // whitespace only
		"a",                      // single char
		"help",                   // standard help
		"???",                    // punctuation only
		"123456",                 // numbers only
		"你好",                     // Chinese
		"مرحبا",                  // Arabic
		"🚀🔥💻",                    // emoji only
		"deploy",                 // single keyword
		"commit push deploy",     // multiple keywords
		"what is 2+2?",           // question
		`{"json": "object"}`,     // JSON
		"<html>test</html>",      // HTML
		"SELECT * FROM users",    // SQL-like
		strings.Repeat("x", 100), // long input
	}

	for i, input := range edgeCaseInputs {
		input := input // capture
		testName := input
		if len(testName) > 20 {
			testName = testName[:20] + "..."
		}
		testName = strings.ReplaceAll(testName, "\n", "\\n")
		if testName == "" {
			testName = "empty"
		}

		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			port := 18000 + i + rand.Intn(500)

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()

			cmd := exec.CommandContext(ctx, binaryPath, "chat", "--port", strconv.Itoa(port))
			cmd.Dir = projectRoot
			cmd.Stdin = strings.NewReader(input + "\nexit\n")

			output, _ := cmd.CombinedOutput()
			outputStr := string(output)

			// CRITICAL: Check for forbidden substrings
			outputLower := strings.ToLower(outputStr)
			for _, forbidden := range forbiddenCLIErrorStrings {
				if strings.Contains(outputLower, forbidden) {
					t.Errorf("G-001 VIOLATION for input %q: Found forbidden substring %q in output:\n%s",
						input, forbidden, outputStr)
				}
			}
		})
	}
}
