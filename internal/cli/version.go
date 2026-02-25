package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	// Version is the current version of the OpenExec CLI
	Version = "0.1.0-dev"
	// Commit is the git commit hash at build time
	Commit = "none"
	// BuildDate is the date the binary was built
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of OpenExec CLI",
	Long:  `Display the current version, build commit, and build date of the OpenExec CLI binary.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("OpenExec CLI v%s\n", Version)
		fmt.Printf("  Commit:     %s\n", Commit)
		fmt.Printf("  Build Date: %s\n", BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
