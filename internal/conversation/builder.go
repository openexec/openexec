package conversation

import (
	"context"
)

// FlowBuilder provides a fluent API for building conversation flows.
type FlowBuilder struct {
	flow *FlowConfig
}

// NewFlow creates a new FlowBuilder with the given flow ID.
func NewFlow(id FlowID) *FlowBuilder {
	return &FlowBuilder{
		flow: &FlowConfig{
			ID:     id,
			States: make(map[StateID]*StateConfig),
		},
	}
}

// StartState sets the initial state for the flow.
func (b *FlowBuilder) StartState(state StateID) *FlowBuilder {
	b.flow.StartState = state
	return b
}

// DefaultTimeout sets the default timeout in seconds for states.
func (b *FlowBuilder) DefaultTimeout(seconds int) *FlowBuilder {
	b.flow.DefaultTimeoutSeconds = seconds
	return b
}

// OnStart sets the callback for when a conversation starts.
func (b *FlowBuilder) OnStart(fn func(ctx context.Context, conv *Conversation) error) *FlowBuilder {
	b.flow.OnStart = fn
	return b
}

// OnComplete sets the callback for when a conversation completes.
func (b *FlowBuilder) OnComplete(fn func(ctx context.Context, conv *Conversation) error) *FlowBuilder {
	b.flow.OnComplete = fn
	return b
}

// OnCancel sets the callback for when a conversation is cancelled.
func (b *FlowBuilder) OnCancel(fn func(ctx context.Context, conv *Conversation) error) *FlowBuilder {
	b.flow.OnCancel = fn
	return b
}

// OnTimeout sets the callback for when a conversation times out.
func (b *FlowBuilder) OnTimeout(fn func(ctx context.Context, conv *Conversation) error) *FlowBuilder {
	b.flow.OnTimeout = fn
	return b
}

// AddState adds a state with the given handler to the flow.
func (b *FlowBuilder) AddState(id StateID, handler StateHandler) *FlowBuilder {
	b.flow.States[id] = &StateConfig{
		ID:      id,
		Handler: handler,
	}
	return b
}

// State returns a StateBuilder for configuring a state.
func (b *FlowBuilder) State(id StateID) *StateBuilder {
	return &StateBuilder{
		flowBuilder: b,
		state: &StateConfig{
			ID: id,
		},
	}
}

// Build validates and returns the completed FlowConfig.
func (b *FlowBuilder) Build() (*FlowConfig, error) {
	if err := b.flow.Validate(); err != nil {
		return nil, err
	}
	return b.flow, nil
}

// MustBuild is like Build but panics on validation error.
func (b *FlowBuilder) MustBuild() *FlowConfig {
	flow, err := b.Build()
	if err != nil {
		panic(err)
	}
	return flow
}

// StateBuilder provides a fluent API for building state configurations.
type StateBuilder struct {
	flowBuilder *FlowBuilder
	state       *StateConfig
}

// Handler sets the state handler.
func (s *StateBuilder) Handler(handler StateHandler) *StateBuilder {
	s.state.Handler = handler
	return s
}

// AllowTransitions sets the allowed transitions from this state.
func (s *StateBuilder) AllowTransitions(states ...StateID) *StateBuilder {
	s.state.AllowedTransitions = states
	return s
}

// Timeout sets the state-specific timeout in seconds.
func (s *StateBuilder) Timeout(seconds int) *StateBuilder {
	s.state.TimeoutSeconds = seconds
	return s
}

// OnEnter sets the callback for when entering this state.
func (s *StateBuilder) OnEnter(fn func(ctx context.Context, conv *Conversation) error) *StateBuilder {
	s.state.OnEnter = fn
	return s
}

// OnExit sets the callback for when exiting this state.
func (s *StateBuilder) OnExit(fn func(ctx context.Context, conv *Conversation) error) *StateBuilder {
	s.state.OnExit = fn
	return s
}

// End finishes configuring this state and returns to the flow builder.
func (s *StateBuilder) End() *FlowBuilder {
	s.flowBuilder.flow.States[s.state.ID] = s.state
	return s.flowBuilder
}

// SimpleState creates a state with just a handler using a concise syntax.
// Usage: builder.SimpleState(StateID, handler).AllowTransitions(targets...).End()
func (b *FlowBuilder) SimpleState(id StateID, handler StateHandler) *SimpleStateBuilder {
	return &SimpleStateBuilder{
		flowBuilder: b,
		state: &StateConfig{
			ID:      id,
			Handler: handler,
		},
	}
}

// SimpleStateBuilder is a minimal builder for simple state configurations.
type SimpleStateBuilder struct {
	flowBuilder *FlowBuilder
	state       *StateConfig
}

// AllowTransitions sets the allowed transitions and returns to the flow builder.
func (s *SimpleStateBuilder) AllowTransitions(states ...StateID) *FlowBuilder {
	s.state.AllowedTransitions = states
	s.flowBuilder.flow.States[s.state.ID] = s.state
	return s.flowBuilder
}

// End adds the state to the flow without transition restrictions.
func (s *SimpleStateBuilder) End() *FlowBuilder {
	s.flowBuilder.flow.States[s.state.ID] = s.state
	return s.flowBuilder
}

// WithTimeout sets a timeout and returns to the flow builder.
func (s *SimpleStateBuilder) WithTimeout(seconds int) *SimpleStateBuilder {
	s.state.TimeoutSeconds = seconds
	return s
}
