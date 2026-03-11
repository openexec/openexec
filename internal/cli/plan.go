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
	"github.com/openexec/openexec/internal/runner"
	"github.com/spf13/cobra"
)

var planCmd = &cobra.Command{
	Use:   "plan [intent-file]",
	Short: "Generate project plan from intent document",
	Long: `Generate a project plan from an intent document using the native Go orchestration engine.

If no file is provided, it searches for INTENT.md in the current directory or docs/ directory.
The generated plan is stored in the Tract store for reference and execution.

By default, INTENT.md is validated before planning. Critical issues will
block planning; warnings are reported but allowed.

Validation flags:
  --validate-only  Validate and exit without planning (exit 1 on critical failures)
  --no-validate    Skip validation entirely
  --fix            With --validate-only, show stubs for missing sections

Examples:
  openexec plan                                  # Search for INTENT.md then plan
  openexec plan docs/intent.md                   # Specific file
  openexec plan --validate-only                  # Validate only`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		intentFile := "INTENT.md"
		if len(args) > 0 {
			intentFile = args[0]
		}

		// Resolve intent file path with fallbacks
		if _, err := os.Stat(intentFile); os.IsNotExist(err) {
			fallbacks := []string{"intent.md", "docs/INTENT.md", "docs/intent.md"}
			found := false
			for _, f := range fallbacks {
				if _, err := os.Stat(f); err == nil {
					intentFile = f
					found = true
					break
				}
			}
			if !found {
				if len(args) > 0 {
					return fmt.Errorf("intent file not found: %s", args[0])
				}
				return fmt.Errorf("no intent file found. Create INTENT.md or run 'openexec wizard' to generate one")
			}
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
				cmd.Println("Validation passed. Run 'openexec plan' to generate stories.")
				return nil
			}

			// If validation failed and not validate-only, fail before planning
			if !result.Valid {
				cmd.Println("\nHint: Fix the issues above or run 'openexec wizard' to re-align your intent.")
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

		if plan == nil || len(plan.Stories) == 0 {
			return fmt.Errorf("planner returned an empty plan. Check your intent file or try a more capable model")
		}

		// 4. Validate and Save to stories.json
		storiesPath := filepath.Join(config.TractStore, "stories.json")
		data, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal plan: %w", err)
		}

		// STRUCTURAL VALIDATION: Ensure it's valid before saving
		var dummy planner.ProjectPlan
		if err := json.Unmarshal(data, &dummy); err != nil {
			return fmt.Errorf("ABORTING: Generated plan is structurally invalid: %w", err)
		}

		if err := os.WriteFile(storiesPath, data, 0644); err != nil {
			return fmt.Errorf("failed to save stories: %w", err)
		}

		cmd.Printf("✓ Stories generated: %s (%d stories)\n", storiesPath, len(plan.Stories))
		cmd.Printf("\n🚀 NEXT STEP: Run 'openexec run' to begin autonomous execution.\n")
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
	// Resolve runner using centralized logic.
	// Can be overridden via env vars for the planner.
	cliCmd, cmdArgs, err := runner.Resolve(
		p.model,
		os.Getenv("OPENEXEC_PLANNER_CLI"),
		strings.Fields(os.Getenv("OPENEXEC_PLANNER_ARGS")),
	)
	if err != nil {
		return "", err
	}

	// For Claude, ensuring we use --print or similar non-interactive flag if it was using defaults.
	if strings.Contains(strings.ToLower(cliCmd), "claude") {
		cmdArgs = []string{"--print"}
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
