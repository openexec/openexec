package gates

import (
	"fmt"
	"os/exec"
	"strings"
)

// PreflightCheck represents a single preflight check.
type PreflightCheck struct {
	Name        string `json:"name"`
	Passed      bool   `json:"passed"`
	Error       string `json:"error,omitempty"`
	FixCommand  string `json:"fix_command,omitempty"`
	Description string `json:"description"`
}

// PreflightReport holds results from all preflight checks.
type PreflightReport struct {
	Passed  bool             `json:"passed"`
	Checks  []PreflightCheck `json:"checks"`
	Summary string           `json:"summary"`
}

// Internal check functions, can be overridden for testing
var (
	dockerCheckFn    = checkDocker
	nodeCheckCheckFn = checkNode
	pythonCheckFn    = checkPython
)

// RunPreflightChecks runs preflight checks based on task requirements.
func RunPreflightChecks(taskTitle string, gateNames []string) *PreflightReport {
	report := &PreflightReport{
		Passed: true,
		Checks: []PreflightCheck{},
	}

	// Determine which checks to run based on task and gates
	needsDocker := containsAnyInSlice(gateNames, "services_start", "services_respond", "frontend_renders", "e2e") ||
		containsAny(strings.ToLower(taskTitle), "docker", "container", "compose")

	needsNode := containsAnyInSlice(gateNames, "deps_install", "frontend_renders", "e2e") ||
		containsAny(strings.ToLower(taskTitle), "npm", "node", "frontend", "next", "react")

	needsPython := containsAny(strings.ToLower(taskTitle), "python", "pip", "fastapi", "backend")

	// Run relevant checks
	if needsDocker {
		check := dockerCheckFn()
		report.Checks = append(report.Checks, check)
		if !check.Passed {
			report.Passed = false
		}
	}

	if needsNode {
		check := nodeCheckCheckFn()
		report.Checks = append(report.Checks, check)
		if !check.Passed {
			report.Passed = false
		}
	}

	if needsPython {
		check := pythonCheckFn()
		report.Checks = append(report.Checks, check)
		if !check.Passed {
			report.Passed = false
		}
	}

	// Generate summary
	if report.Passed {
		report.Summary = fmt.Sprintf("✓ All %d preflight checks passed", len(report.Checks))
	} else {
		failed := []string{}
		for _, c := range report.Checks {
			if !c.Passed {
				failed = append(failed, c.Name)
			}
		}
		report.Summary = fmt.Sprintf("✗ Preflight failed: %s", strings.Join(failed, ", "))
	}

	return report
}

// checkDocker verifies Docker is installed and running.
func checkDocker() PreflightCheck {
	check := PreflightCheck{
		Name:        "docker",
		Description: "Docker is installed and running",
	}

	// Check if docker command exists
	_, err := exec.LookPath("docker")
	if err != nil {
		check.Passed = false
		check.Error = "Docker is not installed"
		check.FixCommand = "Install Docker Desktop from https://docker.com/products/docker-desktop"
		return check
	}

	// Check if docker daemon is running
	cmd := exec.Command("docker", "info")
	output, err := cmd.CombinedOutput()
	if err != nil {
		check.Passed = false
		outputStr := string(output)
		if strings.Contains(outputStr, "Cannot connect") || strings.Contains(outputStr, "connection refused") {
			check.Error = "Docker daemon is not running"
			check.FixCommand = "Start Docker Desktop or run: sudo systemctl start docker"
		} else {
			check.Error = fmt.Sprintf("Docker error: %s", strings.TrimSpace(outputStr))
			check.FixCommand = "Check Docker installation and permissions"
		}
		return check
	}

	// Check docker compose
	cmd = exec.Command("docker", "compose", "version")
	if err := cmd.Run(); err != nil {
		// Try docker-compose (v1)
		_, err := exec.LookPath("docker-compose")
		if err != nil {
			check.Passed = false
			check.Error = "Docker Compose is not installed"
			check.FixCommand = "Docker Compose should be included with Docker Desktop. Update Docker or install separately."
			return check
		}
	}

	check.Passed = true
	return check
}

// checkNode verifies Node.js is installed.
func checkNode() PreflightCheck {
	check := PreflightCheck{
		Name:        "node",
		Description: "Node.js is installed",
	}

	// Check node
	cmd := exec.Command("node", "--version")
	output, err := cmd.Output()
	if err != nil {
		check.Passed = false
		check.Error = "Node.js is not installed"
		check.FixCommand = "Install Node.js from https://nodejs.org or use nvm"
		return check
	}

	version := strings.TrimSpace(string(output))
	check.Description = fmt.Sprintf("Node.js %s installed", version)

	// Check npm
	cmd = exec.Command("npm", "--version")
	if err := cmd.Run(); err != nil {
		check.Passed = false
		check.Error = "npm is not installed"
		check.FixCommand = "npm should come with Node.js. Reinstall Node.js."
		return check
	}

	check.Passed = true
	return check
}

// checkPython verifies Python is installed.
func checkPython() PreflightCheck {
	check := PreflightCheck{
		Name:        "python",
		Description: "Python is installed",
	}

	// Try python3 first, then python
	var version string
	cmd := exec.Command("python3", "--version")
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.Command("python", "--version")
		output, err = cmd.Output()
		if err != nil {
			check.Passed = false
			check.Error = "Python is not installed"
			check.FixCommand = "Install Python from https://python.org or use pyenv"
			return check
		}
	}

	version = strings.TrimSpace(string(output))
	check.Description = fmt.Sprintf("%s installed", version)

	// Check pip
	cmd = exec.Command("pip3", "--version")
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("pip", "--version")
		if err := cmd.Run(); err != nil {
			check.Passed = false
			check.Error = "pip is not installed"
			check.FixCommand = "Install pip: python -m ensurepip --upgrade"
			return check
		}
	}

	check.Passed = true
	return check
}

// FormatPreflightReport formats the preflight report for display.
func FormatPreflightReport(report *PreflightReport) string {
	if report.Passed {
		return report.Summary
	}

	var sb strings.Builder
	sb.WriteString("## Preflight Checks FAILED\n\n")
	sb.WriteString("The following prerequisites are not met:\n\n")

	for _, check := range report.Checks {
		if !check.Passed {
			sb.WriteString(fmt.Sprintf("### ❌ %s\n", check.Name))
			sb.WriteString(fmt.Sprintf("**Error**: %s\n", check.Error))
			if check.FixCommand != "" {
				sb.WriteString(fmt.Sprintf("**Fix**: %s\n", check.FixCommand))
			}
			sb.WriteString("\n")
		}
	}

	sb.WriteString("Please fix these issues before the task can proceed.\n")

	return sb.String()
}

// containsAny checks if s contains any of the substrings.
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// containsAnyInSlice checks if slice contains any of the values.
func containsAnyInSlice(slice []string, values ...string) bool {
	for _, s := range slice {
		for _, v := range values {
			if s == v {
				return true
			}
		}
	}
	return false
}
