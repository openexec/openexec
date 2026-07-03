package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// auditHMACKeyEnv names the environment variable holding the shared secret used
// to HMAC-sign an audit export. When unset, the export is still integrity-sealed
// by its hash chain; the signature adds authenticity only.
const auditHMACKeyEnv = "OPENEXEC_AUDIT_HMAC_KEY"

var governanceAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Export and verify the tamper-evident governance audit trail",
	Long: `Audit trail commands.

Every governance decision is recorded as an append-only, hash-chained decision
event. These commands produce a verifiable export of that chain and re-verify
its integrity.`,
}

var govAuditExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the full decision-event audit trail as a verifiable, sealed JSON document",
	Long: `Export every decision event in insertion order with its hash-chain links,
re-verify the chain, and seal the export with the chain-head hash (which commits
to the entire history). If ` + auditHMACKeyEnv + ` is set, the seal is additionally
signed with HMAC-SHA256, adding authenticity on top of integrity.

The export always succeeds; if the chain is broken it is reported in
"chain_verified": false with "chain_error" naming the break.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		var key []byte
		if v := os.Getenv(auditHMACKeyEnv); v != "" {
			key = []byte(v)
		}
		export, err := svc.ExportAudit(cmd.Context(), key)
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(export, "", "  ")
		if err != nil {
			return fmt.Errorf("encode audit export: %w", err)
		}

		outPath, _ := cmd.Flags().GetString("out")
		if outPath != "" {
			if err := os.WriteFile(outPath, append(data, '\n'), 0o600); err != nil {
				return fmt.Errorf("write audit export to %s: %w", outPath, err)
			}
			cmd.Printf("Wrote %d events to %s (chain_verified=%t, seal=%s)\n",
				export.EventCount, outPath, export.ChainOK, export.Seal)
		} else {
			cmd.Println(string(data))
		}
		// A broken chain is a hard signal: surface it on stderr and fail the command
		// so a CI/audit pipeline notices even when the JSON is redirected to a file.
		if !export.ChainOK {
			return fmt.Errorf("audit chain verification FAILED: %s", export.ChainError)
		}
		return nil
	},
}

var govAuditVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Re-verify the live decision-event hash chain and report any tampering",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		svc, _, db, err := newGovService(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ok, reason, count, err := svc.VerifyAudit(cmd.Context())
		if err != nil {
			return err
		}
		if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
			return printJSON(cmd, map[string]any{
				"chain_verified": ok,
				"chain_error":    reason,
				"events_checked": count,
			})
		}
		if ok {
			cmd.Printf("Audit chain intact: %d events verified.\n", count)
			return nil
		}
		return fmt.Errorf("audit chain verification FAILED after %d events: %s", count, reason)
	},
}

func init() {
	governanceCmd.AddCommand(governanceAuditCmd)

	governanceAuditCmd.AddCommand(govAuditExportCmd)
	govAuditExportCmd.Flags().String("out", "", "Write the export to this file instead of stdout")

	governanceAuditCmd.AddCommand(govAuditVerifyCmd)
	govAuditVerifyCmd.Flags().Bool("json", false, "Output as JSON")
}
