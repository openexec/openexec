package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// fakeRunner returns canned output per invocation and records the argv it saw.
// It never spawns real gh.
type fakeRunner struct {
	out   []byte
	err   error
	calls [][]string
}

func (f *fakeRunner) Run(ctx context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	return f.out, f.err
}

func newStore(t *testing.T) governance.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatalf("mkdir .openexec: %v", err)
	}
	_, store, err := governance.Open(dir)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

const issueJSON = `{
	"number": 123,
	"title": "Login button is misaligned",
	"body": "On mobile the login button overflows.",
	"url": "https://github.com/agenticsnz/unsorry/issues/123",
	"state": "OPEN",
	"labels": [{"name": "bug"}],
	"comments": []
}`

func TestImportIssueMapsFields(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)
	runner := &fakeRunner{out: []byte(issueJSON)}

	rec, err := ImportIssue(ctx, runner, store, "unsorry", "agenticsnz/unsorry", 123)
	if err != nil {
		t.Fatalf("ImportIssue: %v", err)
	}

	if rec.SourceType != governance.SourceGitHubIssue {
		t.Errorf("SourceType = %q, want %q", rec.SourceType, governance.SourceGitHubIssue)
	}
	if rec.SourceID != "123" {
		t.Errorf("SourceID = %q, want %q", rec.SourceID, "123")
	}
	if rec.ProjectID != "unsorry" {
		t.Errorf("ProjectID = %q, want %q", rec.ProjectID, "unsorry")
	}
	if rec.Title != "Login button is misaligned" {
		t.Errorf("Title = %q", rec.Title)
	}
	if rec.RawText != "On mobile the login button overflows." {
		t.Errorf("RawText = %q", rec.RawText)
	}
	if rec.SourceURL != "https://github.com/agenticsnz/unsorry/issues/123" {
		t.Errorf("SourceURL = %q", rec.SourceURL)
	}
	if rec.Status != governance.ChangeStatusCandidate {
		t.Errorf("Status = %q, want candidate", rec.Status)
	}
	if rec.Kind != governance.KindBug {
		t.Errorf("Kind = %q, want %q (from label)", rec.Kind, governance.KindBug)
	}

	// gh was invoked with the documented argv (no shell string).
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(runner.calls))
	}
	want := []string{"issue", "view", "123", "--repo", "agenticsnz/unsorry", "--json", "number,title,body,labels,url,state,comments"}
	if got := strings.Join(runner.calls[0], " "); got != strings.Join(want, " ") {
		t.Errorf("gh argv = %v, want %v", runner.calls[0], want)
	}
}

func TestImportIssueIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	first, err := ImportIssue(ctx, &fakeRunner{out: []byte(issueJSON)}, store, "unsorry", "agenticsnz/unsorry", 123)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}

	// Simulate a governance decision taken after the first import: status and
	// kind move beyond the imported defaults. Re-import must preserve them.
	first.Status = governance.ChangeStatusApprovedForAI
	first.Kind = governance.KindReliability
	if err := store.UpdateChangeRecord(ctx, first); err != nil {
		t.Fatalf("update after import: %v", err)
	}

	// Re-import with an edited title/body (the upstream issue changed).
	updatedJSON := `{
		"number": 123,
		"title": "Login button overflow on mobile",
		"body": "Updated description.",
		"url": "https://github.com/agenticsnz/unsorry/issues/123",
		"state": "OPEN",
		"labels": [{"name": "bug"}],
		"comments": []
	}`
	second, err := ImportIssue(ctx, &fakeRunner{out: []byte(updatedJSON)}, store, "unsorry", "agenticsnz/unsorry", 123)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}

	// Exactly one record exists for this source key.
	all, err := store.ListChangeRecords(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	count := 0
	for _, c := range all {
		if c.SourceType == governance.SourceGitHubIssue && c.SourceID == "123" && c.ProjectID == "unsorry" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected 1 record for source key, got %d", count)
	}

	// Same ID reused.
	if second.ID != first.ID {
		t.Errorf("ID changed on re-import: %q -> %q", first.ID, second.ID)
	}
	// Mutable source fields updated.
	if second.Title != "Login button overflow on mobile" {
		t.Errorf("Title not updated: %q", second.Title)
	}
	if second.RawText != "Updated description." {
		t.Errorf("RawText not updated: %q", second.RawText)
	}
	// Governance decisions preserved.
	if second.Status != governance.ChangeStatusApprovedForAI {
		t.Errorf("Status clobbered: %q, want approved_for_ai", second.Status)
	}
	if second.Kind != governance.KindReliability {
		t.Errorf("Kind clobbered: %q, want reliability", second.Kind)
	}
}

func TestSyncLabelsMapping(t *testing.T) {
	cases := []struct {
		status string
		label  string
	}{
		{governance.ChangeStatusCandidate, "ai:triage"},
		{governance.ChangeStatusPlanned, "ai:plan-ready"},
		{governance.ChangeStatusPlanReady, "ai:plan-ready"},
		{governance.ChangeStatusChangesRequested, "ai:changes-requested"},
		{governance.ChangeStatusApprovedForAI, "ai:approved"},
		{governance.ChangeStatusImplementing, "ai:implementing"},
		{governance.ChangeStatusPROpen, "ai:pr-open"},
		{governance.ChangeStatusReadyForTest, "ai:ready-for-test"},
		{governance.ChangeStatusDone, "ai:done"},
		{governance.ChangeStatusBlocked, "ai:blocked"},
		{governance.ChangeStatusRejected, "ai:rejected"},
		{governance.ChangeStatusDeferred, "ai:deferred"},
	}

	ctx := context.Background()
	for _, tc := range cases {
		runner := &fakeRunner{}
		if err := SyncLabels(ctx, runner, "agenticsnz/unsorry", 7, tc.status); err != nil {
			t.Fatalf("SyncLabels(%s): %v", tc.status, err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("status %s: expected 1 gh call, got %d", tc.status, len(runner.calls))
		}
		argv := runner.calls[0]
		joined := strings.Join(argv, " ")

		// Must add exactly the mapped label.
		if !containsPair(argv, "--add-label", tc.label) {
			t.Errorf("status %s: argv %q missing --add-label %s", tc.status, joined, tc.label)
		}
		// Must remove every other governance label and never the target.
		removeIdx := indexOf(argv, "--remove-label")
		if removeIdx < 0 || removeIdx+1 >= len(argv) {
			t.Fatalf("status %s: missing --remove-label value", tc.status)
		}
		removed := strings.Split(argv[removeIdx+1], ",")
		if contains(removed, tc.label) {
			t.Errorf("status %s: target label %s present in --remove-label", tc.status, tc.label)
		}
		if len(removed) != len(GovernanceLabels)-1 {
			t.Errorf("status %s: removed %d labels, want %d", tc.status, len(removed), len(GovernanceLabels)-1)
		}
	}
}

func TestSyncLabelsUnknownStatus(t *testing.T) {
	runner := &fakeRunner{}
	err := SyncLabels(context.Background(), runner, "agenticsnz/unsorry", 1, "not_a_status")
	if err == nil {
		t.Fatal("expected error for unknown status")
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no gh call on unknown status, got %d", len(runner.calls))
	}
}

func TestPostComment(t *testing.T) {
	runner := &fakeRunner{}
	body := "OpenExec: linked to CHANGE-github-unsorry-123"
	if err := PostComment(context.Background(), runner, "agenticsnz/unsorry", 123, body); err != nil {
		t.Fatalf("PostComment: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected 1 gh call, got %d", len(runner.calls))
	}
	argv := runner.calls[0]
	want := []string{"issue", "comment", "123", "--repo", "agenticsnz/unsorry", "--body", body}
	if strings.Join(argv, "\x00") != strings.Join(want, "\x00") {
		t.Errorf("argv = %v, want %v", argv, want)
	}
}

func TestParseCommentCommand(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantCmd string
		wantArg string
		wantOK  bool
	}{
		{"review", "/openexec review", "review", "", true},
		{"approve", "/openexec approve", "approve", "", true},
		{"ready-for-test", "/openexec ready-for-test", "ready-for-test", "", true},
		{"revise", "/openexec revise please add a test for the empty case", "revise", "please add a test for the empty case", true},
		{"reject", "/openexec reject out of scope for this release", "reject", "out of scope for this release", true},
		{"defer", "/openexec defer revisit after Q3", "defer", "revisit after Q3", true},
		{"embedded in text", "thanks!\n\n/openexec approve\n\n-- the team", "approve", "", true},
		{"leading spaces", "   /openexec review  ", "review", "", true},
		{"non-command line", "this is just a regular comment", "", "", false},
		{"unknown command", "/openexec frobnicate now", "", "", false},
		{"bare prefix", "/openexec", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, arg, ok := ParseCommentCommand(tc.body)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if cmd != tc.wantCmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.wantCmd)
			}
			if arg != tc.wantArg {
				t.Errorf("arg = %q, want %q", arg, tc.wantArg)
			}
		})
	}
}

// --- small slice helpers (test-only) ---

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func containsPair(argv []string, flag, val string) bool {
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == flag && argv[i+1] == val {
			return true
		}
	}
	return false
}
