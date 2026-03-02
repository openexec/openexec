package context

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestNewEnvironmentGatherer(t *testing.T) {
	g := NewEnvironmentGatherer()

	if g.Type() != ContextTypeEnvironment {
		t.Errorf("Type() = %v, want %v", g.Type(), ContextTypeEnvironment)
	}
	if g.Name() != "Environment Info" {
		t.Errorf("Name() = %v, want 'Environment Info'", g.Name())
	}
	if g.Priority() != PriorityHigh {
		t.Errorf("Priority() = %v, want PriorityHigh", g.Priority())
	}
}

func TestEnvironmentGatherer_Gather(t *testing.T) {
	tempDir := t.TempDir()

	g := NewEnvironmentGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if item == nil {
		t.Fatal("Gather() returned nil item")
	}

	// Check item properties
	if item.Type != ContextTypeEnvironment {
		t.Errorf("item.Type = %v, want %v", item.Type, ContextTypeEnvironment)
	}
	if item.Source != "environment" {
		t.Errorf("item.Source = %v, want 'environment'", item.Source)
	}
	if item.TokenCount <= 0 {
		t.Error("item.TokenCount should be positive")
	}

	// Check content contains expected information
	content := item.Content

	if !strings.Contains(content, "Working directory:") {
		t.Error("content should contain 'Working directory:'")
	}
	if !strings.Contains(content, tempDir) {
		t.Error("content should contain the project path")
	}
	if !strings.Contains(content, "Platform:") {
		t.Error("content should contain 'Platform:'")
	}
	if !strings.Contains(content, runtime.GOOS) {
		t.Errorf("content should contain platform '%s'", runtime.GOOS)
	}
	if !strings.Contains(content, "Today's date:") {
		t.Error("content should contain 'Today's date:'")
	}
	if !strings.Contains(content, "Architecture:") {
		t.Error("content should contain 'Architecture:'")
	}
	if !strings.Contains(content, runtime.GOARCH) {
		t.Errorf("content should contain architecture '%s'", runtime.GOARCH)
	}
}

func TestEnvironmentGatherer_GitRepoDetection(t *testing.T) {
	// Create a temp dir without .git
	nonGitDir := t.TempDir()

	g := NewEnvironmentGatherer()
	item, err := g.Gather(context.Background(), nonGitDir)
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !strings.Contains(item.Content, "Is directory a git repo: No") {
		t.Error("content should indicate non-git directory")
	}

	// Create a temp dir with .git
	gitDir := t.TempDir()
	os.MkdirAll(filepath.Join(gitDir, ".git"), 0755)

	item2, err := g.Gather(context.Background(), gitDir)
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	if !strings.Contains(item2.Content, "Is directory a git repo: Yes") {
		t.Error("content should indicate git directory")
	}
}

func TestEnvironmentGatherer_EmptyProjectPath(t *testing.T) {
	g := NewEnvironmentGatherer()

	// Should still work with empty path (uses current directory)
	item, err := g.Gather(context.Background(), "")
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Should contain some info even without project path
	if !strings.Contains(item.Content, "Platform:") {
		t.Error("content should contain 'Platform:'")
	}
}

func TestEnvironmentGatherer_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	g := NewEnvironmentGatherer()
	_, err := g.Gather(ctx, tempDir)

	if err == nil {
		t.Error("Gather() should return error when context is cancelled")
	}
}

func TestEnvironmentGatherer_DateFormat(t *testing.T) {
	tempDir := t.TempDir()

	g := NewEnvironmentGatherer()
	item, err := g.Gather(context.Background(), tempDir)
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	// Check date format (YYYY-MM-DD)
	today := time.Now().Format("2006-01-02")
	if !strings.Contains(item.Content, today) {
		t.Errorf("content should contain today's date in format %s", today)
	}
}

func TestIsGitRepoCheck(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(string)
		expected bool
	}{
		{
			name: "has .git directory",
			setup: func(dir string) {
				os.MkdirAll(filepath.Join(dir, ".git"), 0755)
			},
			expected: true,
		},
		{
			name:     "no .git directory",
			setup:    func(dir string) {},
			expected: false,
		},
		{
			name: ".git is a file (submodule)",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../.git/modules/sub"), 0644)
			},
			expected: false, // Our simple check only looks for directories
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			tt.setup(tempDir)

			result := isGitRepoCheck(tempDir)
			if result != tt.expected {
				t.Errorf("isGitRepoCheck() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBoolToYesNo(t *testing.T) {
	if boolToYesNo(true) != "Yes" {
		t.Error("boolToYesNo(true) should return 'Yes'")
	}
	if boolToYesNo(false) != "No" {
		t.Error("boolToYesNo(false) should return 'No'")
	}
}

func TestGetOSVersion(t *testing.T) {
	version := getOSVersion()

	// Should return something non-empty
	if version == "" {
		t.Error("getOSVersion() should return non-empty string")
	}

	// Should contain OS-specific info
	switch runtime.GOOS {
	case "darwin":
		if !strings.Contains(version, "Darwin") && !strings.Contains(version, "macOS") {
			t.Log("getOSVersion() might not contain expected macOS info:", version)
		}
	case "linux":
		// Linux version can vary widely
		t.Log("Linux version:", version)
	case "windows":
		t.Log("Windows version:", version)
	}
}

func TestDetectDevelopmentTools(t *testing.T) {
	ctx := context.Background()
	tools := detectDevelopmentTools(ctx)

	// Just verify it doesn't panic and returns a slice
	if tools == nil {
		t.Error("detectDevelopmentTools() should return non-nil slice")
	}

	// If git is available (likely in dev environment), it should be detected
	// But don't fail the test if tools aren't available
	t.Logf("Detected tools: %v", tools)
}

func TestDetectDevelopmentTools_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tools := detectDevelopmentTools(ctx)

	// Should return early when context is cancelled
	// May or may not have detected any tools before cancellation
	t.Logf("Tools detected before cancellation: %v", tools)
}
