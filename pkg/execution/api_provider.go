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
}

func NewAPIProvider(config APIProviderConfig) (*APIProvider, error) {
	if config.Adapter == nil {
		return nil, errors.New("API adapter is required")
	}
	if len(config.Tools) > 0 && config.ToolExecutor == nil {
		return nil, errors.New("tool executor is required when API tools are configured")
	}
	if config.MaxSteps <= 0 {
		config.MaxSteps = 16
	}
	return &APIProvider{config: config}, nil
}

func (p *APIProvider) Descriptor() ProviderDescriptor {
	return ProviderDescriptor{
		ID: p.config.Adapter.GetName(), Runtime: "api", Models: p.config.Adapter.GetModels(),
		Capabilities: Capability{
			Streaming: true, Cancellation: true, ReadOnly: true,
			WorkspaceWrite: p.config.ToolExecutor != nil, ToolCalling: p.config.ToolExecutor != nil,
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
	messages := []agent.Message{agent.NewTextMessage(agent.RoleUser, request.Prompt)}
	if len(p.config.Tools) == 0 {
		stream, err := p.config.Adapter.Stream(ctx, agent.Request{Model: request.Model, Messages: messages})
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
			Model: request.Model, Messages: messages, Tools: p.config.Tools, ToolChoice: "auto",
		})
		if err != nil {
			finish()
			_ = sink(Event{Type: EventFailed, Text: err.Error()})
			return result, err
		}
		assistant := agent.Message{Role: agent.RoleAssistant, Content: response.Content}
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
				WorkingDir: request.WorkingDir, Sandbox: request.Sandbox, WritableRoots: append([]string(nil), request.WritableRoots...),
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
	finish()
	err := fmt.Errorf("API tool loop exceeded %d steps", p.config.MaxSteps)
	_ = sink(Event{Type: EventFailed, Text: err.Error()})
	return result, err
}

var _ Provider = (*APIProvider)(nil)
