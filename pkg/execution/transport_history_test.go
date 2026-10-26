package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type wireMessage struct {
	Role       string `json:"role"`
	Content    string `json:"content"`
	ToolCallID string `json:"tool_call_id"`
	ToolCalls  []struct {
		ID string `json:"id"`
	} `json:"tool_calls"`
	Reasoning string `json:"reasoning_content"`
}

type wireRequest struct {
	Messages   []wireMessage     `json:"messages"`
	Tools      []json.RawMessage `json:"tools"`
	ToolChoice any               `json:"tool_choice"`
}

// templateServer behaves like a strict Qwen-style endpoint: it renders a
// request only when a plain user query survives, otherwise it answers HTTP 500
// "No user query found in messages". reply chooses the model's answer.
func templateServer(t *testing.T, reply func(n int, req wireRequest) string) (*httptest.Server, *[]wireRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []wireRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req wireRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode: %v", err)
		}
		mu.Lock()
		seen = append(seen, req)
		n := len(seen)
		mu.Unlock()
		hasQuery := false
		for _, m := range req.Messages {
			if m.Role == "user" && strings.TrimSpace(m.Content) != "" && !strings.HasPrefix(m.Content, "<tool_response>") {
				hasQuery = true
			}
		}
		if !hasQuery {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":{"message":"No user query found in messages."}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reply(n, req))
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func toolCallReply(id string) string {
	return fmt.Sprintf(`{"id":"x","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","reasoning_content":"PRIVATE-THOUGHT","tool_calls":[{"id":%q,"type":"function","function":{"name":"grep","arguments":"{\"pattern\":\"x\"}"}}]}}]}`, id)
}

func textReply(text string) string {
	b, _ := json.Marshal(text)
	return `{"id":"x","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":` + string(b) + `}}]}`
}

type failingToolExecutor struct {
	mu    sync.Mutex
	calls int
}

func (e *failingToolExecutor) ValidateAccess(string, Sandbox, []string) error { return nil }
func (e *failingToolExecutor) ExecuteTool(context.Context, ToolRequest) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.calls++
	if e.calls%2 == 0 {
		return "", errors.New("grep: exit status 1")
	}
	return "match", nil
}

func newCompatibleProvider(t *testing.T, url string, tools ToolExecutor, steps int) *APIProvider {
	t.Helper()
	openAI, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
		Name: "compatible", APIKey: "k", BaseURL: url, Models: []string{"test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: openAI, ToolExecutor: tools, MaxSteps: steps,
		Tools: []agent.ToolDefinition{{Name: "grep", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func TestMultiToolReplayKeepsUserQueryAndOrderingOnTheWire(t *testing.T) {
	server, seen := templateServer(t, func(n int, _ wireRequest) string {
		if n <= 3 {
			return toolCallReply(fmt.Sprintf("call-%d", n))
		}
		return textReply("synthesized answer")
	})
	tools := &failingToolExecutor{}
	provider := newCompatibleProvider(t, server.URL, tools, 3)
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "wire", WorkingDir: t.TempDir(), Prompt: "ORIGINAL-QUERY", Model: "test-model",
		History: []HistoryMessage{{Role: HistoryRoleUser, Content: "earlier"}, {Role: HistoryRoleAssistant, Content: "reply"}},
		Sandbox: Sandbox{Mode: "read-only"},
	}, func(e Event) error { events = append(events, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns || result.FinalText != "synthesized answer" {
		t.Fatalf("result = %+v", result)
	}
	if tools.calls != 3 || len(*seen) != 4 {
		t.Fatalf("tool calls=%d requests=%d", tools.calls, len(*seen))
	}
	for i, req := range *seen {
		query := -1
		for j, m := range req.Messages {
			if m.Role == "user" && m.Content == "ORIGINAL-QUERY" {
				query = j
			}
			if m.Reasoning != "" || strings.Contains(m.Content, "PRIVATE-THOUGHT") {
				t.Fatalf("request %d leaked private reasoning: %+v", i, m)
			}
		}
		if query < 0 || req.Messages[query-2].Content != "earlier" {
			t.Fatalf("request %d: user query at %d, not right after history", i, query)
		}
		// Each assistant tool call is answered by the matching tool message
		// immediately after it.
		for j, m := range req.Messages {
			if len(m.ToolCalls) == 0 {
				continue
			}
			if j+1 >= len(req.Messages) || req.Messages[j+1].Role != "tool" || req.Messages[j+1].ToolCallID != m.ToolCalls[0].ID {
				t.Fatalf("request %d: tool call at %d not answered in order: %+v", i, j, req.Messages)
			}
		}
	}
	final := (*seen)[3]
	if len(final.Tools) != 1 || final.ToolChoice != "none" {
		t.Fatalf("final request tools=%d choice=%v", len(final.Tools), final.ToolChoice)
	}
	failed := 0
	for _, m := range final.Messages {
		if m.Role == "tool" && strings.HasPrefix(m.Content, "Error: ") {
			failed++
		}
	}
	if failed != 1 {
		t.Fatalf("failed tool results on wire = %d, want 1", failed)
	}
	terminals := 0
	for _, e := range events {
		if e.Type == EventInconclusive || e.Type == EventCompleted || e.Type == EventFailed {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal events = %d", terminals)
	}
}

func TestTransportGuardRefusesBeforeAnyRequestWhenNoPlainQuery(t *testing.T) {
	// Removal assertion: without completeChecked the strict endpoint would be
	// reached (and answer 500); the guard must stop the request locally.
	server, seen := templateServer(t, func(int, wireRequest) string { return textReply("never") })
	provider := newCompatibleProvider(t, server.URL, &failingToolExecutor{}, 2)
	_, err := provider.Execute(context.Background(), Request{
		ID: "noquery", WorkingDir: t.TempDir(), Prompt: "<tool_response>\nresult\n</tool_response>", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "no plain user query") {
		t.Fatalf("err = %v", err)
	}
	if len(*seen) != 0 {
		t.Fatalf("endpoint was reached %d times", len(*seen))
	}
}

func TestValidateTransportConversation(t *testing.T) {
	user := func(s string) agent.Message { return agent.NewTextMessage(agent.RoleUser, s) }
	call := func(id string) agent.Message {
		return agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: "grep", ToolInput: json.RawMessage(`{}`)}}}
	}
	result := func(id string, err error) agent.Message { return agent.NewToolResultMessage(id, "out", err) }
	cases := []struct {
		name string
		msgs []agent.Message
		want string
	}{
		{"valid with failed result", []agent.Message{user("q"), call("a"), result("a", errors.New("exit 1")), call("b"), result("b", nil)}, ""},
		{"valid assistant-first history", []agent.Message{agent.NewTextMessage(agent.RoleAssistant, "hi"), user("q")}, ""},
		{"user text mentioning tool_response is still a query", []agent.Message{user("explain <tool_response> tags")}, ""},
		{"empty", nil, "empty"},
		{"empty user text", []agent.Message{user("  ")}, "no plain user query"},
		{"wrapped tool response only", []agent.Message{user("<tool_response>x</tool_response>")}, "no plain user query"},
		{"only tool turns", []agent.Message{call("a"), result("a", nil)}, "no plain user query"},
		{"foreign role", []agent.Message{agent.NewTextMessage(agent.RoleSystem, "s"), user("q")}, "foreign role"},
		{"orphan result", []agent.Message{user("q"), result("zzz", nil)}, "no matching call"},
		{"unanswered then user", []agent.Message{user("q"), call("a"), user("again")}, "unanswered"},
		{"unanswered at end", []agent.Message{user("q"), call("a")}, "ends with an unanswered"},
	}
	for _, tc := range cases {
		err := validateTransportConversation(tc.msgs)
		if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestEndpointIgnoringDisabledToolsEndsInconclusiveWithoutExtraTools(t *testing.T) {
	for name, reply := range map[string]func(int) string{
		"native tool call": func(n int) string { return toolCallReply(fmt.Sprintf("c%d", n)) },
		"serialized tool syntax": func(n int) string {
			if n <= 2 {
				return toolCallReply(fmt.Sprintf("c%d", n))
			}
			return textReply("<tool_call>\n<function=grep>\n</function>\n</tool_call>")
		},
		"reasoning only": func(n int) string {
			if n <= 2 {
				return toolCallReply(fmt.Sprintf("c%d", n))
			}
			return `{"id":"x","model":"test-model","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","reasoning_content":"PRIVATE-THOUGHT"}}]}`
		},
		"empty": func(n int) string {
			if n <= 2 {
				return toolCallReply(fmt.Sprintf("c%d", n))
			}
			return textReply("")
		},
	} {
		t.Run(name, func(t *testing.T) {
			server, seen := templateServer(t, func(n int, _ wireRequest) string { return reply(n) })
			tools := &failingToolExecutor{}
			provider := newCompatibleProvider(t, server.URL, tools, 2)
			var events []Event
			result, err := provider.Execute(context.Background(), Request{
				ID: "cap", WorkingDir: t.TempDir(), Prompt: "q", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"},
			}, func(e Event) error { events = append(events, e); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if tools.calls != 2 {
				t.Fatalf("tools executed = %d, want exactly the cap of 2 (requests=%d)", tools.calls, len(*seen))
			}
			if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
				t.Fatalf("result = %+v", result)
			}
			if !strings.Contains(result.FinalText, "final synthesis failed") || strings.Contains(result.FinalText, "PRIVATE-THOUGHT") {
				t.Fatalf("final text = %q", result.FinalText)
			}
			terminals := 0
			for _, e := range events {
				switch e.Type {
				case EventInconclusive:
					terminals++
				case EventCompleted, EventFailed:
					t.Fatalf("incomplete work reported as %s", e.Type)
				}
			}
			if terminals != 1 {
				t.Fatalf("terminal events = %d", terminals)
			}
		})
	}
}

func TestEndpointErrorAndCancellationAreTruthful(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"boom"}}`)
	}))
	defer server.Close()
	provider := newCompatibleProvider(t, server.URL, &failingToolExecutor{}, 2)
	var events []Event
	_, err := provider.Execute(context.Background(), Request{
		ID: "err", WorkingDir: t.TempDir(), Prompt: "q", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"},
	}, func(e Event) error { events = append(events, e); return nil })
	if err == nil || events[len(events)-1].Type != EventFailed {
		t.Fatalf("err=%v events=%v", err, events)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	events = nil
	result, err := provider.Execute(ctx, Request{
		ID: "cancel", WorkingDir: t.TempDir(), Prompt: "q", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"},
	}, func(e Event) error { events = append(events, e); return nil })
	if !errors.Is(err, context.Canceled) || result.Outcome != OutcomeCancelled || events[len(events)-1].Type != EventCancelled {
		t.Fatalf("result=%+v err=%v events=%v", result, err, events)
	}
}

func TestNormalToolRunStillSucceeds(t *testing.T) {
	server, _ := templateServer(t, func(n int, _ wireRequest) string {
		if n == 1 {
			return toolCallReply("c1")
		}
		return textReply("done")
	})
	tools := &failingToolExecutor{}
	provider := newCompatibleProvider(t, server.URL, tools, 4)
	result, err := provider.Execute(context.Background(), Request{
		ID: "ok", WorkingDir: t.TempDir(), Prompt: "q", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if err != nil || result.Outcome != OutcomeSucceeded || result.FinalText != "done" || tools.calls != 1 {
		t.Fatalf("result=%+v err=%v calls=%d", result, err, tools.calls)
	}
}
