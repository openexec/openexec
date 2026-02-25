package conversation

import (
	"context"
	"testing"
	"time"
)

func TestFlowBuilder_Basic(t *testing.T) {
	flow, err := NewFlow("test-flow").
		StartState("start").
		DefaultTimeout(300).
		AddState("start", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return Done()
		}).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if flow.ID != "test-flow" {
		t.Errorf("expected ID test-flow, got %s", flow.ID)
	}
	if flow.StartState != "start" {
		t.Errorf("expected StartState start, got %s", flow.StartState)
	}
	if flow.DefaultTimeoutSeconds != 300 {
		t.Errorf("expected DefaultTimeoutSeconds 300, got %d", flow.DefaultTimeoutSeconds)
	}
	if len(flow.States) != 1 {
		t.Errorf("expected 1 state, got %d", len(flow.States))
	}
}

func TestFlowBuilder_WithCallbacks(t *testing.T) {
	startCalled := false
	completeCalled := false
	cancelCalled := false
	timeoutCalled := false

	flow, err := NewFlow("callback-flow").
		StartState("start").
		OnStart(func(ctx context.Context, conv *Conversation) error {
			startCalled = true
			return nil
		}).
		OnComplete(func(ctx context.Context, conv *Conversation) error {
			completeCalled = true
			return nil
		}).
		OnCancel(func(ctx context.Context, conv *Conversation) error {
			cancelCalled = true
			return nil
		}).
		OnTimeout(func(ctx context.Context, conv *Conversation) error {
			timeoutCalled = true
			return nil
		}).
		AddState("start", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return Done()
		}).
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx := context.Background()
	conv := NewConversation("test", "callback-flow", "start", 1, 1, time.Now().Add(5*time.Minute))

	// Test callbacks are set
	if flow.OnStart == nil || flow.OnComplete == nil || flow.OnCancel == nil || flow.OnTimeout == nil {
		t.Error("callbacks should be set")
	}

	// Execute callbacks to verify they work
	flow.OnStart(ctx, conv)
	flow.OnComplete(ctx, conv)
	flow.OnCancel(ctx, conv)
	flow.OnTimeout(ctx, conv)

	if !startCalled || !completeCalled || !cancelCalled || !timeoutCalled {
		t.Error("all callbacks should have been called")
	}
}

func TestFlowBuilder_StateBuilder(t *testing.T) {
	enterCalled := false
	exitCalled := false

	flow, err := NewFlow("state-builder-flow").
		StartState("state1").
		State("state1").
		Handler(func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return GoTo("state2")
		}).
		AllowTransitions("state2").
		Timeout(60).
		OnEnter(func(ctx context.Context, conv *Conversation) error {
			enterCalled = true
			return nil
		}).
		OnExit(func(ctx context.Context, conv *Conversation) error {
			exitCalled = true
			return nil
		}).
		End().
		State("state2").
		Handler(func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return Done()
		}).
		End().
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	state1 := flow.States["state1"]
	if state1 == nil {
		t.Fatal("state1 should exist")
	}
	if len(state1.AllowedTransitions) != 1 || state1.AllowedTransitions[0] != "state2" {
		t.Errorf("expected AllowedTransitions [state2], got %v", state1.AllowedTransitions)
	}
	if state1.TimeoutSeconds != 60 {
		t.Errorf("expected TimeoutSeconds 60, got %d", state1.TimeoutSeconds)
	}
	if state1.OnEnter == nil || state1.OnExit == nil {
		t.Error("OnEnter and OnExit should be set")
	}

	// Test callbacks work
	ctx := context.Background()
	conv := NewConversation("test", "flow", "state1", 1, 1, time.Now().Add(5*time.Minute))
	state1.OnEnter(ctx, conv)
	state1.OnExit(ctx, conv)

	if !enterCalled || !exitCalled {
		t.Error("state callbacks should have been called")
	}
}

func TestFlowBuilder_SimpleState(t *testing.T) {
	flow, err := NewFlow("simple-state-flow").
		StartState("start").
		SimpleState("start", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return GoTo("middle")
		}).AllowTransitions("middle").
		SimpleState("middle", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return GoTo("end")
		}).AllowTransitions("end").
		SimpleState("end", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return Done()
		}).End().
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(flow.States) != 3 {
		t.Errorf("expected 3 states, got %d", len(flow.States))
	}

	startState := flow.States["start"]
	if len(startState.AllowedTransitions) != 1 || startState.AllowedTransitions[0] != "middle" {
		t.Errorf("unexpected transitions for start: %v", startState.AllowedTransitions)
	}
}

func TestFlowBuilder_SimpleStateWithTimeout(t *testing.T) {
	flow, err := NewFlow("timeout-flow").
		StartState("start").
		SimpleState("start", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return Done()
		}).WithTimeout(120).End().
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	startState := flow.States["start"]
	if startState.TimeoutSeconds != 120 {
		t.Errorf("expected TimeoutSeconds 120, got %d", startState.TimeoutSeconds)
	}
}

func TestFlowBuilder_MustBuild(t *testing.T) {
	// Valid flow should not panic
	flow := NewFlow("valid-flow").
		StartState("start").
		AddState("start", func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return Done()
		}).
		MustBuild()

	if flow.ID != "valid-flow" {
		t.Errorf("expected ID valid-flow, got %s", flow.ID)
	}
}

func TestFlowBuilder_MustBuild_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustBuild should panic on invalid flow")
		}
	}()

	// Invalid flow - missing start state
	NewFlow("invalid-flow").MustBuild()
}

func TestFlowBuilder_Build_ValidationError(t *testing.T) {
	// Missing start state
	_, err := NewFlow("invalid-flow").Build()
	if err == nil {
		t.Error("expected error for missing start state")
	}

	// Missing handler
	_, err = NewFlow("invalid-flow").
		StartState("start").
		State("start").End().
		Build()
	if err == nil {
		t.Error("expected error for missing handler")
	}
}

func TestFlowBuilder_ComplexFlow(t *testing.T) {
	// Build a realistic multi-step conversation flow
	const (
		StateWelcome StateID = "welcome"
		StateGetName StateID = "get_name"
		StateGetAge  StateID = "get_age"
		StateConfirm StateID = "confirm"
	)

	flow, err := NewFlow("registration-flow").
		StartState(StateWelcome).
		DefaultTimeout(600).
		OnStart(func(ctx context.Context, conv *Conversation) error {
			conv.SetData("step_count", 0)
			return nil
		}).
		OnComplete(func(ctx context.Context, conv *Conversation) error {
			// Process registration
			return nil
		}).
		State(StateWelcome).
		Handler(func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			return GoToWithMessage(StateGetName, "Welcome! What's your name?")
		}).
		AllowTransitions(StateGetName).
		End().
		State(StateGetName).
		Handler(func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			if input.Text == "" {
				return StayWithMessage("Please enter a valid name")
			}
			conv.SetData("name", input.Text)
			return GoToWithMessage(StateGetAge, "How old are you?")
		}).
		AllowTransitions(StateGetAge).
		Timeout(120).
		End().
		State(StateGetAge).
		Handler(func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			// In real code, parse and validate age
			conv.SetData("age", input.Text)
			name := conv.GetString("name")
			return GoToWithMessage(StateConfirm, "Confirm: "+name+", age "+input.Text+"?")
		}).
		AllowTransitions(StateConfirm, StateGetName).
		End().
		State(StateConfirm).
		Handler(func(ctx context.Context, conv *Conversation, input *Input) *TransitionResult {
			if input.Text == "yes" {
				return DoneWithMessage("Registration complete!")
			}
			return GoToWithMessage(StateGetName, "Let's start over. What's your name?")
		}).
		AllowTransitions(StateGetName).
		End().
		Build()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify all states exist
	expectedStates := []StateID{StateWelcome, StateGetName, StateGetAge, StateConfirm}
	for _, stateID := range expectedStates {
		if _, ok := flow.States[stateID]; !ok {
			t.Errorf("missing state: %s", stateID)
		}
	}

	// Verify transitions
	if len(flow.States[StateGetAge].AllowedTransitions) != 2 {
		t.Errorf("expected 2 transitions from get_age, got %d", len(flow.States[StateGetAge].AllowedTransitions))
	}
}
