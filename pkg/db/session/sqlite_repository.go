package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SQLiteRepository is a SQLite-based implementation of Repository.
type SQLiteRepository struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteRepository creates a new SQLiteRepository with the given database connection.
// The database connection should already be opened. The schema will be initialized automatically.
func NewSQLiteRepository(db *sql.DB) (*SQLiteRepository, error) {
	if db == nil {
		return nil, errors.New("database connection is required")
	}

	repo := &SQLiteRepository{db: db}

	// Initialize the schema
	if err := repo.initSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return repo, nil
}

// initSchema creates the session tables if they don't exist.
func (r *SQLiteRepository) initSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, Schema)
	return err
}

// Close closes the database connection.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// Session operations

// CreateSession stores a new session.
func (r *SQLiteRepository) CreateSession(ctx context.Context, session *Session) error {
	if err := session.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		INSERT INTO sessions (id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		session.ID,
		session.ProjectPath,
		session.Provider,
		session.Model,
		session.Title,
		nullStringPtr(session.ParentSessionID),
		nullStringPtr(session.ForkPointMessageID),
		session.Status.String(),
		session.CreatedAt.UTC().Format(time.RFC3339),
		session.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrSessionAlreadyExist
		}
		return fmt.Errorf("failed to create session: %w", err)
	}

	return nil
}

// GetSession retrieves a session by its ID.
func (r *SQLiteRepository) GetSession(ctx context.Context, id string) (*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanSession(row)
}

// UpdateSession modifies an existing session.
func (r *SQLiteRepository) UpdateSession(ctx context.Context, session *Session) error {
	if err := session.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if session exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`
	if err := r.db.QueryRowContext(ctx, checkQuery, session.ID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return ErrSessionNotFound
	}

	query := `
		UPDATE sessions
		SET project_path = ?, provider = ?, model = ?, title = ?,
			parent_session_id = ?, fork_point_message_id = ?, status = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query,
		session.ProjectPath,
		session.Provider,
		session.Model,
		session.Title,
		nullStringPtr(session.ParentSessionID),
		nullStringPtr(session.ForkPointMessageID),
		session.Status.String(),
		session.UpdatedAt.UTC().Format(time.RFC3339),
		session.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	return nil
}

// DeleteSession removes a session by its ID.
func (r *SQLiteRepository) DeleteSession(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `DELETE FROM sessions WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// ListSessions returns all sessions with optional filtering.
func (r *SQLiteRepository) ListSessions(ctx context.Context, opts *ListSessionsOptions) ([]*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at FROM sessions`
	args := []interface{}{}

	if opts != nil && opts.Status != "" {
		query += ` WHERE status = ?`
		args = append(args, opts.Status.String())
	}

	// Default ordering
	orderBy := "created_at"
	orderDir := "DESC"
	if opts != nil {
		if opts.OrderBy != "" {
			orderBy = opts.OrderBy
		}
		if opts.OrderDir != "" {
			orderDir = opts.OrderDir
		}
	}
	query += fmt.Sprintf(` ORDER BY %s %s`, orderBy, orderDir)

	if opts != nil && opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
		if opts.Offset > 0 {
			query += ` OFFSET ?`
			args = append(args, opts.Offset)
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// ListSessionsByProject returns all sessions for a specific project path.
func (r *SQLiteRepository) ListSessionsByProject(ctx context.Context, projectPath string) ([]*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE project_path = ? ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions by project: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// GetSessionForks returns all sessions that were forked from the given session.
func (r *SQLiteRepository) GetSessionForks(ctx context.Context, sessionID string) ([]*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE parent_session_id = ? ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session forks: %w", err)
	}
	defer rows.Close()

	return scanSessions(rows)
}

// ForkSession creates a new session forked from an existing session.
func (r *SQLiteRepository) ForkSession(ctx context.Context, parentSessionID string, opts *ForkOptions) (*Session, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Get parent session
	parentQuery := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, parentQuery, parentSessionID)
	parent, err := scanSession(row)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent session: %w", err)
	}

	// Verify the fork point message exists and belongs to the parent session
	var messageExists bool
	var messageSessionID string
	msgQuery := `SELECT EXISTS(SELECT 1 FROM messages WHERE id = ?), COALESCE((SELECT session_id FROM messages WHERE id = ?), '')`
	if err := r.db.QueryRowContext(ctx, msgQuery, opts.ForkPointMessageID, opts.ForkPointMessageID).Scan(&messageExists, &messageSessionID); err != nil {
		return nil, fmt.Errorf("failed to verify fork point message: %w", err)
	}
	if !messageExists || messageSessionID != parentSessionID {
		return nil, ErrInvalidForkPoint
	}

	// Create the forked session using the Session factory method
	forkedSession, err := parent.ForkSession(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create fork: %w", err)
	}

	// Insert the forked session
	insertQuery := `
		INSERT INTO sessions (id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = r.db.ExecContext(ctx, insertQuery,
		forkedSession.ID,
		forkedSession.ProjectPath,
		forkedSession.Provider,
		forkedSession.Model,
		forkedSession.Title,
		forkedSession.ParentSessionID.String,
		forkedSession.ForkPointMessageID.String,
		forkedSession.Status.String(),
		forkedSession.CreatedAt.UTC().Format(time.RFC3339),
		forkedSession.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert forked session: %w", err)
	}

	// If CopyMessages is true, copy messages up to the fork point
	if opts.CopyMessages {
		if err := r.copyMessagesToFork(ctx, parentSessionID, forkedSession.ID, opts.ForkPointMessageID, opts.CopyToolCalls); err != nil {
			// Rollback by deleting the session
			_, _ = r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, forkedSession.ID)
			return nil, fmt.Errorf("failed to copy messages to fork: %w", err)
		}
	}

	// If CopySummaries is true, copy session summaries
	if opts.CopySummaries {
		if err := r.copySummariesToFork(ctx, parentSessionID, forkedSession.ID); err != nil {
			// Non-fatal error, log and continue
			// In production, you might want to handle this differently
		}
	}

	return forkedSession, nil
}

// copyMessagesToFork copies messages from parent to fork up to the fork point.
func (r *SQLiteRepository) copyMessagesToFork(ctx context.Context, parentSessionID, forkSessionID, forkPointMessageID string, copyToolCalls bool) error {
	// Get the fork point message's created_at timestamp
	var forkPointCreatedAt string
	query := `SELECT created_at FROM messages WHERE id = ?`
	if err := r.db.QueryRowContext(ctx, query, forkPointMessageID).Scan(&forkPointCreatedAt); err != nil {
		return fmt.Errorf("failed to get fork point timestamp: %w", err)
	}

	// Get all messages up to and including the fork point
	msgQuery := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages
		WHERE session_id = ? AND created_at <= ?
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, msgQuery, parentSessionID, forkPointCreatedAt)
	if err != nil {
		return fmt.Errorf("failed to query parent messages: %w", err)
	}
	defer rows.Close()

	messages, err := scanMessages(rows)
	if err != nil {
		return err
	}

	// Map old message IDs to new message IDs for tool call copying
	messageIDMap := make(map[string]string)

	// Copy each message
	for _, msg := range messages {
		newMsgID := uuid.New().String()
		messageIDMap[msg.ID] = newMsgID

		insertQuery := `
			INSERT INTO messages (id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`
		_, err := r.db.ExecContext(ctx, insertQuery,
			newMsgID,
			forkSessionID,
			msg.Role.String(),
			msg.Content,
			msg.TokensInput,
			msg.TokensOutput,
			msg.CostUSD,
			msg.CreatedAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			return fmt.Errorf("failed to copy message: %w", err)
		}

		// Copy tool calls for this message if requested
		if copyToolCalls {
			if err := r.copyToolCallsForMessage(ctx, msg.ID, newMsgID, forkSessionID); err != nil {
				return fmt.Errorf("failed to copy tool calls: %w", err)
			}
		}
	}

	return nil
}

// copyToolCallsForMessage copies tool calls from one message to another.
func (r *SQLiteRepository) copyToolCallsForMessage(ctx context.Context, oldMessageID, newMessageID, newSessionID string) error {
	query := `
		SELECT id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at
		FROM tool_calls WHERE message_id = ?
	`
	rows, err := r.db.QueryContext(ctx, query, oldMessageID)
	if err != nil {
		return err
	}
	defer rows.Close()

	toolCalls, err := scanToolCalls(rows)
	if err != nil {
		return err
	}

	for _, tc := range toolCalls {
		insertQuery := `
			INSERT INTO tool_calls (id, message_id, session_id, tool_name, tool_input, tool_output, status,
				approval_status, approved_by, approved_at, started_at, completed_at, error, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`
		_, err := r.db.ExecContext(ctx, insertQuery,
			uuid.New().String(),
			newMessageID,
			newSessionID,
			tc.ToolName,
			tc.ToolInput,
			nullStringPtr(tc.ToolOutput),
			tc.Status.String(),
			nullStringPtr(tc.ApprovalStatus),
			nullStringPtr(tc.ApprovedBy),
			nullTimePtr(tc.ApprovedAt),
			nullTimePtr(tc.StartedAt),
			nullTimePtr(tc.CompletedAt),
			nullStringPtr(tc.Error),
			tc.CreatedAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// copySummariesToFork copies session summaries from parent to fork.
func (r *SQLiteRepository) copySummariesToFork(ctx context.Context, parentSessionID, forkSessionID string) error {
	query := `
		SELECT id, session_id, summary_text, messages_summarized, tokens_saved, created_at
		FROM session_summaries WHERE session_id = ?
	`
	rows, err := r.db.QueryContext(ctx, query, parentSessionID)
	if err != nil {
		return err
	}
	defer rows.Close()

	summaries, err := scanSummaries(rows)
	if err != nil {
		return err
	}

	for _, summary := range summaries {
		insertQuery := `
			INSERT INTO session_summaries (id, session_id, summary_text, messages_summarized, tokens_saved, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`
		_, err := r.db.ExecContext(ctx, insertQuery,
			uuid.New().String(),
			forkSessionID,
			summary.SummaryText,
			summary.MessagesSummarized,
			summary.TokensSaved,
			summary.CreatedAt.UTC().Format(time.RFC3339),
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetForkInfo returns detailed fork information for a session.
func (r *SQLiteRepository) GetForkInfo(ctx context.Context, sessionID string) (*ForkInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the session
	query := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, sessionID)
	session, err := scanSession(row)
	if err != nil {
		return nil, err
	}

	info := &ForkInfo{
		SessionID:          sessionID,
		ForkCreatedAt:      session.CreatedAt,
		AncestorChain:      []string{},
	}

	// Build ancestor chain and find root
	currentID := sessionID
	chain := []string{sessionID}

	for {
		var parentID sql.NullString
		var forkPointID sql.NullString
		ancestorQuery := `SELECT parent_session_id, fork_point_message_id FROM sessions WHERE id = ?`
		if err := r.db.QueryRowContext(ctx, ancestorQuery, currentID).Scan(&parentID, &forkPointID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				break
			}
			return nil, fmt.Errorf("failed to get ancestor: %w", err)
		}

		if !parentID.Valid || parentID.String == "" {
			// This is the root
			break
		}

		if currentID == sessionID {
			info.ParentSessionID = parentID.String
			info.ForkPointMessageID = forkPointID.String
		}

		chain = append([]string{parentID.String}, chain...)
		currentID = parentID.String
	}

	info.RootSessionID = chain[0]
	info.AncestorChain = chain
	info.ForkDepth = len(chain) - 1

	// Count direct children
	childQuery := `SELECT COUNT(*) FROM sessions WHERE parent_session_id = ?`
	if err := r.db.QueryRowContext(ctx, childQuery, sessionID).Scan(&info.ChildCount); err != nil {
		return nil, fmt.Errorf("failed to count children: %w", err)
	}

	// Count all descendants (recursive)
	descendants, err := r.countDescendantsInternal(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count descendants: %w", err)
	}
	info.TotalDescendants = descendants

	return info, nil
}

// countDescendantsInternal recursively counts all descendants of a session.
func (r *SQLiteRepository) countDescendantsInternal(ctx context.Context, sessionID string) (int, error) {
	// Get direct children
	query := `SELECT id FROM sessions WHERE parent_session_id = ?`
	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var childIDs []string
	for rows.Next() {
		var childID string
		if err := rows.Scan(&childID); err != nil {
			return 0, err
		}
		childIDs = append(childIDs, childID)
	}

	total := len(childIDs)
	for _, childID := range childIDs {
		subCount, err := r.countDescendantsInternal(ctx, childID)
		if err != nil {
			return 0, err
		}
		total += subCount
	}

	return total, nil
}

// GetAncestorChain returns the complete chain of parent sessions from root to the given session.
func (r *SQLiteRepository) GetAncestorChain(ctx context.Context, sessionID string) ([]*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var chain []*Session

	currentID := sessionID
	for {
		query := `
			SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
			FROM sessions WHERE id = ?
		`
		row := r.db.QueryRowContext(ctx, query, currentID)
		session, err := scanSession(row)
		if err != nil {
			if errors.Is(err, ErrSessionNotFound) && len(chain) > 0 {
				break
			}
			return nil, err
		}

		chain = append([]*Session{session}, chain...)

		if !session.ParentSessionID.Valid || session.ParentSessionID.String == "" {
			break
		}
		currentID = session.ParentSessionID.String
	}

	return chain, nil
}

// GetRootSession returns the original root session for any session in a fork tree.
func (r *SQLiteRepository) GetRootSession(ctx context.Context, sessionID string) (*Session, error) {
	chain, err := r.GetAncestorChain(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, ErrSessionNotFound
	}
	return chain[0], nil
}

// ListDescendants returns all descendant sessions of a session.
func (r *SQLiteRepository) ListDescendants(ctx context.Context, sessionID string) ([]*Session, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.listDescendantsInternal(ctx, sessionID)
}

// listDescendantsInternal recursively collects all descendants.
func (r *SQLiteRepository) listDescendantsInternal(ctx context.Context, sessionID string) ([]*Session, error) {
	query := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE parent_session_id = ? ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list descendants: %w", err)
	}
	defer rows.Close()

	directChildren, err := scanSessions(rows)
	if err != nil {
		return nil, err
	}

	var allDescendants []*Session
	for _, child := range directChildren {
		allDescendants = append(allDescendants, child)
		subDescendants, err := r.listDescendantsInternal(ctx, child.ID)
		if err != nil {
			return nil, err
		}
		allDescendants = append(allDescendants, subDescendants...)
	}

	return allDescendants, nil
}

// IsDescendantOf checks if childSessionID is a descendant of ancestorSessionID.
func (r *SQLiteRepository) IsDescendantOf(ctx context.Context, childSessionID, ancestorSessionID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if childSessionID == ancestorSessionID {
		return false, nil
	}

	currentID := childSessionID
	for {
		var parentID sql.NullString
		query := `SELECT parent_session_id FROM sessions WHERE id = ?`
		err := r.db.QueryRowContext(ctx, query, currentID).Scan(&parentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return false, nil
			}
			return false, fmt.Errorf("failed to check ancestry: %w", err)
		}

		if !parentID.Valid || parentID.String == "" {
			return false, nil
		}

		if parentID.String == ancestorSessionID {
			return true, nil
		}

		currentID = parentID.String
	}
}

// Message operations

// CreateMessage stores a new message for a session.
func (r *SQLiteRepository) CreateMessage(ctx context.Context, message *Message) error {
	if err := message.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if session exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`
	if err := r.db.QueryRowContext(ctx, checkQuery, message.SessionID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return ErrSessionNotFound
	}

	query := `
		INSERT INTO messages (id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		message.ID,
		message.SessionID,
		message.Role.String(),
		message.Content,
		message.TokensInput,
		message.TokensOutput,
		message.CostUSD,
		message.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create message: %w", err)
	}

	// Update session's updated_at
	updateQuery := `UPDATE sessions SET updated_at = ? WHERE id = ?`
	_, _ = r.db.ExecContext(ctx, updateQuery, time.Now().UTC().Format(time.RFC3339), message.SessionID)

	return nil
}

// GetMessage retrieves a message by its ID.
func (r *SQLiteRepository) GetMessage(ctx context.Context, id string) (*Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanMessage(row)
}

// UpdateMessage modifies an existing message.
func (r *SQLiteRepository) UpdateMessage(ctx context.Context, message *Message) error {
	if err := message.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if message exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM messages WHERE id = ?)`
	if err := r.db.QueryRowContext(ctx, checkQuery, message.ID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check message existence: %w", err)
	}
	if !exists {
		return ErrMessageNotFound
	}

	query := `
		UPDATE messages
		SET role = ?, content = ?, tokens_input = ?, tokens_output = ?, cost_usd = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query,
		message.Role.String(),
		message.Content,
		message.TokensInput,
		message.TokensOutput,
		message.CostUSD,
		message.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update message: %w", err)
	}

	return nil
}

// DeleteMessage removes a message by its ID.
func (r *SQLiteRepository) DeleteMessage(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `DELETE FROM messages WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete message: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrMessageNotFound
	}

	return nil
}

// ListMessages returns all messages for a session.
func (r *SQLiteRepository) ListMessages(ctx context.Context, sessionID string) ([]*Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages WHERE session_id = ? ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// ListMessagesByRole returns messages for a session filtered by role.
func (r *SQLiteRepository) ListMessagesByRole(ctx context.Context, sessionID string, role Role) ([]*Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages WHERE session_id = ? AND role = ? ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID, role.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list messages by role: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetMessageCount returns the total number of messages in a session.
func (r *SQLiteRepository) GetMessageCount(ctx context.Context, sessionID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT COUNT(*) FROM messages WHERE session_id = ?`
	var count int
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get message count: %w", err)
	}

	return count, nil
}

// ListMessagesUpTo returns messages for a session up to and including the specified message.
func (r *SQLiteRepository) ListMessagesUpTo(ctx context.Context, sessionID, upToMessageID string) ([]*Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get the target message's created_at timestamp
	var targetCreatedAt string
	timestampQuery := `SELECT created_at FROM messages WHERE id = ? AND session_id = ?`
	err := r.db.QueryRowContext(ctx, timestampQuery, upToMessageID, sessionID).Scan(&targetCreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to get message timestamp: %w", err)
	}

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages
		WHERE session_id = ? AND created_at <= ?
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID, targetCreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages up to: %w", err)
	}
	defer rows.Close()

	return scanMessages(rows)
}

// GetFullConversationHistory returns the complete conversation history for a forked session.
func (r *SQLiteRepository) GetFullConversationHistory(ctx context.Context, sessionID string) ([]*Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Get session to check if it's a fork
	query := `
		SELECT id, project_path, provider, model, title, parent_session_id, fork_point_message_id, status, created_at, updated_at
		FROM sessions WHERE id = ?
	`
	row := r.db.QueryRowContext(ctx, query, sessionID)
	session, err := scanSession(row)
	if err != nil {
		return nil, err
	}

	// If not a fork, just return the session's messages
	if !session.IsFork() {
		msgQuery := `
			SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
			FROM messages WHERE session_id = ? ORDER BY created_at ASC
		`
		rows, err := r.db.QueryContext(ctx, msgQuery, sessionID)
		if err != nil {
			return nil, fmt.Errorf("failed to list messages: %w", err)
		}
		defer rows.Close()
		return scanMessages(rows)
	}

	// Build the full history by traversing the ancestor chain
	var allMessages []*Message

	// First, collect messages from ancestors up to their respective fork points
	currentSessionID := sessionID
	var sessionsToProcess []struct {
		sessionID      string
		forkPointMsgID string
		isLeaf         bool
	}

	// Build the chain from current session back to root
	for {
		var parentID, forkPointID sql.NullString
		chainQuery := `SELECT parent_session_id, fork_point_message_id FROM sessions WHERE id = ?`
		err := r.db.QueryRowContext(ctx, chainQuery, currentSessionID).Scan(&parentID, &forkPointID)
		if err != nil {
			return nil, fmt.Errorf("failed to get session chain: %w", err)
		}

		if !parentID.Valid || parentID.String == "" {
			// This is the root or a non-fork session
			sessionsToProcess = append([]struct {
				sessionID      string
				forkPointMsgID string
				isLeaf         bool
			}{{sessionID: currentSessionID, forkPointMsgID: "", isLeaf: currentSessionID == sessionID}}, sessionsToProcess...)
			break
		}

		sessionsToProcess = append([]struct {
			sessionID      string
			forkPointMsgID string
			isLeaf         bool
		}{{sessionID: currentSessionID, forkPointMsgID: forkPointID.String, isLeaf: currentSessionID == sessionID}}, sessionsToProcess...)

		currentSessionID = parentID.String
	}

	// Now collect messages from each session in order
	for i, s := range sessionsToProcess {
		var msgs []*Message
		var queryErr error

		if i == 0 {
			// For the root or first session in chain, get messages up to the fork point of the next session
			if len(sessionsToProcess) > 1 && sessionsToProcess[1].forkPointMsgID != "" {
				// Get messages up to the fork point
				msgs, queryErr = r.listMessagesUpToInternal(ctx, s.sessionID, sessionsToProcess[1].forkPointMsgID)
			} else {
				// Get all messages
				msgQuery := `
					SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
					FROM messages WHERE session_id = ? ORDER BY created_at ASC
				`
				rows, err := r.db.QueryContext(ctx, msgQuery, s.sessionID)
				if err != nil {
					return nil, fmt.Errorf("failed to list messages: %w", err)
				}
				msgs, queryErr = scanMessages(rows)
				rows.Close()
			}
		} else if s.isLeaf {
			// For the leaf session (the one we're querying), get all its own messages
			msgQuery := `
				SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
				FROM messages WHERE session_id = ? ORDER BY created_at ASC
			`
			rows, err := r.db.QueryContext(ctx, msgQuery, s.sessionID)
			if err != nil {
				return nil, fmt.Errorf("failed to list messages: %w", err)
			}
			msgs, queryErr = scanMessages(rows)
			rows.Close()
		} else {
			// For middle sessions, get messages after their fork point up to the next fork point
			if i+1 < len(sessionsToProcess) && sessionsToProcess[i+1].forkPointMsgID != "" {
				msgs, queryErr = r.listMessagesBetweenInternal(ctx, s.sessionID, s.forkPointMsgID, sessionsToProcess[i+1].forkPointMsgID)
			} else {
				msgs, queryErr = r.listMessagesAfterInternal(ctx, s.sessionID, s.forkPointMsgID)
			}
		}

		if queryErr != nil {
			return nil, queryErr
		}
		allMessages = append(allMessages, msgs...)
	}

	return allMessages, nil
}

// listMessagesUpToInternal gets messages up to a specific message (internal, no lock).
func (r *SQLiteRepository) listMessagesUpToInternal(ctx context.Context, sessionID, upToMessageID string) ([]*Message, error) {
	var targetCreatedAt string
	timestampQuery := `SELECT created_at FROM messages WHERE id = ?`
	err := r.db.QueryRowContext(ctx, timestampQuery, upToMessageID).Scan(&targetCreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []*Message{}, nil
		}
		return nil, fmt.Errorf("failed to get message timestamp: %w", err)
	}

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages
		WHERE session_id = ? AND created_at <= ?
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, sessionID, targetCreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages up to: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// listMessagesAfterInternal gets messages after a specific message (internal, no lock).
func (r *SQLiteRepository) listMessagesAfterInternal(ctx context.Context, sessionID, afterMessageID string) ([]*Message, error) {
	var afterCreatedAt string
	timestampQuery := `SELECT created_at FROM messages WHERE id = ?`
	err := r.db.QueryRowContext(ctx, timestampQuery, afterMessageID).Scan(&afterCreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// If the message doesn't exist, return all messages
			query := `
				SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
				FROM messages WHERE session_id = ? ORDER BY created_at ASC
			`
			rows, err := r.db.QueryContext(ctx, query, sessionID)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			return scanMessages(rows)
		}
		return nil, fmt.Errorf("failed to get message timestamp: %w", err)
	}

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages
		WHERE session_id = ? AND created_at > ?
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, sessionID, afterCreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages after: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// listMessagesBetweenInternal gets messages between two specific messages (internal, no lock).
func (r *SQLiteRepository) listMessagesBetweenInternal(ctx context.Context, sessionID, afterMessageID, upToMessageID string) ([]*Message, error) {
	var afterCreatedAt, upToCreatedAt string

	timestampQuery := `SELECT created_at FROM messages WHERE id = ?`
	if err := r.db.QueryRowContext(ctx, timestampQuery, afterMessageID).Scan(&afterCreatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		afterCreatedAt = ""
	}
	if err := r.db.QueryRowContext(ctx, timestampQuery, upToMessageID).Scan(&upToCreatedAt); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return r.listMessagesAfterInternal(ctx, sessionID, afterMessageID)
	}

	query := `
		SELECT id, session_id, role, content, tokens_input, tokens_output, cost_usd, created_at
		FROM messages
		WHERE session_id = ? AND created_at > ? AND created_at <= ?
		ORDER BY created_at ASC
	`
	rows, err := r.db.QueryContext(ctx, query, sessionID, afterCreatedAt, upToCreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to list messages between: %w", err)
	}
	defer rows.Close()
	return scanMessages(rows)
}

// Tool call operations

// CreateToolCall stores a new tool call for a message.
func (r *SQLiteRepository) CreateToolCall(ctx context.Context, toolCall *ToolCall) error {
	if err := toolCall.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if message exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM messages WHERE id = ?)`
	if err := r.db.QueryRowContext(ctx, checkQuery, toolCall.MessageID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check message existence: %w", err)
	}
	if !exists {
		return ErrMessageNotFound
	}

	query := `
		INSERT INTO tool_calls (id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		toolCall.ID,
		toolCall.MessageID,
		toolCall.SessionID,
		toolCall.ToolName,
		toolCall.ToolInput,
		nullStringPtr(toolCall.ToolOutput),
		toolCall.Status.String(),
		nullStringPtr(toolCall.ApprovalStatus),
		nullStringPtr(toolCall.ApprovedBy),
		nullTimePtr(toolCall.ApprovedAt),
		nullTimePtr(toolCall.StartedAt),
		nullTimePtr(toolCall.CompletedAt),
		nullStringPtr(toolCall.Error),
		toolCall.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create tool call: %w", err)
	}

	return nil
}

// GetToolCall retrieves a tool call by its ID.
func (r *SQLiteRepository) GetToolCall(ctx context.Context, id string) (*ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at
		FROM tool_calls WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanToolCall(row)
}

// UpdateToolCall modifies an existing tool call.
func (r *SQLiteRepository) UpdateToolCall(ctx context.Context, toolCall *ToolCall) error {
	if err := toolCall.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if tool call exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM tool_calls WHERE id = ?)`
	if err := r.db.QueryRowContext(ctx, checkQuery, toolCall.ID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check tool call existence: %w", err)
	}
	if !exists {
		return ErrToolCallNotFound
	}

	query := `
		UPDATE tool_calls
		SET tool_input = ?, tool_output = ?, status = ?, approval_status = ?,
			approved_by = ?, approved_at = ?, started_at = ?, completed_at = ?, error = ?
		WHERE id = ?
	`

	_, err := r.db.ExecContext(ctx, query,
		toolCall.ToolInput,
		nullStringPtr(toolCall.ToolOutput),
		toolCall.Status.String(),
		nullStringPtr(toolCall.ApprovalStatus),
		nullStringPtr(toolCall.ApprovedBy),
		nullTimePtr(toolCall.ApprovedAt),
		nullTimePtr(toolCall.StartedAt),
		nullTimePtr(toolCall.CompletedAt),
		nullStringPtr(toolCall.Error),
		toolCall.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update tool call: %w", err)
	}

	return nil
}

// DeleteToolCall removes a tool call by its ID.
func (r *SQLiteRepository) DeleteToolCall(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `DELETE FROM tool_calls WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tool call: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrToolCallNotFound
	}

	return nil
}

// ListToolCalls returns all tool calls for a session.
func (r *SQLiteRepository) ListToolCalls(ctx context.Context, sessionID string) ([]*ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at
		FROM tool_calls WHERE session_id = ? ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tool calls: %w", err)
	}
	defer rows.Close()

	return scanToolCalls(rows)
}

// ListToolCallsByMessage returns all tool calls for a specific message.
func (r *SQLiteRepository) ListToolCallsByMessage(ctx context.Context, messageID string) ([]*ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at
		FROM tool_calls WHERE message_id = ? ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tool calls by message: %w", err)
	}
	defer rows.Close()

	return scanToolCalls(rows)
}

// ListToolCallsByStatus returns tool calls filtered by status.
func (r *SQLiteRepository) ListToolCallsByStatus(ctx context.Context, sessionID string, status ToolCallStatus) ([]*ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at
		FROM tool_calls WHERE session_id = ? AND status = ? ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID, status.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list tool calls by status: %w", err)
	}
	defer rows.Close()

	return scanToolCalls(rows)
}

// ListPendingApprovals returns tool calls awaiting approval.
func (r *SQLiteRepository) ListPendingApprovals(ctx context.Context, sessionID string) ([]*ToolCall, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, message_id, session_id, tool_name, tool_input, tool_output, status,
			approval_status, approved_by, approved_at, started_at, completed_at, error, created_at
		FROM tool_calls
		WHERE session_id = ? AND (approval_status IS NULL OR approval_status = 'pending')
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending approvals: %w", err)
	}
	defer rows.Close()

	return scanToolCalls(rows)
}

// Session summary operations

// CreateSummary stores a new session summary.
func (r *SQLiteRepository) CreateSummary(ctx context.Context, summary *SessionSummary) error {
	if err := summary.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if session exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM sessions WHERE id = ?)`
	if err := r.db.QueryRowContext(ctx, checkQuery, summary.SessionID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check session existence: %w", err)
	}
	if !exists {
		return ErrSessionNotFound
	}

	query := `
		INSERT INTO session_summaries (id, session_id, summary_text, messages_summarized, tokens_saved, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		summary.ID,
		summary.SessionID,
		summary.SummaryText,
		summary.MessagesSummarized,
		summary.TokensSaved,
		summary.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create summary: %w", err)
	}

	return nil
}

// GetSummary retrieves a summary by its ID.
func (r *SQLiteRepository) GetSummary(ctx context.Context, id string) (*SessionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, summary_text, messages_summarized, tokens_saved, created_at
		FROM session_summaries WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanSummary(row)
}

// ListSummaries returns all summaries for a session.
func (r *SQLiteRepository) ListSummaries(ctx context.Context, sessionID string) ([]*SessionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, summary_text, messages_summarized, tokens_saved, created_at
		FROM session_summaries WHERE session_id = ? ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list summaries: %w", err)
	}
	defer rows.Close()

	return scanSummaries(rows)
}

// GetLatestSummary returns the most recent summary for a session.
func (r *SQLiteRepository) GetLatestSummary(ctx context.Context, sessionID string) (*SessionSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, summary_text, messages_summarized, tokens_saved, created_at
		FROM session_summaries WHERE session_id = ? ORDER BY created_at DESC LIMIT 1
	`

	row := r.db.QueryRowContext(ctx, query, sessionID)
	summary, err := scanSummary(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return summary, err
}

// DeleteSummary removes a summary by its ID.
func (r *SQLiteRepository) DeleteSummary(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `DELETE FROM session_summaries WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete summary: %w", err)
	}

	return nil
}

// Aggregation operations

// GetSessionStats returns aggregate statistics for a session.
func (r *SQLiteRepository) GetSessionStats(ctx context.Context, sessionID string) (*SessionStats, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &SessionStats{SessionID: sessionID}

	// Get message stats
	msgQuery := `
		SELECT COUNT(*), COALESCE(SUM(tokens_input), 0), COALESCE(SUM(tokens_output), 0), COALESCE(SUM(cost_usd), 0)
		FROM messages WHERE session_id = ?
	`
	err := r.db.QueryRowContext(ctx, msgQuery, sessionID).Scan(
		&stats.MessageCount,
		&stats.TotalTokensInput,
		&stats.TotalTokensOutput,
		&stats.TotalCostUSD,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get message stats: %w", err)
	}

	// Get tool call count
	tcQuery := `SELECT COUNT(*) FROM tool_calls WHERE session_id = ?`
	err = r.db.QueryRowContext(ctx, tcQuery, sessionID).Scan(&stats.ToolCallCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get tool call count: %w", err)
	}

	// Get summary stats
	sumQuery := `SELECT COUNT(*), COALESCE(SUM(tokens_saved), 0) FROM session_summaries WHERE session_id = ?`
	err = r.db.QueryRowContext(ctx, sumQuery, sessionID).Scan(&stats.SummaryCount, &stats.TokensSaved)
	if err != nil {
		return nil, fmt.Errorf("failed to get summary stats: %w", err)
	}

	return stats, nil
}

// GetUsageByProvider returns usage statistics grouped by provider.
func (r *SQLiteRepository) GetUsageByProvider(ctx context.Context) ([]*ProviderUsage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT
			s.provider,
			s.model,
			COUNT(DISTINCT s.id) as session_count,
			(SELECT COUNT(*) FROM messages m WHERE m.session_id IN (SELECT id FROM sessions WHERE provider = s.provider AND model = s.model)) as message_count,
			(SELECT COALESCE(SUM(tokens_input), 0) FROM messages m WHERE m.session_id IN (SELECT id FROM sessions WHERE provider = s.provider AND model = s.model)) as total_tokens_input,
			(SELECT COALESCE(SUM(tokens_output), 0) FROM messages m WHERE m.session_id IN (SELECT id FROM sessions WHERE provider = s.provider AND model = s.model)) as total_tokens_output,
			(SELECT COALESCE(SUM(cost_usd), 0) FROM messages m WHERE m.session_id IN (SELECT id FROM sessions WHERE provider = s.provider AND model = s.model)) as total_cost_usd
		FROM sessions s
		GROUP BY s.provider, s.model
		ORDER BY total_cost_usd DESC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get usage by provider: %w", err)
	}
	defer rows.Close()

	var usages []*ProviderUsage
	for rows.Next() {
		u := &ProviderUsage{}
		if err := rows.Scan(
			&u.Provider,
			&u.Model,
			&u.SessionCount,
			&u.MessageCount,
			&u.TotalTokensInput,
			&u.TotalTokensOutput,
			&u.TotalCostUSD,
		); err != nil {
			return nil, fmt.Errorf("failed to scan usage: %w", err)
		}
		usages = append(usages, u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating usage: %w", err)
	}

	if usages == nil {
		usages = []*ProviderUsage{}
	}

	return usages, nil
}

// Helper functions

func scanSession(row *sql.Row) (*Session, error) {
	session := &Session{}
	var statusStr string
	var createdAt, updatedAt string

	err := row.Scan(
		&session.ID,
		&session.ProjectPath,
		&session.Provider,
		&session.Model,
		&session.Title,
		&session.ParentSessionID,
		&session.ForkPointMessageID,
		&statusStr,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSessionNotFound
		}
		return nil, fmt.Errorf("failed to scan session: %w", err)
	}

	session.Status = Status(statusStr)
	session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return session, nil
}

func scanSessions(rows *sql.Rows) ([]*Session, error) {
	var sessions []*Session
	for rows.Next() {
		session := &Session{}
		var statusStr string
		var createdAt, updatedAt string

		if err := rows.Scan(
			&session.ID,
			&session.ProjectPath,
			&session.Provider,
			&session.Model,
			&session.Title,
			&session.ParentSessionID,
			&session.ForkPointMessageID,
			&statusStr,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan session: %w", err)
		}

		session.Status = Status(statusStr)
		session.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		session.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		sessions = append(sessions, session)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating sessions: %w", err)
	}

	if sessions == nil {
		sessions = []*Session{}
	}

	return sessions, nil
}

func scanMessage(row *sql.Row) (*Message, error) {
	message := &Message{}
	var roleStr string
	var createdAt string

	err := row.Scan(
		&message.ID,
		&message.SessionID,
		&roleStr,
		&message.Content,
		&message.TokensInput,
		&message.TokensOutput,
		&message.CostUSD,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMessageNotFound
		}
		return nil, fmt.Errorf("failed to scan message: %w", err)
	}

	message.Role = Role(roleStr)
	message.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return message, nil
}

func scanMessages(rows *sql.Rows) ([]*Message, error) {
	var messages []*Message
	for rows.Next() {
		message := &Message{}
		var roleStr string
		var createdAt string

		if err := rows.Scan(
			&message.ID,
			&message.SessionID,
			&roleStr,
			&message.Content,
			&message.TokensInput,
			&message.TokensOutput,
			&message.CostUSD,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan message: %w", err)
		}

		message.Role = Role(roleStr)
		message.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating messages: %w", err)
	}

	if messages == nil {
		messages = []*Message{}
	}

	return messages, nil
}

func scanToolCall(row *sql.Row) (*ToolCall, error) {
	tc := &ToolCall{}
	var statusStr string
	var approvedAt, startedAt, completedAt, createdAt sql.NullString

	err := row.Scan(
		&tc.ID,
		&tc.MessageID,
		&tc.SessionID,
		&tc.ToolName,
		&tc.ToolInput,
		&tc.ToolOutput,
		&statusStr,
		&tc.ApprovalStatus,
		&tc.ApprovedBy,
		&approvedAt,
		&startedAt,
		&completedAt,
		&tc.Error,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrToolCallNotFound
		}
		return nil, fmt.Errorf("failed to scan tool call: %w", err)
	}

	tc.Status = ToolCallStatus(statusStr)
	if approvedAt.Valid {
		t, _ := time.Parse(time.RFC3339, approvedAt.String)
		tc.ApprovedAt = sql.NullTime{Time: t, Valid: true}
	}
	if startedAt.Valid {
		t, _ := time.Parse(time.RFC3339, startedAt.String)
		tc.StartedAt = sql.NullTime{Time: t, Valid: true}
	}
	if completedAt.Valid {
		t, _ := time.Parse(time.RFC3339, completedAt.String)
		tc.CompletedAt = sql.NullTime{Time: t, Valid: true}
	}
	if createdAt.Valid {
		tc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}

	return tc, nil
}

func scanToolCalls(rows *sql.Rows) ([]*ToolCall, error) {
	var toolCalls []*ToolCall
	for rows.Next() {
		tc := &ToolCall{}
		var statusStr string
		var approvedAt, startedAt, completedAt, createdAt sql.NullString

		if err := rows.Scan(
			&tc.ID,
			&tc.MessageID,
			&tc.SessionID,
			&tc.ToolName,
			&tc.ToolInput,
			&tc.ToolOutput,
			&statusStr,
			&tc.ApprovalStatus,
			&tc.ApprovedBy,
			&approvedAt,
			&startedAt,
			&completedAt,
			&tc.Error,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan tool call: %w", err)
		}

		tc.Status = ToolCallStatus(statusStr)
		if approvedAt.Valid {
			t, _ := time.Parse(time.RFC3339, approvedAt.String)
			tc.ApprovedAt = sql.NullTime{Time: t, Valid: true}
		}
		if startedAt.Valid {
			t, _ := time.Parse(time.RFC3339, startedAt.String)
			tc.StartedAt = sql.NullTime{Time: t, Valid: true}
		}
		if completedAt.Valid {
			t, _ := time.Parse(time.RFC3339, completedAt.String)
			tc.CompletedAt = sql.NullTime{Time: t, Valid: true}
		}
		if createdAt.Valid {
			tc.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
		}
		toolCalls = append(toolCalls, tc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tool calls: %w", err)
	}

	if toolCalls == nil {
		toolCalls = []*ToolCall{}
	}

	return toolCalls, nil
}

func scanSummary(row *sql.Row) (*SessionSummary, error) {
	summary := &SessionSummary{}
	var createdAt string

	err := row.Scan(
		&summary.ID,
		&summary.SessionID,
		&summary.SummaryText,
		&summary.MessagesSummarized,
		&summary.TokensSaved,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("failed to scan summary: %w", err)
	}

	summary.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return summary, nil
}

func scanSummaries(rows *sql.Rows) ([]*SessionSummary, error) {
	var summaries []*SessionSummary
	for rows.Next() {
		summary := &SessionSummary{}
		var createdAt string

		if err := rows.Scan(
			&summary.ID,
			&summary.SessionID,
			&summary.SummaryText,
			&summary.MessagesSummarized,
			&summary.TokensSaved,
			&createdAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan summary: %w", err)
		}

		summary.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		summaries = append(summaries, summary)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating summaries: %w", err)
	}

	if summaries == nil {
		summaries = []*SessionSummary{}
	}

	return summaries, nil
}

func nullStringPtr(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullTimePtr(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time.UTC().Format(time.RFC3339)
	}
	return nil
}

// isUniqueViolation checks if the error is a SQLite unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return containsString(errStr, "UNIQUE constraint failed")
}

// containsString checks if s contains substr.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

// searchSubstring performs a simple substring search.
func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure SQLiteRepository implements Repository interface.
var _ Repository = (*SQLiteRepository)(nil)
