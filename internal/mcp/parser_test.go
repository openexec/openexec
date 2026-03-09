// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file contains tests for the BuildOutputParser.
package mcp

import (
	"strings"
	"testing"
)

func TestNewBuildOutputParser(t *testing.T) {
	parser := NewBuildOutputParser()
	if parser == nil {
		t.Fatal("NewBuildOutputParser() returned nil")
	}
	if parser.patterns == nil {
		t.Fatal("parser patterns not initialized")
	}
}

func TestBuildOutputParser_ParseBuildOutput_Empty(t *testing.T) {
	parser := NewBuildOutputParser()
	result := parser.Parse("", "build")

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors for empty input, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("expected 0 warnings for empty input, got %d", len(result.Warnings))
	}
}

func TestBuildOutputParser_ParseBuildOutput_SyntaxError(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `# example.com/pkg
./main.go:10:5: expected declaration, found 'foo'`

	result := parser.Parse(output, "build")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	err := result.Errors[0]
	if err.File != "./main.go" {
		t.Errorf("expected file './main.go', got %q", err.File)
	}
	if err.Line != 10 {
		t.Errorf("expected line 10, got %d", err.Line)
	}
	if err.Column != 5 {
		t.Errorf("expected column 5, got %d", err.Column)
	}
	if err.Category != ErrorCategorySyntax {
		t.Errorf("expected category Syntax, got %v", err.Category)
	}
	if !strings.Contains(err.Message, "expected declaration") {
		t.Errorf("expected message to contain 'expected declaration', got %q", err.Message)
	}
}

func TestBuildOutputParser_ParseBuildOutput_TypeError(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./util.go:25:12: cannot use x (type int) as type string`

	result := parser.Parse(output, "build")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	err := result.Errors[0]
	if err.Category != ErrorCategoryType {
		t.Errorf("expected category Type, got %v", err.Category)
	}
	if err.Suggestion == "" {
		t.Error("expected suggestion for type error")
	}
}

func TestBuildOutputParser_ParseBuildOutput_UndefinedError(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./handler.go:15:8: undefined: myFunction`

	result := parser.Parse(output, "build")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	err := result.Errors[0]
	if err.Category != ErrorCategoryUndefined {
		t.Errorf("expected category Undefined, got %v", err.Category)
	}
	if err.Suggestion == "" {
		t.Error("expected suggestion for undefined error")
	}
}

func TestBuildOutputParser_ParseBuildOutput_ImportError(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:5:2: package foo/bar not found in any of:
	/usr/local/go/src/foo/bar (from $GOROOT)
	/home/user/go/src/foo/bar (from $GOPATH)`

	result := parser.Parse(output, "build")

	if len(result.Errors) < 1 {
		t.Fatalf("expected at least 1 error, got %d", len(result.Errors))
	}

	err := result.Errors[0]
	if err.Category != ErrorCategoryImport {
		t.Errorf("expected category Import, got %v", err.Category)
	}
}

func TestBuildOutputParser_ParseBuildOutput_MultipleErrors(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `# example.com/pkg
./main.go:10:5: undefined: foo
./main.go:15:10: cannot use x (type int) as type string
./util.go:5:1: expected declaration, found 'bar'`

	result := parser.Parse(output, "build")

	if len(result.Errors) != 3 {
		t.Fatalf("expected 3 errors, got %d", len(result.Errors))
	}

	// Check categories
	categories := make(map[ErrorCategory]int)
	for _, err := range result.Errors {
		categories[err.Category]++
	}

	if categories[ErrorCategoryUndefined] != 1 {
		t.Errorf("expected 1 undefined error, got %d", categories[ErrorCategoryUndefined])
	}
	if categories[ErrorCategoryType] != 1 {
		t.Errorf("expected 1 type error, got %d", categories[ErrorCategoryType])
	}
	if categories[ErrorCategorySyntax] != 1 {
		t.Errorf("expected 1 syntax error, got %d", categories[ErrorCategorySyntax])
	}
}

func TestBuildOutputParser_ParseBuildOutput_ErrorWithoutColumn(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:10: package main`

	result := parser.Parse(output, "build")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	err := result.Errors[0]
	if err.Column != 0 {
		t.Errorf("expected column 0 for error without column, got %d", err.Column)
	}
}

func TestBuildOutputParser_ParseTestOutput_TestFailure(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `=== RUN   TestFoo
    main_test.go:15: Expected 1, got 2
--- FAIL: TestFoo (0.00s)
FAIL
exit status 1`

	result := parser.Parse(output, "test")

	if len(result.TestFailures) != 1 {
		t.Fatalf("expected 1 test failure, got %d", len(result.TestFailures))
	}

	tf := result.TestFailures[0]
	if tf.TestName != "TestFoo" {
		t.Errorf("expected test name 'TestFoo', got %q", tf.TestName)
	}
	if tf.Duration != "0.00s" {
		t.Errorf("expected duration '0.00s', got %q", tf.Duration)
	}
}

func TestBuildOutputParser_ParseTestOutput_MultipleFailures(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `=== RUN   TestFoo
    main_test.go:15: assertion failed
--- FAIL: TestFoo (0.01s)
=== RUN   TestBar
    util_test.go:25: expected 5, got 10
--- FAIL: TestBar (0.02s)
FAIL`

	result := parser.Parse(output, "test")

	if len(result.TestFailures) != 2 {
		t.Fatalf("expected 2 test failures, got %d", len(result.TestFailures))
	}
}

func TestBuildOutputParser_ParseTestOutput_CompileError(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `# example.com/pkg
./main.go:10:5: undefined: foo
FAIL	example.com/pkg [build failed]`

	result := parser.Parse(output, "test")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 compile error, got %d", len(result.Errors))
	}
}

func TestBuildOutputParser_ParseVetOutput_Warning(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `# example.com/pkg
./main.go:10:5: result of fmt.Sprintf call not used`

	result := parser.Parse(output, "vet")

	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}

	warn := result.Warnings[0]
	if warn.Category != ErrorCategoryVet {
		t.Errorf("expected category Vet, got %v", warn.Category)
	}
}

func TestBuildOutputParser_ParseVetOutput_MultipleWarnings(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:10:5: result of fmt.Sprintf call not used
./util.go:15:8: unreachable code`

	result := parser.Parse(output, "vet")

	if len(result.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d", len(result.Warnings))
	}
}

func TestBuildOutputParser_AutoDetect_Build(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `# example.com/pkg
./main.go:10:5: undefined: foo`

	result := parser.Parse(output, "")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error from auto-detect, got %d", len(result.Errors))
	}
}

func TestBuildOutputParser_AutoDetect_Test(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `=== RUN   TestFoo
--- FAIL: TestFoo (0.00s)
FAIL`

	result := parser.Parse(output, "")

	if len(result.TestFailures) != 1 {
		t.Fatalf("expected 1 test failure from auto-detect, got %d", len(result.TestFailures))
	}
}

func TestBuildOutputParser_Summary(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:10:5: undefined: foo
./util.go:15:8: cannot use x as type y`

	result := parser.Parse(output, "build")

	if result.Summary.TotalErrors != 2 {
		t.Errorf("expected TotalErrors 2, got %d", result.Summary.TotalErrors)
	}
	if len(result.Summary.AffectedFiles) != 2 {
		t.Errorf("expected 2 affected files, got %d", len(result.Summary.AffectedFiles))
	}
	if !result.Summary.HasCompileErrors {
		t.Error("expected HasCompileErrors to be true")
	}
}

func TestBuildOutputParser_Summary_ErrorsByCategory(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:10:5: undefined: foo
./main.go:15:8: undefined: bar
./util.go:20:3: cannot use x as type y`

	result := parser.Parse(output, "build")

	if result.Summary.ErrorsByCategory[ErrorCategoryUndefined] != 2 {
		t.Errorf("expected 2 undefined errors, got %d", result.Summary.ErrorsByCategory[ErrorCategoryUndefined])
	}
	if result.Summary.ErrorsByCategory[ErrorCategoryType] != 1 {
		t.Errorf("expected 1 type error, got %d", result.Summary.ErrorsByCategory[ErrorCategoryType])
	}
}

func TestCategorizeError_Syntax(t *testing.T) {
	parser := NewBuildOutputParser()

	tests := []struct {
		message  string
		expected ErrorCategory
	}{
		{"expected ';', found '}'", ErrorCategorySyntax},
		{"unexpected EOF", ErrorCategorySyntax},
		{"syntax error: unexpected foo", ErrorCategorySyntax},
		{"missing ')'", ErrorCategorySyntax},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			category := parser.categorizeError(tt.message)
			if category != tt.expected {
				t.Errorf("categorizeError(%q) = %v, want %v", tt.message, category, tt.expected)
			}
		})
	}
}

func TestCategorizeError_Type(t *testing.T) {
	parser := NewBuildOutputParser()

	tests := []struct {
		message  string
		expected ErrorCategory
	}{
		{"cannot use x (type int) as type string", ErrorCategoryType},
		{"cannot convert x to type y", ErrorCategoryType},
		{"type Foo has no field or method Bar", ErrorCategoryType},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			category := parser.categorizeError(tt.message)
			if category != tt.expected {
				t.Errorf("categorizeError(%q) = %v, want %v", tt.message, category, tt.expected)
			}
		})
	}
}

func TestCategorizeError_Undefined(t *testing.T) {
	parser := NewBuildOutputParser()

	tests := []struct {
		message  string
		expected ErrorCategory
	}{
		{"undefined: foo", ErrorCategoryUndefined},
		{"undeclared name: bar", ErrorCategoryUndefined},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			category := parser.categorizeError(tt.message)
			if category != tt.expected {
				t.Errorf("categorizeError(%q) = %v, want %v", tt.message, category, tt.expected)
			}
		})
	}
}

func TestCategorizeError_Import(t *testing.T) {
	parser := NewBuildOutputParser()

	tests := []struct {
		message  string
		expected ErrorCategory
	}{
		{"package foo/bar not found", ErrorCategoryImport},
		{"import cycle not allowed", ErrorCategoryImport},
		{"cannot find package", ErrorCategoryImport},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			category := parser.categorizeError(tt.message)
			if category != tt.expected {
				t.Errorf("categorizeError(%q) = %v, want %v", tt.message, category, tt.expected)
			}
		})
	}
}

func TestExtractSuggestion(t *testing.T) {
	parser := NewBuildOutputParser()

	tests := []struct {
		message          string
		expectSuggestion bool
	}{
		{"undefined: foo", true},
		{"did you mean: bar", true},
		{"package foo/bar not found", true},
		{"cannot use x as type y", true},
		{"expected ';', found '}'", true},
		{"random error message", false},
	}

	for _, tt := range tests {
		t.Run(tt.message, func(t *testing.T) {
			suggestion := parser.extractSuggestion(tt.message)
			hasSuggestion := suggestion != ""
			if hasSuggestion != tt.expectSuggestion {
				t.Errorf("extractSuggestion(%q) returned %q, expectSuggestion=%v", tt.message, suggestion, tt.expectSuggestion)
			}
		})
	}
}

func TestFormatParsedErrors(t *testing.T) {
	errors := []*ParsedError{
		{
			File:       "main.go",
			Line:       10,
			Column:     5,
			Message:    "undefined: foo",
			Category:   ErrorCategoryUndefined,
			Suggestion: "Check if 'foo' is imported or declared",
		},
	}

	output := FormatParsedErrors(errors)

	if !strings.Contains(output, "main.go:10:5") {
		t.Error("expected output to contain file:line:column")
	}
	if !strings.Contains(output, "undefined: foo") {
		t.Error("expected output to contain error message")
	}
	if !strings.Contains(output, "Suggestion:") {
		t.Error("expected output to contain suggestion")
	}
}

func TestFormatParsedErrors_Empty(t *testing.T) {
	output := FormatParsedErrors(nil)
	if output != "" {
		t.Errorf("expected empty string for nil errors, got %q", output)
	}
}

func TestFormatParsedTestFailures(t *testing.T) {
	failures := []*ParsedTestFailure{
		{
			Package:  "example.com/pkg",
			TestName: "TestFoo",
			Duration: "0.00s",
			Expected: "1",
			Actual:   "2",
		},
	}

	output := FormatParsedTestFailures(failures)

	if !strings.Contains(output, "TestFoo") {
		t.Error("expected output to contain test name")
	}
	if !strings.Contains(output, "example.com/pkg") {
		t.Error("expected output to contain package")
	}
	if !strings.Contains(output, "Expected:") {
		t.Error("expected output to contain expected value")
	}
	if !strings.Contains(output, "Actual:") {
		t.Error("expected output to contain actual value")
	}
}

func TestFormatParsedTestFailures_Empty(t *testing.T) {
	output := FormatParsedTestFailures(nil)
	if output != "" {
		t.Errorf("expected empty string for nil failures, got %q", output)
	}
}

func TestGetErrorsForFile(t *testing.T) {
	result := &BuildOutputParseResult{
		Errors: []*ParsedError{
			{File: "main.go", Line: 10},
			{File: "util.go", Line: 15},
			{File: "main.go", Line: 20},
		},
	}

	mainErrors := GetErrorsForFile(result, "main.go")
	if len(mainErrors) != 2 {
		t.Errorf("expected 2 errors for main.go, got %d", len(mainErrors))
	}

	utilErrors := GetErrorsForFile(result, "util.go")
	if len(utilErrors) != 1 {
		t.Errorf("expected 1 error for util.go, got %d", len(utilErrors))
	}
}

func TestGetErrorsByCategory(t *testing.T) {
	result := &BuildOutputParseResult{
		Errors: []*ParsedError{
			{Category: ErrorCategorySyntax},
			{Category: ErrorCategoryType},
			{Category: ErrorCategorySyntax},
		},
	}

	syntaxErrors := GetErrorsByCategory(result, ErrorCategorySyntax)
	if len(syntaxErrors) != 2 {
		t.Errorf("expected 2 syntax errors, got %d", len(syntaxErrors))
	}

	typeErrors := GetErrorsByCategory(result, ErrorCategoryType)
	if len(typeErrors) != 1 {
		t.Errorf("expected 1 type error, got %d", len(typeErrors))
	}
}

func TestGetFixableErrors(t *testing.T) {
	result := &BuildOutputParseResult{
		Errors: []*ParsedError{
			{Message: "error1", Suggestion: "fix1"},
			{Message: "error2", Suggestion: ""},
			{Message: "error3", Suggestion: "fix3"},
		},
	}

	fixable := GetFixableErrors(result)
	if len(fixable) != 2 {
		t.Errorf("expected 2 fixable errors, got %d", len(fixable))
	}
}

func TestParseBuildOutput_ConvenienceFunction(t *testing.T) {
	output := `./main.go:10:5: undefined: foo`
	result := ParseBuildOutput(output)

	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParseGoCompileOutput_ConvenienceFunction(t *testing.T) {
	output := `./main.go:10:5: undefined: foo`
	result := ParseGoCompileOutput(output)

	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func TestParseGoTestOutput_ConvenienceFunction(t *testing.T) {
	output := `=== RUN   TestFoo
--- FAIL: TestFoo (0.00s)
FAIL`
	result := ParseGoTestOutput(output)

	if len(result.TestFailures) != 1 {
		t.Errorf("expected 1 test failure, got %d", len(result.TestFailures))
	}
}

func TestParseGoVetOutput_ConvenienceFunction(t *testing.T) {
	output := `./main.go:10:5: result of call not used`
	result := ParseGoVetOutput(output)

	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
}

func TestBuildOutputParser_RawOutput(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:10:5: test error`
	result := parser.Parse(output, "build")

	if result.RawOutput != output {
		t.Errorf("expected RawOutput to match input, got %q", result.RawOutput)
	}
}

func TestBuildOutputParser_SkippableLines(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `ok  	example.com/pkg	0.001s
?   	example.com/other	[no test files]
PASS
exit status 0`

	result := parser.Parse(output, "test")

	if len(result.Errors) != 0 {
		t.Errorf("expected 0 errors for skippable lines, got %d", len(result.Errors))
	}
}

func TestBuildOutputParser_ComplexOutput(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `# example.com/pkg
./main.go:5:2: cannot find package "nonexistent" in any of:
	/usr/local/go/src/nonexistent (from $GOROOT)
	/home/user/go/src/nonexistent (from $GOPATH)
./main.go:10:5: undefined: someFunc
./util.go:15:8: expected ';', found newline
# example.com/other
./other.go:20:3: cannot use x (type int) as type string in argument to foo`

	result := parser.Parse(output, "build")

	if len(result.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d", len(result.Errors))
	}

	// Verify we have different categories
	categories := make(map[ErrorCategory]bool)
	for _, err := range result.Errors {
		categories[err.Category] = true
	}

	if len(categories) < 2 {
		t.Error("expected multiple error categories")
	}
}

func TestBuildOutputParser_TestWithExpectedGot(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `=== RUN   TestCompare
    compare_test.go:15:
        	Expected: "hello"
        	Got:      "world"
--- FAIL: TestCompare (0.00s)
FAIL`

	result := parser.Parse(output, "test")

	if len(result.TestFailures) != 1 {
		t.Fatalf("expected 1 test failure, got %d", len(result.TestFailures))
	}

	tf := result.TestFailures[0]
	if tf.TestName != "TestCompare" {
		t.Errorf("expected test name 'TestCompare', got %q", tf.TestName)
	}
}

func TestBuildOutputParser_LinkerError(t *testing.T) {
	parser := NewBuildOutputParser()
	output := `./main.go:10:5: undefined reference to 'some_c_function'`

	result := parser.Parse(output, "build")

	if len(result.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(result.Errors))
	}

	// Note: this gets categorized as undefined, but linker errors would match the linker pattern
	// in actual linker output which has a different format
}

func TestAtoi(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"10", 10},
		{"0", 0},
		{"", 0},
		{"abc", 0},
		{"-5", -5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := atoi(tt.input)
			if result != tt.expected {
				t.Errorf("atoi(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
