// Package agent provides the OpenAI provider implementation.
// This file implements the ProviderAdapter interface for OpenAI's API,
// supporting chat completions, streaming, and tool use.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// DefaultOpenAIBaseURL is the default OpenAI API endpoint.
	DefaultOpenAIBaseURL = "https://api.openai.com/v1"

	// DefaultOpenAITimeout is the default HTTP client timeout.
	DefaultOpenAITimeout = 120 * time.Second
)

// OpenAI model identifiers
const (
	ModelGPT4o      = "gpt-4o"
	ModelGPT4oMini  = "gpt-4o-mini"
	ModelGPT4Turbo  = "gpt-4-turbo"
	ModelGPT4       = "gpt-4"
	ModelGPT35Turbo = "gpt-3.5-turbo"
	ModelO1         = "o1"
	ModelO1Mini     = "o1-mini"
	ModelO1Preview  = "o1-preview"
	ModelO3Mini     = "o3-mini"

	// GPT-5.3 (Codex) models
	ModelGPT53           = "gpt-5.3"
	ModelGPT53Codex      = "gpt-5.3-codex"
	ModelGPT53CodexSpark = "gpt-5.3-codex-spark"
)

// OpenAIProviderConfig holds configuration for the OpenAI provider.
type OpenAIProviderConfig struct {
	// APIKey is the OpenAI API key.
	APIKey string

	// BaseURL is the API base URL (defaults to DefaultOpenAIBaseURL).
	BaseURL string

	// Organization is the optional organization ID.
	Organization string

	// Timeout is the HTTP client timeout.
	Timeout time.Duration

	// HTTPClient is an optional custom HTTP client.
	HTTPClient *http.Client
}

// OpenAIProvider implements ProviderAdapter for OpenAI's API.
type OpenAIProvider struct {
	config     OpenAIProviderConfig
	httpClient *http.Client
	models     []string
	modelInfo  map[string]*ModelInfo
}

// Compile-time check that OpenAIProvider implements ProviderAdapter.
var _ ProviderAdapter = (*OpenAIProvider)(nil)

// NewOpenAIProvider creates a new OpenAI provider with the given configuration.
func NewOpenAIProvider(config OpenAIProviderConfig) (*OpenAIProvider, error) {
	if config.APIKey == "" {
		// Try to get from environment
		config.APIKey = os.Getenv("OPENAI_API_KEY")
		if config.APIKey == "" {
			return nil, &ProviderError{
				Code:    ErrCodeAuthentication,
				Message: "OpenAI API key is required (set OPENAI_API_KEY environment variable or provide in config)",
			}
		}
	}

	if config.BaseURL == "" {
		config.BaseURL = DefaultOpenAIBaseURL
	}

	if config.Timeout == 0 {
		config.Timeout = DefaultOpenAITimeout
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	p := &OpenAIProvider{
		config:     config,
		httpClient: httpClient,
		models:     defaultOpenAIModels(),
		modelInfo:  defaultOpenAIModelInfo(),
	}

	return p, nil
}

// NewOpenAIProviderFromEnv creates a new OpenAI provider using environment variables.
func NewOpenAIProviderFromEnv() (*OpenAIProvider, error) {
	return NewOpenAIProvider(OpenAIProviderConfig{})
}

// GetName returns the provider identifier.
func (p *OpenAIProvider) GetName() string {
	return "openai"
}

// GetModels returns the list of available model IDs.
func (p *OpenAIProvider) GetModels() []string {
	return p.models
}

// GetModelInfo returns detailed information about a specific model.
func (p *OpenAIProvider) GetModelInfo(modelID string) (*ModelInfo, error) {
	info, ok := p.modelInfo[modelID]
	if !ok {
		return nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("model %q not found", modelID),
		}
	}
	return info, nil
}

// GetCapabilities returns the capabilities of a specific model.
func (p *OpenAIProvider) GetCapabilities(modelID string) (*ProviderCapabilities, error) {
	info, err := p.GetModelInfo(modelID)
	if err != nil {
		return nil, err
	}
	return &info.Capabilities, nil
}

// Complete sends a request and returns a complete response.
func (p *OpenAIProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := p.ValidateRequest(req); err != nil {
		return nil, err
	}

	openAIReq := p.buildOpenAIRequest(req, false)
	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to create request: %v", err),
		}
	}

	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, p.handleHTTPError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, p.parseErrorResponse(resp)
	}

	var openAIResp openAIChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeServerError,
			Message: fmt.Sprintf("failed to decode response: %v", err),
		}
	}

	return p.convertResponse(&openAIResp), nil
}

// Stream sends a request and returns a channel of streaming events.
func (p *OpenAIProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	if err := p.ValidateRequest(req); err != nil {
		return nil, err
	}

	openAIReq := p.buildOpenAIRequest(req, true)
	body, err := json.Marshal(openAIReq)
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to create request: %v", err),
		}
	}

	p.setHeaders(httpReq)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, p.handleHTTPError(err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, p.parseErrorResponse(resp)
	}

	eventCh := make(chan StreamEvent, 100)
	go p.processStreamResponse(ctx, resp, eventCh)

	return eventCh, nil
}

// ValidateRequest checks if a request is valid for this provider.
func (p *OpenAIProvider) ValidateRequest(req Request) error {
	if req.Model == "" {
		return &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: "model is required",
		}
	}

	// Check if model is supported
	found := false
	for _, m := range p.models {
		if m == req.Model {
			found = true
			break
		}
	}
	if !found {
		return &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("model %q is not supported", req.Model),
		}
	}

	// Check for o1/o3 models which don't support system prompts or certain features
	if isReasoningModel(req.Model) {
		if req.System != "" {
			return &ProviderError{
				Code:    ErrCodeInvalidRequest,
				Message: fmt.Sprintf("model %q does not support system prompts", req.Model),
			}
		}
		if len(req.Tools) > 0 && req.Model != ModelO3Mini {
			return &ProviderError{
				Code:    ErrCodeInvalidRequest,
				Message: fmt.Sprintf("model %q does not support tool use", req.Model),
			}
		}
	}

	return nil
}

// EstimateTokens estimates the token count for the given content.
// This is a rough estimation using ~4 characters per token.
func (p *OpenAIProvider) EstimateTokens(content string) int {
	// OpenAI uses tiktoken for accurate counting, but for estimation
	// we use approximately 4 characters per token (common heuristic)
	return len(content) / 4
}

// setHeaders sets the required HTTP headers for OpenAI API requests.
func (p *OpenAIProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
	if p.config.Organization != "" {
		req.Header.Set("OpenAI-Organization", p.config.Organization)
	}
}

// buildOpenAIRequest converts a unified Request to OpenAI's request format.
func (p *OpenAIProvider) buildOpenAIRequest(req Request, stream bool) *openAIChatCompletionRequest {
	openAIReq := &openAIChatCompletionRequest{
		Model:    req.Model,
		Stream:   stream,
		Messages: make([]openAIMessage, 0),
	}

	// Add system message if present (and not a reasoning model)
	if req.System != "" && !isReasoningModel(req.Model) {
		openAIReq.Messages = append(openAIReq.Messages, openAIMessage{
			Role:    "system",
			Content: req.System,
		})
	}

	// Convert messages
	for _, msg := range req.Messages {
		openAIMsg := p.convertToOpenAIMessage(msg)
		openAIReq.Messages = append(openAIReq.Messages, openAIMsg)
	}

	// Add tool result messages for tool role
	// (handled in convertToOpenAIMessage)

	// Set max tokens
	if req.MaxTokens > 0 {
		openAIReq.MaxCompletionTokens = req.MaxTokens
	}

	// Set temperature (not supported for o1/o3 models)
	if req.Temperature != nil && !isReasoningModel(req.Model) {
		openAIReq.Temperature = req.Temperature
	}

	// Set top_p
	if req.TopP != nil && !isReasoningModel(req.Model) {
		openAIReq.TopP = req.TopP
	}

	// Set stop sequences
	if len(req.StopSequences) > 0 && !isReasoningModel(req.Model) {
		openAIReq.Stop = req.StopSequences
	}

	// Convert tools
	if len(req.Tools) > 0 {
		openAIReq.Tools = make([]openAITool, len(req.Tools))
		for i, tool := range req.Tools {
			openAIReq.Tools[i] = openAITool{
				Type: "function",
				Function: openAIFunction{
					Name:        tool.Name,
					Description: tool.Description,
					Parameters:  tool.InputSchema,
				},
			}
		}

		// Set tool_choice
		switch req.ToolChoice {
		case "auto", "":
			openAIReq.ToolChoice = "auto"
		case "any":
			openAIReq.ToolChoice = "required"
		case "none":
			openAIReq.ToolChoice = "none"
		default:
			// Specific tool name
			openAIReq.ToolChoice = map[string]interface{}{
				"type": "function",
				"function": map[string]string{
					"name": req.ToolChoice,
				},
			}
		}
	}

	// Add stream options for token usage in streaming
	if stream {
		openAIReq.StreamOptions = &openAIStreamOptions{
			IncludeUsage: true,
		}
	}

	return openAIReq
}

// convertToOpenAIMessage converts a unified Message to OpenAI's message format.
func (p *OpenAIProvider) convertToOpenAIMessage(msg Message) openAIMessage {
	openAIMsg := openAIMessage{
		Role: string(msg.Role),
	}

	// Handle tool role messages (tool results)
	if msg.Role == RoleTool {
		for _, block := range msg.Content {
			if block.Type == ContentTypeToolResult {
				openAIMsg.Role = "tool"
				openAIMsg.ToolCallID = block.ToolResultID
				if block.ToolError != "" {
					openAIMsg.Content = fmt.Sprintf("Error: %s", block.ToolError)
				} else {
					openAIMsg.Content = block.ToolOutput
				}
				return openAIMsg
			}
		}
	}

	// Handle assistant messages with tool calls
	if msg.Role == RoleAssistant {
		toolCalls := msg.GetToolCalls()
		if len(toolCalls) > 0 {
			openAIMsg.ToolCalls = make([]openAIToolCall, len(toolCalls))
			for i, tc := range toolCalls {
				openAIMsg.ToolCalls[i] = openAIToolCall{
					ID:   tc.ToolUseID,
					Type: "function",
					Function: openAIFunctionCall{
						Name:      tc.ToolName,
						Arguments: string(tc.ToolInput),
					},
				}
			}
		}
	}

	// Build content (text parts)
	var textParts []string
	for _, block := range msg.Content {
		if block.Type == ContentTypeText {
			textParts = append(textParts, block.Text)
		}
	}
	if len(textParts) > 0 {
		openAIMsg.Content = strings.Join(textParts, "")
	}

	// Handle legacy text field
	if openAIMsg.Content == "" && msg.Text != "" {
		openAIMsg.Content = msg.Text
	}

	return openAIMsg
}

// convertResponse converts an OpenAI response to the unified format.
func (p *OpenAIProvider) convertResponse(resp *openAIChatCompletionResponse) *Response {
	response := &Response{
		ID:    resp.ID,
		Model: resp.Model,
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
		Content: make([]ContentBlock, 0),
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		// Set stop reason
		switch choice.FinishReason {
		case "stop":
			response.StopReason = StopReasonEnd
		case "length":
			response.StopReason = StopReasonMaxTokens
		case "tool_calls":
			response.StopReason = StopReasonToolUse
		case "content_filter":
			response.StopReason = StopReasonStop
		default:
			response.StopReason = StopReasonEnd
		}

		// Add text content
		if choice.Message.Content != "" {
			response.Content = append(response.Content, ContentBlock{
				Type: ContentTypeText,
				Text: choice.Message.Content,
			})
		}

		// Add tool calls
		for _, tc := range choice.Message.ToolCalls {
			response.Content = append(response.Content, ContentBlock{
				Type:      ContentTypeToolUse,
				ToolUseID: tc.ID,
				ToolName:  tc.Function.Name,
				ToolInput: json.RawMessage(tc.Function.Arguments),
			})
		}
	}

	return response
}

// processStreamResponse reads the SSE stream and sends events to the channel.
func (p *OpenAIProvider) processStreamResponse(ctx context.Context, resp *http.Response, eventCh chan<- StreamEvent) {
	defer close(eventCh)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	var currentToolCalls map[int]*openAIToolCall
	currentToolCalls = make(map[int]*openAIToolCall)

	// Send start event
	select {
	case eventCh <- StreamEvent{Type: StreamEventStart}:
	case <-ctx.Done():
		return
	}

	contentBlockIndex := 0
	inToolCall := false

	for {
		select {
		case <-ctx.Done():
			eventCh <- StreamEvent{
				Type:  StreamEventError,
				Error: ctx.Err(),
			}
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			eventCh <- StreamEvent{
				Type:  StreamEventError,
				Error: fmt.Errorf("error reading stream: %w", err),
			}
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // Skip malformed chunks
		}

		if len(chunk.Choices) == 0 {
			// Check for usage in final chunk
			if chunk.Usage != nil {
				eventCh <- StreamEvent{
					Type: StreamEventStop,
					Usage: &Usage{
						PromptTokens:     chunk.Usage.PromptTokens,
						CompletionTokens: chunk.Usage.CompletionTokens,
						TotalTokens:      chunk.Usage.TotalTokens,
					},
					StopReason: StopReasonEnd,
				}
			}
			continue
		}

		delta := chunk.Choices[0].Delta

		// Handle text content
		if delta.Content != "" {
			if !inToolCall && contentBlockIndex == 0 {
				eventCh <- StreamEvent{
					Type:              StreamEventContentStart,
					ContentBlockIndex: contentBlockIndex,
					ContentBlock: &ContentBlock{
						Type: ContentTypeText,
					},
				}
			}
			eventCh <- StreamEvent{
				Type:              StreamEventContentDelta,
				ContentBlockIndex: contentBlockIndex,
				Delta: &StreamDelta{
					Text: delta.Content,
				},
			}
		}

		// Handle tool calls
		for _, tc := range delta.ToolCalls {
			idx := tc.Index
			if _, exists := currentToolCalls[idx]; !exists {
				// New tool call starting
				if contentBlockIndex > 0 || delta.Content == "" {
					// Close previous content block if exists
					if !inToolCall && contentBlockIndex > 0 {
						eventCh <- StreamEvent{
							Type:              StreamEventContentStop,
							ContentBlockIndex: contentBlockIndex - 1,
						}
					}
				}

				currentToolCalls[idx] = &openAIToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: openAIFunctionCall{
						Name:      tc.Function.Name,
						Arguments: "",
					},
				}
				inToolCall = true
				contentBlockIndex++

				eventCh <- StreamEvent{
					Type:              StreamEventContentStart,
					ContentBlockIndex: contentBlockIndex,
					ContentBlock: &ContentBlock{
						Type:      ContentTypeToolUse,
						ToolUseID: tc.ID,
						ToolName:  tc.Function.Name,
					},
				}
			}

			// Accumulate arguments
			if tc.Function.Arguments != "" {
				currentToolCalls[idx].Function.Arguments += tc.Function.Arguments
				eventCh <- StreamEvent{
					Type:              StreamEventContentDelta,
					ContentBlockIndex: contentBlockIndex,
					Delta: &StreamDelta{
						ToolInput: tc.Function.Arguments,
					},
				}
			}
		}

		// Handle finish reason
		if chunk.Choices[0].FinishReason != "" {
			var stopReason StopReason
			switch chunk.Choices[0].FinishReason {
			case "stop":
				stopReason = StopReasonEnd
			case "length":
				stopReason = StopReasonMaxTokens
			case "tool_calls":
				stopReason = StopReasonToolUse
			default:
				stopReason = StopReasonEnd
			}

			// Close any open content blocks
			eventCh <- StreamEvent{
				Type:              StreamEventContentStop,
				ContentBlockIndex: contentBlockIndex,
			}

			eventCh <- StreamEvent{
				Type:       StreamEventStop,
				StopReason: stopReason,
			}
		}
	}
}

// handleHTTPError converts an HTTP error to a ProviderError.
func (p *OpenAIProvider) handleHTTPError(err error) error {
	return &ProviderError{
		Code:      ErrCodeServerError,
		Message:   fmt.Sprintf("HTTP error: %v", err),
		Retryable: true,
	}
}

// parseErrorResponse parses an error response from the OpenAI API.
func (p *OpenAIProvider) parseErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp openAIErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		return &ProviderError{
			Code:       ErrCodeServerError,
			Message:    fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			StatusCode: resp.StatusCode,
			Retryable:  resp.StatusCode >= 500,
		}
	}

	code := ErrCodeServerError
	retryable := false

	switch resp.StatusCode {
	case http.StatusUnauthorized:
		code = ErrCodeAuthentication
	case http.StatusForbidden:
		code = ErrCodePermission
	case http.StatusNotFound:
		code = ErrCodeNotFound
	case http.StatusTooManyRequests:
		code = ErrCodeRateLimit
		retryable = true
	case http.StatusBadRequest:
		code = ErrCodeInvalidRequest
		if strings.Contains(errResp.Error.Message, "context_length") {
			code = ErrCodeContextLength
		}
	default:
		if resp.StatusCode >= 500 {
			retryable = true
		}
	}

	return &ProviderError{
		Code:       code,
		Message:    errResp.Error.Message,
		StatusCode: resp.StatusCode,
		Retryable:  retryable,
		Details: map[string]interface{}{
			"type":  errResp.Error.Type,
			"param": errResp.Error.Param,
			"code":  errResp.Error.Code,
		},
	}
}

// isReasoningModel returns true if the model is an o1 or o3 series model.
func isReasoningModel(model string) bool {
	return strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3")
}

// defaultOpenAIModels returns the default list of supported OpenAI models.
func defaultOpenAIModels() []string {
	return []string{
		ModelGPT4o,
		ModelGPT4oMini,
		ModelGPT4Turbo,
		ModelGPT4,
		ModelGPT35Turbo,
		ModelO1,
		ModelO1Mini,
		ModelO1Preview,
		ModelO3Mini,
		ModelGPT53,
		ModelGPT53Codex,
		ModelGPT53CodexSpark,
	}
}

// defaultOpenAIModelInfo returns model information for all supported models.
func defaultOpenAIModelInfo() map[string]*ModelInfo {
	return map[string]*ModelInfo{
		ModelGPT4o: {
			ID:       ModelGPT4o,
			Name:     "GPT-4o",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  2.50,
			PricePerMOutputTokens: 10.00,
		},
		ModelGPT4oMini: {
			ID:       ModelGPT4oMini,
			Name:     "GPT-4o Mini",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  0.15,
			PricePerMOutputTokens: 0.60,
		},
		ModelGPT4Turbo: {
			ID:       ModelGPT4Turbo,
			Name:     "GPT-4 Turbo",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  10.00,
			PricePerMOutputTokens: 30.00,
		},
		ModelGPT4: {
			ID:       ModelGPT4,
			Name:     "GPT-4",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 8192,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  30.00,
			PricePerMOutputTokens: 60.00,
		},
		ModelGPT35Turbo: {
			ID:       ModelGPT35Turbo,
			Name:     "GPT-3.5 Turbo",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 16385,
				MaxOutputTokens:  4096,
			},
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 1.50,
		},
		ModelO1: {
			ID:       ModelO1,
			Name:     "O1",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  100000,
			},
			PricePerMInputTokens:  15.00,
			PricePerMOutputTokens: 60.00,
		},
		ModelO1Mini: {
			ID:       ModelO1Mini,
			Name:     "O1 Mini",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  65536,
			},
			PricePerMInputTokens:  3.00,
			PricePerMOutputTokens: 12.00,
		},
		ModelO1Preview: {
			ID:         ModelO1Preview,
			Name:       "O1 Preview",
			Provider:   "openai",
			Deprecated: true,
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        true,
				MaxContextTokens: 128000,
				MaxOutputTokens:  32768,
			},
			PricePerMInputTokens:  15.00,
			PricePerMOutputTokens: 60.00,
		},
		ModelO3Mini: {
			ID:       ModelO3Mini,
			Name:     "o3-mini",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     false, // o3-mini uses developer messages, not system prompts
				MultiTurn:        true,
				MaxContextTokens: 200000,
				MaxOutputTokens:  100000,
			},
			PricePerMInputTokens:  1.10,
			PricePerMOutputTokens: 4.40,
		},
		ModelGPT53: {
			ID:       ModelGPT53,
			Name:     "GPT-5.3",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 256000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  5.00,
			PricePerMOutputTokens: 15.00,
		},
		ModelGPT53Codex: {
			ID:       ModelGPT53Codex,
			Name:     "GPT-5.3 Codex",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 256000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  5.00,
			PricePerMOutputTokens: 15.00,
		},
		ModelGPT53CodexSpark: {
			ID:       ModelGPT53CodexSpark,
			Name:     "GPT-5.3 Codex Spark",
			Provider: "openai",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 256000,
				MaxOutputTokens:  16384,
			},
			PricePerMInputTokens:  5.00,
			PricePerMOutputTokens: 15.00,
		},
	}
}

// OpenAI API request/response types

type openAIChatCompletionRequest struct {
	Model               string               `json:"model"`
	Messages            []openAIMessage      `json:"messages"`
	Stream              bool                 `json:"stream,omitempty"`
	MaxCompletionTokens int                  `json:"max_completion_tokens,omitempty"`
	Temperature         *float64             `json:"temperature,omitempty"`
	TopP                *float64             `json:"top_p,omitempty"`
	Stop                []string             `json:"stop,omitempty"`
	Tools               []openAITool         `json:"tools,omitempty"`
	ToolChoice          interface{}          `json:"tool_choice,omitempty"`
	StreamOptions       *openAIStreamOptions `json:"stream_options,omitempty"`
}

type openAIStreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Index    int                `json:"index,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage openAIUsage `json:"usage"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIStreamChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int         `json:"index"`
		Delta        openAIDelta `json:"delta"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *openAIUsage `json:"usage,omitempty"`
}

type openAIDelta struct {
	Role      string           `json:"role,omitempty"`
	Content   string           `json:"content,omitempty"`
	ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
}

type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Param   string `json:"param,omitempty"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}
