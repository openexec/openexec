package session

import (
	"context"
)

// Repository defines the interface for session persistence operations.
// It provides CRUD operations for sessions, messages, tool calls, and summaries.
type Repository interface {
	// Session operations

	// CreateSession stores a new session.
	// Returns ErrSessionAlreadyExist if a session with the same ID exists.
	CreateSession(ctx context.Context, session *Session) error

	// GetSession retrieves a session by its ID.
	// Returns ErrSessionNotFound if the session doesn't exist.
	GetSession(ctx context.Context, id string) (*Session, error)

	// UpdateSession modifies an existing session.
	// Returns ErrSessionNotFound if the session doesn't exist.
	UpdateSession(ctx context.Context, session *Session) error

	// DeleteSession removes a session by its ID.
	// This will cascade delete all associated messages, tool calls, and summaries.
	// Returns ErrSessionNotFound if the session doesn't exist.
	DeleteSession(ctx context.Context, id string) error

	// ListSessions returns all sessions, optionally filtered.
	// Returns an empty slice if no sessions exist.
	ListSessions(ctx context.Context, opts *ListSessionsOptions) ([]*Session, error)

	// ListSessionsByProject returns all sessions for a specific project path.
	// Returns an empty slice if no sessions exist.
	ListSessionsByProject(ctx context.Context, projectPath string) ([]*Session, error)

	// GetSessionForks returns all sessions that were forked from the given session.
	// Returns an empty slice if no forks exist.
	GetSessionForks(ctx context.Context, sessionID string) ([]*Session, error)

	// Fork operations

	// ForkSession creates a new session forked from an existing session.
	// The fork inherits the parent's conversation history up to the fork point.
	// If opts.CopyMessages is true, messages are copied; otherwise, fork references parent history.
	// Returns the newly created forked session.
	ForkSession(ctx context.Context, parentSessionID string, opts *ForkOptions) (*Session, error)

	// GetForkInfo returns detailed fork information for a session.
	// This includes the ancestor chain, depth, and descendant counts.
	GetForkInfo(ctx context.Context, sessionID string) (*ForkInfo, error)

	// GetAncestorChain returns the complete chain of parent sessions from root to the given session.
	// The first element is the root session, and the last is the given session.
	GetAncestorChain(ctx context.Context, sessionID string) ([]*Session, error)

	// GetRootSession returns the original root session for any session in a fork tree.
	// For root sessions, returns the session itself.
	GetRootSession(ctx context.Context, sessionID string) (*Session, error)

	// ListDescendants returns all descendant sessions (direct and indirect) of a session.
	// Results are ordered by fork depth, then by creation time.
	ListDescendants(ctx context.Context, sessionID string) ([]*Session, error)

	// IsDescendantOf checks if childSessionID is a descendant of ancestorSessionID.
	IsDescendantOf(ctx context.Context, childSessionID, ancestorSessionID string) (bool, error)

	// Message operations

	// CreateMessage stores a new message for a session.
	// Returns ErrSessionNotFound if the session doesn't exist.
	CreateMessage(ctx context.Context, message *Message) error

	// GetMessage retrieves a message by its ID.
	// Returns ErrMessageNotFound if the message doesn't exist.
	GetMessage(ctx context.Context, id string) (*Message, error)

	// UpdateMessage modifies an existing message (e.g., to update token usage).
	// Returns ErrMessageNotFound if the message doesn't exist.
	UpdateMessage(ctx context.Context, message *Message) error

	// DeleteMessage removes a message by its ID.
	// This will cascade delete all associated tool calls.
	// Returns ErrMessageNotFound if the message doesn't exist.
	DeleteMessage(ctx context.Context, id string) error

	// ListMessages returns all messages for a session, ordered by created_at.
	// Returns an empty slice if no messages exist.
	ListMessages(ctx context.Context, sessionID string) ([]*Message, error)

	// ListMessagesByRole returns messages for a session filtered by role.
	// Returns an empty slice if no messages match.
	ListMessagesByRole(ctx context.Context, sessionID string, role Role) ([]*Message, error)

	// GetMessageCount returns the total number of messages in a session.
	GetMessageCount(ctx context.Context, sessionID string) (int, error)

	// ListMessagesUpTo returns messages for a session up to and including the specified message.
	// Useful for retrieving conversation history up to a fork point.
	// Messages are ordered by created_at ASC.
	ListMessagesUpTo(ctx context.Context, sessionID, upToMessageID string) ([]*Message, error)

	// GetFullConversationHistory returns the complete conversation history for a forked session,
	// including inherited messages from all ancestor sessions up to each fork point.
	// For root sessions, this is equivalent to ListMessages.
	// Messages are returned in chronological order.
	GetFullConversationHistory(ctx context.Context, sessionID string) ([]*Message, error)

	// Tool call operations

	// CreateToolCall stores a new tool call for a message.
	// Returns ErrMessageNotFound if the message doesn't exist.
	CreateToolCall(ctx context.Context, toolCall *ToolCall) error

	// GetToolCall retrieves a tool call by its ID.
	// Returns ErrToolCallNotFound if the tool call doesn't exist.
	GetToolCall(ctx context.Context, id string) (*ToolCall, error)

	// UpdateToolCall modifies an existing tool call.
	// Returns ErrToolCallNotFound if the tool call doesn't exist.
	UpdateToolCall(ctx context.Context, toolCall *ToolCall) error

	// DeleteToolCall removes a tool call by its ID.
	// Returns ErrToolCallNotFound if the tool call doesn't exist.
	DeleteToolCall(ctx context.Context, id string) error

	// ListToolCalls returns all tool calls for a session.
	// Returns an empty slice if no tool calls exist.
	ListToolCalls(ctx context.Context, sessionID string) ([]*ToolCall, error)

	// ListToolCallsByMessage returns all tool calls for a specific message.
	// Returns an empty slice if no tool calls exist.
	ListToolCallsByMessage(ctx context.Context, messageID string) ([]*ToolCall, error)

	// ListToolCallsByStatus returns tool calls filtered by status.
	// Returns an empty slice if no tool calls match.
	ListToolCallsByStatus(ctx context.Context, sessionID string, status ToolCallStatus) ([]*ToolCall, error)

	// ListPendingApprovals returns tool calls awaiting approval.
	// Returns an empty slice if no tool calls need approval.
	ListPendingApprovals(ctx context.Context, sessionID string) ([]*ToolCall, error)

	// Session summary operations

	// CreateSummary stores a new session summary.
	// Returns ErrSessionNotFound if the session doesn't exist.
	CreateSummary(ctx context.Context, summary *SessionSummary) error

	// GetSummary retrieves a summary by its ID.
	// Returns an error if the summary doesn't exist.
	GetSummary(ctx context.Context, id string) (*SessionSummary, error)

	// ListSummaries returns all summaries for a session, ordered by created_at.
	// Returns an empty slice if no summaries exist.
	ListSummaries(ctx context.Context, sessionID string) ([]*SessionSummary, error)

	// GetLatestSummary returns the most recent summary for a session.
	// Returns nil if no summaries exist.
	GetLatestSummary(ctx context.Context, sessionID string) (*SessionSummary, error)

	// DeleteSummary removes a summary by its ID.
	DeleteSummary(ctx context.Context, id string) error

	// Aggregation operations

	// GetSessionStats returns aggregate statistics for a session.
	GetSessionStats(ctx context.Context, sessionID string) (*SessionStats, error)

	// GetUsageByProvider returns usage statistics grouped by provider.
	GetUsageByProvider(ctx context.Context) ([]*ProviderUsage, error)

	// Lifecycle

	// Close closes the repository and releases any resources.
	Close() error
}

// ListSessionsOptions configures session listing behavior.
type ListSessionsOptions struct {
	// Status filters sessions by status. Empty means all statuses.
	Status Status
	// Limit restricts the number of results. 0 means no limit.
	Limit int
	// Offset skips the first N results.
	Offset int
	// OrderBy specifies the sort order (created_at, updated_at). Default is created_at DESC.
	OrderBy string
	// OrderDir specifies sort direction (ASC, DESC). Default is DESC.
	OrderDir string
}

// SessionStats contains aggregate statistics for a session.
type SessionStats struct {
	// SessionID is the session identifier.
	SessionID string `json:"session_id"`
	// MessageCount is the total number of messages.
	MessageCount int `json:"message_count"`
	// ToolCallCount is the total number of tool calls.
	ToolCallCount int `json:"tool_call_count"`
	// TotalTokensInput is the sum of input tokens.
	TotalTokensInput int `json:"total_tokens_input"`
	// TotalTokensOutput is the sum of output tokens.
	TotalTokensOutput int `json:"total_tokens_output"`
	// TotalCostUSD is the sum of costs.
	TotalCostUSD float64 `json:"total_cost_usd"`
	// SummaryCount is the number of summaries created.
	SummaryCount int `json:"summary_count"`
	// TokensSaved is the total tokens saved via summarization.
	TokensSaved int `json:"tokens_saved"`
}

// ProviderUsage contains usage statistics for a provider.
type ProviderUsage struct {
	// Provider is the provider name.
	Provider string `json:"provider"`
	// Model is the model name.
	Model string `json:"model"`
	// SessionCount is the number of sessions.
	SessionCount int `json:"session_count"`
	// MessageCount is the total number of messages.
	MessageCount int `json:"message_count"`
	// TotalTokensInput is the sum of input tokens.
	TotalTokensInput int `json:"total_tokens_input"`
	// TotalTokensOutput is the sum of output tokens.
	TotalTokensOutput int `json:"total_tokens_output"`
	// TotalCostUSD is the sum of costs.
	TotalCostUSD float64 `json:"total_cost_usd"`
}
