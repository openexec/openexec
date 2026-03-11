package runner

import (
	"fmt"
	"os/exec"
	"strings"
)

// CLI default argument templates
var (
	ClaudeArgs = []string{
		"--dangerously-skip-permissions",
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", "50",
	}
	CodexArgs  = []string{"--prompt", "-"}
	GeminiArgs = []string{"--prompt", "-", "--yolo"}
)

// Resolve maps a model name to a local CLI command and its default arguments.
// It prioritizes explicit overrides from the project configuration.
func Resolve(model string, overrideCmd string, overrideArgs []string) (string, []string, error) {
	cmd := ""
	var args []string

	// 1. Config overrides win
	if overrideCmd != "" {
		cmd = overrideCmd
		args = overrideArgs
	} else {
		// 2. Map model family to default CLI
		m := strings.ToLower(model)
		switch {
		case m == "", strings.Contains(m, "claude"), strings.Contains(m, "sonnet"), strings.Contains(m, "opus"), strings.Contains(m, "haiku"):
			cmd = "claude"
			args = append([]string{}, ClaudeArgs...)
		case strings.HasPrefix(m, "gpt-") || strings.Contains(m, "codex") || strings.Contains(m, "openai"):
			cmd = "codex"
			args = append([]string{}, CodexArgs...)
		case strings.HasPrefix(m, "gemini"):
			cmd = "gemini"
			args = append([]string{}, GeminiArgs...)
		default:
			return "", nil, fmt.Errorf("no known runner for model %q; specify runner_command in config", model)
		}
	}

	// 3. PATH preflight
	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", nil, fmt.Errorf("runner %q not found on PATH. Install it or check your environment", cmd)
	}

	return path, args, nil
}
