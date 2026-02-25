package conversation

import (
	"context"
	"testing"
	"time"
)

func TestNewConversation(t *testing.T) {
	conv := NewConversation(
		"conv-1",
		"flow-1",
		"start",
		12345,
		67890,
		time.Now().Add(5*time.Minute),
	)

	if conv.ID != "conv-1" {
		t.Errorf("expected ID conv-1, got %s", conv.ID)
	}
	if conv.FlowID != "flow-1" {
		t.Errorf("expected FlowID flow-1, got %s", conv.FlowID)
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
	if len(conv.StateHistory) != 1 || conv.StateHistory[0] != "start" {
		t.Errorf("expected StateHistory [start], got %v", conv.StateHistory)
	}
	if conv.Data == nil {
		t.Error("expected Data to be initialized")
	}
}

func TestConversation_IsExpired(t *testing.T) {
	// Non-expired conversation
	conv := NewConversation("conv-1", "flow-1", "start", 1, 1, time.Now().Add(5*time.Minute))
	if conv.IsExpired() {
		t.Error("conversation should not be expired")
	}

	// Expired conversation
	conv2 := NewConversation("conv-2", "flow-1", "start", 1, 1, time.Now().Add(-1*time.Minute))
	if !conv2.IsExpired() {
		t.Error("conversation should be expired")
	}
}

func TestConversation_Data(t *testing.T) {
	conv := NewConversation("conv-1", "flow-1", "start", 1, 1, time.Now().Add(5*time.Minute))

	// Test SetData and GetData
	conv.SetData("key1", "value1")
	conv.SetData("key2", 42)
	conv.SetData("key3", true)

	val1, ok := conv.GetData("key1")
	if !ok || val1 != "value1" {
		t.Errorf("expected value1, got %v (ok=%v)", val1, ok)
	}

	// Test GetString
	if conv.GetString("key1") != "value1" {
		t.Errorf("expected value1, got %s", conv.GetString("key1"))
	}
	if conv.GetString("key2") != "" {
		t.Errorf("expected empty string for non-string value, got %s", conv.GetString("key2"))
	}
	if conv.GetString("nonexistent") != "" {
		t.Errorf("expected empty string for nonexistent key, got %s", conv.GetString("nonexistent"))
	}

	// Test GetInt
	if conv.GetInt("key2") != 42 {
		t.Errorf("expected 42, got %d", conv.GetInt("key2"))
	}
	if conv.GetInt("key1") != 0 {
		t.Errorf("expected 0 for non-int value, got %d", conv.GetInt("key1"))
	}

	// Test GetBool
	if !conv.GetBool("key3") {
		t.Error("expected true, got false")
	}
	if conv.GetBool("key1") {
		t.Error("expected false for non-bool value")
	}

	// Test missing key
	_, ok = conv.GetData("nonexistent")
	if ok {
		t.Error("expected ok=false for nonexistent key")
	}
}

func TestInput_Constructors(t *testing.T) {
	// Test NewTextInput
	textInput := NewTextInput("hello")
	if textInput.Type != InputTypeText {
		t.Errorf("expected InputTypeText, got %s", textInput.Type)
	}
	if textInput.Text != "hello" {
		t.Errorf("expected hello, got %s", textInput.Text)
	}
	if textInput.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	// Test NewCallbackInput
	callbackInput := NewCallbackInput("data123")
	if callbackInput.Type != InputTypeCallback {
		t.Errorf("expected InputTypeCallback, got %s", callbackInput.Type)
	}
	if callbackInput.CallbackData != "data123" {
		t.Errorf("expected data123, got %s", callbackInput.CallbackData)
	}

	// Test NewCommandInput
	commandInput := NewCommandInput("/cancel")
	if commandInput.Type != InputTypeCommand {
		t.Errorf("expected InputTypeCommand, got %s", commandInput.Type)
	}
	if commandInput.Text != "/cancel" {
		t.Errorf("expected /cancel, got %s", commandInput.Text)
	}
}

func TestTransitionResult_Constructors(t *testing.T) {
	// Test Stay
	result := Stay()
	if result.NextState != "" || result.Complete || result.Cancel || result.Error != nil {
		t.Error("Stay() should return empty result")
	}

	// Test StayWithMessage
	result = StayWithMessage("hello")
	if result.Message != "hello" || result.NextState != "" {
		t.Error("StayWithMessage should only set message")
	}

	// Test GoTo
	result = GoTo("next")
	if result.NextState != "next" {
		t.Errorf("expected NextState=next, got %s", result.NextState)
	}

	// Test GoToWithMessage
	result = GoToWithMessage("next", "transitioning")
	if result.NextState != "next" || result.Message != "transitioning" {
		t.Error("GoToWithMessage should set both")
	}

	// Test Done
	result = Done()
	if !result.Complete {
		t.Error("Done() should set Complete=true")
	}

	// Test DoneWithMessage
	result = DoneWithMessage("goodbye")
	if !result.Complete || result.Message != "goodbye" {
		t.Error("DoneWithMessage should set Complete=true and message")
	}

	// Test Cancelled
	result = Cancelled()
	if !result.Cancel {
		t.Error("Cancelled() should set Cancel=true")
	}

	// Test WithError
	testErr := ErrStateNotFound
	result = WithError(testErr)
	if result.Error != testErr {
		t.Error("WithError should set Error")
	}

	// Test WithData
	result = GoTo("next").WithData(map[string]interface{}{"key": "value"})
	if result.Data["key"] != "value" {
		t.Error("WithData should set Data")
	}
}

func TestFlowConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		flow    *FlowConfig
		wantErr bool
		errStr  string
	}{
		{
			name: "valid flow",
			flow: &FlowConfig{
				ID:         "test",
				StartState: "start",
				States: map[StateID]*StateConfig{
					"start": {
						ID:      "start",
						Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult { return Done() },
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			flow: &FlowConfig{
				StartState: "start",
				States: map[StateID]*StateConfig{
					"start": {Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult { return Done() }},
				},
			},
			wantErr: true,
			errStr:  "flow ID is required",
		},
		{
			name: "missing start state",
			flow: &FlowConfig{
				ID: "test",
				States: map[StateID]*StateConfig{
					"start": {Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult { return Done() }},
				},
			},
			wantErr: true,
		},
		{
			name: "start state not found",
			flow: &FlowConfig{
				ID:         "test",
				StartState: "nonexistent",
				States: map[StateID]*StateConfig{
					"start": {Handler: func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult { return Done() }},
				},
			},
			wantErr: true,
		},
		{
			name: "no states",
			flow: &FlowConfig{
				ID:         "test",
				StartState: "start",
				States:     map[StateID]*StateConfig{},
			},
			wantErr: true,
		},
		{
			name: "state missing handler",
			flow: &FlowConfig{
				ID:         "test",
				StartState: "start",
				States: map[StateID]*StateConfig{
					"start": {ID: "start"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid transition target",
			flow: &FlowConfig{
				ID:         "test",
				StartState: "start",
				States: map[StateID]*StateConfig{
					"start": {
						ID:                 "start",
						Handler:            func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult { return Done() },
						AllowedTransitions: []StateID{"nonexistent"},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.flow.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestConversation_TransitionTo(t *testing.T) {
	conv := NewConversation("conv-1", "flow-1", "start", 1, 1, time.Now().Add(5*time.Minute))

	originalUpdateTime := conv.UpdatedAt
	time.Sleep(1 * time.Millisecond) // Ensure time has passed

	conv.transitionTo("next")

	if conv.CurrentState != "next" {
		t.Errorf("expected CurrentState=next, got %s", conv.CurrentState)
	}
	if len(conv.StateHistory) != 2 {
		t.Errorf("expected StateHistory length 2, got %d", len(conv.StateHistory))
	}
	if conv.StateHistory[1] != "next" {
		t.Errorf("expected StateHistory[1]=next, got %s", conv.StateHistory[1])
	}
	if !conv.UpdatedAt.After(originalUpdateTime) {
		t.Error("expected UpdatedAt to be updated")
	}
}

func TestConversation_UpdateExpiry(t *testing.T) {
	conv := NewConversation("conv-1", "flow-1", "start", 1, 1, time.Now().Add(5*time.Minute))

	newExpiry := time.Now().Add(10 * time.Minute)
	conv.updateExpiry(newExpiry)

	if !conv.ExpiresAt.Equal(newExpiry) {
		t.Errorf("expected ExpiresAt to be updated")
	}
}

func TestConversation_SetLastMessageID(t *testing.T) {
	conv := NewConversation("conv-1", "flow-1", "start", 1, 1, time.Now().Add(5*time.Minute))

	conv.setLastMessageID(123)

	if conv.LastMessageID != 123 {
		t.Errorf("expected LastMessageID=123, got %d", conv.LastMessageID)
	}
}
