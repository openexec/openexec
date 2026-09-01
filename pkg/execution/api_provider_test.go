package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type fakeAPIAdapter struct {
	responses      []*agent.Response
	completeErrors []error
	err            error
	requests       []agent.Request
	stream         <-chan agent.StreamEvent
}

func (a *fakeAPIAdapter) GetName() string     { return "openai-compatible" }
func (a *fakeAPIAdapter) GetModels() []string { return []string{"test-model"} }
func (a *fakeAPIAdapter) GetModelInfo(string) (*agent.ModelInfo, error) {
	return &agent.ModelInfo{}, nil
}
func (a *fakeAPIAdapter) GetCapabilities(string) (*agent.ProviderCapabilities, error) {
	return &agent.ProviderCapabilities{Streaming: true, ToolUse: true}, nil
}
func (a *fakeAPIAdapter) Complete(_ context.Context, request agent.Request) (*agent.Response, error) {
	a.requests = append(a.requests, request)
	if len(a.completeErrors) > 0 {
		err := a.completeErrors[0]
		a.completeErrors = a.completeErrors[1:]
		if err != nil {
			return nil, err
		}
	}
	if a.err != nil {
		return nil, a.err
	}
	response := a.responses[0]
	a.responses = a.responses[1:]
	return response, nil
}
func (a *fakeAPIAdapter) Stream(_ context.Context, request agent.Request) (<-chan agent.StreamEvent, error) {
	a.requests = append(a.requests, request)
	if a.err != nil {
		return nil, a.err
	}
	return a.stream, nil
}
func (a *fakeAPIAdapter) ValidateRequest(agent.Request) error { return nil }
func (a *fakeAPIAdapter) EstimateTokens(string) int           { return 1 }

type recordingToolExecutor struct {
	requests []ToolRequest
}

func (e *recordingToolExecutor) ValidateAccess(_ string, sandbox Sandbox, writableRoots []string) error {
	if sandbox.Mode == "workspace-write" && len(writableRoots) == 0 {
		return errors.New("bounded root required")
	}
	return nil
}

func (e *recordingToolExecutor) ExecuteTool(_ context.Context, request ToolRequest) (string, error) {
	e.requests = append(e.requests, request)
	return "tool output", nil
}

func TestAPIProviderPassesAuthorizationContractToTools(t *testing.T) {
	adapter := &fakeAPIAdapter{responses: []*agent.Response{
		{Content: []agent.ContentBlock{{
			Type: agent.ContentTypeToolUse, ToolUseID: "call-1", ToolName: "write_file",
			ToolInput: json.RawMessage(`{"path":"result.txt"}`),
		}}, StopReason: agent.StopReasonToolUse,
			Metadata: map[string]interface{}{"reasoning_content": "provider continuation state"}},
		{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "finished"}}, StopReason: agent.StopReasonEnd},
	}}
	tools := &recordingToolExecutor{}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: tools,
		Tools: []agent.ToolDefinition{{Name: "write_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "api-1", WorkingDir: root, Prompt: "write", Model: "test-model",
		Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{root},
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSucceeded || result.FinalText != "finished" || len(tools.requests) != 1 {
		t.Fatalf("result=%+v tools=%+v", result, tools.requests)
	}
	toolRequest := tools.requests[0]
	if toolRequest.Sandbox.Mode != "workspace-write" || len(toolRequest.WritableRoots) != 1 || toolRequest.WritableRoots[0] != root {
		t.Fatalf("tool authorization = %+v", toolRequest)
	}
	if len(events) != 6 || events[1].Type != EventToolProposed || events[2].Type != EventToolStarted ||
		events[3].Type != EventToolCompleted || events[4].Type != EventAssistantDelta {
		t.Fatalf("events = %#v", events)
	}
	if len(adapter.requests) != 2 || len(adapter.requests[1].Messages) != 3 {
		t.Fatalf("loop requests = %#v", adapter.requests)
	}
	if got := adapter.requests[1].Messages[1].Metadata["reasoning_content"]; got != "provider continuation state" {
		t.Fatalf("assistant replay metadata = %#v", adapter.requests[1].Messages[1].Metadata)
	}
}

func TestAPIProviderRejectsEmptyToolResponse(t *testing.T) {
	adapter := &fakeAPIAdapter{responses: []*agent.Response{
		{StopReason: agent.StopReasonEnd},
		{StopReason: agent.StopReasonEnd},
	}}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: &recordingToolExecutor{},
		Tools: []agent.ToolDefinition{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "empty-tools", WorkingDir: t.TempDir(), Prompt: "inspect", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err == nil || err.Error() != "API provider returned neither assistant text nor tool calls after one recovery attempt" {
		t.Fatalf("error = %v", err)
	}
	if result.Outcome != OutcomeFailed || result.FinalText != "" {
		t.Fatalf("result = %+v", result)
	}
	if len(events) != 2 || events[0].Type != EventStarted || events[1].Type != EventFailed || events[1].Text != err.Error() {
		t.Fatalf("events = %#v", events)
	}
	if len(adapter.requests) != 2 || !strings.Contains(adapter.requests[1].System, emptyCompletionRecoveryInstruction) {
		t.Fatalf("recovery requests = %#v", adapter.requests)
	}
}

func TestAPIProviderRecoversReasoningOnlyToolResponse(t *testing.T) {
	adapter := &fakeAPIAdapter{responses: []*agent.Response{
		{StopReason: agent.StopReasonEnd, Metadata: map[string]interface{}{
			"reasoning_content": "private reasoning that must not become the answer",
		}},
		{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "visible answer"}}, StopReason: agent.StopReasonEnd},
	}}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: &recordingToolExecutor{},
		Tools: []agent.ToolDefinition{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "reasoning-only", WorkingDir: t.TempDir(), Prompt: "inspect", Model: "test-model",
		System: "standing context", Sandbox: Sandbox{Mode: "read-only"},
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSucceeded || result.FinalText != "visible answer" {
		t.Fatalf("result = %+v", result)
	}
	if len(adapter.requests) != 2 || !strings.Contains(adapter.requests[1].System, "standing context") ||
		!strings.Contains(adapter.requests[1].System, emptyCompletionRecoveryInstruction) {
		t.Fatalf("recovery requests = %#v", adapter.requests)
	}
	if len(events) != 3 || events[1].Type != EventAssistantDelta || events[1].Text != "visible answer" ||
		events[2].Type != EventCompleted {
		t.Fatalf("events = %#v", events)
	}
}

func TestAPIProviderSynthesizesFinalAnswerWhenToolBudgetIsReached(t *testing.T) {
	toolCall := func(id string) *agent.Response {
		return &agent.Response{Content: []agent.ContentBlock{{
			Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: "read_file",
			ToolInput: json.RawMessage(`{"path":"evidence.txt"}`),
		}}, StopReason: agent.StopReasonToolUse}
	}
	adapter := &fakeAPIAdapter{responses: []*agent.Response{
		toolCall("call-1"), toolCall("call-2"),
		{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "final analysis"}}, StopReason: agent.StopReasonEnd},
	}}
	tools := &recordingToolExecutor{}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: tools, MaxSteps: 2,
		Tools: []agent.ToolDefinition{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "budget", WorkingDir: t.TempDir(), Prompt: "analyze", Model: "test-model",
		System: "standing context", Sandbox: Sandbox{Mode: "read-only"},
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns || result.FinalText != "final analysis" {
		t.Fatalf("result = %+v", result)
	}
	if len(tools.requests) != 2 || len(adapter.requests) != 3 {
		t.Fatalf("tool requests = %d, model requests = %d", len(tools.requests), len(adapter.requests))
	}
	finalRequest := adapter.requests[2]
	if len(finalRequest.Tools) != 1 || finalRequest.ToolChoice != "none" {
		t.Fatalf("final request did not preserve disabled tool schema: %+v", finalRequest)
	}
	if !strings.Contains(finalRequest.System, finalSynthesisInstruction) ||
		!strings.Contains(finalRequest.System, "standing context") {
		t.Fatalf("final synthesis instruction = %q", finalRequest.System)
	}
	if len(events) == 0 || events[len(events)-1].Type != EventInconclusive || events[len(events)-1].Reason != ReasonMaxTurns {
		t.Fatalf("terminal events = %#v", events)
	}
	for _, event := range events {
		if event.Type == EventFailed {
			t.Fatalf("tool budget still failed the run: %#v", events)
		}
	}
}

func TestAPIProviderRejectsUnboundedWorkspaceWrite(t *testing.T) {
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Execute(context.Background(), Request{
		ID: "api-2", WorkingDir: t.TempDir(), Prompt: "write", Model: "test-model",
		Sandbox: Sandbox{Mode: "workspace-write"},
	}, nil)
	if err == nil || err.Error() != "API provider cannot enforce workspace-write without a bounded tool executor" {
		t.Fatalf("error = %v", err)
	}
}

func TestAPIProviderProbeClassifiesAuthentication(t *testing.T) {
	provider, _ := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{err: errors.New("401 unauthorized API key")}})
	readiness := provider.Probe(context.Background(), "")
	if readiness.State != ReadinessNeedsLogin {
		t.Fatalf("readiness = %+v", readiness)
	}
}

func TestAPIProviderStreamsAndCancels(t *testing.T) {
	stream := make(chan agent.StreamEvent, 2)
	stream <- agent.StreamEvent{Type: agent.StreamEventContentDelta, Delta: &agent.StreamDelta{Text: "one"}}
	stream <- agent.StreamEvent{Type: agent.StreamEventContentDelta, Delta: &agent.StreamDelta{Text: " two"}}
	close(stream)
	provider, _ := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{stream: stream}})
	result, err := provider.Execute(context.Background(), Request{
		ID: "api-stream", WorkingDir: t.TempDir(), Prompt: "answer", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if err != nil || result.FinalText != "one two" || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%+v err=%v", result, err)
	}

	cancelledStream := make(chan agent.StreamEvent)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	close(cancelledStream)
	provider, _ = NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{stream: cancelledStream}})
	result, err = provider.Execute(ctx, Request{
		ID: "api-cancel", WorkingDir: t.TempDir(), Prompt: "answer", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if !errors.Is(err, context.Canceled) || result.Outcome != OutcomeCancelled {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestAPIProviderRejectsEmptyStream(t *testing.T) {
	stream := make(chan agent.StreamEvent)
	close(stream)
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: &fakeAPIAdapter{stream: stream}})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "empty-stream", WorkingDir: t.TempDir(), Prompt: "answer", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err == nil || err.Error() != "API provider returned no assistant text" {
		t.Fatalf("error = %v", err)
	}
	if result.Outcome != OutcomeFailed || result.FinalText != "" {
		t.Fatalf("result = %+v", result)
	}
	if len(events) != 2 || events[0].Type != EventStarted || events[1].Type != EventFailed || events[1].Text != err.Error() {
		t.Fatalf("events = %#v", events)
	}
}

func TestAPIProviderWithOpenAICompatibleServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization header = %q", request.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{
			`{"id":"one","model":"test-model","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
			`{"id":"one","model":"test-model","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			`{"id":"one","model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		} {
			fmt.Fprintf(w, "data: %s\n\n", chunk)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	openAI, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
		Name: "compatible", APIKey: "test-key", BaseURL: server.URL, Models: []string{"test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := NewAPIProvider(APIProviderConfig{Adapter: openAI})
	result, err := provider.Execute(context.Background(), Request{
		ID: "compatible-1", WorkingDir: t.TempDir(), Prompt: "hello", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if err != nil || result.FinalText != "hello world" || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}
