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
	Use:   "status",
	Short: "Show execution engine and run status",
	Long: `Display the current status of the OpenExec execution engine.

Shows:
  - Daemon status (running/stopped, PID, port)
  - Active runs and their progress
  - Recent completed runs

Examples:
  openexec status           # Show full status
  openexec status --json    # Output as JSON`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
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

		// Output format
		jsonOutput, _ := cmd.Flags().GetBool("json")

		if jsonOutput {
			return outputStatusJSON(cmd, daemonRunning, pid, port, config)
		}

		return outputStatusText(cmd, daemonRunning, pid, port, config)
	},
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
		cmd.Printf("  - %s: %s (%s)\n", r.ID, r.Status, r.Phase)
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
			case "completed", "done":
				statusIcon = "✓"
			case "failed", "error":
				statusIcon = "✗"
			case "cancelled":
				statusIcon = "○"
			}
			cmd.Printf("  %s %s: %s\n", statusIcon, r.ID, r.Status)
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
	Status    string `json:"status"`
	Phase     string `json:"phase,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
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

func init() {
	statusCmd.Flags().Bool("json", false, "Output as JSON")
	rootCmd.AddCommand(statusCmd)
}
