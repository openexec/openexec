package governance

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestStore creates a temp dir with .openexec and returns an opened store.
func newTestStore(t *testing.T) (string, *SQLiteStore) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatalf("failed to create .openexec dir: %v", err)
	}
	_, store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return dir, store
}

func TestSchemaCreatesAllTables(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	want := []string{
		"governance_releases", "change_records", "release_items",
		"decision_events", "review_authorities", "evidence", "communication_artifacts",
	}
	for _, table := range want {
		var name string
		err := store.db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist: %v", table, err)
		}
	}
}

func TestReleaseRoundTrip(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	r := &GovernanceRelease{
		ID: "R-1", Name: "July fixes", Description: "desc", Owner: "perttu",
		Status: ReleaseStatusDraft, Goal: "ship", MustHave: []string{"a", "b"},
		OutOfScope: []string{"c"}, Risk: RiskMedium, ApprovedForAI: true, ApprovedVersion: 2,
	}
	if err := store.CreateRelease(ctx, r); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	got, err := store.GetRelease(ctx, "R-1")
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if got.Name != "July fixes" || got.Owner != "perttu" || got.Risk != RiskMedium {
		t.Errorf("unexpected release: %+v", got)
	}
	if !got.ApprovedForAI || got.ApprovedVersion != 2 {
		t.Errorf("approval fields not preserved: %+v", got)
	}
	if len(got.MustHave) != 2 || got.MustHave[0] != "a" || len(got.OutOfScope) != 1 {
		t.Errorf("slices not preserved: %+v", got)
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not set: %+v", got)
	}

	// Update is idempotent: applying twice yields the same stored values.
	got.Status = ReleaseStatusApproved
	if err := store.UpdateRelease(ctx, got); err != nil {
		t.Fatalf("UpdateRelease #1: %v", err)
	}
	after1, _ := store.GetRelease(ctx, "R-1")
	if err := store.UpdateRelease(ctx, after1); err != nil {
		t.Fatalf("UpdateRelease #2: %v", err)
	}
	after2, _ := store.GetRelease(ctx, "R-1")
	if after2.Status != ReleaseStatusApproved {
		t.Errorf("status not updated: %+v", after2)
	}
	if after1.Status != after2.Status || after1.ID != after2.ID {
		t.Errorf("update not idempotent: %+v vs %+v", after1, after2)
	}

	// Missing record returns ErrNotFound.
	if _, err := store.GetRelease(ctx, "nope"); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestChangeRecordRoundTrip(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	c := &ChangeRecord{
		ID: "CHANGE-1", ReleaseID: "R-1", ProjectID: "unsorry",
		SourceType: SourceGitHubIssue, SourceID: "123", SourceURL: "http://x",
		Title: "bug", RawText: "raw", Summary: "sum", Kind: KindBug, Risk: RiskLow,
		Status: ChangeStatusCandidate, ProposalVersion: 1, ApprovedVersion: 0,
		Plan: "plan", AcceptanceCriteria: "ac", VerificationPlan: "vp",
		Branch: "feature/x", PRURL: "http://pr",
	}
	if err := store.CreateChangeRecord(ctx, c); err != nil {
		t.Fatalf("CreateChangeRecord: %v", err)
	}
	got, err := store.GetChangeRecord(ctx, "CHANGE-1")
	if err != nil {
		t.Fatalf("GetChangeRecord: %v", err)
	}
	if got.Title != "bug" || got.Kind != KindBug || got.ProposalVersion != 1 {
		t.Errorf("unexpected change record: %+v", got)
	}
	if got.ClaimedBy != "" || got.ClaimExpiresAt != nil {
		t.Errorf("expected unclaimed: %+v", got)
	}

	// List filters.
	byRelease, _ := store.ListChangeRecordsByRelease(ctx, "R-1")
	if len(byRelease) != 1 {
		t.Errorf("ListChangeRecordsByRelease: want 1, got %d", len(byRelease))
	}
	byStatus, _ := store.ListChangeRecordsByStatus(ctx, ChangeStatusCandidate)
	if len(byStatus) != 1 {
		t.Errorf("ListChangeRecordsByStatus: want 1, got %d", len(byStatus))
	}

	// Update idempotent.
	got.Status = ChangeStatusPlanReady
	if err := store.UpdateChangeRecord(ctx, got); err != nil {
		t.Fatalf("UpdateChangeRecord: %v", err)
	}
	if err := store.UpdateChangeRecord(ctx, got); err != nil {
		t.Fatalf("UpdateChangeRecord again: %v", err)
	}
	after, _ := store.GetChangeRecord(ctx, "CHANGE-1")
	if after.Status != ChangeStatusPlanReady {
		t.Errorf("status not updated: %+v", after)
	}
}

func TestReleaseItemAndDecisionAndEvidenceRoundTrip(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	if err := store.CreateReleaseItem(ctx, &ReleaseItem{
		ReleaseID: "R-1", ChangeID: "CHANGE-1", ItemType: ItemTypeBug, Priority: 1, Required: true,
	}); err != nil {
		t.Fatalf("CreateReleaseItem: %v", err)
	}
	// Upsert on (release_id, change_id) is idempotent.
	if err := store.CreateReleaseItem(ctx, &ReleaseItem{
		ReleaseID: "R-1", ChangeID: "CHANGE-1", ItemType: ItemTypeBug, Priority: 2, Required: true,
	}); err != nil {
		t.Fatalf("CreateReleaseItem upsert: %v", err)
	}
	items, _ := store.ListReleaseItems(ctx, "R-1")
	if len(items) != 1 || items[0].Priority != 2 || !items[0].Required {
		t.Errorf("release items not upserted correctly: %+v", items)
	}

	if err := store.CreateDecisionEvent(ctx, &DecisionEvent{
		ID: "D-1", ReleaseID: "R-1", ChangeID: "CHANGE-1", ProposalVersion: 1,
		Actor: "perttu", ActorType: ActorTypeHuman, Decision: DecisionApproved, Comment: "ok",
	}); err != nil {
		t.Fatalf("CreateDecisionEvent: %v", err)
	}
	events, _ := store.ListDecisionEvents(ctx, "CHANGE-1")
	if len(events) != 1 || events[0].Decision != DecisionApproved {
		t.Errorf("decision events not stored: %+v", events)
	}

	if err := store.CreateEvidence(ctx, &Evidence{
		ID: "E-1", ChangeID: "CHANGE-1", Kind: EvidenceKindTest, Source: EvidenceSourceCLI, Summary: "go test",
	}); err != nil {
		t.Fatalf("CreateEvidence: %v", err)
	}
	ev, _ := store.ListEvidence(ctx, "CHANGE-1")
	if len(ev) != 1 || ev[0].Kind != EvidenceKindTest {
		t.Errorf("evidence not stored: %+v", ev)
	}

	posted := time.Now().UTC().Truncate(time.Second)
	if err := store.CreateCommunicationArtifact(ctx, &CommunicationArtifact{
		ID: "C-1", ReleaseID: "R-1", ChangeID: "CHANGE-1", Audience: AudienceTester,
		Body: "handoff", PostedTo: "slack", PostedAt: &posted,
	}); err != nil {
		t.Fatalf("CreateCommunicationArtifact: %v", err)
	}
	comms, _ := store.ListCommunicationArtifacts(ctx, "R-1")
	if len(comms) != 1 || comms[0].Audience != AudienceTester || comms[0].PostedAt == nil {
		t.Errorf("communication artifact not stored: %+v", comms)
	}
}

func TestListApprovedWork(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	// Approved release with approved work (should appear).
	mustCreateRelease(t, store, "R-approved", ReleaseStatusApproved)
	mustCreateChange(t, store, "C-ok", "R-approved", "proj", ChangeStatusApprovedForAI)

	// Implementing release with approved work (should appear).
	mustCreateRelease(t, store, "R-impl", ReleaseStatusImplementing)
	mustCreateChange(t, store, "C-impl", "R-impl", "proj", ChangeStatusApprovedForAI)

	// Draft (non-approved) release with approved work (should be hidden).
	mustCreateRelease(t, store, "R-draft", ReleaseStatusDraft)
	mustCreateChange(t, store, "C-hidden", "R-draft", "proj", ChangeStatusApprovedForAI)

	// Approved release but change not approved (should be hidden).
	mustCreateChange(t, store, "C-notapproved", "R-approved", "proj", ChangeStatusPlanReady)

	// Different project (should be filtered out when projectID set).
	mustCreateChange(t, store, "C-otherproj", "R-approved", "other", ChangeStatusApprovedForAI)

	work, err := store.ListApprovedWork(ctx, "proj")
	if err != nil {
		t.Fatalf("ListApprovedWork: %v", err)
	}
	got := idSet(work)
	if !got["C-ok"] || !got["C-impl"] {
		t.Errorf("expected C-ok and C-impl in approved work, got %v", got)
	}
	if got["C-hidden"] || got["C-notapproved"] || got["C-otherproj"] {
		t.Errorf("unexpected work surfaced: %v", got)
	}

	// Claiming C-ok removes it from approved work.
	if err := store.ClaimWork(ctx, "C-ok", "codex", time.Hour); err != nil {
		t.Fatalf("ClaimWork: %v", err)
	}
	work, _ = store.ListApprovedWork(ctx, "proj")
	if idSet(work)["C-ok"] {
		t.Errorf("claimed work should be hidden from approved work")
	}
}

func TestClaimWorkDoubleClaimAndExpiry(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	mustCreateRelease(t, store, "R-1", ReleaseStatusApproved)
	mustCreateChange(t, store, "C-1", "R-1", "proj", ChangeStatusApprovedForAI)

	if err := store.ClaimWork(ctx, "C-1", "codex", time.Hour); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	// Second active claim by a different agent is rejected.
	if err := store.ClaimWork(ctx, "C-1", "claude", time.Hour); err != ErrAlreadyClaimed {
		t.Errorf("expected ErrAlreadyClaimed, got %v", err)
	}
	// Same agent may re-claim (extend lease).
	if err := store.ClaimWork(ctx, "C-1", "codex", time.Hour); err != nil {
		t.Errorf("same-agent reclaim should succeed, got %v", err)
	}

	// Expired lease allows reclaim by another agent.
	if err := store.ClaimWork(ctx, "C-1", "codex", -time.Minute); err != nil {
		t.Fatalf("set expired lease: %v", err)
	}
	if err := store.ClaimWork(ctx, "C-1", "claude", time.Hour); err != nil {
		t.Errorf("reclaim after expiry should succeed, got %v", err)
	}
	got, _ := store.GetChangeRecord(ctx, "C-1")
	if got.ClaimedBy != "claude" {
		t.Errorf("expected claimedBy=claude, got %q", got.ClaimedBy)
	}

	// ReleaseClaim clears it.
	if err := store.ReleaseClaim(ctx, "C-1"); err != nil {
		t.Fatalf("ReleaseClaim: %v", err)
	}
	got, _ = store.GetChangeRecord(ctx, "C-1")
	if got.ClaimedBy != "" || got.ClaimExpiresAt != nil {
		t.Errorf("expected cleared claim, got %+v", got)
	}

	// Claiming a non-existent change returns ErrNotFound.
	if err := store.ClaimWork(ctx, "nope", "codex", time.Hour); err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDefaultAuthoritiesSeededOnce(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	db, store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	ctx := context.Background()

	auths, err := store.ListReviewAuthorities(ctx)
	if err != nil {
		t.Fatalf("ListReviewAuthorities: %v", err)
	}
	if len(auths) != 6 {
		t.Fatalf("expected 6 seeded authorities, got %d", len(auths))
	}

	pm, err := store.GetReviewAuthority(ctx, "pm")
	if err != nil {
		t.Fatalf("GetReviewAuthority(pm): %v", err)
	}
	if pm.Type != AuthorityHuman || pm.RiskLimit != RiskCritical || len(pm.Permissions) == 0 {
		t.Errorf("pm authority unexpected: %+v", pm)
	}

	// Re-run NewSQLiteStore on the SAME db handle: seeding must remain idempotent.
	store2, err := NewSQLiteStore(db)
	if err != nil {
		t.Fatalf("second NewSQLiteStore: %v", err)
	}
	auths2, _ := store2.ListReviewAuthorities(ctx)
	if len(auths2) != 6 {
		t.Errorf("expected 6 authorities after re-seed, got %d", len(auths2))
	}
	store.Close()
}

// TestCreateRejectsDuplicate verifies Create* is INSERT-only: a second create
// with the same id must fail with ErrAlreadyExists rather than silently reset
// the existing record.
func TestCreateRejectsDuplicate(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	// Release.
	r := &GovernanceRelease{ID: "R-1", Name: "orig", Status: ReleaseStatusDraft, Risk: RiskLow}
	if err := store.CreateRelease(ctx, r); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	dup := &GovernanceRelease{ID: "R-1", Name: "overwrite", Status: ReleaseStatusApproved, Risk: RiskHigh}
	if err := store.CreateRelease(ctx, dup); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("CreateRelease duplicate: want ErrAlreadyExists, got %v", err)
	}
	got, _ := store.GetRelease(ctx, "R-1")
	if got.Name != "orig" || got.Status != ReleaseStatusDraft {
		t.Errorf("duplicate CreateRelease overwrote record: %+v", got)
	}

	// Change record.
	c := &ChangeRecord{ID: "C-1", ProjectID: "p", SourceType: SourceManual, Title: "orig", Status: ChangeStatusCandidate, Risk: RiskLow}
	if err := store.CreateChangeRecord(ctx, c); err != nil {
		t.Fatalf("CreateChangeRecord: %v", err)
	}
	dupC := &ChangeRecord{ID: "C-1", ProjectID: "p", SourceType: SourceManual, Title: "overwrite", Status: ChangeStatusDone, Risk: RiskHigh}
	if err := store.CreateChangeRecord(ctx, dupC); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("CreateChangeRecord duplicate: want ErrAlreadyExists, got %v", err)
	}
	gotC, _ := store.GetChangeRecord(ctx, "C-1")
	if gotC.Title != "orig" || gotC.Status != ChangeStatusCandidate {
		t.Errorf("duplicate CreateChangeRecord overwrote record: %+v", gotC)
	}
}

// TestAuditRowsAppendOnly verifies decision events and evidence are immutable:
// a duplicate id returns ErrAlreadyExists and never rewrites the audit row.
func TestAuditRowsAppendOnly(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	if err := store.CreateDecisionEvent(ctx, &DecisionEvent{
		ID: "D-1", ChangeID: "C-1", Actor: "pm", Decision: DecisionApproved, Comment: "first",
	}); err != nil {
		t.Fatalf("CreateDecisionEvent: %v", err)
	}
	if err := store.CreateDecisionEvent(ctx, &DecisionEvent{
		ID: "D-1", ChangeID: "C-1", Actor: "attacker", Decision: DecisionRejected, Comment: "rewrite",
	}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate decision event: want ErrAlreadyExists, got %v", err)
	}
	events, _ := store.ListDecisionEvents(ctx, "C-1")
	if len(events) != 1 || events[0].Comment != "first" || events[0].Decision != DecisionApproved {
		t.Errorf("decision event was mutated by duplicate insert: %+v", events)
	}

	if err := store.CreateEvidence(ctx, &Evidence{
		ID: "E-1", ChangeID: "C-1", Kind: EvidenceKindTest, Source: EvidenceSourceCLI, Summary: "first",
	}); err != nil {
		t.Fatalf("CreateEvidence: %v", err)
	}
	if err := store.CreateEvidence(ctx, &Evidence{
		ID: "E-1", ChangeID: "C-1", Kind: EvidenceKindTest, Source: EvidenceSourceCLI, Summary: "rewrite",
	}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("duplicate evidence: want ErrAlreadyExists, got %v", err)
	}
	ev, _ := store.ListEvidence(ctx, "C-1")
	if len(ev) != 1 || ev[0].Summary != "first" {
		t.Errorf("evidence was mutated by duplicate insert: %+v", ev)
	}
}

// TestManualSourceCoexistence verifies the partial unique index: two manual
// change records (both source_id=”) can coexist, while two github_issue
// records with the same (source_id, project_id) still conflict.
func TestManualSourceCoexistence(t *testing.T) {
	_, store := newTestStore(t)
	ctx := context.Background()

	// Two manual records with empty source_id must both insert.
	for _, id := range []string{"M-1", "M-2"} {
		if err := store.CreateChangeRecord(ctx, &ChangeRecord{
			ID: id, ProjectID: "p", SourceType: SourceManual, SourceID: "",
			Title: id, Status: ChangeStatusCandidate, Risk: RiskLow,
		}); err != nil {
			t.Fatalf("CreateChangeRecord(%s) with empty source_id: %v", id, err)
		}
	}

	// Two github_issue records with the same (source_id, project_id) must collide.
	if err := store.CreateChangeRecord(ctx, &ChangeRecord{
		ID: "G-1", ProjectID: "p", SourceType: SourceGitHubIssue, SourceID: "42",
		Title: "issue 42", Status: ChangeStatusCandidate, Risk: RiskLow,
	}); err != nil {
		t.Fatalf("CreateChangeRecord(G-1): %v", err)
	}
	err := store.CreateChangeRecord(ctx, &ChangeRecord{
		ID: "G-2", ProjectID: "p", SourceType: SourceGitHubIssue, SourceID: "42",
		Title: "dup issue 42", Status: ChangeStatusCandidate, Risk: RiskLow,
	})
	if err == nil {
		t.Fatal("expected duplicate github_issue source key to be rejected, got nil")
	}
}

// TestManualSourceIndexIsPartialAfterMigration verifies the migration replaces a
// legacy full unique index with a partial one, so empty-source_id records that
// would have collided under the old index now coexist.
func TestManualSourceIndexIsPartialAfterMigration(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dbPath := filepath.Join(dir, ".openexec", "openexec.db")

	// Build the real schema, then swap the partial source index back to the OLD
	// full (non-partial) unique index to simulate a legacy database.
	if _, seed, err := Open(dir); err != nil {
		t.Fatalf("seed schema: %v", err)
	} else {
		seed.Close()
	}
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(`DROP INDEX IF EXISTS idx_change_records_source;
	CREATE UNIQUE INDEX idx_change_records_source ON change_records(source_type, source_id, project_id);`); err != nil {
		t.Fatalf("install legacy index: %v", err)
	}
	raw.Close()

	// Opening through the store runs the migration (drops + recreates partial).
	_, store, err := Open(dir)
	if err != nil {
		t.Fatalf("Open (migrate): %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()
	for _, id := range []string{"M-1", "M-2"} {
		if err := store.CreateChangeRecord(ctx, &ChangeRecord{
			ID: id, ProjectID: "p", SourceType: SourceManual, SourceID: "",
			Title: id, Status: ChangeStatusCandidate, Risk: RiskLow,
		}); err != nil {
			t.Fatalf("post-migration CreateChangeRecord(%s): %v", id, err)
		}
	}
}

// TestParseTimeTolerance verifies parseTime accepts RFC3339 and SQLite's
// CURRENT_TIMESTAMP format ("2006-01-02 15:04:05", UTC), not just RFC3339.
func TestParseTimeTolerance(t *testing.T) {
	cases := []struct {
		in   string
		want time.Time
	}{
		{"2026-06-29T12:34:56Z", time.Date(2026, 6, 29, 12, 34, 56, 0, time.UTC)},
		{"2026-06-29 12:34:56", time.Date(2026, 6, 29, 12, 34, 56, 0, time.UTC)},
	}
	for _, c := range cases {
		got := parseTime(sql.NullString{Valid: true, String: c.in})
		if !got.Equal(c.want) {
			t.Errorf("parseTime(%q) = %v, want %v", c.in, got, c.want)
		}
	}
	// Unparseable input still falls back to zero (tolerant, no panic).
	if got := parseTime(sql.NullString{Valid: true, String: "not-a-time"}); !got.IsZero() {
		t.Errorf("parseTime(garbage) = %v, want zero", got)
	}
}

// ---- helpers ----

func mustCreateRelease(t *testing.T, store *SQLiteStore, id, status string) {
	t.Helper()
	if err := store.CreateRelease(context.Background(), &GovernanceRelease{
		ID: id, Name: id, Status: status, Risk: RiskLow,
	}); err != nil {
		t.Fatalf("CreateRelease(%s): %v", id, err)
	}
}

func mustCreateChange(t *testing.T, store *SQLiteStore, id, releaseID, projectID, status string) {
	t.Helper()
	if err := store.CreateChangeRecord(context.Background(), &ChangeRecord{
		ID: id, ReleaseID: releaseID, ProjectID: projectID, SourceType: SourceManual,
		SourceID: id, Title: id, Status: status, Risk: RiskLow,
	}); err != nil {
		t.Fatalf("CreateChangeRecord(%s): %v", id, err)
	}
}

func idSet(records []*ChangeRecord) map[string]bool {
	m := map[string]bool{}
	for _, r := range records {
		m[r.ID] = true
	}
	return m
}
