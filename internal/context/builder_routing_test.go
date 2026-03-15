package context

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilterItemsByZones(t *testing.T) {
	items := []*ContextItem{
		{ID: "1", Type: ContextTypeDirectoryStructure, Source: "internal/api/handler.go"},
		{ID: "2", Type: ContextTypeDirectoryStructure, Source: "internal/db/store.go"},
		{ID: "3", Type: ContextTypeProjectInstructions, Source: "CLAUDE.md"},
		{ID: "4", Type: ContextTypePackageInfo, Source: "go.mod"},
	}

	// Filter to api zone only
	filtered := filterItemsByZones(items, []string{"internal/api"})

	// Should include api file + essential types (instructions, package info)
	if len(filtered) < 1 {
		t.Errorf("Expected at least 1 item after filtering, got %d", len(filtered))
	}

	// Essential types should always be included
	foundInstructions := false
	foundPackage := false
	for _, item := range filtered {
		if item.Type == ContextTypeProjectInstructions {
			foundInstructions = true
		}
		if item.Type == ContextTypePackageInfo {
			foundPackage = true
		}
	}

	if !foundInstructions {
		t.Error("Essential type ProjectInstructions should be included")
	}
	if !foundPackage {
		t.Error("Essential type PackageInfo should be included")
	}
}

func TestSourceMatchesZone(t *testing.T) {
	tests := []struct {
		source string
		zone   string
		want   bool
	}{
		{"internal/api/handler.go", "internal/api", true},
		{"internal/api/handler.go", "internal/api/", true},
		{"internal/db/store.go", "internal/api", false},
		{"pkg/util/helper.go", "pkg/", true},
		{"main.go", "cmd/", false},
	}

	for _, tc := range tests {
		t.Run(tc.source, func(t *testing.T) {
			got := sourceMatchesZone(tc.source, tc.zone)
			if got != tc.want {
				t.Errorf("sourceMatchesZone(%q, %q) = %v, want %v", tc.source, tc.zone, got, tc.want)
			}
		})
	}
}

func TestContextTypeToKnowledgeSource(t *testing.T) {
	tests := []struct {
		ct   ContextType
		want string
	}{
		{ContextTypeGitLog, "git_history"},
		{ContextTypeGitDiff, "git_history"},
		{ContextTypeGitStatus, "git_history"},
		{ContextTypeProjectInstructions, "local_docs"},
		{ContextTypeDirectoryStructure, "code_symbols"},
		{ContextTypePackageInfo, "dependencies"},
		{ContextTypeEnvironment, "code_symbols"},
	}

	for _, tc := range tests {
		t.Run(string(tc.ct), func(t *testing.T) {
			got := contextTypeToKnowledgeSource(tc.ct)
			if got != tc.want {
				t.Errorf("contextTypeToKnowledgeSource(%s) = %s, want %s", tc.ct, got, tc.want)
			}
		})
	}
}

// Note: TestBuildContextWithRouting is skipped as it requires a full git repo.
// The integration is tested via the existing builder_test.go tests.

// Silence unused import warnings
var _ = context.Background
var _ = os.TempDir
var _ = filepath.Join
