package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type nativeWireMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolName  string `json:"tool_name"`
	Thinking  string `json:"thinking"`
	ToolCalls []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tool_calls"`
}

type nativeWireRequest struct {
	Messages []nativeWireMessage `json:"messages"`
	Tools    []json.RawMessage   `json:"tools"`
}

// nativeTemplateServer is a version-pinned (0.32.13) Ollama /api/chat fake that,
// like the Qwen template, answers HTTP 500 "No user query found in messages"
// when no plain user message survives, and also refuses a tool message that
// does not directly follow an assistant turn carrying tool calls.
func nativeTemplateServer(t *testing.T, reply func(n int) map[string]any) (*httptest.Server, *[]nativeWireRequest) {
	t.Helper()
	var mu sync.Mutex
	var seen []nativeWireRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.Write([]byte(`{"version":"0.32.13"}`))
			return
		}
		var req nativeWireRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}
		mu.Lock()
		seen = append(seen, req)
		n := len(seen)
		mu.Unlock()
		valid := false
		for i, m := range req.Messages {
			if m.Role == "user" && strings.TrimSpace(m.Content) != "" && !strings.HasPrefix(m.Content, "<tool_response>") {
				valid = true
			}
			if m.Role == "tool" && (i == 0 || (req.Messages[i-1].Role != "assistant" && req.Messages[i-1].Role != "tool")) {
				valid = false
				break
			}
		}
		if !valid {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":"No user query found in messages."}`)
			return
		}
		json.NewEncoder(w).Encode(reply(n))
	}))
	t.Cleanup(server.Close)
	return server, &seen
}

func nativeCall(thinking string) map[string]any {
	return map[string]any{"done": true, "done_reason": "stop", "prompt_eval_count": 100, "eval_count": 10,
		"message": map[string]any{"thinking": thinking, "tool_calls": []map[string]any{{"function": map[string]any{"name": "grep", "arguments": map[string]any{"pattern": "x"}}}}}}
}

func nativeText(text string) map[string]any {
	return map[string]any{"done": true, "done_reason": "stop", "prompt_eval_count": 100, "eval_count": 10,
		"message": map[string]any{"content": text}}
}

func newNativeProvider(t *testing.T, url string) *agent.OpenAIProvider {
	t.Helper()
	openAI, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{Name: "local", APIKey: "k", BaseURL: url + "/v1", Models: []string{"local"}})
	if err != nil || !openAI.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("native fixture unavailable", err)
	}
	return openAI
}

func TestBoundedNativeWireKeepsQueryAndOrderAcrossToolRounds(t *testing.T) {
	server, seen := nativeTemplateServer(t, func(n int) map[string]any {
		if n <= 3 {
			return nativeCall("PRIVATE-THOUGHT")
		}
		return nativeText("synthesized answer")
	})
	tools := &failingToolExecutor{}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: newNativeProvider(t, server.URL), ToolExecutor: tools, MaxSteps: 3,
		Tools: []agent.ToolDefinition{{Name: "grep", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "native-wire", WorkingDir: t.TempDir(), Prompt: "ORIGINAL-QUERY", Model: "local", TokenBudget: 400000,
		History: []HistoryMessage{{Role: HistoryRoleUser, Content: "earlier"}, {Role: HistoryRoleAssistant, Content: "reply"}},
		Sandbox: Sandbox{Mode: "read-only"},
	}, func(e Event) error { events = append(events, e); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns || tools.calls != 3 || len(*seen) != 4 {
		t.Fatalf("result=%+v tools=%d requests=%d", result, tools.calls, len(*seen))
	}
	for i, req := range *seen {
		query := -1
		for j, m := range req.Messages {
			if m.Role == "user" && m.Content == "ORIGINAL-QUERY" {
				query = j
			}
			// Reasoning may only be echoed in the native thinking field of the
			// assistant turn that produced it, never in content or results.
			if strings.Contains(m.Content, "PRIVATE-THOUGHT") || (m.Thinking != "" && m.Role != "assistant") {
				t.Fatalf("request %d leaked private reasoning: %+v", i, m)
			}
		}
		if query < 2 || req.Messages[query-2].Content != "earlier" {
			t.Fatalf("request %d: user query at %d, not right after history", i, query)
		}
		for j, m := range req.Messages {
			if len(m.ToolCalls) > 0 && (j+1 >= len(req.Messages) || req.Messages[j+1].Role != "tool" || req.Messages[j+1].ToolName != "grep") {
				t.Fatalf("request %d: call at %d not answered in order: %+v", i, j, req.Messages)
			}
		}
	}
	final := (*seen)[3]
	failed := 0
	for _, m := range final.Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "exit status 1") {
			failed++
		}
	}
	if failed != 1 || len(final.Tools) != 0 {
		t.Fatalf("failed results on wire=%d tools=%d", failed, len(final.Tools))
	}
	usage := Event{}
	for _, e := range events {
		if e.Type == EventUsage {
			usage = e
		}
	}
	if usage.InputTokens != 400 || usage.OutputTokens != 40 || !usage.UsageFinal {
		t.Fatalf("durable usage = %+v", usage)
	}
}

// The no-dispatch overflow retry rebuilds the request through
// compactRepeatedResults; the rebuilt wire must still carry the user query and
// call/result order, and must be the only request that reaches the endpoint.
func TestBoundedNativeCompactionRetryKeepsQueryAndOrder(t *testing.T) {
	server, seen := nativeTemplateServer(t, func(int) map[string]any { return nativeText("ok") })
	big := strings.Repeat("A", 2600)
	call := func(id string) agent.Message {
		return agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: "grep", ToolInput: json.RawMessage(`{"pattern":"x"}`)}}, Metadata: map[string]any{"thinking": "PRIVATE-THOUGHT"}}
	}
	messages := []agent.Message{
		agent.NewTextMessage(agent.RoleUser, "ORIGINAL-QUERY"),
		call("c1"), agent.NewToolResultMessage("c1", big, nil),
		call("c2"), agent.NewToolResultMessage("c2", "", fmt.Errorf("grep: exit status 1")),
		call("c3"), agent.NewToolResultMessage("c3", big, nil),
	}
	meter := &boundedAPIAdapter{ProviderAdapter: newNativeProvider(t, server.URL), remaining: 16384, contextLimit: 4096}
	if _, err := meter.Complete(context.Background(), agent.Request{Model: "local", Messages: messages}); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 1 {
		t.Fatalf("requests reaching endpoint = %d, want only the compacted retry", len(*seen))
	}
	wire := (*seen)[0].Messages
	if wire[0].Role != "user" || wire[0].Content != "ORIGINAL-QUERY" {
		t.Fatalf("user query lost or moved: %+v", wire[0])
	}
	order := []string{}
	compacted := false
	for _, m := range wire {
		order = append(order, m.Role)
		if strings.Contains(m.Content, "duplicateOfToolResult") {
			compacted = true
		}
		if strings.Contains(m.Content, "PRIVATE-THOUGHT") || (m.Thinking != "" && m.Role != "assistant") {
			t.Fatalf("private reasoning outside its assistant thinking field: %+v", m)
		}
	}
	if !compacted || strings.Join(order, ",") != "user,assistant,tool,assistant,tool,assistant,tool" {
		t.Fatalf("compacted=%v order=%v", compacted, order)
	}
	if !strings.Contains(wire[4].Content, "exit status 1") {
		t.Fatalf("failed result altered by compaction: %q", wire[4].Content)
	}
}

// A conversation whose user query is gone must not reach /api/chat, even
// through the bounded route, and must not consume token grant.
func TestBoundedNativeRefusesQuerylessConversationBeforeWire(t *testing.T) {
	server, seen := nativeTemplateServer(t, func(int) map[string]any { return nativeText("ok") })
	meter := &boundedAPIAdapter{ProviderAdapter: newNativeProvider(t, server.URL), remaining: 65536}
	provider := &APIProvider{config: APIProviderConfig{Adapter: meter}}
	_, err := provider.completeChecked(context.Background(), agent.Request{Model: "local", Messages: []agent.Message{agent.NewTextMessage(agent.RoleAssistant, "only assistant")}})
	if err == nil || len(*seen) != 0 {
		t.Fatalf("queryless conversation reached the wire: err=%v requests=%d", err, len(*seen))
	}
	if meter.remaining != 65536 || meter.input != 0 {
		t.Fatalf("refused request consumed grant: %+v", meter)
	}
}
