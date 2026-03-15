package release

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// SQLiteStore is a SQLite-based implementation of Store.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteStore creates a new SQLiteStore with the given database connection.
// The database connection should already be opened. The schema will be initialized automatically.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is required")
	}

	store := &SQLiteStore{db: db}

	// Initialize the schema
	if err := store.initSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the state tables if they don't exist and migrates existing tables.
func (s *SQLiteStore) initSchema(ctx context.Context) error {
	// Enable foreign keys
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	// Migrate existing tables first so that the full schema (including indexes
	// on newer columns like "priority") can be applied without errors.
	if err := s.migrateSchema(ctx); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, StateSchema)
	return err
}

// migrateSchema ensures all expected columns exist on each table, adding any that are missing.
func (s *SQLiteStore) migrateSchema(ctx context.Context) error {
	type migration struct {
		table      string
		column     string
		definition string
	}

	migrations := []migration{
		// stories columns (covers both state schema and release schema gaps)
		{"stories", "epic_id", "TEXT DEFAULT NULL"},
		{"stories", "goal_id", "TEXT DEFAULT NULL"},
		{"stories", "role", "TEXT DEFAULT ''"},
		{"stories", "want", "TEXT DEFAULT ''"},
		{"stories", "benefit", "TEXT DEFAULT ''"},
		{"stories", "verification_script", "TEXT DEFAULT ''"},
		{"stories", "contract", "TEXT DEFAULT ''"},
		{"stories", "tasks", "TEXT DEFAULT '[]'"},
		{"stories", "story_type", "TEXT DEFAULT 'feature'"},
		{"stories", "priority", "INTEGER DEFAULT 0"},
		{"stories", "git_branch", "TEXT DEFAULT ''"},
		{"stories", "git_base_branch", "TEXT DEFAULT ''"},
		{"stories", "git_merged_to", "TEXT DEFAULT ''"},
		{"stories", "git_merge_commit", "TEXT DEFAULT ''"},
		{"stories", "git_merged_at", "DATETIME DEFAULT NULL"},
		{"stories", "git_commit_count", "INTEGER DEFAULT 0"},
		{"stories", "approval_status", "TEXT DEFAULT ''"},
		{"stories", "approval_approved_by", "TEXT DEFAULT ''"},
		{"stories", "approval_approved_at", "DATETIME DEFAULT NULL"},
		{"stories", "approval_comments", "TEXT DEFAULT ''"},
		{"stories", "approval_rejection_reason", "TEXT DEFAULT ''"},
		{"stories", "approval_review_cycle", "INTEGER DEFAULT 0"},
		{"stories", "started_at", "DATETIME DEFAULT NULL"},
		{"stories", "completed_at", "DATETIME DEFAULT NULL"},
		// tasks columns
		{"tasks", "task_type", "TEXT DEFAULT ''"},
		{"tasks", "priority", "INTEGER DEFAULT 0"},
		{"tasks", "assigned_agent", "TEXT DEFAULT ''"},
		{"tasks", "attempt_count", "INTEGER DEFAULT 0"},
		{"tasks", "max_attempts", "INTEGER DEFAULT 3"},
		{"tasks", "git_commits", "TEXT DEFAULT '[]'"},
		{"tasks", "git_branch", "TEXT DEFAULT ''"},
		{"tasks", "git_pr_number", "INTEGER DEFAULT NULL"},
		{"tasks", "git_pr_url", "TEXT DEFAULT ''"},
		{"tasks", "approval_status", "TEXT DEFAULT ''"},
		{"tasks", "approval_approved_by", "TEXT DEFAULT ''"},
		{"tasks", "approval_approved_at", "DATETIME DEFAULT NULL"},
		{"tasks", "approval_comments", "TEXT DEFAULT ''"},
		{"tasks", "approval_rejection_reason", "TEXT DEFAULT ''"},
		{"tasks", "approval_review_cycle", "INTEGER DEFAULT 0"},
		{"tasks", "needs_review", "INTEGER DEFAULT 0"},
		{"tasks", "review_notes", "TEXT DEFAULT ''"},
		{"tasks", "error_message", "TEXT DEFAULT ''"},
		{"tasks", "metadata", "TEXT DEFAULT '{}'"},
		{"tasks", "started_at", "DATETIME DEFAULT NULL"},
		{"tasks", "completed_at", "DATETIME DEFAULT NULL"},
		// releases columns
		{"releases", "git_branch", "TEXT DEFAULT ''"},
		{"releases", "git_base_branch", "TEXT DEFAULT ''"},
		{"releases", "git_tag", "TEXT DEFAULT ''"},
		{"releases", "git_merge_commit", "TEXT DEFAULT ''"},
		{"releases", "git_head_commit", "TEXT DEFAULT ''"},
		{"releases", "approval_status", "TEXT DEFAULT ''"},
		{"releases", "approval_approved_by", "TEXT DEFAULT ''"},
		{"releases", "approval_approved_at", "DATETIME DEFAULT NULL"},
		{"releases", "approval_comments", "TEXT DEFAULT ''"},
		{"releases", "approval_rejection_reason", "TEXT DEFAULT ''"},
		{"releases", "approval_review_cycle", "INTEGER DEFAULT 0"},
		{"releases", "deployed_to", "TEXT DEFAULT '[]'"},
		{"releases", "changelog", "TEXT DEFAULT ''"},
		{"releases", "deployed_at", "DATETIME DEFAULT NULL"},
	}

	tableExistsCache := make(map[string]bool)

	for _, m := range migrations {
		if exists, cached := tableExistsCache[m.table]; cached && !exists {
			continue
		}
		if _, cached := tableExistsCache[m.table]; !cached {
			te, err := s.tableExists(ctx, m.table)
			if err != nil {
				return fmt.Errorf("checking table %s: %w", m.table, err)
			}
			tableExistsCache[m.table] = te
			if !te {
				continue
			}
		}

		colExists, err := s.columnExists(ctx, m.table, m.column)
		if err != nil {
			return fmt.Errorf("checking column %s.%s: %w", m.table, m.column, err)
		}
		if !colExists {
			alter := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", m.table, m.column, m.definition)
			if _, err := s.db.ExecContext(ctx, alter); err != nil {
				return fmt.Errorf("adding column %s.%s: %w", m.table, m.column, err)
			}
		}
	}

	return nil
}

// tableExists checks whether a table exists in the database.
func (s *SQLiteStore) tableExists(ctx context.Context, table string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// columnExists checks whether a column exists on a table using PRAGMA table_info.
func (s *SQLiteStore) columnExists(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dfltValue *string
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Release operations

// CreateRelease stores a new release.
func (s *SQLiteStore) CreateRelease(ctx context.Context, r *Release) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if release already exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM releases WHERE id = 'current')`
	if err := s.db.QueryRowContext(ctx, checkQuery).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check release existence: %w", err)
	}
	if exists {
		return ErrReleaseAlreadyExist
	}

	storiesJSON, err := json.Marshal(r.Stories)
	if err != nil {
		return fmt.Errorf("failed to marshal stories: %w", err)
	}

	deployedToJSON, err := json.Marshal(r.DeployedTo)
	if err != nil {
		return fmt.Errorf("failed to marshal deployed_to: %w", err)
	}

	query := `
		INSERT INTO releases (
			id, name, version, description, status, stories,
			git_branch, git_base_branch, git_tag, git_merge_commit, git_head_commit,
			approval_status, approval_approved_by, approval_approved_at, approval_comments,
			approval_rejection_reason, approval_review_cycle,
			deployed_to, changelog, created_at, started_at, completed_at, deployed_at
		) VALUES (
			'current', ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?,
			?, ?, ?, ?, ?, ?
		)
	`

	var gitBranch, gitBaseBranch, gitTag, gitMergeCommit, gitHeadCommit string
	if r.Git != nil {
		gitBranch = r.Git.Branch
		gitBaseBranch = r.Git.BaseBranch
		gitTag = r.Git.Tag
		gitMergeCommit = r.Git.MergeCommit
		gitHeadCommit = r.Git.HeadCommit
	}

	af := extractApproval(r.Approval)

	_, err = s.db.ExecContext(ctx, query,
		r.Name, r.Version, r.Description, r.Status, storiesJSON,
		gitBranch, gitBaseBranch, gitTag, gitMergeCommit, gitHeadCommit,
		af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
		af.rejectionReason, af.reviewCycle,
		deployedToJSON, r.Changelog, r.CreatedAt.UTC().Format(time.RFC3339),
		nullTimePtr(r.StartedAt), nullTimePtr(r.CompletedAt), nullTimePtr(r.DeployedAt),
	)
	if err != nil {
		return fmt.Errorf("failed to create release: %w", err)
	}

	return nil
}

// GetRelease retrieves the current release.
func (s *SQLiteStore) GetRelease(ctx context.Context) (*Release, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT name, version, description, status, stories,
			git_branch, git_base_branch, git_tag, git_merge_commit, git_head_commit,
			approval_status, approval_approved_by, approval_approved_at, approval_comments,
			approval_rejection_reason, approval_review_cycle,
			deployed_to, changelog, created_at, started_at, completed_at, deployed_at
		FROM releases WHERE id = 'current'
	`

	var r Release
	var storiesJSON, deployedToJSON string
	var gitBranch, gitBaseBranch, gitTag, gitMergeCommit, gitHeadCommit sql.NullString
	var approvalStatus, approvalApprovedBy, approvalComments, approvalRejectionReason sql.NullString
	var approvalApprovedAt, createdAt, startedAt, completedAt, deployedAt sql.NullString
	var approvalReviewCycle sql.NullInt64

	err := s.db.QueryRowContext(ctx, query).Scan(
		&r.Name, &r.Version, &r.Description, &r.Status, &storiesJSON,
		&gitBranch, &gitBaseBranch, &gitTag, &gitMergeCommit, &gitHeadCommit,
		&approvalStatus, &approvalApprovedBy, &approvalApprovedAt, &approvalComments,
		&approvalRejectionReason, &approvalReviewCycle,
		&deployedToJSON, &r.Changelog, &createdAt, &startedAt, &completedAt, &deployedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get release: %w", err)
	}

	// Parse JSON arrays
	if err := json.Unmarshal([]byte(storiesJSON), &r.Stories); err != nil {
		r.Stories = []string{}
	}
	if err := json.Unmarshal([]byte(deployedToJSON), &r.DeployedTo); err != nil {
		r.DeployedTo = []string{}
	}

	// Parse git info
	if gitBranch.Valid && gitBranch.String != "" {
		r.Git = &ReleaseGitInfo{
			Branch:      gitBranch.String,
			BaseBranch:  gitBaseBranch.String,
			Tag:         gitTag.String,
			MergeCommit: gitMergeCommit.String,
			HeadCommit:  gitHeadCommit.String,
		}
	}

	// Parse approval info
	r.Approval = parseApproval(approvalStatus, approvalApprovedBy, approvalApprovedAt, approvalComments, approvalRejectionReason, approvalReviewCycle)

	// Parse timestamps
	r.CreatedAt = parseTime(createdAt)
	r.StartedAt = parseNullTime(startedAt)
	r.CompletedAt = parseNullTime(completedAt)
	r.DeployedAt = parseNullTime(deployedAt)

	return &r, nil
}

// UpdateRelease modifies the current release.
func (s *SQLiteStore) UpdateRelease(ctx context.Context, r *Release) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if release exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM releases WHERE id = 'current')`
	if err := s.db.QueryRowContext(ctx, checkQuery).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check release existence: %w", err)
	}
	if !exists {
		return ErrReleaseNotFound
	}

	storiesJSON, err := json.Marshal(r.Stories)
	if err != nil {
		return fmt.Errorf("failed to marshal stories: %w", err)
	}

	deployedToJSON, err := json.Marshal(r.DeployedTo)
	if err != nil {
		return fmt.Errorf("failed to marshal deployed_to: %w", err)
	}

	query := `
		UPDATE releases SET
			name = ?, version = ?, description = ?, status = ?, stories = ?,
			git_branch = ?, git_base_branch = ?, git_tag = ?, git_merge_commit = ?, git_head_commit = ?,
			approval_status = ?, approval_approved_by = ?, approval_approved_at = ?, approval_comments = ?,
			approval_rejection_reason = ?, approval_review_cycle = ?,
			deployed_to = ?, changelog = ?, started_at = ?, completed_at = ?, deployed_at = ?
		WHERE id = 'current'
	`

	var gitBranch, gitBaseBranch, gitTag, gitMergeCommit, gitHeadCommit string
	if r.Git != nil {
		gitBranch = r.Git.Branch
		gitBaseBranch = r.Git.BaseBranch
		gitTag = r.Git.Tag
		gitMergeCommit = r.Git.MergeCommit
		gitHeadCommit = r.Git.HeadCommit
	}

	af := extractApproval(r.Approval)

	_, err = s.db.ExecContext(ctx, query,
		r.Name, r.Version, r.Description, r.Status, storiesJSON,
		gitBranch, gitBaseBranch, gitTag, gitMergeCommit, gitHeadCommit,
		af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
		af.rejectionReason, af.reviewCycle,
		deployedToJSON, r.Changelog, nullTimePtr(r.StartedAt), nullTimePtr(r.CompletedAt), nullTimePtr(r.DeployedAt),
	)
	if err != nil {
		return fmt.Errorf("failed to update release: %w", err)
	}

	return nil
}

// DeleteRelease removes the current release.
func (s *SQLiteStore) DeleteRelease(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, `DELETE FROM releases WHERE id = 'current'`)
	return err
}

// Goal operations

// CreateGoal stores a new goal.
func (s *SQLiteStore) CreateGoal(ctx context.Context, g *Goal) error {
	if g.ID == "" {
		return ErrInvalidData
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		INSERT INTO goals (id, title, description, success_criteria, verification_method, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.ExecContext(ctx, query,
		g.ID, g.Title, g.Description, g.SuccessCriteria, g.VerificationMethod,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrGoalAlreadyExist
		}
		return fmt.Errorf("failed to create goal: %w", err)
	}

	return nil
}

// GetGoal retrieves a goal by ID.
func (s *SQLiteStore) GetGoal(ctx context.Context, id string) (*Goal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, title, description, success_criteria, verification_method FROM goals WHERE id = ?`

	var g Goal
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&g.ID, &g.Title, &g.Description, &g.SuccessCriteria, &g.VerificationMethod,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrGoalNotFound
		}
		return nil, fmt.Errorf("failed to get goal: %w", err)
	}

	return &g, nil
}

// ListGoals returns all goals.
func (s *SQLiteStore) ListGoals(ctx context.Context) ([]*Goal, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, title, description, success_criteria, verification_method FROM goals ORDER BY id`

	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list goals: %w", err)
	}
	defer rows.Close()

	var goals []*Goal
	for rows.Next() {
		var g Goal
		if err := rows.Scan(&g.ID, &g.Title, &g.Description, &g.SuccessCriteria, &g.VerificationMethod); err != nil {
			return nil, fmt.Errorf("failed to scan goal: %w", err)
		}
		goals = append(goals, &g)
	}

	if goals == nil {
		goals = []*Goal{}
	}

	return goals, rows.Err()
}

// UpdateGoal modifies an existing goal.
func (s *SQLiteStore) UpdateGoal(ctx context.Context, g *Goal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `
		UPDATE goals SET title = ?, description = ?, success_criteria = ?, verification_method = ?
		WHERE id = ?
	`

	result, err := s.db.ExecContext(ctx, query, g.Title, g.Description, g.SuccessCriteria, g.VerificationMethod, g.ID)
	if err != nil {
		return fmt.Errorf("failed to update goal: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrGoalNotFound
	}

	return nil
}

// DeleteGoal removes a goal by ID.
func (s *SQLiteStore) DeleteGoal(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM goals WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete goal: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrGoalNotFound
	}

	return nil
}

// Story operations

// CreateStory stores a new story.
func (s *SQLiteStore) CreateStory(ctx context.Context, story *Story) error {
	if story.ID == "" {
		return ErrInvalidData
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createStoryInternal(ctx, story)
}

func (s *SQLiteStore) createStoryInternal(ctx context.Context, story *Story) error {
	acceptanceCriteriaJSON, err := json.Marshal(story.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("failed to marshal acceptance_criteria: %w", err)
	}

	tasksJSON, err := json.Marshal(story.Tasks)
	if err != nil {
		return fmt.Errorf("failed to marshal tasks: %w", err)
	}

	dependsOnJSON, err := json.Marshal(story.DependsOn)
	if err != nil {
		return fmt.Errorf("failed to marshal depends_on: %w", err)
	}

	var epicID sql.NullString
	if story.EpicID != nil {
		epicID = sql.NullString{String: *story.EpicID, Valid: true}
	}

	// Treat empty goal_id as NULL for optional relationship
	var goalID interface{}
	if story.GoalID == "" {
		goalID = nil
	} else {
		goalID = story.GoalID
	}

	query := `
		INSERT INTO stories (
			id, epic_id, goal_id, title, description, role, want, benefit,
			acceptance_criteria, verification_script, contract, tasks, depends_on,
			story_type, priority,
			git_branch, git_base_branch, git_merged_to, git_merge_commit, git_merged_at, git_commit_count,
			approval_status, approval_approved_by, approval_approved_at, approval_comments,
			approval_rejection_reason, approval_review_cycle,
			status, created_at, started_at, completed_at
		) VALUES (
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?,
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?,
			?, ?, ?, ?
		)
	`

	var gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit string
	var gitMergedAt *time.Time
	var gitCommitCount int
	if story.Git != nil {
		gitBranch = story.Git.Branch
		gitBaseBranch = story.Git.BaseBranch
		gitMergedTo = story.Git.MergedTo
		gitMergeCommit = story.Git.MergeCommit
		gitMergedAt = story.Git.MergedAt
		gitCommitCount = story.Git.CommitCount
	}

	af := extractApproval(story.Approval)

	_, err = s.db.ExecContext(ctx, query,
		story.ID, epicID, goalID, story.Title, story.Description, story.Role, story.Want, story.Benefit,
		acceptanceCriteriaJSON, story.VerificationScript, story.Contract, tasksJSON, dependsOnJSON,
		story.StoryType, story.Priority,
		gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit, nullTimePtr(gitMergedAt), gitCommitCount,
		af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
		af.rejectionReason, af.reviewCycle,
		story.Status, story.CreatedAt.UTC().Format(time.RFC3339), nullTimePtr(story.StartedAt), nullTimePtr(story.CompletedAt),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrStoryAlreadyExist
		}
		return fmt.Errorf("failed to create story: %w", err)
	}

	return nil
}

// GetStory retrieves a story by ID.
func (s *SQLiteStore) GetStory(ctx context.Context, id string) (*Story, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getStoryInternal(ctx, id)
}

func (s *SQLiteStore) getStoryInternal(ctx context.Context, id string) (*Story, error) {
	query := `
		SELECT id, epic_id, goal_id, title, description, role, want, benefit,
			acceptance_criteria, verification_script, contract, tasks, depends_on,
			story_type, priority,
			git_branch, git_base_branch, git_merged_to, git_merge_commit, git_merged_at, git_commit_count,
			approval_status, approval_approved_by, approval_approved_at, approval_comments,
			approval_rejection_reason, approval_review_cycle,
			status, created_at, started_at, completed_at
		FROM stories WHERE id = ?
	`

	var story Story
	var epicID, goalID sql.NullString
	var acceptanceCriteriaJSON, tasksJSON, dependsOnJSON string
	var gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit sql.NullString
	var gitMergedAt sql.NullString
	var gitCommitCount sql.NullInt64
	var approvalStatus, approvalApprovedBy, approvalComments, approvalRejectionReason sql.NullString
	var approvalApprovedAt sql.NullString
	var approvalReviewCycle sql.NullInt64
	var createdAt, startedAt, completedAt sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&story.ID, &epicID, &goalID, &story.Title, &story.Description, &story.Role, &story.Want, &story.Benefit,
		&acceptanceCriteriaJSON, &story.VerificationScript, &story.Contract, &tasksJSON, &dependsOnJSON,
		&story.StoryType, &story.Priority,
		&gitBranch, &gitBaseBranch, &gitMergedTo, &gitMergeCommit, &gitMergedAt, &gitCommitCount,
		&approvalStatus, &approvalApprovedBy, &approvalApprovedAt, &approvalComments,
		&approvalRejectionReason, &approvalReviewCycle,
		&story.Status, &createdAt, &startedAt, &completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrStoryNotFound
		}
		return nil, fmt.Errorf("failed to get story: %w", err)
	}

	if epicID.Valid {
		story.EpicID = &epicID.String
	}
	if goalID.Valid {
		story.GoalID = goalID.String
	}

	// Parse JSON arrays
	if err := json.Unmarshal([]byte(acceptanceCriteriaJSON), &story.AcceptanceCriteria); err != nil {
		story.AcceptanceCriteria = []string{}
	}
	if err := json.Unmarshal([]byte(tasksJSON), &story.Tasks); err != nil {
		story.Tasks = []string{}
	}
	if err := json.Unmarshal([]byte(dependsOnJSON), &story.DependsOn); err != nil {
		story.DependsOn = []string{}
	}

	// Parse git info
	if gitBranch.Valid && gitBranch.String != "" {
		story.Git = &StoryGitInfo{
			Branch:      gitBranch.String,
			BaseBranch:  gitBaseBranch.String,
			MergedTo:    gitMergedTo.String,
			MergeCommit: gitMergeCommit.String,
			CommitCount: int(gitCommitCount.Int64),
		}
		if gitMergedAt.Valid {
			t, _ := time.Parse(time.RFC3339, gitMergedAt.String)
			story.Git.MergedAt = &t
		}
	}

	// Parse approval info
	story.Approval = parseApproval(approvalStatus, approvalApprovedBy, approvalApprovedAt, approvalComments, approvalRejectionReason, approvalReviewCycle)

	// Parse timestamps
	story.CreatedAt = parseTime(createdAt)
	story.StartedAt = parseNullTime(startedAt)
	story.CompletedAt = parseNullTime(completedAt)

	return &story, nil
}

// ListStories returns all stories.
func (s *SQLiteStore) ListStories(ctx context.Context) ([]*Story, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listStoriesWithFilter(ctx, "", "")
}

// ListStoriesByGoal returns stories for a specific goal.
func (s *SQLiteStore) ListStoriesByGoal(ctx context.Context, goalID string) ([]*Story, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listStoriesWithFilter(ctx, "goal_id = ?", goalID)
}

// ListStoriesByStatus returns stories with a specific status.
func (s *SQLiteStore) ListStoriesByStatus(ctx context.Context, status string) ([]*Story, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listStoriesWithFilter(ctx, "status = ?", status)
}

func (s *SQLiteStore) listStoriesWithFilter(ctx context.Context, where string, arg interface{}) ([]*Story, error) {
	query := `SELECT id FROM stories`
	var args []interface{}
	if where != "" {
		query += " WHERE " + where
		args = append(args, arg)
	}
	query += " ORDER BY priority ASC, created_at ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list stories: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan story id: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	stories := make([]*Story, 0, len(ids))
	for _, id := range ids {
		story, err := s.getStoryInternal(ctx, id)
		if err != nil {
			return nil, err
		}
		stories = append(stories, story)
	}

	return stories, nil
}

// UpdateStory modifies an existing story.
func (s *SQLiteStore) UpdateStory(ctx context.Context, story *Story) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acceptanceCriteriaJSON, err := json.Marshal(story.AcceptanceCriteria)
	if err != nil {
		return fmt.Errorf("failed to marshal acceptance_criteria: %w", err)
	}

	tasksJSON, err := json.Marshal(story.Tasks)
	if err != nil {
		return fmt.Errorf("failed to marshal tasks: %w", err)
	}

	dependsOnJSON, err := json.Marshal(story.DependsOn)
	if err != nil {
		return fmt.Errorf("failed to marshal depends_on: %w", err)
	}

	var epicID sql.NullString
	if story.EpicID != nil {
		epicID = sql.NullString{String: *story.EpicID, Valid: true}
	}

	// Treat empty goal_id as NULL for optional relationship
	var goalID interface{}
	if story.GoalID == "" {
		goalID = nil
	} else {
		goalID = story.GoalID
	}

	query := `
		UPDATE stories SET
			epic_id = ?, goal_id = ?, title = ?, description = ?, role = ?, want = ?, benefit = ?,
			acceptance_criteria = ?, verification_script = ?, contract = ?, tasks = ?, depends_on = ?,
			story_type = ?, priority = ?,
			git_branch = ?, git_base_branch = ?, git_merged_to = ?, git_merge_commit = ?, git_merged_at = ?, git_commit_count = ?,
			approval_status = ?, approval_approved_by = ?, approval_approved_at = ?, approval_comments = ?,
			approval_rejection_reason = ?, approval_review_cycle = ?,
			status = ?, started_at = ?, completed_at = ?
		WHERE id = ?
	`

	var gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit string
	var gitMergedAt *time.Time
	var gitCommitCount int
	if story.Git != nil {
		gitBranch = story.Git.Branch
		gitBaseBranch = story.Git.BaseBranch
		gitMergedTo = story.Git.MergedTo
		gitMergeCommit = story.Git.MergeCommit
		gitMergedAt = story.Git.MergedAt
		gitCommitCount = story.Git.CommitCount
	}

	af := extractApproval(story.Approval)

	result, err := s.db.ExecContext(ctx, query,
		epicID, goalID, story.Title, story.Description, story.Role, story.Want, story.Benefit,
		acceptanceCriteriaJSON, story.VerificationScript, story.Contract, tasksJSON, dependsOnJSON,
		story.StoryType, story.Priority,
		gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit, nullTimePtr(gitMergedAt), gitCommitCount,
		af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
		af.rejectionReason, af.reviewCycle,
		story.Status, nullTimePtr(story.StartedAt), nullTimePtr(story.CompletedAt),
		story.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update story: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrStoryNotFound
	}

	return nil
}

// DeleteStory removes a story by ID.
func (s *SQLiteStore) DeleteStory(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM stories WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete story: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrStoryNotFound
	}

	return nil
}

// Task operations

// CreateTask stores a new task.
func (s *SQLiteStore) CreateTask(ctx context.Context, task *Task) error {
	if task.ID == "" {
		return ErrInvalidData
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.createTaskInternal(ctx, task)
}

func (s *SQLiteStore) createTaskInternal(ctx context.Context, task *Task) error {
	dependsOnJSON, err := json.Marshal(task.DependsOn)
	if err != nil {
		return fmt.Errorf("failed to marshal depends_on: %w", err)
	}

	metadataJSON, err := json.Marshal(task.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	var gitCommitsJSON []byte
	var gitBranch, gitPRUrl string
	var gitPRNumber sql.NullInt64
	if task.Git != nil {
		gitCommitsJSON, err = json.Marshal(task.Git.Commits)
		if err != nil {
			gitCommitsJSON = []byte("[]")
		}
		gitBranch = task.Git.Branch
		gitPRUrl = task.Git.PRUrl
		if task.Git.PRNumber != nil {
			gitPRNumber = sql.NullInt64{Int64: int64(*task.Git.PRNumber), Valid: true}
		}
	} else {
		gitCommitsJSON = []byte("[]")
	}

	query := `
		INSERT INTO tasks (
			id, story_id, title, description, verification_script, depends_on,
			task_type, priority, assigned_agent, attempt_count, max_attempts,
			git_commits, git_branch, git_pr_number, git_pr_url,
			approval_status, approval_approved_by, approval_approved_at, approval_comments,
			approval_rejection_reason, approval_review_cycle,
			needs_review, review_notes, status, created_at, started_at, completed_at, error_message, metadata
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?,
			?, ?, ?, ?, ?, ?, ?, ?
		)
	`

	af := extractApproval(task.Approval)

	needsReview := 0
	if task.NeedsReview {
		needsReview = 1
	}

	_, err = s.db.ExecContext(ctx, query,
		task.ID, task.StoryID, task.Title, task.Description, task.VerificationScript, dependsOnJSON,
		task.TaskType, task.Priority, task.AssignedAgent, task.AttemptCount, task.MaxAttempts,
		gitCommitsJSON, gitBranch, gitPRNumber, gitPRUrl,
		af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
		af.rejectionReason, af.reviewCycle,
		needsReview, task.ReviewNotes, task.Status, task.CreatedAt.UTC().Format(time.RFC3339),
		nullTimePtr(task.StartedAt), nullTimePtr(task.CompletedAt), task.ErrorMessage, metadataJSON,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrTaskAlreadyExist
		}
		return fmt.Errorf("failed to create task: %w", err)
	}

	return nil
}

// GetTask retrieves a task by ID.
func (s *SQLiteStore) GetTask(ctx context.Context, id string) (*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getTaskInternal(ctx, id)
}

func (s *SQLiteStore) getTaskInternal(ctx context.Context, id string) (*Task, error) {
	query := `
		SELECT id, story_id, title, description, verification_script, depends_on,
			task_type, priority, assigned_agent, attempt_count, max_attempts,
			git_commits, git_branch, git_pr_number, git_pr_url,
			approval_status, approval_approved_by, approval_approved_at, approval_comments,
			approval_rejection_reason, approval_review_cycle,
			needs_review, review_notes, status, created_at, started_at, completed_at, error_message, metadata
		FROM tasks WHERE id = ?
	`

	var task Task
	var dependsOnJSON, gitCommitsJSON, metadataJSON string
	var gitBranch, gitPRUrl sql.NullString
	var gitPRNumber sql.NullInt64
	var approvalStatus, approvalApprovedBy, approvalComments, approvalRejectionReason sql.NullString
	var approvalApprovedAt sql.NullString
	var approvalReviewCycle sql.NullInt64
	var needsReview sql.NullInt64
	var createdAt, startedAt, completedAt sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&task.ID, &task.StoryID, &task.Title, &task.Description, &task.VerificationScript, &dependsOnJSON,
		&task.TaskType, &task.Priority, &task.AssignedAgent, &task.AttemptCount, &task.MaxAttempts,
		&gitCommitsJSON, &gitBranch, &gitPRNumber, &gitPRUrl,
		&approvalStatus, &approvalApprovedBy, &approvalApprovedAt, &approvalComments,
		&approvalRejectionReason, &approvalReviewCycle,
		&needsReview, &task.ReviewNotes, &task.Status, &createdAt, &startedAt, &completedAt, &task.ErrorMessage, &metadataJSON,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrTaskNotFound
		}
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	// Parse JSON arrays
	if err := json.Unmarshal([]byte(dependsOnJSON), &task.DependsOn); err != nil {
		task.DependsOn = []string{}
	}
	if err := json.Unmarshal([]byte(metadataJSON), &task.Metadata); err != nil {
		task.Metadata = nil
	}

	// Parse git info
	var commits []string
	if err := json.Unmarshal([]byte(gitCommitsJSON), &commits); err != nil {
		commits = []string{}
	}
	if len(commits) > 0 || (gitBranch.Valid && gitBranch.String != "") {
		task.Git = &TaskGitInfo{
			Commits: commits,
			Branch:  gitBranch.String,
			PRUrl:   gitPRUrl.String,
		}
		if gitPRNumber.Valid {
			prNum := int(gitPRNumber.Int64)
			task.Git.PRNumber = &prNum
		}
	}

	// Parse approval info
	task.Approval = parseApproval(approvalStatus, approvalApprovedBy, approvalApprovedAt, approvalComments, approvalRejectionReason, approvalReviewCycle)

	// Parse timestamps
	task.CreatedAt = parseTime(createdAt)
	task.StartedAt = parseNullTime(startedAt)
	task.CompletedAt = parseNullTime(completedAt)

	task.NeedsReview = needsReview.Valid && needsReview.Int64 == 1

	return &task, nil
}

// ListTasks returns all tasks.
func (s *SQLiteStore) ListTasks(ctx context.Context) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listTasksWithFilter(ctx, "", "")
}

// ListTasksByStory returns tasks for a specific story.
func (s *SQLiteStore) ListTasksByStory(ctx context.Context, storyID string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listTasksWithFilter(ctx, "story_id = ?", storyID)
}

// ListTasksByStatus returns tasks with a specific status.
func (s *SQLiteStore) ListTasksByStatus(ctx context.Context, status string) ([]*Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.listTasksWithFilter(ctx, "status = ?", status)
}

func (s *SQLiteStore) listTasksWithFilter(ctx context.Context, where string, arg interface{}) ([]*Task, error) {
	query := `SELECT id FROM tasks`
	var args []interface{}
	if where != "" {
		query += " WHERE " + where
		args = append(args, arg)
	}
	query += " ORDER BY created_at ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan task id: %w", err)
		}
		ids = append(ids, id)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	tasks := make([]*Task, 0, len(ids))
	for _, id := range ids {
		task, err := s.getTaskInternal(ctx, id)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// UpdateTask modifies an existing task.
func (s *SQLiteStore) UpdateTask(ctx context.Context, task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dependsOnJSON, err := json.Marshal(task.DependsOn)
	if err != nil {
		return fmt.Errorf("failed to marshal depends_on: %w", err)
	}

	metadataJSON, err := json.Marshal(task.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	var gitCommitsJSON []byte
	var gitBranch, gitPRUrl string
	var gitPRNumber sql.NullInt64
	if task.Git != nil {
		gitCommitsJSON, err = json.Marshal(task.Git.Commits)
		if err != nil {
			gitCommitsJSON = []byte("[]")
		}
		gitBranch = task.Git.Branch
		gitPRUrl = task.Git.PRUrl
		if task.Git.PRNumber != nil {
			gitPRNumber = sql.NullInt64{Int64: int64(*task.Git.PRNumber), Valid: true}
		}
	} else {
		gitCommitsJSON = []byte("[]")
	}

	query := `
		UPDATE tasks SET
			story_id = ?, title = ?, description = ?, verification_script = ?, depends_on = ?,
			task_type = ?, priority = ?, assigned_agent = ?, attempt_count = ?, max_attempts = ?,
			git_commits = ?, git_branch = ?, git_pr_number = ?, git_pr_url = ?,
			approval_status = ?, approval_approved_by = ?, approval_approved_at = ?, approval_comments = ?,
			approval_rejection_reason = ?, approval_review_cycle = ?,
			needs_review = ?, review_notes = ?, status = ?, started_at = ?, completed_at = ?, error_message = ?, metadata = ?
		WHERE id = ?
	`

	af := extractApproval(task.Approval)

	needsReview := 0
	if task.NeedsReview {
		needsReview = 1
	}

	result, err := s.db.ExecContext(ctx, query,
		task.StoryID, task.Title, task.Description, task.VerificationScript, dependsOnJSON,
		task.TaskType, task.Priority, task.AssignedAgent, task.AttemptCount, task.MaxAttempts,
		gitCommitsJSON, gitBranch, gitPRNumber, gitPRUrl,
		af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
		af.rejectionReason, af.reviewCycle,
		needsReview, task.ReviewNotes, task.Status, nullTimePtr(task.StartedAt), nullTimePtr(task.CompletedAt), task.ErrorMessage, metadataJSON,
		task.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrTaskNotFound
	}

	return nil
}

// DeleteTask removes a task by ID.
func (s *SQLiteStore) DeleteTask(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return ErrTaskNotFound
	}

	return nil
}

// Bulk operations for bootstrap

// BulkCreateGoals creates multiple goals in a single transaction.
func (s *SQLiteStore) BulkCreateGoals(ctx context.Context, goals []*Goal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO goals (id, title, description, success_criteria, verification_method, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339)
	for _, g := range goals {
		if g.ID == "" {
			continue
		}
		_, err := stmt.ExecContext(ctx, g.ID, g.Title, g.Description, g.SuccessCriteria, g.VerificationMethod, now)
		if err != nil {
			if isUniqueViolation(err) {
				continue // Skip duplicates
			}
			return fmt.Errorf("failed to insert goal %s: %w", g.ID, err)
		}
	}

	return tx.Commit()
}

// BulkCreateStories creates multiple stories in a single transaction.
func (s *SQLiteStore) BulkCreateStories(ctx context.Context, stories []*Story) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, story := range stories {
		if story.ID == "" {
			continue
		}

		acceptanceCriteriaJSON, _ := json.Marshal(story.AcceptanceCriteria)
		tasksJSON, _ := json.Marshal(story.Tasks)
		dependsOnJSON, _ := json.Marshal(story.DependsOn)

		var epicID sql.NullString
		if story.EpicID != nil {
			epicID = sql.NullString{String: *story.EpicID, Valid: true}
		}

		// Treat empty goal_id as NULL for optional relationship
		var goalID interface{}
		if story.GoalID == "" {
			goalID = nil
		} else {
			goalID = story.GoalID
		}

		var gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit string
		var gitMergedAt *time.Time
		var gitCommitCount int
		if story.Git != nil {
			gitBranch = story.Git.Branch
			gitBaseBranch = story.Git.BaseBranch
			gitMergedTo = story.Git.MergedTo
			gitMergeCommit = story.Git.MergeCommit
			gitMergedAt = story.Git.MergedAt
			gitCommitCount = story.Git.CommitCount
		}

		af := extractApproval(story.Approval)

		_, err := tx.ExecContext(ctx, `
			INSERT INTO stories (
				id, epic_id, goal_id, title, description, role, want, benefit,
				acceptance_criteria, verification_script, contract, tasks, depends_on,
				story_type, priority,
				git_branch, git_base_branch, git_merged_to, git_merge_commit, git_merged_at, git_commit_count,
				approval_status, approval_approved_by, approval_approved_at, approval_comments,
				approval_rejection_reason, approval_review_cycle,
				status, created_at, started_at, completed_at
			) VALUES (
				?, ?, ?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?,
				?, ?,
				?, ?, ?, ?, ?, ?,
				?, ?, ?, ?,
				?, ?,
				?, ?, ?, ?
			)
		`,
			story.ID, epicID, goalID, story.Title, story.Description, story.Role, story.Want, story.Benefit,
			acceptanceCriteriaJSON, story.VerificationScript, story.Contract, tasksJSON, dependsOnJSON,
			story.StoryType, story.Priority,
			gitBranch, gitBaseBranch, gitMergedTo, gitMergeCommit, nullTimePtr(gitMergedAt), gitCommitCount,
			af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
			af.rejectionReason, af.reviewCycle,
			story.Status, story.CreatedAt.UTC().Format(time.RFC3339), nullTimePtr(story.StartedAt), nullTimePtr(story.CompletedAt),
		)
		if err != nil {
			if isUniqueViolation(err) {
				continue // Skip duplicates
			}
			return fmt.Errorf("failed to insert story %s: %w", story.ID, err)
		}
	}

	return tx.Commit()
}

// BulkCreateTasks creates multiple tasks in a single transaction.
func (s *SQLiteStore) BulkCreateTasks(ctx context.Context, tasks []*Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, task := range tasks {
		if task.ID == "" {
			continue
		}

		dependsOnJSON, _ := json.Marshal(task.DependsOn)
		metadataJSON, _ := json.Marshal(task.Metadata)
		if metadataJSON == nil {
			metadataJSON = []byte("{}")
		}

		var gitCommitsJSON []byte
		var gitBranch, gitPRUrl string
		var gitPRNumber sql.NullInt64
		if task.Git != nil {
			gitCommitsJSON, _ = json.Marshal(task.Git.Commits)
			gitBranch = task.Git.Branch
			gitPRUrl = task.Git.PRUrl
			if task.Git.PRNumber != nil {
				gitPRNumber = sql.NullInt64{Int64: int64(*task.Git.PRNumber), Valid: true}
			}
		} else {
			gitCommitsJSON = []byte("[]")
		}

		af := extractApproval(task.Approval)

		needsReview := 0
		if task.NeedsReview {
			needsReview = 1
		}

		_, err := tx.ExecContext(ctx, `
			INSERT INTO tasks (
				id, story_id, title, description, verification_script, depends_on,
				task_type, priority, assigned_agent, attempt_count, max_attempts,
				git_commits, git_branch, git_pr_number, git_pr_url,
				approval_status, approval_approved_by, approval_approved_at, approval_comments,
				approval_rejection_reason, approval_review_cycle,
				needs_review, review_notes, status, created_at, started_at, completed_at, error_message, metadata
			) VALUES (
				?, ?, ?, ?, ?, ?,
				?, ?, ?, ?, ?,
				?, ?, ?, ?,
				?, ?, ?, ?,
				?, ?,
				?, ?, ?, ?, ?, ?, ?, ?
			)
		`,
			task.ID, task.StoryID, task.Title, task.Description, task.VerificationScript, dependsOnJSON,
			task.TaskType, task.Priority, task.AssignedAgent, task.AttemptCount, task.MaxAttempts,
			gitCommitsJSON, gitBranch, gitPRNumber, gitPRUrl,
			af.status, af.approvedBy, nullTimePtr(af.approvedAt), af.comments,
			af.rejectionReason, af.reviewCycle,
			needsReview, task.ReviewNotes, task.Status, task.CreatedAt.UTC().Format(time.RFC3339),
			nullTimePtr(task.StartedAt), nullTimePtr(task.CompletedAt), task.ErrorMessage, metadataJSON,
		)
		if err != nil {
			if isUniqueViolation(err) {
				continue // Skip duplicates
			}
			return fmt.Errorf("failed to insert task %s: %w", task.ID, err)
		}
	}

	return tx.Commit()
}

// Count operations

// CountGoals returns the number of goals.
func (s *SQLiteStore) CountGoals(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM goals`).Scan(&count)
	return count, err
}

// CountStories returns the number of stories.
func (s *SQLiteStore) CountStories(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stories`).Scan(&count)
	return count, err
}

// CountTasks returns the number of tasks.
func (s *SQLiteStore) CountTasks(ctx context.Context) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tasks`).Scan(&count)
	return count, err
}

// Helper functions

func nullTimePtr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

func parseNullTime(s sql.NullString) *time.Time {
	if !s.Valid || s.String == "" {
		return nil
	}
	t, _ := time.Parse(time.RFC3339, s.String)
	return &t
}

func parseTime(s sql.NullString) time.Time {
	if !s.Valid || s.String == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s.String)
	return t
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// approvalFields holds approval info values for database operations.
type approvalFields struct {
	status          string
	approvedBy      string
	approvedAt      *time.Time
	comments        string
	rejectionReason string
	reviewCycle     int
}

// extractApproval extracts approval field values from an ApprovalInfo pointer.
func extractApproval(a *ApprovalInfo) approvalFields {
	if a == nil {
		return approvalFields{}
	}
	return approvalFields{
		status:          a.Status,
		approvedBy:      a.ApprovedBy,
		approvedAt:      a.ApprovedAt,
		comments:        a.Comments,
		rejectionReason: a.RejectionReason,
		reviewCycle:     a.ReviewCycle,
	}
}

// parseApproval reconstructs an ApprovalInfo from scanned nullable fields.
func parseApproval(status, approvedBy, approvedAt, comments, rejectionReason sql.NullString, reviewCycle sql.NullInt64) *ApprovalInfo {
	if !status.Valid || status.String == "" {
		return nil
	}
	return &ApprovalInfo{
		Status:          status.String,
		ApprovedBy:      approvedBy.String,
		ApprovedAt:      parseNullTime(approvedAt),
		Comments:        comments.String,
		RejectionReason: rejectionReason.String,
		ReviewCycle:     int(reviewCycle.Int64),
	}
}

// Ensure SQLiteStore implements Store interface.
var _ Store = (*SQLiteStore)(nil)
