// Package agent provides the Anthropic provider implementation.
// This file implements the ProviderAdapter interface for Anthropic's Claude models.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// AnthropicAPIURL is the base URL for the Anthropic API
	AnthropicAPIURL = "https://api.anthropic.com/v1/messages"

	// AnthropicAPIVersion is the API version header value
	AnthropicAPIVersion = "2023-06-01"

	// DefaultMaxTokens is the default maximum tokens for responses
	DefaultMaxTokens = 4096
)

// AnthropicProvider implements the ProviderAdapter interface for Anthropic's Claude models.
type AnthropicProvider struct {
	apiKey     string
	httpClient *http.Client
	models     map[string]*ModelInfo
}

// AnthropicConfig holds configuration options for the Anthropic provider.
type AnthropicConfig struct {
	APIKey     string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// NewAnthropicProvider creates a new Anthropic provider with the given configuration.
func NewAnthropicProvider(cfg AnthropicConfig) (*AnthropicProvider, error) {
	apiKey := cfg.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil, &ProviderError{
			Code:    ErrCodeAuthentication,
			Message: "ANTHROPIC_API_KEY is required",
		}
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
		httpClient = &http.Client{Timeout: timeout}
	}

	provider := &AnthropicProvider{
		apiKey:     apiKey,
		httpClient: httpClient,
		models:     make(map[string]*ModelInfo),
	}

	// Initialize model registry
	provider.initModels()

	return provider, nil
}

// initModels initializes the available Anthropic models.
func (p *AnthropicProvider) initModels() {
	// Claude 4 models (latest)
	p.models["claude-4-6-opus-20260215"] = &ModelInfo{
		ID:       "claude-4-6-opus-20260215",
		Name:     "Claude 4.6 Opus",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 400000,
			MaxOutputTokens:  64000,
		},
		PricePerMInputTokens:  15.0,
		PricePerMOutputTokens: 75.0,
	}

	p.models["claude-4-6-sonnet-20260215"] = &ModelInfo{
		ID:       "claude-4-6-sonnet-20260215",
		Name:     "Claude 4.6 Sonnet",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 400000,
			MaxOutputTokens:  64000,
		},
		PricePerMInputTokens:  3.0,
		PricePerMOutputTokens: 15.0,
	}

	p.models["claude-opus-4-5-20251101"] = &ModelInfo{
		ID:       "claude-opus-4-5-20251101",
		Name:     "Claude Opus 4.5",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  32000,
		},
		PricePerMInputTokens:  15.0,
		PricePerMOutputTokens: 75.0,
	}

	p.models["claude-sonnet-4-20250514"] = &ModelInfo{
		ID:       "claude-sonnet-4-20250514",
		Name:     "Claude Sonnet 4",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  64000,
		},
		PricePerMInputTokens:  3.0,
		PricePerMOutputTokens: 15.0,
	}

	// Claude 3.5 models
	p.models["claude-3-5-sonnet-20241022"] = &ModelInfo{
		ID:       "claude-3-5-sonnet-20241022",
		Name:     "Claude 3.5 Sonnet",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  8192,
		},
		PricePerMInputTokens:  3.0,
		PricePerMOutputTokens: 15.0,
	}

	p.models["claude-3-5-haiku-20241022"] = &ModelInfo{
		ID:       "claude-3-5-haiku-20241022",
		Name:     "Claude 3.5 Haiku",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  8192,
		},
		PricePerMInputTokens:  1.0,
		PricePerMOutputTokens: 5.0,
	}

	// Claude 3 models
	p.models["claude-3-opus-20240229"] = &ModelInfo{
		ID:       "claude-3-opus-20240229",
		Name:     "Claude 3 Opus",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  4096,
		},
		PricePerMInputTokens:  15.0,
		PricePerMOutputTokens: 75.0,
	}

	p.models["claude-3-sonnet-20240229"] = &ModelInfo{
		ID:       "claude-3-sonnet-20240229",
		Name:     "Claude 3 Sonnet",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  4096,
		},
		PricePerMInputTokens:  3.0,
		PricePerMOutputTokens: 15.0,
	}

	p.models["claude-3-haiku-20240307"] = &ModelInfo{
		ID:       "claude-3-haiku-20240307",
		Name:     "Claude 3 Haiku",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming:        true,
			Vision:           true,
			ToolUse:          true,
			SystemPrompt:     true,
			MultiTurn:        true,
			MaxContextTokens: 200000,
			MaxOutputTokens:  4096,
		},
		PricePerMInputTokens:  0.25,
		PricePerMOutputTokens: 1.25,
	}
}

// GetName returns the provider's identifier.
func (p *AnthropicProvider) GetName() string {
	return "anthropic"
}

// GetModels returns the list of available model IDs.
func (p *AnthropicProvider) GetModels() []string {
	models := make([]string, 0, len(p.models))
	for id := range p.models {
		models = append(models, id)
	}
	return models
}

// GetModelInfo returns detailed information about a specific model.
func (p *AnthropicProvider) GetModelInfo(modelID string) (*ModelInfo, error) {
	info, ok := p.models[modelID]
	if !ok {
		return nil, &ProviderError{
			Code:    ErrCodeNotFound,
			Message: fmt.Sprintf("model %q not found", modelID),
		}
	}
	return info, nil
}

// GetCapabilities returns the capabilities of a specific model.
func (p *AnthropicProvider) GetCapabilities(modelID string) (*ProviderCapabilities, error) {
	info, err := p.GetModelInfo(modelID)
	if err != nil {
		return nil, err
	}
	return &info.Capabilities, nil
}

// ValidateRequest checks if a request is valid for this provider.
func (p *AnthropicProvider) ValidateRequest(req Request) error {
	if req.Model == "" {
		return &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: "model is required",
		}
	}

	if len(req.Messages) == 0 {
		return &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: "at least one message is required",
		}
	}

	// Validate model exists
	if _, ok := p.models[req.Model]; !ok {
		return &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("unknown model: %s", req.Model),
		}
	}

	return nil
}

// EstimateTokens estimates the token count for the given content.
// This uses a simple heuristic; actual tokenization would require tiktoken.
func (p *AnthropicProvider) EstimateTokens(content string) int {
	// Anthropic uses roughly 4 characters per token on average
	return len(content) / 4
}

// anthropicRequest is the request format for the Anthropic API.
type anthropicRequest struct {
	Model       string               `json:"model"`
	Messages    []anthropicMessage   `json:"messages"`
	System      string               `json:"system,omitempty"`
	MaxTokens   int                  `json:"max_tokens"`
	Temperature *float64             `json:"temperature,omitempty"`
	TopP        *float64             `json:"top_p,omitempty"`
	Stop        []string             `json:"stop_sequences,omitempty"`
	Tools       []anthropicTool      `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice `json:"tool_choice,omitempty"`
	Stream      bool                 `json:"stream,omitempty"`
	Metadata    map[string]string    `json:"metadata,omitempty"`
}

// anthropicMessage represents a message in the Anthropic format.
type anthropicMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

// anthropicTextContent represents text content in a message.
type anthropicTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// anthropicImageContent represents image content in a message.
type anthropicImageContent struct {
	Type   string               `json:"type"`
	Source anthropicImageSource `json:"source"`
}

// anthropicImageSource contains image data.
type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// anthropicToolUseContent represents a tool use in a message.
type anthropicToolUseContent struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// anthropicToolResultContent represents a tool result in a message.
type anthropicToolResultContent struct {
	Type      string `json:"type"`
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

// anthropicTool represents a tool definition for Anthropic.
type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// anthropicToolChoice represents tool choice configuration.
type anthropicToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

// anthropicResponse is the response format from the Anthropic API.
type anthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []anthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence string                  `json:"stop_sequence,omitempty"`
	Usage        anthropicUsage          `json:"usage"`
}

// anthropicContentBlock represents a content block in the response.
type anthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// anthropicUsage tracks token usage.
type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// anthropicError represents an error response from the API.
type anthropicError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// Complete sends a request and returns a complete response.
func (p *AnthropicProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := p.ValidateRequest(req); err != nil {
		return nil, err
	}

	anthropicReq, err := p.buildRequest(req)
	if err != nil {
		return nil, err
	}
	anthropicReq.Stream = false

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", AnthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to create request: %v", err),
		}
	}

	p.setHeaders(httpReq)

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, p.handleHTTPError(err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, p.parseErrorResponse(httpResp)
	}

	var anthropicResp anthropicResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&anthropicResp); err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeServerError,
			Message: fmt.Sprintf("failed to decode response: %v", err),
		}
	}

	return p.convertResponse(anthropicResp), nil
}

// Stream sends a request and returns a channel of streaming events.
func (p *AnthropicProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	if err := p.ValidateRequest(req); err != nil {
		return nil, err
	}

	anthropicReq, err := p.buildRequest(req)
	if err != nil {
		return nil, err
	}
	anthropicReq.Stream = true

	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", AnthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to create request: %v", err),
		}
	}

	p.setHeaders(httpReq)

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, p.handleHTTPError(err)
	}

	if httpResp.StatusCode != http.StatusOK {
		defer httpResp.Body.Close()
		return nil, p.parseErrorResponse(httpResp)
	}

	ch := make(chan StreamEvent, 100)
	go p.processStream(ctx, httpResp.Body, ch)

	return ch, nil
}

// setHeaders sets the required headers for Anthropic API requests.
func (p *AnthropicProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", AnthropicAPIVersion)
}

// buildRequest converts a unified Request to an Anthropic request.
func (p *AnthropicProvider) buildRequest(req Request) (*anthropicRequest, error) {
	anthropicReq := &anthropicRequest{
		Model:       req.Model,
		System:      req.System,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stop:        req.StopSequences,
	}

	if anthropicReq.MaxTokens == 0 {
		anthropicReq.MaxTokens = DefaultMaxTokens
	}

	// Convert messages
	for _, msg := range req.Messages {
		anthropicMsg, err := p.convertMessage(msg)
		if err != nil {
			return nil, err
		}
		anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		anthropicReq.Tools = append(anthropicReq.Tools, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}

	// Convert tool choice
	if req.ToolChoice != "" {
		switch req.ToolChoice {
		case "auto":
			anthropicReq.ToolChoice = &anthropicToolChoice{Type: "auto"}
		case "any":
			anthropicReq.ToolChoice = &anthropicToolChoice{Type: "any"}
		case "none":
			// Don't include tool_choice for "none", just don't send tools
			anthropicReq.Tools = nil
		default:
			// Specific tool name
			anthropicReq.ToolChoice = &anthropicToolChoice{
				Type: "tool",
				Name: req.ToolChoice,
			}
		}
	}

	return anthropicReq, nil
}

// convertMessage converts a unified Message to an Anthropic message.
func (p *AnthropicProvider) convertMessage(msg Message) (anthropicMessage, error) {
	anthropicMsg := anthropicMessage{
		Role:    string(msg.Role),
		Content: make([]interface{}, 0, len(msg.Content)),
	}

	// Handle legacy text field
	if msg.Text != "" && len(msg.Content) == 0 {
		anthropicMsg.Content = append(anthropicMsg.Content, anthropicTextContent{
			Type: "text",
			Text: msg.Text,
		})
		return anthropicMsg, nil
	}

	// Map tool role to user role with tool_result content
	if msg.Role == RoleTool {
		anthropicMsg.Role = "user"
	}

	for _, block := range msg.Content {
		switch block.Type {
		case ContentTypeText:
			anthropicMsg.Content = append(anthropicMsg.Content, anthropicTextContent{
				Type: "text",
				Text: block.Text,
			})

		case ContentTypeImage:
			var imageData string
			if block.ImageData != nil {
				imageData = base64.StdEncoding.EncodeToString(block.ImageData)
			} else if block.ImageURL != "" {
				// For URL-based images, we'd need to fetch and convert
				// For now, skip URLs and only support base64
				continue
			}
			if imageData != "" {
				anthropicMsg.Content = append(anthropicMsg.Content, anthropicImageContent{
					Type: "image",
					Source: anthropicImageSource{
						Type:      "base64",
						MediaType: block.ImageMedia,
						Data:      imageData,
					},
				})
			}

		case ContentTypeToolUse:
			anthropicMsg.Content = append(anthropicMsg.Content, anthropicToolUseContent{
				Type:  "tool_use",
				ID:    block.ToolUseID,
				Name:  block.ToolName,
				Input: block.ToolInput,
			})

		case ContentTypeToolResult:
			content := block.ToolOutput
			if block.ToolError != "" {
				content = block.ToolError
			}
			anthropicMsg.Content = append(anthropicMsg.Content, anthropicToolResultContent{
				Type:      "tool_result",
				ToolUseID: block.ToolResultID,
				Content:   content,
				IsError:   block.ToolError != "",
			})
		}
	}

	return anthropicMsg, nil
}

// convertResponse converts an Anthropic response to a unified Response.
func (p *AnthropicProvider) convertResponse(resp anthropicResponse) *Response {
	response := &Response{
		ID:    resp.ID,
		Model: resp.Model,
		Usage: Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
			CacheReadTokens:  resp.Usage.CacheReadInputTokens,
			CacheWriteTokens: resp.Usage.CacheCreationInputTokens,
		},
	}

	// Convert stop reason
	switch resp.StopReason {
	case "end_turn":
		response.StopReason = StopReasonEnd
	case "max_tokens":
		response.StopReason = StopReasonMaxTokens
	case "tool_use":
		response.StopReason = StopReasonToolUse
	case "stop_sequence":
		response.StopReason = StopReasonStop
	default:
		response.StopReason = StopReasonEnd
	}

	// Convert content blocks
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			response.Content = append(response.Content, ContentBlock{
				Type: ContentTypeText,
				Text: block.Text,
			})
		case "tool_use":
			response.Content = append(response.Content, ContentBlock{
				Type:      ContentTypeToolUse,
				ToolUseID: block.ID,
				ToolName:  block.Name,
				ToolInput: block.Input,
			})
		}
	}

	return response
}

// processStream processes the SSE stream from Anthropic.
func (p *AnthropicProvider) processStream(ctx context.Context, body io.ReadCloser, ch chan<- StreamEvent) {
	defer close(ch)
	defer body.Close()

	scanner := bufio.NewScanner(body)
	// Increase buffer size for large streaming responses
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var currentBlockIndex int
	var usage *Usage

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			ch <- StreamEvent{
				Type:  StreamEventError,
				Error: ctx.Err(),
			}
			return
		default:
		}

		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE event
		if strings.HasPrefix(line, "event: ") {
			continue // Event type is in the data
		}

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event struct {
			Type         string `json:"type"`
			Index        int    `json:"index"`
			ContentBlock *struct {
				Type  string          `json:"type"`
				ID    string          `json:"id,omitempty"`
				Name  string          `json:"name,omitempty"`
				Text  string          `json:"text,omitempty"`
				Input json.RawMessage `json:"input,omitempty"`
			} `json:"content_block,omitempty"`
			Delta *struct {
				Type        string `json:"type"`
				Text        string `json:"text,omitempty"`
				PartialJSON string `json:"partial_json,omitempty"`
				StopReason  string `json:"stop_reason,omitempty"`
			} `json:"delta,omitempty"`
			Message *struct {
				ID         string         `json:"id"`
				Model      string         `json:"model"`
				StopReason string         `json:"stop_reason,omitempty"`
				Usage      anthropicUsage `json:"usage"`
			} `json:"message,omitempty"`
			Usage *anthropicUsage `json:"usage,omitempty"`
		}

		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue // Skip malformed events
		}

		switch event.Type {
		case "message_start":
			if event.Message != nil {
				usage = &Usage{
					PromptTokens:     event.Message.Usage.InputTokens,
					CacheReadTokens:  event.Message.Usage.CacheReadInputTokens,
					CacheWriteTokens: event.Message.Usage.CacheCreationInputTokens,
				}
			}
			ch <- StreamEvent{
				Type: StreamEventStart,
			}

		case "content_block_start":
			currentBlockIndex = event.Index
			if event.ContentBlock != nil {
				var block *ContentBlock
				switch event.ContentBlock.Type {
				case "text":
					block = &ContentBlock{
						Type: ContentTypeText,
						Text: event.ContentBlock.Text,
					}
				case "tool_use":
					block = &ContentBlock{
						Type:      ContentTypeToolUse,
						ToolUseID: event.ContentBlock.ID,
						ToolName:  event.ContentBlock.Name,
					}
				}
				ch <- StreamEvent{
					Type:              StreamEventContentStart,
					ContentBlock:      block,
					ContentBlockIndex: currentBlockIndex,
				}
			}

		case "content_block_delta":
			if event.Delta != nil {
				delta := &StreamDelta{}
				switch event.Delta.Type {
				case "text_delta":
					delta.Text = event.Delta.Text
				case "input_json_delta":
					delta.ToolInput = event.Delta.PartialJSON
				}
				ch <- StreamEvent{
					Type:              StreamEventContentDelta,
					Delta:             delta,
					ContentBlockIndex: event.Index,
				}
			}

		case "content_block_stop":
			ch <- StreamEvent{
				Type:              StreamEventContentStop,
				ContentBlockIndex: event.Index,
			}

		case "message_delta":
			if event.Delta != nil && event.Delta.StopReason != "" {
				var stopReason StopReason
				switch event.Delta.StopReason {
				case "end_turn":
					stopReason = StopReasonEnd
				case "max_tokens":
					stopReason = StopReasonMaxTokens
				case "tool_use":
					stopReason = StopReasonToolUse
				case "stop_sequence":
					stopReason = StopReasonStop
				}
				// Update usage with output tokens
				if event.Usage != nil && usage != nil {
					usage.CompletionTokens = event.Usage.OutputTokens
					usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
				}
				ch <- StreamEvent{
					Type:       StreamEventStop,
					StopReason: stopReason,
					Usage:      usage,
				}
			}

		case "message_stop":
			// Final event, already handled by message_delta
			continue

		case "ping":
			ch <- StreamEvent{
				Type: StreamEventPing,
			}

		case "error":
			ch <- StreamEvent{
				Type:  StreamEventError,
				Error: fmt.Errorf("stream error: %s", data),
			}
			return
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- StreamEvent{
			Type:  StreamEventError,
			Error: fmt.Errorf("stream scan error: %w", err),
		}
	}
}

// handleHTTPError converts HTTP errors to ProviderError.
func (p *AnthropicProvider) handleHTTPError(err error) error {
	if err == context.DeadlineExceeded {
		return &ProviderError{
			Code:      ErrCodeTimeout,
			Message:   "request timed out",
			Retryable: true,
		}
	}
	if err == context.Canceled {
		return &ProviderError{
			Code:    ErrCodeTimeout,
			Message: "request canceled",
		}
	}
	return &ProviderError{
		Code:      ErrCodeServerError,
		Message:   fmt.Sprintf("HTTP error: %v", err),
		Retryable: true,
	}
}

// parseErrorResponse parses error responses from the Anthropic API.
func (p *AnthropicProvider) parseErrorResponse(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &ProviderError{
			Code:       ErrCodeServerError,
			Message:    fmt.Sprintf("failed to read error response: %v", err),
			StatusCode: resp.StatusCode,
		}
	}

	var apiErr anthropicError
	if err := json.Unmarshal(body, &apiErr); err != nil {
		return &ProviderError{
			Code:       ErrCodeServerError,
			Message:    string(body),
			StatusCode: resp.StatusCode,
		}
	}

	providerErr := &ProviderError{
		Message:    apiErr.Error.Message,
		StatusCode: resp.StatusCode,
	}

	// Map Anthropic error types to our error codes
	switch apiErr.Error.Type {
	case "authentication_error":
		providerErr.Code = ErrCodeAuthentication
	case "permission_error":
		providerErr.Code = ErrCodePermission
	case "not_found_error":
		providerErr.Code = ErrCodeNotFound
	case "rate_limit_error":
		providerErr.Code = ErrCodeRateLimit
		providerErr.Retryable = true
		// Check for Retry-After header
		if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
			if seconds, err := time.ParseDuration(retryAfter + "s"); err == nil {
				providerErr.RetryAfter = &seconds
			}
		}
	case "overloaded_error":
		providerErr.Code = ErrCodeServerError
		providerErr.Retryable = true
	case "invalid_request_error":
		providerErr.Code = ErrCodeInvalidRequest
		// Check for context length issues
		if strings.Contains(apiErr.Error.Message, "context") {
			providerErr.Code = ErrCodeContextLength
		}
	default:
		providerErr.Code = ErrCodeServerError
		if resp.StatusCode >= 500 {
			providerErr.Retryable = true
		}
	}

	return providerErr
}

// AnthropicToolTranslator implements ToolSchemaTranslator for Anthropic.
type AnthropicToolTranslator struct {
	*BaseToolSchemaTranslator
}

// NewAnthropicToolTranslator creates a new Anthropic tool translator.
func NewAnthropicToolTranslator() *AnthropicToolTranslator {
	return &AnthropicToolTranslator{
		BaseToolSchemaTranslator: NewBaseToolSchemaTranslator("anthropic"),
	}
}

// TranslateToProvider converts a unified ToolDefinition to Anthropic format.
func (t *AnthropicToolTranslator) TranslateToProvider(tool ToolDefinition) (interface{}, error) {
	if err := t.ValidateToolDefinition(tool); err != nil {
		return nil, err
	}

	return anthropicTool{
		Name:        tool.Name,
		Description: tool.Description,
		InputSchema: tool.InputSchema,
	}, nil
}

// TranslateFromProvider converts an Anthropic tool call to unified format.
func (t *AnthropicToolTranslator) TranslateFromProvider(providerToolCall interface{}) (*ContentBlock, error) {
	// Handle different input types
	switch tc := providerToolCall.(type) {
	case anthropicToolUseContent:
		return &ContentBlock{
			Type:      ContentTypeToolUse,
			ToolUseID: tc.ID,
			ToolName:  tc.Name,
			ToolInput: tc.Input,
		}, nil

	case *anthropicToolUseContent:
		return &ContentBlock{
			Type:      ContentTypeToolUse,
			ToolUseID: tc.ID,
			ToolName:  tc.Name,
			ToolInput: tc.Input,
		}, nil

	case map[string]interface{}:
		// Handle generic map format
		id, _ := tc["id"].(string)
		name, _ := tc["name"].(string)

		var input json.RawMessage
		if inputData, ok := tc["input"]; ok {
			data, err := json.Marshal(inputData)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal tool input: %w", err)
			}
			input = data
		}

		return &ContentBlock{
			Type:      ContentTypeToolUse,
			ToolUseID: id,
			ToolName:  name,
			ToolInput: input,
		}, nil

	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrInvalidToolCall, providerToolCall)
	}
}

// TranslateToolResult converts a unified tool result to Anthropic format.
func (t *AnthropicToolTranslator) TranslateToolResult(result ContentBlock) (interface{}, error) {
	if result.Type != ContentTypeToolResult {
		return nil, fmt.Errorf("expected tool_result content type, got %s", result.Type)
	}

	if result.ToolResultID == "" {
		return nil, ErrMissingToolID
	}

	content := result.ToolOutput
	isError := false
	if result.ToolError != "" {
		content = result.ToolError
		isError = true
	}

	return anthropicToolResultContent{
		Type:      "tool_result",
		ToolUseID: result.ToolResultID,
		Content:   content,
		IsError:   isError,
	}, nil
}
