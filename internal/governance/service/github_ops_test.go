package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

func TestRepoFromIssueURL(t *testing.T) {
	cases := []struct {
		url      string
		wantRepo string
		wantNum  int
		wantOK   bool
	}{
		{"https://github.com/agenticsnz/unsorry/issues/123", "agenticsnz/unsorry", 123, true},
		{"https://github.com/org/repo/issues/1", "org/repo", 1, true},
		{"https://github.com/org/repo/pull/1", "", 0, false},
		{"https://example.com/x/y/issues/1", "", 0, false},
		{"not a url", "", 0, false},
	}
	for _, c := range cases {
		repo, num, ok := repoFromIssueURL(c.url)
		if ok != c.wantOK || repo != c.wantRepo || num != c.wantNum {
			t.Fatalf("repoFromIssueURL(%q) = (%q,%d,%v), want (%q,%d,%v)", c.url, repo, num, ok, c.wantRepo, c.wantNum, c.wantOK)
		}
	}
}

// recordingRunner captures each gh invocation's argv for assertions.
type recordingRunner struct{ calls [][]string }

func (r *recordingRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	r.calls = append(r.calls, args)
	return []byte("{}"), nil
}

func TestSyncGitHubState(t *testing.T) {
	store := newTestStore(t)
	runner := &recordingRunner{}
	svc := NewService(store, Options{Runner: runner})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:         "CHANGE-github-proj-7",
		SourceType: governance.SourceGitHubIssue,
		SourceID:   "7",
		SourceURL:  "https://github.com/org/repo/issues/7",
		Status:     governance.ChangeStatusApprovedForAI,
	})

	if err := svc.SyncGitHubState(ctx, "CHANGE-github-proj-7"); err != nil {
		t.Fatalf("SyncGitHubState: %v", err)
	}
	// Expect at least a label edit and a comment post, both naming the issue.
	var sawLabel, sawComment bool
	for _, c := range runner.calls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "issue") && strings.Contains(joined, "edit") {
			sawLabel = true
		}
		if strings.Contains(joined, "issue") && strings.Contains(joined, "comment") {
			sawComment = true
		}
	}
	if !sawLabel || !sawComment {
		t.Fatalf("expected a label edit and a comment post, got calls %+v", runner.calls)
	}

	// A non-github-sourced change is rejected.
	seedChange(t, store, &governance.ChangeRecord{ID: "M-1", SourceType: governance.SourceManual, SourceID: "M-1"})
	if err := svc.SyncGitHubState(ctx, "M-1"); err == nil {
		t.Fatalf("expected SyncGitHubState to reject a non-github change")
	}
}
