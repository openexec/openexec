package main

import "github.com/spf13/cobra"

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of axon",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Printf("axon %s (commit: %s, built: %s)\n", version, commit, date)
	},
}
