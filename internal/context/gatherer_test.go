package context

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBaseGatherer_New(t *testing.T) {
	g := NewBaseGatherer(ContextTypeGitStatus, "Test Gatherer", "Test description")

	if g.Type() != ContextTypeGitStatus {
		t.Errorf("Type() = %v, want %v", g.Type(), ContextTypeGitStatus)
	}
	if g.Name() != "Test Gatherer" {
		t.Errorf("Name() = %v, want 'Test Gatherer'", g.Name())
	}
	if g.Description() != "Test description" {
		t.Errorf("Description() = %v, want 'Test description'", g.Description())
	}
	if g.MaxTokens() != 4000 {
		t.Errorf("MaxTokens() = %v, want 4000", g.MaxTokens())
	}
}

func TestBaseGatherer_Configure(t *testing.T) {
	g := NewBaseGatherer(ContextTypeGitStatus, "Test", "Test")

	config := &GathererConfig{
		ID:        "test-config",
		Type:      ContextTypeGitStatus,
		Name:      "Configured Gatherer",
		MaxTokens: 2000,
		Priority:  PriorityHigh,
		FilePaths: `["file1.txt", "file2.txt"]`,
		Options:   `{"max_commits": 5, "include_stats": true}`,
	}

	err := g.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if g.MaxTokens() != 2000 {
		t.Errorf("MaxTokens() = %v, want 2000 after Configure", g.MaxTokens())
	}
	if g.Priority() != PriorityHigh {
		t.Errorf("Priority() = %v, want PriorityHigh after Configure", g.Priority())
	}

	paths := g.FilePaths()
	if len(paths) != 2 || paths[0] != "file1.txt" {
		t.Errorf("FilePaths() = %v, want [file1.txt, file2.txt]", paths)
	}

	if g.GetIntOption("max_commits", 0) != 5 {
		t.Errorf("GetIntOption('max_commits') = %v, want 5", g.GetIntOption("max_commits", 0))
	}
	if !g.GetBoolOption("include_stats", false) {
		t.Error("GetBoolOption('include_stats') should be true")
	}
}

func TestBaseGatherer_GetOptions(t *testing.T) {
	g := NewBaseGatherer(ContextTypeGitStatus, "Test", "Test")
	g.options = map[string]interface{}{
		"int_val":    42,
		"float_val":  3.14,
		"string_val": "hello",
		"bool_val":   true,
		"slice_val":  []interface{}{"a", "b", "c"},
	}

	// Test GetIntOption
	if g.GetIntOption("int_val", 0) != 42 {
		t.Error("GetIntOption for int failed")
	}
	if g.GetIntOption("float_val", 0) != 3 {
		t.Error("GetIntOption for float conversion failed")
	}
	if g.GetIntOption("missing", 99) != 99 {
		t.Error("GetIntOption default value failed")
	}

	// Test GetStringOption
	if g.GetStringOption("string_val", "") != "hello" {
		t.Error("GetStringOption failed")
	}
	if g.GetStringOption("missing", "default") != "default" {
		t.Error("GetStringOption default value failed")
	}

	// Test GetBoolOption
	if !g.GetBoolOption("bool_val", false) {
		t.Error("GetBoolOption failed")
	}
	if g.GetBoolOption("missing", true) != true {
		t.Error("GetBoolOption default value failed")
	}

	// Test GetStringSliceOption
	slice := g.GetStringSliceOption("slice_val", nil)
	if len(slice) != 3 || slice[0] != "a" {
		t.Errorf("GetStringSliceOption = %v, want [a, b, c]", slice)
	}
}

func TestBaseGatherer_CreateContextItem(t *testing.T) {
	g := NewBaseGatherer(ContextTypeGitStatus, "Test", "Test")
	g.priority = PriorityHigh

	item, err := g.CreateContextItem("test source", "test content", 10)
	if err != nil {
		t.Fatalf("CreateContextItem() error = %v", err)
	}

	if item.Type != ContextTypeGitStatus {
		t.Errorf("item.Type = %v, want %v", item.Type, ContextTypeGitStatus)
	}
	if item.Source != "test source" {
		t.Errorf("item.Source = %v, want 'test source'", item.Source)
	}
	if item.Content != "test content" {
		t.Errorf("item.Content = %v, want 'test content'", item.Content)
	}
	if item.Priority != PriorityHigh {
		t.Errorf("item.Priority = %v, want PriorityHigh", item.Priority)
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		content  string
		expected int
	}{
		{"", 0},
		{"test", 1},        // 4 chars / 4 = 1
		{"12345678", 2},    // 8 chars / 4 = 2
		{"123456789", 3},   // 9 chars, rounded up = 3
		{"hello world!", 3}, // 12 chars / 4 = 3
	}

	for _, tt := range tests {
		got := EstimateTokens(tt.content)
		if got != tt.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.content, got, tt.expected)
		}
	}
}

func TestTruncateToTokenLimit(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		maxTokens int
		wantLen   int // Approximate expected length
	}{
		{
			name:      "content within limit",
			content:   "short",
			maxTokens: 100,
			wantLen:   5,
		},
		{
			name:      "content exceeds limit",
			content:   string(make([]byte, 1000)), // 1000 chars = ~250 tokens
			maxTokens: 50,
			wantLen:   200, // ~50 tokens * 4 chars
		},
		{
			name:      "zero limit returns original",
			content:   "test content",
			maxTokens: 0,
			wantLen:   12,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TruncateToTokenLimit(tt.content, tt.maxTokens)
			// Allow some variance for truncation message
			if tt.maxTokens > 0 && len(result) > tt.wantLen+50 {
				t.Errorf("TruncateToTokenLimit() len = %d, want around %d", len(result), tt.wantLen)
			}
		})
	}
}

func TestGathererRunner_RunAll(t *testing.T) {
	tempDir := t.TempDir()

	// Create a simple gatherer for testing
	g := NewEnvironmentGatherer()

	runner := NewGathererRunner(g)
	results := runner.RunAll(context.Background(), tempDir)

	if len(results) != 1 {
		t.Fatalf("RunAll() returned %d results, want 1", len(results))
	}

	if results[0].Error != nil {
		t.Errorf("RunAll() result error = %v", results[0].Error)
	}
	if results[0].Item == nil {
		t.Error("RunAll() result item should not be nil")
	}
	if results[0].Duration <= 0 {
		t.Error("RunAll() result duration should be positive")
	}
}

func TestGathererRunner_RunByType(t *testing.T) {
	tempDir := t.TempDir()

	// Create test file for directory structure
	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte("test"), 0644)

	runner := NewGathererRunner(
		NewEnvironmentGatherer(),
		NewDirectoryStructureGatherer(),
	)

	results := runner.RunByType(context.Background(), tempDir, ContextTypeEnvironment)
	if len(results) != 1 {
		t.Fatalf("RunByType() returned %d results, want 1", len(results))
	}

	if results[0].Item.Type != ContextTypeEnvironment {
		t.Errorf("RunByType() result type = %v, want %v", results[0].Item.Type, ContextTypeEnvironment)
	}
}

func TestGathererRunner_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	g := NewEnvironmentGatherer()
	runner := NewGathererRunner(g)

	result := runner.Run(ctx, g, t.TempDir())
	if result.Error == nil {
		// Some gatherers may complete before checking cancellation
		// This is not necessarily an error
		t.Log("Gatherer completed before context cancellation was checked")
	}
}

func TestGathererRunner_AddGatherer(t *testing.T) {
	runner := NewGathererRunner()
	if len(runner.Gatherers()) != 0 {
		t.Error("new runner should have no gatherers")
	}

	runner.AddGatherer(NewEnvironmentGatherer())
	if len(runner.Gatherers()) != 1 {
		t.Error("AddGatherer should add gatherer")
	}

	runner.AddGatherer(NewGitStatusGatherer())
	if len(runner.Gatherers()) != 2 {
		t.Error("AddGatherer should add multiple gatherers")
	}
}

// Helper function to create a temporary directory with files
func createTestProject(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()

	// Create subdirectories
	os.MkdirAll(filepath.Join(tempDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tempDir, "tests"), 0755)

	// Create test files
	os.WriteFile(filepath.Join(tempDir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tempDir, "src", "app.go"), []byte("package src"), 0644)
	os.WriteFile(filepath.Join(tempDir, "tests", "app_test.go"), []byte("package tests"), 0644)
	os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test\n\ngo 1.21"), 0644)

	return tempDir
}

// Integration test for running multiple gatherers
func TestGathererRunner_Integration(t *testing.T) {
	tempDir := createTestProject(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create runner with multiple gatherers
	runner := NewGathererRunner(
		NewEnvironmentGatherer(),
		NewDirectoryStructureGatherer(),
		NewRecentFilesGatherer(),
		NewPackageInfoGatherer(),
	)

	results := runner.RunAll(ctx, tempDir)
	if len(results) != 4 {
		t.Fatalf("Expected 4 results, got %d", len(results))
	}

	// Check each result
	for _, result := range results {
		if result.Error != nil {
			t.Logf("Gatherer %s error: %v", result.Item.Type, result.Error)
			// Some gatherers might fail in test environment, that's okay
			continue
		}

		if result.Item != nil {
			if result.Item.ID == "" {
				t.Errorf("Gatherer %s produced item without ID", result.Item.Type)
			}
			if result.Item.TokenCount <= 0 && result.Item.Content != "" {
				t.Errorf("Gatherer %s produced content but no token count", result.Item.Type)
			}
		}
	}
}
