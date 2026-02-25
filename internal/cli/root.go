package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/openexec/openexec/internal/config"
)

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "openexec",
	Short: "OpenExec CLI - Command line interface for orchestration and reporting",
	Long: `OpenExec CLI is a command-line tool for interacting with the orchestration plane,
managing projects, kicking off intents, and verifying system statuses.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.InitializeConfig(cfgFile)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.openexec/config.yaml)")
	rootCmd.PersistentFlags().String("log-level", "info", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("data-dir", "", "data directory for local stores (default is $HOME/.openexec)")

	// Bind viper flags
	if err := viper.BindPFlag("log-level", rootCmd.PersistentFlags().Lookup("log-level")); err != nil {
		panic(err)
	}
	if err := viper.BindPFlag("data-dir", rootCmd.PersistentFlags().Lookup("data-dir")); err != nil {
		panic(err)
	}
}
