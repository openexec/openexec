package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/openexec/openexec/internal/intent"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan <intent-file>",
	Short: "Generate project plan from intent document",
	Long: `Generate a project plan from an intent document using the native Go orchestration engine.

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
				cmd.Println(fixer.Preview())
				if !result.Valid {
					return fmt.Errorf("validation failed with %d critical issue(s)", len(result.Critical))
				}
				return nil
			}

			// Show validation result
			if validateOnly || !result.Valid {
				reporter := intent.NewReporter(result)
				cmd.Println(reporter.Generate())
			}

			// If validate-only, exit here
			if validateOnly {
				if !result.Valid {
					return fmt.Errorf("validation failed with %d critical issue(s)", len(result.Critical))
				}
				cmd.Println("Validation passed. Run without --validate-only to generate plan.")
				return nil
			}

			// If validation failed and not validate-only, fail before planning
			if !result.Valid {
				cmd.Println("\nHint: Run 'openexec knowledge show prd' to check requirements")
				return fmt.Errorf("cannot plan: intent document has %d critical issue(s)", len(result.Critical))
			}

			// Show brief summary if validation passed
			if result.Valid && len(result.Warnings) > 0 {
				cmd.Printf("Validation passed with %d warning(s)\n\n", len(result.Warnings))
			}
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

		cmd.Printf("Generating plan from: %s\n", intentFile)
		cmd.Printf("  Engine:         Native Go Orchestrator\n")
		cmd.Printf("  Planner model:  %s\n", plannerModel)
		cmd.Printf("  Tract store:    %s\n", config.TractStore)

		// Fetch PRD context from Knowledge Store
		var prdContext map[string][]*knowledge.PRDRecord
		kStore, err := knowledge.NewStore(".")
		if err == nil {
			defer kStore.Close()
			sections := []string{"personas", "user_journeys", "functional", "non_functional"}
			prdContext = make(map[string][]*knowledge.PRDRecord)
			for _, sec := range sections {
				records, _ := kStore.ListPRDRecords(sec)
				if len(records) > 0 {
					prdContext[sec] = records
				}
			}
		}

		// 1. Initialize Native Planner
		p := planner.New(&cliLLMProvider{
			model: plannerModel,
			cmd:   cmd,
		})

		// 2. Read intent content
		intentContent, err := os.ReadFile(intentFile)
		if err != nil {
			return err
		}

		// 3. Generate Plan
		cmd.Println("  Planning...")
		plan, err := p.GeneratePlan(cmd.Context(), string(intentContent), prdContext)
		if err != nil {
			return err
		}

		// 4. Save to stories.json
		storiesPath := filepath.Join(config.TractStore, "stories.json")
		data, _ := json.MarshalIndent(plan, "", "  ")
		if err := os.WriteFile(storiesPath, data, 0644); err != nil {
			return fmt.Errorf("failed to save stories: %w", err)
		}

		cmd.Printf("✓ Stories generated: %s (%d stories)\n", storiesPath, len(plan.Stories))
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

type cliLLMProvider struct {
	model string
	cmd   *cobra.Command
}

func (p *cliLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	// Simple shell-out to claude or gemini CLI for planning
	cliCmd := "claude"
	if strings.Contains(p.model, "gemini") {
		cliCmd = "gemini"
	}

	var cmdArgs []string
	if cliCmd == "claude" {
		cmdArgs = []string{"--print"}
	} else if cliCmd == "gemini" {
		cmdArgs = []string{"--prompt", "-", "--yolo"}
	}

	c := exec.CommandContext(ctx, cliCmd, cmdArgs...)
	c.Stdin = strings.NewReader(prompt)
	
	output, err := c.CombinedOutput()
	if err != nil {
		outStr := string(output)
		// Detect specific provider errors to give better hints
		if strings.Contains(outStr, "authentication_error") || strings.Contains(outStr, "OAuth token has expired") || strings.Contains(outStr, "/login") {
			return "", fmt.Errorf("\n❌ AI Provider Authentication Failed.\n\nYour '%s' CLI session has expired. Please run:\n  %s login\n\nOriginal error: %s", cliCmd, cliCmd, outStr)
		}
		return "", fmt.Errorf("native LLM provider failed: %w\nOutput: %s", err, outStr)
	}

	return string(output), nil
}
