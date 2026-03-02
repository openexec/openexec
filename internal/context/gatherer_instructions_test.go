package context

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewProjectInstructionsGatherer(t *testing.T) {
	g := NewProjectInstructionsGatherer()

	if g.Type() != ContextTypeProjectInstructions {
		t.Errorf("Type() = %v, want %v", g.Type(), ContextTypeProjectInstructions)
	}
	if g.Name() != "Project Instructions" {
		t.Errorf("Name() = %v, want 'Project Instructions'", g.Name())
	}
	if g.Priority() != PriorityCritical {
		t.Errorf("Priority() = %v, want PriorityCritical", g.Priority())
	}

	// Check default file paths
	paths := g.FilePaths()
	if len(paths) == 0 {
		t.Error("FilePaths() should have default paths")
	}

	hasClaudeMD := false
	for _, p := range paths {
		if p == "CLAUDE.md" {
			hasClaudeMD = true
			break
		}
	}
	if !hasClaudeMD {
		t.Error("FilePaths() should include CLAUDE.md")
	}
}

func TestProjectInstructionsGatherer_Gather(t *testing.T) {
	tempDir := t.TempDir()

	// Create a CLAUDE.md file
	claudeContent := `# Project Instructions

This is a test project.

## Build Commands

- Run tests: go test ./...
- Build: go build ./...
`
	err := os.WriteFile(filepath.Join(tempDir, "CLAUDE.md"), []byte(claudeContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create CLAUDE.md: %v", err)
	}

	g := NewProjectInstructionsGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if item == nil {
		t.Fatal("Gather() returned nil item")
	}

	if item.Type != ContextTypeProjectInstructions {
		t.Errorf("item.Type = %v, want %v", item.Type, ContextTypeProjectInstructions)
	}
	if !strings.Contains(item.Content, "Project Instructions") {
		t.Error("item.Content should contain 'Project Instructions'")
	}
	if !strings.Contains(item.Content, "go test") {
		t.Error("item.Content should contain 'go test'")
	}
	if item.TokenCount <= 0 {
		t.Error("item.TokenCount should be positive")
	}
}

func TestProjectInstructionsGatherer_MultipleFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create CLAUDE.md
	os.WriteFile(filepath.Join(tempDir, "CLAUDE.md"), []byte("# Main Instructions\n\nMain content here."), 0644)

	// Create .github directory and INSTRUCTIONS.md
	os.MkdirAll(filepath.Join(tempDir, ".github"), 0755)
	os.WriteFile(filepath.Join(tempDir, ".github", "INSTRUCTIONS.md"), []byte("# GitHub Instructions\n\nGitHub specific content."), 0644)

	g := NewProjectInstructionsGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Should contain content from both files
	if !strings.Contains(item.Content, "Main Instructions") {
		t.Error("item.Content should contain 'Main Instructions'")
	}
	if !strings.Contains(item.Content, "GitHub Instructions") {
		t.Error("item.Content should contain 'GitHub Instructions'")
	}

	// Source should mention both files
	if !strings.Contains(item.Source, "CLAUDE.md") {
		t.Error("item.Source should mention CLAUDE.md")
	}
}

func TestProjectInstructionsGatherer_NoFiles(t *testing.T) {
	tempDir := t.TempDir()

	g := NewProjectInstructionsGatherer()
	_, err := g.Gather(context.Background(), tempDir)

	if err == nil {
		t.Error("Gather() should return error when no instruction files found")
	}
	if !strings.Contains(err.Error(), "no project instruction files found") {
		t.Errorf("error message should mention no files found, got: %v", err)
	}
}

func TestProjectInstructionsGatherer_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create empty CLAUDE.md
	os.WriteFile(filepath.Join(tempDir, "CLAUDE.md"), []byte(""), 0644)

	// Create another file with content
	os.WriteFile(filepath.Join(tempDir, "INSTRUCTIONS.md"), []byte("# Real Instructions"), 0644)

	g := NewProjectInstructionsGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Should only include non-empty file
	if strings.Contains(item.Source, "CLAUDE.md") {
		t.Error("item.Source should not include empty CLAUDE.md")
	}
	if !strings.Contains(item.Content, "Real Instructions") {
		t.Error("item.Content should contain 'Real Instructions'")
	}
}

func TestProjectInstructionsGatherer_EmptyProjectPath(t *testing.T) {
	g := NewProjectInstructionsGatherer()
	_, err := g.Gather(context.Background(), "")

	if err == nil {
		t.Error("Gather() should return error for empty project path")
	}
}

func TestProjectInstructionsGatherer_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, "CLAUDE.md"), []byte("# Test"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	g := NewProjectInstructionsGatherer()
	_, err := g.Gather(ctx, tempDir)

	if err == nil {
		t.Error("Gather() should return error when context is cancelled")
	}
}

func TestHasInstructionFile(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(string)
		expected bool
	}{
		{
			name: "has CLAUDE.md",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("test"), 0644)
			},
			expected: true,
		},
		{
			name: "has .claude/CLAUDE.md",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
				os.WriteFile(filepath.Join(dir, ".claude", "CLAUDE.md"), []byte("test"), 0644)
			},
			expected: true,
		},
		{
			name: "has INSTRUCTIONS.md",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "INSTRUCTIONS.md"), []byte("test"), 0644)
			},
			expected: true,
		},
		{
			name:     "no instruction files",
			setup:    func(dir string) {},
			expected: false,
		},
		{
			name: "only has random file",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, "README.md"), []byte("test"), 0644)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(tempDir)

			result := HasInstructionFile(tempDir)
			if result != tt.expected {
				t.Errorf("HasInstructionFile() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestProjectInstructionsGatherer_Configure(t *testing.T) {
	g := NewProjectInstructionsGatherer()

	config := &GathererConfig{
		MaxTokens: 1000,
		Priority:  PriorityHigh,
		FilePaths: `["CUSTOM.md", "docs/INSTRUCTIONS.md"]`,
	}

	err := g.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if g.MaxTokens() != 1000 {
		t.Errorf("MaxTokens() = %v, want 1000", g.MaxTokens())
	}

	paths := g.FilePaths()
	if len(paths) != 2 {
		t.Errorf("FilePaths() length = %d, want 2", len(paths))
	}
	if paths[0] != "CUSTOM.md" {
		t.Errorf("FilePaths()[0] = %v, want 'CUSTOM.md'", paths[0])
	}
}
