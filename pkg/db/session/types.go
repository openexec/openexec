package session

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/openexec/openexec/internal/mode"
)

// Common errors returned by session operations.
var (
	ErrSessionNotFound     = errors.New("session not found")
	ErrMessageNotFound     = errors.New("message not found")
	ErrToolCallNotFound    = errors.New("tool call not found")
	ErrInvalidSession      = errors.New("invalid session data")
	ErrInvalidMessage      = errors.New("invalid message data")
	ErrInvalidToolCall     = errors.New("invalid tool call data")
	ErrSessionAlreadyExist = errors.New("session already exists")
	ErrInvalidForkPoint    = errors.New("invalid fork point: message not found or does not belong to session")
	ErrCircularFork        = errors.New("circular fork detected: session cannot fork from its own descendant")
)

// Status represents the lifecycle status of a session.
type Status string

const (
	// StatusActive indicates the session is currently in use.
	StatusActive Status = "active"
	// StatusPaused indicates the session is temporarily paused.
	StatusPaused Status = "paused"
	// StatusArchived indicates the session has been archived and is read-only.
	StatusArchived Status = "archived"
	// StatusDeleted indicates the session has been soft-deleted.
	StatusDeleted Status = "deleted"
)

// ValidStatuses contains all valid session status values.
var ValidStatuses = []Status{StatusActive, StatusPaused, StatusArchived, StatusDeleted}

// IsValid checks if the status is a valid session status value.
func (s Status) IsValid() bool {
	for _, valid := range ValidStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}

// Role represents the role of a message sender.
type Role string

const (
	// RoleUser indicates a message from the user.
	RoleUser Role = "user"
	// RoleAssistant indicates a message from the AI assistant.
	RoleAssistant Role = "assistant"
	// RoleSystem indicates a system message (context injection).
	RoleSystem Role = "system"
)

// ValidRoles contains all valid message role values.
var ValidRoles = []Role{RoleUser, RoleAssistant, RoleSystem}

// IsValid checks if the role is a valid message role value.
func (r Role) IsValid() bool {
	for _, valid := range ValidRoles {
		if r == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the role.
func (r Role) String() string {
	return string(r)
}

// ToolCallStatus represents the execution status of a tool call.
type ToolCallStatus string

const (
	// ToolCallStatusPending indicates the tool call is awaiting execution.
	ToolCallStatusPending ToolCallStatus = "pending"
	// ToolCallStatusRunning indicates the tool call is currently executing.
	ToolCallStatusRunning ToolCallStatus = "running"
	// ToolCallStatusCompleted indicates the tool call completed successfully.
	ToolCallStatusCompleted ToolCallStatus = "completed"
	// ToolCallStatusFailed indicates the tool call failed with an error.
	ToolCallStatusFailed ToolCallStatus = "failed"
	// ToolCallStatusCancelled indicates the tool call was cancelled.
	ToolCallStatusCancelled ToolCallStatus = "cancelled"
)

// ValidToolCallStatuses contains all valid tool call status values.
var ValidToolCallStatuses = []ToolCallStatus{
	ToolCallStatusPending,
	ToolCallStatusRunning,
	ToolCallStatusCompleted,
	ToolCallStatusFailed,
	ToolCallStatusCancelled,
}

// IsValid checks if the status is a valid tool call status value.
func (s ToolCallStatus) IsValid() bool {
	for _, valid := range ValidToolCallStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the tool call status.
func (s ToolCallStatus) String() string {
	return string(s)
}

// ApprovalStatus represents the approval state of a tool call.
type ApprovalStatus string

const (
	// ApprovalStatusPending indicates approval is awaited.
	ApprovalStatusPending ApprovalStatus = "pending"
	// ApprovalStatusApproved indicates the tool call was approved.
	ApprovalStatusApproved ApprovalStatus = "approved"
	// ApprovalStatusRejected indicates the tool call was rejected.
	ApprovalStatusRejected ApprovalStatus = "rejected"
	// ApprovalStatusAutoApproved indicates the tool call was auto-approved.
	ApprovalStatusAutoApproved ApprovalStatus = "auto_approved"
)

// ValidApprovalStatuses contains all valid approval status values.
var ValidApprovalStatuses = []ApprovalStatus{
	ApprovalStatusPending,
	ApprovalStatusApproved,
	ApprovalStatusRejected,
	ApprovalStatusAutoApproved,
}

// IsValid checks if the status is a valid approval status value.
func (s ApprovalStatus) IsValid() bool {
	for _, valid := range ValidApprovalStatuses {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the approval status.
func (s ApprovalStatus) String() string {
	return string(s)
}

// Session represents a chat session bound to a project workspace.
type Session struct {
	// ID is the unique identifier for the session (UUID).
	ID string `json:"id"`
	// ProjectPath is the absolute path to the project workspace.
	ProjectPath string `json:"project_path"`
	// Provider is the LLM provider (e.g., "openai", "anthropic", "gemini").
	Provider string `json:"provider"`
	// Model is the specific model ID (e.g., "gpt-4", "claude-3-opus").
	Model string `json:"model"`
	// Title is a user-friendly title for the session.
	Title string `json:"title,omitempty"`
	// Mode is the current operational mode (chat, task, run).
	Mode mode.Mode `json:"mode"`
	// ModeState tracks mode transitions and history.
	ModeState *mode.State `json:"mode_state,omitempty"`
	// ParentSessionID is the ID of the parent session if this is a fork.
	ParentSessionID sql.NullString `json:"parent_session_id,omitempty"`
	// ForkPointMessageID is the message ID where this session forked from parent.
	ForkPointMessageID sql.NullString `json:"fork_point_message_id,omitempty"`
	// Status is the current lifecycle status of the session.
	Status Status `json:"status"`
	// CreatedAt is when the session was created.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the session was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// NewSession creates a new Session with a generated UUID.
// Sessions start in chat mode by default.
func NewSession(projectPath, provider, model string) (*Session, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("%w: project_path is required", ErrInvalidSession)
	}
	if provider == "" {
		return nil, fmt.Errorf("%w: provider is required", ErrInvalidSession)
	}
	if model == "" {
		return nil, fmt.Errorf("%w: model is required", ErrInvalidSession)
	}

	now := time.Now().UTC()
	return &Session{
		ID:          uuid.New().String(),
		ProjectPath: projectPath,
		Provider:    provider,
		Model:       model,
		Mode:        mode.ModeChat,
		ModeState:   mode.NewState(),
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Validate checks if the session has valid field values.
func (s *Session) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidSession)
	}
	if s.ProjectPath == "" {
		return fmt.Errorf("%w: project_path is required", ErrInvalidSession)
	}
	if s.Provider == "" {
		return fmt.Errorf("%w: provider is required", ErrInvalidSession)
	}
	if s.Model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidSession)
	}
	if !s.Status.IsValid() {
		return fmt.Errorf("%w: invalid status: %s", ErrInvalidSession, s.Status)
	}
	return nil
}

// IsFork returns true if this session was forked from another session.
func (s *Session) IsFork() bool {
	return s.ParentSessionID.Valid && s.ParentSessionID.String != ""
}

// SetTitle sets the session title.
func (s *Session) SetTitle(title string) {
	s.Title = title
	s.UpdatedAt = time.Now().UTC()
}

// Archive marks the session as archived.
func (s *Session) Archive() {
	s.Status = StatusArchived
	s.UpdatedAt = time.Now().UTC()
}

// SetForkParent sets the parent session and fork point for this session.
func (s *Session) SetForkParent(parentID, forkPointMessageID string) {
	s.ParentSessionID = sql.NullString{String: parentID, Valid: true}
	s.ForkPointMessageID = sql.NullString{String: forkPointMessageID, Valid: true}
	s.UpdatedAt = time.Now().UTC()
}

// GetParentID returns the parent session ID if this is a fork, empty string otherwise.
func (s *Session) GetParentID() string {
	if s.ParentSessionID.Valid {
		return s.ParentSessionID.String
	}
	return ""
}

// GetForkPointMessageID returns the fork point message ID if this is a fork, empty string otherwise.
func (s *Session) GetForkPointMessageID() string {
	if s.ForkPointMessageID.Valid {
		return s.ForkPointMessageID.String
	}
	return ""
}

// TransitionMode transitions the session to a new operational mode.
func (s *Session) TransitionMode(to mode.Mode, condition mode.TransitionCondition, reason string) error {
	if s.ModeState == nil {
		s.ModeState = mode.NewState()
	}
	if err := s.ModeState.TransitionTo(to, condition, reason); err != nil {
		return err
	}
	s.Mode = to
	s.UpdatedAt = time.Now().UTC()
	return nil
}

// CanWrite returns true if the current mode allows write operations.
func (s *Session) CanWrite() bool {
	return s.Mode.AllowsWrites()
}

// CanExec returns true if the current mode allows command execution.
func (s *Session) CanExec() bool {
	return s.Mode.AllowsExec()
}

// RequiresApproval returns true if operations require user approval in current mode.
func (s *Session) RequiresApproval() bool {
	return s.Mode.RequiresApproval()
}

// ForkOptions configures session forking behavior.
type ForkOptions struct {
	// ForkPointMessageID is the message ID where the fork diverges from the parent.
	// All messages up to and including this message are inherited by the fork.
	// Required field.
	ForkPointMessageID string
	// Title is an optional title for the forked session.
	// If empty, a default title will be generated.
	Title string
	// Provider overrides the LLM provider for the forked session.
	// If empty, inherits from parent.
	Provider string
	// Model overrides the model for the forked session.
	// If empty, inherits from parent.
	Model string
	// CopyMessages determines whether to copy messages from parent up to fork point.
	// If true, messages are duplicated into the new session.
	// If false, fork references parent history (more space efficient but requires traversal).
	CopyMessages bool
	// CopyToolCalls determines whether to copy tool calls when CopyMessages is true.
	CopyToolCalls bool
	// CopySummaries determines whether to copy session summaries when CopyMessages is true.
	CopySummaries bool
}

// Validate checks if the fork options are valid.
func (o *ForkOptions) Validate() error {
	if o.ForkPointMessageID == "" {
		return fmt.Errorf("%w: fork_point_message_id is required", ErrInvalidForkPoint)
	}
	return nil
}

// ForkInfo contains detailed information about a session's fork relationship.
type ForkInfo struct {
	// SessionID is the session this info belongs to.
	SessionID string `json:"session_id"`
	// ParentSessionID is the immediate parent session, if any.
	ParentSessionID string `json:"parent_session_id,omitempty"`
	// RootSessionID is the original ancestor session in the fork chain.
	RootSessionID string `json:"root_session_id"`
	// ForkPointMessageID is where this session diverged from its parent.
	ForkPointMessageID string `json:"fork_point_message_id,omitempty"`
	// ForkDepth is the number of generations from the root session.
	// Root sessions have depth 0, their forks have depth 1, etc.
	ForkDepth int `json:"fork_depth"`
	// ChildCount is the number of direct child forks of this session.
	ChildCount int `json:"child_count"`
	// TotalDescendants is the total number of descendants (all generations).
	TotalDescendants int `json:"total_descendants"`
	// AncestorChain is the list of session IDs from root to this session.
	AncestorChain []string `json:"ancestor_chain"`
	// ForkCreatedAt is when this fork was created.
	ForkCreatedAt time.Time `json:"fork_created_at,omitempty"`
}

// IsRoot returns true if this session is a root session (not a fork).
func (f *ForkInfo) IsRoot() bool {
	return f.ParentSessionID == ""
}

// ForkSession creates a new session forked from this session at the specified message.
// This is a factory method that creates the Session struct; actual persistence is handled by Repository.
func (s *Session) ForkSession(opts *ForkOptions) (*Session, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	// Determine provider and model
	provider := s.Provider
	if opts.Provider != "" {
		provider = opts.Provider
	}
	model := s.Model
	if opts.Model != "" {
		model = opts.Model
	}

	// Generate title if not provided
	title := opts.Title
	if title == "" {
		title = fmt.Sprintf("Fork of %s", s.Title)
		if s.Title == "" {
			title = fmt.Sprintf("Fork from %s", s.ID[:8])
		}
	}

	forkedSession := &Session{
		ID:                 uuid.New().String(),
		ProjectPath:        s.ProjectPath,
		Provider:           provider,
		Model:              model,
		Title:              title,
		Mode:               mode.ModeChat, // Forks start in chat mode
		ModeState:          mode.NewState(),
		ParentSessionID:    sql.NullString{String: s.ID, Valid: true},
		ForkPointMessageID: sql.NullString{String: opts.ForkPointMessageID, Valid: true},
		Status:             StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	return forkedSession, nil
}

// Message represents a single message in a session conversation.
type Message struct {
	// ID is the unique identifier for the message (UUID).
	ID string `json:"id"`
	// SessionID is the session this message belongs to.
	SessionID string `json:"session_id"`
	// Role indicates who sent the message (user, assistant, system).
	Role Role `json:"role"`
	// Content is the message text content.
	Content string `json:"content"`
	// TokensInput is the number of input tokens for this message (API usage).
	TokensInput int `json:"tokens_input"`
	// TokensOutput is the number of output tokens for this message (API usage).
	TokensOutput int `json:"tokens_output"`
	// CostUSD is the estimated cost in USD for this message.
	CostUSD float64 `json:"cost_usd"`
	// CreatedAt is when the message was created.
	CreatedAt time.Time `json:"created_at"`
}

// NewMessage creates a new Message with a generated UUID.
func NewMessage(sessionID string, role Role, content string) (*Message, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session_id is required", ErrInvalidMessage)
	}
	if !role.IsValid() {
		return nil, fmt.Errorf("%w: invalid role: %s", ErrInvalidMessage, role)
	}

	return &Message{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Validate checks if the message has valid field values.
func (m *Message) Validate() error {
	if m.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidMessage)
	}
	if m.SessionID == "" {
		return fmt.Errorf("%w: session_id is required", ErrInvalidMessage)
	}
	if !m.Role.IsValid() {
		return fmt.Errorf("%w: invalid role: %s", ErrInvalidMessage, m.Role)
	}
	return nil
}

// SetTokenUsage sets the token usage metrics for the message.
func (m *Message) SetTokenUsage(input, output int, costUSD float64) {
	m.TokensInput = input
	m.TokensOutput = output
	m.CostUSD = costUSD
}

// ToolCall represents an MCP tool invocation within a conversation.
type ToolCall struct {
	// ID is the unique identifier for the tool call (UUID).
	ID string `json:"id"`
	// MessageID is the assistant message that initiated this tool call.
	MessageID string `json:"message_id"`
	// SessionID is the session this tool call belongs to.
	SessionID string `json:"session_id"`
	// ToolName is the MCP tool being invoked (e.g., "read_file", "write_file").
	ToolName string `json:"tool_name"`
	// ToolInput is the JSON-encoded input parameters for the tool.
	ToolInput string `json:"tool_input"`
	// ToolOutput is the JSON-encoded output from the tool execution.
	ToolOutput sql.NullString `json:"tool_output,omitempty"`
	// Status is the current execution status of the tool call.
	Status ToolCallStatus `json:"status"`
	// ApprovalStatus is the approval state for write/exec operations.
	ApprovalStatus sql.NullString `json:"approval_status,omitempty"`
	// ApprovedBy is the user who approved/rejected the tool call.
	ApprovedBy sql.NullString `json:"approved_by,omitempty"`
	// ApprovedAt is when the approval decision was made.
	ApprovedAt sql.NullTime `json:"approved_at,omitempty"`
	// StartedAt is when the tool execution started.
	StartedAt sql.NullTime `json:"started_at,omitempty"`
	// CompletedAt is when the tool execution completed.
	CompletedAt sql.NullTime `json:"completed_at,omitempty"`
	// Error contains the error message if the tool call failed.
	Error sql.NullString `json:"error,omitempty"`
	// CreatedAt is when the tool call was created.
	CreatedAt time.Time `json:"created_at"`
}

// NewToolCall creates a new ToolCall with a generated UUID.
func NewToolCall(messageID, sessionID, toolName, toolInput string) (*ToolCall, error) {
	if messageID == "" {
		return nil, fmt.Errorf("%w: message_id is required", ErrInvalidToolCall)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("%w: session_id is required", ErrInvalidToolCall)
	}
	if toolName == "" {
		return nil, fmt.Errorf("%w: tool_name is required", ErrInvalidToolCall)
	}

	return &ToolCall{
		ID:        uuid.New().String(),
		MessageID: messageID,
		SessionID: sessionID,
		ToolName:  toolName,
		ToolInput: toolInput,
		Status:    ToolCallStatusPending,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Validate checks if the tool call has valid field values.
func (t *ToolCall) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidToolCall)
	}
	if t.MessageID == "" {
		return fmt.Errorf("%w: message_id is required", ErrInvalidToolCall)
	}
	if t.SessionID == "" {
		return fmt.Errorf("%w: session_id is required", ErrInvalidToolCall)
	}
	if t.ToolName == "" {
		return fmt.Errorf("%w: tool_name is required", ErrInvalidToolCall)
	}
	if !t.Status.IsValid() {
		return fmt.Errorf("%w: invalid status: %s", ErrInvalidToolCall, t.Status)
	}
	return nil
}

// Start marks the tool call as running.
func (t *ToolCall) Start() {
	t.Status = ToolCallStatusRunning
	now := time.Now().UTC()
	t.StartedAt = sql.NullTime{Time: now, Valid: true}
}

// Complete marks the tool call as completed with output.
func (t *ToolCall) Complete(output string) {
	t.Status = ToolCallStatusCompleted
	t.ToolOutput = sql.NullString{String: output, Valid: true}
	now := time.Now().UTC()
	t.CompletedAt = sql.NullTime{Time: now, Valid: true}
}

// Fail marks the tool call as failed with an error.
func (t *ToolCall) Fail(err string) {
	t.Status = ToolCallStatusFailed
	t.Error = sql.NullString{String: err, Valid: true}
	now := time.Now().UTC()
	t.CompletedAt = sql.NullTime{Time: now, Valid: true}
}

// Cancel marks the tool call as cancelled.
func (t *ToolCall) Cancel() {
	t.Status = ToolCallStatusCancelled
	now := time.Now().UTC()
	t.CompletedAt = sql.NullTime{Time: now, Valid: true}
}

// Approve marks the tool call as approved.
func (t *ToolCall) Approve(approvedBy string) {
	t.ApprovalStatus = sql.NullString{String: string(ApprovalStatusApproved), Valid: true}
	t.ApprovedBy = sql.NullString{String: approvedBy, Valid: true}
	now := time.Now().UTC()
	t.ApprovedAt = sql.NullTime{Time: now, Valid: true}
}

// Reject marks the tool call as rejected.
func (t *ToolCall) Reject(rejectedBy string) {
	t.ApprovalStatus = sql.NullString{String: string(ApprovalStatusRejected), Valid: true}
	t.ApprovedBy = sql.NullString{String: rejectedBy, Valid: true}
	now := time.Now().UTC()
	t.ApprovedAt = sql.NullTime{Time: now, Valid: true}
	t.Status = ToolCallStatusCancelled
}

// AutoApprove marks the tool call as auto-approved.
func (t *ToolCall) AutoApprove() {
	t.ApprovalStatus = sql.NullString{String: string(ApprovalStatusAutoApproved), Valid: true}
	now := time.Now().UTC()
	t.ApprovedAt = sql.NullTime{Time: now, Valid: true}
}

// NeedsApproval returns true if the tool call requires approval.
func (t *ToolCall) NeedsApproval() bool {
	return !t.ApprovalStatus.Valid || t.ApprovalStatus.String == string(ApprovalStatusPending)
}

// IsApproved returns true if the tool call has been approved.
func (t *ToolCall) IsApproved() bool {
	if !t.ApprovalStatus.Valid {
		return false
	}
	status := ApprovalStatus(t.ApprovalStatus.String)
	return status == ApprovalStatusApproved || status == ApprovalStatusAutoApproved
}

// SessionSummary represents a compressed summary of conversation history.
type SessionSummary struct {
	// ID is the unique identifier for the summary (UUID).
	ID string `json:"id"`
	// SessionID is the session this summary belongs to.
	SessionID string `json:"session_id"`
	// SummaryText is the compressed summary content.
	SummaryText string `json:"summary_text"`
	// MessagesSummarized is the count of messages that were summarized.
	MessagesSummarized int `json:"messages_summarized"`
	// TokensSaved is the estimated tokens saved by summarization.
	TokensSaved int `json:"tokens_saved"`
	// CreatedAt is when the summary was created.
	CreatedAt time.Time `json:"created_at"`
}

// NewSessionSummary creates a new SessionSummary with a generated UUID.
func NewSessionSummary(sessionID, summaryText string, messagesSummarized, tokensSaved int) (*SessionSummary, error) {
	if sessionID == "" {
		return nil, errors.New("session_id is required")
	}
	if summaryText == "" {
		return nil, errors.New("summary_text is required")
	}
	if messagesSummarized < 1 {
		return nil, errors.New("messages_summarized must be at least 1")
	}

	return &SessionSummary{
		ID:                 uuid.New().String(),
		SessionID:          sessionID,
		SummaryText:        summaryText,
		MessagesSummarized: messagesSummarized,
		TokensSaved:        tokensSaved,
		CreatedAt:          time.Now().UTC(),
	}, nil
}

// Validate checks if the session summary has valid field values.
func (s *SessionSummary) Validate() error {
	if s.ID == "" {
		return errors.New("id is required")
	}
	if s.SessionID == "" {
		return errors.New("session_id is required")
	}
	if s.SummaryText == "" {
		return errors.New("summary_text is required")
	}
	if s.MessagesSummarized < 1 {
		return errors.New("messages_summarized must be at least 1")
	}
	return nil
}
