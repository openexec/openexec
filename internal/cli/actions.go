package cli

import (
    "fmt"
    "io"
    "os"

    "github.com/openexec/openexec/pkg/actions"
    "github.com/spf13/cobra"
)

var actionsCmd = &cobra.Command{
    Use:   "actions",
    Short: "Work with typed execution actions",
}

var actionsValidateCmd = &cobra.Command{
    Use:   "validate",
    Short: "Validate an Action JSON from stdin",
    RunE: func(cmd *cobra.Command, args []string) error {
        b, err := io.ReadAll(os.Stdin)
        if err != nil { return err }
        a, err := actions.ParseAction(b)
        if err != nil { return err }
        fmt.Fprintf(cmd.OutOrStdout(), "✓ Valid action: %s (%s)\n", a.ID, a.Type)
        return nil
    },
}

func init() {
    actionsCmd.AddCommand(actionsValidateCmd)
    rootCmd.AddCommand(actionsCmd)
}

