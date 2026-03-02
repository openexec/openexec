package context

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDirectoryStructureGatherer(t *testing.T) {
	g := NewDirectoryStructureGatherer()

	if g.Type() != ContextTypeDirectoryStructure {
		t.Errorf("Type() = %v, want %v", g.Type(), ContextTypeDirectoryStructure)
	}
	if g.Name() != "Directory Structure" {
		t.Errorf("Name() = %v, want 'Directory Structure'", g.Name())
	}
	if g.Priority() != PriorityMedium {
		t.Errorf("Priority() = %v, want PriorityMedium", g.Priority())
	}

	// Check default options
	maxDepth := g.GetIntOption("max_depth", 0)
	if maxDepth != 4 {
		t.Errorf("default max_depth = %v, want 4", maxDepth)
	}

	exclude := g.GetStringSliceOption("exclude", nil)
	if len(exclude) == 0 {
		t.Error("default exclude list should not be empty")
	}
}

func TestDirectoryStructureGatherer_Gather(t *testing.T) {
	tempDir := t.TempDir()

	// Create directory structure
	os.MkdirAll(filepath.Join(tempDir, "src", "components"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "tests"), 0755)
	os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test"), 0644)
	os.WriteFile(filepath.Join(tempDir, "src", "app.go"), []byte("package src"), 0644)
	os.WriteFile(filepath.Join(tempDir, "src", "components", "button.go"), []byte("package components"), 0644)
	os.WriteFile(filepath.Join(tempDir, "tests", "app_test.go"), []byte("package tests"), 0644)

	g := NewDirectoryStructureGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if item == nil {
		t.Fatal("Gather() returned nil item")
	}

	// Check item properties
	if item.Type != ContextTypeDirectoryStructure {
		t.Errorf("item.Type = %v, want %v", item.Type, ContextTypeDirectoryStructure)
	}
	if item.Source != "directory structure" {
		t.Errorf("item.Source = %v, want 'directory structure'", item.Source)
	}
	if item.TokenCount <= 0 {
		t.Error("item.TokenCount should be positive")
	}

	// Check content contains expected files and directories
	content := item.Content

	if !strings.Contains(content, "src/") {
		t.Error("content should contain 'src/'")
	}
	if !strings.Contains(content, "tests/") {
		t.Error("content should contain 'tests/'")
	}
	if !strings.Contains(content, "main.go") {
		t.Error("content should contain 'main.go'")
	}
	if !strings.Contains(content, "app.go") {
		t.Error("content should contain 'app.go'")
	}
}

func TestDirectoryStructureGatherer_ExcludeDirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create directories including excluded ones
	os.MkdirAll(filepath.Join(tempDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "node_modules", "react"), 0755)
	os.MkdirAll(filepath.Join(tempDir, ".git", "objects"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "__pycache__"), 0755)

	os.WriteFile(filepath.Join(tempDir, "src", "app.js"), []byte("const x = 1"), 0644)
	os.WriteFile(filepath.Join(tempDir, "node_modules", "react", "index.js"), []byte("react"), 0644)

	g := NewDirectoryStructureGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	// Should include src
	if !strings.Contains(content, "src/") {
		t.Error("content should contain 'src/'")
	}

	// Should not include excluded directories
	if strings.Contains(content, "node_modules/") {
		t.Error("content should not contain 'node_modules/'")
	}
	if strings.Contains(content, "__pycache__/") {
		t.Error("content should not contain '__pycache__/'")
	}
}

func TestDirectoryStructureGatherer_MaxDepth(t *testing.T) {
	tempDir := t.TempDir()

	// Create deep directory structure
	deepPath := filepath.Join(tempDir, "a", "b", "c", "d", "e", "f")
	os.MkdirAll(deepPath, 0755)
	os.WriteFile(filepath.Join(deepPath, "deep.txt"), []byte("deep"), 0644)

	g := NewDirectoryStructureGatherer()
	// Set max depth to 3
	g.options["max_depth"] = 3

	item, err := g.Gather(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	// Should contain directories up to max depth
	if !strings.Contains(content, "a/") {
		t.Error("content should contain 'a/'")
	}
	if !strings.Contains(content, "b/") {
		t.Error("content should contain 'b/'")
	}

	// Should not contain directories beyond max depth
	if strings.Contains(content, "deep.txt") {
		t.Error("content should not contain 'deep.txt' (beyond max depth)")
	}
}

func TestDirectoryStructureGatherer_MaxFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create many files
	for i := 0; i < 20; i++ {
		os.WriteFile(filepath.Join(tempDir, string(rune('a'+i))+".txt"), []byte("content"), 0644)
	}

	g := NewDirectoryStructureGatherer()
	g.options["max_files"] = 10

	item, err := g.Gather(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Should indicate truncation
	if !strings.Contains(item.Content, "truncated") {
		t.Error("content should indicate truncation when exceeding max_files")
	}
}

func TestDirectoryStructureGatherer_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	g := NewDirectoryStructureGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if item == nil {
		t.Fatal("Gather() returned nil item")
	}

	// Should still have header
	if !strings.Contains(item.Content, "Directory structure") {
		t.Error("content should contain header even for empty directory")
	}
}

func TestDirectoryStructureGatherer_EmptyProjectPath(t *testing.T) {
	g := NewDirectoryStructureGatherer()
	_, err := g.Gather(context.Background(), "")

	if err == nil {
		t.Error("Gather() should return error for empty project path")
	}
}

func TestDirectoryStructureGatherer_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()

	// Create some files
	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	g := NewDirectoryStructureGatherer()
	_, err := g.Gather(ctx, tempDir)

	if err == nil {
		t.Error("Gather() should return error when context is cancelled")
	}
}

// Tests for RecentFilesGatherer

func TestNewRecentFilesGatherer(t *testing.T) {
	g := NewRecentFilesGatherer()

	if g.Type() != ContextTypeRecentFiles {
		t.Errorf("Type() = %v, want %v", g.Type(), ContextTypeRecentFiles)
	}
	if g.Name() != "Recent Files" {
		t.Errorf("Name() = %v, want 'Recent Files'", g.Name())
	}
	if g.Priority() != PriorityMedium {
		t.Errorf("Priority() = %v, want PriorityMedium", g.Priority())
	}

	// Check default options
	maxFiles := g.GetIntOption("max_files", 0)
	if maxFiles != 20 {
		t.Errorf("default max_files = %v, want 20", maxFiles)
	}
}

func TestRecentFilesGatherer_Gather(t *testing.T) {
	tempDir := t.TempDir()

	// Create some files
	os.WriteFile(filepath.Join(tempDir, "recent1.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tempDir, "recent2.go"), []byte("package main"), 0644)
	os.MkdirAll(filepath.Join(tempDir, "src"), 0755)
	os.WriteFile(filepath.Join(tempDir, "src", "app.go"), []byte("package src"), 0644)

	g := NewRecentFilesGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if item == nil {
		t.Fatal("Gather() returned nil item")
	}

	// Check item properties
	if item.Type != ContextTypeRecentFiles {
		t.Errorf("item.Type = %v, want %v", item.Type, ContextTypeRecentFiles)
	}

	content := item.Content

	if !strings.Contains(content, "recent1.go") {
		t.Error("content should contain 'recent1.go'")
	}
	if !strings.Contains(content, "recent2.go") {
		t.Error("content should contain 'recent2.go'")
	}
}

func TestRecentFilesGatherer_ExcludeDirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create files in excluded directories
	os.MkdirAll(filepath.Join(tempDir, "node_modules"), 0755)
	os.WriteFile(filepath.Join(tempDir, "node_modules", "package.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(tempDir, "app.js"), []byte("const x = 1"), 0644)

	g := NewRecentFilesGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	// Should include regular files
	if !strings.Contains(content, "app.js") {
		t.Error("content should contain 'app.js'")
	}

	// Should not include files from excluded directories
	if strings.Contains(content, "node_modules") {
		t.Error("content should not contain files from 'node_modules'")
	}
}

func TestRecentFilesGatherer_MaxFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create many files
	for i := 0; i < 30; i++ {
		os.WriteFile(filepath.Join(tempDir, string(rune('a'+i%26))+string(rune('0'+i/26))+".txt"), []byte("content"), 0644)
	}

	g := NewRecentFilesGatherer()
	g.options["max_files"] = 10

	item, err := g.Gather(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Count files in output
	lines := strings.Split(item.Content, "\n")
	fileCount := 0
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "") && strings.HasSuffix(strings.TrimSpace(line), ".txt") {
			fileCount++
		}
	}

	// Should be limited to max_files
	if fileCount > 10 {
		t.Errorf("file count = %d, should be <= 10", fileCount)
	}
}

func TestRecentFilesGatherer_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()

	g := NewRecentFilesGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !strings.Contains(item.Content, "No recently modified files found") {
		t.Error("content should indicate no files found")
	}
}

func TestRecentFilesGatherer_SkipsHiddenFiles(t *testing.T) {
	tempDir := t.TempDir()

	os.WriteFile(filepath.Join(tempDir, ".hidden"), []byte("hidden"), 0644)
	os.WriteFile(filepath.Join(tempDir, "visible.txt"), []byte("visible"), 0644)

	g := NewRecentFilesGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	if !strings.Contains(content, "visible.txt") {
		t.Error("content should contain 'visible.txt'")
	}
	if strings.Contains(content, ".hidden") {
		t.Error("content should not contain hidden files")
	}
}

func TestRecentFilesGatherer_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := NewRecentFilesGatherer()
	_, err := g.Gather(ctx, tempDir)

	if err == nil {
		t.Error("Gather() should return error when context is cancelled")
	}
}
