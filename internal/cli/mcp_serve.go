package cli

import (
	"fmt"
	"os"

	"github.com/openexec/openexec/internal/mcp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(mcpServeCmd)
}

var mcpServeCmd = &cobra.Command{
	Use:    "mcp-serve",
	Short:  "Run the OpenExec MCP signal server (used by Claude Code)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get workspace from current directory or WORKSPACE_ROOT
		workDir, _ := os.Getwd()
		srv, err := mcp.NewServerWithConfig(os.Stdin, os.Stdout, mcp.ServerConfig{
			WorkDir: workDir,
			Mode:    os.Getenv("OPENEXEC_MODE"),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
			fmt.Fprintf(os.Stderr, "Hint: Set WORKSPACE_ROOT env var or run from a valid project directory\n")
			return err
		}

		return srv.Serve()
	},
}
