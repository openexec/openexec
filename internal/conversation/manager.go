package conversation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager manages conversation flows and active conversations.
type Manager struct {
	// flows stores registered conversation flows by ID.
	flows map[FlowID]*FlowConfig
	// conversations stores active conversations by ID.
	conversations map[ConversationID]*Conversation
	// userConversations maps user IDs to their active conversation IDs.
	userConversations map[int64]ConversationID
	// defaultTimeout is the default conversation timeout.
	defaultTimeout time.Duration
	// cleanupInterval is how often expired conversations are cleaned up.
	cleanupInterval time.Duration
	// stopCleanup is used to stop the cleanup goroutine.
	stopCleanup chan struct{}

	mu sync.RWMutex
}

// ManagerOption is a functional option for configuring the Manager.
type ManagerOption func(*Manager)

// WithDefaultTimeout sets the default conversation timeout.
func WithDefaultTimeout(d time.Duration) ManagerOption {
	return func(m *Manager) {
		m.defaultTimeout = d
	}
}

// WithCleanupInterval sets the cleanup interval for expired conversations.
func WithCleanupInterval(d time.Duration) ManagerOption {
	return func(m *Manager) {
		m.cleanupInterval = d
	}
}

// NewManager creates a new conversation manager.
func NewManager(opts ...ManagerOption) *Manager {
	m := &Manager{
		flows:             make(map[FlowID]*FlowConfig),
		conversations:     make(map[ConversationID]*Conversation),
		userConversations: make(map[int64]ConversationID),
		defaultTimeout:    5 * time.Minute,
		cleanupInterval:   1 * time.Minute,
		stopCleanup:       make(chan struct{}),
	}

	for _, opt := range opts {
		opt(m)
	}

	// Start background cleanup goroutine
	go m.cleanupExpired()

	return m
}

// RegisterFlow registers a conversation flow configuration.
func (m *Manager) RegisterFlow(flow *FlowConfig) error {
	if err := flow.Validate(); err != nil {
		return fmt.Errorf("invalid flow configuration: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.flows[flow.ID] = flow
	return nil
}

// GetFlow returns a registered flow by ID.
func (m *Manager) GetFlow(id FlowID) (*FlowConfig, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	flow, ok := m.flows[id]
	return flow, ok
}

// StartConversation starts a new conversation for a user.
// If the user already has an active conversation, it is cancelled first.
func (m *Manager) StartConversation(ctx context.Context, flowID FlowID, userID, chatID int64) (*Conversation, error) {
	m.mu.Lock()

	// Get the flow configuration
	flow, ok := m.flows[flowID]
	if !ok {
		m.mu.Unlock()
		return nil, ErrFlowNotFound
	}

	// Cancel any existing conversation for this user
	if existingID, hasExisting := m.userConversations[userID]; hasExisting {
		if existing, ok := m.conversations[existingID]; ok {
			// Call OnCancel if defined
			if flow.OnCancel != nil {
				m.mu.Unlock()
				_ = flow.OnCancel(ctx, existing)
				m.mu.Lock()
			}
			delete(m.conversations, existingID)
		}
	}

	// Generate a new conversation ID
	convID := ConversationID(uuid.New().String())

	// Calculate expiry time
	timeout := m.defaultTimeout
	if flow.DefaultTimeoutSeconds > 0 {
		timeout = time.Duration(flow.DefaultTimeoutSeconds) * time.Second
	}
	expiresAt := time.Now().Add(timeout)

	// Create the conversation
	conv := NewConversation(convID, flowID, flow.StartState, userID, chatID, expiresAt)

	// Store the conversation
	m.conversations[convID] = conv
	m.userConversations[userID] = convID

	m.mu.Unlock()

	// Call OnStart if defined
	if flow.OnStart != nil {
		if err := flow.OnStart(ctx, conv); err != nil {
			m.mu.Lock()
			delete(m.conversations, convID)
			delete(m.userConversations, userID)
			m.mu.Unlock()
			return nil, fmt.Errorf("flow OnStart failed: %w", err)
		}
	}

	// Call OnEnter for the start state if defined
	startState := flow.States[flow.StartState]
	if startState.OnEnter != nil {
		if err := startState.OnEnter(ctx, conv); err != nil {
			m.mu.Lock()
			delete(m.conversations, convID)
			delete(m.userConversations, userID)
			m.mu.Unlock()
			return nil, fmt.Errorf("start state OnEnter failed: %w", err)
		}
	}

	return conv, nil
}

// GetConversation returns an active conversation by ID.
func (m *Manager) GetConversation(id ConversationID) (*Conversation, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	conv, ok := m.conversations[id]
	if !ok {
		return nil, ErrConversationNotFound
	}

	if conv.IsExpired() {
		return nil, ErrConversationExpired
	}

	return conv, nil
}

// GetUserConversation returns the active conversation for a user.
func (m *Manager) GetUserConversation(userID int64) (*Conversation, error) {
	m.mu.RLock()
	convID, ok := m.userConversations[userID]
	m.mu.RUnlock()

	if !ok {
		return nil, ErrConversationNotFound
	}

	return m.GetConversation(convID)
}

// HasActiveConversation checks if a user has an active conversation.
func (m *Manager) HasActiveConversation(userID int64) bool {
	conv, err := m.GetUserConversation(userID)
	return err == nil && conv != nil
}

// ProcessInput handles user input for their active conversation.
func (m *Manager) ProcessInput(ctx context.Context, userID int64, input *Input) (*TransitionResult, error) {
	// Get the user's active conversation
	conv, err := m.GetUserConversation(userID)
	if err != nil {
		return nil, err
	}

	// Get the flow configuration
	m.mu.RLock()
	flow, ok := m.flows[conv.FlowID]
	m.mu.RUnlock()

	if !ok {
		return nil, ErrFlowNotFound
	}

	// Get the current state configuration
	currentState, ok := flow.States[conv.CurrentState]
	if !ok {
		return nil, ErrStateNotFound
	}

	// Execute the state handler
	result := currentState.Handler(ctx, conv, input)
	if result == nil {
		result = Stay()
	}

	// Handle errors
	if result.Error != nil {
		return result, fmt.Errorf("%w: %v", ErrHandlerError, result.Error)
	}

	// Handle cancellation
	if result.Cancel {
		if err := m.CancelConversation(ctx, conv.ID); err != nil {
			return result, err
		}
		return result, nil
	}

	// Handle completion
	if result.Complete {
		if err := m.completeConversation(ctx, conv, flow); err != nil {
			return result, err
		}
		return result, nil
	}

	// Handle state transition
	if result.NextState != "" && result.NextState != conv.CurrentState {
		if err := m.transitionState(ctx, conv, flow, currentState, result.NextState); err != nil {
			return nil, err
		}
	}

	// Update expiry based on state timeout
	newState, ok := flow.States[conv.CurrentState]
	if ok {
		timeout := m.defaultTimeout
		if newState.TimeoutSeconds > 0 {
			timeout = time.Duration(newState.TimeoutSeconds) * time.Second
		} else if flow.DefaultTimeoutSeconds > 0 {
			timeout = time.Duration(flow.DefaultTimeoutSeconds) * time.Second
		}
		conv.updateExpiry(time.Now().Add(timeout))
	}

	return result, nil
}

// transitionState handles the transition between states.
func (m *Manager) transitionState(ctx context.Context, conv *Conversation, flow *FlowConfig, currentState *StateConfig, nextState StateID) error {
	// Validate the transition is allowed
	if len(currentState.AllowedTransitions) > 0 {
		allowed := false
		for _, t := range currentState.AllowedTransitions {
			if t == nextState {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, conv.CurrentState, nextState)
		}
	}

	// Get the target state config
	targetState, ok := flow.States[nextState]
	if !ok {
		return fmt.Errorf("%w: %s", ErrStateNotFound, nextState)
	}

	// Call OnExit for current state
	if currentState.OnExit != nil {
		if err := currentState.OnExit(ctx, conv); err != nil {
			return fmt.Errorf("state OnExit failed: %w", err)
		}
	}

	// Transition to the new state
	conv.transitionTo(nextState)

	// Call OnEnter for new state
	if targetState.OnEnter != nil {
		if err := targetState.OnEnter(ctx, conv); err != nil {
			return fmt.Errorf("state OnEnter failed: %w", err)
		}
	}

	return nil
}

// completeConversation handles conversation completion.
func (m *Manager) completeConversation(ctx context.Context, conv *Conversation, flow *FlowConfig) error {
	// Call OnComplete if defined
	if flow.OnComplete != nil {
		if err := flow.OnComplete(ctx, conv); err != nil {
			return fmt.Errorf("flow OnComplete failed: %w", err)
		}
	}

	// Remove the conversation
	m.mu.Lock()
	delete(m.conversations, conv.ID)
	delete(m.userConversations, conv.UserID)
	m.mu.Unlock()

	return nil
}

// CancelConversation cancels an active conversation.
func (m *Manager) CancelConversation(ctx context.Context, id ConversationID) error {
	m.mu.Lock()
	conv, ok := m.conversations[id]
	if !ok {
		m.mu.Unlock()
		return ErrConversationNotFound
	}

	flow, flowOk := m.flows[conv.FlowID]
	m.mu.Unlock()

	// Call OnCancel if defined
	if flowOk && flow.OnCancel != nil {
		if err := flow.OnCancel(ctx, conv); err != nil {
			return fmt.Errorf("flow OnCancel failed: %w", err)
		}
	}

	// Remove the conversation
	m.mu.Lock()
	delete(m.conversations, id)
	delete(m.userConversations, conv.UserID)
	m.mu.Unlock()

	return nil
}

// CancelUserConversation cancels the active conversation for a user.
func (m *Manager) CancelUserConversation(ctx context.Context, userID int64) error {
	m.mu.RLock()
	convID, ok := m.userConversations[userID]
	m.mu.RUnlock()

	if !ok {
		return ErrConversationNotFound
	}

	return m.CancelConversation(ctx, convID)
}

// GetActiveConversationCount returns the number of active conversations.
func (m *Manager) GetActiveConversationCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.conversations)
}

// cleanupExpired periodically removes expired conversations.
func (m *Manager) cleanupExpired() {
	ticker := time.NewTicker(m.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.doCleanup()
		case <-m.stopCleanup:
			return
		}
	}
}

// doCleanup removes expired conversations.
func (m *Manager) doCleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for id, conv := range m.conversations {
		if now.After(conv.ExpiresAt) {
			// Call OnTimeout if the flow is available
			if flow, ok := m.flows[conv.FlowID]; ok && flow.OnTimeout != nil {
				// We need to release the lock to call the callback
				m.mu.Unlock()
				_ = flow.OnTimeout(context.Background(), conv)
				m.mu.Lock()
			}
			delete(m.conversations, id)
			delete(m.userConversations, conv.UserID)
		}
	}
}

// Stop stops the manager and cleans up resources.
func (m *Manager) Stop() {
	close(m.stopCleanup)
}

// SetLastMessageID sets the last message ID for a conversation.
func (m *Manager) SetLastMessageID(convID ConversationID, msgID int) error {
	conv, err := m.GetConversation(convID)
	if err != nil {
		return err
	}
	conv.setLastMessageID(msgID)
	return nil
}
