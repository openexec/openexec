package governance

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/pkg/db/sqlitecfg"

	_ "modernc.org/sqlite"
)

// Sentinel errors returned by the store.
var (
	ErrNotFound       = errors.New("governance: record not found")
	ErrInvalidData    = errors.New("governance: invalid data")
	ErrAlreadyClaimed = errors.New("governance: work already claimed by an active executor")
	ErrAlreadyExists  = errors.New("governance: record already exists")
)

// isConstraintErr reports whether err is a SQLite constraint violation
// (PRIMARY KEY / UNIQUE). modernc.org/sqlite surfaces these as "constraint
// failed" in the error text. Used to turn a duplicate-ID INSERT into a clear
// "already exists" error instead of a raw driver error.
func isConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "constraint failed")
}

// Store is the public contract other governance subsystems (CLI, MCP, UI) bind to.
// All methods are safe for concurrent use.
type Store interface {
	// Releases.
	CreateRelease(ctx context.Context, r *GovernanceRelease) error
	GetRelease(ctx context.Context, id string) (*GovernanceRelease, error)
	UpdateRelease(ctx context.Context, r *GovernanceRelease) error
	ListReleases(ctx context.Context) ([]*GovernanceRelease, error)
	ListReleasesByStatus(ctx context.Context, status string) ([]*GovernanceRelease, error)

	// Change records.
	CreateChangeRecord(ctx context.Context, c *ChangeRecord) error
	GetChangeRecord(ctx context.Context, id string) (*ChangeRecord, error)
	UpdateChangeRecord(ctx context.Context, c *ChangeRecord) error
	ListChangeRecords(ctx context.Context) ([]*ChangeRecord, error)
	ListChangeRecordsByRelease(ctx context.Context, releaseID string) ([]*ChangeRecord, error)
	ListChangeRecordsByStatus(ctx context.Context, status string) ([]*ChangeRecord, error)

	// Release items.
	CreateReleaseItem(ctx context.Context, item *ReleaseItem) error
	ListReleaseItems(ctx context.Context, releaseID string) ([]*ReleaseItem, error)

	// Decision events.
	CreateDecisionEvent(ctx context.Context, e *DecisionEvent) error
	ListDecisionEvents(ctx context.Context, changeID string) ([]*DecisionEvent, error)
	// ListAllDecisionEvents returns every event in insertion order (for export /
	// whole-trail verification), including release-level events with no change_id.
	ListAllDecisionEvents(ctx context.Context) ([]*DecisionEvent, error)
	// VerifyAuditChain recomputes the decision-event hash chain and reports the
	// first break (ok=false with a reason), or ok=true when intact.
	VerifyAuditChain(ctx context.Context) (ok bool, reason string, count int, err error)

	// Review authorities.
	GetReviewAuthority(ctx context.Context, id string) (*ReviewAuthority, error)
	ListReviewAuthorities(ctx context.Context) ([]*ReviewAuthority, error)

	// Evidence.
	CreateEvidence(ctx context.Context, e *Evidence) error
	ListEvidence(ctx context.Context, changeID string) ([]*Evidence, error)

	// Communication artifacts.
	CreateCommunicationArtifact(ctx context.Context, a *CommunicationArtifact) error
	ListCommunicationArtifacts(ctx context.Context, releaseID string) ([]*CommunicationArtifact, error)

	// Queue / claim primitives.
	ListApprovedWork(ctx context.Context, projectID string) ([]*ChangeRecord, error)
	ClaimWork(ctx context.Context, changeID, agent string, lease time.Duration) error
	ReleaseClaim(ctx context.Context, changeID string) error

	// GitHub comment-ingestion cursor (highest processed comment id per change).
	GetIngestCursor(ctx context.Context, changeID string) (int64, error)
	SetIngestCursor(ctx context.Context, changeID string, lastCommentID int64) error

	// Change -> planner-story links (deep-triage decomposition).
	LinkChangeStory(ctx context.Context, changeID, storyID string) error
	ListChangeStories(ctx context.Context, changeID string) ([]string, error)

	// File-level impact analysis (JSON ImpactReport) per change.
	SetChangeImpact(ctx context.Context, changeID, reportJSON string) error
	GetChangeImpact(ctx context.Context, changeID string) (string, error)

	// Operability / production-readiness report (JSON) per change.
	SetChangeOperability(ctx context.Context, changeID, reportJSON string) error
	GetChangeOperability(ctx context.Context, changeID string) (string, error)

	Close() error
}

// SQLiteStore is the SQLite-backed implementation of Store.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// Ensure SQLiteStore implements Store.
var _ Store = (*SQLiteStore)(nil)

// NewSQLiteStore creates a SQLiteStore over an already-open database connection.
// It initializes the schema and seeds default review authorities idempotently.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	s := &SQLiteStore{db: db}
	if err := s.initSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	if err := s.seedAuthorities(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to seed review authorities: %w", err)
	}
	return s, nil
}

// Open resolves .openexec/openexec.db under baseDir, opens it via the standard
// sqlitecfg DSN (WAL + pragmas), and returns a ready store. The caller owns the
// returned *sql.DB; calling Close on the store closes it.
func Open(baseDir string) (*sql.DB, *SQLiteStore, error) {
	dbPath := filepath.Join(baseDir, ".openexec", "openexec.db")
	db, err := sql.Open("sqlite", sqlitecfg.DSN(dbPath))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open governance database: %w", err)
	}
	store, err := NewSQLiteStore(db)
	if err != nil {
		db.Close()
		return nil, nil, err
	}
	return db, store, nil
}

func (s *SQLiteStore) initSchema(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// ALTER-migration list applied before the schema const so existing tables
	// gain new columns (CREATE TABLE IF NOT EXISTS won't add them).
	migrations := [][3]string{
		{"change_records", "claimed_by", "TEXT DEFAULT ''"},
		{"change_records", "claim_expires_at", "DATETIME DEFAULT NULL"},
		{"change_records", "light", "INTEGER DEFAULT 0"},
		{"decision_events", "prev_hash", "TEXT DEFAULT ''"},
		{"decision_events", "hash", "TEXT DEFAULT ''"},
	}
	for _, m := range migrations {
		if s.tableExists(ctx, m[0]) && !s.columnExists(ctx, m[0], m[1]) {
			ddl := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", m[0], m[1], m[2])
			if _, err := s.db.ExecContext(ctx, ddl); err != nil {
				return fmt.Errorf("failed to migrate %s.%s: %w", m[0], m[1], err)
			}
		}
	}

	// Migrate idx_change_records_source from a full UNIQUE index to a PARTIAL one
	// (WHERE source_id != ''). The full index collided for any two records with
	// the default empty source_id (e.g. two manual change records). CREATE INDEX
	// IF NOT EXISTS in Schema below will not replace an existing index, so drop
	// the legacy (non-partial) definition first. Guarded on the stored index SQL
	// so this is safe to run repeatedly: once the index carries a WHERE clause it
	// is already partial and we leave it alone.
	var idxSQL sql.NullString
	switch err := s.db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='index' AND name='idx_change_records_source'`).Scan(&idxSQL); {
	case err == sql.ErrNoRows:
		// No such index yet (fresh DB); Schema creates it partial below.
	case err != nil:
		return fmt.Errorf("failed to inspect source index: %w", err)
	default:
		if idxSQL.Valid && !strings.Contains(strings.ToUpper(idxSQL.String), "WHERE") {
			if _, derr := s.db.ExecContext(ctx, `DROP INDEX IF EXISTS idx_change_records_source`); derr != nil {
				return fmt.Errorf("failed to drop legacy source index: %w", derr)
			}
		}
	}

	_, err := s.db.ExecContext(ctx, Schema)
	return err
}

func (s *SQLiteStore) tableExists(ctx context.Context, table string) bool {
	var name string
	err := s.db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
	return err == nil
}

func (s *SQLiteStore) columnExists(ctx context.Context, table, column string) bool {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// seedAuthorities inserts the default review authorities idempotently.
func (s *SQLiteStore) seedAuthorities(ctx context.Context) error {
	defaults := []*ReviewAuthority{
		{
			ID: "pm", Name: "Product Manager", Type: AuthorityHuman,
			Permissions: []string{PermComment, PermRequestChanges, PermRecommendApproval, PermApprove, PermRiskAccept, PermMarkDone},
			RiskLimit:   RiskCritical,
		},
		{
			ID: "developer", Name: "Developer", Type: AuthorityHuman,
			Permissions: []string{PermComment, PermRequestChanges, PermRecommendApproval, PermApproveLowRisk, PermMarkDone},
			RiskLimit:   RiskMedium,
		},
		{
			ID: "bugbot", Name: "Bug Triage AI", Type: AuthorityAI,
			Permissions: []string{PermComment, PermRequestChanges, PermRecommendApproval},
			RiskLimit:   RiskHigh,
		},
		{
			ID: "tester_ai", Name: "Tester AI", Type: AuthorityAI,
			Permissions: []string{PermComment, PermMarkDone},
			RiskLimit:   RiskMedium,
		},
		{
			ID: "security_ai", Name: "Security Review AI", Type: AuthorityAI,
			Permissions: []string{PermComment, PermRequestChanges, PermRecommendApproval},
			RiskLimit:   RiskCritical,
		},
		{
			ID: "ci_verifier", Name: "CI Verifier", Type: AuthorityVerifier,
			Permissions: []string{PermComment, PermMarkDone},
			RiskLimit:   RiskLow,
		},
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, a := range defaults {
		permsJSON, _ := json.Marshal(a.Permissions)
		_, err := s.db.ExecContext(ctx,
			`INSERT OR IGNORE INTO review_authorities (id, name, type, permissions, risk_limit) VALUES (?, ?, ?, ?, ?)`,
			a.ID, a.Name, a.Type, string(permsJSON), a.RiskLimit,
		)
		if err != nil {
			return fmt.Errorf("failed to seed authority %s: %w", a.ID, err)
		}
	}
	return nil
}

// ---- GovernanceRelease ----

func (s *SQLiteStore) CreateRelease(ctx context.Context, r *GovernanceRelease) error {
	if r == nil || r.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}
	// Create is INSERT-only: a second CreateRelease with the same id must fail
	// loudly rather than silently reset an existing release.
	return s.writeRelease(ctx, r, true)
}

func (s *SQLiteStore) UpdateRelease(ctx context.Context, r *GovernanceRelease) error {
	if r == nil || r.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	r.UpdatedAt = time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = r.UpdatedAt
	}
	return s.writeRelease(ctx, r, false)
}

// writeRelease inserts a release, upserting on id conflict unless insertOnly is
// set (in which case a duplicate id returns ErrAlreadyExists).
func (s *SQLiteStore) writeRelease(ctx context.Context, r *GovernanceRelease, insertOnly bool) error {
	mustHaveJSON, _ := json.Marshal(strSlice(r.MustHave))
	outOfScopeJSON, _ := json.Marshal(strSlice(r.OutOfScope))
	query := `
		INSERT INTO governance_releases (
			id, name, description, owner, status, goal, must_have, out_of_scope,
			risk, approved_for_ai, approved_version, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if !insertOnly {
		query += `
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name, description = excluded.description, owner = excluded.owner,
			status = excluded.status, goal = excluded.goal, must_have = excluded.must_have,
			out_of_scope = excluded.out_of_scope, risk = excluded.risk,
			approved_for_ai = excluded.approved_for_ai, approved_version = excluded.approved_version,
			updated_at = excluded.updated_at`
	}
	_, err := s.db.ExecContext(ctx, query,
		r.ID, r.Name, r.Description, r.Owner, r.Status, r.Goal, string(mustHaveJSON), string(outOfScopeJSON),
		r.Risk, boolToInt(r.ApprovedForAI), r.ApprovedVersion, fmtTime(r.CreatedAt), fmtTime(r.UpdatedAt),
	)
	if err != nil {
		if insertOnly && isConstraintErr(err) {
			return fmt.Errorf("release %q: %w", r.ID, ErrAlreadyExists)
		}
		return fmt.Errorf("failed to write release: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetRelease(ctx context.Context, id string) (*GovernanceRelease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getReleaseInternal(ctx, id)
}

func (s *SQLiteStore) getReleaseInternal(ctx context.Context, id string) (*GovernanceRelease, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, description, owner, status, goal, must_have, out_of_scope,
			risk, approved_for_ai, approved_version, created_at, updated_at
		FROM governance_releases WHERE id = ?`, id)
	r, err := scanRelease(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return r, err
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanRelease(sc rowScanner) (*GovernanceRelease, error) {
	var r GovernanceRelease
	var mustHaveJSON, outOfScopeJSON string
	var approvedForAI int
	var createdAt, updatedAt sql.NullString
	if err := sc.Scan(
		&r.ID, &r.Name, &r.Description, &r.Owner, &r.Status, &r.Goal, &mustHaveJSON, &outOfScopeJSON,
		&r.Risk, &approvedForAI, &r.ApprovedVersion, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	r.MustHave = unmarshalList(mustHaveJSON)
	r.OutOfScope = unmarshalList(outOfScopeJSON)
	r.ApprovedForAI = approvedForAI == 1
	r.CreatedAt = parseTime(createdAt)
	r.UpdatedAt = parseTime(updatedAt)
	return &r, nil
}

func (s *SQLiteStore) ListReleases(ctx context.Context) ([]*GovernanceRelease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listReleases(ctx, "", "")
}

func (s *SQLiteStore) ListReleasesByStatus(ctx context.Context, status string) ([]*GovernanceRelease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listReleases(ctx, "status = ?", status)
}

func (s *SQLiteStore) listReleases(ctx context.Context, where, arg string) ([]*GovernanceRelease, error) {
	query := `SELECT id, name, description, owner, status, goal, must_have, out_of_scope,
		risk, approved_for_ai, approved_version, created_at, updated_at FROM governance_releases`
	var args []interface{}
	if where != "" {
		query += " WHERE " + where
		args = append(args, arg)
	}
	query += " ORDER BY updated_at DESC, id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list releases: %w", err)
	}
	defer rows.Close()

	out := []*GovernanceRelease{}
	for rows.Next() {
		r, err := scanRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- ChangeRecord ----

func (s *SQLiteStore) CreateChangeRecord(ctx context.Context, c *ChangeRecord) error {
	if c == nil || c.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = now
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = now
	}
	// Create is INSERT-only: a second CreateChangeRecord with the same id must
	// fail loudly rather than silently reset an existing change record.
	return s.writeChangeRecord(ctx, c, true)
}

func (s *SQLiteStore) UpdateChangeRecord(ctx context.Context, c *ChangeRecord) error {
	if c == nil || c.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	c.UpdatedAt = time.Now().UTC()
	if c.CreatedAt.IsZero() {
		c.CreatedAt = c.UpdatedAt
	}
	return s.writeChangeRecord(ctx, c, false)
}

// writeChangeRecord inserts a change record, upserting on id conflict unless
// insertOnly is set (in which case a duplicate id returns ErrAlreadyExists).
func (s *SQLiteStore) writeChangeRecord(ctx context.Context, c *ChangeRecord, insertOnly bool) error {
	query := `
		INSERT INTO change_records (
			id, release_id, project_id, source_type, source_id, source_url, title, raw_text,
			summary, kind, risk, status, proposal_version, approved_version, plan,
			acceptance_criteria, verification_plan, branch, pr_url, claimed_by, claim_expires_at,
			light, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if !insertOnly {
		query += `
		ON CONFLICT(id) DO UPDATE SET
			release_id = excluded.release_id, project_id = excluded.project_id,
			source_type = excluded.source_type, source_id = excluded.source_id,
			source_url = excluded.source_url, title = excluded.title, raw_text = excluded.raw_text,
			summary = excluded.summary, kind = excluded.kind, risk = excluded.risk,
			status = excluded.status, proposal_version = excluded.proposal_version,
			approved_version = excluded.approved_version, plan = excluded.plan,
			acceptance_criteria = excluded.acceptance_criteria, verification_plan = excluded.verification_plan,
			branch = excluded.branch, pr_url = excluded.pr_url, claimed_by = excluded.claimed_by,
			claim_expires_at = excluded.claim_expires_at, light = excluded.light,
			updated_at = excluded.updated_at`
	}
	_, err := s.db.ExecContext(ctx, query,
		c.ID, c.ReleaseID, c.ProjectID, c.SourceType, c.SourceID, c.SourceURL, c.Title, c.RawText,
		c.Summary, c.Kind, c.Risk, c.Status, c.ProposalVersion, c.ApprovedVersion, c.Plan,
		c.AcceptanceCriteria, c.VerificationPlan, c.Branch, c.PRURL, c.ClaimedBy, nullTime(c.ClaimExpiresAt),
		boolToInt(c.Light), fmtTime(c.CreatedAt), fmtTime(c.UpdatedAt),
	)
	if err != nil {
		if insertOnly && isConstraintErr(err) {
			return fmt.Errorf("change record %q: %w", c.ID, ErrAlreadyExists)
		}
		return fmt.Errorf("failed to write change record: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetChangeRecord(ctx context.Context, id string) (*ChangeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getChangeRecordInternal(ctx, id)
}

func (s *SQLiteStore) getChangeRecordInternal(ctx context.Context, id string) (*ChangeRecord, error) {
	row := s.db.QueryRowContext(ctx, changeRecordSelect+` WHERE id = ?`, id)
	c, err := scanChangeRecord(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

const changeRecordSelect = `
	SELECT id, release_id, project_id, source_type, source_id, source_url, title, raw_text,
		summary, kind, risk, status, proposal_version, approved_version, plan,
		acceptance_criteria, verification_plan, branch, pr_url, claimed_by, claim_expires_at,
		light, created_at, updated_at
	FROM change_records`

func scanChangeRecord(sc rowScanner) (*ChangeRecord, error) {
	var c ChangeRecord
	var claimExpiresAt, createdAt, updatedAt sql.NullString
	var light sql.NullInt64
	if err := sc.Scan(
		&c.ID, &c.ReleaseID, &c.ProjectID, &c.SourceType, &c.SourceID, &c.SourceURL, &c.Title, &c.RawText,
		&c.Summary, &c.Kind, &c.Risk, &c.Status, &c.ProposalVersion, &c.ApprovedVersion, &c.Plan,
		&c.AcceptanceCriteria, &c.VerificationPlan, &c.Branch, &c.PRURL, &c.ClaimedBy, &claimExpiresAt,
		&light, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	c.Light = light.Int64 != 0
	c.ClaimExpiresAt = parseNullTime(claimExpiresAt)
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return &c, nil
}

func (s *SQLiteStore) ListChangeRecords(ctx context.Context) ([]*ChangeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listChangeRecords(ctx, "", "")
}

func (s *SQLiteStore) ListChangeRecordsByRelease(ctx context.Context, releaseID string) ([]*ChangeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listChangeRecords(ctx, "release_id = ?", releaseID)
}

func (s *SQLiteStore) ListChangeRecordsByStatus(ctx context.Context, status string) ([]*ChangeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listChangeRecords(ctx, "status = ?", status)
}

func (s *SQLiteStore) listChangeRecords(ctx context.Context, where, arg string) ([]*ChangeRecord, error) {
	query := changeRecordSelect
	var args []interface{}
	if where != "" {
		query += " WHERE " + where
		args = append(args, arg)
	}
	query += " ORDER BY updated_at DESC, id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list change records: %w", err)
	}
	defer rows.Close()

	out := []*ChangeRecord{}
	for rows.Next() {
		c, err := scanChangeRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- ReleaseItem ----

func (s *SQLiteStore) CreateReleaseItem(ctx context.Context, item *ReleaseItem) error {
	if item == nil || item.ReleaseID == "" || item.ChangeID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO release_items (release_id, change_id, item_type, priority, required, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(release_id, change_id) DO UPDATE SET
			item_type = excluded.item_type, priority = excluded.priority, required = excluded.required
	`,
		item.ReleaseID, item.ChangeID, item.ItemType, item.Priority, boolToInt(item.Required), fmtTime(item.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert release item: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListReleaseItems(ctx context.Context, releaseID string) ([]*ReleaseItem, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT release_id, change_id, item_type, priority, required, created_at
		FROM release_items WHERE release_id = ? ORDER BY priority ASC, created_at ASC`, releaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to list release items: %w", err)
	}
	defer rows.Close()

	out := []*ReleaseItem{}
	for rows.Next() {
		var it ReleaseItem
		var required int
		var createdAt sql.NullString
		if err := rows.Scan(&it.ReleaseID, &it.ChangeID, &it.ItemType, &it.Priority, &required, &createdAt); err != nil {
			return nil, err
		}
		it.Required = required == 1
		it.CreatedAt = parseTime(createdAt)
		out = append(out, &it)
	}
	return out, rows.Err()
}

// ---- DecisionEvent ----

func (s *SQLiteStore) CreateDecisionEvent(ctx context.Context, e *DecisionEvent) error {
	if e == nil || e.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	// Tamper-evident hash chain. Under the write lock, read the hash of the most
	// recently inserted event (by rowid — insertion order) and link this event to
	// it. Any later alteration, deletion, or reordering breaks the chain and is
	// caught by VerifyAuditChain. The lock serialises the read-then-insert within
	// this process; a cross-process interleaving could fork the chain, which is
	// still DETECTED at verify time (that is the guarantee — detection, not a
	// distributed lock).
	var prev sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT hash FROM decision_events ORDER BY rowid DESC LIMIT 1`).Scan(&prev); err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to read audit chain head: %w", err)
	}
	e.PrevHash = prev.String
	e.Hash = chainHash(e.PrevHash, e)

	// Append-only: decision events are immutable audit rows. A duplicate id must
	// never silently rewrite history — it returns ErrAlreadyExists instead.
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO decision_events (id, release_id, change_id, proposal_version, actor, actor_type, decision, comment, created_at, prev_hash, hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.ID, e.ReleaseID, e.ChangeID, e.ProposalVersion, e.Actor, e.ActorType, e.Decision, e.Comment, fmtTime(e.CreatedAt), e.PrevHash, e.Hash,
	)
	if err != nil {
		if isConstraintErr(err) {
			return fmt.Errorf("decision event %q is immutable and already exists: %w", e.ID, ErrAlreadyExists)
		}
		return fmt.Errorf("failed to insert decision event: %w", err)
	}
	return nil
}

// chainHash computes the tamper-evident hash for a decision event: the SHA-256
// of the previous event's hash concatenated with a canonical, unit-separated
// encoding of this event's immutable fields. The unit separator (0x1f) cannot
// appear in the encoded values, so distinct events cannot collide by field
// concatenation. created_at is encoded via fmtTime for a stable representation.
func chainHash(prevHash string, e *DecisionEvent) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x1f%s\x1f%s\x1f%s\x1f%d\x1f%s\x1f%s\x1f%s\x1f%s\x1f%s",
		prevHash, e.ID, e.ReleaseID, e.ChangeID, e.ProposalVersion,
		e.Actor, e.ActorType, e.Decision, e.Comment, fmtTime(e.CreatedAt))
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyAuditChain walks every decision event in insertion order and recomputes
// the hash chain. It returns ok=true when the chain is intact. On the first
// break it returns ok=false and a human-readable reason naming the offending
// event (a mismatched hash => the row was altered; a mismatched prev_hash => a
// row was inserted out of band, deleted, or reordered). count is the number of
// events verified up to and including the break.
func (s *SQLiteStore) VerifyAuditChain(ctx context.Context) (ok bool, reason string, count int, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, change_id, proposal_version, actor, actor_type, decision, comment, created_at, prev_hash, hash
		FROM decision_events ORDER BY rowid ASC`)
	if err != nil {
		return false, "", 0, fmt.Errorf("failed to read decision events: %w", err)
	}
	defer rows.Close()

	prev := ""
	for rows.Next() {
		var e DecisionEvent
		var createdAt, prevHash, hash sql.NullString
		if err := rows.Scan(&e.ID, &e.ReleaseID, &e.ChangeID, &e.ProposalVersion, &e.Actor, &e.ActorType, &e.Decision, &e.Comment, &createdAt, &prevHash, &hash); err != nil {
			return false, "", count, err
		}
		e.CreatedAt = parseTime(createdAt)
		e.PrevHash = prevHash.String
		e.Hash = hash.String
		count++
		if e.PrevHash != prev {
			return false, fmt.Sprintf("chain break at event %q: prev_hash does not match the preceding event (a row was deleted, reordered, or inserted out of band)", e.ID), count, nil
		}
		if want := chainHash(prev, &e); want != e.Hash {
			return false, fmt.Sprintf("tampered event %q: recomputed hash does not match stored hash (a field was altered)", e.ID), count, nil
		}
		prev = e.Hash
	}
	return true, "", count, rows.Err()
}

func (s *SQLiteStore) ListDecisionEvents(ctx context.Context, changeID string) ([]*DecisionEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Order by rowid (insertion order) — the reliable audit ordering. created_at
	// is second-precision and id is a random UUID, so ORDER BY created_at,id could
	// reorder same-second events; rowid is monotonic and, because the table is
	// append-only (no deletes), never reused.
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, change_id, proposal_version, actor, actor_type, decision, comment, created_at, prev_hash, hash
		FROM decision_events WHERE change_id = ? ORDER BY rowid ASC`, changeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list decision events: %w", err)
	}
	defer rows.Close()

	return scanDecisionRows(rows)
}

// ListAllDecisionEvents returns every decision event in the store in insertion
// order (rowid). It underpins the whole-trail audit export and chain
// verification: release-level events (empty change_id) are invisible to
// ListDecisionEvents' per-change filter, so an export must not rely on that.
func (s *SQLiteStore) ListAllDecisionEvents(ctx context.Context) ([]*DecisionEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, change_id, proposal_version, actor, actor_type, decision, comment, created_at, prev_hash, hash
		FROM decision_events ORDER BY rowid ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list decision events: %w", err)
	}
	defer rows.Close()
	return scanDecisionRows(rows)
}

// scanDecisionRows materializes decision-event rows selected in the canonical
// column order (id..hash). Shared by the per-change and whole-store listers.
func scanDecisionRows(rows *sql.Rows) ([]*DecisionEvent, error) {
	out := []*DecisionEvent{}
	for rows.Next() {
		var e DecisionEvent
		var createdAt, prevHash, hash sql.NullString
		if err := rows.Scan(&e.ID, &e.ReleaseID, &e.ChangeID, &e.ProposalVersion, &e.Actor, &e.ActorType, &e.Decision, &e.Comment, &createdAt, &prevHash, &hash); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(createdAt)
		e.PrevHash = prevHash.String
		e.Hash = hash.String
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ---- ReviewAuthority ----

func (s *SQLiteStore) GetReviewAuthority(ctx context.Context, id string) (*ReviewAuthority, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	row := s.db.QueryRowContext(ctx, `SELECT id, name, type, permissions, risk_limit FROM review_authorities WHERE id = ?`, id)
	a, err := scanAuthority(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *SQLiteStore) ListReviewAuthorities(ctx context.Context) ([]*ReviewAuthority, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, permissions, risk_limit FROM review_authorities ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("failed to list review authorities: %w", err)
	}
	defer rows.Close()

	out := []*ReviewAuthority{}
	for rows.Next() {
		a, err := scanAuthority(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func scanAuthority(sc rowScanner) (*ReviewAuthority, error) {
	var a ReviewAuthority
	var permsJSON string
	if err := sc.Scan(&a.ID, &a.Name, &a.Type, &permsJSON, &a.RiskLimit); err != nil {
		return nil, err
	}
	a.Permissions = unmarshalList(permsJSON)
	return &a, nil
}

// ---- Evidence ----

func (s *SQLiteStore) CreateEvidence(ctx context.Context, e *Evidence) error {
	if e == nil || e.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now().UTC()
	}
	// Append-only: evidence rows are immutable audit records. A duplicate id must
	// never silently rewrite recorded evidence — it returns ErrAlreadyExists.
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO evidence (id, change_id, kind, source, summary, url, raw, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		e.ID, e.ChangeID, e.Kind, e.Source, e.Summary, e.URL, e.Raw, fmtTime(e.CreatedAt),
	)
	if err != nil {
		if isConstraintErr(err) {
			return fmt.Errorf("evidence %q is immutable and already exists: %w", e.ID, ErrAlreadyExists)
		}
		return fmt.Errorf("failed to insert evidence: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListEvidence(ctx context.Context, changeID string) ([]*Evidence, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, change_id, kind, source, summary, url, raw, created_at
		FROM evidence WHERE change_id = ? ORDER BY created_at ASC, id ASC`, changeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list evidence: %w", err)
	}
	defer rows.Close()

	out := []*Evidence{}
	for rows.Next() {
		var e Evidence
		var createdAt sql.NullString
		if err := rows.Scan(&e.ID, &e.ChangeID, &e.Kind, &e.Source, &e.Summary, &e.URL, &e.Raw, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = parseTime(createdAt)
		out = append(out, &e)
	}
	return out, rows.Err()
}

// ---- CommunicationArtifact ----

func (s *SQLiteStore) CreateCommunicationArtifact(ctx context.Context, a *CommunicationArtifact) error {
	if a == nil || a.ID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO communication_artifacts (id, release_id, change_id, audience, body, posted_to, posted_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			release_id = excluded.release_id, change_id = excluded.change_id, audience = excluded.audience,
			body = excluded.body, posted_to = excluded.posted_to, posted_at = excluded.posted_at
	`,
		a.ID, a.ReleaseID, a.ChangeID, a.Audience, a.Body, a.PostedTo, nullTime(a.PostedAt), fmtTime(a.CreatedAt),
	)
	if err != nil {
		return fmt.Errorf("failed to upsert communication artifact: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListCommunicationArtifacts(ctx context.Context, releaseID string) ([]*CommunicationArtifact, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, release_id, change_id, audience, body, posted_to, posted_at, created_at
		FROM communication_artifacts WHERE release_id = ? ORDER BY created_at ASC, id ASC`, releaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to list communication artifacts: %w", err)
	}
	defer rows.Close()

	out := []*CommunicationArtifact{}
	for rows.Next() {
		var a CommunicationArtifact
		var postedAt, createdAt sql.NullString
		if err := rows.Scan(&a.ID, &a.ReleaseID, &a.ChangeID, &a.Audience, &a.Body, &a.PostedTo, &postedAt, &createdAt); err != nil {
			return nil, err
		}
		a.PostedAt = parseNullTime(postedAt)
		a.CreatedAt = parseTime(createdAt)
		out = append(out, &a)
	}
	return out, rows.Err()
}

// ---- Queue / claim primitives ----

// ListApprovedWork returns change records that are approved for AI execution, belong
// to a release in approved/implementing status, and are not currently claimed by an
// active executor. If projectID is non-empty, results are scoped to that project.
func (s *SQLiteStore) ListApprovedWork(ctx context.Context, projectID string) ([]*ChangeRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Include implementing changes as well as approved_for_ai: an executor that
	// claimed work and died leaves the change in implementing with an expired
	// lease. The isActivelyClaimed filter below keeps actively-claimed work
	// hidden, so only genuinely reclaimable work (unclaimed or lease-expired) is
	// returned — otherwise crashed-executor work would never requeue.
	query := changeRecordSelect + `
		WHERE status IN (?, ?)
		  AND release_id IN (SELECT id FROM governance_releases WHERE status IN (?, ?))`
	args := []interface{}{ChangeStatusApprovedForAI, ChangeStatusImplementing, ReleaseStatusApproved, ReleaseStatusImplementing}
	if projectID != "" {
		query += " AND project_id = ?"
		args = append(args, projectID)
	}
	query += " ORDER BY updated_at ASC, id ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list approved work: %w", err)
	}
	defer rows.Close()

	now := time.Now().UTC()
	out := []*ChangeRecord{}
	for rows.Next() {
		c, err := scanChangeRecord(rows)
		if err != nil {
			return nil, err
		}
		if isActivelyClaimed(c, now) {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ClaimWork sets the claim on a change record if it is unclaimed or its lease has
// expired. It returns ErrAlreadyClaimed if another executor holds an active claim,
// and ErrNotFound if the change does not exist.
func (s *SQLiteStore) ClaimWork(ctx context.Context, changeID, agent string, lease time.Duration) error {
	if changeID == "" || agent == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	expiry := now.Add(lease)
	// Single conditional UPDATE so the claim is atomic against other processes
	// writing the same DB (CLI + long-lived mcp-serve). The WHERE clause takes
	// the lease only if the row is unclaimed, held by this same agent (renewal),
	// or its lease has expired. RowsAffected==0 means either the change is gone
	// or another executor holds an active claim.
	res, err := s.db.ExecContext(ctx,
		`UPDATE change_records
		    SET claimed_by = ?, claim_expires_at = ?, updated_at = ?
		  WHERE id = ?
		    AND (claimed_by = '' OR claimed_by IS NULL OR claimed_by = ?
		         OR claim_expires_at IS NULL OR claim_expires_at = '' OR claim_expires_at <= ?)`,
		agent, fmtTime(expiry), fmtTime(now), changeID, agent, fmtTime(now))
	if err != nil {
		return fmt.Errorf("failed to claim work: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to claim work: %w", err)
	}
	if n == 0 {
		// Distinguish "does not exist" from "actively claimed by another".
		if _, err := s.getChangeRecordInternal(ctx, changeID); err != nil {
			return err
		}
		return ErrAlreadyClaimed
	}
	return nil
}

// ReleaseClaim clears the claim on a change record. It is a no-op if the change has
// no active claim, but returns ErrNotFound if the change does not exist.
func (s *SQLiteStore) ReleaseClaim(ctx context.Context, changeID string) error {
	if changeID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.getChangeRecordInternal(ctx, changeID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE change_records SET claimed_by = '', claim_expires_at = NULL, updated_at = ? WHERE id = ?`,
		fmtTime(time.Now().UTC()), changeID)
	if err != nil {
		return fmt.Errorf("failed to release claim: %w", err)
	}
	return nil
}

func isActivelyClaimed(c *ChangeRecord, now time.Time) bool {
	if c.ClaimedBy == "" || c.ClaimExpiresAt == nil {
		return false
	}
	return c.ClaimExpiresAt.After(now)
}

// ---- helpers ----

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func strSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func unmarshalList(s string) []string {
	if s == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

func fmtTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func nullTime(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// timeLayouts are tried in order by parseTime. RFC3339 is what fmtTime writes,
// but a row created via SQLite's CURRENT_TIMESTAMP default lands as
// "2006-01-02 15:04:05" (space-separated, UTC, no zone) — layouts without a
// zone parse as UTC, which is correct here.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

func parseTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s.String); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseNullTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, _ := time.Parse(time.RFC3339, s.String)
	return &t
}

// GetIngestCursor returns the highest GitHub comment id already processed for a
// change (0 if none recorded yet).
func (s *SQLiteStore) GetIngestCursor(ctx context.Context, changeID string) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var last int64
	err := s.db.QueryRowContext(ctx,
		`SELECT last_comment_id FROM github_ingest_cursor WHERE change_id = ?`, changeID).Scan(&last)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("get ingest cursor for %q: %w", changeID, err)
	}
	return last, nil
}

// SetIngestCursor records the highest GitHub comment id processed for a change.
func (s *SQLiteStore) SetIngestCursor(ctx context.Context, changeID string, lastCommentID int64) error {
	if changeID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO github_ingest_cursor (change_id, last_comment_id) VALUES (?, ?)
		 ON CONFLICT(change_id) DO UPDATE SET last_comment_id = excluded.last_comment_id`,
		changeID, lastCommentID)
	if err != nil {
		return fmt.Errorf("set ingest cursor for %q: %w", changeID, err)
	}
	return nil
}

// LinkChangeStory records that a planner-generated story decomposes a change's
// intent. Idempotent on (change_id, story_id).
func (s *SQLiteStore) LinkChangeStory(ctx context.Context, changeID, storyID string) error {
	if changeID == "" || storyID == "" {
		return ErrInvalidData
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO change_story_links (change_id, story_id) VALUES (?, ?)
		 ON CONFLICT(change_id, story_id) DO NOTHING`,
		changeID, storyID)
	if err != nil {
		return fmt.Errorf("link change %q to story %q: %w", changeID, storyID, err)
	}
	return nil
}

// ListChangeStories returns the story ids linked to a change, oldest first.
func (s *SQLiteStore) ListChangeStories(ctx context.Context, changeID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.QueryContext(ctx,
		`SELECT story_id FROM change_story_links WHERE change_id = ? ORDER BY created_at ASC, story_id ASC`, changeID)
	if err != nil {
		return nil, fmt.Errorf("list stories for change %q: %w", changeID, err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SetChangeImpact stores (or replaces) the file-level impact report for a change.
func (s *SQLiteStore) SetChangeImpact(ctx context.Context, changeID, reportJSON string) error {
	if changeID == "" {
		return ErrInvalidData
	}
	if reportJSON == "" {
		reportJSON = "{}"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO change_impact (change_id, report_json) VALUES (?, ?)
		 ON CONFLICT(change_id) DO UPDATE SET report_json = excluded.report_json`,
		changeID, reportJSON)
	if err != nil {
		return fmt.Errorf("set impact for change %q: %w", changeID, err)
	}
	return nil
}

// GetChangeImpact returns the stored impact report JSON for a change ("" if none).
func (s *SQLiteStore) GetChangeImpact(ctx context.Context, changeID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var j string
	err := s.db.QueryRowContext(ctx,
		`SELECT report_json FROM change_impact WHERE change_id = ?`, changeID).Scan(&j)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get impact for change %q: %w", changeID, err)
	}
	return j, nil
}

// SetChangeOperability stores (or replaces) the operability report for a change.
func (s *SQLiteStore) SetChangeOperability(ctx context.Context, changeID, reportJSON string) error {
	if changeID == "" {
		return ErrInvalidData
	}
	if reportJSON == "" {
		reportJSON = "{}"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO change_operability (change_id, report_json) VALUES (?, ?)
		 ON CONFLICT(change_id) DO UPDATE SET report_json = excluded.report_json`,
		changeID, reportJSON)
	if err != nil {
		return fmt.Errorf("set operability for change %q: %w", changeID, err)
	}
	return nil
}

// GetChangeOperability returns the stored operability report JSON ("" if none).
func (s *SQLiteStore) GetChangeOperability(ctx context.Context, changeID string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var j string
	err := s.db.QueryRowContext(ctx,
		`SELECT report_json FROM change_operability WHERE change_id = ?`, changeID).Scan(&j)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get operability for change %q: %w", changeID, err)
	}
	return j, nil
}
