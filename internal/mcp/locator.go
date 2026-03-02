// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements the OrchestratorLocator for locating and validating
// orchestrator files for meta self-fix operations.
package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// OrchestratorFileType represents the type of orchestrator file.
type OrchestratorFileType string

const (
	// FileTypeGoSource represents Go source files.
	FileTypeGoSource OrchestratorFileType = "go_source"
	// FileTypePython represents Python source files.
	FileTypePython OrchestratorFileType = "python"
	// FileTypeConfig represents configuration files.
	FileTypeConfig OrchestratorFileType = "config"
	// FileTypeScript represents shell/utility scripts.
	FileTypeScript OrchestratorFileType = "script"
	// FileTypeUnknown represents unrecognized files.
	FileTypeUnknown OrchestratorFileType = "unknown"
)

// OrchestratorFile represents a located orchestrator file.
type OrchestratorFile struct {
	// Path is the absolute path to the file.
	Path string `json:"path"`
	// RelativePath is the path relative to the orchestrator root.
	RelativePath string `json:"relative_path"`
	// Type indicates the type of file.
	Type OrchestratorFileType `json:"type"`
	// Package is the Go package name (for Go files only).
	Package string `json:"package,omitempty"`
	// IsTest indicates if this is a test file.
	IsTest bool `json:"is_test"`
	// Size is the file size in bytes.
	Size int64 `json:"size"`
}

// OrchestratorLocator provides functionality to locate and validate
// orchestrator source files for meta self-fix operations.
type OrchestratorLocator struct {
	// root is the orchestrator's root directory.
	root string
	// sourcePatterns are patterns that identify orchestrator source directories.
	sourcePatterns []string
	// excludePatterns are patterns to exclude from results.
	excludePatterns []string
}

// OrchestratorLocatorConfig holds configuration for the locator.
type OrchestratorLocatorConfig struct {
	// Root is the orchestrator's root directory. If empty, auto-detected.
	Root string
	// SourcePatterns are additional source directory patterns.
	SourcePatterns []string
	// ExcludePatterns are patterns to exclude (e.g., vendor, .git).
	ExcludePatterns []string
}

// Common errors for orchestrator location operations.
var (
	ErrOrchestratorNotFound = errors.New("orchestrator root directory not found")
	ErrNotOrchestratorFile  = errors.New("path is not an orchestrator file")
	ErrInvalidOrchestratorPath = errors.New("invalid orchestrator path")
)

// DefaultOrchestratorLocatorConfig returns the default configuration.
func DefaultOrchestratorLocatorConfig() OrchestratorLocatorConfig {
	return OrchestratorLocatorConfig{
		SourcePatterns: []string{
			"cmd",
			"internal",
			"pkg",
			"scripts",
		},
		ExcludePatterns: []string{
			".git",
			"vendor",
			"node_modules",
			"testdata",
			"coverage*",
			"*.log",
		},
	}
}

// NewOrchestratorLocator creates a new OrchestratorLocator.
func NewOrchestratorLocator(cfg OrchestratorLocatorConfig) (*OrchestratorLocator, error) {
	root := cfg.Root
	if root == "" {
		detected, err := DetectOrchestratorRoot()
		if err != nil {
			return nil, fmt.Errorf("failed to detect orchestrator root: %w", err)
		}
		root = detected
	}

	// Validate root exists and is a directory
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("orchestrator root not accessible: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("orchestrator root is not a directory: %s", root)
	}

	// Use default patterns if not specified
	sourcePatterns := cfg.SourcePatterns
	if len(sourcePatterns) == 0 {
		defaults := DefaultOrchestratorLocatorConfig()
		sourcePatterns = defaults.SourcePatterns
	}

	excludePatterns := cfg.ExcludePatterns
	if len(excludePatterns) == 0 {
		defaults := DefaultOrchestratorLocatorConfig()
		excludePatterns = defaults.ExcludePatterns
	}

	return &OrchestratorLocator{
		root:            root,
		sourcePatterns:  sourcePatterns,
		excludePatterns: excludePatterns,
	}, nil
}

// DetectOrchestratorRoot attempts to locate the orchestrator's root directory.
// It uses multiple strategies:
// 1. Environment variable OPENEXEC_ROOT
// 2. Go runtime caller info (for development)
// 3. Executable location (for installed binaries)
// 4. Current working directory markers
func DetectOrchestratorRoot() (string, error) {
	// Strategy 1: Environment variable
	if envRoot := os.Getenv("OPENEXEC_ROOT"); envRoot != "" {
		if isOrchestratorRoot(envRoot) {
			return filepath.Clean(envRoot), nil
		}
	}

	// Strategy 2: Go runtime caller (development mode)
	// This finds the source file location when running from source
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		// This file is in internal/mcp/locator.go
		// Root would be ../../..
		srcRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
		if isOrchestratorRoot(srcRoot) {
			return srcRoot, nil
		}
	}

	// Strategy 3: Executable location
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		// Check if executable is in bin/ subdirectory
		parentDir := filepath.Dir(execDir)
		if isOrchestratorRoot(parentDir) {
			return parentDir, nil
		}
		// Or directly in root
		if isOrchestratorRoot(execDir) {
			return execDir, nil
		}
	}

	// Strategy 4: Walk up from current working directory
	cwd, err := os.Getwd()
	if err == nil {
		root, found := findOrchestratorRootFromPath(cwd)
		if found {
			return root, nil
		}
	}

	return "", ErrOrchestratorNotFound
}

// isOrchestratorRoot checks if a directory appears to be the orchestrator root.
func isOrchestratorRoot(dir string) bool {
	// Must have go.mod with openexec module
	goModPath := filepath.Join(dir, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		content := string(data)
		if strings.Contains(content, "github.com/openexec/openexec") ||
			strings.Contains(content, "module openexec") {
			return true
		}
	}

	// Must have internal/ directory with expected structure
	internalDir := filepath.Join(dir, "internal")
	if info, err := os.Stat(internalDir); err == nil && info.IsDir() {
		// Check for characteristic subdirectories
		for _, subdir := range []string{"mcp", "loop", "agent"} {
			subdirPath := filepath.Join(internalDir, subdir)
			if info, err := os.Stat(subdirPath); err == nil && info.IsDir() {
				return true
			}
		}
	}

	return false
}

// findOrchestratorRootFromPath walks up the directory tree to find the root.
func findOrchestratorRootFromPath(startPath string) (string, bool) {
	current := filepath.Clean(startPath)
	for {
		if isOrchestratorRoot(current) {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Reached filesystem root
			break
		}
		current = parent
	}
	return "", false
}

// Root returns the orchestrator's root directory.
func (l *OrchestratorLocator) Root() string {
	return l.root
}

// IsOrchestratorPath checks if a path belongs to the orchestrator.
func (l *OrchestratorLocator) IsOrchestratorPath(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Path must be within orchestrator root
	rootWithSep := l.root
	if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
		rootWithSep += string(filepath.Separator)
	}

	if absPath != l.root && !strings.HasPrefix(absPath, rootWithSep) {
		return false
	}

	// Check against exclude patterns
	relPath, err := filepath.Rel(l.root, absPath)
	if err != nil {
		return false
	}

	for _, pattern := range l.excludePatterns {
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return false
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
			return false
		}
		// Check if any path component matches
		parts := strings.Split(relPath, string(filepath.Separator))
		for _, part := range parts {
			if matched, _ := filepath.Match(pattern, part); matched {
				return false
			}
		}
	}

	return true
}

// Locate finds an orchestrator file by path or pattern.
// If the path is relative, it's resolved against the orchestrator root.
func (l *OrchestratorLocator) Locate(pathOrPattern string) (*OrchestratorFile, error) {
	// Resolve path
	var absPath string
	if filepath.IsAbs(pathOrPattern) {
		absPath = pathOrPattern
	} else {
		absPath = filepath.Join(l.root, pathOrPattern)
	}

	// Validate it's an orchestrator path
	if !l.IsOrchestratorPath(absPath) {
		return nil, ErrNotOrchestratorFile
	}

	// Get file info
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", absPath)
	}

	// Build orchestrator file info
	relPath, _ := filepath.Rel(l.root, absPath)
	fileType := l.classifyFile(absPath)

	file := &OrchestratorFile{
		Path:         absPath,
		RelativePath: relPath,
		Type:         fileType,
		IsTest:       l.isTestFile(absPath),
		Size:         info.Size(),
	}

	// Extract Go package name if applicable
	if fileType == FileTypeGoSource {
		file.Package = l.extractGoPackage(absPath)
	}

	return file, nil
}

// LocateByType finds all orchestrator files of a specific type.
func (l *OrchestratorLocator) LocateByType(fileType OrchestratorFileType) ([]*OrchestratorFile, error) {
	var files []*OrchestratorFile

	err := filepath.Walk(l.root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			// Skip excluded directories
			baseName := filepath.Base(path)
			for _, pattern := range l.excludePatterns {
				if matched, _ := filepath.Match(pattern, baseName); matched {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check if path is valid orchestrator file
		if !l.IsOrchestratorPath(path) {
			return nil
		}

		// Check file type
		if l.classifyFile(path) != fileType {
			return nil
		}

		relPath, _ := filepath.Rel(l.root, path)
		file := &OrchestratorFile{
			Path:         path,
			RelativePath: relPath,
			Type:         fileType,
			IsTest:       l.isTestFile(path),
			Size:         info.Size(),
		}

		if fileType == FileTypeGoSource {
			file.Package = l.extractGoPackage(path)
		}

		files = append(files, file)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// LocateInPackage finds orchestrator files in a specific Go package.
func (l *OrchestratorLocator) LocateInPackage(packagePath string) ([]*OrchestratorFile, error) {
	// Determine the directory for the package
	var pkgDir string
	if strings.HasPrefix(packagePath, "github.com/openexec/openexec/") {
		// Full import path
		relPath := strings.TrimPrefix(packagePath, "github.com/openexec/openexec/")
		pkgDir = filepath.Join(l.root, relPath)
	} else {
		// Relative path (e.g., "internal/mcp")
		pkgDir = filepath.Join(l.root, packagePath)
	}

	// Validate directory exists
	info, err := os.Stat(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("package directory not found: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("package path is not a directory: %s", pkgDir)
	}

	// Find all Go files in the package
	var files []*OrchestratorFile

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read package directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(pkgDir, entry.Name())
		if l.classifyFile(path) != FileTypeGoSource {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		relPath, _ := filepath.Rel(l.root, path)
		files = append(files, &OrchestratorFile{
			Path:         path,
			RelativePath: relPath,
			Type:         FileTypeGoSource,
			Package:      l.extractGoPackage(path),
			IsTest:       l.isTestFile(path),
			Size:         info.Size(),
		})
	}

	return files, nil
}

// ValidateForEdit validates that a path is safe to edit as an orchestrator file.
// Returns an error if the path should not be edited.
func (l *OrchestratorLocator) ValidateForEdit(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// Must be an orchestrator path
	if !l.IsOrchestratorPath(absPath) {
		return ErrNotOrchestratorFile
	}

	// File must exist for editing (not for creation)
	if _, err := os.Stat(absPath); err != nil {
		if os.IsNotExist(err) {
			// Allow creating new files in source directories
			relPath, _ := filepath.Rel(l.root, absPath)
			parts := strings.Split(relPath, string(filepath.Separator))
			if len(parts) > 0 {
				for _, sourcePattern := range l.sourcePatterns {
					if parts[0] == sourcePattern {
						return nil // Allow creating in source directories
					}
				}
			}
			return fmt.Errorf("cannot create files outside source directories")
		}
		return fmt.Errorf("cannot access file: %w", err)
	}

	// Validate file type is editable
	fileType := l.classifyFile(absPath)
	switch fileType {
	case FileTypeGoSource, FileTypePython, FileTypeConfig, FileTypeScript:
		return nil // Editable types
	default:
		return fmt.Errorf("file type %s is not editable", fileType)
	}
}

// GetSourceDirectories returns the paths to orchestrator source directories.
func (l *OrchestratorLocator) GetSourceDirectories() []string {
	var dirs []string
	for _, pattern := range l.sourcePatterns {
		dirPath := filepath.Join(l.root, pattern)
		if info, err := os.Stat(dirPath); err == nil && info.IsDir() {
			dirs = append(dirs, dirPath)
		}
	}
	return dirs
}

// classifyFile determines the type of a file based on its extension and path.
func (l *OrchestratorLocator) classifyFile(path string) OrchestratorFileType {
	ext := strings.ToLower(filepath.Ext(path))
	baseName := filepath.Base(path)

	switch ext {
	case ".go":
		return FileTypeGoSource
	case ".py":
		return FileTypePython
	case ".json", ".yaml", ".yml", ".toml":
		return FileTypeConfig
	case ".sh", ".bash":
		return FileTypeScript
	}

	// Check common config file names
	configNames := []string{
		"Makefile", "Dockerfile", ".gitignore", ".golangci.yml",
		"go.mod", "go.sum", "package.json", "tsconfig.json",
	}
	for _, name := range configNames {
		if baseName == name {
			return FileTypeConfig
		}
	}

	return FileTypeUnknown
}

// isTestFile checks if a file is a test file.
func (l *OrchestratorLocator) isTestFile(path string) bool {
	baseName := filepath.Base(path)

	// Go test files
	if strings.HasSuffix(baseName, "_test.go") {
		return true
	}

	// Python test files
	if strings.HasPrefix(baseName, "test_") && strings.HasSuffix(baseName, ".py") {
		return true
	}
	if strings.HasSuffix(baseName, "_test.py") {
		return true
	}

	return false
}

// extractGoPackage extracts the package name from a Go source file.
func (l *OrchestratorLocator) extractGoPackage(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "package ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				return parts[1]
			}
		}
	}

	return ""
}

// ResolveOrchestratorPath resolves a path relative to the orchestrator root.
// Returns the absolute path and validates it's within the orchestrator.
func (l *OrchestratorLocator) ResolveOrchestratorPath(path string) (string, error) {
	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(l.root, path))
	}

	// Validate path is within orchestrator
	if !l.IsOrchestratorPath(absPath) {
		return "", ErrInvalidOrchestratorPath
	}

	return absPath, nil
}

// GetOrchestratorInfo returns metadata about the orchestrator installation.
func (l *OrchestratorLocator) GetOrchestratorInfo() map[string]interface{} {
	info := map[string]interface{}{
		"root":            l.root,
		"source_patterns": l.sourcePatterns,
	}

	// Check for version info
	goModPath := filepath.Join(l.root, "go.mod")
	if data, err := os.ReadFile(goModPath); err == nil {
		content := string(data)
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "go ") {
				info["go_version"] = strings.TrimSpace(strings.TrimPrefix(line, "go "))
				break
			}
		}
	}

	// Count source files
	goCount := 0
	testCount := 0
	if files, err := l.LocateByType(FileTypeGoSource); err == nil {
		goCount = len(files)
		for _, f := range files {
			if f.IsTest {
				testCount++
			}
		}
	}
	info["go_files"] = goCount
	info["test_files"] = testCount

	return info
}
