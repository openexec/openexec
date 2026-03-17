package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProjectConfig holds project-specific configuration
type ProjectConfig struct {
	Name        string `json:"name"`
	ProjectDir  string `json:"project_dir,omitempty"`
	EngramStore string `json:"engram_store"`
	GitEnabled         bool   `json:"git_enabled,omitempty"`
	GitCommitEnabled   bool   `json:"git_commit_enabled,omitempty"` // Allow autonomous local commits
	GitPushEnabled     bool   `json:"git_push_enabled,omitempty"`   // Allow autonomous remote push on release completion
	BaseBranch         string `json:"base_branch,omitempty"`
	ReleaseBranchPrefix string `json:"release_branch_prefix,omitempty"` // e.g. "release/"
	FeatureBranchPrefix string `json:"feature_branch_prefix,omitempty"` // e.g. "feature/"

	// Execution settings
	Execution ExecutionConfig `json:"execution,omitempty"`
}

// ExecutionConfig holds execution engine settings
type ExecutionConfig struct {
	// PlannerModel is the model to use for the planning phase
	PlannerModel string `json:"planner_model,omitempty"`
	// ExecutorModel is the model to use for task execution
	ExecutorModel string `json:"executor_model,omitempty"`
	// RunnerCommand optionally overrides the loop runner binary (e.g., "claude", "gemini", "codex").
	RunnerCommand string `json:"runner_command,omitempty"`
	// RunnerArgs optionally provides arguments for the runner when RunnerCommand is set.
	RunnerArgs []string `json:"runner_args,omitempty"`
	// ReviewEnabled enables code review after task execution
	ReviewEnabled bool `json:"review_enabled"`
	// ReviewerModel is the model to use for code review
	ReviewerModel string `json:"reviewer_model,omitempty"`
	// MaxIterations is the maximum iterations per task
	MaxIterations int `json:"max_iterations,omitempty"`
	// Port is the execution engine port
	Port int `json:"port,omitempty"`
	// ParallelEnabled enables parallel task execution
	ParallelEnabled bool `json:"parallel_enabled"`
    // WorkerCount is the number of concurrent workers for parallel execution
    WorkerCount int `json:"worker_count,omitempty"`
    // TimeoutSeconds sets the default per-task timeout used by run/start when flags are not provided.
    TimeoutSeconds int `json:"timeout_seconds,omitempty"`
    // ExecMode controls the permission level for the AI runner.
    // Values: "suggest" (read-only), "workspace-write", "danger-full-access" (default, skip all permissions)
    ExecMode string `json:"exec_mode,omitempty"`
    // LintCommands overrides the default lint commands in the blueprint.
    // If empty, the lint stage is skipped (auto-pass).
    LintCommands []string `json:"lint_commands,omitempty"`
    // TestCommands overrides the default test commands in the blueprint.
    // If empty, the test stage is skipped (auto-pass).
    TestCommands []string `json:"test_commands,omitempty"`
}

// Initialize initializes a new OpenExec project
func Initialize(projectName string, projectDir string) (*ProjectConfig, error) {
	// Use provided directory or current working directory
	if projectDir == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to determine project directory: %w", err)
		}
	} else {
		// Ensure absolute path
		var err error
		projectDir, err = filepath.Abs(projectDir)
		if err != nil {
			return nil, fmt.Errorf("failed to determine project directory: %w", err)
		}
	}

	// Validate project name
	if projectName == "" {
		projectName = filepath.Base(projectDir)
	}
	if err := validateProjectName(projectName); err != nil {
		return nil, err
	}

	// Create project directory if it doesn't exist
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create project directory: %w", err)
	}

	// Create .openexec directory structure
	openexecDir := filepath.Join(projectDir, ".openexec")
	if err := os.MkdirAll(openexecDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create .openexec directory: %w", err)
	}

	// Create engram subdirectory
	engramDir := filepath.Join(openexecDir, "engram")
	if err := os.MkdirAll(engramDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create engram directory: %w", err)
	}

	// Create data subdirectory for SQLite audit DB
	dataDir := filepath.Join(openexecDir, "data")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Create project config
	config := &ProjectConfig{
		Name:        projectName,
		ProjectDir:  projectDir,
		EngramStore: ".openexec/engram",
		GitEnabled:  true,
		BaseBranch:  "main",
	}

	// Save config.json
	if err := saveProjectConfig(openexecDir, config); err != nil {
		return nil, fmt.Errorf("failed to save project configuration: %w", err)
	}

	// Also create openexec.yaml for discovery and quality gates
	yamlContent := fmt.Sprintf(`project:
  name: %s
  type: fullstack-webapp

quality:
  gates:
    - lint
`, projectName)
	yamlPath := filepath.Join(projectDir, "openexec.yaml")
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		return nil, fmt.Errorf("failed to create openexec.yaml: %w", err)
	}

	return config, nil
}

// LoadProjectConfig loads the project configuration from .openexec directory
func LoadProjectConfig(projectDir string) (*ProjectConfig, error) {
	// Try .openexec first, then fall back to .uaos for backwards compatibility
	openexecDir := filepath.Join(projectDir, ".openexec")
	configFile := filepath.Join(openexecDir, "config.json")

	// Check .openexec/config.json
	if _, err := os.Stat(configFile); err != nil {
		// Try legacy .uaos/project.json
		uaosDir := filepath.Join(projectDir, ".uaos")
		legacyConfig := filepath.Join(uaosDir, "project.json")
		if _, err := os.Stat(legacyConfig); err == nil {
			configFile = legacyConfig
		} else {
			return nil, fmt.Errorf("project not initialized: run 'openexec init' first")
		}
	}

	// Load configuration from file
	config, err := loadProjectConfigFromFile(configFile)
	if err != nil {
		return nil, err
	}

	config.ProjectDir = projectDir
	return config, nil
}

// validateProjectName validates the project name
func validateProjectName(name string) error {
	if name == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if len(name) > 255 {
		return fmt.Errorf("project name too long (max 255 characters)")
	}
	// Allow alphanumeric, hyphens, underscores
	for _, r := range name {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		isHyphen := r == '-'
		isUnderscore := r == '_'

		if !isLower && !isUpper && !isDigit && !isHyphen && !isUnderscore {
			return fmt.Errorf("project name contains invalid characters: only alphanumeric, hyphens, and underscores allowed")
		}
	}
	return nil
}

// saveProjectConfig saves the project configuration to a JSON file (internal)
func saveProjectConfig(openexecDir string, config *ProjectConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	configFile := filepath.Join(openexecDir, "config.json")
	if err := os.WriteFile(configFile, data, 0o600); err != nil {
		return fmt.Errorf("failed to write project config: %w", err)
	}

	return nil
}

// SaveProjectConfig saves the project configuration (public)
func SaveProjectConfig(config *ProjectConfig) error {
	openexecDir := filepath.Join(config.ProjectDir, ".openexec")
	return saveProjectConfig(openexecDir, config)
}

// loadProjectConfigFromFile loads the project configuration from a JSON file
func loadProjectConfigFromFile(configFile string) (*ProjectConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read project config: %w", err)
	}

	config := &ProjectConfig{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse project config: %w", err)
	}

	return config, nil
}
