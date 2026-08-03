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
	ID                string           `json:"id"`
	TaskID            string           `json:"task_id"`
	RunID             string           `json:"run_id,omitempty"`
	Revision          int              `json:"revision"`
	GenerationID      string           `json:"generation_id"`
	WorktreeStateHash string           `json:"worktree_state_hash"`
	PatchHash         string           `json:"patch_hash,omitempty"`
	Status            string           `json:"status"`
	Items             []ValidationItem `json:"items"`
	CreatedAt         time.Time        `json:"created_at"`
	AcceptedAt        *time.Time       `json:"accepted_at,omitempty"`
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
	PlanRevisionID string            `json:"plan_revision_id"`
	Verified       []CompletionClaim `json:"verified"`
	NotVerified    []CompletionClaim `json:"not_verified"`
	CanComplete    bool              `json:"can_complete"`
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
	var generationState string
	if err := s.db.QueryRowContext(ctx, `SELECT worktree_state_hash FROM graph_generations WHERE id = ?`, plan.GenerationID).Scan(&generationState); err != nil {
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
		accepted := plan.CreatedAt
		plan.AcceptedAt = &accepted
		if _, err := tx.ExecContext(ctx, `UPDATE validation_plan_revisions SET status = 'superseded' WHERE task_id = ? AND status = 'accepted'`, plan.TaskID); err != nil {
			return ValidationPlanRevision{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO validation_plan_revisions (id, task_id, run_id, revision, generation_id, worktree_state_hash, patch_hash, status, created_at, accepted_at) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?)`, plan.ID, plan.TaskID, plan.RunID, plan.Revision, plan.GenerationID, plan.WorktreeStateHash, plan.PatchHash, plan.Status, plan.CreatedAt, plan.AcceptedAt); err != nil {
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
	var stateHash, patchHash, planStatus string
	err := s.db.QueryRowContext(ctx, `SELECT p.worktree_state_hash, p.patch_hash, p.status FROM validation_items i JOIN validation_plan_revisions p ON p.id = i.plan_revision_id WHERE i.id = ?`, link.ValidationItemID).Scan(&stateHash, &patchHash, &planStatus)
	if err != nil {
		return err
	}
	if planStatus != "accepted" {
		return fmt.Errorf("evidence can link only to an accepted validation plan")
	}
	if stateHash != link.WorktreeStateHash || patchHash != link.PatchHash {
		return ErrEvidenceStateMismatch
	}
	var stepRunID string
	if err := s.db.QueryRowContext(ctx, `SELECT run_id FROM run_steps WHERE id = ?`, link.RunStepID).Scan(&stepRunID); err != nil {
		return fmt.Errorf("load evidence run step: %w", err)
	}
	if stepRunID != link.RunID {
		return fmt.Errorf("run step does not belong to evidence run")
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

func (s *Store) EvidenceCoverage(ctx context.Context, planRevisionID string) (CompletionReport, error) {
	var planStatus, stateHash, worktreeID string
	if err := s.db.QueryRowContext(ctx, `SELECT p.status, p.worktree_state_hash, g.worktree_id FROM validation_plan_revisions p JOIN graph_generations g ON g.id = p.generation_id WHERE p.id = ?`, planRevisionID).Scan(&planStatus, &stateHash, &worktreeID); err != nil {
		return CompletionReport{}, err
	}
	if planStatus != "accepted" {
		return CompletionReport{}, fmt.Errorf("evidence coverage requires an accepted plan")
	}
	currentStateMatches := false
	var currentState string
	if err := s.db.QueryRowContext(ctx, `SELECT worktree_state_hash FROM graph_generations WHERE worktree_id = ? AND status = 'current' ORDER BY promoted_at DESC LIMIT 1`, worktreeID).Scan(&currentState); err == nil && currentState == stateHash {
		currentStateMatches = true
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, criterion, requirement, disposition, scope FROM validation_items WHERE plan_revision_id = ? ORDER BY id`, planRevisionID)
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
		status, artifacts, err := s.coverageForItem(ctx, item.id, stateHash)
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
		claim := CompletionClaim{ID: "claim_" + uuid.NewString(), ValidationItemID: item.id, Predicate: "validation_item_passed", Scope: item.scope, Status: claimStatus, RepositoryStateHash: stateHash, EvidenceArtifactIDs: artifacts, Criterion: item.criterion, Requirement: item.requirement}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO completion_claims (id, validation_item_id, predicate, scope, status, repository_state_hash, evidence_artifact_ids) VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(validation_item_id, predicate, repository_state_hash) DO UPDATE SET status = excluded.status, evidence_artifact_ids = excluded.evidence_artifact_ids`, claim.ID, claim.ValidationItemID, claim.Predicate, claim.Scope, claim.Status, claim.RepositoryStateHash, encodeJSON(claim.EvidenceArtifactIDs, "[]")); err != nil {
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

func (s *Store) coverageForItem(ctx context.Context, itemID, stateHash string) (string, []string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT status, COALESCE(artifact_hash, '') FROM validation_evidence_links WHERE validation_item_id = ? AND worktree_state_hash = ? ORDER BY created_at DESC`, itemID, stateHash)
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
