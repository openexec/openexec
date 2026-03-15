package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/openexec/openexec/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no subcommand is provided, start the chat mode
		if len(args) == 0 {
			err := chatCmd.RunE(cmd, args)
			if err != nil {
				// Don't return the error directly to prevent Cobra from printing usage
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			return nil
		}
		return cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rootCmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Disable Cobra's default help command (we have our own extended helpCmd)
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

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
