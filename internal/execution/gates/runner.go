package gates

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// GateResult holds the result of running a single gate.
type GateResult struct {
	Name      string        `json:"name"`
	Passed    bool          `json:"passed"`
	Output    string        `json:"output"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration_ms"`
	ExitCode  int           `json:"exit_code"`
	IsWarning bool          `json:"is_warning,omitempty"`
	FixHint   string        `json:"fix_hint,omitempty"`
}

// GateReport holds results from running all gates.
type GateReport struct {
	Passed      bool          `json:"passed"`
	Results     []GateResult  `json:"results"`
	TotalTime   time.Duration `json:"total_time_ms"`
	FailedGates []string      `json:"failed_gates,omitempty"`
	Summary     string        `json:"summary"`
}

// Runner executes quality gates.
type Runner struct {
	config     *Config
	projectDir string
	timeout    time.Duration
}

// NewRunner creates a new gate runner.
func NewRunner(projectDir string, timeout time.Duration) (*Runner, error) {
	cfg, err := LoadConfig(projectDir)
	if err != nil {
		// Return runner without config - will skip gates
		return &Runner{
			projectDir: projectDir,
			timeout:    timeout,
		}, nil
	}

	return &Runner{
		config:     cfg,
		projectDir: projectDir,
		timeout:    timeout,
	}, nil
}

// RunAll executes all configured quality gates.
func (r *Runner) RunAll(ctx context.Context) *GateReport {
	report := &GateReport{
		Passed:  true,
		Results: []GateResult{},
	}

	if r.config == nil {
		report.Summary = "No openexec.yaml found, skipping quality gates"
		return report
	}

	enabledGates := r.config.GetEnabledGates()
	if len(enabledGates) == 0 {
		report.Summary = "No gates configured"
		return report
	}

	startTime := time.Now()

	for _, gateName := range enabledGates {
		result := r.RunGate(ctx, gateName)
		report.Results = append(report.Results, result)

		if !result.Passed && !result.IsWarning {
			report.Passed = false
			report.FailedGates = append(report.FailedGates, gateName)
		}
	}

	report.TotalTime = time.Since(startTime)
	report.Summary = r.generateSummary(report)

	return report
}

// RunGate executes a single quality gate.
func (r *Runner) RunGate(ctx context.Context, name string) GateResult {
	result := GateResult{
		Name: name,
	}

	if r.config == nil {
		result.Passed = true
		result.Output = "No config, skipping"
		return result
	}

	gate := r.config.GetGate(name)
	if gate == nil {
		result.Passed = false
		result.Error = fmt.Sprintf("gate '%s' not found in config", name)
		return result
	}

	// Determine timeout
	timeout := r.timeout
	if gate.Timeout > 0 {
		timeout = time.Duration(gate.Timeout) * time.Second
	}

	// Create context with timeout
	gateCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	startTime := time.Now()

	// Run the command
	cmd := exec.CommandContext(gateCtx, "bash", "-c", gate.Command) // #nosec G204
	cmd.Dir = r.projectDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Duration = time.Since(startTime)

	// Combine stdout and stderr for output
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}
	result.Output = output

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		result.Error = err.Error()

		// Check if warning mode
		if gate.Mode == "warning" {
			result.Passed = true
			result.IsWarning = true
		} else {
			result.Passed = false
			result.FixHint = r.generateFixHint(name, output)
		}
	} else {
		result.Passed = true
		result.ExitCode = 0
	}

	return result
}

// generateSummary creates a human-readable summary.
func (r *Runner) generateSummary(report *GateReport) string {
	total := len(report.Results)
	passed := 0
	warnings := 0

	for _, res := range report.Results {
		if res.Passed {
			passed++
			if res.IsWarning {
				warnings++
			}
		}
	}

	failed := total - passed

	if report.Passed {
		if warnings > 0 {
			return fmt.Sprintf("✓ All %d gates passed (%d warnings)", total, warnings)
		}
		return fmt.Sprintf("✓ All %d gates passed", total)
	}

	return fmt.Sprintf("✗ %d/%d gates failed: %s", failed, total, strings.Join(report.FailedGates, ", "))
}

// generateFixHint provides guidance on how to fix a failed gate.
func (r *Runner) generateFixHint(gateName, output string) string {
	hints := map[string]string{
		"deps_install":     "Check package.json/requirements.txt for missing or incompatible dependencies",
		"services_start":   "Check Docker is running: 'docker ps'. Check container logs for startup errors.",
		"services_respond": "Services started but aren't responding. Check health endpoints and logs.",
		"frontend_renders": "Frontend returns blank/error page. Check browser console and Next.js build.",
		"e2e":              "E2E tests failed. Check test output for specific failures.",
		"typecheck":        "TypeScript/MyPy errors. Fix type annotations.",
		"lint":             "Linting errors. Run formatter or fix code style issues.",
	}

	if hint, ok := hints[gateName]; ok {
		return hint
	}

	// Try to extract hints from output
	outputLower := strings.ToLower(output)
	if strings.Contains(outputLower, "docker") && strings.Contains(outputLower, "not running") {
		return "Docker daemon is not running. Start Docker Desktop or run 'systemctl start docker'."
	}
	if strings.Contains(outputLower, "connection refused") {
		return "Service is not accepting connections. Check if it started correctly."
	}
	if strings.Contains(outputLower, "module not found") || strings.Contains(outputLower, "no such file") {
		return "Missing dependencies or files. Run 'npm install' or check file paths."
	}

	return "Check the output above for error details"
}

// FormatForExecutor formats the gate report for feeding back to the executor.
// Provides senior architect-level guidance on what to fix and how.
func (r *Runner) FormatForExecutor(report *GateReport) string {
	if report.Passed {
		return fmt.Sprintf("Quality Gates: %s", report.Summary)
	}

	var sb strings.Builder
	sb.WriteString("## QUALITY GATES FAILED - Senior Review\n\n")
	sb.WriteString("Your implementation does not pass validation. Here's a detailed analysis:\n\n")

	sb.WriteString("### Assessment\n\n")
	sb.WriteString(fmt.Sprintf("%s\n\n", report.Summary))

	sb.WriteString("### Failed Gates Analysis\n\n")
	for _, res := range report.Results {
		if !res.Passed && !res.IsWarning {
			sb.WriteString(fmt.Sprintf("#### ❌ Gate: `%s`\n\n", res.Name))

			// Analyze the failure
			analysis := r.analyzeFailure(res)
			sb.WriteString(fmt.Sprintf("**What Failed**: %s\n\n", analysis.WhatFailed))
			sb.WriteString(fmt.Sprintf("**Why It Failed**: %s\n\n", analysis.WhyFailed))

			if res.Output != "" {
				// Extract key error from output
				keyError := extractKeyError(res.Output)
				if keyError != "" {
					sb.WriteString(fmt.Sprintf("**Key Error**:\n```\n%s\n```\n\n", keyError))
				}
			}

			sb.WriteString("**How to Fix**:\n")
			for i, step := range analysis.FixSteps {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
			}
			sb.WriteString("\n")

			if analysis.Example != "" {
				sb.WriteString(fmt.Sprintf("**Example**:\n```\n%s\n```\n\n", analysis.Example))
			}
		}
	}

	sb.WriteString("### Required Actions\n\n")
	sb.WriteString("Before signaling completion again, you MUST:\n\n")
	for i, gate := range report.FailedGates {
		sb.WriteString(fmt.Sprintf("%d. Fix the `%s` gate failure as described above\n", i+1, gate))
	}
	sb.WriteString("\n")
	sb.WriteString("The gates will automatically re-run after you signal completion.\n")
	sb.WriteString("Do NOT just signal completion - actually fix the issues first.\n")

	return sb.String()
}

// FailureAnalysis provides structured analysis of a gate failure.
type FailureAnalysis struct {
	WhatFailed string
	WhyFailed  string
	FixSteps   []string
	Example    string
}

// analyzeFailure provides detailed analysis of why a gate failed.
func (r *Runner) analyzeFailure(res GateResult) FailureAnalysis {
	analysis := FailureAnalysis{
		WhatFailed: fmt.Sprintf("The %s validation check", res.Name),
		WhyFailed:  "Unknown error",
		FixSteps:   []string{"Review the error output", "Fix the underlying issue"},
	}

	output := strings.ToLower(res.Output)

	switch res.Name {
	case "deps_install":
		analysis.WhatFailed = "Dependency installation failed"
		if strings.Contains(output, "modulenotfounderror") || strings.Contains(output, "no module") {
			analysis.WhyFailed = "A Python module is missing from requirements.txt"
			analysis.FixSteps = []string{
				"Check the error for the missing module name",
				"Add it to requirements.txt (backend) or pyproject.toml",
				"Ensure the version is compatible with other dependencies",
			}
			analysis.Example = "# Add to requirements.txt:\nfastapi>=0.100.0\nuvicorn>=0.23.0"
		} else if strings.Contains(output, "npm err") || strings.Contains(output, "enoent") {
			analysis.WhyFailed = "NPM package installation failed"
			analysis.FixSteps = []string{
				"Check package.json for invalid packages",
				"Remove node_modules and package-lock.json, then reinstall",
				"Check for version conflicts",
			}
		}

	case "services_start":
		analysis.WhatFailed = "Docker containers failed to start or exited unexpectedly"
		if strings.Contains(output, "no such file") || strings.Contains(output, "not found") {
			analysis.WhyFailed = "A required file or command is missing in the container"
			analysis.FixSteps = []string{
				"Check Dockerfile for correct COPY commands",
				"Verify the entrypoint/command exists",
				"Check file paths in docker-compose.yml",
			}
		} else if strings.Contains(output, "modulenotfounderror") {
			analysis.WhyFailed = "The container is missing Python dependencies"
			analysis.FixSteps = []string{
				"Ensure requirements.txt includes ALL required packages",
				"Add the missing module to requirements.txt",
				"Rebuild the container: docker-compose build --no-cache",
			}
		} else if strings.Contains(output, "port") && strings.Contains(output, "already") {
			analysis.WhyFailed = "The port is already in use"
			analysis.FixSteps = []string{
				"Stop other services using the port",
				"Or change the port in docker-compose.yml",
			}
		} else {
			analysis.WhyFailed = "Container crashed during startup - check logs for details"
			analysis.FixSteps = []string{
				"Review the container logs above for the actual error",
				"Fix the application code that's causing the crash",
				"Ensure environment variables are set correctly",
			}
		}

	case "services_respond":
		analysis.WhatFailed = "Services started but are not responding to HTTP requests"
		if strings.Contains(output, "connection refused") {
			analysis.WhyFailed = "The service is not listening on the expected port"
			analysis.FixSteps = []string{
				"Check the application binds to 0.0.0.0 (not 127.0.0.1) inside Docker",
				"Verify the port mapping in docker-compose.yml",
				"Check the health endpoint exists and is correct",
			}
			analysis.Example = "# In FastAPI:\nuvicorn.run(app, host=\"0.0.0.0\", port=8001)"
		} else if strings.Contains(output, "timeout") {
			analysis.WhyFailed = "The service is too slow to respond"
			analysis.FixSteps = []string{
				"Check if the application is stuck in initialization",
				"Increase the timeout in the gate command",
				"Check for blocking operations on startup",
			}
		}

	case "frontend_renders":
		analysis.WhatFailed = "Frontend returns blank page or error instead of actual content"
		if strings.Contains(output, "failed to compile") {
			analysis.WhyFailed = "Next.js build/compilation failed"
			analysis.FixSteps = []string{
				"Check for TypeScript errors in the code",
				"Fix import statements and missing dependencies",
				"Run 'npm run build' locally to see full errors",
			}
		} else if strings.Contains(output, "hydration") {
			analysis.WhyFailed = "React hydration mismatch between server and client"
			analysis.FixSteps = []string{
				"Check for browser-only code running on server",
				"Use dynamic imports with ssr: false for browser components",
				"Ensure consistent state between server and client render",
			}
		} else if strings.Contains(output, "loading") || strings.Contains(output, "blank") {
			analysis.WhyFailed = "Page stuck in loading state or rendering empty"
			analysis.FixSteps = []string{
				"Check API calls are resolving correctly",
				"Verify environment variables are passed to frontend",
				"Add error boundaries to catch and display errors",
			}
		}

	case "e2e":
		analysis.WhatFailed = "End-to-end tests failed"
		if strings.Contains(output, "timeout") {
			analysis.WhyFailed = "Test timed out waiting for element or action"
			analysis.FixSteps = []string{
				"Increase test timeouts if operations are legitimately slow",
				"Check that UI elements have correct selectors",
				"Ensure the page loads fully before interacting",
			}
		} else if strings.Contains(output, "expected") && strings.Contains(output, "received") {
			analysis.WhyFailed = "Test assertion failed - actual value doesn't match expected"
			analysis.FixSteps = []string{
				"Check the test expectation is correct",
				"Verify the feature implementation matches requirements",
				"Update test if the requirement changed",
			}
		}

	case "typecheck", "typecheck_fullstack":
		analysis.WhatFailed = "Type checking failed"
		analysis.WhyFailed = "TypeScript or MyPy found type errors"
		analysis.FixSteps = []string{
			"Fix the type annotations in the code",
			"Add missing type hints",
			"Ensure return types match actual returns",
		}

	case "lint", "lint_fullstack":
		analysis.WhatFailed = "Linting failed"
		analysis.WhyFailed = "Code style or quality issues detected"
		analysis.FixSteps = []string{
			"Run the formatter: npm run format / black .",
			"Fix any remaining linting errors",
			"Check for unused imports and variables",
		}
	}

	return analysis
}

// extractKeyError extracts the most relevant error message from output.
func extractKeyError(output string) string {
	lines := strings.Split(output, "\n")

	// Look for common error patterns
	errorPatterns := []string{
		"Error:", "ERROR:", "error:",
		"Exception:", "exception:",
		"ModuleNotFoundError:",
		"ImportError:",
		"TypeError:",
		"SyntaxError:",
		"Failed to compile",
		"ENOENT:",
		"Cannot find module",
	}

	var errorLines []string
	for i, line := range lines {
		for _, pattern := range errorPatterns {
			if strings.Contains(line, pattern) {
				// Include this line and up to 3 following lines for context
				end := i + 4
				if end > len(lines) {
					end = len(lines)
				}
				for j := i; j < end; j++ {
					if strings.TrimSpace(lines[j]) != "" {
						errorLines = append(errorLines, strings.TrimSpace(lines[j]))
					}
				}
				break
			}
		}
		if len(errorLines) > 0 {
			break
		}
	}

	if len(errorLines) > 0 {
		return strings.Join(errorLines, "\n")
	}

	// Return last non-empty lines if no error pattern found
	var lastLines []string
	for i := len(lines) - 1; i >= 0 && len(lastLines) < 5; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			lastLines = append([]string{strings.TrimSpace(lines[i])}, lastLines...)
		}
	}
	return strings.Join(lastLines, "\n")
}
