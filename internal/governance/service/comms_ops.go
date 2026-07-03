package service

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/comms"
)

// GenerateHandoff renders an audience-targeted communication artifact for a
// release from its stored records (never from AI claims), persists it as a
// CommunicationArtifact, and returns the rendered body.
//
// Supported audiences:
//   - tester   -> tester handoff (includes per-change evidence, env, version)
//   - pm       -> PM status summary
//   - customer -> customer-safe release notes
//
// env and version apply only to the tester handoff and are ignored for the
// others.
func (s *Service) GenerateHandoff(ctx context.Context, releaseID, audience, env, version string) (string, error) {
	rel, err := s.getRelease(ctx, releaseID)
	if err != nil {
		return "", err
	}
	changes, err := s.store.ListChangeRecordsByRelease(ctx, releaseID)
	if err != nil {
		return "", fmt.Errorf("list changes for release %q: %w", releaseID, err)
	}

	var body string
	switch audience {
	case governance.AudienceTester:
		evidence, err := s.evidenceByChange(ctx, changes)
		if err != nil {
			return "", err
		}
		body = comms.GenerateTesterHandoff(rel, changes, evidence, env, version)
	case governance.AudiencePM:
		body = comms.GeneratePMSummary(rel, changes)
	case governance.AudienceCustomer:
		body = comms.GenerateCustomerSummary(rel, changes)
	default:
		return "", fmt.Errorf("unknown handoff audience %q (supported: %s, %s, %s)",
			audience, governance.AudienceTester, governance.AudiencePM, governance.AudienceCustomer)
	}

	artifact := &governance.CommunicationArtifact{
		ID:        newID(),
		ReleaseID: releaseID,
		Audience:  audience,
		Body:      body,
	}
	if err := s.store.CreateCommunicationArtifact(ctx, artifact); err != nil {
		return "", fmt.Errorf("persist %s handoff for release %q: %w", audience, releaseID, err)
	}
	return body, nil
}

// evidenceByChange builds the change-ID -> evidence map the tester handoff
// generator expects.
func (s *Service) evidenceByChange(ctx context.Context, changes []*governance.ChangeRecord) (map[string][]*governance.Evidence, error) {
	out := make(map[string][]*governance.Evidence, len(changes))
	for _, ch := range changes {
		if ch == nil {
			continue
		}
		ev, err := s.store.ListEvidence(ctx, ch.ID)
		if err != nil {
			return nil, fmt.Errorf("list evidence for change %q: %w", ch.ID, err)
		}
		out[ch.ID] = ev
	}
	return out, nil
}
