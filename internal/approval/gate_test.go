package approval

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/mode"
)

func TestNewInMemoryGate(t *testing.T) {
	// Test with nil config (should use defaults)
	gate := NewInMemoryGate(nil)
	if gate == nil {
		t.Fatal("expected gate to be created")
	}
	if gate.config.DefaultTimeout != 5*time.Minute {
		t.Errorf("expected default timeout of 5 minutes, got %v", gate.config.DefaultTimeout)
	}
	if !gate.config.Enabled {
		t.Error("expected gate to be enabled by default")
	}

	// Test with custom config
	config := &GateConfig{
		DefaultTimeout:       10 * time.Second,
		Enabled:              false,
		AutoApproveInRunMode: false,
	}
	gate = NewInMemoryGate(config)
	if gate.config.DefaultTimeout != 10*time.Second {
		t.Errorf("expected timeout of 10 seconds, got %v", gate.config.DefaultTimeout)
	}
	if gate.config.Enabled {
		t.Error("expected gate to be disabled")
	}
}

func TestGate_SetMode(t *testing.T) {
	gate := NewInMemoryGate(nil)

	gate.SetMode(mode.ModeTask)
	if gate.GetMode() != mode.ModeTask {
		t.Errorf("expected mode to be %v, got %v", mode.ModeTask, gate.GetMode())
	}

	gate.SetMode(mode.ModeRun)
	if gate.GetMode() != mode.ModeRun {
		t.Errorf("expected mode to be %v, got %v", mode.ModeRun, gate.GetMode())
	}

	gate.SetMode(mode.ModeChat)
	if gate.GetMode() != mode.ModeChat {
		t.Errorf("expected mode to be %v, got %v", mode.ModeChat, gate.GetMode())
	}
}

func TestGate_SetEnabled(t *testing.T) {
	gate := NewInMemoryGate(nil)

	if !gate.IsEnabled() {
		t.Error("expected gate to be enabled by default")
	}

	gate.SetEnabled(false)
	if gate.IsEnabled() {
		t.Error("expected gate to be disabled after SetEnabled(false)")
	}

	gate.SetEnabled(true)
	if !gate.IsEnabled() {
		t.Error("expected gate to be enabled after SetEnabled(true)")
	}
}

func TestGate_AutoApproveWhenDisabled(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout: 1 * time.Second,
		Enabled:        false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "write_file",
		ToolArgs:  map[string]any{"path": "/test.txt"},
		RiskLevel: "high",
	}

	ctx := context.Background()
	result, err := gate.RequestApproval(ctx, req)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != RequestStatusAutoApproved {
		t.Errorf("expected auto-approved status, got %v", result.Status)
	}
	if result.ResolvedBy != "auto" {
		t.Errorf("expected resolved by 'auto', got %v", result.ResolvedBy)
	}
}

func TestGate_AutoApproveInRunMode(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       1 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: true,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeRun)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "write_file",
		ToolArgs:  map[string]any{"path": "/test.txt"},
		RiskLevel: "high",
	}

	ctx := context.Background()
	result, err := gate.RequestApproval(ctx, req)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != RequestStatusAutoApproved {
		t.Errorf("expected auto-approved status, got %v", result.Status)
	}
}

func TestGate_AutoApproveLowRisk(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       1 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "read_file",
		ToolArgs:  map[string]any{"path": "/test.txt"},
		RiskLevel: "low",
	}

	ctx := context.Background()
	result, err := gate.RequestApproval(ctx, req)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Status != RequestStatusAutoApproved {
		t.Errorf("expected auto-approved status for low risk, got %v", result.Status)
	}
}

func TestGate_ApproveRequest(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "write_file",
		ToolArgs:  map[string]any{"path": "/test.txt"},
		RiskLevel: "medium",
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	var result *GateRequest
	var approvalErr error

	// Start approval request in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, approvalErr = gate.RequestApproval(ctx, req)
	}()

	// Wait a bit for the request to be registered
	time.Sleep(50 * time.Millisecond)

	// Approve the request
	pending := gate.GetAllPendingApprovals()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}

	err := gate.Approve(pending[0].ID, "test_user")
	if err != nil {
		t.Fatalf("failed to approve: %v", err)
	}

	// Wait for the goroutine to complete
	wg.Wait()

	if approvalErr != nil {
		t.Fatalf("expected no error from RequestApproval, got %v", approvalErr)
	}
	if result.Status != RequestStatusApproved {
		t.Errorf("expected approved status, got %v", result.Status)
	}
	if result.ResolvedBy != "test_user" {
		t.Errorf("expected resolved by 'test_user', got %v", result.ResolvedBy)
	}
}

func TestGate_RejectRequest(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "run_shell_command",
		ToolArgs:  map[string]any{"command": "rm -rf /"},
		RiskLevel: "high",
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	var result *GateRequest
	var approvalErr error

	// Start approval request in goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		result, approvalErr = gate.RequestApproval(ctx, req)
	}()

	// Wait a bit for the request to be registered
	time.Sleep(50 * time.Millisecond)

	// Reject the request
	pending := gate.GetAllPendingApprovals()
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending request, got %d", len(pending))
	}

	err := gate.Reject(pending[0].ID, "test_user", "Dangerous command")
	if err != nil {
		t.Fatalf("failed to reject: %v", err)
	}

	// Wait for the goroutine to complete
	wg.Wait()

	if approvalErr == nil {
		t.Fatal("expected error from rejected request")
	}
	if result.Status != RequestStatusRejected {
		t.Errorf("expected rejected status, got %v", result.Status)
	}
	if result.RejectReason != "Dangerous command" {
		t.Errorf("expected reject reason 'Dangerous command', got %v", result.RejectReason)
	}
}

func TestGate_Timeout(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       100 * time.Millisecond,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "write_file",
		ToolArgs:  map[string]any{"path": "/test.txt"},
		RiskLevel: "medium",
	}

	ctx := context.Background()
	result, err := gate.RequestApproval(ctx, req)

	if err != ErrApprovalTimedOut {
		t.Errorf("expected timeout error, got %v", err)
	}
	if result.Status != RequestStatusExpired {
		t.Errorf("expected expired status, got %v", result.Status)
	}
}

func TestGate_ContextCancellation(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	req := &GateRequest{
		RunID:     "run_1",
		ToolName:  "write_file",
		ToolArgs:  map[string]any{"path": "/test.txt"},
		RiskLevel: "medium",
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var approvalErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		_, approvalErr = gate.RequestApproval(ctx, req)
	}()

	// Wait a bit then cancel
	time.Sleep(50 * time.Millisecond)
	cancel()

	wg.Wait()

	if approvalErr == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestGate_GetPendingApprovals(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	// Create multiple requests
	for i := 0; i < 3; i++ {
		go func(runID string) {
			req := &GateRequest{
				RunID:     runID,
				ToolName:  "write_file",
				ToolArgs:  map[string]any{"path": "/test.txt"},
				RiskLevel: "medium",
			}
			gate.RequestApproval(context.Background(), req)
		}(fmt.Sprintf("run_%d", i))
	}

	// Wait for requests to be registered
	time.Sleep(50 * time.Millisecond)

	// Check total pending
	all := gate.GetAllPendingApprovals()
	if len(all) != 3 {
		t.Errorf("expected 3 pending requests, got %d", len(all))
	}

	// Check filtered by run
	run0 := gate.GetPendingApprovals("run_0")
	if len(run0) != 1 {
		t.Errorf("expected 1 pending request for run_0, got %d", len(run0))
	}

	// Cancel all pending
	for _, p := range all {
		gate.Reject(p.ID, "test", "cleanup")
	}
}

func TestGate_CancelForRun(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	// Create requests for two runs
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(runID string) {
			defer wg.Done()
			req := &GateRequest{
				RunID:     runID,
				ToolName:  "write_file",
				ToolArgs:  map[string]any{"path": "/test.txt"},
				RiskLevel: "medium",
			}
			gate.RequestApproval(context.Background(), req)
		}(fmt.Sprintf("run_%d", i%2))
	}

	// Wait for requests to be registered
	time.Sleep(50 * time.Millisecond)

	// Cancel run_0
	cancelled := gate.CancelForRun("run_0")
	if cancelled != 1 {
		t.Errorf("expected 1 cancelled, got %d", cancelled)
	}

	// Check remaining pending
	remaining := gate.GetAllPendingApprovals()
	if len(remaining) != 1 {
		t.Errorf("expected 1 remaining pending, got %d", len(remaining))
	}

	// Cancel remaining
	gate.CancelForRun("run_1")
	wg.Wait()
}

func TestGate_GetRequest(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	go func() {
		req := &GateRequest{
			ID:        "test-id-123",
			RunID:     "run_1",
			ToolName:  "write_file",
			ToolArgs:  map[string]any{"path": "/test.txt"},
			RiskLevel: "medium",
		}
		gate.RequestApproval(context.Background(), req)
	}()

	// Wait for request to be registered
	time.Sleep(50 * time.Millisecond)

	// Get the request
	req, found := gate.GetRequest("test-id-123")
	if !found {
		t.Fatal("expected to find request")
	}
	if req.ID != "test-id-123" {
		t.Errorf("expected ID 'test-id-123', got %v", req.ID)
	}

	// Get non-existent request
	_, found = gate.GetRequest("non-existent")
	if found {
		t.Error("expected not to find non-existent request")
	}

	// Approve and check resolved
	gate.Approve("test-id-123", "test")
	time.Sleep(50 * time.Millisecond)

	req, found = gate.GetRequest("test-id-123")
	if !found {
		t.Fatal("expected to find resolved request")
	}
	if req.Status != RequestStatusApproved {
		t.Errorf("expected approved status, got %v", req.Status)
	}
}

func TestGate_DoubleApprove(t *testing.T) {
	config := &GateConfig{
		DefaultTimeout:       5 * time.Second,
		Enabled:              true,
		AutoApproveInRunMode: false,
	}
	gate := NewInMemoryGate(config)
	gate.SetMode(mode.ModeTask)

	go func() {
		req := &GateRequest{
			ID:        "double-test",
			RunID:     "run_1",
			ToolName:  "write_file",
			ToolArgs:  map[string]any{"path": "/test.txt"},
			RiskLevel: "medium",
		}
		gate.RequestApproval(context.Background(), req)
	}()

	// Wait for request to be registered
	time.Sleep(50 * time.Millisecond)

	// First approve
	err := gate.Approve("double-test", "user1")
	if err != nil {
		t.Fatalf("first approve should succeed: %v", err)
	}

	// Second approve should fail
	err = gate.Approve("double-test", "user2")
	if err != ErrRequestAlreadyResolved {
		t.Errorf("expected ErrRequestAlreadyResolved, got %v", err)
	}
}

func TestGate_Stats(t *testing.T) {
	gate := NewInMemoryGate(nil)

	stats := gate.Stats()
	if stats["pending"] != 0 {
		t.Errorf("expected 0 pending, got %d", stats["pending"])
	}
	if stats["resolved"] != 0 {
		t.Errorf("expected 0 resolved, got %d", stats["resolved"])
	}
}

func TestFormatRequestDescription(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		args     map[string]any
		want     string
	}{
		{
			name:     "write_file",
			toolName: "write_file",
			args:     map[string]any{"path": "/tmp/test.txt"},
			want:     "Write file: /tmp/test.txt",
		},
		{
			name:     "git_push",
			toolName: "git_push",
			args:     map[string]any{},
			want:     "Push changes to remote repository",
		},
		{
			name:     "git_tag",
			toolName: "git_tag",
			args:     map[string]any{"name": "v1.0.0"},
			want:     "Create git tag: v1.0.0",
		},
		{
			name:     "run_shell_command",
			toolName: "run_shell_command",
			args:     map[string]any{"command": "echo hello"},
			want:     "Run command: echo hello",
		},
		{
			name:     "run_shell_command_long",
			toolName: "run_shell_command",
			args:     map[string]any{"command": "this is a very long command that should be truncated to fit within the display limit"},
			want:     "Run command: this is a very long command that should be truncat...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatRequestDescription(tt.toolName, tt.args)
			if got != tt.want {
				t.Errorf("FormatRequestDescription() = %v, want %v", got, tt.want)
			}
		})
	}
}
