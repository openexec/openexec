// Package agent provides multi-agent orchestration for OpenExec.
// It enables parallel execution of blueprint stages across multiple agents.
package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// AgentType indicates the role of an agent in the swarm.
type AgentType string

const (
	// AgentTypeWorker processes individual file batches.
	AgentTypeWorker AgentType = "worker"
	// AgentTypeCoordinator manages the swarm and merges results.
	AgentTypeCoordinator AgentType = "coordinator"
	// AgentTypeReviewer validates changes without modifying code.
	AgentTypeReviewer AgentType = "reviewer"
)

// AgentStatus indicates the current state of an agent.
type AgentStatus string

const (
	AgentStatusIdle       AgentStatus = "idle"
	AgentStatusRunning    AgentStatus = "running"
	AgentStatusCompleted  AgentStatus = "completed"
	AgentStatusFailed     AgentStatus = "failed"
	AgentStatusCancelled  AgentStatus = "cancelled"
)

// Agent represents a single agent in the swarm.
type Agent struct {
	ID          string      `json:"id"`
	Type        AgentType   `json:"type"`
	Status      AgentStatus `json:"status"`
	BlueprintID string      `json:"blueprint_id"`
	RunID       string      `json:"run_id"`
	StageName   string      `json:"stage_name,omitempty"`
	BatchIndex  int         `json:"batch_index,omitempty"`
	BatchSize   int         `json:"batch_size,omitempty"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty"`
	Result      *AgentResult `json:"result,omitempty"`
	Error       string      `json:"error,omitempty"`
}

// AgentResult contains the output from an agent execution.
type AgentResult struct {
	FilesProcessed []string          `json:"files_processed"`
	Changes        []FileChange      `json:"changes"`
	Summary        string            `json:"summary"`
	Artifacts      map[string]string `json:"artifacts"`
}

// FileChange represents a single file modification.
type FileChange struct {
	Path      string `json:"path"`
	Operation string `json:"operation"` // "modified", "created", "deleted"
	Diff      string `json:"diff,omitempty"`
}

// AgentRegistry manages agent state and coordination.
type AgentRegistry struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewAgentRegistry creates a new agent registry.
func NewAgentRegistry(projectDir string) (*AgentRegistry, error) {
	dbPath := filepath.Join(projectDir, ".openexec", "agents.db")
	db, err := sql.Open("sqlite", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open agent registry db: %w", err)
	}

	registry := &AgentRegistry{db: db}
	if err := registry.migrate(); err != nil {
		return nil, err
	}

	return registry, nil
}

// NewAgentRegistryWithDB creates a registry using an existing database connection.
func NewAgentRegistryWithDB(db *sql.DB) (*AgentRegistry, error) {
	registry := &AgentRegistry{db: db}
	if err := registry.migrate(); err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *AgentRegistry) migrate() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			status TEXT NOT NULL,
			blueprint_id TEXT NOT NULL,
			run_id TEXT NOT NULL,
			stage_name TEXT,
			batch_index INTEGER DEFAULT 0,
			batch_size INTEGER DEFAULT 0,
			started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			completed_at DATETIME,
			result TEXT,
			error TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_blueprint 
			ON agents(blueprint_id, run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_status 
			ON agents(status);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_type 
			ON agents(type, status);`,
	}

	for _, q := range queries {
		if _, err := r.db.Exec(q); err != nil {
			return fmt.Errorf("agent registry migration failed: %w", err)
		}
	}
	return nil
}

// Register creates a new agent entry.
func (r *AgentRegistry) Register(agent *Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO agents 
		(id, type, status, blueprint_id, run_id, stage_name, batch_index, batch_size, started_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	
	_, err := r.db.Exec(query,
		agent.ID,
		agent.Type,
		agent.Status,
		agent.BlueprintID,
		agent.RunID,
		agent.StageName,
		agent.BatchIndex,
		agent.BatchSize,
		agent.StartedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to register agent: %w", err)
	}
	return nil
}

// UpdateStatus updates an agent's status.
func (r *AgentRegistry) UpdateStatus(agentID string, status AgentStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `UPDATE agents SET status = ? WHERE id = ?`
	_, err := r.db.Exec(query, status, agentID)
	if err != nil {
		return fmt.Errorf("failed to update agent status: %w", err)
	}
	return nil
}

// Complete marks an agent as completed with results.
func (r *AgentRegistry) Complete(agentID string, result *AgentResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	completedAt := time.Now().UTC()
	query := `UPDATE agents 
		SET status = ?, result = ?, completed_at = ?
		WHERE id = ?`
	
	_, err = r.db.Exec(query, AgentStatusCompleted, string(resultJSON), completedAt, agentID)
	if err != nil {
		return fmt.Errorf("failed to complete agent: %w", err)
	}
	return nil
}

// Fail marks an agent as failed with an error.
func (r *AgentRegistry) Fail(agentID string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	completedAt := time.Now().UTC()
	query := `UPDATE agents 
		SET status = ?, error = ?, completed_at = ?
		WHERE id = ?`
	
	_, err := r.db.Exec(query, AgentStatusFailed, errMsg, completedAt, agentID)
	if err != nil {
		return fmt.Errorf("failed to mark agent as failed: %w", err)
	}
	return nil
}

// Get retrieves an agent by ID.
func (r *AgentRegistry) Get(agentID string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent := &Agent{}
	var resultJSON, errorMsg sql.NullString
	var completedAt sql.NullTime

	query := `SELECT id, type, status, blueprint_id, run_id, stage_name, 
		batch_index, batch_size, started_at, completed_at, result, error
		FROM agents WHERE id = ?`
	
	err := r.db.QueryRow(query, agentID).Scan(
		&agent.ID,
		&agent.Type,
		&agent.Status,
		&agent.BlueprintID,
		&agent.RunID,
		&agent.StageName,
		&agent.BatchIndex,
		&agent.BatchSize,
		&agent.StartedAt,
		&completedAt,
		&resultJSON,
		&errorMsg,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	if completedAt.Valid {
		agent.CompletedAt = &completedAt.Time
	}
	if errorMsg.Valid {
		agent.Error = errorMsg.String
	}
	if resultJSON.Valid {
		var result AgentResult
		if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
			agent.Result = &result
		}
	}

	return agent, nil
}

// ListByRun retrieves all agents for a blueprint run.
func (r *AgentRegistry) ListByRun(blueprintID, runID string) ([]*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT id, type, status, blueprint_id, run_id, stage_name, 
		batch_index, batch_size, started_at, completed_at, result, error
		FROM agents WHERE blueprint_id = ? AND run_id = ?
		ORDER BY started_at`
	
	rows, err := r.db.Query(query, blueprintID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// ListByStatus retrieves agents by status.
func (r *AgentRegistry) ListByStatus(status AgentStatus) ([]*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT id, type, status, blueprint_id, run_id, stage_name, 
		batch_index, batch_size, started_at, completed_at, result, error
		FROM agents WHERE status = ?
		ORDER BY started_at`
	
	rows, err := r.db.Query(query, status)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents by status: %w", err)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// ListActive retrieves all active (non-completed) agents.
func (r *AgentRegistry) ListActive() ([]*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT id, type, status, blueprint_id, run_id, stage_name, 
		batch_index, batch_size, started_at, completed_at, result, error
		FROM agents WHERE status IN (?, ?)
		ORDER BY started_at`
	
	rows, err := r.db.Query(query, AgentStatusRunning, AgentStatusIdle)
	if err != nil {
		return nil, fmt.Errorf("failed to list active agents: %w", err)
	}
	defer rows.Close()

	return r.scanAgents(rows)
}

// CountByStatus counts agents by status for a run.
func (r *AgentRegistry) CountByStatus(blueprintID, runID string) (map[AgentStatus]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT status, COUNT(*) FROM agents 
		WHERE blueprint_id = ? AND run_id = ?
		GROUP BY status`
	
	rows, err := r.db.Query(query, blueprintID, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to count agents: %w", err)
	}
	defer rows.Close()

	counts := make(map[AgentStatus]int)
	for rows.Next() {
		var status AgentStatus
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}

	return counts, nil
}

// Cleanup removes old agent records.
func (r *AgentRegistry) Cleanup(before time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `DELETE FROM agents WHERE completed_at < ?`
	_, err := r.db.Exec(query, before)
	if err != nil {
		return fmt.Errorf("failed to cleanup agents: %w", err)
	}
	return nil
}

// Close closes the registry database connection.
func (r *AgentRegistry) Close() error {
	return r.db.Close()
}

func (r *AgentRegistry) scanAgents(rows *sql.Rows) ([]*Agent, error) {
	var agents []*Agent

	for rows.Next() {
		agent := &Agent{}
		var resultJSON, errorMsg sql.NullString
		var completedAt sql.NullTime

		err := rows.Scan(
			&agent.ID,
			&agent.Type,
			&agent.Status,
			&agent.BlueprintID,
			&agent.RunID,
			&agent.StageName,
			&agent.BatchIndex,
			&agent.BatchSize,
			&agent.StartedAt,
			&completedAt,
			&resultJSON,
			&errorMsg,
		)
		if err != nil {
			return nil, err
		}

		if completedAt.Valid {
			agent.CompletedAt = &completedAt.Time
		}
		if errorMsg.Valid {
			agent.Error = errorMsg.String
		}
		if resultJSON.Valid {
			var result AgentResult
			if err := json.Unmarshal([]byte(resultJSON.String), &result); err == nil {
				agent.Result = &result
			}
		}

		agents = append(agents, agent)
	}

	return agents, rows.Err()
}


