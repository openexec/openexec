// Package checkpoint provides deterministic checkpointing for OpenExec.
// It enables recovery from crashes and resumes execution from the last known good state.
package checkpoint

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	_ "github.com/mattn/go-sqlite3"
)

// Manager handles checkpoint creation and restoration.
type Manager struct {
	db        *sql.DB
	projectDir string
}

// Checkpoint represents a saved execution state.
type Checkpoint struct {
	ID            string                 `json:"id"`
	RunID         string                 `json:"run_id"`
	BlueprintID   string                 `json:"blueprint_id"`
	StageName     string                 `json:"stage_name"`
	StageResults  []blueprint.StageResult `json:"stage_results"`
	WorkingState  map[string]FileState   `json:"working_state"`
	Variables     map[string]string      `json:"variables"`
	CreatedAt     time.Time              `json:"created_at"`
	Checksum      string                 `json:"checksum"`
	Status        CheckpointStatus       `json:"status"`
}

// FileState tracks the state of a file at checkpoint time.
type FileState struct {
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

// CheckpointStatus represents the status of a checkpoint.
type CheckpointStatus string

const (
	// CheckpointStatusValid means the checkpoint is valid and can be restored.
	CheckpointStatusValid CheckpointStatus = "valid"
	// CheckpointStatusStale means files have changed since the checkpoint.
	CheckpointStatusStale CheckpointStatus = "stale"
	// CheckpointStatusCorrupted means the checkpoint data is corrupted.
	CheckpointStatusCorrupted CheckpointStatus = "corrupted"
)

// NewManager creates a new checkpoint manager.
func NewManager(projectDir string) (*Manager, error) {
	dbPath := filepath.Join(projectDir, ".openexec", "checkpoints.db")
	db, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open checkpoint db: %w", err)
	}

	manager := &Manager{
		db:         db,
		projectDir: projectDir,
	}

	if err := manager.migrate(); err != nil {
		return nil, err
	}

	return manager, nil
}

func (m *Manager) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS checkpoints (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			blueprint_id TEXT NOT NULL,
			stage_name TEXT NOT NULL,
			stage_results TEXT NOT NULL,
			working_state TEXT NOT NULL,
			variables TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			checksum TEXT NOT NULL,
			status TEXT DEFAULT 'valid'
		);`,
		`CREATE INDEX IF NOT EXISTS idx_checkpoints_run 
			ON checkpoints(run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_checkpoints_blueprint 
			ON checkpoints(blueprint_id, run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_checkpoints_status 
			ON checkpoints(status);`,
	}

	for _, q := range queries {
		if _, err := m.db.Exec(q); err != nil {
			return fmt.Errorf("checkpoint migration failed: %w", err)
		}
	}
	return nil
}

// Create creates a new checkpoint.
func (m *Manager) Create(run *blueprint.Run, workingDir string) (*Checkpoint, error) {
	checkpoint := &Checkpoint{
		ID:           generateID(),
		RunID:        run.ID,
		BlueprintID:  run.BlueprintID,
		StageName:    run.CurrentStage,
		StageResults: make([]blueprint.StageResult, len(run.Results)),
		WorkingState: make(map[string]FileState),
		Variables:    make(map[string]string),
		CreatedAt:    time.Now().UTC(),
		Status:       CheckpointStatusValid,
	}

	// Copy stage results (dereference pointers)
	for i, r := range run.Results {
		if r != nil {
			checkpoint.StageResults[i] = *r
		}
	}

	// Capture working state (file hashes)
	if err := m.captureWorkingState(checkpoint, workingDir); err != nil {
		return nil, fmt.Errorf("failed to capture working state: %w", err)
	}

	// Calculate checksum
	checkpoint.Checksum = m.calculateChecksum(checkpoint)

	// Save to database
	if err := m.save(checkpoint); err != nil {
		return nil, fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return checkpoint, nil
}

// Restore restores execution from a checkpoint.
func (m *Manager) Restore(checkpointID string) (*Checkpoint, error) {
	checkpoint, err := m.load(checkpointID)
	if err != nil {
		return nil, err
	}

	// Verify checksum
	if !m.verifyChecksum(checkpoint) {
		checkpoint.Status = CheckpointStatusCorrupted
		_ = m.updateStatus(checkpointID, CheckpointStatusCorrupted)
		return nil, fmt.Errorf("checkpoint %s is corrupted", checkpointID)
	}

	// Verify working state
	if err := m.verifyWorkingState(checkpoint); err != nil {
		checkpoint.Status = CheckpointStatusStale
		_ = m.updateStatus(checkpointID, CheckpointStatusStale)
		return checkpoint, fmt.Errorf("checkpoint %s is stale: %w", checkpointID, err)
	}

	return checkpoint, nil
}

// GetLatest gets the latest checkpoint for a run.
func (m *Manager) GetLatest(runID string) (*Checkpoint, error) {
	var checkpointID string
	query := `SELECT id FROM checkpoints WHERE run_id = ? AND status = 'valid' ORDER BY created_at DESC, rowid DESC LIMIT 1`
	err := m.db.QueryRow(query, runID).Scan(&checkpointID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest checkpoint: %w", err)
	}

	return m.load(checkpointID)
}

// List lists all checkpoints for a run.
func (m *Manager) List(runID string) ([]*Checkpoint, error) {
	query := `SELECT id FROM checkpoints WHERE run_id = ? ORDER BY created_at DESC`
	rows, err := m.db.Query(query, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list checkpoints: %w", err)
	}
	defer rows.Close()

	var checkpoints []*Checkpoint
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		checkpoint, err := m.load(id)
		if err != nil {
			continue
		}
		checkpoints = append(checkpoints, checkpoint)
	}

	return checkpoints, nil
}

// Delete deletes a checkpoint.
func (m *Manager) Delete(checkpointID string) error {
	_, err := m.db.Exec(`DELETE FROM checkpoints WHERE id = ?`, checkpointID)
	if err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}
	return nil
}

// Cleanup removes old checkpoints.
func (m *Manager) Cleanup(olderThan time.Time) error {
	_, err := m.db.Exec(`DELETE FROM checkpoints WHERE created_at < ?`, olderThan.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return fmt.Errorf("failed to cleanup checkpoints: %w", err)
	}
	return nil
}

// captureWorkingState captures the current state of files.
func (m *Manager) captureWorkingState(checkpoint *Checkpoint, workingDir string) error {
	return filepath.Walk(workingDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories and non-files
		if info.IsDir() {
			return nil
		}

		// Skip hidden files and common ignore patterns
		if shouldIgnore(path) {
			return nil
		}

		// Calculate file hash
		hash, err := m.hashFile(path)
		if err != nil {
			return nil // Skip files we can't hash
		}

		checkpoint.WorkingState[path] = FileState{
			Path:       path,
			Hash:       hash,
			Size:       info.Size(),
			ModifiedAt: info.ModTime(),
		}

		return nil
	})
}

// verifyWorkingState verifies that files haven't changed.
func (m *Manager) verifyWorkingState(checkpoint *Checkpoint) error {
	for path, state := range checkpoint.WorkingState {
		// Check if file exists
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("file %s no longer exists", path)
		}

		// Check size
		if info.Size() != state.Size {
			return fmt.Errorf("file %s size changed", path)
		}

		// Check modification time
		if info.ModTime().After(state.ModifiedAt) {
			// Recalculate hash
			hash, err := m.hashFile(path)
			if err != nil {
				return fmt.Errorf("file %s cannot be hashed", path)
			}
			if hash != state.Hash {
				return fmt.Errorf("file %s content changed", path)
			}
		}
	}

	return nil
}

// hashFile calculates the hash of a file.
func (m *Manager) hashFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:]), nil
}

// calculateChecksum calculates a checksum for the checkpoint.
func (m *Manager) calculateChecksum(checkpoint *Checkpoint) string {
	data, _ := json.Marshal(struct {
		RunID        string
		BlueprintID  string
		StageName    string
		StageResults []blueprint.StageResult
		WorkingState map[string]FileState
	}{
		RunID:        checkpoint.RunID,
		BlueprintID:  checkpoint.BlueprintID,
		StageName:    checkpoint.StageName,
		StageResults: checkpoint.StageResults,
		WorkingState: checkpoint.WorkingState,
	})

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// verifyChecksum verifies the checkpoint checksum.
func (m *Manager) verifyChecksum(checkpoint *Checkpoint) bool {
	expected := m.calculateChecksum(checkpoint)
	return expected == checkpoint.Checksum
}

// save saves a checkpoint to the database.
func (m *Manager) save(checkpoint *Checkpoint) error {
	stageResultsJSON, _ := json.Marshal(checkpoint.StageResults)
	workingStateJSON, _ := json.Marshal(checkpoint.WorkingState)
	variablesJSON, _ := json.Marshal(checkpoint.Variables)

	query := `INSERT INTO checkpoints 
		(id, run_id, blueprint_id, stage_name, stage_results, working_state, variables, created_at, checksum, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := m.db.Exec(query,
		checkpoint.ID,
		checkpoint.RunID,
		checkpoint.BlueprintID,
		checkpoint.StageName,
		string(stageResultsJSON),
		string(workingStateJSON),
		string(variablesJSON),
		checkpoint.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
		checkpoint.Checksum,
		string(checkpoint.Status),
	)

	return err
}

// load loads a checkpoint from the database.
func (m *Manager) load(checkpointID string) (*Checkpoint, error) {
	checkpoint := &Checkpoint{}
	var stageResultsJSON, workingStateJSON, variablesJSON string

	query := `SELECT id, run_id, blueprint_id, stage_name, stage_results, 
		working_state, variables, created_at, checksum, status 
		FROM checkpoints WHERE id = ?`

	err := m.db.QueryRow(query, checkpointID).Scan(
		&checkpoint.ID,
		&checkpoint.RunID,
		&checkpoint.BlueprintID,
		&checkpoint.StageName,
		&stageResultsJSON,
		&workingStateJSON,
		&variablesJSON,
		&checkpoint.CreatedAt,
		&checkpoint.Checksum,
		&checkpoint.Status,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("checkpoint %s not found", checkpointID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	// Parse JSON fields
	json.Unmarshal([]byte(stageResultsJSON), &checkpoint.StageResults)
	json.Unmarshal([]byte(workingStateJSON), &checkpoint.WorkingState)
	json.Unmarshal([]byte(variablesJSON), &checkpoint.Variables)

	return checkpoint, nil
}

// updateStatus updates the status of a checkpoint.
func (m *Manager) updateStatus(checkpointID string, status CheckpointStatus) error {
	_, err := m.db.Exec(`UPDATE checkpoints SET status = ? WHERE id = ?`, string(status), checkpointID)
	return err
}

// generateID generates a unique checkpoint ID.
func generateID() string {
	return fmt.Sprintf("cp-%d-%s", time.Now().UnixNano(), randomString(8))
}

// randomString generates a random string.
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// shouldIgnore determines if a file should be ignored.
func shouldIgnore(path string) bool {
	// Ignore hidden files
	if filepath.Base(path)[0] == '.' {
		return true
	}

	// Ignore common directories
	ignorePatterns := []string{
		".git/", ".svn/", ".hg/",
		"node_modules/", "vendor/",
		"__pycache__/", ".pytest_cache/",
		"dist/", "build/", ".next/",
		".openexec/",
	}

	for _, pattern := range ignorePatterns {
		if contains(path, pattern) {
			return true
		}
	}

	return false
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Close closes the checkpoint manager.
func (m *Manager) Close() error {
	return m.db.Close()
}
