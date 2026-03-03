package loop

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/internal/approval"
)

// E2E Test: Tool-Use with Approvals (G-003)
//
// This test suite validates the end-to-end integration between
// the Executor and Approval Manager for tool-use workflows.

// setupTestApprovalDB creates a temporary SQLite database for tests.
func setupTestApprovalDB(t *testing.T) *sql.DB {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "executor_approval_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})

	db, err := sql.Open("sqlite3", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// setupTestApprovalManager creates an approval manager for tests.
func setupTestApprovalManager(t *testing.T) *approval.Manager {
	t.Helper()
	db := setupTestApprovalDB(t)

	repo, err := approval.NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	return approval.NewManager(repo)
}

// TestE2E_ToolUseWithApprovals_AutoApprovalByPolicy tests that low-risk
// tools are auto-approved based on the default policy configuration.
func TestE2E_ToolUseWithApprovals_AutoApprovalByPolicy(t *testing.T) {
	manager := setupTestApprovalManager(t)
	tmpDir := t.TempDir()

	// Create a test file to read
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("Hello, World!"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	var events []*LoopEvent
	var mu sync.Mutex

	executor := NewExecutor(ExecutorConfig{
		WorkDir:               tmpDir,
		SessionID:             "session-auto-approve",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
		EventCallback: func(event *LoopEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	})

	// Execute a low-risk read_file tool call - should be auto-approved
	input, _ := json.Marshal(map[string]interface{}{
		"path": testFile,
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-auto-1",
		ToolName:  "read_file",
		ToolInput: input,
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the tool executed successfully
	if result.IsError {
		t.Errorf("expected success, got error: %s", result.Output)
	}
	if result.Output != "Hello, World!" {
		t.Errorf("expected 'Hello, World!', got %q", result.Output)
	}

	// Verify auto-approval event was emitted
	mu.Lock()
	defer mu.Unlock()

	foundAutoApproval := false
	for _, event := range events {
		if event.Type == ToolAutoApproved {
			foundAutoApproval = true
			break
		}
	}

	if !foundAutoApproval {
		t.Error("expected ToolAutoApproved event to be emitted")
	}

	// Verify the approval request was recorded
	ctx := context.Background()
	request, err := manager.GetRequestByToolCallID(ctx, "call-auto-1")
	if err != nil {
		t.Fatalf("failed to get approval request: %v", err)
	}

	if request.Status != approval.RequestStatusAutoApproved {
		t.Errorf("expected status auto_approved, got %s", request.Status)
	}
	if request.RiskLevel != approval.RiskLevelLow {
		t.Errorf("expected risk level low, got %s", request.RiskLevel)
	}
}

// TestE2E_ToolUseWithApprovals_ManualApproval tests the workflow where
// a high-risk tool requires manual approval before execution.
func TestE2E_ToolUseWithApprovals_ManualApproval(t *testing.T) {
	manager := setupTestApprovalManager(t)

	var events []*LoopEvent
	var mu sync.Mutex

	executor := NewExecutor(ExecutorConfig{
		SessionID:             "session-manual-approve",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
		EventCallback: func(event *LoopEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	})

	// Execute a high-risk run_shell_command - should require approval
	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'test'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-manual-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	// Start execution in a goroutine (it will wait for approval)
	resultCh := make(chan *ToolResult, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := executor.Execute(context.Background(), toolCall)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	// Wait a bit for the approval request to be created
	time.Sleep(100 * time.Millisecond)

	// Verify the approval request is pending
	ctx := context.Background()
	request, err := manager.GetRequestByToolCallID(ctx, "call-manual-1")
	if err != nil {
		t.Fatalf("failed to get approval request: %v", err)
	}

	if request.Status != approval.RequestStatusPending {
		t.Fatalf("expected status pending, got %s", request.Status)
	}
	if request.RiskLevel != approval.RiskLevelHigh {
		t.Errorf("expected risk level high, got %s", request.RiskLevel)
	}

	// Approve the request
	err = manager.Approve(ctx, request.ID, "test-admin", "Approved for testing")
	if err != nil {
		t.Fatalf("failed to approve request: %v", err)
	}

	// Wait for the result
	select {
	case result := <-resultCh:
		if result.IsError {
			t.Errorf("expected success after approval, got error: %s", result.Output)
		}
		if result.Output != "test\n" {
			t.Errorf("expected 'test\\n', got %q", result.Output)
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for tool execution after approval")
	}

	// Verify approval event was emitted
	mu.Lock()
	defer mu.Unlock()

	foundApproval := false
	for _, event := range events {
		if event.Type == ToolCallApproved {
			foundApproval = true
			break
		}
	}

	if !foundApproval {
		t.Error("expected ToolCallApproved event to be emitted")
	}
}

// TestE2E_ToolUseWithApprovals_Rejection tests that a rejected tool
// call returns an error and does not execute.
func TestE2E_ToolUseWithApprovals_Rejection(t *testing.T) {
	manager := setupTestApprovalManager(t)

	var events []*LoopEvent
	var mu sync.Mutex

	executor := NewExecutor(ExecutorConfig{
		SessionID:             "session-reject",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
		EventCallback: func(event *LoopEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	})

	// Execute a high-risk command - will be rejected
	input, _ := json.Marshal(map[string]interface{}{
		"command": "rm -rf /",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-reject-1",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	// Start execution in a goroutine
	resultCh := make(chan *ToolResult, 1)
	errCh := make(chan error, 1)

	go func() {
		result, err := executor.Execute(context.Background(), toolCall)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- result
	}()

	// Wait for the approval request to be created
	time.Sleep(100 * time.Millisecond)

	// Reject the request
	ctx := context.Background()
	request, _ := manager.GetRequestByToolCallID(ctx, "call-reject-1")
	err := manager.Reject(ctx, request.ID, "test-admin", "Command is dangerous")
	if err != nil {
		t.Fatalf("failed to reject request: %v", err)
	}

	// Wait for the result
	select {
	case result := <-resultCh:
		if !result.IsError {
			t.Error("expected error for rejected tool call")
		}
		if result.Output != "Tool call was rejected by approval policy" {
			t.Errorf("unexpected rejection message: %s", result.Output)
		}
	case err := <-errCh:
		t.Fatalf("unexpected error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for rejection result")
	}

	// Verify rejection event was emitted
	mu.Lock()
	defer mu.Unlock()

	foundRejection := false
	for _, event := range events {
		if event.Type == ToolCallRejected {
			foundRejection = true
			break
		}
	}

	if !foundRejection {
		t.Error("expected ToolCallRejected event to be emitted")
	}
}

// TestE2E_ToolUseWithApprovals_DifferentRiskLevels tests that tools
// are correctly classified by risk level and handled accordingly.
func TestE2E_ToolUseWithApprovals_DifferentRiskLevels(t *testing.T) {
	ctx := context.Background()
	manager := setupTestApprovalManager(t)
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)

	executor := NewExecutor(ExecutorConfig{
		WorkDir:               tmpDir,
		SessionID:             "session-risk-levels",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
	})

	tests := []struct {
		name              string
		toolName          string
		toolInput         map[string]interface{}
		expectedRiskLevel approval.RiskLevel
		shouldAutoApprove bool
	}{
		{
			name:     "low_risk_read_file",
			toolName: "read_file",
			toolInput: map[string]interface{}{
				"path": testFile,
			},
			expectedRiskLevel: approval.RiskLevelLow,
			shouldAutoApprove: true,
		},
		{
			name:     "medium_risk_write_file",
			toolName: "write_file",
			toolInput: map[string]interface{}{
				"path":    filepath.Join(tmpDir, "new_file.txt"),
				"content": "new content",
			},
			expectedRiskLevel: approval.RiskLevelMedium,
			shouldAutoApprove: false, // Default policy auto-approves only low risk
		},
		{
			name:     "high_risk_shell_command",
			toolName: "run_shell_command",
			toolInput: map[string]interface{}{
				"command": "ls",
			},
			expectedRiskLevel: approval.RiskLevelHigh,
			shouldAutoApprove: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, _ := json.Marshal(tt.toolInput)
			toolCallID := "call-" + tt.name

			toolCall := agent.ContentBlock{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: toolCallID,
				ToolName:  tt.toolName,
				ToolInput: input,
			}

			if tt.shouldAutoApprove {
				// Execute directly - should auto-approve
				result, err := executor.Execute(context.Background(), toolCall)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if result.IsError {
					t.Errorf("expected success, got error: %s", result.Output)
				}

				// Verify auto-approval
				request, err := manager.GetRequestByToolCallID(ctx, toolCallID)
				if err != nil {
					t.Fatalf("failed to get request: %v", err)
				}
				if request.Status != approval.RequestStatusAutoApproved {
					t.Errorf("expected auto_approved, got %s", request.Status)
				}
			} else {
				// Execute in goroutine and manually approve
				resultCh := make(chan *ToolResult, 1)
				go func() {
					result, _ := executor.Execute(context.Background(), toolCall)
					resultCh <- result
				}()

				// Wait for request to be created
				time.Sleep(100 * time.Millisecond)

				// Verify risk level
				request, err := manager.GetRequestByToolCallID(ctx, toolCallID)
				if err != nil {
					t.Fatalf("failed to get request: %v", err)
				}
				if request.RiskLevel != tt.expectedRiskLevel {
					t.Errorf("expected risk level %s, got %s", tt.expectedRiskLevel, request.RiskLevel)
				}

				// Approve to allow execution to complete
				manager.Approve(ctx, request.ID, "admin", "")

				select {
				case result := <-resultCh:
					if result.IsError {
						t.Errorf("expected success after approval, got error: %s", result.Output)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("timeout waiting for result")
				}
			}
		})
	}
}

// TestE2E_ToolUseWithApprovals_PolicyModes tests different approval policy modes.
func TestE2E_ToolUseWithApprovals_PolicyModes(t *testing.T) {
	ctx := context.Background()

	t.Run("always_ask_mode", func(t *testing.T) {
		manager := setupTestApprovalManager(t)

		// Create and set a policy with always_ask mode
		policy, _ := manager.CreatePolicy(ctx, "Always Ask", approval.ApprovalModeAlwaysAsk, approval.RiskLevelLow)
		manager.SetDefaultPolicy(ctx, policy.ID)

		executor := NewExecutor(ExecutorConfig{
			SessionID:             "session-always-ask",
			ApprovalManager:       manager,
			ApprovalTimeout:       2 * time.Second,
			ApprovalCheckInterval: 50 * time.Millisecond,
		})

		// Even a low-risk read_file should require approval
		input, _ := json.Marshal(map[string]interface{}{
			"path": "/tmp/test.txt",
		})

		toolCall := agent.ContentBlock{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-always-ask",
			ToolName:  "read_file",
			ToolInput: input,
		}

		resultCh := make(chan *ToolResult, 1)
		go func() {
			result, _ := executor.Execute(context.Background(), toolCall)
			resultCh <- result
		}()

		time.Sleep(100 * time.Millisecond)

		// Verify request is pending
		request, _ := manager.GetRequestByToolCallID(ctx, "call-always-ask")
		if request.Status != approval.RequestStatusPending {
			t.Errorf("expected pending status in always_ask mode, got %s", request.Status)
		}

		// Approve to complete the test
		manager.Approve(ctx, request.ID, "admin", "")
		<-resultCh
	})

	t.Run("auto_approve_mode", func(t *testing.T) {
		manager := setupTestApprovalManager(t)

		// Create and set a policy with auto_approve mode
		policy, _ := manager.CreatePolicy(ctx, "Auto Approve", approval.ApprovalModeAutoApprove, approval.RiskLevelCritical)
		manager.SetDefaultPolicy(ctx, policy.ID)

		executor := NewExecutor(ExecutorConfig{
			SessionID:       "session-auto-approve-mode",
			ApprovalManager: manager,
		})

		// Even a high-risk command should be auto-approved
		input, _ := json.Marshal(map[string]interface{}{
			"command": "echo 'test'",
		})

		toolCall := agent.ContentBlock{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-auto-approve-mode",
			ToolName:  "run_shell_command",
			ToolInput: input,
		}

		result, err := executor.Execute(context.Background(), toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Errorf("expected success in auto_approve mode, got error: %s", result.Output)
		}

		// Verify auto-approval
		request, _ := manager.GetRequestByToolCallID(ctx, "call-auto-approve-mode")
		if request.Status != approval.RequestStatusAutoApproved {
			t.Errorf("expected auto_approved in auto_approve mode, got %s", request.Status)
		}
	})

	t.Run("risk_based_mode", func(t *testing.T) {
		manager := setupTestApprovalManager(t)

		// Create a policy that auto-approves medium risk and below
		policy, _ := manager.CreatePolicy(ctx, "Risk Based Medium", approval.ApprovalModeRiskBased, approval.RiskLevelMedium)
		manager.SetDefaultPolicy(ctx, policy.ID)

		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")
		os.WriteFile(testFile, []byte("content"), 0644)

		executor := NewExecutor(ExecutorConfig{
			WorkDir:               tmpDir,
			SessionID:             "session-risk-based",
			ApprovalManager:       manager,
			ApprovalTimeout:       2 * time.Second,
			ApprovalCheckInterval: 50 * time.Millisecond,
		})

		// Medium risk write_file should be auto-approved
		writeInput, _ := json.Marshal(map[string]interface{}{
			"path":    filepath.Join(tmpDir, "output.txt"),
			"content": "new content",
		})

		writeCall := agent.ContentBlock{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-risk-based-write",
			ToolName:  "write_file",
			ToolInput: writeInput,
		}

		result, _ := executor.Execute(context.Background(), writeCall)
		if result.IsError {
			t.Errorf("expected medium-risk write to succeed in risk_based mode: %s", result.Output)
		}

		request, _ := manager.GetRequestByToolCallID(ctx, "call-risk-based-write")
		if request.Status != approval.RequestStatusAutoApproved {
			t.Errorf("expected medium-risk to be auto_approved, got %s", request.Status)
		}

		// High risk shell command should require approval
		shellInput, _ := json.Marshal(map[string]interface{}{
			"command": "ls",
		})

		shellCall := agent.ContentBlock{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-risk-based-shell",
			ToolName:  "run_shell_command",
			ToolInput: shellInput,
		}

		resultCh := make(chan *ToolResult, 1)
		go func() {
			r, _ := executor.Execute(context.Background(), shellCall)
			resultCh <- r
		}()

		time.Sleep(100 * time.Millisecond)

		shellRequest, _ := manager.GetRequestByToolCallID(ctx, "call-risk-based-shell")
		if shellRequest.Status != approval.RequestStatusPending {
			t.Errorf("expected high-risk to be pending, got %s", shellRequest.Status)
		}

		manager.Approve(ctx, shellRequest.ID, "admin", "")
		<-resultCh
	})

	t.Run("session_trust_mode", func(t *testing.T) {
		manager := setupTestApprovalManager(t)

		// Create a policy with session_trust mode
		policy, _ := manager.CreatePolicy(ctx, "Session Trust", approval.ApprovalModeSessionTrust, approval.RiskLevelLow)
		manager.SetDefaultPolicy(ctx, policy.ID)

		sessionID := "session-trust-test"

		executor := NewExecutor(ExecutorConfig{
			SessionID:             sessionID,
			ApprovalManager:       manager,
			ApprovalTimeout:       2 * time.Second,
			ApprovalCheckInterval: 50 * time.Millisecond,
		})

		// Without trust, should require approval
		input1, _ := json.Marshal(map[string]interface{}{
			"command": "echo 'first'",
		})

		toolCall1 := agent.ContentBlock{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-trust-1",
			ToolName:  "run_shell_command",
			ToolInput: input1,
		}

		resultCh := make(chan *ToolResult, 1)
		go func() {
			r, _ := executor.Execute(context.Background(), toolCall1)
			resultCh <- r
		}()

		time.Sleep(100 * time.Millisecond)

		request1, _ := manager.GetRequestByToolCallID(ctx, "call-trust-1")
		if request1.Status != approval.RequestStatusPending {
			t.Errorf("expected pending without session trust, got %s", request1.Status)
		}

		// Approve and establish trust
		manager.Approve(ctx, request1.ID, "admin", "")
		manager.EstablishSessionTrust(sessionID)
		<-resultCh

		// With trust, should auto-approve
		input2, _ := json.Marshal(map[string]interface{}{
			"command": "echo 'second'",
		})

		toolCall2 := agent.ContentBlock{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-trust-2",
			ToolName:  "run_shell_command",
			ToolInput: input2,
		}

		result2, _ := executor.Execute(context.Background(), toolCall2)
		if result2.IsError {
			t.Errorf("expected success with session trust: %s", result2.Output)
		}

		request2, _ := manager.GetRequestByToolCallID(ctx, "call-trust-2")
		if request2.Status != approval.RequestStatusAutoApproved {
			t.Errorf("expected auto_approved with session trust, got %s", request2.Status)
		}
	})
}

// TestE2E_ToolUseWithApprovals_TrustedToolsList tests that tools in
// the trusted list are always auto-approved.
func TestE2E_ToolUseWithApprovals_TrustedToolsList(t *testing.T) {
	ctx := context.Background()
	manager := setupTestApprovalManager(t)

	// Create a policy with always_ask mode but with trusted tools list
	policy, _ := manager.CreatePolicy(ctx, "Trusted Tools Policy", approval.ApprovalModeAlwaysAsk, approval.RiskLevelLow)
	policy.AutoApproveTrustedTools = `["read_file", "run_shell_command"]`
	policy.UpdatedAt = time.Now().UTC()
	manager.UpdatePolicy(ctx, policy)
	manager.SetDefaultPolicy(ctx, policy.ID)

	executor := NewExecutor(ExecutorConfig{
		SessionID:       "session-trusted-list",
		ApprovalManager: manager,
	})

	// High-risk tool in trusted list should be auto-approved
	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'trusted'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-trusted-list",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected trusted tool to succeed: %s", result.Output)
	}

	request, _ := manager.GetRequestByToolCallID(ctx, "call-trusted-list")
	if request.Status != approval.RequestStatusAutoApproved {
		t.Errorf("expected trusted tool to be auto_approved, got %s", request.Status)
	}
}

// TestE2E_ToolUseWithApprovals_AlwaysRequireApprovalList tests that tools
// in the always-require list always require approval, even in auto_approve mode.
func TestE2E_ToolUseWithApprovals_AlwaysRequireApprovalList(t *testing.T) {
	ctx := context.Background()
	manager := setupTestApprovalManager(t)

	// Create a policy with auto_approve mode but with always-require list
	policy, _ := manager.CreatePolicy(ctx, "Always Require Policy", approval.ApprovalModeAutoApprove, approval.RiskLevelCritical)
	policy.AlwaysRequireApprovalTools = `["run_shell_command"]`
	policy.UpdatedAt = time.Now().UTC()
	manager.UpdatePolicy(ctx, policy)
	manager.SetDefaultPolicy(ctx, policy.ID)

	executor := NewExecutor(ExecutorConfig{
		SessionID:             "session-always-require",
		ApprovalManager:       manager,
		ApprovalTimeout:       2 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
	})

	// Shell command should require approval even with auto_approve mode
	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'blocked'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-always-require",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	resultCh := make(chan *ToolResult, 1)
	go func() {
		r, _ := executor.Execute(context.Background(), toolCall)
		resultCh <- r
	}()

	time.Sleep(100 * time.Millisecond)

	request, _ := manager.GetRequestByToolCallID(ctx, "call-always-require")
	if request.Status != approval.RequestStatusPending {
		t.Errorf("expected tool in always-require list to be pending, got %s", request.Status)
	}

	// Approve to complete test
	manager.Approve(ctx, request.ID, "admin", "")
	<-resultCh
}

// TestE2E_ToolUseWithApprovals_Timeout tests that approval requests
// timeout correctly when no decision is made.
func TestE2E_ToolUseWithApprovals_Timeout(t *testing.T) {
	manager := setupTestApprovalManager(t)

	executor := NewExecutor(ExecutorConfig{
		SessionID:             "session-timeout",
		ApprovalManager:       manager,
		ApprovalTimeout:       500 * time.Millisecond, // Short timeout
		ApprovalCheckInterval: 50 * time.Millisecond,
	})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'timeout test'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-timeout",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	// Execute with a context that will timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := executor.Execute(ctx, toolCall)
	if err == nil && !result.IsError {
		t.Error("expected error or error result due to timeout")
	}
}

// TestE2E_ToolUseWithApprovals_NoApprovalManager tests that tool execution
// works correctly when no approval manager is configured.
func TestE2E_ToolUseWithApprovals_NoApprovalManager(t *testing.T) {
	var events []*LoopEvent
	var mu sync.Mutex

	executor := NewExecutor(ExecutorConfig{
		SessionID: "session-no-manager",
		// ApprovalManager is nil
		EventCallback: func(event *LoopEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	})

	// Even high-risk commands should work without approval manager
	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'no manager'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-no-manager",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	result, err := executor.Execute(context.Background(), toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success without approval manager: %s", result.Output)
	}
	if result.Output != "no manager\n" {
		t.Errorf("expected 'no manager\\n', got %q", result.Output)
	}

	// Should emit auto-approval event (no manager = auto-approve)
	mu.Lock()
	defer mu.Unlock()

	foundAutoApproval := false
	for _, event := range events {
		if event.Type == ToolAutoApproved {
			foundAutoApproval = true
			break
		}
	}

	if !foundAutoApproval {
		t.Error("expected ToolAutoApproved event when no manager configured")
	}
}

// TestE2E_ToolUseWithApprovals_BatchExecution tests that batch tool
// execution handles approval correctly for each tool.
func TestE2E_ToolUseWithApprovals_BatchExecution(t *testing.T) {
	ctx := context.Background()
	manager := setupTestApprovalManager(t)
	tmpDir := t.TempDir()

	// Create test file for reading
	testFile := filepath.Join(tmpDir, "batch_test.txt")
	os.WriteFile(testFile, []byte("batch content"), 0644)

	executor := NewExecutor(ExecutorConfig{
		WorkDir:               tmpDir,
		SessionID:             "session-batch",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
	})

	// Create a batch with mixed risk levels
	toolCalls := []agent.ContentBlock{
		{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-batch-read",
			ToolName:  "read_file",
			ToolInput: json.RawMessage(`{"path": "` + testFile + `"}`),
		},
	}

	// Execute batch
	results, err := executor.ExecuteBatch(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify first result (low-risk, auto-approved)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].IsError {
		t.Errorf("expected first tool to succeed: %s", results[0].Output)
	}

	// Verify the request was auto-approved
	request, _ := manager.GetRequestByToolCallID(ctx, "call-batch-read")
	if request.Status != approval.RequestStatusAutoApproved {
		t.Errorf("expected batch read to be auto_approved, got %s", request.Status)
	}
}

// TestE2E_ToolUseWithApprovals_ConcurrentRequests tests that multiple
// concurrent approval requests are handled correctly.
func TestE2E_ToolUseWithApprovals_ConcurrentRequests(t *testing.T) {
	ctx := context.Background()
	manager := setupTestApprovalManager(t)

	executor := NewExecutor(ExecutorConfig{
		SessionID:             "session-concurrent",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
	})

	const numRequests = 5
	var wg sync.WaitGroup
	results := make([]*ToolResult, numRequests)
	errors := make([]error, numRequests)

	// Start multiple concurrent requests
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			input, _ := json.Marshal(map[string]interface{}{
				"command": "echo 'concurrent " + string(rune('0'+idx)) + "'",
			})

			toolCall := agent.ContentBlock{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "call-concurrent-" + string(rune('0'+idx)),
				ToolName:  "run_shell_command",
				ToolInput: input,
			}

			results[idx], errors[idx] = executor.Execute(context.Background(), toolCall)
		}(i)
	}

	// Wait for requests to be created
	time.Sleep(200 * time.Millisecond)

	// Approve all pending requests
	pending, _ := manager.ListPendingRequests(ctx, "session-concurrent")
	for _, req := range pending {
		manager.Approve(ctx, req.ID, "admin", "")
	}

	wg.Wait()

	// Verify all succeeded
	for i := 0; i < numRequests; i++ {
		if errors[i] != nil {
			t.Errorf("request %d error: %v", i, errors[i])
		}
		if results[i] != nil && results[i].IsError {
			t.Errorf("request %d failed: %s", i, results[i].Output)
		}
	}
}

// TestE2E_ToolUseWithApprovals_ApprovalDecisionRecording tests that
// approval decisions are correctly recorded in the database.
func TestE2E_ToolUseWithApprovals_ApprovalDecisionRecording(t *testing.T) {
	ctx := context.Background()
	manager := setupTestApprovalManager(t)

	executor := NewExecutor(ExecutorConfig{
		SessionID:             "session-decision-record",
		ApprovalManager:       manager,
		ApprovalTimeout:       5 * time.Second,
		ApprovalCheckInterval: 50 * time.Millisecond,
	})

	input, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'decision test'",
	})

	toolCall := agent.ContentBlock{
		Type:      agent.ContentTypeToolUse,
		ToolUseID: "call-decision-record",
		ToolName:  "run_shell_command",
		ToolInput: input,
	}

	resultCh := make(chan *ToolResult, 1)
	go func() {
		r, _ := executor.Execute(context.Background(), toolCall)
		resultCh <- r
	}()

	time.Sleep(100 * time.Millisecond)

	// Get the request
	request, _ := manager.GetRequestByToolCallID(ctx, "call-decision-record")

	// Approve with a specific reason
	manager.Approve(ctx, request.ID, "test-approver", "Approved for testing purposes")

	<-resultCh

	// Verify the decision was recorded
	decision, err := manager.GetDecision(ctx, request.ID)
	if err != nil {
		t.Fatalf("failed to get decision: %v", err)
	}

	if decision.DecidedBy != "test-approver" {
		t.Errorf("expected decided_by 'test-approver', got %s", decision.DecidedBy)
	}
	if decision.Reason != "Approved for testing purposes" {
		t.Errorf("expected reason 'Approved for testing purposes', got %s", decision.Reason)
	}
	if decision.Decision != approval.RequestStatusApproved {
		t.Errorf("expected decision 'approved', got %s", decision.Decision)
	}
}
