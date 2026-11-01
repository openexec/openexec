package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestObservedNativeOutputContinuesWithinOriginalGrant(t *testing.T) {
	for _, mode := range []string{"continue", "spent", "stop"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				calls++
				var wire map[string]any
				json.NewDecoder(r.Body).Decode(&wire)
				options := wire["options"].(map[string]any)
				if wire["truncate"] != false || options["num_ctx"] != float64(65536) {
					t.Error("native context or truncation changed")
				}
				if calls == 1 {
					json.NewEncoder(w).Encode(map[string]any{"done": true, "prompt_eval_count": 35003, "eval_count": 8022,
						"message": map[string]any{"content": "inspect", "thinking": strings.Repeat("b", 38000), "tool_calls": []any{map[string]any{"function": map[string]any{"name": "read_file", "arguments": map[string]any{"path": "source.txt"}}}}}})
				} else {
					fmt.Fprint(w, `{"done":true,"prompt_eval_count":44000,"eval_count":20,"message":{"content":"continued original work"}}`)
				}
			}))
			defer srv.Close()
			bounded := &boundedAPIAdapter{ProviderAdapter: newNativeProvider(t, srv.URL), remaining: 250000, contextLimit: 65536}
			request := agent.Request{Model: "local", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, strings.Repeat("a", 125000))}}
			first, err := bounded.Complete(ctx, request)
			if err != nil {
				t.Fatal("could not establish native output observation", err)
			}
			request.Messages = append(request.Messages,
				agent.Message{Role: agent.RoleAssistant, Content: first.Content, Metadata: first.Metadata},
				agent.NewToolResultMessage(first.GetToolCalls()[0].ToolUseID, "small unique result", nil))
			if mode == "spent" {
				bounded.remaining = 4095
			} else if mode == "stop" {
				cancel()
			}
			result, err := bounded.Complete(ctx, request)
			if mode != "continue" {
				if err == nil || calls != 1 {
					t.Fatal("observed output bypassed Stop or original grant")
				}
				return
			}
			if err != nil || calls != 2 || result.GetText() != "continued original work" {
				t.Fatalf("native output could not continue original work: calls=%d err=%v", calls, err)
			}
			if bounded.input != 79003 || bounded.output != 8042 || bounded.remaining != 250000-87045 || bounded.unknown {
				t.Fatalf("native output estimate became usage or changed original grant: %+v", bounded)
			}
		})
	}
}
