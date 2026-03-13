// Package agent provides the Gemini provider implementation.
// This file implements the ProviderAdapter interface for Google's Gemini models.
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
	// DefaultGeminiBaseURL is the default Gemini API endpoint.
	DefaultGeminiBaseURL = "https://generativelanguage.googleapis.com/v1beta"

	// DefaultGeminiTimeout is the default HTTP client timeout.
	DefaultGeminiTimeout = 120 * time.Second
)

// Gemini model identifiers
const (
	ModelGemini20FlashExp    = "gemini-2.0-flash-exp"
	ModelGemini20Flash       = "gemini-2.0-flash"
	ModelGemini15Pro         = "gemini-1.5-pro"
	ModelGemini15ProLatest   = "gemini-1.5-pro-latest"
	ModelGemini15Flash       = "gemini-1.5-flash"
	ModelGemini15FlashLatest = "gemini-1.5-flash-latest"
	ModelGemini15Flash8B     = "gemini-1.5-flash-8b"
	ModelGemini31ProPreview  = "gemini-3.1-pro-preview"
	ModelGemini31FlashPreview = "gemini-3.1-flash-preview"
	ModelGeminiPro           = "gemini-pro"
	ModelGeminiProVision     = "gemini-pro-vision"
)

// GeminiProviderConfig holds configuration for the Gemini provider.
type GeminiProviderConfig struct {
	// APIKey is the Google AI API key.
	APIKey string

	// BaseURL is the API base URL (defaults to DefaultGeminiBaseURL).
	BaseURL string

	// Timeout is the HTTP client timeout.
	Timeout time.Duration

	// HTTPClient is an optional custom HTTP client.
	HTTPClient *http.Client
}

// GeminiProvider implements ProviderAdapter for Google's Gemini API.
type GeminiProvider struct {
	config     GeminiProviderConfig
	httpClient *http.Client
	models     []string
	modelInfo  map[string]*ModelInfo
}

// Compile-time check that GeminiProvider implements ProviderAdapter.
var _ ProviderAdapter = (*GeminiProvider)(nil)

// NewGeminiProvider creates a new Gemini provider with the given configuration.
func NewGeminiProvider(config GeminiProviderConfig) (*GeminiProvider, error) {
	if config.APIKey == "" {
		// Try to get from environment
		config.APIKey = os.Getenv("GEMINI_API_KEY")
		if config.APIKey == "" {
			// Also check for GOOGLE_API_KEY
			config.APIKey = os.Getenv("GOOGLE_API_KEY")
		}
		if config.APIKey == "" {
			return nil, &ProviderError{
				Code:    ErrCodeAuthentication,
				Message: "Gemini API key is required (set GEMINI_API_KEY or GOOGLE_API_KEY environment variable or provide in config)",
			}
		}
	}

	if config.BaseURL == "" {
		config.BaseURL = DefaultGeminiBaseURL
	}

	if config.Timeout == 0 {
		config.Timeout = DefaultGeminiTimeout
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	p := &GeminiProvider{
		config:     config,
		httpClient: httpClient,
		models:     defaultGeminiModels(),
		modelInfo:  defaultGeminiModelInfo(),
	}

	return p, nil
}

// NewGeminiProviderFromEnv creates a new Gemini provider using environment variables.
func NewGeminiProviderFromEnv() (*GeminiProvider, error) {
	return NewGeminiProvider(GeminiProviderConfig{})
}

// GetName returns the provider identifier.
func (p *GeminiProvider) GetName() string {
	return "gemini"
}

// GetModels returns the list of available model IDs.
func (p *GeminiProvider) GetModels() []string {
	return p.models
}

// GetModelInfo returns detailed information about a specific model.
func (p *GeminiProvider) GetModelInfo(modelID string) (*ModelInfo, error) {
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
func (p *GeminiProvider) GetCapabilities(modelID string) (*ProviderCapabilities, error) {
	info, err := p.GetModelInfo(modelID)
	if err != nil {
		return nil, err
	}
	return &info.Capabilities, nil
}

// Complete sends a request and returns a complete response.
func (p *GeminiProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if err := p.ValidateRequest(req); err != nil {
		return nil, err
	}

	geminiReq := p.buildGeminiRequest(req)
	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", p.config.BaseURL, req.Model, p.config.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
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

	var geminiResp geminiGenerateContentResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeServerError,
			Message: fmt.Sprintf("failed to decode response: %v", err),
		}
	}

	return p.convertResponse(&geminiResp, req.Model), nil
}

// Stream sends a request and returns a channel of streaming events.
func (p *GeminiProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	if err := p.ValidateRequest(req); err != nil {
		return nil, err
	}

	geminiReq := p.buildGeminiRequest(req)
	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, &ProviderError{
			Code:    ErrCodeInvalidRequest,
			Message: fmt.Sprintf("failed to marshal request: %v", err),
		}
	}

	url := fmt.Sprintf("%s/models/%s:streamGenerateContent?key=%s&alt=sse", p.config.BaseURL, req.Model, p.config.APIKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
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
	go p.processStreamResponse(ctx, resp, eventCh, req.Model)

	return eventCh, nil
}

// ValidateRequest checks if a request is valid for this provider.
func (p *GeminiProvider) ValidateRequest(req Request) error {
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

	return nil
}

// EstimateTokens estimates the token count for the given content.
// This is a rough estimation using ~4 characters per token.
func (p *GeminiProvider) EstimateTokens(content string) int {
	// Gemini uses a similar tokenization approach
	// we use approximately 4 characters per token (common heuristic)
	return len(content) / 4
}

// setHeaders sets the required HTTP headers for Gemini API requests.
func (p *GeminiProvider) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
}

// buildGeminiRequest converts a unified Request to Gemini's request format.
func (p *GeminiProvider) buildGeminiRequest(req Request) *geminiGenerateContentRequest {
	geminiReq := &geminiGenerateContentRequest{
		Contents: make([]geminiContent, 0),
	}

	// Add system instruction if present
	if req.System != "" {
		geminiReq.SystemInstruction = &geminiContent{
			Parts: []geminiPart{
				{Text: req.System},
			},
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		geminiContent := p.convertToGeminiContent(msg)
		if geminiContent != nil {
			geminiReq.Contents = append(geminiReq.Contents, *geminiContent)
		}
	}

	// Build generation config
	geminiReq.GenerationConfig = &geminiGenerationConfig{}

	if req.MaxTokens > 0 {
		geminiReq.GenerationConfig.MaxOutputTokens = req.MaxTokens
	}

	if req.Temperature != nil {
		geminiReq.GenerationConfig.Temperature = req.Temperature
	}

	if req.TopP != nil {
		geminiReq.GenerationConfig.TopP = req.TopP
	}

	if len(req.StopSequences) > 0 {
		geminiReq.GenerationConfig.StopSequences = req.StopSequences
	}

	// Convert tools
	if len(req.Tools) > 0 {
		geminiTools := make([]geminiTool, 0)
		functionDeclarations := make([]geminiFunctionDeclaration, len(req.Tools))
		for i, tool := range req.Tools {
			var params interface{}
			if len(tool.InputSchema) > 0 {
				_ = json.Unmarshal(tool.InputSchema, &params)
			}
			functionDeclarations[i] = geminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  params,
			}
		}
		geminiTools = append(geminiTools, geminiTool{
			FunctionDeclarations: functionDeclarations,
		})
		geminiReq.Tools = geminiTools

		// Set tool config based on tool choice
		switch req.ToolChoice {
		case "auto", "":
			geminiReq.ToolConfig = &geminiToolConfig{
				FunctionCallingConfig: &geminiFunctionCallingConfig{
					Mode: "AUTO",
				},
			}
		case "any":
			geminiReq.ToolConfig = &geminiToolConfig{
				FunctionCallingConfig: &geminiFunctionCallingConfig{
					Mode: "ANY",
				},
			}
		case "none":
			geminiReq.ToolConfig = &geminiToolConfig{
				FunctionCallingConfig: &geminiFunctionCallingConfig{
					Mode: "NONE",
				},
			}
		default:
			// Specific tool name
			geminiReq.ToolConfig = &geminiToolConfig{
				FunctionCallingConfig: &geminiFunctionCallingConfig{
					Mode:                 "ANY",
					AllowedFunctionNames: []string{req.ToolChoice},
				},
			}
		}
	}

	return geminiReq
}

// convertToGeminiContent converts a unified Message to Gemini's content format.
func (p *GeminiProvider) convertToGeminiContent(msg Message) *geminiContent {
	content := &geminiContent{
		Parts: make([]geminiPart, 0),
	}

	// Map role
	switch msg.Role {
	case RoleUser:
		content.Role = "user"
	case RoleAssistant:
		content.Role = "model"
	case RoleTool:
		// Tool results are sent as user with function response
		content.Role = "user"
	case RoleSystem:
		// System is handled via SystemInstruction
		return nil
	}

	// Handle legacy text field
	if msg.Text != "" && len(msg.Content) == 0 {
		content.Parts = append(content.Parts, geminiPart{Text: msg.Text})
		return content
	}

	// Convert content blocks
	for _, block := range msg.Content {
		switch block.Type {
		case ContentTypeText:
			content.Parts = append(content.Parts, geminiPart{Text: block.Text})

		case ContentTypeImage:
			var imageData string
			if block.ImageData != nil {
				imageData = base64.StdEncoding.EncodeToString(block.ImageData)
			}
			if imageData != "" {
				content.Parts = append(content.Parts, geminiPart{
					InlineData: &geminiBlob{
						MimeType: block.ImageMedia,
						Data:     imageData,
					},
				})
			}

		case ContentTypeToolUse:
			// Assistant's tool calls are function calls
			var args interface{}
			if len(block.ToolInput) > 0 {
				_ = json.Unmarshal(block.ToolInput, &args)
			}
			content.Parts = append(content.Parts, geminiPart{
				FunctionCall: &geminiFunctionCall{
					Name: block.ToolName,
					Args: args,
				},
			})

		case ContentTypeToolResult:
			// Tool results are function responses
			content.Parts = append(content.Parts, geminiPart{
				FunctionResponse: &geminiFunctionResponse{
					Name: block.ToolResultID, // Use ToolResultID as function name mapping
					Response: map[string]interface{}{
						"result": block.ToolOutput,
						"error":  block.ToolError,
					},
				},
			})
		}
	}

	return content
}

// convertResponse converts a Gemini response to the unified format.
func (p *GeminiProvider) convertResponse(resp *geminiGenerateContentResponse, model string) *Response {
	response := &Response{
		ID:      fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
		Model:   model,
		Content: make([]ContentBlock, 0),
	}

	// Extract usage metadata
	if resp.UsageMetadata != nil {
		response.Usage = Usage{
			PromptTokens:     resp.UsageMetadata.PromptTokenCount,
			CompletionTokens: resp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      resp.UsageMetadata.TotalTokenCount,
		}
	}

	// Process candidates
	if len(resp.Candidates) > 0 {
		candidate := resp.Candidates[0]

		// Set stop reason
		switch candidate.FinishReason {
		case "STOP":
			response.StopReason = StopReasonEnd
		case "MAX_TOKENS":
			response.StopReason = StopReasonMaxTokens
		case "SAFETY":
			response.StopReason = StopReasonStop
		case "RECITATION":
			response.StopReason = StopReasonStop
		default:
			response.StopReason = StopReasonEnd
		}

		// Check for tool use in content
		hasToolUse := false

		// Convert content parts
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					response.Content = append(response.Content, ContentBlock{
						Type: ContentTypeText,
						Text: part.Text,
					})
				}
				if part.FunctionCall != nil {
					hasToolUse = true
					var inputJSON json.RawMessage
					if part.FunctionCall.Args != nil {
						data, _ := json.Marshal(part.FunctionCall.Args)
						inputJSON = data
					}
					response.Content = append(response.Content, ContentBlock{
						Type:      ContentTypeToolUse,
						ToolUseID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
						ToolName:  part.FunctionCall.Name,
						ToolInput: inputJSON,
					})
				}
			}
		}

		if hasToolUse {
			response.StopReason = StopReasonToolUse
		}
	}

	return response
}

// processStreamResponse reads the SSE stream and sends events to the channel.
func (p *GeminiProvider) processStreamResponse(ctx context.Context, resp *http.Response, eventCh chan<- StreamEvent, model string) {
	defer close(eventCh)
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)

	// Send start event
	select {
	case eventCh <- StreamEvent{Type: StreamEventStart}:
	case <-ctx.Done():
		return
	}

	contentBlockIndex := 0
	contentBlockStarted := false
	var totalUsage *Usage

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
		if data == "" {
			continue
		}

		var chunk geminiGenerateContentResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // Skip malformed chunks
		}

		// Update usage
		if chunk.UsageMetadata != nil {
			totalUsage = &Usage{
				PromptTokens:     chunk.UsageMetadata.PromptTokenCount,
				CompletionTokens: chunk.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      chunk.UsageMetadata.TotalTokenCount,
			}
		}

		if len(chunk.Candidates) == 0 {
			continue
		}

		candidate := chunk.Candidates[0]
		if candidate.Content == nil {
			// Check for finish reason without content
			if candidate.FinishReason != "" {
				var stopReason StopReason
				switch candidate.FinishReason {
				case "STOP":
					stopReason = StopReasonEnd
				case "MAX_TOKENS":
					stopReason = StopReasonMaxTokens
				default:
					stopReason = StopReasonEnd
				}

				if contentBlockStarted {
					eventCh <- StreamEvent{
						Type:              StreamEventContentStop,
						ContentBlockIndex: contentBlockIndex,
					}
				}

				eventCh <- StreamEvent{
					Type:       StreamEventStop,
					StopReason: stopReason,
					Usage:      totalUsage,
				}
			}
			continue
		}

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				if !contentBlockStarted {
					eventCh <- StreamEvent{
						Type:              StreamEventContentStart,
						ContentBlockIndex: contentBlockIndex,
						ContentBlock: &ContentBlock{
							Type: ContentTypeText,
						},
					}
					contentBlockStarted = true
				}

				eventCh <- StreamEvent{
					Type:              StreamEventContentDelta,
					ContentBlockIndex: contentBlockIndex,
					Delta: &StreamDelta{
						Text: part.Text,
					},
				}
			}

			if part.FunctionCall != nil {
				// Close text block if open
				if contentBlockStarted {
					eventCh <- StreamEvent{
						Type:              StreamEventContentStop,
						ContentBlockIndex: contentBlockIndex,
					}
					contentBlockIndex++
				}

				// Start function call block
				var inputJSON json.RawMessage
				if part.FunctionCall.Args != nil {
					data, _ := json.Marshal(part.FunctionCall.Args)
					inputJSON = data
				}

				eventCh <- StreamEvent{
					Type:              StreamEventContentStart,
					ContentBlockIndex: contentBlockIndex,
					ContentBlock: &ContentBlock{
						Type:      ContentTypeToolUse,
						ToolUseID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
						ToolName:  part.FunctionCall.Name,
						ToolInput: inputJSON,
					},
				}
				contentBlockStarted = true
			}
		}

		// Handle finish reason
		if candidate.FinishReason != "" {
			var stopReason StopReason
			switch candidate.FinishReason {
			case "STOP":
				stopReason = StopReasonEnd
			case "MAX_TOKENS":
				stopReason = StopReasonMaxTokens
			default:
				stopReason = StopReasonEnd
			}

			if contentBlockStarted {
				eventCh <- StreamEvent{
					Type:              StreamEventContentStop,
					ContentBlockIndex: contentBlockIndex,
				}
			}

			eventCh <- StreamEvent{
				Type:       StreamEventStop,
				StopReason: stopReason,
				Usage:      totalUsage,
			}
		}
	}
}

// handleHTTPError converts an HTTP error to a ProviderError.
func (p *GeminiProvider) handleHTTPError(err error) error {
	return &ProviderError{
		Code:      ErrCodeServerError,
		Message:   fmt.Sprintf("HTTP error: %v", err),
		Retryable: true,
	}
}

// parseErrorResponse parses an error response from the Gemini API.
func (p *GeminiProvider) parseErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var errResp geminiErrorResponse
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
		if strings.Contains(errResp.Error.Message, "context") || strings.Contains(errResp.Error.Message, "token") {
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
			"status":  errResp.Error.Status,
			"code":    errResp.Error.Code,
			"details": errResp.Error.Details,
		},
	}
}

// defaultGeminiModels returns the default list of supported Gemini models.
func defaultGeminiModels() []string {
	return []string{
		ModelGemini20FlashExp,
		ModelGemini20Flash,
		ModelGemini15Pro,
		ModelGemini15ProLatest,
		ModelGemini15Flash,
		ModelGemini15FlashLatest,
		ModelGemini15Flash8B,
		ModelGemini31ProPreview,
		ModelGemini31FlashPreview,
		ModelGeminiPro,
		ModelGeminiProVision,
	}
}

// defaultGeminiModelInfo returns model information for all supported models.
func defaultGeminiModelInfo() map[string]*ModelInfo {
	return map[string]*ModelInfo{
		ModelGemini20FlashExp: {
			ID:       ModelGemini20FlashExp,
			Name:     "Gemini 2.0 Flash Experimental",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.0,
			PricePerMOutputTokens: 0.0,
		},
		ModelGemini20Flash: {
			ID:       ModelGemini20Flash,
			Name:     "Gemini 2.0 Flash",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.10,
			PricePerMOutputTokens: 0.40,
		},
		ModelGemini15Pro: {
			ID:       ModelGemini15Pro,
			Name:     "Gemini 1.5 Pro",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 2097152,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  1.25,
			PricePerMOutputTokens: 5.00,
		},
		ModelGemini15ProLatest: {
			ID:       ModelGemini15ProLatest,
			Name:     "Gemini 1.5 Pro Latest",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 2097152,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  1.25,
			PricePerMOutputTokens: 5.00,
		},
		ModelGemini15Flash: {
			ID:       ModelGemini15Flash,
			Name:     "Gemini 1.5 Flash",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.075,
			PricePerMOutputTokens: 0.30,
		},
		ModelGemini15FlashLatest: {
			ID:       ModelGemini15FlashLatest,
			Name:     "Gemini 1.5 Flash Latest",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.075,
			PricePerMOutputTokens: 0.30,
		},
		ModelGemini15Flash8B: {
			ID:       ModelGemini15Flash8B,
			Name:     "Gemini 1.5 Flash 8B",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.0375,
			PricePerMOutputTokens: 0.15,
		},
		ModelGemini31ProPreview: {
			ID:       ModelGemini31ProPreview,
			Name:     "Gemini 3.1 Pro Preview",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 2097152,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  1.25,
			PricePerMOutputTokens: 3.75,
		},
		ModelGemini31FlashPreview: {
			ID:       ModelGemini31FlashPreview,
			Name:     "Gemini 3.1 Flash Preview",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 1048576,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.10,
			PricePerMOutputTokens: 0.40,
		},
		ModelGeminiPro: {
			ID:       ModelGeminiPro,
			Name:     "Gemini Pro",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           false,
				ToolUse:          true,
				SystemPrompt:     true,
				MultiTurn:        true,
				MaxContextTokens: 32760,
				MaxOutputTokens:  8192,
			},
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 1.50,
			Deprecated:            true,
		},
		ModelGeminiProVision: {
			ID:       ModelGeminiProVision,
			Name:     "Gemini Pro Vision",
			Provider: "gemini",
			Capabilities: ProviderCapabilities{
				Streaming:        true,
				Vision:           true,
				ToolUse:          false,
				SystemPrompt:     false,
				MultiTurn:        false,
				MaxContextTokens: 16384,
				MaxOutputTokens:  2048,
			},
			PricePerMInputTokens:  0.50,
			PricePerMOutputTokens: 1.50,
			Deprecated:            true,
		},
	}
}

// Gemini API request/response types

type geminiGenerateContentRequest struct {
	Contents          []geminiContent         `json:"contents"`
	SystemInstruction *geminiContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools             []geminiTool            `json:"tools,omitempty"`
	ToolConfig        *geminiToolConfig       `json:"toolConfig,omitempty"`
	SafetySettings    []geminiSafetySetting   `json:"safetySettings,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                  `json:"text,omitempty"`
	InlineData       *geminiBlob             `json:"inlineData,omitempty"`
	FunctionCall     *geminiFunctionCall     `json:"functionCall,omitempty"`
	FunctionResponse *geminiFunctionResponse `json:"functionResponse,omitempty"`
}

type geminiBlob struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiFunctionCall struct {
	Name string      `json:"name"`
	Args interface{} `json:"args,omitempty"`
}

type geminiFunctionResponse struct {
	Name     string      `json:"name"`
	Response interface{} `json:"response"`
}

type geminiGenerationConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	TopK             *int     `json:"topK,omitempty"`
	MaxOutputTokens  int      `json:"maxOutputTokens,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	CandidateCount   int      `json:"candidateCount,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
}

type geminiTool struct {
	FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

type geminiFunctionDeclaration struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"`
}

type geminiToolConfig struct {
	FunctionCallingConfig *geminiFunctionCallingConfig `json:"functionCallingConfig,omitempty"`
}

type geminiFunctionCallingConfig struct {
	Mode                 string   `json:"mode,omitempty"`
	AllowedFunctionNames []string `json:"allowedFunctionNames,omitempty"`
}

type geminiSafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

type geminiGenerateContentResponse struct {
	Candidates     []geminiCandidate     `json:"candidates,omitempty"`
	PromptFeedback *geminiPromptFeedback `json:"promptFeedback,omitempty"`
	UsageMetadata  *geminiUsageMetadata  `json:"usageMetadata,omitempty"`
}

type geminiCandidate struct {
	Content       *geminiContent       `json:"content,omitempty"`
	FinishReason  string               `json:"finishReason,omitempty"`
	SafetyRatings []geminiSafetyRating `json:"safetyRatings,omitempty"`
	Index         int                  `json:"index,omitempty"`
}

type geminiPromptFeedback struct {
	SafetyRatings []geminiSafetyRating `json:"safetyRatings,omitempty"`
	BlockReason   string               `json:"blockReason,omitempty"`
}

type geminiSafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount,omitempty"`
	CandidatesTokenCount int `json:"candidatesTokenCount,omitempty"`
	TotalTokenCount      int `json:"totalTokenCount,omitempty"`
}

type geminiErrorResponse struct {
	Error struct {
		Code    int           `json:"code"`
		Message string        `json:"message"`
		Status  string        `json:"status"`
		Details []interface{} `json:"details,omitempty"`
	} `json:"error"`
}

// GeminiToolTranslator implements ToolSchemaTranslator for Gemini.
// It translates tool definitions to Gemini's function declaration format
// and converts Gemini function call responses back to the unified format.
type GeminiToolTranslator struct {
	*BaseToolSchemaTranslator
}

// Compile-time verification that GeminiToolTranslator implements ToolSchemaTranslator.
var _ ToolSchemaTranslator = (*GeminiToolTranslator)(nil)

// NewGeminiToolTranslator creates a new Gemini tool translator.
func NewGeminiToolTranslator() *GeminiToolTranslator {
	return &GeminiToolTranslator{
		BaseToolSchemaTranslator: NewBaseToolSchemaTranslator("gemini"),
	}
}

// TranslateToProvider converts a unified ToolDefinition to Gemini format.
func (t *GeminiToolTranslator) TranslateToProvider(tool ToolDefinition) (interface{}, error) {
	if err := t.ValidateToolDefinition(tool); err != nil {
		return nil, err
	}

	var params interface{}
	if len(tool.InputSchema) > 0 {
		_ = json.Unmarshal(tool.InputSchema, &params)
	}

	return geminiFunctionDeclaration{
		Name:        tool.Name,
		Description: tool.Description,
		Parameters:  params,
	}, nil
}

// TranslateFromProvider converts a Gemini function call to unified format.
func (t *GeminiToolTranslator) TranslateFromProvider(providerToolCall interface{}) (*ContentBlock, error) {
	switch tc := providerToolCall.(type) {
	case geminiFunctionCall:
		var inputJSON json.RawMessage
		if tc.Args != nil {
			data, err := json.Marshal(tc.Args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal function args: %w", err)
			}
			inputJSON = data
		}
		return &ContentBlock{
			Type:      ContentTypeToolUse,
			ToolUseID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
			ToolName:  tc.Name,
			ToolInput: inputJSON,
		}, nil

	case *geminiFunctionCall:
		var inputJSON json.RawMessage
		if tc.Args != nil {
			data, err := json.Marshal(tc.Args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal function args: %w", err)
			}
			inputJSON = data
		}
		return &ContentBlock{
			Type:      ContentTypeToolUse,
			ToolUseID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
			ToolName:  tc.Name,
			ToolInput: inputJSON,
		}, nil

	case map[string]interface{}:
		name, _ := tc["name"].(string)
		if name == "" {
			return nil, fmt.Errorf("%w: missing function name", ErrInvalidToolCall)
		}

		var inputJSON json.RawMessage
		if args, ok := tc["args"]; ok {
			data, err := json.Marshal(args)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal function args: %w", err)
			}
			inputJSON = data
		}

		return &ContentBlock{
			Type:      ContentTypeToolUse,
			ToolUseID: fmt.Sprintf("call_%d", time.Now().UnixNano()),
			ToolName:  name,
			ToolInput: inputJSON,
		}, nil

	default:
		return nil, fmt.Errorf("%w: unsupported type %T", ErrInvalidToolCall, providerToolCall)
	}
}

// TranslateToolResult converts a unified tool result to Gemini format.
func (t *GeminiToolTranslator) TranslateToolResult(result ContentBlock) (interface{}, error) {
	if result.Type != ContentTypeToolResult {
		return nil, fmt.Errorf("expected tool_result content type, got %s", result.Type)
	}

	if result.ToolResultID == "" {
		return nil, ErrMissingToolID
	}

	response := map[string]interface{}{
		"result": result.ToolOutput,
	}
	if result.ToolError != "" {
		response["error"] = result.ToolError
	}

	return geminiFunctionResponse{
		Name:     result.ToolResultID,
		Response: response,
	}, nil
}

// TranslateToolChoice converts a unified tool choice to Gemini's format.
// Gemini uses FunctionCallingConfig with modes: AUTO, ANY, NONE.
// For specific tool names, it uses ANY with allowedFunctionNames.
func (t *GeminiToolTranslator) TranslateToolChoice(choice string) *geminiToolConfig {
	switch choice {
	case "auto", "":
		return &geminiToolConfig{
			FunctionCallingConfig: &geminiFunctionCallingConfig{
				Mode: "AUTO",
			},
		}
	case "any":
		// "any" means the model must use at least one tool
		return &geminiToolConfig{
			FunctionCallingConfig: &geminiFunctionCallingConfig{
				Mode: "ANY",
			},
		}
	case "none":
		return &geminiToolConfig{
			FunctionCallingConfig: &geminiFunctionCallingConfig{
				Mode: "NONE",
			},
		}
	default:
		// Specific tool name - use ANY with allowed function names
		return &geminiToolConfig{
			FunctionCallingConfig: &geminiFunctionCallingConfig{
				Mode:                 "ANY",
				AllowedFunctionNames: []string{choice},
			},
		}
	}
}

// TranslateMultipleTools converts multiple tool definitions to Gemini's format.
// Returns a slice of geminiFunctionDeclaration suitable for use in a geminiTool.
func (t *GeminiToolTranslator) TranslateMultipleTools(tools []ToolDefinition) ([]geminiFunctionDeclaration, error) {
	result := make([]geminiFunctionDeclaration, 0, len(tools))
	for _, tool := range tools {
		translated, err := t.TranslateToProvider(tool)
		if err != nil {
			return nil, fmt.Errorf("failed to translate tool %q: %w", tool.Name, err)
		}
		funcDecl, ok := translated.(geminiFunctionDeclaration)
		if !ok {
			return nil, fmt.Errorf("unexpected type %T for tool %q", translated, tool.Name)
		}
		result = append(result, funcDecl)
	}
	return result, nil
}

// TranslateMultipleToolCalls converts multiple Gemini function calls to unified format.
func (t *GeminiToolTranslator) TranslateMultipleToolCalls(funcCalls []geminiFunctionCall) ([]ContentBlock, error) {
	result := make([]ContentBlock, 0, len(funcCalls))
	for _, fc := range funcCalls {
		block, err := t.TranslateFromProvider(fc)
		if err != nil {
			return nil, err
		}
		result = append(result, *block)
	}
	return result, nil
}

// BuildGeminiTools creates a geminiTool struct containing all function declarations.
// This is the format expected by the Gemini API.
func (t *GeminiToolTranslator) BuildGeminiTools(tools []ToolDefinition) ([]geminiTool, error) {
	funcDecls, err := t.TranslateMultipleTools(tools)
	if err != nil {
		return nil, err
	}

	if len(funcDecls) == 0 {
		return nil, nil
	}

	return []geminiTool{
		{
			FunctionDeclarations: funcDecls,
		},
	}, nil
}

// GeminiSchemaConverter provides utilities for converting JSON Schema to Gemini's format.
// Gemini has specific requirements for schema definitions that differ slightly from
// standard JSON Schema.
type GeminiSchemaConverter struct{}

// NewGeminiSchemaConverter creates a new GeminiSchemaConverter.
func NewGeminiSchemaConverter() *GeminiSchemaConverter {
	return &GeminiSchemaConverter{}
}

// ConvertSchema converts a JSON Schema to Gemini's expected format.
// Gemini supports a subset of JSON Schema with some specific conventions.
func (c *GeminiSchemaConverter) ConvertSchema(schema json.RawMessage) (interface{}, error) {
	if len(schema) == 0 {
		return nil, nil
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(schema, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse JSON schema: %w", err)
	}

	// Recursively convert the schema
	return c.convertSchemaObject(parsed), nil
}

// convertSchemaObject recursively processes a schema object.
func (c *GeminiSchemaConverter) convertSchemaObject(schema map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy supported fields
	supportedFields := []string{
		"type", "description", "enum", "items", "properties", "required",
		"minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength",
		"pattern", "format", "nullable",
	}

	for _, field := range supportedFields {
		if v, ok := schema[field]; ok {
			switch field {
			case "items":
				// Handle array items - convert nested schema
				if itemsMap, ok := v.(map[string]interface{}); ok {
					result[field] = c.convertSchemaObject(itemsMap)
				} else {
					result[field] = v
				}
			case "properties":
				// Handle object properties - convert each nested schema
				if propsMap, ok := v.(map[string]interface{}); ok {
					convertedProps := make(map[string]interface{})
					for propName, propSchema := range propsMap {
						if propSchemaMap, ok := propSchema.(map[string]interface{}); ok {
							convertedProps[propName] = c.convertSchemaObject(propSchemaMap)
						} else {
							convertedProps[propName] = propSchema
						}
					}
					result[field] = convertedProps
				} else {
					result[field] = v
				}
			default:
				result[field] = v
			}
		}
	}

	return result
}
