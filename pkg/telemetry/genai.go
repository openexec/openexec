package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// GenAI semantic conventions based on OpenTelemetry GenAI specification.
// See: https://opentelemetry.io/docs/specs/semconv/gen-ai/
//
// These attributes enable standardized observability across LLM providers.

// GenAISystem values for common providers.
const (
	GenAISystemAnthropic = "anthropic"
	GenAISystemOpenAI    = "openai"
	GenAISystemGoogle    = "google"
	GenAISystemGemini    = "gemini"
	GenAISystemOllama    = "ollama"
)

// GenAIFinishReason values for common completion reasons.
const (
	GenAIFinishReasonStop       = "stop"
	GenAIFinishReasonLength     = "length"
	GenAIFinishReasonToolCalls  = "tool_calls"
	GenAIFinishReasonContentFilter = "content_filter"
	GenAIFinishReasonError      = "error"
)

// GenAIRequest holds request-side attributes for an LLM call.
type GenAIRequest struct {
	System       string  // Provider identifier (anthropic, openai, etc.)
	Model        string  // Model ID (claude-3-opus, gpt-4, etc.)
	MaxTokens    int     // Maximum tokens to generate
	Temperature  float64 // Sampling temperature
	TopP         float64 // Nucleus sampling parameter
	PromptHash   string  // SHA-256 hash of the prompt for dedup
	CacheKey     string  // Stable hash for deterministic replay
}

// GenAIResponse holds response-side attributes for an LLM call.
type GenAIResponse struct {
	FinishReason  string // Why the model stopped (stop, length, tool_calls)
	InputTokens   int    // Tokens in the input prompt
	OutputTokens  int    // Tokens generated in response
	CachedTokens  int    // Tokens served from provider cache
	TotalTokens   int    // Total tokens (input + output)
	CostUSD       float64 // Estimated cost in USD
}

// StartGenAISpan creates a span for an LLM completion request.
// This follows the OpenTelemetry GenAI semantic conventions.
func StartGenAISpan(ctx context.Context, operationType string, req GenAIRequest) (context.Context, trace.Span) {
	spanName := "gen_ai." + operationType
	if req.System != "" {
		spanName = req.System + "." + operationType
	}

	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.system", req.System),
		attribute.String("gen_ai.request.model", req.Model),
		attribute.String("gen_ai.operation.name", operationType),
	}

	if req.MaxTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.request.max_tokens", req.MaxTokens))
	}
	if req.Temperature > 0 {
		attrs = append(attrs, attribute.Float64("gen_ai.request.temperature", req.Temperature))
	}
	if req.TopP > 0 {
		attrs = append(attrs, attribute.Float64("gen_ai.request.top_p", req.TopP))
	}
	if req.PromptHash != "" {
		attrs = append(attrs, attribute.String("gen_ai.prompt_hash", req.PromptHash))
	}
	if req.CacheKey != "" {
		attrs = append(attrs, attribute.String("gen_ai.cache_key", req.CacheKey))
	}

	return GetTracer().Start(ctx, spanName,
		trace.WithAttributes(attrs...),
		trace.WithSpanKind(trace.SpanKindClient),
	)
}

// SetGenAIResponse adds response attributes to a GenAI span.
func SetGenAIResponse(span trace.Span, resp GenAIResponse) {
	attrs := []attribute.KeyValue{
		attribute.Int("gen_ai.usage.input_tokens", resp.InputTokens),
		attribute.Int("gen_ai.usage.output_tokens", resp.OutputTokens),
	}

	if resp.CachedTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.usage.cached_tokens", resp.CachedTokens))
	}
	if resp.TotalTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.usage.total_tokens", resp.TotalTokens))
	} else if resp.InputTokens > 0 || resp.OutputTokens > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.usage.total_tokens", resp.InputTokens+resp.OutputTokens))
	}
	if resp.FinishReason != "" {
		attrs = append(attrs, attribute.String("gen_ai.response.finish_reason", resp.FinishReason))
	}
	if resp.CostUSD > 0 {
		attrs = append(attrs, attribute.Float64("gen_ai.cost_usd", resp.CostUSD))
	}

	span.SetAttributes(attrs...)
	span.SetStatus(codes.Ok, "")
}

// SetGenAIError records an error on a GenAI span.
func SetGenAIError(span trace.Span, err error, errorType string) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	span.SetAttributes(
		attribute.String("gen_ai.error.type", errorType),
		attribute.String("gen_ai.error.message", truncateError(err.Error())),
	)
}

// GenAIStreamEvent holds attributes for streaming events.
type GenAIStreamEvent struct {
	ChunkIndex   int    // Index of this chunk in the stream
	TokenCount   int    // Tokens in this chunk
	ContentDelta string // Partial content (should be redacted if sensitive)
}

// RecordGenAIStreamChunk records a streaming chunk as a span event.
func RecordGenAIStreamChunk(span trace.Span, event GenAIStreamEvent) {
	attrs := []attribute.KeyValue{
		attribute.Int("gen_ai.stream.chunk_index", event.ChunkIndex),
	}
	if event.TokenCount > 0 {
		attrs = append(attrs, attribute.Int("gen_ai.stream.token_count", event.TokenCount))
	}
	// Do NOT record content_delta as it may contain sensitive data
	span.AddEvent("gen_ai.stream.chunk", trace.WithAttributes(attrs...))
}

// ToolCallEvent holds attributes for tool/function calls.
type ToolCallEvent struct {
	ToolName   string // Name of the tool being called
	ToolID     string // Provider's tool call ID
	InputHash  string // Hash of tool inputs (for replay without logging content)
	OutputHash string // Hash of tool outputs
}

// RecordGenAIToolCall records a tool call as a span event.
func RecordGenAIToolCall(span trace.Span, event ToolCallEvent) {
	attrs := []attribute.KeyValue{
		attribute.String("gen_ai.tool.name", event.ToolName),
	}
	if event.ToolID != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.id", event.ToolID))
	}
	if event.InputHash != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.input_hash", event.InputHash))
	}
	if event.OutputHash != "" {
		attrs = append(attrs, attribute.String("gen_ai.tool.output_hash", event.OutputHash))
	}
	span.AddEvent("gen_ai.tool_call", trace.WithAttributes(attrs...))
}

// ProviderToGenAISystem maps provider names to GenAI system identifiers.
func ProviderToGenAISystem(provider string) string {
	switch provider {
	case "anthropic", "claude":
		return GenAISystemAnthropic
	case "openai", "gpt":
		return GenAISystemOpenAI
	case "google", "gemini", "vertex":
		return GenAISystemGoogle
	case "ollama":
		return GenAISystemOllama
	default:
		return provider
	}
}

// NormalizeFinishReason maps provider-specific finish reasons to standard values.
func NormalizeFinishReason(provider, reason string) string {
	switch provider {
	case GenAISystemAnthropic:
		switch reason {
		case "end_turn":
			return GenAIFinishReasonStop
		case "max_tokens":
			return GenAIFinishReasonLength
		case "tool_use":
			return GenAIFinishReasonToolCalls
		}
	case GenAISystemOpenAI:
		switch reason {
		case "stop":
			return GenAIFinishReasonStop
		case "length":
			return GenAIFinishReasonLength
		case "function_call", "tool_calls":
			return GenAIFinishReasonToolCalls
		case "content_filter":
			return GenAIFinishReasonContentFilter
		}
	case GenAISystemGoogle:
		switch reason {
		case "STOP":
			return GenAIFinishReasonStop
		case "MAX_TOKENS":
			return GenAIFinishReasonLength
		case "SAFETY":
			return GenAIFinishReasonContentFilter
		}
	}
	// Return original if no mapping found
	return reason
}
