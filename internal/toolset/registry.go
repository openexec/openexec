// Package toolset defines curated tool groups for different operational contexts.
// Instead of exposing a flat list of tools, toolsets group tools by use case
// and risk level, enabling appropriate tool availability per mode and phase.
package toolset

import (
	"fmt"
	"sync"
)

// RiskLevel indicates the risk associated with a toolset.
type RiskLevel string

const (
	// RiskLow indicates tools that only read data, no side effects.
	RiskLow RiskLevel = "low"

	// RiskMedium indicates tools that can modify files or run commands.
	RiskMedium RiskLevel = "medium"

	// RiskHigh indicates tools that can make external changes (push, deploy).
	RiskHigh RiskLevel = "high"
)

// Toolset defines a curated group of tools for a specific use case.
type Toolset struct {
	// Name is the unique identifier for this toolset.
	Name string `json:"name"`

	// Description explains what this toolset is for.
	Description string `json:"description"`

	// Tools is the list of tool names included in this toolset.
	Tools []string `json:"tools"`

	// Phases lists the blueprint phases where this toolset applies.
	Phases []string `json:"phases"`

	// RiskLevel indicates the risk associated with this toolset.
	RiskLevel RiskLevel `json:"risk_level"`

	// RequiresApproval indicates if tools in this set require user approval.
	RequiresApproval bool `json:"requires_approval"`
}

// HasTool checks if the toolset includes a specific tool.
func (t *Toolset) HasTool(toolName string) bool {
	for _, tool := range t.Tools {
		if tool == toolName {
			return true
		}
	}
	return false
}

// AppliesToPhase checks if the toolset applies to a specific blueprint phase.
func (t *Toolset) AppliesToPhase(phase string) bool {
	for _, p := range t.Phases {
		if p == phase {
			return true
		}
	}
	return false
}

// DefaultToolsets defines the standard toolsets available in OpenExec.
var DefaultToolsets = map[string]*Toolset{
	"repo_readonly": {
		Name:        "repo_readonly",
		Description: "Read-only access to repository files and git status",
		Tools: []string{
			"read_file",
			"glob",
			"grep",
			"git_status",
			"git_diff",
			"git_log",
			"list_directory",
		},
		Phases:           []string{"gather_context", "review", "analyze"},
		RiskLevel:        RiskLow,
		RequiresApproval: false,
	},
	"coding_backend": {
		Name:        "coding_backend",
		Description: "Backend development tools for reading, writing, and testing code",
		Tools: []string{
			"read_file",
			"write_file",
			"git_apply_patch",
			"run_shell_command",
			"glob",
			"grep",
		},
		Phases:           []string{"implement", "fix_lint", "fix_tests"},
		RiskLevel:        RiskMedium,
		RequiresApproval: true,
	},
	"coding_frontend": {
		Name:        "coding_frontend",
		Description: "Frontend development tools including npm/yarn commands",
		Tools: []string{
			"read_file",
			"write_file",
			"git_apply_patch",
			"run_shell_command",
			"glob",
			"grep",
			"npm_run",
		},
		Phases:           []string{"implement", "fix_lint", "fix_tests"},
		RiskLevel:        RiskMedium,
		RequiresApproval: true,
	},
	"debug_ci": {
		Name:        "debug_ci",
		Description: "Tools for debugging CI/CD failures",
		Tools: []string{
			"read_file",
			"run_shell_command",
			"git_log",
			"git_diff",
			"ci_status",
			"glob",
			"grep",
		},
		Phases:           []string{"fix_ci", "diagnose"},
		RiskLevel:        RiskMedium,
		RequiresApproval: true,
	},
	"docs_research": {
		Name:        "docs_research",
		Description: "Tools for documentation and research tasks",
		Tools: []string{
			"read_file",
			"glob",
			"grep",
			"web_fetch",
		},
		Phases:           []string{"gather_context", "research"},
		RiskLevel:        RiskLow,
		RequiresApproval: false,
	},
	"release_ops": {
		Name:        "release_ops",
		Description: "Tools for release operations (git push, tagging)",
		Tools: []string{
			"git_tag",
			"git_push",
			"changelog_update",
			"run_shell_command",
		},
		Phases:           []string{"finalize", "release"},
		RiskLevel:        RiskHigh,
		RequiresApproval: true,
	},
}

// Registry manages available toolsets and provides filtering.
type Registry struct {
	mu       sync.RWMutex
	toolsets map[string]*Toolset
	enabled  map[string]bool
}

// NewRegistry creates a new toolset registry with default toolsets.
func NewRegistry() *Registry {
	r := &Registry{
		toolsets: make(map[string]*Toolset),
		enabled:  make(map[string]bool),
	}

	// Register default toolsets
	for name, ts := range DefaultToolsets {
		r.toolsets[name] = ts
		r.enabled[name] = true
	}

	return r
}

// Register adds a custom toolset to the registry.
func (r *Registry) Register(ts *Toolset) error {
	if ts.Name == "" {
		return fmt.Errorf("toolset name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.toolsets[ts.Name] = ts
	r.enabled[ts.Name] = true
	return nil
}

// Get retrieves a toolset by name.
func (r *Registry) Get(name string) (*Toolset, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ts, ok := r.toolsets[name]
	return ts, ok
}

// Enable enables a toolset.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.toolsets[name]; !ok {
		return fmt.Errorf("toolset not found: %s", name)
	}
	r.enabled[name] = true
	return nil
}

// Disable disables a toolset.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.toolsets[name]; !ok {
		return fmt.Errorf("toolset not found: %s", name)
	}
	r.enabled[name] = false
	return nil
}

// IsEnabled checks if a toolset is enabled.
func (r *Registry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.enabled[name]
}

// ListEnabled returns all enabled toolsets.
func (r *Registry) ListEnabled() []*Toolset {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*Toolset
	for name, ts := range r.toolsets {
		if r.enabled[name] {
			result = append(result, ts)
		}
	}
	return result
}

// ListAll returns all registered toolsets.
func (r *Registry) ListAll() []*Toolset {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*Toolset, 0, len(r.toolsets))
	for _, ts := range r.toolsets {
		result = append(result, ts)
	}
	return result
}

// GetToolsForPhase returns all tools available for a specific blueprint phase.
func (r *Registry) GetToolsForPhase(phase string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	toolSet := make(map[string]bool)
	for name, ts := range r.toolsets {
		if !r.enabled[name] {
			continue
		}
		if ts.AppliesToPhase(phase) {
			for _, tool := range ts.Tools {
				toolSet[tool] = true
			}
		}
	}

	result := make([]string, 0, len(toolSet))
	for tool := range toolSet {
		result = append(result, tool)
	}
	return result
}

// GetToolsetForTool finds which enabled toolset contains a specific tool.
// When a tool appears in multiple toolsets, returns the one with lowest risk level.
// Returns nil if no enabled toolset contains the tool.
func (r *Registry) GetToolsetForTool(toolName string) *Toolset {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var best *Toolset
	for name, ts := range r.toolsets {
		if !r.enabled[name] {
			continue
		}
		if ts.HasTool(toolName) {
			if best == nil || riskLevelOrder(ts.RiskLevel) < riskLevelOrder(best.RiskLevel) {
				best = ts
			}
		}
	}
	return best
}

// riskLevelOrder returns a numeric order for risk levels (lower = less risky).
func riskLevelOrder(r RiskLevel) int {
	switch r {
	case RiskLow:
		return 0
	case RiskMedium:
		return 1
	case RiskHigh:
		return 2
	default:
		return 3
	}
}

// IsToolAllowed checks if a tool is allowed based on enabled toolsets.
func (r *Registry) IsToolAllowed(toolName string) bool {
	return r.GetToolsetForTool(toolName) != nil
}

// GetRiskLevel returns the risk level for a tool based on its toolset.
func (r *Registry) GetRiskLevel(toolName string) RiskLevel {
	ts := r.GetToolsetForTool(toolName)
	if ts == nil {
		return RiskHigh // Unknown tools are high risk
	}
	return ts.RiskLevel
}

// RequiresApproval checks if a tool requires user approval.
func (r *Registry) RequiresApproval(toolName string) bool {
	ts := r.GetToolsetForTool(toolName)
	if ts == nil {
		return true // Unknown tools require approval
	}
	return ts.RequiresApproval
}

// Selector helps choose the appropriate toolset for a task.
type Selector struct {
	registry *Registry
}

// NewSelector creates a new toolset selector.
func NewSelector(registry *Registry) *Selector {
	return &Selector{registry: registry}
}

// SelectForPhase returns the best toolset for a blueprint phase.
func (s *Selector) SelectForPhase(phase string) *Toolset {
	toolsets := s.registry.ListEnabled()

	for _, ts := range toolsets {
		if ts.AppliesToPhase(phase) {
			return ts
		}
	}
	return nil
}

// SelectForTask returns toolsets appropriate for a task description.
// Uses simple keyword matching; can be enhanced with local LLM.
func (s *Selector) SelectForTask(task string) []*Toolset {
	keywords := map[string][]string{
		"repo_readonly":   {"search code", "find in repo", "explore repository", "analyze structure", "read"},
		"coding_backend":  {"implement", "backend", "server-side", "database", "api endpoint"},
		"coding_frontend": {"frontend", "react", "css", "html", "component", "ui"},
		"debug_ci":        {"ci pipeline", "build failure", "test failure", "debug ci"},
		"docs_research":   {"documentation", "research", "explain architecture", "summarize"},
		"release_ops":     {"release", "deploy", "push to origin", "publish", "tag release"},
	}

	var result []*Toolset
	for name, kws := range keywords {
		ts, ok := s.registry.Get(name)
		if !ok || !s.registry.IsEnabled(name) {
			continue
		}
		for _, kw := range kws {
			if containsIgnoreCase(task, kw) {
				result = append(result, ts)
				break
			}
		}
	}

	// Default to repo_readonly if no match
	if len(result) == 0 {
		if ts, ok := s.registry.Get("repo_readonly"); ok {
			result = append(result, ts)
		}
	}

	return result
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	sLower := make([]byte, len(s))
	substrLower := make([]byte, len(substr))

	for i, c := range []byte(s) {
		if c >= 'A' && c <= 'Z' {
			sLower[i] = c + 32
		} else {
			sLower[i] = c
		}
	}
	for i, c := range []byte(substr) {
		if c >= 'A' && c <= 'Z' {
			substrLower[i] = c + 32
		} else {
			substrLower[i] = c
		}
	}

	return bytesContains(sLower, substrLower)
}

// bytesContains checks if a contains b.
func bytesContains(a, b []byte) bool {
	if len(b) > len(a) {
		return false
	}
	for i := 0; i <= len(a)-len(b); i++ {
		match := true
		for j := 0; j < len(b); j++ {
			if a[i+j] != b[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
