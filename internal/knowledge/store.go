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

func (s *Store) Close() error {
	return s.db.Close()
}
