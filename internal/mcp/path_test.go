package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathValidationError(t *testing.T) {
	err := &PathValidationError{Path: "/test/path", Message: "test error"}
	expected := "/test/path: test error"
	if err.Error() != expected {
		t.Errorf("Error() = %q, want %q", err.Error(), expected)
	}
}

func TestDefaultPathValidatorConfig(t *testing.T) {
    // Ensure deterministic root via env
    wd, _ := os.Getwd()
    os.Setenv("WORKSPACE_ROOT", wd)
    defer os.Unsetenv("WORKSPACE_ROOT")

    config := DefaultPathValidatorConfig()

    if config.AllowedRoots == nil || len(config.AllowedRoots) == 0 {
        t.Fatal("AllowedRoots should default to workspace root")
    }
    if config.AllowedRoots[0] != wd {
        t.Errorf("AllowedRoots[0] = %q, want %q", config.AllowedRoots[0], wd)
    }
	if config.AllowSymlinks {
		t.Error("AllowSymlinks should be false by default")
	}
	if !config.RequireAbsolute {
		t.Error("RequireAbsolute should be true by default")
	}
	if !config.RequireExists {
		t.Error("RequireExists should be true by default")
	}
	if !config.RequireFile {
		t.Error("RequireFile should be true by default")
	}
}

func TestPathValidator_Validate_EmptyPath(t *testing.T) {
	validator := NewPathValidator(PathValidatorConfig{
		RequireExists: false,
	})

	_, err := validator.Validate("")
	if err == nil {
		t.Error("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "path is empty") {
		t.Errorf("expected empty path error, got: %v", err)
	}
}

func TestPathValidator_Validate_NullByte(t *testing.T) {
	validator := NewPathValidator(PathValidatorConfig{
		RequireExists:   false,
		RequireAbsolute: false,
	})

	_, err := validator.Validate("/path/with\x00null")
	if err == nil {
		t.Error("expected error for path with null byte")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected null byte error, got: %v", err)
	}
}

func TestPathValidator_Validate_PathTraversal(t *testing.T) {
	validator := NewPathValidator(PathValidatorConfig{
		RequireExists:   false,
		RequireAbsolute: false,
	})

	traversalPaths := []string{
		"../etc/passwd",
		"foo/../../../etc/passwd",
		"/root/../etc/passwd",
		"..\\windows\\system32",
		"foo\\..\\..\\windows",
	}

	for _, path := range traversalPaths {
		t.Run(path, func(t *testing.T) {
			_, err := validator.Validate(path)
			if err == nil {
				t.Errorf("expected error for path traversal: %s", path)
			}
			if !strings.Contains(err.Error(), "path traversal") {
				t.Errorf("expected path traversal error for %s, got: %v", path, err)
			}
		})
	}
}

func TestPathValidator_Validate_RequireAbsolute(t *testing.T) {
	validator := NewPathValidator(PathValidatorConfig{
		RequireExists:   false,
		RequireAbsolute: true,
	})

	_, err := validator.Validate("relative/path/file.txt")
	if err == nil {
		t.Error("expected error for relative path when RequireAbsolute is true")
	}
	if !strings.Contains(err.Error(), "must be absolute") {
		t.Errorf("expected absolute path error, got: %v", err)
	}
}

func TestPathValidator_Validate_AllowedRoots(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	disallowedDir := filepath.Join(tmpDir, "disallowed")

	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("failed to create allowed dir: %v", err)
	}
	if err := os.MkdirAll(disallowedDir, 0755); err != nil {
		t.Fatalf("failed to create disallowed dir: %v", err)
	}

	// Create test files
	allowedFile := filepath.Join(allowedDir, "file.txt")
	disallowedFile := filepath.Join(disallowedDir, "file.txt")

	if err := os.WriteFile(allowedFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create allowed file: %v", err)
	}
	if err := os.WriteFile(disallowedFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create disallowed file: %v", err)
	}

	validator := NewPathValidator(PathValidatorConfig{
		AllowedRoots:    []string{allowedDir},
		RequireExists:   true,
		RequireAbsolute: true,
		RequireFile:     true,
	})

	// Test allowed path
	result, err := validator.Validate(allowedFile)
	if err != nil {
		t.Errorf("unexpected error for allowed path: %v", err)
	}
	if result != allowedFile {
		t.Errorf("result = %q, want %q", result, allowedFile)
	}

	// Test disallowed path
	_, err = validator.Validate(disallowedFile)
	if err == nil {
		t.Error("expected error for path outside allowed root")
	}
	if !strings.Contains(err.Error(), "outside allowed root") {
		t.Errorf("expected outside root error, got: %v", err)
	}
}

func TestPathValidator_Validate_RequireExists(t *testing.T) {
	validator := NewPathValidator(PathValidatorConfig{
		RequireExists:   true,
		RequireAbsolute: true,
	})

	_, err := validator.Validate("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected not exists error, got: %v", err)
	}
}

func TestPathValidator_Validate_RequireFile(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	validator := NewPathValidator(PathValidatorConfig{
		RequireExists:   true,
		RequireAbsolute: true,
		RequireFile:     true,
	})

	_, err := validator.Validate(tmpDir)
	if err == nil {
		t.Error("expected error for directory path")
	}
	if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("expected directory error, got: %v", err)
	}
}

func TestPathValidator_Validate_Symlinks(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create a real file
	realFile := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create a symlink
	symlinkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, symlinkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// Test with symlinks disallowed
	t.Run("symlinks_disallowed", func(t *testing.T) {
		validator := NewPathValidator(PathValidatorConfig{
			AllowSymlinks:   false,
			RequireExists:   true,
			RequireAbsolute: true,
			RequireFile:     true,
		})

		_, err := validator.Validate(symlinkFile)
		if err == nil {
			t.Error("expected error for symlink when disallowed")
		}
		if !strings.Contains(err.Error(), "symlinks are not allowed") {
			t.Errorf("expected symlink error, got: %v", err)
		}
	})

	// Test with symlinks allowed
	t.Run("symlinks_allowed", func(t *testing.T) {
		validator := NewPathValidator(PathValidatorConfig{
			AllowSymlinks:   true,
			RequireExists:   true,
			RequireAbsolute: true,
			RequireFile:     true,
		})

		result, err := validator.Validate(symlinkFile)
		if err != nil {
			t.Errorf("unexpected error for symlink when allowed: %v", err)
		}
		// Result should be the resolved real path
		// On macOS, /var is a symlink to /private/var, so we need to resolve both paths
		expectedPath, _ := filepath.EvalSymlinks(realFile)
		if result != expectedPath {
			t.Errorf("result = %q, want %q (resolved real path)", result, expectedPath)
		}
	})
}

func TestPathValidator_Validate_SymlinkOutsideRoot(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	outsideDir := filepath.Join(tmpDir, "outside")

	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("failed to create allowed dir: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatalf("failed to create outside dir: %v", err)
	}

	// Create a file outside allowed root
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to create outside file: %v", err)
	}

	// Create a symlink inside allowed root pointing outside
	symlinkFile := filepath.Join(allowedDir, "escape.txt")
	if err := os.Symlink(outsideFile, symlinkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	validator := NewPathValidator(PathValidatorConfig{
		AllowedRoots:    []string{allowedDir},
		AllowSymlinks:   true, // Allow symlinks but validate target
		RequireExists:   true,
		RequireAbsolute: true,
		RequireFile:     true,
	})

	_, err := validator.Validate(symlinkFile)
	if err == nil {
		t.Error("expected error for symlink pointing outside root")
	}
	if !strings.Contains(err.Error(), "symlink target is outside") {
		t.Errorf("expected symlink outside root error, got: %v", err)
	}
}

func TestPathValidator_Validate_ValidFile(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	validator := NewPathValidator(PathValidatorConfig{
		RequireExists:   true,
		RequireAbsolute: true,
		RequireFile:     true,
	})

	result, err := validator.Validate(testFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != testFile {
		t.Errorf("result = %q, want %q", result, testFile)
	}
}

func TestValidatePath(t *testing.T) {
	// Create a temporary file for testing
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Test valid path
	result, err := ValidatePath(testFile)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != testFile {
		t.Errorf("result = %q, want %q", result, testFile)
	}

	// Test invalid path (relative)
	_, err = ValidatePath("relative/path.txt")
	if err == nil {
		t.Error("expected error for relative path")
	}
}

func TestValidatePathWithRoots(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")

	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("failed to create allowed dir: %v", err)
	}

	// Create test file
	testFile := filepath.Join(allowedDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result, err := ValidatePathWithRoots(testFile, []string{allowedDir})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != testFile {
		t.Errorf("result = %q, want %q", result, testFile)
	}
}

func TestValidatePathForRead(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Test with allowed roots
	result, err := ValidatePathForRead(testFile, []string{tmpDir})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != testFile {
		t.Errorf("result = %q, want %q", result, testFile)
	}

	// Test with empty roots (no restriction)
	result, err = ValidatePathForRead(testFile, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != testFile {
		t.Errorf("result = %q, want %q", result, testFile)
	}
}

func TestValidatePathForRead_RelativePath(t *testing.T) {
	// Create a temporary file in current directory
	tmpFile, err := os.CreateTemp(".", "test-*.txt")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// ValidatePathForRead should allow relative paths
	result, err := ValidatePathForRead(tmpFile.Name(), nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Result should be an absolute path
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute result path, got %q", result)
	}
}

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "absolute path",
			path:    "/some/path/file.txt",
			wantErr: false,
		},
		{
			name:    "relative path",
			path:    "some/path/file.txt",
			wantErr: false,
		},
		{
			name:    "path with dots",
			path:    "/some/./path/file.txt",
			wantErr: false,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "path with null byte",
			path:    "/path/with\x00null",
			wantErr: true,
		},
		{
			name:    "path traversal",
			path:    "../../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SanitizePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for path %q", tt.path)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for path %q: %v", tt.path, err)
				}
				if !filepath.IsAbs(result) {
					t.Errorf("expected absolute path result, got %q", result)
				}
			}
		})
	}
}

func TestIsPathSafe(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "valid absolute path",
			path: "/some/path/file.txt",
			want: true,
		},
		{
			name: "valid relative path",
			path: "some/path/file.txt",
			want: true,
		},
		{
			name: "empty path",
			path: "",
			want: false,
		},
		{
			name: "path with null byte",
			path: "/path/with\x00null",
			want: false,
		},
		{
			name: "path traversal with ../",
			path: "../../../etc/passwd",
			want: false,
		},
		{
			name: "path traversal with ..\\ ",
			path: "..\\..\\windows\\system32",
			want: false,
		},
		{
			name: "path with embedded traversal",
			path: "/root/../etc/passwd",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPathSafe(tt.path)
			if result != tt.want {
				t.Errorf("IsPathSafe(%q) = %v, want %v", tt.path, result, tt.want)
			}
		})
	}
}

func TestPathValidator_containsTraversal(t *testing.T) {
	validator := &PathValidator{}

	tests := []struct {
		path         string
		hasTraversal bool
	}{
		{"/normal/path", false},
		{"normal/path", false},
		{"../parent", true},
		{"..\\parent", true},
		{"/root/../etc", true},
		{"foo/../../bar", true},
		{"/path/./file", false},
		{"./current", false},
		{"file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := validator.containsTraversal(tt.path)
			if result != tt.hasTraversal {
				t.Errorf("containsTraversal(%q) = %v, want %v", tt.path, result, tt.hasTraversal)
			}
		})
	}
}

func TestPathValidator_isWithinAllowedRoots(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir1 := filepath.Join(tmpDir, "allowed1")
	allowedDir2 := filepath.Join(tmpDir, "allowed2")
	disallowedDir := filepath.Join(tmpDir, "disallowed")

	// Create directories
	for _, dir := range []string{allowedDir1, allowedDir2, disallowedDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", dir, err)
		}
	}

	validator := NewPathValidator(PathValidatorConfig{
		AllowedRoots: []string{allowedDir1, allowedDir2},
	})

	tests := []struct {
		path     string
		expected bool
	}{
		{filepath.Join(allowedDir1, "file.txt"), true},
		{filepath.Join(allowedDir2, "subdir", "file.txt"), true},
		{allowedDir1, true},
		{filepath.Join(disallowedDir, "file.txt"), false},
		{tmpDir, false}, // Parent of allowed dirs
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := validator.isWithinAllowedRoots(tt.path)
			if result != tt.expected {
				t.Errorf("isWithinAllowedRoots(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestPathValidator_MultipleAllowedRoots(t *testing.T) {
	tmpDir := t.TempDir()
	root1 := filepath.Join(tmpDir, "root1")
	root2 := filepath.Join(tmpDir, "root2")

	for _, dir := range []string{root1, root2} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	// Create files in both roots
	file1 := filepath.Join(root1, "file1.txt")
	file2 := filepath.Join(root2, "file2.txt")

	for _, f := range []string{file1, file2} {
		if err := os.WriteFile(f, []byte("test"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	validator := NewPathValidator(PathValidatorConfig{
		AllowedRoots:    []string{root1, root2},
		RequireExists:   true,
		RequireAbsolute: true,
		RequireFile:     true,
	})

	// Both files should be accessible
	for _, f := range []string{file1, file2} {
		_, err := validator.Validate(f)
		if err != nil {
			t.Errorf("unexpected error for %q: %v", f, err)
		}
	}
}

// ==================== ValidatePathForWrite Tests ====================

func TestValidatePathForWrite(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()

	// Test with allowed roots
	newFile := filepath.Join(tmpDir, "newfile.txt")
	result, err := ValidatePathForWrite(newFile, []string{tmpDir})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != newFile {
		t.Errorf("result = %q, want %q", result, newFile)
	}

	// Test with empty roots (no restriction)
	result, err = ValidatePathForWrite(newFile, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != newFile {
		t.Errorf("result = %q, want %q", result, newFile)
	}
}

func TestValidatePathForWrite_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Non-existent file should be valid for writing
	newFile := filepath.Join(tmpDir, "nonexistent.txt")
	result, err := ValidatePathForWrite(newFile, nil)
	if err != nil {
		t.Errorf("unexpected error for non-existent file: %v", err)
	}
	if result != newFile {
		t.Errorf("result = %q, want %q", result, newFile)
	}
}

func TestValidatePathForWrite_RelativePath(t *testing.T) {
	// ValidatePathForWrite should allow relative paths and resolve them
	result, err := ValidatePathForWrite("relative/path/file.txt", nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Result should be an absolute path
	if !filepath.IsAbs(result) {
		t.Errorf("expected absolute result path, got %q", result)
	}
}

func TestValidatePathForWrite_PathTraversal(t *testing.T) {
	// Path traversal should be rejected
	_, err := ValidatePathForWrite("../../../etc/passwd", nil)
	if err == nil {
		t.Error("expected error for path traversal")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("expected path traversal error, got: %v", err)
	}
}

func TestValidatePathForWrite_OutsideRoot(t *testing.T) {
	tmpDir := t.TempDir()
	allowedDir := filepath.Join(tmpDir, "allowed")
	outsideFile := filepath.Join(tmpDir, "outside", "file.txt")

	if err := os.MkdirAll(allowedDir, 0755); err != nil {
		t.Fatalf("failed to create allowed dir: %v", err)
	}

	_, err := ValidatePathForWrite(outsideFile, []string{allowedDir})
	if err == nil {
		t.Error("expected error for path outside allowed root")
	}
	if !strings.Contains(err.Error(), "outside allowed root") {
		t.Errorf("expected outside root error, got: %v", err)
	}
}

func TestValidatePathForWrite_EmptyPath(t *testing.T) {
	_, err := ValidatePathForWrite("", nil)
	if err == nil {
		t.Error("expected error for empty path")
	}
	if !strings.Contains(err.Error(), "path is empty") {
		t.Errorf("expected empty path error, got: %v", err)
	}
}

func TestValidatePathForWrite_NullByte(t *testing.T) {
	_, err := ValidatePathForWrite("/path/with\x00null", nil)
	if err == nil {
		t.Error("expected error for path with null byte")
	}
	if !strings.Contains(err.Error(), "null byte") {
		t.Errorf("expected null byte error, got: %v", err)
	}
}

func TestValidateParentDirectoryForWrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with existing parent directory
	newFile := filepath.Join(tmpDir, "newfile.txt")
	err := ValidateParentDirectoryForWrite(newFile)
	if err != nil {
		t.Errorf("unexpected error for existing parent: %v", err)
	}
}

func TestValidateParentDirectoryForWrite_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with non-existent parent directory
	newFile := filepath.Join(tmpDir, "nonexistent", "subdir", "file.txt")
	err := ValidateParentDirectoryForWrite(newFile)
	if err == nil {
		t.Error("expected error for non-existent parent")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected parent not exist error, got: %v", err)
	}
}

func TestValidateParentDirectoryForWrite_ParentIsFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file where a directory should be
	fakeDir := filepath.Join(tmpDir, "fakedir")
	if err := os.WriteFile(fakeDir, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("failed to create fake dir file: %v", err)
	}

	// Try to write inside the "directory" that's actually a file
	newFile := filepath.Join(fakeDir, "file.txt")
	err := ValidateParentDirectoryForWrite(newFile)
	if err == nil {
		t.Error("expected error when parent is a file")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected not a directory error, got: %v", err)
	}
}

func TestValidatePathForWrite_WithExistingFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create an existing file
	existingFile := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("existing content"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}

	// Should still validate successfully (file will be overwritten)
	result, err := ValidatePathForWrite(existingFile, nil)
	if err != nil {
		t.Errorf("unexpected error for existing file: %v", err)
	}
	if result != existingFile {
		t.Errorf("result = %q, want %q", result, existingFile)
	}
}

func TestValidatePathForWrite_NestedNewPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Path with nested directories that don't exist yet
	nestedFile := filepath.Join(tmpDir, "level1", "level2", "level3", "file.txt")

	// ValidatePathForWrite should succeed (it doesn't check parent existence)
	result, err := ValidatePathForWrite(nestedFile, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != nestedFile {
		t.Errorf("result = %q, want %q", result, nestedFile)
	}
}

func TestValidatePathForWrite_SymlinkHandling(t *testing.T) {
	// Note: ValidatePathForWrite with RequireExists=false does NOT check for
	// symlinks since the file may not exist yet. Symlink checking only happens
	// when RequireExists=true and the file is found to be a symlink.
	//
	// For existing symlinks that need to be validated before overwriting,
	// the application should use a validator with RequireExists=true and
	// AllowSymlinks=false.

	tmpDir := t.TempDir()

	// Create a real file
	realFile := filepath.Join(tmpDir, "real.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create real file: %v", err)
	}

	// Create a symlink
	symlinkFile := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(realFile, symlinkFile); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// ValidatePathForWrite does not check for existing symlinks (RequireExists=false)
	// This allows writing to new files that don't exist yet
	result, err := ValidatePathForWrite(symlinkFile, nil)
	if err != nil {
		t.Errorf("ValidatePathForWrite doesn't check existing file type: %v", err)
	}
	if result != symlinkFile {
		t.Errorf("result = %q, want %q", result, symlinkFile)
	}

	// If an application wants to reject existing symlinks, it should use
	// a custom validator with RequireExists=true and AllowSymlinks=false
	t.Run("custom_validator_rejects_symlinks", func(t *testing.T) {
		config := PathValidatorConfig{
			AllowSymlinks:   false,
			RequireExists:   true,
			RequireAbsolute: false,
			RequireFile:     true,
		}
		validator := NewPathValidator(config)

		_, err := validator.Validate(symlinkFile)
		if err == nil {
			t.Error("expected error for symlink when disallowed")
		}
		if !strings.Contains(err.Error(), "symlinks are not allowed") {
			t.Errorf("expected symlink error, got: %v", err)
		}
	})
}
