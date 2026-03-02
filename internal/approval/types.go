// Package approval provides tool approval workflow management for the OpenExec orchestrator.
// It defines risk levels, approval policies, and request/decision data models for
// controlling which tool calls require human approval before execution.
package approval

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Common errors returned by approval operations.
var (
	ErrApprovalRequestNotFound = errors.New("approval request not found")
	ErrApprovalPolicyNotFound  = errors.New("approval policy not found")
	ErrInvalidApprovalRequest  = errors.New("invalid approval request data")
	ErrInvalidApprovalPolicy   = errors.New("invalid approval policy data")
	ErrInvalidApprovalDecision = errors.New("invalid approval decision data")
	ErrAlreadyDecided          = errors.New("approval request already has a decision")
	ErrRequestExpired          = errors.New("approval request has expired")
)

// RiskLevel represents the risk category of a tool operation.
// Higher risk levels require more stringent approval controls.
type RiskLevel string

const (
	// RiskLevelLow indicates a read-only operation with minimal risk.
	// Examples: read_file, list_directory, search_code
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium indicates operations that modify local files.
	// Examples: write_file, edit_file, create_directory
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh indicates operations that execute code or system commands.
	// Examples: run_shell_command, execute_script
	RiskLevelHigh RiskLevel = "high"
	// RiskLevelCritical indicates operations that could affect the orchestrator itself.
	// Examples: modify orchestrator files, system restart, git push
	RiskLevelCritical RiskLevel = "critical"
)

// ValidRiskLevels contains all valid risk level values.
var ValidRiskLevels = []RiskLevel{
	RiskLevelLow,
	RiskLevelMedium,
	RiskLevelHigh,
	RiskLevelCritical,
}

// IsValid checks if the risk level is a valid value.
func (r RiskLevel) IsValid() bool {
	for _, valid := range ValidRiskLevels {
		if r == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the risk level.
func (r RiskLevel) String() string {
	return string(r)
}

// Priority returns the numeric priority of the risk level (0 = lowest risk).
func (r RiskLevel) Priority() int {
	switch r {
	case RiskLevelLow:
		return 0
	case RiskLevelMedium:
		return 1
	case RiskLevelHigh:
		return 2
	case RiskLevelCritical:
		return 3
	default:
		return -1
	}
}

// RequestStatus represents the lifecycle status of an approval request.
type RequestStatus string

const (
	// RequestStatusPending indicates the request is awaiting a decision.
	RequestStatusPending RequestStatus = "pending"
	// RequestStatusApproved indicates the request was approved.
	RequestStatusApproved RequestStatus = "approved"
	// RequestStatusRejected indicates the request was rejected.
	RequestStatusRejected RequestStatus = "rejected"
	// RequestStatusAutoApproved indicates the request was auto-approved by policy.
	RequestStatusAutoApproved RequestStatus = "auto_approved"
	// RequestStatusExpired indicates the request timed out without a decision.
	RequestStatusExpired RequestStatus = "expired"
	// RequestStatusCancelled indicates the request was cancelled by the system.
	RequestStatusCancelled RequestStatus = "cancelled"
)

// ValidRequestStatuses contains all valid request status values.
var ValidRequestStatuses = []RequestStatus{
	RequestStatusPending,
	RequestStatusApproved,
	RequestStatusRejected,
	RequestStatusAutoApproved,
	RequestStatusExpired,
	RequestStatusCancelled,
}

// IsValid checks if the status is a valid request status value.
func (s RequestStatus) IsValid() bool {
	for _, valid := range ValidRequestStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the request status.
func (s RequestStatus) String() string {
	return string(s)
}

// IsFinal returns true if the status is terminal (no further changes expected).
func (s RequestStatus) IsFinal() bool {
	return s != RequestStatusPending
}

// ApprovalMode defines how approval decisions are made.
type ApprovalMode string

const (
	// ApprovalModeAlwaysAsk requires human approval for every operation.
	ApprovalModeAlwaysAsk ApprovalMode = "always_ask"
	// ApprovalModeAutoApprove automatically approves all operations.
	ApprovalModeAutoApprove ApprovalMode = "auto_approve"
	// ApprovalModeRiskBased approves based on risk level thresholds.
	ApprovalModeRiskBased ApprovalMode = "risk_based"
	// ApprovalModeSessionTrust auto-approves after trust established in session.
	ApprovalModeSessionTrust ApprovalMode = "session_trust"
)

// ValidApprovalModes contains all valid approval mode values.
var ValidApprovalModes = []ApprovalMode{
	ApprovalModeAlwaysAsk,
	ApprovalModeAutoApprove,
	ApprovalModeRiskBased,
	ApprovalModeSessionTrust,
}

// IsValid checks if the mode is a valid approval mode value.
func (m ApprovalMode) IsValid() bool {
	for _, valid := range ValidApprovalModes {
		if m == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the approval mode.
func (m ApprovalMode) String() string {
	return string(m)
}

// ApprovalRequest represents a request for approval of a tool operation.
type ApprovalRequest struct {
	// ID is the unique identifier for the request (UUID).
	ID string `json:"id"`
	// SessionID is the session that originated this request.
	SessionID string `json:"session_id"`
	// ToolCallID links to the tool_calls table for the operation.
	ToolCallID string `json:"tool_call_id"`
	// ToolName is the name of the tool being invoked.
	ToolName string `json:"tool_name"`
	// ToolInput is the JSON-encoded input parameters.
	ToolInput string `json:"tool_input"`
	// RiskLevel is the assessed risk level of this operation.
	RiskLevel RiskLevel `json:"risk_level"`
	// Status is the current status of the approval request.
	Status RequestStatus `json:"status"`
	// PolicyID is the approval policy that applies to this request.
	PolicyID sql.NullString `json:"policy_id,omitempty"`
	// RequestedBy identifies who/what triggered the request (agent ID, user ID).
	RequestedBy string `json:"requested_by"`
	// ExpiresAt is the deadline for the approval decision.
	ExpiresAt sql.NullTime `json:"expires_at,omitempty"`
	// CreatedAt is when the request was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the request was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewApprovalRequest creates a new ApprovalRequest with a generated UUID.
func NewApprovalRequest(sessionID, toolCallID, toolName, toolInput string, riskLevel RiskLevel, requestedBy string) (*ApprovalRequest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session_id is required", ErrInvalidApprovalRequest)
	}
	if toolCallID == "" {
		return nil, fmt.Errorf("%w: tool_call_id is required", ErrInvalidApprovalRequest)
	}
	if toolName == "" {
		return nil, fmt.Errorf("%w: tool_name is required", ErrInvalidApprovalRequest)
	}
	if !riskLevel.IsValid() {
		return nil, fmt.Errorf("%w: invalid risk level: %s", ErrInvalidApprovalRequest, riskLevel)
	}
	if requestedBy == "" {
		return nil, fmt.Errorf("%w: requested_by is required", ErrInvalidApprovalRequest)
	}

	now := time.Now().UTC()
	return &ApprovalRequest{
		ID:          uuid.New().String(),
		SessionID:   sessionID,
		ToolCallID:  toolCallID,
		ToolName:    toolName,
		ToolInput:   toolInput,
		RiskLevel:   riskLevel,
		Status:      RequestStatusPending,
		RequestedBy: requestedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Validate checks if the approval request has valid field values.
func (r *ApprovalRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidApprovalRequest)
	}
	if r.SessionID == "" {
		return fmt.Errorf("%w: session_id is required", ErrInvalidApprovalRequest)
	}
	if r.ToolCallID == "" {
		return fmt.Errorf("%w: tool_call_id is required", ErrInvalidApprovalRequest)
	}
	if r.ToolName == "" {
		return fmt.Errorf("%w: tool_name is required", ErrInvalidApprovalRequest)
	}
	if !r.RiskLevel.IsValid() {
		return fmt.Errorf("%w: invalid risk level: %s", ErrInvalidApprovalRequest, r.RiskLevel)
	}
	if !r.Status.IsValid() {
		return fmt.Errorf("%w: invalid status: %s", ErrInvalidApprovalRequest, r.Status)
	}
	if r.RequestedBy == "" {
		return fmt.Errorf("%w: requested_by is required", ErrInvalidApprovalRequest)
	}
	return nil
}

// IsPending returns true if the request is still awaiting a decision.
func (r *ApprovalRequest) IsPending() bool {
	return r.Status == RequestStatusPending
}

// IsApproved returns true if the request was approved (manually or automatically).
func (r *ApprovalRequest) IsApproved() bool {
	return r.Status == RequestStatusApproved || r.Status == RequestStatusAutoApproved
}

// IsExpired returns true if the request has passed its expiration time.
func (r *ApprovalRequest) IsExpired() bool {
	if !r.ExpiresAt.Valid {
		return false
	}
	return time.Now().UTC().After(r.ExpiresAt.Time)
}

// SetExpiration sets the expiration time for the request.
func (r *ApprovalRequest) SetExpiration(expiresAt time.Time) {
	r.ExpiresAt = sql.NullTime{Time: expiresAt.UTC(), Valid: true}
	r.UpdatedAt = time.Now().UTC()
}

// SetPolicy links this request to an approval policy.
func (r *ApprovalRequest) SetPolicy(policyID string) {
	r.PolicyID = sql.NullString{String: policyID, Valid: policyID != ""}
	r.UpdatedAt = time.Now().UTC()
}

// Approve marks the request as approved.
func (r *ApprovalRequest) Approve() error {
	if r.Status.IsFinal() {
		return ErrAlreadyDecided
	}
	r.Status = RequestStatusApproved
	r.UpdatedAt = time.Now().UTC()
	return nil
}

// Reject marks the request as rejected.
func (r *ApprovalRequest) Reject() error {
	if r.Status.IsFinal() {
		return ErrAlreadyDecided
	}
	r.Status = RequestStatusRejected
	r.UpdatedAt = time.Now().UTC()
	return nil
}

// AutoApprove marks the request as auto-approved.
func (r *ApprovalRequest) AutoApprove() error {
	if r.Status.IsFinal() {
		return ErrAlreadyDecided
	}
	r.Status = RequestStatusAutoApproved
	r.UpdatedAt = time.Now().UTC()
	return nil
}

// Expire marks the request as expired.
func (r *ApprovalRequest) Expire() error {
	if r.Status.IsFinal() {
		return ErrAlreadyDecided
	}
	r.Status = RequestStatusExpired
	r.UpdatedAt = time.Now().UTC()
	return nil
}

// Cancel marks the request as cancelled.
func (r *ApprovalRequest) Cancel() error {
	if r.Status.IsFinal() {
		return ErrAlreadyDecided
	}
	r.Status = RequestStatusCancelled
	r.UpdatedAt = time.Now().UTC()
	return nil
}

// ApprovalPolicy defines the rules for when approval is required.
type ApprovalPolicy struct {
	// ID is the unique identifier for the policy (UUID).
	ID string `json:"id"`
	// Name is a human-readable name for the policy.
	Name string `json:"name"`
	// Description explains the policy purpose.
	Description string `json:"description,omitempty"`
	// ProjectPath restricts this policy to a specific project (empty = global).
	ProjectPath sql.NullString `json:"project_path,omitempty"`
	// Mode determines the approval decision strategy.
	Mode ApprovalMode `json:"mode"`
	// AutoApproveRiskLevel is the maximum risk level to auto-approve (for risk_based mode).
	AutoApproveRiskLevel RiskLevel `json:"auto_approve_risk_level"`
	// AutoApproveTrustedTools lists tools that are always auto-approved.
	AutoApproveTrustedTools string `json:"auto_approve_trusted_tools,omitempty"` // JSON array
	// AlwaysRequireApprovalTools lists tools that always require approval.
	AlwaysRequireApprovalTools string `json:"always_require_approval_tools,omitempty"` // JSON array
	// TimeoutSeconds is the default timeout for approval requests.
	TimeoutSeconds int `json:"timeout_seconds"`
	// IsDefault indicates if this is the fallback policy.
	IsDefault bool `json:"is_default"`
	// Priority determines policy precedence (lower = higher priority).
	Priority int `json:"priority"`
	// IsActive indicates if the policy is currently active.
	IsActive bool `json:"is_active"`
	// CreatedAt is when the policy was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the policy was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewApprovalPolicy creates a new ApprovalPolicy with a generated UUID.
func NewApprovalPolicy(name string, mode ApprovalMode, autoApproveRiskLevel RiskLevel) (*ApprovalPolicy, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidApprovalPolicy)
	}
	if !mode.IsValid() {
		return nil, fmt.Errorf("%w: invalid mode: %s", ErrInvalidApprovalPolicy, mode)
	}
	if !autoApproveRiskLevel.IsValid() {
		return nil, fmt.Errorf("%w: invalid auto_approve_risk_level: %s", ErrInvalidApprovalPolicy, autoApproveRiskLevel)
	}

	now := time.Now().UTC()
	return &ApprovalPolicy{
		ID:                   uuid.New().String(),
		Name:                 name,
		Mode:                 mode,
		AutoApproveRiskLevel: autoApproveRiskLevel,
		TimeoutSeconds:       300, // 5 minutes default
		Priority:             100, // Default priority
		IsActive:             true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}, nil
}

// Validate checks if the approval policy has valid field values.
func (p *ApprovalPolicy) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidApprovalPolicy)
	}
	if p.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidApprovalPolicy)
	}
	if !p.Mode.IsValid() {
		return fmt.Errorf("%w: invalid mode: %s", ErrInvalidApprovalPolicy, p.Mode)
	}
	if !p.AutoApproveRiskLevel.IsValid() {
		return fmt.Errorf("%w: invalid auto_approve_risk_level: %s", ErrInvalidApprovalPolicy, p.AutoApproveRiskLevel)
	}
	if p.TimeoutSeconds < 0 {
		return fmt.Errorf("%w: timeout_seconds must be non-negative", ErrInvalidApprovalPolicy)
	}
	return nil
}

// SetProjectPath restricts this policy to a specific project.
func (p *ApprovalPolicy) SetProjectPath(path string) {
	p.ProjectPath = sql.NullString{String: path, Valid: path != ""}
	p.UpdatedAt = time.Now().UTC()
}

// SetDescription sets the policy description.
func (p *ApprovalPolicy) SetDescription(description string) {
	p.Description = description
	p.UpdatedAt = time.Now().UTC()
}

// Activate enables the policy.
func (p *ApprovalPolicy) Activate() {
	p.IsActive = true
	p.UpdatedAt = time.Now().UTC()
}

// Deactivate disables the policy.
func (p *ApprovalPolicy) Deactivate() {
	p.IsActive = false
	p.UpdatedAt = time.Now().UTC()
}

// ShouldAutoApprove determines if a tool call should be auto-approved based on this policy.
func (p *ApprovalPolicy) ShouldAutoApprove(toolName string, riskLevel RiskLevel) bool {
	if p.Mode == ApprovalModeAlwaysAsk {
		return false
	}
	if p.Mode == ApprovalModeAutoApprove {
		return true
	}
	if p.Mode == ApprovalModeRiskBased {
		return riskLevel.Priority() <= p.AutoApproveRiskLevel.Priority()
	}
	// ApprovalModeSessionTrust handled externally
	return false
}

// GetTimeout returns the timeout duration for this policy.
func (p *ApprovalPolicy) GetTimeout() time.Duration {
	return time.Duration(p.TimeoutSeconds) * time.Second
}

// ApprovalDecision records the decision made on an approval request.
type ApprovalDecision struct {
	// ID is the unique identifier for the decision (UUID).
	ID string `json:"id"`
	// RequestID links to the approval request.
	RequestID string `json:"request_id"`
	// Decision is the final status assigned.
	Decision RequestStatus `json:"decision"`
	// DecidedBy identifies who made the decision (user ID, "system", "policy").
	DecidedBy string `json:"decided_by"`
	// Reason provides justification for the decision.
	Reason string `json:"reason,omitempty"`
	// PolicyApplied is the policy ID if auto-decided.
	PolicyApplied sql.NullString `json:"policy_applied,omitempty"`
	// CreatedAt is when the decision was made.
	CreatedAt time.Time `json:"created_at"`
}

// NewApprovalDecision creates a new ApprovalDecision with a generated UUID.
func NewApprovalDecision(requestID string, decision RequestStatus, decidedBy string) (*ApprovalDecision, error) {
	if requestID == "" {
		return nil, fmt.Errorf("%w: request_id is required", ErrInvalidApprovalDecision)
	}
	if !decision.IsValid() {
		return nil, fmt.Errorf("%w: invalid decision: %s", ErrInvalidApprovalDecision, decision)
	}
	if !decision.IsFinal() {
		return nil, fmt.Errorf("%w: decision must be a final status", ErrInvalidApprovalDecision)
	}
	if decidedBy == "" {
		return nil, fmt.Errorf("%w: decided_by is required", ErrInvalidApprovalDecision)
	}

	return &ApprovalDecision{
		ID:        uuid.New().String(),
		RequestID: requestID,
		Decision:  decision,
		DecidedBy: decidedBy,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Validate checks if the approval decision has valid field values.
func (d *ApprovalDecision) Validate() error {
	if d.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidApprovalDecision)
	}
	if d.RequestID == "" {
		return fmt.Errorf("%w: request_id is required", ErrInvalidApprovalDecision)
	}
	if !d.Decision.IsValid() {
		return fmt.Errorf("%w: invalid decision: %s", ErrInvalidApprovalDecision, d.Decision)
	}
	if !d.Decision.IsFinal() {
		return fmt.Errorf("%w: decision must be a final status", ErrInvalidApprovalDecision)
	}
	if d.DecidedBy == "" {
		return fmt.Errorf("%w: decided_by is required", ErrInvalidApprovalDecision)
	}
	return nil
}

// SetReason sets the decision reason.
func (d *ApprovalDecision) SetReason(reason string) {
	d.Reason = reason
}

// SetPolicyApplied records which policy was used for auto-decisions.
func (d *ApprovalDecision) SetPolicyApplied(policyID string) {
	d.PolicyApplied = sql.NullString{String: policyID, Valid: policyID != ""}
}

// IsManual returns true if a human made the decision.
func (d *ApprovalDecision) IsManual() bool {
	return d.DecidedBy != "system" && d.DecidedBy != "policy"
}

// ToolRiskMapping defines the default risk level for a tool.
type ToolRiskMapping struct {
	// ToolName is the MCP tool name.
	ToolName string `json:"tool_name"`
	// RiskLevel is the default risk assessment for this tool.
	RiskLevel RiskLevel `json:"risk_level"`
	// Description explains why this risk level was assigned.
	Description string `json:"description,omitempty"`
}

// DefaultToolRiskMappings returns the default risk mappings for common tools.
func DefaultToolRiskMappings() []ToolRiskMapping {
	return []ToolRiskMapping{
		// Low risk - read-only operations
		{ToolName: "read_file", RiskLevel: RiskLevelLow, Description: "Reads file content without modification"},
		{ToolName: "list_directory", RiskLevel: RiskLevelLow, Description: "Lists directory contents"},
		{ToolName: "search_code", RiskLevel: RiskLevelLow, Description: "Searches code without modification"},
		{ToolName: "glob", RiskLevel: RiskLevelLow, Description: "Pattern matching for files"},
		{ToolName: "grep", RiskLevel: RiskLevelLow, Description: "Text search in files"},

		// Medium risk - file modifications
		{ToolName: "write_file", RiskLevel: RiskLevelMedium, Description: "Creates or overwrites files"},
		{ToolName: "edit_file", RiskLevel: RiskLevelMedium, Description: "Modifies file content"},
		{ToolName: "create_directory", RiskLevel: RiskLevelMedium, Description: "Creates directories"},
		{ToolName: "delete_file", RiskLevel: RiskLevelMedium, Description: "Deletes files"},
		{ToolName: "git_apply_patch", RiskLevel: RiskLevelMedium, Description: "Applies git patches to files"},

		// High risk - command execution
		{ToolName: "run_shell_command", RiskLevel: RiskLevelHigh, Description: "Executes shell commands"},
		{ToolName: "execute_script", RiskLevel: RiskLevelHigh, Description: "Runs executable scripts"},
		{ToolName: "git_commit", RiskLevel: RiskLevelHigh, Description: "Creates git commits"},
		{ToolName: "git_push", RiskLevel: RiskLevelHigh, Description: "Pushes changes to remote"},

		// Critical risk - orchestrator modifications
		{ToolName: "modify_orchestrator", RiskLevel: RiskLevelCritical, Description: "Modifies orchestrator code"},
		{ToolName: "restart_orchestrator", RiskLevel: RiskLevelCritical, Description: "Restarts the orchestrator"},
		{ToolName: "deploy", RiskLevel: RiskLevelCritical, Description: "Deploys to production"},
	}
}
