// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file contains integration tests for the meta self-fix file edit capability.
//
// Meta Edit combines three components:
// 1. OrchestratorLocator - Detects and validates orchestrator file paths
// 2. PythonValidator - Validates Python syntax before writes
// 3. OrchestratorRiskEscalator (via approval package) - Escalates risk for orchestrator edits
package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestMetaEditLocatorAndValidatorIntegration tests the integration between
// the OrchestratorLocator and PythonValidator for meta self-fix operations.
func TestMetaEditLocatorAndValidatorIntegration(t *testing.T) {
	// Skip if we can't detect the orchestrator root (not running from source)
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	validator := NewPythonValidator()

	t.Run("validates_python_files_in_orchestrator", func(t *testing.T) {
		// A valid Python file should pass validation
		validCode := "def hello():\n    return 'world'\n"
		result := validator.ValidateCode(validCode, "test_module.py")
		if !result.Valid {
			t.Errorf("Expected valid Python code to pass validation, got errors: %v", result.Errors)
		}
	})

	t.Run("rejects_invalid_python_for_orchestrator_paths", func(t *testing.T) {
		// An invalid Python file should fail validation
		invalidCode := "def hello(\n    return 'world'\n"
		result := validator.ValidateCode(invalidCode, "test_module.py")
		if result.Valid {
			t.Error("Expected invalid Python code to fail validation")
		}
		if len(result.Errors) == 0 {
			t.Error("Expected at least one error for invalid Python")
		}
	})

	t.Run("locator_identifies_orchestrator_internal_paths", func(t *testing.T) {
		internalPath := filepath.Join(root, "internal", "mcp", "server.go")
		if !locator.IsOrchestratorPath(internalPath) {
			t.Errorf("Expected %s to be identified as orchestrator path", internalPath)
		}
	})

	t.Run("locator_rejects_external_paths", func(t *testing.T) {
		externalPath := "/tmp/some_external_file.go"
		if locator.IsOrchestratorPath(externalPath) {
			t.Errorf("Expected %s to NOT be identified as orchestrator path", externalPath)
		}
	})

	t.Run("locator_identifies_go_source_files", func(t *testing.T) {
		serverFile := filepath.Join(root, "internal", "mcp", "server.go")
		file, err := locator.Locate(serverFile)
		if err != nil {
			t.Fatalf("Failed to locate server.go: %v", err)
		}
		if file.Type != FileTypeGoSource {
			t.Errorf("Expected FileTypeGoSource, got %v", file.Type)
		}
		if file.Package != "mcp" {
			t.Errorf("Expected package 'mcp', got %q", file.Package)
		}
	})
}

// TestMetaEditValidateForEditIntegration tests the ValidateForEdit functionality
// which is crucial for the meta self-fix capability.
func TestMetaEditValidateForEditIntegration(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	t.Run("allows_editing_existing_source_files", func(t *testing.T) {
		// An existing Go source file in internal/ should be editable
		serverFile := filepath.Join(root, "internal", "mcp", "server.go")
		if err := locator.ValidateForEdit(serverFile); err != nil {
			t.Errorf("Expected to allow editing existing source file, got error: %v", err)
		}
	})

	t.Run("allows_editing_existing_test_files", func(t *testing.T) {
		// Test files should also be editable
		testFile := filepath.Join(root, "internal", "mcp", "server_test.go")
		if err := locator.ValidateForEdit(testFile); err != nil {
			t.Errorf("Expected to allow editing existing test file, got error: %v", err)
		}
	})

	t.Run("allows_creating_new_files_in_source_directories", func(t *testing.T) {
		// A new file in internal/ should be allowed
		newFile := filepath.Join(root, "internal", "mcp", "new_feature.go")
		// ValidateForEdit should allow this even if the file doesn't exist
		if err := locator.ValidateForEdit(newFile); err != nil {
			t.Errorf("Expected to allow creating new file in source directory, got error: %v", err)
		}
	})

	t.Run("rejects_creating_files_outside_source_directories", func(t *testing.T) {
		// A new file outside recognized source directories should be rejected
		newFile := filepath.Join(root, "random_new_dir", "file.go")
		if err := locator.ValidateForEdit(newFile); err == nil {
			t.Error("Expected to reject creating file outside source directories")
		}
	})

	t.Run("rejects_editing_external_paths", func(t *testing.T) {
		// A file outside the orchestrator root should be rejected
		externalFile := "/tmp/malicious_file.go"
		if err := locator.ValidateForEdit(externalFile); err == nil {
			t.Error("Expected to reject editing external file")
		}
	})

	t.Run("rejects_editing_git_directory", func(t *testing.T) {
		// Files in .git should be rejected
		gitFile := filepath.Join(root, ".git", "config")
		if err := locator.ValidateForEdit(gitFile); err == nil {
			t.Error("Expected to reject editing .git directory files")
		}
	})

	t.Run("rejects_editing_vendor_directory", func(t *testing.T) {
		// Files in vendor/ should be rejected
		vendorFile := filepath.Join(root, "vendor", "some_package", "file.go")
		if err := locator.ValidateForEdit(vendorFile); err == nil {
			t.Error("Expected to reject editing vendor directory files")
		}
	})
}

// TestMetaEditPythonSyntaxValidationForOrchestratorFiles tests Python validation
// specifically for orchestrator Python files (if any exist).
func TestMetaEditPythonSyntaxValidationForOrchestratorFiles(t *testing.T) {
	skipIfNoPython(t)

	validator := NewPythonValidatorWithConfig(&PythonValidatorConfig{
		PythonPath:     "python3",
		Timeout:        5 * time.Second,
		SkipIfNoPython: false,
	})

	t.Run("valid_python_passes_for_orchestrator_file", func(t *testing.T) {
		validCode := `#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""Orchestrator utility script."""

import os
import sys

def main():
    """Main entry point."""
    print("Hello from orchestrator")
    return 0

if __name__ == "__main__":
    sys.exit(main())
`
		result := validator.ValidateCode(validCode, "internal/scripts/util.py")
		if !result.Valid {
			t.Errorf("Expected valid Python to pass, got errors: %v", result.Errors)
		}
		// Check stats
		if !result.Stats.HasShebang {
			t.Error("Expected HasShebang to be true")
		}
		if !result.Stats.HasEncoding {
			t.Error("Expected HasEncoding to be true")
		}
	})

	t.Run("invalid_syntax_rejected_for_orchestrator_file", func(t *testing.T) {
		invalidCode := `def broken_function(
    # Missing closing parenthesis
    return None
`
		result := validator.ValidateCode(invalidCode, "internal/scripts/broken.py")
		if result.Valid {
			t.Error("Expected invalid Python to fail validation")
		}
	})

	t.Run("async_python_syntax_validated", func(t *testing.T) {
		asyncCode := `async def fetch_data():
    import asyncio
    await asyncio.sleep(1)
    return {"status": "ok"}
`
		result := validator.ValidateCode(asyncCode, "internal/scripts/async_util.py")
		if !result.Valid {
			t.Errorf("Expected async Python to pass, got errors: %v", result.Errors)
		}
	})

	t.Run("type_hints_validated", func(t *testing.T) {
		typedCode := `from typing import Dict, List, Optional

def process(items: List[str], config: Optional[Dict[str, any]] = None) -> bool:
    if config is None:
        config = {}
    return len(items) > 0
`
		result := validator.ValidateCode(typedCode, "internal/scripts/typed.py")
		if !result.Valid {
			t.Errorf("Expected typed Python to pass, got errors: %v", result.Errors)
		}
	})
}

// TestMetaEditFileTypeClassification tests proper classification of file types
// for the meta edit capability.
func TestMetaEditFileTypeClassification(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	tests := []struct {
		relativePath string
		expectedType OrchestratorFileType
		expectedTest bool
	}{
		{"internal/mcp/server.go", FileTypeGoSource, false},
		{"internal/mcp/server_test.go", FileTypeGoSource, true},
		{"go.mod", FileTypeConfig, false},
		{"go.sum", FileTypeConfig, false},
		{"Makefile", FileTypeConfig, false},
		{"scripts/run.sh", FileTypeScript, false},
	}

	for _, tc := range tests {
		t.Run(tc.relativePath, func(t *testing.T) {
			fullPath := filepath.Join(root, tc.relativePath)

			// Check if file exists before testing
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				t.Skipf("File %s does not exist, skipping", tc.relativePath)
			}

			file, err := locator.Locate(fullPath)
			if err != nil {
				t.Fatalf("Failed to locate file: %v", err)
			}

			if file.Type != tc.expectedType {
				t.Errorf("Expected type %v, got %v", tc.expectedType, file.Type)
			}

			if file.IsTest != tc.expectedTest {
				t.Errorf("Expected IsTest=%v, got %v", tc.expectedTest, file.IsTest)
			}
		})
	}
}

// TestMetaEditLocateByType tests finding orchestrator files by type,
// which is essential for meta self-fix operations that need to discover files.
func TestMetaEditLocateByType(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	t.Run("finds_go_source_files", func(t *testing.T) {
		files, err := locator.LocateByType(FileTypeGoSource)
		if err != nil {
			t.Fatalf("Failed to locate Go files: %v", err)
		}

		if len(files) == 0 {
			t.Error("Expected to find at least some Go source files")
		}

		// All files should be Go files
		for _, f := range files {
			if f.Type != FileTypeGoSource {
				t.Errorf("Expected FileTypeGoSource, got %v for %s", f.Type, f.RelativePath)
			}
			if !strings.HasSuffix(f.Path, ".go") {
				t.Errorf("Expected .go extension, got %s", f.Path)
			}
		}

		// Should include test files
		hasTestFiles := false
		for _, f := range files {
			if f.IsTest {
				hasTestFiles = true
				break
			}
		}
		if !hasTestFiles {
			t.Error("Expected to find some test files")
		}
	})

	t.Run("finds_config_files", func(t *testing.T) {
		files, err := locator.LocateByType(FileTypeConfig)
		if err != nil {
			t.Fatalf("Failed to locate config files: %v", err)
		}

		// Should find at least go.mod
		foundGoMod := false
		for _, f := range files {
			if strings.HasSuffix(f.Path, "go.mod") {
				foundGoMod = true
				break
			}
		}
		if !foundGoMod {
			t.Error("Expected to find go.mod in config files")
		}
	})
}

// TestMetaEditLocateInPackage tests finding files in specific packages,
// which is useful for targeted meta self-fix operations.
func TestMetaEditLocateInPackage(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	t.Run("finds_files_in_mcp_package", func(t *testing.T) {
		files, err := locator.LocateInPackage("internal/mcp")
		if err != nil {
			t.Fatalf("Failed to locate files in mcp package: %v", err)
		}

		if len(files) == 0 {
			t.Error("Expected to find files in internal/mcp package")
		}

		// All files should have package name "mcp"
		for _, f := range files {
			if f.Package != "mcp" {
				t.Errorf("Expected package 'mcp', got %q for %s", f.Package, f.RelativePath)
			}
		}

		// Should include server.go
		foundServer := false
		for _, f := range files {
			if strings.HasSuffix(f.Path, "server.go") {
				foundServer = true
				break
			}
		}
		if !foundServer {
			t.Error("Expected to find server.go in mcp package")
		}
	})

	t.Run("handles_nonexistent_package", func(t *testing.T) {
		_, err := locator.LocateInPackage("internal/nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent package")
		}
	})
}

// TestMetaEditSourceDirectories tests GetSourceDirectories which returns
// the directories that can be edited by meta self-fix operations.
func TestMetaEditSourceDirectories(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	dirs := locator.GetSourceDirectories()
	if len(dirs) == 0 {
		t.Error("Expected at least one source directory")
	}

	// Should include internal/
	foundInternal := false
	for _, dir := range dirs {
		if strings.HasSuffix(dir, "internal") || strings.Contains(dir, "/internal") {
			foundInternal = true
			break
		}
	}
	if !foundInternal {
		t.Error("Expected internal/ in source directories")
	}
}

// TestMetaEditResolveOrchestratorPath tests path resolution and validation
// for meta edit operations.
func TestMetaEditResolveOrchestratorPath(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	t.Run("resolves_relative_paths", func(t *testing.T) {
		resolved, err := locator.ResolveOrchestratorPath("internal/mcp/server.go")
		if err != nil {
			t.Fatalf("Failed to resolve path: %v", err)
		}

		expected := filepath.Join(root, "internal/mcp/server.go")
		if resolved != expected {
			t.Errorf("Expected %s, got %s", expected, resolved)
		}
	})

	t.Run("validates_absolute_paths_in_orchestrator", func(t *testing.T) {
		absPath := filepath.Join(root, "internal/mcp/server.go")
		resolved, err := locator.ResolveOrchestratorPath(absPath)
		if err != nil {
			t.Fatalf("Failed to resolve absolute path: %v", err)
		}
		if resolved != absPath {
			t.Errorf("Expected %s, got %s", absPath, resolved)
		}
	})

	t.Run("rejects_path_traversal", func(t *testing.T) {
		_, err := locator.ResolveOrchestratorPath("../../../etc/passwd")
		if err == nil {
			t.Error("Expected error for path traversal")
		}
	})

	t.Run("rejects_external_absolute_paths", func(t *testing.T) {
		_, err := locator.ResolveOrchestratorPath("/tmp/file.go")
		if err == nil {
			t.Error("Expected error for external absolute path")
		}
	})
}

// TestMetaEditOrchestratorInfo tests GetOrchestratorInfo which provides
// metadata about the orchestrator installation for self-fix operations.
func TestMetaEditOrchestratorInfo(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	info := locator.GetOrchestratorInfo()

	t.Run("includes_root_path", func(t *testing.T) {
		if info["root"] != root {
			t.Errorf("Expected root %s, got %v", root, info["root"])
		}
	})

	t.Run("includes_source_patterns", func(t *testing.T) {
		if info["source_patterns"] == nil {
			t.Error("Expected source_patterns in info")
		}
	})

	t.Run("includes_file_counts", func(t *testing.T) {
		goFiles, ok := info["go_files"].(int)
		if !ok || goFiles == 0 {
			t.Error("Expected non-zero go_files count")
		}

		testFiles, ok := info["test_files"].(int)
		if !ok {
			t.Error("Expected test_files count")
		}
		if testFiles == 0 {
			t.Error("Expected some test files")
		}
	})
}

// TestMetaEditDetectOrchestratorRoot tests the root detection mechanism
// which is fundamental to meta self-fix operations.
func TestMetaEditDetectOrchestratorRoot(t *testing.T) {
	t.Run("detects_root_from_source", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Fatalf("Failed to detect root: %v", err)
		}

		// Verify it looks like the orchestrator root
		if !isOrchestratorRoot(root) {
			t.Errorf("Detected root %s doesn't appear to be orchestrator root", root)
		}
	})

	t.Run("detected_root_contains_go_mod", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Fatalf("Failed to detect root: %v", err)
		}

		goModPath := filepath.Join(root, "go.mod")
		if _, err := os.Stat(goModPath); err != nil {
			t.Errorf("go.mod not found at detected root: %v", err)
		}
	})

	t.Run("detected_root_contains_internal", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Fatalf("Failed to detect root: %v", err)
		}

		internalPath := filepath.Join(root, "internal")
		if info, err := os.Stat(internalPath); err != nil || !info.IsDir() {
			t.Errorf("internal/ directory not found at detected root: %v", err)
		}
	})
}

// TestMetaEditShouldValidatePythonSyntax tests the filter that determines
// which files should have Python syntax validation applied.
func TestMetaEditShouldValidatePythonSyntax(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Should validate
		{"internal/scripts/util.py", true},
		{"scripts/deploy.py", true},
		{"tools/generator.py", true},

		// Should NOT validate (not Python)
		{"internal/mcp/server.go", false},
		{"README.md", false},
		{"config.yaml", false},

		// Should NOT validate (virtual environments)
		{"venv/lib/python3.9/site-packages/module.py", false},
		{".venv/lib/site-packages/pkg.py", false},
		{"virtualenv/lib/module.py", false},

		// Should NOT validate (cache/compiled)
		{"__pycache__/module.cpython-39.pyc", false},

		// Should NOT validate (tox/testing environments)
		{".tox/py39/lib/module.py", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := ShouldValidatePythonSyntax(tc.path)
			if result != tc.expected {
				t.Errorf("ShouldValidatePythonSyntax(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

// TestMetaEditIsPythonFile tests the Python file detection used in meta edit.
func TestMetaEditIsPythonFile(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"script.py", true},
		{"script.PY", true},
		{"script.pyw", true},
		{"internal/mcp/server.go", false},
		{"README.md", false},
		{"config.json", false},
		{".py", true}, // Edge case: just the extension
		{"no_extension", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			result := IsPythonFile(tc.path)
			if result != tc.expected {
				t.Errorf("IsPythonFile(%q) = %v, want %v", tc.path, result, tc.expected)
			}
		})
	}
}

// TestMetaEditExcludePatterns tests that excluded patterns are properly
// filtered from orchestrator path detection.
func TestMetaEditExcludePatterns(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root - skipping integration test")
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	excludedPaths := []string{
		filepath.Join(root, ".git", "config"),
		filepath.Join(root, "vendor", "github.com", "pkg", "file.go"),
		filepath.Join(root, "node_modules", "package", "index.js"),
		filepath.Join(root, "testdata", "fixture.txt"),
		filepath.Join(root, "coverage.out"),
	}

	for _, path := range excludedPaths {
		t.Run(path, func(t *testing.T) {
			if locator.IsOrchestratorPath(path) {
				t.Errorf("Expected %s to be excluded from orchestrator paths", path)
			}
		})
	}
}

// TestMetaEditConfigValidation tests that the locator configuration
// is properly validated and defaults are applied.
func TestMetaEditConfigValidation(t *testing.T) {
	t.Run("default_config_has_expected_patterns", func(t *testing.T) {
		cfg := DefaultOrchestratorLocatorConfig()

		// Check source patterns
		expectedSource := []string{"cmd", "internal", "pkg", "scripts"}
		for _, expected := range expectedSource {
			found := false
			for _, pattern := range cfg.SourcePatterns {
				if pattern == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected source pattern %q in defaults", expected)
			}
		}

		// Check exclude patterns
		expectedExclude := []string{".git", "vendor", "node_modules"}
		for _, expected := range expectedExclude {
			found := false
			for _, pattern := range cfg.ExcludePatterns {
				if pattern == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected exclude pattern %q in defaults", expected)
			}
		}
	})

	t.Run("custom_config_overrides_defaults", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Skip("Cannot detect orchestrator root")
		}

		customCfg := OrchestratorLocatorConfig{
			Root:            root,
			SourcePatterns:  []string{"internal"},
			ExcludePatterns: []string{".git"},
		}

		locator, err := NewOrchestratorLocator(customCfg)
		if err != nil {
			t.Fatalf("Failed to create locator with custom config: %v", err)
		}

		// Should still work
		if locator.Root() != root {
			t.Errorf("Expected root %s, got %s", root, locator.Root())
		}
	})

	t.Run("rejects_nonexistent_root", func(t *testing.T) {
		cfg := OrchestratorLocatorConfig{
			Root: "/nonexistent/path/to/orchestrator",
		}

		_, err := NewOrchestratorLocator(cfg)
		if err == nil {
			t.Error("Expected error for nonexistent root")
		}
	})

	t.Run("rejects_file_as_root", func(t *testing.T) {
		root, err := DetectOrchestratorRoot()
		if err != nil {
			t.Skip("Cannot detect orchestrator root")
		}

		cfg := OrchestratorLocatorConfig{
			Root: filepath.Join(root, "go.mod"), // File, not directory
		}

		_, err = NewOrchestratorLocator(cfg)
		if err == nil {
			t.Error("Expected error for file as root")
		}
	})
}

// TestMetaEditEnvironmentVariableRoot tests that the OPENEXEC_ROOT
// environment variable is respected for meta edit operations.
func TestMetaEditEnvironmentVariableRoot(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Skip("Cannot detect orchestrator root")
	}

	// Save and restore original value
	originalValue := os.Getenv("OPENEXEC_ROOT")
	defer func() {
		if originalValue != "" {
			os.Setenv("OPENEXEC_ROOT", originalValue)
		} else {
			os.Unsetenv("OPENEXEC_ROOT")
		}
	}()

	t.Run("respects_valid_env_var", func(t *testing.T) {
		os.Setenv("OPENEXEC_ROOT", root)

		detected, err := DetectOrchestratorRoot()
		if err != nil {
			t.Fatalf("Failed to detect with env var: %v", err)
		}
		if detected != root {
			t.Errorf("Expected %s from env var, got %s", root, detected)
		}
	})

	t.Run("falls_back_when_env_var_invalid", func(t *testing.T) {
		os.Setenv("OPENEXEC_ROOT", "/nonexistent/invalid/path")

		// Should still detect root via other methods
		detected, err := DetectOrchestratorRoot()
		if err != nil {
			// If running from source, this might work via runtime.Caller
			t.Logf("Detection failed with invalid env var (expected in some environments): %v", err)
			return
		}

		// If it succeeded, it should have fallen back to a valid path
		if !isOrchestratorRoot(detected) {
			t.Error("Fallback detection returned invalid root")
		}
	})
}
