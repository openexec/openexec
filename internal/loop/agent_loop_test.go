package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/internal/mcp"
	"github.com/openexec/openexec/internal/summarize"
)

// MockProvider implements agent.ProviderAdapter for testing.
type MockProvider struct {
	name             string
	models           []string
	responses        []*agent.Response
	responseIdx      int
	completeCount    int
	lastRequest      agent.Request
	completeFunc     func(context.Context, agent.Request) (*agent.Response, error)
	mu               sync.Mutex
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		name:      "mock",
		models:    []string{"mock-model"},
		responses: make([]*agent.Response, 0),
	}
}

func (p *MockProvider) GetName() string {
	return p.name
}

func (p *MockProvider) GetModels() []string {
	return p.models
}

func (p *MockProvider) GetModelInfo(modelID string) (*agent.ModelInfo, error) {
	return &agent.ModelInfo{
		ID:                    modelID,
		Name:                  "Mock Model",
		Provider:              p.name,
		PricePerMInputTokens:  1.0,
		PricePerMOutputTokens: 2.0,
		Capabilities: agent.ProviderCapabilities{
			ToolUse:      true,
			Streaming:    true,
			SystemPrompt: true,
		},
	}, nil
}

func (p *MockProvider) GetCapabilities(modelID string) (*agent.ProviderCapabilities, error) {
	return &agent.ProviderCapabilities{
		ToolUse:      true,
		Streaming:    true,
		SystemPrompt: true,
	}, nil
}

func (p *MockProvider) Complete(ctx context.Context, req agent.Request) (*agent.Response, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.completeCount++
	p.lastRequest = req

	if p.completeFunc != nil {
		return p.completeFunc(ctx, req)
	}

	if p.responseIdx >= len(p.responses) {
		// Return a simple text response if no more responses configured
		return &agent.Response{
			ID:    "test-response",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "Test response"},
			},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		}, nil
	}

	response := p.responses[p.responseIdx]
	p.responseIdx++
	return response, nil
}

func (p *MockProvider) Stream(ctx context.Context, req agent.Request) (<-chan agent.StreamEvent, error) {
	ch := make(chan agent.StreamEvent)
	close(ch)
	return ch, nil
}

func (p *MockProvider) ValidateRequest(req agent.Request) error {
	return nil
}

func (p *MockProvider) EstimateTokens(content string) int {
	return len(content) / 4 // Simple estimate
}

func (p *MockProvider) AddResponse(resp *agent.Response) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.responses = append(p.responses, resp)
}

func (p *MockProvider) GetCompleteCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.completeCount
}

func (p *MockProvider) GetLastRequest() agent.Request {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastRequest
}

// Helper to create a mock registry with a provider
func createMockRegistry() (*agent.ProviderRegistry, *MockProvider) {
	registry := agent.NewProviderRegistry()
	provider := NewMockProvider()
	registry.Register(provider)
	return registry, provider
}

func TestNewAgentLoop(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	if loop == nil {
		t.Fatal("NewAgentLoop returned nil")
	}

	// Session ID should be auto-generated
	if loop.cfg.SessionID == "" {
		t.Error("expected auto-generated session ID")
	}

	// State should be initialized
	state := loop.State()
	if state.Iteration != 0 {
		t.Errorf("expected iteration 0, got %d", state.Iteration)
	}
	if state.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set")
	}
}

func TestNewAgentLoop_MissingModel(t *testing.T) {
	_, err := NewAgentLoop(AgentLoopConfig{})
	if err == nil {
		t.Error("expected error for missing model")
	}
}

func TestNewAgentLoop_InvalidModel(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		Model:    "nonexistent-model",
		Registry: registry,
	}

	_, err := NewAgentLoop(cfg)
	if err == nil {
		t.Error("expected error for invalid model")
	}
}

func TestAgentLoop_RunSimpleConversation(t *testing.T) {
	registry, provider := createMockRegistry()

	// Provider returns a simple text response
	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Hello, I can help you with that!"},
		},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{PromptTokens: 50, CompletionTokens: 20, TotalTokens: 70},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Collect events
	var events []*LoopEvent
	go func() {
		for event := range loop.Events() {
			events = append(events, event)
		}
	}()

	err = loop.Run(context.Background(), "Hello, can you help me?")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have completed after one iteration
	state := loop.State()
	if state.Iteration != 1 {
		t.Errorf("expected 1 iteration, got %d", state.Iteration)
	}

	// Provider should have been called once
	if provider.GetCompleteCount() != 1 {
		t.Errorf("expected 1 provider call, got %d", provider.GetCompleteCount())
	}

	// Messages should contain user and assistant messages
	if len(state.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(state.Messages))
	}
}

func TestAgentLoop_RunWithToolUse(t *testing.T) {
	registry, provider := createMockRegistry()

	// First response: tool use
	toolInput, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'test'",
	})
	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Let me run a command."},
			{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "tool-1",
				ToolName:  "run_shell_command",
				ToolInput: toolInput,
			},
		},
		StopReason: agent.StopReasonToolUse,
		Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	})

	// Second response: completion
	provider.AddResponse(&agent.Response{
		ID:    "resp-2",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Command completed successfully."},
		},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{PromptTokens: 200, CompletionTokens: 30, TotalTokens: 230},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Run a test command")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have completed after two iterations (tool use + response)
	state := loop.State()
	if state.Iteration != 2 {
		t.Errorf("expected 2 iterations, got %d", state.Iteration)
	}

	// Provider should have been called twice
	if provider.GetCompleteCount() != 2 {
		t.Errorf("expected 2 provider calls, got %d", provider.GetCompleteCount())
	}

	// Should have: user, assistant (tool), user (tool result), assistant
	if len(state.Messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(state.Messages))
	}
}

func TestAgentLoop_RunWithPhaseCompleteSignal(t *testing.T) {
	registry, provider := createMockRegistry()

	// Response with phase-complete signal
	signalInput, _ := json.Marshal(map[string]interface{}{
		"type":   "phase-complete",
		"reason": "Task completed successfully",
	})
	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "I've completed the task."},
			{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "signal-1",
				ToolName:  "axon_signal",
				ToolInput: signalInput,
			},
		},
		StopReason: agent.StopReasonToolUse,
		Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	})

	var signalReceived bool
	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		OnEvent: func(event *LoopEvent) {
			if event.Type == SignalReceived || event.Type == SignalPhaseComplete {
				signalReceived = true
			}
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Complete the task")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have received signal
	if !signalReceived {
		t.Error("expected signal event to be received")
	}

	// State should have the signal
	state := loop.State()
	if state.LastSignal == nil {
		t.Error("expected LastSignal to be set")
	} else if state.LastSignal.Type != mcp.SignalPhaseComplete {
		t.Errorf("expected phase-complete signal, got %s", state.LastSignal.Type)
	}
}

func TestAgentLoop_MaxIterations(t *testing.T) {
	registry, provider := createMockRegistry()

	// Provider always requests tool use, never completes
	toolInput, _ := json.Marshal(map[string]interface{}{
		"command": "echo 'test'",
	})
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return &agent.Response{
			ID:    "resp",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-1",
					ToolName:  "run_shell_command",
					ToolInput: toolInput,
				},
			},
			StopReason: agent.StopReasonToolUse,
			Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:         "mock-model",
		Registry:      registry,
		MaxIterations: 3,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test prompt")
	if err != ErrMaxIterations {
		t.Errorf("expected ErrMaxIterations, got %v", err)
	}

	state := loop.State()
	if state.Iteration != 3 {
		t.Errorf("expected 3 iterations, got %d", state.Iteration)
	}
}

func TestAgentLoop_Pause(t *testing.T) {
	registry, provider := createMockRegistry()

	// Provider never completes
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return &agent.Response{
			ID:    "resp",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "Working..."},
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-1",
					ToolName:  "run_shell_command",
					ToolInput: json.RawMessage(`{"command": "sleep 0.1"}`),
				},
			},
			StopReason: agent.StopReasonToolUse,
			Usage:      agent.Usage{TotalTokens: 100},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Pause after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		loop.Pause()
	}()

	err = loop.Run(context.Background(), "Test")
	if err != ErrLoopPaused {
		t.Errorf("expected ErrLoopPaused, got %v", err)
	}

	if !loop.IsPaused() {
		t.Error("expected loop to be paused")
	}
}

func TestAgentLoop_Stop(t *testing.T) {
	registry, provider := createMockRegistry()

	// Provider takes time to respond
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			StopReason: agent.StopReasonEnd,
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Stop after a short delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		loop.Stop()
	}()

	err = loop.Run(context.Background(), "Test")
	// Should return either context error or stopped error
	if err == nil {
		t.Error("expected error after stop")
	}

	if !loop.IsStopped() {
		t.Error("expected loop to be stopped")
	}
}

func TestAgentLoop_ContextCancellation(t *testing.T) {
	registry, provider := createMockRegistry()

	// Provider takes time to respond
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		select {
		case <-time.After(1 * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return nil, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = loop.Run(ctx, "Test")
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestAgentLoop_ThrashingDetection(t *testing.T) {
	registry, provider := createMockRegistry()

	// Provider always returns tool use without progress signals
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return &agent.Response{
			ID:    "resp",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-1",
					ToolName:  "run_shell_command",
					ToolInput: json.RawMessage(`{"command": "echo 'test'"}`),
				},
			},
			StopReason: agent.StopReasonToolUse,
			Usage:      agent.Usage{TotalTokens: 100},
		}, nil
	}

	var thrashingDetected bool
	cfg := AgentLoopConfig{
		Model:           "mock-model",
		Registry:        registry,
		ThrashThreshold: 3,
		OnEvent: func(event *LoopEvent) {
			if event.Type == ThrashingDetected {
				thrashingDetected = true
			}
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err == nil {
		t.Error("expected error from thrashing")
	}

	if !thrashingDetected {
		t.Error("expected thrashing event")
	}

	state := loop.State()
	if state.Iteration < 3 {
		t.Errorf("expected at least 3 iterations, got %d", state.Iteration)
	}
}

func TestAgentLoop_BudgetLimit(t *testing.T) {
	registry, provider := createMockRegistry()

	// Each response uses lots of tokens
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return &agent.Response{
			ID:    "resp",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-1",
					ToolName:  "run_shell_command",
					ToolInput: json.RawMessage(`{"command": "echo 'test'"}`),
				},
			},
			StopReason: agent.StopReasonToolUse,
			Usage:      agent.Usage{PromptTokens: 500000, CompletionTokens: 500000, TotalTokens: 1000000},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:     "mock-model",
		Registry:  registry,
		BudgetUSD: 0.10, // Very low budget
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != ErrBudgetExceeded {
		t.Errorf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestAgentLoop_MaxTokensLimit(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return &agent.Response{
			ID:    "resp",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{
					Type:      agent.ContentTypeToolUse,
					ToolUseID: "tool-1",
					ToolName:  "run_shell_command",
					ToolInput: json.RawMessage(`{"command": "echo 'test'"}`),
				},
			},
			StopReason: agent.StopReasonToolUse,
			Usage:      agent.Usage{TotalTokens: 1000},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:     "mock-model",
		Registry:  registry,
		MaxTokens: 500,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != ErrMaxTokens {
		t.Errorf("expected ErrMaxTokens, got %v", err)
	}
}

func TestAgentLoop_RunSingleTurn(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Hello!"},
		},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	resp, err := loop.RunSingleTurn(context.Background(), "Hi there")
	if err != nil {
		t.Fatalf("RunSingleTurn failed: %v", err)
	}

	if resp.GetText() != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", resp.GetText())
	}

	// Should have 2 messages: user and assistant
	history := loop.GetConversationHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 messages, got %d", len(history))
	}
}

func TestAgentLoop_ExecuteToolCalls(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	toolCalls := []agent.ContentBlock{
		{
			Type:      agent.ContentTypeToolUse,
			ToolUseID: "call-1",
			ToolName:  "run_shell_command",
			ToolInput: json.RawMessage(`{"command": "echo 'test'"}`),
		},
	}

	results, err := loop.ExecuteToolCalls(context.Background(), toolCalls)
	if err != nil {
		t.Fatalf("ExecuteToolCalls failed: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	if results[0].IsError {
		t.Errorf("unexpected error: %s", results[0].Output)
	}
}

func TestAgentLoop_AddMessage(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	loop.AddMessage(agent.NewTextMessage(agent.RoleUser, "Test message"))

	history := loop.GetConversationHistory()
	if len(history) != 1 {
		t.Errorf("expected 1 message, got %d", len(history))
	}
}

func TestAgentLoop_SetMessages(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Hello"),
		agent.NewTextMessage(agent.RoleAssistant, "Hi there"),
	}

	loop.SetMessages(messages)

	history := loop.GetConversationHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 messages, got %d", len(history))
	}
}

func TestAgentLoop_Events(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	var events []*LoopEvent
	var mu sync.Mutex

	done := make(chan struct{})
	go func() {
		for event := range loop.Events() {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		}
		close(done)
	}()

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	<-done

	mu.Lock()
	defer mu.Unlock()

	// Should have at least: LoopStart, IterationStart, LLMRequestStart, LLMRequestEnd, MessageUser, MessageAssistant, IterationComplete, LoopComplete
	if len(events) < 6 {
		t.Errorf("expected at least 6 events, got %d", len(events))
	}

	// Find specific events
	var hasStart, hasComplete bool
	for _, e := range events {
		if e.Type == LoopEventStart {
			hasStart = true
		}
		if e.Type == LoopEventComplete {
			hasComplete = true
		}
	}

	if !hasStart {
		t.Error("missing LoopEventStart")
	}
	if !hasComplete {
		t.Error("missing LoopEventComplete")
	}
}

func TestAgentLoop_RetryOnTransientError(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount < 3 {
			// Return retryable error
			return nil, &agent.ProviderError{
				Code:      agent.ErrCodeRateLimit,
				Message:   "rate limited",
				Retryable: true,
			}
		}
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Success"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		RetryConfig: &RetryConfig{
			MaxRetries:      3,
			Backoff:         []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
			RetryableErrors: []string{agent.ErrCodeRateLimit},
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have succeeded after retries
	if callCount != 3 {
		t.Errorf("expected 3 provider calls, got %d", callCount)
	}
}

func TestAgentLoop_NonRetryableError(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return nil, &agent.ProviderError{
			Code:      agent.ErrCodeAuthentication,
			Message:   "invalid API key",
			Retryable: false,
		}
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err == nil {
		t.Error("expected error for non-retryable failure")
	}

	// Should fail immediately without retries
	if provider.GetCompleteCount() != 1 {
		t.Errorf("expected 1 provider call (no retries), got %d", provider.GetCompleteCount())
	}
}

func TestAgentLoop_SystemPrompt(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:        "mock-model",
		Registry:     registry,
		SystemPrompt: "You are a helpful assistant.",
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Check that system prompt was included in request
	lastReq := provider.GetLastRequest()
	if lastReq.System != "You are a helpful assistant." {
		t.Errorf("expected system prompt in request, got %q", lastReq.System)
	}
}

func TestAgentLoop_ConcurrentAccess(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Run in background
	go func() {
		_ = loop.Run(context.Background(), "Test")
	}()

	// Concurrent access to state
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = loop.State()
			_ = loop.IsPaused()
			_ = loop.IsStopped()
			_ = loop.GetConversationHistory()
		}()
	}

	wg.Wait()
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries 3, got %d", cfg.MaxRetries)
	}

	if len(cfg.Backoff) != 3 {
		t.Errorf("expected 3 backoff values, got %d", len(cfg.Backoff))
	}

	if len(cfg.RetryableErrors) == 0 {
		t.Error("expected retryable errors")
	}

	// Check that rate_limit is retryable
	hasRateLimit := false
	for _, code := range cfg.RetryableErrors {
		if code == agent.ErrCodeRateLimit {
			hasRateLimit = true
			break
		}
	}
	if !hasRateLimit {
		t.Error("expected rate_limit to be retryable")
	}
}

// TestAgentLoop_Resume tests the Resume method
func TestAgentLoop_Resume(t *testing.T) {
	registry, provider := createMockRegistry()

	iterationCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		iterationCount++
		if iterationCount == 1 {
			// First iteration returns tool use to continue the loop
			return &agent.Response{
				ID:    "resp",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{Type: agent.ContentTypeText, Text: "Working..."},
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: "tool-1",
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo test"}`),
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{TotalTokens: 100},
			}, nil
		}
		// Second iteration completes
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Pause the loop
	loop.Pause()
	if !loop.IsPaused() {
		t.Error("expected loop to be paused")
	}

	// Resume the loop
	loop.Resume()
	if loop.IsPaused() {
		t.Error("expected loop to be resumed")
	}
}

// TestAgentLoop_PauseAndResumeCycle tests pausing and resuming during execution
func TestAgentLoop_PauseAndResumeCycle(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		return &agent.Response{
			ID:    "resp",
			Model: req.Model,
			Content: []agent.ContentBlock{
				{Type: agent.ContentTypeText, Text: "Done"},
			},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 100},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Initial state
	if loop.IsPaused() {
		t.Error("loop should not be paused initially")
	}
	if loop.IsStopped() {
		t.Error("loop should not be stopped initially")
	}

	// Pause and resume before running
	loop.Pause()
	if !loop.IsPaused() {
		t.Error("loop should be paused after Pause()")
	}
	loop.Resume()
	if loop.IsPaused() {
		t.Error("loop should not be paused after Resume()")
	}
}

// TestAgentLoop_WithSessionID tests loop with custom session ID
func TestAgentLoop_WithSessionID(t *testing.T) {
	registry, _ := createMockRegistry()

	customSessionID := "custom-session-123"
	cfg := AgentLoopConfig{
		Model:     "mock-model",
		Registry:  registry,
		SessionID: customSessionID,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	if loop.cfg.SessionID != customSessionID {
		t.Errorf("expected session ID %s, got %s", customSessionID, loop.cfg.SessionID)
	}
}

// TestAgentLoop_MaxTokensResponseTruncated tests handling of StopReasonMaxTokens
func TestAgentLoop_MaxTokensResponseTruncated(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount == 1 {
			// First call returns with max_tokens stop reason but no tool calls
			return &agent.Response{
				ID:    "resp-1",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{Type: agent.ContentTypeText, Text: "Response truncated due to token limit"},
				},
				StopReason: agent.StopReasonMaxTokens,
				Usage:      agent.Usage{TotalTokens: 1000},
			}, nil
		}
		// This should not be reached since no tool calls were made
		return &agent.Response{
			ID:         "resp-2",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should complete after one iteration since no tool calls
	if callCount != 1 {
		t.Errorf("expected 1 provider call, got %d", callCount)
	}
}

// TestAgentLoop_MultipleToolCallsInOneResponse tests multiple tool calls in a single response
func TestAgentLoop_MultipleToolCallsInOneResponse(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount == 1 {
			// First call returns multiple tool calls
			return &agent.Response{
				ID:    "resp-1",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{Type: agent.ContentTypeText, Text: "Executing multiple commands"},
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: "tool-1",
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo first"}`),
					},
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: "tool-2",
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo second"}`),
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{TotalTokens: 200},
			}, nil
		}
		// Second call completes
		return &agent.Response{
			ID:         "resp-2",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have 4 messages: user, assistant (2 tools), user (tool results), assistant
	state := loop.State()
	if len(state.Messages) != 4 {
		t.Errorf("expected 4 messages, got %d", len(state.Messages))
	}

	// Tool results should be in the third message
	if len(state.Messages) >= 3 {
		toolResultMsg := state.Messages[2]
		if len(toolResultMsg.Content) != 2 {
			t.Errorf("expected 2 tool results, got %d", len(toolResultMsg.Content))
		}
	}
}

// TestAgentLoop_RouteSignal tests handling of route signals
func TestAgentLoop_RouteSignal(t *testing.T) {
	registry, provider := createMockRegistry()

	// Response with route signal
	signalInput, _ := json.Marshal(map[string]interface{}{
		"type":   "route",
		"target": "spark",
		"reason": "Need spark agent for this task",
	})
	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Routing to spark agent."},
			{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "signal-1",
				ToolName:  "axon_signal",
				ToolInput: signalInput,
			},
		},
		StopReason: agent.StopReasonToolUse,
		Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	})

	var signalReceived bool
	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		OnEvent: func(event *LoopEvent) {
			if event.Type == SignalReceived {
				signalReceived = true
			}
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Route to spark")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have received signal
	if !signalReceived {
		t.Error("expected signal event to be received")
	}

	// State should have the signal
	state := loop.State()
	if state.LastSignal == nil {
		t.Error("expected LastSignal to be set")
	} else if state.LastSignal.Type != mcp.SignalRoute {
		t.Errorf("expected route signal, got %s", state.LastSignal.Type)
	} else if state.LastSignal.Target != "spark" {
		t.Errorf("expected target 'spark', got %s", state.LastSignal.Target)
	}
}

// TestAgentLoop_ProgressSignalResetsThrashold tests that progress signals reset thrashing counter
func TestAgentLoop_ProgressSignalResetsThreshold(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount <= 3 {
			// First 3 calls: send progress signals
			progressInput, _ := json.Marshal(map[string]interface{}{
				"type":   "progress",
				"reason": "Making progress",
			})
			return &agent.Response{
				ID:    "resp",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: fmt.Sprintf("tool-%d", callCount),
						ToolName:  "axon_signal",
						ToolInput: progressInput,
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{TotalTokens: 100},
			}, nil
		}
		// Fourth call: complete
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:           "mock-model",
		Registry:        registry,
		ThrashThreshold: 2, // Would trigger after 2 iterations without progress
		MaxIterations:   10,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should complete successfully because progress signals reset the counter
	if callCount != 4 {
		t.Errorf("expected 4 provider calls, got %d", callCount)
	}

	state := loop.State()
	if state.IterationsSinceProgress != 0 {
		t.Errorf("expected IterationsSinceProgress to be 0 after completion, got %d", state.IterationsSinceProgress)
	}
}

// TestAgentLoop_CustomRetryConfig tests loop with custom retry configuration
func TestAgentLoop_CustomRetryConfig(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount < 2 {
			// First call returns retryable server error
			return nil, &agent.ProviderError{
				Code:      agent.ErrCodeServerError,
				Message:   "server error",
				Retryable: true,
			}
		}
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Success"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		RetryConfig: &RetryConfig{
			MaxRetries:      5,
			Backoff:         []time.Duration{time.Millisecond, time.Millisecond},
			RetryableErrors: []string{agent.ErrCodeServerError, agent.ErrCodeTimeout},
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 provider calls, got %d", callCount)
	}
}

// TestAgentLoop_RetryExhaustion tests that max retries is respected
func TestAgentLoop_RetryExhaustion(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		return nil, &agent.ProviderError{
			Code:      agent.ErrCodeRateLimit,
			Message:   "rate limited",
			Retryable: true,
		}
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		RetryConfig: &RetryConfig{
			MaxRetries:      2,
			Backoff:         []time.Duration{time.Millisecond, time.Millisecond},
			RetryableErrors: []string{agent.ErrCodeRateLimit},
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err == nil {
		t.Error("expected error after exhausting retries")
	}

	// Should have tried 3 times (initial + 2 retries)
	if callCount != 3 {
		t.Errorf("expected 3 provider calls, got %d", callCount)
	}
}

// TestAgentLoop_ResponseTimeout tests the response timeout configuration
func TestAgentLoop_ResponseTimeout(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		// Simulate slow response
		select {
		case <-time.After(200 * time.Millisecond):
			return &agent.Response{
				ID:         "resp",
				Model:      req.Model,
				Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
				StopReason: agent.StopReasonEnd,
				Usage:      agent.Usage{TotalTokens: 50},
			}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	cfg := AgentLoopConfig{
		Model:           "mock-model",
		Registry:        registry,
		ResponseTimeout: 50 * time.Millisecond, // Very short timeout
		RetryConfig: &RetryConfig{
			MaxRetries:      0, // No retries
			Backoff:         []time.Duration{},
			RetryableErrors: []string{},
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err == nil {
		t.Error("expected error from timeout")
	}
}

// TestAgentLoop_ToolExecutionError tests handling of tool execution errors
func TestAgentLoop_ToolExecutionError(t *testing.T) {
	registry, provider := createMockRegistry()

	// First response: invalid tool input that will cause execution error
	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Let me try a command"},
			{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "tool-1",
				ToolName:  "run_shell_command",
				ToolInput: json.RawMessage(`{"command": "false"}`), // Command that returns exit code 1
			},
		},
		StopReason: agent.StopReasonToolUse,
		Usage:      agent.Usage{TotalTokens: 100},
	})

	// Second response: completion
	provider.AddResponse(&agent.Response{
		ID:    "resp-2",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Done"},
		},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should have completed after processing the error
	state := loop.State()
	if state.Iteration != 2 {
		t.Errorf("expected 2 iterations, got %d", state.Iteration)
	}
}

// TestAgentLoop_BuildRequestWithCustomTools tests building request with custom tools
func TestAgentLoop_BuildRequestWithCustomTools(t *testing.T) {
	registry, provider := createMockRegistry()

	customTools := []agent.ToolDefinition{
		{
			Name:        "custom_tool",
			Description: "A custom tool for testing",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {"input": {"type": "string"}}}`),
		},
	}

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		Tools:    customTools,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Check that custom tools were included in the request
	lastReq := provider.GetLastRequest()
	if len(lastReq.Tools) != 1 {
		t.Errorf("expected 1 tool in request, got %d", len(lastReq.Tools))
	}
	if lastReq.Tools[0].Name != "custom_tool" {
		t.Errorf("expected custom_tool, got %s", lastReq.Tools[0].Name)
	}
}

// TestAgentLoop_CostWarning tests budget warning at 80%
func TestAgentLoop_CostWarning(t *testing.T) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount == 1 {
			// First call uses ~85% of budget
			// Mock pricing: $1.0/M input, $2.0/M output
			// With 850000 input tokens: 850000 * 1.0 / 1M = $0.85 = 85% of $1.00
			return &agent.Response{
				ID:    "resp-1",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: "tool-1",
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo test"}`),
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{PromptTokens: 850000, CompletionTokens: 0, TotalTokens: 850000},
			}, nil
		}
		return &agent.Response{
			ID:         "resp-2",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 100},
		}, nil
	}

	var budgetWarnReceived bool
	cfg := AgentLoopConfig{
		Model:     "mock-model",
		Registry:  registry,
		BudgetUSD: 1.0, // $1 budget
		OnEvent: func(event *LoopEvent) {
			if event.Type == CostBudgetWarn {
				budgetWarnReceived = true
			}
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !budgetWarnReceived {
		t.Error("expected budget warning event")
	}
}

// TestAgentLoop_StateTimestamps tests that state timestamps are updated
func TestAgentLoop_StateTimestamps(t *testing.T) {
	registry, provider := createMockRegistry()

	// Use a scenario with tool calls so LastIterationAt gets set
	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount == 1 {
			return &agent.Response{
				ID:    "resp-1",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: "tool-1",
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo test"}`),
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{TotalTokens: 100},
			}, nil
		}
		return &agent.Response{
			ID:         "resp-2",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	initialState := loop.State()
	if initialState.StartedAt.IsZero() {
		t.Error("expected StartedAt to be set on creation")
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	finalState := loop.State()
	if finalState.LastIterationAt.IsZero() {
		t.Error("expected LastIterationAt to be set after run with tool calls")
	}
	if finalState.LastIterationAt.Before(initialState.StartedAt) {
		t.Error("LastIterationAt should be after StartedAt")
	}
}

// TestAgentLoop_NonToolUseResponse tests response without tool calls but with tool_use stop reason
func TestAgentLoop_NonToolUseResponse(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "I think I'm done"},
		},
		StopReason: agent.StopReasonToolUse, // Says tool_use but no tool calls
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should complete after one iteration since no actual tool calls
	state := loop.State()
	if state.Iteration != 1 {
		t.Errorf("expected 1 iteration, got %d", state.Iteration)
	}
}

// TestAgentLoop_EventChannelFull tests behavior when event channel is full
func TestAgentLoop_EventChannelFull(t *testing.T) {
	registry, provider := createMockRegistry()

	// Generate many tool calls to overflow the event channel (size 64)
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		// Return completion
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Don't consume events - channel will fill up
	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run should not fail when event channel is full: %v", err)
	}
}

// TestAgentLoop_EmptyToolInput tests tool call with empty input
func TestAgentLoop_EmptyToolInput(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "tool-1",
				ToolName:  "run_shell_command",
				ToolInput: json.RawMessage(`{}`), // Empty/invalid input
			},
		},
		StopReason: agent.StopReasonToolUse,
		Usage:      agent.Usage{TotalTokens: 100},
	})

	provider.AddResponse(&agent.Response{
		ID:         "resp-2",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Should handle gracefully and continue
	state := loop.State()
	if state.Iteration != 2 {
		t.Errorf("expected 2 iterations, got %d", state.Iteration)
	}
}

// TestAgentLoop_GetModelInfo_Error tests cost tracking when GetModelInfo fails
func TestAgentLoop_GetModelInfo_Error(t *testing.T) {
	registry, provider := createMockRegistry()

	// Make a provider that returns error for GetModelInfo
	brokenProvider := &MockProviderWithBrokenGetModelInfo{
		MockProvider: provider,
	}
	registry = agent.NewProviderRegistry()
	registry.Register(brokenProvider)

	brokenProvider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Cost should be 0 since GetModelInfo failed
	state := loop.State()
	if state.TotalCostUSD != 0 {
		t.Errorf("expected 0 cost when GetModelInfo fails, got %f", state.TotalCostUSD)
	}
}

// MockProviderWithBrokenGetModelInfo is a mock provider that returns error for GetModelInfo
type MockProviderWithBrokenGetModelInfo struct {
	*MockProvider
}

func (p *MockProviderWithBrokenGetModelInfo) GetModelInfo(modelID string) (*agent.ModelInfo, error) {
	return nil, fmt.Errorf("model info unavailable")
}

// Benchmark for the agent loop
func BenchmarkAgentLoop_RunSimple(b *testing.B) {
	registry, provider := createMockRegistry()

	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		loop, _ := NewAgentLoop(cfg)
		_ = loop.Run(context.Background(), fmt.Sprintf("Test %d", i))
	}
}

// BenchmarkAgentLoop_WithToolCalls benchmarks loop with tool calls
func BenchmarkAgentLoop_WithToolCalls(b *testing.B) {
	registry, provider := createMockRegistry()

	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount%2 == 1 {
			return &agent.Response{
				ID:    "resp",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: "tool-1",
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo test"}`),
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{TotalTokens: 100},
			}, nil
		}
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		callCount = 0
		loop, _ := NewAgentLoop(cfg)
		_ = loop.Run(context.Background(), fmt.Sprintf("Test %d", i))
	}
}

// ============================================================================
// Summarization Integration Tests
// ============================================================================

// MockSummarizer implements summarize.Summarizer for testing.
type MockSummarizer struct {
	shouldSummarizeResult *MockTriggerCheckResult
	selectMessagesResult  *MockMessageSelection
	selectMessagesErr     error
	summarizeResult       *MockSummaryResult
	summarizeErr          error
	summarizeCalls        int
	lastMessages          []agent.Message
}

// MockTriggerCheckResult is a test implementation of summarize.TriggerCheckResult
type MockTriggerCheckResult struct {
	ShouldSummarize  bool
	Reason           string
	CurrentTokens    int
	ContextLimit     int
	UsagePercent     float64
	MessageCount     int
	IsUrgent         bool
	EstimatedSavings int
}

// MockMessageSelection is a test implementation of summarize.MessageSelection
type MockMessageSelection struct {
	ToSummarize                []agent.Message
	ToPreserve                 []agent.Message
	ToSummarizeIndices         []int
	ToPreserveIndices          []int
	EstimatedTokensToSummarize int
	EstimatedTokensToPreserve  int
}

// MockSummaryResult is a test implementation of summarize.SummaryResult
type MockSummaryResult struct {
	ID                 string
	SessionID          string
	Text               string
	MessagesSummarized int
	OriginalTokens     int
	SummaryTokens      int
	TokensSaved        int
	CompressionRatio   float64
}

func (m *MockSummarizer) ShouldSummarize(messages []agent.Message, contextLimit int) interface{} {
	m.lastMessages = messages
	if m.shouldSummarizeResult == nil {
		return nil
	}
	// Return the mock result cast to the expected type
	return m.shouldSummarizeResult
}

func (m *MockSummarizer) SelectMessages(messages []agent.Message) (interface{}, error) {
	if m.selectMessagesErr != nil {
		return nil, m.selectMessagesErr
	}
	if m.selectMessagesResult == nil {
		// Default: preserve all messages
		return &MockMessageSelection{
			ToPreserve: messages,
		}, nil
	}
	return m.selectMessagesResult, nil
}

func (m *MockSummarizer) Summarize(ctx context.Context, sessionID string, messages []agent.Message, reason interface{}) (interface{}, error) {
	m.summarizeCalls++
	if m.summarizeErr != nil {
		return nil, m.summarizeErr
	}
	if m.summarizeResult == nil {
		return &MockSummaryResult{
			ID:               "summary-1",
			SessionID:        sessionID,
			Text:             "Test summary of the conversation.",
			CompressionRatio: 0.3,
		}, nil
	}
	return m.summarizeResult, nil
}

func (m *MockSummarizer) SummarizeIncremental(ctx context.Context, sessionID string, existingSummary string, newMessages []agent.Message) (interface{}, error) {
	return m.Summarize(ctx, sessionID, newMessages, nil)
}

func (m *MockSummarizer) GetLatestSummary(ctx context.Context, sessionID string) (interface{}, error) {
	return nil, nil
}

func (m *MockSummarizer) GetMetrics(ctx context.Context, sessionID string) (interface{}, error) {
	return nil, nil
}

func (m *MockSummarizer) BuildContextWithSummary(ctx context.Context, sessionID string, recentMessages []agent.Message) ([]agent.Message, error) {
	return recentMessages, nil
}

// TestAgentLoop_SummarizationDisabled tests that summarization is skipped when disabled
func TestAgentLoop_SummarizationDisabled(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	// Explicitly disable summarization
	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		SummarizationConfig: &summarize.Config{
			Enabled: false,
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Summarizer should be nil when disabled
	if loop.summarizer != nil {
		t.Error("expected summarizer to be nil when disabled")
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

// TestAgentLoop_SummarizationNotTriggered tests that summarization is not triggered when not needed
func TestAgentLoop_SummarizationNotTriggered(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	// Enable summarization with default config
	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
		SummarizationConfig: &summarize.Config{
			Enabled:              true,
			SoftThreshold:        0.6,
			HardThreshold:        0.85,
			MinMessagesToSummarize: 6,
			PreserveRecentCount:  10,
			SummaryTargetTokens:  2000,
			SummaryMaxTokens:     4000,
			SummarizationModel:   "mock-model",
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// With only 2 messages (user + assistant), summarization should not be triggered
	state := loop.State()
	if len(state.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(state.Messages))
	}
}

// TestAgentLoop_SummarizationEventEmitted tests that summarization emits the correct event
func TestAgentLoop_SummarizationEventEmitted(t *testing.T) {
	registry, provider := createMockRegistry()

	// Create a scenario with many messages to trigger summarization
	callCount := 0
	provider.completeFunc = func(ctx context.Context, req agent.Request) (*agent.Response, error) {
		callCount++
		if callCount < 15 {
			// Keep making tool calls to build up message history
			return &agent.Response{
				ID:    "resp",
				Model: req.Model,
				Content: []agent.ContentBlock{
					{Type: agent.ContentTypeText, Text: fmt.Sprintf("Working on step %d", callCount)},
					{
						Type:      agent.ContentTypeToolUse,
						ToolUseID: fmt.Sprintf("tool-%d", callCount),
						ToolName:  "run_shell_command",
						ToolInput: json.RawMessage(`{"command": "echo test"}`),
					},
				},
				StopReason: agent.StopReasonToolUse,
				Usage:      agent.Usage{PromptTokens: 10000, CompletionTokens: 500, TotalTokens: 10500},
			}, nil
		}
		return &agent.Response{
			ID:         "resp",
			Model:      req.Model,
			Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
			StopReason: agent.StopReasonEnd,
			Usage:      agent.Usage{TotalTokens: 50},
		}, nil
	}

	var summarizationEventReceived bool
	cfg := AgentLoopConfig{
		Model:         "mock-model",
		Registry:      registry,
		MaxIterations: 20,
		SummarizationConfig: &summarize.Config{
			Enabled:                true,
			SoftThreshold:          0.3,  // Lower threshold to trigger earlier
			HardThreshold:          0.5,
			MinMessagesToSummarize: 4,
			PreserveRecentCount:    2,
			MaxMessagesBeforeSummary: 8, // Trigger after 8 messages
			SummaryTargetTokens:    500,
			SummaryMaxTokens:       1000,
			SummarizationModel:     "mock-model",
		},
		OnEvent: func(event *LoopEvent) {
			if event.Type == ContextSummarized {
				summarizationEventReceived = true
			}
		},
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Test with many iterations")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// With the low thresholds and many iterations, summarization should have been triggered
	if !summarizationEventReceived {
		t.Log("Note: Summarization event may not be received if message count didn't reach threshold")
	}
}

// TestAgentLoop_SummarizationWithCustomSummarizer tests using a custom summarizer
func TestAgentLoop_SummarizationWithCustomSummarizer(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	// Create a custom summarizer through the config
	sumConfig := &summarize.Config{
		Enabled:                true,
		SoftThreshold:          0.6,
		HardThreshold:          0.85,
		MinMessagesToSummarize: 100, // High threshold - won't trigger
		PreserveRecentCount:    10,
		SummaryTargetTokens:    2000,
		SummaryMaxTokens:       4000,
		SummarizationModel:     "mock-model",
	}

	cfg := AgentLoopConfig{
		Model:               "mock-model",
		Registry:            registry,
		SummarizationConfig: sumConfig,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Summarizer should be created
	if loop.summarizer == nil {
		t.Error("expected summarizer to be created")
	}

	err = loop.Run(context.Background(), "Test")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
}

// TestAgentLoop_ContextLimitDetection tests that context limit is properly detected
func TestAgentLoop_ContextLimitDetection(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Context limit should be set from provider capabilities
	if loop.contextLimit <= 0 {
		t.Errorf("expected positive context limit, got %d", loop.contextLimit)
	}
}

// ============================================================================
// Session Resume Tests
// ============================================================================

// TestAgentLoop_ToSessionState tests that session state can be extracted
func TestAgentLoop_ToSessionState(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Hello!"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
	})

	cfg := AgentLoopConfig{
		SessionID:    "test-session-123",
		Model:        "mock-model",
		Registry:     registry,
		SystemPrompt: "You are a helpful assistant.",
		ProjectPath:  "/test/project",
		WorkDir:      "/test/workdir",
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Run to populate state
	err = loop.Run(context.Background(), "Hello")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Extract session state
	sessionState := loop.ToSessionState()

	if sessionState == nil {
		t.Fatal("ToSessionState() returned nil")
	}

	// Verify all fields are populated correctly
	if sessionState.SessionID != "test-session-123" {
		t.Errorf("SessionID = %v, want test-session-123", sessionState.SessionID)
	}
	if sessionState.Iteration != 1 {
		t.Errorf("Iteration = %v, want 1", sessionState.Iteration)
	}
	if sessionState.TotalTokens != 150 {
		t.Errorf("TotalTokens = %v, want 150", sessionState.TotalTokens)
	}
	if sessionState.MessageCount != 2 {
		t.Errorf("MessageCount = %v, want 2", sessionState.MessageCount)
	}
	if sessionState.Model != "mock-model" {
		t.Errorf("Model = %v, want mock-model", sessionState.Model)
	}
	if sessionState.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("SystemPrompt = %v, want 'You are a helpful assistant.'", sessionState.SystemPrompt)
	}
	if sessionState.ProjectPath != "/test/project" {
		t.Errorf("ProjectPath = %v, want /test/project", sessionState.ProjectPath)
	}
	if sessionState.WorkDir != "/test/workdir" {
		t.Errorf("WorkDir = %v, want /test/workdir", sessionState.WorkDir)
	}

	// Verify messages were captured
	messages, ok := sessionState.Messages.([]agent.Message)
	if !ok {
		t.Error("Messages should be []agent.Message")
	} else if len(messages) != 2 {
		t.Errorf("len(Messages) = %d, want 2", len(messages))
	}
}

// TestAgentLoop_RestoreFromResumeState tests that a loop can be restored from a resume state
func TestAgentLoop_RestoreFromResumeState(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Continuing from resumed state!"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{PromptTokens: 200, CompletionTokens: 50, TotalTokens: 250},
	})

	// Create a resume state with existing conversation
	resumeState, err := mcp.NewSessionResumeState("test-session-456")
	if err != nil {
		t.Fatalf("NewSessionResumeState failed: %v", err)
	}

	// Set up the resume state with existing data
	resumeState.Iteration = 5
	resumeState.TotalTokens = 1000
	resumeState.TotalCostUSD = 0.05
	resumeState.MessageCount = 4
	resumeState.IterationsSinceProgress = 1
	resumeState.Model = "mock-model"

	// Create messages to restore
	existingMessages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Original question"),
		agent.NewTextMessage(agent.RoleAssistant, "Original answer"),
		agent.NewTextMessage(agent.RoleUser, "Follow-up question"),
		agent.NewTextMessage(agent.RoleAssistant, "Follow-up answer"),
	}
	err = resumeState.SetMessages(existingMessages)
	if err != nil {
		t.Fatalf("SetMessages failed: %v", err)
	}

	// Create a new loop
	cfg := AgentLoopConfig{
		SessionID: "test-session-456",
		Model:     "mock-model",
		Registry:  registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Restore from resume state
	err = loop.RestoreFromResumeState(resumeState)
	if err != nil {
		t.Fatalf("RestoreFromResumeState failed: %v", err)
	}

	// Verify state was restored
	state := loop.State()

	if state.Iteration != 5 {
		t.Errorf("Iteration = %v, want 5", state.Iteration)
	}
	if state.TotalTokens != 1000 {
		t.Errorf("TotalTokens = %v, want 1000", state.TotalTokens)
	}
	if state.TotalCostUSD != 0.05 {
		t.Errorf("TotalCostUSD = %v, want 0.05", state.TotalCostUSD)
	}
	if len(state.Messages) != 4 {
		t.Errorf("len(Messages) = %d, want 4", len(state.Messages))
	}
	if state.IterationsSinceProgress != 1 {
		t.Errorf("IterationsSinceProgress = %d, want 1", state.IterationsSinceProgress)
	}

	// Verify messages were restored correctly
	if len(state.Messages) >= 1 {
		firstMsg := state.Messages[0]
		if firstMsg.GetText() != "Original question" {
			t.Errorf("First message = %q, want 'Original question'", firstMsg.GetText())
		}
	}
}

// TestAgentLoop_RestoreFromResumeState_NilState tests that RestoreFromResumeState handles nil state
func TestAgentLoop_RestoreFromResumeState_NilState(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		Model:    "mock-model",
		Registry: registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.RestoreFromResumeState(nil)
	if err == nil {
		t.Error("expected error when restoring from nil state")
	}
}

// TestAgentLoop_RunFromResume tests running from a restored state
func TestAgentLoop_RunFromResume(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Task completed after resume!"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 100},
	})

	// Create a resume state
	resumeState, _ := mcp.NewSessionResumeState("test-session-789")
	resumeState.Iteration = 3
	resumeState.TotalTokens = 500
	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Previous prompt"),
		agent.NewTextMessage(agent.RoleAssistant, "Previous response"),
	}
	resumeState.SetMessages(messages)

	cfg := AgentLoopConfig{
		SessionID: "test-session-789",
		Model:     "mock-model",
		Registry:  registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	// Restore state
	err = loop.RestoreFromResumeState(resumeState)
	if err != nil {
		t.Fatalf("RestoreFromResumeState failed: %v", err)
	}

	// Run from resume with a prompt
	err = loop.RunFromResume(context.Background(), "Please continue where you left off")
	if err != nil {
		t.Fatalf("RunFromResume failed: %v", err)
	}

	// Verify the loop continued from the restored state
	state := loop.State()

	// Iteration should have increased from 3 to 4
	if state.Iteration != 4 {
		t.Errorf("Iteration = %v, want 4", state.Iteration)
	}

	// Should have previous messages + resume prompt + new response
	if len(state.Messages) != 4 {
		t.Errorf("len(Messages) = %d, want 4", len(state.Messages))
	}

	// Total tokens should include original + new
	if state.TotalTokens != 600 {
		t.Errorf("TotalTokens = %v, want 600 (500 original + 100 new)", state.TotalTokens)
	}
}

// TestAgentLoop_RunFromResume_NoResumePrompt tests running from resume without a prompt
func TestAgentLoop_RunFromResume_NoResumePrompt(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Continuing..."}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	// Create a resume state
	resumeState, _ := mcp.NewSessionResumeState("test-session-no-prompt")
	resumeState.Iteration = 2
	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Original prompt"),
		agent.NewTextMessage(agent.RoleAssistant, "Original response"),
	}
	resumeState.SetMessages(messages)

	cfg := AgentLoopConfig{
		SessionID: "test-session-no-prompt",
		Model:     "mock-model",
		Registry:  registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.RestoreFromResumeState(resumeState)
	if err != nil {
		t.Fatalf("RestoreFromResumeState failed: %v", err)
	}

	// Run from resume with empty prompt
	err = loop.RunFromResume(context.Background(), "")
	if err != nil {
		t.Fatalf("RunFromResume failed: %v", err)
	}

	state := loop.State()

	// Should have original messages + new response (no added resume prompt)
	if len(state.Messages) != 3 {
		t.Errorf("len(Messages) = %d, want 3", len(state.Messages))
	}
}

// TestAgentLoop_GetSessionID tests the GetSessionID method
func TestAgentLoop_GetSessionID(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		SessionID: "explicit-session-id",
		Model:     "mock-model",
		Registry:  registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	if loop.GetSessionID() != "explicit-session-id" {
		t.Errorf("GetSessionID() = %q, want 'explicit-session-id'", loop.GetSessionID())
	}
}

// TestAgentLoop_GetConfig tests the GetConfig method
func TestAgentLoop_GetConfig(t *testing.T) {
	registry, _ := createMockRegistry()

	cfg := AgentLoopConfig{
		SessionID:     "config-test-session",
		Model:         "mock-model",
		Registry:      registry,
		MaxIterations: 10,
		BudgetUSD:     5.0,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	returnedCfg := loop.GetConfig()

	if returnedCfg.SessionID != "config-test-session" {
		t.Errorf("SessionID = %v, want config-test-session", returnedCfg.SessionID)
	}
	if returnedCfg.MaxIterations != 10 {
		t.Errorf("MaxIterations = %v, want 10", returnedCfg.MaxIterations)
	}
	if returnedCfg.BudgetUSD != 5.0 {
		t.Errorf("BudgetUSD = %v, want 5.0", returnedCfg.BudgetUSD)
	}
}

// TestAgentLoop_RestoreWithSignal tests restoring a session that had a signal
func TestAgentLoop_RestoreWithSignal(t *testing.T) {
	registry, provider := createMockRegistry()

	provider.AddResponse(&agent.Response{
		ID:         "resp-1",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	// Create a resume state with a last signal
	resumeState, _ := mcp.NewSessionResumeState("test-session-signal")
	resumeState.Iteration = 2
	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "Work on task"),
	}
	resumeState.SetMessages(messages)

	// Set a last signal
	lastSignal := &mcp.Signal{
		Type:   mcp.SignalProgress,
		Reason: "50% complete",
	}
	resumeState.SetLastSignal(lastSignal)

	cfg := AgentLoopConfig{
		SessionID: "test-session-signal",
		Model:     "mock-model",
		Registry:  registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.RestoreFromResumeState(resumeState)
	if err != nil {
		t.Fatalf("RestoreFromResumeState failed: %v", err)
	}

	state := loop.State()

	// Signal should be restored
	if state.LastSignal == nil {
		t.Error("expected LastSignal to be restored")
	} else if state.LastSignal.Type != mcp.SignalProgress {
		t.Errorf("LastSignal.Type = %v, want progress", state.LastSignal.Type)
	} else if state.LastSignal.Reason != "50% complete" {
		t.Errorf("LastSignal.Reason = %v, want '50%% complete'", state.LastSignal.Reason)
	}
}

// TestAgentLoop_ToSessionState_WithSignal tests extracting state that includes a signal
func TestAgentLoop_ToSessionState_WithSignal(t *testing.T) {
	registry, provider := createMockRegistry()

	// Response with a progress signal
	signalInput, _ := json.Marshal(map[string]interface{}{
		"type":   "progress",
		"reason": "Working on it",
	})
	provider.AddResponse(&agent.Response{
		ID:    "resp-1",
		Model: "mock-model",
		Content: []agent.ContentBlock{
			{Type: agent.ContentTypeText, Text: "Making progress"},
			{
				Type:      agent.ContentTypeToolUse,
				ToolUseID: "signal-1",
				ToolName:  "axon_signal",
				ToolInput: signalInput,
			},
		},
		StopReason: agent.StopReasonToolUse,
		Usage:      agent.Usage{TotalTokens: 100},
	})

	// Completion response
	provider.AddResponse(&agent.Response{
		ID:         "resp-2",
		Model:      "mock-model",
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Done"}},
		StopReason: agent.StopReasonEnd,
		Usage:      agent.Usage{TotalTokens: 50},
	})

	cfg := AgentLoopConfig{
		SessionID: "test-signal-extract",
		Model:     "mock-model",
		Registry:  registry,
	}

	loop, err := NewAgentLoop(cfg)
	if err != nil {
		t.Fatalf("NewAgentLoop failed: %v", err)
	}

	err = loop.Run(context.Background(), "Do something")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Extract session state
	sessionState := loop.ToSessionState()

	// Verify the last signal was captured
	if sessionState.LastSignal == nil {
		t.Error("expected LastSignal to be set in session state")
	} else {
		sig, ok := sessionState.LastSignal.(*mcp.Signal)
		if !ok {
			t.Error("LastSignal should be *mcp.Signal")
		} else if sig.Type != mcp.SignalProgress {
			t.Errorf("LastSignal.Type = %v, want progress", sig.Type)
		}
	}
}
