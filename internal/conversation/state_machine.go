// Package conversation provides a generic state machine for handling multi-step user interactions.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Errors for state machine operations.
var (
	ErrConversationNotFound  = errors.New("conversation not found")
	ErrConversationExpired   = errors.New("conversation expired")
	ErrInvalidTransition     = errors.New("invalid state transition")
	ErrStateNotFound         = errors.New("state not found")
	ErrNoStartState          = errors.New("no start state defined")
	ErrFlowNotFound          = errors.New("conversation flow not found")
	ErrHandlerError          = errors.New("state handler error")
	ErrConversationCancelled = errors.New("conversation cancelled")
)

// StateID is a unique identifier for a state within a conversation flow.
type StateID string

// FlowID is a unique identifier for a conversation flow type.
type FlowID string

// ConversationID is a unique identifier for an active conversation instance.
type ConversationID string

// InputType represents the type of input received.
type InputType string

const (
	// InputTypeText represents a text message input.
	InputTypeText InputType = "text"
	// InputTypeCallback represents a callback button press.
	InputTypeCallback InputType = "callback"
	// InputTypeCommand represents a command input (e.g., /cancel).
	InputTypeCommand InputType = "command"
)

// Input represents user input to the state machine.
type Input struct {
	// Type is the type of input (text, callback, command).
	Type InputType
	// Text is the raw text content of the input.
	Text string
	// CallbackData contains parsed callback data for callback inputs.
	CallbackData string
	// MessageID is the ID of the message that triggered this input.
	MessageID int
	// Timestamp is when the input was received.
	Timestamp time.Time
}

// NewTextInput creates a new text input.
func NewTextInput(text string) *Input {
	return &Input{
		Type:      InputTypeText,
		Text:      text,
		Timestamp: time.Now(),
	}
}

// NewCallbackInput creates a new callback input.
func NewCallbackInput(data string) *Input {
	return &Input{
		Type:         InputTypeCallback,
		CallbackData: data,
		Timestamp:    time.Now(),
	}
}

// NewCommandInput creates a new command input.
func NewCommandInput(command string) *Input {
	return &Input{
		Type:      InputTypeCommand,
		Text:      command,
		Timestamp: time.Now(),
	}
}

// TransitionResult represents the result of a state handler execution.
type TransitionResult struct {
	// NextState is the state to transition to. Empty means stay in current state.
	NextState StateID
	// Message is an optional message to send to the user.
	Message string
	// Data contains any state-specific data to pass to the response renderer.
	Data map[string]interface{}
	// Complete indicates whether the conversation is complete.
	Complete bool
	// Error indicates a handler error occurred.
	Error error
	// Cancel indicates the conversation should be cancelled.
	Cancel bool
}

// Stay returns a result that stays in the current state.
func Stay() *TransitionResult {
	return &TransitionResult{}
}

// StayWithMessage returns a result that stays in the current state with a message.
func StayWithMessage(msg string) *TransitionResult {
	return &TransitionResult{Message: msg}
}

// GoTo returns a result that transitions to another state.
func GoTo(state StateID) *TransitionResult {
	return &TransitionResult{NextState: state}
}

// GoToWithMessage returns a result that transitions to another state with a message.
func GoToWithMessage(state StateID, msg string) *TransitionResult {
	return &TransitionResult{NextState: state, Message: msg}
}

// Done returns a result indicating the conversation is complete.
func Done() *TransitionResult {
	return &TransitionResult{Complete: true}
}

// DoneWithMessage returns a result indicating completion with a final message.
func DoneWithMessage(msg string) *TransitionResult {
	return &TransitionResult{Complete: true, Message: msg}
}

// Cancelled returns a result indicating the conversation was cancelled.
func Cancelled() *TransitionResult {
	return &TransitionResult{Cancel: true}
}

// WithError returns a result with an error.
func WithError(err error) *TransitionResult {
	return &TransitionResult{Error: err}
}

// WithData adds data to a transition result and returns it.
func (r *TransitionResult) WithData(data map[string]interface{}) *TransitionResult {
	r.Data = data
	return r
}

// StateHandler is a function that handles input for a particular state.
// It receives the conversation context, the current input, and returns a transition result.
type StateHandler func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult

// StateConfig defines the configuration for a single state.
type StateConfig struct {
	// ID is the unique identifier for this state.
	ID StateID
	// Handler is the function that processes input for this state.
	Handler StateHandler
	// AllowedTransitions lists the states this state can transition to.
	// If empty, all transitions are allowed.
	AllowedTransitions []StateID
	// TimeoutSeconds specifies a state-specific timeout. 0 uses the flow default.
	TimeoutSeconds int
	// OnEnter is called when entering this state (optional).
	OnEnter func(ctx context.Context, conv *Conversation) error
	// OnExit is called when exiting this state (optional).
	OnExit func(ctx context.Context, conv *Conversation) error
}

// FlowConfig defines a conversation flow with multiple states.
type FlowConfig struct {
	// ID is the unique identifier for this flow.
	ID FlowID
	// StartState is the initial state when a conversation begins.
	StartState StateID
	// States maps state IDs to their configurations.
	States map[StateID]*StateConfig
	// DefaultTimeoutSeconds is the default timeout for states without explicit timeouts.
	DefaultTimeoutSeconds int
	// OnStart is called when a conversation of this flow starts (optional).
	OnStart func(ctx context.Context, conv *Conversation) error
	// OnComplete is called when a conversation completes (optional).
	OnComplete func(ctx context.Context, conv *Conversation) error
	// OnCancel is called when a conversation is cancelled (optional).
	OnCancel func(ctx context.Context, conv *Conversation) error
	// OnTimeout is called when a conversation times out (optional).
	OnTimeout func(ctx context.Context, conv *Conversation) error
}

// Validate checks if the flow configuration is valid.
func (f *FlowConfig) Validate() error {
	if f.ID == "" {
		return errors.New("flow ID is required")
	}
	if f.StartState == "" {
		return ErrNoStartState
	}
	if len(f.States) == 0 {
		return errors.New("at least one state is required")
	}
	if _, ok := f.States[f.StartState]; !ok {
		return fmt.Errorf("start state %q not found in states", f.StartState)
	}
	for stateID, state := range f.States {
		if state.ID == "" {
			state.ID = stateID
		}
		if state.Handler == nil {
			return fmt.Errorf("state %q has no handler", stateID)
		}
		// Validate allowed transitions reference existing states
		for _, target := range state.AllowedTransitions {
			if _, ok := f.States[target]; !ok {
				return fmt.Errorf("state %q has invalid transition target %q", stateID, target)
			}
		}
	}
	return nil
}

// Conversation represents an active conversation instance.
type Conversation struct {
	// ID is the unique identifier for this conversation.
	ID ConversationID
	// FlowID is the identifier of the flow this conversation follows.
	FlowID FlowID
	// CurrentState is the current state of the conversation.
	CurrentState StateID
	// UserID identifies the user participating in the conversation.
	UserID int64
	// ChatID is the chat where the conversation is happening.
	ChatID int64
	// Data holds arbitrary conversation data that persists across states.
	Data map[string]interface{}
	// CreatedAt is when the conversation was started.
	CreatedAt time.Time
	// UpdatedAt is when the conversation was last updated.
	UpdatedAt time.Time
	// ExpiresAt is when the conversation will expire.
	ExpiresAt time.Time
	// StateHistory tracks the states this conversation has been through.
	StateHistory []StateID
	// LastMessageID is the ID of the last message sent in this conversation.
	LastMessageID int

	mu sync.RWMutex
}

// NewConversation creates a new conversation instance.
func NewConversation(id ConversationID, flowID FlowID, startState StateID, userID, chatID int64, expiresAt time.Time) *Conversation {
	now := time.Now()
	return &Conversation{
		ID:           id,
		FlowID:       flowID,
		CurrentState: startState,
		UserID:       userID,
		ChatID:       chatID,
		Data:         make(map[string]interface{}),
		CreatedAt:    now,
		UpdatedAt:    now,
		ExpiresAt:    expiresAt,
		StateHistory: []StateID{startState},
	}
}

// IsExpired checks if the conversation has expired.
func (c *Conversation) IsExpired() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().After(c.ExpiresAt)
}

// SetData sets a value in the conversation data store.
func (c *Conversation) SetData(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Data[key] = value
	c.UpdatedAt = time.Now()
}

// GetData retrieves a value from the conversation data store.
func (c *Conversation) GetData(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.Data[key]
	return val, ok
}

// GetString retrieves a string value from the conversation data store.
func (c *Conversation) GetString(key string) string {
	val, ok := c.GetData(key)
	if !ok {
		return ""
	}
	s, ok := val.(string)
	if !ok {
		return ""
	}
	return s
}

// GetInt retrieves an int value from the conversation data store.
func (c *Conversation) GetInt(key string) int {
	val, ok := c.GetData(key)
	if !ok {
		return 0
	}
	i, ok := val.(int)
	if !ok {
		return 0
	}
	return i
}

// GetBool retrieves a bool value from the conversation data store.
func (c *Conversation) GetBool(key string) bool {
	val, ok := c.GetData(key)
	if !ok {
		return false
	}
	b, ok := val.(bool)
	if !ok {
		return false
	}
	return b
}

// transitionTo updates the conversation state.
func (c *Conversation) transitionTo(newState StateID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.CurrentState = newState
	c.StateHistory = append(c.StateHistory, newState)
	c.UpdatedAt = time.Now()
}

// updateExpiry updates the conversation expiry time.
func (c *Conversation) updateExpiry(expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ExpiresAt = expiresAt
	c.UpdatedAt = time.Now()
}

// setLastMessageID sets the last message ID.
func (c *Conversation) setLastMessageID(msgID int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastMessageID = msgID
	c.UpdatedAt = time.Now()
}
