package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/connectors/github"
)

// repoFromIssueURL parses "https://github.com/<owner>/<repo>/issues/<n>" into
// "<owner>/<repo>" and the issue number. Repo is not stored on the change
// record; it is derived from the imported issue's SourceURL.
func repoFromIssueURL(u string) (repo string, number int, ok bool) {
	const marker = "github.com/"
	i := strings.Index(u, marker)
	if i < 0 {
		return "", 0, false
	}
	parts := strings.Split(strings.Trim(u[i+len(marker):], "/"), "/")
	if len(parts) < 4 || parts[2] != "issues" {
		return "", 0, false
	}
	n, err := strconv.Atoi(parts[3])
	if err != nil {
		return "", 0, false
	}
	return parts[0] + "/" + parts[1], n, true
}

// SyncGitHubState mirrors a github-sourced change's current governance status
// back to its GitHub issue: it sets the single ai:* state label and posts a
// status + next-action comment. Requires a configured Runner (gh CLI). This is
// the explicit, operator-invokable write-back path (DoD #10 — "GitHub issue
// comments show status and next action"); status-changing ops additionally
// best-effort mirror the label via mirrorGitHubLabel.
func (s *Service) SyncGitHubState(ctx context.Context, changeID string) error {
	if s.runner == nil {
		return errMissingRunner
	}
	ch, err := s.getChange(ctx, changeID)
	if err != nil {
		return err
	}
	if ch.SourceType != governance.SourceGitHubIssue {
		return fmt.Errorf("change %s is not sourced from a GitHub issue", changeID)
	}
	repo, number, ok := repoFromIssueURL(ch.SourceURL)
	if !ok {
		return fmt.Errorf("cannot derive repo/issue from source URL %q", ch.SourceURL)
	}
	if err := github.SyncLabels(ctx, s.runner, repo, number, ch.Status); err != nil {
		return fmt.Errorf("sync labels for change %s: %w", changeID, err)
	}
	if err := github.PostComment(ctx, s.runner, repo, number, githubStatusComment(ch)); err != nil {
		return fmt.Errorf("post status comment for change %s: %w", changeID, err)
	}
	return nil
}

// mirrorGitHubLabel best-effort updates the GitHub issue label to match a
// github-sourced change's status. Errors are intentionally ignored: a gh
// failure must never block a governance state change — the label is a
// projection of OpenExec state, never the source of truth or an approval input.
func (s *Service) mirrorGitHubLabel(ctx context.Context, ch *governance.ChangeRecord) {
	if s.runner == nil || ch == nil || ch.SourceType != governance.SourceGitHubIssue {
		return
	}
	repo, number, ok := repoFromIssueURL(ch.SourceURL)
	if !ok {
		return
	}
	_ = github.SyncLabels(ctx, s.runner, repo, number, ch.Status)
}

// githubStatusComment renders the status + next-action comment posted to the
// source issue.
func githubStatusComment(ch *governance.ChangeRecord) string {
	next := githubNextAction(ch.Status)
	body := fmt.Sprintf("**OpenExec governance** — change `%s` is now **%s**.", ch.ID, ch.Status)
	if next != "" {
		body += "\n\n" + next
	}
	return body
}

// githubNextAction describes what happens next for a change in the given status.
func githubNextAction(status string) string {
	switch status {
	case governance.ChangeStatusPlanReady:
		return "Next: a reviewer comments and a human operator approves the plan."
	case governance.ChangeStatusChangesRequested:
		return "Next: address the requested changes; the plan is then re-triaged."
	case governance.ChangeStatusApprovedForAI:
		return "Next: an executor claims and implements the approved work."
	case governance.ChangeStatusPROpen:
		return "Next: CI/test evidence is recorded, then the change moves to ready-for-test."
	case governance.ChangeStatusReadyForTest:
		return "Next: a tester verifies against the acceptance criteria."
	case governance.ChangeStatusDone:
		return "Done: the change has been verified and closed."
	default:
		return ""
	}
}
