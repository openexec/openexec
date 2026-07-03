package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// checksRunner fakes gh: it returns a head SHA for `pr view` and a canned
// check-runs payload for the `api .../check-runs` call.
type checksRunner struct {
	sha    string
	checks string
}

func (r *checksRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "pr view"):
		return []byte(r.sha + "\n"), nil
	case strings.Contains(joined, "check-runs"):
		return []byte(r.checks), nil
	default:
		return []byte("{}"), nil
	}
}

func TestRecordEvidence_RefusesTrustedSource(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusPROpen})

	// The manual path must not be able to assert a trusted source.
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindCI, governance.EvidenceSourceGitHub, "spoof", "", ""); err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("expected github source to be refused, got %v", err)
	}
	// An untrusted source is still allowed.
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindTest, governance.EvidenceSourceAgent, "ok", "", ""); err != nil {
		t.Fatalf("agent source should be allowed: %v", err)
	}
}

func TestSyncGitHubChecks_RecordsTrustedEvidenceWhenGreen(t *testing.T) {
	store := newTestStore(t)
	green := `{"check_runs":[{"id":1,"name":"build","status":"completed","conclusion":"success"},{"id":2,"name":"test","status":"completed","conclusion":"success"}]}`
	svc := NewService(store, Options{Runner: &checksRunner{sha: "abc123def", checks: green}})
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusPROpen, PRURL: "https://github.com/o/r/pull/7"})

	if err := svc.SyncGitHubChecks(ctx, "C-1"); err != nil {
		t.Fatalf("SyncGitHubChecks: %v", err)
	}
	ev, _ := store.ListEvidence(ctx, "C-1")
	if len(ev) != 1 || ev[0].Source != governance.EvidenceSourceGitHub || ev[0].Kind != governance.EvidenceKindCI {
		t.Fatalf("expected 1 trusted github CI evidence, got %+v", ev)
	}
	if !strings.Contains(ev[0].Summary, "abc123def") || !strings.Contains(ev[0].Summary, "sha256") {
		t.Errorf("evidence must carry SHA + payload-hash provenance: %q", ev[0].Summary)
	}
}

func TestSyncGitHubChecks_NoEvidenceWhenNotGreen(t *testing.T) {
	store := newTestStore(t)
	red := `{"check_runs":[{"id":1,"name":"build","status":"completed","conclusion":"success"},{"id":2,"name":"test","status":"completed","conclusion":"failure"}]}`
	svc := NewService(store, Options{Runner: &checksRunner{sha: "def456", checks: red}})
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusPROpen, PRURL: "https://github.com/o/r/pull/7"})

	if err := svc.SyncGitHubChecks(ctx, "C-1"); err == nil || !strings.Contains(err.Error(), "not green") {
		t.Fatalf("expected not-green error, got %v", err)
	}
	if ev, _ := store.ListEvidence(ctx, "C-1"); len(ev) != 0 {
		t.Fatalf("no evidence should be recorded when checks are not green, got %d", len(ev))
	}
}
