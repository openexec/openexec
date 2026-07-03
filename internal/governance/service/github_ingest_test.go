package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// scriptedRunner returns a fixed comments payload for the list-comments gh api
// call and captures posted comment bodies; everything else returns "{}".
type scriptedRunner struct {
	comments string
	posted   []string
}

func (r *scriptedRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	if len(args) > 0 && args[0] == "api" && strings.Contains(joined, "/comments") {
		return []byte(r.comments), nil
	}
	if len(args) > 1 && args[0] == "issue" && args[1] == "comment" {
		for i, a := range args {
			if a == "--body" && i+1 < len(args) {
				r.posted = append(r.posted, args[i+1])
			}
		}
	}
	return []byte("{}"), nil
}

func githubChange(t *testing.T, store governance.Store, id string, number int, status string) {
	t.Helper()
	seedChange(t, store, &governance.ChangeRecord{
		ID:                 id,
		ProjectID:          "proj",
		SourceType:         governance.SourceGitHubIssue,
		SourceID:           fmt.Sprintf("%d", number),
		SourceURL:          fmt.Sprintf("https://github.com/org/repo/issues/%d", number),
		Status:             status,
		Risk:               governance.RiskLow,
		ProposalVersion:    1,
		AcceptanceCriteria: "works",
		VerificationPlan:   "go test",
	})
}

func TestIngestGitHubComments_AuthorizationAndIdempotency(t *testing.T) {
	store := newTestStore(t)
	runner := &scriptedRunner{
		// Comment 10: approve from an unauthorized author. Comment 20: approve
		// from pm (mapped). Oldest-first ordering is enforced by the ingester.
		comments: `[
			{"id":20,"body":"/openexec approve","created_at":"2026-07-02T10:01:00Z","user":{"login":"perttu"}},
			{"id":10,"body":"/openexec approve","created_at":"2026-07-02T10:00:00Z","user":{"login":"randopatch"}}
		]`,
	}
	svc := NewService(store, Options{Runner: runner, OperatorSession: true})
	ctx := context.Background()

	githubChange(t, store, "CHANGE-github-proj-7", 7, governance.ChangeStatusPlanReady)
	authorMap := map[string]string{"perttu": "pm"} // randopatch is NOT mapped

	rep, err := svc.IngestGitHubComments(ctx, "proj", "org/repo", authorMap)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if rep.ScannedChanges != 1 || len(rep.Actions) != 2 {
		t.Fatalf("expected 1 change / 2 actions, got %d / %d", rep.ScannedChanges, len(rep.Actions))
	}

	// The unauthorized approve must NOT be applied; pm's must be.
	var unauthApplied, pmApplied bool
	for _, a := range rep.Actions {
		if a.Author == "randopatch" {
			unauthApplied = a.Applied
		}
		if a.Author == "perttu" {
			pmApplied = a.Applied
		}
	}
	if unauthApplied {
		t.Fatalf("unauthorized author must not drive approve")
	}
	if !pmApplied {
		t.Fatalf("mapped author pm should have approved")
	}

	got, _ := store.GetChangeRecord(ctx, "CHANGE-github-proj-7")
	if got.Status != governance.ChangeStatusApprovedForAI {
		t.Fatalf("expected approved_for_ai after pm approval, got %q", got.Status)
	}

	// Idempotency: a second poll with the same comments does nothing new (cursor
	// advanced past comment 20).
	rep2, err := svc.IngestGitHubComments(ctx, "proj", "org/repo", authorMap)
	if err != nil {
		t.Fatalf("second ingest: %v", err)
	}
	if len(rep2.Actions) != 0 {
		t.Fatalf("expected no new actions on re-poll, got %d", len(rep2.Actions))
	}
}
