package service

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/connectors/github"
)

// SyncAction records what the sync-in did for one issue.
type SyncAction struct {
	IssueNum int
	ChangeID string
	Imported bool // true if this was a fresh import (candidate)
	Triaged  bool // true if auto-deep-triaged this run
	StoryIDs []string
	Error    string
}

// SyncReport summarizes an automation sync-in poll.
type SyncReport struct {
	Repo    string
	Label   string
	Scanned int
	Actions []SyncAction
}

// SyncGitHubIssues is the passive automation input: it discovers open issues in
// a repo (optionally filtered to a label, e.g. "ai:triage"), imports each as a
// change record (idempotent), and — when autoTriage is set and a Completer +
// PlanStore are available — auto-deep-triages FRESHLY-imported (candidate)
// changes into stories/tasks so they arrive as reviewable interpretations
// awaiting human approval. Schedule it (cron / loop) to make new/flagged work
// show up triaged without a human running import by hand.
//
// Idempotency: import never duplicates; auto-triage runs only on candidate-status
// changes, so re-syncing does not re-triage already-triaged or human-decided
// (approved/rejected/deferred) work. Requires a Runner; auto-triage additionally
// needs a Completer + PlanStore (absent → import only, no error).
func (s *Service) SyncGitHubIssues(ctx context.Context, projectID, repo, label string, autoTriage bool) (*SyncReport, error) {
	if s.runner == nil {
		return nil, errMissingRunner
	}
	numbers, err := github.ListOpenIssues(ctx, s.runner, repo, label)
	if err != nil {
		return nil, err
	}

	report := &SyncReport{Repo: repo, Label: label, Scanned: len(numbers)}
	canTriage := autoTriage && s.completer != nil && s.planStore != nil

	for _, n := range numbers {
		act := SyncAction{IssueNum: n}
		ch, err := github.ImportIssue(ctx, s.runner, s.store, projectID, repo, n)
		if err != nil {
			act.Error = fmt.Sprintf("import: %v", err)
			report.Actions = append(report.Actions, act)
			continue
		}
		act.ChangeID = ch.ID

		// Only freshly-imported (candidate) work is auto-triaged; anything already
		// triaged or human-decided is left untouched so scheduled re-syncs are safe.
		if ch.Status == governance.ChangeStatusCandidate {
			act.Imported = true
			if canTriage {
				res, tErr := s.TriageDeep(ctx, ch.ID, "", "sync_planner")
				if tErr != nil {
					act.Error = fmt.Sprintf("triage: %v", tErr)
				} else {
					act.Triaged = true
					act.StoryIDs = res.StoryIDs
				}
			}
		}
		report.Actions = append(report.Actions, act)
	}
	return report, nil
}
