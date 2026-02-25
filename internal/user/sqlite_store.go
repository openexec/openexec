package user

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
)

// SQLiteStore is a SQLite-based implementation of Store for persistence.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex // protects concurrent access
}

// NewSQLiteStore creates a new SQLiteStore with the given database connection.
// The database connection should already be opened and configured.
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, errors.New("database connection is required")
	}

	store := &SQLiteStore{db: db}

	// Initialize the schema
	if err := store.initSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the users table if it doesn't exist.
func (s *SQLiteStore) initSchema(ctx context.Context) error {
	query := `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			telegram_id INTEGER NOT NULL UNIQUE,
			role TEXT NOT NULL,
			current_project_id TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
	`

	_, err := s.db.ExecContext(ctx, query)
	return err
}

// Create stores a new user in the database.
func (s *SQLiteStore) Create(ctx context.Context, user *User) error {
	if err := user.Validate(); err != nil {
		return ErrInvalidUser
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT INTO users (id, telegram_id, role, current_project_id) VALUES (?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query, user.ID, user.TelegramID, string(user.Role), user.CurrentProjectID)
	if err != nil {
		// Check for unique constraint violation
		if isUniqueViolation(err) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to create user: %w", err)
	}

	return nil
}

// GetByID retrieves a user by their ID.
func (s *SQLiteStore) GetByID(ctx context.Context, id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, telegram_id, role, current_project_id FROM users WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, id)

	user := &User{}
	var roleStr string
	err := row.Scan(&user.ID, &user.TelegramID, &roleStr, &user.CurrentProjectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by ID: %w", err)
	}

	user.Role = Role(roleStr)
	return user, nil
}

// GetByTelegramID retrieves a user by their Telegram ID.
func (s *SQLiteStore) GetByTelegramID(ctx context.Context, telegramID int64) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, telegram_id, role, current_project_id FROM users WHERE telegram_id = ?`
	row := s.db.QueryRowContext(ctx, query, telegramID)

	user := &User{}
	var roleStr string
	err := row.Scan(&user.ID, &user.TelegramID, &roleStr, &user.CurrentProjectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to get user by Telegram ID: %w", err)
	}

	user.Role = Role(roleStr)
	return user, nil
}

// Update modifies an existing user.
func (s *SQLiteStore) Update(ctx context.Context, user *User) error {
	if err := user.Validate(); err != nil {
		return ErrInvalidUser
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// First check if the user exists
	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM users WHERE id = ?)`
	if err := s.db.QueryRowContext(ctx, checkQuery, user.ID).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check user existence: %w", err)
	}
	if !exists {
		return ErrUserNotFound
	}

	// Check if the new TelegramID is taken by another user
	var conflictID string
	conflictQuery := `SELECT id FROM users WHERE telegram_id = ? AND id != ?`
	err := s.db.QueryRowContext(ctx, conflictQuery, user.TelegramID, user.ID).Scan(&conflictID)
	if err == nil {
		// Found a conflicting user
		return ErrUserAlreadyExists
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to check telegram_id conflict: %w", err)
	}

	query := `UPDATE users SET telegram_id = ?, role = ?, current_project_id = ? WHERE id = ?`
	_, err = s.db.ExecContext(ctx, query, user.TelegramID, string(user.Role), user.CurrentProjectID, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrUserAlreadyExists
		}
		return fmt.Errorf("failed to update user: %w", err)
	}

	return nil
}

// Delete removes a user by their ID.
func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `DELETE FROM users WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrUserNotFound
	}

	return nil
}

// List returns all users.
func (s *SQLiteStore) List(ctx context.Context) ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `SELECT id, telegram_id, role, current_project_id FROM users`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list users: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var users []*User
	for rows.Next() {
		user := &User{}
		var roleStr string
		if err := rows.Scan(&user.ID, &user.TelegramID, &roleStr, &user.CurrentProjectID); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		user.Role = Role(roleStr)
		users = append(users, user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	// Return empty slice instead of nil for consistency
	if users == nil {
		users = []*User{}
	}

	return users, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// isUniqueViolation checks if the error is a SQLite unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// SQLite unique constraint error contains "UNIQUE constraint failed"
	return containsString(err.Error(), "UNIQUE constraint failed")
}

// containsString checks if s contains substr (case-sensitive).
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

// Ensure SQLiteStore implements Store interface.
var _ Store = (*SQLiteStore)(nil)
