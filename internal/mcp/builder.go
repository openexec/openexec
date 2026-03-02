// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements the OrchestratorBuilder for compiling and building
// the orchestrator as part of the meta self-fix capability.
package mcp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// BuildTarget represents a build target type.
type BuildTarget string

const (
	// BuildTargetAll builds the entire project.
	BuildTargetAll BuildTarget = "all"
	// BuildTargetMain builds the main binary.
	BuildTargetMain BuildTarget = "main"
	// BuildTargetPackage builds a specific package.
	BuildTargetPackage BuildTarget = "package"
	// BuildTargetTest runs tests.
	BuildTargetTest BuildTarget = "test"
)

// BuildError represents a single build error from the Go compiler.
type BuildError struct {
	// File is the path to the file with the error.
	File string `json:"file"`
	// Line is the line number (1-indexed).
	Line int `json:"line"`
	// Column is the column number (1-indexed), if available.
	Column int `json:"column,omitempty"`
	// Message is the error description.
	Message string `json:"message"`
	// Code is the error code if available (e.g., for vet errors).
	Code string `json:"code,omitempty"`
}

func (e *BuildError) Error() string {
	if e.Column > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Message)
}

// BuildWarning represents a build warning (e.g., from go vet).
type BuildWarning struct {
	// File is the path to the file with the warning.
	File string `json:"file"`
	// Line is the line number (1-indexed).
	Line int `json:"line"`
	// Column is the column number (1-indexed), if available.
	Column int `json:"column,omitempty"`
	// Message is the warning description.
	Message string `json:"message"`
	// Tool is the tool that generated the warning (e.g., "vet", "staticcheck").
	Tool string `json:"tool,omitempty"`
}

func (w *BuildWarning) Error() string {
	prefix := ""
	if w.Tool != "" {
		prefix = fmt.Sprintf("[%s] ", w.Tool)
	}
	if w.Column > 0 {
		return fmt.Sprintf("%s%s:%d:%d: %s", prefix, w.File, w.Line, w.Column, w.Message)
	}
	return fmt.Sprintf("%s%s:%d: %s", prefix, w.File, w.Line, w.Message)
}

// BuildResult represents the result of a build operation.
type BuildResult struct {
	// Success indicates whether the build completed successfully.
	Success bool `json:"success"`
	// Output is the combined stdout/stderr from the build process.
	Output string `json:"output"`
	// Errors contains parsed build errors.
	Errors []*BuildError `json:"errors,omitempty"`
	// Warnings contains parsed build warnings.
	Warnings []*BuildWarning `json:"warnings,omitempty"`
	// Duration is how long the build took.
	Duration time.Duration `json:"duration"`
	// ArtifactPath is the path to the compiled binary (if applicable).
	ArtifactPath string `json:"artifact_path,omitempty"`
	// ExitCode is the exit code from the build command.
	ExitCode int `json:"exit_code"`
	// Command is the command that was executed.
	Command string `json:"command"`
	// Target is what was built.
	Target BuildTarget `json:"target"`
}

// OrchestratorBuilderConfig holds configuration for the builder.
type OrchestratorBuilderConfig struct {
	// GoPath is the path to the Go binary. If empty, "go" from PATH is used.
	GoPath string
	// Timeout is the maximum time for build operations. Default is 5 minutes.
	Timeout time.Duration
	// Verbose enables verbose output.
	Verbose bool
	// Race enables the race detector.
	Race bool
	// CGOEnabled controls whether CGO is enabled.
	CGOEnabled bool
	// GOOS is the target operating system.
	GOOS string
	// GOARCH is the target architecture.
	GOARCH string
	// LDFlags are linker flags to pass to go build.
	LDFlags []string
	// Tags are build tags to include.
	Tags []string
	// OutputDir is the directory for build artifacts.
	OutputDir string
}

// DefaultOrchestratorBuilderConfig returns a configuration with sensible defaults.
func DefaultOrchestratorBuilderConfig() *OrchestratorBuilderConfig {
	return &OrchestratorBuilderConfig{
		GoPath:     "go",
		Timeout:    5 * time.Minute,
		Verbose:    false,
		Race:       false,
		CGOEnabled: true,
	}
}

// OrchestratorBuilder provides functionality to build the orchestrator.
type OrchestratorBuilder struct {
	// locator is used to find orchestrator source files.
	locator *OrchestratorLocator
	// config holds the build configuration.
	config *OrchestratorBuilderConfig
	// targets are the specific targets to build.
	targets []string
	// runTests indicates whether to run tests as part of the build.
	runTests bool
	// runVet indicates whether to run go vet.
	runVet bool
}

// NewOrchestratorBuilder creates a new OrchestratorBuilder with the given locator.
func NewOrchestratorBuilder(locator *OrchestratorLocator) *OrchestratorBuilder {
	return &OrchestratorBuilder{
		locator:  locator,
		config:   DefaultOrchestratorBuilderConfig(),
		targets:  nil,
		runTests: false,
		runVet:   false,
	}
}

// NewOrchestratorBuilderWithConfig creates a new builder with custom configuration.
func NewOrchestratorBuilderWithConfig(locator *OrchestratorLocator, config *OrchestratorBuilderConfig) *OrchestratorBuilder {
	if config == nil {
		config = DefaultOrchestratorBuilderConfig()
	}
	return &OrchestratorBuilder{
		locator:  locator,
		config:   config,
		targets:  nil,
		runTests: false,
		runVet:   false,
	}
}

// WithGoPath sets the path to the Go binary.
func (b *OrchestratorBuilder) WithGoPath(goPath string) *OrchestratorBuilder {
	b.config.GoPath = goPath
	return b
}

// WithTimeout sets the build timeout.
func (b *OrchestratorBuilder) WithTimeout(timeout time.Duration) *OrchestratorBuilder {
	b.config.Timeout = timeout
	return b
}

// WithTargets sets specific targets to build.
// Targets can be package paths relative to the orchestrator root (e.g., "./cmd/openexec").
func (b *OrchestratorBuilder) WithTargets(targets ...string) *OrchestratorBuilder {
	b.targets = targets
	return b
}

// WithVerbose enables verbose build output.
func (b *OrchestratorBuilder) WithVerbose(verbose bool) *OrchestratorBuilder {
	b.config.Verbose = verbose
	return b
}

// WithRaceDetector enables the race detector.
func (b *OrchestratorBuilder) WithRaceDetector(enabled bool) *OrchestratorBuilder {
	b.config.Race = enabled
	return b
}

// WithCGO enables or disables CGO.
func (b *OrchestratorBuilder) WithCGO(enabled bool) *OrchestratorBuilder {
	b.config.CGOEnabled = enabled
	return b
}

// WithGOOS sets the target operating system for cross-compilation.
func (b *OrchestratorBuilder) WithGOOS(goos string) *OrchestratorBuilder {
	b.config.GOOS = goos
	return b
}

// WithGOARCH sets the target architecture for cross-compilation.
func (b *OrchestratorBuilder) WithGOARCH(goarch string) *OrchestratorBuilder {
	b.config.GOARCH = goarch
	return b
}

// WithLDFlags sets linker flags.
func (b *OrchestratorBuilder) WithLDFlags(flags ...string) *OrchestratorBuilder {
	b.config.LDFlags = flags
	return b
}

// WithTags sets build tags.
func (b *OrchestratorBuilder) WithTags(tags ...string) *OrchestratorBuilder {
	b.config.Tags = tags
	return b
}

// WithOutputDir sets the output directory for build artifacts.
func (b *OrchestratorBuilder) WithOutputDir(dir string) *OrchestratorBuilder {
	b.config.OutputDir = dir
	return b
}

// WithTests enables running tests after the build.
func (b *OrchestratorBuilder) WithTests(enabled bool) *OrchestratorBuilder {
	b.runTests = enabled
	return b
}

// WithVet enables running go vet.
func (b *OrchestratorBuilder) WithVet(enabled bool) *OrchestratorBuilder {
	b.runVet = enabled
	return b
}

// Validate checks if the builder is properly configured.
func (b *OrchestratorBuilder) Validate() error {
	if b.locator == nil {
		return errors.New("orchestrator locator is required")
	}

	// Check if Go is available
	goPath := b.config.GoPath
	if goPath == "" {
		goPath = "go"
	}

	cmd := exec.Command(goPath, "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go binary not found or not executable: %w", err)
	}

	// Validate orchestrator root exists
	root := b.locator.Root()
	if _, err := os.Stat(root); err != nil {
		return fmt.Errorf("orchestrator root not accessible: %w", err)
	}

	// Check for go.mod
	goModPath := filepath.Join(root, "go.mod")
	if _, err := os.Stat(goModPath); err != nil {
		return fmt.Errorf("go.mod not found in orchestrator root: %w", err)
	}

	return nil
}

// Build compiles the orchestrator.
func (b *OrchestratorBuilder) Build(ctx context.Context) (*BuildResult, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Apply timeout
	if b.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.config.Timeout)
		defer cancel()
	}

	// Determine targets
	targets := b.targets
	if len(targets) == 0 {
		targets = []string{"./..."}
	}

	// Build the command arguments
	args := []string{"build"}

	// Add verbose flag
	if b.config.Verbose {
		args = append(args, "-v")
	}

	// Add race detector
	if b.config.Race {
		args = append(args, "-race")
	}

	// Add build tags
	if len(b.config.Tags) > 0 {
		args = append(args, "-tags", strings.Join(b.config.Tags, ","))
	}

	// Add linker flags
	if len(b.config.LDFlags) > 0 {
		args = append(args, "-ldflags", strings.Join(b.config.LDFlags, " "))
	}

	// Add output directory if specified for main binaries
	outputPath := ""
	if b.config.OutputDir != "" && len(targets) == 1 && !strings.HasSuffix(targets[0], "/...") {
		outputPath = filepath.Join(b.config.OutputDir, filepath.Base(targets[0]))
		args = append(args, "-o", outputPath)
	}

	// Add targets
	args = append(args, targets...)

	// Execute the build
	result, err := b.runCommand(ctx, args, BuildTargetMain)
	if err != nil {
		return result, err
	}

	// If build succeeded and we have an output path, record it
	if result.Success && outputPath != "" {
		result.ArtifactPath = outputPath
	}

	return result, nil
}

// Test runs tests for the orchestrator.
func (b *OrchestratorBuilder) Test(ctx context.Context) (*BuildResult, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Apply timeout
	if b.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.config.Timeout)
		defer cancel()
	}

	// Determine targets
	targets := b.targets
	if len(targets) == 0 {
		targets = []string{"./..."}
	}

	// Build the command arguments
	args := []string{"test"}

	// Add verbose flag
	if b.config.Verbose {
		args = append(args, "-v")
	}

	// Add race detector
	if b.config.Race {
		args = append(args, "-race")
	}

	// Add build tags
	if len(b.config.Tags) > 0 {
		args = append(args, "-tags", strings.Join(b.config.Tags, ","))
	}

	// Add targets
	args = append(args, targets...)

	return b.runCommand(ctx, args, BuildTargetTest)
}

// Vet runs go vet on the orchestrator.
func (b *OrchestratorBuilder) Vet(ctx context.Context) (*BuildResult, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Apply timeout - vet is usually faster
	timeout := b.config.Timeout
	if timeout > 2*time.Minute {
		timeout = 2 * time.Minute
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Determine targets
	targets := b.targets
	if len(targets) == 0 {
		targets = []string{"./..."}
	}

	// Build the command arguments
	args := []string{"vet"}

	// Add targets
	args = append(args, targets...)

	return b.runCommand(ctx, args, BuildTargetAll)
}

// BuildAll runs a comprehensive build: vet, build, and optionally test.
func (b *OrchestratorBuilder) BuildAll(ctx context.Context) (*BuildResult, error) {
	// Start timing
	start := time.Now()

	// Run vet first
	vetResult, err := b.Vet(ctx)
	if err != nil {
		return nil, fmt.Errorf("vet failed: %w", err)
	}

	// Collect warnings from vet
	allWarnings := vetResult.Warnings

	// Run build
	buildResult, err := b.Build(ctx)
	if err != nil {
		return nil, fmt.Errorf("build failed: %w", err)
	}

	// If build failed, return immediately
	if !buildResult.Success {
		buildResult.Warnings = append(allWarnings, buildResult.Warnings...)
		buildResult.Duration = time.Since(start)
		buildResult.Target = BuildTargetAll
		return buildResult, nil
	}

	// Run tests if enabled
	if b.runTests {
		testResult, err := b.Test(ctx)
		if err != nil {
			return nil, fmt.Errorf("test failed: %w", err)
		}

		// Combine results
		combined := &BuildResult{
			Success:      testResult.Success,
			Output:       buildResult.Output + "\n\n" + testResult.Output,
			Errors:       append(buildResult.Errors, testResult.Errors...),
			Warnings:     append(allWarnings, buildResult.Warnings...),
			Duration:     time.Since(start),
			ArtifactPath: buildResult.ArtifactPath,
			ExitCode:     testResult.ExitCode,
			Command:      "go build + go test",
			Target:       BuildTargetAll,
		}
		return combined, nil
	}

	// Return build result with accumulated warnings
	buildResult.Warnings = append(allWarnings, buildResult.Warnings...)
	buildResult.Duration = time.Since(start)
	buildResult.Target = BuildTargetAll
	return buildResult, nil
}

// runCommand executes a go command and parses the output.
func (b *OrchestratorBuilder) runCommand(ctx context.Context, args []string, target BuildTarget) (*BuildResult, error) {
	start := time.Now()

	// Build the command
	goPath := b.config.GoPath
	if goPath == "" {
		goPath = "go"
	}

	cmd := exec.CommandContext(ctx, goPath, args...)
	cmd.Dir = b.locator.Root()

	// Set environment
	env := os.Environ()
	if b.config.GOOS != "" {
		env = append(env, "GOOS="+b.config.GOOS)
	}
	if b.config.GOARCH != "" {
		env = append(env, "GOARCH="+b.config.GOARCH)
	}
	if b.config.CGOEnabled {
		env = append(env, "CGO_ENABLED=1")
	} else {
		env = append(env, "CGO_ENABLED=0")
	}
	cmd.Env = env

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	duration := time.Since(start)

	// Combine output
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	// Determine exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Context cancellation or other error
			exitCode = -1
		}
	}

	// Parse errors and warnings
	errors, warnings := parseGoBuildOutput(output, args[0])

	result := &BuildResult{
		Success:  exitCode == 0,
		Output:   output,
		Errors:   errors,
		Warnings: warnings,
		Duration: duration,
		ExitCode: exitCode,
		Command:  goPath + " " + strings.Join(args, " "),
		Target:   target,
	}

	return result, nil
}

// parseGoBuildOutput parses Go compiler output into structured errors and warnings.
func parseGoBuildOutput(output string, cmd string) ([]*BuildError, []*BuildWarning) {
	var errors []*BuildError
	var warnings []*BuildWarning

	// Pattern for Go compiler errors: file.go:line:column: message
	// or: file.go:line: message
	errorPattern := regexp.MustCompile(`^(.+\.go):(\d+):(\d+)?:?\s*(.+)$`)

	// Pattern for vet warnings
	vetPattern := regexp.MustCompile(`^(.+\.go):(\d+):(\d+)?:?\s*(.+)$`)

	lines := strings.Split(output, "\n")
	isVet := cmd == "vet"

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip non-error lines
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "? ") {
			continue
		}
		if strings.HasPrefix(line, "PASS") || strings.HasPrefix(line, "FAIL") {
			continue
		}

		matches := errorPattern.FindStringSubmatch(line)
		if matches == nil {
			// Try vet pattern
			matches = vetPattern.FindStringSubmatch(line)
		}

		if matches != nil {
			file := matches[1]
			lineNum, _ := strconv.Atoi(matches[2])
			column := 0
			message := ""

			if len(matches) > 3 && matches[3] != "" {
				column, _ = strconv.Atoi(matches[3])
				if len(matches) > 4 {
					message = strings.TrimSpace(matches[4])
				}
			} else if len(matches) > 4 {
				message = strings.TrimSpace(matches[4])
			}

			if isVet {
				warnings = append(warnings, &BuildWarning{
					File:    file,
					Line:    lineNum,
					Column:  column,
					Message: message,
					Tool:    "vet",
				})
			} else {
				errors = append(errors, &BuildError{
					File:    file,
					Line:    lineNum,
					Column:  column,
					Message: message,
				})
			}
		}
	}

	return errors, warnings
}

// CheckSyntax validates Go syntax for files before building.
// This provides faster feedback than a full build.
func (b *OrchestratorBuilder) CheckSyntax(ctx context.Context, files ...string) (*BuildResult, error) {
	if err := b.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Use a shorter timeout for syntax checking
	timeout := 30 * time.Second
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// If no specific files, check all
	if len(files) == 0 {
		// Build without output to just check syntax
		args := []string{"build", "-n", "./..."}
		return b.runCommand(ctx, args, BuildTargetAll)
	}

	// For specific files, use go build -n (dry run)
	args := append([]string{"build", "-n"}, files...)
	return b.runCommand(ctx, args, BuildTargetPackage)
}

// GetBuildTargets returns the available build targets in the orchestrator.
func (b *OrchestratorBuilder) GetBuildTargets() ([]string, error) {
	if b.locator == nil {
		return nil, errors.New("orchestrator locator is required")
	}

	root := b.locator.Root()
	var targets []string

	// Check cmd/ directory for main packages
	cmdDir := filepath.Join(root, "cmd")
	if info, err := os.Stat(cmdDir); err == nil && info.IsDir() {
		entries, err := os.ReadDir(cmdDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					targetPath := "./cmd/" + entry.Name()
					targets = append(targets, targetPath)
				}
			}
		}
	}

	// Check for main.go in root
	mainPath := filepath.Join(root, "main.go")
	if _, err := os.Stat(mainPath); err == nil {
		targets = append(targets, ".")
	}

	// Add ./... as a catch-all
	if len(targets) > 0 {
		targets = append(targets, "./...")
	}

	return targets, nil
}

// QuickBuild is a convenience function for building with default settings.
func QuickBuild(ctx context.Context, orchestratorRoot string) (*BuildResult, error) {
	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{
		Root: orchestratorRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create locator: %w", err)
	}

	builder := NewOrchestratorBuilder(locator)
	return builder.Build(ctx)
}

// QuickTest is a convenience function for testing with default settings.
func QuickTest(ctx context.Context, orchestratorRoot string) (*BuildResult, error) {
	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{
		Root: orchestratorRoot,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create locator: %w", err)
	}

	builder := NewOrchestratorBuilder(locator)
	return builder.Test(ctx)
}

// IsBuildError checks if an error is a build error (as opposed to a setup error).
func IsBuildError(err error) bool {
	// Build errors are returned in the result, not as errors
	return false
}

// FormatBuildErrors formats build errors for display.
func FormatBuildErrors(errors []*BuildError) string {
	if len(errors) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Build failed with %d error(s):\n", len(errors)))
	for i, err := range errors {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, err.Error()))
	}
	return sb.String()
}

// FormatBuildWarnings formats build warnings for display.
func FormatBuildWarnings(warnings []*BuildWarning) string {
	if len(warnings) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d warning(s):\n", len(warnings)))
	for i, warn := range warnings {
		sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, warn.Error()))
	}
	return sb.String()
}

// ParsedOutput returns an enhanced parsed representation of the build output.
// This provides more detailed error categorization and suggestions for fixes.
func (r *BuildResult) ParsedOutput() *BuildOutputParseResult {
	parser := NewBuildOutputParser()

	// Determine command type based on the result
	cmdType := "build"
	if r.Target == BuildTargetTest {
		cmdType = "test"
	} else if strings.Contains(r.Command, "vet") {
		cmdType = "vet"
	}

	return parser.Parse(r.Output, cmdType)
}

// GetCategorizedErrors returns errors grouped by category from the parsed output.
func (r *BuildResult) GetCategorizedErrors() map[ErrorCategory][]*ParsedError {
	parsed := r.ParsedOutput()
	result := make(map[ErrorCategory][]*ParsedError)

	for _, err := range parsed.Errors {
		result[err.Category] = append(result[err.Category], err)
	}

	return result
}

// GetSuggestionsForFix returns errors that have fix suggestions.
func (r *BuildResult) GetSuggestionsForFix() []*ParsedError {
	parsed := r.ParsedOutput()
	return GetFixableErrors(parsed)
}
