// Package github is the GitHub connector for OpenExec release governance.
//
// It imports GitHub issues into governance ChangeRecords, mirrors governance
// status to a fixed set of ai:* labels, posts OpenExec comments, and parses the
// /openexec slash commands humans use to drive governance from GitHub.
//
// All GitHub interaction goes through the Runner abstraction, which shells out
// to the `gh` CLI (reusing the operator's existing gh auth). Tests inject a fake
// Runner so the connector is exercised without ever invoking real gh.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/governance"
)

// Runner executes a `gh` invocation as an argv array and returns its stdout.
// The interface exists so tests capture argv and return canned JSON instead of
// spawning the real gh binary.
type Runner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// DefaultTimeout bounds a single gh invocation.
const DefaultTimeout = 60 * time.Second

// ExecRunner is the default Runner. It runs `gh` via exec.CommandContext with a
// strict argv array: there is no shell, no `sh -c`, and no string joining, so an
// argument like "; rm -rf /" is one literal argument gh will reject.
type ExecRunner struct {
	// Timeout per invocation; zero means DefaultTimeout.
	Timeout time.Duration
}

// Run implements Runner by shelling out to gh.
func (e *ExecRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	timeout := e.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

// GovernanceLabels is the fixed governance label set applied to issues. Exactly
// one of these mirrors the ChangeRecord status at any time.
var GovernanceLabels = []string{
	"ai:triage",
	"ai:plan-ready",
	"ai:changes-requested",
	"ai:approved",
	"ai:implementing",
	"ai:pr-open",
	"ai:ready-for-test",
	"ai:done",
	"ai:blocked",
	"ai:rejected",
	"ai:deferred",
}

// StatusLabels maps a governance ChangeRecord status to its mirrored ai:* label.
// Both "planned" and "plan_ready" map to ai:plan-ready (a drafted plan and a
// review-ready plan are the same signal to a GitHub watcher).
var StatusLabels = map[string]string{
	governance.ChangeStatusCandidate:        "ai:triage",
	governance.ChangeStatusPlanned:          "ai:plan-ready",
	governance.ChangeStatusPlanReady:        "ai:plan-ready",
	governance.ChangeStatusChangesRequested: "ai:changes-requested",
	governance.ChangeStatusApprovedForAI:    "ai:approved",
	governance.ChangeStatusImplementing:     "ai:implementing",
	governance.ChangeStatusPROpen:           "ai:pr-open",
	governance.ChangeStatusReadyForTest:     "ai:ready-for-test",
	governance.ChangeStatusDone:             "ai:done",
	governance.ChangeStatusBlocked:          "ai:blocked",
	governance.ChangeStatusRejected:         "ai:rejected",
	governance.ChangeStatusDeferred:         "ai:deferred",
}

// LabelForStatus returns the governance label for a status, and ok=false for an
// unrecognized status.
func LabelForStatus(status string) (label string, ok bool) {
	label, ok = StatusLabels[status]
	return label, ok
}

// ghLabel is one label in `gh issue view --json labels` output.
type ghLabel struct {
	Name string `json:"name"`
}

// ghIssue is the subset of `gh issue view --json ...` we consume.
type ghIssue struct {
	Number int       `json:"number"`
	Title  string    `json:"title"`
	Body   string    `json:"body"`
	URL    string    `json:"url"`
	State  string    `json:"state"`
	Labels []ghLabel `json:"labels"`
}

// ImportIssue imports a GitHub issue into a governance ChangeRecord, idempotently.
//
// It runs `gh issue view`, maps the issue onto a ChangeRecord
// (SourceType=github_issue, SourceID=stringified number, SourceURL, Title,
// RawText=body), then upserts via the store:
//   - If a record with the same source key (github_issue, number, projectID)
//     already exists, its ID, Status, Kind, and Risk are preserved and only the
//     source-derived mutable fields (Title, RawText, SourceURL) are updated.
//   - If none exists, a new record is created with status "candidate" and an
//     initial Kind inferred from the issue labels (Risk is left for AI triage).
//
// runner is taken explicitly (like SyncLabels/PostComment) so callers and tests
// control GitHub access; the contract is "never invoke real gh in a test".
func ImportIssue(ctx context.Context, runner Runner, store governance.Store, projectID, repo string, number int) (*governance.ChangeRecord, error) {
	out, err := runner.Run(ctx,
		"issue", "view", strconv.Itoa(number),
		"--repo", repo,
		"--json", "number,title,body,labels,url,state,comments",
	)
	if err != nil {
		return nil, fmt.Errorf("github: view issue %d in %s: %w", number, repo, err)
	}

	var issue ghIssue
	if err := json.Unmarshal(out, &issue); err != nil {
		return nil, fmt.Errorf("github: parse issue %d: %w", number, err)
	}

	sourceID := strconv.Itoa(number)
	existing, err := findBySourceKey(ctx, store, projectID, sourceID)
	if err != nil {
		return nil, err
	}

	if existing != nil {
		// Update source-derived fields only; preserve governance decisions
		// (ID, Status, Kind, Risk) made after the original import.
		existing.Title = issue.Title
		existing.RawText = issue.Body
		existing.SourceURL = issue.URL
		if err := store.UpdateChangeRecord(ctx, existing); err != nil {
			return nil, fmt.Errorf("github: update change record %s: %w", existing.ID, err)
		}
		return existing, nil
	}

	rec := &governance.ChangeRecord{
		ID:         fmt.Sprintf("CHANGE-github-%s-%d", projectID, number),
		ProjectID:  projectID,
		SourceType: governance.SourceGitHubIssue,
		SourceID:   sourceID,
		SourceURL:  issue.URL,
		Title:      issue.Title,
		RawText:    issue.Body,
		Kind:       kindFromLabels(issue.Labels),
		Status:     governance.ChangeStatusCandidate,
	}
	if err := store.CreateChangeRecord(ctx, rec); err != nil {
		return nil, fmt.Errorf("github: create change record for issue %d: %w", number, err)
	}
	return rec, nil
}

// findBySourceKey returns the existing change record matching the GitHub source
// key (github_issue, sourceID, projectID), or nil if none exists. The store
// enforces this key as UNIQUE, so at most one record matches.
func findBySourceKey(ctx context.Context, store governance.Store, projectID, sourceID string) (*governance.ChangeRecord, error) {
	all, err := store.ListChangeRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("github: list change records: %w", err)
	}
	for _, c := range all {
		if c.SourceType == governance.SourceGitHubIssue && c.SourceID == sourceID && c.ProjectID == projectID {
			return c, nil
		}
	}
	return nil, nil
}

// kindFromLabels infers an initial change Kind from issue labels. It is a cheap
// triage hint only: AI triage (Phase 6) sets the authoritative Kind/Risk, and
// re-import never overwrites it. Returns "" (unset) when no label matches.
func kindFromLabels(labels []ghLabel) string {
	for _, l := range labels {
		switch strings.ToLower(l.Name) {
		case "bug", "defect":
			return governance.KindBug
		case "feature", "enhancement":
			return governance.KindFeature
		case "docs", "documentation":
			return governance.KindDocs
		case "ops", "infra", "infrastructure":
			return governance.KindOps
		case "security":
			return governance.KindSecurity
		case "reliability":
			return governance.KindReliability
		case "question", "support":
			return governance.KindSupportQuestion
		}
	}
	return ""
}

// SyncLabels mirrors a governance status onto the issue's labels: it adds the
// status's ai:* label and removes every other governance label, so the issue
// carries exactly one governance label matching OpenExec state. It returns an
// error for an unrecognized status (no gh call is made in that case).
func SyncLabels(ctx context.Context, runner Runner, repo string, number int, status string) error {
	target, ok := LabelForStatus(status)
	if !ok {
		return fmt.Errorf("github: no governance label for status %q", status)
	}

	remove := make([]string, 0, len(GovernanceLabels)-1)
	for _, l := range GovernanceLabels {
		if l != target {
			remove = append(remove, l)
		}
	}

	_, err := runner.Run(ctx,
		"issue", "edit", strconv.Itoa(number),
		"--repo", repo,
		"--add-label", target,
		"--remove-label", strings.Join(remove, ","),
	)
	if err != nil {
		return fmt.Errorf("github: sync labels for issue %d (%s): %w", number, status, err)
	}
	return nil
}

// PostComment posts a comment to the issue via `gh issue comment`. body is passed
// as a single argv element, never interpolated into a shell string.
func PostComment(ctx context.Context, runner Runner, repo string, number int, body string) error {
	_, err := runner.Run(ctx,
		"issue", "comment", strconv.Itoa(number),
		"--repo", repo,
		"--body", body,
	)
	if err != nil {
		return fmt.Errorf("github: post comment to issue %d: %w", number, err)
	}
	return nil
}

// knownCommands is the set of /openexec slash commands ParseCommentCommand
// recognizes. The value reports whether the command takes a free-text argument.
var knownCommands = map[string]bool{
	"review":         false,
	"approve":        false,
	"ready-for-test": false,
	"revise":         true,
	"reject":         true,
	"defer":          true,
}

// ParseCommentCommand parses the first `/openexec <cmd> [arg]` line in a GitHub
// comment body. It is a pure function (no I/O).
//
// Recognized commands: review, approve, ready-for-test (no argument) and
// revise <comment>, reject <reason>, defer <reason> (free-text argument).
// It returns ok=false when no recognized command line is present.
func ParseCommentCommand(body string) (cmd string, arg string, ok bool) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "/openexec") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "/openexec"))
		if rest == "" {
			continue
		}
		fields := strings.Fields(rest)
		name := strings.ToLower(fields[0])
		if _, known := knownCommands[name]; !known {
			continue
		}
		argText := strings.TrimSpace(strings.TrimPrefix(rest, fields[0]))
		return name, argText, true
	}
	return "", "", false
}

// IssueComment is a single GitHub issue comment, the unit of command ingestion.
type IssueComment struct {
	ID        int64
	Author    string // GitHub login of the comment author
	Body      string
	CreatedAt time.Time
}

// ListIssueComments returns an issue's comments (oldest first) via
// `gh api repos/<repo>/issues/<number>/comments`. The numeric comment ID is a
// stable cursor for idempotent ingestion; the author login drives authorization.
func ListIssueComments(ctx context.Context, runner Runner, repo string, number int) ([]IssueComment, error) {
	out, err := runner.Run(ctx, "api", fmt.Sprintf("repos/%s/issues/%d/comments", repo, number), "--paginate")
	if err != nil {
		return nil, fmt.Errorf("gh api list comments for %s#%d: %w", repo, number, err)
	}
	var raw []struct {
		ID        int64  `json:"id"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse issue comments: %w", err)
	}
	comments := make([]IssueComment, 0, len(raw))
	for _, r := range raw {
		t, _ := time.Parse(time.RFC3339, r.CreatedAt)
		comments = append(comments, IssueComment{ID: r.ID, Author: r.User.Login, Body: r.Body, CreatedAt: t})
	}
	return comments, nil
}

// ListOpenIssues returns the numbers of open issues in a repo, optionally
// filtered to a single label, via `gh issue list`. Used by the automation
// sync-in to discover flagged work (e.g. label "ai:triage").
func ListOpenIssues(ctx context.Context, runner Runner, repo, label string) ([]int, error) {
	args := []string{"issue", "list", "--repo", repo, "--state", "open", "--json", "number", "--limit", "200"}
	if label != "" {
		args = append(args, "--label", label)
	}
	out, err := runner.Run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("gh issue list for %s: %w", repo, err)
	}
	var raw []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}
	nums := make([]int, 0, len(raw))
	for _, r := range raw {
		nums = append(nums, r.Number)
	}
	return nums, nil
}
