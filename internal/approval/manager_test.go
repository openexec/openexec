package approval

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Create a temporary file for the test database
	tmpFile, err := os.CreateTemp("", "approval_test_*.db")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	tmpFile.Close()

	t.Cleanup(func() {
		os.Remove(tmpFile.Name())
	})

	db, err := sql.Open("sqlite", tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

func setupTestRepo(t *testing.T) *SQLiteRepository {
	t.Helper()
	db := setupTestDB(t)

	repo, err := NewSQLiteRepository(db)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}

	return repo
}

func setupTestManager(t *testing.T) *Manager {
	t.Helper()
	repo := setupTestRepo(t)
	return NewManager(repo)
}

// TestNewSQLiteRepository tests repository creation.
func TestNewSQLiteRepository(t *testing.T) {
	t.Run("creates repository with valid db", func(t *testing.T) {
		db := setupTestDB(t)
		repo, err := NewSQLiteRepository(db)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if repo == nil {
			t.Fatal("expected non-nil repository")
		}
	})

	t.Run("returns error for nil db", func(t *testing.T) {
		_, err := NewSQLiteRepository(nil)
		if err == nil {
			t.Fatal("expected error for nil db")
		}
	})
}

// TestSQLiteRepository_RequestOperations tests CRUD operations for approval requests.
func TestSQLiteRepository_RequestOperations(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	t.Run("creates and retrieves request", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "tool-call-1", "write_file", `{"path": "/test.txt"}`, RiskLevelMedium, "user-1")
		err := repo.CreateRequest(ctx, request)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		retrieved, err := repo.GetRequest(ctx, request.ID)
		if err != nil {
			t.Fatalf("failed to get request: %v", err)
		}
		if retrieved.ID != request.ID {
			t.Errorf("expected ID %s, got %s", request.ID, retrieved.ID)
		}
		if retrieved.ToolName != "write_file" {
			t.Errorf("expected tool_name 'write_file', got %s", retrieved.ToolName)
		}
	})

	t.Run("retrieves request by tool call ID", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-2", "tool-call-2", "read_file", "{}", RiskLevelLow, "user-1")
		repo.CreateRequest(ctx, request)

		retrieved, err := repo.GetRequestByToolCallID(ctx, "tool-call-2")
		if err != nil {
			t.Fatalf("failed to get request by tool call ID: %v", err)
		}
		if retrieved.ToolCallID != "tool-call-2" {
			t.Errorf("expected tool_call_id 'tool-call-2', got %s", retrieved.ToolCallID)
		}
	})

	t.Run("updates request", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-3", "tool-call-3", "run_shell_command", "{}", RiskLevelHigh, "user-1")
		repo.CreateRequest(ctx, request)

		request.Approve()
		err := repo.UpdateRequest(ctx, request)
		if err != nil {
			t.Fatalf("failed to update request: %v", err)
		}

		retrieved, _ := repo.GetRequest(ctx, request.ID)
		if retrieved.Status != RequestStatusApproved {
			t.Errorf("expected status 'approved', got %s", retrieved.Status)
		}
	})

	t.Run("lists pending requests", func(t *testing.T) {
		sessionID := "session-pending"
		req1, _ := NewApprovalRequest(sessionID, "tc-p-1", "tool1", "{}", RiskLevelLow, "user-1")
		req2, _ := NewApprovalRequest(sessionID, "tc-p-2", "tool2", "{}", RiskLevelMedium, "user-1")
		req3, _ := NewApprovalRequest(sessionID, "tc-p-3", "tool3", "{}", RiskLevelHigh, "user-1")

		repo.CreateRequest(ctx, req1)
		repo.CreateRequest(ctx, req2)
		repo.CreateRequest(ctx, req3)

		// Approve one
		req1.Approve()
		repo.UpdateRequest(ctx, req1)

		pending, err := repo.ListPendingRequests(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to list pending requests: %v", err)
		}
		if len(pending) != 2 {
			t.Errorf("expected 2 pending requests, got %d", len(pending))
		}
	})

	t.Run("returns error for non-existent request", func(t *testing.T) {
		_, err := repo.GetRequest(ctx, "non-existent")
		if err != ErrApprovalRequestNotFound {
			t.Errorf("expected ErrApprovalRequestNotFound, got %v", err)
		}
	})
}

// TestSQLiteRepository_PolicyOperations tests CRUD operations for approval policies.
func TestSQLiteRepository_PolicyOperations(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	t.Run("creates and retrieves policy", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Test Policy", ApprovalModeRiskBased, RiskLevelMedium)
		err := repo.CreatePolicy(ctx, policy)
		if err != nil {
			t.Fatalf("failed to create policy: %v", err)
		}

		retrieved, err := repo.GetPolicy(ctx, policy.ID)
		if err != nil {
			t.Fatalf("failed to get policy: %v", err)
		}
		if retrieved.Name != "Test Policy" {
			t.Errorf("expected name 'Test Policy', got %s", retrieved.Name)
		}
		if retrieved.Mode != ApprovalModeRiskBased {
			t.Errorf("expected mode 'risk_based', got %s", retrieved.Mode)
		}
	})

	t.Run("retrieves policy by name", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Named Policy", ApprovalModeAlwaysAsk, RiskLevelLow)
		repo.CreatePolicy(ctx, policy)

		retrieved, err := repo.GetPolicyByName(ctx, "Named Policy")
		if err != nil {
			t.Fatalf("failed to get policy by name: %v", err)
		}
		if retrieved.ID != policy.ID {
			t.Errorf("expected ID %s, got %s", policy.ID, retrieved.ID)
		}
	})

	t.Run("updates policy", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Update Test", ApprovalModeRiskBased, RiskLevelLow)
		repo.CreatePolicy(ctx, policy)

		policy.Mode = ApprovalModeAutoApprove
		policy.UpdatedAt = time.Now().UTC()
		err := repo.UpdatePolicy(ctx, policy)
		if err != nil {
			t.Fatalf("failed to update policy: %v", err)
		}

		retrieved, _ := repo.GetPolicy(ctx, policy.ID)
		if retrieved.Mode != ApprovalModeAutoApprove {
			t.Errorf("expected mode 'auto_approve', got %s", retrieved.Mode)
		}
	})

	t.Run("deletes policy", func(t *testing.T) {
		policy, _ := NewApprovalPolicy("Delete Test", ApprovalModeRiskBased, RiskLevelLow)
		repo.CreatePolicy(ctx, policy)

		err := repo.DeletePolicy(ctx, policy.ID)
		if err != nil {
			t.Fatalf("failed to delete policy: %v", err)
		}

		_, err = repo.GetPolicy(ctx, policy.ID)
		if err != ErrApprovalPolicyNotFound {
			t.Errorf("expected ErrApprovalPolicyNotFound, got %v", err)
		}
	})

	t.Run("lists active policies", func(t *testing.T) {
		p1, _ := NewApprovalPolicy("Active 1", ApprovalModeRiskBased, RiskLevelLow)
		p2, _ := NewApprovalPolicy("Active 2", ApprovalModeAlwaysAsk, RiskLevelLow)
		p3, _ := NewApprovalPolicy("Inactive", ApprovalModeAutoApprove, RiskLevelLow)
		p3.IsActive = false

		repo.CreatePolicy(ctx, p1)
		repo.CreatePolicy(ctx, p2)
		repo.CreatePolicy(ctx, p3)

		policies, err := repo.ListActivePolicies(ctx)
		if err != nil {
			t.Fatalf("failed to list active policies: %v", err)
		}

		// Should include default policy + the two active ones we created
		activeCount := 0
		for _, p := range policies {
			if p.IsActive {
				activeCount++
			}
		}
		if activeCount < 2 {
			t.Errorf("expected at least 2 active policies, got %d", activeCount)
		}
	})

	t.Run("gets default policy", func(t *testing.T) {
		policy, err := repo.GetDefaultPolicy(ctx)
		if err != nil {
			t.Fatalf("failed to get default policy: %v", err)
		}
		if !policy.IsDefault {
			t.Error("expected is_default to be true")
		}
	})
}

// TestSQLiteRepository_DecisionOperations tests CRUD operations for approval decisions.
func TestSQLiteRepository_DecisionOperations(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	// Create a request first
	request, _ := NewApprovalRequest("session-d", "tool-call-d", "test_tool", "{}", RiskLevelMedium, "user-1")
	repo.CreateRequest(ctx, request)

	t.Run("creates and retrieves decision", func(t *testing.T) {
		decision, _ := NewApprovalDecision(request.ID, RequestStatusApproved, "user-1")
		decision.SetReason("Test approval")

		err := repo.CreateDecision(ctx, decision)
		if err != nil {
			t.Fatalf("failed to create decision: %v", err)
		}

		retrieved, err := repo.GetDecision(ctx, decision.ID)
		if err != nil {
			t.Fatalf("failed to get decision: %v", err)
		}
		if retrieved.Decision != RequestStatusApproved {
			t.Errorf("expected decision 'approved', got %s", retrieved.Decision)
		}
		if retrieved.Reason != "Test approval" {
			t.Errorf("expected reason 'Test approval', got %s", retrieved.Reason)
		}
	})

	t.Run("retrieves decision by request ID", func(t *testing.T) {
		req, _ := NewApprovalRequest("session-d2", "tool-call-d2", "test_tool", "{}", RiskLevelMedium, "user-1")
		repo.CreateRequest(ctx, req)

		decision, _ := NewApprovalDecision(req.ID, RequestStatusRejected, "admin")
		repo.CreateDecision(ctx, decision)

		retrieved, err := repo.GetDecisionByRequestID(ctx, req.ID)
		if err != nil {
			t.Fatalf("failed to get decision by request ID: %v", err)
		}
		if retrieved.RequestID != req.ID {
			t.Errorf("expected request_id %s, got %s", req.ID, retrieved.RequestID)
		}
	})
}

// TestSQLiteRepository_Statistics tests statistics queries.
func TestSQLiteRepository_Statistics(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	sessionID := "session-stats"

	// Create requests with various statuses
	req1, _ := NewApprovalRequest(sessionID, "tc-s-1", "tool1", "{}", RiskLevelLow, "user-1")
	req2, _ := NewApprovalRequest(sessionID, "tc-s-2", "tool2", "{}", RiskLevelMedium, "user-1")
	req3, _ := NewApprovalRequest(sessionID, "tc-s-3", "tool3", "{}", RiskLevelHigh, "user-1")
	req4, _ := NewApprovalRequest(sessionID, "tc-s-4", "tool4", "{}", RiskLevelCritical, "user-1")

	repo.CreateRequest(ctx, req1)
	repo.CreateRequest(ctx, req2)
	repo.CreateRequest(ctx, req3)
	repo.CreateRequest(ctx, req4)

	req1.Approve()
	repo.UpdateRequest(ctx, req1)

	req2.Reject()
	repo.UpdateRequest(ctx, req2)

	t.Run("counts pending requests", func(t *testing.T) {
		count, err := repo.CountPendingRequests(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to count pending requests: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 pending requests, got %d", count)
		}
	})

	t.Run("counts requests by status", func(t *testing.T) {
		counts, err := repo.CountRequestsByStatus(ctx, sessionID)
		if err != nil {
			t.Fatalf("failed to count requests by status: %v", err)
		}

		if counts[RequestStatusApproved] != 1 {
			t.Errorf("expected 1 approved, got %d", counts[RequestStatusApproved])
		}
		if counts[RequestStatusRejected] != 1 {
			t.Errorf("expected 1 rejected, got %d", counts[RequestStatusRejected])
		}
		if counts[RequestStatusPending] != 2 {
			t.Errorf("expected 2 pending, got %d", counts[RequestStatusPending])
		}
	})
}

// TestManager_GetRiskLevel tests risk level lookup.
func TestManager_GetRiskLevel(t *testing.T) {
	manager := setupTestManager(t)

	t.Run("returns correct risk level for known tools", func(t *testing.T) {
		if manager.GetRiskLevel("read_file") != RiskLevelLow {
			t.Error("expected read_file to have low risk")
		}
		if manager.GetRiskLevel("write_file") != RiskLevelMedium {
			t.Error("expected write_file to have medium risk")
		}
		if manager.GetRiskLevel("run_shell_command") != RiskLevelHigh {
			t.Error("expected run_shell_command to have high risk")
		}
		if manager.GetRiskLevel("modify_orchestrator") != RiskLevelCritical {
			t.Error("expected modify_orchestrator to have critical risk")
		}
	})

	t.Run("returns medium for unknown tools", func(t *testing.T) {
		if manager.GetRiskLevel("unknown_tool") != RiskLevelMedium {
			t.Error("expected unknown tool to default to medium risk")
		}
	})

	t.Run("allows setting custom risk level", func(t *testing.T) {
		err := manager.SetRiskLevel("custom_tool", RiskLevelCritical)
		if err != nil {
			t.Fatalf("failed to set risk level: %v", err)
		}
		if manager.GetRiskLevel("custom_tool") != RiskLevelCritical {
			t.Error("expected custom_tool to have critical risk")
		}
	})
}

// TestManager_RequestApproval tests the approval request workflow.
func TestManager_RequestApproval(t *testing.T) {
	ctx := context.Background()

	t.Run("auto-approves low risk with default policy", func(t *testing.T) {
		manager := setupTestManager(t)

		request, autoApproved, err := manager.RequestApproval(ctx, "session-1", "tc-1", "read_file", "{}", "agent-1", "")
		if err != nil {
			t.Fatalf("failed to request approval: %v", err)
		}
		if !autoApproved {
			t.Error("expected low risk read_file to be auto-approved")
		}
		if request.Status != RequestStatusAutoApproved {
			t.Errorf("expected status 'auto_approved', got %s", request.Status)
		}
	})

	t.Run("requires approval for high risk tools", func(t *testing.T) {
		manager := setupTestManager(t)

		request, autoApproved, err := manager.RequestApproval(ctx, "session-2", "tc-2", "run_shell_command", "{}", "agent-1", "")
		if err != nil {
			t.Fatalf("failed to request approval: %v", err)
		}
		if autoApproved {
			t.Error("expected high risk run_shell_command to require approval")
		}
		if request.Status != RequestStatusPending {
			t.Errorf("expected status 'pending', got %s", request.Status)
		}
	})
}

// TestManager_ApproveReject tests manual approval and rejection.
func TestManager_ApproveReject(t *testing.T) {
	ctx := context.Background()

	t.Run("approves pending request", func(t *testing.T) {
		manager := setupTestManager(t)

		request, _, _ := manager.RequestApproval(ctx, "session-1", "tc-1", "run_shell_command", "{}", "agent-1", "")

		err := manager.Approve(ctx, request.ID, "admin", "Looks safe")
		if err != nil {
			t.Fatalf("failed to approve: %v", err)
		}

		updated, _ := manager.GetRequest(ctx, request.ID)
		if updated.Status != RequestStatusApproved {
			t.Errorf("expected status 'approved', got %s", updated.Status)
		}

		decision, _ := manager.GetDecision(ctx, request.ID)
		if decision.Reason != "Looks safe" {
			t.Errorf("expected reason 'Looks safe', got %s", decision.Reason)
		}
	})

	t.Run("rejects pending request", func(t *testing.T) {
		manager := setupTestManager(t)

		request, _, _ := manager.RequestApproval(ctx, "session-2", "tc-2", "run_shell_command", "{}", "agent-1", "")

		err := manager.Reject(ctx, request.ID, "admin", "Not allowed")
		if err != nil {
			t.Fatalf("failed to reject: %v", err)
		}

		updated, _ := manager.GetRequest(ctx, request.ID)
		if updated.Status != RequestStatusRejected {
			t.Errorf("expected status 'rejected', got %s", updated.Status)
		}
	})

	t.Run("cannot approve already decided request", func(t *testing.T) {
		manager := setupTestManager(t)

		request, _, _ := manager.RequestApproval(ctx, "session-3", "tc-3", "run_shell_command", "{}", "agent-1", "")
		manager.Approve(ctx, request.ID, "admin", "First approval")

		err := manager.Reject(ctx, request.ID, "admin", "Try to reject")
		if err != ErrAlreadyDecided {
			t.Errorf("expected ErrAlreadyDecided, got %v", err)
		}
	})
}

// TestManager_SessionTrust tests session trust management.
func TestManager_SessionTrust(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	// Create a policy with session_trust mode
	policy, _ := manager.CreatePolicy(ctx, "Session Trust Policy", ApprovalModeSessionTrust, RiskLevelLow)
	manager.SetDefaultPolicy(ctx, policy.ID)

	t.Run("requires approval without trust", func(t *testing.T) {
		_, autoApproved, _ := manager.RequestApproval(ctx, "session-t1", "tc-t1", "write_file", "{}", "agent-1", "")
		if autoApproved {
			t.Error("expected to require approval without session trust")
		}
	})

	t.Run("auto-approves with trust", func(t *testing.T) {
		manager.EstablishSessionTrust("session-t2")

		_, autoApproved, _ := manager.RequestApproval(ctx, "session-t2", "tc-t2", "write_file", "{}", "agent-1", "")
		if !autoApproved {
			t.Error("expected auto-approve with session trust")
		}
	})

	t.Run("trust can be revoked", func(t *testing.T) {
		manager.EstablishSessionTrust("session-t3")
		if !manager.HasSessionTrust("session-t3") {
			t.Error("expected session to have trust")
		}

		manager.RevokeSessionTrust("session-t3")
		if manager.HasSessionTrust("session-t3") {
			t.Error("expected session trust to be revoked")
		}
	})
}

// TestManager_CheckApproval tests approval status checking.
func TestManager_CheckApproval(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("returns pending status", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-c1", "tc-c1", "run_shell_command", "{}", "agent-1", "")

		result, err := manager.CheckApproval(ctx, "tc-c1")
		if err != nil {
			t.Fatalf("failed to check approval: %v", err)
		}
		if !result.Pending {
			t.Error("expected pending to be true")
		}
		if result.Request.ID != request.ID {
			t.Error("expected request IDs to match")
		}
	})

	t.Run("returns approved status", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-c2", "tc-c2", "run_shell_command", "{}", "agent-1", "")
		manager.Approve(ctx, request.ID, "admin", "")

		result, err := manager.CheckApproval(ctx, "tc-c2")
		if err != nil {
			t.Fatalf("failed to check approval: %v", err)
		}
		if !result.Approved {
			t.Error("expected approved to be true")
		}
		if result.Pending {
			t.Error("expected pending to be false")
		}
	})

	t.Run("returns error for non-existent tool call", func(t *testing.T) {
		_, err := manager.CheckApproval(ctx, "non-existent")
		if err != ErrApprovalRequestNotFound {
			t.Errorf("expected ErrApprovalRequestNotFound, got %v", err)
		}
	})
}

// TestManager_Cancel tests request cancellation.
func TestManager_Cancel(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("cancels pending request", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-x1", "tc-x1", "run_shell_command", "{}", "agent-1", "")

		err := manager.Cancel(ctx, request.ID, "Session ended")
		if err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}

		updated, _ := manager.GetRequest(ctx, request.ID)
		if updated.Status != RequestStatusCancelled {
			t.Errorf("expected status 'cancelled', got %s", updated.Status)
		}
	})
}

// TestManager_PolicyManagement tests policy CRUD operations through the manager.
func TestManager_PolicyManagement(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("creates policy", func(t *testing.T) {
		policy, err := manager.CreatePolicy(ctx, "Manager Policy", ApprovalModeRiskBased, RiskLevelMedium)
		if err != nil {
			t.Fatalf("failed to create policy: %v", err)
		}
		if policy.ID == "" {
			t.Error("expected policy ID to be generated")
		}
	})

	t.Run("gets policy by name", func(t *testing.T) {
		manager.CreatePolicy(ctx, "Named Manager Policy", ApprovalModeAlwaysAsk, RiskLevelLow)

		policy, err := manager.GetPolicyByName(ctx, "Named Manager Policy")
		if err != nil {
			t.Fatalf("failed to get policy by name: %v", err)
		}
		if policy.Mode != ApprovalModeAlwaysAsk {
			t.Errorf("expected mode 'always_ask', got %s", policy.Mode)
		}
	})

	t.Run("updates policy", func(t *testing.T) {
		policy, _ := manager.CreatePolicy(ctx, "Update Manager Policy", ApprovalModeRiskBased, RiskLevelLow)

		policy.Mode = ApprovalModeAutoApprove
		policy.UpdatedAt = time.Now().UTC()
		err := manager.UpdatePolicy(ctx, policy)
		if err != nil {
			t.Fatalf("failed to update policy: %v", err)
		}

		retrieved, _ := manager.GetPolicy(ctx, policy.ID)
		if retrieved.Mode != ApprovalModeAutoApprove {
			t.Errorf("expected mode 'auto_approve', got %s", retrieved.Mode)
		}
	})

	t.Run("sets default policy", func(t *testing.T) {
		policy, _ := manager.CreatePolicy(ctx, "New Default", ApprovalModeAlwaysAsk, RiskLevelLow)

		err := manager.SetDefaultPolicy(ctx, policy.ID)
		if err != nil {
			t.Fatalf("failed to set default policy: %v", err)
		}

		defaultPolicy, _ := manager.GetDefaultPolicy(ctx)
		if defaultPolicy.ID != policy.ID {
			t.Errorf("expected default policy ID %s, got %s", policy.ID, defaultPolicy.ID)
		}
	})
}

// TestManager_ExpireRequests tests request expiration.
func TestManager_ExpireRequests(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("expires pending requests past deadline", func(t *testing.T) {
		// Create a request with past expiration
		request, _ := NewApprovalRequest("session-e1", "tc-e1", "test_tool", "{}", RiskLevelMedium, "agent-1")
		request.SetExpiration(time.Now().Add(-1 * time.Hour)) // Already expired

		// Use the repo directly to avoid auto-approval
		manager.repo.(*SQLiteRepository).CreateRequest(ctx, request)

		count, err := manager.ExpireRequests(ctx)
		if err != nil {
			t.Fatalf("failed to expire requests: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 expired request, got %d", count)
		}

		updated, _ := manager.GetRequest(ctx, request.ID)
		if updated.Status != RequestStatusExpired {
			t.Errorf("expected status 'expired', got %s", updated.Status)
		}
	})
}

// TestManager_ApprovalStats tests statistics retrieval.
func TestManager_ApprovalStats(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	sessionID := "session-stats"

	// Create various requests
	_, _, _ = manager.RequestApproval(ctx, sessionID, "tc-st-1", "read_file", "{}", "agent-1", "")
	req2, _, _ := manager.RequestApproval(ctx, sessionID, "tc-st-2", "run_shell_command", "{}", "agent-1", "")
	req3, _, _ := manager.RequestApproval(ctx, sessionID, "tc-st-3", "run_shell_command", "{}", "agent-1", "")

	// First request is auto-approved (low risk read_file)
	// Approve req2, reject req3
	manager.Approve(ctx, req2.ID, "admin", "")
	manager.Reject(ctx, req3.ID, "admin", "")

	stats, err := manager.GetApprovalStats(ctx, sessionID)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	// req1 is auto_approved, req2 is approved, req3 is rejected
	if stats[RequestStatusAutoApproved] != 1 {
		t.Errorf("expected 1 auto_approved, got %d", stats[RequestStatusAutoApproved])
	}
	if stats[RequestStatusApproved] != 1 {
		t.Errorf("expected 1 approved, got %d", stats[RequestStatusApproved])
	}
	if stats[RequestStatusRejected] != 1 {
		t.Errorf("expected 1 rejected, got %d", stats[RequestStatusRejected])
	}
}

// TestManager_TrustedToolsList tests auto-approve trusted tools list.
func TestManager_TrustedToolsList(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	// Create a policy with trusted tools
	policy, _ := manager.CreatePolicy(ctx, "Trusted Tools Policy", ApprovalModeAlwaysAsk, RiskLevelLow)
	policy.AutoApproveTrustedTools = `["my_safe_tool", "another_safe_tool"]`
	policy.UpdatedAt = time.Now().UTC()
	manager.UpdatePolicy(ctx, policy)
	manager.SetDefaultPolicy(ctx, policy.ID)

	t.Run("auto-approves tools in trusted list", func(t *testing.T) {
		_, autoApproved, _ := manager.RequestApproval(ctx, "session-tt1", "tc-tt1", "my_safe_tool", "{}", "agent-1", "")
		if !autoApproved {
			t.Error("expected trusted tool to be auto-approved")
		}
	})

	t.Run("requires approval for tools not in trusted list", func(t *testing.T) {
		_, autoApproved, _ := manager.RequestApproval(ctx, "session-tt2", "tc-tt2", "untrusted_tool", "{}", "agent-1", "")
		if autoApproved {
			t.Error("expected untrusted tool to require approval")
		}
	})
}

// TestManager_AlwaysRequireApprovalList tests always-require-approval tools list.
func TestManager_AlwaysRequireApprovalList(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	// Create a policy that auto-approves everything except dangerous tools
	policy, _ := manager.CreatePolicy(ctx, "Dangerous Tools Policy", ApprovalModeAutoApprove, RiskLevelCritical)
	policy.AlwaysRequireApprovalTools = `["dangerous_tool", "deploy"]`
	policy.UpdatedAt = time.Now().UTC()
	manager.UpdatePolicy(ctx, policy)
	manager.SetDefaultPolicy(ctx, policy.ID)

	t.Run("requires approval for tools in always-require list", func(t *testing.T) {
		_, autoApproved, _ := manager.RequestApproval(ctx, "session-ar1", "tc-ar1", "dangerous_tool", "{}", "agent-1", "")
		if autoApproved {
			t.Error("expected dangerous tool to require approval despite auto_approve mode")
		}
	})

	t.Run("auto-approves other tools", func(t *testing.T) {
		_, autoApproved, _ := manager.RequestApproval(ctx, "session-ar2", "tc-ar2", "safe_tool", "{}", "agent-1", "")
		if !autoApproved {
			t.Error("expected other tools to be auto-approved")
		}
	})
}

// TestManager_SetRiskLevel tests risk level setting.
func TestManager_SetRiskLevel(t *testing.T) {
	manager := setupTestManager(t)

	t.Run("sets valid risk level", func(t *testing.T) {
		err := manager.SetRiskLevel("new_tool", RiskLevelHigh)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if manager.GetRiskLevel("new_tool") != RiskLevelHigh {
			t.Error("expected new_tool to have high risk")
		}
	})

	t.Run("returns error for invalid risk level", func(t *testing.T) {
		err := manager.SetRiskLevel("invalid_tool", RiskLevel("invalid"))
		if err == nil {
			t.Error("expected error for invalid risk level")
		}
	})

	t.Run("overrides existing risk level", func(t *testing.T) {
		manager.SetRiskLevel("override_tool", RiskLevelLow)
		if manager.GetRiskLevel("override_tool") != RiskLevelLow {
			t.Error("expected override_tool to have low risk")
		}

		manager.SetRiskLevel("override_tool", RiskLevelCritical)
		if manager.GetRiskLevel("override_tool") != RiskLevelCritical {
			t.Error("expected override_tool to now have critical risk")
		}
	})
}

// TestManager_WaitForApproval tests waiting for approval decisions.
func TestManager_WaitForApproval(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("returns when request is approved", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-w1", "tc-w1", "run_shell_command", "{}", "agent-1", "")

		// Approve in a goroutine
		go func() {
			time.Sleep(50 * time.Millisecond)
			manager.Approve(ctx, request.ID, "admin", "Approved")
		}()

		// Wait with timeout
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		result, err := manager.WaitForApproval(waitCtx, request.ID, 20*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Approved {
			t.Error("expected result to be approved")
		}
	})

	t.Run("returns when request is rejected", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-w2", "tc-w2", "run_shell_command", "{}", "agent-1", "")

		// Reject in a goroutine
		go func() {
			time.Sleep(50 * time.Millisecond)
			manager.Reject(ctx, request.ID, "admin", "Rejected")
		}()

		// Wait with timeout
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		result, err := manager.WaitForApproval(waitCtx, request.ID, 20*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Approved {
			t.Error("expected result to not be approved")
		}
	})

	t.Run("times out when context cancelled", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-w3", "tc-w3", "run_shell_command", "{}", "agent-1", "")

		// Short timeout context
		waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()

		_, err := manager.WaitForApproval(waitCtx, request.ID, 20*time.Millisecond)
		// The error could be context.DeadlineExceeded or wrapped in another error
		if err == nil {
			t.Error("expected error when context times out")
		}
	})

	t.Run("returns when request expires", func(t *testing.T) {
		// Create a request that expires quickly
		request, _ := NewApprovalRequest("session-w4", "tc-w4", "test_tool", "{}", RiskLevelMedium, "agent-1")
		request.SetExpiration(time.Now().Add(100 * time.Millisecond))
		manager.repo.(*SQLiteRepository).CreateRequest(ctx, request)

		// Wait for expiration
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		result, err := manager.WaitForApproval(waitCtx, request.ID, 50*time.Millisecond)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Approved {
			t.Error("expected expired request to not be approved")
		}
		if result.Request.Status != RequestStatusExpired {
			t.Errorf("expected status 'expired', got %s", result.Request.Status)
		}
	})
}

// TestManager_ApproveExpiredRequest tests approval of expired requests.
func TestManager_ApproveExpiredRequest(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("cannot approve expired request", func(t *testing.T) {
		// Create a request that is already expired
		request, _ := NewApprovalRequest("session-ex1", "tc-ex1", "test_tool", "{}", RiskLevelMedium, "agent-1")
		request.SetExpiration(time.Now().Add(-1 * time.Hour)) // Already expired
		manager.repo.(*SQLiteRepository).CreateRequest(ctx, request)

		err := manager.Approve(ctx, request.ID, "admin", "Too late")
		if err != ErrRequestExpired {
			t.Errorf("expected ErrRequestExpired, got %v", err)
		}
	})

	t.Run("cannot reject expired request", func(t *testing.T) {
		// Create a request that is already expired
		request, _ := NewApprovalRequest("session-ex2", "tc-ex2", "test_tool", "{}", RiskLevelMedium, "agent-1")
		request.SetExpiration(time.Now().Add(-1 * time.Hour))
		manager.repo.(*SQLiteRepository).CreateRequest(ctx, request)

		err := manager.Reject(ctx, request.ID, "admin", "Too late")
		if err != ErrRequestExpired {
			t.Errorf("expected ErrRequestExpired, got %v", err)
		}
	})
}

// TestManager_ProjectSpecificPolicy tests project-specific policy resolution.
func TestManager_ProjectSpecificPolicy(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("uses project-specific policy over default", func(t *testing.T) {
		// Create a project-specific policy that auto-approves everything
		policy, _ := manager.CreatePolicy(ctx, "Project Policy", ApprovalModeAutoApprove, RiskLevelCritical)
		policy.SetProjectPath("/my/project")
		manager.UpdatePolicy(ctx, policy)

		// Request approval for a high-risk tool with project path
		_, autoApproved, err := manager.RequestApproval(ctx, "session-pp1", "tc-pp1", "run_shell_command", "{}", "agent-1", "/my/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !autoApproved {
			t.Error("expected project-specific policy to auto-approve")
		}
	})

	t.Run("falls back to default policy when no project match", func(t *testing.T) {
		// Request with different project path
		_, autoApproved, err := manager.RequestApproval(ctx, "session-pp2", "tc-pp2", "run_shell_command", "{}", "agent-1", "/other/project")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Default policy is risk-based with low threshold, so high-risk should not be auto-approved
		if autoApproved {
			t.Error("expected default policy to not auto-approve high-risk tool")
		}
	})
}

// TestManager_ListRequestsBySession tests listing requests with filters.
func TestManager_ListRequestsBySession(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)
	sessionID := "session-list"

	// Create several requests with different statuses
	req1, _, _ := manager.RequestApproval(ctx, sessionID, "tc-list-1", "read_file", "{}", "agent-1", "")
	req2, _, _ := manager.RequestApproval(ctx, sessionID, "tc-list-2", "run_shell_command", "{}", "agent-1", "")
	req3, _, _ := manager.RequestApproval(ctx, sessionID, "tc-list-3", "run_shell_command", "{}", "agent-1", "")
	manager.Approve(ctx, req2.ID, "admin", "")
	manager.Reject(ctx, req3.ID, "admin", "")

	t.Run("lists all requests for session", func(t *testing.T) {
		requests, err := manager.ListRequestsBySession(ctx, sessionID, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(requests) != 3 {
			t.Errorf("expected 3 requests, got %d", len(requests))
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		opts := &ListOptions{
			StatusFilter: []RequestStatus{RequestStatusApproved},
		}
		requests, err := manager.ListRequestsBySession(ctx, sessionID, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(requests) != 1 {
			t.Errorf("expected 1 approved request, got %d", len(requests))
		}
		if requests[0].ID != req2.ID {
			t.Error("expected the approved request")
		}
	})

	t.Run("limits results", func(t *testing.T) {
		opts := &ListOptions{
			Limit: 2,
		}
		requests, err := manager.ListRequestsBySession(ctx, sessionID, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(requests) != 2 {
			t.Errorf("expected 2 requests, got %d", len(requests))
		}
	})

	t.Run("respects auto-approved status", func(t *testing.T) {
		opts := &ListOptions{
			StatusFilter: []RequestStatus{RequestStatusAutoApproved},
		}
		requests, err := manager.ListRequestsBySession(ctx, sessionID, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(requests) != 1 {
			t.Errorf("expected 1 auto-approved request, got %d", len(requests))
		}
		if requests[0].ID != req1.ID {
			t.Error("expected the auto-approved read_file request")
		}
	})
}

// TestManager_GetRequestByToolCallID tests retrieval by tool call ID.
func TestManager_GetRequestByToolCallID(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("retrieves request by tool call ID", func(t *testing.T) {
		request, _, _ := manager.RequestApproval(ctx, "session-tc1", "unique-tool-call-id", "write_file", "{}", "agent-1", "")

		retrieved, err := manager.GetRequestByToolCallID(ctx, "unique-tool-call-id")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if retrieved.ID != request.ID {
			t.Errorf("expected request ID %s, got %s", request.ID, retrieved.ID)
		}
	})

	t.Run("returns error for non-existent tool call ID", func(t *testing.T) {
		_, err := manager.GetRequestByToolCallID(ctx, "non-existent-tc-id")
		if err != ErrApprovalRequestNotFound {
			t.Errorf("expected ErrApprovalRequestNotFound, got %v", err)
		}
	})
}

// TestManager_CountPendingRequests tests counting pending requests.
func TestManager_CountPendingRequests(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)
	sessionID := "session-count"

	t.Run("counts pending requests correctly", func(t *testing.T) {
		// Create some pending requests
		manager.RequestApproval(ctx, sessionID, "tc-cnt-1", "run_shell_command", "{}", "agent-1", "")
		manager.RequestApproval(ctx, sessionID, "tc-cnt-2", "run_shell_command", "{}", "agent-1", "")
		req3, _, _ := manager.RequestApproval(ctx, sessionID, "tc-cnt-3", "run_shell_command", "{}", "agent-1", "")

		// Approve one
		manager.Approve(ctx, req3.ID, "admin", "")

		count, err := manager.CountPendingRequests(ctx, sessionID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 2 {
			t.Errorf("expected 2 pending requests, got %d", count)
		}
	})

	t.Run("returns zero for session with no pending requests", func(t *testing.T) {
		count, err := manager.CountPendingRequests(ctx, "non-existent-session")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 pending requests, got %d", count)
		}
	})
}

// TestManager_ListPendingRequests tests listing pending requests.
func TestManager_ListPendingRequests(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)
	sessionID := "session-pending-list"

	t.Run("lists only pending requests", func(t *testing.T) {
		// Create requests with different statuses
		manager.RequestApproval(ctx, sessionID, "tc-pl-1", "run_shell_command", "{}", "agent-1", "")
		req2, _, _ := manager.RequestApproval(ctx, sessionID, "tc-pl-2", "run_shell_command", "{}", "agent-1", "")
		manager.RequestApproval(ctx, sessionID, "tc-pl-3", "run_shell_command", "{}", "agent-1", "")

		// Approve one
		manager.Approve(ctx, req2.ID, "admin", "")

		pending, err := manager.ListPendingRequests(ctx, sessionID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(pending) != 2 {
			t.Errorf("expected 2 pending requests, got %d", len(pending))
		}

		// Verify all returned requests are pending
		for _, req := range pending {
			if req.Status != RequestStatusPending {
				t.Errorf("expected pending status, got %s", req.Status)
			}
		}
	})
}

// TestManager_ListActivePolicies tests listing active policies.
func TestManager_ListActivePolicies(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("lists only active policies", func(t *testing.T) {
		// Create active and inactive policies
		policy1, _ := manager.CreatePolicy(ctx, "Active Policy 1", ApprovalModeRiskBased, RiskLevelLow)
		policy2, _ := manager.CreatePolicy(ctx, "Active Policy 2", ApprovalModeAlwaysAsk, RiskLevelLow)
		policy3, _ := manager.CreatePolicy(ctx, "Inactive Policy", ApprovalModeAutoApprove, RiskLevelLow)

		// Deactivate one
		policy3.Deactivate()
		manager.UpdatePolicy(ctx, policy3)

		policies, err := manager.ListActivePolicies(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Check that all returned policies are active
		for _, p := range policies {
			if !p.IsActive {
				t.Errorf("expected only active policies, got inactive policy: %s", p.Name)
			}
		}

		// Should include policy1 and policy2
		foundPolicy1 := false
		foundPolicy2 := false
		for _, p := range policies {
			if p.ID == policy1.ID {
				foundPolicy1 = true
			}
			if p.ID == policy2.ID {
				foundPolicy2 = true
			}
		}
		if !foundPolicy1 {
			t.Error("expected to find Active Policy 1")
		}
		if !foundPolicy2 {
			t.Error("expected to find Active Policy 2")
		}
	})
}

// TestManager_DeletePolicy tests policy deletion.
func TestManager_DeletePolicy(t *testing.T) {
	ctx := context.Background()
	manager := setupTestManager(t)

	t.Run("deletes existing policy", func(t *testing.T) {
		policy, _ := manager.CreatePolicy(ctx, "Delete Me", ApprovalModeRiskBased, RiskLevelLow)

		err := manager.DeletePolicy(ctx, policy.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		_, err = manager.GetPolicy(ctx, policy.ID)
		if err != ErrApprovalPolicyNotFound {
			t.Errorf("expected ErrApprovalPolicyNotFound, got %v", err)
		}
	})

	t.Run("returns error for non-existent policy", func(t *testing.T) {
		err := manager.DeletePolicy(ctx, "non-existent-policy-id")
		if err != ErrApprovalPolicyNotFound {
			t.Errorf("expected ErrApprovalPolicyNotFound, got %v", err)
		}
	})
}

// TestManager_Close tests manager resource cleanup.
func TestManager_Close(t *testing.T) {
	t.Run("closes without error", func(t *testing.T) {
		manager := setupTestManager(t)
		err := manager.Close()
		if err != nil {
			t.Errorf("unexpected error on close: %v", err)
		}
	})
}

// TestManager_ConcurrentSessionTrust tests concurrent access to session trust.
func TestManager_ConcurrentSessionTrust(t *testing.T) {
	manager := setupTestManager(t)

	t.Run("handles concurrent session trust operations", func(t *testing.T) {
		const numGoroutines = 100
		done := make(chan bool, numGoroutines*2)

		// Concurrently establish and check trust
		for i := 0; i < numGoroutines; i++ {
			sessionID := fmt.Sprintf("session-%d", i)

			go func(id string) {
				manager.EstablishSessionTrust(id)
				done <- true
			}(sessionID)

			go func(id string) {
				manager.HasSessionTrust(id)
				done <- true
			}(sessionID)
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines*2; i++ {
			<-done
		}
	})
}

// TestManager_ConcurrentRiskLevel tests concurrent access to risk level mappings.
func TestManager_ConcurrentRiskLevel(t *testing.T) {
	manager := setupTestManager(t)

	t.Run("handles concurrent risk level operations", func(t *testing.T) {
		const numGoroutines = 100
		done := make(chan bool, numGoroutines*2)

		// Concurrently set and get risk levels
		for i := 0; i < numGoroutines; i++ {
			toolName := fmt.Sprintf("tool-%d", i)

			go func(name string) {
				manager.SetRiskLevel(name, RiskLevelHigh)
				done <- true
			}(toolName)

			go func(name string) {
				manager.GetRiskLevel(name)
				done <- true
			}(toolName)
		}

		// Wait for all goroutines
		for i := 0; i < numGoroutines*2; i++ {
			<-done
		}
	})
}

// TestSQLiteRepository_GetNonExistentRequest tests error handling for missing requests.
func TestSQLiteRepository_GetNonExistentRequest(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	t.Run("returns error for non-existent request ID", func(t *testing.T) {
		_, err := repo.GetRequest(ctx, "non-existent-id")
		if err != ErrApprovalRequestNotFound {
			t.Errorf("expected ErrApprovalRequestNotFound, got %v", err)
		}
	})

	t.Run("returns error for non-existent tool call ID", func(t *testing.T) {
		_, err := repo.GetRequestByToolCallID(ctx, "non-existent-tc-id")
		if err != ErrApprovalRequestNotFound {
			t.Errorf("expected ErrApprovalRequestNotFound, got %v", err)
		}
	})
}

// TestSQLiteRepository_UpdateNonExistentRequest tests error handling for updating missing requests.
func TestSQLiteRepository_UpdateNonExistentRequest(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	t.Run("returns error for non-existent request", func(t *testing.T) {
		request, _ := NewApprovalRequest("session-1", "tc-1", "tool", "{}", RiskLevelLow, "user-1")
		request.ID = "non-existent-id"

		err := repo.UpdateRequest(ctx, request)
		if err != ErrApprovalRequestNotFound {
			t.Errorf("expected ErrApprovalRequestNotFound, got %v", err)
		}
	})
}

// TestSQLiteRepository_DuplicatePolicyName tests unique constraint on policy names.
func TestSQLiteRepository_DuplicatePolicyName(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	t.Run("returns error for duplicate policy name", func(t *testing.T) {
		policy1, _ := NewApprovalPolicy("Unique Name", ApprovalModeRiskBased, RiskLevelLow)
		err := repo.CreatePolicy(ctx, policy1)
		if err != nil {
			t.Fatalf("failed to create first policy: %v", err)
		}

		policy2, _ := NewApprovalPolicy("Unique Name", ApprovalModeAlwaysAsk, RiskLevelMedium)
		err = repo.CreatePolicy(ctx, policy2)
		if err == nil {
			t.Error("expected error for duplicate policy name")
		}
	})
}

// TestSQLiteRepository_ListRequestsBySessionWithOffset tests pagination.
func TestSQLiteRepository_ListRequestsBySessionWithOffset(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	sessionID := "session-offset"

	// Create several requests
	for i := 0; i < 5; i++ {
		req, _ := NewApprovalRequest(sessionID, fmt.Sprintf("tc-off-%d", i), "tool", "{}", RiskLevelLow, "user-1")
		repo.CreateRequest(ctx, req)
	}

	t.Run("respects offset and limit", func(t *testing.T) {
		opts := &ListOptions{
			Limit:     2,
			Offset:    2,
			OrderDesc: false,
		}
		requests, err := repo.ListRequestsBySession(ctx, sessionID, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(requests) != 2 {
			t.Errorf("expected 2 requests, got %d", len(requests))
		}
	})
}

// TestSQLiteRepository_ListPoliciesWithOptions tests policy listing with options.
func TestSQLiteRepository_ListPoliciesWithOptions(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	// Create several policies including inactive ones
	for i := 0; i < 5; i++ {
		policy, _ := NewApprovalPolicy(fmt.Sprintf("Policy %d", i), ApprovalModeRiskBased, RiskLevelLow)
		if i%2 == 0 {
			policy.Deactivate()
		}
		repo.CreatePolicy(ctx, policy)
	}

	t.Run("includes inactive when requested", func(t *testing.T) {
		opts := &ListOptions{
			IncludeInactive: true,
		}
		policies, err := repo.ListPolicies(ctx, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		hasInactive := false
		for _, p := range policies {
			if !p.IsActive {
				hasInactive = true
				break
			}
		}
		if !hasInactive {
			t.Error("expected to include inactive policies")
		}
	})

	t.Run("excludes inactive by default", func(t *testing.T) {
		policies, err := repo.ListPolicies(ctx, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		for _, p := range policies {
			if !p.IsActive {
				t.Errorf("expected only active policies, got inactive: %s", p.Name)
			}
		}
	})
}

// TestManager_isInToolList tests the isInToolList helper method.
func TestManager_isInToolList(t *testing.T) {
	manager := setupTestManager(t)

	t.Run("returns false for empty list", func(t *testing.T) {
		if manager.isInToolList("", "any_tool") {
			t.Error("expected false for empty list")
		}
	})

	t.Run("returns false for invalid JSON", func(t *testing.T) {
		if manager.isInToolList("not valid json", "any_tool") {
			t.Error("expected false for invalid JSON")
		}
	})

	t.Run("returns true when tool is in list", func(t *testing.T) {
		list := `["tool1", "tool2", "tool3"]`
		if !manager.isInToolList(list, "tool2") {
			t.Error("expected true when tool is in list")
		}
	})

	t.Run("returns false when tool is not in list", func(t *testing.T) {
		list := `["tool1", "tool2", "tool3"]`
		if manager.isInToolList(list, "tool4") {
			t.Error("expected false when tool is not in list")
		}
	})
}

// TestDefaultListOptions tests the DefaultListOptions function.
func TestDefaultListOptions(t *testing.T) {
	t.Run("returns expected defaults", func(t *testing.T) {
		opts := DefaultListOptions()

		if opts.Limit != 100 {
			t.Errorf("expected limit 100, got %d", opts.Limit)
		}
		if opts.Offset != 0 {
			t.Errorf("expected offset 0, got %d", opts.Offset)
		}
		if opts.OrderBy != "created_at" {
			t.Errorf("expected order by 'created_at', got %s", opts.OrderBy)
		}
		if !opts.OrderDesc {
			t.Error("expected order desc to be true")
		}
	})
}
