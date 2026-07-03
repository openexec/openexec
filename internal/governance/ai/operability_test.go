package ai

import (
	"context"
	"testing"
)

func TestAnalyzeOperability_ClampsConservatively(t *testing.T) {
	// Garbled/unknown values must clamp to the worst-case, so the report can only
	// make the merge gate stricter.
	rep, err := AnalyzeOperability(context.Background(), fixedCompleter{"rollback_safe: maybe\ndb_migration: huh\ndeploy_risk: ???\n"}, "x", "y")
	if err != nil {
		t.Fatalf("AnalyzeOperability: %v", err)
	}
	if rep.RollbackSafe != "no" || rep.DBMigration != "destructive" || rep.DeployRisk != "high" {
		t.Fatalf("expected conservative clamp, got %+v", rep)
	}
	if rep.AutoMergeSafe() {
		t.Fatalf("clamped-worst report must not be auto-merge-safe")
	}
}

func TestOperabilityAutoMergeSafe(t *testing.T) {
	cases := []struct {
		rollback, dbmig, risk string
		want                  bool
	}{
		{"yes", "none", "low", true},
		{"yes", "additive", "medium", true},
		{"yes", "none", "high", false},        // high risk blocks
		{"conditional", "none", "low", false}, // not cleanly rollback-safe
		{"yes", "destructive", "low", false},  // destructive migration blocks
		{"no", "none", "low", false},
	}
	for _, c := range cases {
		r := &OperabilityReport{RollbackSafe: c.rollback, DBMigration: c.dbmig, DeployRisk: c.risk}
		if got := r.AutoMergeSafe(); got != c.want {
			t.Fatalf("AutoMergeSafe(%s,%s,%s)=%v want %v", c.rollback, c.dbmig, c.risk, got, c.want)
		}
	}
	// nil report is never safe.
	var nilRep *OperabilityReport
	if nilRep.AutoMergeSafe() {
		t.Fatalf("nil operability report must not be auto-merge-safe")
	}
}
