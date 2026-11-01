package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Exercise the real native serializer, observation and token limiter. The
// endpoint is synthetic; this is not a replay or proof of Solar recovery.
func TestEmptyRecoveryPreservesObservedNativePrefix(t *testing.T) {
	for _, outcome := range []string{"answer", "empty", "stop"} {
		t.Run(outcome, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			var firstMessages []any
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				calls++
				var wire map[string]any
				if json.NewDecoder(r.Body).Decode(&wire) != nil {
					t.Error("invalid native request")
					return
				}
				options := wire["options"].(map[string]any)
				messages := wire["messages"].([]any)
				if wire["truncate"] != false || options["num_ctx"] != float64(65536) || options["num_predict"] != float64(2048) {
					t.Error("native context/output bounds changed")
				}
				if calls == 1 {
					fmt.Fprint(w, `{"done":true,"prompt_eval_count":18000,"eval_count":20,"message":{"content":"earlier useful result"}}`)
					return
				}
				if calls == 2 {
					firstMessages = messages
					fmt.Fprint(w, `{"done":true,"done_reason":"length","prompt_eval_count":33573,"eval_count":2048,"message":{"thinking":"PRIVATE_REASONING","content":""}}`)
					if outcome == "stop" {
						cancel()
					}
					return
				}
				if calls != 3 || outcome == "stop" {
					t.Error("recovery exceeded one attempt or ignored Stop")
				}
				if len(messages) != len(firstMessages)+1 || !reflect.DeepEqual(messages[:len(firstMessages)], firstMessages) {
					t.Error("recovery changed the observed native prefix")
				}
				last := messages[len(messages)-1].(map[string]any)
				if last["role"] != "user" || last["content"] != emptyCompletionRecoveryInstruction {
					t.Error("recovery instruction missing from appended message")
				}
				if outcome == "answer" {
					fmt.Fprint(w, `{"done":true,"prompt_eval_count":33673,"eval_count":20,"message":{"content":"useful answer"}}`)
				} else {
					fmt.Fprint(w, `{"done":true,"done_reason":"length","prompt_eval_count":33673,"eval_count":2048,"message":{"thinking":"PRIVATE_REASONING","content":""}}`)
				}
			}))
			defer server.Close()
			bounded := &boundedAPIAdapter{ProviderAdapter: newNativeProvider(t, server.URL), remaining: 250000, contextLimit: 65536}
			provider := &APIProvider{config: APIProviderConfig{Adapter: bounded}}
			// Seed the real native observation, then grow the conversation past
			// conservative whole-request admission. Native prefix accounting
			// still admits it, until the old recovery edits its system prefix.
			request := agent.Request{Model: "local", MaxTokens: 2048,
				System:   "Retain the original task and authority.",
				Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, strings.Repeat("a", 145000))}}
			if _, err := provider.completeChecked(ctx, request); err != nil {
				t.Fatal("could not establish native context observation", err)
			}
			request.Messages = append(request.Messages, agent.NewTextMessage(agent.RoleAssistant, strings.Repeat("b", 16000)))
			response, err := provider.completeWithEmptyRecovery(ctx, request)
			if outcome == "stop" {
				if err == nil || calls != 2 {
					t.Fatal("Stop did not prevent recovery dispatch")
				}
				return
			}
			if calls != 3 {
				t.Fatalf("observed prefix recovery refused before dispatch: calls=%d err=%v", calls, err)
			}
			if outcome == "answer" {
				if err != nil || response.GetText() != "useful answer" || bounded.input != 85246 || bounded.output != 2088 || bounded.remaining != 250000-85246-2088 {
					t.Fatalf("useful recovery or native accounting lost: response=%v err=%v limiter=%+v", response, err, bounded)
				}
			} else if err == nil || !strings.Contains(err.Error(), "after one recovery attempt") || strings.Contains(err.Error(), "PRIVATE_REASONING") {
				t.Fatal("second empty response was accepted or private reasoning leaked", err)
			}
		})
	}
}
