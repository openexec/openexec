package gates

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunner_RunGate(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "runner-test-*")
	defer os.RemoveAll(tmpDir)

	yamlContent := `
project:
  name: "test"
quality:
  gates: ["success", "fail", "warn"]
  custom:
    - name: "success"
      command: "exit 0"
      mode: "blocking"
    - name: "fail"
      command: "echo 'Error: something went wrong'; exit 1"
      mode: "blocking"
    - name: "warn"
      command: "exit 1"
      mode: "warning"
`
	os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte(yamlContent), 0644)

	runner, err := NewRunner(tmpDir, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to create runner: %v", err)
	}

	ctx := context.Background()

	// Test success
	res := runner.RunGate(ctx, "success")
	if !res.Passed {
		t.Error("expected success gate to pass")
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}

	// Test failure
	res = runner.RunGate(ctx, "fail")
	if res.Passed {
		t.Error("expected fail gate to fail")
	}
	if res.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", res.ExitCode)
	}
	if !strings.Contains(res.FixHint, "Check the output") {
		t.Errorf("unexpected fix hint: %q", res.FixHint)
	}

	// Test warning
	res = runner.RunGate(ctx, "warn")
	if !res.Passed {
		t.Error("expected warning gate to pass (marked as passed but is_warning=true)")
	}
	if !res.IsWarning {
		t.Error("expected IsWarning to be true")
	}
}

func TestRunner_RunAll(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "runner-all-test-*")
	defer os.RemoveAll(tmpDir)

	yamlContent := `
project:
  name: "test"
quality:
  gates: ["g1", "g2"]
  custom:
    - name: "g1"
      command: "exit 0"
    - name: "g2"
      command: "exit 0"
`
	os.WriteFile(filepath.Join(tmpDir, "openexec.yaml"), []byte(yamlContent), 0644)

	runner, _ := NewRunner(tmpDir, 5*time.Second)
	report := runner.RunAll(context.Background())

	if !report.Passed {
		t.Error("expected all gates to pass")
	}
	if len(report.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(report.Results))
	}
}

func TestRunner_FormatForExecutor(t *testing.T) {
	runner := &Runner{}
	
	// Case 1: All passed
	report := &GateReport{
		Passed:  true,
		Summary: "✓ OK",
	}
	formatted := runner.FormatForExecutor(report)
	if !strings.Contains(formatted, "Quality Gates: ✓ OK") {
		t.Errorf("unexpected formatting for pass: %q", formatted)
	}

	// Case 2: Failure
	report = &GateReport{
		Passed:      false,
		Summary:     "✗ 1/1 failed",
		FailedGates: []string{"deps_install"},
		Results: []GateResult{
			{
				Name:   "deps_install",
				Passed: false,
				Output: "ModuleNotFoundError: No module named 'fastapi'",
			},
		},
	}
	
	formatted = runner.FormatForExecutor(report)
	if !strings.Contains(formatted, "QUALITY GATES FAILED") {
		t.Error("missing failure header")
	}
	if !strings.Contains(formatted, "ModuleNotFoundError") {
		t.Error("missing error output in formatted report")
	}
	if !strings.Contains(formatted, "fastapi") {
		t.Error("missing fix hint/example in formatted report")
	}
}

func TestExtractKeyError(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
	}{
		{
			name:  "standard error",
			input: "some log\nError: something broke\nmore log",
			want:  "Error: something broke",
		},
		{
			name:  "python exception",
			input: "traceback\nModuleNotFoundError: No module named 'x'\nend",
			want:  "ModuleNotFoundError: No module named 'x'",
		},
		{
			name:  "no pattern - returns last lines",
			input: "line 1\nline 2\nline 3",
			want:  "line 1\nline 2\nline 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKeyError(tt.input)
			if !strings.Contains(got, tt.want) {
				t.Errorf("extractKeyError() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}
