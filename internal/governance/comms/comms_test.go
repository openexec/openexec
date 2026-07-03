package comms

import (
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

func sampleRelease() *governance.GovernanceRelease {
	return &governance.GovernanceRelease{
		ID:         "R-2026-07",
		Name:       "July customer fixes",
		Owner:      "perttu",
		Status:     governance.ReleaseStatusReadyForTest,
		Goal:       "Ship the admin export fix and login docs",
		MustHave:   []string{"Export works for admins"},
		OutOfScope: []string{"Billing changes", "Mobile app"},
		Risk:       governance.RiskMedium,
	}
}

func sampleChanges() []*governance.ChangeRecord {
	return []*governance.ChangeRecord{
		{
			ID:                 "CHANGE-200",
			ReleaseID:          "R-2026-07",
			Title:              "Add CSV export to admin panel",
			Summary:            "Admins can now export the user list as CSV.",
			Kind:               governance.KindFeature,
			Risk:               governance.RiskMedium,
			Status:             governance.ChangeStatusDone,
			Plan:               "INTERNAL: refactor exportSvc and bypass auth cache",
			AcceptanceCriteria: "- Export button visible to admins\n- CSV has all columns",
			VerificationPlan:   "Log in as admin\nClick export\nOpen CSV",
			Branch:             "feat/csv-export",
			PRURL:              "https://github.com/org/repo/pull/200",
			SourceURL:          "https://github.com/org/repo/issues/123",
		},
		{
			ID:                 "CHANGE-100",
			ReleaseID:          "R-2026-07",
			Title:              "Fix login redirect loop",
			Summary:            "Resolved an issue where some users were stuck on the login screen.",
			Kind:               governance.KindBug,
			Risk:               governance.RiskLow,
			Status:             governance.ChangeStatusReadyForTest,
			AcceptanceCriteria: "No redirect loop after login",
			VerificationPlan:   "Log in with a fresh session",
			PRURL:              "https://github.com/org/repo/pull/100",
		},
		{
			ID:      "CHANGE-300",
			Title:   "Patch SQL injection in search",
			Summary: "Hardened the search endpoint against injection.",
			Kind:    governance.KindSecurity,
			Risk:    governance.RiskCritical,
			Status:  governance.ChangeStatusDone,
			Plan:    "SECRET: parameterize query in searchHandler.go line 88",
			Branch:  "sec/sql-injection",
		},
		{
			ID:     "CHANGE-400",
			Title:  "Investigate flaky CI",
			Kind:   governance.KindOps,
			Status: governance.ChangeStatusBlocked,
		},
	}
}

func TestGenerateExecutorBrief(t *testing.T) {
	rel := sampleRelease()
	ch := sampleChanges()[0]
	out := GenerateExecutorBrief(rel, ch, "/Users/perttu/projects/unsorry")

	for _, want := range []string{
		"CHANGE-200",
		"R-2026-07",
		"/Users/perttu/projects/unsorry",
		"## Allowed scope",
		"## Acceptance criteria",
		"## Verification",
		"## Required reporting",
		"PR URL",
		"Known risks",
		"Export button visible to admins", // acceptance criteria bullet
		"Billing changes",                 // out-of-scope guardrail from release
	} {
		if !strings.Contains(out, want) {
			t.Errorf("executor brief missing %q\n---\n%s", want, out)
		}
	}
}

func TestGenerateTesterHandoff(t *testing.T) {
	rel := sampleRelease()
	changes := sampleChanges()
	evidence := map[string][]*governance.Evidence{
		"CHANGE-200": {
			{ID: "EV-1", ChangeID: "CHANGE-200", Kind: governance.EvidenceKindTest, Source: governance.EvidenceSourceCLI, Summary: "go test ./... passed", URL: "https://ci/run/1"},
		},
	}
	out := GenerateTesterHandoff(rel, changes, evidence, "staging", "2026.07.1")

	for _, want := range []string{
		"# Tester Handoff",
		"**Version:** 2026.07.1",
		"**Environment:** staging",
		"## Included changes",
		"CHANGE-200",
		"CHANGE-100",
		"**Acceptance criteria:**", // per-change bold label
		"Verification steps",
		"## Known risks",
		"## Out of scope",
		"Billing changes",
		"https://github.com/org/repo/pull/200",   // PR link
		"https://github.com/org/repo/issues/123", // issue link
		"go test ./... passed",                   // recorded evidence
	} {
		if !strings.Contains(out, want) {
			t.Errorf("tester handoff missing %q\n---\n%s", want, out)
		}
	}

	// Deterministic ordering: CHANGE-100 sorts before CHANGE-200.
	if strings.Index(out, "CHANGE-100") > strings.Index(out, "CHANGE-200") {
		t.Errorf("changes not sorted by ID; CHANGE-100 should precede CHANGE-200\n%s", out)
	}
}

func TestGeneratePMSummary(t *testing.T) {
	rel := sampleRelease()
	out := GeneratePMSummary(rel, sampleChanges())

	for _, want := range []string{
		"# PM Summary",
		"**Status:** " + governance.ReleaseStatusReadyForTest,
		"## By status",
		"Ready for test", // CHANGE-100
		"Done",           // CHANGE-200 / CHANGE-300
		"Blocked",        // CHANGE-400
		"## Blockers",
		"CHANGE-400",
		"**Total items:** 4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("PM summary missing %q\n---\n%s", want, out)
		}
	}
}

func TestGeneratePMSummaryNoBlockers(t *testing.T) {
	rel := sampleRelease()
	changes := []*governance.ChangeRecord{
		{ID: "CHANGE-1", Title: "ok", Status: governance.ChangeStatusDone},
	}
	out := GeneratePMSummary(rel, changes)
	if !strings.Contains(out, "_No blockers._") {
		t.Errorf("expected no-blockers marker\n%s", out)
	}
}

func TestGenerateCustomerSummaryIncludesUserFacing(t *testing.T) {
	rel := sampleRelease()
	out := GenerateCustomerSummary(rel, sampleChanges())

	for _, want := range []string{
		"July customer fixes",
		"## What's New",
		"Admins can now export the user list as CSV.",
		// CHANGE-100 is ready_for_test (not done/deployed) -> excluded, so its
		// fix summary must NOT appear; we assert exclusion below.
	} {
		if !strings.Contains(out, want) {
			t.Errorf("customer summary missing %q\n---\n%s", want, out)
		}
	}
}

// TestGenerateCustomerSummaryExcludesInternalDetail is the safety test: the
// customer summary must never leak internal fields or non-customer-facing work.
func TestGenerateCustomerSummaryExcludesInternalDetail(t *testing.T) {
	rel := sampleRelease()
	out := GenerateCustomerSummary(rel, sampleChanges())

	forbidden := []string{
		// Internal Plan text (both changes).
		"INTERNAL: refactor exportSvc",
		"SECRET: parameterize query",
		"searchHandler.go",
		// Branch names.
		"feat/csv-export",
		"sec/sql-injection",
		// PR / issue URLs.
		"github.com/org/repo/pull/200",
		"github.com/org/repo/issues/123",
		// Security change must be fully excluded (kind=security), incl. its summary.
		"Hardened the search endpoint against injection.",
		"Patch SQL injection in search",
		// Risk tiers must not surface to customers.
		"critical",
		"medium",
		// Ops/blocked work excluded.
		"Investigate flaky CI",
		// Not-yet-shipped bug (ready_for_test) excluded.
		"Resolved an issue where some users were stuck",
	}
	for _, bad := range forbidden {
		if strings.Contains(out, bad) {
			t.Errorf("customer summary LEAKED internal/excluded content %q\n---\n%s", bad, out)
		}
	}
}

func TestGeneratorsHandleNilAndEmpty(t *testing.T) {
	// Should not panic on nil release / nil change / empty slices.
	_ = GenerateExecutorBrief(nil, nil, "")
	_ = GenerateTesterHandoff(nil, nil, nil, "", "")
	_ = GeneratePMSummary(nil, nil)
	_ = GenerateCustomerSummary(nil, nil)
}
