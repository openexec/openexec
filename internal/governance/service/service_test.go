package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/validation"
)

// --- test harness ----------------------------------------------------------

// newTestStore opens a real SQLite-backed governance store in a temp workspace,
// mirroring internal/release/manager_test.go's temp-dir + .openexec setup.
func newTestStore(t *testing.T) governance.Store {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatalf("create .openexec dir: %v", err)
	}
	db, store, err := governance.Open(dir)
	if err != nil {
		t.Fatalf("open governance store: %v", err)
	}
	t.Cleanup(func() {
		store.Close()
		db.Close()
	})
	return store
}

// fakeCompleter returns canned model output (here, structured planner/reviewer
// YAML) without any provider call.
type fakeCompleter struct {
	reply string
}

func (f fakeCompleter) Complete(_ context.Context, _ string) (string, error) {
	return f.reply, nil
}

// fakeRunner stands in for the gh CLI; it returns canned stdout per call.
type fakeRunner struct {
	out []byte
}

func (f fakeRunner) Run(_ context.Context, _ ...string) ([]byte, error) {
	return f.out, nil
}

// seedChange inserts a change record directly, bypassing the AI path, so a test
// controls exactly which fields and status a gate sees.
func seedChange(t *testing.T, store governance.Store, ch *governance.ChangeRecord) {
	t.Helper()
	if err := store.CreateChangeRecord(context.Background(), ch); err != nil {
		t.Fatalf("seed change %s: %v", ch.ID, err)
	}
}

func seedRelease(t *testing.T, store governance.Store, rel *governance.GovernanceRelease) {
	t.Helper()
	if err := store.CreateRelease(context.Background(), rel); err != nil {
		t.Fatalf("seed release %s: %v", rel.ID, err)
	}
}

func seedDecision(t *testing.T, store governance.Store, e *governance.DecisionEvent) {
	t.Helper()
	if e.ID == "" {
		e.ID = newID()
	}
	if err := store.CreateDecisionEvent(context.Background(), e); err != nil {
		t.Fatalf("seed decision event: %v", err)
	}
}

// --- ApproveChange gates ---------------------------------------------------

func TestApproveChange_RefusedMissingCriteria(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:              "C-1",
		Status:          governance.ChangeStatusPlanReady,
		Risk:            governance.RiskLow,
		ProposalVersion: 1,
		// no acceptance criteria / verification plan
	})

	err := svc.ApproveChange(ctx, "C-1", "pm")
	if err == nil || !errors.Is(err, validation.ErrMissingAcceptanceCriteria) {
		t.Fatalf("expected ErrMissingAcceptanceCriteria, got %v", err)
	}
}

func TestApproveChange_RefusedAIAuthorityOnMediumRisk(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:                 "C-2",
		Status:             governance.ChangeStatusPlanReady,
		Risk:               governance.RiskMedium,
		ProposalVersion:    1,
		AcceptanceCriteria: "works",
		VerificationPlan:   "go test ./...",
	})

	// bugbot is an AI authority; medium-risk approval is human-only by policy and
	// bugbot also lacks an approve permission.
	err := svc.ApproveChange(ctx, "C-2", "bugbot")
	if err == nil || !strings.Contains(err.Error(), "approval refused") {
		t.Fatalf("expected policy approval refusal, got %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, "C-2")
	if got.Status != governance.ChangeStatusPlanReady {
		t.Fatalf("status must be unchanged after refusal, got %q", got.Status)
	}
	if got.ApprovedVersion != 0 {
		t.Fatalf("ApprovedVersion must remain 0 after refusal, got %d", got.ApprovedVersion)
	}
}

func TestApproveChange_AllowedHumanLowRisk(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:                 "C-3",
		ReleaseID:          "R-1",
		Status:             governance.ChangeStatusPlanReady,
		Risk:               governance.RiskLow,
		ProposalVersion:    2,
		AcceptanceCriteria: "works",
		VerificationPlan:   "go test ./...",
	})

	if err := svc.ApproveChange(ctx, "C-3", "pm"); err != nil {
		t.Fatalf("expected approval to succeed, got %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, "C-3")
	if got.Status != governance.ChangeStatusApprovedForAI {
		t.Fatalf("expected status approved_for_ai, got %q", got.Status)
	}
	if got.ApprovedVersion != got.ProposalVersion {
		t.Fatalf("expected ApprovedVersion==ProposalVersion, got %d != %d", got.ApprovedVersion, got.ProposalVersion)
	}

	events, _ := store.ListDecisionEvents(ctx, "C-3")
	if len(events) != 1 || events[0].Decision != governance.DecisionApproved {
		t.Fatalf("expected one approved decision event, got %+v", events)
	}
}

// --- ClaimWork gates -------------------------------------------------------

func TestClaimWork_RefusedReleaseUnapproved(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedRelease(t, store, &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusPlanned})
	seedChange(t, store, &governance.ChangeRecord{
		ID:              "C-1",
		ReleaseID:       "R-1",
		Status:          governance.ChangeStatusApprovedForAI,
		ProposalVersion: 1,
		ApprovedVersion: 1,
	})

	err := svc.ClaimWork(ctx, "C-1", "codex", time.Hour)
	if err == nil || !errors.Is(err, validation.ErrReleaseNotImplementable) {
		t.Fatalf("expected ErrReleaseNotImplementable, got %v", err)
	}
}

func TestClaimWork_RefusedAlreadyClaimed(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedRelease(t, store, &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusApproved})
	seedChange(t, store, &governance.ChangeRecord{
		ID:              "C-1",
		ReleaseID:       "R-1",
		Status:          governance.ChangeStatusApprovedForAI,
		ProposalVersion: 1,
		ApprovedVersion: 1,
	})

	if err := svc.ClaimWork(ctx, "C-1", "codex", time.Hour); err != nil {
		t.Fatalf("first claim should succeed, got %v", err)
	}
	err := svc.ClaimWork(ctx, "C-1", "claude", time.Hour)
	if err == nil || !errors.Is(err, validation.ErrAlreadyClaimed) {
		t.Fatalf("expected ErrAlreadyClaimed for second agent, got %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusImplementing {
		t.Fatalf("expected implementing after first claim, got %q", got.Status)
	}
	if got.ClaimedBy != "codex" {
		t.Fatalf("expected claim retained by codex, got %q", got.ClaimedBy)
	}
}

func TestClaimWork_RefusedUnapprovedChange(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	// Release is approved, but the change itself was never approved_for_ai. A
	// direct claim by ID must NOT bypass approved_for_ai (the bypass guard).
	seedRelease(t, store, &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusApproved})
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", ReleaseID: "R-1", Status: governance.ChangeStatusCandidate})

	err := svc.ClaimWork(ctx, "C-1", "codex", time.Hour)
	if err == nil || !errors.Is(err, validation.ErrChangeNotApproved) {
		t.Fatalf("expected ErrChangeNotApproved claiming an unapproved change, got %v", err)
	}
	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.ClaimedBy != "" || got.Status != governance.ChangeStatusCandidate {
		t.Fatalf("change must be untouched after refused claim, got status=%q claimedBy=%q", got.Status, got.ClaimedBy)
	}
}

// --- ApproveRelease authority gate -----------------------------------------

func TestApproveRelease_RequiresAuthorizedApprover(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedRelease(t, store, &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusPlanned, Risk: governance.RiskMedium})
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusPlanReady})
	if err := svc.AttachChange(ctx, "R-1", "C-1", 1, true); err != nil {
		t.Fatalf("attach change: %v", err)
	}

	// AI/verifier authorities must not be able to approve release scope, even
	// when they hold broad change-level permissions.
	for _, id := range []string{"bugbot", "ci_verifier", "tester_ai", "security_ai", "developer"} {
		if err := svc.ApproveRelease(ctx, "R-1", id); err == nil {
			t.Fatalf("expected release approval refusal for authority %q", id)
		}
	}
	rel, _ := store.GetRelease(ctx, "R-1")
	if rel.Status != governance.ReleaseStatusPlanned || rel.ApprovedForAI {
		t.Fatalf("release must stay unapproved after refused approvals, got status=%q approvedForAI=%v", rel.Status, rel.ApprovedForAI)
	}

	// pm (human, approve permission, critical risk limit) may approve.
	if err := svc.ApproveRelease(ctx, "R-1", "pm"); err != nil {
		t.Fatalf("expected pm to approve release, got %v", err)
	}
	rel, _ = store.GetRelease(ctx, "R-1")
	if rel.Status != governance.ReleaseStatusApproved || !rel.ApprovedForAI {
		t.Fatalf("expected release approved by pm, got status=%q approvedForAI=%v", rel.Status, rel.ApprovedForAI)
	}
}

// --- ReadyForTest gates ----------------------------------------------------

func TestReadyForTest_RefusedThenAllowedAfterEvidence(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	// pr_open status but no PR URL recorded and no evidence yet.
	seedChange(t, store, &governance.ChangeRecord{
		ID:     "C-1",
		Status: governance.ChangeStatusPROpen,
	})

	err := svc.ReadyForTest(ctx, "C-1")
	if err == nil || !errors.Is(err, validation.ErrNoPRNoEvidence) {
		t.Fatalf("expected ErrNoPRNoEvidence, got %v", err)
	}

	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindManual, governance.EvidenceSourceHuman, "manual smoke test", "", ""); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if err := svc.ReadyForTest(ctx, "C-1"); err != nil {
		t.Fatalf("expected ready-for-test to succeed after evidence, got %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusReadyForTest {
		t.Fatalf("expected ready_for_test, got %q", got.Status)
	}
}

func TestReadyForTest_ManualEvidenceFromImplementing(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	// A change verified WITHOUT a PR (manual evidence) is in implementing status.
	// The implementing -> ready_for_test edge must let it advance, otherwise the
	// manual-evidence allowance is dead.
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusImplementing})
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindManual, governance.EvidenceSourceHuman, "manual smoke test", "", ""); err != nil {
		t.Fatalf("record evidence: %v", err)
	}
	if err := svc.ReadyForTest(ctx, "C-1"); err != nil {
		t.Fatalf("expected ready-for-test from implementing with manual evidence, got %v", err)
	}
	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusReadyForTest {
		t.Fatalf("expected ready_for_test, got %q", got.Status)
	}
}

// --- MarkDone gates --------------------------------------------------------

func TestMarkDone_RefusedWithoutEvidence(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:     "C-1",
		Status: governance.ChangeStatusReadyForTest,
		Risk:   governance.RiskLow,
	})

	// tester_ai holds mark_done with a medium risk limit, so the authority gate
	// passes and the missing-evidence gate is what fires.
	err := svc.MarkDone(ctx, "C-1", "tester_ai")
	if err == nil || !errors.Is(err, validation.ErrNoVerificationEvidence) {
		t.Fatalf("expected ErrNoVerificationEvidence, got %v", err)
	}
}

func TestMarkDone_RefusedWithoutPermission(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:     "C-1",
		Status: governance.ChangeStatusReadyForTest,
		Risk:   governance.RiskLow,
	})
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindTest, governance.EvidenceSourceCLI, "go test ./...", "", ""); err != nil {
		t.Fatalf("record evidence: %v", err)
	}

	// bugbot lacks the mark_done permission; mark-done must be refused even with
	// evidence present.
	if err := svc.MarkDone(ctx, "C-1", "bugbot"); err == nil {
		t.Fatalf("expected mark-done refusal for bugbot (no mark_done permission)")
	}
	// An unknown authority is also refused.
	if err := svc.MarkDone(ctx, "C-1", "nobody"); err == nil {
		t.Fatalf("expected mark-done refusal for unknown authority")
	}
	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusReadyForTest {
		t.Fatalf("status must be unchanged after refusal, got %q", got.Status)
	}
}

func TestMarkDone_RefusedSeparationOfDuties(t *testing.T) {
	store := newTestStore(t)
	// Operator session so the human-authority guard passes and SoD is what fires.
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:     "C-1",
		Status: governance.ChangeStatusReadyForTest,
		Risk:   governance.RiskHigh,
	})
	// pm proposed AND approved this high-risk change. pm is used because the
	// done-marker must hold mark_done with a risk limit covering high risk
	// (only pm qualifies among seeded authorities), so the SoD check — not the
	// authority gate — is what fires when pm also marks done.
	seedDecision(t, store, &governance.DecisionEvent{ChangeID: "C-1", Actor: "pm", Decision: governance.DecisionProposed})
	seedDecision(t, store, &governance.DecisionEvent{ChangeID: "C-1", Actor: "pm", Decision: governance.DecisionApproved})
	// verification evidence is present, so the SoD check is the gate that fires.
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindTest, governance.EvidenceSourceCLI, "go test ./...", "", ""); err != nil {
		t.Fatalf("record evidence: %v", err)
	}

	err := svc.MarkDone(ctx, "C-1", "pm")
	if err == nil || !errors.Is(err, validation.ErrSeparationOfDuties) {
		t.Fatalf("expected ErrSeparationOfDuties, got %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusReadyForTest {
		t.Fatalf("status must be unchanged after SoD refusal, got %q", got.Status)
	}
}

func TestMarkDone_HumanAuthorityRequiresOperatorSession(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{}) // NOT an operator session
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusReadyForTest, Risk: governance.RiskLow,
	})
	_ = svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindTest, governance.EvidenceSourceCLI, "t", "", "")

	// An agent session attributing "done" to a HUMAN authority (pm) must be
	// refused — otherwise it could forge a human-attributed audit record.
	if err := svc.MarkDone(ctx, "C-1", "pm"); err == nil || !strings.Contains(err.Error(), "operator session") {
		t.Fatalf("expected human-authority forgery guard, got %v", err)
	}
	// An AI/verifier authority may still mark done in an agent session.
	if err := svc.MarkDone(ctx, "C-1", "ci_verifier"); err != nil {
		t.Fatalf("verifier should be allowed to mark done, got %v", err)
	}
}

func TestMarkDone_Allowed(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedChange(t, store, &governance.ChangeRecord{
		ID:     "C-1",
		Status: governance.ChangeStatusReadyForTest,
		Risk:   governance.RiskMedium,
	})
	// distinct actors: alice proposed, bob approved, tester_ai (a non-human
	// verifier with mark_done and a medium risk limit) marks done.
	seedDecision(t, store, &governance.DecisionEvent{ChangeID: "C-1", Actor: "alice", Decision: governance.DecisionProposed})
	seedDecision(t, store, &governance.DecisionEvent{ChangeID: "C-1", Actor: "bob", Decision: governance.DecisionApproved})
	if err := svc.RecordEvidence(ctx, "C-1", governance.EvidenceKindTest, governance.EvidenceSourceCLI, "go test ./...", "", ""); err != nil {
		t.Fatalf("record evidence: %v", err)
	}

	if err := svc.MarkDone(ctx, "C-1", "tester_ai"); err != nil {
		t.Fatalf("expected mark done to succeed, got %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusDone {
		t.Fatalf("expected done, got %q", got.Status)
	}
	events, _ := store.ListDecisionEvents(ctx, "C-1")
	foundDone := false
	for _, e := range events {
		if e.Decision == governance.DecisionMarkedDone && e.Actor == "tester_ai" {
			// The recorded actor type must be the authority's real type, not a
			// hardcoded "human".
			if e.ActorType != governance.ActorTypeAI {
				t.Fatalf("expected marked_done ActorType ai for tester_ai, got %q", e.ActorType)
			}
			foundDone = true
		}
	}
	if !foundDone {
		t.Fatalf("expected a marked_done event by tester_ai, got %+v", events)
	}
}

// --- AttachChange re-approval gate -----------------------------------------

func TestAttachChange_FlipsApprovedReleaseOutOfApproved(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedRelease(t, store, &governance.GovernanceRelease{
		ID:            "R-1",
		Status:        governance.ReleaseStatusApproved,
		ApprovedForAI: true,
	})
	seedChange(t, store, &governance.ChangeRecord{ID: "C-NEW", Status: governance.ChangeStatusPlanReady})

	if err := svc.AttachChange(ctx, "R-1", "C-NEW", 1, true); err != nil {
		t.Fatalf("attach change: %v", err)
	}

	rel, _ := store.GetRelease(ctx, "R-1")
	if rel.Status != governance.ReleaseStatusPlanned {
		t.Fatalf("expected release reset to planned, got %q", rel.Status)
	}
	if rel.ApprovedForAI {
		t.Fatalf("expected ApprovedForAI cleared after scope add")
	}

	// The invalidation must be recorded for audit.
	events, _ := store.ListDecisionEvents(ctx, "C-NEW")
	found := false
	for _, e := range events {
		if e.Decision == governance.DecisionChangesRequested && e.ActorType == governance.ActorTypeSystem {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a system invalidation decision event, got %+v", events)
	}
}

// --- GenerateHandoff -------------------------------------------------------

func TestGenerateHandoff_PersistsArtifact(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{})
	ctx := context.Background()

	seedRelease(t, store, &governance.GovernanceRelease{
		ID:     "R-1",
		Name:   "July fixes",
		Status: governance.ReleaseStatusReadyForTest,
	})
	seedChange(t, store, &governance.ChangeRecord{
		ID:                 "C-1",
		ReleaseID:          "R-1",
		Title:              "Fix login",
		Status:             governance.ChangeStatusReadyForTest,
		AcceptanceCriteria: "login works",
		VerificationPlan:   "manual login",
	})

	body, err := svc.GenerateHandoff(ctx, "R-1", governance.AudienceTester, "staging", "2026.07.1")
	if err != nil {
		t.Fatalf("generate handoff: %v", err)
	}
	if strings.TrimSpace(body) == "" {
		t.Fatal("expected non-empty handoff body")
	}

	arts, _ := store.ListCommunicationArtifacts(ctx, "R-1")
	if len(arts) != 1 {
		t.Fatalf("expected one persisted artifact, got %d", len(arts))
	}
	if arts[0].Audience != governance.AudienceTester || arts[0].Body != body {
		t.Fatalf("persisted artifact mismatch: %+v", arts[0])
	}
}

// --- dependency gating -----------------------------------------------------

func TestMissingDependencies(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{ID: "C-1", Status: governance.ChangeStatusCandidate})

	svc := NewService(store, Options{}) // no completer, no runner

	if _, err := svc.Triage(ctx, "C-1", "", "bugbot"); !errors.Is(err, errMissingCompleter) {
		t.Fatalf("expected errMissingCompleter, got %v", err)
	}
	if _, err := svc.ImportGitHubIssue(ctx, "proj", "org/repo", 1); !errors.Is(err, errMissingRunner) {
		t.Fatalf("expected errMissingRunner, got %v", err)
	}
}

// --- AI path wired through fakes (smoke) -----------------------------------

func TestTriage_WithFakeCompleter(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedChange(t, store, &governance.ChangeRecord{
		ID:              "C-1",
		Status:          governance.ChangeStatusCandidate,
		ProposalVersion: 0,
	})

	planner := fakeCompleter{reply: `summary: Fix the thing
kind: bug
risk: low
acceptance_criteria:
  - it works
verification_plan:
  - go test ./...
implementation_notes: do it
`}
	svc := NewService(store, Options{Completer: planner})

	out, err := svc.Triage(ctx, "C-1", "repo ctx", "bugbot")
	if err != nil {
		t.Fatalf("triage: %v", err)
	}
	if out.Summary == "" {
		t.Fatal("expected parsed planner summary")
	}
	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.Status != governance.ChangeStatusPlanReady {
		t.Fatalf("expected plan_ready after triage, got %q", got.Status)
	}
	if got.ProposalVersion != 1 {
		t.Fatalf("expected ProposalVersion bumped to 1, got %d", got.ProposalVersion)
	}
}

// --- audit round 2: operator gate, required reviews, triage gate, lifecycle --

func TestApprove_RefusedOutsideOperatorSession(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{}) // no operator session
	ctx := context.Background()

	seedRelease(t, store, &governance.GovernanceRelease{ID: "R-1", Status: governance.ReleaseStatusPlanned})
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", ReleaseID: "R-1", Status: governance.ChangeStatusPlanReady,
		Risk: governance.RiskLow, ProposalVersion: 1,
		AcceptanceCriteria: "works", VerificationPlan: "go test",
	})

	if err := svc.ApproveChange(ctx, "C-1", "pm"); !errors.Is(err, errNotOperator) {
		t.Fatalf("ApproveChange: expected errNotOperator, got %v", err)
	}
	if err := svc.ApproveRelease(ctx, "R-1", "pm"); !errors.Is(err, errNotOperator) {
		t.Fatalf("ApproveRelease: expected errNotOperator, got %v", err)
	}
}

func TestApproveChange_RequiresRecordedReview(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	// Medium risk: DefaultPolicy requires an AI review before approval.
	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusPlanReady, Risk: governance.RiskMedium,
		ProposalVersion: 1, AcceptanceCriteria: "works", VerificationPlan: "go test",
	})

	if err := svc.ApproveChange(ctx, "C-1", "pm"); !errors.Is(err, validation.ErrReviewMissing) {
		t.Fatalf("expected ErrReviewMissing without a recorded review, got %v", err)
	}

	// Record an AI review; approval then succeeds.
	seedDecision(t, store, &governance.DecisionEvent{
		ChangeID: "C-1", Actor: "bugbot", ActorType: governance.ActorTypeAI,
		Decision: governance.DecisionReviewed,
	})
	if err := svc.ApproveChange(ctx, "C-1", "pm"); err != nil {
		t.Fatalf("expected approval to succeed after review, got %v", err)
	}
}

func TestTriage_RefusedOnInFlightOrTerminalChange(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{Completer: fakeCompleter{reply: "summary: x\nrisk: low\n"}})
	ctx := context.Background()

	for _, st := range []string{
		governance.ChangeStatusImplementing,
		governance.ChangeStatusDone,
		governance.ChangeStatusPROpen,
	} {
		id := "C-" + st
		seedChange(t, store, &governance.ChangeRecord{ID: id, Status: st, SourceType: governance.SourceManual, SourceID: id})
		if _, err := svc.Triage(ctx, id, "", "planner"); !errors.Is(err, validation.ErrChangeNotTriageable) {
			t.Fatalf("triage on %q: expected ErrChangeNotTriageable, got %v", st, err)
		}
	}
}

// TestReleaseLifecycle_CreateAttachApprove exercises the full slice from a
// freshly-created draft release (create -> attach -> approve-change ->
// approve-release -> queue) WITHOUT seeding status directly — the regression
// that would have caught the draft->planned dead-end.
func TestReleaseLifecycle_CreateAttachApprove(t *testing.T) {
	store := newTestStore(t)
	svc := NewService(store, Options{OperatorSession: true})
	ctx := context.Background()

	rel, err := svc.CreateRelease(ctx, "R-1", "July", "pm")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if rel.Status != governance.ReleaseStatusDraft {
		t.Fatalf("expected draft on create, got %q", rel.Status)
	}

	seedChange(t, store, &governance.ChangeRecord{
		ID: "C-1", Status: governance.ChangeStatusPlanReady, Risk: governance.RiskLow,
		ProposalVersion: 1, AcceptanceCriteria: "works", VerificationPlan: "go test",
	})
	if err := svc.AttachChange(ctx, "R-1", "C-1", 1, true); err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, _ := store.GetRelease(ctx, "R-1")
	if got.Status != governance.ReleaseStatusPlanned {
		t.Fatalf("expected release planned after first attach, got %q", got.Status)
	}

	if err := svc.ApproveChange(ctx, "C-1", "pm"); err != nil {
		t.Fatalf("approve change: %v", err)
	}
	if err := svc.ApproveRelease(ctx, "R-1", "pm"); err != nil {
		t.Fatalf("approve release: %v", err)
	}
	got, _ = store.GetRelease(ctx, "R-1")
	if got.Status != governance.ReleaseStatusApproved || !got.ApprovedForAI {
		t.Fatalf("expected approved release, got status=%q approvedForAI=%v", got.Status, got.ApprovedForAI)
	}

	work, err := svc.ListApprovedWork(ctx, "")
	if err != nil {
		t.Fatalf("list approved work: %v", err)
	}
	if len(work) != 1 || work[0].ID != "C-1" {
		t.Fatalf("expected C-1 in approved work queue, got %+v", work)
	}
}
