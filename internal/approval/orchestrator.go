// Package approval provides tool approval workflow management for the OpenExec orchestrator.
// This file implements orchestrator file edit detection for risk level escalation.
package approval

import (
	"encoding/json"
	"strings"
)

// OrchestratorEditChecker detects when tool operations target orchestrator files
// and escalates the risk level to critical.
type OrchestratorEditChecker interface {
	// IsOrchestratorPath checks if a path belongs to the orchestrator.
	IsOrchestratorPath(path string) bool
}

// OrchestratorRiskEscalator provides risk level escalation for orchestrator edits.
type OrchestratorRiskEscalator struct {
	checker OrchestratorEditChecker
}

// NewOrchestratorRiskEscalator creates a new OrchestratorRiskEscalator.
// If checker is nil, orchestrator path detection is disabled.
func NewOrchestratorRiskEscalator(checker OrchestratorEditChecker) *OrchestratorRiskEscalator {
	return &OrchestratorRiskEscalator{
		checker: checker,
	}
}

// EscalateRiskLevel checks if a tool call should have its risk level escalated
// due to targeting orchestrator files. Returns the effective risk level and
// a boolean indicating if escalation occurred.
func (e *OrchestratorRiskEscalator) EscalateRiskLevel(toolName, toolInput string, baseRiskLevel RiskLevel) (RiskLevel, bool) {
	// If no checker is configured, return the base risk level
	if e.checker == nil {
		return baseRiskLevel, false
	}

	// Only escalate for file modification tools
	if !isFileModificationTool(toolName) {
		return baseRiskLevel, false
	}

	// Extract paths from tool input
	paths := extractPathsFromToolInput(toolName, toolInput)
	if len(paths) == 0 {
		return baseRiskLevel, false
	}

	// Check if any path is an orchestrator path
	for _, path := range paths {
		if e.checker.IsOrchestratorPath(path) {
			// Escalate to critical if not already critical
			if baseRiskLevel.Priority() < RiskLevelCritical.Priority() {
				return RiskLevelCritical, true
			}
			return RiskLevelCritical, false // Already critical
		}
	}

	return baseRiskLevel, false
}

// IsOrchestratorEdit checks if a tool call is modifying orchestrator files.
func (e *OrchestratorRiskEscalator) IsOrchestratorEdit(toolName, toolInput string) bool {
	if e.checker == nil {
		return false
	}

	if !isFileModificationTool(toolName) {
		return false
	}

	paths := extractPathsFromToolInput(toolName, toolInput)
	for _, path := range paths {
		if e.checker.IsOrchestratorPath(path) {
			return true
		}
	}

	return false
}

// isFileModificationTool checks if a tool can modify files.
var fileModificationTools = map[string]bool{
	"write_file":       true,
	"edit_file":        true,
	"delete_file":      true,
	"create_directory": true,
	"git_apply_patch":  true,
	"rename_file":      true,
	"move_file":        true,
	"copy_file":        true,
}

func isFileModificationTool(toolName string) bool {
	return fileModificationTools[toolName]
}

// extractPathsFromToolInput extracts file paths from tool input JSON.
// Different tools have different input schemas.
func extractPathsFromToolInput(toolName, toolInput string) []string {
	if toolInput == "" {
		return nil
	}

	var input map[string]interface{}
	if err := json.Unmarshal([]byte(toolInput), &input); err != nil {
		return nil
	}

	var paths []string

	switch toolName {
	case "write_file", "edit_file", "delete_file", "read_file":
		// Standard file operations: {"path": "..."}
		if path, ok := input["path"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		// Some tools use "file_path" instead
		if path, ok := input["file_path"].(string); ok && path != "" {
			paths = append(paths, path)
		}

	case "create_directory":
		// Directory creation: {"path": "..."} or {"directory": "..."}
		if path, ok := input["path"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["directory"].(string); ok && path != "" {
			paths = append(paths, path)
		}

	case "git_apply_patch":
		// Git patch operations: may have target files embedded in patch
		// or explicit target directory
		if path, ok := input["target_path"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["directory"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["working_dir"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		// Also check for file paths embedded in the patch content
		if patch, ok := input["patch"].(string); ok && patch != "" {
			paths = append(paths, extractPathsFromPatch(patch)...)
		}

	case "rename_file", "move_file":
		// Operations with source and destination: {"source": "...", "destination": "..."}
		if path, ok := input["source"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["destination"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["from"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["to"].(string); ok && path != "" {
			paths = append(paths, path)
		}

	case "copy_file":
		// Copy operation: {"source": "...", "destination": "..."}
		if path, ok := input["source"].(string); ok && path != "" {
			paths = append(paths, path)
		}
		if path, ok := input["destination"].(string); ok && path != "" {
			paths = append(paths, path)
		}
	}

	return paths
}

// extractPathsFromPatch extracts file paths from a unified diff patch.
func extractPathsFromPatch(patch string) []string {
	var paths []string
	seenPaths := make(map[string]bool)

	lines := strings.Split(patch, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match --- a/path and +++ b/path lines
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			// Extract path after "--- " or "+++ "
			path := strings.TrimPrefix(line, "--- ")
			path = strings.TrimPrefix(path, "+++ ")

			// Handle a/ or b/ prefix
			path = strings.TrimPrefix(path, "a/")
			path = strings.TrimPrefix(path, "b/")

			// Skip /dev/null (new or deleted files)
			if path == "/dev/null" || path == "" {
				continue
			}

			// Handle paths with timestamps (e.g., "file.go	2024-01-01 00:00:00")
			if idx := strings.Index(path, "\t"); idx != -1 {
				path = path[:idx]
			}

			if !seenPaths[path] {
				paths = append(paths, path)
				seenPaths[path] = true
			}
		}

		// Also match diff --git lines
		if strings.HasPrefix(line, "diff --git ") {
			// Extract paths from "diff --git a/path b/path"
			parts := strings.Split(line, " ")
			for _, part := range parts {
				if strings.HasPrefix(part, "a/") {
					path := strings.TrimPrefix(part, "a/")
					if !seenPaths[path] {
						paths = append(paths, path)
						seenPaths[path] = true
					}
				}
				if strings.HasPrefix(part, "b/") {
					path := strings.TrimPrefix(part, "b/")
					if !seenPaths[path] {
						paths = append(paths, path)
						seenPaths[path] = true
					}
				}
			}
		}
	}

	return paths
}

// OrchestratorEditReason returns a human-readable reason for the risk escalation.
func OrchestratorEditReason(toolName string, paths []string) string {
	if len(paths) == 0 {
		return "Orchestrator file modification detected"
	}
	if len(paths) == 1 {
		return "Orchestrator file modification: " + paths[0]
	}
	return "Orchestrator file modifications: " + strings.Join(paths, ", ")
}
