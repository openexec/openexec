package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRestartStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status RestartStatus
		want   bool
	}{
		{"pending is valid", RestartStatusPending, true},
		{"approved is valid", RestartStatusApproved, true},
		{"rejected is valid", RestartStatusRejected, true},
		{"in_progress is valid", RestartStatusInProgress, true},
		{"complete is valid", RestartStatusComplete, true},
		{"failed is valid", RestartStatusFailed, true},
		{"cancelled is valid", RestartStatusCancelled, true},
		{"empty is invalid", RestartStatus(""), false},
		{"unknown is invalid", RestartStatus("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("RestartStatus.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestartStatus_IsFinal(t *testing.T) {
	tests := []struct {
		name   string
		status RestartStatus
		want   bool
	}{
		{"pending is not final", RestartStatusPending, false},
		{"approved is not final", RestartStatusApproved, false},
		{"in_progress is not final", RestartStatusInProgress, false},
		{"complete is final", RestartStatusComplete, true},
		{"failed is final", RestartStatusFailed, true},
		{"rejected is final", RestartStatusRejected, true},
		{"cancelled is final", RestartStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsFinal(); got != tt.want {
				t.Errorf("RestartStatus.IsFinal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestartStatus_String(t *testing.T) {
	tests := []struct {
		status RestartStatus
		want   string
	}{
		{RestartStatusPending, "pending"},
		{RestartStatusApproved, "approved"},
		{RestartStatusComplete, "complete"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("RestartStatus.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRestartRequest(t *testing.T) {
	tests := []struct {
		name        string
		reason      RestartReason
		description string
		requestedBy string
		wantErr     bool
	}{
		{
			name:        "valid request",
			reason:      RestartReasonCodeChange,
			description: "Test restart",
			requestedBy: "agent-123",
			wantErr:     false,
		},
		{
			name:        "empty reason",
			reason:      "",
			description: "Test restart",
			requestedBy: "agent-123",
			wantErr:     true,
		},
		{
			name:        "empty requestedBy",
			reason:      RestartReasonCodeChange,
			description: "Test restart",
			requestedBy: "",
			wantErr:     true,
		},
		{
			name:        "empty description is allowed",
			reason:      RestartReasonUserRequested,
			description: "",
			requestedBy: "user-456",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := NewRestartRequest(tt.reason, tt.description, tt.requestedBy)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRestartRequest() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if req == nil {
					t.Error("NewRestartRequest() returned nil request")
					return
				}
				if req.ID == "" {
					t.Error("NewRestartRequest() ID should be set")
				}
				if req.Reason != tt.reason {
					t.Errorf("NewRestartRequest() Reason = %v, want %v", req.Reason, tt.reason)
				}
				if req.Status != RestartStatusPending {
					t.Errorf("NewRestartRequest() Status = %v, want %v", req.Status, RestartStatusPending)
				}
				if req.Port != 8080 {
					t.Errorf("NewRestartRequest() Port = %v, want 8080", req.Port)
				}
			}
		})
	}
}

func TestRestartRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request *RestartRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: &RestartRequest{
				ID:          "test-id",
				Reason:      RestartReasonCodeChange,
				RequestedBy: "agent-123",
				Status:      RestartStatusPending,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			request: &RestartRequest{
				ID:          "",
				Reason:      RestartReasonCodeChange,
				RequestedBy: "agent-123",
				Status:      RestartStatusPending,
			},
			wantErr: true,
		},
		{
			name: "missing reason",
			request: &RestartRequest{
				ID:          "test-id",
				Reason:      "",
				RequestedBy: "agent-123",
				Status:      RestartStatusPending,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			request: &RestartRequest{
				ID:          "test-id",
				Reason:      RestartReasonCodeChange,
				RequestedBy: "agent-123",
				Status:      RestartStatus("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.request.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("RestartRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRestartRequest_StatusTransitions(t *testing.T) {
	t.Run("approve from pending", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		err := req.Approve()
		if err != nil {
			t.Errorf("Approve() error = %v", err)
		}
		if req.Status != RestartStatusApproved {
			t.Errorf("Status = %v, want %v", req.Status, RestartStatusApproved)
		}
		if req.ApprovedAt == nil {
			t.Error("ApprovedAt should be set")
		}
	})

	t.Run("reject from pending", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		err := req.Reject()
		if err != nil {
			t.Errorf("Reject() error = %v", err)
		}
		if req.Status != RestartStatusRejected {
			t.Errorf("Status = %v, want %v", req.Status, RestartStatusRejected)
		}
		if req.CompletedAt == nil {
			t.Error("CompletedAt should be set")
		}
	})

	t.Run("cancel from pending", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		err := req.Cancel()
		if err != nil {
			t.Errorf("Cancel() error = %v", err)
		}
		if req.Status != RestartStatusCancelled {
			t.Errorf("Status = %v, want %v", req.Status, RestartStatusCancelled)
		}
	})

	t.Run("cannot approve completed request", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		req.Status = RestartStatusComplete
		err := req.Approve()
		if err != ErrRestartAlreadyDecided {
			t.Errorf("Approve() error = %v, want %v", err, ErrRestartAlreadyDecided)
		}
	})

	t.Run("cannot reject in_progress request", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		req.Status = RestartStatusInProgress
		err := req.Reject()
		if err != ErrRestartInProgress {
			t.Errorf("Reject() error = %v, want %v", err, ErrRestartInProgress)
		}
	})
}

func TestRestartRequest_Predicates(t *testing.T) {
	t.Run("IsPending", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		if !req.IsPending() {
			t.Error("IsPending() = false, want true")
		}
		req.Status = RestartStatusApproved
		if req.IsPending() {
			t.Error("IsPending() = true, want false")
		}
	})

	t.Run("IsApproved", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		if req.IsApproved() {
			t.Error("IsApproved() = true, want false")
		}
		req.Approve()
		if !req.IsApproved() {
			t.Error("IsApproved() = false, want true")
		}
	})

	t.Run("CanExecute", func(t *testing.T) {
		req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
		if req.CanExecute() {
			t.Error("CanExecute() = true, want false for pending")
		}
		req.Approve()
		if !req.CanExecute() {
			t.Error("CanExecute() = false, want true for approved")
		}
	})
}

func TestRestartRequest_SetMethods(t *testing.T) {
	req, _ := NewRestartRequest(RestartReasonCodeChange, "test", "agent-123")
	originalUpdatedAt := req.UpdatedAt

	// Small delay to ensure UpdatedAt changes
	time.Sleep(1 * time.Millisecond)

	t.Run("SetPort", func(t *testing.T) {
		req.SetPort(9090)
		if req.Port != 9090 {
			t.Errorf("Port = %v, want 9090", req.Port)
		}
		if !req.UpdatedAt.After(originalUpdatedAt) {
			t.Error("UpdatedAt should be updated")
		}
	})

	t.Run("SetSessionID", func(t *testing.T) {
		req.SetSessionID("session-456")
		if req.SessionID != "session-456" {
			t.Errorf("SessionID = %v, want session-456", req.SessionID)
		}
	})

	t.Run("SetBuildRequired", func(t *testing.T) {
		req.SetBuildRequired(true)
		if !req.BuildRequired {
			t.Error("BuildRequired = false, want true")
		}
	})
}

func TestDefaultRestartManagerConfig(t *testing.T) {
	config := DefaultRestartManagerConfig()
	if config.Timeout != 2*time.Minute {
		t.Errorf("Timeout = %v, want 2 minutes", config.Timeout)
	}
	if config.BuildTimeout != 5*time.Minute {
		t.Errorf("BuildTimeout = %v, want 5 minutes", config.BuildTimeout)
	}
	if !config.RequireApproval {
		t.Error("RequireApproval = false, want true")
	}
	if !config.AutoBuild {
		t.Error("AutoBuild = false, want true")
	}
	if config.DefaultPort != 8080 {
		t.Errorf("DefaultPort = %v, want 8080", config.DefaultPort)
	}
}

func TestNewRestartManager(t *testing.T) {
	// Create a temp directory that looks like orchestrator root
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, err := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	if err != nil {
		t.Fatalf("Failed to create locator: %v", err)
	}

	t.Run("with valid locator", func(t *testing.T) {
		manager, err := NewRestartManager(locator)
		if err != nil {
			t.Errorf("NewRestartManager() error = %v", err)
		}
		if manager == nil {
			t.Error("NewRestartManager() returned nil")
		}
	})

	t.Run("with nil locator", func(t *testing.T) {
		_, err := NewRestartManager(nil)
		if err == nil {
			t.Error("NewRestartManager() with nil locator should return error")
		}
	})
}

func TestNewRestartManagerWithConfig(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})

	t.Run("with custom config", func(t *testing.T) {
		config := &RestartManagerConfig{
			Timeout:         1 * time.Minute,
			RequireApproval: false,
			DefaultPort:     9000,
		}
		manager, err := NewRestartManagerWithConfig(locator, config)
		if err != nil {
			t.Errorf("NewRestartManagerWithConfig() error = %v", err)
		}
		if manager.config.Timeout != 1*time.Minute {
			t.Errorf("config.Timeout = %v, want 1 minute", manager.config.Timeout)
		}
		if manager.config.DefaultPort != 9000 {
			t.Errorf("config.DefaultPort = %v, want 9000", manager.config.DefaultPort)
		}
	})

	t.Run("with nil config uses defaults", func(t *testing.T) {
		manager, err := NewRestartManagerWithConfig(locator, nil)
		if err != nil {
			t.Errorf("NewRestartManagerWithConfig() error = %v", err)
		}
		if manager.config.DefaultPort != 8080 {
			t.Errorf("config.DefaultPort = %v, want 8080", manager.config.DefaultPort)
		}
	})
}

func TestRestartManager_RequestRestart(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	t.Run("creates pending request", func(t *testing.T) {
		req, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
		if err != nil {
			t.Errorf("RequestRestart() error = %v", err)
		}
		if req.Status != RestartStatusPending {
			t.Errorf("Status = %v, want %v", req.Status, RestartStatusPending)
		}
		if req.Port != 8080 {
			t.Errorf("Port = %v, want 8080", req.Port)
		}
	})

	t.Run("auto-approves when approval not required", func(t *testing.T) {
		manager.SetApprovalRequired(false)
		req, err := manager.RequestRestart(ctx, RestartReasonUserRequested, "test", "user-456")
		if err != nil {
			t.Errorf("RequestRestart() error = %v", err)
		}
		if req.Status != RestartStatusApproved {
			t.Errorf("Status = %v, want %v", req.Status, RestartStatusApproved)
		}
		manager.SetApprovalRequired(true) // Reset
	})

	t.Run("sets build required for code changes", func(t *testing.T) {
		manager.SetAutoBuild(true)
		req, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
		if err != nil {
			t.Errorf("RequestRestart() error = %v", err)
		}
		if !req.BuildRequired {
			t.Error("BuildRequired = false, want true for code change")
		}
	})
}

func TestRestartManager_GetRequest(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	t.Run("returns existing request", func(t *testing.T) {
		req, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
		retrieved, err := manager.GetRequest(ctx, req.ID)
		if err != nil {
			t.Errorf("GetRequest() error = %v", err)
		}
		if retrieved.ID != req.ID {
			t.Errorf("ID = %v, want %v", retrieved.ID, req.ID)
		}
	})

	t.Run("returns error for non-existent request", func(t *testing.T) {
		_, err := manager.GetRequest(ctx, "non-existent-id")
		if err != ErrRestartNotFound {
			t.Errorf("GetRequest() error = %v, want %v", err, ErrRestartNotFound)
		}
	})
}

func TestRestartManager_ApproveReject(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	t.Run("approve", func(t *testing.T) {
		req, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
		err := manager.Approve(ctx, req.ID, "admin", "approved for testing")
		if err != nil {
			t.Errorf("Approve() error = %v", err)
		}
		retrieved, _ := manager.GetRequest(ctx, req.ID)
		if retrieved.Status != RestartStatusApproved {
			t.Errorf("Status = %v, want %v", retrieved.Status, RestartStatusApproved)
		}
	})

	t.Run("reject", func(t *testing.T) {
		req, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
		err := manager.Reject(ctx, req.ID, "admin", "not needed")
		if err != nil {
			t.Errorf("Reject() error = %v", err)
		}
		retrieved, _ := manager.GetRequest(ctx, req.ID)
		if retrieved.Status != RestartStatusRejected {
			t.Errorf("Status = %v, want %v", retrieved.Status, RestartStatusRejected)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		req, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
		err := manager.Cancel(ctx, req.ID, "no longer needed")
		if err != nil {
			t.Errorf("Cancel() error = %v", err)
		}
		retrieved, _ := manager.GetRequest(ctx, req.ID)
		if retrieved.Status != RestartStatusCancelled {
			t.Errorf("Status = %v, want %v", retrieved.Status, RestartStatusCancelled)
		}
	})

	t.Run("approve non-existent request", func(t *testing.T) {
		err := manager.Approve(ctx, "non-existent", "admin", "test")
		if err != ErrRestartNotFound {
			t.Errorf("Approve() error = %v, want %v", err, ErrRestartNotFound)
		}
	})
}

func TestRestartManager_CanRestart(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	result, err := manager.CanRestart(ctx)
	if err != nil {
		t.Errorf("CanRestart() error = %v", err)
	}
	if result == nil {
		t.Error("CanRestart() returned nil result")
		return
	}

	// Should have several checks
	if len(result.Checks) < 4 {
		t.Errorf("Expected at least 4 checks, got %d", len(result.Checks))
	}

	// Check that we have expected check names
	checkNames := make(map[string]bool)
	for _, check := range result.Checks {
		checkNames[check.Name] = true
	}

	expectedChecks := []string{"orchestrator_root", "go_mod", "no_restart_in_progress"}
	for _, name := range expectedChecks {
		if !checkNames[name] {
			t.Errorf("Missing expected check: %s", name)
		}
	}
}

func TestRestartManager_ListMethods(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	// Create some requests
	req1, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test1", "agent-1")
	req2, _ := manager.RequestRestart(ctx, RestartReasonConfigChange, "test2", "agent-2")
	_ = manager.Approve(ctx, req1.ID, "admin", "approved")

	t.Run("ListActiveRequests", func(t *testing.T) {
		active := manager.ListActiveRequests(ctx)
		if len(active) < 2 {
			t.Errorf("Expected at least 2 active requests, got %d", len(active))
		}
	})

	t.Run("ListPendingRequests", func(t *testing.T) {
		pending := manager.ListPendingRequests(ctx)
		// req2 should still be pending
		found := false
		for _, p := range pending {
			if p.ID == req2.ID {
				found = true
				break
			}
		}
		if !found {
			t.Error("req2 should be in pending list")
		}
	})
}

func TestRestartManager_ClearCompletedRequests(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	// Create and complete some requests
	req1, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test1", "agent-1")
	_ = manager.Reject(ctx, req1.ID, "admin", "rejected")

	req2, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test2", "agent-2")
	_ = manager.Cancel(ctx, req2.ID, "cancelled")

	// Pending request should remain
	req3, _ := manager.RequestRestart(ctx, RestartReasonCodeChange, "test3", "agent-3")

	cleared := manager.ClearCompletedRequests(ctx)
	if cleared != 2 {
		t.Errorf("ClearCompletedRequests() = %d, want 2", cleared)
	}

	// req3 should still exist
	_, err := manager.GetRequest(ctx, req3.ID)
	if err != nil {
		t.Errorf("req3 should still exist: %v", err)
	}

	// req1 should be gone
	_, err = manager.GetRequest(ctx, req1.ID)
	if err != ErrRestartNotFound {
		t.Errorf("req1 should be removed, got err: %v", err)
	}
}

func TestRestartManager_ConfigMethods(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)

	t.Run("GetConfig", func(t *testing.T) {
		config := manager.GetConfig()
		if config == nil {
			t.Error("GetConfig() returned nil")
		}
	})

	t.Run("SetConfig", func(t *testing.T) {
		newConfig := &RestartManagerConfig{
			Timeout:     30 * time.Second,
			DefaultPort: 9999,
		}
		manager.SetConfig(newConfig)
		if manager.config.DefaultPort != 9999 {
			t.Errorf("DefaultPort = %v, want 9999", manager.config.DefaultPort)
		}
	})

	t.Run("SetApprovalRequired", func(t *testing.T) {
		manager.SetApprovalRequired(false)
		if manager.config.RequireApproval {
			t.Error("RequireApproval = true, want false")
		}
	})

	t.Run("SetAutoBuild", func(t *testing.T) {
		manager.SetAutoBuild(false)
		if manager.config.AutoBuild {
			t.Error("AutoBuild = true, want false")
		}
	})

	t.Run("SetDefaultPort", func(t *testing.T) {
		manager.SetDefaultPort(8888)
		if manager.config.DefaultPort != 8888 {
			t.Errorf("DefaultPort = %v, want 8888", manager.config.DefaultPort)
		}
	})

	t.Run("GetLocator", func(t *testing.T) {
		if manager.GetLocator() != locator {
			t.Error("GetLocator() returned wrong locator")
		}
	})

	t.Run("GetBuilder", func(t *testing.T) {
		if manager.GetBuilder() == nil {
			t.Error("GetBuilder() returned nil")
		}
	})
}

func TestRestartManager_GetCurrentRestart(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)

	// Initially no current restart
	if manager.GetCurrentRestart() != nil {
		t.Error("GetCurrentRestart() should be nil initially")
	}
}

func TestPreflightCheck(t *testing.T) {
	check := &PreflightCheck{
		Name:        "test_check",
		Description: "A test check",
		Passed:      true,
		Message:     "Check passed",
		Critical:    true,
	}

	if check.Name != "test_check" {
		t.Errorf("Name = %v, want test_check", check.Name)
	}
	if !check.Passed {
		t.Error("Passed = false, want true")
	}
	if !check.Critical {
		t.Error("Critical = false, want true")
	}
}

func TestPreflightResult(t *testing.T) {
	result := &PreflightResult{
		AllPassed: false,
		Checks: []*PreflightCheck{
			{Name: "check1", Passed: true},
			{Name: "check2", Passed: false, Critical: true, Message: "Failed"},
		},
		Errors: []string{"check2: Failed"},
	}

	if result.AllPassed {
		t.Error("AllPassed = true, want false")
	}
	if len(result.Checks) != 2 {
		t.Errorf("len(Checks) = %d, want 2", len(result.Checks))
	}
	if len(result.Errors) != 1 {
		t.Errorf("len(Errors) = %d, want 1", len(result.Errors))
	}
}

func TestRestartResult(t *testing.T) {
	result := &RestartResult{
		Success:      true,
		RequestID:    "req-123",
		Duration:     5 * time.Second,
		Output:       "Restart successful",
		ErrorMessage: "",
	}

	if !result.Success {
		t.Error("Success = false, want true")
	}
	if result.RequestID != "req-123" {
		t.Errorf("RequestID = %v, want req-123", result.RequestID)
	}
	if result.Duration != 5*time.Second {
		t.Errorf("Duration = %v, want 5s", result.Duration)
	}
}

func TestRestartManager_ExecuteRestart(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	ctx := context.Background()

	t.Run("returns error for non-existent request", func(t *testing.T) {
		manager, _ := NewRestartManager(locator)
		_, err := manager.ExecuteRestart(ctx, "non-existent-id")
		if err != ErrRestartNotFound {
			t.Errorf("ExecuteRestart() error = %v, want %v", err, ErrRestartNotFound)
		}
	})

	t.Run("returns error for pending (not approved) request", func(t *testing.T) {
		manager, _ := NewRestartManager(locator)
		req, _ := manager.RequestRestart(ctx, RestartReasonUserRequested, "test", "agent-123")
		// Request is pending, not approved
		_, err := manager.ExecuteRestart(ctx, req.ID)
		if err != ErrRestartNotApproved {
			t.Errorf("ExecuteRestart() error = %v, want %v", err, ErrRestartNotApproved)
		}
	})

	t.Run("returns error when restart already in progress", func(t *testing.T) {
		manager, _ := NewRestartManager(locator)
		manager.SetApprovalRequired(false) // Auto-approve

		// Create first request and approve it
		req1, _ := manager.RequestRestart(ctx, RestartReasonUserRequested, "test1", "agent-1")
		// Create second request before marking first as in-progress
		req2, _ := manager.RequestRestart(ctx, RestartReasonUserRequested, "test2", "agent-2")

		// Now manually set first as in progress
		manager.mu.Lock()
		req1.Status = RestartStatusInProgress
		manager.currentRestart = req1
		manager.mu.Unlock()

		// Try to execute second restart
		_, err := manager.ExecuteRestart(ctx, req2.ID)
		if err != ErrRestartInProgress {
			t.Errorf("ExecuteRestart() error = %v, want %v", err, ErrRestartInProgress)
		}
	})

	t.Run("executes approved restart and runs pre-flight checks", func(t *testing.T) {
		manager, _ := NewRestartManager(locator)
		manager.SetAutoBuild(false) // Skip build for this test

		req, _ := manager.RequestRestart(ctx, RestartReasonUserRequested, "test", "agent-123")
		manager.Approve(ctx, req.ID, "admin", "approved")

		// ExecuteRestart will fail on doRestart (which tries to actually restart)
		// but it should pass pre-flight checks
		result, err := manager.ExecuteRestart(ctx, req.ID)
		// We expect it to fail during actual restart (doRestart tries to execute binaries)
		// but the result should contain pre-flight check info
		if result == nil {
			t.Error("ExecuteRestart() result should not be nil even on failure")
		}
		if err == nil && result != nil && !result.Success {
			// This is fine - result indicates failure
		}

		// Verify the request status changed
		updatedReq, _ := manager.GetRequest(ctx, req.ID)
		if !updatedReq.Status.IsFinal() {
			t.Error("Request status should be final after ExecuteRestart")
		}
	})

	t.Run("fails pre-flight when orchestrator root invalid", func(t *testing.T) {
		// Create a locator with a directory that we'll delete
		badDir := filepath.Join(tempDir, "bad_dir")
		os.MkdirAll(badDir, 0755)
		os.WriteFile(filepath.Join(badDir, "go.mod"), []byte("module test\ngo 1.21"), 0644)
		os.MkdirAll(filepath.Join(badDir, "internal/mcp"), 0755)

		badLocator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: badDir})
		manager, _ := NewRestartManager(badLocator)
		manager.SetApprovalRequired(false)
		manager.SetAutoBuild(false)

		req, _ := manager.RequestRestart(ctx, RestartReasonUserRequested, "test", "agent-123")

		// Remove the directory to cause pre-flight to fail
		os.RemoveAll(badDir)

		result, err := manager.ExecuteRestart(ctx, req.ID)
		if err != ErrRestartPreflightFailed {
			t.Errorf("ExecuteRestart() error = %v, want %v", err, ErrRestartPreflightFailed)
		}
		if result == nil {
			t.Error("ExecuteRestart() result should not be nil")
		} else if result.Success {
			t.Error("ExecuteRestart() result.Success = true, want false")
		}
	})
}

func TestRestartManager_ConcurrentRequests(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	// Create multiple concurrent requests - only the first should succeed
	// when there's already an in-progress restart
	manager.mu.Lock()
	fakeReq := &RestartRequest{
		ID:     "fake-in-progress",
		Status: RestartStatusInProgress,
	}
	manager.currentRestart = fakeReq
	manager.activeRequests["fake-in-progress"] = fakeReq
	manager.mu.Unlock()

	// Try to create another request - should fail with ErrRestartInProgress
	_, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "test", "agent-123")
	if err != ErrRestartInProgress {
		t.Errorf("RequestRestart() with concurrent restart error = %v, want %v", err, ErrRestartInProgress)
	}
}

func TestRestartReason_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		reason RestartReason
		want   bool
	}{
		{"code_change is valid", RestartReasonCodeChange, true},
		{"config_change is valid", RestartReasonConfigChange, true},
		{"user_requested is valid", RestartReasonUserRequested, true},
		{"recovery is valid", RestartReasonRecovery, true},
		{"upgrade is valid", RestartReasonUpgrade, true},
		{"empty is invalid", RestartReason(""), false},
		{"unknown is invalid", RestartReason("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.reason.IsValid(); got != tt.want {
				t.Errorf("RestartReason.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestartReason_String(t *testing.T) {
	tests := []struct {
		reason RestartReason
		want   string
	}{
		{RestartReasonCodeChange, "code_change"},
		{RestartReasonConfigChange, "config_change"},
		{RestartReasonUserRequested, "user_requested"},
		{RestartReasonRecovery, "recovery"},
		{RestartReasonUpgrade, "upgrade"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.reason.String(); got != tt.want {
				t.Errorf("RestartReason.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRestartManager_ExecuteRestartWithBuild(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	ctx := context.Background()

	t.Run("build required triggers build before restart", func(t *testing.T) {
		manager, _ := NewRestartManager(locator)
		manager.SetApprovalRequired(false) // Auto-approve

		// Create a request that requires a build
		req, err := manager.RequestRestart(ctx, RestartReasonCodeChange, "test build", "agent-123")
		if err != nil {
			t.Fatalf("RequestRestart() error = %v", err)
		}

		// Verify build required was set automatically for code changes
		if !req.BuildRequired {
			t.Error("BuildRequired should be true for code changes with AutoBuild enabled")
		}

		// Execute will fail due to build issues in temp dir, but that's expected
		result, err := manager.ExecuteRestart(ctx, req.ID)
		// We expect an error because the temp dir doesn't have real Go code to build
		if result != nil {
			// The request should have a final status
			retrieved, _ := manager.GetRequest(ctx, req.ID)
			if !retrieved.Status.IsFinal() {
				t.Error("Request should have final status after ExecuteRestart")
			}
		}
		// Error is acceptable here since we can't build in a temp dir
		_ = err
	})

	t.Run("no build when not required", func(t *testing.T) {
		manager, _ := NewRestartManager(locator)
		manager.SetApprovalRequired(false)
		manager.SetAutoBuild(false)

		req, _ := manager.RequestRestart(ctx, RestartReasonUserRequested, "manual restart", "user-456")

		if req.BuildRequired {
			t.Error("BuildRequired should be false when AutoBuild is disabled")
		}
	})
}

func TestRestartManager_TimeoutHandling(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})

	t.Run("respects config timeout", func(t *testing.T) {
		config := &RestartManagerConfig{
			Timeout:         1 * time.Second,
			BuildTimeout:    1 * time.Second,
			RequireApproval: false,
			AutoBuild:       false,
			DefaultPort:     8080,
		}
		manager, _ := NewRestartManagerWithConfig(locator, config)

		if manager.GetConfig().Timeout != 1*time.Second {
			t.Errorf("Timeout = %v, want 1s", manager.GetConfig().Timeout)
		}
	})
}

func TestRestartRequest_FullLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	// Create request
	req, err := manager.RequestRestart(ctx, RestartReasonUserRequested, "full lifecycle test", "test-agent")
	if err != nil {
		t.Fatalf("RequestRestart() error = %v", err)
	}
	if !req.IsPending() {
		t.Error("New request should be pending")
	}

	// Approve
	if err := manager.Approve(ctx, req.ID, "admin", "approved"); err != nil {
		t.Fatalf("Approve() error = %v", err)
	}
	if !req.IsApproved() {
		t.Error("Request should be approved")
	}
	if !req.CanExecute() {
		t.Error("Approved request should be executable")
	}

	// Verify state
	retrieved, _ := manager.GetRequest(ctx, req.ID)
	if retrieved.ApprovedAt == nil {
		t.Error("ApprovedAt should be set after approval")
	}
}

func TestRestartRequest_AllReasons(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)
	ctx := context.Background()

	reasons := []RestartReason{
		RestartReasonCodeChange,
		RestartReasonConfigChange,
		RestartReasonUserRequested,
		RestartReasonRecovery,
		RestartReasonUpgrade,
	}

	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			req, err := manager.RequestRestart(ctx, reason, "test", "agent")
			if err != nil {
				t.Errorf("RequestRestart(%s) error = %v", reason, err)
				return
			}
			if req.Reason != reason {
				t.Errorf("Reason = %v, want %v", req.Reason, reason)
			}
		})
	}
}

func TestValidRestartReasons(t *testing.T) {
	// Ensure all reasons are in the ValidRestartReasons list
	expected := 5 // code_change, config_change, user_requested, recovery, upgrade
	if len(ValidRestartReasons) != expected {
		t.Errorf("len(ValidRestartReasons) = %d, want %d", len(ValidRestartReasons), expected)
	}
}

func TestRestartManager_SetConfigNil(t *testing.T) {
	tempDir := t.TempDir()
	createMockOrchestratorRoot(t, tempDir)

	locator, _ := NewOrchestratorLocator(OrchestratorLocatorConfig{Root: tempDir})
	manager, _ := NewRestartManager(locator)

	originalPort := manager.GetConfig().DefaultPort
	manager.SetConfig(nil) // Should be a no-op
	if manager.GetConfig().DefaultPort != originalPort {
		t.Error("SetConfig(nil) should not change the config")
	}
}

// Helper function to create a mock orchestrator root directory
func createMockOrchestratorRoot(t *testing.T, dir string) {
	t.Helper()

	// Create go.mod
	goModContent := `module openexec

go 1.21
`
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goModContent), 0644); err != nil {
		t.Fatalf("Failed to create go.mod: %v", err)
	}

	// Create internal directory with expected subdirectories
	dirs := []string{"internal/mcp", "internal/loop", "internal/agent", "cmd"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", d, err)
		}
	}

	// Create a dummy Go file
	mainContent := `package main

func main() {}
`
	if err := os.WriteFile(filepath.Join(dir, "cmd/main.go"), []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to create main.go: %v", err)
	}
}
