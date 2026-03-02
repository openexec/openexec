// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// EnvironmentGatherer collects environment information (OS, platform, runtime, etc.).
type EnvironmentGatherer struct {
	*BaseGatherer
}

// NewEnvironmentGatherer creates a new EnvironmentGatherer.
func NewEnvironmentGatherer() *EnvironmentGatherer {
	return &EnvironmentGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeEnvironment,
			"Environment Info",
			"Collects OS, platform, runtime, and working directory information",
		),
	}
}

// Gather collects environment information.
func (g *EnvironmentGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var content strings.Builder

	// Working directory
	content.WriteString("Working directory: ")
	if projectPath != "" {
		content.WriteString(projectPath)
	} else {
		wd, err := os.Getwd()
		if err != nil {
			content.WriteString("unknown")
		} else {
			content.WriteString(wd)
		}
	}
	content.WriteString("\n")

	// Check if it's a git repo
	isGitRepoFlag := isGitRepoCheck(projectPath)
	content.WriteString(fmt.Sprintf("Is directory a git repo: %s\n", boolToYesNo(isGitRepoFlag)))

	// Platform
	content.WriteString(fmt.Sprintf("Platform: %s\n", runtime.GOOS))

	// OS Version
	osVersion := getOSVersion()
	if osVersion != "" {
		content.WriteString(fmt.Sprintf("OS Version: %s\n", osVersion))
	}

	// Today's date
	content.WriteString(fmt.Sprintf("Today's date: %s\n", time.Now().Format("2006-01-02")))

	// Additional runtime info
	content.WriteString(fmt.Sprintf("Architecture: %s\n", runtime.GOARCH))

	// Shell environment
	shell := os.Getenv("SHELL")
	if shell != "" {
		content.WriteString(fmt.Sprintf("Shell: %s\n", filepath.Base(shell)))
	}

	// Go version (useful for Go projects)
	goVersion := runtime.Version()
	if goVersion != "" {
		content.WriteString(fmt.Sprintf("Go version: %s\n", goVersion))
	}

	// Check for common development tools
	tools := detectDevelopmentTools(ctx)
	if len(tools) > 0 {
		content.WriteString(fmt.Sprintf("Detected tools: %s\n", strings.Join(tools, ", ")))
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	return g.CreateContextItem("environment", finalContent, tokenCount)
}

// isGitRepoCheck checks if the given path is a git repository.
func isGitRepoCheck(path string) bool {
	if path == "" {
		var err error
		path, err = os.Getwd()
		if err != nil {
			return false
		}
	}
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// getOSVersion returns the OS version string.
func getOSVersion() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sw_vers", "-productVersion").Output()
		if err == nil {
			return "macOS " + strings.TrimSpace(string(out))
		}
		out, err = exec.Command("uname", "-rs").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "linux":
		// Try to read /etc/os-release
		data, err := os.ReadFile("/etc/os-release")
		if err == nil {
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PRETTY_NAME=") {
					name := strings.TrimPrefix(line, "PRETTY_NAME=")
					name = strings.Trim(name, "\"")
					return name
				}
			}
		}
		out, err := exec.Command("uname", "-rs").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "windows":
		out, err := exec.Command("cmd", "/c", "ver").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	default:
		out, err := exec.Command("uname", "-rs").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

// detectDevelopmentTools detects common development tools.
func detectDevelopmentTools(ctx context.Context) []string {
	tools := []string{}

	toolCommands := []struct {
		name    string
		command string
		args    []string
	}{
		{"node", "node", []string{"--version"}},
		{"npm", "npm", []string{"--version"}},
		{"python", "python3", []string{"--version"}},
		{"pip", "pip3", []string{"--version"}},
		{"docker", "docker", []string{"--version"}},
		{"git", "git", []string{"--version"}},
	}

	for _, tool := range toolCommands {
		select {
		case <-ctx.Done():
			return tools
		default:
		}

		cmd := exec.CommandContext(ctx, tool.command, tool.args...)
		if err := cmd.Run(); err == nil {
			tools = append(tools, tool.name)
		}
	}

	return tools
}

// boolToYesNo converts a boolean to "Yes" or "No".
func boolToYesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}
