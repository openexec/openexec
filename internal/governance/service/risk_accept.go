package service

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
)

// AcceptRisk records an explicit human risk-acceptance for a change at its
// CURRENT proposal version. Tiers that demand it (critical, by default) cannot
// be approved or merged until this is on record. Requires an operator session
// and an authority holding the risk_accept permission (and, for such tiers, a
// human authority — enforced by policy.CanRiskAccept). Because the acceptance is
// stamped with the proposal version, a later plan revision (which bumps the
// version) invalidates it, so risk must be re-accepted against the new plan.
func (s *Service) AcceptRisk(ctx context.Context, changeID, authorityID, note string) error {
	if !s.operatorSession {
		return errNotOperator
	}
	if authorityID == "" {
		return fmt.Errorf("a review authority is required to accept risk")
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	authority, err := s.getAuthority(ctx, authorityID)
	if err != nil {
		return err
	}
	if err := s.guardHumanAuthority(authority); err != nil {
		return err
	}
	if ok, reason := s.evaluator.CanRiskAccept(authority, ch); !ok {
		return fmt.Errorf("risk acceptance refused: %s", reason)
	}
	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authority.ID,
		ActorType:       authority.Type,
		Decision:        governance.DecisionRiskAccepted,
		Comment:         fmt.Sprintf("Risk accepted by %s%s (plan v%d): %s", authority.Name, s.operatorSuffix(), ch.ProposalVersion, note),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return fmt.Errorf("record risk acceptance for change %q: %w", changeID, err)
	}
	return nil
}

// ensureRiskAccepted enforces that a change whose tier requires human risk
// acceptance has a CURRENT one on record (at the change's proposal version). A
// no-op for tiers that do not require it.
func (s *Service) ensureRiskAccepted(ctx context.Context, ch *governance.ChangeRecord) error {
	if !s.evaluator.RequiresRiskAcceptance(ch) {
		return nil
	}
	events, err := s.store.ListDecisionEvents(ctx, ch.ID)
	if err != nil {
		return fmt.Errorf("list decision history for change %q: %w", ch.ID, err)
	}
	for _, e := range events {
		if e != nil && e.Decision == governance.DecisionRiskAccepted && e.ProposalVersion == ch.ProposalVersion {
			return nil
		}
	}
	return fmt.Errorf("change %s is %s-risk and requires an explicit human risk-acceptance at the current plan version before approval/merge — run `governance work risk-accept %s --by <human> --note ...`", ch.ID, ch.Risk, ch.ID)
}
