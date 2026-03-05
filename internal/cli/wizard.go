package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/openexec/openexec/internal/project"
	"github.com/spf13/cobra"
)

// WizardResponse matches the JSON output from openexec-planner wizard
type WizardResponse struct {
	UpdatedState    map[string]interface{} `json:"updated_state"`
	NextQuestion    string                 `json:"next_question"`
	Acknowledgement string                 `json:"acknowledgement"`
	IsComplete      bool                   `json:"is_complete"`
	NewFacts        []string               `json:"new_facts"`
	NewAssumptions  []string               `json:"new_assumptions"`
}

var wizardCmd = &cobra.Command{
	Use:   "wizard",
	Short: "Start the guided intent interviewer",
	Long: `Start an interactive chat-based interview to define your project intent.
The wizard will help you pin down core constraints like platform, application shape,
and contracts before generating your INTENT.md and stories.`,
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

		// Check for planner binary
		if _, err := exec.LookPath("openexec-planner"); err != nil {
			return fmt.Errorf("openexec-planner not found in PATH. Please install it first")
		}

		cmd.Println(color.CyanString("=== OpenExec Guided Intent Interviewer ==="))
		cmd.Printf("   Project: %s\n", config.Name)
		cmd.Printf("   Model:   %s\n", model)
		
		statePath := filepath.Join(".openexec", "wizard_state.json")
		stateJSON := ""
		
		// Try to resume existing session
		if data, err := os.ReadFile(statePath); err == nil {
			cmd.Println(color.YellowString("   [Resuming existing session from %s]", statePath))
			stateJSON = string(data)
		}

		cmd.Println("Tell me about your project (free-form dump, or type 'exit' to quit):")
		cmd.Println()

		reader := bufio.NewReader(os.Stdin)

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

			// Call orchestration wizard
			cmd.Print(color.CyanString("Thinking... "))
			resp, err := callOrchestrationWizard(message, stateJSON, model)
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
				
				md, err := renderIntentMD(stateJSON, model)
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

func callOrchestrationWizard(message string, state string, model string) (*WizardResponse, error) {
	args := []string{"wizard", "--message", message, "--model", model}
	if state != "" {
		args = append(args, "--state", state)
	}

	cmd := exec.Command("openexec-planner", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("wizard failed: %v\nOutput: %s", err, string(output))
	}

	var resp WizardResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse wizard response: %v\nOutput: %s", err, string(output))
	}

	return &resp, nil
}

func renderIntentMD(state string, model string) (string, error) {
	cmd := exec.Command("openexec-planner", "wizard", "--render", "--state", state, "--model", model)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to render intent: %w\nOutput: %s", err, string(output))
	}
	return string(output), nil
}
