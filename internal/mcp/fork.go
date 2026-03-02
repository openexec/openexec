// Package mcp provides MCP (Model Context Protocol) server functionality.
// This file implements session fork operations for the MCP server.
package mcp

import (
	"context"
	"fmt"

	"github.com/openexec/openexec/pkg/db/session"
)

// SessionForkManager handles session fork operations.
// It wraps the session repository to provide fork-related functionality
// exposed through MCP tools.
type SessionForkManager struct {
	repository session.Repository
}

// NewSessionForkManager creates a new SessionForkManager with the given repository.
func NewSessionForkManager(repository session.Repository) *SessionForkManager {
	return &SessionForkManager{
		repository: repository,
	}
}

// ForkSession creates a new session forked from an existing session.
// It validates the request, performs the fork operation, and returns detailed results.
func (m *SessionForkManager) ForkSession(ctx context.Context, req *ForkSessionRequest) (*ForkSessionResult, error) {
	if err := ValidateForkSessionRequest(req); err != nil {
		return nil, err
	}

	// Build fork options from request
	opts := &session.ForkOptions{
		ForkPointMessageID: req.ForkPointMessageID,
		Title:              req.Title,
		Provider:           req.Provider,
		Model:              req.Model,
		CopyMessages:       req.CopyMessages,
		CopyToolCalls:      req.CopyToolCalls,
		CopySummaries:      req.CopySummaries,
	}

	// Perform the fork operation
	forkedSession, err := m.repository.ForkSession(ctx, req.ParentSessionID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fork session: %w", err)
	}

	// Get fork info for the new session
	forkInfo, err := m.repository.GetForkInfo(ctx, forkedSession.ID)
	if err != nil {
		// Fork succeeded but couldn't get info - return with partial result
		return &ForkSessionResult{
			ForkedSessionID:    forkedSession.ID,
			ParentSessionID:    req.ParentSessionID,
			ForkPointMessageID: req.ForkPointMessageID,
			Title:              forkedSession.Title,
			Provider:           forkedSession.Provider,
			Model:              forkedSession.Model,
		}, nil
	}

	// Count copied items if applicable
	var messagesCopied, toolCallsCopied, summariesCopied int
	if req.CopyMessages {
		messages, err := m.repository.ListMessages(ctx, forkedSession.ID)
		if err == nil {
			messagesCopied = len(messages)
		}
		if req.CopyToolCalls {
			toolCalls, err := m.repository.ListToolCalls(ctx, forkedSession.ID)
			if err == nil {
				toolCallsCopied = len(toolCalls)
			}
		}
		if req.CopySummaries {
			summaries, err := m.repository.ListSummaries(ctx, forkedSession.ID)
			if err == nil {
				summariesCopied = len(summaries)
			}
		}
	}

	return &ForkSessionResult{
		ForkedSessionID:    forkedSession.ID,
		ParentSessionID:    req.ParentSessionID,
		ForkPointMessageID: req.ForkPointMessageID,
		Title:              forkedSession.Title,
		Provider:           forkedSession.Provider,
		Model:              forkedSession.Model,
		MessagesCopied:     messagesCopied,
		ToolCallsCopied:    toolCallsCopied,
		SummariesCopied:    summariesCopied,
		ForkDepth:          forkInfo.ForkDepth,
		AncestorChain:      forkInfo.AncestorChain,
	}, nil
}

// GetForkInfo retrieves detailed fork information for a session.
func (m *SessionForkManager) GetForkInfo(ctx context.Context, req *GetForkInfoRequest) (*session.ForkInfo, error) {
	if err := ValidateGetForkInfoRequest(req); err != nil {
		return nil, err
	}

	return m.repository.GetForkInfo(ctx, req.SessionID)
}

// ListSessionForks returns all direct forks of a session.
func (m *SessionForkManager) ListSessionForks(ctx context.Context, req *ListSessionForksRequest) ([]*session.Session, error) {
	if err := ValidateListSessionForksRequest(req); err != nil {
		return nil, err
	}

	return m.repository.GetSessionForks(ctx, req.SessionID)
}

// GetFullConversationHistory returns the complete conversation history including inherited messages.
func (m *SessionForkManager) GetFullConversationHistory(ctx context.Context, sessionID string) ([]*session.Message, error) {
	if sessionID == "" {
		return nil, &ValidationError{Field: "session_id", Message: "session_id is required"}
	}

	return m.repository.GetFullConversationHistory(ctx, sessionID)
}

// GetAncestorChain returns the complete chain of parent sessions from root to the given session.
func (m *SessionForkManager) GetAncestorChain(ctx context.Context, sessionID string) ([]*session.Session, error) {
	if sessionID == "" {
		return nil, &ValidationError{Field: "session_id", Message: "session_id is required"}
	}

	return m.repository.GetAncestorChain(ctx, sessionID)
}

// IsDescendantOf checks if one session is a descendant of another.
func (m *SessionForkManager) IsDescendantOf(ctx context.Context, childSessionID, ancestorSessionID string) (bool, error) {
	if childSessionID == "" {
		return false, &ValidationError{Field: "child_session_id", Message: "child_session_id is required"}
	}
	if ancestorSessionID == "" {
		return false, &ValidationError{Field: "ancestor_session_id", Message: "ancestor_session_id is required"}
	}

	return m.repository.IsDescendantOf(ctx, childSessionID, ancestorSessionID)
}
