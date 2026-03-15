// Package approval provides tool approval workflow management for the OpenExec orchestrator.
// This file implements the ApprovalGate interface for blocking tool execution pending approval.
package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openexec/openexec/internal/mode"
)

// Common gate errors.
var (
	ErrApprovalGateNotConfigured = errors.New("approval gate not configured")
	ErrApprovalTimedOut          = errors.New("approval request timed out")
	ErrApprovalRejected          = errors.New("approval request rejected")
	ErrApprovalCancelled         = errors.New("approval request cancelled")
	ErrRequestAlreadyResolved    = errors.New("approval request already resolved")
)

// GateConfig holds configuration for the approval gate.
type GateConfig struct {
	// DefaultTimeout is the default timeout for approval requests.
	// Defaults to 5 minutes if not set.
	DefaultTimeout time.Duration

	// Enabled controls whether the gate is active.
	// When disabled, all requests are auto-approved.
	Enabled bool

	// AutoApproveInRunMode controls whether to auto-approve in ModeRun.
	// Defaults to true (full automation mode).
	AutoApproveInRunMode bool
}

// DefaultGateConfig returns the default gate configuration.
func DefaultGateConfig() *GateConfig {
	return &GateConfig{
		DefaultTimeout:       5 * time.Minute,
		Enabled:              true,
		AutoApproveInRunMode: true,
	}
}

// GateRequest represents a pending approval request in the gate.
type GateRequest struct {
	// ID is the unique identifier for the request.
	ID string `json:"id"`

	// RunID is the run that triggered this request.
	RunID string `json:"run_id"`

	// ToolName is the name of the tool being invoked.
	ToolName string `json:"tool_name"`

	// ToolArgs are the arguments passed to the tool.
	ToolArgs map[string]any `json:"tool_args"`

	// Description is a human-readable description of the operation.
	Description string `json:"description"`

	// RiskLevel is the assessed risk level of the operation.
	RiskLevel string `json:"risk_level"`

	// Status is the current status of the request.
	Status RequestStatus `json:"status"`

	// CreatedAt is when the request was created.
	CreatedAt time.Time `json:"created_at"`

	// ResolvedAt is when the request was resolved (approved/rejected/timeout).
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	// ResolvedBy identifies who resolved the request ("user", "auto", "timeout").
	ResolvedBy string `json:"resolved_by,omitempty"`

	// RejectReason is the reason for rejection (if rejected).
	RejectReason string `json:"reject_reason,omitempty"`
}

// ApprovalGate defines the interface for managing tool approval blocking.
type ApprovalGate interface {
	// RequestApproval creates a new approval request and blocks until resolved.
	// Returns the resolved request or an error if timed out/cancelled.
	RequestApproval(ctx context.Context, req *GateRequest) (*GateRequest, error)

	// GetPendingApprovals returns all pending approvals for a run.
	GetPendingApprovals(runID string) []*GateRequest

	// GetAllPendingApprovals returns all pending approvals across all runs.
	GetAllPendingApprovals() []*GateRequest

	// Approve approves a pending request.
	Approve(id string, by string) error

	// Reject rejects a pending request with a reason.
	Reject(id string, by string, reason string) error

	// Cancel cancels all pending requests for a run.
	CancelForRun(runID string) int
}

// pendingRequest wraps a gate request with a channel for signaling completion.
type pendingRequest struct {
	request *GateRequest
	done    chan struct{}
}

// InMemoryGate implements ApprovalGate with in-memory storage.
type InMemoryGate struct {
	config   *GateConfig
	manager  *Manager // Optional manager for persistence
	mode     mode.Mode
	pending  map[string]*pendingRequest
	resolved map[string]*GateRequest
	mu       sync.RWMutex
}

// NewInMemoryGate creates a new in-memory approval gate.
func NewInMemoryGate(config *GateConfig) *InMemoryGate {
	if config == nil {
		config = DefaultGateConfig()
	}
	return &InMemoryGate{
		config:   config,
		mode:     mode.ModeTask, // Default to task mode
		pending:  make(map[string]*pendingRequest),
		resolved: make(map[string]*GateRequest),
	}
}

// NewInMemoryGateWithManager creates a gate with persistence via Manager.
func NewInMemoryGateWithManager(config *GateConfig, manager *Manager) *InMemoryGate {
	gate := NewInMemoryGate(config)
	gate.manager = manager
	return gate
}

// SetMode sets the current operational mode.
func (g *InMemoryGate) SetMode(m mode.Mode) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.mode = m
}

// GetMode returns the current operational mode.
func (g *InMemoryGate) GetMode() mode.Mode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.mode
}

// IsEnabled returns whether the gate is enabled.
func (g *InMemoryGate) IsEnabled() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.config.Enabled
}

// SetEnabled enables or disables the gate.
func (g *InMemoryGate) SetEnabled(enabled bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.config.Enabled = enabled
}

// shouldAutoApprove determines if a request should be auto-approved.
func (g *InMemoryGate) shouldAutoApprove(req *GateRequest) bool {
	// If gate is disabled, auto-approve everything
	if !g.config.Enabled {
		return true
	}

	// In ModeRun, auto-approve if configured
	if g.mode == mode.ModeRun && g.config.AutoApproveInRunMode {
		return true
	}

	// In ModeChat, auto-approve (should not even reach here for writes)
	if g.mode == mode.ModeChat {
		return true
	}

	// Low risk operations can be auto-approved
	if req.RiskLevel == string(RiskLevelLow) {
		return true
	}

	// Otherwise, require explicit approval
	return false
}

// RequestApproval creates a new approval request and blocks until resolved.
func (g *InMemoryGate) RequestApproval(ctx context.Context, req *GateRequest) (*GateRequest, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}

	// Generate ID if not set
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now().UTC()
	req.CreatedAt = now
	req.Status = RequestStatusPending

	// Check for auto-approval
	g.mu.RLock()
	autoApprove := g.shouldAutoApprove(req)
	g.mu.RUnlock()

	if autoApprove {
		req.Status = RequestStatusAutoApproved
		req.ResolvedAt = &now
		req.ResolvedBy = "auto"
		return req, nil
	}

	// Create pending request with completion channel
	pending := &pendingRequest{
		request: req,
		done:    make(chan struct{}),
	}

	// Add to pending map
	g.mu.Lock()
	g.pending[req.ID] = pending
	g.mu.Unlock()

	// Calculate timeout
	timeout := g.config.DefaultTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}

	// Create a timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Wait for resolution
	select {
	case <-pending.done:
		// Request was resolved
		g.mu.RLock()
		resolved := g.resolved[req.ID]
		g.mu.RUnlock()

		if resolved == nil {
			resolved = req
		}

		if resolved.Status == RequestStatusRejected {
			return resolved, fmt.Errorf("%w: %s", ErrApprovalRejected, resolved.RejectReason)
		}

		return resolved, nil

	case <-timeoutCtx.Done():
		// Timeout or context cancellation
		g.mu.Lock()
		if p, ok := g.pending[req.ID]; ok {
			now := time.Now().UTC()
			p.request.Status = RequestStatusExpired
			p.request.ResolvedAt = &now
			p.request.ResolvedBy = "timeout"
			g.resolved[req.ID] = p.request
			delete(g.pending, req.ID)
		}
		g.mu.Unlock()

		if ctx.Err() != nil {
			return req, fmt.Errorf("%w: %v", ErrApprovalCancelled, ctx.Err())
		}
		return req, ErrApprovalTimedOut
	}
}

// GetPendingApprovals returns all pending approvals for a run.
func (g *InMemoryGate) GetPendingApprovals(runID string) []*GateRequest {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var result []*GateRequest
	for _, p := range g.pending {
		if p.request.RunID == runID {
			result = append(result, p.request)
		}
	}
	return result
}

// GetAllPendingApprovals returns all pending approvals across all runs.
func (g *InMemoryGate) GetAllPendingApprovals() []*GateRequest {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*GateRequest, 0, len(g.pending))
	for _, p := range g.pending {
		result = append(result, p.request)
	}
	return result
}

// GetRequest returns a pending or resolved request by ID.
func (g *InMemoryGate) GetRequest(id string) (*GateRequest, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if p, ok := g.pending[id]; ok {
		return p.request, true
	}
	if r, ok := g.resolved[id]; ok {
		return r, true
	}
	return nil, false
}

// Approve approves a pending request.
func (g *InMemoryGate) Approve(id string, by string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, ok := g.pending[id]
	if !ok {
		// Check if already resolved
		if _, resolved := g.resolved[id]; resolved {
			return ErrRequestAlreadyResolved
		}
		return ErrApprovalRequestNotFound
	}

	now := time.Now().UTC()
	p.request.Status = RequestStatusApproved
	p.request.ResolvedAt = &now
	p.request.ResolvedBy = by

	// Move to resolved
	g.resolved[id] = p.request
	delete(g.pending, id)

	// Signal completion
	close(p.done)

	return nil
}

// Reject rejects a pending request with a reason.
func (g *InMemoryGate) Reject(id string, by string, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	p, ok := g.pending[id]
	if !ok {
		// Check if already resolved
		if _, resolved := g.resolved[id]; resolved {
			return ErrRequestAlreadyResolved
		}
		return ErrApprovalRequestNotFound
	}

	now := time.Now().UTC()
	p.request.Status = RequestStatusRejected
	p.request.ResolvedAt = &now
	p.request.ResolvedBy = by
	p.request.RejectReason = reason

	// Move to resolved
	g.resolved[id] = p.request
	delete(g.pending, id)

	// Signal completion
	close(p.done)

	return nil
}

// CancelForRun cancels all pending requests for a run.
func (g *InMemoryGate) CancelForRun(runID string) int {
	g.mu.Lock()
	defer g.mu.Unlock()

	count := 0
	now := time.Now().UTC()

	for id, p := range g.pending {
		if p.request.RunID == runID {
			p.request.Status = RequestStatusCancelled
			p.request.ResolvedAt = &now
			p.request.ResolvedBy = "system"

			g.resolved[id] = p.request
			delete(g.pending, id)

			close(p.done)
			count++
		}
	}

	return count
}

// Stats returns statistics about the gate.
func (g *InMemoryGate) Stats() map[string]int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stats := map[string]int{
		"pending":  len(g.pending),
		"resolved": len(g.resolved),
	}

	// Count by status in resolved
	statusCounts := make(map[RequestStatus]int)
	for _, r := range g.resolved {
		statusCounts[r.Status]++
	}

	for status, count := range statusCounts {
		stats[string(status)] = count
	}

	return stats
}

// Clear clears all pending and resolved requests.
func (g *InMemoryGate) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Cancel all pending
	for _, p := range g.pending {
		close(p.done)
	}

	g.pending = make(map[string]*pendingRequest)
	g.resolved = make(map[string]*GateRequest)
}

// FormatRequestDescription generates a human-readable description of a tool call.
func FormatRequestDescription(toolName string, toolArgs map[string]any) string {
	switch toolName {
	case "write_file":
		if path, ok := toolArgs["path"].(string); ok {
			return fmt.Sprintf("Write file: %s", path)
		}
	case "git_apply_patch":
		if files, ok := getAffectedFiles(toolArgs); len(files) > 0 && ok {
			if len(files) == 1 {
				return fmt.Sprintf("Apply patch to: %s", files[0])
			}
			return fmt.Sprintf("Apply patch to %d files", len(files))
		}
		return "Apply git patch"
	case "run_shell_command":
		if cmd, ok := toolArgs["command"].(string); ok {
			if len(cmd) > 50 {
				cmd = cmd[:50] + "..."
			}
			return fmt.Sprintf("Run command: %s", cmd)
		}
	case "git_push":
		return "Push changes to remote repository"
	case "git_tag":
		if tag, ok := toolArgs["name"].(string); ok {
			return fmt.Sprintf("Create git tag: %s", tag)
		}
	}

	// Default description
	argsJSON, _ := json.Marshal(toolArgs)
	if len(argsJSON) > 100 {
		argsJSON = argsJSON[:100]
	}
	return fmt.Sprintf("Execute %s: %s", toolName, string(argsJSON))
}

// getAffectedFiles extracts file paths from tool arguments (for patches).
func getAffectedFiles(args map[string]any) ([]string, bool) {
	if files, ok := args["affected_files"].([]interface{}); ok {
		result := make([]string, 0, len(files))
		for _, f := range files {
			if s, ok := f.(string); ok {
				result = append(result, s)
			}
		}
		return result, true
	}
	return nil, false
}

// Ensure InMemoryGate implements ApprovalGate interface.
var _ ApprovalGate = (*InMemoryGate)(nil)
