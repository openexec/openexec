// Package loop provides the agent loop execution engine and event types
// for orchestrating AI agent interactions with tool execution.
package loop

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Common errors returned by event operations.
var (
	ErrInvalidEvent     = errors.New("invalid event data")
	ErrInvalidEventType = errors.New("invalid event type")
	ErrInvalidEventKind = errors.New("invalid event kind")
)

// LoopEventType identifies the kind of loop event in the agent execution cycle.
// This extends the base EventType with more granular events for tool execution.
type LoopEventType string

const (
	// Lifecycle events
	LoopEventStart       LoopEventType = "loop.start"       // Loop execution begins
	LoopEventPause       LoopEventType = "loop.pause"       // Loop paused by user/system
	LoopEventResume      LoopEventType = "loop.resume"      // Loop resumed after pause
	LoopEventStop        LoopEventType = "loop.stop"        // Loop stopped by user/system
	LoopEventComplete    LoopEventType = "loop.complete"    // Loop completed successfully
	LoopEventError       LoopEventType = "loop.error"       // Loop terminated with error
	LoopEventTimeout     LoopEventType = "loop.timeout"     // Loop timed out
	LoopEventMaxReached  LoopEventType = "loop.max_reached" // Max iterations reached

	// Iteration events
	IterationStart    LoopEventType = "iteration.start"    // Iteration begins
	IterationComplete LoopEventType = "iteration.complete" // Iteration completes
	IterationRetry    LoopEventType = "iteration.retry"    // Iteration will be retried
	IterationSkip     LoopEventType = "iteration.skip"     // Iteration skipped

	// LLM interaction events
	LLMRequestStart  LoopEventType = "llm.request_start"  // LLM request initiated
	LLMRequestEnd    LoopEventType = "llm.request_end"    // LLM request completed
	LLMStreamStart   LoopEventType = "llm.stream_start"   // Streaming response begins
	LLMStreamChunk   LoopEventType = "llm.stream_chunk"   // Streaming chunk received
	LLMStreamEnd     LoopEventType = "llm.stream_end"     // Streaming response ends
	LLMError         LoopEventType = "llm.error"          // LLM request error
	LLMRateLimit     LoopEventType = "llm.rate_limit"     // Rate limit hit
	LLMContextWindow LoopEventType = "llm.context_window" // Context window exceeded

	// Tool execution events
	ToolCallRequested  LoopEventType = "tool.call_requested"  // Tool call requested by LLM
	ToolCallQueued     LoopEventType = "tool.call_queued"     // Tool call queued for approval
	ToolCallApproved   LoopEventType = "tool.call_approved"   // Tool call approved
	ToolCallRejected   LoopEventType = "tool.call_rejected"   // Tool call rejected
	ToolCallStart      LoopEventType = "tool.call_start"      // Tool execution begins
	ToolCallProgress   LoopEventType = "tool.call_progress"   // Tool execution progress update
	ToolCallComplete   LoopEventType = "tool.call_complete"   // Tool execution completes
	ToolCallError      LoopEventType = "tool.call_error"      // Tool execution error
	ToolCallTimeout    LoopEventType = "tool.call_timeout"    // Tool execution timed out
	ToolCallCancelled  LoopEventType = "tool.call_cancelled"  // Tool execution cancelled
	ToolResultSent     LoopEventType = "tool.result_sent"     // Tool result sent to LLM
	ToolAutoApproved   LoopEventType = "tool.auto_approved"   // Tool auto-approved by policy

	// Context events
	ContextInjected    LoopEventType = "context.injected"    // Auto-context injected
	ContextTruncated   LoopEventType = "context.truncated"   // Context was truncated
	ContextSummarized  LoopEventType = "context.summarized"  // Context was summarized
	ContextRefreshed   LoopEventType = "context.refreshed"   // Context was refreshed

	// Message events
	MessageUser      LoopEventType = "message.user"      // User message received
	MessageAssistant LoopEventType = "message.assistant" // Assistant response
	MessageSystem    LoopEventType = "message.system"    // System message injected

	// Quality gate events
	GateCheckStart LoopEventType = "gate.check_start" // Quality gate check begins
	GateCheckPass  LoopEventType = "gate.check_pass"  // Quality gate passed
	GateCheckFail  LoopEventType = "gate.check_fail"  // Quality gate failed
	GateFixStart   LoopEventType = "gate.fix_start"   // Auto-fix attempt begins
	GateFixSuccess LoopEventType = "gate.fix_success" // Auto-fix succeeded
	GateFixFail    LoopEventType = "gate.fix_fail"    // Auto-fix failed

	// Signal events (for orchestration)
	SignalReceived   LoopEventType = "signal.received"    // Signal received from agent
	SignalSent       LoopEventType = "signal.sent"        // Signal sent to orchestrator
	SignalPhaseComplete LoopEventType = "signal.phase_complete" // Phase completion signal

	// Cost tracking events
	CostUpdated     LoopEventType = "cost.updated"       // Cost tracking updated
	CostBudgetWarn  LoopEventType = "cost.budget_warn"   // Budget warning threshold
	CostBudgetExceeded LoopEventType = "cost.budget_exceeded" // Budget exceeded

	// Session events
	SessionCreated    LoopEventType = "session.created"    // New session created
	SessionRestored   LoopEventType = "session.restored"   // Session restored from storage
	SessionPersisted  LoopEventType = "session.persisted"  // Session state persisted
	SessionForked     LoopEventType = "session.forked"     // Session forked

	// Thrashing detection events
	ThrashingDetected LoopEventType = "thrashing.detected" // Loop thrashing detected
	ThrashingResolved LoopEventType = "thrashing.resolved" // Thrashing resolved
)

// ValidLoopEventTypes contains all valid loop event type values.
var ValidLoopEventTypes = []LoopEventType{
	// Lifecycle events
	LoopEventStart, LoopEventPause, LoopEventResume, LoopEventStop,
	LoopEventComplete, LoopEventError, LoopEventTimeout, LoopEventMaxReached,

	// Iteration events
	IterationStart, IterationComplete, IterationRetry, IterationSkip,

	// LLM interaction events
	LLMRequestStart, LLMRequestEnd, LLMStreamStart, LLMStreamChunk,
	LLMStreamEnd, LLMError, LLMRateLimit, LLMContextWindow,

	// Tool execution events
	ToolCallRequested, ToolCallQueued, ToolCallApproved, ToolCallRejected,
	ToolCallStart, ToolCallProgress, ToolCallComplete, ToolCallError,
	ToolCallTimeout, ToolCallCancelled, ToolResultSent, ToolAutoApproved,

	// Context events
	ContextInjected, ContextTruncated, ContextSummarized, ContextRefreshed,

	// Message events
	MessageUser, MessageAssistant, MessageSystem,

	// Quality gate events
	GateCheckStart, GateCheckPass, GateCheckFail,
	GateFixStart, GateFixSuccess, GateFixFail,

	// Signal events
	SignalReceived, SignalSent, SignalPhaseComplete,

	// Cost tracking events
	CostUpdated, CostBudgetWarn, CostBudgetExceeded,

	// Session events
	SessionCreated, SessionRestored, SessionPersisted, SessionForked,

	// Thrashing events
	ThrashingDetected, ThrashingResolved,
}

// IsValid checks if the loop event type is a valid value.
func (t LoopEventType) IsValid() bool {
	for _, valid := range ValidLoopEventTypes {
		if t == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the event type.
func (t LoopEventType) String() string {
	return string(t)
}

// Category returns the category of the event (loop, iteration, llm, tool, etc.).
func (t LoopEventType) Category() string {
	s := string(t)
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			return s[:i]
		}
	}
	return s
}

// IsTerminal returns true if this event type indicates loop termination.
func (t LoopEventType) IsTerminal() bool {
	switch t {
	case LoopEventComplete, LoopEventError, LoopEventTimeout, LoopEventMaxReached, LoopEventStop:
		return true
	default:
		return false
	}
}

// RequiresAction returns true if this event type typically requires user/system action.
func (t LoopEventType) RequiresAction() bool {
	switch t {
	case ToolCallQueued, GateCheckFail, CostBudgetExceeded, ThrashingDetected:
		return true
	default:
		return false
	}
}

// EventKind categorizes events for filtering and routing.
type EventKind string

const (
	EventKindLifecycle EventKind = "lifecycle" // Loop lifecycle events
	EventKindIteration EventKind = "iteration" // Iteration-level events
	EventKindLLM       EventKind = "llm"       // LLM interaction events
	EventKindTool      EventKind = "tool"      // Tool execution events
	EventKindContext   EventKind = "context"   // Context management events
	EventKindMessage   EventKind = "message"   // Message events
	EventKindGate      EventKind = "gate"      // Quality gate events
	EventKindSignal    EventKind = "signal"    // Orchestration signals
	EventKindCost      EventKind = "cost"      // Cost tracking events
	EventKindSession   EventKind = "session"   // Session events
	EventKindThrashing EventKind = "thrashing" // Thrashing detection events
)

// ValidEventKinds contains all valid event kind values.
var ValidEventKinds = []EventKind{
	EventKindLifecycle, EventKindIteration, EventKindLLM, EventKindTool,
	EventKindContext, EventKindMessage, EventKindGate, EventKindSignal,
	EventKindCost, EventKindSession, EventKindThrashing,
}

// IsValid checks if the event kind is a valid value.
func (k EventKind) IsValid() bool {
	for _, valid := range ValidEventKinds {
		if k == valid {
			return true
		}
	}
	return false
}

// String returns the string representation of the event kind.
func (k EventKind) String() string {
	return string(k)
}

// GetKind returns the EventKind for a LoopEventType.
func (t LoopEventType) GetKind() EventKind {
	category := t.Category()
	switch category {
	case "loop":
		return EventKindLifecycle
	case "iteration":
		return EventKindIteration
	case "llm":
		return EventKindLLM
	case "tool":
		return EventKindTool
	case "context":
		return EventKindContext
	case "message":
		return EventKindMessage
	case "gate":
		return EventKindGate
	case "signal":
		return EventKindSignal
	case "cost":
		return EventKindCost
	case "session":
		return EventKindSession
	case "thrashing":
		return EventKindThrashing
	default:
		return EventKind(category)
	}
}

// ToolCallStatus represents the current status of a tool call.
type ToolCallStatus string

const (
	ToolCallStatusPending      ToolCallStatus = "pending"       // Awaiting approval
	ToolCallStatusApproved     ToolCallStatus = "approved"      // Approved, not yet started
	ToolCallStatusRejected     ToolCallStatus = "rejected"      // Rejected by user/policy
	ToolCallStatusRunning      ToolCallStatus = "running"       // Currently executing
	ToolCallStatusCompleted    ToolCallStatus = "completed"     // Completed successfully
	ToolCallStatusFailed       ToolCallStatus = "failed"        // Execution failed
	ToolCallStatusTimeout      ToolCallStatus = "timeout"       // Execution timed out
	ToolCallStatusCancelled    ToolCallStatus = "cancelled"     // Cancelled by user/system
	ToolCallStatusAutoApproved ToolCallStatus = "auto_approved" // Auto-approved by policy
)

// ValidToolCallStatuses contains all valid tool call status values.
var ValidToolCallStatuses = []ToolCallStatus{
	ToolCallStatusPending, ToolCallStatusApproved, ToolCallStatusRejected,
	ToolCallStatusRunning, ToolCallStatusCompleted, ToolCallStatusFailed,
	ToolCallStatusTimeout, ToolCallStatusCancelled, ToolCallStatusAutoApproved,
}

// IsValid checks if the tool call status is a valid value.
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

// IsFinal returns true if the status is terminal (no further changes expected).
func (s ToolCallStatus) IsFinal() bool {
	switch s {
	case ToolCallStatusCompleted, ToolCallStatusFailed, ToolCallStatusTimeout,
		ToolCallStatusCancelled, ToolCallStatusRejected:
		return true
	default:
		return false
	}
}

// IsSuccess returns true if the status indicates successful completion.
func (s ToolCallStatus) IsSuccess() bool {
	return s == ToolCallStatusCompleted
}

// ToolCallInfo contains detailed information about a tool call.
type ToolCallInfo struct {
	// ID is the unique identifier for the tool call.
	ID string `json:"id"`

	// Name is the name of the tool being called.
	Name string `json:"name"`

	// Input is the input parameters for the tool (JSON-encoded).
	Input json.RawMessage `json:"input,omitempty"`

	// Output is the result from the tool execution.
	Output string `json:"output,omitempty"`

	// Status is the current status of the tool call.
	Status ToolCallStatus `json:"status"`

	// Error contains any error message from the tool execution.
	Error string `json:"error,omitempty"`

	// StartedAt is when the tool execution started.
	StartedAt *time.Time `json:"started_at,omitempty"`

	// CompletedAt is when the tool execution completed.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// DurationMs is the execution duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`

	// ApprovedBy identifies who approved the tool call.
	ApprovedBy string `json:"approved_by,omitempty"`

	// RejectedBy identifies who rejected the tool call.
	RejectedBy string `json:"rejected_by,omitempty"`

	// RejectionReason provides the reason for rejection.
	RejectionReason string `json:"rejection_reason,omitempty"`

	// RiskLevel is the assessed risk level of this tool call.
	RiskLevel string `json:"risk_level,omitempty"`

	// ProgressPercent is the execution progress (0-100).
	ProgressPercent int `json:"progress_percent,omitempty"`

	// ProgressMessage is a human-readable progress description.
	ProgressMessage string `json:"progress_message,omitempty"`
}

// NewToolCallInfo creates a new ToolCallInfo with the given ID and name.
func NewToolCallInfo(id, name string) *ToolCallInfo {
	if id == "" {
		id = uuid.New().String()
	}
	return &ToolCallInfo{
		ID:     id,
		Name:   name,
		Status: ToolCallStatusPending,
	}
}

// SetInput sets the input parameters for the tool call.
func (t *ToolCallInfo) SetInput(input interface{}) error {
	data, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("failed to marshal tool input: %w", err)
	}
	t.Input = data
	return nil
}

// GetInput unmarshals the input into the provided value.
func (t *ToolCallInfo) GetInput(v interface{}) error {
	if t.Input == nil {
		return nil
	}
	return json.Unmarshal(t.Input, v)
}

// LLMRequestInfo contains information about an LLM request.
type LLMRequestInfo struct {
	// Provider is the LLM provider name.
	Provider string `json:"provider"`

	// Model is the model identifier.
	Model string `json:"model"`

	// InputTokens is the number of input tokens.
	InputTokens int `json:"input_tokens"`

	// OutputTokens is the number of output tokens.
	OutputTokens int `json:"output_tokens"`

	// TotalTokens is the total token count.
	TotalTokens int `json:"total_tokens"`

	// CacheReadTokens is tokens read from cache.
	CacheReadTokens int `json:"cache_read_tokens,omitempty"`

	// CacheWriteTokens is tokens written to cache.
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`

	// CostUSD is the cost of this request in USD.
	CostUSD float64 `json:"cost_usd,omitempty"`

	// DurationMs is the request duration in milliseconds.
	DurationMs int64 `json:"duration_ms,omitempty"`

	// StopReason is why the model stopped generating.
	StopReason string `json:"stop_reason,omitempty"`

	// RequestID is the provider's request identifier.
	RequestID string `json:"request_id,omitempty"`
}

// CostInfo contains cost tracking information.
type CostInfo struct {
	// SessionTotal is the total cost for the current session.
	SessionTotal float64 `json:"session_total"`

	// IterationCost is the cost for the current iteration.
	IterationCost float64 `json:"iteration_cost"`

	// BudgetLimit is the configured budget limit.
	BudgetLimit float64 `json:"budget_limit,omitempty"`

	// BudgetRemaining is the remaining budget.
	BudgetRemaining float64 `json:"budget_remaining,omitempty"`

	// BudgetPercent is the percentage of budget used.
	BudgetPercent float64 `json:"budget_percent,omitempty"`

	// TotalTokensInput is the total input tokens used.
	TotalTokensInput int64 `json:"total_tokens_input"`

	// TotalTokensOutput is the total output tokens used.
	TotalTokensOutput int64 `json:"total_tokens_output"`
}

// ContextInfo contains context management information.
type ContextInfo struct {
	// TokenCount is the current context token count.
	TokenCount int `json:"token_count"`

	// MaxTokens is the maximum context tokens allowed.
	MaxTokens int `json:"max_tokens"`

	// UsagePercent is the percentage of context used.
	UsagePercent float64 `json:"usage_percent"`

	// WasTruncated indicates if the context was truncated.
	WasTruncated bool `json:"was_truncated,omitempty"`

	// TruncatedTokens is how many tokens were removed.
	TruncatedTokens int `json:"truncated_tokens,omitempty"`

	// WasSummarized indicates if the context was summarized.
	WasSummarized bool `json:"was_summarized,omitempty"`

	// SourceFiles lists the files included in context.
	SourceFiles []string `json:"source_files,omitempty"`
}

// GateInfo contains quality gate check information.
type GateInfo struct {
	// GateName is the name of the quality gate.
	GateName string `json:"gate_name"`

	// Passed indicates if the gate passed.
	Passed bool `json:"passed"`

	// Message provides details about the gate result.
	Message string `json:"message,omitempty"`

	// FixAttempt is the current fix attempt number.
	FixAttempt int `json:"fix_attempt,omitempty"`

	// MaxFixAttempts is the maximum fix attempts allowed.
	MaxFixAttempts int `json:"max_fix_attempts,omitempty"`

	// Details contains gate-specific result details.
	Details map[string]interface{} `json:"details,omitempty"`
}

// SignalInfo contains orchestration signal information.
type SignalInfo struct {
	// SignalType is the type of signal.
	SignalType string `json:"signal_type"`

	// Target is the target of the signal (for routing).
	Target string `json:"target,omitempty"`

	// Reason provides context for the signal.
	Reason string `json:"reason,omitempty"`

	// Metadata contains additional signal data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// LoopEvent represents a detailed event in the agent loop execution.
type LoopEvent struct {
	// ID is the unique identifier for this event.
	ID string `json:"id"`

	// Type is the event type.
	Type LoopEventType `json:"type"`

	// Kind is the event category.
	Kind EventKind `json:"kind"`

	// Timestamp is when the event occurred.
	Timestamp time.Time `json:"timestamp"`

	// SessionID is the session this event belongs to.
	SessionID string `json:"session_id,omitempty"`

	// Iteration is the loop iteration number.
	Iteration int `json:"iteration,omitempty"`

	// Message is a human-readable event description.
	Message string `json:"message,omitempty"`

	// Error contains any error information.
	Error string `json:"error,omitempty"`

	// ToolCall contains tool call information (for tool events).
	ToolCall *ToolCallInfo `json:"tool_call,omitempty"`

	// LLMRequest contains LLM request information (for LLM events).
	LLMRequest *LLMRequestInfo `json:"llm_request,omitempty"`

	// Cost contains cost information (for cost events).
	Cost *CostInfo `json:"cost,omitempty"`

	// Context contains context information (for context events).
	Context *ContextInfo `json:"context,omitempty"`

	// Gate contains quality gate information (for gate events).
	Gate *GateInfo `json:"gate,omitempty"`

	// Signal contains signal information (for signal events).
	Signal *SignalInfo `json:"signal,omitempty"`

	// Metadata contains additional event-specific data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// LoopEventBuilder provides a fluent interface for constructing loop events.
type LoopEventBuilder struct {
	event *LoopEvent
}

// NewLoopEvent creates a new loop event builder with required fields.
func NewLoopEvent(eventType LoopEventType) (*LoopEventBuilder, error) {
	if !eventType.IsValid() {
		return nil, fmt.Errorf("%w: %s", ErrInvalidEventType, eventType)
	}

	event := &LoopEvent{
		ID:        uuid.New().String(),
		Type:      eventType,
		Kind:      eventType.GetKind(),
		Timestamp: time.Now().UTC(),
	}

	return &LoopEventBuilder{event: event}, nil
}

// WithSession sets the session ID.
func (b *LoopEventBuilder) WithSession(sessionID string) *LoopEventBuilder {
	b.event.SessionID = sessionID
	return b
}

// WithIteration sets the iteration number.
func (b *LoopEventBuilder) WithIteration(iteration int) *LoopEventBuilder {
	b.event.Iteration = iteration
	return b
}

// WithMessage sets the event message.
func (b *LoopEventBuilder) WithMessage(message string) *LoopEventBuilder {
	b.event.Message = message
	return b
}

// WithError sets the error message.
func (b *LoopEventBuilder) WithError(err error) *LoopEventBuilder {
	if err != nil {
		b.event.Error = err.Error()
	}
	return b
}

// WithErrorString sets the error message from a string.
func (b *LoopEventBuilder) WithErrorString(errStr string) *LoopEventBuilder {
	b.event.Error = errStr
	return b
}

// WithToolCall sets the tool call information.
func (b *LoopEventBuilder) WithToolCall(toolCall *ToolCallInfo) *LoopEventBuilder {
	b.event.ToolCall = toolCall
	return b
}

// WithLLMRequest sets the LLM request information.
func (b *LoopEventBuilder) WithLLMRequest(llmRequest *LLMRequestInfo) *LoopEventBuilder {
	b.event.LLMRequest = llmRequest
	return b
}

// WithCost sets the cost information.
func (b *LoopEventBuilder) WithCost(cost *CostInfo) *LoopEventBuilder {
	b.event.Cost = cost
	return b
}

// WithContext sets the context information.
func (b *LoopEventBuilder) WithContext(ctx *ContextInfo) *LoopEventBuilder {
	b.event.Context = ctx
	return b
}

// WithGate sets the gate information.
func (b *LoopEventBuilder) WithGate(gate *GateInfo) *LoopEventBuilder {
	b.event.Gate = gate
	return b
}

// WithSignal sets the signal information.
func (b *LoopEventBuilder) WithSignal(signal *SignalInfo) *LoopEventBuilder {
	b.event.Signal = signal
	return b
}

// WithMetadata sets additional metadata.
func (b *LoopEventBuilder) WithMetadata(metadata map[string]interface{}) *LoopEventBuilder {
	b.event.Metadata = metadata
	return b
}

// WithTimestamp overrides the default timestamp.
func (b *LoopEventBuilder) WithTimestamp(timestamp time.Time) *LoopEventBuilder {
	b.event.Timestamp = timestamp
	return b
}

// Build validates and returns the loop event.
func (b *LoopEventBuilder) Build() (*LoopEvent, error) {
	if err := b.event.Validate(); err != nil {
		return nil, err
	}
	return b.event, nil
}

// Validate checks if the loop event has valid field values.
func (e *LoopEvent) Validate() error {
	if e.ID == "" {
		return fmt.Errorf("%w: id is required", ErrInvalidEvent)
	}
	if !e.Type.IsValid() {
		return fmt.Errorf("%w: invalid type: %s", ErrInvalidEvent, e.Type)
	}
	if !e.Kind.IsValid() {
		return fmt.Errorf("%w: invalid kind: %s", ErrInvalidEvent, e.Kind)
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("%w: timestamp is required", ErrInvalidEvent)
	}
	return nil
}

// IsTerminal returns true if this event indicates loop termination.
func (e *LoopEvent) IsTerminal() bool {
	return e.Type.IsTerminal()
}

// RequiresAction returns true if this event requires user/system action.
func (e *LoopEvent) RequiresAction() bool {
	return e.Type.RequiresAction()
}

// EventFilter allows filtering events by various criteria.
type EventFilter struct {
	// Types filters by event types.
	Types []LoopEventType

	// Kinds filters by event kinds.
	Kinds []EventKind

	// SessionID filters by session ID.
	SessionID string

	// Since filters to events after this time.
	Since time.Time

	// Until filters to events before this time.
	Until time.Time

	// IterationMin filters to events with iteration >= this value.
	IterationMin int

	// IterationMax filters to events with iteration <= this value.
	IterationMax int

	// IncludeErrors filters to include only error events.
	IncludeErrors bool

	// ExcludeTypes excludes specific event types.
	ExcludeTypes []LoopEventType
}

// Matches checks if an event matches the filter criteria.
func (f *EventFilter) Matches(event *LoopEvent) bool {
	// Check type filter
	if len(f.Types) > 0 {
		found := false
		for _, t := range f.Types {
			if event.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check excluded types
	for _, t := range f.ExcludeTypes {
		if event.Type == t {
			return false
		}
	}

	// Check kind filter
	if len(f.Kinds) > 0 {
		found := false
		for _, k := range f.Kinds {
			if event.Kind == k {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check session ID
	if f.SessionID != "" && event.SessionID != f.SessionID {
		return false
	}

	// Check time range
	if !f.Since.IsZero() && event.Timestamp.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && event.Timestamp.After(f.Until) {
		return false
	}

	// Check iteration range
	if f.IterationMin > 0 && event.Iteration < f.IterationMin {
		return false
	}
	if f.IterationMax > 0 && event.Iteration > f.IterationMax {
		return false
	}

	// Check error filter
	if f.IncludeErrors && event.Error == "" {
		return false
	}

	return true
}

// EventHandler is a function that handles a loop event.
type EventHandler func(*LoopEvent)

// EventDispatcher manages event subscriptions and dispatching.
type EventDispatcher struct {
	handlers map[EventKind][]EventHandler
	allHandlers []EventHandler
}

// NewEventDispatcher creates a new event dispatcher.
func NewEventDispatcher() *EventDispatcher {
	return &EventDispatcher{
		handlers:    make(map[EventKind][]EventHandler),
		allHandlers: make([]EventHandler, 0),
	}
}

// Subscribe adds a handler for events of a specific kind.
func (d *EventDispatcher) Subscribe(kind EventKind, handler EventHandler) {
	d.handlers[kind] = append(d.handlers[kind], handler)
}

// SubscribeAll adds a handler for all events.
func (d *EventDispatcher) SubscribeAll(handler EventHandler) {
	d.allHandlers = append(d.allHandlers, handler)
}

// Dispatch sends an event to all relevant handlers.
func (d *EventDispatcher) Dispatch(event *LoopEvent) {
	// Call kind-specific handlers
	if handlers, ok := d.handlers[event.Kind]; ok {
		for _, handler := range handlers {
			handler(event)
		}
	}

	// Call all-event handlers
	for _, handler := range d.allHandlers {
		handler(event)
	}
}
