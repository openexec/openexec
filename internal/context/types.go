// Package context provides automatic context gathering and injection for AI agent sessions.
// It defines schemas for context items that are automatically collected from the project
// workspace and injected into the conversation to provide relevant background information.
package context

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Common errors returned by context operations.
var (
	ErrContextItemNotFound  = errors.New("context item not found")
	ErrGathererNotFound     = errors.New("gatherer not found")
	ErrInvalidContextItem   = errors.New("invalid context item data")
	ErrInvalidGatherer      = errors.New("invalid gatherer data")
	ErrInvalidContextConfig = errors.New("invalid context config data")
	ErrTokenBudgetExceeded  = errors.New("token budget exceeded")
)

// ContextType represents the category of context being gathered.
type ContextType string

const (
	// ContextTypeProjectInstructions is CLAUDE.md or similar project instruction files.
	ContextTypeProjectInstructions ContextType = "project_instructions"
	// ContextTypeGitStatus is the current git repository status.
	ContextTypeGitStatus ContextType = "git_status"
	// ContextTypeGitDiff is recent git diff information.
	ContextTypeGitDiff ContextType = "git_diff"
	// ContextTypeGitLog is recent commit history.
	ContextTypeGitLog ContextType = "git_log"
	// ContextTypeDirectoryStructure is the project directory tree.
	ContextTypeDirectoryStructure ContextType = "directory_structure"
	// ContextTypeRecentFiles is recently modified files.
	ContextTypeRecentFiles ContextType = "recent_files"
	// ContextTypeOpenFiles is currently open files in the session.
	ContextTypeOpenFiles ContextType = "open_files"
	// ContextTypePackageInfo is package.json, go.mod, requirements.txt, etc.
	ContextTypePackageInfo ContextType = "package_info"
	// ContextTypeEnvironment is environment information (OS, runtime, etc.).
	ContextTypeEnvironment ContextType = "environment"
	// ContextTypeSessionSummary is a summary of previous conversation.
	ContextTypeSessionSummary ContextType = "session_summary"
	// ContextTypeCustom is user-defined custom context.
	ContextTypeCustom ContextType = "custom"
)

// ValidContextTypes contains all valid context type values.
var ValidContextTypes = []ContextType{
	ContextTypeProjectInstructions,
	ContextTypeGitStatus,
	ContextTypeGitDiff,
	ContextTypeGitLog,
	ContextTypeDirectoryStructure,
	ContextTypeRecentFiles,
	ContextTypeOpenFiles,
	ContextTypePackageInfo,
	ContextTypeEnvironment,
	ContextTypeSessionSummary,
	ContextTypeCustom,
}

// IsValid checks if the context type is a valid value.
func (t ContextType) IsValid() bool {
	for _, valid := range ValidContextTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the context type.
func (t ContextType) String() string {
	return string(t)
}

// Priority represents the injection priority of a context item.
// Higher priority items are included first when token budgets are limited.
type Priority int

const (
	// PriorityCritical is always included first (e.g., project instructions).
	PriorityCritical Priority = 100
	// PriorityHigh is included if budget allows (e.g., git status).
	PriorityHigh Priority = 75
	// PriorityMedium is included if there's room (e.g., recent files).
	PriorityMedium Priority = 50
	// PriorityLow is included only if plenty of budget remains.
	PriorityLow Priority = 25
	// PriorityOptional is included only if explicitly requested or budget permits.
	PriorityOptional Priority = 10
)

// DefaultPriorityForType returns the default priority for a context type.
func DefaultPriorityForType(t ContextType) Priority {
	switch t {
	case ContextTypeProjectInstructions:
		return PriorityCritical
	case ContextTypeSessionSummary:
		return PriorityCritical
	case ContextTypeGitStatus:
		return PriorityHigh
	case ContextTypeEnvironment:
		return PriorityHigh
	case ContextTypePackageInfo:
		return PriorityMedium
	case ContextTypeDirectoryStructure:
		return PriorityMedium
	case ContextTypeRecentFiles:
		return PriorityMedium
	case ContextTypeGitDiff:
		return PriorityLow
	case ContextTypeGitLog:
		return PriorityLow
	case ContextTypeOpenFiles:
		return PriorityLow
	case ContextTypeCustom:
		return PriorityMedium
	default:
		return PriorityOptional
	}
}

// GathererStatus represents the execution status of a context gatherer.
type GathererStatus string

const (
	// GathererStatusIdle indicates the gatherer is not running.
	GathererStatusIdle GathererStatus = "idle"
	// GathererStatusRunning indicates the gatherer is currently executing.
	GathererStatusRunning GathererStatus = "running"
	// GathererStatusCompleted indicates the gatherer finished successfully.
	GathererStatusCompleted GathererStatus = "completed"
	// GathererStatusFailed indicates the gatherer failed with an error.
	GathererStatusFailed GathererStatus = "failed"
	// GathererStatusDisabled indicates the gatherer has been disabled.
	GathererStatusDisabled GathererStatus = "disabled"
)

// ValidGathererStatuses contains all valid gatherer status values.
var ValidGathererStatuses = []GathererStatus{
	GathererStatusIdle,
	GathererStatusRunning,
	GathererStatusCompleted,
	GathererStatusFailed,
	GathererStatusDisabled,
}

// IsValid checks if the status is a valid gatherer status value.
func (s GathererStatus) IsValid() bool {
	for _, valid := range ValidGathererStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the gatherer status.
func (s GathererStatus) String() string {
	return string(s)
}

// ContextItem represents a single piece of context gathered from the project.
type ContextItem struct {
	// ID is the unique identifier for the context item (UUID).
	ID string `json:"id"`
	// SessionID is the session this context belongs to (optional, empty for global cache).
	SessionID sql.NullString `json:"session_id,omitempty"`
	// Type is the category of context.
	Type ContextType `json:"type"`
	// Source identifies where the context was gathered from (file path, command, etc.).
	Source string `json:"source"`
	// Content is the actual context content.
	Content string `json:"content"`
	// ContentHash is a hash of the content for change detection.
	ContentHash string `json:"content_hash"`
	// TokenCount is the estimated token count for this content.
	TokenCount int `json:"token_count"`
	// Priority determines injection order when budget is limited.
	Priority Priority `json:"priority"`
	// Metadata contains additional gatherer-specific data as JSON.
	Metadata string `json:"metadata,omitempty"`
	// IsStale indicates if the content may be outdated.
	IsStale bool `json:"is_stale"`
	// GatheredAt is when this context was collected.
	GatheredAt time.Time `json:"gathered_at"`
	// ExpiresAt is when this context should be refreshed (optional).
	ExpiresAt sql.NullTime `json:"expires_at,omitempty"`
	// CreatedAt is when this record was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this record was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewContextItem creates a new ContextItem with a generated UUID.
func NewContextItem(contextType ContextType, source, content string, tokenCount int) (*ContextItem, error) {
	if !contextType.IsValid() {
		return nil, fmt.Errorf("%w: invalid context type: %s", ErrInvalidContextItem, contextType)
	}
	if source == "" {
		return nil, fmt.Errorf("%w: source is required", ErrInvalidContextItem)
	}
	if tokenCount < 0 {
		return nil, fmt.Errorf("%w: token_count must be non-negative", ErrInvalidContextItem)
	}

	now := time.Now().UTC()
	return &ContextItem{
		ID:          uuid.New().String(),
		Type:        contextType,
		Source:      source,
		Content:     content,
		ContentHash: computeContentHash(content),
		TokenCount:  tokenCount,
		Priority:    DefaultPriorityForType(contextType),
		IsStale:     false,
		GatheredAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Validate checks if the context item has valid field values.
func (c *ContextItem) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidContextItem)
	}
	if !c.Type.IsValid() {
		return fmt.Errorf("%w: invalid context type: %s", ErrInvalidContextItem, c.Type)
	}
	if c.Source == "" {
		return fmt.Errorf("%w: source is required", ErrInvalidContextItem)
	}
	if c.TokenCount < 0 {
		return fmt.Errorf("%w: token_count must be non-negative", ErrInvalidContextItem)
	}
	return nil
}

// SetSessionID associates this context item with a session.
func (c *ContextItem) SetSessionID(sessionID string) {
	c.SessionID = sql.NullString{String: sessionID, Valid: sessionID != ""}
	c.UpdatedAt = time.Now().UTC()
}

// SetExpiration sets when this context should be refreshed.
func (c *ContextItem) SetExpiration(expiresAt time.Time) {
	c.ExpiresAt = sql.NullTime{Time: expiresAt.UTC(), Valid: true}
	c.UpdatedAt = time.Now().UTC()
}

// MarkStale marks the context item as potentially outdated.
func (c *ContextItem) MarkStale() {
	c.IsStale = true
	c.UpdatedAt = time.Now().UTC()
}

// Refresh updates the context content and resets the stale flag.
func (c *ContextItem) Refresh(content string, tokenCount int) {
	c.Content = content
	c.ContentHash = computeContentHash(content)
	c.TokenCount = tokenCount
	c.IsStale = false
	c.GatheredAt = time.Now().UTC()
	c.UpdatedAt = time.Now().UTC()
}

// IsExpired returns true if the context item has passed its expiration time.
func (c *ContextItem) IsExpired() bool {
	if !c.ExpiresAt.Valid {
		return false
	}
	return time.Now().UTC().After(c.ExpiresAt.Time)
}

// NeedsRefresh returns true if the context should be re-gathered.
func (c *ContextItem) NeedsRefresh() bool {
	return c.IsStale || c.IsExpired()
}

// SetMetadata sets the JSON metadata for this context item.
func (c *ContextItem) SetMetadata(metadata string) {
	c.Metadata = metadata
	c.UpdatedAt = time.Now().UTC()
}

// SetPriority updates the injection priority.
func (c *ContextItem) SetPriority(priority Priority) {
	c.Priority = priority
	c.UpdatedAt = time.Now().UTC()
}

// GathererConfig defines the configuration for a context gatherer.
type GathererConfig struct {
	// ID is the unique identifier for the gatherer config (UUID).
	ID string `json:"id"`
	// ProjectPath restricts this config to a specific project (empty = global).
	ProjectPath sql.NullString `json:"project_path,omitempty"`
	// Type is the context type this gatherer produces.
	Type ContextType `json:"type"`
	// Name is a human-readable name for the gatherer.
	Name string `json:"name"`
	// Description explains what this gatherer collects.
	Description string `json:"description,omitempty"`
	// Priority is the default priority for items from this gatherer.
	Priority Priority `json:"priority"`
	// MaxTokens is the maximum tokens this gatherer should use.
	MaxTokens int `json:"max_tokens"`
	// RefreshIntervalSeconds is how often to refresh the context.
	RefreshIntervalSeconds int `json:"refresh_interval_seconds"`
	// Command is the shell command to execute (for command-based gatherers).
	Command sql.NullString `json:"command,omitempty"`
	// FilePaths are the files to read (for file-based gatherers).
	FilePaths string `json:"file_paths,omitempty"` // JSON array
	// FilePatterns are glob patterns to match (for pattern-based gatherers).
	FilePatterns string `json:"file_patterns,omitempty"` // JSON array
	// Options contains gatherer-specific options as JSON.
	Options string `json:"options,omitempty"`
	// IsEnabled indicates if this gatherer is active.
	IsEnabled bool `json:"is_enabled"`
	// CreatedAt is when this config was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this config was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewGathererConfig creates a new GathererConfig with a generated UUID.
func NewGathererConfig(contextType ContextType, name string, maxTokens int) (*GathererConfig, error) {
	if !contextType.IsValid() {
		return nil, fmt.Errorf("%w: invalid context type: %s", ErrInvalidGatherer, contextType)
	}
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidGatherer)
	}
	if maxTokens < 0 {
		return nil, fmt.Errorf("%w: max_tokens must be non-negative", ErrInvalidGatherer)
	}

	now := time.Now().UTC()
	return &GathererConfig{
		ID:                     uuid.New().String(),
		Type:                   contextType,
		Name:                   name,
		Priority:               DefaultPriorityForType(contextType),
		MaxTokens:              maxTokens,
		RefreshIntervalSeconds: 300, // 5 minutes default
		IsEnabled:              true,
		CreatedAt:              now,
		UpdatedAt:              now,
	}, nil
}

// Validate checks if the gatherer config has valid field values.
func (g *GathererConfig) Validate() error {
	if g.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidGatherer)
	}
	if !g.Type.IsValid() {
		return fmt.Errorf("%w: invalid context type: %s", ErrInvalidGatherer, g.Type)
	}
	if g.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidGatherer)
	}
	if g.MaxTokens < 0 {
		return fmt.Errorf("%w: max_tokens must be non-negative", ErrInvalidGatherer)
	}
	if g.RefreshIntervalSeconds < 0 {
		return fmt.Errorf("%w: refresh_interval_seconds must be non-negative", ErrInvalidGatherer)
	}
	return nil
}

// SetProjectPath restricts this gatherer to a specific project.
func (g *GathererConfig) SetProjectPath(path string) {
	g.ProjectPath = sql.NullString{String: path, Valid: path != ""}
	g.UpdatedAt = time.Now().UTC()
}

// SetCommand sets the shell command for command-based gatherers.
func (g *GathererConfig) SetCommand(command string) {
	g.Command = sql.NullString{String: command, Valid: command != ""}
	g.UpdatedAt = time.Now().UTC()
}

// SetDescription sets the gatherer description.
func (g *GathererConfig) SetDescription(description string) {
	g.Description = description
	g.UpdatedAt = time.Now().UTC()
}

// SetOptions sets the JSON options for this gatherer.
func (g *GathererConfig) SetOptions(options string) {
	g.Options = options
	g.UpdatedAt = time.Now().UTC()
}

// Enable activates the gatherer.
func (g *GathererConfig) Enable() {
	g.IsEnabled = true
	g.UpdatedAt = time.Now().UTC()
}

// Disable deactivates the gatherer.
func (g *GathererConfig) Disable() {
	g.IsEnabled = false
	g.UpdatedAt = time.Now().UTC()
}

// GetRefreshInterval returns the refresh interval as a time.Duration.
func (g *GathererConfig) GetRefreshInterval() time.Duration {
	return time.Duration(g.RefreshIntervalSeconds) * time.Second
}

// ContextBudget defines token budget constraints for context injection.
type ContextBudget struct {
	// ID is the unique identifier for the budget config (UUID).
	ID string `json:"id"`
	// ProjectPath restricts this budget to a specific project (empty = global).
	ProjectPath sql.NullString `json:"project_path,omitempty"`
	// TotalTokenBudget is the maximum tokens for all context combined.
	TotalTokenBudget int `json:"total_token_budget"`
	// ReservedForSystemPrompt is tokens reserved for the base system prompt.
	ReservedForSystemPrompt int `json:"reserved_for_system_prompt"`
	// ReservedForConversation is tokens reserved for conversation history.
	ReservedForConversation int `json:"reserved_for_conversation"`
	// MaxPerType limits tokens per context type (JSON map: type -> max tokens).
	MaxPerType string `json:"max_per_type,omitempty"`
	// MinPriorityToInclude is the minimum priority required for inclusion.
	MinPriorityToInclude Priority `json:"min_priority_to_include"`
	// IsDefault indicates if this is the fallback budget.
	IsDefault bool `json:"is_default"`
	// CreatedAt is when this budget was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when this budget was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewContextBudget creates a new ContextBudget with a generated UUID.
func NewContextBudget(totalTokenBudget int) (*ContextBudget, error) {
	if totalTokenBudget < 0 {
		return nil, fmt.Errorf("%w: total_token_budget must be non-negative", ErrInvalidContextConfig)
	}

	now := time.Now().UTC()
	return &ContextBudget{
		ID:                      uuid.New().String(),
		TotalTokenBudget:        totalTokenBudget,
		ReservedForSystemPrompt: 1000, // Default: 1K tokens
		ReservedForConversation: 4000, // Default: 4K tokens
		MinPriorityToInclude:    PriorityOptional,
		CreatedAt:               now,
		UpdatedAt:               now,
	}, nil
}

// Validate checks if the context budget has valid field values.
func (b *ContextBudget) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidContextConfig)
	}
	if b.TotalTokenBudget < 0 {
		return fmt.Errorf("%w: total_token_budget must be non-negative", ErrInvalidContextConfig)
	}
	if b.ReservedForSystemPrompt < 0 {
		return fmt.Errorf("%w: reserved_for_system_prompt must be non-negative", ErrInvalidContextConfig)
	}
	if b.ReservedForConversation < 0 {
		return fmt.Errorf("%w: reserved_for_conversation must be non-negative", ErrInvalidContextConfig)
	}
	return nil
}

// AvailableForContext returns the tokens available for context after reservations.
func (b *ContextBudget) AvailableForContext() int {
	available := b.TotalTokenBudget - b.ReservedForSystemPrompt - b.ReservedForConversation
	if available < 0 {
		return 0
	}
	return available
}

// SetProjectPath restricts this budget to a specific project.
func (b *ContextBudget) SetProjectPath(path string) {
	b.ProjectPath = sql.NullString{String: path, Valid: path != ""}
	b.UpdatedAt = time.Now().UTC()
}

// SetMaxPerType sets the per-type token limits as JSON.
func (b *ContextBudget) SetMaxPerType(maxPerType string) {
	b.MaxPerType = maxPerType
	b.UpdatedAt = time.Now().UTC()
}

// GathererExecution records a single execution of a context gatherer.
type GathererExecution struct {
	// ID is the unique identifier for the execution (UUID).
	ID string `json:"id"`
	// GathererID links to the gatherer config.
	GathererID string `json:"gatherer_id"`
	// SessionID is the session that triggered this execution (optional).
	SessionID sql.NullString `json:"session_id,omitempty"`
	// Status is the execution status.
	Status GathererStatus `json:"status"`
	// ContextItemID is the resulting context item (if successful).
	ContextItemID sql.NullString `json:"context_item_id,omitempty"`
	// TokensGathered is the number of tokens in the result.
	TokensGathered int `json:"tokens_gathered"`
	// DurationMs is the execution time in milliseconds.
	DurationMs int64 `json:"duration_ms"`
	// Error contains the error message if execution failed.
	Error sql.NullString `json:"error,omitempty"`
	// StartedAt is when execution started.
	StartedAt time.Time `json:"started_at"`
	// CompletedAt is when execution finished.
	CompletedAt sql.NullTime `json:"completed_at,omitempty"`
}

// NewGathererExecution creates a new GathererExecution with a generated UUID.
func NewGathererExecution(gathererID string) (*GathererExecution, error) {
	if gathererID == "" {
		return nil, fmt.Errorf("%w: gatherer_id is required", ErrInvalidGatherer)
	}

	return &GathererExecution{
		ID:         uuid.New().String(),
		GathererID: gathererID,
		Status:     GathererStatusRunning,
		StartedAt:  time.Now().UTC(),
	}, nil
}

// Validate checks if the gatherer execution has valid field values.
func (e *GathererExecution) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidGatherer)
	}
	if e.GathererID == "" {
		return fmt.Errorf("%w: gatherer_id is required", ErrInvalidGatherer)
	}
	if !e.Status.IsValid() {
		return fmt.Errorf("%w: invalid status: %s", ErrInvalidGatherer, e.Status)
	}
	return nil
}

// SetSessionID associates this execution with a session.
func (e *GathererExecution) SetSessionID(sessionID string) {
	e.SessionID = sql.NullString{String: sessionID, Valid: sessionID != ""}
}

// Complete marks the execution as completed successfully.
func (e *GathererExecution) Complete(contextItemID string, tokensGathered int) {
	now := time.Now().UTC()
	e.Status = GathererStatusCompleted
	e.ContextItemID = sql.NullString{String: contextItemID, Valid: contextItemID != ""}
	e.TokensGathered = tokensGathered
	e.CompletedAt = sql.NullTime{Time: now, Valid: true}
	e.DurationMs = now.Sub(e.StartedAt).Milliseconds()
}

// Fail marks the execution as failed with an error.
func (e *GathererExecution) Fail(err string) {
	now := time.Now().UTC()
	e.Status = GathererStatusFailed
	e.Error = sql.NullString{String: err, Valid: true}
	e.CompletedAt = sql.NullTime{Time: now, Valid: true}
	e.DurationMs = now.Sub(e.StartedAt).Milliseconds()
}

// computeContentHash computes a hash of the content for change detection.
// Using a simple approach here; in production, use crypto/sha256.
func computeContentHash(content string) string {
	if content == "" {
		return ""
	}
	// Simple hash using fnv for now - can be upgraded to sha256 if needed
	var h uint64 = 14695981039346656037 // FNV-1a offset basis
	for i := 0; i < len(content); i++ {
		h ^= uint64(content[i])
		h *= 1099511628211 // FNV-1a prime
	}
	return fmt.Sprintf("%016x", h)
}

// DefaultGathererConfigs returns the default set of gatherer configurations.
func DefaultGathererConfigs() []GathererConfig {
	configs := []GathererConfig{
		{
			Type:                   ContextTypeProjectInstructions,
			Name:                   "Project Instructions",
			Description:            "Reads CLAUDE.md and similar instruction files",
			Priority:               PriorityCritical,
			MaxTokens:              8000,
			RefreshIntervalSeconds: 300,
			FilePaths:              `["CLAUDE.md", ".claude/CLAUDE.md", "INSTRUCTIONS.md", ".github/INSTRUCTIONS.md"]`,
			IsEnabled:              true,
		},
		{
			Type:                   ContextTypeGitStatus,
			Name:                   "Git Status",
			Description:            "Gathers current git repository status",
			Priority:               PriorityHigh,
			MaxTokens:              2000,
			RefreshIntervalSeconds: 30,
			IsEnabled:              true,
		},
		{
			Type:                   ContextTypeEnvironment,
			Name:                   "Environment Info",
			Description:            "Collects OS, platform, and runtime information",
			Priority:               PriorityHigh,
			MaxTokens:              500,
			RefreshIntervalSeconds: 3600,
			IsEnabled:              true,
		},
		{
			Type:                   ContextTypePackageInfo,
			Name:                   "Package Info",
			Description:            "Reads package.json, go.mod, requirements.txt, etc.",
			Priority:               PriorityMedium,
			MaxTokens:              2000,
			RefreshIntervalSeconds: 300,
			FilePaths:              `["package.json", "go.mod", "requirements.txt", "Cargo.toml", "pom.xml"]`,
			IsEnabled:              true,
		},
		{
			Type:                   ContextTypeDirectoryStructure,
			Name:                   "Directory Structure",
			Description:            "Generates project directory tree",
			Priority:               PriorityMedium,
			MaxTokens:              3000,
			RefreshIntervalSeconds: 120,
			Options:                `{"max_depth": 4, "exclude": ["node_modules", ".git", "__pycache__", "vendor"]}`,
			IsEnabled:              true,
		},
		{
			Type:                   ContextTypeRecentFiles,
			Name:                   "Recent Files",
			Description:            "Lists recently modified files",
			Priority:               PriorityMedium,
			MaxTokens:              1000,
			RefreshIntervalSeconds: 60,
			Options:                `{"max_files": 20, "max_age_hours": 24}`,
			IsEnabled:              true,
		},
		{
			Type:                   ContextTypeGitDiff,
			Name:                   "Git Diff",
			Description:            "Shows unstaged changes in the repository",
			Priority:               PriorityLow,
			MaxTokens:              4000,
			RefreshIntervalSeconds: 60,
			IsEnabled:              false, // Disabled by default to save tokens
		},
		{
			Type:                   ContextTypeGitLog,
			Name:                   "Git Log",
			Description:            "Shows recent commit history",
			Priority:               PriorityLow,
			MaxTokens:              2000,
			RefreshIntervalSeconds: 120,
			Options:                `{"max_commits": 10}`,
			IsEnabled:              false, // Disabled by default
		},
	}

	// Generate UUIDs and set timestamps for each config
	now := time.Now().UTC()
	for i := range configs {
		configs[i].ID = uuid.New().String()
		configs[i].CreatedAt = now
		configs[i].UpdatedAt = now
	}

	return configs
}

// DefaultContextBudget returns the default token budget configuration.
func DefaultContextBudget() *ContextBudget {
	now := time.Now().UTC()
	return &ContextBudget{
		ID:                      uuid.New().String(),
		TotalTokenBudget:        128000, // 128K tokens for modern models
		ReservedForSystemPrompt: 2000,
		ReservedForConversation: 32000, // Reserve 32K for conversation
		MinPriorityToInclude:    PriorityOptional,
		IsDefault:               true,
		CreatedAt:               now,
		UpdatedAt:               now,
	}
}
