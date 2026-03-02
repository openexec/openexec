// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitStatusGatherer collects the current git repository status.
type GitStatusGatherer struct {
	*BaseGatherer
}

// NewGitStatusGatherer creates a new GitStatusGatherer.
func NewGitStatusGatherer() *GitStatusGatherer {
	return &GitStatusGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeGitStatus,
			"Git Status",
			"Gathers current git repository status including branch, staged/unstaged changes, and untracked files",
		),
	}
}

// Gather collects git status information.
func (g *GitStatusGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	// Check if this is a git repository
	if !isGitRepo(projectPath) {
		return nil, fmt.Errorf("not a git repository: %s", projectPath)
	}

	// Get current branch
	branch, err := runGitCommand(ctx, projectPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}

	// Get git status
	status, err := runGitCommand(ctx, projectPath, "status", "--porcelain", "-b")
	if err != nil {
		return nil, fmt.Errorf("failed to get git status: %w", err)
	}

	// Format the output
	var content strings.Builder
	content.WriteString(fmt.Sprintf("Current branch: %s\n\n", strings.TrimSpace(branch)))

	if strings.TrimSpace(status) == "" || status == fmt.Sprintf("## %s\n", strings.TrimSpace(branch)) {
		content.WriteString("Status: Clean working tree (no uncommitted changes)\n")
	} else {
		content.WriteString("Status:\n")
		content.WriteString(status)
	}

	// Get recent commits summary (last 5)
	logOutput, err := runGitCommand(ctx, projectPath, "log", "--oneline", "-5")
	if err == nil && strings.TrimSpace(logOutput) != "" {
		content.WriteString("\nRecent commits:\n")
		content.WriteString(logOutput)
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	return g.CreateContextItem("git status", finalContent, tokenCount)
}

// GitDiffGatherer collects git diff information.
type GitDiffGatherer struct {
	*BaseGatherer
}

// NewGitDiffGatherer creates a new GitDiffGatherer.
func NewGitDiffGatherer() *GitDiffGatherer {
	return &GitDiffGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeGitDiff,
			"Git Diff",
			"Shows unstaged changes in the repository",
		),
	}
}

// Gather collects git diff information.
func (g *GitDiffGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	// Check if this is a git repository
	if !isGitRepo(projectPath) {
		return nil, fmt.Errorf("not a git repository: %s", projectPath)
	}

	var content strings.Builder

	// Get unstaged changes
	unstagedDiff, err := runGitCommand(ctx, projectPath, "diff")
	if err != nil {
		return nil, fmt.Errorf("failed to get unstaged diff: %w", err)
	}

	// Get staged changes
	stagedDiff, err := runGitCommand(ctx, projectPath, "diff", "--cached")
	if err != nil {
		return nil, fmt.Errorf("failed to get staged diff: %w", err)
	}

	hasChanges := false

	if strings.TrimSpace(stagedDiff) != "" {
		content.WriteString("=== Staged Changes ===\n")
		content.WriteString(stagedDiff)
		content.WriteString("\n")
		hasChanges = true
	}

	if strings.TrimSpace(unstagedDiff) != "" {
		content.WriteString("=== Unstaged Changes ===\n")
		content.WriteString(unstagedDiff)
		hasChanges = true
	}

	if !hasChanges {
		content.WriteString("No changes detected (working tree clean)")
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	return g.CreateContextItem("git diff", finalContent, tokenCount)
}

// GitLogGatherer collects git commit history.
type GitLogGatherer struct {
	*BaseGatherer
}

// NewGitLogGatherer creates a new GitLogGatherer.
func NewGitLogGatherer() *GitLogGatherer {
	return &GitLogGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeGitLog,
			"Git Log",
			"Shows recent commit history",
		),
	}
}

// Gather collects git log information.
func (g *GitLogGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	// Check if this is a git repository
	if !isGitRepo(projectPath) {
		return nil, fmt.Errorf("not a git repository: %s", projectPath)
	}

	// Get max commits from options
	maxCommits := g.GetIntOption("max_commits", 10)

	// Get commit log
	format := "%h|%s|%an|%ar"
	logOutput, err := runGitCommand(ctx, projectPath, "log",
		fmt.Sprintf("--format=%s", format),
		fmt.Sprintf("-n%d", maxCommits),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get git log: %w", err)
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("Recent commits (last %d):\n\n", maxCommits))

	if strings.TrimSpace(logOutput) == "" {
		content.WriteString("No commits found\n")
	} else {
		lines := strings.Split(strings.TrimSpace(logOutput), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, "|", 4)
			if len(parts) >= 4 {
				content.WriteString(fmt.Sprintf("  %s - %s (%s, %s)\n",
					parts[0], parts[1], parts[2], parts[3]))
			}
		}
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	return g.CreateContextItem("git log", finalContent, tokenCount)
}

// runGitCommand executes a git command and returns its output.
func runGitCommand(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, stderr.String())
	}

	return stdout.String(), nil
}

// isGitRepo checks if the given path is a git repository.
func isGitRepo(path string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	return cmd.Run() == nil
}
