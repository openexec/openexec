package service

import (
	"context"
	"fmt"
	"sort"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/connectors/github"
)

// IngestAction records what happened for one /openexec comment command.
type IngestAction struct {
	ChangeID  string
	IssueNum  int
	CommentID int64
	Author    string
	Command   string
	Applied   bool
	Message   string
}

// IngestReport summarizes one ingestion poll.
type IngestReport struct {
	ScannedChanges int
	Actions        []IngestAction
}

// IngestGitHubComments polls the GitHub issues backing this project's
// github-sourced change records, executes any new /openexec commands found in
// their comments, and posts a reply with the result. It requires a Runner.
//
// SECURITY — author authorization: authorMap maps a GitHub login to a review
// authority id. A command from an author NOT in the map is ignored (never
// executed) and answered with a "not authorized" reply. Combined with the
// service's own gates (approve additionally requires an operator session, and
// the mapped authority must actually hold the needed permission/risk limit),
// this prevents an arbitrary issue commenter from driving privileged actions.
// The map must be operator-owned (loaded outside the agent-writable workspace);
// the caller is responsible for supplying a trusted map.
//
// Idempotency: a per-change comment-id cursor (store.GetIngestCursor/
// SetIngestCursor) ensures each comment is processed at most once across polls.
func (s *Service) IngestGitHubComments(ctx context.Context, projectID, repo string, authorMap map[string]string) (*IngestReport, error) {
	if s.runner == nil {
		return nil, errMissingRunner
	}
	all, err := s.store.ListChangeRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("list change records: %w", err)
	}
	report := &IngestReport{}
	for _, ch := range all {
		if ch.SourceType != governance.SourceGitHubIssue || ch.ProjectID != projectID {
			continue
		}
		chRepo, number, ok := repoFromIssueURL(ch.SourceURL)
		if !ok || chRepo != repo {
			continue
		}
		report.ScannedChanges++

		cursor, err := s.store.GetIngestCursor(ctx, ch.ID)
		if err != nil {
			return nil, err
		}
		comments, err := github.ListIssueComments(ctx, s.runner, repo, number)
		if err != nil {
			return nil, err
		}
		// Process strictly in comment-id order so a later cursor write can't skip
		// an earlier unprocessed command.
		sort.Slice(comments, func(i, j int) bool { return comments[i].ID < comments[j].ID })

		maxSeen := cursor
		for _, c := range comments {
			if c.ID <= cursor {
				continue
			}
			if c.ID > maxSeen {
				maxSeen = c.ID
			}
			cmd, arg, isCmd := github.ParseCommentCommand(c.Body)
			if !isCmd {
				continue
			}
			applied, msg := s.dispatchCommentCommand(ctx, ch, cmd, arg, c.Author, authorMap)
			report.Actions = append(report.Actions, IngestAction{
				ChangeID: ch.ID, IssueNum: number, CommentID: c.ID,
				Author: c.Author, Command: cmd, Applied: applied, Message: msg,
			})
			// Reply on the issue so the human sees the outcome; best-effort.
			_ = github.PostComment(ctx, s.runner, repo, number, "**OpenExec** — "+msg)
		}
		if maxSeen != cursor {
			if err := s.store.SetIngestCursor(ctx, ch.ID, maxSeen); err != nil {
				return nil, err
			}
		}
	}
	return report, nil
}

// dispatchCommentCommand authorizes the author and routes a parsed /openexec
// command to the matching Service operation. It returns whether the action was
// applied and a human-readable message for the reply comment.
func (s *Service) dispatchCommentCommand(ctx context.Context, ch *governance.ChangeRecord, cmd, arg, author string, authorMap map[string]string) (bool, string) {
	authorityID, ok := authorMap[author]
	if !ok {
		return false, fmt.Sprintf("ignored `/openexec %s` from @%s — not an authorized approver (add them to the operator's github-approvers map).", cmd, author)
	}

	var err error
	switch cmd {
	case "review":
		_, err = s.ReviewPlan(ctx, ch.ID, authorityID, authorityID)
	case "approve":
		err = s.ApproveChange(ctx, ch.ID, authorityID)
	case "reject":
		err = s.RejectChange(ctx, ch.ID, authorityID, arg)
	case "defer":
		err = s.DeferChange(ctx, ch.ID, authorityID, arg)
	case "revise":
		err = s.RequestRevision(ctx, ch.ID, authorityID, arg)
	case "ready-for-test":
		err = s.ReadyForTest(ctx, ch.ID)
	default:
		return false, fmt.Sprintf("unknown command `/openexec %s`.", cmd)
	}
	if err != nil {
		return false, fmt.Sprintf("`/openexec %s` by @%s (as `%s`) was refused: %v", cmd, author, authorityID, err)
	}
	return true, fmt.Sprintf("`/openexec %s` by @%s applied as authority `%s` (change %s is now %s).", cmd, author, authorityID, ch.ID, s.currentStatus(ctx, ch.ID))
}

// currentStatus re-reads a change's status for the reply message (best-effort).
func (s *Service) currentStatus(ctx context.Context, changeID string) string {
	if ch, err := s.getChange(ctx, changeID); err == nil {
		return ch.Status
	}
	return "updated"
}
