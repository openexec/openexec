package gates

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExtractKeyError(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected string
	}{
		{
			name: "ModuleNotFoundError",
			output: `Some noise
ModuleNotFoundError: No module named 'fastapi'
  File "app.py", line 1
More noise`,
			expected: "ModuleNotFoundError: No module named 'fastapi'\nFile \"app.py\", line 1\nMore noise",
		},
		{
			name: "NPM Error",
			output: `npm ERR! code ENOENT
npm ERR! syscall open
npm ERR! path /package.json
npm ERR! errno -2`,
			expected: "npm ERR! code ENOENT\nnpm ERR! syscall open\nnpm ERR! path /package.json\nnpm ERR! errno -2",
		},
		{
			name: "Generic Error",
			output: `something happened
Error: failed to connect to database
at connection.go:12
at main.go:5`,
			expected: "Error: failed to connect to database\nat connection.go:12\nat main.go:5",
		},
		{
			name: "No pattern found - returns last lines",
			output: `line 1
line 2
line 3
line 4
line 5
line 6`,
			expected: "line 2\nline 3\nline 4\nline 5\nline 6",
		},
		{
			name:     "Empty output",
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKeyError(tt.output)
			// Using strings.Contains because extractKeyError might have different whitespace handling
			// or slightly different line selection, but let's try exact match first.
			if got != tt.expected {
				t.Errorf("extractKeyError() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestGenerateFixHint(t *testing.T) {
	r := &Runner{}
	tests := []struct {
		name     string
		gateName string
		output   string
		contains string
	}{
		{
			name:     "Known gate deps_install",
			gateName: "deps_install",
			output:   "",
			contains: "Check package.json/requirements.txt",
		},
		{
			name:     "Docker not running in output",
			gateName: "unknown",
			output:   "docker is not running",
			contains: "Docker daemon is not running",
		},
		{
			name:     "Connection refused in output",
			gateName: "unknown",
			output:   "connection refused",
			contains: "Service is not accepting connections",
		},
		{
			name:     "Default hint",
			gateName: "unknown",
			output:   "some random error",
			contains: "Check the output above",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.generateFixHint(tt.gateName, tt.output)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("generateFixHint() = %q, want it to contain %q", got, tt.contains)
			}
		})
	}
}

func TestGenerateSummary(t *testing.T) {
	r := &Runner{}
	tests := []struct {
		name     string
		report   *GateReport
		contains string
	}{
		{
			name: "All passed",
			report: &GateReport{
				Passed: true,
				Results: []GateResult{
					{Passed: true},
					{Passed: true},
				},
			},
			contains: "✓ All 2 gates passed",
		},
		{
			name: "Passed with warnings",
			report: &GateReport{
				Passed: true,
				Results: []GateResult{
					{Passed: true},
					{Passed: true, IsWarning: true},
				},
			},
			contains: "✓ All 2 gates passed (1 warnings)",
		},
		{
			name: "One failed",
			report: &GateReport{
				Passed: false,
				Results: []GateResult{
					{Passed: true},
					{Passed: false},
				},
				FailedGates: []string{"lint"},
			},
			contains: "✗ 1/2 gates failed: lint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.generateSummary(tt.report)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("generateSummary() = %q, want it to contain %q", got, tt.contains)
			}
		})
	}
}

func TestAnalyzeFailure(t *testing.T) {
	r := &Runner{}
	tests := []struct {
		name     string
		res      GateResult
		expected string // Check part of WhyFailed
	}{
		{
			name: "deps_install - missing module",
			res: GateResult{
				Name:   "deps_install",
				Output: "ModuleNotFoundError: No module named 'fastapi'",
			},
			expected: "missing from requirements.txt",
		},
		{
			name: "services_start - port already in use",
			res: GateResult{
				Name:   "services_start",
				Output: "port already in use",
			},
			expected: "port is already in use",
		},
		{
			name: "services_respond - connection refused",
			res: GateResult{
				Name:   "services_respond",
				Output: "connection refused",
			},
			expected: "not listening on the expected port",
		},
		{
			name: "frontend_renders - hydration mismatch",
			res: GateResult{
				Name:   "frontend_renders",
				Output: "hydration mismatch",
			},
			expected: "React hydration mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.analyzeFailure(tt.res)
			if !strings.Contains(got.WhyFailed, tt.expected) {
				t.Errorf("analyzeFailure().WhyFailed = %q, want it to contain %q", got.WhyFailed, tt.expected)
			}
		})
	}
}

func TestRunGate(t *testing.T) {
	tmpDir := t.TempDir()
	r := &Runner{
		projectDir: tmpDir,
		timeout:    5 * time.Second,
		config: &Config{
			Quality: QualityConfig{
				Custom: []CustomGate{
					{Name: "success", Command: "echo 'hello world'"},
					{Name: "fail", Command: "exit 1"},
					{Name: "warning", Command: "exit 1", Mode: "warning"},
				},
			},
		},
	}

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		res := r.RunGate(ctx, "success")
		if !res.Passed {
			t.Errorf("expected passed, got failed: %s", res.Error)
		}
		if !strings.Contains(res.Output, "hello world") {
			t.Errorf("expected output to contain 'hello world', got %q", res.Output)
		}
	})

	t.Run("fail", func(t *testing.T) {
		res := r.RunGate(ctx, "fail")
		if res.Passed {
			t.Error("expected failed, got passed")
		}
		if res.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", res.ExitCode)
		}
	})

	t.Run("warning", func(t *testing.T) {
		res := r.RunGate(ctx, "warning")
		if !res.Passed {
			t.Error("expected passed (warning mode), got failed")
		}
		if !res.IsWarning {
			t.Error("expected IsWarning to be true")
		}
	})

	t.Run("not found", func(t *testing.T) {
		res := r.RunGate(ctx, "non-existent")
		if res.Passed {
			t.Error("expected failed for non-existent gate")
		}
		if !strings.Contains(res.Error, "not found") {
			t.Errorf("expected error to contain 'not found', got %q", res.Error)
		}
	})
}
