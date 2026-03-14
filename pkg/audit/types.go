// Package audit provides types and utilities for tracking all system events.
package audit

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Common errors returned by audit operations.
var (
	ErrAuditEntryNotFound = errors.New("audit entry not found")
	ErrInvalidAuditEntry  = errors.New("invalid audit entry data")
	ErrInvalidEventType   = errors.New("invalid event type")
)

// EventType represents the type of audit event being logged.
type EventType string

const (
	// Session lifecycle events
	EventSessionCreated  EventType = "session.created"
	EventSessionForked   EventType = "session.forked"
	EventSessionPaused   EventType = "session.paused"
	EventSessionResumed  EventType = "session.resumed"
	EventSessionArchived EventType = "session.archived"
	EventSessionDeleted  EventType = "session.deleted"

	// Message events
	EventMessageSent     EventType = "message.sent"
	EventMessageReceived EventType = "message.received"

	// Tool call events
	EventToolCallRequested    EventType = "tool_call.requested"
	EventToolCallApproved     EventType = "tool_call.approved"
	EventToolCallRejected     EventType = "tool_call.rejected"
	EventToolCallAutoApproved EventType = "tool_call.auto_approved"
	EventToolCallStarted      EventType = "tool_call.started"
	EventToolCallCompleted    EventType = "tool_call.completed"
	EventToolCallFailed       EventType = "tool_call.failed"
	EventToolCallCancelled    EventType = "tool_call.cancelled"

	// LLM provider events
	EventLLMRequestSent      EventType = "llm.request_sent"
	EventLLMResponseReceived EventType = "llm.response_received"
	EventLLMStreamChunk      EventType = "llm.stream_chunk"
	EventLLMError            EventType = "llm.error"

	// Cost and usage events
	EventUsageRecorded  EventType = "usage.recorded"
	EventBudgetWarning  EventType = "usage.budget_warning"
	EventBudgetExceeded EventType = "usage.budget_exceeded"

	// Context events
	EventContextInjected   EventType = "context.injected"
	EventContextSummarized EventType = "context.summarized"

	// Security events
	EventSecurityPathViolation EventType = "security.path_violation"
	EventSecurityRateLimit     EventType = "security.rate_limit"
	EventSecurityAuthFailure   EventType = "security.auth_failure"

	// System events
	EventSystemStartup  EventType = "system.startup"
	EventSystemShutdown EventType = "system.shutdown"
    EventSystemError    EventType = "system.error"

    // Run lifecycle events (deterministic execution)
    EventRunCreated EventType = "run.created"
    EventRunStep    EventType = "run.step"
)

// ValidEventTypes contains all valid audit event type values.
var ValidEventTypes = []EventType{
	EventSessionCreated,
	EventSessionForked,
	EventSessionPaused,
	EventSessionResumed,
	EventSessionArchived,
	EventSessionDeleted,
	EventMessageSent,
	EventMessageReceived,
	EventToolCallRequested,
	EventToolCallApproved,
	EventToolCallRejected,
	EventToolCallAutoApproved,
	EventToolCallStarted,
	EventToolCallCompleted,
	EventToolCallFailed,
	EventToolCallCancelled,
	EventLLMRequestSent,
	EventLLMResponseReceived,
	EventLLMStreamChunk,
	EventLLMError,
	EventUsageRecorded,
	EventBudgetWarning,
	EventBudgetExceeded,
	EventContextInjected,
	EventContextSummarized,
	EventSecurityPathViolation,
	EventSecurityRateLimit,
	EventSecurityAuthFailure,
	EventSystemStartup,
	EventSystemShutdown,
    EventSystemError,
    EventRunCreated,
    EventRunStep,
}

// IsValid checks if the event type is a valid audit event type value.
func (e EventType) IsValid() bool {
	for _, valid := range ValidEventTypes {
		if e == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the event type.
func (e EventType) String() string {
	return string(e)
}

// Category returns the category of the event (session, tool_call, llm, etc.).
func (e EventType) Category() string {
	s := string(e)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i]
		}
	}
	return s
}

// Severity represents the severity level of an audit event.
type Severity string

const (
	// SeverityDebug is for detailed diagnostic information.
	SeverityDebug Severity = "debug"
	// SeverityInfo is for routine operational information.
	SeverityInfo Severity = "info"
	// SeverityWarning is for potentially harmful situations.
	SeverityWarning Severity = "warning"
	// SeverityError is for error events that might still allow the application to continue.
	SeverityError Severity = "error"
	// SeverityCritical is for critical conditions requiring immediate attention.
	SeverityCritical Severity = "critical"
)

// ValidSeverities contains all valid severity level values.
var ValidSeverities = []Severity{SeverityDebug, SeverityInfo, SeverityWarning, SeverityError, SeverityCritical}

// IsValid checks if the severity is a valid severity level value.
func (s Severity) IsValid() bool {
	for _, valid := range ValidSeverities {
		if s == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the severity.
func (s Severity) String() string {
	return string(s)
}

// Entry represents a single audit log entry.
type Entry struct {
	// ID is the unique identifier for the audit entry (UUID).
	ID string `json:"id"`

	// Timestamp is when the event occurred (RFC3339 format).
	Timestamp time.Time `json:"timestamp"`

	// EventType is the type of event being logged.
	EventType EventType `json:"event_type"`

	// Severity is the severity level of the event.
	Severity Severity `json:"severity"`

	// SessionID is the session this event belongs to (if applicable).
	SessionID sql.NullString `json:"session_id,omitempty"`

	// MessageID is the message this event relates to (if applicable).
	MessageID sql.NullString `json:"message_id,omitempty"`

	// ToolCallID is the tool call this event relates to (if applicable).
	ToolCallID sql.NullString `json:"tool_call_id,omitempty"`

	// ActorID identifies who/what triggered this event (user ID, system, etc.).
	ActorID string `json:"actor_id"`

	// ActorType identifies the type of actor (user, system, llm).
	ActorType string `json:"actor_type"`

	// ProjectPath is the project workspace associated with this event.
	ProjectPath sql.NullString `json:"project_path,omitempty"`

	// Provider is the LLM provider (if applicable).
	Provider sql.NullString `json:"provider,omitempty"`

	// Model is the LLM model (if applicable).
	Model sql.NullString `json:"model,omitempty"`

	// TokensInput is the number of input tokens (for LLM events).
	TokensInput sql.NullInt64 `json:"tokens_input,omitempty"`

	// TokensOutput is the number of output tokens (for LLM events).
	TokensOutput sql.NullInt64 `json:"tokens_output,omitempty"`

	// CostUSD is the cost in USD (for usage events).
	CostUSD sql.NullFloat64 `json:"cost_usd,omitempty"`

	// DurationMs is the duration of the operation in milliseconds.
	DurationMs sql.NullInt64 `json:"duration_ms,omitempty"`

	// Success indicates whether the operation succeeded.
	Success sql.NullBool `json:"success,omitempty"`

	// ErrorMessage contains any error message.
	ErrorMessage sql.NullString `json:"error_message,omitempty"`

	// Metadata contains additional structured data as JSON.
	Metadata sql.NullString `json:"metadata,omitempty"`

	// Iteration is the agent loop iteration number (if applicable).
	Iteration sql.NullInt64 `json:"iteration,omitempty"`

	// IPAddress is the IP address of the client (if applicable).
	IPAddress sql.NullString `json:"ip_address,omitempty"`

	// UserAgent is the user agent string (if applicable).
	UserAgent sql.NullString `json:"user_agent,omitempty"`

	// CreatedAt is when the entry was persisted to the database.
	CreatedAt time.Time `json:"created_at"`
}

// EntryBuilder provides a fluent interface for constructing audit entries.
type EntryBuilder struct {
	entry *Entry
}

// NewEntry creates a new audit entry with required fields.
func NewEntry(eventType EventType, actorID, actorType string) (*EntryBuilder, error) {
	if !eventType.IsValid() {
		return nil, fmt.Errorf("%w: %s", ErrInvalidEventType, eventType)
	}
	if actorID == "" {
		return nil, fmt.Errorf("%w: actor_id is required", ErrInvalidAuditEntry)
	}
	if actorType == "" {
		return nil, fmt.Errorf("%w: actor_type is required", ErrInvalidAuditEntry)
	}

	now := time.Now().UTC()
	entry := &Entry{
		ID:        uuid.New().String(),
		Timestamp: now,
		EventType: eventType,
		Severity:  SeverityInfo, // default severity
		ActorID:   actorID,
		ActorType: actorType,
		CreatedAt: now,
	}

	return &EntryBuilder{entry: entry}, nil
}

// WithSeverity sets the severity level.
func (b *EntryBuilder) WithSeverity(severity Severity) *EntryBuilder {
	if severity.IsValid() {
		b.entry.Severity = severity
	}
	return b
}

// WithSession sets the session ID.
func (b *EntryBuilder) WithSession(sessionID string) *EntryBuilder {
	if sessionID != "" {
		b.entry.SessionID = sql.NullString{String: sessionID, Valid: true}
	}
	return b
}

// WithMessage sets the message ID.
func (b *EntryBuilder) WithMessage(messageID string) *EntryBuilder {
	if messageID != "" {
		b.entry.MessageID = sql.NullString{String: messageID, Valid: true}
	}
	return b
}

// WithToolCall sets the tool call ID.
func (b *EntryBuilder) WithToolCall(toolCallID string) *EntryBuilder {
	if toolCallID != "" {
		b.entry.ToolCallID = sql.NullString{String: toolCallID, Valid: true}
	}
	return b
}

// WithProject sets the project path.
func (b *EntryBuilder) WithProject(projectPath string) *EntryBuilder {
	if projectPath != "" {
		b.entry.ProjectPath = sql.NullString{String: projectPath, Valid: true}
	}
	return b
}

// WithProvider sets the LLM provider and model.
func (b *EntryBuilder) WithProvider(provider, model string) *EntryBuilder {
	if provider != "" {
		b.entry.Provider = sql.NullString{String: provider, Valid: true}
	}
	if model != "" {
		b.entry.Model = sql.NullString{String: model, Valid: true}
	}
	return b
}

// WithTokens sets the token counts.
func (b *EntryBuilder) WithTokens(input, output int64) *EntryBuilder {
	b.entry.TokensInput = sql.NullInt64{Int64: input, Valid: true}
	b.entry.TokensOutput = sql.NullInt64{Int64: output, Valid: true}
	return b
}

// WithCost sets the cost in USD.
func (b *EntryBuilder) WithCost(costUSD float64) *EntryBuilder {
	b.entry.CostUSD = sql.NullFloat64{Float64: costUSD, Valid: true}
	return b
}

// WithDuration sets the duration in milliseconds.
func (b *EntryBuilder) WithDuration(durationMs int64) *EntryBuilder {
	b.entry.DurationMs = sql.NullInt64{Int64: durationMs, Valid: true}
	return b
}

// WithSuccess sets whether the operation succeeded.
func (b *EntryBuilder) WithSuccess(success bool) *EntryBuilder {
	b.entry.Success = sql.NullBool{Bool: success, Valid: true}
	return b
}

// WithError sets the error message.
func (b *EntryBuilder) WithError(errorMessage string) *EntryBuilder {
	if errorMessage != "" {
		b.entry.ErrorMessage = sql.NullString{String: errorMessage, Valid: true}
		// Also set success to false when there's an error
		b.entry.Success = sql.NullBool{Bool: false, Valid: true}
	}
	return b
}

// WithMetadata sets the metadata as JSON.
func (b *EntryBuilder) WithMetadata(metadata interface{}) *EntryBuilder {
	if metadata != nil {
		data, err := json.Marshal(metadata)
		if err == nil {
			b.entry.Metadata = sql.NullString{String: string(data), Valid: true}
		}
	}
	return b
}

// WithIteration sets the agent loop iteration number.
func (b *EntryBuilder) WithIteration(iteration int64) *EntryBuilder {
	b.entry.Iteration = sql.NullInt64{Int64: iteration, Valid: true}
	return b
}

// WithClientInfo sets the IP address and user agent.
func (b *EntryBuilder) WithClientInfo(ipAddress, userAgent string) *EntryBuilder {
	if ipAddress != "" {
		b.entry.IPAddress = sql.NullString{String: ipAddress, Valid: true}
	}
	if userAgent != "" {
		b.entry.UserAgent = sql.NullString{String: userAgent, Valid: true}
	}
	return b
}

// WithTimestamp overrides the default timestamp.
func (b *EntryBuilder) WithTimestamp(timestamp time.Time) *EntryBuilder {
	b.entry.Timestamp = timestamp
	return b
}

// Build validates and returns the audit entry.
func (b *EntryBuilder) Build() (*Entry, error) {
	if err := b.entry.Validate(); err != nil {
		return nil, err
	}
	return b.entry, nil
}

// Validate checks if the audit entry has valid field values.
func (e *Entry) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidAuditEntry)
	}
	if !e.EventType.IsValid() {
		return fmt.Errorf("%w: invalid event_type: %s", ErrInvalidAuditEntry, e.EventType)
	}
	if !e.Severity.IsValid() {
		return fmt.Errorf("%w: invalid severity: %s", ErrInvalidAuditEntry, e.Severity)
	}
	if e.ActorID == "" {
		return fmt.Errorf("%w: actor_id is required", ErrInvalidAuditEntry)
	}
	if e.ActorType == "" {
		return fmt.Errorf("%w: actor_type is required", ErrInvalidAuditEntry)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp is required", ErrInvalidAuditEntry)
	}
	return nil
}

// GetMetadata parses and returns the metadata as the given type.
func (e *Entry) GetMetadata(v interface{}) error {
	if !e.Metadata.Valid {
		return nil
	}
	return json.Unmarshal([]byte(e.Metadata.String), v)
}

// QueryFilter represents filter criteria for querying audit entries.
type QueryFilter struct {
	// EventTypes filters by event types.
	EventTypes []EventType
	// Severities filters by severity levels.
	Severities []Severity
	// SessionID filters by session ID.
	SessionID string
	// ActorID filters by actor ID.
	ActorID string
	// ProjectPath filters by project path.
	ProjectPath string
	// Since filters to entries after this time.
	Since time.Time
	// Until filters to entries before this time.
	Until time.Time
	// Limit limits the number of results.
	Limit int
	// Offset skips the first N results.
	Offset int
}

// QueryResult represents the result of an audit query.
type QueryResult struct {
	// Entries contains the matching audit entries.
	Entries []*Entry
	// TotalCount is the total number of matching entries (before pagination).
	TotalCount int64
	// HasMore indicates if there are more results available.
	HasMore bool
}

// UsageStats represents aggregated usage statistics from audit entries.
type UsageStats struct {
	// TotalTokensInput is the total input tokens used.
	TotalTokensInput int64 `json:"total_tokens_input"`
	// TotalTokensOutput is the total output tokens used.
	TotalTokensOutput int64 `json:"total_tokens_output"`
	// TotalCostUSD is the total cost in USD.
	TotalCostUSD float64 `json:"total_cost_usd"`
	// TotalRequests is the total number of LLM requests.
	TotalRequests int64 `json:"total_requests"`
	// SuccessfulRequests is the number of successful requests.
	SuccessfulRequests int64 `json:"successful_requests"`
	// FailedRequests is the number of failed requests.
	FailedRequests int64 `json:"failed_requests"`
	// AverageDurationMs is the average request duration in milliseconds.
	AverageDurationMs float64 `json:"average_duration_ms"`
	// ByProvider contains stats broken down by provider.
	ByProvider map[string]*ProviderStats `json:"by_provider,omitempty"`
}

// ProviderStats represents usage statistics for a single provider.
type ProviderStats struct {
	Provider          string  `json:"provider"`
	TotalTokensInput  int64   `json:"total_tokens_input"`
	TotalTokensOutput int64   `json:"total_tokens_output"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalRequests     int64   `json:"total_requests"`
}

// ToolCallStats represents aggregated tool call statistics.
type ToolCallStats struct {
	// TotalRequested is the total number of tool calls requested.
	TotalRequested int64 `json:"total_requested"`
	// TotalApproved is the number of approved tool calls.
	TotalApproved int64 `json:"total_approved"`
	// TotalRejected is the number of rejected tool calls.
	TotalRejected int64 `json:"total_rejected"`
	// TotalAutoApproved is the number of auto-approved tool calls.
	TotalAutoApproved int64 `json:"total_auto_approved"`
	// TotalCompleted is the number of successfully completed tool calls.
	TotalCompleted int64 `json:"total_completed"`
	// TotalFailed is the number of failed tool calls.
	TotalFailed int64 `json:"total_failed"`
	// ByTool contains stats broken down by tool name.
	ByTool map[string]int64 `json:"by_tool,omitempty"`
}
