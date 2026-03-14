package cli

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"

    "github.com/spf13/cobra"
)

type checkpointLine struct {
    RunID     string            `json:"run_id"`
    Phase     string            `json:"phase"`
    Iteration int               `json:"iteration"`
    Timestamp string            `json:"timestamp"`
    Artifacts map[string]string `json:"artifacts"`
}

var replayCmd = &cobra.Command{
    Use:   "replay [run-id]",
    Short: "Inspect checkpointed artifacts for a run (patch sequence)",
    Args:  cobra.ExactArgs(1),
    RunE: func(cmd *cobra.Command, args []string) error {
        runID := args[0]
        wd, _ := os.Getwd()
        ckPath := filepath.Join(wd, ".openexec", "checkpoints", runID+".jsonl")
        f, err := os.Open(ckPath)
        if err != nil {
            return fmt.Errorf("open checkpoints: %w", err)
        }
        defer f.Close()
        fmt.Fprintf(cmd.OutOrStdout(), "Replay for %s\n", runID)
        fmt.Fprintln(cmd.OutOrStdout(), "----------------------------------------")
        s := bufio.NewScanner(f)
        idx := 0
        for s.Scan() {
            var line checkpointLine
            if err := json.Unmarshal([]byte(s.Text()), &line); err != nil {
                continue
            }
            idx++
            ph := line.Phase
            if ph == "" { ph = "(unknown)" }
            fmt.Fprintf(cmd.OutOrStdout(), "#%d [%s] iter=%d time=%s\n", idx, ph, line.Iteration, line.Timestamp)
            if line.Artifacts != nil {
                if h, ok := line.Artifacts["patch_hash"]; ok {
                    fmt.Fprintf(cmd.OutOrStdout(), "  patch: %s\n", h)
                }
                if p, ok := line.Artifacts["patch_path"]; ok {
                    fmt.Fprintf(cmd.OutOrStdout(), "  path : %s\n", p)
                }
            }
        }
        return nil
    },
}

func init() {
    rootCmd.AddCommand(replayCmd)
}

