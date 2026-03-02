// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectoryStructureGatherer collects the project directory tree.
type DirectoryStructureGatherer struct {
	*BaseGatherer
}

// NewDirectoryStructureGatherer creates a new DirectoryStructureGatherer.
func NewDirectoryStructureGatherer() *DirectoryStructureGatherer {
	g := &DirectoryStructureGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeDirectoryStructure,
			"Directory Structure",
			"Generates a project directory tree showing files and folders",
		),
	}
	// Set default options
	g.options = map[string]interface{}{
		"max_depth": 4,
		"exclude": []string{
			"node_modules",
			".git",
			"__pycache__",
			"vendor",
			".venv",
			"venv",
			".idea",
			".vscode",
			"dist",
			"build",
			"target",
			".next",
			".nuxt",
			"coverage",
		},
		"max_files": 500,
	}
	return g
}

// Gather collects directory structure information.
func (g *DirectoryStructureGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("project path is required")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get options
	maxDepth := g.GetIntOption("max_depth", 4)
	excludeDirs := g.GetStringSliceOption("exclude", []string{})
	maxFiles := g.GetIntOption("max_files", 500)

	// Build exclude map for fast lookup
	excludeMap := make(map[string]bool)
	for _, dir := range excludeDirs {
		excludeMap[dir] = true
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("Directory structure of %s:\n\n", filepath.Base(projectPath)))

	fileCount := 0
	truncated := false

	// Walk the directory tree
	err := walkDirectory(ctx, projectPath, "", 0, maxDepth, excludeMap, &content, &fileCount, maxFiles, &truncated)
	if err != nil {
		return nil, err
	}

	if truncated {
		content.WriteString(fmt.Sprintf("\n... (truncated, showing first %d files)\n", maxFiles))
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	return g.CreateContextItem("directory structure", finalContent, tokenCount)
}

// walkDirectory recursively walks a directory and builds the tree structure.
func walkDirectory(ctx context.Context, basePath, relativePath string, depth, maxDepth int, excludeMap map[string]bool, content *strings.Builder, fileCount *int, maxFiles int, truncated *bool) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check file limit
	if *fileCount >= maxFiles {
		*truncated = true
		return nil
	}

	// Check depth limit
	if depth > maxDepth {
		return nil
	}

	currentPath := basePath
	if relativePath != "" {
		currentPath = filepath.Join(basePath, relativePath)
	}

	entries, err := os.ReadDir(currentPath)
	if err != nil {
		return nil // Skip unreadable directories
	}

	// Sort entries: directories first, then files
	sort.Slice(entries, func(i, j int) bool {
		iDir := entries[i].IsDir()
		jDir := entries[j].IsDir()
		if iDir != jDir {
			return iDir
		}
		return entries[i].Name() < entries[j].Name()
	})

	// Build indent prefix
	indent := strings.Repeat("  ", depth)

	for _, entry := range entries {
		if *fileCount >= maxFiles {
			*truncated = true
			return nil
		}

		name := entry.Name()

		// Skip hidden files at root level (except specific important ones)
		if depth == 0 && strings.HasPrefix(name, ".") {
			// Allow certain important hidden files/dirs
			if name != ".github" && name != ".gitlab-ci.yml" && name != ".gitignore" && name != ".env.example" {
				continue
			}
		}

		// Skip excluded directories
		if entry.IsDir() && excludeMap[name] {
			continue
		}

		*fileCount++

		if entry.IsDir() {
			content.WriteString(fmt.Sprintf("%s%s/\n", indent, name))

			// Recurse into subdirectory
			subPath := name
			if relativePath != "" {
				subPath = filepath.Join(relativePath, name)
			}
			err := walkDirectory(ctx, basePath, subPath, depth+1, maxDepth, excludeMap, content, fileCount, maxFiles, truncated)
			if err != nil {
				return err
			}
		} else {
			content.WriteString(fmt.Sprintf("%s%s\n", indent, name))
		}
	}

	return nil
}

// RecentFilesGatherer collects recently modified files.
type RecentFilesGatherer struct {
	*BaseGatherer
}

// NewRecentFilesGatherer creates a new RecentFilesGatherer.
func NewRecentFilesGatherer() *RecentFilesGatherer {
	g := &RecentFilesGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypeRecentFiles,
			"Recent Files",
			"Lists recently modified files in the project",
		),
	}
	// Set default options
	g.options = map[string]interface{}{
		"max_files":     20,
		"max_age_hours": 24,
		"exclude": []string{
			"node_modules",
			".git",
			"__pycache__",
			"vendor",
			".venv",
			"venv",
		},
	}
	return g
}

// fileInfo holds file path and modification time.
type fileInfo struct {
	path    string
	modTime int64
}

// Gather collects recently modified files.
func (g *RecentFilesGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("project path is required")
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Get options
	maxFiles := g.GetIntOption("max_files", 20)
	excludeDirs := g.GetStringSliceOption("exclude", []string{})

	// Build exclude map
	excludeMap := make(map[string]bool)
	for _, dir := range excludeDirs {
		excludeMap[dir] = true
	}

	// Collect all files with modification times
	var files []fileInfo
	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories from results but check if we should descend
		if info.IsDir() {
			name := info.Name()
			if excludeMap[name] || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		relPath, _ := filepath.Rel(projectPath, path)
		files = append(files, fileInfo{
			path:    relPath,
			modTime: info.ModTime().Unix(),
		})

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort by modification time (most recent first)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})

	// Take only the most recent files
	if len(files) > maxFiles {
		files = files[:maxFiles]
	}

	var content strings.Builder
	content.WriteString(fmt.Sprintf("Recently modified files (top %d):\n\n", len(files)))

	for _, f := range files {
		content.WriteString(fmt.Sprintf("  %s\n", f.path))
	}

	if len(files) == 0 {
		content.WriteString("  No recently modified files found\n")
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	return g.CreateContextItem("recent files", finalContent, tokenCount)
}
