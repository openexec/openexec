package state

import (
	"context"
	"database/sql"
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
	frozen, err := store.FinalizeEvidenceCoverage(ctx, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if frozen.ID == "" || frozen.CreatedAt.IsZero() || !frozen.CanComplete {
		t.Fatalf("completion report was not frozen: %#v", frozen)
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
	stable, err := store.FinalizeEvidenceCoverage(ctx, accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stable.ID != frozen.ID || !stable.CanComplete || len(stable.Verified) != 1 {
		t.Fatalf("frozen completion report changed after graph drift: before=%#v after=%#v", frozen, stable)
	}
}

func TestValidationPlanImpactMetadataRoundTripsAndAcceptanceCreatesRevision(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(filepath.Join(t.TempDir(), "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	insertVerificationGraph(t, store, "graph-one", "state-one", "current")
	proposed, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{
		TaskID: "task-impact", GenerationID: "graph-one", WorktreeStateHash: "state-one", Status: "proposed",
		ImpactQuery:   ValidationImpactQuery{Files: []string{"service.go"}, SymbolIDs: []string{"symbol-one"}, MaxDepth: 2},
		ImpactSummary: ValidationImpactSummary{ChangedSymbolIDs: []string{"symbol-one"}, AffectedNodeIDs: []string{"consumer-one"}, RelatedTestFiles: []string{"service_test.go"}, Limitations: []string{"dynamic wiring unresolved"}, Truncated: true},
		Items: []ValidationItem{
			{Source: "graph", Disposition: "suggested", Requirement: "optional", Criterion: "Related tests pass", Scope: "related_tests"},
			{Source: "graph", Disposition: "suggested", Requirement: "optional", Criterion: "Lint passes", Scope: "lint"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := store.GetValidationPlanRevision(ctx, proposed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ImpactQuery.Files) != 1 || reloaded.ImpactSummary.AffectedNodeIDs[0] != "consumer-one" || len(reloaded.Items) != 2 {
		t.Fatalf("validation impact metadata did not round-trip: %#v", reloaded)
	}
	accepted, err := store.AcceptValidationPlanRevision(ctx, proposed.ID, "", []ValidationItemDecision{{ID: proposed.Items[0].ID, Disposition: "accepted", Requirement: "required"}})
	if err != nil {
		t.Fatal(err)
	}
	acceptedByCriterion := make(map[string]ValidationItem, len(accepted.Items))
	for _, item := range accepted.Items {
		acceptedByCriterion[item.Criterion] = item
	}
	related := acceptedByCriterion["Related tests pass"]
	lint := acceptedByCriterion["Lint passes"]
	if accepted.Revision != 2 || accepted.Status != "accepted" || accepted.AcceptedAt == nil || accepted.SourceRevisionID != proposed.ID || related.Disposition != "accepted" || related.Requirement != "required" || related.ID == proposed.Items[0].ID || lint.Disposition != "rejected" || lint.Requirement != "optional" {
		t.Fatalf("accepted validation revision = %#v", accepted)
	}
	if accepted.ImpactSummary.AffectedNodeIDs[0] != "consumer-one" || accepted.ImpactQuery.MaxDepth != 2 {
		t.Fatalf("accepted revision lost impact identity: %#v", accepted)
	}
	retried, err := store.AcceptValidationPlanRevision(ctx, proposed.ID, "different-run-is-ignored", nil)
	if err != nil || retried.ID != accepted.ID || retried.Revision != accepted.Revision || retried.RunID != accepted.RunID {
		t.Fatalf("proposal retry created different authority: retried=%#v err=%v", retried, err)
	}
	if _, err := store.AcceptValidationPlanRevision(ctx, accepted.ID, "", nil); err == nil {
		t.Fatal("accepted revision was accepted again")
	}
}

func TestValidationEvidenceAndCompletionRefuseInconsistentIrreversibleState(t *testing.T) {
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
	if err := store.AddRunStep(ctx, "failed-step", "run-one", "trace", "verify", 1, "failed"); err != nil {
		t.Fatal(err)
	}
	if err := store.AddRunStep(ctx, "passed-step", "run-one", "trace", "verify", 2, "completed"); err != nil {
		t.Fatal(err)
	}
	accepted, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{
		TaskID: "task", RunID: "run-one", GenerationID: "graph-one", WorktreeStateHash: "state-one", Status: "accepted",
		Items: []ValidationItem{{Source: "policy", Disposition: "accepted", Requirement: "required", Criterion: "Tests pass"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	link := ValidationEvidenceLink{ValidationItemID: accepted.Items[0].ID, RunID: "run-one", RunStepID: "failed-step", WorktreeStateHash: "state-one", Status: "passed"}
	if err := store.LinkValidationEvidence(ctx, link); !errors.Is(err, ErrEvidenceStepMismatch) {
		t.Fatalf("store accepted evidence inconsistent with its run step: %v", err)
	}
	if _, err := store.FinalizeEvidenceCoverage(ctx, accepted.ID); !errors.Is(err, ErrCompletionEvidenceEmpty) {
		t.Fatalf("completion froze without evidence: %v", err)
	}
	link.RunStepID = "passed-step"
	if err := store.LinkValidationEvidence(ctx, link); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDB().Exec(`UPDATE graph_generations SET status = 'superseded' WHERE id = 'graph-one'`); err != nil {
		t.Fatal(err)
	}
	insertVerificationGraph(t, store, "graph-two", "state-two", "current")
	if _, err := store.FinalizeEvidenceCoverage(ctx, accepted.ID); !errors.Is(err, ErrCompletionStateMoved) {
		t.Fatalf("completion froze after repository state moved: %v", err)
	}
	if _, err := store.ReadCompletionReport(ctx, accepted.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("refused completion left a permanent report: %v", err)
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

func TestAcceptedRevisionDoesNotSupersedeSameTaskIDInAnotherCheckout(t *testing.T) {
	ctx := context.Background()
	store, err := NewStore(filepath.Join(t.TempDir(), "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	insertVerificationGraph(t, store, "graph-one", "state-one", "current")
	if _, err := store.GetDB().Exec(`INSERT INTO repositories (id, persisted_uuid) VALUES ('repo-two', 'persisted-two');
		INSERT INTO checkouts (id, repository_id, root_path) VALUES ('checkout-two', 'repo-two', '/project-two');
		INSERT INTO worktrees (id, repository_id, checkout_id, root_path) VALUES ('worktree-two', 'repo-two', 'checkout-two', '/project-two');
		INSERT INTO graph_generations (id, schema_version, repository_id, checkout_id, worktree_id, worktree_state_hash, configuration_digest, extractor_version, manifest_hash, status)
		VALUES ('graph-two', 1, 'repo-two', 'checkout-two', 'worktree-two', 'state-two', 'config', 'extractor', 'state-two', 'current')`); err != nil {
		t.Fatal(err)
	}
	first, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "shared-external-task", GenerationID: "graph-one", WorktreeStateHash: "state-one", Status: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateValidationPlanRevision(ctx, ValidationPlanRevision{TaskID: "shared-external-task", GenerationID: "graph-two", WorktreeStateHash: "state-two", Status: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{first.ID, second.ID} {
		var status string
		if err := store.GetDB().QueryRow(`SELECT status FROM validation_plan_revisions WHERE id = ?`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "accepted" {
			t.Fatalf("cross-checkout plan %s was superseded: %s", id, status)
		}
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
