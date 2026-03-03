// Package loop provides the agent loop execution engine.
// This file implements the Agent Loop Core that orchestrates LLM interactions
// with tool execution in a turn-based conversation loop.
package loop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"github.com/openexec/openexec/pkg/agent"
	octx "github.com/openexec/openexec/internal/context"
	"github.com/openexec/openexec/internal/mcp"
	"github.com/openexec/openexec/internal/summarize"
)

// Common errors for agent loop operations.
var (
	ErrLoopStopped       = errors.New("agent loop stopped")
	ErrLoopPaused        = errors.New("agent loop paused")
	ErrMaxIterations     = errors.New("max iterations reached")
	ErrMaxTokens         = errors.New("max tokens exceeded")
	ErrBudgetExceeded    = errors.New("budget exceeded")
	ErrContextOverflow   = errors.New("context window overflow")
	ErrProviderError     = errors.New("provider error")
	ErrToolExecutionFail = errors.New("tool execution failed")
	ErrNoProvider        = errors.New("no provider available")
)

// AgentLoopConfig configures the agent loop behavior.
type AgentLoopConfig struct {
	// SessionID is the unique identifier for this session.
	SessionID string

	// Model is the model identifier to use (e.g., "claude-3-opus").
	Model string

	// SystemPrompt is the system prompt/instruction for the agent.
	SystemPrompt string

	// MaxIterations is the maximum number of conversation turns.
	// 0 means unlimited.
	MaxIterations int

	// MaxTokens is the maximum total tokens to consume.
	// 0 means unlimited.
	MaxTokens int

	// BudgetUSD is the maximum cost in USD.
	// 0 means unlimited.
	BudgetUSD float64

	// ProjectPath is the path for auto-context gathering.
	ProjectPath string

	// WorkDir is the working directory for tool execution.
	WorkDir string

	// AutoContext enables automatic context injection.
	AutoContext bool

	// ContextBudget is the token budget for auto-context.
	// If nil, default budget is used.
	ContextBudget *octx.ContextBudget

	// Tools is the list of available tool definitions.
	// If nil, default executor tools are used.
	Tools []agent.ToolDefinition

	// Registry is the provider registry to use.
	// If nil, the default registry is used.
	Registry *agent.ProviderRegistry

	// Executor is the tool executor to use.
	// If nil, a default executor is created.
	Executor *Executor

	// OnEvent is called for each loop event.
	// May be nil for no event handling.
	OnEvent func(*LoopEvent)

	// RetryConfig controls retry behavior for transient failures.
	RetryConfig *RetryConfig

	// ThrashThreshold is the number of iterations without progress
	// before stopping. 0 disables thrashing detection.
	ThrashThreshold int

	// ResponseTimeout is the timeout for each LLM request.
	// 0 means no timeout.
	ResponseTimeout time.Duration

	// SummarizationConfig configures automatic session history summarization.
	// If nil, default summarization config is used. Set Enabled=false to disable.
	SummarizationConfig *summarize.Config

	// Summarizer is the summarizer to use for context management.
	// If nil and SummarizationConfig.Enabled is true, a default summarizer is created.
	Summarizer summarize.Summarizer
}

// RetryConfig controls retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retries for transient errors.
	MaxRetries int

	// Backoff is the sequence of delays between retries.
	Backoff []time.Duration

	// RetryableErrors is a list of error codes that should be retried.
	RetryableErrors []string
}

// DefaultRetryConfig returns sensible retry defaults.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries: 3,
		Backoff:    []time.Duration{time.Second, 5 * time.Second, 15 * time.Second},
		RetryableErrors: []string{
			agent.ErrCodeRateLimit,
			agent.ErrCodeServerError,
			agent.ErrCodeTimeout,
		},
	}
}

// AgentLoopState tracks the current state of the loop.
type AgentLoopState struct {
	// Iteration is the current iteration number.
	Iteration int `json:"iteration"`

	// TotalTokens is the total tokens consumed so far.
	TotalTokens int `json:"total_tokens"`

	// TotalCostUSD is the total cost in USD.
	TotalCostUSD float64 `json:"total_cost_usd"`

	// Messages is the conversation history.
	Messages []agent.Message `json:"messages"`

	// LastSignal is the last signal received from the agent.
	LastSignal *mcp.Signal `json:"last_signal,omitempty"`

	// IterationsSinceProgress tracks iterations without a progress signal.
	IterationsSinceProgress int `json:"iterations_since_progress"`

	// StartedAt is when the loop started.
	StartedAt time.Time `json:"started_at"`

	// LastIterationAt is when the last iteration completed.
	LastIterationAt time.Time `json:"last_iteration_at"`
}

// AgentLoop is the core agent loop that orchestrates LLM interactions
// with tool execution.
type AgentLoop struct {
	cfg        AgentLoopConfig
	state      AgentLoopState
	provider   agent.ProviderAdapter
	executor   *Executor
	summarizer summarize.Summarizer
	eventCh    chan *LoopEvent

	// contextLimit caches the model's context window size
	contextLimit int

	mu      sync.Mutex
	paused  atomic.Bool
	stopped atomic.Bool
	cancel  context.CancelFunc
}

// NewAgentLoop creates a new agent loop with the given configuration.
func NewAgentLoop(cfg AgentLoopConfig) (*AgentLoop, error) {
	if cfg.SessionID == "" {
		cfg.SessionID = uuid.New().String()
	}

	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	if cfg.RetryConfig == nil {
		cfg.RetryConfig = DefaultRetryConfig()
	}

	// Get provider from registry
	registry := cfg.Registry
	if registry == nil {
		registry = agent.DefaultRegistry
	}

	provider, err := registry.GetForModel(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider for model %s: %w", cfg.Model, err)
	}

	// Create or use provided executor
	executor := cfg.Executor
	if executor == nil {
		executor = NewExecutor(ExecutorConfig{
			WorkDir:     cfg.WorkDir,
			SessionID:   cfg.SessionID,
			ProjectPath: cfg.ProjectPath,
			EventCallback: func(event *LoopEvent) {
				if cfg.OnEvent != nil {
					cfg.OnEvent(event)
				}
			},
		})
	}

	// Get context window limit from provider
	var contextLimit int
	if caps, err := provider.GetCapabilities(cfg.Model); err == nil && caps != nil {
		contextLimit = caps.MaxContextTokens
	}
	if contextLimit == 0 {
		// Default to a conservative value if not available
		contextLimit = 128000
	}

	// Initialize summarizer if enabled
	var summarizer summarize.Summarizer
	if cfg.Summarizer != nil {
		summarizer = cfg.Summarizer
	} else if cfg.SummarizationConfig == nil || cfg.SummarizationConfig.Enabled {
		// Create default summarizer if not disabled
		sumConfig := cfg.SummarizationConfig
		if sumConfig == nil {
			sumConfig = summarize.DefaultConfig()
		}
		// Only create summarizer if enabled
		if sumConfig.Enabled {
			summarizer, err = summarize.NewSessionSummarizer(sumConfig, provider, nil)
			if err != nil {
				// Log warning but continue without summarization
				summarizer = nil
			}
		}
	}

	loop := &AgentLoop{
		cfg:          cfg,
		provider:     provider,
		executor:     executor,
		summarizer:   summarizer,
		contextLimit: contextLimit,
		eventCh:      make(chan *LoopEvent, 64),
		state: AgentLoopState{
			Messages:  make([]agent.Message, 0),
			StartedAt: time.Now(),
		},
	}

	return loop, nil
}

// Events returns a read-only channel of loop events.
// The channel is closed when the loop finishes.
func (l *AgentLoop) Events() <-chan *LoopEvent {
	return l.eventCh
}

// State returns a copy of the current loop state.
func (l *AgentLoop) State() AgentLoopState {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.state
}

// Run executes the agent loop until completion, stop, or error.
func (l *AgentLoop) Run(ctx context.Context, initialPrompt string) error {
	defer close(l.eventCh)

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	defer cancel()

	// Emit loop start event
	l.emitEvent(LoopEventStart, "", nil)

	// Build initial messages
	if err := l.initializeMessages(ctx, initialPrompt); err != nil {
		l.emitEvent(LoopEventError, err.Error(), err)
		return err
	}

	// Main loop
	for {
		// Check lifecycle state
		if l.stopped.Load() {
			l.emitEvent(LoopEventStop, "loop stopped by user", nil)
			return ErrLoopStopped
		}

		if l.paused.Load() {
			l.emitEvent(LoopEventPause, "loop paused", nil)
			return ErrLoopPaused
		}

		// Check context cancellation
		if err := ctx.Err(); err != nil {
			l.emitEvent(LoopEventError, "context cancelled", err)
			return err
		}

		// Check iteration limit
		if l.cfg.MaxIterations > 0 && l.state.Iteration >= l.cfg.MaxIterations {
			l.emitEvent(LoopEventMaxReached, fmt.Sprintf("max iterations %d reached", l.cfg.MaxIterations), nil)
			return ErrMaxIterations
		}

		// Check thrashing
		if l.cfg.ThrashThreshold > 0 && l.state.IterationsSinceProgress >= l.cfg.ThrashThreshold {
			l.emitEvent(ThrashingDetected, fmt.Sprintf("no progress signal in %d iterations", l.cfg.ThrashThreshold), nil)
			return fmt.Errorf("thrashing detected: no progress in %d iterations", l.cfg.ThrashThreshold)
		}

		// Run one iteration
		completed, err := l.runIteration(ctx)
		if err != nil {
			l.emitEvent(LoopEventError, err.Error(), err)
			return err
		}

		if completed {
			l.emitEvent(LoopEventComplete, "task completed", nil)
			return nil
		}
	}
}

// initializeMessages sets up the initial conversation with optional auto-context.
func (l *AgentLoop) initializeMessages(ctx context.Context, initialPrompt string) error {
	// Build auto-context if enabled
	var contextStr string
	if l.cfg.AutoContext && l.cfg.ProjectPath != "" {
		result, err := l.buildContext(ctx)
		if err != nil {
			// Non-fatal, just log and continue without context
			l.emitEvent(ContextInjected, fmt.Sprintf("context build failed: %v", err), err)
		} else if result != nil && result.Context != "" {
			contextStr = result.Context
			l.emitEvent(ContextInjected, fmt.Sprintf("injected %d tokens of context", result.TotalTokens), nil)
		}
	}

	// Build the initial user message
	var fullPrompt string
	if contextStr != "" {
		fullPrompt = fmt.Sprintf("## Context\n\n%s\n\n---\n\n## Task\n\n%s", contextStr, initialPrompt)
	} else {
		fullPrompt = initialPrompt
	}

	// Add initial user message
	l.state.Messages = append(l.state.Messages, agent.NewTextMessage(agent.RoleUser, fullPrompt))

	l.emitEvent(MessageUser, "", nil)

	return nil
}

// buildContext gathers auto-context for the conversation.
func (l *AgentLoop) buildContext(ctx context.Context) (*octx.BuilderResult, error) {
	opts := octx.DefaultBuilderOptions()
	if l.cfg.ContextBudget != nil {
		opts.Budget = l.cfg.ContextBudget
	}

	return octx.BuildContext(ctx, l.cfg.ProjectPath, opts)
}

// runIteration executes a single conversation turn.
func (l *AgentLoop) runIteration(ctx context.Context) (bool, error) {
	l.state.Iteration++
	l.emitEvent(IterationStart, fmt.Sprintf("iteration %d", l.state.Iteration), nil)

	// Check if summarization is needed before building request
	if err := l.checkAndSummarize(ctx); err != nil {
		// Log but don't fail - summarization is not critical
		l.emitEvent(ContextSummarized, fmt.Sprintf("summarization failed: %v", err), err)
	}

	// Build request
	req := l.buildRequest()

	// Make LLM request with retries
	l.emitEvent(LLMRequestStart, "", nil)
	startTime := time.Now()

	response, err := l.completeWithRetry(ctx, req)
	if err != nil {
		l.emitEvent(LLMError, err.Error(), err)
		return false, fmt.Errorf("LLM request failed: %w", err)
	}

	duration := time.Since(startTime)
	l.emitEvent(LLMRequestEnd, fmt.Sprintf("completed in %v", duration), nil)

	// Update token counts and cost
	l.updateUsage(response.Usage)

	// Check budget limits
	if err := l.checkBudget(); err != nil {
		return false, err
	}

	// Add assistant response to history
	assistantMsg := agent.Message{
		Role:    agent.RoleAssistant,
		Content: response.Content,
	}
	l.state.Messages = append(l.state.Messages, assistantMsg)

	// Emit assistant message event
	l.emitEvent(MessageAssistant, response.GetText(), nil)

	// Check stop reason
	if response.StopReason == agent.StopReasonEnd {
		// Model finished naturally without tool use
		return true, nil
	}

	if response.StopReason == agent.StopReasonMaxTokens {
		// Hit token limit - may need to continue
		l.emitEvent(LLMContextWindow, "max tokens reached in response", nil)
	}

	// Process tool calls if any
	toolCalls := response.GetToolCalls()
	if len(toolCalls) == 0 {
		// No tool calls and stop reason wasn't end_turn - treat as complete
		return true, nil
	}

	// Execute tool calls and check for completion signals
	completed, err := l.processToolCalls(ctx, toolCalls)
	if err != nil {
		return false, err
	}

	// Update last iteration time
	l.state.LastIterationAt = time.Now()

	// Increment iterations without progress (reset if we received a progress signal)
	if l.state.LastSignal != nil && (l.state.LastSignal.Type == mcp.SignalProgress ||
		l.state.LastSignal.Type == mcp.SignalPhaseComplete) {
		l.state.IterationsSinceProgress = 0
	} else {
		l.state.IterationsSinceProgress++
	}

	l.emitEvent(IterationComplete, fmt.Sprintf("iteration %d complete", l.state.Iteration), nil)

	return completed, nil
}

// buildRequest constructs an LLM request from current state.
func (l *AgentLoop) buildRequest() agent.Request {
	// Get tool definitions
	var tools []agent.ToolDefinition
	if l.cfg.Tools != nil {
		tools = l.cfg.Tools
	} else if l.executor != nil {
		// Convert executor tool definitions to agent.ToolDefinition
		defs := l.executor.GetToolDefinitions()
		tools = make([]agent.ToolDefinition, len(defs))
		for i, def := range defs {
			name, _ := def["name"].(string)
			desc, _ := def["description"].(string)
			schema, _ := def["inputSchema"].(map[string]interface{})

			schemaBytes, _ := json.Marshal(schema)
			tools[i] = agent.ToolDefinition{
				Name:        name,
				Description: desc,
				InputSchema: schemaBytes,
			}
		}
	}

	return agent.Request{
		Model:    l.cfg.Model,
		Messages: l.state.Messages,
		System:   l.cfg.SystemPrompt,
		Tools:    tools,
	}
}

// completeWithRetry makes an LLM request with retry logic.
func (l *AgentLoop) completeWithRetry(ctx context.Context, req agent.Request) (*agent.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= l.cfg.RetryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			// Calculate backoff
			backoffIdx := attempt - 1
			if backoffIdx >= len(l.cfg.RetryConfig.Backoff) {
				backoffIdx = len(l.cfg.RetryConfig.Backoff) - 1
			}
			backoff := l.cfg.RetryConfig.Backoff[backoffIdx]

			l.emitEvent(IterationRetry, fmt.Sprintf("retry %d after %v", attempt, backoff), lastErr)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		// Apply timeout if configured
		reqCtx := ctx
		if l.cfg.ResponseTimeout > 0 {
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(ctx, l.cfg.ResponseTimeout)
			defer cancel()
		}

		response, err := l.provider.Complete(reqCtx, req)
		if err == nil {
			return response, nil
		}

		lastErr = err

		// Check if error is retryable
		if !l.isRetryable(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// isRetryable checks if an error should be retried.
func (l *AgentLoop) isRetryable(err error) bool {
	var provErr *agent.ProviderError
	if !errors.As(err, &provErr) {
		return false
	}

	if provErr.Retryable {
		return true
	}

	for _, code := range l.cfg.RetryConfig.RetryableErrors {
		if provErr.Code == code {
			return true
		}
	}

	return false
}

// updateUsage updates token and cost tracking.
func (l *AgentLoop) updateUsage(usage agent.Usage) {
	l.state.TotalTokens += usage.TotalTokens

	// Estimate cost based on model pricing
	modelInfo, err := l.provider.GetModelInfo(l.cfg.Model)
	if err == nil && modelInfo != nil {
		inputCost := float64(usage.PromptTokens) * modelInfo.PricePerMInputTokens / 1_000_000
		outputCost := float64(usage.CompletionTokens) * modelInfo.PricePerMOutputTokens / 1_000_000
		l.state.TotalCostUSD += inputCost + outputCost
	}

	// Emit cost event
	l.emitEvent(CostUpdated, fmt.Sprintf("total: $%.4f, tokens: %d", l.state.TotalCostUSD, l.state.TotalTokens), nil)
}

// checkBudget verifies we haven't exceeded token or cost limits.
func (l *AgentLoop) checkBudget() error {
	if l.cfg.MaxTokens > 0 && l.state.TotalTokens >= l.cfg.MaxTokens {
		l.emitEvent(CostBudgetExceeded, fmt.Sprintf("max tokens %d exceeded", l.cfg.MaxTokens), nil)
		return ErrMaxTokens
	}

	if l.cfg.BudgetUSD > 0 && l.state.TotalCostUSD >= l.cfg.BudgetUSD {
		l.emitEvent(CostBudgetExceeded, fmt.Sprintf("budget $%.2f exceeded", l.cfg.BudgetUSD), nil)
		return ErrBudgetExceeded
	}

	// Emit warning at 80% budget
	if l.cfg.BudgetUSD > 0 && l.state.TotalCostUSD >= l.cfg.BudgetUSD*0.8 {
		l.emitEvent(CostBudgetWarn, fmt.Sprintf("approaching budget limit: $%.4f of $%.2f", l.state.TotalCostUSD, l.cfg.BudgetUSD), nil)
	}

	return nil
}

// checkAndSummarize checks if the conversation history needs summarization
// and performs it if necessary. This prevents context window overflow.
func (l *AgentLoop) checkAndSummarize(ctx context.Context) error {
	// Skip if summarizer is not available
	if l.summarizer == nil {
		return nil
	}

	// Get current messages
	l.mu.Lock()
	messages := make([]agent.Message, len(l.state.Messages))
	copy(messages, l.state.Messages)
	l.mu.Unlock()

	// Check if summarization is needed
	checkResult := l.summarizer.ShouldSummarize(messages, l.contextLimit)
	if checkResult == nil || !checkResult.ShouldSummarize {
		return nil
	}

	// Select messages for summarization
	selection, err := l.summarizer.SelectMessages(messages)
	if err != nil {
		return fmt.Errorf("message selection failed: %w", err)
	}

	// Perform summarization on the selected messages
	result, err := l.summarizer.Summarize(ctx, l.cfg.SessionID, messages, checkResult.Reason)
	if err != nil {
		return fmt.Errorf("summarization failed: %w", err)
	}

	// Build new message list:
	// 1. A system message containing the summary
	// 2. The preserved recent messages
	var newMessages []agent.Message

	// Add summary as a system message if we have one
	if result.Text != "" {
		summaryMsg := agent.Message{
			Role: agent.RoleSystem,
			Content: []agent.ContentBlock{
				{
					Type: agent.ContentTypeText,
					Text: fmt.Sprintf("[SESSION SUMMARY]\n\nThe following is a summary of the earlier conversation that was condensed to manage context length:\n\n%s\n\n[END OF SUMMARY]\n\nContinue the conversation from here:", result.Text),
				},
			},
		}
		newMessages = append(newMessages, summaryMsg)
	}

	// Add the preserved messages
	newMessages = append(newMessages, selection.ToPreserve...)

	// Update state with new messages
	l.mu.Lock()
	l.state.Messages = newMessages
	l.mu.Unlock()

	// Emit summarization event with context info
	l.emitSummarizationEvent(result, checkResult)

	return nil
}

// emitSummarizationEvent emits an event when summarization occurs.
func (l *AgentLoop) emitSummarizationEvent(result *summarize.SummaryResult, trigger *summarize.TriggerCheckResult) {
	builder, err := NewLoopEvent(ContextSummarized)
	if err != nil {
		return
	}

	msg := fmt.Sprintf("summarized %d messages, saved %d tokens (%.1f%% compression)",
		result.MessagesSummarized, result.TokensSaved, result.CompressionRatio*100)

	contextInfo := &ContextInfo{
		TokenCount:      trigger.CurrentTokens - result.TokensSaved,
		MaxTokens:       trigger.ContextLimit,
		UsagePercent:    float64(trigger.CurrentTokens-result.TokensSaved) / float64(trigger.ContextLimit) * 100,
		WasSummarized:   true,
		TruncatedTokens: result.TokensSaved,
	}

	event, err := builder.
		WithSession(l.cfg.SessionID).
		WithIteration(l.state.Iteration).
		WithMessage(msg).
		WithContext(contextInfo).
		Build()

	if err != nil {
		return
	}

	select {
	case l.eventCh <- event:
	default:
	}

	if l.cfg.OnEvent != nil {
		l.cfg.OnEvent(event)
	}
}

// processToolCalls executes tool calls and adds results to conversation.
func (l *AgentLoop) processToolCalls(ctx context.Context, toolCalls []agent.ContentBlock) (bool, error) {
	var toolResults []agent.ContentBlock
	var completed bool

	for _, tc := range toolCalls {
		// Check if this is a signal
		if IsAxonSignal(tc) {
			sig, err := GetSignalFromToolCall(tc)
			if err == nil {
				l.state.LastSignal = sig
				l.emitSignalEvent(sig)

				// Check for completion signals
				if sig.Type == mcp.SignalPhaseComplete ||
					sig.Type == mcp.SignalRoute {
					completed = true
				}
			}
		}

		// Execute the tool
		result, err := l.executor.Execute(ctx, tc)
		if err != nil {
			// Convert error to tool result
			result = &ToolResult{
				Output:  fmt.Sprintf("Tool execution error: %v", err),
				IsError: true,
			}
		}

		// Convert to content block
		resultBlock := l.executor.ToContentBlock(tc.ToolUseID, result)
		toolResults = append(toolResults, resultBlock)
	}

	// Add tool results as a user message (tool results are sent as user role)
	if len(toolResults) > 0 {
		toolMsg := agent.Message{
			Role:    agent.RoleUser,
			Content: toolResults,
		}
		l.state.Messages = append(l.state.Messages, toolMsg)

		l.emitEvent(ToolResultSent, fmt.Sprintf("sent %d tool results", len(toolResults)), nil)
	}

	return completed, nil
}

// emitEvent sends a loop event.
func (l *AgentLoop) emitEvent(eventType LoopEventType, message string, err error) {
	builder, buildErr := NewLoopEvent(eventType)
	if buildErr != nil {
		return
	}

	builder.WithSession(l.cfg.SessionID).
		WithIteration(l.state.Iteration).
		WithMessage(message)

	if err != nil {
		builder.WithError(err)
	}

	event, buildErr := builder.Build()
	if buildErr != nil {
		return
	}

	// Send to channel (non-blocking)
	select {
	case l.eventCh <- event:
	default:
		// Channel full, drop event
	}

	// Also call callback if provided
	if l.cfg.OnEvent != nil {
		l.cfg.OnEvent(event)
	}
}

// emitSignalEvent emits a signal-specific event.
func (l *AgentLoop) emitSignalEvent(sig *mcp.Signal) {
	builder, err := NewLoopEvent(SignalReceived)
	if err != nil {
		return
	}

	event, err := builder.
		WithSession(l.cfg.SessionID).
		WithIteration(l.state.Iteration).
		WithMessage(fmt.Sprintf("signal: %s", sig.Type)).
		WithSignal(&SignalInfo{
			SignalType: string(sig.Type),
			Target:     sig.Target,
			Reason:     sig.Reason,
			Metadata:   sig.Metadata,
		}).
		Build()

	if err != nil {
		return
	}

	select {
	case l.eventCh <- event:
	default:
	}

	if l.cfg.OnEvent != nil {
		l.cfg.OnEvent(event)
	}

	// Emit specific event type for phase complete
	if sig.Type == mcp.SignalPhaseComplete {
		l.emitEvent(SignalPhaseComplete, sig.Reason, nil)
	}
}

// Pause signals the loop to pause after the current iteration.
func (l *AgentLoop) Pause() {
	l.paused.Store(true)
}

// Resume resumes a paused loop.
func (l *AgentLoop) Resume() {
	l.paused.Store(false)
}

// Stop signals the loop to stop immediately.
func (l *AgentLoop) Stop() {
	l.stopped.Store(true)
	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	l.mu.Unlock()
}

// IsPaused returns whether the loop is paused.
func (l *AgentLoop) IsPaused() bool {
	return l.paused.Load()
}

// IsStopped returns whether the loop is stopped.
func (l *AgentLoop) IsStopped() bool {
	return l.stopped.Load()
}

// AddMessage adds a message to the conversation history.
// This can be used to inject context or continue a conversation.
func (l *AgentLoop) AddMessage(msg agent.Message) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state.Messages = append(l.state.Messages, msg)
}

// SetMessages replaces the entire conversation history.
// This can be used to restore a previous session.
func (l *AgentLoop) SetMessages(messages []agent.Message) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.state.Messages = messages
}

// GetConversationHistory returns a copy of the conversation history.
func (l *AgentLoop) GetConversationHistory() []agent.Message {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]agent.Message, len(l.state.Messages))
	copy(result, l.state.Messages)
	return result
}

// RunSingleTurn executes a single conversation turn and returns the response.
// This is useful for one-shot interactions without a full loop.
func (l *AgentLoop) RunSingleTurn(ctx context.Context, userMessage string) (*agent.Response, error) {
	// Add user message
	l.state.Messages = append(l.state.Messages, agent.NewTextMessage(agent.RoleUser, userMessage))

	// Build request
	req := l.buildRequest()

	// Make request
	response, err := l.completeWithRetry(ctx, req)
	if err != nil {
		return nil, err
	}

	// Add assistant response
	assistantMsg := agent.Message{
		Role:    agent.RoleAssistant,
		Content: response.Content,
	}
	l.state.Messages = append(l.state.Messages, assistantMsg)

	// Update usage
	l.updateUsage(response.Usage)

	return response, nil
}

// ExecuteToolCalls processes tool calls from a response and returns results.
// This is useful for manual control of the tool execution loop.
func (l *AgentLoop) ExecuteToolCalls(ctx context.Context, toolCalls []agent.ContentBlock) ([]*ToolResult, error) {
	results, err := l.executor.ExecuteBatch(ctx, toolCalls)
	if err != nil {
		return nil, err
	}

	// Add tool results to conversation
	var resultBlocks []agent.ContentBlock
	for i, tc := range toolCalls {
		resultBlock := l.executor.ToContentBlock(tc.ToolUseID, results[i])
		resultBlocks = append(resultBlocks, resultBlock)
	}

	if len(resultBlocks) > 0 {
		toolMsg := agent.Message{
			Role:    agent.RoleUser,
			Content: resultBlocks,
		}
		l.state.Messages = append(l.state.Messages, toolMsg)
	}

	return results, nil
}

// =====================================================
// Session Resume Support
// =====================================================

// ToSessionState extracts the current session state for persistence.
// This is used by the RestartManager to persist session state before restart.
func (l *AgentLoop) ToSessionState() *mcp.SessionState {
	l.mu.Lock()
	defer l.mu.Unlock()

	return &mcp.SessionState{
		SessionID:               l.cfg.SessionID,
		Iteration:               l.state.Iteration,
		TotalTokens:             l.state.TotalTokens,
		TotalCostUSD:            l.state.TotalCostUSD,
		Messages:                l.state.Messages,
		MessageCount:            len(l.state.Messages),
		LastSignal:              l.state.LastSignal,
		IterationsSinceProgress: l.state.IterationsSinceProgress,
		Model:                   l.cfg.Model,
		SystemPrompt:            l.cfg.SystemPrompt,
		ProjectPath:             l.cfg.ProjectPath,
		WorkDir:                 l.cfg.WorkDir,
	}
}

// RestoreFromResumeState restores the agent loop state from a SessionResumeState.
// This should be called before Run() to resume a session from a previous state.
func (l *AgentLoop) RestoreFromResumeState(state *mcp.SessionResumeState) error {
	if state == nil {
		return fmt.Errorf("cannot restore from nil state")
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Restore messages
	var messages []agent.Message
	if err := state.GetMessages(&messages); err != nil {
		return fmt.Errorf("failed to restore messages: %w", err)
	}

	// Restore last signal
	var lastSignal *mcp.Signal
	if state.LastSignal != nil {
		lastSignal = &mcp.Signal{}
		if err := state.GetLastSignal(lastSignal); err != nil {
			// Non-fatal, just log and continue without the signal
			lastSignal = nil
		}
	}

	// Update state
	l.state.Iteration = state.Iteration
	l.state.TotalTokens = state.TotalTokens
	l.state.TotalCostUSD = state.TotalCostUSD
	l.state.Messages = messages
	l.state.LastSignal = lastSignal
	l.state.IterationsSinceProgress = state.IterationsSinceProgress
	l.state.LastIterationAt = state.UpdatedAt

	// Emit restore event
	l.emitEvent(LoopEventStart, fmt.Sprintf("session resumed from iteration %d", state.Iteration), nil)

	return nil
}

// RunFromResume executes the agent loop continuing from a restored state.
// This is similar to Run() but expects the state to already be restored.
func (l *AgentLoop) RunFromResume(ctx context.Context, resumePrompt string) error {
	defer close(l.eventCh)

	// Create cancellable context
	ctx, cancel := context.WithCancel(ctx)
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()
	defer cancel()

	// Emit loop resume event
	l.emitEvent(LoopEventStart, "resuming from saved state", nil)

	// Add a resume prompt if provided to acknowledge the restart
	if resumePrompt != "" {
		l.state.Messages = append(l.state.Messages, agent.NewTextMessage(agent.RoleUser, resumePrompt))
		l.emitEvent(MessageUser, "resume prompt added", nil)
	}

	// Main loop (same as Run)
	for {
		// Check lifecycle state
		if l.stopped.Load() {
			l.emitEvent(LoopEventStop, "loop stopped by user", nil)
			return ErrLoopStopped
		}

		if l.paused.Load() {
			l.emitEvent(LoopEventPause, "loop paused", nil)
			return ErrLoopPaused
		}

		// Check context cancellation
		if err := ctx.Err(); err != nil {
			l.emitEvent(LoopEventError, "context cancelled", err)
			return err
		}

		// Check iteration limit
		if l.cfg.MaxIterations > 0 && l.state.Iteration >= l.cfg.MaxIterations {
			l.emitEvent(LoopEventMaxReached, fmt.Sprintf("max iterations %d reached", l.cfg.MaxIterations), nil)
			return ErrMaxIterations
		}

		// Check thrashing
		if l.cfg.ThrashThreshold > 0 && l.state.IterationsSinceProgress >= l.cfg.ThrashThreshold {
			l.emitEvent(ThrashingDetected, fmt.Sprintf("no progress signal in %d iterations", l.cfg.ThrashThreshold), nil)
			return fmt.Errorf("thrashing detected: no progress in %d iterations", l.cfg.ThrashThreshold)
		}

		// Run one iteration
		completed, err := l.runIteration(ctx)
		if err != nil {
			l.emitEvent(LoopEventError, err.Error(), err)
			return err
		}

		if completed {
			l.emitEvent(LoopEventComplete, "task completed", nil)
			return nil
		}
	}
}

// GetSessionID returns the session ID for this loop.
func (l *AgentLoop) GetSessionID() string {
	return l.cfg.SessionID
}

// GetConfig returns a copy of the loop configuration.
func (l *AgentLoop) GetConfig() AgentLoopConfig {
	return l.cfg
}
