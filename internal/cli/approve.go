package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

// approveCmd represents the approve command group.
var approveCmd = &cobra.Command{
	Use:   "approve",
	Short: "Manage tool approval requests",
	Long: `Manage pending approval requests for tool execution.

In Task mode, certain operations require user approval before execution.
This command group allows you to view, approve, or reject pending requests.

Examples:
  openexec approve list              # List all pending approvals
  openexec approve list --all        # Include resolved approvals
  openexec approve yes <id>          # Approve a request
  openexec approve no <id> [reason]  # Reject a request`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// approveListCmd lists pending approval requests.
var approveListCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending approval requests",
	Long: `List all pending approval requests awaiting decision.

Shows details including:
  - Request ID
  - Tool name and description
  - Risk level
  - Time waiting

Examples:
  openexec approve list           # List pending only
  openexec approve list --all     # Include resolved
  openexec approve list --json    # Output as JSON
  openexec approve list --run-id run_123  # Filter by run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Check daemon status
		pid, port, pidErr := readPID(config.ProjectDir)
		if pidErr != nil {
			return fmt.Errorf("daemon not running: %w", pidErr)
		}

		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(syscall.Signal(0)) != nil {
			return fmt.Errorf("daemon not running (stale PID file)")
		}

		// Get flags
		showAll, _ := cmd.Flags().GetBool("all")
		jsonOutput, _ := cmd.Flags().GetBool("json")
		runIDFilter, _ := cmd.Flags().GetString("run-id")

		// Fetch approvals from API
		endpoint := fmt.Sprintf("http://localhost:%d/api/v1/approvals", port)
		if !showAll {
			endpoint += "?status=pending"
		}
		if runIDFilter != "" {
			if strings.Contains(endpoint, "?") {
				endpoint += "&run_id=" + runIDFilter
			} else {
				endpoint += "?run_id=" + runIDFilter
			}
		}

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(endpoint)
		if err != nil {
			return fmt.Errorf("failed to fetch approvals: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s", string(body))
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}

		if jsonOutput {
			cmd.Println(string(body))
			return nil
		}

		// Parse and display
		var approvals []approvalInfo
		if err := json.Unmarshal(body, &approvals); err != nil {
			// Try parsing as wrapper
			var wrapper struct {
				Approvals []approvalInfo `json:"approvals"`
			}
			if err := json.Unmarshal(body, &wrapper); err != nil {
				return fmt.Errorf("failed to parse approvals: %w", err)
			}
			approvals = wrapper.Approvals
		}

		if len(approvals) == 0 {
			cmd.Println("No pending approval requests.")
			return nil
		}

		cmd.Println("Pending Approval Requests")
		cmd.Println("=========================")
		cmd.Println()

		for _, a := range approvals {
			statusIcon := "?"
			switch a.Status {
			case "pending":
				statusIcon = "..."
			case "approved", "auto_approved":
				statusIcon = "[OK]"
			case "rejected":
				statusIcon = "[X]"
			case "expired":
				statusIcon = "[T]"
			case "cancelled":
				statusIcon = "[C]"
			}

			cmd.Printf("%s %s\n", statusIcon, a.ID)
			cmd.Printf("    Tool: %s\n", a.ToolName)
			if a.Description != "" {
				cmd.Printf("    Description: %s\n", a.Description)
			}
			cmd.Printf("    Risk: %s\n", a.RiskLevel)
			cmd.Printf("    Run: %s\n", a.RunID)
			if a.CreatedAt != "" {
				cmd.Printf("    Created: %s\n", a.CreatedAt)
			}
			cmd.Println()
		}

		if !showAll {
			cmd.Printf("\nTo approve: openexec approve yes <id>\n")
			cmd.Printf("To reject:  openexec approve no <id> [reason]\n")
		}

		return nil
	},
}

// approveYesCmd approves a pending request.
var approveYesCmd = &cobra.Command{
	Use:   "yes <id>",
	Short: "Approve a pending request",
	Long: `Approve a pending approval request, allowing the tool to execute.

Examples:
  openexec approve yes abc123
  openexec approve yes abc123 --note "Reviewed and looks safe"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		requestID := args[0]

		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Check daemon status
		pid, port, pidErr := readPID(config.ProjectDir)
		if pidErr != nil {
			return fmt.Errorf("daemon not running: %w", pidErr)
		}

		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(syscall.Signal(0)) != nil {
			return fmt.Errorf("daemon not running (stale PID file)")
		}

		note, _ := cmd.Flags().GetString("note")

		// Send approval to API
		endpoint := fmt.Sprintf("http://localhost:%d/api/v1/approvals/%s/approve", port, requestID)
		payload := map[string]string{
			"decided_by": "cli_user",
		}
		if note != "" {
			payload["reason"] = note
		}

		payloadJSON, _ := json.Marshal(payload)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(endpoint, "application/json", strings.NewReader(string(payloadJSON)))
		if err != nil {
			return fmt.Errorf("failed to send approval: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s", string(body))
		}

		cmd.Printf("Approved request: %s\n", requestID)
		return nil
	},
}

// approveNoCmd rejects a pending request.
var approveNoCmd = &cobra.Command{
	Use:   "no <id> [reason]",
	Short: "Reject a pending request",
	Long: `Reject a pending approval request, preventing the tool from executing.

Examples:
  openexec approve no abc123
  openexec approve no abc123 "Command looks dangerous"`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		requestID := args[0]
		reason := "Rejected by user"
		if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}

		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Check daemon status
		pid, port, pidErr := readPID(config.ProjectDir)
		if pidErr != nil {
			return fmt.Errorf("daemon not running: %w", pidErr)
		}

		process, err := os.FindProcess(pid)
		if err != nil || process.Signal(syscall.Signal(0)) != nil {
			return fmt.Errorf("daemon not running (stale PID file)")
		}

		// Send rejection to API
		endpoint := fmt.Sprintf("http://localhost:%d/api/v1/approvals/%s/reject", port, requestID)
		payload := map[string]string{
			"decided_by": "cli_user",
			"reason":     reason,
		}

		payloadJSON, _ := json.Marshal(payload)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(endpoint, "application/json", strings.NewReader(string(payloadJSON)))
		if err != nil {
			return fmt.Errorf("failed to send rejection: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("API error: %s", string(body))
		}

		cmd.Printf("Rejected request: %s (reason: %s)\n", requestID, reason)
		return nil
	},
}

// approvalInfo represents an approval request from the API.
type approvalInfo struct {
	ID          string `json:"id"`
	RunID       string `json:"run_id"`
	ToolName    string `json:"tool_name"`
	Description string `json:"description"`
	RiskLevel   string `json:"risk_level"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at,omitempty"`
	ResolvedAt  string `json:"resolved_at,omitempty"`
	ResolvedBy  string `json:"resolved_by,omitempty"`
}

func init() {
	// List command flags
	approveListCmd.Flags().Bool("all", false, "Include resolved approvals")
	approveListCmd.Flags().Bool("json", false, "Output as JSON")
	approveListCmd.Flags().String("run-id", "", "Filter by run ID")

	// Yes command flags
	approveYesCmd.Flags().String("note", "", "Optional note for the approval")

	// Add subcommands
	approveCmd.AddCommand(approveListCmd)
	approveCmd.AddCommand(approveYesCmd)
	approveCmd.AddCommand(approveNoCmd)

	// Add to root
	rootCmd.AddCommand(approveCmd)
}
