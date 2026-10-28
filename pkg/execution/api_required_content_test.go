package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Hosted endpoints can require content even for an assistant message that
// contains only tool calls. Exercise the real adapter and execution loop: the
// omission is only visible on the second HTTP request, after a tool completes.
func TestAPIProviderRequiredContentAfterToolCall(t *testing.T) {
	for _, prose := range []string{"", "Inspecting the directory."} {
		t.Run(fmt.Sprintf("assistant=%q", prose), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var request struct {
					Messages []map[string]json.RawMessage `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				for i, message := range request.Messages {
					content, exists := message["content"]
					var text string
					if !exists || string(content) == "null" || json.Unmarshal(content, &text) != nil {
						w.WriteHeader(http.StatusBadRequest)
						_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
							"type": "invalid_request_error", "message": fmt.Sprintf("Missing required parameter: 'messages[%d].content'.", i),
						}})
						return
					}
				}
				if calls == 1 {
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
						"finish_reason": "tool_calls", "message": map[string]any{
							"role": "assistant", "content": prose, "tool_calls": []any{map[string]any{
								"id": "call-list", "type": "function", "function": map[string]any{
									"name": "list_directory", "arguments": `{"path":"."}`,
								},
							}},
						},
					}}})
					return
				}
				if calls != 2 || len(request.Messages) != 4 {
					t.Errorf("continuation: calls=%d messages=%d", calls, len(request.Messages))
				} else {
					assistant, tool := request.Messages[2], request.Messages[3]
					if string(assistant["role"]) != `"assistant"` || string(assistant["content"]) != fmt.Sprintf("%q", prose) || len(assistant["tool_calls"]) == 0 {
						t.Errorf("assistant history changed: %s", assistant)
					}
					if string(tool["role"]) != `"tool"` || string(tool["tool_call_id"]) != `"call-list"` || string(tool["content"]) != `"tool output"` {
						t.Errorf("tool history changed: %s", tool)
					}
				}
				_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"Directory inspected."}}]}`))
			}))
			defer server.Close()
			adapter, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
				APIKey: "fixture-key", BaseURL: server.URL, Models: []string{"fixture-model"},
			})
			if err != nil {
				t.Fatal(err)
			}
			tools := &recordingToolExecutor{}
			provider, err := NewAPIProvider(APIProviderConfig{
				Adapter: adapter, ToolExecutor: tools,
				Tools: []agent.ToolDefinition{{Name: "list_directory", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Execute(context.Background(), Request{
				ID: "required-content", WorkingDir: t.TempDir(), System: "Inspect files read-only.",
				Prompt: "List this directory.", Model: "fixture-model", Sandbox: Sandbox{Mode: "read-only"},
			}, nil)
			if err != nil {
				t.Fatalf("tool continuation rejected: %v", err)
			}
			if calls != 2 || len(tools.requests) != 1 || result.Outcome != OutcomeSucceeded || result.FinalText != prose+"Directory inspected." {
				t.Fatalf("calls=%d tools=%+v result=%+v", calls, tools.requests, result)
			}
		})
	}
}
