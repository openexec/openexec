package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Start the guided intent interviewer",
	Long: `Start an interactive chat-based interview to define your project intent.
The wizard will help you pin down core constraints like platform, application shape,
and contracts before generating your INTENT.md and stories using the native Go engine.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Load project configuration
		config, err := project.LoadProjectConfig(".")
		if err != nil {
			return fmt.Errorf("project not initialized: run 'openexec init' first")
		}

		// Use planner model from project config, fallback to executor
		model := config.Execution.PlannerModel
		if model == "" {
			model = config.Execution.ExecutorModel
		}
		if model == "" {
			model = "sonnet" // default
		}

		cmd.Println(color.CyanString("=== OpenExec Guided Intent Interviewer ==="))
		cmd.Printf("   Project: %s\n", config.Name)
		cmd.Printf("   Engine:  Native Go Wizard\n")
		cmd.Printf("   Model:   %s\n", model)

		statePath := filepath.Join(".openexec", "wizard_state.json")
		stateJSON := "{}"

		// Try to resume existing session
		if data, err := os.ReadFile(statePath); err == nil {
			cmd.Println(color.YellowString("   [Resuming existing session from %s]", statePath))
			stateJSON = string(data)
		}

		cmd.Println("Tell me about your project (free-form dump, or type 'exit' to quit):")
		cmd.Println()

		reader := bufio.NewReader(os.Stdin)
		p := planner.New(&cliLLMProvider{model: model, cmd: cmd})

		for {
			cmd.Print(color.GreenString("> "))
			input, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			message := strings.TrimSpace(input)
			if message == "exit" || message == "quit" {
				cmd.Println("Goodbye! Your progress is saved in " + statePath)
				return nil
			}

			if message == "" {
				continue
			}

			// Call orchestration wizard natively
			cmd.Print(color.CyanString("Thinking... "))
			resp, err := p.ProcessWizardMessage(cmd.Context(), message, stateJSON)
			cmd.Print("\r") // Clear Thinking line
			if err != nil {
				return err
			}

			// Update state for next turn
			stateBytes, _ := json.Marshal(resp.UpdatedState)
			stateJSON = string(stateBytes)

			// Persist state to disk
			_ = os.WriteFile(statePath, stateBytes, 0644)

			// Show feedback
			if resp.Acknowledgement != "" {
				cmd.Println()
				cmd.Println(color.BlueString("🤖 %s", resp.Acknowledgement))
			}

			if len(resp.NewFacts) > 0 {
				cmd.Println(color.WhiteString("\n  ✔ Explicit:"))
				for _, f := range resp.NewFacts {
					cmd.Printf("    - %s\n", f)
				}
			}

			if len(resp.NewAssumptions) > 0 {
				cmd.Println(color.YellowString("\n  ⚠ Assumed:"))
				for _, a := range resp.NewAssumptions {
					cmd.Printf("    - %s\n", a)
				}
			}

			if resp.IsComplete {
				cmd.Println()
				cmd.Println(color.CyanString("✔ Intent is complete! Rendering INTENT.md..."))

				md, err := p.RenderIntent(cmd.Context(), stateJSON)
				if err != nil {
					return err
				}

				if err := os.WriteFile("INTENT.md", []byte(md), 0644); err != nil {
					return fmt.Errorf("failed to write INTENT.md: %w", err)
				}

				cmd.Println("Written to INTENT.md")
				cmd.Println("\nYou can now run: " + color.GreenString("openexec plan INTENT.md"))
				return nil
			}

			// Ask next question
			cmd.Println()
			cmd.Println(color.GreenString("? %s", resp.NextQuestion))
			cmd.Println()
		}
	},
}

func init() {
	rootCmd.AddCommand(wizardCmd)
}
