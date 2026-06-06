package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openexec/openexec/internal/approval"
	"github.com/openexec/openexec/pkg/db/sqlitecfg"
)

// --local mode for `openexec approve`: operate directly on the shared
// approvals database (.openexec/approvals.db) written by the mcp-serve
// persistent gate (Phase 3 of the SRE roadmap). No daemon required — this
// is the terminal sign-off channel for blocked apply-class infra commands.

func localApprovalManager() (*approval.Manager, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(wd, ".openexec", "approvals.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("no approvals database at %s — nothing has requested approval in this workspace yet", dbPath)
	}
	db, err := sql.Open("sqlite", sqlitecfg.DSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open approvals db: %w", err)
	}
	repo, err := approval.NewSQLiteRepository(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return approval.NewManager(repo), nil
}

func approveListLocal() error {
	mgr, err := localApprovalManager()
	if err != nil {
		return err
	}
	pending, err := mgr.ListPendingRequests(context.Background(), approval.PersistentGateSessionID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		fmt.Println("No pending approval requests.")
		return nil
	}
	for _, r := range pending {
		age := time.Since(r.CreatedAt).Round(time.Second)
		fmt.Printf("%s\n  tool: %s  risk: %s  waiting: %s\n  args: %s\n",
			r.ID, r.ToolName, r.RiskLevel, age, r.ToolInput)
	}
	fmt.Printf("\n%d pending. Resolve with: openexec approve yes|no <id> --local\n", len(pending))
	return nil
}

func approveYesLocal(requestID, note string) error {
	mgr, err := localApprovalManager()
	if err != nil {
		return err
	}
	if err := mgr.Approve(context.Background(), requestID, "cli-operator", note); err != nil {
		return err
	}
	fmt.Printf("Approved %s. The blocked command will observe the decision within seconds.\n", requestID)
	return nil
}

func approveNoLocal(requestID, reason string) error {
	mgr, err := localApprovalManager()
	if err != nil {
		return err
	}
	if err := mgr.Reject(context.Background(), requestID, "cli-operator", reason); err != nil {
		return err
	}
	fmt.Printf("Rejected %s: %s\n", requestID, reason)
	return nil
}

func init() {
	// --local on the approve group: bypass the daemon HTTP API and operate
	// on the shared approvals database directly (light-mode sign-off).
	approveCmd.PersistentFlags().Bool("local", false, "Operate directly on .openexec/approvals.db (no daemon; used for infra approval sign-off)")
}
