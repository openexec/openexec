package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Compile-time verification that OpenAIProvider implements ProviderAdapter
var _ ProviderAdapter = (*OpenAIProvider)(nil)

func TestNewOpenAIProvider(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		provider, err := NewOpenAIProvider(OpenAIProviderConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider to be non-nil")
		}
		if provider.GetName() != "openai" {
			t.Errorf("expected name 'openai', got %q", provider.GetName())
		}
	})

	t.Run("without API key", func(t *testing.T) {
		// Temporarily unset environment variable
		t.Setenv("OPENAI_API_KEY", "")

		_, err := NewOpenAIProvider(OpenAIProviderConfig{})
		if err == nil {
			t.Fatal("expected error for missing API key")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeAuthentication {
			t.Errorf("expected code %q, got %q", ErrCodeAuthentication, providerErr.Code)
		}
	})

	t.Run("with environment variable", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "env-api-key")

		provider, err := NewOpenAIProvider(OpenAIProviderConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider to be non-nil")
		}
	})

	t.Run("with custom base URL", func(t *testing.T) {
		provider, err := NewOpenAIProvider(OpenAIProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: "https://custom.api.example.com/v1",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.config.BaseURL != "https://custom.api.example.com/v1" {
			t.Errorf("expected custom base URL, got %q", provider.config.BaseURL)
		}
	})

	t.Run("with custom timeout", func(t *testing.T) {
		provider, err := NewOpenAIProvider(OpenAIProviderConfig{
			APIKey:  "test-api-key",
			Timeout: 30 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.config.Timeout != 30*time.Second {
			t.Errorf("expected 30s timeout, got %v", provider.config.Timeout)
		}
	})
}

func TestNewOpenAIProviderFromEnv(t *testing.T) {
	t.Run("with environment variable set", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "env-test-key")

		provider, err := NewOpenAIProviderFromEnv()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider to be non-nil")
		}
		if provider.GetName() != "openai" {
			t.Errorf("expected name 'openai', got %q", provider.GetName())
		}
	})

	t.Run("without environment variable", func(t *testing.T) {
		t.Setenv("OPENAI_API_KEY", "")

		_, err := NewOpenAIProviderFromEnv()
		if err == nil {
			t.Fatal("expected error for missing API key")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeAuthentication {
			t.Errorf("expected code %q, got %q", ErrCodeAuthentication, providerErr.Code)
		}
	})
}

func TestOpenAIProvider_GetName(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	name := provider.GetName()
	if name != "openai" {
		t.Errorf("expected 'openai', got %q", name)
	}
}

func TestOpenAIProvider_GetModels(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	models := provider.GetModels()
	if len(models) == 0 {
		t.Error("expected at least one model")
	}

	// Check for expected models
	expectedModels := []string{ModelGPT4o, ModelGPT4oMini, ModelGPT4, ModelO1}
	for _, expected := range expectedModels {
		found := false
		for _, m := range models {
			if m == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected model %q not found", expected)
		}
	}
}

func TestOpenAIProvider_GetModelInfo(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	t.Run("existing model", func(t *testing.T) {
		info, err := provider.GetModelInfo(ModelGPT4o)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != ModelGPT4o {
			t.Errorf("expected ID %q, got %q", ModelGPT4o, info.ID)
		}
		if info.Provider != "openai" {
			t.Errorf("expected provider 'openai', got %q", info.Provider)
		}
		if !info.Capabilities.ToolUse {
			t.Error("expected gpt-4o to support tool use")
		}
		if !info.Capabilities.Vision {
			t.Error("expected gpt-4o to support vision")
		}
	})

	t.Run("non-existent model", func(t *testing.T) {
		_, err := provider.GetModelInfo("non-existent-model")
		if err == nil {
			t.Fatal("expected error for non-existent model")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeNotFound {
			t.Errorf("expected code %q, got %q", ErrCodeNotFound, providerErr.Code)
		}
	})

	t.Run("o1 model capabilities", func(t *testing.T) {
		info, err := provider.GetModelInfo(ModelO1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Capabilities.SystemPrompt {
			t.Error("expected o1 to NOT support system prompts")
		}
		if info.Capabilities.ToolUse {
			t.Error("expected o1 to NOT support tool use")
		}
	})

	t.Run("o3-mini model capabilities", func(t *testing.T) {
		info, err := provider.GetModelInfo(ModelO3Mini)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.Capabilities.SystemPrompt {
			t.Error("expected o3-mini to NOT support system prompts")
		}
		if !info.Capabilities.ToolUse {
			t.Error("expected o3-mini to support tool use")
		}
	})
}

func TestOpenAIProvider_GetCapabilities(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	caps, err := provider.GetCapabilities(ModelGPT4o)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !caps.Streaming {
		t.Error("expected streaming support")
	}
	if !caps.MultiTurn {
		t.Error("expected multi-turn support")
	}
	if caps.MaxContextTokens <= 0 {
		t.Error("expected positive max context tokens")
	}
}

func TestOpenAIProvider_ValidateRequest(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	t.Run("valid request", func(t *testing.T) {
		req := Request{
			Model:    ModelGPT4o,
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		}
		if err := provider.ValidateRequest(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		req := Request{
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Fatal("expected error for missing model")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok || providerErr.Code != ErrCodeInvalidRequest {
			t.Errorf("expected invalid_request error, got %v", err)
		}
	})

	t.Run("unsupported model", func(t *testing.T) {
		req := Request{
			Model:    "unsupported-model",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Fatal("expected error for unsupported model")
		}
	})

	t.Run("o1 with system prompt", func(t *testing.T) {
		req := Request{
			Model:    ModelO1,
			System:   "You are a helpful assistant.",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Fatal("expected error for o1 with system prompt")
		}
		if !strings.Contains(err.Error(), "does not support system prompts") {
			t.Errorf("expected system prompt error, got %v", err)
		}
	})

	t.Run("o1 with tools", func(t *testing.T) {
		req := Request{
			Model:    ModelO1,
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
			Tools: []ToolDefinition{
				{Name: "test_tool", Description: "A test tool"},
			},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Fatal("expected error for o1 with tools")
		}
		if !strings.Contains(err.Error(), "does not support tool use") {
			t.Errorf("expected tool use error, got %v", err)
		}
	})

	t.Run("o3-mini with tools is valid", func(t *testing.T) {
		req := Request{
			Model:    ModelO3Mini,
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
			Tools: []ToolDefinition{
				{Name: "test_tool", Description: "A test tool"},
			},
		}
		err := provider.ValidateRequest(req)
		if err != nil {
			t.Errorf("o3-mini should support tools: %v", err)
		}
	})
}

func TestOpenAIProvider_EstimateTokens(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	tests := []struct {
		content  string
		expected int
	}{
		{"", 0},
		{"test", 1},
		{"Hello, world!", 3}, // 13 chars / 4 = 3
		{"This is a longer piece of text for testing.", 10}, // 44 chars / 4 = 11
	}

	for _, tc := range tests {
		t.Run(tc.content, func(t *testing.T) {
			tokens := provider.EstimateTokens(tc.content)
			// Allow some variance since it's an estimation
			if tokens < tc.expected-1 || tokens > tc.expected+1 {
				t.Errorf("expected ~%d tokens for %q, got %d", tc.expected, tc.content, tokens)
			}
		})
	}
}

func TestOpenAIProvider_Complete(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing or incorrect Authorization header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing or incorrect Content-Type header")
		}

		// Parse request body
		body, _ := io.ReadAll(r.Body)
		var req openAIChatCompletionRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("failed to parse request: %v", err)
		}

		// Verify request structure
		if req.Model != ModelGPT4o {
			t.Errorf("expected model %q, got %q", ModelGPT4o, req.Model)
		}

		// Send response
		resp := openAIChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   ModelGPT4o,
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: openAIMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
					FinishReason: "stop",
				},
			},
			Usage: openAIUsage{
				PromptTokens:     10,
				CompletionTokens: 8,
				TotalTokens:      18,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected ID 'chatcmpl-123', got %q", resp.ID)
	}
	if resp.GetText() != "Hello! How can I help you?" {
		t.Errorf("unexpected response text: %q", resp.GetText())
	}
	if resp.StopReason != StopReasonEnd {
		t.Errorf("expected stop reason %q, got %q", StopReasonEnd, resp.StopReason)
	}
	if resp.Usage.TotalTokens != 18 {
		t.Errorf("expected 18 total tokens, got %d", resp.Usage.TotalTokens)
	}
}

func TestOpenAIProvider_CompleteWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req openAIChatCompletionRequest
		json.Unmarshal(body, &req)

		// Verify tools were sent
		if len(req.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Function.Name != "get_weather" {
			t.Errorf("expected tool name 'get_weather', got %q", req.Tools[0].Function.Name)
		}

		// Send tool call response
		resp := openAIChatCompletionResponse{
			ID:    "chatcmpl-456",
			Model: ModelGPT4o,
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Index: 0,
					Message: openAIMessage{
						Role: "assistant",
						ToolCalls: []openAIToolCall{
							{
								ID:   "call_abc123",
								Type: "function",
								Function: openAIFunctionCall{
									Name:      "get_weather",
									Arguments: `{"location": "San Francisco"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
			Usage: openAIUsage{TotalTokens: 50},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "What's the weather in San Francisco?")},
		Tools: []ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get the current weather",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}}}`),
			},
		},
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StopReason != StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", StopReasonToolUse, resp.StopReason)
	}

	toolCalls := resp.GetToolCalls()
	if len(toolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
	}
	if toolCalls[0].ToolName != "get_weather" {
		t.Errorf("expected tool name 'get_weather', got %q", toolCalls[0].ToolName)
	}
	if toolCalls[0].ToolUseID != "call_abc123" {
		t.Errorf("expected tool ID 'call_abc123', got %q", toolCalls[0].ToolUseID)
	}
}

func TestOpenAIProvider_CompleteWithError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		errorResponse  string
		expectedCode   string
		expectedRetry  bool
	}{
		{
			name:       "rate limit",
			statusCode: 429,
			errorResponse: `{
				"error": {
					"message": "Rate limit exceeded",
					"type": "rate_limit_error",
					"code": "rate_limit_exceeded"
				}
			}`,
			expectedCode:  ErrCodeRateLimit,
			expectedRetry: true,
		},
		{
			name:       "authentication error",
			statusCode: 401,
			errorResponse: `{
				"error": {
					"message": "Invalid API key",
					"type": "invalid_request_error",
					"code": "invalid_api_key"
				}
			}`,
			expectedCode:  ErrCodeAuthentication,
			expectedRetry: false,
		},
		{
			name:       "context length exceeded",
			statusCode: 400,
			errorResponse: `{
				"error": {
					"message": "This model's maximum context_length is 128000 tokens",
					"type": "invalid_request_error",
					"code": "context_length_exceeded"
				}
			}`,
			expectedCode:  ErrCodeContextLength,
			expectedRetry: false,
		},
		{
			name:       "server error",
			statusCode: 500,
			errorResponse: `{
				"error": {
					"message": "Internal server error",
					"type": "server_error"
				}
			}`,
			expectedCode:  ErrCodeServerError,
			expectedRetry: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.statusCode)
				w.Write([]byte(tc.errorResponse))
			}))
			defer server.Close()

			provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
				APIKey:  "test-key",
				BaseURL: server.URL,
			})

			req := Request{
				Model:    ModelGPT4o,
				Messages: []Message{NewTextMessage(RoleUser, "Hello")},
			}

			_, err := provider.Complete(context.Background(), req)
			if err == nil {
				t.Fatal("expected error")
			}

			providerErr, ok := err.(*ProviderError)
			if !ok {
				t.Fatalf("expected ProviderError, got %T", err)
			}
			if providerErr.Code != tc.expectedCode {
				t.Errorf("expected code %q, got %q", tc.expectedCode, providerErr.Code)
			}
			if providerErr.Retryable != tc.expectedRetry {
				t.Errorf("expected retryable=%v, got %v", tc.expectedRetry, providerErr.Retryable)
			}
		})
	}
}

func TestOpenAIProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req openAIChatCompletionRequest
		json.Unmarshal(body, &req)

		if !req.Stream {
			t.Error("expected stream to be true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected ResponseWriter to be a Flusher")
		}

		// Send stream chunks
		chunks := []string{
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4o","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	eventCh, err := provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []StreamEvent
	for event := range eventCh {
		events = append(events, event)
	}

	if len(events) < 3 {
		t.Fatalf("expected at least 3 events, got %d", len(events))
	}

	// Check for start event
	foundStart := false
	foundDelta := false
	foundStop := false

	for _, e := range events {
		switch e.Type {
		case StreamEventStart:
			foundStart = true
		case StreamEventContentDelta:
			foundDelta = true
		case StreamEventStop:
			foundStop = true
		}
	}

	if !foundStart {
		t.Error("expected start event")
	}
	if !foundDelta {
		t.Error("expected content delta event")
	}
	if !foundStop {
		t.Error("expected stop event")
	}
}

func TestOpenAIProvider_StreamWithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"location\":"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"NYC\"}"}}]},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Weather?")},
		Tools: []ToolDefinition{
			{Name: "get_weather", Description: "Get weather"},
		},
	}

	eventCh, err := provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundToolStart, foundToolDelta bool
	for event := range eventCh {
		if event.Type == StreamEventContentStart && event.ContentBlock != nil {
			if event.ContentBlock.Type == ContentTypeToolUse {
				foundToolStart = true
				if event.ContentBlock.ToolName != "get_weather" {
					t.Errorf("expected tool name 'get_weather', got %q", event.ContentBlock.ToolName)
				}
			}
		}
		if event.Type == StreamEventContentDelta && event.Delta != nil && event.Delta.ToolInput != "" {
			foundToolDelta = true
		}
	}

	if !foundToolStart {
		t.Error("expected tool call start event")
	}
	if !foundToolDelta {
		t.Error("expected tool input delta event")
	}
}

func TestOpenAIProvider_BuildOpenAIRequest(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	t.Run("basic request", func(t *testing.T) {
		req := Request{
			Model:    ModelGPT4o,
			System:   "You are helpful.",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		}

		openAIReq := provider.buildOpenAIRequest(req, false)

		if openAIReq.Model != ModelGPT4o {
			t.Errorf("expected model %q, got %q", ModelGPT4o, openAIReq.Model)
		}
		if len(openAIReq.Messages) != 2 {
			t.Errorf("expected 2 messages (system + user), got %d", len(openAIReq.Messages))
		}
		if openAIReq.Messages[0].Role != "system" {
			t.Errorf("expected first message role 'system', got %q", openAIReq.Messages[0].Role)
		}
	})

	t.Run("request with temperature", func(t *testing.T) {
		temp := 0.7
		req := Request{
			Model:       ModelGPT4o,
			Messages:    []Message{NewTextMessage(RoleUser, "Hello")},
			Temperature: &temp,
		}

		openAIReq := provider.buildOpenAIRequest(req, false)

		if openAIReq.Temperature == nil || *openAIReq.Temperature != 0.7 {
			t.Error("expected temperature 0.7")
		}
	})

	t.Run("request with tools and tool_choice", func(t *testing.T) {
		req := Request{
			Model:      ModelGPT4o,
			Messages:   []Message{NewTextMessage(RoleUser, "Hello")},
			ToolChoice: "any",
			Tools: []ToolDefinition{
				{Name: "test_tool", Description: "Test"},
			},
		}

		openAIReq := provider.buildOpenAIRequest(req, false)

		if len(openAIReq.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(openAIReq.Tools))
		}
		if openAIReq.ToolChoice != "required" {
			t.Errorf("expected tool_choice 'required' (mapped from 'any'), got %v", openAIReq.ToolChoice)
		}
	})

	t.Run("streaming request includes stream_options", func(t *testing.T) {
		req := Request{
			Model:    ModelGPT4o,
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		}

		openAIReq := provider.buildOpenAIRequest(req, true)

		if !openAIReq.Stream {
			t.Error("expected stream to be true")
		}
		if openAIReq.StreamOptions == nil {
			t.Fatal("expected stream_options to be set")
		}
		if !openAIReq.StreamOptions.IncludeUsage {
			t.Error("expected include_usage to be true")
		}
	})
}

func TestOpenAIProvider_ConvertToOpenAIMessage(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	t.Run("user message", func(t *testing.T) {
		msg := NewTextMessage(RoleUser, "Hello")
		openAIMsg := provider.convertToOpenAIMessage(msg)

		if openAIMsg.Role != "user" {
			t.Errorf("expected role 'user', got %q", openAIMsg.Role)
		}
		if openAIMsg.Content != "Hello" {
			t.Errorf("expected content 'Hello', got %q", openAIMsg.Content)
		}
	})

	t.Run("assistant message with tool calls", func(t *testing.T) {
		msg := Message{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: ContentTypeText, Text: "Let me check."},
				{
					Type:      ContentTypeToolUse,
					ToolUseID: "call_123",
					ToolName:  "get_data",
					ToolInput: json.RawMessage(`{"key": "value"}`),
				},
			},
		}

		openAIMsg := provider.convertToOpenAIMessage(msg)

		if openAIMsg.Role != "assistant" {
			t.Errorf("expected role 'assistant', got %q", openAIMsg.Role)
		}
		if len(openAIMsg.ToolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(openAIMsg.ToolCalls))
		}
		if openAIMsg.ToolCalls[0].ID != "call_123" {
			t.Errorf("expected tool call ID 'call_123', got %q", openAIMsg.ToolCalls[0].ID)
		}
	})

	t.Run("tool result message", func(t *testing.T) {
		msg := NewToolResultMessage("call_123", "result data", nil)
		openAIMsg := provider.convertToOpenAIMessage(msg)

		if openAIMsg.Role != "tool" {
			t.Errorf("expected role 'tool', got %q", openAIMsg.Role)
		}
		if openAIMsg.ToolCallID != "call_123" {
			t.Errorf("expected tool_call_id 'call_123', got %q", openAIMsg.ToolCallID)
		}
		if openAIMsg.Content != "result data" {
			t.Errorf("expected content 'result data', got %q", openAIMsg.Content)
		}
	})

	t.Run("tool error result", func(t *testing.T) {
		msg := NewToolResultMessage("call_456", "", fmt.Errorf("something failed"))
		openAIMsg := provider.convertToOpenAIMessage(msg)

		if !strings.Contains(openAIMsg.Content, "Error:") {
			t.Errorf("expected error prefix in content, got %q", openAIMsg.Content)
		}
	})
}

func TestOpenAIProvider_ConvertResponse(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	t.Run("text response", func(t *testing.T) {
		openAIResp := &openAIChatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: ModelGPT4o,
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Message:      openAIMessage{Role: "assistant", Content: "Hello!"},
					FinishReason: "stop",
				},
			},
			Usage: openAIUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}

		resp := provider.convertResponse(openAIResp)

		if resp.ID != "chatcmpl-123" {
			t.Errorf("expected ID 'chatcmpl-123', got %q", resp.ID)
		}
		if resp.StopReason != StopReasonEnd {
			t.Errorf("expected stop reason %q, got %q", StopReasonEnd, resp.StopReason)
		}
		if resp.GetText() != "Hello!" {
			t.Errorf("expected text 'Hello!', got %q", resp.GetText())
		}
		if resp.Usage.TotalTokens != 15 {
			t.Errorf("expected 15 total tokens, got %d", resp.Usage.TotalTokens)
		}
	})

	t.Run("tool call response", func(t *testing.T) {
		openAIResp := &openAIChatCompletionResponse{
			ID:    "chatcmpl-456",
			Model: ModelGPT4o,
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Message: openAIMessage{
						Role: "assistant",
						ToolCalls: []openAIToolCall{
							{
								ID:   "call_abc",
								Type: "function",
								Function: openAIFunctionCall{
									Name:      "search",
									Arguments: `{"query": "test"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}

		resp := provider.convertResponse(openAIResp)

		if resp.StopReason != StopReasonToolUse {
			t.Errorf("expected stop reason %q, got %q", StopReasonToolUse, resp.StopReason)
		}
		if !resp.HasToolCalls() {
			t.Error("expected response to have tool calls")
		}

		toolCalls := resp.GetToolCalls()
		if len(toolCalls) != 1 {
			t.Fatalf("expected 1 tool call, got %d", len(toolCalls))
		}
		if toolCalls[0].ToolName != "search" {
			t.Errorf("expected tool name 'search', got %q", toolCalls[0].ToolName)
		}
	})

	t.Run("max tokens response", func(t *testing.T) {
		openAIResp := &openAIChatCompletionResponse{
			ID:    "chatcmpl-789",
			Model: ModelGPT4o,
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Message:      openAIMessage{Content: "Truncated..."},
					FinishReason: "length",
				},
			},
		}

		resp := provider.convertResponse(openAIResp)

		if resp.StopReason != StopReasonMaxTokens {
			t.Errorf("expected stop reason %q, got %q", StopReasonMaxTokens, resp.StopReason)
		}
	})
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		model    string
		expected bool
	}{
		{ModelGPT4o, false},
		{ModelGPT4oMini, false},
		{ModelGPT4, false},
		{ModelGPT35Turbo, false},
		{ModelO1, true},
		{ModelO1Mini, true},
		{ModelO1Preview, true},
		{ModelO3Mini, true},
		{"o1-custom", true},
		{"o3-large", true},
	}

	for _, tc := range tests {
		t.Run(tc.model, func(t *testing.T) {
			result := isReasoningModel(tc.model)
			if result != tc.expected {
				t.Errorf("isReasoningModel(%q) = %v, expected %v", tc.model, result, tc.expected)
			}
		})
	}
}

func TestOpenAIProvider_Organization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify organization header
		org := r.Header.Get("OpenAI-Organization")
		if org != "org-123" {
			t.Errorf("expected organization 'org-123', got %q", org)
		}

		resp := openAIChatCompletionResponse{
			ID:    "chatcmpl-123",
			Model: ModelGPT4o,
			Choices: []struct {
				Index        int           `json:"index"`
				Message      openAIMessage `json:"message"`
				FinishReason string        `json:"finish_reason"`
			}{
				{
					Message:      openAIMessage{Role: "assistant", Content: "OK"},
					FinishReason: "stop",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:       "test-key",
		BaseURL:      server.URL,
		Organization: "org-123",
	})
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err = provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAIProvider_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
		Timeout: 10 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err := provider.Complete(ctx, req)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
}

func TestNewOpenAIProvider_WithCustomHTTPClient(t *testing.T) {
	customClient := &http.Client{
		Timeout: 60 * time.Second,
	}

	provider, err := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:     "test-key",
		HTTPClient: customClient,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The provider should use the custom HTTP client
	if provider.httpClient != customClient {
		t.Error("expected provider to use custom HTTP client")
	}
}

func TestNewOpenAIProvider_DefaultValues(t *testing.T) {
	provider, err := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.config.BaseURL != DefaultOpenAIBaseURL {
		t.Errorf("expected default base URL %q, got %q", DefaultOpenAIBaseURL, provider.config.BaseURL)
	}
	if provider.config.Timeout != DefaultOpenAITimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultOpenAITimeout, provider.config.Timeout)
	}
}

func TestOpenAIProvider_BuildOpenAIRequest_SpecificToolChoice(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	req := Request{
		Model:      ModelGPT4o,
		Messages:   []Message{NewTextMessage(RoleUser, "Hello")},
		ToolChoice: "specific_tool_name",
		Tools: []ToolDefinition{
			{Name: "specific_tool_name", Description: "A specific tool"},
		},
	}

	openAIReq := provider.buildOpenAIRequest(req, false)

	// Verify tool_choice is a map with the specific function name
	toolChoice, ok := openAIReq.ToolChoice.(map[string]interface{})
	if !ok {
		t.Fatalf("expected tool_choice to be a map, got %T", openAIReq.ToolChoice)
	}
	if toolChoice["type"] != "function" {
		t.Errorf("expected type 'function', got %v", toolChoice["type"])
	}
	funcMap, ok := toolChoice["function"].(map[string]string)
	if !ok {
		t.Fatalf("expected function to be map[string]string, got %T", toolChoice["function"])
	}
	if funcMap["name"] != "specific_tool_name" {
		t.Errorf("expected function name 'specific_tool_name', got %q", funcMap["name"])
	}
}

func TestOpenAIProvider_BuildOpenAIRequest_NoneToolChoice(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	req := Request{
		Model:      ModelGPT4o,
		Messages:   []Message{NewTextMessage(RoleUser, "Hello")},
		ToolChoice: "none",
		Tools: []ToolDefinition{
			{Name: "test_tool", Description: "Test"},
		},
	}

	openAIReq := provider.buildOpenAIRequest(req, false)

	if openAIReq.ToolChoice != "none" {
		t.Errorf("expected tool_choice 'none', got %v", openAIReq.ToolChoice)
	}
}

func TestOpenAIProvider_BuildOpenAIRequest_ReasoningModelRestrictions(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	temp := 0.7
	topP := 0.9
	req := Request{
		Model:         ModelO1,
		Messages:      []Message{NewTextMessage(RoleUser, "Hello")},
		Temperature:   &temp,
		TopP:          &topP,
		StopSequences: []string{"STOP"},
		MaxTokens:     100,
	}

	openAIReq := provider.buildOpenAIRequest(req, false)

	// Reasoning models should not have temperature, top_p, or stop sequences
	if openAIReq.Temperature != nil {
		t.Error("expected temperature to be nil for reasoning model")
	}
	if openAIReq.TopP != nil {
		t.Error("expected top_p to be nil for reasoning model")
	}
	if len(openAIReq.Stop) > 0 {
		t.Error("expected stop to be empty for reasoning model")
	}
	// But max_tokens should still be set
	if openAIReq.MaxCompletionTokens != 100 {
		t.Errorf("expected max_completion_tokens 100, got %d", openAIReq.MaxCompletionTokens)
	}
}

func TestOpenAIProvider_ConvertResponse_ContentFilterFinishReason(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	openAIResp := &openAIChatCompletionResponse{
		ID:    "chatcmpl-123",
		Model: ModelGPT4o,
		Choices: []struct {
			Index        int           `json:"index"`
			Message      openAIMessage `json:"message"`
			FinishReason string        `json:"finish_reason"`
		}{
			{
				Message:      openAIMessage{Role: "assistant", Content: "Content filtered"},
				FinishReason: "content_filter",
			},
		},
	}

	resp := provider.convertResponse(openAIResp)

	if resp.StopReason != StopReasonStop {
		t.Errorf("expected stop reason %q for content_filter, got %q", StopReasonStop, resp.StopReason)
	}
}

func TestOpenAIProvider_ConvertResponse_EmptyChoices(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	openAIResp := &openAIChatCompletionResponse{
		ID:      "chatcmpl-123",
		Model:   ModelGPT4o,
		Choices: []struct {
			Index        int           `json:"index"`
			Message      openAIMessage `json:"message"`
			FinishReason string        `json:"finish_reason"`
		}{},
		Usage: openAIUsage{TotalTokens: 10},
	}

	resp := provider.convertResponse(openAIResp)

	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected ID 'chatcmpl-123', got %q", resp.ID)
	}
	if len(resp.Content) != 0 {
		t.Errorf("expected empty content, got %d blocks", len(resp.Content))
	}
}

func TestOpenAIProvider_ConvertToOpenAIMessage_LegacyTextField(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	// Message with only legacy Text field (no Content blocks)
	msg := Message{
		Role: RoleUser,
		Text: "Legacy text message",
	}

	openAIMsg := provider.convertToOpenAIMessage(msg)

	if openAIMsg.Content != "Legacy text message" {
		t.Errorf("expected content 'Legacy text message', got %q", openAIMsg.Content)
	}
}

func TestOpenAIProvider_ParseErrorResponse_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err := provider.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	// Should fall back to including raw body in error message
	if !strings.Contains(providerErr.Message, "not valid json") {
		t.Errorf("expected error message to contain raw body, got %q", providerErr.Message)
	}
}

func TestOpenAIProvider_ParseErrorResponse_NotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":{"message":"Model not found","type":"invalid_request_error"}}`))
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err := provider.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Code != ErrCodeNotFound {
		t.Errorf("expected code %q, got %q", ErrCodeNotFound, providerErr.Code)
	}
}

func TestOpenAIProvider_ParseErrorResponse_ForbiddenError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"message":"Access denied","type":"permission_error"}}`))
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err := provider.Complete(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Code != ErrCodePermission {
		t.Errorf("expected code %q, got %q", ErrCodePermission, providerErr.Code)
	}
}

func TestOpenAIProvider_StreamWithUsageInFinalChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-123","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			// Final chunk with usage only (no choices)
			`{"id":"chatcmpl-123","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		}

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	eventCh, err := provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundUsage bool
	for event := range eventCh {
		if event.Type == StreamEventStop && event.Usage != nil {
			foundUsage = true
			if event.Usage.TotalTokens != 15 {
				t.Errorf("expected 15 total tokens, got %d", event.Usage.TotalTokens)
			}
		}
	}

	if !foundUsage {
		t.Error("expected to find usage in final chunk")
	}
}

func TestOpenAIProvider_StreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		flusher, _ := w.(http.Flusher)

		// Send first chunk
		fmt.Fprintf(w, "data: %s\n\n", `{"id":"chatcmpl-123","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`)
		flusher.Flush()

		// Simulate delay to allow context cancellation
		time.Sleep(2 * time.Second)

		// This should not be reached if context is cancelled
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	eventCh, err := provider.Stream(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var foundError bool
	for event := range eventCh {
		if event.Type == StreamEventError && event.Error != nil {
			foundError = true
		}
	}

	if !foundError {
		t.Error("expected error event due to context cancellation")
	}
}

func TestOpenAIProvider_StreamErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"message":"Rate limit exceeded","type":"rate_limit_error"}}`))
	}))
	defer server.Close()

	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model:    ModelGPT4o,
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err := provider.Stream(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Code != ErrCodeRateLimit {
		t.Errorf("expected code %q, got %q", ErrCodeRateLimit, providerErr.Code)
	}
}

func TestOpenAIProvider_StreamValidationError(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	// Request without model should fail validation
	req := Request{
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	}

	_, err := provider.Stream(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Code != ErrCodeInvalidRequest {
		t.Errorf("expected code %q, got %q", ErrCodeInvalidRequest, providerErr.Code)
	}
}

func TestOpenAIProvider_GetCapabilities_Error(t *testing.T) {
	provider, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "test-key"})

	_, err := provider.GetCapabilities("non-existent-model")
	if err == nil {
		t.Fatal("expected error for non-existent model")
	}

	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Code != ErrCodeNotFound {
		t.Errorf("expected code %q, got %q", ErrCodeNotFound, providerErr.Code)
	}
}
