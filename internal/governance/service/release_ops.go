package service

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/validation"
)

// CreateRelease creates a new release in draft status. id is required; name and
// owner are descriptive. The release carries no items yet — attach changes with
// AttachChange before approving.
func (s *Service) CreateRelease(ctx context.Context, id, name, owner string) (*governance.GovernanceRelease, error) {
	if id == "" {
		return nil, fmt.Errorf("release id is required")
	}
	rel := &governance.GovernanceRelease{
		ID:     id,
		Name:   name,
		Owner:  owner,
		Status: governance.ReleaseStatusDraft,
		Risk:   governance.RiskLow,
	}
	if err := s.store.CreateRelease(ctx, rel); err != nil {
		return nil, fmt.Errorf("create release %q: %w", id, err)
	}
	return rel, nil
}

// AttachChange links a change record to a release as a release item.
//
// If the release is currently approved, growing its scope invalidates the prior
// approval (which covered only the previous item set): the release is reset to
// planned with ApprovedForAI cleared, and a system DecisionEvent is recorded so
// the invalidation is visible in history. Re-approval is then required before
// the new scope can be implemented.
func (s *Service) AttachChange(ctx context.Context, releaseID, changeID string, priority int, required bool) error {
	rel, err := s.getRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}

	item := &governance.ReleaseItem{
		ReleaseID: releaseID,
		ChangeID:  changeID,
		ItemType:  itemTypeForChange(ch),
		Priority:  priority,
		Required:  required,
	}
	if err := s.store.CreateReleaseItem(ctx, item); err != nil {
		return fmt.Errorf("attach change %q to release %q: %w", changeID, releaseID, err)
	}

	// Keep the change's denormalized release pointer in sync so queue/handoff
	// lookups by release see this change.
	if ch.ReleaseID != releaseID {
		ch.ReleaseID = releaseID
		if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
			return fmt.Errorf("link change %q to release %q: %w", changeID, releaseID, err)
		}
	}

	// A draft release gains scope and moves to planned on first attach, so the
	// create -> attach -> approve path can reach approval (draft cannot be
	// approved directly). Idempotent: a release already past draft is untouched.
	if rel.Status == governance.ReleaseStatusDraft {
		if err := validation.ValidateReleaseTransition(rel.Status, governance.ReleaseStatusPlanned); err != nil {
			return err
		}
		rel.Status = governance.ReleaseStatusPlanned
		if err := s.store.UpdateRelease(ctx, rel); err != nil {
			return fmt.Errorf("move release %q to planned: %w", releaseID, err)
		}
	}

	// Scope-add into an approved release invalidates the approval.
	if validation.ValidateScopeAddInvalidatesApproval(rel) {
		rel.ApprovedForAI = false
		rel.Status = governance.ReleaseStatusPlanned
		if err := s.store.UpdateRelease(ctx, rel); err != nil {
			return fmt.Errorf("reset approval on release %q: %w", releaseID, err)
		}
		ev := &governance.DecisionEvent{
			ID:        newID(),
			ReleaseID: releaseID,
			ChangeID:  changeID,
			Actor:     "system",
			ActorType: governance.ActorTypeSystem,
			Decision:  governance.DecisionChangesRequested,
			Comment:   fmt.Sprintf("Release approval invalidated: change %s added to approved release; re-approval required.", changeID),
		}
		if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
			return fmt.Errorf("record approval-invalidation event for release %q: %w", releaseID, err)
		}
	}
	return nil
}

// ApproveRelease approves a release's scope, opening its approved work to AI
// executors. It requires at least one item (ValidateApproveRelease), an
// authority permitted to approve release scope (evaluator.CanApproveRelease —
// human + approve permission), and a legal planned -> approved transition; it
// sets ApprovedForAI and records a DecisionEvent attributed to the authority.
func (s *Service) ApproveRelease(ctx context.Context, releaseID, authorityID string) error {
	if !s.operatorSession {
		return errNotOperator
	}
	rel, err := s.getRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	authority, err := s.getAuthority(ctx, authorityID)
	if err != nil {
		return err
	}

	items, err := s.store.ListReleaseItems(ctx, releaseID)
	if err != nil {
		return fmt.Errorf("list items for release %q: %w", releaseID, err)
	}
	if err := validation.ValidateApproveRelease(rel, items); err != nil {
		return err
	}
	if ok, reason := s.evaluator.CanApproveRelease(authority, rel); !ok {
		return fmt.Errorf("release approval refused: %s", reason)
	}
	if err := validation.ValidateReleaseTransition(rel.Status, governance.ReleaseStatusApproved); err != nil {
		return err
	}

	rel.Status = governance.ReleaseStatusApproved
	rel.ApprovedForAI = true
	if err := s.store.UpdateRelease(ctx, rel); err != nil {
		return fmt.Errorf("approve release %q: %w", releaseID, err)
	}

	ev := &governance.DecisionEvent{
		ID:        newID(),
		ReleaseID: releaseID,
		Actor:     authority.ID,
		ActorType: authority.Type,
		Decision:  governance.DecisionApproved,
		Comment:   fmt.Sprintf("Release %s approved by %s", releaseID, authority.Name),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return fmt.Errorf("record release approval for %q: %w", releaseID, err)
	}
	return nil
}

// PlanRelease moves a draft release to planned explicitly (the create -> plan
// -> approve path). AttachChange also performs this promotion on first attach;
// this method exists so a release can be planned without attaching, and so the
// CLI mirrors the plan's `release plan` verb. Idempotent for an already-planned
// release; rejects any other current status via the transition validator.
func (s *Service) PlanRelease(ctx context.Context, releaseID string) error {
	rel, err := s.getRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	if rel.Status == governance.ReleaseStatusPlanned {
		return nil
	}
	if err := validation.ValidateReleaseTransition(rel.Status, governance.ReleaseStatusPlanned); err != nil {
		return err
	}
	rel.Status = governance.ReleaseStatusPlanned
	if err := s.store.UpdateRelease(ctx, rel); err != nil {
		return fmt.Errorf("plan release %q: %w", releaseID, err)
	}
	return nil
}

// StartRelease moves an approved release into implementing. Validation enforces
// the approved -> implementing transition.
func (s *Service) StartRelease(ctx context.Context, releaseID string) error {
	rel, err := s.getRelease(ctx, releaseID)
	if err != nil {
		return err
	}
	if err := validation.ValidateReleaseTransition(rel.Status, governance.ReleaseStatusImplementing); err != nil {
		return err
	}
	rel.Status = governance.ReleaseStatusImplementing
	if err := s.store.UpdateRelease(ctx, rel); err != nil {
		return fmt.Errorf("start release %q: %w", releaseID, err)
	}
	return nil
}

// itemTypeForChange maps a change's kind to a release-item type. The plan's
// ReleaseItem.ItemType (epic|story|task|bug|remediation) is not 1:1 with change
// Kind, so this is a best-effort default; bugs and SRE findings get their own
// item types, everything else is a task.
func itemTypeForChange(ch *governance.ChangeRecord) string {
	if ch == nil {
		return governance.ItemTypeTask
	}
	switch ch.Kind {
	case governance.KindBug:
		return governance.ItemTypeBug
	case governance.KindReliability, governance.KindSecurity, governance.KindOps:
		if ch.SourceType == governance.SourceOpenSREFinding {
			return governance.ItemTypeRemediation
		}
		return governance.ItemTypeTask
	default:
		return governance.ItemTypeTask
	}
}
