package cli

import (
    "fmt"
    "os"

    ipatch "github.com/openexec/openexec/internal/patch"
    "github.com/spf13/cobra"
)

var (
    patchRoot  string
    patchDryRun bool
)

var patchCmd = &cobra.Command{
    Use:   "patch",
    Short: "Apply deterministic unified diffs to the workspace",
}

var patchApplyCmd = &cobra.Command{
    Use:   "apply [diff-file]",
    Short: "Apply a unified diff file (scoped to workspace root)",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        diffFile := args[0]
        f, err := os.Open(diffFile)
        if err != nil { return err }
        defer f.Close()
        if patchRoot == "" { patchRoot = "." }
        // Enforce read-only mode via environment contract
        if os.Getenv("OPENEXEC_MODE") == "read-only" && !patchDryRun {
            return fmt.Errorf("patch apply is not allowed in read-only mode. Use --dry-run or set OPENEXEC_MODE to workspace-write/danger-full-access")
        }
        cmd.Printf("Applying patch to %s (dry-run=%v)\n", patchRoot, patchDryRun)
        if err := ipatch.ApplyUnifiedDiff(patchRoot, f, patchDryRun); err != nil {
            return err
        }
        if patchDryRun {
            cmd.Println("✓ Patch validated (dry-run)")
        } else {
            cmd.Println("✓ Patch applied")
        }
        return nil
    },
}

func init() {
    patchApplyCmd.Flags().StringVar(&patchRoot, "root", ".", "Workspace root (guarded)")
    patchApplyCmd.Flags().BoolVar(&patchDryRun, "dry-run", false, "Validate without writing files")
    patchCmd.AddCommand(patchApplyCmd)
    rootCmd.AddCommand(patchCmd)
}
