package state

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

// RunSpec represents a deterministic specification for a run.
// RunSpecs are immutable once created and serve as the canonical
// record of what inputs were used to produce a run's outputs.
type RunSpec struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Intent      string    `json:"intent"`       // The user's original intent/prompt
	ContextHash string    `json:"context_hash"` // Hash of context files used
	PromptHash  string    `json:"prompt_hash"`  // Hash of the composed prompt
	Model       string    `json:"model"`        // Model identifier
	Mode        string    `json:"mode"`         // Execution mode
	CreatedAt   time.Time `json:"created_at"`
}

// CreateRunSpec creates a new run specification and returns its ID.
// RunSpecs are immutable - once created, they cannot be modified.
func (s *Store) CreateRunSpec(ctx context.Context, spec *RunSpec) error {
	if spec.ID == "" {
		spec.ID = uuid.New().String()
	}
	if spec.CreatedAt.IsZero() {
		spec.CreatedAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO run_specs (id, session_id, intent, context_hash, prompt_hash, model, mode, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, spec.ID, spec.SessionID, spec.Intent, spec.ContextHash, spec.PromptHash, spec.Model, spec.Mode, spec.CreatedAt)
	return err
}

// GetRunSpec retrieves a run specification by ID.
func (s *Store) GetRunSpec(ctx context.Context, id string) (*RunSpec, error) {
	var spec RunSpec
	err := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, intent, context_hash, prompt_hash, model, mode, created_at
		FROM run_specs WHERE id = ?
	`, id).Scan(&spec.ID, &spec.SessionID, &spec.Intent, &spec.ContextHash, &spec.PromptHash, &spec.Model, &spec.Mode, &spec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

// GetRunSpecByContextHash finds a run spec by its context hash.
// This enables cache hits when the same context is used again.
func (s *Store) GetRunSpecByContextHash(ctx context.Context, contextHash string) (*RunSpec, error) {
	var spec RunSpec
	err := s.db.QueryRowContext(ctx, `
		SELECT id, session_id, intent, context_hash, prompt_hash, model, mode, created_at
		FROM run_specs WHERE context_hash = ? ORDER BY created_at DESC LIMIT 1
	`, contextHash).Scan(&spec.ID, &spec.SessionID, &spec.Intent, &spec.ContextHash, &spec.PromptHash, &spec.Model, &spec.Mode, &spec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &spec, nil
}

// ListRunSpecsBySession retrieves all run specs for a session.
func (s *Store) ListRunSpecsBySession(ctx context.Context, sessionID string, limit, offset int) ([]*RunSpec, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, session_id, intent, context_hash, prompt_hash, model, mode, created_at
		FROM run_specs WHERE session_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?
	`, sessionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var specs []*RunSpec
	for rows.Next() {
		var spec RunSpec
		if err := rows.Scan(&spec.ID, &spec.SessionID, &spec.Intent, &spec.ContextHash, &spec.PromptHash, &spec.Model, &spec.Mode, &spec.CreatedAt); err != nil {
			return nil, err
		}
		specs = append(specs, &spec)
	}
	return specs, rows.Err()
}
