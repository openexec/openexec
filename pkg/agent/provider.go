// Package agent provides types and interfaces for AI provider adapters.
// It defines the unified interface that all AI providers (OpenAI, Anthropic, Gemini)
// must implement to be used by the OpenExec orchestration engine.
//
// IMPORTANT: This package contains ABSTRACTIONS only, not implementations.
// OpenExec currently uses CLI subprocesses (claude, codex, gemini) rather than
// direct API clients. See internal/runner/ for model resolution and
// internal/loop/ for process spawning.
//
// Future implementations may use these interfaces for direct API integration.
package agent

import (
	"context"
	"encoding/json"
	"time"
)

// Role represents the role of a message sender in a conversation.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleSystem    Role = "system"
	RoleTool      Role = "tool"
)

// ContentType represents the type of content in a message.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeImage      ContentType = "image"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ContentBlock represents a single piece of content within a message.
// Messages can contain multiple content blocks (e.g., text + tool calls).
type ContentBlock struct {
	Type ContentType `json:"type"`

	// Text content (when Type == ContentTypeText)
	Text string `json:"text,omitempty"`

	// Image content (when Type == ContentTypeImage)
	ImageURL   string `json:"image_url,omitempty"`
	ImageData  []byte `json:"image_data,omitempty"`
	ImageMedia string `json:"image_media,omitempty"` // MIME type

	// Tool use (when Type == ContentTypeToolUse)
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolName  string          `json:"tool_name,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`

	// Tool result (when Type == ContentTypeToolResult)
	ToolResultID string `json:"tool_result_id,omitempty"`
	ToolOutput   string `json:"tool_output,omitempty"`
	ToolError    string `json:"tool_error,omitempty"`
}

// Message represents a single turn in a conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`

	// Legacy field for simple text-only messages
	// Deprecated: Use Content with a single ContentBlock instead
	Text string `json:"text,omitempty"`
}

// NewTextMessage creates a simple text message.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: text},
		},
	}
}

// NewToolResultMessage creates a tool result message.
func NewToolResultMessage(toolUseID, output string, err error) Message {
	block := ContentBlock{
		Type:         ContentTypeToolResult,
		ToolResultID: toolUseID,
		ToolOutput:   output,
	}
	if err != nil {
		block.ToolError = err.Error()
	}
	return Message{
		Role:    RoleTool,
		Content: []ContentBlock{block},
	}
}

// GetText returns the concatenated text content from all text blocks.
func (m Message) GetText() string {
	// Handle legacy format
	if m.Text != "" && len(m.Content) == 0 {
		return m.Text
	}
	var result string
	for _, block := range m.Content {
		if block.Type == ContentTypeText {
			result += block.Text
		}
	}
	return result
}

// GetToolCalls returns all tool use blocks from the message.
func (m Message) GetToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, block := range m.Content {
		if block.Type == ContentTypeToolUse {
			calls = append(calls, block)
		}
	}
	return calls
}

// ToolDefinition describes a tool that can be used by the AI model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"` // JSON Schema
}

// Request is the payload sent to a provider.
type Request struct {
	// Model is the model identifier (e.g., "claude-3-opus", "gpt-4", "gemini-pro")
	Model string `json:"model"`

	// Messages is the conversation history
	Messages []Message `json:"messages"`

	// System is the system prompt/instruction
	System string `json:"system,omitempty"`

	// Tools is the list of available tools
	Tools []ToolDefinition `json:"tools,omitempty"`

	// ToolChoice controls how the model uses tools
	// Options: "auto", "any", "none", or a specific tool name
	ToolChoice string `json:"tool_choice,omitempty"`

	// MaxTokens limits the response length
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature controls randomness (0.0 to 2.0)
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP is nucleus sampling parameter
	TopP *float64 `json:"top_p,omitempty"`

	// StopSequences are sequences that stop generation
	StopSequences []string `json:"stop_sequences,omitempty"`

	// Metadata is provider-specific metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopReasonEnd       StopReason = "end_turn"      // Natural end of response
	StopReasonMaxTokens StopReason = "max_tokens"    // Hit token limit
	StopReasonToolUse   StopReason = "tool_use"      // Model wants to use a tool
	StopReasonStop      StopReason = "stop_sequence" // Hit a stop sequence
	StopReasonError     StopReason = "error"         // Error occurred
)

// Usage tracks token consumption for a request.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`

	// CacheReadTokens is tokens read from cache (Anthropic-specific)
	CacheReadTokens int `json:"cache_read_tokens,omitempty"`

	// CacheWriteTokens is tokens written to cache (Anthropic-specific)
	CacheWriteTokens int `json:"cache_write_tokens,omitempty"`
}

// Response is the complete response from a provider.
type Response struct {
	// ID is the unique response identifier
	ID string `json:"id"`

	// Model is the model that generated the response
	Model string `json:"model"`

	// Content is the response content blocks
	Content []ContentBlock `json:"content"`

	// StopReason indicates why generation stopped
	StopReason StopReason `json:"stop_reason"`

	// Usage tracks token consumption
	Usage Usage `json:"usage"`

	// Metadata is provider-specific response metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// GetText returns the concatenated text content from the response.
func (r Response) GetText() string {
	var result string
	for _, block := range r.Content {
		if block.Type == ContentTypeText {
			result += block.Text
		}
	}
	return result
}

// GetToolCalls returns all tool use blocks from the response.
func (r Response) GetToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, block := range r.Content {
		if block.Type == ContentTypeToolUse {
			calls = append(calls, block)
		}
	}
	return calls
}

// HasToolCalls returns true if the response contains tool calls.
func (r Response) HasToolCalls() bool {
	return len(r.GetToolCalls()) > 0
}

// StreamEvent represents a single event in a streaming response.
type StreamEvent struct {
	// Type is the event type
	Type StreamEventType `json:"type"`

	// Delta contains incremental content updates
	Delta *StreamDelta `json:"delta,omitempty"`

	// ContentBlock contains content block information (for start events)
	ContentBlock *ContentBlock `json:"content_block,omitempty"`

	// ContentBlockIndex is the index of the content block being updated
	ContentBlockIndex int `json:"content_block_index,omitempty"`

	// Usage contains token usage (for final events)
	Usage *Usage `json:"usage,omitempty"`

	// StopReason is present when generation stops
	StopReason StopReason `json:"stop_reason,omitempty"`

	// Error is present when an error occurs
	Error error `json:"error,omitempty"`

	// Raw is the original event data (for debugging)
	Raw json.RawMessage `json:"raw,omitempty"`
}

// StreamEventType categorizes streaming events.
type StreamEventType string

const (
	StreamEventStart        StreamEventType = "message_start"
	StreamEventContentStart StreamEventType = "content_block_start"
	StreamEventContentDelta StreamEventType = "content_block_delta"
	StreamEventContentStop  StreamEventType = "content_block_stop"
	StreamEventStop         StreamEventType = "message_stop"
	StreamEventPing         StreamEventType = "ping"
	StreamEventError        StreamEventType = "error"
)

// StreamDelta contains incremental updates in a stream.
type StreamDelta struct {
	// Text is incremental text content
	Text string `json:"text,omitempty"`

	// ToolInput is incremental tool input JSON
	ToolInput string `json:"tool_input,omitempty"`
}

// ResponseChunk is a simplified streaming chunk (legacy compatibility).
// Deprecated: Use StreamEvent for new code.
type ResponseChunk struct {
	Text  string `json:"text,omitempty"`
	Error error  `json:"error,omitempty"`
	Done  bool   `json:"done"`
}

// ProviderCapabilities describes what features a provider supports.
type ProviderCapabilities struct {
	// Streaming indicates if the provider supports streaming responses
	Streaming bool `json:"streaming"`

	// Vision indicates if the provider supports image inputs
	Vision bool `json:"vision"`

	// ToolUse indicates if the provider supports tool/function calling
	ToolUse bool `json:"tool_use"`

	// SystemPrompt indicates if the provider supports system prompts
	SystemPrompt bool `json:"system_prompt"`

	// MultiTurn indicates if the provider supports multi-turn conversations
	MultiTurn bool `json:"multi_turn"`

	// MaxContextTokens is the maximum context window size
	MaxContextTokens int `json:"max_context_tokens"`

	// MaxOutputTokens is the maximum output tokens
	MaxOutputTokens int `json:"max_output_tokens"`
}

// ModelInfo describes a specific model's characteristics.
type ModelInfo struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Provider     string               `json:"provider"`
	Capabilities ProviderCapabilities `json:"capabilities"`

	// Pricing per million tokens (in USD)
	PricePerMInputTokens  float64 `json:"price_per_m_input_tokens"`
	PricePerMOutputTokens float64 `json:"price_per_m_output_tokens"`

	// Deprecated indicates if the model is being phased out
	Deprecated bool `json:"deprecated,omitempty"`

	// Enabled indicates if the model is currently available for use by the user
	Enabled bool `json:"enabled"`

	// DeprecationDate is when the model will be removed
	DeprecationDate *time.Time `json:"deprecation_date,omitempty"`
}

// ProviderError represents an error from a provider.
type ProviderError struct {
	// Code is a machine-readable error code
	Code string `json:"code"`

	// Message is a human-readable error message
	Message string `json:"message"`

	// StatusCode is the HTTP status code (if applicable)
	StatusCode int `json:"status_code,omitempty"`

	// Retryable indicates if the request should be retried
	Retryable bool `json:"retryable"`

	// RetryAfter suggests when to retry (if applicable)
	RetryAfter *time.Duration `json:"retry_after,omitempty"`

	// Details contains additional error context
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return e.Code + " (" + string(rune(e.StatusCode)) + "): " + e.Message
	}
	return e.Code + ": " + e.Message
}

// Common error codes
const (
	ErrCodeRateLimit      = "rate_limit"
	ErrCodeInvalidRequest = "invalid_request"
	ErrCodeAuthentication = "authentication_error"
	ErrCodePermission     = "permission_error"
	ErrCodeNotFound       = "not_found"
	ErrCodeServerError    = "server_error"
	ErrCodeContextLength  = "context_length_exceeded"
	ErrCodeContentFilter  = "content_filter"
	ErrCodeTimeout        = "timeout"
)

// ProviderAdapter is the interface that all AI providers must implement.
// This is the "Abstract Base Class" for provider implementations.
type ProviderAdapter interface {
	// GetName returns the provider's identifier (e.g., "openai", "anthropic", "gemini")
	GetName() string

	// GetModels returns the list of available model IDs
	GetModels() []string

	// GetModelInfo returns detailed information about a specific model
	GetModelInfo(modelID string) (*ModelInfo, error)

	// GetCapabilities returns the capabilities of a specific model
	GetCapabilities(modelID string) (*ProviderCapabilities, error)

	// Complete sends a request and returns a complete response
	Complete(ctx context.Context, req Request) (*Response, error)

	// Stream sends a request and returns a channel of streaming events
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)

	// ValidateRequest checks if a request is valid for this provider
	ValidateRequest(req Request) error

	// EstimateTokens estimates the token count for the given content
	// This is used for context budgeting before making API calls
	EstimateTokens(content string) int
}

// ToolSchemaTranslator translates tool definitions between the unified format
// and provider-specific formats.
type ToolSchemaTranslator interface {
	// TranslateToProvider converts a unified ToolDefinition to provider format
	TranslateToProvider(tool ToolDefinition) (interface{}, error)

	// TranslateFromProvider converts a provider tool call to unified format
	TranslateFromProvider(providerToolCall interface{}) (*ContentBlock, error)

	// TranslateToolResult converts a unified tool result to provider format
	TranslateToolResult(result ContentBlock) (interface{}, error)
}

// Provider is a convenience alias that combines ProviderAdapter with streaming.
// Deprecated: Use ProviderAdapter directly
type Provider interface {
	GenerateStream(ctx context.Context, req Request) (<-chan ResponseChunk, error)
	GetName() string
	GetModels() []string
}
