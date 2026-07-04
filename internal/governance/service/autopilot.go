package service

import (
	"context"
	"sort"

	"github.com/openexec/openexec/internal/governance"
)

// autonomousActions maps a change status to the next action the autonomous loop
// can take WITHOUT a human. Everything else is deliberately excluded:
//   - plan_ready waits for a human (or low-risk auto-) approval,
//   - changes_requested / blocked are parked for a human answer,
//   - implementing is the in-flight slot,
//   - done / rejected / deferred are terminal.
//
// Because parked and terminal changes are skipped, "work one, park on
// clarification, move to the next actionable, return when answered" falls out of
// the selector for free — an answered change re-triages back into this set.
var autonomousActions = map[string]string{
	governance.ChangeStatusCandidate:     "triage",
	governance.ChangeStatusPlanned:       "review",
	governance.ChangeStatusApprovedForAI: "execute",
}

// NextActionable returns the change the autonomous loop should advance next for a
// project (or all projects when projectID is ""), plus the action to take, and
// honors a single work slot: if any change is already implementing, that IS the
// slot and it is returned with action "in-progress" so the caller starts nothing
// new. When no change is actionable it returns (nil, "", nil). Selection among
// actionable candidates is FIFO by CreatedAt.
//
// The AI-Fix control-label gate is enforced upstream at sync time (only labeled
// issues become change records), so every record here is already in scope.
func (s *Service) NextActionable(ctx context.Context, projectID string) (*governance.ChangeRecord, string, error) {
	all, err := s.store.ListChangeRecords(ctx)
	if err != nil {
		return nil, "", err
	}
	var candidates []*governance.ChangeRecord
	for _, ch := range all {
		if projectID != "" && ch.ProjectID != projectID {
			continue
		}
		// Single slot: an in-flight change occupies it — nothing new starts.
		if ch.Status == governance.ChangeStatusImplementing {
			return ch, "in-progress", nil
		}
		if _, ok := autonomousActions[ch.Status]; ok {
			candidates = append(candidates, ch)
		}
	}
	if len(candidates) == 0 {
		return nil, "", nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	ch := candidates[0]
	return ch, autonomousActions[ch.Status], nil
}
