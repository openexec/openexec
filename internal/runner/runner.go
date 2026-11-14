package runner

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// CLI default argument templates
var (
	ClaudeDefaultArgs = []string{
		"--output-format", "stream-json",
		"--verbose",
		"--max-turns", "50",
	}
	CodexDefaultArgs  = []string{"--prompt", "-"}
	GeminiDefaultArgs = []string{"--prompt", "-"}
	// CopilotDefaultArgs is GitHub's documented non-interactive form; the
	// loop adds -p <prompt>, --model and --allow-tool.
	CopilotDefaultArgs = []string{"-s", "--no-ask-user"}
)

// IsCopilotModel reports whether a model name selects the GitHub Copilot CLI:
// "copilot" (the CLI's default model) or "copilot:<model>".
func IsCopilotModel(model string) bool {
	m := strings.ToLower(model)
	return m == "copilot" || strings.HasPrefix(m, "copilot:")
}

// CopilotCLIModel returns the value for `copilot --model`, or "" for the CLI
// default.
func CopilotCLIModel(model string) string {
	if !IsCopilotModel(model) {
		return ""
	}
	_, name, _ := strings.Cut(model, ":")
	return strings.TrimSpace(name)
}

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
		case IsCopilotModel(m):
			cmd = "copilot"
			args = append([]string{}, CopilotDefaultArgs...)
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
			// Unknown models fall back to claude as the default runner
			cmd = "claude"
			args = append([]string{}, ClaudeDefaultArgs...)
		}
	}

	// 3. PATH preflight
	path, err := exec.LookPath(cmd)
	if err != nil {
		return "", nil, fmt.Errorf("runner %q not found on PATH. Install it or check your environment", cmd)
	}

	return path, args, nil
}

// CopilotEnv keeps other vendors' API keys away from GitHub and refuses a
// classic personal access token (ghp_), which Copilot does not accept, before
// the CLI starts. Sign-in and token refresh stay with the CLI. Every route that
// launches the copilot binary (loop, planner) uses it.
func CopilotEnv(environ []string) ([]string, error) {
	drop := map[string]bool{"OPENAI_API_KEY": true, "ANTHROPIC_API_KEY": true, "CODEX_API_KEY": true, "GEMINI_API_KEY": true}
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		name, value, _ := strings.Cut(e, "=")
		if drop[name] {
			continue
		}
		switch name {
		case "COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN":
			if strings.HasPrefix(value, "ghp_") {
				return nil, errors.New(name + ": GitHub Copilot does not accept classic personal access tokens (ghp_); sign in with 'copilot' (/login) or use a gho_, ghu_ or github_pat_ token")
			}
		}
		out = append(out, e)
	}
	return out, nil
}
