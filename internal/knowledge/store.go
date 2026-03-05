package knowledge

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// SymbolRecord represents detailed function/struct metadata (OpenCode)
type SymbolRecord struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"` // func, struct, interface
	FilePath     string `json:"file_path"`
	StartLine    int    `json:"start_line"`
	EndLine      int    `json:"end_line"`
	Purpose      string `json:"purpose"`
	InputParams  string `json:"input_params"`  // Formatted string or JSON
	OutputParams string `json:"output_params"` // Formatted string or JSON
	Signature    string `json:"signature"`
}

// ServerNode represents a single server in an environment
type ServerNode struct {
	IP       string   `json:"ip"`
	Services []string `json:"services"`
	Role     string   `json:"role"` // e.g., "worker", "control-plane", "database"
}

// EnvironmentRecord represents a complex deployment environment
type EnvironmentRecord struct {
	Env          string       `json:"env"`           // e.g., dev, prod, local
	RuntimeType  string       `json:"runtime_type"`  // e.g., k8s, docker-compose, vm-docker
	AuthSteps    string       `json:"auth_steps"`    // JSON array of strings: ["gcloud auth login"]
	DeploySteps  string       `json:"deploy_steps"`  // JSON array of strings
	Topology     string       `json:"topology"`      // JSON array of ServerNode
	Instructions string       `json:"instructions"`  // Human readable fallback context
}

// APIDocRecord represents contract metadata
type APIDocRecord struct {
	Path           string `json:"path"`   // e.g. "/api/v1/login"
	Method         string `json:"method"` // GET, POST
	RequestSchema  string `json:"request_schema"`
	ResponseSchema string `json:"response_schema"`
	Description    string `json:"description"`
}

// PolicyRecord represents a deterministic rule
type PolicyRecord struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
}

// PRDRecord represents a structured product requirement (Persona, Flow, Requirement)
type PRDRecord struct {
	Section     string `json:"section"` // e.g., "personas", "user_journeys", "functional"
	Key         string `json:"key"`     // e.g., "admin_user", "login_flow"
	Content     string `json:"content"` // Detailed markdown or JSON
	Metadata    string `json:"metadata"`
}

type Store struct {
	db *sql.DB
}

func NewStore(projectDir string) (*Store, error) {
	dbPath := filepath.Join(projectDir, ".openexec", "knowledge.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0750); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open knowledge db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Store) migrate() error {
	queries := []string{
		// Symbols Table
		`CREATE TABLE IF NOT EXISTS symbols (
			name TEXT PRIMARY KEY,
			kind TEXT,
			file_path TEXT,
			start_line INTEGER,
			end_line INTEGER,
			purpose TEXT,
			input_params TEXT,
			output_params TEXT,
			signature TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		// Environments Table (Upgraded from deployments)
		`CREATE TABLE IF NOT EXISTS environments (
			env TEXT PRIMARY KEY,
			runtime_type TEXT,
			auth_steps TEXT,
			deploy_steps TEXT,
			topology TEXT,
			instructions TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		// API Docs Table
		`CREATE TABLE IF NOT EXISTS api_docs (
			path TEXT,
			method TEXT,
			request_schema TEXT,
			response_schema TEXT,
			description TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (path, method)
		);`,
		// Policies Table
		`CREATE TABLE IF NOT EXISTS policies (
			key TEXT PRIMARY KEY,
			value TEXT,
			description TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		// PRD Table
		`CREATE TABLE IF NOT EXISTS prd_specs (
			section TEXT,
			key TEXT,
			content TEXT,
			metadata TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (section, key)
		);`,
		// Durable Task Queue
		`CREATE TABLE IF NOT EXISTS task_queue (
			id TEXT PRIMARY KEY,
			type TEXT,
			status TEXT,      -- pending, running, completed, failed
			payload TEXT,     -- JSON input for the agent
			error_log TEXT,
			retries INTEGER DEFAULT 0,
			last_ping DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
	}

	for _, q := range queries {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

// --- Symbol Methods ---

func (s *Store) SetSymbol(r *SymbolRecord) error {
	query := `INSERT OR REPLACE INTO symbols (name, kind, file_path, start_line, end_line, purpose, input_params, output_params, signature) 
	          VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, r.Name, r.Kind, r.FilePath, r.StartLine, r.EndLine, r.Purpose, r.InputParams, r.OutputParams, r.Signature)
	return err
}

func (s *Store) GetSymbol(name string) (*SymbolRecord, error) {
	r := &SymbolRecord{}
	query := `SELECT name, kind, file_path, start_line, end_line, purpose, input_params, output_params, signature FROM symbols WHERE name = ?`
	err := s.db.QueryRow(query, name).Scan(&r.Name, &r.Kind, &r.FilePath, &r.StartLine, &r.EndLine, &r.Purpose, &r.InputParams, &r.OutputParams, &r.Signature)
	if err == sql.ErrNoRows { return nil, nil }
	return r, err
}

func (s *Store) ListSymbols() ([]*SymbolRecord, error) {
	query := `SELECT name, kind, file_path, start_line, end_line, purpose FROM symbols ORDER BY name`
	rows, err := s.db.Query(query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []*SymbolRecord
	for rows.Next() {
		r := &SymbolRecord{}
		if err := rows.Scan(&r.Name, &r.Kind, &r.FilePath, &r.StartLine, &r.EndLine, &r.Purpose); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

func (s *Store) DeleteSymbolsByFile(filePath string) error {
	query := `DELETE FROM symbols WHERE file_path = ?`
	_, err := s.db.Exec(query, filePath)
	return err
}

// --- Environment Methods ---

func (s *Store) SetEnvironment(r *EnvironmentRecord) error {
	query := `INSERT OR REPLACE INTO environments (env, runtime_type, auth_steps, deploy_steps, topology, instructions) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, r.Env, r.RuntimeType, r.AuthSteps, r.DeploySteps, r.Topology, r.Instructions)
	return err
}

func (s *Store) GetEnvironment(env string) (*EnvironmentRecord, error) {
	r := &EnvironmentRecord{}
	query := `SELECT env, runtime_type, auth_steps, deploy_steps, topology, instructions FROM environments WHERE env = ?`
	err := s.db.QueryRow(query, env).Scan(&r.Env, &r.RuntimeType, &r.AuthSteps, &r.DeploySteps, &r.Topology, &r.Instructions)
	if err == sql.ErrNoRows { return nil, nil }
	return r, err
}

func (s *Store) ListEnvironments() ([]*EnvironmentRecord, error) {
	query := `SELECT env, runtime_type, topology FROM environments`
	rows, err := s.db.Query(query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []*EnvironmentRecord
	for rows.Next() {
		r := &EnvironmentRecord{}
		if err := rows.Scan(&r.Env, &r.RuntimeType, &r.Topology); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// --- API Doc Methods ---

func (s *Store) SetAPIDoc(r *APIDocRecord) error {
	query := `INSERT OR REPLACE INTO api_docs (path, method, request_schema, response_schema, description) VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, r.Path, r.Method, r.RequestSchema, r.ResponseSchema, r.Description)
	return err
}

func (s *Store) GetAPIDoc(path, method string) (*APIDocRecord, error) {
	r := &APIDocRecord{}
	query := `SELECT path, method, request_schema, response_schema, description FROM api_docs WHERE path = ? AND method = ?`
	err := s.db.QueryRow(query, path, method).Scan(&r.Path, &r.Method, &r.RequestSchema, &r.ResponseSchema, &r.Description)
	if err == sql.ErrNoRows { return nil, nil }
	return r, err
}

func (s *Store) ListAPIDocs() ([]*APIDocRecord, error) {
	query := `SELECT path, method, description FROM api_docs ORDER BY path`
	rows, err := s.db.Query(query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []*APIDocRecord
	for rows.Next() {
		r := &APIDocRecord{}
		if err := rows.Scan(&r.Path, &r.Method, &r.Description); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// --- Policy Methods ---

func (s *Store) SetPolicy(r *PolicyRecord) error {
	query := `INSERT OR REPLACE INTO policies (key, value, description) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, r.Key, r.Value, r.Description)
	return err
}

func (s *Store) GetPolicy(key string) (*PolicyRecord, error) {
	r := &PolicyRecord{}
	query := `SELECT key, value, description FROM policies WHERE key = ?`
	err := s.db.QueryRow(query, key).Scan(&r.Key, &r.Value, &r.Description)
	if err == sql.ErrNoRows { return nil, nil }
	return r, err
}

func (s *Store) ListPolicies() ([]*PolicyRecord, error) {
	query := `SELECT key, value, description FROM policies ORDER BY key`
	rows, err := s.db.Query(query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []*PolicyRecord
	for rows.Next() {
		r := &PolicyRecord{}
		if err := rows.Scan(&r.Key, &r.Value, &r.Description); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// --- PRD Methods ---

func (s *Store) SetPRDRecord(r *PRDRecord) error {
	query := `INSERT OR REPLACE INTO prd_specs (section, key, content, metadata) VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(query, r.Section, r.Key, r.Content, r.Metadata)
	return err
}

func (s *Store) GetPRDRecord(section, key string) (*PRDRecord, error) {
	r := &PRDRecord{}
	query := `SELECT section, key, content, metadata FROM prd_specs WHERE section = ? AND key = ?`
	err := s.db.QueryRow(query, section, key).Scan(&r.Section, &r.Key, &r.Content, &r.Metadata)
	if err == sql.ErrNoRows { return nil, nil }
	return r, err
}

func (s *Store) ListPRDRecords(section string) ([]*PRDRecord, error) {
	query := `SELECT section, key, content, metadata FROM prd_specs WHERE section = ?`
	rows, err := s.db.Query(query, section)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []*PRDRecord
	for rows.Next() {
		r := &PRDRecord{}
		if err := rows.Scan(&r.Section, &r.Key, &r.Content, &r.Metadata); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}

// --- Queue Methods ---

func (s *Store) EnqueueTask(id, tType, payload string) error {
	query := `INSERT INTO task_queue (id, type, status, payload, last_ping) VALUES (?, ?, 'pending', ?, CURRENT_TIMESTAMP)`
	_, err := s.db.Exec(query, id, tType, payload)
	return err
}

func (s *Store) ClaimTask(workerID string) (id, tType, payload string, err error) {
	query := `UPDATE task_queue SET status = 'running', last_ping = CURRENT_TIMESTAMP WHERE id = (
		SELECT id FROM task_queue WHERE status = 'pending' LIMIT 1
	) RETURNING id, type, payload`
	
	err = s.db.QueryRow(query).Scan(&id, &tType, &payload)
	if err == sql.ErrNoRows {
		return "", "", "", nil
	}
	return
}

func (s *Store) UpdateTaskStatus(id, status, errorLog string) error {
	query := `UPDATE task_queue SET status = ?, error_log = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(query, status, errorLog, id)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
