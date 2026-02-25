package main

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "axon",
	Short: "Axon CLI",
	Long:  `Axon is a CLI tool by Hyperengineering.`,
}
