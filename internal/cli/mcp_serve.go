package cli

import (
	"os"

	"github.com/openexec/openexec/internal/mcp"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(mcpServeCmd)
}

var mcpServeCmd = &cobra.Command{
	Use:    "mcp-serve",
	Short:  "Run the Axon MCP signal server (used by Claude Code)",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		srv := mcp.NewServer(os.Stdin, os.Stdout)
		return srv.Serve()
	},
}
