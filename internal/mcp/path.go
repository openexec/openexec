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
func DefaultPathValidatorConfig() PathValidatorConfig {
	return PathValidatorConfig{
		AllowedRoots:    nil, // No restriction by default
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
	for _, root := range v.config.AllowedRoots {
		cleanRoot := filepath.Clean(root)
		absRoot, err := filepath.Abs(cleanRoot)
		if err != nil {
			continue
		}

		// Ensure root ends with separator for proper prefix checking
		rootWithSep := absRoot
		if !strings.HasSuffix(rootWithSep, string(filepath.Separator)) {
			rootWithSep += string(filepath.Separator)
		}

		// Check if path is exactly the root or starts with root/
		if path == absRoot || strings.HasPrefix(path, rootWithSep) {
			return true
		}
	}
	return false
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
