package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultOrchestratorLocatorConfig(t *testing.T) {
	cfg := DefaultOrchestratorLocatorConfig()

	// Verify source patterns include expected directories
	expectedPatterns := []string{"cmd", "internal", "pkg", "scripts"}
	for _, expected := range expectedPatterns {
		found := false
		for _, pattern := range cfg.SourcePatterns {
			if pattern == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected source pattern %q not found", expected)
		}
	}

	// Verify exclude patterns include expected directories
	expectedExcludes := []string{".git", "vendor", "node_modules"}
	for _, expected := range expectedExcludes {
		found := false
		for _, pattern := range cfg.ExcludePatterns {
			if pattern == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected exclude pattern %q not found", expected)
		}
	}
}

func TestDetectOrchestratorRoot(t *testing.T) {
	// This test should work when run from within the openexec project
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	// Verify the detected root looks correct
	if !isOrchestratorRoot(root) {
		t.Errorf("detected root %q does not appear to be orchestrator root", root)
	}

	// Verify go.mod exists
	goModPath := filepath.Join(root, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		t.Errorf("go.mod not found at detected root: %v", err)
	}

	// Verify internal/ directory exists
	internalPath := filepath.Join(root, "internal")
	if _, err := os.Stat(internalPath); err != nil {
		t.Errorf("internal/ not found at detected root: %v", err)
	}
}

func TestNewOrchestratorLocator(t *testing.T) {
	// Get the actual root for testing
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	tests := []struct {
		name    string
		cfg     OrchestratorLocatorConfig
		wantErr bool
	}{
		{
			name: "with explicit root",
			cfg: OrchestratorLocatorConfig{
				Root: root,
			},
			wantErr: false,
		},
		{
			name:    "with auto-detection",
			cfg:     OrchestratorLocatorConfig{},
			wantErr: false,
		},
		{
			name: "with invalid root",
			cfg: OrchestratorLocatorConfig{
				Root: "/nonexistent/path",
			},
			wantErr: true,
		},
		{
			name: "with file as root",
			cfg: OrchestratorLocatorConfig{
				Root: filepath.Join(root, "go.mod"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locator, err := NewOrchestratorLocator(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewOrchestratorLocator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && locator == nil {
				t.Error("NewOrchestratorLocator() returned nil locator without error")
			}
		})
	}
}

func TestOrchestratorLocator_Root(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	if locator.Root() != root {
		t.Errorf("Root() = %q, want %q", locator.Root(), root)
	}
}

func TestOrchestratorLocator_IsOrchestratorPath(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "root directory",
			path: root,
			want: true,
		},
		{
			name: "internal/mcp path",
			path: filepath.Join(root, "internal", "mcp"),
			want: true,
		},
		{
			name: "go.mod file",
			path: filepath.Join(root, "go.mod"),
			want: true,
		},
		{
			name: "outside root",
			path: "/tmp",
			want: false,
		},
		{
			name: ".git directory",
			path: filepath.Join(root, ".git"),
			want: false,
		},
		{
			name: "vendor directory",
			path: filepath.Join(root, "vendor"),
			want: false,
		},
		{
			name: "file in .git",
			path: filepath.Join(root, ".git", "config"),
			want: false,
		},
		{
			name: "coverage file",
			path: filepath.Join(root, "coverage.out"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := locator.IsOrchestratorPath(tt.path)
			if got != tt.want {
				t.Errorf("IsOrchestratorPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestOrchestratorLocator_Locate(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		name        string
		path        string
		wantErr     bool
		wantType    OrchestratorFileType
		wantIsTest  bool
		wantPackage string
	}{
		{
			name:        "this test file",
			path:        "internal/mcp/locator_test.go",
			wantErr:     false,
			wantType:    FileTypeGoSource,
			wantIsTest:  true,
			wantPackage: "mcp",
		},
		{
			name:        "locator source file",
			path:        "internal/mcp/locator.go",
			wantErr:     false,
			wantType:    FileTypeGoSource,
			wantIsTest:  false,
			wantPackage: "mcp",
		},
		{
			name:     "go.mod config",
			path:     "go.mod",
			wantErr:  false,
			wantType: FileTypeConfig,
		},
		{
			name:    "nonexistent file",
			path:    "internal/mcp/nonexistent.go",
			wantErr: true,
		},
		{
			name:    "outside orchestrator",
			path:    "/tmp/somefile.go",
			wantErr: true,
		},
		{
			name:    "directory not file",
			path:    "internal/mcp",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := locator.Locate(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("Locate(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if file.Type != tt.wantType {
				t.Errorf("Locate(%q) type = %v, want %v", tt.path, file.Type, tt.wantType)
			}
			if file.IsTest != tt.wantIsTest {
				t.Errorf("Locate(%q) IsTest = %v, want %v", tt.path, file.IsTest, tt.wantIsTest)
			}
			if tt.wantPackage != "" && file.Package != tt.wantPackage {
				t.Errorf("Locate(%q) Package = %q, want %q", tt.path, file.Package, tt.wantPackage)
			}
		})
	}
}

func TestOrchestratorLocator_LocateByType(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Test finding Go source files
	goFiles, err := locator.LocateByType(FileTypeGoSource)
	if err != nil {
		t.Fatalf("LocateByType(GoSource) error = %v", err)
	}

	if len(goFiles) == 0 {
		t.Error("LocateByType(GoSource) returned no files")
	}

	// Verify all returned files are Go files
	for _, f := range goFiles {
		if f.Type != FileTypeGoSource {
			t.Errorf("LocateByType returned file with wrong type: %v", f.Type)
		}
		if !strings.HasSuffix(f.Path, ".go") {
			t.Errorf("LocateByType returned non-Go file: %s", f.Path)
		}
	}

	// Test finding config files
	configFiles, err := locator.LocateByType(FileTypeConfig)
	if err != nil {
		t.Fatalf("LocateByType(Config) error = %v", err)
	}

	// Should find at least go.mod
	foundGoMod := false
	for _, f := range configFiles {
		if strings.HasSuffix(f.Path, "go.mod") {
			foundGoMod = true
			break
		}
	}
	if !foundGoMod {
		t.Error("LocateByType(Config) did not find go.mod")
	}
}

func TestOrchestratorLocator_LocateInPackage(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		name           string
		packagePath    string
		wantErr        bool
		wantMinFiles   int
		expectPackage  string
	}{
		{
			name:          "internal/mcp package",
			packagePath:   "internal/mcp",
			wantErr:       false,
			wantMinFiles:  3, // At least locator.go, path.go, and some others
			expectPackage: "mcp",
		},
		{
			name:          "full import path",
			packagePath:   "github.com/openexec/openexec/internal/mcp",
			wantErr:       false,
			wantMinFiles:  3,
			expectPackage: "mcp",
		},
		{
			name:        "nonexistent package",
			packagePath: "internal/nonexistent",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			files, err := locator.LocateInPackage(tt.packagePath)
			if (err != nil) != tt.wantErr {
				t.Errorf("LocateInPackage(%q) error = %v, wantErr %v", tt.packagePath, err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			if len(files) < tt.wantMinFiles {
				t.Errorf("LocateInPackage(%q) returned %d files, want at least %d", tt.packagePath, len(files), tt.wantMinFiles)
			}

			// Verify package names
			for _, f := range files {
				if tt.expectPackage != "" && f.Package != tt.expectPackage {
					t.Errorf("LocateInPackage(%q) file %s has package %q, want %q",
						tt.packagePath, f.RelativePath, f.Package, tt.expectPackage)
				}
			}
		})
	}
}

func TestOrchestratorLocator_ValidateForEdit(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "existing Go file",
			path:    filepath.Join(root, "internal/mcp/locator.go"),
			wantErr: false,
		},
		{
			name:    "existing test file",
			path:    filepath.Join(root, "internal/mcp/locator_test.go"),
			wantErr: false,
		},
		{
			name:    "existing config file",
			path:    filepath.Join(root, "go.mod"),
			wantErr: false,
		},
		{
			name:    "new file in source directory",
			path:    filepath.Join(root, "internal/mcp/new_feature.go"),
			wantErr: false,
		},
		{
			name:    "new file outside source directories",
			path:    filepath.Join(root, "random_dir/file.go"),
			wantErr: true,
		},
		{
			name:    "outside orchestrator",
			path:    "/tmp/somefile.go",
			wantErr: true,
		},
		{
			name:    "in .git directory",
			path:    filepath.Join(root, ".git/config"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := locator.ValidateForEdit(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateForEdit(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestOrchestratorLocator_GetSourceDirectories(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	dirs := locator.GetSourceDirectories()
	if len(dirs) == 0 {
		t.Error("GetSourceDirectories() returned no directories")
	}

	// Should include internal/
	foundInternal := false
	for _, dir := range dirs {
		if strings.HasSuffix(dir, "internal") {
			foundInternal = true
			break
		}
	}
	if !foundInternal {
		t.Error("GetSourceDirectories() did not include internal/")
	}
}

func TestOrchestratorLocator_ResolveOrchestratorPath(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		name     string
		path     string
		wantErr  bool
		wantPath string
	}{
		{
			name:     "relative path",
			path:     "internal/mcp/locator.go",
			wantErr:  false,
			wantPath: filepath.Join(root, "internal/mcp/locator.go"),
		},
		{
			name:     "absolute path within orchestrator",
			path:     filepath.Join(root, "internal/mcp/locator.go"),
			wantErr:  false,
			wantPath: filepath.Join(root, "internal/mcp/locator.go"),
		},
		{
			name:    "path outside orchestrator",
			path:    "/tmp/file.go",
			wantErr: true,
		},
		{
			name:    "path traversal attempt",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := locator.ResolveOrchestratorPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveOrchestratorPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantPath {
				t.Errorf("ResolveOrchestratorPath(%q) = %q, want %q", tt.path, got, tt.wantPath)
			}
		})
	}
}

func TestOrchestratorLocator_GetOrchestratorInfo(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	info := locator.GetOrchestratorInfo()

	// Check required fields
	if info["root"] != root {
		t.Errorf("GetOrchestratorInfo() root = %v, want %v", info["root"], root)
	}

	if info["source_patterns"] == nil {
		t.Error("GetOrchestratorInfo() missing source_patterns")
	}

	// Should have some Go files
	if goFiles, ok := info["go_files"].(int); !ok || goFiles == 0 {
		t.Error("GetOrchestratorInfo() should report some Go files")
	}
}

func TestClassifyFile(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		path     string
		wantType OrchestratorFileType
	}{
		{"/path/to/file.go", FileTypeGoSource},
		{"/path/to/file_test.go", FileTypeGoSource},
		{"/path/to/file.py", FileTypePython},
		{"/path/to/test_file.py", FileTypePython},
		{"/path/to/config.json", FileTypeConfig},
		{"/path/to/config.yaml", FileTypeConfig},
		{"/path/to/config.yml", FileTypeConfig},
		{"/path/to/config.toml", FileTypeConfig},
		{"/path/to/script.sh", FileTypeScript},
		{"/path/to/script.bash", FileTypeScript},
		{"/path/to/Makefile", FileTypeConfig},
		{"/path/to/Dockerfile", FileTypeConfig},
		{"/path/to/go.mod", FileTypeConfig},
		{"/path/to/go.sum", FileTypeConfig},
		{"/path/to/.gitignore", FileTypeConfig},
		{"/path/to/file.txt", FileTypeUnknown},
		{"/path/to/file.exe", FileTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := locator.classifyFile(tt.path)
			if got != tt.wantType {
				t.Errorf("classifyFile(%q) = %v, want %v", tt.path, got, tt.wantType)
			}
		})
	}
}

func TestIsTestFile(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	tests := []struct {
		path   string
		isTest bool
	}{
		{"/path/to/file.go", false},
		{"/path/to/file_test.go", true},
		{"/path/to/main.go", false},
		{"/path/to/main_test.go", true},
		{"/path/to/file.py", false},
		{"/path/to/test_file.py", true},
		{"/path/to/file_test.py", true},
		{"/path/to/testing.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := locator.isTestFile(tt.path)
			if got != tt.isTest {
				t.Errorf("isTestFile(%q) = %v, want %v", tt.path, got, tt.isTest)
			}
		})
	}
}

func TestIsOrchestratorRoot(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{
			name: "actual orchestrator root",
			dir:  root,
			want: true,
		},
		{
			name: "internal directory",
			dir:  filepath.Join(root, "internal"),
			want: false,
		},
		{
			name: "random temp directory",
			dir:  os.TempDir(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isOrchestratorRoot(tt.dir)
			if got != tt.want {
				t.Errorf("isOrchestratorRoot(%q) = %v, want %v", tt.dir, got, tt.want)
			}
		})
	}
}

func TestEnvironmentVariableRoot(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	// Save original value
	original := os.Getenv("OPENEXEC_ROOT")
	defer func() {
		if original != "" {
			os.Setenv("OPENEXEC_ROOT", original)
		} else {
			os.Unsetenv("OPENEXEC_ROOT")
		}
	}()

	// Set environment variable to the actual root
	os.Setenv("OPENEXEC_ROOT", root)

	detected, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("DetectOrchestratorRoot() with env var error = %v", err)
	}
	if detected != root {
		t.Errorf("DetectOrchestratorRoot() with env var = %q, want %q", detected, root)
	}

	// Test with invalid environment variable
	os.Setenv("OPENEXEC_ROOT", "/nonexistent/path")
	_, err = DetectOrchestratorRoot()
	// Should still succeed by falling back to other methods
	if err != nil {
		t.Logf("DetectOrchestratorRoot() with invalid env var correctly fell back: %v", err)
	}
}
