package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/service"
)

// Release-governance tools (Phase 7, executor handoff) expose the approved-work
// queue and the report-back surface to external MCP clients (Codex/Claude)
// without booting the daemon. They are a THIN ADAPTER over
// internal/governance/service: each handler opens the governance store fresh,
// builds a stateless Service, calls ONE Service method, and renders the result.
// All validation/policy/store sequencing lives in the Service — never here.
//
// AUTHORIZATION (mirrors backlog.go exactly): like
// backlog_claim_story / backlog_complete_task, these tools mutate orchestrator
// bookkeeping (the .openexec governance database) — claims, plan/evidence/PR
// records, decision events — NOT workspace files. They are therefore allowed in
// every permission mode, including read-only chat (see the
// openexec_*-governance case in broker.go Authorize and isControlPlaneTool).
// This is the SAME bookkeeping exception the backlog tools carry; it grants no
// authority beyond reading/writing governance state. The one privileged action
// in this plane — human sign-off/approval — is deliberately NOT exposed here;
// approval stays behind approval_decide in an operator session (approvals.go).

// errNoGovernance indicates the workspace has no .openexec project directory —
// no governance database can exist yet.
var errNoGovernance = errors.New("no OpenExec project in this workspace — run `openexec init` first to create a governance database")

// govHandle holds the lazily-opened governance store and its owning *sql.DB.
// The SQLiteStore reads live from SQLite on every query, so caching the handle
// is safe: handlers re-query through it and never cache results across calls
// (mcp-serve is long-lived and other processes write the same database).
type govHandle struct {
	db    *sql.DB
	store *governance.SQLiteStore
}

// governanceStore lazily opens the governance store rooted at the workspace.
func (s *Server) governanceStore() (*governance.SQLiteStore, error) {
	s.govMu.Lock()
	defer s.govMu.Unlock()

	if s.govHandle != nil {
		return s.govHandle.store, nil
	}
	if len(s.workspaceRoots) == 0 || s.workspaceRoots[0] == "" {
		return nil, fmt.Errorf("no workspace root configured")
	}
	if _, err := os.Stat(filepath.Join(s.workspaceRoots[0], ".openexec")); os.IsNotExist(err) {
		return nil, errNoGovernance
	}
	db, store, err := governance.Open(s.workspaceRoots[0])
	if err != nil {
		return nil, fmt.Errorf("open governance store: %w", err)
	}
	s.govHandle = &govHandle{db: db, store: store}
	return store, nil
}

// closeGovernance releases the lazily-opened governance store and its owning
// *sql.DB. Safe to call multiple times and when no handle was ever opened.
// Wire this into a server teardown hook when one exists (see the note in the
// hardening report: at present mcp-serve has no explicit shutdown path — the
// process exits on stdin EOF and the OS reclaims the fd, matching how
// backlogMgr and the approval manager are handled).
func (s *Server) closeGovernance() error {
	s.govMu.Lock()
	defer s.govMu.Unlock()
	if s.govHandle == nil {
		return nil
	}
	err := s.govHandle.store.Close()
	s.govHandle = nil
	return err
}

// governanceService builds a fresh, stateless Service over the cached store.
// opts wires optional dependencies (e.g. a canned Completer for record_plan).
func (s *Server) governanceService(opts service.Options) (*service.Service, error) {
	store, err := s.governanceStore()
	if err != nil {
		return nil, err
	}
	return service.NewService(store, opts), nil
}

// toolCtx returns the server's tracing context or a background context.
func (s *Server) toolCtx() context.Context {
	if s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// cannedCompleter is an ai.Completer that returns a fixed response, ignoring the
// prompt. record_plan / request_revision use it to feed an EXTERNALLY-produced
// plan or review through the Service's existing Triage / ReviewPlan paths — so
// the version bump, status transition, and decision-event recording all run
// exactly once, in the shared Service code, with no live AI call.
type cannedCompleter struct{ response string }

func (c cannedCompleter) Complete(_ context.Context, _ string) (string, error) {
	return c.response, nil
}

// changeSummary renders a compact view of a change record for tool output.
func changeSummary(ch *governance.ChangeRecord) map[string]interface{} {
	return map[string]interface{}{
		"id":         ch.ID,
		"title":      ch.Title,
		"kind":       ch.Kind,
		"risk":       ch.Risk,
		"status":     ch.Status,
		"project_id": ch.ProjectID,
		"release_id": ch.ReleaseID,
	}
}

// --- Tool definitions ---

func ListReleasesToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_list_releases",
		"description": "List governance releases with id, name, owner, status, and risk. Use this to see which releases exist and which are approved or implementing before picking up work. Optionally filter by status.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status": map[string]interface{}{
					"type":        "string",
					"description": "Only return releases with this status (e.g. draft, approved, implementing, deployed). Omit to list all releases.",
				},
			},
		},
	}
}

func ListApprovedWorkToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_list_approved_work",
		"description": "List change records an executor may pick up: approved for AI, belonging to an approved or implementing release, and not actively claimed. Unapproved or unreleased work is never returned. This is the queue Codex/Claude should poll.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"project_id": map[string]interface{}{
					"type":        "string",
					"description": "Scope the queue to a single project id. Omit to list approved work across all projects.",
				},
			},
		},
	}
}

func GetWorkBriefToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_get_work_brief",
		"description": "Generate the self-contained executor brief for a change: approved scope, acceptance criteria, verification plan, and the reporting contract. Generated from stored records only. Call this after claiming work to learn exactly what is allowed.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record to brief, e.g. \"CHANGE-123\".",
				},
				"repo_path": map[string]interface{}{
					"type":        "string",
					"description": "Absolute path to the local repository the work targets, included verbatim in the brief. Optional.",
				},
			},
			"required": []string{"change_id"},
		},
	}
}

func ClaimWorkToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_claim_work",
		"description": "Claim approved work for an executor with a lease. Refused if the release is not implementable, the approved plan is stale, or another active executor already holds the change. Advances an approved change to implementing on first claim.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record to claim, e.g. \"CHANGE-123\".",
				},
				"agent": map[string]interface{}{
					"type":        "string",
					"description": "Name of the executor claiming the work, e.g. \"codex\" or \"claude\".",
				},
				"lease_seconds": map[string]interface{}{
					"type":        "integer",
					"description": "How long the claim is held before it expires and the work can be re-claimed. Defaults to 1800 (30 minutes).",
					"minimum":     1,
				},
			},
			"required": []string{"change_id", "agent"},
		},
	}
}

func RecordPlanToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_record_plan",
		"description": "Record an externally-produced triage plan for a change: summary, kind, risk, acceptance criteria, and verification plan. Stores it as a new proposal version (invalidating any prior approval) and moves the change to plan_ready. This records YOUR plan; it does not run a separate planner AI.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record the plan applies to, e.g. \"CHANGE-123\".",
				},
				"actor": map[string]interface{}{
					"type":        "string",
					"description": "Name of the planner recording this plan (the agent or model), recorded in decision history.",
				},
				"summary": map[string]interface{}{
					"type":        "string",
					"description": "One-paragraph summary of the proposed change and its intent.",
				},
				"kind": map[string]interface{}{
					"type":        "string",
					"description": "Change kind: one of bug, feature, docs, ops, support_question, security, reliability. Optional.",
				},
				"risk": map[string]interface{}{
					"type":        "string",
					"description": "Risk tier: one of low, medium, high, critical. Drives approval policy. Optional.",
				},
				"acceptance_criteria": map[string]interface{}{
					"type":        "array",
					"description": "Concrete, checkable conditions the change must satisfy. Required before the plan can later be approved.",
					"items":       map[string]interface{}{"type": "string"},
				},
				"verification_plan": map[string]interface{}{
					"type":        "array",
					"description": "Exact steps or commands that prove the change works. Required before the plan can later be approved.",
					"items":       map[string]interface{}{"type": "string"},
				},
				"implementation_notes": map[string]interface{}{
					"type":        "string",
					"description": "Free-form notes on how the change should be implemented. Optional.",
				},
				"repo_context": map[string]interface{}{
					"type":        "string",
					"description": "Optional repository context string carried into the stored plan for reference.",
				},
			},
			"required": []string{"change_id", "actor", "summary", "acceptance_criteria", "verification_plan"},
		},
	}
}

func RequestRevisionToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_request_revision",
		"description": "Request changes to a change's current plan as a reviewer. Records the concerns as a review decision and moves the change to changes_requested. A reviewer may request changes but can never approve — approval stays with a human authority.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record whose plan needs revision, e.g. \"CHANGE-123\".",
				},
				"authority": map[string]interface{}{
					"type":        "string",
					"description": "Review authority id requesting changes (seeded: pm, developer, bugbot, tester_ai, security_ai, ci_verifier).",
				},
				"actor": map[string]interface{}{
					"type":        "string",
					"description": "Name of the reviewer recording this request, stored in decision history.",
				},
				"concerns": map[string]interface{}{
					"type":        "array",
					"description": "Specific concerns or required changes. Provide at least one concern or a comment.",
					"items":       map[string]interface{}{"type": "string"},
				},
				"comment": map[string]interface{}{
					"type":        "string",
					"description": "Free-form explanation of why the plan needs changes. Used if concerns is empty.",
				},
			},
			"required": []string{"change_id", "authority", "actor"},
		},
	}
}

func RecordPRToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_record_pr",
		"description": "Record the pull request opened for a change: pins the PR URL and branch and advances the change from implementing to pr_open. Call this once you have opened the PR for claimed work.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record the PR implements, e.g. \"CHANGE-123\".",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Full URL of the pull request, e.g. https://github.com/org/repo/pull/123.",
				},
				"branch": map[string]interface{}{
					"type":        "string",
					"description": "Name of the branch the PR is built from. Optional but recommended.",
				},
			},
			"required": []string{"change_id", "url"},
		},
	}
}

func RecordTestEvidenceToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_record_test_evidence",
		"description": "Attach structured test/CI/review/deploy evidence to a change. Evidence is required before a change can move to ready_for_test or done. Record the command you ran and its result so the handoff is built from facts, not claims.",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record the evidence belongs to, e.g. \"CHANGE-123\".",
				},
				"kind": map[string]interface{}{
					"type":        "string",
					"description": "Evidence kind: one of test, ci, review, deploy, monitoring, manual. Defaults to test.",
				},
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Where the evidence came from: one of cli, github, jira, webhook, human. Defaults to cli.",
				},
				"summary": map[string]interface{}{
					"type":        "string",
					"description": "Short summary of what was run and the outcome, e.g. \"go test ./... — all passing\".",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "Optional link to the full evidence (CI run, log, report).",
				},
				"raw": map[string]interface{}{
					"type":        "string",
					"description": "Optional raw output or a pointer to it, captured for the audit trail.",
				},
			},
			"required": []string{"change_id", "summary"},
		},
	}
}

func GenerateHandoffToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_generate_handoff",
		"description": "Generate an audience-targeted communication artifact for a release from its stored records (never from AI claims) and persist it. Audiences: tester (handoff with evidence), pm (status summary), customer (release notes).",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"release_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the release to generate communication for, e.g. \"R-2026-07\".",
				},
				"audience": map[string]interface{}{
					"type":        "string",
					"description": "Target audience: one of tester, pm, customer.",
					"enum":        []string{"tester", "pm", "customer"},
				},
				"env": map[string]interface{}{
					"type":        "string",
					"description": "Deployment environment for the tester handoff, e.g. \"staging\". Ignored for pm/customer.",
				},
				"version": map[string]interface{}{
					"type":        "string",
					"description": "Release version for the tester handoff, e.g. \"2026.07.1\". Ignored for pm/customer.",
				},
			},
			"required": []string{"release_id", "audience"},
		},
	}
}

func RequestDoneToolDef() map[string]interface{} {
	return map[string]interface{}{
		"name":        "openexec_request_done",
		"description": "Mark a change as done after it is ready for test and has verification evidence. Refused unless the authority holds the mark_done permission, without evidence, and for medium+ risk if that authority also proposed and approved the change (separation of duties).",
		"inputSchema": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"change_id": map[string]interface{}{
					"type":        "string",
					"description": "Identifier of the change record to mark done, e.g. \"CHANGE-123\".",
				},
				"authority": map[string]interface{}{
					"type":        "string",
					"description": "Review authority ID marking the work done; must hold the mark_done permission (e.g. \"tester_ai\", \"ci_verifier\", \"pm\"). Recorded in decision history.",
				},
			},
			"required": []string{"change_id", "authority"},
		},
	}
}

// --- Request structs ---

type ListReleasesRequest struct {
	Status string `json:"status,omitempty"`
}

func (r *ListReleasesRequest) Validate() error { return nil }

type ListApprovedWorkRequest struct {
	ProjectID string `json:"project_id,omitempty"`
}

func (r *ListApprovedWorkRequest) Validate() error { return nil }

type GetWorkBriefRequest struct {
	ChangeID string `json:"change_id"`
	RepoPath string `json:"repo_path,omitempty"`
}

func (r *GetWorkBriefRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	return nil
}

type ClaimWorkRequest struct {
	ChangeID     string `json:"change_id"`
	Agent        string `json:"agent"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

func (r *ClaimWorkRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	if strings.TrimSpace(r.Agent) == "" {
		return &ValidationError{Field: "agent", Message: "agent is required to claim work"}
	}
	if r.LeaseSeconds < 0 {
		return &ValidationError{Field: "lease_seconds", Message: "lease_seconds must not be negative"}
	}
	if r.LeaseSeconds == 0 {
		r.LeaseSeconds = 1800
	}
	return nil
}

type RecordPlanRequest struct {
	ChangeID            string   `json:"change_id"`
	Actor               string   `json:"actor"`
	Summary             string   `json:"summary"`
	Kind                string   `json:"kind,omitempty"`
	Risk                string   `json:"risk,omitempty"`
	AcceptanceCriteria  []string `json:"acceptance_criteria"`
	VerificationPlan    []string `json:"verification_plan"`
	ImplementationNotes string   `json:"implementation_notes,omitempty"`
	RepoContext         string   `json:"repo_context,omitempty"`
}

func (r *RecordPlanRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	if strings.TrimSpace(r.Actor) == "" {
		return &ValidationError{Field: "actor", Message: "actor is required to record a plan"}
	}
	if strings.TrimSpace(r.Summary) == "" {
		return &ValidationError{Field: "summary", Message: "summary is required"}
	}
	if len(r.AcceptanceCriteria) == 0 {
		return &ValidationError{Field: "acceptance_criteria", Message: "at least one acceptance criterion is required (approval needs it)"}
	}
	if len(r.VerificationPlan) == 0 {
		return &ValidationError{Field: "verification_plan", Message: "at least one verification step is required (approval needs it)"}
	}
	return nil
}

type RequestRevisionRequest struct {
	ChangeID  string   `json:"change_id"`
	Authority string   `json:"authority"`
	Actor     string   `json:"actor"`
	Concerns  []string `json:"concerns,omitempty"`
	Comment   string   `json:"comment,omitempty"`
}

func (r *RequestRevisionRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	if strings.TrimSpace(r.Authority) == "" {
		return &ValidationError{Field: "authority", Message: "authority is required (e.g. bugbot, developer, pm)"}
	}
	if strings.TrimSpace(r.Actor) == "" {
		return &ValidationError{Field: "actor", Message: "actor is required to request a revision"}
	}
	if len(r.Concerns) == 0 && strings.TrimSpace(r.Comment) == "" {
		return &ValidationError{Field: "concerns", Message: "provide at least one concern or a comment explaining the requested revision"}
	}
	return nil
}

type RecordPRRequest struct {
	ChangeID string `json:"change_id"`
	URL      string `json:"url"`
	Branch   string `json:"branch,omitempty"`
}

func (r *RecordPRRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	if strings.TrimSpace(r.URL) == "" {
		return &ValidationError{Field: "url", Message: "url is required"}
	}
	return nil
}

type RecordTestEvidenceRequest struct {
	ChangeID string `json:"change_id"`
	Kind     string `json:"kind,omitempty"`
	Source   string `json:"source,omitempty"`
	Summary  string `json:"summary"`
	URL      string `json:"url,omitempty"`
	Raw      string `json:"raw,omitempty"`
}

func (r *RecordTestEvidenceRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	if strings.TrimSpace(r.Summary) == "" {
		return &ValidationError{Field: "summary", Message: "summary is required"}
	}
	if strings.TrimSpace(r.Kind) == "" {
		r.Kind = governance.EvidenceKindTest
	}
	if strings.TrimSpace(r.Source) == "" {
		r.Source = governance.EvidenceSourceCLI
	}
	return nil
}

type GenerateHandoffRequest struct {
	ReleaseID string `json:"release_id"`
	Audience  string `json:"audience"`
	Env       string `json:"env,omitempty"`
	Version   string `json:"version,omitempty"`
}

func (r *GenerateHandoffRequest) Validate() error {
	if strings.TrimSpace(r.ReleaseID) == "" {
		return &ValidationError{Field: "release_id", Message: "release_id is required"}
	}
	if strings.TrimSpace(r.Audience) == "" {
		return &ValidationError{Field: "audience", Message: "audience is required (tester, pm, or customer)"}
	}
	return nil
}

type RequestDoneRequest struct {
	ChangeID  string `json:"change_id"`
	Authority string `json:"authority"`
}

func (r *RequestDoneRequest) Validate() error {
	if strings.TrimSpace(r.ChangeID) == "" {
		return &ValidationError{Field: "change_id", Message: "change_id is required"}
	}
	if strings.TrimSpace(r.Authority) == "" {
		return &ValidationError{Field: "authority", Message: "authority is required to mark work done"}
	}
	return nil
}

// --- Handlers ---

func (s *Server) handleListReleases(req Request, params toolsCallParams) {
	var args ListReleasesRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_list_releases arguments: %v", err))
			return
		}
	}

	store, err := s.governanceStore()
	if errors.Is(err, errNoGovernance) {
		s.writeResult(req.ID, map[string]interface{}{
			"content":  []interface{}{map[string]interface{}{"type": "text", "text": "No governance releases — this workspace has no OpenExec project yet."}},
			"releases": []interface{}{},
		})
		return
	}
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	ctx := s.toolCtx()
	var releases []*governance.GovernanceRelease
	if strings.TrimSpace(args.Status) != "" {
		releases, err = store.ListReleasesByStatus(ctx, args.Status)
	} else {
		releases, err = store.ListReleases(ctx)
	}
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("list releases: %v", err))
		return
	}

	out := []map[string]interface{}{}
	var lines []string
	for _, rel := range releases {
		out = append(out, map[string]interface{}{
			"id":              rel.ID,
			"name":            rel.Name,
			"owner":           rel.Owner,
			"status":          rel.Status,
			"risk":            rel.Risk,
			"approved_for_ai": rel.ApprovedForAI,
		})
		lines = append(lines, fmt.Sprintf("- %s [%s] %s", rel.ID, rel.Status, rel.Name))
	}

	text := fmt.Sprintf("%d release(s)", len(out))
	if len(lines) > 0 {
		text += "\n" + strings.Join(lines, "\n")
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content":  []interface{}{map[string]interface{}{"type": "text", "text": text}},
		"releases": out,
	})
}

func (s *Server) handleListApprovedWork(req Request, params toolsCallParams) {
	var args ListApprovedWorkRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_list_approved_work arguments: %v", err))
			return
		}
	}

	svc, err := s.governanceService(service.Options{})
	if errors.Is(err, errNoGovernance) {
		s.writeResult(req.ID, map[string]interface{}{
			"content": []interface{}{map[string]interface{}{"type": "text", "text": "No approved work — this workspace has no OpenExec project yet."}},
			"work":    []interface{}{},
		})
		return
	}
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	work, err := svc.ListApprovedWork(s.toolCtx(), args.ProjectID)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	out := []map[string]interface{}{}
	var lines []string
	for _, ch := range work {
		out = append(out, changeSummary(ch))
		lines = append(lines, fmt.Sprintf("- %s [%s/%s] %s", ch.ID, ch.Kind, ch.Risk, ch.Title))
	}

	text := fmt.Sprintf("%d approved, claimable change(s)", len(out))
	if len(lines) > 0 {
		text += "\n" + strings.Join(lines, "\n")
	} else {
		text += " — nothing to pick up right now."
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": text}},
		"work":    out,
	})
}

func (s *Server) handleGetWorkBrief(req Request, params toolsCallParams) {
	var args GetWorkBriefRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_get_work_brief arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	svc, err := s.governanceService(service.Options{})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	brief, err := svc.WorkBrief(s.toolCtx(), args.ChangeID, args.RepoPath)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content":   []interface{}{map[string]interface{}{"type": "text", "text": brief}},
		"change_id": args.ChangeID,
		"brief":     brief,
	})
}

func (s *Server) handleClaimWork(req Request, params toolsCallParams) {
	var args ClaimWorkRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_claim_work arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	svc, err := s.governanceService(service.Options{})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	lease := time.Duration(args.LeaseSeconds) * time.Second
	if err := svc.ClaimWork(s.toolCtx(), args.ChangeID, args.Agent, lease); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Claimed %s for %s (lease %s). Call openexec_get_work_brief for the approved scope.", args.ChangeID, args.Agent, lease),
		}},
		"change_id": args.ChangeID,
		"agent":     args.Agent,
	})
}

func (s *Server) handleRecordPlan(req Request, params toolsCallParams) {
	var args RecordPlanRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_record_plan arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	// Feed the externally-produced plan through the Service's Triage path via a
	// canned Completer: this reuses the version bump, approval invalidation,
	// status transition, and proposed-decision recording with no live AI call.
	planner := ai.PlannerOutput{
		Summary:             args.Summary,
		Kind:                args.Kind,
		Risk:                args.Risk,
		AcceptanceCriteria:  args.AcceptanceCriteria,
		VerificationPlan:    args.VerificationPlan,
		ImplementationNotes: args.ImplementationNotes,
	}
	encoded, err := json.Marshal(planner)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("encode plan: %v", err))
		return
	}

	svc, err := s.governanceService(service.Options{Completer: cannedCompleter{response: string(encoded)}})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	out, err := svc.Triage(s.toolCtx(), args.ChangeID, args.RepoContext, args.Actor)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Recorded plan for %s (now plan_ready). Summary: %s", args.ChangeID, out.Summary),
		}},
		"change_id": args.ChangeID,
		"status":    governance.ChangeStatusPlanReady,
	})
}

func (s *Server) handleRequestRevision(req Request, params toolsCallParams) {
	var args RequestRevisionRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_request_revision arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	concerns := args.Concerns
	if len(concerns) == 0 && strings.TrimSpace(args.Comment) != "" {
		concerns = []string{strings.TrimSpace(args.Comment)}
	}
	// Feed the reviewer verdict through the Service's ReviewPlan path via a
	// canned Completer: request_changes is recorded as a review decision and the
	// change moves to changes_requested. A reviewer can never approve.
	reviewer := ai.ReviewerOutput{
		Decision: ai.ReviewerRequestChanges,
		Concerns: concerns,
	}
	encoded, err := json.Marshal(reviewer)
	if err != nil {
		s.writeToolError(req.ID, fmt.Sprintf("encode review: %v", err))
		return
	}

	svc, err := s.governanceService(service.Options{Completer: cannedCompleter{response: string(encoded)}})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	if _, err := svc.ReviewPlan(s.toolCtx(), args.ChangeID, args.Authority, args.Actor); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	// The Service downgrades request_changes to a plain comment when the reviewer
	// lacks the request_changes permission — in that case the status does NOT move
	// to changes_requested. Re-read the change and report its ACTUAL status rather
	// than asserting changes_requested unconditionally.
	status := governance.ChangeStatusChangesRequested
	if store, serr := s.governanceStore(); serr == nil {
		if ch, cerr := store.GetChangeRecord(s.toolCtx(), args.ChangeID); cerr == nil {
			status = ch.Status
		}
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Recorded revision request on %s by %s (now %s).", args.ChangeID, args.Authority, status),
		}},
		"change_id": args.ChangeID,
		"status":    status,
	})
}

func (s *Server) handleRecordPR(req Request, params toolsCallParams) {
	var args RecordPRRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_record_pr arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	svc, err := s.governanceService(service.Options{})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	if err := svc.RecordPR(s.toolCtx(), args.ChangeID, args.URL, args.Branch); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Recorded PR %s for %s (now pr_open).", args.URL, args.ChangeID),
		}},
		"change_id": args.ChangeID,
		"status":    governance.ChangeStatusPROpen,
	})
}

func (s *Server) handleRecordTestEvidence(req Request, params toolsCallParams) {
	var args RecordTestEvidenceRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_record_test_evidence arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	svc, err := s.governanceService(service.Options{})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	// Provenance: evidence recorded over the agent plane is stamped source=agent.
	// An agent session cannot claim source=human/ci/github — the source field is
	// self-report and must not read as independently verified. (Binding evidence
	// to an observed CI run is future work.)
	if err := svc.RecordEvidence(s.toolCtx(), args.ChangeID, args.Kind, governance.EvidenceSourceAgent, args.Summary, args.URL, args.Raw); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Recorded %s evidence (source=agent) for %s.", args.Kind, args.ChangeID),
		}},
		"change_id": args.ChangeID,
		"kind":      args.Kind,
	})
}

func (s *Server) handleGenerateHandoff(req Request, params toolsCallParams) {
	var args GenerateHandoffRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_generate_handoff arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	svc, err := s.governanceService(service.Options{})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	body, err := svc.GenerateHandoff(s.toolCtx(), args.ReleaseID, args.Audience, args.Env, args.Version)
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content":    []interface{}{map[string]interface{}{"type": "text", "text": body}},
		"release_id": args.ReleaseID,
		"audience":   args.Audience,
		"body":       body,
	})
}

func (s *Server) handleRequestDone(req Request, params toolsCallParams) {
	var args RequestDoneRequest
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			s.writeError(req.ID, -32602, fmt.Sprintf("invalid openexec_request_done arguments: %v", err))
			return
		}
	}
	if err := args.Validate(); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}

	svc, err := s.governanceService(service.Options{})
	if err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	if err := svc.MarkDone(s.toolCtx(), args.ChangeID, args.Authority); err != nil {
		s.writeToolError(req.ID, err.Error())
		return
	}
	s.writeResult(req.ID, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": fmt.Sprintf("Marked %s done (by %s).", args.ChangeID, args.Authority),
		}},
		"change_id": args.ChangeID,
		"status":    governance.ChangeStatusDone,
	})
}
