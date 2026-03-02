// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements a Python syntax validator using the Python interpreter.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PythonSyntaxError represents a single syntax error in Python code.
type PythonSyntaxError struct {
	Line    int    `json:"line"`              // Line number (1-indexed)
	Column  int    `json:"column,omitempty"`  // Column number (0-indexed), if available
	Message string `json:"message"`           // Error description
	Context string `json:"context,omitempty"` // Code context where error occurred
}

func (e *PythonSyntaxError) Error() string {
	if e.Column > 0 {
		return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("line %d: %s", e.Line, e.Message)
}

// PythonSyntaxStats provides statistics about the validated Python code.
type PythonSyntaxStats struct {
	LineCount   int  `json:"line_count"`   // Total number of lines
	HasShebang  bool `json:"has_shebang"`  // True if file starts with #!
	IsEmpty     bool `json:"is_empty"`     // True if file contains only whitespace/comments
	HasEncoding bool `json:"has_encoding"` // True if file has encoding declaration
}

// PythonSyntaxValidationResult contains the result of Python syntax validation.
type PythonSyntaxValidationResult struct {
	Valid    bool                 `json:"valid"`    // True if syntax is valid
	Errors   []PythonSyntaxError  `json:"errors"`   // Syntax errors found
	Warnings []PythonSyntaxError  `json:"warnings"` // Warnings (e.g., deprecated syntax)
	Stats    PythonSyntaxStats    `json:"stats"`    // Code statistics
}

// PythonValidatorConfig contains configuration for the Python validator.
type PythonValidatorConfig struct {
	// PythonPath is the path to the Python interpreter.
	// If empty, "python3" is used from PATH.
	PythonPath string

	// Timeout is the maximum time allowed for validation.
	// Default is 5 seconds if not set.
	Timeout time.Duration

	// SkipIfNoPython controls behavior when Python is not available.
	// If true, validation passes with a warning when Python is missing.
	// If false (default), validation fails when Python is missing.
	SkipIfNoPython bool
}

// DefaultPythonValidatorConfig returns a configuration with sensible defaults.
func DefaultPythonValidatorConfig() *PythonValidatorConfig {
	return &PythonValidatorConfig{
		PythonPath:     "python3",
		Timeout:        5 * time.Second,
		SkipIfNoPython: false,
	}
}

// PythonValidator validates Python syntax.
type PythonValidator struct {
	config *PythonValidatorConfig
}

// NewPythonValidator creates a new Python syntax validator with default configuration.
func NewPythonValidator() *PythonValidator {
	return &PythonValidator{
		config: DefaultPythonValidatorConfig(),
	}
}

// NewPythonValidatorWithConfig creates a new Python syntax validator with custom configuration.
func NewPythonValidatorWithConfig(config *PythonValidatorConfig) *PythonValidator {
	if config == nil {
		config = DefaultPythonValidatorConfig()
	}
	if config.PythonPath == "" {
		config.PythonPath = "python3"
	}
	if config.Timeout == 0 {
		config.Timeout = 5 * time.Second
	}
	return &PythonValidator{
		config: config,
	}
}

// ValidateCode validates Python source code provided as a string.
// The filename parameter is used for error reporting.
func (v *PythonValidator) ValidateCode(code string, filename string) *PythonSyntaxValidationResult {
	result := &PythonSyntaxValidationResult{
		Valid:    true,
		Errors:   make([]PythonSyntaxError, 0),
		Warnings: make([]PythonSyntaxError, 0),
		Stats:    computePythonStats(code),
	}

	// Empty files are valid Python
	if result.Stats.IsEmpty {
		return result
	}

	// Use default filename for error reporting if not provided
	if filename == "" {
		filename = "<string>"
	}

	// Validate using Python's compile() function
	err := v.runPythonSyntaxCheck(code, filename, result)
	if err != nil {
		// If Python is not available and SkipIfNoPython is true, return with warning
		if v.config.SkipIfNoPython && isPythonNotFound(err) {
			result.Warnings = append(result.Warnings, PythonSyntaxError{
				Line:    0,
				Message: "Python interpreter not found; syntax validation skipped",
			})
			return result
		}

		// Check if it's actually a syntax error (expected) vs an execution error
		if syntaxErrors := parsePythonSyntaxErrors(err.Error(), filename); len(syntaxErrors) > 0 {
			result.Valid = false
			result.Errors = syntaxErrors
		} else {
			// Unexpected error from Python
			result.Valid = false
			result.Errors = append(result.Errors, PythonSyntaxError{
				Line:    1,
				Message: fmt.Sprintf("validation failed: %v", err),
			})
		}
	}

	return result
}

// ValidateFile validates a Python file at the given path.
func (v *PythonValidator) ValidateFile(path string) (*PythonSyntaxValidationResult, error) {
	// Validate path has .py extension
	ext := filepath.Ext(path)
	if ext != ".py" && ext != ".pyw" {
		return nil, fmt.Errorf("not a Python file: %s", path)
	}

	// Use Python to read and compile the file
	result := &PythonSyntaxValidationResult{
		Valid:    true,
		Errors:   make([]PythonSyntaxError, 0),
		Warnings: make([]PythonSyntaxError, 0),
	}

	// Run py_compile on the file
	ctx, cancel := context.WithTimeout(context.Background(), v.config.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, v.config.PythonPath, "-m", "py_compile", path)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if v.config.SkipIfNoPython && isPythonNotFound(err) {
			result.Warnings = append(result.Warnings, PythonSyntaxError{
				Line:    0,
				Message: "Python interpreter not found; syntax validation skipped",
			})
			return result, nil
		}

		// Parse syntax errors from stderr
		if syntaxErrors := parsePythonSyntaxErrors(stderr.String(), path); len(syntaxErrors) > 0 {
			result.Valid = false
			result.Errors = syntaxErrors
		} else if stderr.Len() > 0 {
			result.Valid = false
			result.Errors = append(result.Errors, PythonSyntaxError{
				Line:    1,
				Message: strings.TrimSpace(stderr.String()),
			})
		} else {
			result.Valid = false
			result.Errors = append(result.Errors, PythonSyntaxError{
				Line:    1,
				Message: err.Error(),
			})
		}
	}

	return result, nil
}

// runPythonSyntaxCheck runs Python's compile() to check syntax.
func (v *PythonValidator) runPythonSyntaxCheck(code string, filename string, result *PythonSyntaxValidationResult) error {
	ctx, cancel := context.WithTimeout(context.Background(), v.config.Timeout)
	defer cancel()

	// Use Python to validate syntax via compile()
	// The script outputs JSON with error details if there's a syntax error
	pythonScript := `
import sys
import json
import ast

code = sys.stdin.read()
filename = sys.argv[1] if len(sys.argv) > 1 else '<string>'

try:
    compile(code, filename, 'exec')
except SyntaxError as e:
    error = {
        'type': 'SyntaxError',
        'msg': e.msg if e.msg else str(e),
        'lineno': e.lineno or 1,
        'offset': e.offset or 0,
        'text': e.text.rstrip() if e.text else ''
    }
    print(json.dumps(error))
    sys.exit(1)
except Exception as e:
    error = {
        'type': type(e).__name__,
        'msg': str(e),
        'lineno': 1,
        'offset': 0,
        'text': ''
    }
    print(json.dumps(error))
    sys.exit(1)

# Success - no output needed
sys.exit(0)
`

	cmd := exec.CommandContext(ctx, v.config.PythonPath, "-c", pythonScript, filename)
	cmd.Stdin = strings.NewReader(code)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Check for Python not found
		if isPythonNotFound(err) {
			return err
		}

		// Try to parse JSON error from stdout
		if stdout.Len() > 0 {
			var errorInfo struct {
				Type   string `json:"type"`
				Msg    string `json:"msg"`
				LineNo int    `json:"lineno"`
				Offset int    `json:"offset"`
				Text   string `json:"text"`
			}

			if jsonErr := json.Unmarshal(stdout.Bytes(), &errorInfo); jsonErr == nil {
				result.Valid = false
				result.Errors = append(result.Errors, PythonSyntaxError{
					Line:    errorInfo.LineNo,
					Column:  errorInfo.Offset,
					Message: errorInfo.Msg,
					Context: errorInfo.Text,
				})
				return nil
			}
		}

		// Fall back to stderr parsing
		if stderr.Len() > 0 {
			return fmt.Errorf("%s", strings.TrimSpace(stderr.String()))
		}
		return err
	}

	return nil
}

// computePythonStats computes statistics about Python code.
func computePythonStats(code string) PythonSyntaxStats {
	lines := strings.Split(code, "\n")
	stats := PythonSyntaxStats{
		LineCount: len(lines),
	}

	// Check for shebang
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		stats.HasShebang = true
	}

	// Check for encoding declaration (PEP 263)
	// Must be in first two lines
	encodingPattern := regexp.MustCompile(`^[ \t\f]*#.*?coding[:=][ \t]*([-\w.]+)`)
	for i := 0; i < 2 && i < len(lines); i++ {
		if encodingPattern.MatchString(lines[i]) {
			stats.HasEncoding = true
			break
		}
	}

	// Check if empty (only whitespace, comments, or empty lines)
	isEmpty := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			isEmpty = false
			break
		}
	}
	stats.IsEmpty = isEmpty

	return stats
}

// isPythonNotFound checks if an error indicates Python is not installed.
func isPythonNotFound(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "executable file not found") ||
		strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "no such file")
}

// parsePythonSyntaxErrors parses Python syntax error output into structured errors.
func parsePythonSyntaxErrors(output string, filename string) []PythonSyntaxError {
	var errors []PythonSyntaxError

	// Pattern for Python syntax errors:
	// File "filename.py", line N
	//   code
	//     ^
	// SyntaxError: message
	//
	// Or for simpler format:
	// SyntaxError: message (filename, line N)
	//
	// Or from py_compile:
	// File "filename", line N
	//   ...
	// SyntaxError: invalid syntax

	lines := strings.Split(output, "\n")

	// Pattern to match "File "...", line N"
	fileLinePattern := regexp.MustCompile(`File "(.+)", line (\d+)`)
	// Pattern to match SyntaxError messages
	syntaxErrPattern := regexp.MustCompile(`(SyntaxError|IndentationError|TabError):\s*(.+)`)

	var currentLine int
	var currentContext string

	for i, line := range lines {
		// Check for File/line indicator
		if matches := fileLinePattern.FindStringSubmatch(line); matches != nil {
			lineNum, _ := strconv.Atoi(matches[2])
			currentLine = lineNum

			// Next line might be the code context
			if i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "SyntaxError") &&
				!strings.HasPrefix(lines[i+1], "IndentationError") &&
				!strings.HasPrefix(lines[i+1], "TabError") {
				currentContext = strings.TrimRight(lines[i+1], " \t")
			}
			continue
		}

		// Check for syntax error message
		if matches := syntaxErrPattern.FindStringSubmatch(line); matches != nil {
			errType := matches[1]
			message := strings.TrimSpace(matches[2])

			// Combine error type with message if message doesn't start with type
			if message == "" || message == "invalid syntax" {
				message = errType + ": invalid syntax"
			} else if !strings.HasPrefix(message, errType) {
				message = errType + ": " + message
			}

			// If we have a line number from earlier, use it
			if currentLine > 0 {
				errors = append(errors, PythonSyntaxError{
					Line:    currentLine,
					Message: message,
					Context: currentContext,
				})
			} else {
				// Try to extract line number from message
				lineNumPattern := regexp.MustCompile(`line (\d+)`)
				if lineMatch := lineNumPattern.FindStringSubmatch(message); lineMatch != nil {
					lineNum, _ := strconv.Atoi(lineMatch[1])
					errors = append(errors, PythonSyntaxError{
						Line:    lineNum,
						Message: message,
					})
				} else {
					errors = append(errors, PythonSyntaxError{
						Line:    1,
						Message: message,
					})
				}
			}

			// Reset for next error
			currentLine = 0
			currentContext = ""
		}
	}

	return errors
}

// ValidatePythonSyntax is a convenience function that validates Python code with default settings.
func ValidatePythonSyntax(code string, filename string) *PythonSyntaxValidationResult {
	validator := NewPythonValidator()
	return validator.ValidateCode(code, filename)
}

// ValidatePythonSyntaxWithConfig validates Python code with custom configuration.
func ValidatePythonSyntaxWithConfig(code string, filename string, config *PythonValidatorConfig) *PythonSyntaxValidationResult {
	validator := NewPythonValidatorWithConfig(config)
	return validator.ValidateCode(code, filename)
}

// IsPythonFile checks if a path refers to a Python file based on extension.
func IsPythonFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".py" || ext == ".pyw"
}

// ShouldValidatePythonSyntax determines if Python syntax validation should be performed
// for the given file path. Returns false for common non-source Python files.
func ShouldValidatePythonSyntax(path string) bool {
	if !IsPythonFile(path) {
		return false
	}

	// Skip common non-source paths
	lowerPath := strings.ToLower(path)
	skipPatterns := []string{
		"__pycache__",
		".pyc",
		"site-packages",
		"dist-packages",
		".egg",
		"venv/",
		".venv/",
		"virtualenv/",
		".tox/",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(lowerPath, pattern) {
			return false
		}
	}

	return true
}
