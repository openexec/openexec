package service

import (
	"context"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// syncRunner scripts gh for the sync-in: `issue list` returns the issue-list
// JSON; `issue view <n>` returns that issue's JSON; other calls return "{}".
type syncRunner struct {
	list  string
	views map[string]string
}

func (r *syncRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	if len(args) >= 2 && args[0] == "issue" && args[1] == "list" {
		return []byte(r.list), nil
	}
	if len(args) >= 3 && args[0] == "issue" && args[1] == "view" {
		if v, ok := r.views[args[2]]; ok {
			return []byte(v), nil
		}
	}
	return []byte("{}"), nil
}

func TestSyncGitHubIssues_ImportsAndAutoTriages(t *testing.T) {
	store := newTestStore(t)
	ps := newFakePlanStore()
	runner := &syncRunner{
		list: `[{"number":7},{"number":8}]`,
		views: map[string]string{
			"7": `{"number":7,"title":"Login broken","body":"cannot log in","url":"https://github.com/org/repo/issues/7","state":"OPEN","labels":[{"name":"bug"}]}`,
			"8": `{"number":8,"title":"Dark mode","body":"add dark mode","url":"https://github.com/org/repo/issues/8","state":"OPEN","labels":[{"name":"enhancement"}]}`,
		},
	}
	svc := NewService(store, Options{
		Runner:    runner,
		PlanStore: ps,
		Completer: routingCompleter{plan: deepPlanJSON, classi: "kind: bug\nrisk: high\n"},
	})
	ctx := context.Background()

	rep, err := svc.SyncGitHubIssues(ctx, "proj", "org/repo", "", true)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if rep.Scanned != 2 {
		t.Fatalf("expected 2 scanned, got %d", rep.Scanned)
	}
	triaged := 0
	for _, a := range rep.Actions {
		if a.Error != "" {
			t.Fatalf("unexpected error for issue #%d: %s", a.IssueNum, a.Error)
		}
		if a.Triaged {
			triaged++
			ch, _ := store.GetChangeRecord(ctx, a.ChangeID)
			if ch.Status != governance.ChangeStatusPlanReady {
				t.Fatalf("triaged change %s should be plan_ready, got %q", a.ChangeID, ch.Status)
			}
		}
	}
	if triaged != 2 {
		t.Fatalf("expected both fresh issues auto-triaged, got %d", triaged)
	}

	// Re-sync must be idempotent: nothing re-imported or re-triaged.
	rep2, err := svc.SyncGitHubIssues(ctx, "proj", "org/repo", "", true)
	if err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	for _, a := range rep2.Actions {
		if a.Imported || a.Triaged {
			t.Fatalf("re-sync must not re-import/re-triage issue #%d (%+v)", a.IssueNum, a)
		}
	}
}

func TestSyncGitHubIssues_ImportOnlyWithoutCompleter(t *testing.T) {
	store := newTestStore(t)
	runner := &syncRunner{
		list:  `[{"number":7}]`,
		views: map[string]string{"7": `{"number":7,"title":"x","body":"y","url":"https://github.com/org/repo/issues/7","state":"OPEN"}`},
	}
	// No Completer/PlanStore: sync imports but does not triage, and does not error.
	svc := NewService(store, Options{Runner: runner})
	rep, err := svc.SyncGitHubIssues(context.Background(), "proj", "org/repo", "", true)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(rep.Actions) != 1 || !rep.Actions[0].Imported || rep.Actions[0].Triaged {
		t.Fatalf("expected import-only, got %+v", rep.Actions)
	}
}
