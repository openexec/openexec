// Package approval provides tool approval workflow management for the OpenExec orchestrator.
package approval

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// Manager orchestrates the approval workflow for tool operations.
// It combines policy evaluation with request lifecycle management.
type Manager struct {
	repo                      Repository
	riskMappings              map[string]RiskLevel
	sessionTrust              map[string]bool // sessionID -> has established trust
	orchestratorRiskEscalator *OrchestratorRiskEscalator
	mu                        sync.RWMutex
}

// NewManager creates a new approval manager with the given repository.
func NewManager(repo Repository) *Manager {
	m := &Manager{
		repo:         repo,
		riskMappings: make(map[string]RiskLevel),
		sessionTrust: make(map[string]bool),
	}
	// Initialize default risk mappings
	for _, mapping := range DefaultToolRiskMappings() {
		m.riskMappings[mapping.ToolName] = mapping.RiskLevel
	}
	return m
}

// NewManagerWithOrchestratorCheck creates a new approval manager with orchestrator
// edit detection enabled. When file modification tools target orchestrator files,
// the risk level is automatically escalated to critical.
func NewManagerWithOrchestratorCheck(repo Repository, checker OrchestratorEditChecker) *Manager {
	m := NewManager(repo)
	if checker != nil {
		m.orchestratorRiskEscalator = NewOrchestratorRiskEscalator(checker)
	}
	return m
}

// SetOrchestratorEditChecker sets the orchestrator edit checker for risk escalation.
// This allows enabling orchestrator protection after manager creation.
func (m *Manager) SetOrchestratorEditChecker(checker OrchestratorEditChecker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if checker != nil {
		m.orchestratorRiskEscalator = NewOrchestratorRiskEscalator(checker)
	} else {
		m.orchestratorRiskEscalator = nil
	}
}

// GetRiskLevel returns the risk level for a tool, defaulting to medium if unknown.
func (m *Manager) GetRiskLevel(toolName string) RiskLevel {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level, ok := m.riskMappings[toolName]; ok {
		return level
	}
	return RiskLevelMedium // Default to medium risk for unknown tools
}

// GetEffectiveRiskLevel returns the risk level for a tool after considering
// orchestrator edit escalation. Use this method when you need to know the
// actual risk level that will be applied for a specific tool call.
func (m *Manager) GetEffectiveRiskLevel(toolName, toolInput string) RiskLevel {
	baseLevel := m.GetRiskLevel(toolName)

	m.mu.RLock()
	escalator := m.orchestratorRiskEscalator
	m.mu.RUnlock()

	if escalator != nil {
		level, _ := escalator.EscalateRiskLevel(toolName, toolInput, baseLevel)
		return level
	}
	return baseLevel
}

// IsOrchestratorEdit checks if a tool call is modifying orchestrator files.
// Returns true if the tool call targets orchestrator files.
func (m *Manager) IsOrchestratorEdit(toolName, toolInput string) bool {
	m.mu.RLock()
	escalator := m.orchestratorRiskEscalator
	m.mu.RUnlock()

	if escalator != nil {
		return escalator.IsOrchestratorEdit(toolName, toolInput)
	}
	return false
}

// SetRiskLevel sets or updates the risk level for a tool.
func (m *Manager) SetRiskLevel(toolName string, level RiskLevel) error {
	if !level.IsValid() {
		return fmt.Errorf("invalid risk level: %s", level)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.riskMappings[toolName] = level
	return nil
}

// RequestApproval creates a new approval request and determines if it should be auto-approved.
// Returns the request and a boolean indicating if execution can proceed immediately.
func (m *Manager) RequestApproval(ctx context.Context, sessionID, toolCallID, toolName, toolInput, requestedBy string, projectPath string) (*ApprovalRequest, bool, error) {
	riskLevel := m.GetRiskLevel(toolName)

	// Check for orchestrator file edits and escalate risk level if necessary
	escalated := false
	if m.orchestratorRiskEscalator != nil {
		riskLevel, escalated = m.orchestratorRiskEscalator.EscalateRiskLevel(toolName, toolInput, riskLevel)
	}
	_ = escalated // Used for potential future logging/audit

	// Create the approval request
	request, err := NewApprovalRequest(sessionID, toolCallID, toolName, toolInput, riskLevel, requestedBy)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create approval request: %w", err)
	}

	// Get applicable policy
	policy, err := m.getApplicablePolicy(ctx, projectPath, toolName)
	if err != nil && !errors.Is(err, ErrApprovalPolicyNotFound) {
		return nil, false, fmt.Errorf("failed to get applicable policy: %w", err)
	}

	// Apply policy if found
	if policy != nil {
		request.SetPolicy(policy.ID)
		request.SetExpiration(time.Now().Add(policy.GetTimeout()))

		// Check for auto-approval
		shouldAutoApprove := m.shouldAutoApprove(ctx, policy, sessionID, toolName, riskLevel)
		if shouldAutoApprove {
			if err := request.AutoApprove(); err != nil {
				return nil, false, fmt.Errorf("failed to auto-approve request: %w", err)
			}

			// Store the request
			if err := m.repo.CreateRequest(ctx, request); err != nil {
				return nil, false, fmt.Errorf("failed to create request: %w", err)
			}

			// Record the auto-approval decision
			decision, err := NewApprovalDecision(request.ID, RequestStatusAutoApproved, "policy")
			if err != nil {
				return nil, false, fmt.Errorf("failed to create auto-approval decision: %w", err)
			}
			decision.SetPolicyApplied(policy.ID)
			decision.SetReason(fmt.Sprintf("Auto-approved by policy '%s' (mode: %s, risk: %s <= %s)",
				policy.Name, policy.Mode, riskLevel, policy.AutoApproveRiskLevel))

			if err := m.repo.CreateDecision(ctx, decision); err != nil {
				return nil, false, fmt.Errorf("failed to record auto-approval decision: %w", err)
			}

			return request, true, nil
		}
	}

	// Store the pending request
	if err := m.repo.CreateRequest(ctx, request); err != nil {
		return nil, false, fmt.Errorf("failed to create request: %w", err)
	}

	return request, false, nil
}

// shouldAutoApprove determines if a tool call should be auto-approved based on policy and session state.
func (m *Manager) shouldAutoApprove(ctx context.Context, policy *ApprovalPolicy, sessionID, toolName string, riskLevel RiskLevel) bool {
	if policy == nil {
		return false
	}

	// Check if tool is in always-require-approval list
	if m.isInToolList(policy.AlwaysRequireApprovalTools, toolName) {
		return false
	}

	// Check if tool is in trusted list
	if m.isInToolList(policy.AutoApproveTrustedTools, toolName) {
		return true
	}

	// Handle different approval modes
	switch policy.Mode {
	case ApprovalModeAutoApprove:
		return true
	case ApprovalModeAlwaysAsk:
		return false
	case ApprovalModeRiskBased:
		return policy.ShouldAutoApprove(toolName, riskLevel)
	case ApprovalModeSessionTrust:
		m.mu.RLock()
		hasTrust := m.sessionTrust[sessionID]
		m.mu.RUnlock()
		return hasTrust
	default:
		return false
	}
}

// isInToolList checks if a tool name is in a JSON array of tool names.
func (m *Manager) isInToolList(jsonList string, toolName string) bool {
	if jsonList == "" {
		return false
	}

	var tools []string
	if err := json.Unmarshal([]byte(jsonList), &tools); err != nil {
		return false
	}

	for _, t := range tools {
		if t == toolName {
			return true
		}
	}
	return false
}

// getApplicablePolicy finds the most applicable policy for a project and tool.
func (m *Manager) getApplicablePolicy(ctx context.Context, projectPath, toolName string) (*ApprovalPolicy, error) {
	// First, try to get a project-specific policy
	if projectPath != "" {
		policy, err := m.repo.GetPolicyForProject(ctx, projectPath)
		if err == nil && policy != nil {
			return policy, nil
		}
	}

	// Fall back to default policy
	return m.repo.GetDefaultPolicy(ctx)
}

// Approve manually approves a pending request.
func (m *Manager) Approve(ctx context.Context, requestID, decidedBy, reason string) error {
	request, err := m.repo.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}

	if request.IsExpired() {
		return ErrRequestExpired
	}

	if err := request.Approve(); err != nil {
		return err
	}

	if err := m.repo.UpdateRequest(ctx, request); err != nil {
		return fmt.Errorf("failed to update request: %w", err)
	}

	// Record the decision
	decision, err := NewApprovalDecision(requestID, RequestStatusApproved, decidedBy)
	if err != nil {
		return fmt.Errorf("failed to create decision: %w", err)
	}
	decision.SetReason(reason)

	if err := m.repo.CreateDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to record decision: %w", err)
	}

	return nil
}

// Reject manually rejects a pending request.
func (m *Manager) Reject(ctx context.Context, requestID, decidedBy, reason string) error {
	request, err := m.repo.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}

	if request.IsExpired() {
		return ErrRequestExpired
	}

	if err := request.Reject(); err != nil {
		return err
	}

	if err := m.repo.UpdateRequest(ctx, request); err != nil {
		return fmt.Errorf("failed to update request: %w", err)
	}

	// Record the decision
	decision, err := NewApprovalDecision(requestID, RequestStatusRejected, decidedBy)
	if err != nil {
		return fmt.Errorf("failed to create decision: %w", err)
	}
	decision.SetReason(reason)

	if err := m.repo.CreateDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to record decision: %w", err)
	}

	return nil
}

// Cancel cancels a pending request (e.g., when the session ends).
func (m *Manager) Cancel(ctx context.Context, requestID, reason string) error {
	request, err := m.repo.GetRequest(ctx, requestID)
	if err != nil {
		return fmt.Errorf("failed to get request: %w", err)
	}

	if err := request.Cancel(); err != nil {
		return err
	}

	if err := m.repo.UpdateRequest(ctx, request); err != nil {
		return fmt.Errorf("failed to update request: %w", err)
	}

	// Record the cancellation
	decision, err := NewApprovalDecision(requestID, RequestStatusCancelled, "system")
	if err != nil {
		return fmt.Errorf("failed to create decision: %w", err)
	}
	decision.SetReason(reason)

	if err := m.repo.CreateDecision(ctx, decision); err != nil {
		return fmt.Errorf("failed to record decision: %w", err)
	}

	return nil
}

// ExpireRequests marks all expired pending requests as expired.
func (m *Manager) ExpireRequests(ctx context.Context) (int, error) {
	requests, err := m.repo.ListExpiredRequests(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to list expired requests: %w", err)
	}

	count := 0
	for _, request := range requests {
		if !request.IsPending() {
			continue
		}

		if err := request.Expire(); err != nil {
			continue
		}

		if err := m.repo.UpdateRequest(ctx, request); err != nil {
			continue
		}

		// Record the expiration
		decision, err := NewApprovalDecision(request.ID, RequestStatusExpired, "system")
		if err != nil {
			continue
		}
		decision.SetReason("Request expired without a decision")

		if err := m.repo.CreateDecision(ctx, decision); err != nil {
			continue
		}

		count++
	}

	return count, nil
}

// EstablishSessionTrust marks a session as trusted for session_trust mode.
func (m *Manager) EstablishSessionTrust(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessionTrust[sessionID] = true
}

// RevokeSessionTrust removes trust from a session.
func (m *Manager) RevokeSessionTrust(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessionTrust, sessionID)
}

// HasSessionTrust checks if a session has established trust.
func (m *Manager) HasSessionTrust(sessionID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessionTrust[sessionID]
}

// GetRequest retrieves an approval request by ID.
func (m *Manager) GetRequest(ctx context.Context, id string) (*ApprovalRequest, error) {
	return m.repo.GetRequest(ctx, id)
}

// GetRequestByToolCallID retrieves an approval request by tool call ID.
func (m *Manager) GetRequestByToolCallID(ctx context.Context, toolCallID string) (*ApprovalRequest, error) {
	return m.repo.GetRequestByToolCallID(ctx, toolCallID)
}

// ListPendingRequests returns all pending requests for a session.
func (m *Manager) ListPendingRequests(ctx context.Context, sessionID string) ([]*ApprovalRequest, error) {
	return m.repo.ListPendingRequests(ctx, sessionID)
}

// ListRequestsBySession returns all requests for a session.
func (m *Manager) ListRequestsBySession(ctx context.Context, sessionID string, opts *ListOptions) ([]*ApprovalRequest, error) {
	return m.repo.ListRequestsBySession(ctx, sessionID, opts)
}

// CountPendingRequests returns the count of pending requests for a session.
func (m *Manager) CountPendingRequests(ctx context.Context, sessionID string) (int, error) {
	return m.repo.CountPendingRequests(ctx, sessionID)
}

// GetDecision retrieves the decision for a request.
func (m *Manager) GetDecision(ctx context.Context, requestID string) (*ApprovalDecision, error) {
	return m.repo.GetDecisionByRequestID(ctx, requestID)
}

// CreatePolicy creates a new approval policy.
func (m *Manager) CreatePolicy(ctx context.Context, name string, mode ApprovalMode, autoApproveRiskLevel RiskLevel) (*ApprovalPolicy, error) {
	policy, err := NewApprovalPolicy(name, mode, autoApproveRiskLevel)
	if err != nil {
		return nil, err
	}

	if err := m.repo.CreatePolicy(ctx, policy); err != nil {
		return nil, fmt.Errorf("failed to create policy: %w", err)
	}

	return policy, nil
}

// GetPolicy retrieves a policy by ID.
func (m *Manager) GetPolicy(ctx context.Context, id string) (*ApprovalPolicy, error) {
	return m.repo.GetPolicy(ctx, id)
}

// GetPolicyByName retrieves a policy by name.
func (m *Manager) GetPolicyByName(ctx context.Context, name string) (*ApprovalPolicy, error) {
	return m.repo.GetPolicyByName(ctx, name)
}

// UpdatePolicy updates an existing policy.
func (m *Manager) UpdatePolicy(ctx context.Context, policy *ApprovalPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	return m.repo.UpdatePolicy(ctx, policy)
}

// DeletePolicy removes a policy.
func (m *Manager) DeletePolicy(ctx context.Context, id string) error {
	return m.repo.DeletePolicy(ctx, id)
}

// ListPolicies returns all policies.
func (m *Manager) ListPolicies(ctx context.Context, opts *ListOptions) ([]*ApprovalPolicy, error) {
	return m.repo.ListPolicies(ctx, opts)
}

// ListActivePolicies returns all active policies.
func (m *Manager) ListActivePolicies(ctx context.Context) ([]*ApprovalPolicy, error) {
	return m.repo.ListActivePolicies(ctx)
}

// GetDefaultPolicy retrieves the default policy.
func (m *Manager) GetDefaultPolicy(ctx context.Context) (*ApprovalPolicy, error) {
	return m.repo.GetDefaultPolicy(ctx)
}

// SetDefaultPolicy sets a policy as the default.
func (m *Manager) SetDefaultPolicy(ctx context.Context, policyID string) error {
	// First, unset any existing default
	policies, err := m.repo.ListPolicies(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to list policies: %w", err)
	}

	for _, p := range policies {
		if p.IsDefault {
			p.IsDefault = false
			if err := m.repo.UpdatePolicy(ctx, p); err != nil {
				return fmt.Errorf("failed to unset default policy: %w", err)
			}
		}
	}

	// Set the new default
	policy, err := m.repo.GetPolicy(ctx, policyID)
	if err != nil {
		return fmt.Errorf("failed to get policy: %w", err)
	}

	policy.IsDefault = true
	policy.UpdatedAt = time.Now().UTC()

	return m.repo.UpdatePolicy(ctx, policy)
}

// ApprovalResult represents the result of checking approval status.
type ApprovalResult struct {
	Request  *ApprovalRequest
	Decision *ApprovalDecision
	Approved bool
	Pending  bool
	Reason   string
}

// CheckApproval checks the current approval status for a tool call.
func (m *Manager) CheckApproval(ctx context.Context, toolCallID string) (*ApprovalResult, error) {
	request, err := m.repo.GetRequestByToolCallID(ctx, toolCallID)
	if err != nil {
		if errors.Is(err, ErrApprovalRequestNotFound) {
			return nil, ErrApprovalRequestNotFound
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}

	result := &ApprovalResult{
		Request: request,
	}

	// Check if expired
	if request.IsExpired() && request.IsPending() {
		// Mark as expired
		if err := request.Expire(); err == nil {
			m.repo.UpdateRequest(ctx, request)
		}
	}

	if request.IsPending() {
		result.Pending = true
		result.Reason = "Awaiting approval"
		return result, nil
	}

	// Get decision
	decision, err := m.repo.GetDecisionByRequestID(ctx, request.ID)
	if err == nil {
		result.Decision = decision
		result.Reason = decision.Reason
	}

	result.Approved = request.IsApproved()
	if !result.Approved {
		if result.Reason == "" {
			result.Reason = fmt.Sprintf("Request %s", request.Status)
		}
	}

	return result, nil
}

// WaitForApproval waits for an approval decision with timeout.
// Returns the approval result or an error if timeout/cancelled.
func (m *Manager) WaitForApproval(ctx context.Context, requestID string, checkInterval time.Duration) (*ApprovalResult, error) {
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			request, err := m.repo.GetRequest(ctx, requestID)
			if err != nil {
				return nil, err
			}

			// Check expiration
			if request.IsExpired() && request.IsPending() {
				if err := request.Expire(); err == nil {
					m.repo.UpdateRequest(ctx, request)

					decision, _ := NewApprovalDecision(requestID, RequestStatusExpired, "system")
					decision.SetReason("Request expired while waiting")
					m.repo.CreateDecision(ctx, decision)
				}

				return &ApprovalResult{
					Request:  request,
					Approved: false,
					Reason:   "Request expired",
				}, nil
			}

			if !request.IsPending() {
				decision, _ := m.repo.GetDecisionByRequestID(ctx, request.ID)
				return &ApprovalResult{
					Request:  request,
					Decision: decision,
					Approved: request.IsApproved(),
					Reason:   m.getDecisionReason(decision, request),
				}, nil
			}
		}
	}
}

func (m *Manager) getDecisionReason(decision *ApprovalDecision, request *ApprovalRequest) string {
	if decision != nil && decision.Reason != "" {
		return decision.Reason
	}
	return fmt.Sprintf("Request %s", request.Status)
}

// GetApprovalStats returns statistics about approvals for a session.
func (m *Manager) GetApprovalStats(ctx context.Context, sessionID string) (map[RequestStatus]int, error) {
	return m.repo.CountRequestsByStatus(ctx, sessionID)
}

// Close releases any resources held by the manager.
func (m *Manager) Close() error {
	return m.repo.Close()
}

// SQLiteRepository implements Repository using SQLite.
type SQLiteRepository struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteRepository creates a new SQLite-based approval repository.
func NewSQLiteRepository(db *sql.DB) (*SQLiteRepository, error) {
	if db == nil {
		return nil, errors.New("database connection is required")
	}

	repo := &SQLiteRepository{db: db}

	// Initialize schema
	if err := repo.initSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Seed default policy
	if err := repo.seedDefaultPolicy(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to seed default policy: %w", err)
	}

	return repo, nil
}

func (r *SQLiteRepository) initSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, Schema)
	return err
}

func (r *SQLiteRepository) seedDefaultPolicy(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, SeedDefaultPolicy)
	return err
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

// ApprovalRequest operations

func (r *SQLiteRepository) CreateRequest(ctx context.Context, request *ApprovalRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		INSERT INTO approval_requests (id, session_id, tool_call_id, tool_name, tool_input, risk_level, status, policy_id, requested_by, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		request.ID,
		request.SessionID,
		request.ToolCallID,
		request.ToolName,
		request.ToolInput,
		request.RiskLevel.String(),
		request.Status.String(),
		nullStringVal(request.PolicyID),
		request.RequestedBy,
		nullTimeVal(request.ExpiresAt),
		request.CreatedAt.UTC().Format(time.RFC3339),
		request.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create approval request: %w", err)
	}

	return nil
}

func (r *SQLiteRepository) GetRequest(ctx context.Context, id string) (*ApprovalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, tool_call_id, tool_name, tool_input, risk_level, status, policy_id, requested_by, expires_at, created_at, updated_at
		FROM approval_requests WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanApprovalRequest(row)
}

func (r *SQLiteRepository) GetRequestByToolCallID(ctx context.Context, toolCallID string) (*ApprovalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, tool_call_id, tool_name, tool_input, risk_level, status, policy_id, requested_by, expires_at, created_at, updated_at
		FROM approval_requests WHERE tool_call_id = ?
	`

	row := r.db.QueryRowContext(ctx, query, toolCallID)
	return scanApprovalRequest(row)
}

func (r *SQLiteRepository) UpdateRequest(ctx context.Context, request *ApprovalRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		UPDATE approval_requests
		SET status = ?, policy_id = ?, expires_at = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		request.Status.String(),
		nullStringVal(request.PolicyID),
		nullTimeVal(request.ExpiresAt),
		request.UpdatedAt.UTC().Format(time.RFC3339),
		request.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update approval request: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ErrApprovalRequestNotFound
	}

	return nil
}

func (r *SQLiteRepository) ListPendingRequests(ctx context.Context, sessionID string) ([]*ApprovalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, tool_call_id, tool_name, tool_input, risk_level, status, policy_id, requested_by, expires_at, created_at, updated_at
		FROM approval_requests
		WHERE session_id = ? AND status = 'pending'
		ORDER BY created_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending requests: %w", err)
	}
	defer rows.Close()

	return scanApprovalRequests(rows)
}

func (r *SQLiteRepository) ListRequestsBySession(ctx context.Context, sessionID string, opts *ListOptions) ([]*ApprovalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, session_id, tool_call_id, tool_name, tool_input, risk_level, status, policy_id, requested_by, expires_at, created_at, updated_at
		FROM approval_requests
		WHERE session_id = ?
	`
	args := []interface{}{sessionID}

	if opts != nil && len(opts.StatusFilter) > 0 {
		placeholders := ""
		for i, status := range opts.StatusFilter {
			if i > 0 {
				placeholders += ", "
			}
			placeholders += "?"
			args = append(args, status.String())
		}
		query += fmt.Sprintf(" AND status IN (%s)", placeholders)
	}

	orderBy := "created_at"
	if opts != nil && opts.OrderBy != "" {
		orderBy = opts.OrderBy
	}
	orderDir := "DESC"
	if opts != nil && !opts.OrderDesc {
		orderDir = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s", orderBy, orderDir)

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
		if opts.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, opts.Offset)
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list requests by session: %w", err)
	}
	defer rows.Close()

	return scanApprovalRequests(rows)
}

func (r *SQLiteRepository) ListExpiredRequests(ctx context.Context) ([]*ApprovalRequest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	now := time.Now().UTC().Format(time.RFC3339)
	query := `
		SELECT id, session_id, tool_call_id, tool_name, tool_input, risk_level, status, policy_id, requested_by, expires_at, created_at, updated_at
		FROM approval_requests
		WHERE status = 'pending' AND expires_at IS NOT NULL AND expires_at < ?
		ORDER BY expires_at ASC
	`

	rows, err := r.db.QueryContext(ctx, query, now)
	if err != nil {
		return nil, fmt.Errorf("failed to list expired requests: %w", err)
	}
	defer rows.Close()

	return scanApprovalRequests(rows)
}

// ApprovalPolicy operations

func (r *SQLiteRepository) CreatePolicy(ctx context.Context, policy *ApprovalPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		INSERT INTO approval_policies (id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		policy.ID,
		policy.Name,
		policy.Description,
		nullStringVal(policy.ProjectPath),
		policy.Mode.String(),
		policy.AutoApproveRiskLevel.String(),
		nullString(policy.AutoApproveTrustedTools),
		nullString(policy.AlwaysRequireApprovalTools),
		policy.TimeoutSeconds,
		boolToInt(policy.IsDefault),
		policy.Priority,
		boolToInt(policy.IsActive),
		policy.CreatedAt.UTC().Format(time.RFC3339),
		policy.UpdatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("policy with name '%s' already exists", policy.Name)
		}
		return fmt.Errorf("failed to create approval policy: %w", err)
	}

	return nil
}

func (r *SQLiteRepository) GetPolicy(ctx context.Context, id string) (*ApprovalPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at
		FROM approval_policies WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanApprovalPolicy(row)
}

func (r *SQLiteRepository) GetPolicyByName(ctx context.Context, name string) (*ApprovalPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at
		FROM approval_policies WHERE name = ?
	`

	row := r.db.QueryRowContext(ctx, query, name)
	return scanApprovalPolicy(row)
}

func (r *SQLiteRepository) UpdatePolicy(ctx context.Context, policy *ApprovalPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		UPDATE approval_policies
		SET name = ?, description = ?, project_path = ?, mode = ?, auto_approve_risk_level = ?,
			auto_approve_trusted_tools = ?, always_require_approval_tools = ?, timeout_seconds = ?,
			is_default = ?, priority = ?, is_active = ?, updated_at = ?
		WHERE id = ?
	`

	result, err := r.db.ExecContext(ctx, query,
		policy.Name,
		policy.Description,
		nullStringVal(policy.ProjectPath),
		policy.Mode.String(),
		policy.AutoApproveRiskLevel.String(),
		nullString(policy.AutoApproveTrustedTools),
		nullString(policy.AlwaysRequireApprovalTools),
		policy.TimeoutSeconds,
		boolToInt(policy.IsDefault),
		policy.Priority,
		boolToInt(policy.IsActive),
		policy.UpdatedAt.UTC().Format(time.RFC3339),
		policy.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update approval policy: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ErrApprovalPolicyNotFound
	}

	return nil
}

func (r *SQLiteRepository) DeletePolicy(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `DELETE FROM approval_policies WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete approval policy: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ErrApprovalPolicyNotFound
	}

	return nil
}

func (r *SQLiteRepository) ListPolicies(ctx context.Context, opts *ListOptions) ([]*ApprovalPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at
		FROM approval_policies
	`
	args := []interface{}{}

	if opts == nil || !opts.IncludeInactive {
		query += " WHERE is_active = 1"
	}

	query += " ORDER BY priority ASC, created_at DESC"

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
		if opts.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, opts.Offset)
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list policies: %w", err)
	}
	defer rows.Close()

	return scanApprovalPolicies(rows)
}

func (r *SQLiteRepository) ListActivePolicies(ctx context.Context) ([]*ApprovalPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at
		FROM approval_policies
		WHERE is_active = 1
		ORDER BY priority ASC
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list active policies: %w", err)
	}
	defer rows.Close()

	return scanApprovalPolicies(rows)
}

func (r *SQLiteRepository) GetDefaultPolicy(ctx context.Context) (*ApprovalPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at
		FROM approval_policies
		WHERE is_default = 1 AND is_active = 1
		LIMIT 1
	`

	row := r.db.QueryRowContext(ctx, query)
	return scanApprovalPolicy(row)
}

func (r *SQLiteRepository) GetPolicyForProject(ctx context.Context, projectPath string) (*ApprovalPolicy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// First try exact match, then prefix match, then default
	query := `
		SELECT id, name, description, project_path, mode, auto_approve_risk_level, auto_approve_trusted_tools, always_require_approval_tools, timeout_seconds, is_default, priority, is_active, created_at, updated_at
		FROM approval_policies
		WHERE is_active = 1 AND (
			project_path = ? OR
			? LIKE project_path || '%' OR
			is_default = 1
		)
		ORDER BY
			CASE
				WHEN project_path = ? THEN 0
				WHEN ? LIKE project_path || '%' THEN 1
				WHEN is_default = 1 THEN 2
				ELSE 3
			END,
			priority ASC
		LIMIT 1
	`

	row := r.db.QueryRowContext(ctx, query, projectPath, projectPath, projectPath, projectPath)
	return scanApprovalPolicy(row)
}

// ApprovalDecision operations

func (r *SQLiteRepository) CreateDecision(ctx context.Context, decision *ApprovalDecision) error {
	if err := decision.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	query := `
		INSERT INTO approval_decisions (id, request_id, decision, decided_by, reason, policy_applied, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := r.db.ExecContext(ctx, query,
		decision.ID,
		decision.RequestID,
		decision.Decision.String(),
		decision.DecidedBy,
		nullString(decision.Reason),
		nullStringVal(decision.PolicyApplied),
		decision.CreatedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("failed to create approval decision: %w", err)
	}

	return nil
}

func (r *SQLiteRepository) GetDecision(ctx context.Context, id string) (*ApprovalDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, request_id, decision, decided_by, reason, policy_applied, created_at
		FROM approval_decisions WHERE id = ?
	`

	row := r.db.QueryRowContext(ctx, query, id)
	return scanApprovalDecision(row)
}

func (r *SQLiteRepository) GetDecisionByRequestID(ctx context.Context, requestID string) (*ApprovalDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT id, request_id, decision, decided_by, reason, policy_applied, created_at
		FROM approval_decisions WHERE request_id = ?
	`

	row := r.db.QueryRowContext(ctx, query, requestID)
	return scanApprovalDecision(row)
}

func (r *SQLiteRepository) ListDecisionsBySession(ctx context.Context, sessionID string, opts *ListOptions) ([]*ApprovalDecision, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `
		SELECT d.id, d.request_id, d.decision, d.decided_by, d.reason, d.policy_applied, d.created_at
		FROM approval_decisions d
		JOIN approval_requests r ON d.request_id = r.id
		WHERE r.session_id = ?
		ORDER BY d.created_at DESC
	`
	args := []interface{}{sessionID}

	if opts != nil && opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
		if opts.Offset > 0 {
			query += " OFFSET ?"
			args = append(args, opts.Offset)
		}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list decisions by session: %w", err)
	}
	defer rows.Close()

	return scanApprovalDecisions(rows)
}

// Statistics

func (r *SQLiteRepository) CountPendingRequests(ctx context.Context, sessionID string) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT COUNT(*) FROM approval_requests WHERE session_id = ? AND status = 'pending'`
	var count int
	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count pending requests: %w", err)
	}

	return count, nil
}

func (r *SQLiteRepository) CountRequestsByStatus(ctx context.Context, sessionID string) (map[RequestStatus]int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	query := `SELECT status, COUNT(*) FROM approval_requests WHERE session_id = ? GROUP BY status`
	rows, err := r.db.QueryContext(ctx, query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to count requests by status: %w", err)
	}
	defer rows.Close()

	counts := make(map[RequestStatus]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[RequestStatus(status)] = count
	}

	return counts, nil
}

// Helper functions

func scanApprovalRequest(row *sql.Row) (*ApprovalRequest, error) {
	req := &ApprovalRequest{}
	var riskLevel, status string
	var expiresAt, createdAt, updatedAt sql.NullString

	err := row.Scan(
		&req.ID,
		&req.SessionID,
		&req.ToolCallID,
		&req.ToolName,
		&req.ToolInput,
		&riskLevel,
		&status,
		&req.PolicyID,
		&req.RequestedBy,
		&expiresAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrApprovalRequestNotFound
		}
		return nil, fmt.Errorf("failed to scan approval request: %w", err)
	}

	req.RiskLevel = RiskLevel(riskLevel)
	req.Status = RequestStatus(status)

	if expiresAt.Valid {
		t, _ := time.Parse(time.RFC3339, expiresAt.String)
		req.ExpiresAt = sql.NullTime{Time: t, Valid: true}
	}
	if createdAt.Valid {
		req.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
	}
	if updatedAt.Valid {
		req.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
	}

	return req, nil
}

func scanApprovalRequests(rows *sql.Rows) ([]*ApprovalRequest, error) {
	var requests []*ApprovalRequest
	for rows.Next() {
		req := &ApprovalRequest{}
		var riskLevel, status string
		var expiresAt, createdAt, updatedAt sql.NullString

		err := rows.Scan(
			&req.ID,
			&req.SessionID,
			&req.ToolCallID,
			&req.ToolName,
			&req.ToolInput,
			&riskLevel,
			&status,
			&req.PolicyID,
			&req.RequestedBy,
			&expiresAt,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval request: %w", err)
		}

		req.RiskLevel = RiskLevel(riskLevel)
		req.Status = RequestStatus(status)

		if expiresAt.Valid {
			t, _ := time.Parse(time.RFC3339, expiresAt.String)
			req.ExpiresAt = sql.NullTime{Time: t, Valid: true}
		}
		if createdAt.Valid {
			req.CreatedAt, _ = time.Parse(time.RFC3339, createdAt.String)
		}
		if updatedAt.Valid {
			req.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt.String)
		}

		requests = append(requests, req)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approval requests: %w", err)
	}

	if requests == nil {
		requests = []*ApprovalRequest{}
	}

	return requests, nil
}

func scanApprovalPolicy(row *sql.Row) (*ApprovalPolicy, error) {
	policy := &ApprovalPolicy{}
	var mode, autoApproveRiskLevel string
	var isDefault, isActive int
	var createdAt, updatedAt string
	var autoApproveTrustedTools, alwaysRequireApprovalTools sql.NullString

	err := row.Scan(
		&policy.ID,
		&policy.Name,
		&policy.Description,
		&policy.ProjectPath,
		&mode,
		&autoApproveRiskLevel,
		&autoApproveTrustedTools,
		&alwaysRequireApprovalTools,
		&policy.TimeoutSeconds,
		&isDefault,
		&policy.Priority,
		&isActive,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrApprovalPolicyNotFound
		}
		return nil, fmt.Errorf("failed to scan approval policy: %w", err)
	}

	policy.Mode = ApprovalMode(mode)
	policy.AutoApproveRiskLevel = RiskLevel(autoApproveRiskLevel)
	policy.IsDefault = isDefault == 1
	policy.IsActive = isActive == 1

	if autoApproveTrustedTools.Valid {
		policy.AutoApproveTrustedTools = autoApproveTrustedTools.String
	}
	if alwaysRequireApprovalTools.Valid {
		policy.AlwaysRequireApprovalTools = alwaysRequireApprovalTools.String
	}

	policy.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	policy.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	return policy, nil
}

func scanApprovalPolicies(rows *sql.Rows) ([]*ApprovalPolicy, error) {
	var policies []*ApprovalPolicy
	for rows.Next() {
		policy := &ApprovalPolicy{}
		var mode, autoApproveRiskLevel string
		var isDefault, isActive int
		var createdAt, updatedAt string
		var autoApproveTrustedTools, alwaysRequireApprovalTools sql.NullString

		err := rows.Scan(
			&policy.ID,
			&policy.Name,
			&policy.Description,
			&policy.ProjectPath,
			&mode,
			&autoApproveRiskLevel,
			&autoApproveTrustedTools,
			&alwaysRequireApprovalTools,
			&policy.TimeoutSeconds,
			&isDefault,
			&policy.Priority,
			&isActive,
			&createdAt,
			&updatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval policy: %w", err)
		}

		policy.Mode = ApprovalMode(mode)
		policy.AutoApproveRiskLevel = RiskLevel(autoApproveRiskLevel)
		policy.IsDefault = isDefault == 1
		policy.IsActive = isActive == 1

		if autoApproveTrustedTools.Valid {
			policy.AutoApproveTrustedTools = autoApproveTrustedTools.String
		}
		if alwaysRequireApprovalTools.Valid {
			policy.AlwaysRequireApprovalTools = alwaysRequireApprovalTools.String
		}

		policy.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		policy.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

		policies = append(policies, policy)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approval policies: %w", err)
	}

	if policies == nil {
		policies = []*ApprovalPolicy{}
	}

	return policies, nil
}

func scanApprovalDecision(row *sql.Row) (*ApprovalDecision, error) {
	decision := &ApprovalDecision{}
	var decisionStatus string
	var reason sql.NullString
	var createdAt string

	err := row.Scan(
		&decision.ID,
		&decision.RequestID,
		&decisionStatus,
		&decision.DecidedBy,
		&reason,
		&decision.PolicyApplied,
		&createdAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrApprovalRequestNotFound
		}
		return nil, fmt.Errorf("failed to scan approval decision: %w", err)
	}

	decision.Decision = RequestStatus(decisionStatus)
	if reason.Valid {
		decision.Reason = reason.String
	}
	decision.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

	return decision, nil
}

func scanApprovalDecisions(rows *sql.Rows) ([]*ApprovalDecision, error) {
	var decisions []*ApprovalDecision
	for rows.Next() {
		decision := &ApprovalDecision{}
		var decisionStatus string
		var reason sql.NullString
		var createdAt string

		err := rows.Scan(
			&decision.ID,
			&decision.RequestID,
			&decisionStatus,
			&decision.DecidedBy,
			&reason,
			&decision.PolicyApplied,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan approval decision: %w", err)
		}

		decision.Decision = RequestStatus(decisionStatus)
		if reason.Valid {
			decision.Reason = reason.String
		}
		decision.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)

		decisions = append(decisions, decision)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating approval decisions: %w", err)
	}

	if decisions == nil {
		decisions = []*ApprovalDecision{}
	}

	return decisions, nil
}

func nullStringVal(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullTimeVal(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time.UTC().Format(time.RFC3339)
	}
	return nil
}

func nullString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation checks if the error is a SQLite unique constraint violation.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// Ensure SQLiteRepository implements Repository interface.
var _ Repository = (*SQLiteRepository)(nil)
