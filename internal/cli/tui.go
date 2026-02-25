package cli

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/openexec/openexec/internal/tui"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui [directory]",
	Short: "Launch the interactive TUI dashboard",
	Long: `Launch an interactive terminal UI showing multi-project execution status and worker activity.

By default, scans the current directory for projects with .openexec/ directories.
Specify a directory to scan a different location.

Examples:
  openexec tui                  # Scan current directory
  openexec tui /path/to/projects  # Scan specific directory`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine base directory
		baseDir := "."
		if len(args) > 0 {
			baseDir = args[0]
		}

		// Verify directory exists
		if info, err := os.Stat(baseDir); err != nil || !info.IsDir() {
			cmd.PrintErrf("Error: %s is not a valid directory\n", baseDir)
			return err
		}

		// Create file-based source for real project data
		source := tui.NewFileSource(baseDir)
		defer source.Close()

		// Create and run the TUI app
		app := tui.NewApp(source)
		p := tea.NewProgram(app, tea.WithAltScreen())

		if _, err := p.Run(); err != nil {
			return err
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
