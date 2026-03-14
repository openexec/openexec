// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements path validation utilities for secure file operations.
package mcp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// PathValidationError represents an error that occurred during path validation.
type PathValidationError struct {
	Path    string
	Message string
}

func (e *PathValidationError) Error() string {
	return e.Path + ": " + e.Message
}

// Common path validation errors.
var (
	ErrEmptyPath          = errors.New("path is empty")
	ErrPathTraversal      = errors.New("path traversal detected")
	ErrNullByte           = errors.New("path contains null byte")
	ErrPathNotAbsolute    = errors.New("path must be absolute")
	ErrPathOutsideRoot    = errors.New("path is outside allowed root")
	ErrPathNotExists      = errors.New("path does not exist")
	ErrPathIsDirectory    = errors.New("path is a directory, not a file")
	ErrSymlinkNotAllowed  = errors.New("symlinks are not allowed")
	ErrSymlinkOutsideRoot = errors.New("symlink target is outside allowed root")
)

// PathValidatorConfig holds configuration for the PathValidator.
type PathValidatorConfig struct {
	// AllowedRoots specifies the root directories that paths must be within.
	// If empty, no root restriction is enforced.
	AllowedRoots []string

	// AllowSymlinks determines whether symbolic links are allowed.
	AllowSymlinks bool

	// RequireAbsolute determines whether paths must be absolute.
	RequireAbsolute bool

	// RequireExists determines whether the path must exist on the filesystem.
	RequireExists bool

	// RequireFile determines whether the path must be a file (not a directory).
	RequireFile bool
}

// DefaultPathValidatorConfig returns a default configuration with secure defaults.
// AllowedRoots defaults to WORKSPACE_ROOT when set, otherwise the current working directory.
func DefaultPathValidatorConfig() PathValidatorConfig {
    root := os.Getenv("WORKSPACE_ROOT")
    if root == "" {
        if wd, err := os.Getwd(); err == nil {
            root = wd
        }
    }
    return PathValidatorConfig{
        AllowedRoots:    []string{root},
        AllowSymlinks:   false,
        RequireAbsolute: true,
        RequireExists:   true,
        RequireFile:     true,
    }
}

// PathValidator provides path validation with configurable security checks.
type PathValidator struct {
	config PathValidatorConfig
}

// NewPathValidator creates a new PathValidator with the given configuration.
func NewPathValidator(config PathValidatorConfig) *PathValidator {
	return &PathValidator{config: config}
}

// isDenied returns true if the path points to a sensitive system or project file.
// This is a best-effort denylist to prevent accidental exposure of secrets.
// The list is subject to expansion; see internal security guidelines for updates.
// Defense in depth: this check runs before path validation as an early exit.
func isDenied(path string) bool {
	// Normalize path for case-insensitive matching (handles mixed-case on macOS/Windows)
	lowerPath := strings.ToLower(path)
	base := filepath.Base(path)
	lowerBase := strings.ToLower(base)

	// Block hidden configuration and secret files
	sensitiveFiles := []string{
		".env", ".netrc", ".ssh", ".gnupg",
		"id_rsa", "id_ed25519", "id_dsa",
		"credentials", "secrets", "passwords",
	}

	for _, s := range sensitiveFiles {
		if strings.Contains(lowerBase, s) {
			return true
		}
	}

	// Block certificate/key files by extension
	if strings.HasSuffix(lowerBase, ".pem") || strings.HasSuffix(lowerBase, ".key") || strings.HasSuffix(lowerBase, ".p12") {
		return true
	}

	// Block access to the .git directory (case-insensitive)
	if strings.Contains(lowerPath, "/.git/") || strings.HasSuffix(lowerPath, "/.git") {
		return true
	}

	// Block AWS/cloud credentials directories
	if strings.Contains(lowerPath, "/.aws/") || strings.Contains(lowerPath, "/.gcp/") || strings.Contains(lowerPath, "/.azure/") {
		return true
	}

	return false
}

// Validate validates a file path according to the validator's configuration.
// It returns the canonicalized absolute path if valid, or an error if invalid.
func (v *PathValidator) Validate(path string) (string, error) {
	// Check for empty path
	if path == "" {
		return "", &PathValidationError{Path: path, Message: ErrEmptyPath.Error()}
	}

	// Check for null bytes (security: null byte injection)
	if strings.ContainsRune(path, '\x00') {
		return "", &PathValidationError{Path: path, Message: ErrNullByte.Error()}
	}

	// V1 Hardening: Block sensitive files early
	if isDenied(path) {
		return "", &PathValidationError{Path: path, Message: "access to sensitive file is prohibited by policy"}
	}

	// Clean the path to remove redundant separators and resolve . and ..
	cleanPath := filepath.Clean(path)

	// Check for absolute path requirement
	if v.config.RequireAbsolute && !filepath.IsAbs(cleanPath) {
		return "", &PathValidationError{Path: path, Message: ErrPathNotAbsolute.Error()}
	}

	// Convert to absolute path for consistent handling
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", &PathValidationError{Path: path, Message: "failed to resolve absolute path: " + err.Error()}
	}

	// Detect path traversal attempts by comparing cleaned path with original intent
	// This catches cases like "/allowed/../etc/passwd"
	if v.containsTraversal(path) {
		return "", &PathValidationError{Path: path, Message: ErrPathTraversal.Error()}
	}

	// Check allowed roots
	if len(v.config.AllowedRoots) > 0 {
		if !v.isWithinAllowedRoots(absPath) {
			return "", &PathValidationError{Path: path, Message: ErrPathOutsideRoot.Error()}
		}
	}

	// Check if path exists (if required)
	if v.config.RequireExists {
		info, err := os.Lstat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				return "", &PathValidationError{Path: path, Message: ErrPathNotExists.Error()}
			}
			return "", &PathValidationError{Path: path, Message: "failed to stat path: " + err.Error()}
		}

		// Check for symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			if !v.config.AllowSymlinks {
				return "", &PathValidationError{Path: path, Message: ErrSymlinkNotAllowed.Error()}
			}

			// If symlinks are allowed, resolve and validate the target
			realPath, err := filepath.EvalSymlinks(absPath)
			if err != nil {
				return "", &PathValidationError{Path: path, Message: "failed to resolve symlink: " + err.Error()}
			}

			// Ensure symlink target is within allowed roots
			if len(v.config.AllowedRoots) > 0 && !v.isWithinAllowedRoots(realPath) {
				return "", &PathValidationError{Path: path, Message: ErrSymlinkOutsideRoot.Error()}
			}

			// Update absPath to the resolved path
			absPath = realPath
			info, err = os.Stat(absPath)
			if err != nil {
				return "", &PathValidationError{Path: path, Message: "failed to stat symlink target: " + err.Error()}
			}
		}

		// Check if path is a file (not a directory)
		if v.config.RequireFile && info.IsDir() {
			return "", &PathValidationError{Path: path, Message: ErrPathIsDirectory.Error()}
		}
	}

	return absPath, nil
}

// containsTraversal checks if a path contains directory traversal sequences.
func (v *PathValidator) containsTraversal(path string) bool {
	// Normalize path separators for consistent checking
	normalized := filepath.ToSlash(path)

	// Check for common traversal patterns
	traversalPatterns := []string{
		"../",
		"/..",
		"..\\",
		"\\..",
	}

	for _, pattern := range traversalPatterns {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}

	// Check if path starts with .. (relative traversal from current directory)
	if strings.HasPrefix(normalized, "..") {
		return true
	}

	return false
}

// isWithinAllowedRoots checks if the given path is within any of the allowed root directories.
func (v *PathValidator) isWithinAllowedRoots(path string) bool {
	// Resolve symlinks in the path for accurate comparison (e.g., /var -> /private/var on macOS)
	resolvedPath := resolvePathSymlinks(path)

	for _, root := range v.config.AllowedRoots {
		cleanRoot := filepath.Clean(root)
		absRoot, err := filepath.Abs(cleanRoot)
		if err != nil {
			continue
		}

		// Resolve symlinks in the root as well using the same resolution logic
		// This handles cases where the root path doesn't exist but contains symlinked components
		absRoot = resolvePathSymlinks(absRoot)

		// Ensure root ends with separator for proper prefix checking
		rootWithSep := absRoot
		if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
			rootWithSep += string(filepath.Separator)
		}

		// Check if path is exactly the root or starts with root/
		if resolvedPath == absRoot || strings.HasPrefix(resolvedPath, rootWithSep) {
			return true
		}
	}
	return false
}

// resolvePathSymlinks resolves symlinks in a path, handling cases where the path
// or its parent directories may not exist yet (e.g., for new files in nested dirs).
func resolvePathSymlinks(path string) string {
	// First try direct resolution (works if path exists)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}

	// Get absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return path
	}

	// Walk up the directory tree to find the first existing directory,
	// then rebuild the path with resolved symlinks
	parts := strings.Split(absPath, string(filepath.Separator))
	for i := len(parts) - 1; i >= 0; i-- {
		partialPath := strings.Join(parts[:i+1], string(filepath.Separator))
		if partialPath == "" {
			partialPath = string(filepath.Separator)
		}

		if resolvedPartial, err := filepath.EvalSymlinks(partialPath); err == nil {
			// Found an existing directory - append the remaining parts
			remaining := parts[i+1:]
			if len(remaining) > 0 {
				return filepath.Join(resolvedPartial, filepath.Join(remaining...))
			}
			return resolvedPartial
		}
	}

	// Couldn't resolve any part; return original absolute path
	return absPath
}

// ValidatePath is a convenience function that validates a path using default configuration.
func ValidatePath(path string) (string, error) {
	validator := NewPathValidator(DefaultPathValidatorConfig())
	return validator.Validate(path)
}

// ValidatePathWithRoots validates a path ensuring it's within the specified root directories.
func ValidatePathWithRoots(path string, allowedRoots []string) (string, error) {
	config := DefaultPathValidatorConfig()
	config.AllowedRoots = allowedRoots
	validator := NewPathValidator(config)
	return validator.Validate(path)
}

// ValidatePathForRead validates a path for reading, applying appropriate security checks.
// This is the primary entry point for read_file path validation.
func ValidatePathForRead(path string, allowedRoots []string) (string, error) {
	config := PathValidatorConfig{
		AllowedRoots:    allowedRoots,
		AllowSymlinks:   true,  // Allow symlinks for reading, but validate target
		RequireAbsolute: false, // Allow relative paths, they will be resolved
		RequireExists:   true,
		RequireFile:     true,
	}
	validator := NewPathValidator(config)
	return validator.Validate(path)
}

// SanitizePath cleans a path without requiring it to exist.
// Returns the cleaned, absolute path or an error if the path is invalid.
func SanitizePath(path string) (string, error) {
	config := PathValidatorConfig{
		AllowedRoots:    nil,
		AllowSymlinks:   true,
		RequireAbsolute: false,
		RequireExists:   false,
		RequireFile:     false,
	}
	validator := NewPathValidator(config)
	return validator.Validate(path)
}

// IsPathSafe performs basic security checks on a path without filesystem operations.
// It returns true if the path passes basic security checks.
func IsPathSafe(path string) bool {
	if path == "" {
		return false
	}
	if strings.ContainsRune(path, '\x00') {
		return false
	}
	validator := &PathValidator{}
	if validator.containsTraversal(path) {
		return false
	}
	return true
}

// ValidatePathForWrite validates a path for writing, applying appropriate security checks.
// This is the primary entry point for write_file path validation.
// Unlike read validation, write validation doesn't require the file to exist.
func ValidatePathForWrite(path string, allowedRoots []string) (string, error) {
	config := PathValidatorConfig{
		AllowedRoots:    allowedRoots,
		AllowSymlinks:   false, // Disallow symlinks for writing (security)
		RequireAbsolute: false, // Allow relative paths, they will be resolved
		RequireExists:   false, // File doesn't need to exist for writing
		RequireFile:     false, // Can't require file since it may not exist
	}
	validator := NewPathValidator(config)
	return validator.Validate(path)
}

// ValidateParentDirectoryForWrite validates that the parent directory of a path exists and is writable.
// This should be called after ValidatePathForWrite when create_directories is false.
func ValidateParentDirectoryForWrite(path string) error {
	// Get the parent directory
	dir := filepath.Dir(path)

	// Check if parent directory exists
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return &PathValidationError{Path: dir, Message: "parent directory does not exist"}
		}
		return &PathValidationError{Path: dir, Message: "failed to stat parent directory: " + err.Error()}
	}

	// Check that parent is actually a directory
	if !info.IsDir() {
		return &PathValidationError{Path: dir, Message: "parent path is not a directory"}
	}

	return nil
}
