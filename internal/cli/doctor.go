package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/runner"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check your environment for common issues",
	Long:  `Check if required runner CLIs are installed and authenticated, and verify project configuration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoctor(cmd)
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
			}
		}
	}

	cmd.Println()
	if issues > 0 {
		return fmt.Errorf("doctor found %d issue(s)", issues)
	}
	
	cmd.Println(color.GreenString("✨ Your environment looks healthy!"))
	return nil
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
