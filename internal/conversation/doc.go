// Package conversation provides a generic state machine for handling multi-step user interactions.
//
// The package implements a finite state machine pattern that enables building complex
// conversational flows with multiple states, transitions, and context management.
//
// # Basic Concepts
//
//   - FlowConfig: Defines a conversation flow with its states and transitions
//   - StateConfig: Defines a single state with its handler and allowed transitions
//   - Conversation: An active instance of a flow for a specific user
//   - Manager: Manages flows and active conversations
//
// # Example Usage
//
//	// Define states
//	const (
//	    StateStart  = conversation.StateID("start")
//	    StateInput  = conversation.StateID("input")
//	    StateConfirm = conversation.StateID("confirm")
//	)
//
//	// Create a flow
//	flow := &conversation.FlowConfig{
//	    ID:         "example_flow",
//	    StartState: StateStart,
//	    States: map[conversation.StateID]*conversation.StateConfig{
//	        StateStart: {
//	            Handler: func(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
//	                return conversation.GoToWithMessage(StateInput, "Please enter your name:")
//	            },
//	        },
//	        StateInput: {
//	            Handler: func(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
//	                conv.SetData("name", input.Text)
//	                return conversation.GoToWithMessage(StateConfirm, "Confirm name: " + input.Text + "?")
//	            },
//	            AllowedTransitions: []conversation.StateID{StateConfirm},
//	        },
//	        StateConfirm: {
//	            Handler: func(ctx context.Context, conv *conversation.Conversation, input *conversation.Input) *conversation.TransitionResult {
//	                if input.Text == "yes" {
//	                    return conversation.DoneWithMessage("Thank you!")
//	                }
//	                return conversation.GoToWithMessage(StateInput, "Please enter your name again:")
//	            },
//	            AllowedTransitions: []conversation.StateID{StateInput},
//	        },
//	    },
//	    DefaultTimeoutSeconds: 300,
//	}
//
//	// Create manager and register flow
//	manager := conversation.NewManager()
//	manager.RegisterFlow(flow)
//
//	// Start a conversation
//	conv, _ := manager.StartConversation(ctx, "example_flow", userID, chatID)
//
//	// Process input
//	result, _ := manager.ProcessInput(ctx, userID, conversation.NewTextInput("John"))
package conversation
