package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/runner"
	"github.com/spf13/cobra"
)

var (
	doctorAPIBase  string
	doctorIntentFix bool
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check your environment for common issues",
	Long:  `Check if required runner CLIs are installed and authenticated, and verify project configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(cmd)
	},
}

var doctorIntentCmd = &cobra.Command{
	Use:   "intent",
	Short: "Validate and optionally fix INTENT.md",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Just call plan validation logic
		return GenerateAndSave(cmd, "INTENT.md", ".")
	},
}

func runDoctor(cmd *cobra.Command) error {
	cmd.Println(color.CyanString("=== OpenExec Doctor: Checking Health ==="))
	
	issues := 0

	// 1. Check Project Config
	config, err := project.LoadProjectConfig(".")
	if err != nil {
		cmd.Printf("  %s Project not initialized: %v\n", color.RedString("✗"), err)
		cmd.Println("     Hint: Run 'openexec init'")
		issues++
	} else {
		cmd.Printf("  %s Project config valid (%s)\n", color.GreenString("✓"), config.Name)
	}

	// 2. Check Intent File
	intentFile := "INTENT.md"
	if _, err := os.Stat(intentFile); err != nil {
		cmd.Printf("  %s INTENT.md not found\n", color.YellowString("!"))
		cmd.Println("     Hint: Create INTENT.md or run 'openexec wizard'")
	} else {
		cmd.Printf("  %s INTENT.md present\n", color.GreenString("✓"))
	}

	// 3. Resolve and Check Runner
	if config != nil {
		rCmd, _, err := runner.Resolve(
			config.Execution.ExecutorModel,
			config.Execution.RunnerCommand,
			config.Execution.RunnerArgs,
		)
		if err != nil {
			cmd.Printf("  %s Runner resolution failed: %v\n", color.RedString("✗"), err)
			issues++
		} else {
			cmd.Printf("  %s Runner available: %s\n", color.GreenString("✓"), rCmd)
			
			// 4. Try basic runner version/auth check (Best effort)
			base := strings.ToLower(rCmd)
			if strings.Contains(base, "claude") {
				check := exec.Command(rCmd, "--version")
				if err := check.Run(); err != nil {
					cmd.Printf("  %s Claude CLI found but failed to execute. May need authentication.\n", color.YellowString("!"))
					cmd.Println("     Hint: Run 'claude login'")
				}
			} else if strings.Contains(base, "gemini") {
				cmd.Printf("  %s Gemini CLI found. If authentication fails later, run 'gemini auth login' or visit https://ai.google.dev/gemini-api/docs/api-key\n", color.YellowString("!"))
			} else if strings.Contains(base, "codex") {
				cmd.Printf("  %s Codex CLI found. Ensure you have set CODEX_API_KEY in your environment.\n", color.YellowString("!"))
			}
		}
	}

	// 5. Check Execution API Health (if flag provided)
	if doctorAPIBase != "" {
		cmd.Println()
		cmd.Println(color.CyanString("=== Checking Execution API Health ==="))
		client := &http.Client{Timeout: 5 * time.Second}
		healthURL := strings.TrimRight(doctorAPIBase, "/") + "/api/health"
		resp, err := client.Get(healthURL)
		if err != nil {
			cmd.Printf("  %s API unreachable: %v\n", color.RedString("✗"), err)
			issues++
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				cmd.Printf("  %s API returned status %d\n", color.RedString("✗"), resp.StatusCode)
				issues++
			} else {
				var h struct {
					Status string `json:"status"`
					Runner any    `json:"runner"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&h); err == nil {
					cmd.Printf("  %s api_health: OK\n", color.GreenString("[PASS]"))
					if h.Runner != nil {
						rData, _ := json.Marshal(h.Runner)
						cmd.Printf("     Resolved Runner: %s\n", string(rData))
					}
				}
			}
		}
	}

	cmd.Println()
	if issues > 0 {
		cmd.Printf("%s Doctor found %d issue(s). Resolve them to ensure stable execution.\n", color.RedString("✗"), issues)
		return fmt.Errorf("doctor check failed")
	}
	
	cmd.Println(color.GreenString("✨ Your environment looks healthy!"))
	return nil
}

func init() {
	doctorCmd.Flags().StringVar(&doctorAPIBase, "api", "", "Check remote execution API health (e.g. http://localhost:8765)")
	
	doctorIntentCmd.Flags().BoolVar(&doctorIntentFix, "fix", false, "Scaffold missing sections in INTENT.md")
	doctorCmd.AddCommand(doctorIntentCmd)
	
	rootCmd.AddCommand(doctorCmd)
}
