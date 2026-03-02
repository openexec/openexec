package context

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewPackageInfoGatherer(t *testing.T) {
	g := NewPackageInfoGatherer()

	if g.Type() != ContextTypePackageInfo {
		t.Errorf("Type() = %v, want %v", g.Type(), ContextTypePackageInfo)
	}
	if g.Name() != "Package Info" {
		t.Errorf("Name() = %v, want 'Package Info'", g.Name())
	}
	if g.Priority() != PriorityMedium {
		t.Errorf("Priority() = %v, want PriorityMedium", g.Priority())
	}

	// Check default file paths
	paths := g.FilePaths()
	if len(paths) == 0 {
		t.Error("FilePaths() should have default paths")
	}

	expectedFiles := []string{"package.json", "go.mod", "requirements.txt"}
	for _, expected := range expectedFiles {
		found := false
		for _, p := range paths {
			if p == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("FilePaths() should include %s", expected)
		}
	}
}

func TestPackageInfoGatherer_Gather_GoMod(t *testing.T) {
	tempDir := t.TempDir()

	goModContent := `module github.com/example/test

go 1.21

require (
	github.com/google/uuid v1.4.0
	github.com/stretchr/testify v1.8.4
)
`
	err := os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte(goModContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	g := NewPackageInfoGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	if item == nil {
		t.Fatal("Gather() returned nil item")
	}

	if item.Type != ContextTypePackageInfo {
		t.Errorf("item.Type = %v, want %v", item.Type, ContextTypePackageInfo)
	}

	content := item.Content

	if !strings.Contains(content, "github.com/example/test") {
		t.Error("content should contain module name")
	}
	if !strings.Contains(content, "1.21") {
		t.Error("content should contain Go version")
	}
	if !strings.Contains(content, "Dependencies:") {
		t.Error("content should mention dependencies")
	}
}

func TestPackageInfoGatherer_Gather_PackageJSON(t *testing.T) {
	tempDir := t.TempDir()

	packageJSONContent := `{
  "name": "my-app",
  "version": "1.0.0",
  "description": "A test application",
  "scripts": {
    "start": "node index.js",
    "test": "jest",
    "build": "tsc"
  },
  "dependencies": {
    "express": "^4.18.0",
    "lodash": "^4.17.21"
  },
  "devDependencies": {
    "jest": "^29.0.0",
    "typescript": "^5.0.0"
  }
}`
	err := os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(packageJSONContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create package.json: %v", err)
	}

	g := NewPackageInfoGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	if !strings.Contains(content, "my-app") {
		t.Error("content should contain package name")
	}
	if !strings.Contains(content, "1.0.0") {
		t.Error("content should contain version")
	}
	if !strings.Contains(content, "A test application") {
		t.Error("content should contain description")
	}
	if !strings.Contains(content, "Scripts:") {
		t.Error("content should contain scripts section")
	}
	if !strings.Contains(content, "start:") || !strings.Contains(content, "node index.js") {
		t.Error("content should contain start script")
	}
	if !strings.Contains(content, "Dependencies:") {
		t.Error("content should contain dependencies section")
	}
}

func TestPackageInfoGatherer_Gather_RequirementsTxt(t *testing.T) {
	tempDir := t.TempDir()

	requirementsContent := `# Core dependencies
flask==2.0.0
requests>=2.28.0,<3.0.0
pydantic~=1.10.0

# Testing
pytest==7.4.0
pytest-cov
`
	err := os.WriteFile(filepath.Join(tempDir, "requirements.txt"), []byte(requirementsContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create requirements.txt: %v", err)
	}

	g := NewPackageInfoGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	if !strings.Contains(content, "Python dependencies:") {
		t.Error("content should indicate Python dependencies")
	}
	if !strings.Contains(content, "flask") {
		t.Error("content should contain flask")
	}
	if !strings.Contains(content, "pytest") {
		t.Error("content should contain pytest")
	}
}

func TestPackageInfoGatherer_Gather_CargoToml(t *testing.T) {
	tempDir := t.TempDir()

	cargoContent := `[package]
name = "my-rust-app"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = "1.0"
tokio = { version = "1.0", features = ["full"] }

[dev-dependencies]
pretty_assertions = "1.0"
`
	err := os.WriteFile(filepath.Join(tempDir, "Cargo.toml"), []byte(cargoContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create Cargo.toml: %v", err)
	}

	g := NewPackageInfoGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	if !strings.Contains(content, "my-rust-app") {
		t.Error("content should contain crate name")
	}
	if !strings.Contains(content, "0.1.0") {
		t.Error("content should contain version")
	}
}

func TestPackageInfoGatherer_Gather_MultipleFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create both go.mod and package.json (monorepo scenario)
	os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test\n\ngo 1.21"), 0644)
	os.WriteFile(filepath.Join(tempDir, "package.json"), []byte(`{"name": "frontend", "version": "1.0.0"}`), 0644)

	g := NewPackageInfoGatherer()
	item, err := g.Gather(context.Background(), tempDir)

	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}

	content := item.Content

	// Should contain info from both files
	if !strings.Contains(content, "go.mod") {
		t.Error("content should mention go.mod")
	}
	if !strings.Contains(content, "package.json") {
		t.Error("content should mention package.json")
	}

	// Source should mention both files
	if !strings.Contains(item.Source, "go.mod") || !strings.Contains(item.Source, "package.json") {
		t.Errorf("item.Source should mention both files, got: %s", item.Source)
	}
}

func TestPackageInfoGatherer_NoPackageFiles(t *testing.T) {
	tempDir := t.TempDir()

	g := NewPackageInfoGatherer()
	_, err := g.Gather(context.Background(), tempDir)

	if err == nil {
		t.Error("Gather() should return error when no package files found")
	}
	if !strings.Contains(err.Error(), "no package files found") {
		t.Errorf("error message should mention no files found, got: %v", err)
	}
}

func TestPackageInfoGatherer_EmptyProjectPath(t *testing.T) {
	g := NewPackageInfoGatherer()
	_, err := g.Gather(context.Background(), "")

	if err == nil {
		t.Error("Gather() should return error for empty project path")
	}
}

func TestPackageInfoGatherer_ContextCancellation(t *testing.T) {
	tempDir := t.TempDir()
	os.WriteFile(filepath.Join(tempDir, "go.mod"), []byte("module test"), 0644)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	g := NewPackageInfoGatherer()
	_, err := g.Gather(ctx, tempDir)

	if err == nil {
		t.Error("Gather() should return error when context is cancelled")
	}
}

func TestProcessPackageJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // Expected substrings
	}{
		{
			name:     "full package.json",
			input:    `{"name": "test", "version": "1.0.0", "description": "Test app"}`,
			expected: []string{"Name: test", "Version: 1.0.0", "Description: Test app"},
		},
		{
			name:     "with scripts",
			input:    `{"name": "test", "scripts": {"start": "npm run dev"}}`,
			expected: []string{"Scripts:", "start: npm run dev"},
		},
		{
			name:     "with dependencies",
			input:    `{"name": "test", "dependencies": {"react": "^18.0.0"}}`,
			expected: []string{"Dependencies: 1 packages", "react: ^18.0.0"},
		},
		{
			name:     "invalid json returns raw",
			input:    "not valid json",
			expected: []string{"not valid json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processPackageJSON(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("processPackageJSON() result missing %q\nGot: %s", exp, result)
				}
			}
		})
	}
}

func TestProcessGoMod(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "basic go.mod",
			input:    "module github.com/test\n\ngo 1.21",
			expected: []string{"Module: github.com/test", "Go version: 1.21"},
		},
		{
			name: "with dependencies",
			input: `module test

go 1.21

require (
	github.com/pkg/errors v0.9.1
	github.com/stretchr/testify v1.8.0
)`,
			expected: []string{"Dependencies: 2 modules", "github.com/pkg/errors"},
		},
		{
			name:     "single line require",
			input:    "module test\n\ngo 1.21\n\nrequire github.com/pkg/errors v0.9.1",
			expected: []string{"Dependencies: 1 modules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processGoMod(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("processGoMod() result missing %q\nGot: %s", exp, result)
				}
			}
		})
	}
}

func TestProcessRequirementsTxt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "basic requirements",
			input:    "flask==2.0.0\nrequests>=2.28.0",
			expected: []string{"Python dependencies: 2 packages", "flask==2.0.0", "requests>=2.28.0"},
		},
		{
			name:     "with comments",
			input:    "# Core\nflask==2.0.0\n# Testing\npytest",
			expected: []string{"Python dependencies: 2 packages", "flask", "pytest"},
		},
		{
			name:     "empty requirements",
			input:    "# Just a comment\n\n",
			expected: []string{"Python dependencies: 0 packages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processRequirementsTxt(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("processRequirementsTxt() result missing %q\nGot: %s", exp, result)
				}
			}
		})
	}
}

func TestProcessCargoToml(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "basic cargo.toml",
			input:    "[package]\nname = \"test-crate\"\nversion = \"0.1.0\"",
			expected: []string{"Name: test-crate", "Version: 0.1.0"},
		},
		{
			name:     "with dependencies",
			input:    "[package]\nname = \"test\"\n\n[dependencies]\nserde = \"1.0\"\ntokio = \"1.0\"",
			expected: []string{"Dependencies: 2 crates"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processCargoToml(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("processCargoToml() result missing %q\nGot: %s", exp, result)
				}
			}
		})
	}
}

func TestProcessPyprojectToml(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "project section",
			input:    "[project]\nname = \"my-project\"\nversion = \"1.0.0\"\ndescription = \"A test\"",
			expected: []string{"Name: my-project", "Version: 1.0.0", "Description: A test"},
		},
		{
			name:     "poetry section",
			input:    "[tool.poetry]\nname = \"poetry-app\"\nversion = \"0.1.0\"",
			expected: []string{"Name: poetry-app", "Version: 0.1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processPyprojectToml(tt.input)
			for _, exp := range tt.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("processPyprojectToml() result missing %q\nGot: %s", exp, result)
				}
			}
		})
	}
}

func TestPackageInfoGatherer_Configure(t *testing.T) {
	g := NewPackageInfoGatherer()

	config := &GathererConfig{
		MaxTokens: 500,
		Priority:  PriorityLow,
		FilePaths: `["custom.json", "deps.txt"]`,
	}

	err := g.Configure(config)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if g.MaxTokens() != 500 {
		t.Errorf("MaxTokens() = %v, want 500", g.MaxTokens())
	}

	paths := g.FilePaths()
	if len(paths) != 2 || paths[0] != "custom.json" {
		t.Errorf("FilePaths() = %v, want [custom.json, deps.txt]", paths)
	}
}
