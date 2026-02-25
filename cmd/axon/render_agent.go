package main

import (
	"fmt"
	"os"

	"github.com/openexec/openexec/internal/prompt/render"
	"github.com/spf13/cobra"
)

var (
	renderNameFlag      string
	renderOutputFlag    string
	renderAgentsDirFlag string
)

func init() {
	renderAgentCmd.Flags().StringVar(&renderNameFlag, "name", "", "Agent name (required)")
	renderAgentCmd.Flags().StringVarP(&renderOutputFlag, "output", "o", "", "Output file path (default: stdout)")
	renderAgentCmd.Flags().StringVar(&renderAgentsDirFlag, "agents-dir", "./agents", "Directory containing agent definitions")

	_ = renderAgentCmd.MarkFlagRequired("name")

	rootCmd.AddCommand(renderAgentCmd)
}

var renderAgentCmd = &cobra.Command{
	Use:   "render-agent",
	Short: "Render a standalone agent file from decomposed definitions",
	Long: `Reassembles decomposed agent definitions (manifest, persona, workflows)
into a standalone monolithic markdown+XML file for use in HITL Claude Code sessions.

The output file can be used directly as a system prompt:
  claude --system-prompt "$(cat spark.md)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := render.RenderAgent(renderAgentsDirFlag, renderNameFlag)
		if err != nil {
			return err
		}

		if renderOutputFlag == "" {
			fmt.Print(result)
			return nil
		}

		if err := os.WriteFile(renderOutputFlag, []byte(result), 0644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}

		_, _ = fmt.Fprintf(cmd.OutOrStderr(), "rendered %s to %s\n", renderNameFlag, renderOutputFlag)
		return nil
	},
}
