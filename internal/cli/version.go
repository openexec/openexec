package cli

import (
	"github.com/openexec/openexec/pkg/version"
	"github.com/spf13/cobra"
)

var (
	// BuildDate is the date the binary was built, injected at build time
	BuildDate = "unknown"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of OpenExec CLI",
	Long:  `Display the current version, build commit, and build date of the OpenExec CLI binary.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("OpenExec CLI v%s\n", version.Version)
		cmd.Printf("  Commit:     %s\n", version.Commit)
		cmd.Printf("  Build Date: %s\n", BuildDate)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
