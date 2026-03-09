package runner

import (
	"fmt"
	"os/exec"
	"strings"
)

// Default arguments for supported CLIs
var (
	ClaudeDefaultArgs = []string{
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", "50",
	}

	CodexDefaultArgs = []string{
		"--prompt", "-",
	}

	GeminiDefaultArgs = []string{
		"--prompt", "-",
		"--yolo",
	}
)

// Resolve returns the command name and arguments for a given model and optional overrides.
// It also performs a PATH preflight check.
func Resolve(model string, overrideCmd string, overrideArgs []string) (string, []string, error) {
	cmd := ""
	var args []string

	// 1. Config overrides win
	if overrideCmd != "" {
		cmd = overrideCmd
		args = overrideArgs
	} else {
		// 2. Map model to runner
		m := strings.ToLower(model)
		switch {
		case m == "", strings.Contains(m, "claude"), strings.Contains(m, "sonnet"), strings.Contains(m, "opus"), strings.Contains(m, "haiku"):
			cmd = "claude"
			args = append([]string{}, ClaudeDefaultArgs...)
		case strings.HasPrefix(m, "gpt-") || strings.Contains(m, "codex") || strings.Contains(m, "openai"):
			cmd = "codex"
			args = append([]string{}, CodexDefaultArgs...)
		case strings.HasPrefix(m, "gemini"):
			cmd = "gemini"
			args = append([]string{}, GeminiDefaultArgs...)
		default:
			// Default to claude if unknown
			cmd = "claude"
			args = append([]string{}, ClaudeDefaultArgs...)
		}
	}

	// 3. PATH preflight
	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", nil, fmt.Errorf("runner '%s' not found on PATH: %w", cmd, err)
	}

	return path, args, nil
}
