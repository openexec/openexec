// Package quality provides automatic lint/test gates for OpenExec.
// It ensures code quality by running checks before execution and can block or warn on failures.
package quality

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GateType represents the type of quality gate.
type GateType string

const (
	// GateTypeLint runs linting checks.
	GateTypeLint GateType = "lint"
	// GateTypeTest runs test suites.
	GateTypeTest GateType = "test"
	// GateTypeFormat checks code formatting.
	GateTypeFormat GateType = "format"
	// GateTypeSecurity runs security scans.
	GateTypeSecurity GateType = "security"
	// GateTypeCustom runs custom commands.
	GateTypeCustom GateType = "custom"
)

// GateMode determines how failures are handled.
type GateMode string

const (
	// GateModeBlock prevents execution on failure.
	GateModeBlock GateMode = "block"
	// GateModeWarn allows execution with a warning.
	GateModeWarn GateMode = "warn"
	// GateModeIgnore silently ignores failures.
	GateModeIgnore GateMode = "ignore"
)

// Gate represents a single quality gate.
type Gate struct {
	Name        string            `json:"name"`
	Type        GateType          `json:"type"`
	Command     string            `json:"command"`
	Args        []string          `json:"args"`
	Mode        GateMode          `json:"mode"`
	Timeout     time.Duration     `json:"timeout"`
	AutoFix     bool              `json:"auto_fix"`
	FixCommand  string            `json:"fix_command,omitempty"`
	FixArgs     []string          `json:"fix_args,omitempty"`
	Patterns    []string          `json:"patterns,omitempty"` // File patterns to check
	Env         map[string]string `json:"env,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty"`
}

// GateResult represents the result of running a gate.
type GateResult struct {
	GateName    string        `json:"gate_name"`
	Passed      bool          `json:"passed"`
	Output      string        `json:"output"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	Fixed       bool          `json:"fixed"`
	FixOutput   string        `json:"fix_output,omitempty"`
}

// GateSummary represents the summary of all gate results.
type GateSummary struct {
	Results      []GateResult  `json:"results"`
	TotalGates   int           `json:"total_gates"`
	PassedGates  int           `json:"passed_gates"`
	FailedGates  int           `json:"failed_gates"`
	Blocked      bool          `json:"blocked"`
	TotalDuration time.Duration `json:"total_duration"`
}

// Manager manages quality gates.
type Manager struct {
	gates      []Gate
	workingDir string
}

// ManagerConfig contains configuration for the quality manager.
type ManagerConfig struct {
	WorkingDir string `json:"working_dir"`
}

// DefaultGates returns the default set of quality gates for common project types.
func DefaultGates(projectType string) []Gate {
	switch projectType {
	case "go":
		return []Gate{
			{
				Name:    "go-vet",
				Type:    GateTypeLint,
				Command: "go",
				Args:    []string{"vet", "./..."},
				Mode:    GateModeBlock,
				Timeout: 2 * time.Minute,
			},
			{
				Name:    "go-test",
				Type:    GateTypeTest,
				Command: "go",
				Args:    []string{"test", "./...", "-short"},
				Mode:    GateModeBlock,
				Timeout: 5 * time.Minute,
			},
			{
				Name:    "gofmt",
				Type:    GateTypeFormat,
				Command: "gofmt",
				Args:    []string{"-l", "."},
				Mode:    GateModeWarn,
				Timeout: 30 * time.Second,
				AutoFix: true,
				FixCommand: "gofmt",
				FixArgs: []string{"-w", "."},
			},
			{
				Name:    "golangci-lint",
				Type:    GateTypeLint,
				Command: "golangci-lint",
				Args:    []string{"run", "--fast"},
				Mode:    GateModeWarn,
				Timeout: 3 * time.Minute,
			},
		}
	case "python":
		return []Gate{
			{
				Name:    "flake8",
				Type:    GateTypeLint,
				Command: "flake8",
				Args:    []string{"."},
				Mode:    GateModeBlock,
				Timeout: 2 * time.Minute,
			},
			{
				Name:    "pytest",
				Type:    GateTypeTest,
				Command: "pytest",
				Args:    []string{"-xvs"},
				Mode:    GateModeBlock,
				Timeout: 5 * time.Minute,
			},
			{
				Name:    "black",
				Type:    GateTypeFormat,
				Command: "black",
				Args:    []string{"--check", "."},
				Mode:    GateModeWarn,
				Timeout: 1 * time.Minute,
				AutoFix: true,
				FixCommand: "black",
				FixArgs: []string{"."},
			},
			{
				Name:    "mypy",
				Type:    GateTypeLint,
				Command: "mypy",
				Args:    []string{"."},
				Mode:    GateModeWarn,
				Timeout: 2 * time.Minute,
			},
		}
	case "typescript", "javascript":
		return []Gate{
			{
				Name:    "eslint",
				Type:    GateTypeLint,
				Command: "eslint",
				Args:    []string{".", "--ext", ".ts,.tsx,.js,.jsx"},
				Mode:    GateModeBlock,
				Timeout: 2 * time.Minute,
				AutoFix: true,
				FixCommand: "eslint",
				FixArgs: []string{".", "--fix", "--ext", ".ts,.tsx,.js,.jsx"},
			},
			{
				Name:    "jest",
				Type:    GateTypeTest,
				Command: "jest",
				Args:    []string{"--passWithNoTests"},
				Mode:    GateModeBlock,
				Timeout: 5 * time.Minute,
			},
			{
				Name:    "prettier",
				Type:    GateTypeFormat,
				Command: "prettier",
				Args:    []string{"--check", "."},
				Mode:    GateModeWarn,
				Timeout: 1 * time.Minute,
				AutoFix: true,
				FixCommand: "prettier",
				FixArgs: []string{"--write", "."},
			},
			{
				Name:    "tsc",
				Type:    GateTypeLint,
				Command: "tsc",
				Args:    []string{"--noEmit"},
				Mode:    GateModeBlock,
				Timeout: 2 * time.Minute,
			},
		}
	case "rust":
		return []Gate{
			{
				Name:    "cargo-check",
				Type:    GateTypeLint,
				Command: "cargo",
				Args:    []string{"check"},
				Mode:    GateModeBlock,
				Timeout: 3 * time.Minute,
			},
			{
				Name:    "cargo-test",
				Type:    GateTypeTest,
				Command: "cargo",
				Args:    []string{"test"},
				Mode:    GateModeBlock,
				Timeout: 5 * time.Minute,
			},
			{
				Name:    "cargo-clippy",
				Type:    GateTypeLint,
				Command: "cargo",
				Args:    []string{"clippy", "--", "-D", "warnings"},
				Mode:    GateModeWarn,
				Timeout: 3 * time.Minute,
				AutoFix: true,
				FixCommand: "cargo",
				FixArgs: []string{"clippy", "--fix", "--allow-dirty"},
			},
			{
				Name:    "cargo-fmt",
				Type:    GateTypeFormat,
				Command: "cargo",
				Args:    []string{"fmt", "--", "--check"},
				Mode:    GateModeWarn,
				Timeout: 30 * time.Second,
				AutoFix: true,
				FixCommand: "cargo",
				FixArgs: []string{"fmt"},
			},
		}
	default:
		return []Gate{}
	}
}

// NewManager creates a new quality gate manager.
func NewManager(workingDir string, gates []Gate) *Manager {
	return &Manager{
		gates:      gates,
		workingDir: workingDir,
	}
}

// RunAll runs all quality gates and returns a summary.
func (m *Manager) RunAll(ctx context.Context) (*GateSummary, error) {
	summary := &GateSummary{
		Results: make([]GateResult, 0, len(m.gates)),
		TotalGates: len(m.gates),
	}

	startTime := time.Now()

	for _, gate := range m.gates {
		result := m.runGate(ctx, gate)
		summary.Results = append(summary.Results, result)

		if result.Passed {
			summary.PassedGates++
		} else {
			summary.FailedGates++
			if gate.Mode == GateModeBlock {
				summary.Blocked = true
			}
		}
	}

	summary.TotalDuration = time.Since(startTime)

	return summary, nil
}

// RunGate runs a single quality gate by name.
func (m *Manager) RunGate(ctx context.Context, gateName string) (*GateResult, error) {
	for _, gate := range m.gates {
		if gate.Name == gateName {
			result := m.runGate(ctx, gate)
			return &result, nil
		}
	}
	return nil, fmt.Errorf("gate %s not found", gateName)
}

// runGate executes a single gate.
func (m *Manager) runGate(ctx context.Context, gate Gate) GateResult {
	result := GateResult{
		GateName: gate.Name,
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	// Check if command exists
	if _, err := exec.LookPath(gate.Command); err != nil {
		result.Passed = gate.Mode == GateModeIgnore
		result.Error = fmt.Sprintf("command not found: %s", gate.Command)
		return result
	}

	// Create command
	workingDir := gate.WorkingDir
	if workingDir == "" {
		workingDir = m.workingDir
	}

	cmdCtx, cancel := context.WithTimeout(ctx, gate.Timeout)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, gate.Command, gate.Args...)
	cmd.Dir = workingDir
	cmd.Env = m.buildEnv(gate.Env)

	// Run command
	output, err := cmd.CombinedOutput()
	result.Output = string(output)

	if err != nil {
		result.Passed = false
		result.Error = err.Error()

		// Try auto-fix if enabled
		if gate.AutoFix && gate.FixCommand != "" {
			fixed, fixOutput := m.tryAutoFix(ctx, gate)
			result.Fixed = fixed
			result.FixOutput = fixOutput
			if fixed {
				// Re-run the gate after fix
				cmd = exec.CommandContext(cmdCtx, gate.Command, gate.Args...)
				cmd.Dir = workingDir
				cmd.Env = m.buildEnv(gate.Env)
				output, err = cmd.CombinedOutput()
				result.Output = string(output)
				result.Passed = err == nil
				result.Error = ""
			}
		}
	} else {
		result.Passed = true
	}

	return result
}

// tryAutoFix attempts to auto-fix issues.
func (m *Manager) tryAutoFix(ctx context.Context, gate Gate) (bool, string) {
	if _, err := exec.LookPath(gate.FixCommand); err != nil {
		return false, fmt.Sprintf("fix command not found: %s", gate.FixCommand)
	}

	workingDir := gate.WorkingDir
	if workingDir == "" {
		workingDir = m.workingDir
	}

	fixCtx, cancel := context.WithTimeout(ctx, gate.Timeout)
	defer cancel()

	cmd := exec.CommandContext(fixCtx, gate.FixCommand, gate.FixArgs...)
	cmd.Dir = workingDir
	cmd.Env = m.buildEnv(gate.Env)

	output, err := cmd.CombinedOutput()
	return err == nil, string(output)
}

// buildEnv builds the environment for a command.
func (m *Manager) buildEnv(gateEnv map[string]string) []string {
	env := os.Environ()
	for key, value := range gateEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}
	return env
}

// AddGate adds a gate to the manager.
func (m *Manager) AddGate(gate Gate) {
	m.gates = append(m.gates, gate)
}

// RemoveGate removes a gate by name.
func (m *Manager) RemoveGate(gateName string) bool {
	for i, gate := range m.gates {
		if gate.Name == gateName {
			m.gates = append(m.gates[:i], m.gates[i+1:]...)
			return true
		}
	}
	return false
}

// GetGates returns all configured gates.
func (m *Manager) GetGates() []Gate {
	return append([]Gate{}, m.gates...)
}

// DetectProjectType attempts to detect the project type.
func DetectProjectType(projectDir string) string {
	// Check for Go
	if _, err := os.Stat(filepath.Join(projectDir, "go.mod")); err == nil {
		return "go"
	}

	// Check for Python
	if _, err := os.Stat(filepath.Join(projectDir, "requirements.txt")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(projectDir, "pyproject.toml")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(projectDir, "setup.py")); err == nil {
		return "python"
	}

	// Check for TypeScript/JavaScript
	if _, err := os.Stat(filepath.Join(projectDir, "package.json")); err == nil {
		// Check if TypeScript
		if _, err := os.Stat(filepath.Join(projectDir, "tsconfig.json")); err == nil {
			return "typescript"
		}
		return "javascript"
	}

	// Check for Rust
	if _, err := os.Stat(filepath.Join(projectDir, "Cargo.toml")); err == nil {
		return "rust"
	}

	return ""
}

// FormatSummary formats a gate summary for display.
func FormatSummary(summary *GateSummary) string {
	var sb strings.Builder

	sb.WriteString("\n╔════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║           Quality Gate Results                         ║\n")
	sb.WriteString("╠════════════════════════════════════════════════════════╣\n")

	for _, result := range summary.Results {
		status := "✓ PASS"
		if !result.Passed {
			status = "✗ FAIL"
		}

		sb.WriteString(fmt.Sprintf("║ %-20s %s (%v)\n", result.GateName+":", status, result.Duration.Round(time.Millisecond)))

		if !result.Passed && result.Error != "" {
			sb.WriteString(fmt.Sprintf("║   Error: %s\n", truncate(result.Error, 50)))
		}

		if result.Fixed {
			sb.WriteString(fmt.Sprintf("║   ⚡ Auto-fixed\n"))
		}
	}

	sb.WriteString("╠════════════════════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║ Total: %d | Passed: %d | Failed: %d | Duration: %v\n",
		summary.TotalGates, summary.PassedGates, summary.FailedGates, summary.TotalDuration.Round(time.Millisecond)))

	if summary.Blocked {
		sb.WriteString("║ ⚠️  EXECUTION BLOCKED - Fix failing gates to continue\n")
	}

	sb.WriteString("╚════════════════════════════════════════════════════════╝\n")

	return sb.String()
}

// truncate truncates a string to max length.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
