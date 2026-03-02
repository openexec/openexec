// Package context provides automatic context gathering and injection for AI agent sessions.
package context

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PackageInfoGatherer collects package/dependency information from project files.
type PackageInfoGatherer struct {
	*BaseGatherer
}

// NewPackageInfoGatherer creates a new PackageInfoGatherer.
func NewPackageInfoGatherer() *PackageInfoGatherer {
	g := &PackageInfoGatherer{
		BaseGatherer: NewBaseGatherer(
			ContextTypePackageInfo,
			"Package Info",
			"Reads package.json, go.mod, requirements.txt, Cargo.toml, and other package files",
		),
	}
	// Set default file paths for package info
	g.filePaths = []string{
		"package.json",
		"go.mod",
		"requirements.txt",
		"Cargo.toml",
		"pom.xml",
		"pyproject.toml",
		"Gemfile",
		"composer.json",
	}
	return g
}

// Gather collects package information from various package manager files.
func (g *PackageInfoGatherer) Gather(ctx context.Context, projectPath string) (*ContextItem, error) {
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

		// Check if file exists
		info, err := os.Stat(fullPath)
		if err != nil || info.IsDir() {
			continue
		}

		// Read and process file
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		fileContent := strings.TrimSpace(string(data))
		if fileContent == "" {
			continue
		}

		// Process based on file type
		processed := processPackageFile(relPath, fileContent)
		if processed == "" {
			continue
		}

		// Add file header and content
		if content.Len() > 0 {
			content.WriteString("\n\n")
		}
		content.WriteString(fmt.Sprintf("## %s\n\n", relPath))
		content.WriteString(processed)
		foundFiles = append(foundFiles, relPath)

		// Check context cancellation between files
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
	}

	if len(foundFiles) == 0 {
		return nil, fmt.Errorf("no package files found in %s", projectPath)
	}

	// Truncate if needed
	finalContent := TruncateToTokenLimit(content.String(), g.MaxTokens())
	tokenCount := EstimateTokens(finalContent)

	source := fmt.Sprintf("files: %s", strings.Join(foundFiles, ", "))
	return g.CreateContextItem(source, finalContent, tokenCount)
}

// processPackageFile extracts relevant information from a package file.
func processPackageFile(filename, content string) string {
	switch {
	case filename == "package.json":
		return processPackageJSON(content)
	case filename == "go.mod":
		return processGoMod(content)
	case filename == "requirements.txt":
		return processRequirementsTxt(content)
	case filename == "Cargo.toml":
		return processCargoToml(content)
	case filename == "pyproject.toml":
		return processPyprojectToml(content)
	default:
		// For other files, return truncated content
		if len(content) > 2000 {
			return content[:2000] + "\n... [truncated]"
		}
		return content
	}
}

// processPackageJSON extracts key info from package.json.
func processPackageJSON(content string) string {
	var pkg map[string]interface{}
	if err := json.Unmarshal([]byte(content), &pkg); err != nil {
		// Return raw content if parsing fails
		if len(content) > 1500 {
			return content[:1500] + "\n... [truncated]"
		}
		return content
	}

	var result strings.Builder

	// Name and version
	if name, ok := pkg["name"].(string); ok {
		result.WriteString(fmt.Sprintf("Name: %s\n", name))
	}
	if version, ok := pkg["version"].(string); ok {
		result.WriteString(fmt.Sprintf("Version: %s\n", version))
	}
	if desc, ok := pkg["description"].(string); ok && desc != "" {
		result.WriteString(fmt.Sprintf("Description: %s\n", desc))
	}

	// Scripts (useful for understanding build commands)
	if scripts, ok := pkg["scripts"].(map[string]interface{}); ok && len(scripts) > 0 {
		result.WriteString("\nScripts:\n")
		for name, cmd := range scripts {
			if cmdStr, ok := cmd.(string); ok {
				result.WriteString(fmt.Sprintf("  %s: %s\n", name, cmdStr))
			}
		}
	}

	// Dependencies (count only to save tokens)
	if deps, ok := pkg["dependencies"].(map[string]interface{}); ok && len(deps) > 0 {
		result.WriteString(fmt.Sprintf("\nDependencies: %d packages\n", len(deps)))
		// Show first 10 dependencies
		count := 0
		for name, version := range deps {
			if count >= 10 {
				result.WriteString(fmt.Sprintf("  ... and %d more\n", len(deps)-10))
				break
			}
			if versionStr, ok := version.(string); ok {
				result.WriteString(fmt.Sprintf("  %s: %s\n", name, versionStr))
			}
			count++
		}
	}

	// DevDependencies (count only)
	if devDeps, ok := pkg["devDependencies"].(map[string]interface{}); ok && len(devDeps) > 0 {
		result.WriteString(fmt.Sprintf("\nDev Dependencies: %d packages\n", len(devDeps)))
	}

	return result.String()
}

// processGoMod extracts key info from go.mod.
func processGoMod(content string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	var deps []string
	inRequire := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "module ") {
			result.WriteString(fmt.Sprintf("Module: %s\n", strings.TrimPrefix(line, "module ")))
		} else if strings.HasPrefix(line, "go ") {
			result.WriteString(fmt.Sprintf("Go version: %s\n", strings.TrimPrefix(line, "go ")))
		} else if line == "require (" {
			inRequire = true
		} else if line == ")" && inRequire {
			inRequire = false
		} else if inRequire && !strings.HasPrefix(line, "//") {
			deps = append(deps, line)
		} else if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			// Single-line require
			deps = append(deps, strings.TrimPrefix(line, "require "))
		}
	}

	if len(deps) > 0 {
		result.WriteString(fmt.Sprintf("\nDependencies: %d modules\n", len(deps)))
		// Show first 10
		for i, dep := range deps {
			if i >= 10 {
				result.WriteString(fmt.Sprintf("  ... and %d more\n", len(deps)-10))
				break
			}
			result.WriteString(fmt.Sprintf("  %s\n", dep))
		}
	}

	return result.String()
}

// processRequirementsTxt extracts key info from requirements.txt.
func processRequirementsTxt(content string) string {
	lines := strings.Split(content, "\n")
	var deps []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		deps = append(deps, line)
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("Python dependencies: %d packages\n\n", len(deps)))

	for i, dep := range deps {
		if i >= 15 {
			result.WriteString(fmt.Sprintf("... and %d more\n", len(deps)-15))
			break
		}
		result.WriteString(fmt.Sprintf("  %s\n", dep))
	}

	return result.String()
}

// processCargoToml extracts key info from Cargo.toml.
func processCargoToml(content string) string {
	// Simple TOML parsing for key fields
	lines := strings.Split(content, "\n")
	var result strings.Builder
	var depCount int
	section := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}

		if section == "package" {
			if strings.HasPrefix(line, "name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					result.WriteString(fmt.Sprintf("Name: %s\n", strings.Trim(strings.TrimSpace(parts[1]), "\"")))
				}
			} else if strings.HasPrefix(line, "version") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					result.WriteString(fmt.Sprintf("Version: %s\n", strings.Trim(strings.TrimSpace(parts[1]), "\"")))
				}
			}
		}

		if section == "dependencies" || section == "dev-dependencies" {
			depCount++
		}
	}

	if depCount > 0 {
		result.WriteString(fmt.Sprintf("\nDependencies: %d crates\n", depCount))
	}

	return result.String()
}

// processPyprojectToml extracts key info from pyproject.toml.
func processPyprojectToml(content string) string {
	lines := strings.Split(content, "\n")
	var result strings.Builder
	section := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}

		if section == "project" || section == "tool.poetry" {
			if strings.HasPrefix(line, "name") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					result.WriteString(fmt.Sprintf("Name: %s\n", strings.Trim(strings.TrimSpace(parts[1]), "\"")))
				}
			} else if strings.HasPrefix(line, "version") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					result.WriteString(fmt.Sprintf("Version: %s\n", strings.Trim(strings.TrimSpace(parts[1]), "\"")))
				}
			} else if strings.HasPrefix(line, "description") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					result.WriteString(fmt.Sprintf("Description: %s\n", strings.Trim(strings.TrimSpace(parts[1]), "\"")))
				}
			}
		}
	}

	return result.String()
}
