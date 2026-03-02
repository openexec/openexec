// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements the BuildOutputParser for parsing Go compiler, linker,
// and test output into structured errors and warnings for meta self-fix capability.
package mcp

import (
	"regexp"
	"strconv"
	"strings"
)

// ErrorCategory categorizes the type of build error for targeted fixes.
type ErrorCategory string

const (
	// ErrorCategorySyntax represents syntax errors (missing brackets, invalid tokens).
	ErrorCategorySyntax ErrorCategory = "syntax"
	// ErrorCategoryType represents type errors (type mismatch, invalid conversions).
	ErrorCategoryType ErrorCategory = "type"
	// ErrorCategoryUndefined represents undefined identifier errors.
	ErrorCategoryUndefined ErrorCategory = "undefined"
	// ErrorCategoryImport represents import-related errors.
	ErrorCategoryImport ErrorCategory = "import"
	// ErrorCategoryLinker represents linker errors (undefined references, duplicates).
	ErrorCategoryLinker ErrorCategory = "linker"
	// ErrorCategoryTest represents test failures.
	ErrorCategoryTest ErrorCategory = "test"
	// ErrorCategoryVet represents go vet analysis warnings.
	ErrorCategoryVet ErrorCategory = "vet"
	// ErrorCategoryOther represents uncategorized errors.
	ErrorCategoryOther ErrorCategory = "other"
)

// ParsedError represents a structured build error with enhanced metadata.
type ParsedError struct {
	// File is the path to the file with the error.
	File string `json:"file"`
	// Line is the line number (1-indexed).
	Line int `json:"line"`
	// Column is the column number (1-indexed), if available.
	Column int `json:"column,omitempty"`
	// EndLine is the end line number for multi-line errors.
	EndLine int `json:"end_line,omitempty"`
	// EndColumn is the end column number for multi-line errors.
	EndColumn int `json:"end_column,omitempty"`
	// Message is the error description.
	Message string `json:"message"`
	// Category classifies the error type.
	Category ErrorCategory `json:"category"`
	// Code is the error code if available.
	Code string `json:"code,omitempty"`
	// Context provides surrounding code or additional details.
	Context string `json:"context,omitempty"`
	// Suggestion is a recommended fix if available.
	Suggestion string `json:"suggestion,omitempty"`
	// RelatedErrors are errors that are related to this one.
	RelatedErrors []RelatedError `json:"related_errors,omitempty"`
}

// RelatedError represents an error related to the main error.
type RelatedError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

// ParsedTestFailure represents a parsed test failure.
type ParsedTestFailure struct {
	// Package is the Go package that failed.
	Package string `json:"package"`
	// TestName is the name of the failing test.
	TestName string `json:"test_name"`
	// File is the source file of the test.
	File string `json:"file,omitempty"`
	// Line is the line number where the failure occurred.
	Line int `json:"line,omitempty"`
	// Message contains the failure message.
	Message string `json:"message"`
	// Expected is the expected value if applicable.
	Expected string `json:"expected,omitempty"`
	// Actual is the actual value if applicable.
	Actual string `json:"actual,omitempty"`
	// Duration is the test duration.
	Duration string `json:"duration,omitempty"`
	// Output is the full test output.
	Output string `json:"output,omitempty"`
}

// BuildOutputParseResult contains the parsed build output.
type BuildOutputParseResult struct {
	// Errors are the parsed build errors.
	Errors []*ParsedError `json:"errors"`
	// Warnings are the parsed warnings.
	Warnings []*ParsedError `json:"warnings"`
	// TestFailures are the parsed test failures.
	TestFailures []*ParsedTestFailure `json:"test_failures,omitempty"`
	// Summary provides a high-level summary.
	Summary ParseSummary `json:"summary"`
	// RawOutput is the original output.
	RawOutput string `json:"raw_output"`
}

// ParseSummary provides summary statistics of the parse result.
type ParseSummary struct {
	// TotalErrors is the total number of errors.
	TotalErrors int `json:"total_errors"`
	// TotalWarnings is the total number of warnings.
	TotalWarnings int `json:"total_warnings"`
	// TotalTestFailures is the number of test failures.
	TotalTestFailures int `json:"total_test_failures"`
	// ErrorsByCategory counts errors by category.
	ErrorsByCategory map[ErrorCategory]int `json:"errors_by_category"`
	// AffectedFiles lists unique files with errors.
	AffectedFiles []string `json:"affected_files"`
	// AffectedPackages lists unique packages with errors.
	AffectedPackages []string `json:"affected_packages,omitempty"`
	// HasCompileErrors indicates if there are compile-time errors.
	HasCompileErrors bool `json:"has_compile_errors"`
	// HasTestFailures indicates if there are test failures.
	HasTestFailures bool `json:"has_test_failures"`
	// HasLinkerErrors indicates if there are linker errors.
	HasLinkerErrors bool `json:"has_linker_errors"`
}

// BuildOutputParser parses Go build, test, and vet output.
type BuildOutputParser struct {
	// patterns are precompiled regex patterns for efficiency.
	patterns *parserPatterns
}

// parserPatterns holds precompiled regex patterns.
type parserPatterns struct {
	// goError matches standard Go compiler errors: file.go:line:column: message
	goError *regexp.Regexp
	// goErrorNoColumn matches errors without column: file.go:line: message
	goErrorNoColumn *regexp.Regexp
	// packageLine matches package lines: # package/path
	packageLine *regexp.Regexp
	// testFail matches test failure lines: --- FAIL: TestName (duration)
	testFail *regexp.Regexp
	// testRun matches test start lines: === RUN TestName
	testRun *regexp.Regexp
	// testOutput matches test output lines with file/line: file_test.go:line: message
	testOutput *regexp.Regexp
	// linkerError matches linker errors
	linkerError *regexp.Regexp
	// importCycle matches import cycle errors
	importCycle *regexp.Regexp
	// undefinedError matches undefined identifier errors
	undefinedError *regexp.Regexp
	// typeError matches type mismatch errors
	typeError *regexp.Regexp
	// syntaxError matches syntax errors
	syntaxError *regexp.Regexp
	// vetWarning matches go vet warnings
	vetWarning *regexp.Regexp
	// testPanic matches test panic messages
	testPanic *regexp.Regexp
	// testExpect matches expected/got patterns
	testExpect *regexp.Regexp
}

// NewBuildOutputParser creates a new BuildOutputParser.
func NewBuildOutputParser() *BuildOutputParser {
	return &BuildOutputParser{
		patterns: compileParserPatterns(),
	}
}

// compileParserPatterns compiles all regex patterns.
func compileParserPatterns() *parserPatterns {
	return &parserPatterns{
		// Standard Go error format: file.go:line:column: message
		goError: regexp.MustCompile(`^(.+\.go):(\d+):(\d+):\s*(.+)$`),
		// Go error without column: file.go:line: message
		goErrorNoColumn: regexp.MustCompile(`^(.+\.go):(\d+):\s*(.+)$`),
		// Package line: # package/path
		packageLine: regexp.MustCompile(`^#\s+(.+)$`),
		// Test failure: --- FAIL: TestName (0.00s)
		testFail: regexp.MustCompile(`^---\s*FAIL:\s*(\S+)\s*\(([^)]+)\)`),
		// Test run start: === RUN TestName
		testRun: regexp.MustCompile(`^===\s*RUN\s+(\S+)`),
		// Test output with file/line
		testOutput: regexp.MustCompile(`^\s*(.+_test\.go):(\d+):\s*(.+)$`),
		// Linker error patterns
		linkerError: regexp.MustCompile(`^(.+): undefined reference to '(.+)'$`),
		// Import cycle: import cycle not allowed
		importCycle: regexp.MustCompile(`import cycle not allowed`),
		// Undefined identifier: undefined: identifier
		undefinedError: regexp.MustCompile(`undefined:\s*(\S+)`),
		// Type mismatch: cannot use X as Y
		typeError: regexp.MustCompile(`cannot use|cannot convert|type .+ has no field or method`),
		// Syntax errors: expected X, found Y
		syntaxError: regexp.MustCompile(`expected|syntax error|unexpected`),
		// Vet warnings
		vetWarning: regexp.MustCompile(`^(.+\.go):(\d+):(\d+)?:?\s*(.+)$`),
		// Test panic
		testPanic: regexp.MustCompile(`panic:|runtime error:`),
		// Expected/got pattern in tests
		testExpect: regexp.MustCompile(`(?i)expected[:\s]+(.+?)(?:,\s*)?(?:got|actual|but got)[:\s]+(.+)`),
	}
}

// Parse parses build output into structured results.
func (p *BuildOutputParser) Parse(output string, commandType string) *BuildOutputParseResult {
	result := &BuildOutputParseResult{
		Errors:       make([]*ParsedError, 0),
		Warnings:     make([]*ParsedError, 0),
		TestFailures: make([]*ParsedTestFailure, 0),
		Summary: ParseSummary{
			ErrorsByCategory: make(map[ErrorCategory]int),
			AffectedFiles:    make([]string, 0),
			AffectedPackages: make([]string, 0),
		},
		RawOutput: output,
	}

	switch commandType {
	case "build":
		p.parseBuildOutput(output, result)
	case "test":
		p.parseTestOutput(output, result)
	case "vet":
		p.parseVetOutput(output, result)
	default:
		// Try to auto-detect based on content
		p.parseAutoDetect(output, result)
	}

	// Compute summary
	p.computeSummary(result)

	return result
}

// parseBuildOutput parses go build output.
func (p *BuildOutputParser) parseBuildOutput(output string, result *BuildOutputParseResult) {
	lines := strings.Split(output, "\n")
	var currentPackage string

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Track current package
		if matches := p.patterns.packageLine.FindStringSubmatch(line); matches != nil {
			currentPackage = matches[1]
			continue
		}

		// Skip non-error lines
		if p.isSkippableLine(line) {
			continue
		}

		// Try to parse as Go error with column
		if matches := p.patterns.goError.FindStringSubmatch(line); matches != nil {
			parsedErr := p.parseGoError(matches, currentPackage)
			result.Errors = append(result.Errors, parsedErr)
			continue
		}

		// Try to parse as Go error without column
		if matches := p.patterns.goErrorNoColumn.FindStringSubmatch(line); matches != nil {
			parsedErr := p.parseGoErrorNoColumn(matches, currentPackage)
			result.Errors = append(result.Errors, parsedErr)
			continue
		}
	}
}

// parseTestOutput parses go test output.
func (p *BuildOutputParser) parseTestOutput(output string, result *BuildOutputParseResult) {
	lines := strings.Split(output, "\n")
	var currentTest string
	var currentPackage string
	var testOutput strings.Builder

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmedLine := strings.TrimSpace(line)

		// Track current package
		if matches := p.patterns.packageLine.FindStringSubmatch(trimmedLine); matches != nil {
			currentPackage = matches[1]
			continue
		}

		// Track test runs
		if matches := p.patterns.testRun.FindStringSubmatch(trimmedLine); matches != nil {
			currentTest = matches[1]
			testOutput.Reset()
			continue
		}

		// Parse test failures
		if matches := p.patterns.testFail.FindStringSubmatch(trimmedLine); matches != nil {
			failure := &ParsedTestFailure{
				Package:  currentPackage,
				TestName: matches[1],
				Duration: matches[2],
				Output:   testOutput.String(),
			}

			// Try to extract expected/got values from output
			if expMatches := p.patterns.testExpect.FindStringSubmatch(testOutput.String()); expMatches != nil {
				failure.Expected = strings.TrimSpace(expMatches[1])
				failure.Actual = strings.TrimSpace(expMatches[2])
			}

			result.TestFailures = append(result.TestFailures, failure)
			currentTest = ""
			continue
		}

		// Collect test output
		if currentTest != "" {
			testOutput.WriteString(line)
			testOutput.WriteString("\n")
		}

		// Parse compile errors in test output
		if p.patterns.goError.MatchString(trimmedLine) || p.patterns.goErrorNoColumn.MatchString(trimmedLine) {
			if matches := p.patterns.goError.FindStringSubmatch(trimmedLine); matches != nil {
				result.Errors = append(result.Errors, p.parseGoError(matches, currentPackage))
			} else if matches := p.patterns.goErrorNoColumn.FindStringSubmatch(trimmedLine); matches != nil {
				result.Errors = append(result.Errors, p.parseGoErrorNoColumn(matches, currentPackage))
			}
		}

		// Parse test-specific error lines
		if matches := p.patterns.testOutput.FindStringSubmatch(trimmedLine); matches != nil {
			parsedErr := &ParsedError{
				File:     matches[1],
				Line:     atoi(matches[2]),
				Message:  matches[3],
				Category: ErrorCategoryTest,
			}
			result.Errors = append(result.Errors, parsedErr)
		}
	}
}

// parseVetOutput parses go vet output.
func (p *BuildOutputParser) parseVetOutput(output string, result *BuildOutputParseResult) {
	lines := strings.Split(output, "\n")
	var currentPackage string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Track current package
		if matches := p.patterns.packageLine.FindStringSubmatch(line); matches != nil {
			currentPackage = matches[1]
			continue
		}

		// Skip non-warning lines
		if p.isSkippableLine(line) {
			continue
		}

		// Parse vet warnings (same format as errors)
		if matches := p.patterns.vetWarning.FindStringSubmatch(line); matches != nil {
			warning := &ParsedError{
				File:     matches[1],
				Line:     atoi(matches[2]),
				Category: ErrorCategoryVet,
			}

			if matches[3] != "" {
				warning.Column = atoi(matches[3])
				if len(matches) > 4 {
					warning.Message = strings.TrimSpace(matches[4])
				}
			} else if len(matches) > 4 {
				warning.Message = strings.TrimSpace(matches[4])
			}

			// Extract additional context
			if currentPackage != "" {
				warning.Context = "package: " + currentPackage
			}

			result.Warnings = append(result.Warnings, warning)
		}
	}
}

// parseAutoDetect attempts to auto-detect and parse output.
func (p *BuildOutputParser) parseAutoDetect(output string, result *BuildOutputParseResult) {
	// Check for test indicators
	if strings.Contains(output, "=== RUN") || strings.Contains(output, "--- FAIL") ||
		strings.Contains(output, "PASS") || strings.Contains(output, "testing: warning") {
		p.parseTestOutput(output, result)
		return
	}

	// Check for vet indicators (usually less errors, more warnings format)
	if strings.Contains(output, "go vet") {
		p.parseVetOutput(output, result)
		return
	}

	// Default to build output
	p.parseBuildOutput(output, result)
}

// parseGoError parses a Go error with column number.
func (p *BuildOutputParser) parseGoError(matches []string, currentPackage string) *ParsedError {
	file := matches[1]
	line := atoi(matches[2])
	column := atoi(matches[3])
	message := strings.TrimSpace(matches[4])

	err := &ParsedError{
		File:     file,
		Line:     line,
		Column:   column,
		Message:  message,
		Category: p.categorizeError(message),
	}

	// Extract suggestions for common errors
	err.Suggestion = p.extractSuggestion(message)

	// Add package context
	if currentPackage != "" {
		err.Context = "package: " + currentPackage
	}

	return err
}

// parseGoErrorNoColumn parses a Go error without column number.
func (p *BuildOutputParser) parseGoErrorNoColumn(matches []string, currentPackage string) *ParsedError {
	file := matches[1]
	line := atoi(matches[2])
	message := strings.TrimSpace(matches[3])

	err := &ParsedError{
		File:     file,
		Line:     line,
		Message:  message,
		Category: p.categorizeError(message),
	}

	// Extract suggestions for common errors
	err.Suggestion = p.extractSuggestion(message)

	// Add package context
	if currentPackage != "" {
		err.Context = "package: " + currentPackage
	}

	return err
}

// categorizeError determines the category of an error message.
func (p *BuildOutputParser) categorizeError(message string) ErrorCategory {
	msg := strings.ToLower(message)

	// Import-related errors
	if strings.Contains(msg, "import") || strings.Contains(msg, "package") {
		if strings.Contains(msg, "cycle") {
			return ErrorCategoryImport
		}
		if strings.Contains(msg, "not found") || strings.Contains(msg, "cannot find") {
			return ErrorCategoryImport
		}
	}

	// Undefined identifier
	if strings.Contains(msg, "undefined:") || strings.Contains(msg, "undeclared") {
		return ErrorCategoryUndefined
	}

	// Type errors
	if strings.Contains(msg, "cannot use") || strings.Contains(msg, "cannot convert") ||
		strings.Contains(msg, "type mismatch") || strings.Contains(msg, "has no field or method") ||
		strings.Contains(msg, "incompatible type") || strings.Contains(msg, "wrong type") {
		return ErrorCategoryType
	}

	// Syntax errors
	if strings.Contains(msg, "expected") || strings.Contains(msg, "unexpected") ||
		strings.Contains(msg, "syntax error") || strings.Contains(msg, "missing") ||
		strings.Contains(msg, "invalid character") {
		return ErrorCategorySyntax
	}

	// Linker errors
	if strings.Contains(msg, "undefined reference") || strings.Contains(msg, "multiple definition") ||
		strings.Contains(msg, "linker") {
		return ErrorCategoryLinker
	}

	return ErrorCategoryOther
}

// extractSuggestion extracts fix suggestions from error messages.
func (p *BuildOutputParser) extractSuggestion(message string) string {
	msg := strings.ToLower(message)

	// "did you mean" suggestions
	if idx := strings.Index(msg, "did you mean"); idx != -1 {
		return message[idx:]
	}

	// Undefined identifier suggestions
	if strings.Contains(msg, "undefined:") {
		if idx := strings.Index(message, "undefined:"); idx != -1 {
			identifier := strings.TrimSpace(message[idx+len("undefined:"):])
			// Extract just the identifier name
			if spaceIdx := strings.Index(identifier, " "); spaceIdx != -1 {
				identifier = identifier[:spaceIdx]
			}
			return "Check if '" + identifier + "' is imported or declared"
		}
	}

	// Import suggestions
	if strings.Contains(msg, "package") && strings.Contains(msg, "not found") {
		return "Run 'go mod tidy' or check import path"
	}

	// Type mismatch suggestions
	if strings.Contains(msg, "cannot use") && strings.Contains(msg, "as type") {
		return "Check type compatibility or use type conversion"
	}

	// Missing bracket suggestions
	if strings.Contains(msg, "expected '") {
		start := strings.Index(message, "expected '")
		if start != -1 {
			end := strings.Index(message[start+10:], "'")
			if end != -1 {
				expected := message[start+10 : start+10+end]
				return "Add missing '" + expected + "'"
			}
		}
	}

	return ""
}

// isSkippableLine checks if a line should be skipped during parsing.
func (p *BuildOutputParser) isSkippableLine(line string) bool {
	// Skip empty lines
	if line == "" {
		return true
	}

	// Skip status lines
	prefixes := []string{"ok ", "? ", "PASS", "FAIL ", "exit status"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}

	// Skip common noise
	if strings.HasPrefix(line, "# ") && !strings.Contains(line, ".go") {
		return false // Package lines are handled separately
	}

	return false
}

// computeSummary computes the summary statistics.
func (p *BuildOutputParser) computeSummary(result *BuildOutputParseResult) {
	result.Summary.TotalErrors = len(result.Errors)
	result.Summary.TotalWarnings = len(result.Warnings)
	result.Summary.TotalTestFailures = len(result.TestFailures)

	// Count errors by category
	for _, err := range result.Errors {
		result.Summary.ErrorsByCategory[err.Category]++
	}

	// Collect unique affected files
	fileSet := make(map[string]bool)
	for _, err := range result.Errors {
		if err.File != "" {
			fileSet[err.File] = true
		}
	}
	for _, warn := range result.Warnings {
		if warn.File != "" {
			fileSet[warn.File] = true
		}
	}
	for file := range fileSet {
		result.Summary.AffectedFiles = append(result.Summary.AffectedFiles, file)
	}

	// Collect unique affected packages
	pkgSet := make(map[string]bool)
	for _, tf := range result.TestFailures {
		if tf.Package != "" {
			pkgSet[tf.Package] = true
		}
	}
	for pkg := range pkgSet {
		result.Summary.AffectedPackages = append(result.Summary.AffectedPackages, pkg)
	}

	// Set flags
	result.Summary.HasCompileErrors = result.Summary.ErrorsByCategory[ErrorCategorySyntax] > 0 ||
		result.Summary.ErrorsByCategory[ErrorCategoryType] > 0 ||
		result.Summary.ErrorsByCategory[ErrorCategoryUndefined] > 0 ||
		result.Summary.ErrorsByCategory[ErrorCategoryImport] > 0

	result.Summary.HasTestFailures = len(result.TestFailures) > 0
	result.Summary.HasLinkerErrors = result.Summary.ErrorsByCategory[ErrorCategoryLinker] > 0
}

// atoi converts string to int, returning 0 on error.
func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// ParseBuildOutput is a convenience function to parse build output with auto-detection.
func ParseBuildOutput(output string) *BuildOutputParseResult {
	parser := NewBuildOutputParser()
	return parser.Parse(output, "")
}

// ParseGoCompileOutput is a convenience function specifically for go build output.
func ParseGoCompileOutput(output string) *BuildOutputParseResult {
	parser := NewBuildOutputParser()
	return parser.Parse(output, "build")
}

// ParseGoTestOutput is a convenience function specifically for go test output.
func ParseGoTestOutput(output string) *BuildOutputParseResult {
	parser := NewBuildOutputParser()
	return parser.Parse(output, "test")
}

// ParseGoVetOutput is a convenience function specifically for go vet output.
func ParseGoVetOutput(output string) *BuildOutputParseResult {
	parser := NewBuildOutputParser()
	return parser.Parse(output, "vet")
}

// FormatParsedErrors formats parsed errors for human-readable output.
func FormatParsedErrors(errors []*ParsedError) string {
	if len(errors) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Build Errors:\n")

	// Group by category
	byCategory := make(map[ErrorCategory][]*ParsedError)
	for _, err := range errors {
		byCategory[err.Category] = append(byCategory[err.Category], err)
	}

	// Output by category
	categoryOrder := []ErrorCategory{
		ErrorCategorySyntax,
		ErrorCategoryType,
		ErrorCategoryUndefined,
		ErrorCategoryImport,
		ErrorCategoryLinker,
		ErrorCategoryTest,
		ErrorCategoryOther,
	}

	for _, cat := range categoryOrder {
		errs, ok := byCategory[cat]
		if !ok || len(errs) == 0 {
			continue
		}

		sb.WriteString("\n")
		sb.WriteString(string(cat))
		sb.WriteString(" errors:\n")

		for i, err := range errs {
			sb.WriteString("  ")
			sb.WriteString(strconv.Itoa(i + 1))
			sb.WriteString(". ")
			if err.Column > 0 {
				sb.WriteString(err.File)
				sb.WriteString(":")
				sb.WriteString(strconv.Itoa(err.Line))
				sb.WriteString(":")
				sb.WriteString(strconv.Itoa(err.Column))
			} else {
				sb.WriteString(err.File)
				sb.WriteString(":")
				sb.WriteString(strconv.Itoa(err.Line))
			}
			sb.WriteString(": ")
			sb.WriteString(err.Message)
			sb.WriteString("\n")

			if err.Suggestion != "" {
				sb.WriteString("     Suggestion: ")
				sb.WriteString(err.Suggestion)
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// FormatParsedTestFailures formats test failures for human-readable output.
func FormatParsedTestFailures(failures []*ParsedTestFailure) string {
	if len(failures) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Test Failures:\n")

	for i, tf := range failures {
		sb.WriteString("\n")
		sb.WriteString(strconv.Itoa(i + 1))
		sb.WriteString(". ")
		sb.WriteString(tf.TestName)
		if tf.Package != "" {
			sb.WriteString(" (")
			sb.WriteString(tf.Package)
			sb.WriteString(")")
		}
		if tf.Duration != "" {
			sb.WriteString(" [")
			sb.WriteString(tf.Duration)
			sb.WriteString("]")
		}
		sb.WriteString("\n")

		if tf.Expected != "" || tf.Actual != "" {
			sb.WriteString("   Expected: ")
			sb.WriteString(tf.Expected)
			sb.WriteString("\n")
			sb.WriteString("   Actual:   ")
			sb.WriteString(tf.Actual)
			sb.WriteString("\n")
		}

		if tf.Message != "" {
			sb.WriteString("   Message: ")
			sb.WriteString(tf.Message)
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// GetErrorsForFile returns all errors for a specific file.
func GetErrorsForFile(result *BuildOutputParseResult, filePath string) []*ParsedError {
	var errors []*ParsedError
	for _, err := range result.Errors {
		if err.File == filePath || strings.HasSuffix(err.File, "/"+filePath) {
			errors = append(errors, err)
		}
	}
	return errors
}

// GetErrorsByCategory returns all errors of a specific category.
func GetErrorsByCategory(result *BuildOutputParseResult, category ErrorCategory) []*ParsedError {
	var errors []*ParsedError
	for _, err := range result.Errors {
		if err.Category == category {
			errors = append(errors, err)
		}
	}
	return errors
}

// GetFixableErrors returns errors that have suggestions for fixes.
func GetFixableErrors(result *BuildOutputParseResult) []*ParsedError {
	var errors []*ParsedError
	for _, err := range result.Errors {
		if err.Suggestion != "" {
			errors = append(errors, err)
		}
	}
	return errors
}
