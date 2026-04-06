package loop

import (
	"testing"
)

// TestBuildAPIToolDefinitions_AllKnownTools verifies the unfiltered path
// returns every tool the API runner knows about, in deterministic order.
func TestBuildAPIToolDefinitions_AllKnownTools(t *testing.T) {
	tools := BuildAPIToolDefinitions()
	if len(tools) == 0 {
		t.Fatal("BuildAPIToolDefinitions returned empty list")
	}

	// Should match the size of the canonical map.
	if len(tools) != len(apiToolDefinitions) {
		t.Errorf("BuildAPIToolDefinitions returned %d tools, want %d", len(tools), len(apiToolDefinitions))
	}

	// Sanity: the tools we definitely expect to exist today.
	expected := map[string]bool{
		"read_file":         false,
		"write_file":        false,
		"run_shell_command": false,
		"git_apply_patch":   false,
	}
	for _, tool := range tools {
		if _, ok := expected[tool.Name]; ok {
			expected[tool.Name] = true
		}
	}
	for name, found := range expected {
		if !found {
			t.Errorf("expected tool %q in unfiltered list, not present", name)
		}
	}

	// Determinism: a second call should return the same order.
	tools2 := BuildAPIToolDefinitions()
	for i := range tools {
		if tools[i].Name != tools2[i].Name {
			t.Errorf("non-deterministic ordering at index %d: first=%q second=%q", i, tools[i].Name, tools2[i].Name)
		}
	}
}

// TestBuildAPIToolDefinitionsFor_EmptyInput returns no tools when given an
// empty name list. Important: the caller code falls back to the unfiltered
// list when this returns nil/empty, but the filter itself should not panic
// or return anything for an empty input.
func TestBuildAPIToolDefinitionsFor_EmptyInput(t *testing.T) {
	tools := BuildAPIToolDefinitionsFor(nil)
	if len(tools) != 0 {
		t.Errorf("expected empty result for nil input, got %d tools", len(tools))
	}

	tools = BuildAPIToolDefinitionsFor([]string{})
	if len(tools) != 0 {
		t.Errorf("expected empty result for empty slice, got %d tools", len(tools))
	}
}

// TestBuildAPIToolDefinitionsFor_SingleTool returns exactly the requested tool.
func TestBuildAPIToolDefinitionsFor_SingleTool(t *testing.T) {
	tools := BuildAPIToolDefinitionsFor([]string{"read_file"})
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "read_file" {
		t.Errorf("expected read_file, got %q", tools[0].Name)
	}
}

// TestBuildAPIToolDefinitionsFor_MultipleTools returns the requested subset
// in the order they were requested.
func TestBuildAPIToolDefinitionsFor_MultipleTools(t *testing.T) {
	requested := []string{"write_file", "read_file"}
	tools := BuildAPIToolDefinitionsFor(requested)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0].Name != "write_file" {
		t.Errorf("expected first tool write_file, got %q", tools[0].Name)
	}
	if tools[1].Name != "read_file" {
		t.Errorf("expected second tool read_file, got %q", tools[1].Name)
	}
}

// TestBuildAPIToolDefinitionsFor_UnknownNamesSkipped silently drops tool
// names that have no definition in the API runner. The toolset registry
// declares aspirational tools (glob, grep, web_fetch, etc.) that are not
// yet implemented; the filter must degrade gracefully rather than error.
func TestBuildAPIToolDefinitionsFor_UnknownNamesSkipped(t *testing.T) {
	requested := []string{
		"read_file",       // known
		"glob",            // unknown (declared in toolset.repo_readonly)
		"grep",            // unknown
		"git_status",      // unknown
		"run_shell_command", // known
	}
	tools := BuildAPIToolDefinitionsFor(requested)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools (only known names), got %d", len(tools))
	}

	gotNames := make(map[string]bool, len(tools))
	for _, tool := range tools {
		gotNames[tool.Name] = true
	}
	if !gotNames["read_file"] {
		t.Error("expected read_file in result")
	}
	if !gotNames["run_shell_command"] {
		t.Error("expected run_shell_command in result")
	}
}

// TestBuildAPIToolDefinitionsFor_DuplicatesIgnored handles duplicate names in
// the input list — return each tool at most once.
func TestBuildAPIToolDefinitionsFor_DuplicatesIgnored(t *testing.T) {
	tools := BuildAPIToolDefinitionsFor([]string{"read_file", "read_file", "read_file"})
	if len(tools) != 1 {
		t.Errorf("expected 1 tool (dedup), got %d", len(tools))
	}
}

// TestBuildAPIToolDefinitionsFor_FilteredSubsetSmallerThanFull is the
// scenario the user actually cares about: a repo_readonly task should send
// strictly fewer tool schemas than the unfiltered baseline.
func TestBuildAPIToolDefinitionsFor_FilteredSubsetSmallerThanFull(t *testing.T) {
	// repo_readonly declares: read_file, glob, grep, git_status, git_diff,
	// git_log, list_directory. Of these only read_file is implemented in the
	// API runner today, so the filtered result should be exactly 1 tool.
	repoReadonlyTools := []string{
		"read_file", "glob", "grep", "git_status", "git_diff", "git_log", "list_directory",
	}
	filtered := BuildAPIToolDefinitionsFor(repoReadonlyTools)
	full := BuildAPIToolDefinitions()

	if len(filtered) >= len(full) {
		t.Errorf("filtered list (%d) should be smaller than full list (%d) for repo_readonly", len(filtered), len(full))
	}
	if len(filtered) != 1 {
		t.Errorf("expected exactly 1 tool for repo_readonly, got %d", len(filtered))
	}
}
