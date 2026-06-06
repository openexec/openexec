// This file implements PersistentGate: a SQLite-backed ApprovalGate for
// cross-process, async human sign-off (Phase 3 of the SRE roadmap).
//
// InMemoryGate blocks on an in-process channel, so only the same process
// can resolve a request — useless for the light-mode flow where the agent
// runs inside `openexec mcp-serve` and the operator signs off from another
// terminal. PersistentGate writes the pending request to a shared SQLite
// database and polls it; `openexec approve list/yes/no --local` (or the
// operator-session MCP tools) resolve it from any process.
package approval

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openexec/openexec/pkg/db/sqlitecfg"
	_ "modernc.org/sqlite" // register the "sqlite" driver for the shared approvals DB
)

const (
	// PersistentGateSessionID scopes infra approval requests in the shared DB.
	PersistentGateSessionID = "infra"

	// DefaultWaitTimeout bounds how long an agent call blocks awaiting
	// sign-off. Async-friendly but finite: a forgotten request should not
	// pin an MCP call forever.
	DefaultWaitTimeout = 30 * time.Minute

	// pollInterval is how often the gate re-reads the shared database.
	pollInterval = 2 * time.Second
)

// PersistentGate implements ApprovalGate over the approval Manager's
// SQLite repository.
type PersistentGate struct {
	manager     *Manager
	waitTimeout time.Duration
	projectPath string
}

// OpenPersistentGate opens (or creates) the shared approvals database and
// returns a gate plus the Manager (for CLI/operator-tool reuse).
// dbPath is the SQLite file, e.g. <workspace>/.openexec/approvals.db —
// always opened via sqlitecfg.DSN (WAL + busy_timeout) because the gate
// and the approving process write concurrently.
func OpenPersistentGate(dbPath, projectPath string, waitTimeout time.Duration) (*PersistentGate, *Manager, error) {
	db, err := sql.Open("sqlite", sqlitecfg.DSN(dbPath))
	if err != nil {
		return nil, nil, fmt.Errorf("open approvals db: %w", err)
	}
	repo, err := NewSQLiteRepository(db)
	if err != nil {
		db.Close()
		return nil, nil, fmt.Errorf("init approvals db: %w", err)
	}
	mgr := NewManager(repo)
	if waitTimeout <= 0 {
		waitTimeout = DefaultWaitTimeout
	}
	return &PersistentGate{
		manager:     mgr,
		waitTimeout: waitTimeout,
		projectPath: projectPath,
	}, mgr, nil
}

// SetToolRisk marks tools with an explicit risk level (callers register
// their apply-class tools as high so the seeded risk-based policy can
// never auto-approve them — unknown tools default to medium).
func (g *PersistentGate) SetToolRisk(level RiskLevel, toolNames ...string) error {
	for _, name := range toolNames {
		if err := g.manager.SetRiskLevel(name, level); err != nil {
			return err
		}
	}
	return nil
}

// RequestApproval persists a pending request and blocks (polling the
// shared DB) until an operator resolves it, the wait times out, or ctx is
// cancelled.
func (g *PersistentGate) RequestApproval(ctx context.Context, req *GateRequest) (*GateRequest, error) {
	if req.ID == "" {
		req.ID = uuid.New().String()
	}
	toolInput, err := json.Marshal(req.ToolArgs)
	if err != nil {
		return nil, fmt.Errorf("marshal tool args: %w", err)
	}

	apReq, canExec, err := g.manager.RequestApproval(ctx, PersistentGateSessionID, req.ID, req.ToolName, string(toolInput), "agent", g.projectPath)
	if err != nil {
		return nil, err
	}
	req.CreatedAt = apReq.CreatedAt
	if canExec {
		// Auto-approved by policy (risk-based, low risk only).
		req.Status = RequestStatusAutoApproved
		req.ResolvedBy = "policy"
		return req, nil
	}

	// The seeded default policy expires requests after 5 minutes — far too
	// short for async sign-off. Extend the expiration to the gate's wait
	// window. (Same package: direct repo access is intentional.)
	apReq.SetExpiration(time.Now().Add(g.waitTimeout))
	if err := g.manager.repo.UpdateRequest(ctx, apReq); err != nil {
		return nil, fmt.Errorf("extend request expiration: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, g.waitTimeout)
	defer cancel()
	result, err := g.manager.WaitForApproval(waitCtx, apReq.ID, pollInterval)
	if err != nil {
		if waitCtx.Err() != nil {
			req.Status = RequestStatusExpired
			req.ResolvedBy = "timeout"
			return req, fmt.Errorf("approval request %s timed out after %s awaiting sign-off (resolve with `openexec approve yes/no %s --local`)", apReq.ID, g.waitTimeout, apReq.ID)
		}
		return nil, err
	}

	if result.Approved {
		req.Status = RequestStatusApproved
		req.ResolvedBy = "operator"
		return req, nil
	}
	req.Status = RequestStatusRejected
	req.RejectReason = result.Reason
	req.ResolvedBy = "operator"
	return req, nil
}

// gateRequestFromApproval maps a persisted request into the gate's shape.
func gateRequestFromApproval(r *ApprovalRequest) *GateRequest {
	args := map[string]any{}
	_ = json.Unmarshal([]byte(r.ToolInput), &args)
	return &GateRequest{
		ID:          r.ID,
		RunID:       r.SessionID,
		ToolName:    r.ToolName,
		ToolArgs:    args,
		Description: fmt.Sprintf("%s (risk: %s)", r.ToolName, r.RiskLevel),
		RiskLevel:   string(r.RiskLevel),
		Status:      r.Status,
		CreatedAt:   r.CreatedAt,
	}
}

// GetPendingApprovals returns pending infra requests (runID is ignored:
// the persistent gate scopes by its fixed session).
func (g *PersistentGate) GetPendingApprovals(runID string) []*GateRequest {
	return g.GetAllPendingApprovals()
}

// GetAllPendingApprovals returns all pending infra approval requests.
func (g *PersistentGate) GetAllPendingApprovals() []*GateRequest {
	reqs, err := g.manager.ListPendingRequests(context.Background(), PersistentGateSessionID)
	if err != nil {
		return nil
	}
	out := make([]*GateRequest, 0, len(reqs))
	for _, r := range reqs {
		out = append(out, gateRequestFromApproval(r))
	}
	return out
}

// Approve resolves a pending request; the blocked agent unblocks on its
// next poll.
func (g *PersistentGate) Approve(id string, by string) error {
	return g.manager.Approve(context.Background(), id, by, "")
}

// Reject resolves a pending request negatively.
func (g *PersistentGate) Reject(id string, by string, reason string) error {
	return g.manager.Reject(context.Background(), id, by, reason)
}

// CancelForRun cancels all pending infra requests.
func (g *PersistentGate) CancelForRun(runID string) int {
	reqs, err := g.manager.ListPendingRequests(context.Background(), PersistentGateSessionID)
	if err != nil {
		return 0
	}
	n := 0
	for _, r := range reqs {
		if err := g.manager.Cancel(context.Background(), r.ID, "cancelled"); err == nil {
			n++
		}
	}
	return n
}
