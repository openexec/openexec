package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [run-id]",
	Short: "Show execution engine and run status",
	Long: `Display the current status of the OpenExec execution engine.

If [run-id] is provided, shows detailed status for that specific run.
Otherwise, shows general engine status and active runs.

Shows:
  - Daemon status (running/stopped, PID, port)
  - Active runs and their progress
  - Recent completed runs

Examples:
  openexec status           # Show full status
  openexec status run_123   # Show status for specific run
  openexec status --watch   # Watch status in real-time
  openexec status --json    # Output as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Watch mode
		watch, _ := cmd.Flags().GetBool("watch")
		if watch {
			return runStatusWatch(cmd, config)
		}

		// Check daemon status
		pid, port, pidErr := readPID(config.ProjectDir)
		daemonRunning := false

		if pidErr == nil {
			// Check if process is alive
			process, err := os.FindProcess(pid)
			if err == nil && process.Signal(syscall.Signal(0)) == nil {
				daemonRunning = true
			}
		}

		// Handle specific run ID if provided
		if len(args) > 0 {
			runID := args[0]
			if !daemonRunning {
				return fmt.Errorf("engine not running: cannot fetch details for run %s", runID)
			}
			return showRunDetail(cmd, port, runID)
		}

		// Output format
		jsonOutput, _ := cmd.Flags().GetBool("json")

		if jsonOutput {
			return outputStatusJSON(cmd, daemonRunning, pid, port, config)
		}

		return outputStatusText(cmd, daemonRunning, pid, port, config)
	},
}

func showRunDetail(cmd *cobra.Command, port int, runID string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/runs/%s", port, runID))
	if err != nil {
		return fmt.Errorf("failed to fetch run details: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("run %s not found", runID)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		cmd.Println(string(body))
		return nil
	}

	var run struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Phase     string `json:"phase"`
		Blueprint string `json:"blueprint_id"`
		Task      string `json:"task_description"`
		Iteration int    `json:"iteration"`
		Error     string `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &run); err != nil {
		return fmt.Errorf("failed to parse run details: %w", err)
	}

	cmd.Printf("Run Details: %s\n", run.ID)
	cmd.Println("========================================")
	cmd.Printf("Status:    %s\n", run.Status)
	cmd.Printf("Phase:     %s\n", run.Phase)
	cmd.Printf("Blueprint: %s\n", run.Blueprint)
	cmd.Printf("Iteration: %d\n", run.Iteration)
	cmd.Printf("Task:      %s\n", run.Task)
	if run.Error != "" {
		cmd.Printf("Error:     %s\n", run.Error)
	}

	return nil
}

func outputStatusText(cmd *cobra.Command, daemonRunning bool, pid, port int, config *project.ProjectConfig) error {
	cmd.Println("OpenExec Status")
	cmd.Println("===============")
	cmd.Println()

	// Daemon status
	if daemonRunning {
		cmd.Printf("Daemon:  running (PID %d, port %d)\n", pid, port)
	} else {
		cmd.Println("Daemon:  stopped")
		cmd.Println()
		cmd.Println("Start the daemon with: openexec start --daemon")
		return nil
	}

	cmd.Println()

	// Fetch runs from API
	runs, err := fetchRuns(port)
	if err != nil {
		cmd.Printf("API:     error fetching runs: %v\n", err)
		return nil
	}

	// Active runs
	var activeRuns []runInfo
	var completedRuns []runInfo
	for _, r := range runs {
		switch r.Status {
		case "running", "pending", "starting":
			activeRuns = append(activeRuns, r)
		default:
			completedRuns = append(completedRuns, r)
		}
	}

	cmd.Printf("Active:  %d run(s)\n", len(activeRuns))
	for _, r := range activeRuns {
		cmd.Printf("  - %s: %s (%s)\n", r.effectiveID(), r.Status, r.effectiveStage())
	}

	cmd.Println()

	// Recent completed runs (last 5)
	cmd.Println("Recent Runs:")
	if len(completedRuns) == 0 {
		cmd.Println("  (none)")
	} else {
		limit := 5
		if len(completedRuns) < limit {
			limit = len(completedRuns)
		}
		for i := 0; i < limit; i++ {
			r := completedRuns[i]
			statusIcon := "?"
			switch r.Status {
			case "completed", "complete", "done":
				statusIcon = "✓"
			case "failed", "error":
				statusIcon = "✗"
			case "cancelled", "stopped":
				statusIcon = "○"
			}
			cmd.Printf("  %s %s: %s\n", statusIcon, r.effectiveID(), r.Status)
		}
	}

	cmd.Println()
	cmd.Printf("Web UI:  http://localhost:%d\n", port)
	cmd.Printf("API:     http://localhost:%d/api/v1/runs\n", port)

	return nil
}

func outputStatusJSON(cmd *cobra.Command, daemonRunning bool, pid, port int, config *project.ProjectConfig) error {
	status := map[string]interface{}{
		"daemon": map[string]interface{}{
			"running": daemonRunning,
			"pid":     pid,
			"port":    port,
		},
		"project": map[string]interface{}{
			"name": config.Name,
			"path": config.ProjectDir,
		},
	}

	if daemonRunning {
		runs, err := fetchRuns(port)
		if err == nil {
			status["runs"] = runs
		} else {
			status["error"] = err.Error()
		}
	}

	output, _ := json.MarshalIndent(status, "", "  ")
	cmd.Println(string(output))
	return nil
}

type runInfo struct {
	ID        string `json:"id"`
	FWUID     string `json:"fwu_id"`
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Phase     string `json:"phase,omitempty"`
	Stage     string `json:"stage,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

// effectiveID returns the best available ID for display.
func (r runInfo) effectiveID() string {
	if r.FWUID != "" {
		return r.FWUID
	}
	if r.RunID != "" {
		return r.RunID
	}
	return r.ID
}

// effectiveStage returns the best available stage/phase for display.
func (r runInfo) effectiveStage() string {
	if r.Stage != "" {
		return r.Stage
	}
	return r.Phase
}

func fetchRuns(port int) ([]runInfo, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/runs", port))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Try parsing as array first
	var runs []runInfo
	if err := json.Unmarshal(body, &runs); err != nil {
		// Try parsing as object with "runs" field
		var wrapper struct {
			Runs []runInfo `json:"runs"`
		}
		if err := json.Unmarshal(body, &wrapper); err != nil {
			return nil, fmt.Errorf("failed to parse runs: %w", err)
		}
		runs = wrapper.Runs
	}

	return runs, nil
}

func runStatusWatch(cmd *cobra.Command, config *project.ProjectConfig) error {
	for {
		// Clear screen
		cmd.Print("\033[H\033[2J")
		
		pid, port, pidErr := readPID(config.ProjectDir)
		daemonRunning := false
		if pidErr == nil {
			process, err := os.FindProcess(pid)
			if err == nil && process.Signal(syscall.Signal(0)) == nil {
				daemonRunning = true
			}
		}

		_ = outputStatusText(cmd, daemonRunning, pid, port, config)
		
		cmd.Println("\n(Press Ctrl+C to stop watching)")
		
		time.Sleep(2 * time.Second)
	}
}

func init() {
	statusCmd.Flags().Bool("json", false, "Output as JSON")
	statusCmd.Flags().BoolP("watch", "w", false, "Watch status in real-time")
	rootCmd.AddCommand(statusCmd)
}
