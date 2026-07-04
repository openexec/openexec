package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ProjectConfig holds project-specific configuration
type ProjectConfig struct {
	Name                string `json:"name"`
	ProjectDir          string `json:"project_dir,omitempty"`
	GitEnabled          bool   `json:"git_enabled,omitempty"`
	GitCommitEnabled    *bool  `json:"git_commit_enabled,omitempty"` // Autonomous local commits; nil-default-true (set false to disable)
	BaseBranch          string `json:"base_branch,omitempty"`
	ReleaseBranchPrefix string `json:"release_branch_prefix,omitempty"` // e.g. "release/"
	FeatureBranchPrefix string `json:"feature_branch_prefix,omitempty"` // e.g. "feature/"

	// Execution settings
	Execution ExecutionConfig `json:"execution,omitempty"`

	// QualityGates holds project-level quality gate toggles and overrides.
	// Fields are additive and backwards compatible: missing values leave
	// gates enabled with default severities.
	QualityGates QualityGatesConfig `json:"quality_gates,omitempty"`

	// Modules gates optional (mostly proprietary) modules. Missing/unset means
	// enabled — existing projects keep every module — so the composition root
	// can turn a module off explicitly without changing default behavior.
	Modules ModulesConfig `json:"modules,omitempty"`
}

// IsGitCommitEnabled reports whether autonomous local commits are allowed. The
// flag is nil-default-TRUE: an absent value enables committing, and only an
// explicit `"git_commit_enabled": false` disables it. (Autonomous local commits
// are safe — they never push or open a PR; those are separate, gated steps.)
func (c *ProjectConfig) IsGitCommitEnabled() bool {
	return c.GitCommitEnabled == nil || *c.GitCommitEnabled
}

// ModulesConfig gates the optional modules layered on the MIT core, keyed by
// module name (e.g. `{"sre": {"enabled": false}}`). Each toggle is
// nil-default-true: an absent entry means the module loads, so this is a
// backwards-compatible opt-OUT, not an opt-in. Being a map, core hardcodes no
// module names — any module a build ships can be gated here.
type ModulesConfig map[string]ModuleToggle

// ModuleToggle configures a single module.
type ModuleToggle struct {
	// Enabled: nil (unset) = enabled; explicit false disables the module.
	Enabled *bool `json:"enabled,omitempty"`
	// License is an opaque entitlement reference passed to EntitlementCheck.
	// Empty in open-core; a proprietary build reads it to verify a purchase.
	License string `json:"license,omitempty"`
}

// EntitlementCheck is the licensing seam. The open-core default permits every
// module; a proprietary build overrides this var to enforce entitlements. It
// runs AFTER the enabled flag, so "disabled by config" and "not licensed" stay
// distinct outcomes. Returning an error prevents the module from loading.
var EntitlementCheck = func(module, license string) error { return nil }

func (m ModulesConfig) toggle(module string) *ModuleToggle {
	if t, ok := m[module]; ok {
		return &t
	}
	return nil
}

// ShouldLoad reports whether the composition root should register a module:
// enabled by config (nil-default-true) AND permitted by EntitlementCheck. The
// returned reason is non-empty only when the module should NOT load.
func (m ModulesConfig) ShouldLoad(module string) (bool, string) {
	t := m.toggle(module)
	if t != nil && t.Enabled != nil && !*t.Enabled {
		return false, "disabled by config"
	}
	license := ""
	if t != nil {
		license = t.License
	}
	if err := EntitlementCheck(module, license); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// QualityGatesConfig configures the project-level quality gates that run
// outside of the openexec.yaml command-based gates.
type QualityGatesConfig struct {
	// NoStubs toggles the no-stubs verifier gate. Pointer with nil-default-true
	// semantics: leave unset to enable; set to false to disable explicitly.
	NoStubs *bool `json:"no_stubs,omitempty"`

	// NoStubsRules maps a no-stubs rule ID to a severity string ("high",
	// "warn", "low", or "off"). Empty map leaves rules at their defaults.
	NoStubsRules map[string]string `json:"no_stubs_rules,omitempty"`

	// ProductionReady toggles the production-readiness checklist gate
	// (env vars documented, no committed secrets, reversible migrations,
	// session checks on API handlers, health endpoint present). Pointer
	// with nil-default-true semantics: leave unset to enable; set to
	// false to disable explicitly.
	ProductionReady *bool `json:"production_ready,omitempty"`

	// ProductionReadySkip lists checklist check IDs to skip when the
	// gate runs. Empty slice runs all default checkers.
	ProductionReadySkip []string `json:"production_ready_skip,omitempty"`
}

// IsNoStubsEnabled reports whether the no-stubs gate should run. Defaults to
// true when the field is unset.
func (q *QualityGatesConfig) IsNoStubsEnabled() bool {
	if q.NoStubs == nil {
		return true
	}
	return *q.NoStubs
}

// IsProductionReadyEnabled reports whether the production-readiness
// checklist gate should run. Defaults to true when the field is unset.
func (q *QualityGatesConfig) IsProductionReadyEnabled() bool {
	if q.ProductionReady == nil {
		return true
	}
	return *q.ProductionReady
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
	// Port is the execution engine port
	Port int `json:"port,omitempty"`
	// ReviewEnabled enables code review after task execution
	ReviewEnabled bool `json:"review_enabled"`
	// ReviewerModel is the model to use for code review
	ReviewerModel string `json:"reviewer_model,omitempty"`
	// WorkerCount is the number of concurrent workers for parallel execution
	WorkerCount int `json:"worker_count,omitempty"`
	// TimeoutSeconds sets the default per-task timeout used by run/start when flags are not provided.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// ExecMode controls the permission level for the AI runner.
	// Values: "suggest" (read-only), "workspace-write" (default), "danger-full-access" (skip all permissions)
	ExecMode string `json:"exec_mode,omitempty"`
	// LintCommands overrides the default lint commands in the blueprint.
	// If empty, the lint stage is skipped (auto-pass).
	LintCommands []string `json:"lint_commands,omitempty"`
	// TestCommands overrides the default test commands in the blueprint.
	// If empty, the test stage is skipped (auto-pass).
	TestCommands []string `json:"test_commands,omitempty"`

	// Feature flags for V2 subsystems
	QualityGatesV2    bool   `json:"quality_gates_v2,omitempty"`
	CacheEnabled      bool   `json:"cache_enabled,omitempty"`
	PredictiveLoad    bool   `json:"predictive_load,omitempty"`
	MemoryEnabled     bool   `json:"memory_enabled,omitempty"`
	CheckpointEnabled bool   `json:"checkpoint_enabled,omitempty"`
	BitNetRouting     bool   `json:"bitnet_routing,omitempty"`
	BitNetModel       string `json:"bitnet_model,omitempty"`
	// ToolsetFiltering, when true, restricts the tool definitions sent to the
	// frontier model on the API path to the toolset selected by the local
	// router. Off by default for safety: see ADR-002. Has no effect on the CLI
	// runner path.
	ToolsetFiltering bool `json:"toolset_filtering,omitempty"`
	// SymbolIndexing controls whether the daemon automatically indexes the
	// project's source code into the knowledge.Store symbols table at startup.
	// Pointer with nil-default-true semantics: leave unset to enable; set to
	// false explicitly to disable. The indexer runs in a background goroutine
	// so startup is not blocked. See ADR-003 (Layer 1).
	SymbolIndexing *bool `json:"symbol_indexing,omitempty"`
	// LocalPreResolve controls whether the pre-resolver pre-pass runs before
	// the implement stage. It extracts symbol references from the task
	// description and injects their signatures + first N lines into the
	// briefing. Pointer with nil-default-true semantics: leave unset to
	// enable; set to false explicitly to disable. See ADR-003 (Layer 2).
	LocalPreResolve *bool `json:"local_pre_resolve,omitempty"`

	// API provider settings (OpenAI-compatible endpoints).
	//
	// Two shapes are supported. Prefer the named-providers shape:
	//
	//   "providers": {
	//     "agentics-personal": { "base_url": "...", "api_key": "$AGENTICSNZ_API_KEY", "model": "..." },
	//     "vllm-local":        { "base_url": "http://localhost:8000/v1", "api_key": "...", "model": "..." }
	//   },
	//   "active_provider": "agentics-personal"
	//
	// The legacy fields (APIProvider/APIBaseURL/APIKey/APIModel) remain readable
	// for older configs. Use ActiveAPI() to resolve the effective endpoint.
	Providers      map[string]ProviderConfig `json:"providers,omitempty"`
	ActiveProvider string                    `json:"active_provider,omitempty"`

	APIProvider string `json:"api_provider,omitempty"` // legacy: "openai_compat", "agenticsnz"
	APIBaseURL  string `json:"api_base_url,omitempty"` // legacy: e.g. "https://api.moonshot.cn/v1"
	APIKey      string `json:"api_key,omitempty"`      // legacy: API key or "$ENV_VAR" reference
	APIModel    string `json:"api_model,omitempty"`    // legacy: e.g. "moonshot-v1-128k"

	// Coordinator settings for multi-agent execution (Phase C)
	CoordinatorModel  string `json:"coordinator_model,omitempty"`   // Frontier model for planning/merging
	WorkerModel       string `json:"worker_model,omitempty"`        // Model for worker agents (can be cheaper)
	WorkerAPIProvider string `json:"worker_api_provider,omitempty"` // Provider for workers (defaults to APIProvider)
	WorkerAPIBaseURL  string `json:"worker_api_base_url,omitempty"` // Base URL for workers (defaults to APIBaseURL)
	WorkerAPIKey      string `json:"worker_api_key,omitempty"`      // API key for workers (defaults to APIKey)
}

// ProviderConfig holds a single named OpenAI-compatible endpoint.
// All entries today are openai_compat — the wire protocol is the same for
// AgenticsNZ, OpenAI, Kimi, vLLM, Ollama, etc.
type ProviderConfig struct {
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
}

// ActiveAPI resolves the currently active API endpoint. It returns the
// provider name, base URL, API key (raw or "$ENV_VAR"), and model.
//
// Resolution order:
//  1. Providers[ActiveProvider] when both are set.
//  2. The single entry in Providers when ActiveProvider is empty.
//  3. The legacy APIProvider/APIBaseURL/APIKey/APIModel fields.
//
// Returns ("", "", "", "") when no API config is present (CLI-runner mode).
func (e *ExecutionConfig) ActiveAPI() (name, baseURL, apiKey, model string) {
	if len(e.Providers) > 0 {
		key := e.ActiveProvider
		if key == "" && len(e.Providers) == 1 {
			for k := range e.Providers {
				key = k
			}
		}
		if entry, ok := e.Providers[key]; ok {
			return key, entry.BaseURL, entry.APIKey, entry.Model
		}
	}
	if e.APIProvider != "" {
		return e.APIProvider, e.APIBaseURL, e.APIKey, e.APIModel
	}
	return "", "", "", ""
}

// IsSymbolIndexingEnabled returns true when the symbol indexer should run.
// The flag is opt-out: nil (the default for existing configs) means enabled,
// and only an explicit `"symbol_indexing": false` disables it.
func (e *ExecutionConfig) IsSymbolIndexingEnabled() bool {
	if e.SymbolIndexing == nil {
		return true
	}
	return *e.SymbolIndexing
}

// IsLocalPreResolveEnabled returns true when the pre-resolver pre-pass should
// run. The flag is opt-out: nil (the default for existing configs) means
// enabled, and only an explicit `"local_pre_resolve": false` disables it.
func (e *ExecutionConfig) IsLocalPreResolveEnabled() bool {
	if e.LocalPreResolve == nil {
		return true
	}
	return *e.LocalPreResolve
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
		Name:       projectName,
		ProjectDir: projectDir,
		GitEnabled: true,
		BaseBranch: "main",
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
	absProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("failed to determine project directory: %w", err)
	}

	// Try .openexec first, then fall back to .uaos for backwards compatibility
	openexecDir := filepath.Join(absProjectDir, ".openexec")
	configFile := filepath.Join(openexecDir, "config.json")

	// Check .openexec/config.json
	if _, err := os.Stat(configFile); err != nil {
		// Try legacy .uaos/project.json
		uaosDir := filepath.Join(absProjectDir, ".uaos")
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

	config.ProjectDir = absProjectDir
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
