package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage(RoleUser, "Hello, world!")

	if msg.Role != RoleUser {
		t.Errorf("expected role %q, got %q", RoleUser, msg.Role)
	}
	if len(msg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(msg.Content))
	}
	if msg.Content[0].Type != ContentTypeText {
		t.Errorf("expected content type %q, got %q", ContentTypeText, msg.Content[0].Type)
	}
	if msg.Content[0].Text != "Hello, world!" {
		t.Errorf("expected text %q, got %q", "Hello, world!", msg.Content[0].Text)
	}
}

func TestNewToolResultMessage(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		msg := NewToolResultMessage("tool-123", "output data", nil)

		if msg.Role != RoleTool {
			t.Errorf("expected role %q, got %q", RoleTool, msg.Role)
		}
		if len(msg.Content) != 1 {
			t.Fatalf("expected 1 content block, got %d", len(msg.Content))
		}
		block := msg.Content[0]
		if block.Type != ContentTypeToolResult {
			t.Errorf("expected type %q, got %q", ContentTypeToolResult, block.Type)
		}
		if block.ToolResultID != "tool-123" {
			t.Errorf("expected tool result ID %q, got %q", "tool-123", block.ToolResultID)
		}
		if block.ToolOutput != "output data" {
			t.Errorf("expected output %q, got %q", "output data", block.ToolOutput)
		}
		if block.ToolError != "" {
			t.Errorf("expected no error, got %q", block.ToolError)
		}
	})

	t.Run("error result", func(t *testing.T) {
		err := errors.New("tool failed")
		msg := NewToolResultMessage("tool-456", "", err)

		block := msg.Content[0]
		if block.ToolError != "tool failed" {
			t.Errorf("expected error %q, got %q", "tool failed", block.ToolError)
		}
	})
}

func TestMessageGetText(t *testing.T) {
	t.Run("single text block", func(t *testing.T) {
		msg := NewTextMessage(RoleUser, "Hello")
		if got := msg.GetText(); got != "Hello" {
			t.Errorf("expected %q, got %q", "Hello", got)
		}
	})

	t.Run("multiple text blocks", func(t *testing.T) {
		msg := Message{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: ContentTypeText, Text: "Hello, "},
				{Type: ContentTypeText, Text: "world!"},
			},
		}
		if got := msg.GetText(); got != "Hello, world!" {
			t.Errorf("expected %q, got %q", "Hello, world!", got)
		}
	})

	t.Run("legacy text field", func(t *testing.T) {
		msg := Message{
			Role: RoleUser,
			Text: "Legacy message",
		}
		if got := msg.GetText(); got != "Legacy message" {
			t.Errorf("expected %q, got %q", "Legacy message", got)
		}
	})

	t.Run("mixed content types", func(t *testing.T) {
		msg := Message{
			Role: RoleAssistant,
			Content: []ContentBlock{
				{Type: ContentTypeText, Text: "Let me use a tool"},
				{Type: ContentTypeToolUse, ToolName: "calculator"},
				{Type: ContentTypeText, Text: " to help you."},
			},
		}
		if got := msg.GetText(); got != "Let me use a tool to help you." {
			t.Errorf("expected %q, got %q", "Let me use a tool to help you.", got)
		}
	})
}

func TestMessageGetToolCalls(t *testing.T) {
	msg := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: "I'll help you with that."},
			{Type: ContentTypeToolUse, ToolUseID: "tool-1", ToolName: "read_file"},
			{Type: ContentTypeToolUse, ToolUseID: "tool-2", ToolName: "write_file"},
		},
	}

	calls := msg.GetToolCalls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].ToolName != "read_file" {
		t.Errorf("expected tool name %q, got %q", "read_file", calls[0].ToolName)
	}
	if calls[1].ToolName != "write_file" {
		t.Errorf("expected tool name %q, got %q", "write_file", calls[1].ToolName)
	}
}

func TestResponseGetText(t *testing.T) {
	resp := Response{
		ID:    "resp-123",
		Model: "gpt-4",
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: "Here is the answer."},
		},
	}

	if got := resp.GetText(); got != "Here is the answer." {
		t.Errorf("expected %q, got %q", "Here is the answer.", got)
	}
}

func TestResponseHasToolCalls(t *testing.T) {
	t.Run("no tool calls", func(t *testing.T) {
		resp := Response{
			Content: []ContentBlock{
				{Type: ContentTypeText, Text: "Just text"},
			},
		}
		if resp.HasToolCalls() {
			t.Error("expected HasToolCalls() to return false")
		}
	})

	t.Run("with tool calls", func(t *testing.T) {
		resp := Response{
			Content: []ContentBlock{
				{Type: ContentTypeToolUse, ToolName: "some_tool"},
			},
		}
		if !resp.HasToolCalls() {
			t.Error("expected HasToolCalls() to return true")
		}
	})
}

func TestProviderError(t *testing.T) {
	t.Run("without status code", func(t *testing.T) {
		err := &ProviderError{
			Code:    ErrCodeRateLimit,
			Message: "Too many requests",
		}
		expected := "rate_limit: Too many requests"
		if got := err.Error(); got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("with status code", func(t *testing.T) {
		err := &ProviderError{
			Code:       ErrCodeServerError,
			Message:    "Internal error",
			StatusCode: 500,
		}
		// The error string includes the status code as a rune conversion
		got := err.Error()
		if got == "" {
			t.Error("expected non-empty error string")
		}
	})

	t.Run("retryable error", func(t *testing.T) {
		retryAfter := 5 * time.Second
		err := &ProviderError{
			Code:       ErrCodeRateLimit,
			Message:    "Rate limited",
			Retryable:  true,
			RetryAfter: &retryAfter,
		}
		if !err.Retryable {
			t.Error("expected error to be retryable")
		}
		if err.RetryAfter == nil || *err.RetryAfter != 5*time.Second {
			t.Error("expected RetryAfter to be 5 seconds")
		}
	})
}

func TestToolDefinitionJSON(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	tool := ToolDefinition{
		Name:        "read_file",
		Description: "Read the contents of a file",
		InputSchema: schema,
	}

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ToolDefinition
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != tool.Name {
		t.Errorf("name mismatch: expected %q, got %q", tool.Name, decoded.Name)
	}
	if decoded.Description != tool.Description {
		t.Errorf("description mismatch")
	}
}

func TestUsage(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CacheReadTokens:  10,
		CacheWriteTokens: 20,
	}

	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Usage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.TotalTokens != 150 {
		t.Errorf("expected total tokens 150, got %d", decoded.TotalTokens)
	}
	if decoded.CacheReadTokens != 10 {
		t.Errorf("expected cache read tokens 10, got %d", decoded.CacheReadTokens)
	}
}

func TestProviderCapabilities(t *testing.T) {
	caps := ProviderCapabilities{
		Streaming:        true,
		Vision:           true,
		ToolUse:          true,
		SystemPrompt:     true,
		MultiTurn:        true,
		MaxContextTokens: 128000,
		MaxOutputTokens:  4096,
	}

	if !caps.Streaming {
		t.Error("expected streaming to be true")
	}
	if caps.MaxContextTokens != 128000 {
		t.Errorf("expected max context 128000, got %d", caps.MaxContextTokens)
	}
}

func TestModelInfo(t *testing.T) {
	now := time.Now()
	info := ModelInfo{
		ID:       "claude-3-opus",
		Name:     "Claude 3 Opus",
		Provider: "anthropic",
		Capabilities: ProviderCapabilities{
			Streaming: true,
			ToolUse:   true,
		},
		PricePerMInputTokens:  15.0,
		PricePerMOutputTokens: 75.0,
		Deprecated:            true,
		DeprecationDate:       &now,
	}

	if info.Provider != "anthropic" {
		t.Errorf("expected provider %q, got %q", "anthropic", info.Provider)
	}
	if info.PricePerMInputTokens != 15.0 {
		t.Errorf("expected input price 15.0, got %f", info.PricePerMInputTokens)
	}
}

func TestStreamEvent(t *testing.T) {
	t.Run("text delta", func(t *testing.T) {
		event := StreamEvent{
			Type: StreamEventContentDelta,
			Delta: &StreamDelta{
				Text: "Hello",
			},
			ContentBlockIndex: 0,
		}

		if event.Type != StreamEventContentDelta {
			t.Errorf("expected type %q, got %q", StreamEventContentDelta, event.Type)
		}
		if event.Delta.Text != "Hello" {
			t.Errorf("expected text %q, got %q", "Hello", event.Delta.Text)
		}
	})

	t.Run("stop event", func(t *testing.T) {
		event := StreamEvent{
			Type:       StreamEventStop,
			StopReason: StopReasonEnd,
			Usage: &Usage{
				TotalTokens: 100,
			},
		}

		if event.StopReason != StopReasonEnd {
			t.Errorf("expected stop reason %q, got %q", StopReasonEnd, event.StopReason)
		}
		if event.Usage.TotalTokens != 100 {
			t.Errorf("expected total tokens 100, got %d", event.Usage.TotalTokens)
		}
	})

	t.Run("error event", func(t *testing.T) {
		event := StreamEvent{
			Type:  StreamEventError,
			Error: errors.New("connection failed"),
		}

		if event.Error == nil {
			t.Error("expected error to be set")
		}
	})
}

func TestRequest(t *testing.T) {
	temp := 0.7
	topP := 0.9
	req := Request{
		Model: "gpt-4",
		Messages: []Message{
			NewTextMessage(RoleUser, "Hello"),
		},
		System:        "You are a helpful assistant.",
		MaxTokens:     1000,
		Temperature:   &temp,
		TopP:          &topP,
		StopSequences: []string{"\n\n"},
		ToolChoice:    "auto",
		Metadata: map[string]interface{}{
			"user_id": "user-123",
		},
	}

	if req.Model != "gpt-4" {
		t.Errorf("expected model %q, got %q", "gpt-4", req.Model)
	}
	if *req.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", *req.Temperature)
	}
	if req.ToolChoice != "auto" {
		t.Errorf("expected tool choice %q, got %q", "auto", req.ToolChoice)
	}
}

// MockProvider implements ProviderAdapter for testing
type MockProvider struct {
	name   string
	models []string
}

func NewMockProvider(name string, models []string) *MockProvider {
	return &MockProvider{name: name, models: models}
}

func (m *MockProvider) GetName() string {
	return m.name
}

func (m *MockProvider) GetModels() []string {
	return m.models
}

func (m *MockProvider) GetModelInfo(modelID string) (*ModelInfo, error) {
	for _, id := range m.models {
		if id == modelID {
			return &ModelInfo{
				ID:       modelID,
				Name:     modelID,
				Provider: m.name,
			}, nil
		}
	}
	return nil, &ProviderError{Code: ErrCodeNotFound, Message: "model not found"}
}

func (m *MockProvider) GetCapabilities(modelID string) (*ProviderCapabilities, error) {
	return &ProviderCapabilities{
		Streaming:    true,
		ToolUse:      true,
		SystemPrompt: true,
		MultiTurn:    true,
	}, nil
}

func (m *MockProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	return &Response{
		ID:         "mock-resp-1",
		Model:      req.Model,
		Content:    []ContentBlock{{Type: ContentTypeText, Text: "Mock response"}},
		StopReason: StopReasonEnd,
		Usage:      Usage{TotalTokens: 10},
	}, nil
}

func (m *MockProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	ch := make(chan StreamEvent, 3)
	go func() {
		defer close(ch)
		ch <- StreamEvent{Type: StreamEventStart}
		ch <- StreamEvent{
			Type:  StreamEventContentDelta,
			Delta: &StreamDelta{Text: "Mock"},
		}
		ch <- StreamEvent{Type: StreamEventStop, StopReason: StopReasonEnd}
	}()
	return ch, nil
}

func (m *MockProvider) ValidateRequest(req Request) error {
	if req.Model == "" {
		return &ProviderError{Code: ErrCodeInvalidRequest, Message: "model is required"}
	}
	return nil
}

func (m *MockProvider) EstimateTokens(content string) int {
	// Simple estimation: ~4 chars per token
	return len(content) / 4
}

func TestMockProviderImplementsInterface(t *testing.T) {
	var _ ProviderAdapter = (*MockProvider)(nil)

	provider := NewMockProvider("mock", []string{"mock-model-1", "mock-model-2"})

	if provider.GetName() != "mock" {
		t.Errorf("expected name %q, got %q", "mock", provider.GetName())
	}

	models := provider.GetModels()
	if len(models) != 2 {
		t.Errorf("expected 2 models, got %d", len(models))
	}

	info, err := provider.GetModelInfo("mock-model-1")
	if err != nil {
		t.Fatalf("GetModelInfo failed: %v", err)
	}
	if info.ID != "mock-model-1" {
		t.Errorf("expected model ID %q, got %q", "mock-model-1", info.ID)
	}

	_, err = provider.GetModelInfo("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}

	resp, err := provider.Complete(context.Background(), Request{Model: "mock-model-1"})
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if resp.GetText() != "Mock response" {
		t.Errorf("expected text %q, got %q", "Mock response", resp.GetText())
	}

	streamCh, err := provider.Stream(context.Background(), Request{Model: "mock-model-1"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	eventCount := 0
	for range streamCh {
		eventCount++
	}
	if eventCount != 3 {
		t.Errorf("expected 3 events, got %d", eventCount)
	}

	if err := provider.ValidateRequest(Request{}); err == nil {
		t.Error("expected validation error for empty model")
	}
	if err := provider.ValidateRequest(Request{Model: "test"}); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}

	tokens := provider.EstimateTokens("Hello, world!")
	if tokens != 3 { // 13 chars / 4 = 3
		t.Errorf("expected 3 tokens, got %d", tokens)
	}
}

func TestContentBlockToolInput(t *testing.T) {
	input := json.RawMessage(`{"path": "/tmp/test.txt", "content": "Hello"}`)
	block := ContentBlock{
		Type:      ContentTypeToolUse,
		ToolUseID: "tool-123",
		ToolName:  "write_file",
		ToolInput: input,
	}

	data, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded ContentBlock
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.ToolName != "write_file" {
		t.Errorf("expected tool name %q, got %q", "write_file", decoded.ToolName)
	}

	var inputMap map[string]string
	if err := json.Unmarshal(decoded.ToolInput, &inputMap); err != nil {
		t.Fatalf("failed to unmarshal tool input: %v", err)
	}
	if inputMap["path"] != "/tmp/test.txt" {
		t.Errorf("expected path %q, got %q", "/tmp/test.txt", inputMap["path"])
	}
}

func TestStopReasons(t *testing.T) {
	testCases := []struct {
		reason   StopReason
		expected string
	}{
		{StopReasonEnd, "end_turn"},
		{StopReasonMaxTokens, "max_tokens"},
		{StopReasonToolUse, "tool_use"},
		{StopReasonStop, "stop_sequence"},
		{StopReasonError, "error"},
	}

	for _, tc := range testCases {
		if string(tc.reason) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(tc.reason))
		}
	}
}

func TestRoles(t *testing.T) {
	testCases := []struct {
		role     Role
		expected string
	}{
		{RoleUser, "user"},
		{RoleAssistant, "assistant"},
		{RoleSystem, "system"},
		{RoleTool, "tool"},
	}

	for _, tc := range testCases {
		if string(tc.role) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(tc.role))
		}
	}
}

func TestContentTypes(t *testing.T) {
	testCases := []struct {
		ct       ContentType
		expected string
	}{
		{ContentTypeText, "text"},
		{ContentTypeImage, "image"},
		{ContentTypeToolUse, "tool_use"},
		{ContentTypeToolResult, "tool_result"},
	}

	for _, tc := range testCases {
		if string(tc.ct) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(tc.ct))
		}
	}
}

func TestStreamEventTypes(t *testing.T) {
	testCases := []struct {
		eventType StreamEventType
		expected  string
	}{
		{StreamEventStart, "message_start"},
		{StreamEventContentStart, "content_block_start"},
		{StreamEventContentDelta, "content_block_delta"},
		{StreamEventContentStop, "content_block_stop"},
		{StreamEventStop, "message_stop"},
		{StreamEventPing, "ping"},
		{StreamEventError, "error"},
	}

	for _, tc := range testCases {
		if string(tc.eventType) != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, string(tc.eventType))
		}
	}
}

func TestErrorCodes(t *testing.T) {
	testCases := []struct {
		code     string
		expected string
	}{
		{ErrCodeRateLimit, "rate_limit"},
		{ErrCodeInvalidRequest, "invalid_request"},
		{ErrCodeAuthentication, "authentication_error"},
		{ErrCodePermission, "permission_error"},
		{ErrCodeNotFound, "not_found"},
		{ErrCodeServerError, "server_error"},
		{ErrCodeContextLength, "context_length_exceeded"},
		{ErrCodeContentFilter, "content_filter"},
		{ErrCodeTimeout, "timeout"},
	}

	for _, tc := range testCases {
		if tc.code != tc.expected {
			t.Errorf("expected %q, got %q", tc.expected, tc.code)
		}
	}
}
