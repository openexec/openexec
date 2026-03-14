package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"
)

var tractStorePath string

var tractServeCmd = &cobra.Command{
	Use:    "tract-serve",
	Short:  "Internal: Proxy to tract serve for builtin integration",
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		bin := "tract"
		if _, err := exec.LookPath(bin); err != nil {
			// Try sibling
			execPath, _ := os.Executable()
			siblingTract := filepath.Join(filepath.Dir(filepath.Dir(execPath)), "tract", "tract")
			if _, err := os.Stat(siblingTract); err == nil {
				bin = siblingTract
			} else {
				return fmt.Errorf("tract binary not found in path or sibling directory. Please install it first.")
			}
		}

		// Pass-through to real tract
		serveArgs := []string{"serve", "--store", tractStorePath}
		
		// Use syscall.Exec to replace current process
		return syscall.Exec(bin, append([]string{bin}, serveArgs...), os.Environ())
	},
}

func init() {
	tractServeCmd.Flags().StringVar(&tractStorePath, "store", "", "Tract store name")
	rootCmd.AddCommand(tractServeCmd)
}
