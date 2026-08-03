package state

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationPlansBindEvidenceAndClaimsToCurrentRepositoryState(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(filepath.Join(t.TempDir(), "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	insertVerificationGraph(t, store, "graph-one", "state-one", "current")
	if err := store.CreateRun(ctx, "run-one", "", "", "/project", "workspace-write"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddRunStep(ctx, "step-one", "run-one", "trace", "verify", 1, "completed"); err != nil {
		t.Fatal(err)
	}
	if err := store.RecordArtifact(ctx, "artifact-one", "test_log", ".openexec/artifacts/test.log", 10); err != nil {
		t.Fatal(err)
	}

	proposed, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "task-one", GenerationID: "graph-one", WorktreeStateHash: "state-one", PatchHash: "patch-one", Status: "proposed", Items: []ValidationItem{{Source: "graph", Disposition: "suggested", Requirement: "required", Criterion: "Related tests pass", Scope: "related_tests"}}})
	if err != nil {
		t.Fatal(err)
	}
	if proposed.Revision != 1 {
		t.Fatalf("unexpected proposed revision: %#v", proposed)
	}
	if _, err := store.EvidenceCoverage(ctx, proposed.ID); err == nil {
		t.Fatal("proposed validation plan was treated as completion authority")
	}
	if _, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "task-one", GenerationID: "graph-one", WorktreeStateHash: "state-one", PatchHash: "patch-one", Status: "accepted", Items: []ValidationItem{{Source: "graph", Disposition: "suggested", Requirement: "blocking", Criterion: "Tests pass"}}}); err == nil {
		t.Fatal("accepted plan retained a suggested item")
	}

	accepted, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "task-one", RunID: "run-one", GenerationID: "graph-one", WorktreeStateHash: "state-one", PatchHash: "patch-one", Status: "accepted", Items: []ValidationItem{{Source: "graph", Disposition: "accepted", Requirement: "blocking", Criterion: "Related tests pass", Scope: "related_tests"}, {Source: "user", Disposition: "rejected", Requirement: "optional", Criterion: "Full suite passes", Scope: "full_suite"}}})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.Revision != 2 || accepted.AcceptedAt == nil {
		t.Fatalf("accepted revision was not recorded: %#v", accepted)
	}
	if err := store.UpdateValidationItem(ctx, accepted.Items[0]); !errors.Is(err, ErrImmutableValidationPlan) {
		t.Fatalf("accepted item was mutable: %v", err)
	}

	report, err := store.EvidenceCoverage(ctx, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.CanComplete || len(report.Verified) != 0 || len(report.NotVerified) != 1 || report.NotVerified[0].Status != "not_run" {
		t.Fatalf("unrun blocking obligation was not enforced: %#v", report)
	}
	badLink := ValidationEvidenceLink{ValidationItemID: accepted.Items[0].ID, RunID: "run-one", RunStepID: "step-one", ArtifactHash: "artifact-one", WorktreeStateHash: "different", PatchHash: "patch-one", Status: "passed"}
	if err := store.LinkValidationEvidence(ctx, badLink); !errors.Is(err, ErrEvidenceStateMismatch) {
		t.Fatalf("mismatched evidence was linked: %v", err)
	}
	goodLink := badLink
	goodLink.WorktreeStateHash = "state-one"
	if err := store.LinkValidationEvidence(ctx, goodLink); err != nil {
		t.Fatal(err)
	}
	report, err = store.EvidenceCoverage(ctx, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !report.CanComplete || len(report.Verified) != 1 || len(report.NotVerified) != 0 || len(report.Verified[0].EvidenceArtifactIDs) != 1 {
		t.Fatalf("eligible evidence did not support the claim: %#v", report)
	}

	// A newer worktree generation makes the old evidence ineligible.
	if _, err := store.GetDB().Exec(`UPDATE graph_generations SET status = 'superseded' WHERE id = 'graph-one'`); err != nil {
		t.Fatal(err)
	}
	insertVerificationGraph(t, store, "graph-two", "state-two", "current")
	report, err = store.EvidenceCoverage(ctx, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if report.CanComplete || len(report.NotVerified) != 1 || report.NotVerified[0].Status != "unavailable" {
		t.Fatalf("old evidence remained eligible after worktree changed: %#v", report)
	}
}

func TestAcceptedRevisionSupersedesPriorPlanAndNormalizesClaims(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(filepath.Join(t.TempDir(), "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	insertVerificationGraph(t, store, "graph-one", "state-one", "current")
	first, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "task", GenerationID: "graph-one", WorktreeStateHash: "state-one", Status: "accepted", Items: []ValidationItem{{Source: "policy", Disposition: "accepted", Requirement: "required", Criterion: "Lint passes", Scope: "lint"}}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "task", GenerationID: "graph-one", WorktreeStateHash: "state-one", Status: "accepted", Items: []ValidationItem{{Source: "policy", Disposition: "accepted", Requirement: "required", Criterion: "Tests pass", Scope: "tests"}}})
	if err != nil {
		t.Fatal(err)
	}
	var firstStatus string
	if err := store.GetDB().QueryRow(`SELECT status FROM validation_plan_revisions WHERE id = ?`, first.ID).Scan(&firstStatus); err != nil {
		t.Fatal(err)
	}
	if firstStatus != "superseded" || second.Revision != 2 {
		t.Fatalf("accepted plan lineage is wrong: first=%s second=%#v", firstStatus, second)
	}
	report, err := store.EvidenceCoverage(ctx, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	normalized, replaced := NormalizeCompletionSummary("Everything works and no regressions", report)
	if !replaced || strings.Contains(strings.ToLower(normalized), "everything works") || !strings.Contains(normalized, "Not verified:") {
		t.Fatalf("unsupported broad claim was not normalized: %q replaced=%t", normalized, replaced)
	}
}

func insertVerificationGraph(t *testing.T, store *Store, generationID, stateHash, status string) {
	t.Helper()
	db := store.GetDB()
	if _, err := db.Exec(`INSERT OR IGNORE INTO repositories (id, persisted_uuid) VALUES ('repo', 'persisted'); INSERT OR IGNORE INTO checkouts (id, repository_id, root_path) VALUES ('checkout', 'repo', '/project'); INSERT OR IGNORE INTO worktrees (id, repository_id, checkout_id, root_path) VALUES ('worktree', 'repo', 'checkout', '/project')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO graph_generations (id, schema_version, repository_id, checkout_id, worktree_id, worktree_state_hash, configuration_digest, extractor_version, manifest_hash, status) VALUES (?, 1, 'repo', 'checkout', 'worktree', ?, 'config', 'extractor', ?, ?)`, generationID, stateHash, stateHash, status); err != nil {
		t.Fatal(err)
	}
}
