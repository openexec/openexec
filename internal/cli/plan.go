package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/runner"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/spf13/cobra"
)

var (
	planValidateOnly bool
	planNoValidate   bool
	planFix          bool
	planExport       bool
)

var planCmd = &cobra.Command{
	Use:   "plan [intent-file]",
	Short: "Advanced: Generate a project plan manually (Deprecated: use 'openexec run' instead)",
	Long: `Generate a project plan from an intent document. 
Note: 'openexec run' now performs this step automatically if your plan is missing or stale.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		intentFile := "INTENT.md"
		if len(args) > 0 {
			intentFile = args[0]
		}

		// Show deprecation notice
		cmd.Println(color.New(color.FgYellow).Sprint("💡 Note: 'openexec plan' is now an advanced command. 'openexec run' handles auto-planning by default."))

		// 1. Initialize Manager
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		sStore, err := state.NewStore(filepath.Join(config.ProjectDir, ".openexec", "openexec.db"))
		if err != nil {
			return err
		}
		defer sStore.Close()

		mgr, err := manager.New(manager.Config{
			WorkDir:    config.ProjectDir,
			StateStore: sStore,
		})
		if err != nil {
			return err
		}

		// 2. Execute Plan
		cmd.Printf("Generating plan from: %s\n", intentFile)
		cmd.Println("  Planning...")
		
		res, err := mgr.Plan(cmd.Context(), manager.PlanRequest{
			IntentFile: intentFile,
			NoValidate: planNoValidate,
			AutoImport: true, // Always import to SQLite
		})
		if err != nil {
			return err
		}

		if !res.Valid {
			cmd.Println(color.RedString("\nPLANNING GATE FAILED:"))
			for _, issue := range res.Issues {
				cmd.Printf("  - %s\n", issue)
			}
			return fmt.Errorf("intent validation failed")
		}

		cmd.Printf("✓ Stories generated and imported to SQLite (%d stories)\n", len(res.Plan.Stories))

		// 3. Optional Export
		if planExport {
			storiesPath := filepath.Join(config.ProjectDir, ".openexec", "stories.json")
			if err := mgr.ExportJSON(filepath.Dir(storiesPath)); err != nil {
				return fmt.Errorf("export failed: %w", err)
			}
			cmd.Printf("✓ Exported to %s\n", storiesPath)
		}

		return nil
	},
}

type cliLLMProvider struct {
	model string
	cmd   *cobra.Command
}

func (p *cliLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	cliCmd, cmdArgs, err := runner.Resolve(
		p.model,
		os.Getenv("OPENEXEC_PLANNER_CLI"),
		strings.Fields(os.Getenv("OPENEXEC_PLANNER_ARGS")),
	)
	if err != nil {
		return "", err
	}

	if strings.Contains(strings.ToLower(cliCmd), "claude") {
		cmdArgs = []string{"--print"}
	}

	c := exec.CommandContext(ctx, cliCmd, cmdArgs...)
	c.Stdin = strings.NewReader(prompt)

	output, err := c.CombinedOutput()
	if err != nil {
		outStr := string(output)
		if strings.Contains(outStr, "authentication_error") || strings.Contains(outStr, "OAuth token has expired") {
			return "", fmt.Errorf("\n❌ AI Provider Authentication Failed. Please run: %s login", cliCmd)
		}
		return "", fmt.Errorf("native LLM provider failed: %w\nOutput: %s", err, outStr)
	}

	return string(output), nil
}

func init() {
	planCmd.Flags().BoolVar(&planValidateOnly, "validate-only", false, "Validate and exit without planning")
	planCmd.Flags().BoolVar(&planNoValidate, "no-validate", false, "Skip validation entirely")
	planCmd.Flags().BoolVar(&planFix, "fix", false, "Show stubs for missing sections")
	planCmd.Flags().BoolVar(&planExport, "export", false, "Export generated plan to .openexec/stories.json")
	rootCmd.AddCommand(planCmd)
}
