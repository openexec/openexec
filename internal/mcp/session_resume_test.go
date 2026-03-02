package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionResumeStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status SessionResumeStatus
		want   bool
	}{
		{"pending is valid", SessionResumeStatusPending, true},
		{"resuming is valid", SessionResumeStatusResuming, true},
		{"resumed is valid", SessionResumeStatusResumed, true},
		{"failed is valid", SessionResumeStatusFailed, true},
		{"expired is valid", SessionResumeStatusExpired, true},
		{"empty is invalid", SessionResumeStatus(""), false},
		{"unknown is invalid", SessionResumeStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("SessionResumeStatus.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSessionResumeStatus_IsFinal(t *testing.T) {
	tests := []struct {
		name   string
		status SessionResumeStatus
		want   bool
	}{
		{"pending is not final", SessionResumeStatusPending, false},
		{"resuming is not final", SessionResumeStatusResuming, false},
		{"resumed is final", SessionResumeStatusResumed, true},
		{"failed is final", SessionResumeStatusFailed, true},
		{"expired is final", SessionResumeStatusExpired, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsFinal(); got != tt.want {
				t.Errorf("SessionResumeStatus.IsFinal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSessionResumeState(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
	}{
		{"valid session ID", "session-123", false},
		{"empty session ID", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, err := NewSessionResumeState(tt.sessionID)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewSessionResumeState() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if state == nil {
					t.Error("NewSessionResumeState() returned nil state")
					return
				}
				if state.ID == "" {
					t.Error("NewSessionResumeState() ID should be set")
				}
				if state.SessionID != tt.sessionID {
					t.Errorf("NewSessionResumeState() SessionID = %v, want %v", state.SessionID, tt.sessionID)
				}
				if state.Status != SessionResumeStatusPending {
					t.Errorf("NewSessionResumeState() Status = %v, want %v", state.Status, SessionResumeStatusPending)
				}
				if state.ExpiresAt.Before(time.Now()) {
					t.Error("NewSessionResumeState() ExpiresAt should be in the future")
				}
			}
		})
	}
}

func TestSessionResumeState_Validate(t *testing.T) {
	tests := []struct {
		name    string
		state   *SessionResumeState
		wantErr bool
	}{
		{
			name: "valid state",
			state: &SessionResumeState{
				ID:        "state-123",
				SessionID: "session-456",
				Status:    SessionResumeStatusPending,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			state: &SessionResumeState{
				ID:        "",
				SessionID: "session-456",
				Status:    SessionResumeStatusPending,
			},
			wantErr: true,
		},
		{
			name: "missing session ID",
			state: &SessionResumeState{
				ID:        "state-123",
				SessionID: "",
				Status:    SessionResumeStatusPending,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			state: &SessionResumeState{
				ID:        "state-123",
				SessionID: "session-456",
				Status:    SessionResumeStatus("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.state.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("SessionResumeState.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSessionResumeState_IsExpired(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		if state.IsExpired() {
			t.Error("IsExpired() = true, want false for fresh state")
		}
	})

	t.Run("expired", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.ExpiresAt = time.Now().Add(-1 * time.Hour) // Set to past
		if !state.IsExpired() {
			t.Error("IsExpired() = false, want true for expired state")
		}
	})
}

func TestSessionResumeState_CanResume(t *testing.T) {
	t.Run("can resume pending non-expired", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		if !state.CanResume() {
			t.Error("CanResume() = false, want true for pending non-expired")
		}
	})

	t.Run("cannot resume expired", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.ExpiresAt = time.Now().Add(-1 * time.Hour)
		if state.CanResume() {
			t.Error("CanResume() = true, want false for expired")
		}
	})

	t.Run("cannot resume resumed", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.Status = SessionResumeStatusResumed
		if state.CanResume() {
			t.Error("CanResume() = true, want false for already resumed")
		}
	})
}

func TestSessionResumeState_StatusTransitions(t *testing.T) {
	t.Run("mark resuming from pending", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		err := state.MarkResuming()
		if err != nil {
			t.Errorf("MarkResuming() error = %v", err)
		}
		if state.Status != SessionResumeStatusResuming {
			t.Errorf("Status = %v, want %v", state.Status, SessionResumeStatusResuming)
		}
	})

	t.Run("mark resuming from expired", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.ExpiresAt = time.Now().Add(-1 * time.Hour)
		err := state.MarkResuming()
		if err != ErrSessionResumeExpired {
			t.Errorf("MarkResuming() error = %v, want %v", err, ErrSessionResumeExpired)
		}
		if state.Status != SessionResumeStatusExpired {
			t.Errorf("Status = %v, want %v", state.Status, SessionResumeStatusExpired)
		}
	})

	t.Run("mark resuming from final state", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.Status = SessionResumeStatusResumed
		err := state.MarkResuming()
		if err != ErrSessionAlreadyResumed {
			t.Errorf("MarkResuming() error = %v, want %v", err, ErrSessionAlreadyResumed)
		}
	})

	t.Run("mark resumed", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.MarkResuming()
		err := state.MarkResumed()
		if err != nil {
			t.Errorf("MarkResumed() error = %v", err)
		}
		if state.Status != SessionResumeStatusResumed {
			t.Errorf("Status = %v, want %v", state.Status, SessionResumeStatusResumed)
		}
		if state.ResumedAt == nil {
			t.Error("ResumedAt should be set")
		}
	})

	t.Run("mark failed", func(t *testing.T) {
		state, _ := NewSessionResumeState("session-123")
		state.MarkFailed("test error")
		if state.Status != SessionResumeStatusFailed {
			t.Errorf("Status = %v, want %v", state.Status, SessionResumeStatusFailed)
		}
		if state.Error != "test error" {
			t.Errorf("Error = %v, want 'test error'", state.Error)
		}
	})
}

func TestSessionResumeState_Messages(t *testing.T) {
	state, _ := NewSessionResumeState("session-123")

	// Test setting messages
	messages := []map[string]string{
		{"role": "user", "content": "Hello"},
		{"role": "assistant", "content": "Hi there!"},
	}

	err := state.SetMessages(messages)
	if err != nil {
		t.Errorf("SetMessages() error = %v", err)
	}

	// Test getting messages
	var retrieved []map[string]string
	err = state.GetMessages(&retrieved)
	if err != nil {
		t.Errorf("GetMessages() error = %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("len(retrieved) = %d, want 2", len(retrieved))
	}
	if retrieved[0]["content"] != "Hello" {
		t.Errorf("retrieved[0][content] = %v, want 'Hello'", retrieved[0]["content"])
	}
}

func TestSessionResumeState_LastSignal(t *testing.T) {
	state, _ := NewSessionResumeState("session-123")

	signal := map[string]interface{}{
		"type":   "progress",
		"target": "task-1",
	}

	err := state.SetLastSignal(signal)
	if err != nil {
		t.Errorf("SetLastSignal() error = %v", err)
	}

	var retrieved map[string]interface{}
	err = state.GetLastSignal(&retrieved)
	if err != nil {
		t.Errorf("GetLastSignal() error = %v", err)
	}

	if retrieved["type"] != "progress" {
		t.Errorf("retrieved[type] = %v, want 'progress'", retrieved["type"])
	}
}

func TestSessionResumeState_Metadata(t *testing.T) {
	state, _ := NewSessionResumeState("session-123")

	state.SetMetadata("key1", "value1")
	state.SetMetadata("key2", "value2")

	value, ok := state.GetMetadata("key1")
	if !ok || value != "value1" {
		t.Errorf("GetMetadata(key1) = %v, %v, want value1, true", value, ok)
	}

	_, ok = state.GetMetadata("nonexistent")
	if ok {
		t.Error("GetMetadata(nonexistent) should return false")
	}
}

func TestDefaultSessionResumeManagerConfig(t *testing.T) {
	config := DefaultSessionResumeManagerConfig()
	if config.StoragePath != ".openexec/resume" {
		t.Errorf("StoragePath = %v, want .openexec/resume", config.StoragePath)
	}
	if config.DefaultExpiry != 24*time.Hour {
		t.Errorf("DefaultExpiry = %v, want 24h", config.DefaultExpiry)
	}
	if config.MaxStatesPerSession != 5 {
		t.Errorf("MaxStatesPerSession = %v, want 5", config.MaxStatesPerSession)
	}
	if !config.AutoCleanup {
		t.Error("AutoCleanup = false, want true")
	}
}

func TestNewSessionResumeManager(t *testing.T) {
	t.Run("with valid config", func(t *testing.T) {
		tempDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath:         filepath.Join(tempDir, "resume"),
			DefaultExpiry:       1 * time.Hour,
			MaxStatesPerSession: 3,
			AutoCleanup:         false,
		}

		manager, err := NewSessionResumeManager(config)
		if err != nil {
			t.Errorf("NewSessionResumeManager() error = %v", err)
		}
		if manager == nil {
			t.Error("NewSessionResumeManager() returned nil")
		}

		// Verify directory was created
		if _, err := os.Stat(config.StoragePath); os.IsNotExist(err) {
			t.Error("Storage directory was not created")
		}
	})

	t.Run("with nil config uses defaults", func(t *testing.T) {
		manager, err := NewSessionResumeManager(nil)
		if err != nil {
			t.Errorf("NewSessionResumeManager() error = %v", err)
		}
		if manager == nil {
			t.Error("NewSessionResumeManager() returned nil")
		}
		manager.Close()
	})
}

func TestSessionResumeManager_CreateResumeState(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath:         filepath.Join(tempDir, "resume"),
		DefaultExpiry:       1 * time.Hour,
		MaxStatesPerSession: 2,
		AutoCleanup:         false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	t.Run("creates state", func(t *testing.T) {
		state, err := manager.CreateResumeState(ctx, "session-123")
		if err != nil {
			t.Errorf("CreateResumeState() error = %v", err)
		}
		if state == nil {
			t.Error("CreateResumeState() returned nil")
			return
		}
		if state.SessionID != "session-123" {
			t.Errorf("SessionID = %v, want session-123", state.SessionID)
		}

		// Verify persisted to disk
		filePath := filepath.Join(config.StoragePath, state.ID+".json")
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			t.Error("State file was not created")
		}
	})

	t.Run("enforces max states per session", func(t *testing.T) {
		// Create max states
		state1, _ := manager.CreateResumeState(ctx, "session-max")
		_, _ = manager.CreateResumeState(ctx, "session-max")

		// Create one more - should evict oldest
		state3, err := manager.CreateResumeState(ctx, "session-max")
		if err != nil {
			t.Errorf("CreateResumeState() error = %v", err)
		}
		if state3 == nil {
			t.Error("CreateResumeState() returned nil")
			return
		}

		// First state should be gone
		_, err = manager.GetResumeState(ctx, state1.ID)
		if err != ErrSessionResumeNotFound {
			t.Errorf("GetResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
		}
	})
}

func TestSessionResumeManager_GetResumeState(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	t.Run("returns existing state", func(t *testing.T) {
		state, _ := manager.CreateResumeState(ctx, "session-123")
		retrieved, err := manager.GetResumeState(ctx, state.ID)
		if err != nil {
			t.Errorf("GetResumeState() error = %v", err)
		}
		if retrieved.ID != state.ID {
			t.Errorf("ID = %v, want %v", retrieved.ID, state.ID)
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		_, err := manager.GetResumeState(ctx, "non-existent")
		if err != ErrSessionResumeNotFound {
			t.Errorf("GetResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
		}
	})
}

func TestSessionResumeManager_GetLatestResumeState(t *testing.T) {
	t.Run("returns latest pending state", func(t *testing.T) {
		tempDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath:         filepath.Join(tempDir, "resume"),
			MaxStatesPerSession: 5,
			AutoCleanup:         false,
		}

		manager, _ := NewSessionResumeManager(config)
		defer manager.Close()
		ctx := context.Background()

		_, _ = manager.CreateResumeState(ctx, "session-123")
		state2, _ := manager.CreateResumeState(ctx, "session-123")

		latest, err := manager.GetLatestResumeState(ctx, "session-123")
		if err != nil {
			t.Fatalf("GetLatestResumeState() error = %v", err)
		}
		if latest.ID != state2.ID {
			t.Errorf("ID = %v, want %v", latest.ID, state2.ID)
		}
	})

	t.Run("skips non-pending states", func(t *testing.T) {
		tempDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath:         filepath.Join(tempDir, "resume"),
			MaxStatesPerSession: 5,
			AutoCleanup:         false,
		}

		manager, _ := NewSessionResumeManager(config)
		defer manager.Close()
		ctx := context.Background()

		state1, _ := manager.CreateResumeState(ctx, "session-skip")
		state2, _ := manager.CreateResumeState(ctx, "session-skip")

		// Mark state2 as resumed
		state2.MarkResuming()
		state2.MarkResumed()
		manager.UpdateResumeState(ctx, state2)

		latest, err := manager.GetLatestResumeState(ctx, "session-skip")
		if err != nil {
			t.Fatalf("GetLatestResumeState() error = %v", err)
		}
		if latest.ID != state1.ID {
			t.Errorf("ID = %v, want %v", latest.ID, state1.ID)
		}
	})

	t.Run("returns error when no pending states", func(t *testing.T) {
		tempDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath:         filepath.Join(tempDir, "resume"),
			MaxStatesPerSession: 5,
			AutoCleanup:         false,
		}

		manager, _ := NewSessionResumeManager(config)
		defer manager.Close()
		ctx := context.Background()

		state, _ := manager.CreateResumeState(ctx, "session-none")
		state.MarkResuming()
		state.MarkResumed()
		manager.UpdateResumeState(ctx, state)

		_, err := manager.GetLatestResumeState(ctx, "session-none")
		if err != ErrNoSessionToResume {
			t.Errorf("GetLatestResumeState() error = %v, want %v", err, ErrNoSessionToResume)
		}
	})

	t.Run("returns error for non-existent session", func(t *testing.T) {
		tempDir := t.TempDir()
		config := &SessionResumeManagerConfig{
			StoragePath: filepath.Join(tempDir, "resume"),
			AutoCleanup: false,
		}

		manager, _ := NewSessionResumeManager(config)
		defer manager.Close()
		ctx := context.Background()

		_, err := manager.GetLatestResumeState(ctx, "non-existent")
		if err != ErrSessionResumeNotFound {
			t.Errorf("GetLatestResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
		}
	})
}

func TestSessionResumeManager_UpdateResumeState(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	t.Run("updates existing state", func(t *testing.T) {
		state, _ := manager.CreateResumeState(ctx, "session-123")
		state.Iteration = 5
		state.TotalTokens = 1000

		err := manager.UpdateResumeState(ctx, state)
		if err != nil {
			t.Errorf("UpdateResumeState() error = %v", err)
		}

		retrieved, _ := manager.GetResumeState(ctx, state.ID)
		if retrieved.Iteration != 5 {
			t.Errorf("Iteration = %v, want 5", retrieved.Iteration)
		}
		if retrieved.TotalTokens != 1000 {
			t.Errorf("TotalTokens = %v, want 1000", retrieved.TotalTokens)
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		state := &SessionResumeState{
			ID:        "non-existent",
			SessionID: "session-123",
			Status:    SessionResumeStatusPending,
		}
		err := manager.UpdateResumeState(ctx, state)
		if err != ErrSessionResumeNotFound {
			t.Errorf("UpdateResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
		}
	})
}

func TestSessionResumeManager_DeleteResumeState(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	t.Run("deletes existing state", func(t *testing.T) {
		state, _ := manager.CreateResumeState(ctx, "session-123")
		filePath := filepath.Join(config.StoragePath, state.ID+".json")

		err := manager.DeleteResumeState(ctx, state.ID)
		if err != nil {
			t.Errorf("DeleteResumeState() error = %v", err)
		}

		// Verify removed from memory
		_, err = manager.GetResumeState(ctx, state.ID)
		if err != ErrSessionResumeNotFound {
			t.Errorf("GetResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
		}

		// Verify removed from disk
		if _, err := os.Stat(filePath); !os.IsNotExist(err) {
			t.Error("State file was not removed")
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		err := manager.DeleteResumeState(ctx, "non-existent")
		if err != ErrSessionResumeNotFound {
			t.Errorf("DeleteResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
		}
	})
}

func TestSessionResumeManager_ListResumeStates(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath:         filepath.Join(tempDir, "resume"),
		MaxStatesPerSession: 10,
		AutoCleanup:         false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	// Create states for multiple sessions
	manager.CreateResumeState(ctx, "session-1")
	manager.CreateResumeState(ctx, "session-1")
	manager.CreateResumeState(ctx, "session-2")

	t.Run("lists states for session", func(t *testing.T) {
		states, err := manager.ListResumeStates(ctx, "session-1")
		if err != nil {
			t.Errorf("ListResumeStates() error = %v", err)
		}
		if len(states) != 2 {
			t.Errorf("len(states) = %d, want 2", len(states))
		}
	})

	t.Run("returns empty for unknown session", func(t *testing.T) {
		states, err := manager.ListResumeStates(ctx, "unknown")
		if err != nil {
			t.Errorf("ListResumeStates() error = %v", err)
		}
		if len(states) != 0 {
			t.Errorf("len(states) = %d, want 0", len(states))
		}
	})
}

func TestSessionResumeManager_ListPendingStates(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	// Create states with different statuses
	state1, _ := manager.CreateResumeState(ctx, "session-1")
	state2, _ := manager.CreateResumeState(ctx, "session-2")

	// Mark state2 as resumed
	state2.MarkResuming()
	state2.MarkResumed()
	manager.UpdateResumeState(ctx, state2)

	pending := manager.ListPendingStates(ctx)
	if len(pending) != 1 {
		t.Errorf("len(pending) = %d, want 1", len(pending))
	}
	if len(pending) > 0 && pending[0].ID != state1.ID {
		t.Errorf("pending[0].ID = %v, want %v", pending[0].ID, state1.ID)
	}
}

func TestSessionResumeManager_ResumeSession(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	t.Run("marks state as resuming", func(t *testing.T) {
		state, _ := manager.CreateResumeState(ctx, "session-123")

		resumed, err := manager.ResumeSession(ctx, state.ID)
		if err != nil {
			t.Errorf("ResumeSession() error = %v", err)
		}
		if resumed.Status != SessionResumeStatusResuming {
			t.Errorf("Status = %v, want %v", resumed.Status, SessionResumeStatusResuming)
		}
	})

	t.Run("returns error for non-existent state", func(t *testing.T) {
		_, err := manager.ResumeSession(ctx, "non-existent")
		if err != ErrSessionResumeNotFound {
			t.Errorf("ResumeSession() error = %v, want %v", err, ErrSessionResumeNotFound)
		}
	})
}

func TestSessionResumeManager_CompleteResume(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	state, _ := manager.CreateResumeState(ctx, "session-123")
	manager.ResumeSession(ctx, state.ID)

	err := manager.CompleteResume(ctx, state.ID)
	if err != nil {
		t.Errorf("CompleteResume() error = %v", err)
	}

	retrieved, _ := manager.GetResumeState(ctx, state.ID)
	if retrieved.Status != SessionResumeStatusResumed {
		t.Errorf("Status = %v, want %v", retrieved.Status, SessionResumeStatusResumed)
	}
}

func TestSessionResumeManager_FailResume(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	state, _ := manager.CreateResumeState(ctx, "session-123")

	err := manager.FailResume(ctx, state.ID, "test error")
	if err != nil {
		t.Errorf("FailResume() error = %v", err)
	}

	retrieved, _ := manager.GetResumeState(ctx, state.ID)
	if retrieved.Status != SessionResumeStatusFailed {
		t.Errorf("Status = %v, want %v", retrieved.Status, SessionResumeStatusFailed)
	}
	if retrieved.Error != "test error" {
		t.Errorf("Error = %v, want 'test error'", retrieved.Error)
	}
}

func TestSessionResumeManager_CleanupExpired(t *testing.T) {
	tempDir := t.TempDir()
	config := &SessionResumeManagerConfig{
		StoragePath: filepath.Join(tempDir, "resume"),
		AutoCleanup: false,
	}

	manager, _ := NewSessionResumeManager(config)
	defer manager.Close()
	ctx := context.Background()

	// Create some states
	state1, _ := manager.CreateResumeState(ctx, "session-1")
	state2, _ := manager.CreateResumeState(ctx, "session-2")

	// Expire state1
	state1.ExpiresAt = time.Now().Add(-1 * time.Hour)
	manager.UpdateResumeState(ctx, state1)

	cleaned := manager.CleanupExpired(ctx)
	if cleaned != 1 {
		t.Errorf("CleanupExpired() = %d, want 1", cleaned)
	}

	// state1 should be gone
	_, err := manager.GetResumeState(ctx, state1.ID)
	if err != ErrSessionResumeNotFound {
		t.Errorf("GetResumeState() error = %v, want %v", err, ErrSessionResumeNotFound)
	}

	// state2 should remain
	_, err = manager.GetResumeState(ctx, state2.ID)
	if err != nil {
		t.Errorf("GetResumeState() error = %v", err)
	}
}

func TestSessionResumeManager_LoadStates(t *testing.T) {
	tempDir := t.TempDir()
	storagePath := filepath.Join(tempDir, "resume")

	// Create a manager and add some states
	config1 := &SessionResumeManagerConfig{
		StoragePath: storagePath,
		AutoCleanup: false,
	}
	manager1, _ := NewSessionResumeManager(config1)
	ctx := context.Background()

	state, _ := manager1.CreateResumeState(ctx, "session-123")
	state.Iteration = 10
	state.TotalTokens = 5000
	manager1.UpdateResumeState(ctx, state)
	manager1.Close()

	// Create a new manager - should load the persisted state
	config2 := &SessionResumeManagerConfig{
		StoragePath: storagePath,
		AutoCleanup: false,
	}
	manager2, _ := NewSessionResumeManager(config2)
	defer manager2.Close()

	loaded, err := manager2.GetResumeState(ctx, state.ID)
	if err != nil {
		t.Errorf("GetResumeState() error = %v", err)
	}
	if loaded.Iteration != 10 {
		t.Errorf("Iteration = %v, want 10", loaded.Iteration)
	}
	if loaded.TotalTokens != 5000 {
		t.Errorf("TotalTokens = %v, want 5000", loaded.TotalTokens)
	}
}

func TestSessionResumeState_JSONSerialization(t *testing.T) {
	state, _ := NewSessionResumeState("session-123")
	state.Iteration = 5
	state.TotalTokens = 1000
	state.Model = "gpt-4"
	state.SetMetadata("key", "value")

	messages := []map[string]string{{"role": "user", "content": "test"}}
	state.SetMessages(messages)

	// Serialize
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Deserialize
	var loaded SessionResumeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if loaded.ID != state.ID {
		t.Errorf("ID = %v, want %v", loaded.ID, state.ID)
	}
	if loaded.SessionID != state.SessionID {
		t.Errorf("SessionID = %v, want %v", loaded.SessionID, state.SessionID)
	}
	if loaded.Iteration != state.Iteration {
		t.Errorf("Iteration = %v, want %v", loaded.Iteration, state.Iteration)
	}
	if loaded.Model != state.Model {
		t.Errorf("Model = %v, want %v", loaded.Model, state.Model)
	}

	// Check messages can be retrieved
	var loadedMessages []map[string]string
	if err := loaded.GetMessages(&loadedMessages); err != nil {
		t.Errorf("GetMessages() error = %v", err)
	}
	if len(loadedMessages) != 1 {
		t.Errorf("len(loadedMessages) = %d, want 1", len(loadedMessages))
	}
}
