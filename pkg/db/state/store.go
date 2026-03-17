package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	// MaxAsyncQueueSize is the maximum number of pending async writes.
	// When exceeded, oldest writes are dropped with a warning.
	MaxAsyncQueueSize = 1000

	// AsyncFlushTimeout is the timeout for flushing async writes on Close.
	AsyncFlushTimeout = 5 * time.Second
)

// Store manages the unified state of the OpenExec orchestrator.
// It is the single source of truth for Sessions, Runs, Steps, and Audit events.
type Store struct {
	db           *sql.DB
	mu           sync.RWMutex
	asyncQueue   chan func(context.Context) error
	asyncWg      sync.WaitGroup
	asyncDropped atomic.Int64
	closed       atomic.Bool
}

// NewStore creates a new state store using the provided SQLite database path.
func NewStore(dbPath string) (*Store, error) {
	// Standard OpenExec DB optimization: WAL mode + foreign keys
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open state database: %w", err)
	}

	s := &Store{
		db:         db,
		asyncQueue: make(chan func(context.Context) error, MaxAsyncQueueSize),
	}

	if err := s.Init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	// Start async write worker
	go s.asyncWorker()

	log.Printf("[StateStore] Initialized: path=%s, schema=v1, async_queue_size=%d", dbPath, MaxAsyncQueueSize)

	return s, nil
}

// Init initializes the database schema.
func (s *Store) Init() error {
    s.mu.Lock()
    defer s.mu.Unlock()

    if _, err := s.db.Exec(UnifiedSchema); err != nil {
        return fmt.Errorf("failed to initialize unified schema: %w", err)
    }
    // Apply forward-safe migrations for columns that may be missing on older DBs.
    // These are idempotent: we check the table schema before attempting ALTERs.
    // Covers drift between this unified schema and internal/release/schema.go.
    migrations := [][3]string{
        // stories columns
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
        {"tasks", "started_at", "DATETIME DEFAULT NULL"},
        {"tasks", "completed_at", "DATETIME DEFAULT NULL"},
        {"tasks", "error_message", "TEXT DEFAULT ''"},
        {"tasks", "metadata", "TEXT DEFAULT '{}'"},
    }
    for _, m := range migrations {
        if err := s.ensureColumn(m[0], m[1], m[2]); err != nil {
            return fmt.Errorf("failed to migrate %s.%s: %w", m[0], m[1], err)
        }
    }
    return nil
}

// ensureColumn checks for a column and adds it if missing.
func (s *Store) ensureColumn(table, column, definition string) error {
    // Probe table info for existing column
    query := fmt.Sprintf("PRAGMA table_info(%s)", table)
    rows, err := s.db.Query(query)
    if err != nil {
        return err
    }
    defer func() { _ = rows.Close() }()

    type colInfo struct {
        cid        int
        name       string
        colType    string
        notnull    int
        dflt_value any
        pk         int
    }
    for rows.Next() {
        var info colInfo
        // PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
        // Use Scan into placeholders matching types above
        if scanErr := rows.Scan(&info.cid, &info.name, &info.colType, &info.notnull, &info.dflt_value, &info.pk); scanErr != nil {
            return scanErr
        }
        if info.name == column {
            return nil // already exists
        }
    }
    if err := rows.Err(); err != nil {
        return err
    }

    // Add the missing column
    alter := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)
    if _, err := s.db.Exec(alter); err != nil {
        return err
    }
    return nil
}

// GetDB returns the underlying database handle.
func (s *Store) GetDB() *sql.DB {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.db
}

// Close flushes pending async writes and closes the database connection.
// It waits up to AsyncFlushTimeout for pending writes to complete.
func (s *Store) Close() error {
	s.closed.Store(true)

	// Close async queue to signal worker to stop
	close(s.asyncQueue)

	// Wait for pending writes with timeout
	done := make(chan struct{})
	go func() {
		s.asyncWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All writes completed
	case <-time.After(AsyncFlushTimeout):
		log.Printf("[StateStore] Warning: async flush timeout after %v, %d writes pending",
			AsyncFlushTimeout, len(s.asyncQueue))
	}

	// Report dropped writes if any
	if dropped := s.asyncDropped.Load(); dropped > 0 {
		log.Printf("[StateStore] Warning: %d async writes were dropped due to backpressure", dropped)
	}

	return s.db.Close()
}

// asyncWorker processes the async write queue.
func (s *Store) asyncWorker() {
	for fn := range s.asyncQueue {
		if fn != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := fn(ctx); err != nil {
				log.Printf("[StateStore] Async write error: %v", err)
			}
			cancel()
		}
		s.asyncWg.Done()
	}
}

// --- RUN OPERATIONS ---

// CreateRun persists a new execution run.
// Empty sessionID or taskID are stored as NULL to satisfy foreign key constraints.
func (s *Store) CreateRun(ctx context.Context, runID, sessionID, taskID, projectPath, mode string) error {
    query := `INSERT OR IGNORE INTO runs (id, session_id, task_id, project_path, mode, status) VALUES (?, ?, ?, ?, ?, 'starting')`
    var sessVal, taskVal interface{}
    if sessionID != "" {
        sessVal = sessionID
    }
    if taskID != "" {
        taskVal = taskID
    }
    _, err := s.db.ExecContext(ctx, query, runID, sessVal, taskVal, projectPath, mode)
    return err
}

// UpdateRunStatus updates the status of an active run.
func (s *Store) UpdateRunStatus(ctx context.Context, runID, status, errorMessage string) error {
    query := `UPDATE runs SET status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
    _, err := s.db.ExecContext(ctx, query, status, errorMessage, runID)
    return err
}

// --- STEP OPERATIONS ---

// AddRunStep records a single iteration step of a run.
func (s *Store) AddRunStep(ctx context.Context, stepID, runID, traceID, phase string, iteration int, status string) error {
    query := `INSERT OR IGNORE INTO run_steps (id, run_id, trace_id, phase, iteration, status) VALUES (?, ?, ?, ?, ?, ?)`
    _, err := s.db.ExecContext(ctx, query, stepID, runID, traceID, phase, iteration, status)
    return err
}

// --- ARTIFACT OPERATIONS ---

// RecordArtifact registers a content-addressed artifact pointer.
func (s *Store) RecordArtifact(ctx context.Context, hash, artifactType, path string, size int64) error {
    query := `INSERT OR IGNORE INTO artifacts (hash, type, path, size) VALUES (?, ?, ?, ?)`
    _, err := s.db.ExecContext(ctx, query, hash, artifactType, path, size)
    return err
}

// RecordArtifactWithMetadata registers an artifact with additional metadata.
func (s *Store) RecordArtifactWithMetadata(ctx context.Context, hash, artifactType, path string, size int64, metadata string) error {
    query := `INSERT OR IGNORE INTO artifacts (hash, type, path, size, metadata) VALUES (?, ?, ?, ?, ?)`
    _, err := s.db.ExecContext(ctx, query, hash, artifactType, path, size, metadata)
    return err
}

// --- TOOL CALL OPERATIONS ---

// RecordToolCall persists a tool invocation with idempotency key support.
func (s *Store) RecordToolCall(ctx context.Context, id, messageID, sessionID, toolName, toolInput, idempotencyKey string) error {
    query := `INSERT OR IGNORE INTO tool_calls (id, message_id, session_id, tool_name, tool_input, status, idempotency_key)
              VALUES (?, ?, ?, ?, ?, 'pending', ?)`
    _, err := s.db.ExecContext(ctx, query, id, messageID, sessionID, toolName, toolInput, idempotencyKey)
    return err
}

// UpdateToolCallStatus updates a tool call's status and output.
func (s *Store) UpdateToolCallStatus(ctx context.Context, id, status, output, errorMsg string) error {
    query := `UPDATE tool_calls SET status = ?, tool_output = ?, error = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`
    _, err := s.db.ExecContext(ctx, query, status, output, errorMsg, id)
    return err
}

// CheckIdempotencyKey returns true if a tool call with this key was already applied.
func (s *Store) CheckIdempotencyKey(ctx context.Context, idempotencyKey string) (bool, error) {
    query := `SELECT COUNT(*) FROM tool_calls WHERE idempotency_key = ? AND status = 'completed'`
    var count int
    err := s.db.QueryRowContext(ctx, query, idempotencyKey).Scan(&count)
    if err != nil {
        return false, err
    }
    return count > 0, nil
}

// --- CHECKPOINT OPERATIONS ---

// CheckpointData represents a run checkpoint for replay.
type CheckpointData struct {
    ID             string
    RunID          string
    Phase          string
    Iteration      int
    Timestamp      string
    Artifacts      map[string]string
    MessageHistory []byte // JSON array of messages for resume
    ToolCallLog    []byte // JSON array of completed tool call IDs
}

// RecordCheckpoint persists a checkpoint for a run.
func (s *Store) RecordCheckpoint(ctx context.Context, cp CheckpointData) error {
    artifactsJSON := "{}"
    if len(cp.Artifacts) > 0 {
        if data, err := json.Marshal(cp.Artifacts); err == nil {
            artifactsJSON = string(data)
        }
    }
    messageHistory := "[]"
    if len(cp.MessageHistory) > 0 {
        messageHistory = string(cp.MessageHistory)
    }
    toolCallLog := "[]"
    if len(cp.ToolCallLog) > 0 {
        toolCallLog = string(cp.ToolCallLog)
    }
    query := `INSERT INTO run_checkpoints (id, run_id, phase, iteration, timestamp, artifacts, message_history, tool_call_log)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
    _, err := s.db.ExecContext(ctx, query, cp.ID, cp.RunID, cp.Phase, cp.Iteration, cp.Timestamp, artifactsJSON, messageHistory, toolCallLog)
    return err
}

// RecordCheckpointWithHistory persists a checkpoint with full message history for resume.
func (s *Store) RecordCheckpointWithHistory(ctx context.Context, cp CheckpointData, messages []byte, toolCalls []byte) error {
    cp.MessageHistory = messages
    cp.ToolCallLog = toolCalls
    return s.RecordCheckpoint(ctx, cp)
}

// GetLatestCheckpoint returns the most recent checkpoint for a run.
func (s *Store) GetLatestCheckpoint(ctx context.Context, runID string) (*CheckpointData, error) {
    query := `SELECT id, run_id, phase, iteration, timestamp, artifacts,
              COALESCE(message_history, '[]'), COALESCE(tool_call_log, '[]')
              FROM run_checkpoints WHERE run_id = ? ORDER BY timestamp DESC LIMIT 1`
    row := s.db.QueryRowContext(ctx, query, runID)

    var cp CheckpointData
    var artifactsJSON, messageHistory, toolCallLog string
    err := row.Scan(&cp.ID, &cp.RunID, &cp.Phase, &cp.Iteration, &cp.Timestamp,
                    &artifactsJSON, &messageHistory, &toolCallLog)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, err
    }

    if artifactsJSON != "" {
        _ = json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts)
    }
    cp.MessageHistory = []byte(messageHistory)
    cp.ToolCallLog = []byte(toolCallLog)
    return &cp, nil
}

// GetCheckpointByID retrieves a specific checkpoint by ID.
func (s *Store) GetCheckpointByID(ctx context.Context, checkpointID string) (*CheckpointData, error) {
    query := `SELECT id, run_id, phase, iteration, timestamp, artifacts,
              COALESCE(message_history, '[]'), COALESCE(tool_call_log, '[]')
              FROM run_checkpoints WHERE id = ?`
    row := s.db.QueryRowContext(ctx, query, checkpointID)

    var cp CheckpointData
    var artifactsJSON, messageHistory, toolCallLog string
    err := row.Scan(&cp.ID, &cp.RunID, &cp.Phase, &cp.Iteration, &cp.Timestamp,
                    &artifactsJSON, &messageHistory, &toolCallLog)
    if err != nil {
        if err == sql.ErrNoRows {
            return nil, nil
        }
        return nil, err
    }

    if artifactsJSON != "" {
        _ = json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts)
    }
    cp.MessageHistory = []byte(messageHistory)
    cp.ToolCallLog = []byte(toolCallLog)
    return &cp, nil
}

// ListAppliedToolCalls returns the idempotency keys of tool calls that have been completed.
// This is used during resume to skip already-applied tool calls.
func (s *Store) ListAppliedToolCalls(ctx context.Context, runID string) ([]string, error) {
    query := `SELECT DISTINCT tc.idempotency_key
              FROM tool_calls tc
              JOIN messages m ON tc.message_id = m.id
              JOIN sessions s ON m.session_id = s.id
              JOIN runs r ON r.session_id = s.id
              WHERE r.id = ? AND tc.status = 'completed' AND tc.idempotency_key IS NOT NULL`
    rows, err := s.db.QueryContext(ctx, query, runID)
    if err != nil {
        return nil, err
    }
    defer func() { _ = rows.Close() }()

    var keys []string
    for rows.Next() {
        var key string
        if err := rows.Scan(&key); err != nil {
            return nil, err
        }
        keys = append(keys, key)
    }
    return keys, rows.Err()
}

// GetToolCallByIdempotencyKey retrieves the result of a previously completed tool call.
// Returns nil if not found or not completed.
func (s *Store) GetToolCallByIdempotencyKey(ctx context.Context, idempotencyKey string) (string, error) {
    query := `SELECT tool_output FROM tool_calls
              WHERE idempotency_key = ? AND status = 'completed' LIMIT 1`
    var output sql.NullString
    err := s.db.QueryRowContext(ctx, query, idempotencyKey).Scan(&output)
    if err != nil {
        if err == sql.ErrNoRows {
            return "", nil
        }
        return "", err
    }
    return output.String, nil
}

// --- SESSION OPERATIONS ---

// CreateSession persists a new session.
func (s *Store) CreateSession(ctx context.Context, id, projectPath, provider, model, title string) error {
    query := `INSERT INTO sessions (id, project_path, provider, model, title) VALUES (?, ?, ?, ?, ?)`
    _, err := s.db.ExecContext(ctx, query, id, projectPath, provider, model, title)
    return err
}

// UpdateSessionStatus updates the status of a session.
func (s *Store) UpdateSessionStatus(ctx context.Context, id, status string) error {
    query := `UPDATE sessions SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
    _, err := s.db.ExecContext(ctx, query, status, id)
    return err
}

// --- RUN STEP EXTENDED OPERATIONS ---

// UpdateRunStepCompleted marks a run step as completed with output hash.
func (s *Store) UpdateRunStepCompleted(ctx context.Context, stepID, outputsHash string) error {
    query := `UPDATE run_steps SET status = 'completed', outputs_hash = ?, completed_at = CURRENT_TIMESTAMP WHERE id = ?`
    _, err := s.db.ExecContext(ctx, query, outputsHash, stepID)
    return err
}

// AddRunStepFull records a run step with all fields.
func (s *Store) AddRunStepFull(ctx context.Context, stepID, runID, traceID, phase, agent string, iteration int, status, inputsHash, metadata string) error {
    query := `INSERT OR IGNORE INTO run_steps (id, run_id, trace_id, phase, agent, iteration, status, inputs_hash, metadata)
              VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
    _, err := s.db.ExecContext(ctx, query, stepID, runID, traceID, phase, agent, iteration, status, inputsHash, metadata)
    return err
}

// --- ASYNC PARALLEL WRITE HELPERS ---

// WriteAsync queues data for asynchronous writing without blocking the caller.
// If the queue is full, the oldest write is dropped with a warning.
// Errors are logged but don't block execution.
func (s *Store) WriteAsync(ctx context.Context, fn func(context.Context) error) {
	if s.closed.Load() {
		return
	}

	s.asyncWg.Add(1)

	select {
	case s.asyncQueue <- fn:
		// Queued successfully
	default:
		// Queue full - drop with warning (backpressure)
		s.asyncWg.Done()
		s.asyncDropped.Add(1)
		if s.asyncDropped.Load()%100 == 1 {
			log.Printf("[StateStore] Warning: async write dropped (queue full, total dropped: %d)", s.asyncDropped.Load())
		}
	}
}

// WriteRunStepWithArtifacts atomically writes a run step along with any artifacts and checkpoint.
// This ensures no partial writes occur.
func (s *Store) WriteRunStepWithArtifacts(ctx context.Context, step RunStepData, artifacts map[string]ArtifactData, checkpoint *CheckpointData) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Write run step
	_, err = tx.ExecContext(ctx,
		`INSERT INTO run_steps (id, run_id, trace_id, phase, agent, iteration, status, inputs_hash, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO NOTHING`,
		step.ID, step.RunID, step.TraceID, step.Phase, step.Agent, step.Iteration, step.Status, step.InputsHash, step.Metadata)
	if err != nil {
		return fmt.Errorf("insert run_step: %w", err)
	}

	// Write artifacts
	for hash, art := range artifacts {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO artifacts (hash, type, path, size, metadata)
			 VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(hash) DO NOTHING`,
			hash, art.Type, art.Path, art.Size, art.Metadata)
		if err != nil {
			return fmt.Errorf("insert artifact: %w", err)
		}
	}

	// Write checkpoint if provided
	if checkpoint != nil {
		artifactsJSON := "{}"
		if len(checkpoint.Artifacts) > 0 {
			if data, err := json.Marshal(checkpoint.Artifacts); err == nil {
				artifactsJSON = string(data)
			}
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO run_checkpoints (id, run_id, phase, iteration, timestamp, artifacts)
			 VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			checkpoint.ID, checkpoint.RunID, checkpoint.Phase, checkpoint.Iteration, checkpoint.Timestamp, artifactsJSON)
		if err != nil {
			return fmt.Errorf("insert checkpoint: %w", err)
		}
	}

	return tx.Commit()
}

// RunStepData holds data for a run step insert.
type RunStepData struct {
	ID         string
	RunID      string
	TraceID    string
	Phase      string
	Agent      string
	Iteration  int
	Status     string
	InputsHash string
	Metadata   string
}

// ArtifactData holds data for an artifact insert.
type ArtifactData struct {
	Type     string
	Path     string
	Size     int64
	Metadata string
}

// --- RUN READER OPERATIONS ---

// RunRecord represents a run stored in the database.
type RunRecord struct {
	ID           string
	SessionID    sql.NullString
	TaskID       sql.NullString
	ProjectPath  string
	Mode         string
	Status       string
	ErrorMessage sql.NullString
	Metadata     string
	CreatedAt    string
	UpdatedAt    string
	StartedAt    sql.NullString
	CompletedAt  sql.NullString
}

// GetRun retrieves a single run by ID.
func (s *Store) GetRun(ctx context.Context, runID string) (*RunRecord, error) {
	query := `SELECT id, session_id, task_id, project_path, mode, status, error_message,
	          COALESCE(metadata, '{}'), created_at, updated_at, started_at, completed_at
	          FROM runs WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, runID)

	var r RunRecord
	err := row.Scan(&r.ID, &r.SessionID, &r.TaskID, &r.ProjectPath, &r.Mode, &r.Status,
		&r.ErrorMessage, &r.Metadata, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.CompletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

// RunFilter provides filtering options for ListRuns.
type RunFilter struct {
	SessionID   string
	ProjectPath string
	Status      string
	Limit       int
	Offset      int
}

// ListRuns retrieves runs matching the given filter.
func (s *Store) ListRuns(ctx context.Context, filter RunFilter) ([]RunRecord, error) {
	query := `SELECT id, session_id, task_id, project_path, mode, status, error_message,
	          COALESCE(metadata, '{}'), created_at, updated_at, started_at, completed_at
	          FROM runs WHERE 1=1`
	args := []interface{}{}

	if filter.SessionID != "" {
		query += " AND session_id = ?"
		args = append(args, filter.SessionID)
	}
	if filter.ProjectPath != "" {
		query += " AND project_path = ?"
		args = append(args, filter.ProjectPath)
	}
	if filter.Status != "" {
		query += " AND status = ?"
		args = append(args, filter.Status)
	}

	query += " ORDER BY created_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var runs []RunRecord
	for rows.Next() {
		var r RunRecord
		err := rows.Scan(&r.ID, &r.SessionID, &r.TaskID, &r.ProjectPath, &r.Mode, &r.Status,
			&r.ErrorMessage, &r.Metadata, &r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.CompletedAt)
		if err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// --- RUN STEP READER OPERATIONS ---

// RunStepRecord represents a run step stored in the database.
type RunStepRecord struct {
	ID          string
	RunID       string
	TraceID     sql.NullString
	Phase       string
	Agent       sql.NullString
	Iteration   int
	Status      string
	InputsHash  sql.NullString
	OutputsHash sql.NullString
	StartedAt   string
	CompletedAt sql.NullString
	Metadata    string
}

// ListRunSteps retrieves steps for a run with pagination.
func (s *Store) ListRunSteps(ctx context.Context, runID string, limit, offset int) ([]RunStepRecord, error) {
	query := `SELECT id, run_id, trace_id, phase, agent, iteration, status,
	          inputs_hash, outputs_hash, started_at, completed_at, COALESCE(metadata, '{}')
	          FROM run_steps WHERE run_id = ? ORDER BY iteration ASC, started_at ASC`
	args := []interface{}{runID}

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	if offset > 0 {
		query += " OFFSET ?"
		args = append(args, offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var steps []RunStepRecord
	for rows.Next() {
		var step RunStepRecord
		err := rows.Scan(&step.ID, &step.RunID, &step.TraceID, &step.Phase, &step.Agent,
			&step.Iteration, &step.Status, &step.InputsHash, &step.OutputsHash,
			&step.StartedAt, &step.CompletedAt, &step.Metadata)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

// GetRunStep retrieves a single run step by ID.
func (s *Store) GetRunStep(ctx context.Context, stepID string) (*RunStepRecord, error) {
	query := `SELECT id, run_id, trace_id, phase, agent, iteration, status,
	          inputs_hash, outputs_hash, started_at, completed_at, COALESCE(metadata, '{}')
	          FROM run_steps WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, stepID)

	var step RunStepRecord
	err := row.Scan(&step.ID, &step.RunID, &step.TraceID, &step.Phase, &step.Agent,
		&step.Iteration, &step.Status, &step.InputsHash, &step.OutputsHash,
		&step.StartedAt, &step.CompletedAt, &step.Metadata)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &step, nil
}

// --- ARTIFACT READER OPERATIONS ---

// ArtifactRecord represents an artifact stored in the database.
type ArtifactRecord struct {
	Hash      string
	Type      string
	Path      string
	Size      int64
	Metadata  string
	CreatedAt string
}

// GetArtifact retrieves an artifact by its content hash.
func (s *Store) GetArtifact(ctx context.Context, hash string) (*ArtifactRecord, error) {
	query := `SELECT hash, type, path, size, COALESCE(metadata, '{}'), created_at
	          FROM artifacts WHERE hash = ?`
	row := s.db.QueryRowContext(ctx, query, hash)

	var a ArtifactRecord
	err := row.Scan(&a.Hash, &a.Type, &a.Path, &a.Size, &a.Metadata, &a.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

// ListArtifacts retrieves all artifacts, optionally filtered by type.
func (s *Store) ListArtifacts(ctx context.Context, artifactType string, limit int) ([]ArtifactRecord, error) {
	query := `SELECT hash, type, path, size, COALESCE(metadata, '{}'), created_at FROM artifacts`
	args := []interface{}{}

	if artifactType != "" {
		query += " WHERE type = ?"
		args = append(args, artifactType)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var artifacts []ArtifactRecord
	for rows.Next() {
		var a ArtifactRecord
		err := rows.Scan(&a.Hash, &a.Type, &a.Path, &a.Size, &a.Metadata, &a.CreatedAt)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, rows.Err()
}

// --- CHECKPOINT READER OPERATIONS ---

// ListCheckpoints retrieves all checkpoints for a run.
func (s *Store) ListCheckpoints(ctx context.Context, runID string) ([]CheckpointData, error) {
	query := `SELECT id, run_id, phase, iteration, timestamp, artifacts
	          FROM run_checkpoints WHERE run_id = ? ORDER BY timestamp ASC`
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var checkpoints []CheckpointData
	for rows.Next() {
		var cp CheckpointData
		var artifactsJSON string
		err := rows.Scan(&cp.ID, &cp.RunID, &cp.Phase, &cp.Iteration, &cp.Timestamp, &artifactsJSON)
		if err != nil {
			return nil, err
		}
		if artifactsJSON != "" {
			_ = json.Unmarshal([]byte(artifactsJSON), &cp.Artifacts)
		}
		checkpoints = append(checkpoints, cp)
	}
	return checkpoints, rows.Err()
}

// --- SESSION READER OPERATIONS ---

// SessionRecord represents a session stored in the database.
type SessionRecord struct {
	ID                 string
	ProjectPath        string
	Provider           string
	Model              string
	Title              string
	ParentSessionID    sql.NullString
	ForkPointMessageID sql.NullString
	Status             string
	Metadata           string
	CreatedAt          string
	UpdatedAt          string
}

// GetSession retrieves a single session by ID.
func (s *Store) GetSession(ctx context.Context, sessionID string) (*SessionRecord, error) {
	query := `SELECT id, project_path, provider, model, title, parent_session_id,
	          fork_point_message_id, status, COALESCE(metadata, '{}'), created_at, updated_at
	          FROM sessions WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, sessionID)

	var sess SessionRecord
	err := row.Scan(&sess.ID, &sess.ProjectPath, &sess.Provider, &sess.Model, &sess.Title,
		&sess.ParentSessionID, &sess.ForkPointMessageID, &sess.Status, &sess.Metadata,
		&sess.CreatedAt, &sess.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

// ListSessions retrieves sessions, optionally filtered by project path.
func (s *Store) ListSessions(ctx context.Context, projectPath string, limit int) ([]SessionRecord, error) {
	query := `SELECT id, project_path, provider, model, title, parent_session_id,
	          fork_point_message_id, status, COALESCE(metadata, '{}'), created_at, updated_at
	          FROM sessions`
	args := []interface{}{}

	if projectPath != "" {
		query += " WHERE project_path = ?"
		args = append(args, projectPath)
	}

	query += " ORDER BY updated_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var sessions []SessionRecord
	for rows.Next() {
		var sess SessionRecord
		err := rows.Scan(&sess.ID, &sess.ProjectPath, &sess.Provider, &sess.Model, &sess.Title,
			&sess.ParentSessionID, &sess.ForkPointMessageID, &sess.Status, &sess.Metadata,
			&sess.CreatedAt, &sess.UpdatedAt)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, sess)
	}
	return sessions, rows.Err()
}
