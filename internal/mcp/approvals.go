package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/approval"
)

// Operator-session approval tools (Phase 3 of the SRE roadmap): the stdio
// sign-off channel. When an apply-class infra command blocks awaiting
// approval, the operator opens a SECOND light-mode client with
// OPENEXEC_OPERATOR_SESSION=1 and resolves the request over MCP — or uses
// `openexec approve list/yes/no --local` from a terminal.
//
// SECURITY: these tools must NEVER be available to the agent session that
// triggers the applies — an agent that can approve its own requests has no
// approval gate at all. They exist only when the server was started as an
// operator session (the env var is read once at construction), are not
// advertised otherwise, and `backlog_complete_task`-style reuse is
// deliberately impossible: approval is its own tool with its own broker
// rule.

func isApprovalTool(name string) bool {
	return name == "approval_list" || name == "approval_decide"
}

// SetApprovalManager wires the shared approvals store for operator tools.
func (s *Server) SetApprovalManager(mgr *approval.Manager) {
	s.approvalMgr = mgr
}

type ApprovalDecideRequest struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
}

func ApprovalListToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "approval_list",
		"description": "List infra approval requests pending operator sign-off: id, tool, risk level, requested arguments (including any detected destructive changes), and age. Operator sessions only.",
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func ApprovalDecideToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "approval_decide",
		"description": "Approve or reject a pending infra approval request by id. The blocked apply-class command unblocks (or aborts) within seconds. Operator sessions only.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"request_id": map[string]interface{}{
					"type":        "string",
					"description": "The approval request id from approval_list.",
				},
				"decision": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"approve", "reject"},
					"description": "Whether to approve or reject the pending request.",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Optional reason recorded in the audit trail (required context for rejections).",
				},
			},
			"required": []string{"request_id", "decision"},
		},
	}
}

func (s *Server) approvalToolsEnabled(req Request) bool {
	if !s.operatorSession {
		s.writeToolError(req.ID, "approval tools are only available in an operator session (start the server with OPENEXEC_OPERATOR_SESSION=1)")
		return false
	}
	if s.approvalMgr == nil {
		s.writeToolError(req.ID, "no approvals store is wired on this server (is the infra allowlist enabled?)")
		return false
	}
	return true
}

func (s *Server) handleApprovalList(req Request, params toolsCallParams) {
	if !s.approvalToolsEnabled(req) {
		return
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	pending, err := s.approvalMgr.ListPendingRequests(ctx, approval.PersistentGateSessionID)
	if err != nil {
		s.writeToolError(req.ID, "list approvals: "+err.Error())
		return
	}
	if len(pending) == 0 {
		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"type": "text", "text": "No pending approval requests."}},
			"count":   0,
		})
		return
	}
	var b strings.Builder
	items := make([]interface{}, 0, len(pending))
	for _, r := range pending {
		fmt.Fprintf(&b, "%s  tool=%s risk=%s requested=%s\n  args: %s\n",
			r.ID, r.ToolName, r.RiskLevel, r.CreatedAt.Format("2006-01-02 15:04:05"), r.ToolInput)
		var args map[string]interface{}
		_ = json.Unmarshal([]byte(r.ToolInput), &args)
		items = append(items, map[string]interface{}{
			"request_id": r.ID,
			"tool":       r.ToolName,
			"risk":       string(r.RiskLevel),
			"args":       args,
			"created_at": r.CreatedAt,
		})
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": b.String()}},
		"count":   len(pending),
		"pending": items,
	})
}

func (s *Server) handleApprovalDecide(req Request, params toolsCallParams) {
	if !s.approvalToolsEnabled(req) {
		return
	}
	var r ApprovalDecideRequest
	if err := json.Unmarshal(params.Arguments, &r); err != nil {
		s.writeToolError(req.ID, "invalid arguments: "+err.Error())
		return
	}
	ctx := s.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	switch r.Decision {
	case "approve":
		err = s.approvalMgr.Approve(ctx, r.RequestID, "operator-mcp", r.Reason)
	case "reject":
		err = s.approvalMgr.Reject(ctx, r.RequestID, "operator-mcp", r.Reason)
	default:
		s.writeToolError(req.ID, fmt.Sprintf("decision must be \"approve\" or \"reject\", got %q", r.Decision))
		return
	}
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("%s %s: %v", r.Decision, r.RequestID, err))
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Request %s %sd. The blocked command will observe the decision within seconds.", r.RequestID, r.Decision),
		}},
		"request_id": r.RequestID,
		"decision":   r.Decision,
	})
}
