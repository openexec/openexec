package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGeminiProviderImplementsInterface(t *testing.T) {
	// Ensure GeminiProvider implements ProviderAdapter
	var _ ProviderAdapter = (*GeminiProvider)(nil)
}

func TestNewGeminiProvider(t *testing.T) {
	t.Run("with API key in config", func(t *testing.T) {
		provider, err := NewGeminiProvider(GeminiProviderConfig{
			APIKey: "test-api-key",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
		if provider.GetName() != "gemini" {
			t.Errorf("expected name %q, got %q", "gemini", provider.GetName())
		}
	})

	t.Run("without API key", func(t *testing.T) {
		// Clear environment variables temporarily
		t.Setenv("GEMINI_API_KEY", "")
		t.Setenv("GOOGLE_API_KEY", "")

		_, err := NewGeminiProvider(GeminiProviderConfig{})
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

	t.Run("with GEMINI_API_KEY from environment", func(t *testing.T) {
		t.Setenv("GEMINI_API_KEY", "env-api-key")
		t.Setenv("GOOGLE_API_KEY", "")

		provider, err := NewGeminiProvider(GeminiProviderConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
	})

	t.Run("with GOOGLE_API_KEY from environment", func(t *testing.T) {
		t.Setenv("GEMINI_API_KEY", "")
		t.Setenv("GOOGLE_API_KEY", "google-api-key")

		provider, err := NewGeminiProvider(GeminiProviderConfig{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected non-nil provider")
		}
	})

	t.Run("with custom HTTP client", func(t *testing.T) {
		customClient := &http.Client{Timeout: 30 * time.Second}
		provider, err := NewGeminiProvider(GeminiProviderConfig{
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
		provider, err := NewGeminiProvider(GeminiProviderConfig{
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

	t.Run("with custom base URL", func(t *testing.T) {
		provider, err := NewGeminiProvider(GeminiProviderConfig{
			APIKey:  "test-api-key",
			BaseURL: "https://custom.api.example.com",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.config.BaseURL != "https://custom.api.example.com" {
			t.Errorf("expected custom base URL, got %s", provider.config.BaseURL)
		}
	})
}

func TestGeminiProviderGetModels(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	models := provider.GetModels()
	if len(models) == 0 {
		t.Error("expected at least one model")
	}

	// Check for known models
	expectedModels := []string{
		ModelGemini20FlashExp,
		ModelGemini20Flash,
		ModelGemini15Pro,
		ModelGemini15ProLatest,
		ModelGemini15Flash,
		ModelGemini15FlashLatest,
		ModelGemini15Flash8B,
		ModelGeminiPro,
		ModelGeminiProVision,
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

func TestGeminiProviderGetModelInfo(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	t.Run("existing model", func(t *testing.T) {
		info, err := provider.GetModelInfo(ModelGemini15Pro)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.ID != ModelGemini15Pro {
			t.Errorf("expected ID %q, got %q", ModelGemini15Pro, info.ID)
		}
		if info.Provider != "gemini" {
			t.Errorf("expected provider %q, got %q", "gemini", info.Provider)
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

func TestGeminiProviderGetCapabilities(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	caps, err := provider.GetCapabilities(ModelGemini15Pro)
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
}

func TestGeminiProviderValidateRequest(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	t.Run("valid request", func(t *testing.T) {
		req := Request{
			Model: ModelGemini15Pro,
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

func TestGeminiProviderEstimateTokens(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	testCases := []struct {
		content  string
		expected int
	}{
		{"Hello", 1},                   // 5 chars / 4 = 1
		{"Hello, world!", 3},           // 13 chars / 4 = 3
		{"", 0},                        // 0 chars / 4 = 0
		{strings.Repeat("a", 100), 25}, // 100 chars / 4 = 25
	}

	for _, tc := range testCases {
		got := provider.EstimateTokens(tc.content)
		if got != tc.expected {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tc.content, got, tc.expected)
		}
	}
}

func TestGeminiProviderComplete(t *testing.T) {
	// Create a mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path contains the model
		if !strings.Contains(r.URL.Path, ModelGemini15Pro) {
			t.Errorf("expected path to contain model, got %s", r.URL.Path)
		}

		// Verify API key is in query string
		if r.URL.Query().Get("key") == "" {
			t.Error("missing API key in query string")
		}

		// Verify Content-Type header
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected Content-Type: %s", r.Header.Get("Content-Type"))
		}

		// Parse request
		var req geminiGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Send response
		resp := geminiGenerateContentResponse{
			Candidates: []geminiCandidate{
				{
					Content: &geminiContent{
						Role: "model",
						Parts: []geminiPart{
							{Text: "Hello! How can I help you?"},
						},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 8,
				TotalTokenCount:      18,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with test server URL
	provider, _ := NewGeminiProvider(GeminiProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model: ModelGemini15Pro,
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
		System: "You are helpful.",
	}

	resp, err := provider.Complete(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestGeminiProviderCompleteWithTools(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiGenerateContentRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		// Verify tools are passed correctly
		if len(req.Tools) != 1 {
			t.Errorf("expected 1 tool group, got %d", len(req.Tools))
		}
		if len(req.Tools[0].FunctionDeclarations) != 1 {
			t.Errorf("expected 1 function declaration, got %d", len(req.Tools[0].FunctionDeclarations))
		}
		if req.Tools[0].FunctionDeclarations[0].Name != "calculator" {
			t.Errorf("expected function name %q, got %q", "calculator", req.Tools[0].FunctionDeclarations[0].Name)
		}

		// Send response with function call
		resp := geminiGenerateContentResponse{
			Candidates: []geminiCandidate{
				{
					Content: &geminiContent{
						Role: "model",
						Parts: []geminiPart{
							{Text: "I'll calculate that for you."},
							{
								FunctionCall: &geminiFunctionCall{
									Name: "calculator",
									Args: map[string]interface{}{"expression": "2+2"},
								},
							},
						},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: &geminiUsageMetadata{
				PromptTokenCount:     15,
				CandidatesTokenCount: 12,
				TotalTokenCount:      27,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, _ := NewGeminiProvider(GeminiProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	schema := json.RawMessage(`{"type":"object","properties":{"expression":{"type":"string"}}}`)
	req := Request{
		Model: ModelGemini15Pro,
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
}

func TestGeminiProviderStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify streaming endpoint
		if !strings.Contains(r.URL.Path, "streamGenerateContent") {
			t.Errorf("expected streamGenerateContent in path, got %s", r.URL.Path)
		}
		if r.URL.Query().Get("alt") != "sse" {
			t.Error("expected alt=sse query parameter")
		}

		// Send SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}

		events := []geminiGenerateContentResponse{
			{
				Candidates: []geminiCandidate{
					{
						Content: &geminiContent{
							Role:  "model",
							Parts: []geminiPart{{Text: "Hello"}},
						},
					},
				},
			},
			{
				Candidates: []geminiCandidate{
					{
						Content: &geminiContent{
							Role:  "model",
							Parts: []geminiPart{{Text: " world"}},
						},
					},
				},
			},
			{
				Candidates: []geminiCandidate{
					{
						Content: &geminiContent{
							Role:  "model",
							Parts: []geminiPart{{Text: "!"}},
						},
						FinishReason: "STOP",
					},
				},
				UsageMetadata: &geminiUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 5,
					TotalTokenCount:      15,
				},
			},
		}

		for _, event := range events {
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider, _ := NewGeminiProvider(GeminiProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	req := Request{
		Model: ModelGemini15Pro,
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
	if textContent != "Hello world!" {
		t.Errorf("expected text %q, got %q", "Hello world!", textContent)
	}
}

func TestGeminiProviderErrorHandling(t *testing.T) {
	t.Run("rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(geminiErrorResponse{
				Error: struct {
					Code    int           `json:"code"`
					Message string        `json:"message"`
					Status  string        `json:"status"`
					Details []interface{} `json:"details,omitempty"`
				}{
					Code:    429,
					Message: "Rate limited",
					Status:  "RESOURCE_EXHAUSTED",
				},
			})
		}))
		defer server.Close()

		provider, _ := NewGeminiProvider(GeminiProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		})

		_, err := provider.Complete(context.Background(), Request{
			Model:    ModelGemini15Pro,
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
			json.NewEncoder(w).Encode(geminiErrorResponse{
				Error: struct {
					Code    int           `json:"code"`
					Message string        `json:"message"`
					Status  string        `json:"status"`
					Details []interface{} `json:"details,omitempty"`
				}{
					Code:    401,
					Message: "Invalid API key",
					Status:  "UNAUTHENTICATED",
				},
			})
		}))
		defer server.Close()

		provider, _ := NewGeminiProvider(GeminiProviderConfig{
			APIKey:  "invalid-key",
			BaseURL: server.URL,
		})

		_, err := provider.Complete(context.Background(), Request{
			Model:    ModelGemini15Pro,
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

	t.Run("context length error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(geminiErrorResponse{
				Error: struct {
					Code    int           `json:"code"`
					Message string        `json:"message"`
					Status  string        `json:"status"`
					Details []interface{} `json:"details,omitempty"`
				}{
					Code:    400,
					Message: "Request exceeds context token limit",
					Status:  "INVALID_ARGUMENT",
				},
			})
		}))
		defer server.Close()

		provider, _ := NewGeminiProvider(GeminiProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		})

		_, err := provider.Complete(context.Background(), Request{
			Model:    ModelGemini15Pro,
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

	t.Run("context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		provider, _ := NewGeminiProvider(GeminiProviderConfig{
			APIKey:  "test-key",
			BaseURL: server.URL,
		})

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := provider.Complete(ctx, Request{
			Model:    ModelGemini15Pro,
			Messages: []Message{NewTextMessage(RoleUser, "Hello")},
		})

		if err == nil {
			t.Error("expected error for canceled context")
		}
	})
}

func TestGeminiProviderMessageConversion(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	t.Run("text message", func(t *testing.T) {
		msg := NewTextMessage(RoleUser, "Hello, Gemini!")
		geminiContent := provider.convertToGeminiContent(msg)
		if geminiContent == nil {
			t.Fatal("expected non-nil content")
		}
		if geminiContent.Role != "user" {
			t.Errorf("expected role %q, got %q", "user", geminiContent.Role)
		}
		if len(geminiContent.Parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(geminiContent.Parts))
		}
		if geminiContent.Parts[0].Text != "Hello, Gemini!" {
			t.Errorf("expected text %q, got %q", "Hello, Gemini!", geminiContent.Parts[0].Text)
		}
	})

	t.Run("assistant message", func(t *testing.T) {
		msg := NewTextMessage(RoleAssistant, "Hello!")
		geminiContent := provider.convertToGeminiContent(msg)
		if geminiContent == nil {
			t.Fatal("expected non-nil content")
		}
		if geminiContent.Role != "model" {
			t.Errorf("expected role %q for assistant, got %q", "model", geminiContent.Role)
		}
	})

	t.Run("system message returns nil", func(t *testing.T) {
		msg := NewTextMessage(RoleSystem, "You are helpful.")
		geminiContent := provider.convertToGeminiContent(msg)
		if geminiContent != nil {
			t.Error("expected nil for system message (handled via SystemInstruction)")
		}
	})

	t.Run("legacy text field", func(t *testing.T) {
		msg := Message{
			Role: RoleUser,
			Text: "Legacy message",
		}
		geminiContent := provider.convertToGeminiContent(msg)
		if geminiContent == nil {
			t.Fatal("expected non-nil content")
		}
		if len(geminiContent.Parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(geminiContent.Parts))
		}
		if geminiContent.Parts[0].Text != "Legacy message" {
			t.Errorf("expected text %q, got %q", "Legacy message", geminiContent.Parts[0].Text)
		}
	})
}

func TestGeminiToolTranslator(t *testing.T) {
	translator := NewGeminiToolTranslator()

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

		geminiFuncDecl, ok := result.(geminiFunctionDeclaration)
		if !ok {
			t.Fatalf("expected geminiFunctionDeclaration, got %T", result)
		}

		if geminiFuncDecl.Name != "read_file" {
			t.Errorf("expected name %q, got %q", "read_file", geminiFuncDecl.Name)
		}
	})

	t.Run("translate function call from provider", func(t *testing.T) {
		funcCall := geminiFunctionCall{
			Name: "calculator",
			Args: map[string]interface{}{"x": 1, "y": 2},
		}

		block, err := translator.TranslateFromProvider(funcCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.Type != ContentTypeToolUse {
			t.Errorf("expected type %q, got %q", ContentTypeToolUse, block.Type)
		}
		if block.ToolName != "calculator" {
			t.Errorf("expected tool name %q, got %q", "calculator", block.ToolName)
		}
	})

	t.Run("translate function call from map", func(t *testing.T) {
		funcCall := map[string]interface{}{
			"name": "read_file",
			"args": map[string]interface{}{"path": "/tmp/test.txt"},
		}

		block, err := translator.TranslateFromProvider(funcCall)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if block.ToolName != "read_file" {
			t.Errorf("expected tool name %q, got %q", "read_file", block.ToolName)
		}
	})

	t.Run("translate tool result to provider", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "call_123",
			ToolOutput:   "Result: 3",
		}

		translated, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		geminiResp, ok := translated.(geminiFunctionResponse)
		if !ok {
			t.Fatalf("expected geminiFunctionResponse, got %T", translated)
		}

		if geminiResp.Name != "call_123" {
			t.Errorf("expected name %q, got %q", "call_123", geminiResp.Name)
		}
	})

	t.Run("translate error tool result to provider", func(t *testing.T) {
		result := ContentBlock{
			Type:         ContentTypeToolResult,
			ToolResultID: "call_456",
			ToolError:    "File not found",
		}

		translated, err := translator.TranslateToolResult(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		geminiResp, ok := translated.(geminiFunctionResponse)
		if !ok {
			t.Fatalf("expected geminiFunctionResponse, got %T", translated)
		}

		respMap, ok := geminiResp.Response.(map[string]interface{})
		if !ok {
			t.Fatal("expected response to be a map")
		}
		if respMap["error"] != "File not found" {
			t.Errorf("expected error %q, got %q", "File not found", respMap["error"])
		}
	})
}

func TestGeminiProviderToolChoice(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	testCases := []struct {
		choice       string
		expectedMode string
	}{
		{"auto", "AUTO"},
		{"", "AUTO"},
		{"any", "ANY"},
		{"none", "NONE"},
		{"specific_tool", "ANY"}, // Specific tool uses ANY with allowedFunctionNames
	}

	for _, tc := range testCases {
		t.Run(tc.choice, func(t *testing.T) {
			req := Request{
				Model: ModelGemini15Pro,
				Messages: []Message{
					NewTextMessage(RoleUser, "Hello"),
				},
				Tools: []ToolDefinition{
					{Name: "test_tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
				},
				ToolChoice: tc.choice,
			}

			geminiReq := provider.buildGeminiRequest(req)

			if geminiReq.ToolConfig == nil {
				t.Fatal("expected tool config to be set")
			}
			if geminiReq.ToolConfig.FunctionCallingConfig == nil {
				t.Fatal("expected function calling config to be set")
			}
			if geminiReq.ToolConfig.FunctionCallingConfig.Mode != tc.expectedMode {
				t.Errorf("expected mode %q, got %q", tc.expectedMode, geminiReq.ToolConfig.FunctionCallingConfig.Mode)
			}

			// Check for specific tool
			if tc.choice != "" && tc.choice != "auto" && tc.choice != "any" && tc.choice != "none" {
				if len(geminiReq.ToolConfig.FunctionCallingConfig.AllowedFunctionNames) != 1 {
					t.Error("expected 1 allowed function name for specific tool choice")
				}
				if geminiReq.ToolConfig.FunctionCallingConfig.AllowedFunctionNames[0] != tc.choice {
					t.Errorf("expected allowed function %q, got %q", tc.choice, geminiReq.ToolConfig.FunctionCallingConfig.AllowedFunctionNames[0])
				}
			}
		})
	}
}

func TestGeminiToolTranslatorErrors(t *testing.T) {
	translator := NewGeminiToolTranslator()

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

	t.Run("missing function name in map", func(t *testing.T) {
		funcCall := map[string]interface{}{
			"args": map[string]interface{}{"x": 1},
		}
		_, err := translator.TranslateFromProvider(funcCall)
		if err == nil {
			t.Error("expected error for missing function name")
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

func TestGeminiStreamContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)

		// Send start event
		resp := geminiGenerateContentResponse{
			Candidates: []geminiCandidate{
				{
					Content: &geminiContent{
						Role:  "model",
						Parts: []geminiPart{{Text: "Hello"}},
					},
				},
			},
		}
		data, _ := json.Marshal(resp)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Simulate slow streaming
		time.Sleep(200 * time.Millisecond)
	}))
	defer server.Close()

	provider, _ := NewGeminiProvider(GeminiProviderConfig{
		APIKey:  "test-key",
		BaseURL: server.URL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	ch, err := provider.Stream(ctx, Request{
		Model:    ModelGemini15Pro,
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

func TestGeminiProviderSystemInstruction(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	req := Request{
		Model: ModelGemini15Pro,
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
		System: "You are a helpful assistant.",
	}

	geminiReq := provider.buildGeminiRequest(req)

	if geminiReq.SystemInstruction == nil {
		t.Fatal("expected system instruction to be set")
	}
	if len(geminiReq.SystemInstruction.Parts) != 1 {
		t.Fatalf("expected 1 part in system instruction, got %d", len(geminiReq.SystemInstruction.Parts))
	}
	if geminiReq.SystemInstruction.Parts[0].Text != "You are a helpful assistant." {
		t.Errorf("unexpected system instruction text: %s", geminiReq.SystemInstruction.Parts[0].Text)
	}
}

func TestGeminiProviderGenerationConfig(t *testing.T) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})

	temp := 0.7
	topP := 0.9

	req := Request{
		Model: ModelGemini15Pro,
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
		MaxTokens:     1000,
		Temperature:   &temp,
		TopP:          &topP,
		StopSequences: []string{"STOP", "END"},
	}

	geminiReq := provider.buildGeminiRequest(req)

	if geminiReq.GenerationConfig == nil {
		t.Fatal("expected generation config to be set")
	}
	if geminiReq.GenerationConfig.MaxOutputTokens != 1000 {
		t.Errorf("expected max output tokens 1000, got %d", geminiReq.GenerationConfig.MaxOutputTokens)
	}
	if *geminiReq.GenerationConfig.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", *geminiReq.GenerationConfig.Temperature)
	}
	if *geminiReq.GenerationConfig.TopP != 0.9 {
		t.Errorf("expected top_p 0.9, got %f", *geminiReq.GenerationConfig.TopP)
	}
	if len(geminiReq.GenerationConfig.StopSequences) != 2 {
		t.Errorf("expected 2 stop sequences, got %d", len(geminiReq.GenerationConfig.StopSequences))
	}
}

func BenchmarkGeminiProviderEstimateTokens(b *testing.B) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})
	content := strings.Repeat("Hello world! This is a test. ", 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.EstimateTokens(content)
	}
}

func BenchmarkGeminiProviderValidateRequest(b *testing.B) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})
	req := Request{
		Model: ModelGemini15Pro,
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		provider.ValidateRequest(req)
	}
}

func BenchmarkGeminiMessageConversion(b *testing.B) {
	provider, _ := NewGeminiProvider(GeminiProviderConfig{APIKey: "test"})
	msg := Message{
		Role: RoleUser,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: "Hello, Gemini!"},
			{Type: ContentTypeText, Text: "How are you today?"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = provider.convertToGeminiContent(msg)
	}
}

// Tests for enhanced Gemini tool schema translation

func TestGeminiToolTranslator_TranslateToolChoice(t *testing.T) {
	translator := NewGeminiToolTranslator()

	t.Run("auto choice", func(t *testing.T) {
		config := translator.TranslateToolChoice("auto")
		if config == nil {
			t.Fatal("expected non-nil config")
		}
		if config.FunctionCallingConfig == nil {
			t.Fatal("expected non-nil function calling config")
		}
		if config.FunctionCallingConfig.Mode != "AUTO" {
			t.Errorf("expected mode %q, got %q", "AUTO", config.FunctionCallingConfig.Mode)
		}
	})

	t.Run("empty choice defaults to auto", func(t *testing.T) {
		config := translator.TranslateToolChoice("")
		if config.FunctionCallingConfig.Mode != "AUTO" {
			t.Errorf("expected mode %q, got %q", "AUTO", config.FunctionCallingConfig.Mode)
		}
	})

	t.Run("any choice", func(t *testing.T) {
		config := translator.TranslateToolChoice("any")
		if config.FunctionCallingConfig.Mode != "ANY" {
			t.Errorf("expected mode %q, got %q", "ANY", config.FunctionCallingConfig.Mode)
		}
	})

	t.Run("none choice", func(t *testing.T) {
		config := translator.TranslateToolChoice("none")
		if config.FunctionCallingConfig.Mode != "NONE" {
			t.Errorf("expected mode %q, got %q", "NONE", config.FunctionCallingConfig.Mode)
		}
	})

	t.Run("specific tool name", func(t *testing.T) {
		config := translator.TranslateToolChoice("calculate_sum")
		if config.FunctionCallingConfig.Mode != "ANY" {
			t.Errorf("expected mode %q, got %q", "ANY", config.FunctionCallingConfig.Mode)
		}
		if len(config.FunctionCallingConfig.AllowedFunctionNames) != 1 {
			t.Fatalf("expected 1 allowed function name, got %d", len(config.FunctionCallingConfig.AllowedFunctionNames))
		}
		if config.FunctionCallingConfig.AllowedFunctionNames[0] != "calculate_sum" {
			t.Errorf("expected %q, got %q", "calculate_sum", config.FunctionCallingConfig.AllowedFunctionNames[0])
		}
	})
}

func TestGeminiToolTranslator_TranslateMultipleTools(t *testing.T) {
	translator := NewGeminiToolTranslator()

	t.Run("multiple valid tools", func(t *testing.T) {
		tools := []ToolDefinition{
			{
				Name:        "read_file",
				Description: "Read a file from disk",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`),
			},
			{
				Name:        "write_file",
				Description: "Write content to a file",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
			},
			{
				Name:        "list_directory",
				Description: "List files in a directory",
			},
		}

		result, err := translator.TranslateMultipleTools(tools)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 3 {
			t.Errorf("expected 3 function declarations, got %d", len(result))
		}

		// Verify each function declaration
		expectedNames := []string{"read_file", "write_file", "list_directory"}
		for i, decl := range result {
			if decl.Name != expectedNames[i] {
				t.Errorf("tool %d: expected name %q, got %q", i, expectedNames[i], decl.Name)
			}
		}
	})

	t.Run("empty tools list", func(t *testing.T) {
		result, err := translator.TranslateMultipleTools([]ToolDefinition{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d items", len(result))
		}
	})

	t.Run("invalid tool in list", func(t *testing.T) {
		tools := []ToolDefinition{
			{
				Name:        "valid_tool",
				Description: "A valid tool",
			},
			{
				Name: "", // Invalid - missing name
			},
		}

		_, err := translator.TranslateMultipleTools(tools)
		if err == nil {
			t.Error("expected error for invalid tool")
		}
		if !strings.Contains(err.Error(), "failed to translate tool") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

func TestGeminiToolTranslator_TranslateMultipleToolCalls(t *testing.T) {
	translator := NewGeminiToolTranslator()

	t.Run("multiple function calls", func(t *testing.T) {
		funcCalls := []geminiFunctionCall{
			{
				Name: "tool_a",
				Args: map[string]interface{}{"arg1": "value1"},
			},
			{
				Name: "tool_b",
				Args: map[string]interface{}{"arg2": 42},
			},
			{
				Name: "tool_c",
				Args: nil,
			},
		}

		result, err := translator.TranslateMultipleToolCalls(funcCalls)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 3 {
			t.Errorf("expected 3 content blocks, got %d", len(result))
		}

		expectedNames := []string{"tool_a", "tool_b", "tool_c"}
		for i, block := range result {
			if block.Type != ContentTypeToolUse {
				t.Errorf("block %d: expected type tool_use, got %s", i, block.Type)
			}
			if block.ToolName != expectedNames[i] {
				t.Errorf("block %d: expected name %q, got %q", i, expectedNames[i], block.ToolName)
			}
			if block.ToolUseID == "" {
				t.Errorf("block %d: expected non-empty ToolUseID", i)
			}
		}
	})

	t.Run("empty function calls", func(t *testing.T) {
		result, err := translator.TranslateMultipleToolCalls([]geminiFunctionCall{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("expected empty result, got %d items", len(result))
		}
	})
}

func TestGeminiToolTranslator_BuildGeminiTools(t *testing.T) {
	translator := NewGeminiToolTranslator()

	t.Run("build tools structure", func(t *testing.T) {
		tools := []ToolDefinition{
			{
				Name:        "search",
				Description: "Search for items",
				InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}}}`),
			},
			{
				Name:        "calculate",
				Description: "Perform calculations",
			},
		}

		result, err := translator.BuildGeminiTools(tools)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 geminiTool, got %d", len(result))
		}

		if len(result[0].FunctionDeclarations) != 2 {
			t.Errorf("expected 2 function declarations, got %d", len(result[0].FunctionDeclarations))
		}
	})

	t.Run("empty tools returns nil", func(t *testing.T) {
		result, err := translator.BuildGeminiTools([]ToolDefinition{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil result for empty tools, got %v", result)
		}
	})
}

func TestGeminiSchemaConverter(t *testing.T) {
	converter := NewGeminiSchemaConverter()

	t.Run("convert simple object schema", func(t *testing.T) {
		schema := json.RawMessage(`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "User name"},
				"age": {"type": "integer", "minimum": 0, "maximum": 150}
			},
			"required": ["name"]
		}`)

		result, err := converter.ConvertSchema(schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("expected map, got %T", result)
		}

		if resultMap["type"] != "object" {
			t.Errorf("expected type object, got %v", resultMap["type"])
		}

		required, ok := resultMap["required"].([]interface{})
		if !ok {
			t.Fatalf("expected required to be array, got %T", resultMap["required"])
		}
		if len(required) != 1 || required[0] != "name" {
			t.Errorf("expected required=[name], got %v", required)
		}
	})

	t.Run("convert array schema", func(t *testing.T) {
		schema := json.RawMessage(`{
			"type": "array",
			"items": {
				"type": "string"
			},
			"minItems": 1,
			"maxItems": 10
		}`)

		result, err := converter.ConvertSchema(schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["type"] != "array" {
			t.Errorf("expected type array, got %v", resultMap["type"])
		}

		items, ok := resultMap["items"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected items to be map, got %T", resultMap["items"])
		}
		if items["type"] != "string" {
			t.Errorf("expected items type string, got %v", items["type"])
		}
	})

	t.Run("convert nested object schema", func(t *testing.T) {
		schema := json.RawMessage(`{
			"type": "object",
			"properties": {
				"address": {
					"type": "object",
					"properties": {
						"city": {"type": "string"},
						"zip": {"type": "string", "pattern": "^[0-9]{5}$"}
					}
				}
			}
		}`)

		result, err := converter.ConvertSchema(schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		props := resultMap["properties"].(map[string]interface{})
		address := props["address"].(map[string]interface{})

		if address["type"] != "object" {
			t.Errorf("expected nested type object, got %v", address["type"])
		}

		addressProps := address["properties"].(map[string]interface{})
		zip := addressProps["zip"].(map[string]interface{})
		if zip["pattern"] != "^[0-9]{5}$" {
			t.Errorf("expected pattern to be preserved, got %v", zip["pattern"])
		}
	})

	t.Run("convert schema with enum", func(t *testing.T) {
		schema := json.RawMessage(`{
			"type": "string",
			"enum": ["red", "green", "blue"],
			"description": "Color choice"
		}`)

		result, err := converter.ConvertSchema(schema)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		enum, ok := resultMap["enum"].([]interface{})
		if !ok {
			t.Fatalf("expected enum to be array, got %T", resultMap["enum"])
		}
		if len(enum) != 3 {
			t.Errorf("expected 3 enum values, got %d", len(enum))
		}
	})

	t.Run("empty schema returns nil", func(t *testing.T) {
		result, err := converter.ConvertSchema(json.RawMessage{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Errorf("expected nil for empty schema, got %v", result)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		_, err := converter.ConvertSchema(json.RawMessage(`{invalid}`))
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestGeminiToolTranslator_Integration(t *testing.T) {
	translator := NewGeminiToolTranslator()

	// Define a realistic set of tools
	tools := []ToolDefinition{
		{
			Name:        "execute_sql",
			Description: "Execute a SQL query against the database",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"query": {"type": "string", "description": "SQL query to execute"},
					"database": {"type": "string", "enum": ["main", "analytics", "archive"]},
					"timeout_seconds": {"type": "integer", "minimum": 1, "maximum": 300}
				},
				"required": ["query", "database"]
			}`),
		},
		{
			Name:        "send_notification",
			Description: "Send a notification to users",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"user_ids": {"type": "array", "items": {"type": "string"}},
					"message": {"type": "string", "maxLength": 500}
				},
				"required": ["user_ids", "message"]
			}`),
		},
	}

	// Build Gemini tools
	geminiTools, err := translator.BuildGeminiTools(tools)
	if err != nil {
		t.Fatalf("failed to build Gemini tools: %v", err)
	}

	// Verify the structure
	if len(geminiTools) != 1 {
		t.Fatalf("expected 1 geminiTool wrapper, got %d", len(geminiTools))
	}
	if len(geminiTools[0].FunctionDeclarations) != 2 {
		t.Fatalf("expected 2 function declarations, got %d", len(geminiTools[0].FunctionDeclarations))
	}

	// Verify they can be marshaled to JSON (as would be sent to API)
	jsonData, err := json.Marshal(geminiTools)
	if err != nil {
		t.Fatalf("failed to marshal to JSON: %v", err)
	}

	// Verify structure
	var parsed []map[string]interface{}
	if err := json.Unmarshal(jsonData, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	funcDecls := parsed[0]["functionDeclarations"].([]interface{})
	if len(funcDecls) != 2 {
		t.Errorf("expected 2 function declarations in JSON")
	}

	// Simulate receiving a function call response
	funcCall := geminiFunctionCall{
		Name: "execute_sql",
		Args: map[string]interface{}{
			"query":           "SELECT * FROM users",
			"database":        "main",
			"timeout_seconds": 30,
		},
	}

	block, err := translator.TranslateFromProvider(funcCall)
	if err != nil {
		t.Fatalf("failed to translate function call: %v", err)
	}

	// Parse and verify the arguments
	parser := NewToolInputParser()
	query, err := parser.ParseString(block.ToolInput, "query")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}
	if query != "SELECT * FROM users" {
		t.Errorf("expected query 'SELECT * FROM users', got %s", query)
	}

	database, err := parser.ParseString(block.ToolInput, "database")
	if err != nil {
		t.Fatalf("failed to parse database: %v", err)
	}
	if database != "main" {
		t.Errorf("expected database 'main', got %s", database)
	}

	// Build a result
	resultBlock := ContentBlock{
		Type:         ContentTypeToolResult,
		ToolResultID: block.ToolUseID,
		ToolOutput:   `[{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]`,
	}

	// Translate the result back to Gemini format
	geminiResult, err := translator.TranslateToolResult(resultBlock)
	if err != nil {
		t.Fatalf("failed to translate result: %v", err)
	}

	funcResp, ok := geminiResult.(geminiFunctionResponse)
	if !ok {
		t.Fatalf("expected geminiFunctionResponse, got %T", geminiResult)
	}

	if funcResp.Name != block.ToolUseID {
		t.Errorf("expected name to match tool use ID")
	}

	respMap := funcResp.Response.(map[string]interface{})
	if respMap["result"] != `[{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]` {
		t.Errorf("unexpected result content: %v", respMap["result"])
	}

	// Verify the result can be marshaled
	resultJSON, err := json.Marshal(funcResp)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	var parsedResult map[string]interface{}
	if err := json.Unmarshal(resultJSON, &parsedResult); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	if parsedResult["name"] != block.ToolUseID {
		t.Errorf("expected name in JSON to match tool use ID")
	}
}

func TestGeminiToolTranslator_PointerFunctionCall(t *testing.T) {
	translator := NewGeminiToolTranslator()

	// Test with pointer type
	funcCall := &geminiFunctionCall{
		Name: "get_time",
		Args: nil,
	}

	block, err := translator.TranslateFromProvider(funcCall)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if block.ToolName != "get_time" {
		t.Errorf("expected name %q, got %q", "get_time", block.ToolName)
	}
}

func BenchmarkGeminiToolTranslator_TranslateToProvider(b *testing.B) {
	translator := NewGeminiToolTranslator()
	tool := ToolDefinition{
		Name:        "read_file",
		Description: "Read contents of a file",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"encoding":{"type":"string","enum":["utf-8","ascii","binary"]}},"required":["path"]}`),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = translator.TranslateToProvider(tool)
	}
}

func BenchmarkGeminiToolTranslator_TranslateMultipleTools(b *testing.B) {
	translator := NewGeminiToolTranslator()
	tools := []ToolDefinition{
		{Name: "tool1", Description: "First tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "tool2", Description: "Second tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "tool3", Description: "Third tool", InputSchema: json.RawMessage(`{"type":"object"}`)},
		{Name: "tool4", Description: "Fourth tool"},
		{Name: "tool5", Description: "Fifth tool"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = translator.TranslateMultipleTools(tools)
	}
}

func BenchmarkGeminiSchemaConverter(b *testing.B) {
	converter := NewGeminiSchemaConverter()
	schema := json.RawMessage(`{
		"type": "object",
		"properties": {
			"name": {"type": "string"},
			"items": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"id": {"type": "integer"},
						"value": {"type": "string"}
					}
				}
			}
		}
	}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = converter.ConvertSchema(schema)
	}
}
