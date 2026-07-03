package service

import (
	"context"
	"fmt"
	"time"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/comms"
	"github.com/openexec/openexec/internal/governance/validation"
)

// decisionClaimed is the service-local audit token recorded when an executor
// claims work. The governance Decision* vocabulary covers the plan lifecycle
// (propose/review/approve/done) but has no "claimed" verb; this token keeps the
// claim visible in history without polluting the separation-of-duties roles
// (which only consider proposed/approved/marked_done).
const decisionClaimed = "claimed"

// Service-local audit tokens for system-driven status advances that the
// governance Decision* vocabulary (propose/review/approve/done) does not name.
// These transitions are performed by the orchestrator as part of the governed
// workflow, so they are attributed to "system" — recording them keeps the audit
// trail continuous (no status changes without a corresponding event).
const (
	decisionPROpened       = "pr_opened"
	decisionReadyForTest   = "ready_for_test"
	decisionReleasePlanned = "release_planned"
	decisionReleaseStarted = "release_started"
)

// ListApprovedWork returns the change records an executor may pick up: approved
// for AI, belonging to a release in approved/implementing status, and not
// actively claimed. The store enforces these invariants; this method is a thin,
// named entry point so CLI/MCP share it.
func (s *Service) ListApprovedWork(ctx context.Context, projectID string) ([]*governance.ChangeRecord, error) {
	work, err := s.store.ListApprovedWork(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list approved work: %w", err)
	}
	return work, nil
}

// ClaimWork leases a change to an executor. It enforces, in order:
//  1. implementability: the release is approved/implementing AND the change's
//     approved plan version still matches its proposal (ValidateImplementable);
//  2. claimability: no other executor holds an active lease (ValidateClaim);
//
// then takes the store-level lease, advances an approved_for_ai change to
// implementing, and records a claim DecisionEvent.
func (s *Service) ClaimWork(ctx context.Context, changeID, agent string, lease time.Duration) error {
	if agent == "" {
		return fmt.Errorf("an agent name is required to claim work")
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	rel, err := s.getRelease(ctx, ch.ReleaseID)
	if err != nil {
		return err
	}

	if err := validation.ValidateImplementable(rel, ch); err != nil {
		return err
	}
	if err := validation.ValidateClaim(ch, agent, time.Now().UTC()); err != nil {
		return err
	}

	// Take the lease FIRST via the atomic conditional claim, so a race loser
	// never advances status. If this fails (already-claimed / I/O) nothing has
	// been mutated.
	if err := s.store.ClaimWork(ctx, changeID, agent, lease); err != nil {
		return fmt.Errorf("claim change %q for %q: %w", changeID, agent, err)
	}

	// Re-load so the in-memory record carries the claim columns just written;
	// otherwise the status-advance upsert below would clobber the fresh claim.
	ch, err = s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	// Advance approved_for_ai -> implementing on the first claim. A renewal by
	// the same agent (already implementing) skips the transition.
	if ch.Status == governance.ChangeStatusApprovedForAI {
		if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusImplementing); err != nil {
			return err
		}
		ch.Status = governance.ChangeStatusImplementing
		if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
			return fmt.Errorf("advance change %q to implementing: %w", changeID, err)
		}
	}

	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           agent,
		ActorType:       governance.ActorTypeAI,
		Decision:        decisionClaimed,
		Comment:         fmt.Sprintf("Claimed by %s (lease %s)", agent, lease),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return fmt.Errorf("record claim for change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// WorkBrief renders the self-contained executor brief for a change: approved
// scope, acceptance criteria, verification plan, and the reporting contract. It
// is generated from stored records only.
func (s *Service) WorkBrief(ctx context.Context, changeID, repoPath string) (string, error) {
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return "", err
	}
	var rel *governance.GovernanceRelease
	if ch.ReleaseID != "" {
		rel, err = s.getRelease(ctx, ch.ReleaseID)
		if err != nil {
			return "", err
		}
	}
	return comms.GenerateExecutorBrief(rel, ch, repoPath), nil
}

// RecordPR records the pull request opened for a change: it pins the PR URL and
// branch and advances the change implementing -> pr_open. PR/branch are stored
// fields, not evidence, so no Evidence row is created here.
func (s *Service) RecordPR(ctx context.Context, changeID, url, branch string) error {
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusPROpen); err != nil {
		return err
	}
	ch.PRURL = url
	if branch != "" {
		ch.Branch = branch
	}
	ch.Status = governance.ChangeStatusPROpen
	if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
		return fmt.Errorf("record PR for change %q: %w", changeID, err)
	}
	if err := s.recordSystemTransition(ctx, ch.ReleaseID, ch.ID, ch.ProposalVersion, decisionPROpened, fmt.Sprintf("PR opened: %s", url)); err != nil {
		return fmt.Errorf("record PR-opened event for change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// RecordEvidence attaches a structured evidence record to a change (test, CI,
// review, deploy, monitoring, or manual proof). The change must exist; evidence
// itself imposes no status change.
func (s *Service) RecordEvidence(ctx context.Context, changeID, kind, source, summary, url, raw string) error {
	if _, err := s.getChange(ctx, changeID); err != nil {
		return err
	}
	ev := &governance.Evidence{
		ID:       newID(),
		ChangeID: changeID,
		Kind:     kind,
		Source:   source,
		Summary:  summary,
		URL:      url,
		Raw:      raw,
	}
	if err := s.store.CreateEvidence(ctx, ev); err != nil {
		return fmt.Errorf("record evidence for change %q: %w", changeID, err)
	}
	return nil
}

// ReadyForTest moves a change to ready_for_test. The domain gate runs first: a
// PR URL or manual/test/CI evidence must exist (ValidateReadyForTest); then the
// pr_open -> ready_for_test transition is enforced.
func (s *Service) ReadyForTest(ctx context.Context, changeID string) error {
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	evidence, err := s.store.ListEvidence(ctx, changeID)
	if err != nil {
		return fmt.Errorf("list evidence for change %q: %w", changeID, err)
	}
	if err := validation.ValidateReadyForTest(ch, hasManualEvidence(evidence)); err != nil {
		return err
	}
	if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusReadyForTest); err != nil {
		return err
	}
	ch.Status = governance.ChangeStatusReadyForTest
	if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
		return fmt.Errorf("mark change %q ready for test: %w", changeID, err)
	}
	if err := s.recordSystemTransition(ctx, ch.ReleaseID, ch.ID, ch.ProposalVersion, decisionReadyForTest, "Advanced to ready_for_test"); err != nil {
		return fmt.Errorf("record ready-for-test event for change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// MarkDone closes a change as done. authorityID identifies a review authority;
// the full gate runs in order:
//  1. the authority holds the mark_done permission and its risk limit covers the
//     change (evaluator.CanMarkDone);
//  2. verification evidence exists (ValidateMarkDone);
//  3. separation of duties holds for medium+ risk — the authority is folded into
//     the done-marker role before the check, so an authority that already
//     proposed AND approved this change cannot also mark it done;
//  4. the ready_for_test -> done transition is legal.
//
// A marked_done DecisionEvent is then recorded against the real authority.
func (s *Service) MarkDone(ctx context.Context, changeID, authorityID string) error {
	if authorityID == "" {
		return fmt.Errorf("a review authority is required to mark work done")
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
	if ok, reason := s.evaluator.CanMarkDone(authority, ch); !ok {
		return fmt.Errorf("mark done refused: %s", reason)
	}

	evidence, err := s.store.ListEvidence(ctx, changeID)
	if err != nil {
		return fmt.Errorf("list evidence for change %q: %w", changeID, err)
	}
	if err := validation.ValidateMarkDone(evidence); err != nil {
		return err
	}

	history, err := s.store.ListDecisionEvents(ctx, changeID)
	if err != nil {
		return fmt.Errorf("list decision history for change %q: %w", changeID, err)
	}
	// Fold the pending mark-done into history so the authority is counted as the
	// done-marker for the separation-of-duties check, without persisting an
	// event that a failed check would leave behind.
	pending := append(history, &governance.DecisionEvent{
		ChangeID: changeID,
		Actor:    authority.ID,
		Decision: governance.DecisionMarkedDone,
	})
	if err := validation.ValidateSeparationOfDuties(ch, pending, ch.Risk); err != nil {
		return err
	}

	if err := validation.ValidateChangeTransition(ch.Status, governance.ChangeStatusDone); err != nil {
		return err
	}
	ch.Status = governance.ChangeStatusDone
	if err := s.store.UpdateChangeRecord(ctx, ch); err != nil {
		return fmt.Errorf("mark change %q done: %w", changeID, err)
	}

	ev := &governance.DecisionEvent{
		ID:              newID(),
		ReleaseID:       ch.ReleaseID,
		ChangeID:        ch.ID,
		ProposalVersion: ch.ProposalVersion,
		Actor:           authority.ID,
		ActorType:       authority.Type,
		Decision:        governance.DecisionMarkedDone,
		Comment:         fmt.Sprintf("Marked done by %s", authority.Name),
	}
	if err := s.store.CreateDecisionEvent(ctx, ev); err != nil {
		return fmt.Errorf("record done for change %q: %w", changeID, err)
	}
	s.mirrorGitHubLabel(ctx, ch)
	return nil
}

// History returns the full audit trail for a change: its decision events
// (chronological) and its evidence records.
func (s *Service) History(ctx context.Context, changeID string) ([]*governance.DecisionEvent, []*governance.Evidence, error) {
	if _, err := s.getChange(ctx, changeID); err != nil {
		return nil, nil, err
	}
	events, err := s.store.ListDecisionEvents(ctx, changeID)
	if err != nil {
		return nil, nil, fmt.Errorf("list decision history for change %q: %w", changeID, err)
	}
	evidence, err := s.store.ListEvidence(ctx, changeID)
	if err != nil {
		return nil, nil, fmt.Errorf("list evidence for change %q: %w", changeID, err)
	}
	return events, evidence, nil
}

// hasManualEvidence reports whether any recorded evidence is of a kind that
// satisfies the ready-for-test gate's "manual evidence" allowance: manual, test,
// or CI proof.
func hasManualEvidence(evidence []*governance.Evidence) bool {
	for _, e := range evidence {
		if e == nil {
			continue
		}
		switch e.Kind {
		case governance.EvidenceKindManual, governance.EvidenceKindTest, governance.EvidenceKindCI:
			return true
		}
	}
	return false
}
