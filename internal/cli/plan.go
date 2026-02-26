package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/openexec/openexec/internal/intent"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan <intent-file>",
	Short: "Generate project plan from intent document",
	Long: `Generate a project plan from an intent document.

This command reads an intent.md file and invokes the orchestration engine
to generate a Goal Tree and Functional Work Units (FWUs) that define
the execution plan for the project.

The generated plan is stored in the Tract store for reference and execution.

By default, INTENT.md is validated before planning. Critical issues will
block planning; warnings are reported but allowed.

Validation flags:
  --validate-only  Validate and exit without planning (exit 1 on critical failures)
  --no-validate    Skip validation entirely
  --fix            With --validate-only, show stubs for missing sections

Examples:
  openexec plan INTENT.md                        # Validate then plan
  openexec plan INTENT.md --validate-only        # Validate only
  openexec plan INTENT.md --validate-only --fix  # Show missing section stubs
  openexec plan INTENT.md --no-validate          # Skip validation`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		intentFile := args[0]

		// Validate intent file exists
		if _, err := os.Stat(intentFile); err != nil {
			return fmt.Errorf("intent file not found: %s", intentFile)
		}

		// Get flags
		validateOnly, _ := cmd.Flags().GetBool("validate-only")
		noValidate, _ := cmd.Flags().GetBool("no-validate")
		fixMode, _ := cmd.Flags().GetBool("fix")

		// Run validation unless explicitly skipped
		if !noValidate {
			validator := intent.NewValidator(intentFile)
			result, err := validator.Validate()
			if err != nil {
				return fmt.Errorf("validation error: %w", err)
			}

			// Handle fix mode
			if fixMode && validateOnly {
				fixer := intent.NewFixer(result)
				fmt.Println(fixer.Preview())
				if !result.Valid {
					return fmt.Errorf("validation failed with %d critical issue(s)", len(result.Critical))
				}
				return nil
			}

			// Show validation result
			if validateOnly || !result.Valid {
				reporter := intent.NewReporter(result)
				fmt.Println(reporter.Generate())
			}

			// If validate-only, exit here
			if validateOnly {
				if !result.Valid {
					return fmt.Errorf("validation failed with %d critical issue(s)", len(result.Critical))
				}
				fmt.Println("Validation passed. Run without --validate-only to generate plan.")
				return nil
			}

			// If validation failed and not validate-only, fail before planning
			if !result.Valid {
				fmt.Println("\nHint: Run 'openexec doctor intent --fix' to scaffold missing sections")
				fmt.Println("      Or use --no-validate to skip validation")
				return fmt.Errorf("cannot plan: intent document has %d critical issue(s)", len(result.Critical))
			}

			// Show brief summary if validation passed
			if result.Valid && len(result.Warnings) > 0 {
				fmt.Printf("Validation passed with %d warning(s)\n\n", len(result.Warnings))
			}
		}

		// Get absolute path to intent file
		absIntentFile, err := filepath.Abs(intentFile)
		if err != nil {
			return fmt.Errorf("failed to resolve intent file path: %w", err)
		}

		// Load project configuration
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Use planner model from project config, fallback to executor
		plannerModel := config.Execution.PlannerModel
		if plannerModel == "" {
			plannerModel = config.Execution.ExecutorModel
		}
		if plannerModel == "" {
			plannerModel = "sonnet" // default
		}

		// Check if review is enabled
		reviewEnabled := config.Execution.ReviewEnabled
		reviewerModel := config.Execution.ReviewerModel

		fmt.Printf("Generating plan from: %s\n", intentFile)
		fmt.Printf("  Planner model:  %s\n", plannerModel)
		if reviewEnabled {
			fmt.Printf("  Review enabled: yes (reviewer: %s)\n", reviewerModel)
		} else {
			fmt.Printf("  Review enabled: no\n")
		}
		fmt.Printf("  Tract store: %s\n", config.TractStore)
		fmt.Printf("  Engram context: %s\n\n", config.EngramStore)

		// Check if openexec-orchestration is available
		orchestrationBinary := "openexec-orchestration"
		if _, err := exec.LookPath(orchestrationBinary); err != nil {
			return fmt.Errorf("orchestration engine not found: ensure openexec-orchestration is installed and in PATH\n\nInstall with:\n  cd ../openexec-orchestration && pip install -e .")
		}

		// Output path for generated stories
		storiesPath := filepath.Join(config.TractStore, "stories.json")

		// Build orchestration command arguments
		orchestrationArgs := []string{"generate", absIntentFile, "--output", storiesPath, "--model", plannerModel}
		if reviewEnabled && reviewerModel != "" {
			orchestrationArgs = append(orchestrationArgs, "--reviewer", reviewerModel)
		}

		// Invoke orchestration engine to generate stories
		// #nosec G204 - absIntentFile is validated to exist
		orchestrationCmd := exec.Command(orchestrationBinary, orchestrationArgs...)

		// Capture output
		output, err := orchestrationCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("orchestration engine failed: %w\nOutput: %s", err, string(output))
		}

		fmt.Printf("%s\n", string(output))
		fmt.Printf("Stories generated: %s\n", storiesPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(planCmd)

	// Validation flags
	planCmd.Flags().Bool("validate-only", false, "Validate INTENT.md and exit (no planning)")
	planCmd.Flags().Bool("no-validate", false, "Skip INTENT.md validation")
	planCmd.Flags().Bool("fix", false, "With --validate-only, show missing section stubs")
}
