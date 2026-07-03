package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/openexec/openexec/internal/governance"
)

// CreateManualChange files a change record from free text (no external tracker),
// e.g. a bug or feature you type directly. It starts as a candidate awaiting
// triage, exactly like an imported issue. SourceType=manual; a stable id is
// derived so re-runs and links are predictable.
func (s *Service) CreateManualChange(ctx context.Context, projectID, title, body string) (*governance.ChangeRecord, error) {
	if strings.TrimSpace(title) == "" {
		return nil, fmt.Errorf("a title is required")
	}
	id := "CHANGE-manual-" + shortID()
	ch := &governance.ChangeRecord{
		ID:         id,
		ProjectID:  projectID,
		SourceType: governance.SourceManual,
		SourceID:   id, // non-empty so it is queryable; unique per record
		Title:      title,
		RawText:    body,
		Status:     governance.ChangeStatusCandidate,
	}
	if err := s.store.CreateChangeRecord(ctx, ch); err != nil {
		return nil, fmt.Errorf("create manual change: %w", err)
	}
	return ch, nil
}

// shortID returns the first 8 chars of a fresh uuid for a compact change id.
func shortID() string {
	u := newID()
	if len(u) >= 8 {
		return u[:8]
	}
	return u
}
