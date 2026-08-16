package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrImmutableValidationPlan = errors.New("accepted validation plan is immutable")
	ErrEvidenceStateMismatch   = errors.New("evidence was produced against a different repository state")
	ErrEvidenceStepMismatch    = errors.New("evidence status does not match the structured run step")
	ErrInvalidItemDecision     = errors.New("invalid validation item decision")
	ErrValidationProposalStale = errors.New("validation proposal is stale; recompute impact before accepting it")
	ErrCompletionStateMoved    = errors.New("repository state moved; completion cannot be frozen")
	ErrCompletionEvidenceEmpty = errors.New("validation evidence is missing; completion cannot be frozen")
)

type TaskGraphBinding struct {
	TaskID            string `json:"task_id"`
	Kind              string `json:"binding_kind"`
	GenerationID      string `json:"generation_id"`
	BaseCommit        string `json:"base_commit"`
	WorktreeStateHash string `json:"worktree_state_hash"`
	PatchHash         string `json:"patch_hash,omitempty"`
}

type ValidationPlanRevision struct {
	ID                string                  `json:"id"`
	TaskID            string                  `json:"task_id"`
	RunID             string                  `json:"run_id,omitempty"`
	Revision          int                     `json:"revision"`
	GenerationID      string                  `json:"generation_id"`
	WorktreeStateHash string                  `json:"worktree_state_hash"`
	PatchHash         string                  `json:"patch_hash,omitempty"`
	ImpactQuery       ValidationImpactQuery   `json:"impact_query"`
	ImpactSummary     ValidationImpactSummary `json:"impact_summary"`
	SourceRevisionID  string                  `json:"source_revision_id,omitempty"`
	Status            string                  `json:"status"`
	Items             []ValidationItem        `json:"items"`
	CreatedAt         time.Time               `json:"created_at"`
	AcceptedAt        *time.Time              `json:"accepted_at,omitempty"`
}

type ValidationImpactQuery struct {
	Files     []string `json:"files"`
	SymbolIDs []string `json:"symbol_ids"`
	MaxDepth  int      `json:"max_depth"`
}

type ValidationImpactSummary struct {
	ChangedSymbolIDs []string `json:"changed_symbol_ids"`
	AffectedNodeIDs  []string `json:"affected_node_ids"`
	RelatedTestFiles []string `json:"related_test_files"`
	UnresolvedFiles  []string `json:"unresolved_files"`
	Unresolved       []string `json:"unresolved"`
	Limitations      []string `json:"limitations"`
	Truncated        bool     `json:"truncated"`
}

type ValidationItem struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Disposition string   `json:"disposition"`
	Requirement string   `json:"requirement"`
	Criterion   string   `json:"criterion"`
	CommandArgv []string `json:"command_argv,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	GraphPaths  []string `json:"graph_paths,omitempty"`
	Limitations []string `json:"limitations,omitempty"`
}

// ValidationItemDecision is the explicit policy decision applied when an
// advisory proposal becomes execution authority. Suggestions not named by a
// decision are rejected instead of silently becoming required gates.
type ValidationItemDecision struct {
	ID          string `json:"id"`
	Disposition string `json:"disposition"`
	Requirement string `json:"requirement"`
}

type ValidationEvidenceLink struct {
	ValidationItemID  string `json:"validation_item_id"`
	RunID             string `json:"run_id"`
	RunStepID         string `json:"run_step_id"`
	ArtifactHash      string `json:"artifact_hash,omitempty"`
	WorktreeStateHash string `json:"worktree_state_hash"`
	PatchHash         string `json:"patch_hash,omitempty"`
	Status            string `json:"status"`
}

type CompletionClaim struct {
	ID                  string   `json:"id"`
	ValidationItemID    string   `json:"validation_item_id"`
	Predicate           string   `json:"predicate"`
	Scope               string   `json:"scope"`
	Status              string   `json:"status"`
	RepositoryStateHash string   `json:"repository_state_hash"`
	EvidenceArtifactIDs []string `json:"evidence_artifact_ids,omitempty"`
	Criterion           string   `json:"criterion"`
	Requirement         string   `json:"requirement"`
}

type CompletionReport struct {
	ID             string            `json:"id"`
	PlanRevisionID string            `json:"plan_revision_id"`
	Verified       []CompletionClaim `json:"verified"`
	NotVerified    []CompletionClaim `json:"not_verified"`
	CanComplete    bool              `json:"can_complete"`
	CreatedAt      time.Time         `json:"created_at"`
}

type verificationQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) BindTaskGraphState(ctx context.Context, binding TaskGraphBinding) error {
	if binding.TaskID == "" || binding.GenerationID == "" || binding.WorktreeStateHash == "" {
		return fmt.Errorf("task graph binding requires task, generation, and worktree state")
	}
	if binding.Kind != "planning" && binding.Kind != "validation" {
		return fmt.Errorf("invalid task graph binding kind %q", binding.Kind)
	}
	if binding.Kind == "validation" && binding.PatchHash == "" {
		return fmt.Errorf("validation graph binding requires patch hash")
	}
	var generationState string
	if err := s.db.QueryRowContext(ctx, `SELECT worktree_state_hash FROM graph_generations WHERE id = ?`, binding.GenerationID).Scan(&generationState); err != nil {
		return fmt.Errorf("load binding generation: %w", err)
	}
	if generationState != binding.WorktreeStateHash {
		return fmt.Errorf("binding worktree state does not match graph generation")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO task_graph_bindings (task_id, binding_kind, generation_id, base_commit, worktree_state_hash, patch_hash) VALUES (?, ?, ?, ?, ?, ?)`, binding.TaskID, binding.Kind, binding.GenerationID, binding.BaseCommit, binding.WorktreeStateHash, binding.PatchHash)
	return err
}

// CreateValidationPlanRevision creates a new immutable revision. Accepted
// revisions supersede earlier accepted revisions for the same task atomically.
func (s *Store) CreateValidationPlanRevision(ctx context.Context, plan ValidationPlanRevision) (ValidationPlanRevision, error) {
	if plan.TaskID == "" || plan.GenerationID == "" || plan.WorktreeStateHash == "" {
		return ValidationPlanRevision{}, fmt.Errorf("validation plan requires task, generation, and worktree state")
	}
	if plan.Status != "proposed" && plan.Status != "accepted" {
		return ValidationPlanRevision{}, fmt.Errorf("invalid validation plan status %q", plan.Status)
	}
	var generationState, generationCheckout string
	if err := s.db.QueryRowContext(ctx, `SELECT worktree_state_hash, checkout_id FROM graph_generations WHERE id = ?`, plan.GenerationID).Scan(&generationState, &generationCheckout); err != nil {
		return ValidationPlanRevision{}, fmt.Errorf("load validation generation: %w", err)
	}
	if generationState != plan.WorktreeStateHash {
		return ValidationPlanRevision{}, fmt.Errorf("validation plan state does not match generation")
	}
	for index := range plan.Items {
		if err := validateValidationItem(plan.Items[index], plan.Status); err != nil {
			return ValidationPlanRevision{}, err
		}
		if plan.Items[index].ID == "" {
			plan.Items[index].ID = "validation_item_" + uuid.NewString()
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ValidationPlanRevision{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) + 1 FROM validation_plan_revisions WHERE task_id = ?`, plan.TaskID).Scan(&plan.Revision); err != nil {
		return ValidationPlanRevision{}, err
	}
	if plan.ID == "" {
		plan.ID = "validation_plan_" + uuid.NewString()
	}
	plan.CreatedAt = time.Now().UTC()
	if plan.Status == "accepted" {
		if plan.SourceRevisionID != "" {
			var isCurrent int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM graph_generations WHERE id = ? AND worktree_state_hash = ? AND status = 'current'`, plan.GenerationID, plan.WorktreeStateHash).Scan(&isCurrent); err != nil {
				return ValidationPlanRevision{}, err
			}
			if isCurrent != 1 {
				return ValidationPlanRevision{}, ErrValidationProposalStale
			}
		}
		accepted := plan.CreatedAt
		plan.AcceptedAt = &accepted
		if _, err := tx.ExecContext(ctx, `UPDATE validation_plan_revisions SET status = 'superseded' WHERE task_id = ? AND status = 'accepted' AND generation_id IN (SELECT id FROM graph_generations WHERE checkout_id = ?)`, plan.TaskID, generationCheckout); err != nil {
			return ValidationPlanRevision{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO validation_plan_revisions (id, task_id, run_id, revision, generation_id, worktree_state_hash, patch_hash, impact_query, impact_summary, source_revision_id, status, created_at, accepted_at) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, plan.ID, plan.TaskID, plan.RunID, plan.Revision, plan.GenerationID, plan.WorktreeStateHash, plan.PatchHash, encodeJSON(plan.ImpactQuery, "{}"), encodeJSON(plan.ImpactSummary, "{}"), plan.SourceRevisionID, plan.Status, plan.CreatedAt, plan.AcceptedAt); err != nil {
		return ValidationPlanRevision{}, err
	}
	for _, item := range plan.Items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO validation_items (id, plan_revision_id, source, disposition, requirement, criterion, command_argv, scope, graph_paths, limitations) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.ID, plan.ID, item.Source, item.Disposition, item.Requirement, item.Criterion, encodeJSON(item.CommandArgv, "[]"), item.Scope, encodeJSON(item.GraphPaths, "[]"), encodeJSON(item.Limitations, "[]")); err != nil {
			return ValidationPlanRevision{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ValidationPlanRevision{}, err
	}
	return plan, nil
}

func (s *Store) GetValidationPlanRevision(ctx context.Context, id string) (ValidationPlanRevision, error) {
	var plan ValidationPlanRevision
	var runID sql.NullString
	var acceptedAt sql.NullTime
	var impactQueryJSON, impactSummaryJSON string
	err := s.db.QueryRowContext(ctx, `SELECT id, task_id, run_id, revision, generation_id, worktree_state_hash, patch_hash, impact_query, impact_summary, source_revision_id, status, created_at, accepted_at FROM validation_plan_revisions WHERE id = ?`, id).Scan(
		&plan.ID, &plan.TaskID, &runID, &plan.Revision, &plan.GenerationID, &plan.WorktreeStateHash, &plan.PatchHash, &impactQueryJSON, &impactSummaryJSON, &plan.SourceRevisionID, &plan.Status, &plan.CreatedAt, &acceptedAt,
	)
	if err != nil {
		return ValidationPlanRevision{}, err
	}
	plan.RunID = runID.String
	if acceptedAt.Valid {
		plan.AcceptedAt = &acceptedAt.Time
	}
	if err := json.Unmarshal([]byte(impactQueryJSON), &plan.ImpactQuery); err != nil {
		return ValidationPlanRevision{}, fmt.Errorf("decode validation impact query: %w", err)
	}
	if err := json.Unmarshal([]byte(impactSummaryJSON), &plan.ImpactSummary); err != nil {
		return ValidationPlanRevision{}, fmt.Errorf("decode validation impact summary: %w", err)
	}
	plan.ImpactQuery.Files = append([]string{}, plan.ImpactQuery.Files...)
	plan.ImpactQuery.SymbolIDs = append([]string{}, plan.ImpactQuery.SymbolIDs...)
	plan.ImpactSummary.ChangedSymbolIDs = append([]string{}, plan.ImpactSummary.ChangedSymbolIDs...)
	plan.ImpactSummary.AffectedNodeIDs = append([]string{}, plan.ImpactSummary.AffectedNodeIDs...)
	plan.ImpactSummary.RelatedTestFiles = append([]string{}, plan.ImpactSummary.RelatedTestFiles...)
	plan.ImpactSummary.UnresolvedFiles = append([]string{}, plan.ImpactSummary.UnresolvedFiles...)
	plan.ImpactSummary.Unresolved = append([]string{}, plan.ImpactSummary.Unresolved...)
	plan.ImpactSummary.Limitations = append([]string{}, plan.ImpactSummary.Limitations...)
	plan.Items = make([]ValidationItem, 0)
	rows, err := s.db.QueryContext(ctx, `SELECT id, source, disposition, requirement, criterion, command_argv, scope, graph_paths, limitations FROM validation_items WHERE plan_revision_id = ? ORDER BY id`, id)
	if err != nil {
		return ValidationPlanRevision{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ValidationItem
		var commandArgvJSON, graphPathsJSON, limitationsJSON string
		if err := rows.Scan(&item.ID, &item.Source, &item.Disposition, &item.Requirement, &item.Criterion, &commandArgvJSON, &item.Scope, &graphPathsJSON, &limitationsJSON); err != nil {
			return ValidationPlanRevision{}, err
		}
		if err := json.Unmarshal([]byte(commandArgvJSON), &item.CommandArgv); err != nil {
			return ValidationPlanRevision{}, fmt.Errorf("decode validation command argv: %w", err)
		}
		if err := json.Unmarshal([]byte(graphPathsJSON), &item.GraphPaths); err != nil {
			return ValidationPlanRevision{}, fmt.Errorf("decode validation graph paths: %w", err)
		}
		if err := json.Unmarshal([]byte(limitationsJSON), &item.Limitations); err != nil {
			return ValidationPlanRevision{}, fmt.Errorf("decode validation limitations: %w", err)
		}
		plan.Items = append(plan.Items, item)
	}
	return plan, rows.Err()
}

// AcceptValidationPlanRevision creates one accepted revision from an immutable
// proposal. Retrying the same proposal returns the existing accepted revision.
// Item identities are regenerated for the new authority; the proposal remains
// readable as the historical decision input.
func (s *Store) AcceptValidationPlanRevision(ctx context.Context, proposedID, runID string, decisions []ValidationItemDecision) (ValidationPlanRevision, error) {
	if accepted, err := s.acceptedRevisionForProposal(ctx, proposedID); err == nil {
		return accepted, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ValidationPlanRevision{}, err
	}
	proposed, err := s.GetValidationPlanRevision(ctx, proposedID)
	if err != nil {
		return ValidationPlanRevision{}, err
	}
	if proposed.Status != "proposed" {
		return ValidationPlanRevision{}, fmt.Errorf("validation plan %s is %s, not proposed", proposedID, proposed.Status)
	}
	decisionsByID := make(map[string]ValidationItemDecision, len(decisions))
	for _, decision := range decisions {
		if decision.ID == "" || decisionsByID[decision.ID].ID != "" {
			return ValidationPlanRevision{}, fmt.Errorf("%w: item ids must be non-empty and unique", ErrInvalidItemDecision)
		}
		if decision.Disposition != "accepted" && decision.Disposition != "rejected" {
			return ValidationPlanRevision{}, fmt.Errorf("%w: item %q has disposition %q", ErrInvalidItemDecision, decision.ID, decision.Disposition)
		}
		if decision.Disposition == "accepted" && decision.Requirement != "optional" && decision.Requirement != "required" && decision.Requirement != "blocking" {
			return ValidationPlanRevision{}, fmt.Errorf("%w: accepted item %q has requirement %q", ErrInvalidItemDecision, decision.ID, decision.Requirement)
		}
		decisionsByID[decision.ID] = decision
	}
	for index := range proposed.Items {
		decision, decided := decisionsByID[proposed.Items[index].ID]
		if decided {
			delete(decisionsByID, proposed.Items[index].ID)
			proposed.Items[index].Disposition = decision.Disposition
			if decision.Disposition == "accepted" {
				proposed.Items[index].Requirement = decision.Requirement
			} else {
				proposed.Items[index].Requirement = "optional"
			}
		} else {
			proposed.Items[index].Disposition = "rejected"
			proposed.Items[index].Requirement = "optional"
		}
		proposed.Items[index].ID = ""
	}
	if len(decisionsByID) != 0 {
		for id := range decisionsByID {
			return ValidationPlanRevision{}, fmt.Errorf("%w: item %q does not belong to proposal", ErrInvalidItemDecision, id)
		}
	}
	proposed.ID = ""
	proposed.Revision = 0
	proposed.Status = "accepted"
	proposed.CreatedAt = time.Time{}
	proposed.AcceptedAt = nil
	proposed.SourceRevisionID = proposedID
	if runID != "" {
		proposed.RunID = runID
	}
	accepted, err := s.CreateValidationPlanRevision(ctx, proposed)
	if err == nil {
		return accepted, nil
	}
	// The unique source revision index closes the two-tab/retry race. If the
	// other request won, return the authority it created.
	if existing, lookupErr := s.acceptedRevisionForProposal(ctx, proposedID); lookupErr == nil {
		return existing, nil
	}
	return ValidationPlanRevision{}, err
}

func (s *Store) acceptedRevisionForProposal(ctx context.Context, proposedID string) (ValidationPlanRevision, error) {
	var id string
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM validation_plan_revisions WHERE source_revision_id = ?`, proposedID).Scan(&id); err != nil {
		return ValidationPlanRevision{}, err
	}
	plan, err := s.GetValidationPlanRevision(ctx, id)
	if err != nil {
		return ValidationPlanRevision{}, err
	}
	if plan.Status != "accepted" {
		return ValidationPlanRevision{}, fmt.Errorf("validation proposal %s was already consumed by %s revision %s", proposedID, plan.Status, plan.ID)
	}
	return plan, nil
}

// AcceptedValidationPlanRevisionForProposal returns the single execution
// authority already created from a proposal. It lets HTTP/CLI retries converge
// before refreshing a worktree that may legitimately have moved since accept.
func (s *Store) AcceptedValidationPlanRevisionForProposal(ctx context.Context, proposedID string) (ValidationPlanRevision, error) {
	return s.acceptedRevisionForProposal(ctx, proposedID)
}

func validateValidationItem(item ValidationItem, planStatus string) error {
	validSource := map[string]bool{"graph": true, "blueprint": true, "policy": true, "user": true, "agent": true}
	validDisposition := map[string]bool{"suggested": true, "accepted": true, "rejected": true}
	validRequirement := map[string]bool{"optional": true, "required": true, "blocking": true}
	if !validSource[item.Source] || !validDisposition[item.Disposition] || !validRequirement[item.Requirement] || strings.TrimSpace(item.Criterion) == "" {
		return fmt.Errorf("invalid validation item %q", item.ID)
	}
	if planStatus == "accepted" && item.Disposition == "suggested" {
		return fmt.Errorf("accepted validation plan cannot contain suggested item %q", item.ID)
	}
	return nil
}

// UpdateValidationItem always refuses changes beneath accepted plans. Proposed
// plans may be discarded in favor of another revision; mutation is not needed.
func (s *Store) UpdateValidationItem(ctx context.Context, item ValidationItem) error {
	var status string
	if err := s.db.QueryRowContext(ctx, `SELECT p.status FROM validation_items i JOIN validation_plan_revisions p ON p.id = i.plan_revision_id WHERE i.id = ?`, item.ID).Scan(&status); err != nil {
		return err
	}
	if status == "accepted" {
		return ErrImmutableValidationPlan
	}
	return fmt.Errorf("validation items are revision-based; create a new plan revision")
}

func (s *Store) LinkValidationEvidence(ctx context.Context, link ValidationEvidenceLink) error {
	validStatus := map[string]bool{"passed": true, "failed": true, "inconclusive": true, "not_run": true, "unavailable": true}
	if !validStatus[link.Status] {
		return fmt.Errorf("invalid evidence status %q", link.Status)
	}
	var stateHash, patchHash, planStatus, disposition string
	err := s.db.QueryRowContext(ctx, `SELECT p.worktree_state_hash, p.patch_hash, p.status, i.disposition FROM validation_items i JOIN validation_plan_revisions p ON p.id = i.plan_revision_id WHERE i.id = ?`, link.ValidationItemID).Scan(&stateHash, &patchHash, &planStatus, &disposition)
	if err != nil {
		return err
	}
	if planStatus != "accepted" {
		return fmt.Errorf("evidence can link only to an accepted validation plan")
	}
	if disposition != "accepted" {
		return fmt.Errorf("evidence can link only to an accepted validation item")
	}
	if stateHash != link.WorktreeStateHash || patchHash != link.PatchHash {
		return ErrEvidenceStateMismatch
	}
	var stepRunID, stepStatus string
	if err := s.db.QueryRowContext(ctx, `SELECT run_id, status FROM run_steps WHERE id = ?`, link.RunStepID).Scan(&stepRunID, &stepStatus); err != nil {
		return fmt.Errorf("load evidence run step: %w", err)
	}
	if stepRunID != link.RunID {
		return fmt.Errorf("run step does not belong to evidence run")
	}
	if !EvidenceStatusMatchesRunStep(link.Status, stepStatus) {
		return ErrEvidenceStepMismatch
	}
	if link.ArtifactHash != "" {
		var exists int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM artifacts WHERE hash = ?`, link.ArtifactHash).Scan(&exists); err != nil || exists != 1 {
			return fmt.Errorf("evidence artifact %q does not exist", link.ArtifactHash)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO validation_evidence_links (validation_item_id, run_id, run_step_id, artifact_hash, worktree_state_hash, patch_hash, status) VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?) ON CONFLICT(validation_item_id, run_step_id) DO UPDATE SET artifact_hash = excluded.artifact_hash, worktree_state_hash = excluded.worktree_state_hash, patch_hash = excluded.patch_hash, status = excluded.status`, link.ValidationItemID, link.RunID, link.RunStepID, link.ArtifactHash, link.WorktreeStateHash, link.PatchHash, link.Status)
	return err
}

// EvidenceStatusMatchesRunStep is the authoritative mapping between evidence
// outcomes and the structured run-step terminal state that may support them.
func EvidenceStatusMatchesRunStep(evidenceStatus, stepStatus string) bool {
	expected := map[string]string{
		"passed":       "completed",
		"failed":       "failed",
		"inconclusive": "inconclusive",
		"not_run":      "unavailable",
		"unavailable":  "unavailable",
	}
	return expected[evidenceStatus] != "" && expected[evidenceStatus] == stepStatus
}

func (s *Store) EvidenceCoverage(ctx context.Context, planRevisionID string) (CompletionReport, error) {
	return s.evidenceCoverage(ctx, s.db, planRevisionID)
}

func (s *Store) evidenceCoverage(ctx context.Context, db verificationQuerier, planRevisionID string) (CompletionReport, error) {
	var planStatus, stateHash, worktreeID string
	if err := db.QueryRowContext(ctx, `SELECT p.status, p.worktree_state_hash, g.worktree_id FROM validation_plan_revisions p JOIN graph_generations g ON g.id = p.generation_id WHERE p.id = ?`, planRevisionID).Scan(&planStatus, &stateHash, &worktreeID); err != nil {
		return CompletionReport{}, err
	}
	if planStatus != "accepted" {
		return CompletionReport{}, fmt.Errorf("evidence coverage requires an accepted plan")
	}
	currentStateMatches := false
	var currentState string
	if err := db.QueryRowContext(ctx, `SELECT worktree_state_hash FROM graph_generations WHERE worktree_id = ? AND status = 'current' ORDER BY promoted_at DESC LIMIT 1`, worktreeID).Scan(&currentState); err == nil && currentState == stateHash {
		currentStateMatches = true
	}
	rows, err := db.QueryContext(ctx, `SELECT id, criterion, requirement, disposition, scope FROM validation_items WHERE plan_revision_id = ? ORDER BY id`, planRevisionID)
	if err != nil {
		return CompletionReport{}, err
	}
	type coverageItem struct{ id, criterion, requirement, disposition, scope string }
	var items []coverageItem
	for rows.Next() {
		var item coverageItem
		if err := rows.Scan(&item.id, &item.criterion, &item.requirement, &item.disposition, &item.scope); err != nil {
			rows.Close()
			return CompletionReport{}, err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return CompletionReport{}, err
	}
	report := CompletionReport{PlanRevisionID: planRevisionID, CanComplete: true}
	for _, item := range items {
		if item.disposition != "accepted" {
			continue
		}
		status, artifacts, err := coverageForItem(ctx, db, item.id, stateHash)
		if err != nil {
			return CompletionReport{}, err
		}
		if !currentStateMatches {
			status = "unavailable"
		}
		claimStatus := "not_run"
		switch status {
		case "passed":
			claimStatus = "supported"
		case "failed":
			claimStatus = "unsupported"
		case "inconclusive":
			claimStatus = "inconclusive"
		case "unavailable":
			claimStatus = "unavailable"
		}
		claimID := "claim_" + uuid.NewSHA1(uuid.NameSpaceOID, []byte(item.id+"\x00"+stateHash)).String()
		claim := CompletionClaim{ID: claimID, ValidationItemID: item.id, Predicate: "validation_item_passed", Scope: item.scope, Status: claimStatus, RepositoryStateHash: stateHash, EvidenceArtifactIDs: artifacts, Criterion: item.criterion, Requirement: item.requirement}
		if _, err := db.ExecContext(ctx, `INSERT INTO completion_claims (id, validation_item_id, predicate, scope, status, repository_state_hash, evidence_artifact_ids) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(validation_item_id, predicate, repository_state_hash) DO UPDATE SET status = excluded.status, evidence_artifact_ids = excluded.evidence_artifact_ids`, claim.ID, claim.ValidationItemID, claim.Predicate, claim.Scope, claim.Status, claim.RepositoryStateHash, encodeJSON(claim.EvidenceArtifactIDs, "[]")); err != nil {
			return CompletionReport{}, err
		}
		if claim.Status == "supported" {
			report.Verified = append(report.Verified, claim)
		} else {
			report.NotVerified = append(report.NotVerified, claim)
			if item.requirement == "required" || item.requirement == "blocking" {
				report.CanComplete = false
			}
		}
	}
	return report, nil
}

// FinalizeEvidenceCoverage freezes the first report produced for an accepted
// revision. Later evidence or worktree drift cannot rewrite this historical
// completion decision.
func (s *Store) FinalizeEvidenceCoverage(ctx context.Context, planRevisionID string) (CompletionReport, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CompletionReport{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if existing, err := readCompletionReport(ctx, tx, planRevisionID); err == nil {
		return existing, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return CompletionReport{}, err
	}
	var planStatus, acceptedState, currentState string
	err = tx.QueryRowContext(ctx, `SELECT p.status, p.worktree_state_hash, COALESCE(current.worktree_state_hash, '')
		FROM validation_plan_revisions p
		JOIN graph_generations bound ON bound.id = p.generation_id
		LEFT JOIN graph_generations current ON current.worktree_id = bound.worktree_id AND current.status = 'current'
		WHERE p.id = ?
		ORDER BY current.promoted_at DESC LIMIT 1`, planRevisionID).Scan(&planStatus, &acceptedState, &currentState)
	if err != nil {
		return CompletionReport{}, err
	}
	if planStatus != "accepted" {
		return CompletionReport{}, fmt.Errorf("evidence coverage requires an accepted plan")
	}
	if currentState == "" || currentState != acceptedState {
		return CompletionReport{}, ErrCompletionStateMoved
	}
	var acceptedItems, evidenceLinks int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM validation_items WHERE plan_revision_id = ? AND disposition = 'accepted'`, planRevisionID).Scan(&acceptedItems); err != nil {
		return CompletionReport{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM validation_evidence_links links JOIN validation_items items ON items.id = links.validation_item_id WHERE items.plan_revision_id = ? AND items.disposition = 'accepted'`, planRevisionID).Scan(&evidenceLinks); err != nil {
		return CompletionReport{}, err
	}
	if acceptedItems > 0 && evidenceLinks == 0 {
		return CompletionReport{}, ErrCompletionEvidenceEmpty
	}
	report, err := s.evidenceCoverage(ctx, tx, planRevisionID)
	if err != nil {
		return CompletionReport{}, err
	}
	report.ID = "completion_report_" + uuid.NewString()
	report.CreatedAt = time.Now().UTC()
	report.Verified = append([]CompletionClaim{}, report.Verified...)
	report.NotVerified = append([]CompletionClaim{}, report.NotVerified...)
	encoded, err := json.Marshal(report)
	if err != nil {
		return CompletionReport{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO completion_reports (id, plan_revision_id, report_json, created_at) VALUES (?, ?, ?, ?) ON CONFLICT(plan_revision_id) DO NOTHING`, report.ID, planRevisionID, string(encoded), report.CreatedAt); err != nil {
		return CompletionReport{}, err
	}
	stored, err := readCompletionReport(ctx, tx, planRevisionID)
	if err != nil {
		return CompletionReport{}, err
	}
	if err := tx.Commit(); err != nil {
		return CompletionReport{}, err
	}
	return stored, nil
}

func (s *Store) ReadCompletionReport(ctx context.Context, planRevisionID string) (CompletionReport, error) {
	return readCompletionReport(ctx, s.db, planRevisionID)
}

func readCompletionReport(ctx context.Context, db verificationQuerier, planRevisionID string) (CompletionReport, error) {
	var encoded string
	if err := db.QueryRowContext(ctx, `SELECT report_json FROM completion_reports WHERE plan_revision_id = ?`, planRevisionID).Scan(&encoded); err != nil {
		return CompletionReport{}, err
	}
	var report CompletionReport
	if err := json.Unmarshal([]byte(encoded), &report); err != nil {
		return CompletionReport{}, fmt.Errorf("decode completion report: %w", err)
	}
	report.Verified = append([]CompletionClaim{}, report.Verified...)
	report.NotVerified = append([]CompletionClaim{}, report.NotVerified...)
	return report, nil
}

func coverageForItem(ctx context.Context, db verificationQuerier, itemID, stateHash string) (string, []string, error) {
	rows, err := db.QueryContext(ctx, `SELECT status, COALESCE(artifact_hash, '') FROM validation_evidence_links WHERE validation_item_id = ? AND worktree_state_hash = ? ORDER BY created_at DESC`, itemID, stateHash)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	status := "not_run"
	var artifacts []string
	for rows.Next() {
		var current, artifact string
		if err := rows.Scan(&current, &artifact); err != nil {
			return "", nil, err
		}
		if artifact != "" {
			artifacts = append(artifacts, artifact)
		}
		if status == "not_run" || status == "unavailable" || current == "failed" {
			status = current
		}
	}
	sort.Strings(artifacts)
	return status, artifacts, rows.Err()
}

func (r CompletionReport) Render() string {
	var builder strings.Builder
	builder.WriteString("Verified:\n")
	if len(r.Verified) == 0 {
		builder.WriteString("- Nothing was verified by eligible evidence.\n")
	}
	for _, claim := range r.Verified {
		fmt.Fprintf(&builder, "- %s\n", claim.Criterion)
	}
	builder.WriteString("\nNot verified:\n")
	if len(r.NotVerified) == 0 {
		builder.WriteString("- No accepted validation obligation is outstanding.\n")
	}
	for _, claim := range r.NotVerified {
		fmt.Fprintf(&builder, "- %s (%s)\n", claim.Criterion, claim.Status)
	}
	return builder.String()
}

// NormalizeCompletionSummary rejects overbroad free-form verification language
// by replacing it with the evidence-derived report.
func NormalizeCompletionSummary(agentText string, report CompletionReport) (string, bool) {
	lower := strings.ToLower(agentText)
	overbroad := []string{"everything works", "fully verified", "no regressions", "all affected functionality passed", "all tests passed"}
	for _, phrase := range overbroad {
		if strings.Contains(lower, phrase) {
			return report.Render(), true
		}
	}
	return agentText + "\n\n" + report.Render(), false
}

func encodeJSON(value any, fallback string) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fallback
	}
	return string(data)
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
