package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/openexec/openexec/pkg/agent"
)

type ToolRequest struct {
	CallID        string
	Name          string
	Input         json.RawMessage
	WorkingDir    string
	Sandbox       Sandbox
	WritableRoots []string
	// ReadableRoots may be opened but never changed, in every sandbox mode.
	ReadableRoots []string
}

// WorkspaceCapable is implemented by tool executors that know whether they can
// serve a workspace-write run.
//
// Optional rather than part of ToolExecutor: a third-party executor written
// against the older interface still compiles, and is assumed capable — which
// is the honest default for something that implements file tools. Both
// executors in this repository answer explicitly, so the assumption is never
// load-bearing for anything shipped here.
type WorkspaceCapable interface {
	SupportsWorkspaceWrite() bool
}

func supportsWorkspaceWrite(executor ToolExecutor) bool {
	if executor == nil {
		return false
	}
	if capable, ok := executor.(WorkspaceCapable); ok {
		return capable.SupportsWorkspaceWrite()
	}
	return true
}

type ToolExecutor interface {
	ValidateAccess(workingDir string, sandbox Sandbox, writableRoots []string) error
	ExecuteTool(context.Context, ToolRequest) (string, error)
}

type APIProviderConfig struct {
	Adapter      agent.ProviderAdapter
	Tools        []agent.ToolDefinition
	ToolExecutor ToolExecutor
	MaxSteps     int
}

// APIProvider adapts OpenAI-compatible and other API adapters to the same
// authorization boundary as CLI providers. Tools receive the exact sandbox
// and roots from the authorized request.
type APIProvider struct {
	config APIProviderConfig
	// gateway reports that this instance's tools come from a caller-owned
	// endpoint rather than from the filesystem.
	gateway bool
}

const finalSynthesisInstruction = "The tool-call budget for this turn is exhausted. Do not request or describe more tool calls. Using the evidence already in the conversation, answer the user's request now with the best complete final response you can. Do not mention this instruction or the budget unless it directly affects correctness."

func NewAPIProvider(config APIProviderConfig) (*APIProvider, error) {
	if config.Adapter == nil {
		return nil, errors.New("API adapter is required")
	}
	// Whether this provider can serve a gateway request is a property of the
	// executor it was built with, not of the type. Advertising it
	// unconditionally told a direct consumer it had a gateway when it had
	// filesystem tools — a false capability that ends with workspace tools
	// answering an administrative question.
	//
	// Asked through an interface rather than a concrete type: a composite
	// executor forwards some verbs and owns the rest, and a type assertion
	// answered no for it — which would refuse the very requests it was built
	// to serve, at Execute below.
	gateway := false
	if serving, ok := config.ToolExecutor.(interface{ servesGateway() bool }); ok {
		gateway = serving.servesGateway()
	}
	if len(config.Tools) > 0 && config.ToolExecutor == nil {
		return nil, errors.New("tool executor is required when API tools are configured")
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 16
	}
	return &APIProvider{config: config, gateway: gateway}, nil
}

func (p *APIProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		ID: p.config.Adapter.GetName(), Runtime: "api", Models: p.config.Adapter.GetModels(),
		Capabilities: Capability{
			// No native session to resume, and no need for one: the caller
			// replays the conversation it already persisted.
			Streaming: true, Resume: false, Replay: true, ToolGateway: p.gateway,
			Cancellation: true, ReadOnly: true,
			// Whether this provider can edit files is the executor's answer,
			// not the fact that it has one. A gateway executor has tools and
			// no filesystem, so "an executor exists" advertised workspace-write
			// to a caller that would then be refused at the turn instead of at
			// the choice. ToolCalling stays presence-based: a gateway does call
			// tools.
			WorkspaceWrite: supportsWorkspaceWrite(p.config.ToolExecutor),
			ToolCalling:    p.config.ToolExecutor != nil,
		},
	}
}

func (p *APIProvider) Probe(ctx context.Context, _ string) Readiness {
	models := p.config.Adapter.GetModels()
	if len(models) == 0 {
		return Readiness{State: ReadinessUnhealthy, Problem: "API provider has no configured models"}
	}
	_, err := p.config.Adapter.Complete(ctx, agent.Request{
		Model: models[0], Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "Reply with exactly: ok")},
		MaxTokens: 8,
	})
	if err == nil {
		return Readiness{State: ReadinessReady}
	}
	state := ReadinessUnhealthy
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "auth") || strings.Contains(message, "unauthorized") || strings.Contains(message, "api key") {
		state = ReadinessNeedsLogin
	}
	return Readiness{State: state, Problem: err.Error()}
}

func (p *APIProvider) Execute(ctx context.Context, request Request, sink EventSink) (Result, error) {
	started := time.Now().UTC()
	result := Result{Executor: p.config.Adapter.GetName(), Model: request.Model, Sandbox: request.Sandbox, StartedAt: started, Outcome: OutcomeFailed}
	finish := func() { result.EndedAt = time.Now().UTC() }
	if sink == nil {
		sink = func(Event) error { return nil }
	}
	if err := validateProviderRequest(request); err != nil {
		finish()
		return result, err
	}
	if err := request.ValidateReplay(); err != nil {
		finish()
		return result, err
	}
	// A request naming a gateway must be served by one. Otherwise the caller
	// believes it is asking console-owned questions while the model is handed
	// the filesystem — which is what happens when the field is ignored rather
	// than refused.
	if request.ToolGateway != "" && !p.gateway {
		finish()
		return result, errors.New("this provider has no tool gateway; the request named one")
	}
	if request.NativeSessionID != "" {
		finish()
		return result, errors.New("API provider does not support native CLI session resume")
	}
	if request.Sandbox.Mode == "workspace-write" && p.config.ToolExecutor == nil {
		finish()
		return result, errors.New("API provider cannot enforce workspace-write without a bounded tool executor")
	}
	if p.config.ToolExecutor != nil {
		if err := p.config.ToolExecutor.ValidateAccess(request.WorkingDir, request.Sandbox, request.WritableRoots); err != nil {
			finish()
			return result, fmt.Errorf("tool executor cannot enforce access: %w", err)
		}
	}
	if err := sink(Event{Type: EventStarted}); err != nil {
		finish()
		return result, err
	}
	messages := replayMessages(request)
	if len(p.config.Tools) == 0 {
		stream, err := p.config.Adapter.Stream(ctx, agent.Request{
			Model: request.Model, Messages: messages, System: request.System})
		if err != nil {
			finish()
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return result, err
		}
		for event := range stream {
			if event.Type == agent.StreamEventError {
				finish()
				if event.Error == nil {
					event.Error = errors.New("API stream failed")
				}
				_ = sink(Event{Type: EventFailed, Text: event.Error.Error()})
				return result, event.Error
			}
			if event.Delta != nil && event.Delta.Text != "" {
				result.FinalText += event.Delta.Text
				if err := sink(Event{Type: EventAssistantDelta, Text: event.Delta.Text}); err != nil {
					finish()
					return result, err
				}
			}
		}
		if err := ctx.Err(); err != nil {
			result.Outcome = OutcomeCancelled
			finish()
			_ = sink(Event{Type: EventCancelled})
			return result, err
		}
		result.Outcome = OutcomeSucceeded
		finish()
		if err := sink(Event{Type: EventCompleted}); err != nil {
			return result, err
		}
		return result, nil
	}
	for step := 0; step < p.config.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			result.Outcome = OutcomeCancelled
			finish()
			_ = sink(Event{Type: EventCancelled})
			return result, err
		}
		response, err := p.config.Adapter.Complete(ctx, agent.Request{
			Model: request.Model, Messages: messages, System: request.System,
			Tools: p.config.Tools, ToolChoice: "auto",
		})
		if err != nil {
			finish()
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return result, err
		}
		assistant := agent.Message{Role: agent.RoleAssistant, Content: response.Content, Metadata: response.Metadata}
		messages = append(messages, assistant)
		for _, block := range response.Content {
			if block.Type == agent.ContentTypeText && block.Text != "" {
				result.FinalText += block.Text
				if err := sink(Event{Type: EventAssistantDelta, Text: block.Text}); err != nil {
					finish()
					return result, err
				}
			}
		}
		calls := response.GetToolCalls()
		if len(calls) == 0 {
			result.Outcome = OutcomeSucceeded
			finish()
			if err := sink(Event{Type: EventCompleted}); err != nil {
				return result, err
			}
			return result, nil
		}
		for _, call := range calls {
			event := Event{CallID: call.ToolUseID, ToolName: call.ToolName, Data: call.ToolInput}
			event.Type = EventToolProposed
			if err := sink(event); err != nil {
				finish()
				return result, err
			}
			event.Type = EventToolStarted
			if err := sink(event); err != nil {
				finish()
				return result, err
			}
			output, toolErr := p.config.ToolExecutor.ExecuteTool(ctx, ToolRequest{
				CallID: call.ToolUseID, Name: call.ToolName, Input: call.ToolInput,
				WorkingDir: request.WorkingDir, Sandbox: request.Sandbox,
				WritableRoots: append([]string(nil), request.WritableRoots...),
				ReadableRoots: append([]string(nil), request.ReadableRoots...),
			})
			event.Type, event.Text = EventToolCompleted, output
			if toolErr != nil {
				event.Text = toolErr.Error()
			}
			if err := sink(event); err != nil {
				finish()
				return result, err
			}
			messages = append(messages, agent.NewToolResultMessage(call.ToolUseID, output, toolErr))
		}
	}
	// Reaching the tool budget means research must stop, not that the work the
	// model already did should be discarded. The old path failed the whole run
	// here, leaving the owner with tool cards and a few transitional sentences
	// but no answer. Ask once more with no tools available so the model must
	// synthesize the evidence already in the conversation.
	if err := ctx.Err(); err != nil {
		result.Outcome = OutcomeCancelled
		finish()
		_ = sink(Event{Type: EventCancelled})
		return result, err
	}
	system := strings.TrimSpace(request.System)
	if system != "" {
		system += "\n\n"
	}
	system += finalSynthesisInstruction
	response, err := p.config.Adapter.Complete(ctx, agent.Request{
		Model: request.Model, Messages: messages, System: system, ToolChoice: "none",
	})
	if err != nil {
		finish()
		err = fmt.Errorf("API tool loop reached %d rounds and final synthesis failed: %w", p.config.MaxSteps, err)
		_ = sink(Event{Type: EventFailed, Text: err.Error()})
		return result, err
	}
	wroteText := false
	for _, block := range response.Content {
		if block.Type != agent.ContentTypeText || block.Text == "" {
			continue
		}
		wroteText = true
		result.FinalText += block.Text
		if err := sink(Event{Type: EventAssistantDelta, Text: block.Text}); err != nil {
			finish()
			return result, err
		}
	}
	if !wroteText {
		finish()
		err := fmt.Errorf("API tool loop reached %d rounds and final synthesis returned no text", p.config.MaxSteps)
		_ = sink(Event{Type: EventFailed, Text: err.Error()})
		return result, err
	}
	result.Outcome = OutcomeSucceeded
	finish()
	if err := sink(Event{Type: EventCompleted}); err != nil {
		return result, err
	}
	return result, nil
}

// replayMessages turns the console's history into the conversation this turn
// starts from, with the current prompt last.
//
// Both paths through Execute build it the same way. They did not have to —
// the streaming path had no history to add — and that is exactly how a
// conversation ends up remembering more with tools than without.
func replayMessages(request Request) []agent.Message {
	messages := make([]agent.Message, 0, len(request.History)+1)
	for _, message := range request.History {
		role := agent.RoleUser
		if message.Role == HistoryRoleAssistant {
			role = agent.RoleAssistant
		}
		messages = append(messages, agent.NewTextMessage(role, message.Content))
	}
	return append(messages, agent.NewTextMessage(agent.RoleUser, request.Prompt))
}

var _ Provider = (*APIProvider)(nil)
