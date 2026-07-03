package ai

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/internal/governance"
)

// fakeCompleter returns canned responses in sequence, ignoring the prompt.
type fakeCompleter struct {
	responses []string
	calls     int
	err       error
}

func (f *fakeCompleter) Complete(ctx context.Context, prompt string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	if f.calls >= len(f.responses) {
		return f.responses[len(f.responses)-1], nil
	}
	r := f.responses[f.calls]
	f.calls++
	return r, nil
}

func newTestStore(t *testing.T) *governance.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatalf("mkdir .openexec: %v", err)
	}
	_, store, err := governance.Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func seedChange(t *testing.T, store governance.Store) *governance.ChangeRecord {
	t.Helper()
	ctx := context.Background()
	ch := &governance.ChangeRecord{
		ID:        "CHANGE-1",
		ReleaseID: "R-1",
		ProjectID: "proj",
		Title:     "Fix login crash",
		RawText:   "Users see a crash when logging in with an expired token.",
		Status:    governance.ChangeStatusCandidate,
	}
	if err := store.CreateChangeRecord(ctx, ch); err != nil {
		t.Fatalf("CreateChangeRecord: %v", err)
	}
	return ch
}

const plannerYAML = "```yaml\n" +
	"summary: Handle expired tokens on login\n" +
	"kind: bug\n" +
	"risk: medium\n" +
	"affected_projects:\n  - auth\n" +
	"acceptance_criteria:\n  - Expired token shows re-login prompt\n  - No crash on expiry\n" +
	"verification_plan:\n  - go test ./auth/...\n" +
	"implementation_notes: Guard the token refresh path.\n" +
	"open_questions: []\n" +
	"```\n"

func TestTriageAppliesFieldsAndBumpsVersion(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)

	fc := &fakeCompleter{responses: []string{plannerYAML}}
	out, err := Triage(ctx, fc, store, ch, "repo ctx", "bugbot")
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}

	if out.Summary != "Handle expired tokens on login" || out.Kind != "bug" || out.Risk != "medium" {
		t.Fatalf("parsed planner output wrong: %+v", out)
	}

	got, err := store.GetChangeRecord(ctx, ch.ID)
	if err != nil {
		t.Fatalf("GetChangeRecord: %v", err)
	}
	if got.Summary != "Handle expired tokens on login" {
		t.Errorf("Summary not applied: %q", got.Summary)
	}
	if got.Kind != governance.KindBug || got.Risk != governance.RiskMedium {
		t.Errorf("Kind/Risk not applied: %q/%q", got.Kind, got.Risk)
	}
	if got.AcceptanceCriteria != "Expired token shows re-login prompt\nNo crash on expiry" {
		t.Errorf("AcceptanceCriteria join wrong: %q", got.AcceptanceCriteria)
	}
	if got.VerificationPlan != "go test ./auth/..." {
		t.Errorf("VerificationPlan join wrong: %q", got.VerificationPlan)
	}
	if got.Plan != "Guard the token refresh path." {
		t.Errorf("Plan not applied: %q", got.Plan)
	}
	if got.Status != governance.ChangeStatusPlanReady {
		t.Errorf("Status = %q, want plan_ready", got.Status)
	}
	if got.ProposalVersion != 1 {
		t.Errorf("ProposalVersion = %d, want 1", got.ProposalVersion)
	}

	events, err := store.ListDecisionEvents(ctx, ch.ID)
	if err != nil {
		t.Fatalf("ListDecisionEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("want 1 decision event, got %d", len(events))
	}
	ev := events[0]
	if ev.Decision != governance.DecisionProposed || ev.ActorType != governance.ActorTypeAI {
		t.Errorf("event decision/actor = %q/%q, want proposed/ai", ev.Decision, ev.ActorType)
	}
	if ev.Actor != "bugbot" || ev.ProposalVersion != 1 {
		t.Errorf("event actor/version = %q/%d, want bugbot/1", ev.Actor, ev.ProposalVersion)
	}
}

func TestReTriageBumpsVersionAndClearsApproval(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)

	fc := &fakeCompleter{responses: []string{plannerYAML}}
	if _, err := Triage(ctx, fc, store, ch, "", "bugbot"); err != nil {
		t.Fatalf("first Triage: %v", err)
	}
	if ch.ProposalVersion != 1 {
		t.Fatalf("after first triage version = %d, want 1", ch.ProposalVersion)
	}

	// Simulate a human approval of version 1, then a re-triage.
	ch.ApprovedVersion = 1
	if err := store.UpdateChangeRecord(ctx, ch); err != nil {
		t.Fatalf("set approved: %v", err)
	}

	if _, err := Triage(ctx, fc, store, ch, "", "bugbot"); err != nil {
		t.Fatalf("second Triage: %v", err)
	}

	got, err := store.GetChangeRecord(ctx, ch.ID)
	if err != nil {
		t.Fatalf("GetChangeRecord: %v", err)
	}
	if got.ProposalVersion != 2 {
		t.Errorf("ProposalVersion = %d, want 2", got.ProposalVersion)
	}
	if got.ApprovedVersion != 0 {
		t.Errorf("ApprovedVersion = %d, want 0 (stale approval cleared)", got.ApprovedVersion)
	}

	events, _ := store.ListDecisionEvents(ctx, ch.ID)
	if len(events) != 2 {
		t.Fatalf("want 2 proposed events after re-triage, got %d", len(events))
	}
	// Order-independent: both proposal versions must be recorded in history
	// (plan revisions preserve old versions). Timestamps share a second so
	// list order is not reliable.
	versions := map[int]bool{}
	for _, e := range events {
		if e.Decision != governance.DecisionProposed {
			t.Errorf("event decision = %q, want proposed", e.Decision)
		}
		versions[e.ProposalVersion] = true
	}
	if !versions[1] || !versions[2] {
		t.Errorf("decision history versions = %v, want both 1 and 2", versions)
	}
}

const reviewApproveYAML = "Here is my review:\n\n" +
	"decision: approve\n" +
	"concerns: []\n" +
	"missing_acceptance_criteria: []\n" +
	"missing_tests:\n  - add expiry regression test\n" +
	"risk_comments: []\n" +
	"recommended_policy: require_low_risk_approval\n"

// recommendingAuthority has the recommend_approval permission (mirrors the
// seeded bugbot AI authority).
func recommendingAuthority() *governance.ReviewAuthority {
	return &governance.ReviewAuthority{
		ID:          "bugbot",
		Name:        "Bug Triage AI",
		Type:        governance.AuthorityAI,
		Permissions: []string{governance.PermComment, governance.PermRequestChanges, governance.PermRecommendApproval},
		RiskLimit:   governance.RiskHigh,
	}
}

func TestReviewPlanRecommendsButNeverApproves(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)
	// Give it a plan + a prior approval to prove ReviewPlan never touches it.
	ch.ProposalVersion = 1
	ch.ApprovedVersion = 1
	ch.Plan = "Guard the token refresh path."
	ch.AcceptanceCriteria = "No crash on expiry"
	if err := store.UpdateChangeRecord(ctx, ch); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	fc := &fakeCompleter{responses: []string{reviewApproveYAML}}
	out, err := ReviewPlan(ctx, fc, store, ch, recommendingAuthority(), "bugbot")
	if err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}
	if out.Decision != ReviewerApprove {
		t.Fatalf("parsed reviewer decision = %q, want approve", out.Decision)
	}

	events, _ := store.ListDecisionEvents(ctx, ch.ID)
	if len(events) != 1 {
		t.Fatalf("want 1 review event, got %d", len(events))
	}
	ev := events[0]
	// THE SECURITY CRUX: approve must be recorded as recommended_approval,
	// never approved.
	if ev.Decision != governance.DecisionRecommendedApproval {
		t.Fatalf("review decision = %q, want recommended_approval (reviewer never approves)", ev.Decision)
	}
	if ev.Decision == governance.DecisionApproved {
		t.Fatal("reviewer AI recorded DecisionApproved — invariant violated")
	}
	if ev.ActorType != governance.ActorTypeAI {
		t.Errorf("actor type = %q, want ai", ev.ActorType)
	}

	// ApprovedVersion must be untouched by the reviewer.
	got, _ := store.GetChangeRecord(ctx, ch.ID)
	if got.ApprovedVersion != 1 {
		t.Errorf("ApprovedVersion = %d, want 1 (reviewer must not mutate approval)", got.ApprovedVersion)
	}
}

func TestReviewPlanRequestChangesSetsStatus(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)
	ch.Status = governance.ChangeStatusPlanReady
	if err := store.UpdateChangeRecord(ctx, ch); err != nil {
		t.Fatalf("seed: %v", err)
	}

	const reqYAML = "decision: request_changes\nconcerns:\n  - acceptance criteria too vague\nmissing_tests: []\n"
	fc := &fakeCompleter{responses: []string{reqYAML}}
	if _, err := ReviewPlan(ctx, fc, store, ch, recommendingAuthority(), "bugbot"); err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, ch.ID)
	if got.Status != governance.ChangeStatusChangesRequested {
		t.Errorf("Status = %q, want changes_requested", got.Status)
	}
	events, _ := store.ListDecisionEvents(ctx, ch.ID)
	if events[0].Decision != governance.DecisionChangesRequested {
		t.Errorf("decision = %q, want changes_requested", events[0].Decision)
	}
}

func TestReviewPlanDowngradesChangesRequestedWithoutPermission(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)
	ch.Status = governance.ChangeStatusPlanReady
	if err := store.UpdateChangeRecord(ctx, ch); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Authority may comment but lacks request_changes: a request_changes verdict
	// must be downgraded to a neutral review note and must NOT move the status.
	weak := &governance.ReviewAuthority{
		ID: "weak", Name: "Weak", Type: governance.AuthorityAI,
		Permissions: []string{governance.PermComment},
		RiskLimit:   governance.RiskLow,
	}
	const reqYAML = "decision: request_changes\nconcerns:\n  - vague\n"
	fc := &fakeCompleter{responses: []string{reqYAML}}
	if _, err := ReviewPlan(ctx, fc, store, ch, weak, "weak"); err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}

	got, _ := store.GetChangeRecord(ctx, ch.ID)
	if got.Status != governance.ChangeStatusPlanReady {
		t.Errorf("Status = %q, want plan_ready unchanged (under-privileged reviewer)", got.Status)
	}
	events, _ := store.ListDecisionEvents(ctx, ch.ID)
	if events[0].Decision != governance.DecisionReviewed {
		t.Errorf("decision = %q, want reviewed (changes-requested downgraded)", events[0].Decision)
	}
}

func TestReviewPlanDowngradesRecommendationWithoutPermission(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)

	// Authority lacks recommend_approval.
	weak := &governance.ReviewAuthority{
		ID: "weak", Name: "Weak", Type: governance.AuthorityAI,
		Permissions: []string{governance.PermComment},
		RiskLimit:   governance.RiskLow,
	}
	fc := &fakeCompleter{responses: []string{reviewApproveYAML}}
	if _, err := ReviewPlan(ctx, fc, store, ch, weak, "weak"); err != nil {
		t.Fatalf("ReviewPlan: %v", err)
	}
	events, _ := store.ListDecisionEvents(ctx, ch.ID)
	if events[0].Decision != governance.DecisionReviewed {
		t.Errorf("decision = %q, want reviewed (recommendation downgraded)", events[0].Decision)
	}
}

func TestMapReviewDecision(t *testing.T) {
	cases := map[string]string{
		ReviewerApprove:        governance.DecisionRecommendedApproval,
		ReviewerRequestChanges: governance.DecisionChangesRequested,
		ReviewerHumanRequired:  governance.DecisionReviewed,
		"something_unknown":    governance.DecisionReviewed,
	}
	for in, want := range cases {
		if got := MapReviewDecision(in); got != want {
			t.Errorf("MapReviewDecision(%q) = %q, want %q", in, got, want)
		}
	}
	// Hard invariant: approve never maps to approved.
	if MapReviewDecision(ReviewerApprove) == governance.DecisionApproved {
		t.Fatal("MapReviewDecision mapped approve to approved — invariant violated")
	}
}

func TestParsersHandleFencedAndProse(t *testing.T) {
	// Fenced planner.
	p, err := ParsePlannerOutput(plannerYAML)
	if err != nil {
		t.Fatalf("fenced planner parse: %v", err)
	}
	if p.Kind != "bug" || len(p.AcceptanceCriteria) != 2 {
		t.Errorf("fenced planner wrong: %+v", p)
	}

	// Prose-wrapped, unfenced reviewer.
	r, err := ParseReviewerOutput(reviewApproveYAML)
	if err != nil {
		t.Fatalf("prose reviewer parse: %v", err)
	}
	if r.Decision != "approve" || len(r.MissingTests) != 1 {
		t.Errorf("prose reviewer wrong: %+v", r)
	}

	// JSON body (yaml.v3 parses JSON as a subset).
	jsonBody := "Sure!\n```json\n{\"decision\": \"human_required\", \"concerns\": [\"too risky\"]}\n```\n"
	rj, err := ParseReviewerOutput(jsonBody)
	if err != nil {
		t.Fatalf("json reviewer parse: %v", err)
	}
	if rj.Decision != "human_required" || len(rj.Concerns) != 1 {
		t.Errorf("json reviewer wrong: %+v", rj)
	}
}

func TestTriagePropagatesCompleterError(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ch := seedChange(t, store)
	fc := &fakeCompleter{err: errors.New("boom")}
	if _, err := Triage(ctx, fc, store, ch, "", "bugbot"); err == nil {
		t.Fatal("expected error from completer failure")
	}
}
