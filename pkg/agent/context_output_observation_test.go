package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The synthetic native endpoint supplies the same count/byte discrepancy as
// the private Solar trace. No captured source, prompt or reasoning is replayed.
func TestNativeOutputObservationBindsExactReturnedMessage(t *testing.T) {
	for _, mode := range []string{"exact", "thinking", "text", "arguments", "tool-name", "large-result", "zero-output", "native-refusal"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			thinking := strings.Repeat("b", 38000)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var wire map[string]any
				if json.NewDecoder(r.Body).Decode(&wire) != nil {
					t.Error("invalid native request")
					return
				}
				options := wire["options"].(map[string]any)
				wantOutput := float64(2048)
				if calls == 1 {
					wantOutput = 10000
				}
				if wire["truncate"] != false || options["num_ctx"] != float64(65536) || options["num_predict"] != wantOutput {
					t.Error("native context, output or truncation bound changed")
				}
				if calls == 1 {
					output := 8022
					if mode == "zero-output" {
						output = 0
					}
					json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "prompt_eval_count": 35003, "eval_count": output,
						"message": map[string]any{"content": "inspect", "thinking": thinking, "tool_calls": []any{map[string]any{"function": map[string]any{"name": "read_file", "arguments": map[string]any{"path": "source.txt", "line": 1}}}}}})
				} else if mode == "native-refusal" {
					w.WriteHeader(http.StatusBadRequest)
					w.Write([]byte(`{"error":"context length exceeded"}`))
				} else {
					w.Write([]byte(`{"done":true,"prompt_eval_count":44000,"eval_count":20,"message":{"content":"useful continuation"}}`))
				}
			}))
			defer srv.Close()
			p := &OpenAIProvider{ollamaBudgetURL: srv.URL, httpClient: srv.Client()}
			r := Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("a", 125000))}}
			// A real successful protocol reply establishes both native counts.
			first, err := p.CompleteBounded(context.Background(), r, 65536, 10000)
			if err != nil {
				t.Fatal(err)
			}
			assistant := Message{Role: RoleAssistant, Content: first.Content, Metadata: first.Metadata}
			toolOutput := "small unique result"
			switch mode {
			case "thinking":
				assistant.Metadata["thinking"] = thinking + "edited"
			case "text":
				assistant.Content[0].Text += "edited"
			case "arguments":
				assistant.Content[1].ToolInput = json.RawMessage(`{"path":"changed.txt","line":1}`)
			case "tool-name":
				assistant.Content[1].ToolName = "different_tool"
			case "large-result":
				toolOutput = strings.Repeat("c", 40000)
			}
			r.Messages = append(r.Messages, assistant, NewToolResultMessage(first.Content[1].ToolUseID, toolOutput, nil))
			result, err := p.CompleteBounded(context.Background(), r, 65536, 2048)
			if mode == "exact" {
				if err != nil || calls != 2 || result.GetText() != "useful continuation" || result.Usage.PromptTokens != 44000 || result.Usage.CompletionTokens != 20 {
					t.Fatalf("exact native output refused or accounting changed: calls=%d err=%v", calls, err)
				}
				return
			}
			var overflow *ContextOverflowError
			if !errors.As(err, &overflow) {
				t.Fatalf("unbound or oversized output admitted: mode=%s calls=%d err=%v", mode, calls, err)
			}
			if mode == "native-refusal" {
				if calls != 2 || !overflow.Dispatched {
					t.Fatal("native context refusal lost dispatched attribution")
				}
			} else if calls != 1 || overflow.Dispatched {
				t.Fatal("unbound output or oversized unique result reached inference")
			}
		})
	}
}
