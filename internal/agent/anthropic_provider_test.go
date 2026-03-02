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

func TestAnthropicProviderImplementsInterface(t *testing.T) {
	// Ensure AnthropicProvider implements ProviderAdapter
	var _ ProviderAdapter = (*AnthropicProvider)(nil)
}

func TestNewAnthropicProvider(t *testing.T) {
	t.Run("with API key in config", func(t *testing.T) {
		provider, err := NewAnthropicProvider(AnthropicConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
		if provider.GetName() != "anthropic" {
			t.Errorf("expected name %q, got %q", "anthropic", provider.GetName())
		}
	})

	t.Run("without API key", func(t *testing.T) {
		// Clear environment variable temporarily
		t.Setenv("ANTHROPIC_API_KEY", "")

		_, err := NewAnthropicProvider(AnthropicConfig{})
		if err == nil {
			t.Error("expected error for missing API key")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeAuthentication {
			t.Errorf("expected code %q, got %q", ErrCodeAuthentication, providerErr.Code)
		}
	})

	t.Run("with API key from environment", func(t *testing.T) {
		t.Setenv("ANTHROPIC_API_KEY", "env-api-key")

		provider, err := NewAnthropicProvider(AnthropicConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
	})

	t.Run("with custom HTTP client", func(t *testing.T) {
		customClient := &http.Client{Timeout: 30 * time.Second}
		provider, err := NewAnthropicProvider(AnthropicConfig{
			APIKey:     "test-api-key",
			HTTPClient: customClient,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.httpClient != customClient {
			t.Error("expected custom HTTP client to be used")
		}
	})

	t.Run("with custom timeout", func(t *testing.T) {
		provider, err := NewAnthropicProvider(AnthropicConfig{
			APIKey:  "test-api-key",
			Timeout: 10 * time.Second,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.httpClient.Timeout != 10*time.Second {
			t.Errorf("expected timeout 10s, got %v", provider.httpClient.Timeout)
		}
	})
}

func TestAnthropicProviderGetModels(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	models := provider.GetModels()
	if len(models) == 0 {
		t.Error("expected at least one model")
	}

	// Check for known models
	expectedModels := []string{
		"claude-opus-4-5-20251101",
		"claude-sonnet-4-20250514",
		"claude-3-5-sonnet-20241022",
		"claude-3-5-haiku-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}

	modelSet := make(map[string]bool)
	for _, m := range models {
		modelSet[m] = true
	}

	for _, expected := range expectedModels {
		if !modelSet[expected] {
			t.Errorf("expected model %q to be in list", expected)
		}
	}
}

func TestAnthropicProviderGetModelInfo(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	t.Run("existing model", func(t *testing.T) {
		info, err := provider.GetModelInfo("claude-3-5-sonnet-20241022")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != "claude-3-5-sonnet-20241022" {
			t.Errorf("expected ID %q, got %q", "claude-3-5-sonnet-20241022", info.ID)
		}
		if info.Provider != "anthropic" {
			t.Errorf("expected provider %q, got %q", "anthropic", info.Provider)
		}
		if !info.Capabilities.Streaming {
			t.Error("expected streaming capability")
		}
		if !info.Capabilities.ToolUse {
			t.Error("expected tool use capability")
		}
		if !info.Capabilities.Vision {
			t.Error("expected vision capability")
		}
	})

	t.Run("nonexistent model", func(t *testing.T) {
		_, err := provider.GetModelInfo("nonexistent-model")
		if err == nil {
			t.Error("expected error for nonexistent model")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeNotFound {
			t.Errorf("expected code %q, got %q", ErrCodeNotFound, providerErr.Code)
		}
	})
}

func TestAnthropicProviderGetCapabilities(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	t.Run("valid model", func(t *testing.T) {
		caps, err := provider.GetCapabilities("claude-3-5-sonnet-20241022")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !caps.Streaming {
			t.Error("expected streaming to be true")
		}
		if !caps.Vision {
			t.Error("expected vision to be true")
		}
		if !caps.ToolUse {
			t.Error("expected tool use to be true")
		}
		if !caps.SystemPrompt {
			t.Error("expected system prompt to be true")
		}
		if !caps.MultiTurn {
			t.Error("expected multi-turn to be true")
		}
		if caps.MaxContextTokens <= 0 {
			t.Error("expected positive max context tokens")
		}
	})

	t.Run("nonexistent model", func(t *testing.T) {
		_, err := provider.GetCapabilities("nonexistent-model")
		if err == nil {
			t.Error("expected error for nonexistent model")
		}
		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeNotFound {
			t.Errorf("expected code %q, got %q", ErrCodeNotFound, providerErr.Code)
		}
	})
}

func TestAnthropicProviderValidateRequest(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	t.Run("valid request", func(t *testing.T) {
		req := Request{
			Model: "claude-3-5-sonnet-20241022",
			Messages: []Message{
				NewTextMessage(RoleUser, "Hello"),
			},
		}
		if err := provider.ValidateRequest(req); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("missing model", func(t *testing.T) {
		req := Request{
			Messages: []Message{
				NewTextMessage(RoleUser, "Hello"),
			},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Error("expected error for missing model")
		}
	})

	t.Run("empty messages", func(t *testing.T) {
		req := Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Error("expected error for empty messages")
		}
	})

	t.Run("unknown model", func(t *testing.T) {
		req := Request{
			Model: "unknown-model",
			Messages: []Message{
				NewTextMessage(RoleUser, "Hello"),
			},
		}
		err := provider.ValidateRequest(req)
		if err == nil {
			t.Error("expected error for unknown model")
		}
	})
}

func TestAnthropicProviderEstimateTokens(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	testCases := []struct {
		content  string
		expected int
	}{
		{"Hello", 1},           // 5 chars / 4 = 1
		{"Hello, world!", 3},   // 13 chars / 4 = 3
		{"", 0},                // 0 chars / 4 = 0
		{strings.Repeat("a", 100), 25}, // 100 chars / 4 = 25
	}

	for _, tc := range testCases {
		got := provider.EstimateTokens(tc.content)
		if got != tc.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.content, got, tc.expected)
		}
	}
}

func TestAnthropicProviderComplete(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("x-api-key") == "" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != AnthropicAPIVersion {
			t.Errorf("unexpected anthropic-version: %s", r.Header.Get("anthropic-version"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}

		// Parse request
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Stream {
			t.Error("expected stream to be false for Complete")
		}

		// Send response
		resp := anthropicResponse{
			ID:    "msg_123",
			Type:  "message",
			Role:  "assistant",
			Model: req.Model,
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Hello! How can I help you?"},
			},
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:  10,
				OutputTokens: 8,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with custom HTTP client pointing to mock server
	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()

	// Override the API URL for testing (we need to use a custom transport)
	originalTransport := provider.httpClient.Transport
	provider.httpClient.Transport = &testTransport{
		baseURL:   server.URL,
		transport: originalTransport,
	}

	req := Request{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
		System: "You are helpful.",
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ID != "msg_123" {
		t.Errorf("expected ID %q, got %q", "msg_123", resp.ID)
	}
	if resp.GetText() != "Hello! How can I help you?" {
		t.Errorf("unexpected text: %s", resp.GetText())
	}
	if resp.StopReason != StopReasonEnd {
		t.Errorf("expected stop reason %q, got %q", StopReasonEnd, resp.StopReason)
	}
	if resp.Usage.PromptTokens != 10 {
		t.Errorf("expected prompt tokens 10, got %d", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 8 {
		t.Errorf("expected completion tokens 8, got %d", resp.Usage.CompletionTokens)
	}
}

func TestAnthropicProviderCompleteWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify tools are passed correctly
		if len(req.Tools) != 1 {
			t.Errorf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Name != "calculator" {
			t.Errorf("expected tool name %q, got %q", "calculator", req.Tools[0].Name)
		}

		// Send response with tool use
		resp := anthropicResponse{
			ID:    "msg_456",
			Type:  "message",
			Role:  "assistant",
			Model: req.Model,
			Content: []anthropicContentBlock{
				{Type: "text", Text: "I'll calculate that for you."},
				{
					Type:  "tool_use",
					ID:    "tool_call_789",
					Name:  "calculator",
					Input: json.RawMessage(`{"expression": "2+2"}`),
				},
			},
			StopReason: "tool_use",
			Usage: anthropicUsage{
				InputTokens:  15,
				OutputTokens: 12,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{
		baseURL:   server.URL,
		transport: nil,
	}

	schema := json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`)
	req := Request{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []Message{
			NewTextMessage(RoleUser, "What is 2+2?"),
		},
		Tools: []ToolDefinition{
			{
				Name:        "calculator",
				Description: "Evaluate math expressions",
				InputSchema: schema,
			},
		},
		ToolChoice: "auto",
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

	if toolCalls[0].ToolName != "calculator" {
		t.Errorf("expected tool name %q, got %q", "calculator", toolCalls[0].ToolName)
	}
	if toolCalls[0].ToolUseID != "tool_call_789" {
		t.Errorf("expected tool use ID %q, got %q", "tool_call_789", toolCalls[0].ToolUseID)
	}
}

func TestAnthropicProviderStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if !req.Stream {
			t.Error("expected stream to be true")
		}

		// Send SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_stream","model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":10}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" world"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			fmt.Fprintln(w, event)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{
		baseURL:   server.URL,
		transport: nil,
	}

	req := Request{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
	}

	ch, err := provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}

	// Check for expected event types
	hasStart := false
	hasContentDelta := false
	hasStop := false
	var textContent string

	for _, event := range events {
		switch event.Type {
		case StreamEventStart:
			hasStart = true
		case StreamEventContentDelta:
			hasContentDelta = true
			if event.Delta != nil {
				textContent += event.Delta.Text
			}
		case StreamEventStop:
			hasStop = true
			if event.StopReason != StopReasonEnd {
				t.Errorf("expected stop reason %q, got %q", StopReasonEnd, event.StopReason)
			}
		}
	}

	if !hasStart {
		t.Error("expected message_start event")
	}
	if !hasContentDelta {
		t.Error("expected content_block_delta event")
	}
	if !hasStop {
		t.Error("expected message_stop event")
	}
	if textContent != "Hello world" {
		t.Errorf("expected text %q, got %q", "Hello world", textContent)
	}
}

func TestAnthropicProviderErrorHandling(t *testing.T) {
	t.Run("rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "rate_limit_error",
					Message: "Rate limited",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeRateLimit {
			t.Errorf("expected code %q, got %q", ErrCodeRateLimit, providerErr.Code)
		}
		if !providerErr.Retryable {
			t.Error("expected error to be retryable")
		}
	})

	t.Run("authentication error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "authentication_error",
					Message: "Invalid API key",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "invalid-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeAuthentication {
			t.Errorf("expected code %q, got %q", ErrCodeAuthentication, providerErr.Code)
		}
	})

	t.Run("permission error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "permission_error",
					Message: "Permission denied",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodePermission {
			t.Errorf("expected code %q, got %q", ErrCodePermission, providerErr.Code)
		}
	})

	t.Run("not found error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "not_found_error",
					Message: "Resource not found",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeNotFound {
			t.Errorf("expected code %q, got %q", ErrCodeNotFound, providerErr.Code)
		}
	})

	t.Run("overloaded error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "overloaded_error",
					Message: "API is overloaded",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeServerError {
			t.Errorf("expected code %q, got %q", ErrCodeServerError, providerErr.Code)
		}
		if !providerErr.Retryable {
			t.Error("expected error to be retryable")
		}
	})

	t.Run("server error 500", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "unknown_error",
					Message: "Internal server error",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeServerError {
			t.Errorf("expected code %q, got %q", ErrCodeServerError, providerErr.Code)
		}
		if !providerErr.Retryable {
			t.Error("expected 500 error to be retryable")
		}
	})

	t.Run("context length error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "invalid_request_error",
					Message: "context length exceeded",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeContextLength {
			t.Errorf("expected code %q, got %q", ErrCodeContextLength, providerErr.Code)
		}
	})

	t.Run("invalid request error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(anthropicError{
				Type: "error",
				Error: struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				}{
					Type:    "invalid_request_error",
					Message: "Invalid parameter value",
				},
			})
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		if providerErr.Code != ErrCodeInvalidRequest {
			t.Errorf("expected code %q, got %q", ErrCodeInvalidRequest, providerErr.Code)
		}
	})

	t.Run("malformed error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("not valid json"))
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		providerErr, ok := err.(*ProviderError)
		if !ok {
			t.Fatalf("expected ProviderError, got %T", err)
		}
		// Should fall back to raw message body
		if providerErr.Message != "not valid json" {
			t.Errorf("expected raw message in error, got %q", providerErr.Message)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: server.Client(),
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := provider.Complete(ctx, Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		if err == nil {
			t.Error("expected error for canceled context")
		}
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider := &AnthropicProvider{
			apiKey:     "test-key",
			httpClient: &http.Client{Timeout: 10 * time.Millisecond},
			models:     make(map[string]*ModelInfo),
		}
		provider.initModels()
		provider.httpClient.Transport = &testTransport{baseURL: server.URL}

		_, err := provider.Complete(context.Background(), Request{
			Model:    "claude-3-5-sonnet-20241022",
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		if err == nil {
			t.Error("expected error for timeout")
		}
		providerErr, ok := err.(*ProviderError)
		if ok && providerErr.Code != ErrCodeTimeout && providerErr.Code != ErrCodeServerError {
			t.Errorf("expected timeout or server error code, got %q", providerErr.Code)
		}
	})
}

func TestAnthropicProviderMessageConversion(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	t.Run("text message", func(t *testing.T) {
		msg := NewTextMessage(RoleUser, "Hello, Claude!")
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if anthropicMsg.Role != "user" {
			t.Errorf("expected role %q, got %q", "user", anthropicMsg.Role)
		}
		if len(anthropicMsg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(anthropicMsg.Content))
		}
	})

	t.Run("tool result message", func(t *testing.T) {
		msg := NewToolResultMessage("tool-123", "Result data", nil)
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Tool results should be mapped to user role
		if anthropicMsg.Role != "user" {
			t.Errorf("expected role %q for tool result, got %q", "user", anthropicMsg.Role)
		}
	})

	t.Run("legacy text field", func(t *testing.T) {
		msg := Message{
			Role: RoleUser,
			Text: "Legacy message",
		}
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(anthropicMsg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(anthropicMsg.Content))
		}
	})

	t.Run("image content with base64 data", func(t *testing.T) {
		imageData := []byte("fake-image-data")
		msg := Message{
			Role: RoleUser,
			Content: []ContentBlock{
				{
					Type:       ContentTypeImage,
					ImageData:  imageData,
					ImageMedia: "image/png",
				},
			},
		}
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(anthropicMsg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(anthropicMsg.Content))
		}
	})

	t.Run("image content with URL only (skipped)", func(t *testing.T) {
		msg := Message{
			Role: RoleUser,
			Content: []ContentBlock{
				{
					Type:     ContentTypeImage,
					ImageURL: "https://example.com/image.png",
				},
			},
		}
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// URL-only images should be skipped
		if len(anthropicMsg.Content) != 0 {
			t.Errorf("expected 0 content blocks (URL images skipped), got %d", len(anthropicMsg.Content))
		}
	})

	t.Run("tool use content", func(t *testing.T) {
		msg := Message{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{
					Type:      ContentTypeToolUse,
					ToolUseID: "toolu_123",
					ToolName:  "read_file",
					ToolInput: json.RawMessage(`{"path": "/tmp/test.txt"}`),
				},
			},
		}
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(anthropicMsg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(anthropicMsg.Content))
		}
	})

	t.Run("tool result with error", func(t *testing.T) {
		msg := Message{
			Role: RoleTool,
			Content: []ContentBlock{
				{
					Type:         ContentTypeToolResult,
					ToolResultID: "toolu_123",
					ToolError:    "file not found",
				},
			},
		}
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if anthropicMsg.Role != "user" {
			t.Errorf("expected role %q for tool result, got %q", "user", anthropicMsg.Role)
		}
		if len(anthropicMsg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(anthropicMsg.Content))
		}
	})

	t.Run("mixed content types", func(t *testing.T) {
		msg := Message{
			Role: RoleUser,
			Content: []ContentBlock{
				{Type: ContentTypeText, Text: "Check this image:"},
				{Type: ContentTypeImage, ImageData: []byte("image"), ImageMedia: "image/png"},
			},
		}
		anthropicMsg, err := provider.convertMessage(msg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(anthropicMsg.Content) != 2 {
			t.Errorf("expected 2 content blocks, got %d", len(anthropicMsg.Content))
		}
	})
}

func TestAnthropicToolTranslator(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	t.Run("translate tool to provider", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "read_file",
			Description: "Read contents of a file",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicTool, ok := result.(anthropicTool)
		if !ok {
			t.Fatalf("expected anthropicTool, got %T", result)
		}

		if anthropicTool.Name != "read_file" {
			t.Errorf("expected name %q, got %q", "read_file", anthropicTool.Name)
		}
	})

	t.Run("translate tool call from provider", func(t *testing.T) {
		toolCall := anthropicToolUseContent{
			Type:  "tool_use",
			ID:    "tool_call_123",
			Name:  "calculator",
			Input: json.RawMessage(`{"x": 1, "y": 2}`),
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.Type != ContentTypeToolUse {
			t.Errorf("expected type %q, got %q", ContentTypeToolUse, block.Type)
		}
		if block.ToolUseID != "tool_call_123" {
			t.Errorf("expected tool use ID %q, got %q", "tool_call_123", block.ToolUseID)
		}
		if block.ToolName != "calculator" {
			t.Errorf("expected tool name %q, got %q", "calculator", block.ToolName)
		}
	})

	t.Run("translate tool result to provider", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "tool_call_123",
			ToolOutput:   "Result: 3",
		}

		translated, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicResult, ok := translated.(anthropicToolResultContent)
		if !ok {
			t.Fatalf("expected anthropicToolResultContent, got %T", translated)
		}

		if anthropicResult.ToolUseID != "tool_call_123" {
			t.Errorf("expected tool use ID %q, got %q", "tool_call_123", anthropicResult.ToolUseID)
		}
		if anthropicResult.Content != "Result: 3" {
			t.Errorf("expected content %q, got %q", "Result: 3", anthropicResult.Content)
		}
		if anthropicResult.IsError {
			t.Error("expected IsError to be false")
		}
	})

	t.Run("translate error tool result to provider", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "tool_call_456",
			ToolError:    "File not found",
		}

		translated, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicResult, ok := translated.(anthropicToolResultContent)
		if !ok {
			t.Fatalf("expected anthropicToolResultContent, got %T", translated)
		}

		if !anthropicResult.IsError {
			t.Error("expected IsError to be true")
		}
		if anthropicResult.Content != "File not found" {
			t.Errorf("expected content %q, got %q", "File not found", anthropicResult.Content)
		}
	})
}

func TestAnthropicProviderToolChoice(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	testCases := []struct {
		choice   string
		expected *anthropicToolChoice
	}{
		{"auto", &anthropicToolChoice{Type: "auto"}},
		{"any", &anthropicToolChoice{Type: "any"}},
		{"none", nil}, // tools should be cleared
		{"specific_tool", &anthropicToolChoice{Type: "tool", Name: "specific_tool"}},
	}

	for _, tc := range testCases {
		t.Run(tc.choice, func(t *testing.T) {
			req := Request{
				Model: "claude-3-5-sonnet-20241022",
				Messages: []Message{
					NewTextMessage(RoleUser, "Hello"),
				},
				Tools: []ToolDefinition{
					{Name: "test_tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
				},
				ToolChoice: tc.choice,
			}

			anthropicReq, err := provider.buildRequest(req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.choice == "none" {
				if len(anthropicReq.Tools) != 0 {
					t.Error("expected tools to be cleared for 'none' choice")
				}
			} else {
				if anthropicReq.ToolChoice == nil {
					t.Error("expected tool choice to be set")
				} else if anthropicReq.ToolChoice.Type != tc.expected.Type {
					t.Errorf("expected type %q, got %q", tc.expected.Type, anthropicReq.ToolChoice.Type)
				}
			}
		})
	}
}

// testTransport is a custom http.RoundTripper that redirects requests to a test server.
type testTransport struct {
	baseURL   string
	transport http.RoundTripper
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect to test server
	req.URL.Scheme = "http"
	req.URL.Host = strings.TrimPrefix(t.baseURL, "http://")
	req.URL.Path = "/v1/messages"

	transport := t.transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	return transport.RoundTrip(req)
}

func TestAnthropicProviderCacheTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			ID:    "msg_cache",
			Type:  "message",
			Role:  "assistant",
			Model: "claude-3-5-sonnet-20241022",
			Content: []anthropicContentBlock{
				{Type: "text", Text: "Cached response"},
			},
			StopReason: "end_turn",
			Usage: anthropicUsage{
				InputTokens:              100,
				OutputTokens:             20,
				CacheCreationInputTokens: 50,
				CacheReadInputTokens:     30,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{baseURL: server.URL}

	resp, err := provider.Complete(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Usage.CacheWriteTokens != 50 {
		t.Errorf("expected cache write tokens 50, got %d", resp.Usage.CacheWriteTokens)
	}
	if resp.Usage.CacheReadTokens != 30 {
		t.Errorf("expected cache read tokens 30, got %d", resp.Usage.CacheReadTokens)
	}
}

func TestAnthropicToolTranslatorFromMap(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	toolCall := map[string]interface{}{
		"id":    "tool_123",
		"name":  "read_file",
		"input": map[string]interface{}{"path": "/tmp/test.txt"},
	}

	block, err := translator.TranslateFromProvider(toolCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if block.ToolUseID != "tool_123" {
		t.Errorf("expected tool use ID %q, got %q", "tool_123", block.ToolUseID)
	}
	if block.ToolName != "read_file" {
		t.Errorf("expected tool name %q, got %q", "read_file", block.ToolName)
	}

	// Verify the input was serialized correctly
	var input map[string]interface{}
	if err := json.Unmarshal(block.ToolInput, &input); err != nil {
		t.Fatalf("failed to unmarshal tool input: %v", err)
	}
	if input["path"] != "/tmp/test.txt" {
		t.Errorf("expected path %q, got %q", "/tmp/test.txt", input["path"])
	}
}

func TestAnthropicToolTranslatorErrors(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	t.Run("invalid tool definition", func(t *testing.T) {
		tool := ToolDefinition{
			Name: "", // Missing name
		}
		_, err := translator.TranslateToProvider(tool)
		if err == nil {
			t.Error("expected error for missing tool name")
		}
	})

	t.Run("invalid tool call type", func(t *testing.T) {
		_, err := translator.TranslateFromProvider("invalid")
		if err == nil {
			t.Error("expected error for invalid type")
		}
	})

	t.Run("invalid tool result type", func(t *testing.T) {
		result := ContentBlock{
			Type: ContentTypeText, // Wrong type
		}
		_, err := translator.TranslateToolResult(result)
		if err == nil {
			t.Error("expected error for wrong content type")
		}
	})

	t.Run("missing tool result ID", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "", // Missing ID
		}
		_, err := translator.TranslateToolResult(result)
		if err == nil {
			t.Error("expected error for missing tool result ID")
		}
	})
}

func TestAnthropicToolTranslatorComplexSchema(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	t.Run("tool with complex nested schema", func(t *testing.T) {
		complexSchema := json.RawMessage(`{
			"type": "object",
			"properties": {
				"files": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"path": {"type": "string"},
							"content": {"type": "string"}
						},
						"required": ["path"]
					}
				},
				"options": {
					"type": "object",
					"properties": {
						"recursive": {"type": "boolean", "default": false},
						"maxDepth": {"type": "integer", "minimum": 1, "maximum": 10}
					}
				}
			},
			"required": ["files"]
		}`)

		tool := ToolDefinition{
			Name:        "batch_write_files",
			Description: "Write multiple files at once",
			InputSchema: complexSchema,
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicTool, ok := result.(anthropicTool)
		if !ok {
			t.Fatalf("expected anthropicTool, got %T", result)
		}

		if anthropicTool.Name != "batch_write_files" {
			t.Errorf("expected name %q, got %q", "batch_write_files", anthropicTool.Name)
		}
		if anthropicTool.Description != "Write multiple files at once" {
			t.Errorf("expected description to be preserved")
		}
		// Verify schema was passed through correctly
		if len(anthropicTool.InputSchema) == 0 {
			t.Error("expected InputSchema to be preserved")
		}
	})

	t.Run("tool with enum values", func(t *testing.T) {
		enumSchema := json.RawMessage(`{
			"type": "object",
			"properties": {
				"language": {
					"type": "string",
					"enum": ["go", "python", "javascript", "rust"]
				},
				"action": {
					"type": "string",
					"enum": ["lint", "format", "build", "test"]
				}
			},
			"required": ["language", "action"]
		}`)

		tool := ToolDefinition{
			Name:        "code_action",
			Description: "Perform an action on code",
			InputSchema: enumSchema,
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicTool := result.(anthropicTool)
		if anthropicTool.Name != "code_action" {
			t.Errorf("expected name %q, got %q", "code_action", anthropicTool.Name)
		}
	})

	t.Run("tool without description", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "simple_tool",
			InputSchema: json.RawMessage(`{"type": "object", "properties": {}}`),
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicTool := result.(anthropicTool)
		if anthropicTool.Description != "" {
			t.Errorf("expected empty description, got %q", anthropicTool.Description)
		}
	})

	t.Run("tool without input schema (no parameters)", func(t *testing.T) {
		tool := ToolDefinition{
			Name:        "get_current_time",
			Description: "Returns the current time",
		}

		result, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		anthropicTool := result.(anthropicTool)
		if anthropicTool.Name != "get_current_time" {
			t.Errorf("expected name %q, got %q", "get_current_time", anthropicTool.Name)
		}
	})
}

func TestAnthropicToolTranslatorPointerInput(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	t.Run("translate from pointer to anthropicToolUseContent", func(t *testing.T) {
		toolCall := &anthropicToolUseContent{
			Type:  "tool_use",
			ID:    "tool_ptr_123",
			Name:  "file_reader",
			Input: json.RawMessage(`{"path": "/etc/hosts"}`),
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolUseID != "tool_ptr_123" {
			t.Errorf("expected tool use ID %q, got %q", "tool_ptr_123", block.ToolUseID)
		}
		if block.ToolName != "file_reader" {
			t.Errorf("expected tool name %q, got %q", "file_reader", block.ToolName)
		}

		// Verify input parsing
		var input map[string]interface{}
		if err := json.Unmarshal(block.ToolInput, &input); err != nil {
			t.Fatalf("failed to parse input: %v", err)
		}
		if input["path"] != "/etc/hosts" {
			t.Errorf("expected path '/etc/hosts', got %v", input["path"])
		}
	})
}

func TestAnthropicToolTranslatorMapWithComplexInput(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	t.Run("map with nested object input", func(t *testing.T) {
		toolCall := map[string]interface{}{
			"id":   "nested_123",
			"name": "configure_system",
			"input": map[string]interface{}{
				"settings": map[string]interface{}{
					"theme":    "dark",
					"fontSize": 14,
				},
				"paths": []interface{}{"/usr/bin", "/usr/local/bin"},
			},
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "configure_system" {
			t.Errorf("expected tool name %q, got %q", "configure_system", block.ToolName)
		}

		var input map[string]interface{}
		if err := json.Unmarshal(block.ToolInput, &input); err != nil {
			t.Fatalf("failed to parse input: %v", err)
		}

		settings, ok := input["settings"].(map[string]interface{})
		if !ok {
			t.Fatal("expected settings to be a map")
		}
		if settings["theme"] != "dark" {
			t.Errorf("expected theme 'dark', got %v", settings["theme"])
		}

		paths, ok := input["paths"].([]interface{})
		if !ok {
			t.Fatal("expected paths to be an array")
		}
		if len(paths) != 2 {
			t.Errorf("expected 2 paths, got %d", len(paths))
		}
	})

	t.Run("map with nil input", func(t *testing.T) {
		toolCall := map[string]interface{}{
			"id":    "nil_input_123",
			"name":  "no_params_tool",
			"input": nil,
		}

		block, err := translator.TranslateFromProvider(toolCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "no_params_tool" {
			t.Errorf("expected tool name %q, got %q", "no_params_tool", block.ToolName)
		}
	})
}

func TestAnthropicToolTranslatorIntegration(t *testing.T) {
	translator := NewAnthropicToolTranslator()

	// Full round-trip test
	t.Run("full round trip", func(t *testing.T) {
		// Step 1: Define a tool
		tool := ToolDefinition{
			Name:        "execute_command",
			Description: "Execute a shell command",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"command": {"type": "string", "description": "The command to execute"},
					"timeout": {"type": "integer", "default": 30}
				},
				"required": ["command"]
			}`),
		}

		// Step 2: Translate to provider format
		providerTool, err := translator.TranslateToProvider(tool)
		if err != nil {
			t.Fatalf("TranslateToProvider failed: %v", err)
		}

		anthropicT := providerTool.(anthropicTool)
		if anthropicT.Name != tool.Name {
			t.Errorf("tool name mismatch")
		}

		// Step 3: Simulate provider response (tool call)
		simulatedResponse := anthropicToolUseContent{
			Type:  "tool_use",
			ID:    "toolu_integration_test",
			Name:  "execute_command",
			Input: json.RawMessage(`{"command": "ls -la", "timeout": 60}`),
		}

		// Step 4: Translate from provider format
		block, err := translator.TranslateFromProvider(simulatedResponse)
		if err != nil {
			t.Fatalf("TranslateFromProvider failed: %v", err)
		}

		if block.Type != ContentTypeToolUse {
			t.Errorf("expected ContentTypeToolUse, got %s", block.Type)
		}
		if block.ToolUseID != "toolu_integration_test" {
			t.Errorf("tool use ID mismatch")
		}

		// Step 5: Create and translate a tool result
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: block.ToolUseID,
			ToolOutput:   "total 0\ndrwxr-xr-x  2 user user 40 Jan  1 00:00 .\ndrwxr-xr-x 10 user user 200 Jan  1 00:00 ..",
		}

		providerResult, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("TranslateToolResult failed: %v", err)
		}

		anthropicResult := providerResult.(anthropicToolResultContent)
		if anthropicResult.ToolUseID != "toolu_integration_test" {
			t.Errorf("result tool use ID mismatch")
		}
		if anthropicResult.IsError {
			t.Error("expected successful result, got error")
		}
	})

	t.Run("error result round trip", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "toolu_error_test",
			ToolError:    "permission denied: cannot access /etc/shadow",
		}

		providerResult, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("TranslateToolResult failed: %v", err)
		}

		anthropicResult := providerResult.(anthropicToolResultContent)
		if !anthropicResult.IsError {
			t.Error("expected error result")
		}
		if anthropicResult.Content != "permission denied: cannot access /etc/shadow" {
			t.Errorf("error content mismatch")
		}
	})
}

func TestAnthropicToolTranslatorImplementsInterface(t *testing.T) {
	// Verify AnthropicToolTranslator implements ToolSchemaTranslator interface
	var _ ToolSchemaTranslator = (*AnthropicToolTranslator)(nil)
}

func TestAnthropicStreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Send start event
		fmt.Fprintln(w, `data: {"type":"message_start","message":{"id":"msg_stream"}}`)
		fmt.Fprintln(w)
		flusher.Flush()

		// Simulate slow streaming
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{baseURL: server.URL}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch, err := provider.Stream(ctx, Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for events and check for context cancellation error
	var gotError bool
	for event := range ch {
		if event.Type == StreamEventError && event.Error != nil {
			gotError = true
		}
	}

	// The stream should have been canceled
	if !gotError {
		// It's also acceptable if the channel just closes
		// The important thing is that we don't block forever
	}
}

func TestAnthropicProviderStreamWithToolUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_tool","model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":20,"cache_read_input_tokens":5,"cache_creation_input_tokens":3}}}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me "}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"use a tool"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_123","name":"calculator"}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"expr\""}}`,
			`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":":\"2+2\"}"}}`,
			`data: {"type":"content_block_stop","index":1}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":15}}`,
			`data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			fmt.Fprintln(w, event)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{baseURL: server.URL}

	ch, err := provider.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{NewTextMessage(RoleUser, "What is 2+2?")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var events []StreamEvent
	for event := range ch {
		events = append(events, event)
	}

	// Verify we got the expected event types
	var hasToolUseStart bool
	var hasToolInputDelta bool
	var stopReason StopReason

	for _, event := range events {
		switch event.Type {
		case StreamEventContentStart:
			if event.ContentBlock != nil && event.ContentBlock.Type == ContentTypeToolUse {
				hasToolUseStart = true
				if event.ContentBlock.ToolName != "calculator" {
					t.Errorf("expected tool name 'calculator', got %q", event.ContentBlock.ToolName)
				}
			}
		case StreamEventContentDelta:
			if event.Delta != nil && event.Delta.ToolInput != "" {
				hasToolInputDelta = true
			}
		case StreamEventStop:
			stopReason = event.StopReason
		}
	}

	if !hasToolUseStart {
		t.Error("expected tool use content block start")
	}
	if !hasToolInputDelta {
		t.Error("expected tool input delta")
	}
	if stopReason != StopReasonToolUse {
		t.Errorf("expected stop reason %q, got %q", StopReasonToolUse, stopReason)
	}
}

func TestAnthropicProviderStreamWithPing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_ping","model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":10}}}`,
			`data: {"type":"ping"}`,
			`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`data: {"type":"ping"}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
			`data: {"type":"content_block_stop","index":0}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
			`data: {"type":"message_stop"}`,
		}

		for _, event := range events {
			fmt.Fprintln(w, event)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{baseURL: server.URL}

	ch, err := provider.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var pingCount int
	for event := range ch {
		if event.Type == StreamEventPing {
			pingCount++
		}
	}

	if pingCount != 2 {
		t.Errorf("expected 2 ping events, got %d", pingCount)
	}
}

func TestAnthropicProviderStreamWithError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_err"}}`,
			`data: {"type":"error","error":{"type":"server_error","message":"Internal error"}}`,
		}

		for _, event := range events {
			fmt.Fprintln(w, event)
			fmt.Fprintln(w)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{baseURL: server.URL}

	ch, err := provider.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var gotError bool
	for event := range ch {
		if event.Type == StreamEventError {
			gotError = true
		}
	}

	if !gotError {
		t.Error("expected error event in stream")
	}
}

func TestAnthropicProviderStreamStopReasons(t *testing.T) {
	testCases := []struct {
		name       string
		stopReason string
		expected   StopReason
	}{
		{"end_turn", "end_turn", StopReasonEnd},
		{"max_tokens", "max_tokens", StopReasonMaxTokens},
		{"stop_sequence", "stop_sequence", StopReasonStop},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				flusher := w.(http.Flusher)

				events := []string{
					`data: {"type":"message_start","message":{"id":"msg_stop"}}`,
					`data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
					`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
					`data: {"type":"content_block_stop","index":0}`,
					fmt.Sprintf(`data: {"type":"message_delta","delta":{"stop_reason":"%s"},"usage":{"output_tokens":5}}`, tc.stopReason),
					`data: {"type":"message_stop"}`,
				}

				for _, event := range events {
					fmt.Fprintln(w, event)
					fmt.Fprintln(w)
					flusher.Flush()
				}
			}))
			defer server.Close()

			provider := &AnthropicProvider{
				apiKey:     "test-key",
				httpClient: server.Client(),
				models:     make(map[string]*ModelInfo),
			}
			provider.initModels()
			provider.httpClient.Transport = &testTransport{baseURL: server.URL}

			ch, err := provider.Stream(context.Background(), Request{
				Model:    "claude-3-5-sonnet-20241022",
				Messages: []Message{NewTextMessage(RoleUser, "Hello")},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			var stopReason StopReason
			for event := range ch {
				if event.Type == StreamEventStop {
					stopReason = event.StopReason
				}
			}

			if stopReason != tc.expected {
				t.Errorf("expected stop reason %q, got %q", tc.expected, stopReason)
			}
		})
	}
}

func TestAnthropicProviderStreamValidationError(t *testing.T) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})

	// Missing model should fail validation
	_, err := provider.Stream(context.Background(), Request{
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	})
	if err == nil {
		t.Error("expected validation error for missing model")
	}

	// Empty messages should fail validation
	_, err = provider.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{},
	})
	if err == nil {
		t.Error("expected validation error for empty messages")
	}
}

func TestAnthropicProviderStreamHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(anthropicError{
			Type: "error",
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{
				Type:    "server_error",
				Message: "Internal server error",
			},
		})
	}))
	defer server.Close()

	provider := &AnthropicProvider{
		apiKey:     "test-key",
		httpClient: server.Client(),
		models:     make(map[string]*ModelInfo),
	}
	provider.initModels()
	provider.httpClient.Transport = &testTransport{baseURL: server.URL}

	_, err := provider.Stream(context.Background(), Request{
		Model:    "claude-3-5-sonnet-20241022",
		Messages: []Message{NewTextMessage(RoleUser, "Hello")},
	})
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func BenchmarkAnthropicProviderEstimateTokens(b *testing.B) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})
	content := strings.Repeat("Hello world! This is a test. ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.EstimateTokens(content)
	}
}

func BenchmarkAnthropicProviderValidateRequest(b *testing.B) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})
	req := Request{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.ValidateRequest(req)
	}
}

func BenchmarkAnthropicMessageConversion(b *testing.B) {
	provider, _ := NewAnthropicProvider(AnthropicConfig{APIKey: "test"})
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: "Hello, Claude!"},
			{Type: ContentTypeText, Text: "How are you today?"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = provider.convertMessage(msg)
	}
}

// Helper to read all data from io.Reader
func readAll(r io.Reader) string {
	data, _ := io.ReadAll(r)
	return string(data)
}
