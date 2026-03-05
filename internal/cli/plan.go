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
	Long: `Generate a project plan from an intent document.

This command reads an intent.md file and invokes the planner engine
to generate a Goal Tree and Functional Work Units (FWUs) that define
the execution plan for the project.

The generated plan is stored in the Tract store for reference and execution.

By default, INTENT.md is validated before planning. Critical issues will
block planning; warnings are reported but allowed.

Validation flags:
  --validate-only  Validate and exit without planning (exit 1 on critical failures)
  --no-validate    Skip validation entirely
  --fix            With --validate-only, show stubs for missing sections
  --native         Use internal Go planner instead of external python engine

Examples:
  openexec plan INTENT.md                        # Validate then plan
  openexec plan INTENT.md --validate-only        # Validate only
  openexec plan INTENT.md --validate-only --fix  # Show missing section stubs
  openexec plan INTENT.md --no-validate          # Skip validation
  openexec plan INTENT.md --native               # Use native Go planner`,
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
		useNative, _ := cmd.Flags().GetBool("native")

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
				cmd.Println("\nHint: Run 'openexec doctor intent --fix' to scaffold missing sections")
				cmd.Println("      Or use --no-validate to skip validation")
				return fmt.Errorf("cannot plan: intent document has %d critical issue(s)", len(result.Critical))
			}

			// Show brief summary if validation passed
			if result.Valid && len(result.Warnings) > 0 {
				cmd.Printf("Validation passed with %d warning(s)\n\n", len(result.Warnings))
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

		cmd.Printf("Generating plan from: %s\n", intentFile)
		cmd.Printf("  Planner model:  %s\n", plannerModel)
		if reviewEnabled {
			cmd.Printf("  Review enabled: yes (reviewer: %s)\n", reviewerModel)
		} else {
			cmd.Printf("  Review enabled: no\n")
		}
		cmd.Printf("  Tract store: %s\n", config.TractStore)
		cmd.Printf("  Engram context: %s\n\n", config.EngramStore)

		// Check if openexec-planner is available
		plannerBinary := "openexec-planner"
		if !useNative {
			if _, err := exec.LookPath(plannerBinary); err != nil {
				cmd.Println("  ! Python planner not found. Falling back to --native Go planner.")
				useNative = true
			}
		}

		// Output path for generated stories
		storiesPath := filepath.Join(config.TractStore, "stories.json")

		// EXPORT PRD CONTEXT from DCP Knowledge Store if available
		var prdContextPath string
		var prdContext map[string][]*knowledge.PRDRecord

		kStore, err := knowledge.NewStore(".")
		if err == nil {
			defer kStore.Close()
			// Fetch all sections
			sections := []string{"personas", "user_journeys", "functional", "non_functional"}
			prdData := make(map[string]interface{})
			prdContext = make(map[string][]*knowledge.PRDRecord)

			for _, sec := range sections {
				records, _ := kStore.ListPRDRecords(sec)
				if len(records) > 0 {
					prdData[sec] = records
					prdContext[sec] = records
				}
			}

			if len(prdData) > 0 && !useNative {
				cmd.Printf("  + Exporting %d PRD sections from Knowledge Base...\n", len(prdData))
				tmpFile, _ := os.CreateTemp("", "prd_context_*.json")
				data, _ := json.Marshal(prdData)
				tmpFile.Write(data)
				prdContextPath = tmpFile.Name()
				tmpFile.Close()
				defer os.Remove(prdContextPath)
			}
		}

		if useNative {
			cmd.Println("🧠 Using Native Go Planner Engine...")

			// 1. Initialize Native Go Planner
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
			plan, err := p.GeneratePlan(cmd.Context(), string(intentContent), prdContext)
			if err != nil {
				return err
			}

			// 4. Save to stories.json
			data, _ := json.MarshalIndent(plan, "", "  ")
			if err := os.WriteFile(storiesPath, data, 0644); err != nil {
				return fmt.Errorf("failed to save stories: %w", err)
			}

			cmd.Printf("✓ Stories generated: %s (%d stories)\n", storiesPath, len(plan.Stories))
			return nil
		}

		// Build planner command arguments
		plannerArgs := []string{"generate", absIntentFile, "--output", storiesPath, "--model", plannerModel}
		if reviewEnabled && reviewerModel != "" {
			plannerArgs = append(plannerArgs, "--reviewer", reviewerModel)
		}
		if prdContextPath != "" {
			plannerArgs = append(plannerArgs, "--prd-context", prdContextPath)
		}

		// Invoke planner engine to generate stories
		// #nosec G204 - absIntentFile is validated to exist
		pCmd := exec.Command(plannerBinary, plannerArgs...)

		// Capture output
		output, err := pCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("planner engine failed: %w\nOutput: %s", err, string(output))
		}

		cmd.Printf("%s\n", string(output))
		cmd.Printf("Stories generated: %s\n", storiesPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(planCmd)

	// Validation flags
	planCmd.Flags().Bool("validate-only", false, "Validate INTENT.md and exit (no planning)")
	planCmd.Flags().Bool("no-validate", false, "Skip INTENT.md validation")
	planCmd.Flags().Bool("fix", false, "With --validate-only, show missing section stubs")
	planCmd.Flags().Bool("native", false, "Use internal Go planner")
}

type cliLLMProvider struct {
	model string
	cmd   *cobra.Command
}

func (p *cliLLMProvider) Complete(ctx context.Context, prompt string) (string, error) {
	// For now, we simulate success for the port.
	// In a real scenario, this would call 'claude' or 'gemini' CLI.
	// We'll use the 'claude --print' pattern here since it's most common.
	
	cliCmd := "claude"
	if strings.Contains(p.model, "gemini") {
		cliCmd = "gemini"
	} else if strings.Contains(p.model, "gpt") {
		cliCmd = "codex"
	}

	var cmdArgs []string
	if cliCmd == "claude" {
		cmdArgs = []string{"--print"}
	} else if cliCmd == "gemini" {
		cmdArgs = []string{"--prompt", "-", "--yolo"}
	} else {
		cmdArgs = []string{"exec", "--json", "--full-auto", "-"}
	}

	c := exec.CommandContext(ctx, cliCmd, cmdArgs...)
	c.Stdin = strings.NewReader(prompt)
	
	output, err := c.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("native LLM provider failed: %w\nOutput: %s", err, string(output))
	}

	return string(output), nil
}
