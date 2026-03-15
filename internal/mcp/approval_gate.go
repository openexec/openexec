// Package mcp provides the Model Context Protocol server implementation.
// This file implements approval gate integration for tool execution.
package mcp

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/internal/approval"
)

// ApprovalGateServer is an interface extension for servers that support approval gates.
type ApprovalGateServer interface {
	SetApprovalGate(gate approval.ApprovalGate)
	GetApprovalGate() approval.ApprovalGate
	SetApprovalEnabled(enabled bool)
	IsApprovalEnabled() bool
}

// approvalGateState holds the approval gate state for the server.
// This is embedded as a separate type to avoid polluting the main Server struct.
type approvalGateState struct {
	gate    approval.ApprovalGate
	enabled bool
}

// serverApprovalGate provides approval gate access for servers.
// Use GetApprovalGate() and SetApprovalGate() methods.
var serverApprovalGates = make(map[*Server]*approvalGateState)

// SetApprovalGateForServer sets the approval gate for a server instance.
func SetApprovalGateForServer(s *Server, gate approval.ApprovalGate) {
	if s == nil {
		return
	}
	state, ok := serverApprovalGates[s]
	if !ok {
		state = &approvalGateState{}
		serverApprovalGates[s] = state
	}
	state.gate = gate
}

// GetApprovalGateForServer returns the approval gate for a server instance.
func GetApprovalGateForServer(s *Server) approval.ApprovalGate {
	if s == nil {
		return nil
	}
	if state, ok := serverApprovalGates[s]; ok {
		return state.gate
	}
	return nil
}

// SetApprovalEnabledForServer enables or disables approval for a server.
func SetApprovalEnabledForServer(s *Server, enabled bool) {
	if s == nil {
		return
	}
	state, ok := serverApprovalGates[s]
	if !ok {
		state = &approvalGateState{}
		serverApprovalGates[s] = state
	}
	state.enabled = enabled
}

// IsApprovalEnabledForServer returns whether approval is enabled for a server.
func IsApprovalEnabledForServer(s *Server) bool {
	if s == nil {
		return false
	}
	if state, ok := serverApprovalGates[s]; ok {
		return state.enabled && state.gate != nil
	}
	return false
}

// CleanupApprovalGateForServer removes the approval gate state for a server.
// Call this when the server is being shut down.
func CleanupApprovalGateForServer(s *Server) {
	delete(serverApprovalGates, s)
}

// RequiresApprovalForTool checks if a tool call requires approval.
// This is used by handleToolsCall to determine if blocking is needed.
func RequiresApprovalForTool(s *Server, toolName string) bool {
	if s == nil {
		return false
	}

	// Approval only applies in Task mode
	if !s.currentMode.RequiresApproval() {
		return false
	}

	// Check if approval gate is configured and enabled
	if !IsApprovalEnabledForServer(s) {
		return false
	}

	// Check toolset registry for approval requirements
	if s.toolsetRegistry != nil {
		return s.toolsetRegistry.RequiresApproval(toolName)
	}

	// Default: require approval for write/exec tools
	writeTools := map[string]bool{
		"write_file":        true,
		"git_apply_patch":   true,
		"run_shell_command": true,
		"git_push":          true,
		"git_tag":           true,
		"changelog_update":  true,
	}
	return writeTools[toolName]
}

// RequestToolApproval requests approval for a tool call through the gate.
// Returns nil if approved, error if rejected or not configured.
func RequestToolApproval(s *Server, ctx context.Context, toolName string, toolArgs map[string]interface{}) error {
	gate := GetApprovalGateForServer(s)
	if gate == nil {
		return nil // No gate configured, proceed
	}

	// Convert args to map[string]any
	argsAny := make(map[string]any)
	for k, v := range toolArgs {
		argsAny[k] = v
	}

	// Determine risk level from toolset
	riskLevel := "medium"
	if s.toolsetRegistry != nil {
		riskLevel = string(s.toolsetRegistry.GetRiskLevel(toolName))
	}

	// Create approval request
	gateReq := &approval.GateRequest{
		RunID:       s.runID,
		ToolName:    toolName,
		ToolArgs:    argsAny,
		Description: approval.FormatRequestDescription(toolName, argsAny),
		RiskLevel:   riskLevel,
	}

	// Request approval (blocks until resolved)
	result, err := gate.RequestApproval(ctx, gateReq)
	if err != nil {
		return fmt.Errorf("approval required: %w", err)
	}

	// Check if approved
	if result.Status != approval.RequestStatusApproved && result.Status != approval.RequestStatusAutoApproved {
		reason := "Request was not approved"
		if result.RejectReason != "" {
			reason = result.RejectReason
		}
		return fmt.Errorf("approval denied: %s", reason)
	}

	return nil
}
