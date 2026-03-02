// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ProjectInstructionsGatherer collects project instruction files like CLAUDE.md.
type ProjectInstructionsGatherer struct {
	*BaseGatherer
}

// NewProjectInstructionsGatherer creates a new ProjectInstructionsGatherer.
func NewProjectInstructionsGatherer() *ProjectInstructionsGatherer {
	g := &ProjectInstructionsGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeProjectInstructions,
			"Project Instructions",
			"Reads CLAUDE.md, INSTRUCTIONS.md, and similar project instruction files",
		),
	}
	// Set default file paths for project instructions
	g.filePaths = []string{
		"CLAUDE.md",
		".claude/CLAUDE.md",
		"INSTRUCTIONS.md",
		".github/INSTRUCTIONS.md",
		".cursor/INSTRUCTIONS.md",
	}
	return g
}

// Gather collects project instruction file contents.
func (g *ProjectInstructionsGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("project path is required")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	var content strings.Builder
	var foundFiles []string
	filePaths := g.FilePaths()

	for _, relPath := range filePaths {
		fullPath := filepath.Join(projectPath, relPath)

		// Check if file exists and is readable
		info, err := os.Stat(fullPath)
		if err != nil {
			continue // File doesn't exist, try next
		}
		if info.IsDir() {
			continue // Skip directories
		}

		// Read file content
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue // Skip unreadable files
		}

		fileContent := string(data)
		if strings.TrimSpace(fileContent) == "" {
			continue // Skip empty files
		}

		// Add file header and content
		if content.Len() > 0 {
			content.WriteString("\n\n---\n\n")
		}
		content.WriteString(fmt.Sprintf("# %s\n\n", relPath))
		content.WriteString(fileContent)
		foundFiles = append(foundFiles, relPath)

		// Check context cancellation between files
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	if len(foundFiles) == 0 {
		return nil, fmt.Errorf("no project instruction files found in %s", projectPath)
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	source := fmt.Sprintf("files: %s", strings.Join(foundFiles, ", "))
	return g.CreateContextItem(source, finalContent, tokenCount)
}

// HasInstructionFile checks if a project has any instruction files.
func HasInstructionFile(projectPath string) bool {
	defaultPaths := []string{
		"CLAUDE.md",
		".claude/CLAUDE.md",
		"INSTRUCTIONS.md",
		".github/INSTRUCTIONS.md",
		".cursor/INSTRUCTIONS.md",
	}

	for _, relPath := range defaultPaths {
		fullPath := filepath.Join(projectPath, relPath)
		info, err := os.Stat(fullPath)
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
