package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/intent"
	"github.com/spf13/cobra"
)

// CheckResult represents the outcome of a single check
type CheckResult struct {
	Name        string
	Status      string // "pass", "warn", "fail"
	Message     string
	Remediation string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor [directory]",
	Short: "Check system health and project configuration",
	Long: `Run diagnostic checks on OpenExec projects and system configuration.

This command validates:
- Project .openexec/ directory structure
- state.json and tasks.json file integrity
- Read/write permissions
- Execution API connectivity (if --api flag is set)

Examples:
  openexec doctor                    # Check current directory
  openexec doctor ./projects         # Check all projects in ./projects
  openexec doctor --api http://localhost:8080  # Also check execution API`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoctor,
}

var (
	executionAPIURL string
	verboseOutput   bool
)

var doctorIntentCmd = &cobra.Command{
	Use:   "intent [file]",
	Short: "Validate INTENT.md structure and content",
	Long: `Run diagnostic checks on an INTENT.md file to ensure it meets
the required structure for planning and orchestration.

This command validates:
- Document structure (title, required sections)
- Content minima (goals, requirements, constraints)
- Story/acceptance criteria quality
- Formatting consistency

Exit codes:
  0 - All critical checks passed (warnings allowed)
  1 - One or more critical checks failed

Examples:
  openexec doctor intent                    # Check ./INTENT.md
  openexec doctor intent path/to/INTENT.md  # Check specific file
  openexec doctor intent --json             # JSON output
  openexec doctor intent --fix              # Preview missing section stubs`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoctorIntent,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().StringVar(&executionAPIURL, "api", "", "Execution API URL to check (e.g., http://localhost:8080)")
	doctorCmd.Flags().BoolVarP(&verboseOutput, "verbose", "v", false, "Show detailed output for all checks")

	// Add intent subcommand
	doctorCmd.AddCommand(doctorIntentCmd)
	doctorIntentCmd.Flags().Bool("json", false, "Output as JSON")
	doctorIntentCmd.Flags().Bool("compact", false, "Compact checklist output")
	doctorIntentCmd.Flags().Bool("fix", false, "Preview missing section stubs that would be added")
	doctorIntentCmd.Flags().BoolP("verbose", "v", false, "Include info-level suggestions")
}

func runDoctor(cmd *cobra.Command, args []string) error {
	baseDir := "."
	if len(args) > 0 {
		baseDir = args[0]
	}

	cmd.Println("OpenExec Doctor")
	cmd.Println("===============")
	cmd.Println()

	var allResults []CheckResult
	passCount, warnCount, failCount := 0, 0, 0

	// Check base directory
	results := checkBaseDirectory(baseDir)
	allResults = append(allResults, results...)

	// Scan for projects
	projects := findProjects(baseDir)
	if len(projects) == 0 {
		cmd.Printf("No OpenExec projects found in %s\n", baseDir)
		cmd.Println()
		if executionAPIURL == "" {
			cmd.Println("To initialize a new project, run:")
			cmd.Println("  openexec init <project-name>")
			return nil
		}
	} else {
		cmd.Printf("Found %d project(s)\n\n", len(projects))
	}

	// Check each project
	for _, projectPath := range projects {

		projectName := filepath.Base(projectPath)
		cmd.Printf("Project: %s\n", projectName)
		cmd.Println(repeatChar('-', len("Project: ")+len(projectName)))

		projectResults := checkProject(projectPath)
		allResults = append(allResults, projectResults...)

		for _, r := range projectResults {
			printCheckResult(cmd, r)
			switch r.Status {
			case "pass":
				passCount++
			case "warn":
				warnCount++
			case "fail":
				failCount++
			}
		}
		cmd.Println()
	}

	// Check execution API if specified
	if executionAPIURL != "" {
		cmd.Println("Execution API")
		cmd.Println("-------------")

		apiResults := checkExecutionAPI(executionAPIURL)
		allResults = append(allResults, apiResults...)

		for _, r := range apiResults {
			printCheckResult(cmd, r)
			switch r.Status {
			case "pass":
				passCount++
			case "warn":
				warnCount++
			case "fail":
				failCount++
			}
		}
		cmd.Println()
	}

	// Print summary
	cmd.Println("Summary")
	cmd.Println("-------")
	cmd.Printf("  Passed:   %d\n", passCount)
	cmd.Printf("  Warnings: %d\n", warnCount)
	cmd.Printf("  Failed:   %d\n", failCount)
	cmd.Println()

	// Print remediation hints for failures
	hasRemediation := false
	for _, r := range allResults {
		if r.Status == "fail" && r.Remediation != "" {
			if !hasRemediation {
				cmd.Println("Remediation Steps")
				cmd.Println("-----------------")
				hasRemediation = true
			}
			cmd.Printf("  - %s: %s\n", r.Name, r.Remediation)
		}
	}

	if failCount > 0 {
		return fmt.Errorf("doctor found %d issue(s)", failCount)
	}

	cmd.Println("All checks passed!")
	return nil
}

func checkBaseDirectory(baseDir string) []CheckResult {
	var results []CheckResult

	info, err := os.Stat(baseDir)
	if err != nil {
		results = append(results, CheckResult{
			Name:        "base_directory",
			Status:      "fail",
			Message:     fmt.Sprintf("Cannot access directory: %v", err),
			Remediation: fmt.Sprintf("Ensure %s exists and is readable", baseDir),
		})
		return results
	}

	if !info.IsDir() {
		results = append(results, CheckResult{
			Name:        "base_directory",
			Status:      "fail",
			Message:     fmt.Sprintf("%s is not a directory", baseDir),
			Remediation: "Provide a directory path, not a file",
		})
	}

	return results
}

func findProjects(baseDir string) []string {
	var projects []string

	// Check if baseDir itself is a project
	if isProject(baseDir) {
		projects = append(projects, baseDir)
		return projects
	}

	// Scan subdirectories
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return projects
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(baseDir, entry.Name())
		if isProject(projectPath) {
			projects = append(projects, projectPath)
		}
	}

	return projects
}

func isProject(dir string) bool {
	// Check for .openexec directory
	openexecDir := filepath.Join(dir, ".openexec")
	if info, err := os.Stat(openexecDir); err == nil && info.IsDir() {
		return true
	}

	// Also check for .uaos directory (legacy)
	uaosDir := filepath.Join(dir, ".uaos")
	if info, err := os.Stat(uaosDir); err == nil && info.IsDir() {
		return true
	}

	return false
}

func checkProject(projectPath string) []CheckResult {
	var results []CheckResult

	// Determine which config directory exists
	openexecDir := filepath.Join(projectPath, ".openexec")
	uaosDir := filepath.Join(projectPath, ".uaos")

	var configDir string
	if _, err := os.Stat(openexecDir); err == nil {
		configDir = openexecDir
	} else if _, err := os.Stat(uaosDir); err == nil {
		configDir = uaosDir
	}

	// Check directory permissions
	results = append(results, checkDirectoryPermissions(configDir)...)

	// Check state.json
	results = append(results, checkStateFile(configDir)...)

	// Check tasks.json
	results = append(results, checkTasksFile(configDir)...)

	// Check for lock file (stale lock detection)
	results = append(results, checkLockFile(configDir)...)

	// Check for error log
	results = append(results, checkErrorLog(configDir)...)

	return results
}

func checkDirectoryPermissions(configDir string) []CheckResult {
	var results []CheckResult

	// Check read access
	if _, err := os.ReadDir(configDir); err != nil {
		results = append(results, CheckResult{
			Name:        "directory_read",
			Status:      "fail",
			Message:     fmt.Sprintf("Cannot read %s: %v", configDir, err),
			Remediation: fmt.Sprintf("Check permissions on %s", configDir),
		})
		return results
	}

	results = append(results, CheckResult{
		Name:    "directory_read",
		Status:  "pass",
		Message: "Directory readable",
	})

	// Check write access
	testFile := filepath.Join(configDir, ".doctor_test")
	f, err := os.Create(testFile)
	if err != nil {
		results = append(results, CheckResult{
			Name:        "directory_write",
			Status:      "fail",
			Message:     fmt.Sprintf("Cannot write to %s: %v", configDir, err),
			Remediation: fmt.Sprintf("Check write permissions on %s", configDir),
		})
	} else {
		_ = f.Close()
		_ = os.Remove(testFile)
		results = append(results, CheckResult{
			Name:    "directory_write",
			Status:  "pass",
			Message: "Directory writable",
		})
	}

	return results
}

func checkStateFile(configDir string) []CheckResult {
	var results []CheckResult

	stateFile := filepath.Join(configDir, "state.json")
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			results = append(results, CheckResult{
				Name:    "state_file",
				Status:  "warn",
				Message: "state.json not found (may be created on first run)",
			})
		} else {
			results = append(results, CheckResult{
				Name:        "state_file",
				Status:      "fail",
				Message:     fmt.Sprintf("Cannot read state.json: %v", err),
				Remediation: "Check file permissions or recreate state.json",
			})
		}
		return results
	}

	// Validate JSON structure
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		results = append(results, CheckResult{
			Name:        "state_file",
			Status:      "fail",
			Message:     fmt.Sprintf("Invalid JSON in state.json: %v", err),
			Remediation: "Fix or regenerate state.json",
		})
		return results
	}

	// Check for expected fields
	expectedFields := []string{"status", "phase"}
	missingFields := []string{}
	for _, field := range expectedFields {
		if _, ok := state[field]; !ok {
			missingFields = append(missingFields, field)
		}
	}

	if len(missingFields) > 0 {
		results = append(results, CheckResult{
			Name:    "state_file",
			Status:  "warn",
			Message: fmt.Sprintf("state.json missing fields: %v", missingFields),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "state_file",
			Status:  "pass",
			Message: fmt.Sprintf("state.json valid (status: %v)", state["status"]),
		})
	}

	return results
}

func checkTasksFile(configDir string) []CheckResult {
	var results []CheckResult

	tasksFile := filepath.Join(configDir, "tasks.json")
	data, err := os.ReadFile(tasksFile)
	if err != nil {
		if os.IsNotExist(err) {
			results = append(results, CheckResult{
				Name:    "tasks_file",
				Status:  "warn",
				Message: "tasks.json not found (may be created when tasks are added)",
			})
		} else {
			results = append(results, CheckResult{
				Name:        "tasks_file",
				Status:      "fail",
				Message:     fmt.Sprintf("Cannot read tasks.json: %v", err),
				Remediation: "Check file permissions or recreate tasks.json",
			})
		}
		return results
	}

	// Validate JSON structure
	var tasks struct {
		Tasks []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"tasks"`
	}

	if err := json.Unmarshal(data, &tasks); err != nil {
		results = append(results, CheckResult{
			Name:        "tasks_file",
			Status:      "fail",
			Message:     fmt.Sprintf("Invalid JSON in tasks.json: %v", err),
			Remediation: "Fix or regenerate tasks.json",
		})
		return results
	}

	// Count task states
	total := len(tasks.Tasks)
	completed := 0
	for _, t := range tasks.Tasks {
		if t.Status == "completed" || t.Status == "done" {
			completed++
		}
	}

	results = append(results, CheckResult{
		Name:    "tasks_file",
		Status:  "pass",
		Message: fmt.Sprintf("tasks.json valid (%d tasks, %d completed)", total, completed),
	})

	return results
}

func checkLockFile(configDir string) []CheckResult {
	var results []CheckResult

	lockFile := filepath.Join(configDir, ".lock")
	info, err := os.Stat(lockFile)
	if err != nil {
		if os.IsNotExist(err) {
			// No lock file is fine
			return results
		}
		return results
	}

	// Check if lock file is stale (older than 1 hour)
	if time.Since(info.ModTime()) > time.Hour {
		results = append(results, CheckResult{
			Name:        "lock_file",
			Status:      "warn",
			Message:     fmt.Sprintf("Stale lock file (last modified %s)", info.ModTime().Format(time.RFC3339)),
			Remediation: fmt.Sprintf("If no process is running, remove %s", lockFile),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "lock_file",
			Status:  "pass",
			Message: "Lock file present (process may be running)",
		})
	}

	return results
}

func checkErrorLog(configDir string) []CheckResult {
	var results []CheckResult

	errorLog := filepath.Join(configDir, "error.log")
	info, err := os.Stat(errorLog)
	if err != nil {
		if os.IsNotExist(err) {
			// No error log is good
			return results
		}
		return results
	}

	if info.Size() > 0 {
		results = append(results, CheckResult{
			Name:    "error_log",
			Status:  "warn",
			Message: fmt.Sprintf("Error log contains %d bytes", info.Size()),
		})
	}

	return results
}

func checkExecutionAPI(apiURL string) []CheckResult {
	var results []CheckResult

	// Check /health endpoint
	results = append(results, checkAPIEndpoint(apiURL, "/health", "health")...)

	// Check /api/v1/loops endpoint
	results = append(results, checkAPIEndpoint(apiURL, "/api/v1/loops", "loops")...)

	return results
}

func checkAPIEndpoint(baseURL, path, name string) []CheckResult {
	var results []CheckResult

	url := baseURL + path
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		results = append(results, CheckResult{
			Name:        fmt.Sprintf("api_%s", name),
			Status:      "fail",
			Message:     fmt.Sprintf("Invalid URL %s: %v", url, err),
			Remediation: "Check the --api URL format",
		})
		return results
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		results = append(results, CheckResult{
			Name:        fmt.Sprintf("api_%s", name),
			Status:      "fail",
			Message:     fmt.Sprintf("Cannot reach %s: %v", url, err),
			Remediation: fmt.Sprintf("Ensure the execution API is running at %s", baseURL),
		})
		return results
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		results = append(results, CheckResult{
			Name:    fmt.Sprintf("api_%s", name),
			Status:  "fail",
			Message: fmt.Sprintf("%s returned status %d", url, resp.StatusCode),
		})
		return results
	}

	// Try to parse response as JSON
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024)) // 1MB limit
	if err != nil {
		results = append(results, CheckResult{
			Name:    fmt.Sprintf("api_%s", name),
			Status:  "warn",
			Message: fmt.Sprintf("%s reachable but response unreadable", url),
		})
		return results
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		// Try as array
		var arr []interface{}
		if err := json.Unmarshal(body, &arr); err != nil {
			results = append(results, CheckResult{
				Name:    fmt.Sprintf("api_%s", name),
				Status:  "warn",
				Message: fmt.Sprintf("%s reachable but response is not JSON", url),
			})
			return results
		}
		results = append(results, CheckResult{
			Name:    fmt.Sprintf("api_%s", name),
			Status:  "pass",
			Message: fmt.Sprintf("%s OK (returned %d items)", url, len(arr)),
		})
		return results
	}

	// For health endpoint, check status field
	if name == "health" {
		if status, ok := data["status"].(string); ok {
			results = append(results, CheckResult{
				Name:    fmt.Sprintf("api_%s", name),
				Status:  "pass",
				Message: fmt.Sprintf("%s OK (status: %s)", url, status),
			})
			return results
		}
	}

	results = append(results, CheckResult{
		Name:    fmt.Sprintf("api_%s", name),
		Status:  "pass",
		Message: fmt.Sprintf("%s OK", url),
	})

	return results
}

func printCheckResult(cmd *cobra.Command, r CheckResult) {
	var symbol string
	switch r.Status {
	case "pass":
		symbol = "[PASS]"
	case "warn":
		symbol = "[WARN]"
	case "fail":
		symbol = "[FAIL]"
	}

	cmd.Printf("  %s %s: %s\n", symbol, r.Name, r.Message)

	if verboseOutput && r.Remediation != "" {
		cmd.Printf("         Hint: %s\n", r.Remediation)
	}
}

func repeatChar(c rune, n int) string {
	result := make([]rune, n)
	for i := range result {
		result[i] = c
	}
	return string(result)
}

func runDoctorIntent(cmd *cobra.Command, args []string) error {
	// Determine file path
	filePath := "INTENT.md"
	if len(args) > 0 {
		filePath = args[0]
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// Try common variations
		variations := []string{"INTENT.md", "intent.md", "docs/INTENT.md", "docs/intent.md"}
		found := false
		for _, v := range variations {
			if _, err := os.Stat(v); err == nil {
				filePath = v
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("INTENT.md not found; tried: %s", strings.Join(variations, ", "))
		}
	}

	// Run validation
	validator := intent.NewValidator(filePath)
	result, err := validator.Validate()
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Handle output format
	jsonOutput, _ := cmd.Flags().GetBool("json")
	compactOutput, _ := cmd.Flags().GetBool("compact")
	fixMode, _ := cmd.Flags().GetBool("fix")

	// Handle fix mode
	if fixMode {
		fixer := intent.NewFixer(result)
		cmd.Println(fixer.Preview())
		if result.Valid {
			cmd.Println("No critical sections missing.")
		} else {
			cmd.Println("To add these stubs, copy the content above into your INTENT.md")
		}
		return nil
	}

	// Generate report
	reporter := intent.NewReporter(result)
	if jsonOutput {
		reporter.SetFormat(intent.ReportFormatJSON)
	} else if compactOutput {
		reporter.SetFormat(intent.ReportFormatCompact)
	}

	cmd.Println(reporter.Generate())

	// Exit with error if not valid
	if !result.Valid {
		return fmt.Errorf("validation failed with %d critical issue(s)", len(result.Critical))
	}

	return nil
}
