package service

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// routedCompleter returns impact YAML for the impact prompt and operability YAML
// for the operability prompt, so a single fake drives both analyses.
type routedCompleter struct{ impact, operability string }

func (r routedCompleter) Complete(_ context.Context, prompt string) (string, error) {
	if strings.Contains(prompt, "which files a change will touch") {
		return r.impact, nil
	}
	return r.operability, nil
}

func TestAssessChange_RecordsAuditEventAndRenders(t *testing.T) {
	store := newTestStore(t)
	impactYAML := "```yaml\nfiles:\n  - path: storefront/src/app/api/upload/route.ts\n    action: modify\n    reason: bump MAX_BYTES to 40 MB\nnotes: only one file\n```"
	opYAML := "```yaml\nrollback_safe: yes\ndb_migration: none\ndeploy_risk: low\nmitigations: []\nmonitoring: []\nnotes: pure constant change\n```"
	svc := NewService(store, Options{Completer: routedCompleter{impactYAML, opYAML}})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusPROpen, Kind: governance.KindFeature,
		Risk: governance.RiskMedium, Light: true, Title: "Raise limit", RawText: "raise MAX_BYTES to 40",
	})

	imp, op, err := svc.AssessChange(ctx, "C-1", "excerpt: const MAX_BYTES = 20 * 1024 * 1024")
	if err != nil {
		t.Fatalf("AssessChange: %v", err)
	}
	if imp == nil || len(imp.Files) != 1 || op == nil || op.RollbackSafe != "yes" || op.DBMigration != "none" {
		t.Fatalf("unexpected reports: imp=%+v op=%+v", imp, op)
	}

	// The AI risk evaluation must be recorded in the hash-chained trail.
	events, _, _ := svc.History(ctx, "C-1")
	var sawAssessed bool
	for _, e := range events {
		if e.Decision == decisionAssessed && e.ActorType == governance.ActorTypeAI {
			sawAssessed = true
			if !strings.Contains(e.Comment, "rollback=yes") || !strings.Contains(e.Comment, "deploy_risk=low") {
				t.Errorf("assessment event summary missing risk fields: %q", e.Comment)
			}
		}
	}
	if !sawAssessed {
		t.Fatalf("no 'assessed' decision event recorded")
	}

	// The rendered PR comment must show the file and the operability verdicts.
	md, err := svc.PRAssessmentMarkdown(ctx, "C-1")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for _, want := range []string{"upload/route.ts", "Rollback safe:** yes", "DB migration:** none", "Deploy risk:** low", "lightweight"} {
		if !strings.Contains(md, want) {
			t.Errorf("assessment markdown missing %q:\n%s", want, md)
		}
	}

	// The chain must still verify with the assessment event in it.
	if ok, reason, _, _ := store.VerifyAuditChain(ctx); !ok {
		t.Fatalf("audit chain broken after assessment: %s", reason)
	}
}
