package toolset

import (
	"testing"
)

func TestToolset_HasTool(t *testing.T) {
	ts := &Toolset{
		Name:  "test",
		Tools: []string{"read_file", "write_file"},
	}

	tests := []struct {
		tool string
		want bool
	}{
		{"read_file", true},
		{"write_file", true},
		{"delete_file", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			if got := ts.HasTool(tc.tool); got != tc.want {
				t.Errorf("HasTool(%q) = %v, want %v", tc.tool, got, tc.want)
			}
		})
	}
}

func TestToolset_AppliesToPhase(t *testing.T) {
	ts := &Toolset{
		Name:   "test",
		Phases: []string{"implement", "fix_lint"},
	}

	tests := []struct {
		phase string
		want  bool
	}{
		{"implement", true},
		{"fix_lint", true},
		{"review", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.phase, func(t *testing.T) {
			if got := ts.AppliesToPhase(tc.phase); got != tc.want {
				t.Errorf("AppliesToPhase(%q) = %v, want %v", tc.phase, got, tc.want)
			}
		})
	}
}

func TestRegistry_NewRegistry(t *testing.T) {
	r := NewRegistry()

	// Check default toolsets are registered
	defaultNames := []string{
		"repo_readonly",
		"coding_backend",
		"coding_frontend",
		"debug_ci",
		"docs_research",
		"release_ops",
	}

	for _, name := range defaultNames {
		ts, ok := r.Get(name)
		if !ok {
			t.Errorf("default toolset %q not found", name)
			continue
		}
		if ts.Name != name {
			t.Errorf("toolset name = %q, want %q", ts.Name, name)
		}
		if !r.IsEnabled(name) {
			t.Errorf("default toolset %q should be enabled", name)
		}
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	custom := &Toolset{
		Name:        "custom",
		Description: "Custom toolset",
		Tools:       []string{"custom_tool"},
		Phases:      []string{"custom_phase"},
		RiskLevel:   RiskMedium,
	}

	err := r.Register(custom)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	ts, ok := r.Get("custom")
	if !ok {
		t.Fatal("custom toolset not found after register")
	}
	if ts.Description != "Custom toolset" {
		t.Errorf("description = %q, want %q", ts.Description, "Custom toolset")
	}
}

func TestRegistry_RegisterError(t *testing.T) {
	r := NewRegistry()

	err := r.Register(&Toolset{}) // No name
	if err == nil {
		t.Error("expected error for empty toolset name")
	}
}

func TestRegistry_EnableDisable(t *testing.T) {
	r := NewRegistry()

	// Initially enabled
	if !r.IsEnabled("repo_readonly") {
		t.Error("repo_readonly should be enabled by default")
	}

	// Disable
	err := r.Disable("repo_readonly")
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}
	if r.IsEnabled("repo_readonly") {
		t.Error("repo_readonly should be disabled")
	}

	// Enable again
	err = r.Enable("repo_readonly")
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}
	if !r.IsEnabled("repo_readonly") {
		t.Error("repo_readonly should be enabled again")
	}

	// Error on unknown toolset
	err = r.Disable("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent toolset")
	}
}

func TestRegistry_ListEnabled(t *testing.T) {
	r := NewRegistry()

	r.Disable("release_ops")

	enabled := r.ListEnabled()

	// Should have all default toolsets except release_ops
	foundReleaseOps := false
	for _, ts := range enabled {
		if ts.Name == "release_ops" {
			foundReleaseOps = true
		}
	}
	if foundReleaseOps {
		t.Error("release_ops should not be in enabled list")
	}

	// Should have repo_readonly
	foundRepoReadonly := false
	for _, ts := range enabled {
		if ts.Name == "repo_readonly" {
			foundRepoReadonly = true
		}
	}
	if !foundRepoReadonly {
		t.Error("repo_readonly should be in enabled list")
	}
}

func TestRegistry_GetToolsForPhase(t *testing.T) {
	r := NewRegistry()

	tools := r.GetToolsForPhase("gather_context")

	// Should include read_file from repo_readonly
	foundReadFile := false
	for _, tool := range tools {
		if tool == "read_file" {
			foundReadFile = true
			break
		}
	}
	if !foundReadFile {
		t.Error("read_file should be available in gather_context phase")
	}

	// Should include web_fetch from docs_research
	foundWebFetch := false
	for _, tool := range tools {
		if tool == "web_fetch" {
			foundWebFetch = true
			break
		}
	}
	if !foundWebFetch {
		t.Error("web_fetch should be available in gather_context phase")
	}
}

func TestRegistry_GetToolsetForTool(t *testing.T) {
	r := NewRegistry()

	ts := r.GetToolsetForTool("read_file")
	if ts == nil {
		t.Fatal("expected toolset for read_file")
	}

	// read_file is in multiple low-risk toolsets (repo_readonly, docs_research)
	// Just verify it's a low-risk toolset
	if ts.RiskLevel != RiskLow {
		t.Errorf("expected low risk toolset, got %s", ts.RiskLevel)
	}
	if !ts.HasTool("read_file") {
		t.Error("returned toolset should contain read_file")
	}

	// Unknown tool
	ts = r.GetToolsetForTool("unknown_tool")
	if ts != nil {
		t.Error("expected nil for unknown tool")
	}
}

func TestRegistry_IsToolAllowed(t *testing.T) {
	r := NewRegistry()

	if !r.IsToolAllowed("read_file") {
		t.Error("read_file should be allowed")
	}

	if r.IsToolAllowed("unknown_tool") {
		t.Error("unknown_tool should not be allowed")
	}

	// Disable the toolset containing read_file
	r.Disable("repo_readonly")
	r.Disable("coding_backend")
	r.Disable("coding_frontend")
	r.Disable("debug_ci")
	r.Disable("docs_research")

	if r.IsToolAllowed("read_file") {
		t.Error("read_file should not be allowed when its toolsets are disabled")
	}
}

func TestRegistry_GetRiskLevel(t *testing.T) {
	r := NewRegistry()

	// Low risk tool
	risk := r.GetRiskLevel("read_file")
	if risk != RiskLow {
		t.Errorf("read_file risk = %s, want low", risk)
	}

	// Medium risk tool
	risk = r.GetRiskLevel("write_file")
	if risk != RiskMedium {
		t.Errorf("write_file risk = %s, want medium", risk)
	}

	// High risk tool
	risk = r.GetRiskLevel("git_push")
	if risk != RiskHigh {
		t.Errorf("git_push risk = %s, want high", risk)
	}

	// Unknown tool (defaults to high)
	risk = r.GetRiskLevel("unknown_tool")
	if risk != RiskHigh {
		t.Errorf("unknown_tool risk = %s, want high", risk)
	}
}

func TestRegistry_RequiresApproval(t *testing.T) {
	r := NewRegistry()

	// Low risk tools don't require approval
	if r.RequiresApproval("read_file") {
		t.Error("read_file should not require approval")
	}

	// Medium/high risk tools require approval
	if !r.RequiresApproval("write_file") {
		t.Error("write_file should require approval")
	}

	// Unknown tools require approval
	if !r.RequiresApproval("unknown_tool") {
		t.Error("unknown_tool should require approval")
	}
}

func TestSelector_SelectForPhase(t *testing.T) {
	r := NewRegistry()
	s := NewSelector(r)

	ts := s.SelectForPhase("implement")
	if ts == nil {
		t.Fatal("expected toolset for implement phase")
	}
	if !ts.AppliesToPhase("implement") {
		t.Errorf("selected toolset %s does not apply to implement phase", ts.Name)
	}

	// Unknown phase
	ts = s.SelectForPhase("nonexistent")
	if ts != nil {
		t.Error("expected nil for nonexistent phase")
	}
}

func TestSelector_SelectForTask(t *testing.T) {
	r := NewRegistry()
	s := NewSelector(r)

	tests := []struct {
		task     string
		contains string
	}{
		{"read the main.go file", "repo_readonly"},
		{"implement a new feature", "coding_backend"},
		{"fix the CI pipeline", "debug_ci"},
		{"create frontend component", "coding_frontend"},
		{"research best practices", "docs_research"},
		{"release version 1.0", "release_ops"},
	}

	for _, tc := range tests {
		t.Run(tc.task, func(t *testing.T) {
			toolsets := s.SelectForTask(tc.task)

			found := false
			for _, ts := range toolsets {
				if ts.Name == tc.contains {
					found = true
					break
				}
			}
			if !found {
				names := make([]string, len(toolsets))
				for i, ts := range toolsets {
					names[i] = ts.Name
				}
				t.Errorf("expected %s in selection, got %v", tc.contains, names)
			}
		})
	}
}

func TestSelector_SelectForTaskDefault(t *testing.T) {
	r := NewRegistry()
	s := NewSelector(r)

	// Task with no matching keywords should default to repo_readonly
	toolsets := s.SelectForTask("something completely random xyz123")

	if len(toolsets) == 0 {
		t.Fatal("expected at least one toolset")
	}
	if toolsets[0].Name != "repo_readonly" {
		t.Errorf("expected repo_readonly as default, got %s", toolsets[0].Name)
	}
}
