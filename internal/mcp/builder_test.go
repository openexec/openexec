// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file contains tests for the OrchestratorBuilder.
package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultOrchestratorBuilderConfig(t *testing.T) {
	config := DefaultOrchestratorBuilderConfig()

	if config.GoPath != "go" {
		t.Errorf("expected GoPath 'go', got %q", config.GoPath)
	}
	if config.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout 5 minutes, got %v", config.Timeout)
	}
	if config.Verbose {
		t.Error("expected Verbose to be false by default")
	}
	if config.Race {
		t.Error("expected Race to be false by default")
	}
	if !config.CGOEnabled {
		t.Error("expected CGOEnabled to be true by default")
	}
}

func TestNewOrchestratorBuilder(t *testing.T) {
	// Get orchestrator root
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator)

	if builder.locator != locator {
		t.Error("expected locator to be set")
	}
	if builder.config == nil {
		t.Error("expected config to be initialized")
	}
	if builder.runTests {
		t.Error("expected runTests to be false by default")
	}
	if builder.runVet {
		t.Error("expected runVet to be false by default")
	}
}

func TestNewOrchestratorBuilderWithConfig(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	customConfig := &OrchestratorBuilderConfig{
		GoPath:  "/custom/go",
		Timeout: 10 * time.Minute,
		Verbose: true,
	}

	builder := NewOrchestratorBuilderWithConfig(locator, customConfig)

	if builder.config.GoPath != "/custom/go" {
		t.Errorf("expected GoPath '/custom/go', got %q", builder.config.GoPath)
	}
	if builder.config.Timeout != 10*time.Minute {
		t.Errorf("expected Timeout 10 minutes, got %v", builder.config.Timeout)
	}
	if !builder.config.Verbose {
		t.Error("expected Verbose to be true")
	}
}

func TestNewOrchestratorBuilderWithConfig_NilConfig(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilderWithConfig(locator, nil)

	// Should use defaults
	if builder.config.GoPath != "go" {
		t.Errorf("expected default GoPath 'go', got %q", builder.config.GoPath)
	}
}

func TestOrchestratorBuilder_FluentMethods(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithGoPath("/usr/local/go/bin/go").
		WithTimeout(3 * time.Minute).
		WithTargets("./cmd/openexec", "./internal/...").
		WithVerbose(true).
		WithRaceDetector(true).
		WithCGO(false).
		WithGOOS("linux").
		WithGOARCH("amd64").
		WithLDFlags("-s", "-w").
		WithTags("production", "secure").
		WithOutputDir("/tmp/build").
		WithTests(true).
		WithVet(true)

	// Verify all options are set
	if builder.config.GoPath != "/usr/local/go/bin/go" {
		t.Errorf("expected GoPath '/usr/local/go/bin/go', got %q", builder.config.GoPath)
	}
	if builder.config.Timeout != 3*time.Minute {
		t.Errorf("expected Timeout 3 minutes, got %v", builder.config.Timeout)
	}
	if len(builder.targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(builder.targets))
	}
	if !builder.config.Verbose {
		t.Error("expected Verbose to be true")
	}
	if !builder.config.Race {
		t.Error("expected Race to be true")
	}
	if builder.config.CGOEnabled {
		t.Error("expected CGOEnabled to be false")
	}
	if builder.config.GOOS != "linux" {
		t.Errorf("expected GOOS 'linux', got %q", builder.config.GOOS)
	}
	if builder.config.GOARCH != "amd64" {
		t.Errorf("expected GOARCH 'amd64', got %q", builder.config.GOARCH)
	}
	if len(builder.config.LDFlags) != 2 {
		t.Errorf("expected 2 LDFlags, got %d", len(builder.config.LDFlags))
	}
	if len(builder.config.Tags) != 2 {
		t.Errorf("expected 2 Tags, got %d", len(builder.config.Tags))
	}
	if builder.config.OutputDir != "/tmp/build" {
		t.Errorf("expected OutputDir '/tmp/build', got %q", builder.config.OutputDir)
	}
	if !builder.runTests {
		t.Error("expected runTests to be true")
	}
	if !builder.runVet {
		t.Error("expected runVet to be true")
	}
}

func TestOrchestratorBuilder_Validate(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() *OrchestratorBuilder
		wantErr bool
	}{
		{
			name: "valid configuration",
			setup: func() *OrchestratorBuilder {
				root, _ := DetectOrchestratorRoot()
				locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
				return NewOrchestratorBuilder(locator)
			},
			wantErr: false,
		},
		{
			name: "nil locator",
			setup: func() *OrchestratorBuilder {
				return &OrchestratorBuilder{
					locator: nil,
					config:  DefaultOrchestratorBuilderConfig(),
				}
			},
			wantErr: true,
		},
		{
			name: "invalid go path",
			setup: func() *OrchestratorBuilder {
				root, _ := DetectOrchestratorRoot()
				locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
				return NewOrchestratorBuilder(locator).WithGoPath("/nonexistent/go")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := tt.setup()
			err := builder.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOrchestratorBuilder_Build(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./internal/mcp")

	ctx := context.Background()
	result, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Build() failed with exit code %d: %s", result.ExitCode, result.Output)
	}
	if result.Target != BuildTargetMain {
		t.Errorf("expected target BuildTargetMain, got %v", result.Target)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
	if result.Command == "" {
		t.Error("expected command to be recorded")
	}
}

func TestOrchestratorBuilder_Vet(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping vet test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(1 * time.Minute).
		WithTargets("./internal/mcp")

	ctx := context.Background()
	result, err := builder.Vet(ctx)
	if err != nil {
		t.Fatalf("Vet() error = %v", err)
	}

	if !result.Success {
		t.Errorf("Vet() failed with exit code %d: %s", result.ExitCode, result.Output)
	}
}

func TestOrchestratorBuilder_CheckSyntax(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping syntax check test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator)

	ctx := context.Background()
	result, err := builder.CheckSyntax(ctx)
	if err != nil {
		t.Fatalf("CheckSyntax() error = %v", err)
	}

	if !result.Success {
		t.Errorf("CheckSyntax() failed: %s", result.Output)
	}
}

func TestOrchestratorBuilder_GetBuildTargets(t *testing.T) {
	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator)

	targets, err := builder.GetBuildTargets()
	if err != nil {
		t.Fatalf("GetBuildTargets() error = %v", err)
	}

	if len(targets) == 0 {
		t.Error("expected at least one build target")
	}

	// Check that we found the cmd directories
	foundOpenexec := false
	foundAxon := false
	for _, target := range targets {
		if strings.Contains(target, "openexec") {
			foundOpenexec = true
		}
		if strings.Contains(target, "axon") {
			foundAxon = true
		}
	}

	if !foundOpenexec {
		t.Error("expected to find openexec in build targets")
	}
	if !foundAxon {
		t.Error("expected to find axon in build targets")
	}
}

func TestParseGoBuildOutput(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		cmd          string
		wantErrors   int
		wantWarnings int
	}{
		{
			name:         "empty output",
			output:       "",
			cmd:          "build",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "successful build",
			output:       "# package\nok",
			cmd:          "build",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "syntax error",
			output:       "main.go:10:5: undefined: foo",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "multiple errors",
			output:       "main.go:10:5: undefined: foo\nmain.go:15:10: cannot use x",
			cmd:          "build",
			wantErrors:   2,
			wantWarnings: 0,
		},
		{
			name:         "vet warnings",
			output:       "main.go:10:5: result of fmt.Sprint not used",
			cmd:          "vet",
			wantErrors:   0,
			wantWarnings: 1,
		},
		{
			name:         "error without column",
			output:       "main.go:10: package main",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, warnings := parseGoBuildOutput(tt.output, tt.cmd)
			if len(errors) != tt.wantErrors {
				t.Errorf("parseGoBuildOutput() errors = %d, want %d", len(errors), tt.wantErrors)
			}
			if len(warnings) != tt.wantWarnings {
				t.Errorf("parseGoBuildOutput() warnings = %d, want %d", len(warnings), tt.wantWarnings)
			}
		})
	}
}

func TestBuildError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *BuildError
		expected string
	}{
		{
			name: "with column",
			err: &BuildError{
				File:    "main.go",
				Line:    10,
				Column:  5,
				Message: "undefined: foo",
			},
			expected: "main.go:10:5: undefined: foo",
		},
		{
			name: "without column",
			err: &BuildError{
				File:    "main.go",
				Line:    10,
				Message: "syntax error",
			},
			expected: "main.go:10: syntax error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.expected {
				t.Errorf("BuildError.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildWarning_Error(t *testing.T) {
	tests := []struct {
		name     string
		warn     *BuildWarning
		expected string
	}{
		{
			name: "with tool and column",
			warn: &BuildWarning{
				File:    "main.go",
				Line:    10,
				Column:  5,
				Message: "result not used",
				Tool:    "vet",
			},
			expected: "[vet] main.go:10:5: result not used",
		},
		{
			name: "without tool",
			warn: &BuildWarning{
				File:    "main.go",
				Line:    10,
				Message: "warning message",
			},
			expected: "main.go:10: warning message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.warn.Error(); got != tt.expected {
				t.Errorf("BuildWarning.Error() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatBuildErrors(t *testing.T) {
	tests := []struct {
		name   string
		errors []*BuildError
		want   string
	}{
		{
			name:   "empty",
			errors: nil,
			want:   "",
		},
		{
			name: "single error",
			errors: []*BuildError{
				{File: "main.go", Line: 10, Message: "error"},
			},
			want: "Build failed with 1 error(s):\n  1. main.go:10: error\n",
		},
		{
			name: "multiple errors",
			errors: []*BuildError{
				{File: "main.go", Line: 10, Message: "error1"},
				{File: "util.go", Line: 20, Column: 5, Message: "error2"},
			},
			want: "Build failed with 2 error(s):\n  1. main.go:10: error1\n  2. util.go:20:5: error2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatBuildErrors(tt.errors); got != tt.want {
				t.Errorf("FormatBuildErrors() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatBuildWarnings(t *testing.T) {
	tests := []struct {
		name     string
		warnings []*BuildWarning
		want     string
	}{
		{
			name:     "empty",
			warnings: nil,
			want:     "",
		},
		{
			name: "single warning",
			warnings: []*BuildWarning{
				{File: "main.go", Line: 10, Message: "warning", Tool: "vet"},
			},
			want: "1 warning(s):\n  1. [vet] main.go:10: warning\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatBuildWarnings(tt.warnings); got != tt.want {
				t.Errorf("FormatBuildWarnings() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQuickBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping quick build test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	ctx := context.Background()
	result, err := QuickBuild(ctx, root)
	if err != nil {
		t.Fatalf("QuickBuild() error = %v", err)
	}

	// The build may or may not succeed depending on the state of the codebase,
	// but we should at least get a result
	if result.Command == "" {
		t.Error("expected command to be recorded")
	}
}

func TestQuickBuild_InvalidRoot(t *testing.T) {
	ctx := context.Background()
	_, err := QuickBuild(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("QuickBuild() expected error for invalid root")
	}
}

func TestOrchestratorBuilder_BuildWithTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Set a very short timeout
	builder := NewOrchestratorBuilder(locator).
		WithTimeout(1 * time.Millisecond)

	ctx := context.Background()
	result, err := builder.Build(ctx)

	// Either we get an error or the build should fail due to timeout
	// The behavior depends on how fast the command starts
	if err == nil && result.Success {
		// Build completed before timeout - this is fine for fast machines
		t.Log("Build completed before timeout (fast machine)")
	}
}

func TestOrchestratorBuilder_BuildWithOutputDir(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping build with output test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Create temp directory for output
	tmpDir, err := os.MkdirTemp("", "openexec-build-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./cmd/openexec").
		WithOutputDir(tmpDir)

	ctx := context.Background()
	result, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if result.Success {
		// Check that artifact path is set
		if result.ArtifactPath == "" {
			t.Error("expected ArtifactPath to be set for successful build with output dir")
		} else {
			// Verify the binary was created
			expectedPath := filepath.Join(tmpDir, "openexec")
			if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
				t.Errorf("expected binary at %s but it doesn't exist", expectedPath)
			}
		}
	}
}

func TestIsBuildError(t *testing.T) {
	// IsBuildError always returns false since build errors are in the result
	if IsBuildError(nil) {
		t.Error("IsBuildError(nil) should return false")
	}
	if IsBuildError(os.ErrNotExist) {
		t.Error("IsBuildError(os.ErrNotExist) should return false")
	}
}

func TestBuildResult_ParsedOutput(t *testing.T) {
	result := &BuildResult{
		Output: `./main.go:10:5: undefined: foo
./util.go:15:8: cannot use x as type y`,
		Target:  BuildTargetMain,
		Command: "go build ./...",
	}

	parsed := result.ParsedOutput()

	if parsed == nil {
		t.Fatal("ParsedOutput() returned nil")
	}
	if len(parsed.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(parsed.Errors))
	}
	if parsed.Summary.TotalErrors != 2 {
		t.Errorf("expected TotalErrors 2, got %d", parsed.Summary.TotalErrors)
	}
}

func TestBuildResult_ParsedOutput_Test(t *testing.T) {
	result := &BuildResult{
		Output: `=== RUN   TestFoo
--- FAIL: TestFoo (0.00s)
FAIL`,
		Target:  BuildTargetTest,
		Command: "go test ./...",
	}

	parsed := result.ParsedOutput()

	if parsed == nil {
		t.Fatal("ParsedOutput() returned nil")
	}
	if len(parsed.TestFailures) != 1 {
		t.Errorf("expected 1 test failure, got %d", len(parsed.TestFailures))
	}
}

func TestBuildResult_ParsedOutput_Vet(t *testing.T) {
	result := &BuildResult{
		Output:  `./main.go:10:5: result not used`,
		Target:  BuildTargetAll,
		Command: "go vet ./...",
	}

	parsed := result.ParsedOutput()

	if parsed == nil {
		t.Fatal("ParsedOutput() returned nil")
	}
	if len(parsed.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(parsed.Warnings))
	}
}

func TestBuildResult_GetCategorizedErrors(t *testing.T) {
	result := &BuildResult{
		Output: `./main.go:10:5: undefined: foo
./util.go:15:8: cannot use x as type y
./handler.go:20:3: expected ';'`,
		Target:  BuildTargetMain,
		Command: "go build ./...",
	}

	categorized := result.GetCategorizedErrors()

	if len(categorized) == 0 {
		t.Error("expected categorized errors")
	}

	// Should have errors in different categories
	totalErrors := 0
	for _, errs := range categorized {
		totalErrors += len(errs)
	}
	if totalErrors != 3 {
		t.Errorf("expected 3 total errors, got %d", totalErrors)
	}
}

func TestBuildResult_GetSuggestionsForFix(t *testing.T) {
	result := &BuildResult{
		Output: `./main.go:10:5: undefined: myFunc
./util.go:15:8: random error with no suggestion`,
		Target:  BuildTargetMain,
		Command: "go build ./...",
	}

	suggestions := result.GetSuggestionsForFix()

	// At least the undefined error should have a suggestion
	foundSuggestion := false
	for _, s := range suggestions {
		if strings.Contains(s.Message, "undefined") && s.Suggestion != "" {
			foundSuggestion = true
			break
		}
	}
	if !foundSuggestion {
		t.Error("expected to find suggestion for undefined error")
	}
}

func TestOrchestratorBuilder_Test(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test method test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./internal/mcp")

	ctx := context.Background()
	result, err := builder.Test(ctx)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if result.Target != BuildTargetTest {
		t.Errorf("expected target BuildTargetTest, got %v", result.Target)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
	if result.Command == "" {
		t.Error("expected command to be recorded")
	}
	if !strings.Contains(result.Command, "test") {
		t.Error("expected command to contain 'test'")
	}
}

func TestOrchestratorBuilder_TestWithVerbose(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping verbose test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(1 * time.Minute).
		WithTargets("./internal/mcp").
		WithVerbose(true)

	ctx := context.Background()
	result, err := builder.Test(ctx)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if !strings.Contains(result.Command, "-v") {
		t.Error("expected command to contain '-v' flag for verbose")
	}
}

func TestOrchestratorBuilder_TestWithRaceDetector(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping race detector test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./internal/mcp").
		WithRaceDetector(true)

	ctx := context.Background()
	result, err := builder.Test(ctx)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if !strings.Contains(result.Command, "-race") {
		t.Error("expected command to contain '-race' flag")
	}
}

func TestOrchestratorBuilder_TestWithTags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping tags test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(1 * time.Minute).
		WithTargets("./internal/mcp").
		WithTags("integration")

	ctx := context.Background()
	result, err := builder.Test(ctx)
	if err != nil {
		t.Fatalf("Test() error = %v", err)
	}

	if !strings.Contains(result.Command, "-tags") {
		t.Error("expected command to contain '-tags' flag")
	}
}

func TestOrchestratorBuilder_BuildAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping BuildAll test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(3 * time.Minute).
		WithTargets("./internal/mcp")

	ctx := context.Background()
	result, err := builder.BuildAll(ctx)
	if err != nil {
		t.Fatalf("BuildAll() error = %v", err)
	}

	if result.Target != BuildTargetAll {
		t.Errorf("expected target BuildTargetAll, got %v", result.Target)
	}
	if result.Duration == 0 {
		t.Error("expected non-zero duration")
	}
}

func TestOrchestratorBuilder_BuildAllWithTests(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping BuildAll with tests in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(3 * time.Minute).
		WithTargets("./internal/mcp").
		WithTests(true)

	ctx := context.Background()
	result, err := builder.BuildAll(ctx)
	if err != nil {
		t.Fatalf("BuildAll() with tests error = %v", err)
	}

	if result.Target != BuildTargetAll {
		t.Errorf("expected target BuildTargetAll, got %v", result.Target)
	}
	// Command should indicate both build and test were run
	if result.Command == "" {
		t.Error("expected command to be recorded")
	}
}

func TestQuickTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping quick test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	ctx := context.Background()
	result, err := QuickTest(ctx, root)
	if err != nil {
		t.Fatalf("QuickTest() error = %v", err)
	}

	if result.Command == "" {
		t.Error("expected command to be recorded")
	}
	if result.Target != BuildTargetTest {
		t.Errorf("expected target BuildTargetTest, got %v", result.Target)
	}
}

func TestQuickTest_InvalidRoot(t *testing.T) {
	ctx := context.Background()
	_, err := QuickTest(ctx, "/nonexistent/path")
	if err == nil {
		t.Error("QuickTest() expected error for invalid root")
	}
}

func TestParseGoBuildOutput_EdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		cmd          string
		wantErrors   int
		wantWarnings int
	}{
		{
			name:         "import error",
			output:       `./main.go:5:2: cannot find package "nonexistent" in any of`,
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "type error",
			output:       `./main.go:10:5: cannot use x (type int) as type string in argument`,
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "package comment line",
			output:       `# github.com/example/package`,
			cmd:          "build",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "test ok line",
			output:       `ok  	github.com/example/package	0.001s`,
			cmd:          "test",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "test skipped line",
			output:       `? 	github.com/example/package	[no test files]`,
			cmd:          "test",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "PASS line",
			output:       `PASS`,
			cmd:          "test",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "FAIL line",
			output:       `FAIL`,
			cmd:          "test",
			wantErrors:   0,
			wantWarnings: 0,
		},
		{
			name:         "multiple vet warnings",
			output:       "main.go:10:5: result of fmt.Sprint not used\nutil.go:20:3: unreachable code",
			cmd:          "vet",
			wantErrors:   0,
			wantWarnings: 2,
		},
		{
			name:         "mixed with non-error lines",
			output:       "# building\nmain.go:10:5: error\nok done",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "error with only file and line",
			output:       "main.go:10: syntax error",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
		{
			name:         "deep path error",
			output:       "./internal/pkg/sub/file.go:100:20: undefined: SomeFunc",
			cmd:          "build",
			wantErrors:   1,
			wantWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors, warnings := parseGoBuildOutput(tt.output, tt.cmd)
			if len(errors) != tt.wantErrors {
				t.Errorf("parseGoBuildOutput() errors = %d, want %d; got: %v", len(errors), tt.wantErrors, errors)
			}
			if len(warnings) != tt.wantWarnings {
				t.Errorf("parseGoBuildOutput() warnings = %d, want %d; got: %v", len(warnings), tt.wantWarnings, warnings)
			}
		})
	}
}

func TestParseGoBuildOutput_ErrorDetails(t *testing.T) {
	output := "./main.go:10:5: undefined: foo"
	errors, _ := parseGoBuildOutput(output, "build")

	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(errors))
	}

	err := errors[0]
	if err.File != "./main.go" {
		t.Errorf("expected file './main.go', got %q", err.File)
	}
	if err.Line != 10 {
		t.Errorf("expected line 10, got %d", err.Line)
	}
	if err.Column != 5 {
		t.Errorf("expected column 5, got %d", err.Column)
	}
	if err.Message != "undefined: foo" {
		t.Errorf("expected message 'undefined: foo', got %q", err.Message)
	}
}

func TestParseGoBuildOutput_WarningDetails(t *testing.T) {
	output := "./main.go:10:5: result of fmt.Sprint not used"
	_, warnings := parseGoBuildOutput(output, "vet")

	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(warnings))
	}

	warn := warnings[0]
	if warn.File != "./main.go" {
		t.Errorf("expected file './main.go', got %q", warn.File)
	}
	if warn.Line != 10 {
		t.Errorf("expected line 10, got %d", warn.Line)
	}
	if warn.Column != 5 {
		t.Errorf("expected column 5, got %d", warn.Column)
	}
	if warn.Tool != "vet" {
		t.Errorf("expected tool 'vet', got %q", warn.Tool)
	}
}

func TestOrchestratorBuilder_CheckSyntaxWithSpecificFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping syntax check with specific files test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator)

	// Check syntax for specific files
	ctx := context.Background()
	builderFile := filepath.Join(root, "internal/mcp/builder.go")
	result, err := builder.CheckSyntax(ctx, builderFile)
	if err != nil {
		t.Fatalf("CheckSyntax() error = %v", err)
	}

	if result.Target != BuildTargetPackage {
		t.Errorf("expected target BuildTargetPackage, got %v", result.Target)
	}
}

func TestOrchestratorBuilder_GetBuildTargets_NilLocator(t *testing.T) {
	builder := &OrchestratorBuilder{
		locator: nil,
		config:  DefaultOrchestratorBuilderConfig(),
	}

	_, err := builder.GetBuildTargets()
	if err == nil {
		t.Error("GetBuildTargets() expected error for nil locator")
	}
}

func TestOrchestratorBuilder_ValidateOrchestrator_InvalidRoot(t *testing.T) {
	// Create a locator with a path that has no go.mod
	tmpDir, err := os.MkdirTemp("", "test-no-gomod")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create locator manually to bypass validation
	locator := &OrchestratorLocator{root: tmpDir}
	builder := &OrchestratorBuilder{
		locator: locator,
		config:  DefaultOrchestratorBuilderConfig(),
	}

	err = builder.Validate()
	if err == nil {
		t.Error("Validate() expected error for directory without go.mod")
	}
	if !strings.Contains(err.Error(), "go.mod not found") {
		t.Errorf("expected error about go.mod, got: %v", err)
	}
}

func TestOrchestratorBuilder_BuildWithCrossCompilation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping cross-compilation test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./internal/mcp").
		WithGOOS("linux").
		WithGOARCH("amd64").
		WithCGO(false)

	ctx := context.Background()
	result, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	// Build should succeed for package that has no CGO dependencies
	if !result.Success {
		t.Errorf("Build() failed for cross-compilation: %s", result.Output)
	}
}

func TestOrchestratorBuilder_BuildWithLDFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping LDFlags test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(2 * time.Minute).
		WithTargets("./internal/mcp").
		WithLDFlags("-s", "-w")

	ctx := context.Background()
	result, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !strings.Contains(result.Command, "-ldflags") {
		t.Error("expected command to contain '-ldflags'")
	}
}

func TestOrchestratorBuilder_BuildContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping context cancellation test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	builder := NewOrchestratorBuilder(locator).
		WithTimeout(0) // No timeout from builder

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := builder.Build(ctx)

	// Either we get an error or the build fails due to cancellation
	if err == nil && result.Success {
		t.Log("Build completed before cancellation took effect")
	}
}

func TestOrchestratorBuilder_VetTimeoutAdjustment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping vet timeout test in short mode")
	}

	root, err := DetectOrchestratorRoot()
	if err != nil {
		t.Fatalf("failed to detect orchestrator root: %v", err)
	}

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: root})
	if err != nil {
		t.Fatalf("failed to create locator: %v", err)
	}

	// Set a long timeout that should be capped to 2 minutes for vet
	builder := NewOrchestratorBuilder(locator).
		WithTimeout(10 * time.Minute).
		WithTargets("./internal/mcp")

	ctx := context.Background()
	result, err := builder.Vet(ctx)
	if err != nil {
		t.Fatalf("Vet() error = %v", err)
	}

	// Vet should complete much faster than 10 minutes due to the cap
	if result.Duration > 2*time.Minute {
		t.Error("Vet took longer than expected - timeout cap may not be working")
	}
}

func TestBuildTargetConstants(t *testing.T) {
	// Verify the build target constants have expected values
	if BuildTargetAll != "all" {
		t.Errorf("expected BuildTargetAll to be 'all', got %q", BuildTargetAll)
	}
	if BuildTargetMain != "main" {
		t.Errorf("expected BuildTargetMain to be 'main', got %q", BuildTargetMain)
	}
	if BuildTargetPackage != "package" {
		t.Errorf("expected BuildTargetPackage to be 'package', got %q", BuildTargetPackage)
	}
	if BuildTargetTest != "test" {
		t.Errorf("expected BuildTargetTest to be 'test', got %q", BuildTargetTest)
	}
}

func TestBuildResult_Fields(t *testing.T) {
	result := &BuildResult{
		Success:      true,
		Output:       "build output",
		Errors:       []*BuildError{{File: "test.go", Line: 1, Message: "error"}},
		Warnings:     []*BuildWarning{{File: "test.go", Line: 2, Message: "warning"}},
		Duration:     time.Second,
		ArtifactPath: "/path/to/artifact",
		ExitCode:     0,
		Command:      "go build",
		Target:       BuildTargetMain,
	}

	if !result.Success {
		t.Error("expected Success to be true")
	}
	if result.Output != "build output" {
		t.Errorf("expected Output 'build output', got %q", result.Output)
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
	if len(result.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.Duration != time.Second {
		t.Errorf("expected duration 1s, got %v", result.Duration)
	}
	if result.ArtifactPath != "/path/to/artifact" {
		t.Errorf("expected ArtifactPath '/path/to/artifact', got %q", result.ArtifactPath)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected ExitCode 0, got %d", result.ExitCode)
	}
	if result.Command != "go build" {
		t.Errorf("expected Command 'go build', got %q", result.Command)
	}
	if result.Target != BuildTargetMain {
		t.Errorf("expected Target BuildTargetMain, got %v", result.Target)
	}
}

func TestOrchestratorBuilderConfig_Fields(t *testing.T) {
	config := &OrchestratorBuilderConfig{
		GoPath:     "/usr/bin/go",
		Timeout:    5 * time.Minute,
		Verbose:    true,
		Race:       true,
		CGOEnabled: false,
		GOOS:       "darwin",
		GOARCH:     "arm64",
		LDFlags:    []string{"-s", "-w"},
		Tags:       []string{"tag1", "tag2"},
		OutputDir:  "/tmp/output",
	}

	if config.GoPath != "/usr/bin/go" {
		t.Errorf("expected GoPath '/usr/bin/go', got %q", config.GoPath)
	}
	if config.Timeout != 5*time.Minute {
		t.Errorf("expected Timeout 5m, got %v", config.Timeout)
	}
	if !config.Verbose {
		t.Error("expected Verbose to be true")
	}
	if !config.Race {
		t.Error("expected Race to be true")
	}
	if config.CGOEnabled {
		t.Error("expected CGOEnabled to be false")
	}
	if config.GOOS != "darwin" {
		t.Errorf("expected GOOS 'darwin', got %q", config.GOOS)
	}
	if config.GOARCH != "arm64" {
		t.Errorf("expected GOARCH 'arm64', got %q", config.GOARCH)
	}
	if len(config.LDFlags) != 2 {
		t.Errorf("expected 2 LDFlags, got %d", len(config.LDFlags))
	}
	if len(config.Tags) != 2 {
		t.Errorf("expected 2 Tags, got %d", len(config.Tags))
	}
	if config.OutputDir != "/tmp/output" {
		t.Errorf("expected OutputDir '/tmp/output', got %q", config.OutputDir)
	}
}

func TestBuildWarning_ErrorWithoutTool(t *testing.T) {
	warn := &BuildWarning{
		File:    "main.go",
		Line:    10,
		Column:  5,
		Message: "result not used",
		Tool:    "", // No tool specified
	}

	expected := "main.go:10:5: result not used"
	if got := warn.Error(); got != expected {
		t.Errorf("BuildWarning.Error() = %q, want %q", got, expected)
	}
}

func TestBuildError_ErrorWithCode(t *testing.T) {
	err := &BuildError{
		File:    "main.go",
		Line:    10,
		Column:  5,
		Message: "undefined: foo",
		Code:    "E001",
	}

	// Error() doesn't include Code, just verifying field access
	if err.Code != "E001" {
		t.Errorf("expected Code 'E001', got %q", err.Code)
	}

	expected := "main.go:10:5: undefined: foo"
	if got := err.Error(); got != expected {
		t.Errorf("BuildError.Error() = %q, want %q", got, expected)
	}
}
