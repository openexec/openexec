package execution

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestNativeEmptyEvidenceSurvivesBudgetRefusal(t *testing.T) {
	for _, reason := range []string{"length", "unexpected private provider data"} {
		t.Run(reason, func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					w.Write([]byte(`{"version":"0.32.13"}`))
					return
				}
				calls++
				var wire map[string]any
				json.NewDecoder(r.Body).Decode(&wire)
				options := wire["options"].(map[string]any)
				if wire["truncate"] != false || options["num_ctx"] != float64(32768) || options["num_predict"] != float64(2048) {
					t.Error("native admission bounds changed")
				}
				json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": reason,
					"prompt_eval_count": 32000, "eval_count": 2048,
					"message": map[string]any{"thinking": "PRIVATE_REASONING_DO_NOT_EMIT", "content": ""}})
			}))
			defer srv.Close()
			a, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{APIKey: "local-placeholder", BaseURL: srv.URL + "/v1"})
			if err != nil || !a.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("native fixture unavailable", err)
			}
			bounded := &boundedAPIAdapter{ProviderAdapter: a, remaining: 65536}
			p := &APIProvider{config: APIProviderConfig{Adapter: bounded}}
			_, err = p.completeWithEmptyRecovery(context.Background(), agent.Request{Model: "local", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "perform assigned work")}})
			if !errors.Is(err, errHardTokenGrantExhausted) || calls != 1 || bounded.input != 32000 || bounded.output != 2048 || bounded.remaining != 31488 {
				t.Fatalf("admission/accounting changed: calls=%d limiter=%+v err=%v", calls, bounded, err)
			}
			want := reason
			if reason != "length" {
				want = "unknown"
			}
			for _, fact := range []string{`"reason":"` + want + `"`, `"input":32000`, `"output":2048`, `"outputCap":2048`, `"hasText":false`, `"hasReasoning":true`, `"toolCalls":0`} {
				if !strings.Contains(err.Error(), fact) {
					t.Fatalf("missing native fact %s: %v", fact, err)
				}
			}
			if strings.Contains(err.Error(), "PRIVATE_REASONING") || strings.Contains(err.Error(), "private provider data") {
				t.Fatal("private content escaped into diagnostics")
			}
		})
	}
}

func TestNativeThinkingOnlyIsNotMisreportedAsNoReasoning(t *testing.T) {
	a := &fakeAPIAdapter{responses: []*agent.Response{
		{Metadata: map[string]any{"thinking": "PRIVATE_REASONING"}},
		{Metadata: map[string]any{"thinking": "PRIVATE_REASONING"}},
	}}
	p := &APIProvider{config: APIProviderConfig{Adapter: a}}
	_, err := p.completeWithEmptyRecovery(context.Background(), agent.Request{})
	if err == nil || !strings.Contains(err.Error(), "returned reasoning but neither") || strings.Contains(err.Error(), "PRIVATE_REASONING") || len(a.requests) != 2 {
		t.Fatalf("native reasoning misclassified or leaked: %v", err)
	}
}
