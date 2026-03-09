package mcp

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// skipIfNoPython skips the test if Python 3 is not available
func skipIfNoPython(t *testing.T) {
	t.Helper()
	_, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("Python 3 not available, skipping test")
	}
}

func TestValidatePythonSyntax_ValidCode(t *testing.T) {
	skipIfNoPython(t)

	tests := []struct {
		name     string
		code     string
		filename string
	}{
		{
			name:     "simple print",
			code:     `print("hello world")`,
			filename: "test.py",
		},
		{
			name:     "function definition",
			code:     "def foo(x):\n    return x * 2\n",
			filename: "func.py",
		},
		{
			name:     "class definition",
			code:     "class Foo:\n    def __init__(self):\n        self.x = 1\n",
			filename: "class.py",
		},
		{
			name:     "empty file",
			code:     "",
			filename: "empty.py",
		},
		{
			name:     "only comments",
			code:     "# This is a comment\n# Another comment",
			filename: "comments.py",
		},
		{
			name:     "with shebang",
			code:     "#!/usr/bin/env python3\nprint('hello')\n",
			filename: "script.py",
		},
		{
			name:     "with encoding declaration",
			code:     "# -*- coding: utf-8 -*-\nprint('hello')\n",
			filename: "encoded.py",
		},
		{
			name: "multiline string",
			code: `x = """
This is a
multiline string
"""`,
			filename: "multiline.py",
		},
		{
			name:     "list comprehension",
			code:     `squares = [x**2 for x in range(10)]`,
			filename: "comprehension.py",
		},
		{
			name:     "import statement",
			code:     "import os\nimport sys\nfrom pathlib import Path\n",
			filename: "imports.py",
		},
		{
			name:     "async function",
			code:     "async def fetch():\n    await something()\n",
			filename: "async.py",
		},
		{
			name:     "type hints",
			code:     "def greet(name: str) -> str:\n    return f'Hello, {name}'\n",
			filename: "typed.py",
		},
		{
			name:     "decorators",
			code:     "@staticmethod\ndef foo():\n    pass\n",
			filename: "decorated.py",
		},
		{
			name:     "context manager",
			code:     "with open('file.txt') as f:\n    data = f.read()\n",
			filename: "context.py",
		},
		{
			name:     "exception handling",
			code:     "try:\n    risky()\nexcept Exception as e:\n    handle(e)\nfinally:\n    cleanup()\n",
			filename: "exceptions.py",
		},
		{
			name:     "walrus operator",
			code:     "if (n := len(a)) > 10:\n    print(f'too long: {n}')\n",
			filename: "walrus.py",
		},
		{
			name:     "f-strings",
			code:     "name = \"world\"\nprint(f\"Hello, {name}!\")",
			filename: "fstring.py",
		},
		{
			name:     "match statement (Python 3.10+)",
			code:     "match point:\n    case (0, 0):\n        print('origin')\n    case (x, y):\n        print(f'({x}, {y})')\n",
			filename: "match.py",
		},
	}

	validator := NewPythonValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateCode(tt.code, tt.filename)
			if !result.Valid {
				t.Errorf("expected valid Python code, got errors: %v", result.Errors)
			}
			if len(result.Errors) > 0 {
				t.Errorf("expected no errors, got: %v", result.Errors)
			}
		})
	}
}

func TestValidatePythonSyntax_InvalidCode(t *testing.T) {
	skipIfNoPython(t)

	tests := []struct {
		name         string
		code         string
		filename     string
		expectedLine int
	}{
		{
			name:         "missing colon",
			code:         "def foo()\n    pass\n",
			filename:     "missing_colon.py",
			expectedLine: 1,
		},
		{
			name:         "unmatched parenthesis",
			code:         "print('hello'\n",
			filename:     "unmatched.py",
			expectedLine: 1,
		},
		{
			name:         "invalid indentation",
			code:         "def foo():\npass\n",
			filename:     "indent.py",
			expectedLine: 2,
		},
		{
			name:         "invalid syntax keyword",
			code:         "x = return 5\n",
			filename:     "invalid_keyword.py",
			expectedLine: 1,
		},
		{
			name:         "incomplete string",
			code:         `x = "hello`,
			filename:     "incomplete_str.py",
			expectedLine: 1,
		},
		{
			name:         "incomplete multiline string",
			code:         "x = '''\ntest\n",
			filename:     "incomplete_multiline.py",
			expectedLine: 1,
		},
		{
			name:         "invalid assignment target",
			code:         "5 = x\n",
			filename:     "invalid_assign.py",
			expectedLine: 1,
		},
		{
			name:         "break outside loop",
			code:         "break\n",
			filename:     "break_outside.py",
			expectedLine: 1,
		},
		{
			name:         "continue outside loop",
			code:         "continue\n",
			filename:     "continue_outside.py",
			expectedLine: 1,
		},
		{
			name:         "duplicate argument names",
			code:         "def foo(x, x):\n    pass\n",
			filename:     "dup_args.py",
			expectedLine: 1,
		},
		{
			name:         "missing expression after operator",
			code:         "x = 5 +\n",
			filename:     "missing_expr.py",
			expectedLine: 1,
		},
		{
			name:         "invalid decorator",
			code:         "@\ndef foo():\n    pass\n",
			filename:     "invalid_decorator.py",
			expectedLine: 1,
		},
		{
			name:         "unmatched brackets",
			code:         "x = [1, 2, 3\n",
			filename:     "unmatched_bracket.py",
			expectedLine: 1,
		},
	}

	validator := NewPythonValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateCode(tt.code, tt.filename)
			if result.Valid {
				t.Errorf("expected invalid Python code, but got valid")
			}
			if len(result.Errors) == 0 {
				t.Errorf("expected errors, but got none")
				return
			}

			// Check that at least one error matches expected line
			foundLine := false
			for _, err := range result.Errors {
				if err.Line == tt.expectedLine {
					foundLine = true
					break
				}
			}

			if !foundLine && tt.expectedLine > 0 {
				t.Errorf("expected error on line %d, got errors: %v", tt.expectedLine, result.Errors)
			}

			// Verify error message is non-empty
			for _, err := range result.Errors {
				if err.Message == "" {
					t.Errorf("expected non-empty error message, got empty")
				}
			}
		})
	}
}

func TestPythonSyntaxStats(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		wantLines    int
		wantShebang  bool
		wantEmpty    bool
		wantEncoding bool
	}{
		{
			name:      "simple code",
			code:      "x = 1\ny = 2\n",
			wantLines: 3,
		},
		{
			name:        "with shebang",
			code:        "#!/usr/bin/env python3\nprint('hi')\n",
			wantLines:   3,
			wantShebang: true,
		},
		{
			name:      "empty file",
			code:      "",
			wantLines: 1,
			wantEmpty: true,
		},
		{
			name:      "only whitespace",
			code:      "   \n\n   \n",
			wantLines: 4,
			wantEmpty: true,
		},
		{
			name:      "only comments",
			code:      "# comment 1\n# comment 2\n",
			wantLines: 3,
			wantEmpty: true,
		},
		{
			name:         "with encoding",
			code:         "# -*- coding: utf-8 -*-\nprint('hi')\n",
			wantLines:    3,
			wantEncoding: true,
		},
		{
			name:         "encoding in second line",
			code:         "#!/usr/bin/python\n# coding: utf-8\nprint('hi')\n",
			wantLines:    4,
			wantShebang:  true,
			wantEncoding: true,
		},
		{
			name:         "encoding with equals",
			code:         "# coding=utf-8\nprint('hi')\n",
			wantLines:    3,
			wantEncoding: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := computePythonStats(tt.code)

			if stats.LineCount != tt.wantLines {
				t.Errorf("LineCount = %d, want %d", stats.LineCount, tt.wantLines)
			}
			if stats.HasShebang != tt.wantShebang {
				t.Errorf("HasShebang = %v, want %v", stats.HasShebang, tt.wantShebang)
			}
			if stats.IsEmpty != tt.wantEmpty {
				t.Errorf("IsEmpty = %v, want %v", stats.IsEmpty, tt.wantEmpty)
			}
			if stats.HasEncoding != tt.wantEncoding {
				t.Errorf("HasEncoding = %v, want %v", stats.HasEncoding, tt.wantEncoding)
			}
		})
	}
}

func TestIsPythonFile(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"script.py", true},
		{"script.PY", true},
		{"script.pyw", true},
		{"script.PYW", true},
		{"script.txt", false},
		{"script.pyc", false},
		{"script", false},
		{"/path/to/script.py", true},
		{"", false},
		{".py", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := IsPythonFile(tt.path)
			if result != tt.expect {
				t.Errorf("IsPythonFile(%q) = %v, want %v", tt.path, result, tt.expect)
			}
		})
	}
}

func TestShouldValidatePythonSyntax(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{"script.py", true},
		{"src/main.py", true},
		{"script.txt", false},
		{"__pycache__/module.cpython-39.pyc", false},
		{"venv/lib/python3.9/site-packages/pkg.py", false},
		{".venv/lib/site-packages/module.py", false},
		{"dist-packages/pkg.py", false},
		{".tox/py39/lib/module.py", false},
		{"package.egg/module.py", false},
		{"virtualenv/lib/module.py", false},
		{"/path/to/project/app.py", true},
		{"my_venv_project/app.py", true}, // 'venv' as substring in project name is OK
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := ShouldValidatePythonSyntax(tt.path)
			if result != tt.expect {
				t.Errorf("ShouldValidatePythonSyntax(%q) = %v, want %v", tt.path, result, tt.expect)
			}
		})
	}
}

func TestValidatePythonSyntax_Convenience(t *testing.T) {
	skipIfNoPython(t)

	// Test the convenience function
	result := ValidatePythonSyntax("print('hello')", "test.py")
	if !result.Valid {
		t.Errorf("expected valid Python code, got errors: %v", result.Errors)
	}

	result = ValidatePythonSyntax("print('hello'", "test.py")
	if result.Valid {
		t.Error("expected invalid Python code for unclosed string")
	}
}

func TestValidatePythonSyntaxWithConfig(t *testing.T) {
	skipIfNoPython(t)

	config := &PythonValidatorConfig{
		PythonPath: "python3",
		Timeout:    10 * time.Second,
	}

	result := ValidatePythonSyntaxWithConfig("x = 1 + 2", "test.py", config)
	if !result.Valid {
		t.Errorf("expected valid Python code, got errors: %v", result.Errors)
	}
}

func TestPythonValidator_CustomConfig(t *testing.T) {
	// Test with nil config (should use defaults)
	validator := NewPythonValidatorWithConfig(nil)
	if validator.config.PythonPath != "python3" {
		t.Error("expected default python path to be 'python3'")
	}
	if validator.config.Timeout != 5*time.Second {
		t.Error("expected default timeout to be 5 seconds")
	}

	// Test with empty python path (should use default)
	config := &PythonValidatorConfig{
		PythonPath: "",
	}
	validator = NewPythonValidatorWithConfig(config)
	if validator.config.PythonPath != "python3" {
		t.Error("expected empty python path to default to 'python3'")
	}

	// Test with zero timeout (should use default)
	config = &PythonValidatorConfig{
		Timeout: 0,
	}
	validator = NewPythonValidatorWithConfig(config)
	if validator.config.Timeout != 5*time.Second {
		t.Error("expected zero timeout to default to 5 seconds")
	}
}

func TestPythonValidator_SkipIfNoPython(t *testing.T) {
	// Test with invalid python path and SkipIfNoPython = true
	config := &PythonValidatorConfig{
		PythonPath:     "nonexistent-python-interpreter-12345",
		SkipIfNoPython: true,
		Timeout:        1 * time.Second,
	}

	validator := NewPythonValidatorWithConfig(config)
	result := validator.ValidateCode("x = 1", "test.py")

	// Should pass with warning when Python is not found and skip is enabled
	if !result.Valid {
		t.Error("expected validation to pass with SkipIfNoPython=true")
	}
	if len(result.Warnings) == 0 {
		t.Error("expected warning about missing Python interpreter")
	}

	// Test with SkipIfNoPython = false
	config.SkipIfNoPython = false
	validator = NewPythonValidatorWithConfig(config)
	result = validator.ValidateCode("x = 1", "test.py")

	// Should fail when Python is not found and skip is disabled
	if result.Valid {
		t.Error("expected validation to fail with SkipIfNoPython=false and missing Python")
	}
}

func TestPythonSyntaxError_Error(t *testing.T) {
	tests := []struct {
		name   string
		err    PythonSyntaxError
		expect string
	}{
		{
			name: "with line only",
			err: PythonSyntaxError{
				Line:    5,
				Message: "invalid syntax",
			},
			expect: "line 5: invalid syntax",
		},
		{
			name: "with line and column",
			err: PythonSyntaxError{
				Line:    10,
				Column:  15,
				Message: "unexpected EOF",
			},
			expect: "line 10, column 15: unexpected EOF",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expect {
				t.Errorf("Error() = %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestDefaultPythonValidatorConfig(t *testing.T) {
	config := DefaultPythonValidatorConfig()

	if config.PythonPath != "python3" {
		t.Errorf("PythonPath = %q, want %q", config.PythonPath, "python3")
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want %v", config.Timeout, 5*time.Second)
	}
	if config.SkipIfNoPython != false {
		t.Errorf("SkipIfNoPython = %v, want %v", config.SkipIfNoPython, false)
	}
}

func TestParsePythonSyntaxErrors(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		filename   string
		wantCount  int
		wantLine   int
		wantSubstr string
	}{
		{
			name: "standard syntax error",
			output: `  File "test.py", line 5
    print('hello'
         ^
SyntaxError: unexpected EOF while parsing`,
			filename:   "test.py",
			wantCount:  1,
			wantLine:   5,
			wantSubstr: "SyntaxError",
		},
		{
			name: "indentation error",
			output: `  File "test.py", line 3
    return x
    ^
IndentationError: unexpected indent`,
			filename:   "test.py",
			wantCount:  1,
			wantLine:   3,
			wantSubstr: "IndentationError",
		},
		{
			name: "tab error",
			output: `  File "test.py", line 2
    	pass
       ^
TabError: inconsistent use of tabs and spaces`,
			filename:   "test.py",
			wantCount:  1,
			wantLine:   2,
			wantSubstr: "TabError",
		},
		{
			name:       "empty output",
			output:     "",
			filename:   "test.py",
			wantCount:  0,
			wantLine:   0,
			wantSubstr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := parsePythonSyntaxErrors(tt.output, tt.filename)

			if len(errors) != tt.wantCount {
				t.Errorf("got %d errors, want %d", len(errors), tt.wantCount)
				return
			}

			if tt.wantCount > 0 {
				if errors[0].Line != tt.wantLine {
					t.Errorf("Line = %d, want %d", errors[0].Line, tt.wantLine)
				}
				if !strings.Contains(errors[0].Message, tt.wantSubstr) {
					t.Errorf("Message = %q, want to contain %q", errors[0].Message, tt.wantSubstr)
				}
			}
		})
	}
}

func TestValidatePythonSyntax_EmptyFilename(t *testing.T) {
	skipIfNoPython(t)

	// Empty filename should use <string> as default
	validator := NewPythonValidator()
	result := validator.ValidateCode("x = 1", "")

	if !result.Valid {
		t.Errorf("expected valid code, got errors: %v", result.Errors)
	}
}

func TestValidatePythonSyntax_UnicodeCode(t *testing.T) {
	skipIfNoPython(t)

	tests := []struct {
		name  string
		code  string
		valid bool
	}{
		{
			name:  "unicode string literal",
			code:  `msg = "Hello, \u4e16\u754c"`,
			valid: true,
		},
		{
			name:  "unicode variable name",
			code:  "nombre = 'Juan'\n",
			valid: true,
		},
		{
			name:  "emoji in string",
			code:  `x = "Hello 🌍"`,
			valid: true,
		},
		{
			name:  "chinese characters in string",
			code:  `x = "你好世界"`,
			valid: true,
		},
	}

	validator := NewPythonValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.ValidateCode(tt.code, "test.py")
			if result.Valid != tt.valid {
				t.Errorf("Valid = %v, want %v; errors: %v", result.Valid, tt.valid, result.Errors)
			}
		})
	}
}

func TestValidatePythonSyntax_LargeFile(t *testing.T) {
	skipIfNoPython(t)

	// Generate a large but valid Python file
	var builder strings.Builder
	builder.WriteString("# Large test file\n\n")
	for i := 0; i < 1000; i++ {
		builder.WriteString("def func_")
		builder.WriteString(strings.Repeat("a", 5))
		builder.WriteString("_")
		builder.WriteString(string(rune('0' + (i % 10))))
		builder.WriteString("():\n    pass\n\n")
	}

	validator := NewPythonValidator()
	result := validator.ValidateCode(builder.String(), "large.py")

	if !result.Valid {
		t.Errorf("expected large valid file to pass, got errors: %v", result.Errors)
	}
	if result.Stats.LineCount < 3000 {
		t.Errorf("expected many lines, got %d", result.Stats.LineCount)
	}
}
