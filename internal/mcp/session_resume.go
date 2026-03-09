// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements session resume functionality for orchestrator restarts,
// enabling sessions to continue seamlessly after restart operations.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Common errors for session resume operations.
var (
	ErrSessionResumeNotFound = errors.New("session resume state not found")
	ErrSessionResumeInvalid  = errors.New("invalid session resume state")
	ErrSessionResumeExpired  = errors.New("session resume state expired")
	ErrSessionAlreadyResumed = errors.New("session already resumed")
	ErrNoSessionToResume     = errors.New("no session to resume")
	ErrSessionResumeFailed   = errors.New("session resume failed")
	ErrSessionPersistFailed  = errors.New("session persist failed")
)

// SessionResumeStatus represents the lifecycle status of a session resume state.
type SessionResumeStatus string

const (
	// SessionResumeStatusPending indicates the state is awaiting resume.
	SessionResumeStatusPending SessionResumeStatus = "pending"
	// SessionResumeStatusResuming indicates resume is in progress.
	SessionResumeStatusResuming SessionResumeStatus = "resuming"
	// SessionResumeStatusResumed indicates the session was successfully resumed.
	SessionResumeStatusResumed SessionResumeStatus = "resumed"
	// SessionResumeStatusFailed indicates the resume attempt failed.
	SessionResumeStatusFailed SessionResumeStatus = "failed"
	// SessionResumeStatusExpired indicates the resume state has expired.
	SessionResumeStatusExpired SessionResumeStatus = "expired"
)

// ValidSessionResumeStatuses contains all valid session resume status values.
var ValidSessionResumeStatuses = []SessionResumeStatus{
	SessionResumeStatusPending,
	SessionResumeStatusResuming,
	SessionResumeStatusResumed,
	SessionResumeStatusFailed,
	SessionResumeStatusExpired,
}

// IsValid checks if the status is a valid session resume status value.
func (s SessionResumeStatus) IsValid() bool {
	for _, valid := range ValidSessionResumeStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the session resume status.
func (s SessionResumeStatus) String() string {
	return string(s)
}

// IsFinal returns true if the status is terminal (no further changes expected).
func (s SessionResumeStatus) IsFinal() bool {
	return s == SessionResumeStatusResumed || s == SessionResumeStatusFailed ||
		s == SessionResumeStatusExpired
}

// SessionResumeState captures the state of a session for resume after restart.
// This structure preserves all necessary information to continue a session
// seamlessly after an orchestrator restart.
type SessionResumeState struct {
	// ID is the unique identifier for this resume state (UUID).
	ID string `json:"id"`

	// SessionID is the original session identifier.
	SessionID string `json:"session_id"`

	// RestartRequestID links to the restart request that triggered this state save.
	RestartRequestID string `json:"restart_request_id,omitempty"`

	// Status is the current status of this resume state.
	Status SessionResumeStatus `json:"status"`

	// Iteration is the iteration number when the session was paused.
	Iteration int `json:"iteration"`

	// TotalTokens is the total tokens consumed at the time of pause.
	TotalTokens int `json:"total_tokens"`

	// TotalCostUSD is the total cost in USD at the time of pause.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// Messages is the serialized conversation history.
	// JSON-encoded array of agent.Message.
	Messages json.RawMessage `json:"messages"`

	// MessageCount tracks the number of messages for quick reference.
	MessageCount int `json:"message_count"`

	// LastSignal stores the last received signal if any.
	LastSignal json.RawMessage `json:"last_signal,omitempty"`

	// IterationsSinceProgress tracks iterations without progress signal.
	IterationsSinceProgress int `json:"iterations_since_progress"`

	// Model is the LLM model being used.
	Model string `json:"model"`

	// SystemPrompt is the system prompt for the session.
	SystemPrompt string `json:"system_prompt,omitempty"`

	// ProjectPath is the path for the project context.
	ProjectPath string `json:"project_path,omitempty"`

	// WorkDir is the working directory for tool execution.
	WorkDir string `json:"work_dir,omitempty"`

	// PendingPrompt is any user prompt that was in-flight when restart occurred.
	PendingPrompt string `json:"pending_prompt,omitempty"`

	// ContextSummary stores the latest context summary if summarization was used.
	ContextSummary string `json:"context_summary,omitempty"`

	// ToolsState stores any stateful tool configurations.
	ToolsState json.RawMessage `json:"tools_state,omitempty"`

	// Metadata stores arbitrary key-value pairs for extensibility.
	Metadata map[string]string `json:"metadata,omitempty"`

	// ExpiresAt is when this resume state expires and should no longer be used.
	ExpiresAt time.Time `json:"expires_at"`

	// CreatedAt is when this resume state was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when this resume state was last modified.
	UpdatedAt time.Time `json:"updated_at"`

	// ResumedAt is when the session was resumed (nil if not yet resumed).
	ResumedAt *time.Time `json:"resumed_at,omitempty"`

	// Error stores any error message if resume failed.
	Error string `json:"error,omitempty"`
}

// NewSessionResumeState creates a new SessionResumeState with generated UUID.
func NewSessionResumeState(sessionID string) (*SessionResumeState, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session_id is required", ErrSessionResumeInvalid)
	}

	now := time.Now().UTC()
	// Default expiry of 24 hours
	expiresAt := now.Add(24 * time.Hour)

	return &SessionResumeState{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Status:    SessionResumeStatusPending,
		Metadata:  make(map[string]string),
		ExpiresAt: expiresAt,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// Validate checks if the session resume state has valid field values.
func (s *SessionResumeState) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("%w: id is required", ErrSessionResumeInvalid)
	}
	if s.SessionID == "" {
		return fmt.Errorf("%w: session_id is required", ErrSessionResumeInvalid)
	}
	if !s.Status.IsValid() {
		return fmt.Errorf("%w: invalid status: %s", ErrSessionResumeInvalid, s.Status)
	}
	return nil
}

// IsExpired returns true if the resume state has expired.
func (s *SessionResumeState) IsExpired() bool {
	return time.Now().UTC().After(s.ExpiresAt)
}

// CanResume returns true if this state can be used to resume a session.
func (s *SessionResumeState) CanResume() bool {
	return s.Status == SessionResumeStatusPending && !s.IsExpired()
}

// MarkResuming marks the state as currently resuming.
func (s *SessionResumeState) MarkResuming() error {
	if s.Status.IsFinal() {
		return ErrSessionAlreadyResumed
	}
	if s.IsExpired() {
		s.Status = SessionResumeStatusExpired
		s.UpdatedAt = time.Now().UTC()
		return ErrSessionResumeExpired
	}
	s.Status = SessionResumeStatusResuming
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkResumed marks the state as successfully resumed.
func (s *SessionResumeState) MarkResumed() error {
	if s.Status != SessionResumeStatusResuming && s.Status != SessionResumeStatusPending {
		return fmt.Errorf("cannot mark as resumed from status %s", s.Status)
	}
	now := time.Now().UTC()
	s.Status = SessionResumeStatusResumed
	s.ResumedAt = &now
	s.UpdatedAt = now
	return nil
}

// MarkFailed marks the state as failed with an error message.
func (s *SessionResumeState) MarkFailed(errMsg string) {
	s.Status = SessionResumeStatusFailed
	s.Error = errMsg
	s.UpdatedAt = time.Now().UTC()
}

// SetMessages sets the messages from a slice of any type that can be JSON marshaled.
func (s *SessionResumeState) SetMessages(messages interface{}) error {
	data, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}
	s.Messages = data
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// GetMessages unmarshals the messages into the provided slice.
func (s *SessionResumeState) GetMessages(target interface{}) error {
	if s.Messages == nil {
		return nil
	}
	return json.Unmarshal(s.Messages, target)
}

// SetLastSignal sets the last signal from any type that can be JSON marshaled.
func (s *SessionResumeState) SetLastSignal(signal interface{}) error {
	if signal == nil {
		s.LastSignal = nil
		return nil
	}
	data, err := json.Marshal(signal)
	if err != nil {
		return fmt.Errorf("failed to marshal signal: %w", err)
	}
	s.LastSignal = data
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// GetLastSignal unmarshals the last signal into the provided type.
func (s *SessionResumeState) GetLastSignal(target interface{}) error {
	if s.LastSignal == nil {
		return nil
	}
	return json.Unmarshal(s.LastSignal, target)
}

// SetToolsState sets the tools state from any type that can be JSON marshaled.
func (s *SessionResumeState) SetToolsState(state interface{}) error {
	if state == nil {
		s.ToolsState = nil
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal tools state: %w", err)
	}
	s.ToolsState = data
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// GetToolsState unmarshals the tools state into the provided type.
func (s *SessionResumeState) GetToolsState(target interface{}) error {
	if s.ToolsState == nil {
		return nil
	}
	return json.Unmarshal(s.ToolsState, target)
}

// SetMetadata sets a metadata key-value pair.
func (s *SessionResumeState) SetMetadata(key, value string) {
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}
	s.Metadata[key] = value
	s.UpdatedAt = time.Now().UTC()
}

// GetMetadata retrieves a metadata value by key.
func (s *SessionResumeState) GetMetadata(key string) (string, bool) {
	if s.Metadata == nil {
		return "", false
	}
	value, ok := s.Metadata[key]
	return value, ok
}

// SessionResumeManagerConfig configures the SessionResumeManager.
type SessionResumeManagerConfig struct {
	// StoragePath is the directory where resume states are persisted.
	StoragePath string

	// DefaultExpiry is the default expiry duration for resume states.
	DefaultExpiry time.Duration

	// MaxStatesPerSession limits how many resume states can exist per session.
	MaxStatesPerSession int

	// CleanupInterval is how often to clean up expired states.
	CleanupInterval time.Duration

	// AutoCleanup enables automatic cleanup of expired states.
	AutoCleanup bool
}

// DefaultSessionResumeManagerConfig returns sensible defaults.
func DefaultSessionResumeManagerConfig() *SessionResumeManagerConfig {
	return &SessionResumeManagerConfig{
		StoragePath:         ".openexec/resume",
		DefaultExpiry:       24 * time.Hour,
		MaxStatesPerSession: 5,
		CleanupInterval:     1 * time.Hour,
		AutoCleanup:         true,
	}
}

// SessionResumeManager manages session resume states for orchestrator restarts.
type SessionResumeManager struct {
	config *SessionResumeManagerConfig

	mu     sync.RWMutex
	states map[string]*SessionResumeState // keyed by state ID

	// bySession indexes states by session ID for quick lookup
	bySession map[string][]string // session ID -> list of state IDs

	stopCleanup chan struct{}
}

// NewSessionResumeManager creates a new SessionResumeManager.
func NewSessionResumeManager(config *SessionResumeManagerConfig) (*SessionResumeManager, error) {
	if config == nil {
		config = DefaultSessionResumeManagerConfig()
	}

	// Ensure storage directory exists
	if config.StoragePath != "" {
		if err := os.MkdirAll(config.StoragePath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create storage directory: %w", err)
		}
	}

	m := &SessionResumeManager{
		config:      config,
		states:      make(map[string]*SessionResumeState),
		bySession:   make(map[string][]string),
		stopCleanup: make(chan struct{}),
	}

	// Load existing states from disk
	if err := m.loadStates(); err != nil {
		// Non-fatal, just log
		_ = err
	}

	// Start cleanup goroutine if enabled
	if config.AutoCleanup && config.CleanupInterval > 0 {
		go m.cleanupLoop()
	}

	return m, nil
}

// CreateResumeState creates a new resume state for a session.
func (m *SessionResumeManager) CreateResumeState(ctx context.Context, sessionID string) (*SessionResumeState, error) {
	state, err := NewSessionResumeState(sessionID)
	if err != nil {
		return nil, err
	}

	// Apply expiry from config, defaulting to 24 hours if not set
	expiry := m.config.DefaultExpiry
	if expiry == 0 {
		expiry = 24 * time.Hour
	}
	state.ExpiresAt = time.Now().UTC().Add(expiry)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check max states per session
	if existing, ok := m.bySession[sessionID]; ok {
		if len(existing) >= m.config.MaxStatesPerSession {
			// Remove oldest state
			oldestID := existing[0]
			m.removeStateLocked(oldestID)
		}
	}

	// Store the state
	m.states[state.ID] = state
	m.bySession[sessionID] = append(m.bySession[sessionID], state.ID)

	// Persist to disk
	if err := m.persistState(state); err != nil {
		// Non-fatal but log
		_ = err
	}

	return state, nil
}

// GetResumeState retrieves a resume state by ID.
func (m *SessionResumeManager) GetResumeState(ctx context.Context, id string) (*SessionResumeState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.states[id]
	if !ok {
		return nil, ErrSessionResumeNotFound
	}

	return state, nil
}

// GetLatestResumeState retrieves the most recent pending resume state for a session.
func (m *SessionResumeManager) GetLatestResumeState(ctx context.Context, sessionID string) (*SessionResumeState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateIDs, ok := m.bySession[sessionID]
	if !ok || len(stateIDs) == 0 {
		return nil, ErrSessionResumeNotFound
	}

	// Find the latest pending state
	var latest *SessionResumeState
	for i := len(stateIDs) - 1; i >= 0; i-- {
		state, ok := m.states[stateIDs[i]]
		if ok && state.CanResume() {
			latest = state
			break
		}
	}

	if latest == nil {
		return nil, ErrNoSessionToResume
	}

	return latest, nil
}

// UpdateResumeState updates a resume state.
func (m *SessionResumeManager) UpdateResumeState(ctx context.Context, state *SessionResumeState) error {
	if err := state.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.states[state.ID]; !ok {
		return ErrSessionResumeNotFound
	}

	state.UpdatedAt = time.Now().UTC()
	m.states[state.ID] = state

	// Persist to disk
	if err := m.persistState(state); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	return nil
}

// DeleteResumeState removes a resume state.
func (m *SessionResumeManager) DeleteResumeState(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.removeStateLocked(id)
}

// removeStateLocked removes a state without acquiring the lock.
func (m *SessionResumeManager) removeStateLocked(id string) error {
	state, ok := m.states[id]
	if !ok {
		return ErrSessionResumeNotFound
	}

	// Remove from states map
	delete(m.states, id)

	// Remove from session index
	if stateIDs, ok := m.bySession[state.SessionID]; ok {
		for i, sid := range stateIDs {
			if sid == id {
				m.bySession[state.SessionID] = append(stateIDs[:i], stateIDs[i+1:]...)
				break
			}
		}
		if len(m.bySession[state.SessionID]) == 0 {
			delete(m.bySession, state.SessionID)
		}
	}

	// Remove from disk
	if m.config.StoragePath != "" {
		filePath := filepath.Join(m.config.StoragePath, id+".json")
		os.Remove(filePath)
	}

	return nil
}

// ListResumeStates returns all resume states for a session.
func (m *SessionResumeManager) ListResumeStates(ctx context.Context, sessionID string) ([]*SessionResumeState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stateIDs, ok := m.bySession[sessionID]
	if !ok {
		return []*SessionResumeState{}, nil
	}

	states := make([]*SessionResumeState, 0, len(stateIDs))
	for _, id := range stateIDs {
		if state, ok := m.states[id]; ok {
			states = append(states, state)
		}
	}

	return states, nil
}

// ListPendingStates returns all pending resume states.
func (m *SessionResumeManager) ListPendingStates(ctx context.Context) []*SessionResumeState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var pending []*SessionResumeState
	for _, state := range m.states {
		if state.CanResume() {
			pending = append(pending, state)
		}
	}

	return pending
}

// ResumeSession marks a state as resuming and returns it for restoration.
func (m *SessionResumeManager) ResumeSession(ctx context.Context, stateID string) (*SessionResumeState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[stateID]
	if !ok {
		return nil, ErrSessionResumeNotFound
	}

	if err := state.MarkResuming(); err != nil {
		return nil, err
	}

	if err := m.persistState(state); err != nil {
		return nil, fmt.Errorf("failed to persist state: %w", err)
	}

	return state, nil
}

// CompleteResume marks a resume as successfully completed.
func (m *SessionResumeManager) CompleteResume(ctx context.Context, stateID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[stateID]
	if !ok {
		return ErrSessionResumeNotFound
	}

	if err := state.MarkResumed(); err != nil {
		return err
	}

	if err := m.persistState(state); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	return nil
}

// FailResume marks a resume as failed.
func (m *SessionResumeManager) FailResume(ctx context.Context, stateID, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.states[stateID]
	if !ok {
		return ErrSessionResumeNotFound
	}

	state.MarkFailed(errMsg)

	if err := m.persistState(state); err != nil {
		return fmt.Errorf("failed to persist state: %w", err)
	}

	return nil
}

// CleanupExpired removes all expired resume states.
func (m *SessionResumeManager) CleanupExpired(ctx context.Context) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	var toRemove []string
	for id, state := range m.states {
		if state.IsExpired() {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		m.removeStateLocked(id)
	}

	return len(toRemove)
}

// cleanupLoop runs periodic cleanup of expired states.
func (m *SessionResumeManager) cleanupLoop() {
	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.CleanupExpired(context.Background())
		case <-m.stopCleanup:
			return
		}
	}
}

// Close stops the cleanup loop and persists all states.
func (m *SessionResumeManager) Close() error {
	close(m.stopCleanup)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Persist all states
	for _, state := range m.states {
		if err := m.persistState(state); err != nil {
			// Continue on error, try to save as many as possible
			_ = err
		}
	}

	return nil
}

// persistState saves a state to disk.
func (m *SessionResumeManager) persistState(state *SessionResumeState) error {
	if m.config.StoragePath == "" {
		return nil
	}

	filePath := filepath.Join(m.config.StoragePath, state.ID+".json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// loadStates loads all states from disk.
func (m *SessionResumeManager) loadStates() error {
	if m.config.StoragePath == "" {
		return nil
	}

	entries, err := os.ReadDir(m.config.StoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read storage directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(m.config.StoragePath, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var state SessionResumeState
		if err := json.Unmarshal(data, &state); err != nil {
			continue
		}

		// Validate and add to maps
		if err := state.Validate(); err != nil {
			continue
		}

		m.states[state.ID] = &state
		m.bySession[state.SessionID] = append(m.bySession[state.SessionID], state.ID)
	}

	return nil
}

// GetConfig returns the current configuration.
func (m *SessionResumeManager) GetConfig() *SessionResumeManagerConfig {
	return m.config
}
