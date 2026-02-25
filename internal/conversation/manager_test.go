package conversation

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// createTestFlow creates a simple test flow with three states.
func createTestFlow(id FlowID) *FlowConfig {
	return &FlowConfig{
		ID:         id,
		StartState: "start",
		States: map[StateID]*StateConfig{
			"start": {
				ID: "start",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return GoTo("middle")
				},
				AllowedTransitions: []StateID{"middle"},
			},
			"middle": {
				ID: "middle",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					if input.Text == "done" {
						return GoTo("end")
					}
					return Stay()
				},
				AllowedTransitions: []StateID{"end", "start"},
			},
			"end": {
				ID: "end",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return Done()
				},
			},
		},
		DefaultTimeoutSeconds: 300,
	}
}

func TestNewManager(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	if m == nil {
		t.Fatal("expected non-nil manager")
	}
	if m.GetActiveConversationCount() != 0 {
		t.Errorf("expected 0 active conversations, got %d", m.GetActiveConversationCount())
	}
}

func TestManager_WithOptions(t *testing.T) {
	m := NewManager(
		WithDefaultTimeout(10*time.Minute),
		WithCleanupInterval(30*time.Second),
	)
	defer m.Stop()

	if m.defaultTimeout != 10*time.Minute {
		t.Errorf("expected defaultTimeout 10m, got %v", m.defaultTimeout)
	}
	if m.cleanupInterval != 30*time.Second {
		t.Errorf("expected cleanupInterval 30s, got %v", m.cleanupInterval)
	}
}

func TestManager_RegisterFlow(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	err := m.RegisterFlow(flow)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify flow is registered
	retrieved, ok := m.GetFlow("test-flow")
	if !ok {
		t.Fatal("flow should be registered")
	}
	if retrieved.ID != flow.ID {
		t.Errorf("expected flow ID %s, got %s", flow.ID, retrieved.ID)
	}
}

func TestManager_RegisterFlow_Invalid(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	invalidFlow := &FlowConfig{
		ID: "invalid",
		// Missing StartState and States
	}
	err := m.RegisterFlow(invalidFlow)
	if err == nil {
		t.Error("expected error for invalid flow")
	}
}

func TestManager_StartConversation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	if err := m.RegisterFlow(flow); err != nil {
		t.Fatalf("failed to register flow: %v", err)
	}

	ctx := context.Background()
	conv, err := m.StartConversation(ctx, "test-flow", 12345, 67890)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if conv.FlowID != "test-flow" {
		t.Errorf("expected FlowID test-flow, got %s", conv.FlowID)
	}
	if conv.CurrentState != "start" {
		t.Errorf("expected CurrentState start, got %s", conv.CurrentState)
	}
	if conv.UserID != 12345 {
		t.Errorf("expected UserID 12345, got %d", conv.UserID)
	}
	if conv.ChatID != 67890 {
		t.Errorf("expected ChatID 67890, got %d", conv.ChatID)
	}

	if m.GetActiveConversationCount() != 1 {
		t.Errorf("expected 1 active conversation, got %d", m.GetActiveConversationCount())
	}
}

func TestManager_StartConversation_FlowNotFound(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	ctx := context.Background()
	_, err := m.StartConversation(ctx, "nonexistent", 1, 1)
	if !errors.Is(err, ErrFlowNotFound) {
		t.Errorf("expected ErrFlowNotFound, got %v", err)
	}
}

func TestManager_StartConversation_ReplacesExisting(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	if err := m.RegisterFlow(flow); err != nil {
		t.Fatalf("failed to register flow: %v", err)
	}

	ctx := context.Background()

	// Start first conversation
	conv1, _ := m.StartConversation(ctx, "test-flow", 12345, 67890)
	conv1ID := conv1.ID

	// Start second conversation for same user
	conv2, _ := m.StartConversation(ctx, "test-flow", 12345, 67890)

	// Old conversation should be removed
	_, err := m.GetConversation(conv1ID)
	if !errors.Is(err, ErrConversationNotFound) {
		t.Error("old conversation should be removed")
	}

	// New conversation should exist
	retrieved, err := m.GetConversation(conv2.ID)
	if err != nil {
		t.Errorf("new conversation should exist: %v", err)
	}
	if retrieved.ID != conv2.ID {
		t.Error("should be the new conversation")
	}

	if m.GetActiveConversationCount() != 1 {
		t.Errorf("expected 1 active conversation, got %d", m.GetActiveConversationCount())
	}
}

func TestManager_GetUserConversation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	conv, _ := m.StartConversation(ctx, "test-flow", 12345, 67890)

	// Get by user ID
	retrieved, err := m.GetUserConversation(12345)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if retrieved.ID != conv.ID {
		t.Errorf("expected conversation ID %s, got %s", conv.ID, retrieved.ID)
	}

	// Non-existent user
	_, err = m.GetUserConversation(99999)
	if !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("expected ErrConversationNotFound, got %v", err)
	}
}

func TestManager_HasActiveConversation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	if m.HasActiveConversation(12345) {
		t.Error("should not have active conversation before starting")
	}

	ctx := context.Background()
	m.StartConversation(ctx, "test-flow", 12345, 67890)

	if !m.HasActiveConversation(12345) {
		t.Error("should have active conversation after starting")
	}
}

func TestManager_ProcessInput(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "test-flow", 12345, 67890)

	// First input - should transition from start to middle
	result, err := m.ProcessInput(ctx, 12345, NewTextInput("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextState != "middle" {
		t.Errorf("expected NextState=middle, got %s", result.NextState)
	}

	// Verify state changed
	conv, _ := m.GetUserConversation(12345)
	if conv.CurrentState != "middle" {
		t.Errorf("expected CurrentState=middle, got %s", conv.CurrentState)
	}
}

func TestManager_ProcessInput_NoConversation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	ctx := context.Background()
	_, err := m.ProcessInput(ctx, 12345, NewTextInput("hello"))
	if !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("expected ErrConversationNotFound, got %v", err)
	}
}

func TestManager_ProcessInput_Completion(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "test-flow", 12345, 67890)

	// Transition to middle state
	m.ProcessInput(ctx, 12345, NewTextInput("hello"))

	// Now send "done" to transition to end and complete
	m.ProcessInput(ctx, 12345, NewTextInput("done"))

	// Process in end state - should complete
	result, err := m.ProcessInput(ctx, 12345, NewTextInput("final"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Complete {
		t.Error("expected Complete=true")
	}

	// Conversation should be removed
	if m.HasActiveConversation(12345) {
		t.Error("conversation should be removed after completion")
	}
}

func TestManager_ProcessInput_Cancellation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	cancelCalled := false
	flow := &FlowConfig{
		ID:         "cancel-flow",
		StartState: "start",
		States: map[StateID]*StateConfig{
			"start": {
				ID: "start",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					if input.Text == "cancel" {
						return Cancelled()
					}
					return Stay()
				},
			},
		},
		OnCancel: func(ctx context.Context, conv *Conversation) error {
			cancelCalled = true
			return nil
		},
	}
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "cancel-flow", 12345, 67890)

	result, err := m.ProcessInput(ctx, 12345, NewTextInput("cancel"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Cancel {
		t.Error("expected Cancel=true")
	}
	if !cancelCalled {
		t.Error("OnCancel should have been called")
	}

	// Conversation should be removed
	if m.HasActiveConversation(12345) {
		t.Error("conversation should be removed after cancellation")
	}
}

func TestManager_ProcessInput_InvalidTransition(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	// Create flow where start can only go to middle
	flow := &FlowConfig{
		ID:         "strict-flow",
		StartState: "start",
		States: map[StateID]*StateConfig{
			"start": {
				ID: "start",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					// Try to transition to end (not allowed)
					return GoTo("end")
				},
				AllowedTransitions: []StateID{"middle"},
			},
			"middle": {
				ID: "middle",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return Done()
				},
			},
			"end": {
				ID: "end",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return Done()
				},
			},
		},
	}
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "strict-flow", 12345, 67890)

	_, err := m.ProcessInput(ctx, 12345, NewTextInput("hello"))
	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("expected ErrInvalidTransition, got %v", err)
	}
}

func TestManager_CancelConversation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	conv, _ := m.StartConversation(ctx, "test-flow", 12345, 67890)

	err := m.CancelConversation(ctx, conv.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.HasActiveConversation(12345) {
		t.Error("conversation should be removed after cancellation")
	}
}

func TestManager_CancelUserConversation(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "test-flow", 12345, 67890)

	err := m.CancelUserConversation(ctx, 12345)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if m.HasActiveConversation(12345) {
		t.Error("conversation should be removed")
	}
}

func TestManager_CancelConversation_NotFound(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	ctx := context.Background()
	err := m.CancelConversation(ctx, "nonexistent")
	if !errors.Is(err, ErrConversationNotFound) {
		t.Errorf("expected ErrConversationNotFound, got %v", err)
	}
}

func TestManager_SetLastMessageID(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	conv, _ := m.StartConversation(ctx, "test-flow", 12345, 67890)

	err := m.SetLastMessageID(conv.ID, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updated, _ := m.GetConversation(conv.ID)
	if updated.LastMessageID != 999 {
		t.Errorf("expected LastMessageID=999, got %d", updated.LastMessageID)
	}
}

func TestManager_FlowCallbacks(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	var callOrder []string
	var mu sync.Mutex

	flow := &FlowConfig{
		ID:         "callback-flow",
		StartState: "start",
		States: map[StateID]*StateConfig{
			"start": {
				ID: "start",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return GoTo("end")
				},
				OnEnter: func(ctx context.Context, conv *Conversation) error {
					mu.Lock()
					callOrder = append(callOrder, "start_enter")
					mu.Unlock()
					return nil
				},
				OnExit: func(ctx context.Context, conv *Conversation) error {
					mu.Lock()
					callOrder = append(callOrder, "start_exit")
					mu.Unlock()
					return nil
				},
				AllowedTransitions: []StateID{"end"},
			},
			"end": {
				ID: "end",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return Done()
				},
				OnEnter: func(ctx context.Context, conv *Conversation) error {
					mu.Lock()
					callOrder = append(callOrder, "end_enter")
					mu.Unlock()
					return nil
				},
			},
		},
		OnStart: func(ctx context.Context, conv *Conversation) error {
			mu.Lock()
			callOrder = append(callOrder, "flow_start")
			mu.Unlock()
			return nil
		},
		OnComplete: func(ctx context.Context, conv *Conversation) error {
			mu.Lock()
			callOrder = append(callOrder, "flow_complete")
			mu.Unlock()
			return nil
		},
	}
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "callback-flow", 12345, 67890)

	// Verify start callbacks
	mu.Lock()
	if len(callOrder) != 2 || callOrder[0] != "flow_start" || callOrder[1] != "start_enter" {
		t.Errorf("unexpected callback order after start: %v", callOrder)
	}
	mu.Unlock()

	// Transition to end
	m.ProcessInput(ctx, 12345, NewTextInput("hello"))

	mu.Lock()
	if len(callOrder) != 4 || callOrder[2] != "start_exit" || callOrder[3] != "end_enter" {
		t.Errorf("unexpected callback order after transition: %v", callOrder)
	}
	mu.Unlock()

	// Complete
	m.ProcessInput(ctx, 12345, NewTextInput("bye"))

	mu.Lock()
	if len(callOrder) != 5 || callOrder[4] != "flow_complete" {
		t.Errorf("unexpected callback order after complete: %v", callOrder)
	}
	mu.Unlock()
}

func TestManager_ConversationData(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := &FlowConfig{
		ID:         "data-flow",
		StartState: "collect",
		States: map[StateID]*StateConfig{
			"collect": {
				ID: "collect",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					// Store input in conversation data
					conv.SetData("input", input.Text)
					return GoTo("confirm")
				},
				AllowedTransitions: []StateID{"confirm"},
			},
			"confirm": {
				ID: "confirm",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					// Retrieve stored data
					stored := conv.GetString("input")
					if input.Text == "yes" && stored != "" {
						return DoneWithMessage("Confirmed: " + stored)
					}
					return Stay()
				},
			},
		},
	}
	m.RegisterFlow(flow)

	ctx := context.Background()
	m.StartConversation(ctx, "data-flow", 12345, 67890)

	// Collect input
	m.ProcessInput(ctx, 12345, NewTextInput("test data"))

	// Verify data is stored
	conv, _ := m.GetUserConversation(12345)
	if conv.GetString("input") != "test data" {
		t.Errorf("expected 'test data', got '%s'", conv.GetString("input"))
	}

	// Confirm
	result, _ := m.ProcessInput(ctx, 12345, NewTextInput("yes"))
	if !result.Complete {
		t.Error("expected completion")
	}
	if result.Message != "Confirmed: test data" {
		t.Errorf("expected 'Confirmed: test data', got '%s'", result.Message)
	}
}

func TestManager_ExpiredConversation(t *testing.T) {
	m := NewManager(WithDefaultTimeout(1 * time.Millisecond))
	defer m.Stop()

	// Create flow without explicit timeout so manager's default is used
	flow := &FlowConfig{
		ID:         "expiry-test-flow",
		StartState: "start",
		States: map[StateID]*StateConfig{
			"start": {
				ID: "start",
				Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
					return Done()
				},
			},
		},
		// No DefaultTimeoutSeconds - will use manager's 1ms default
	}
	m.RegisterFlow(flow)

	ctx := context.Background()
	conv, _ := m.StartConversation(ctx, "expiry-test-flow", 12345, 67890)

	// Wait for expiry
	time.Sleep(5 * time.Millisecond)

	_, err := m.GetConversation(conv.ID)
	if !errors.Is(err, ErrConversationExpired) {
		t.Errorf("expected ErrConversationExpired, got %v", err)
	}
}

func TestManager_ConcurrentAccess(t *testing.T) {
	m := NewManager()
	defer m.Stop()

	flow := createTestFlow("test-flow")
	m.RegisterFlow(flow)

	ctx := context.Background()
	const numGoroutines = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(userID int64) {
			defer wg.Done()

			m.StartConversation(ctx, "test-flow", userID, userID)
			m.ProcessInput(ctx, userID, NewTextInput("hello"))
			m.GetUserConversation(userID)
			m.HasActiveConversation(userID)
			m.CancelUserConversation(ctx, userID)
		}(int64(i))
	}

	wg.Wait()

	if m.GetActiveConversationCount() != 0 {
		t.Errorf("expected 0 active conversations, got %d", m.GetActiveConversationCount())
	}
}
