package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func observationBody(messages []map[string]any, model string) []byte {
	b, _ := json.Marshal(map[string]any{"model": model, "messages": messages,
		"tools": []any{}, "options": map[string]int{"num_ctx": 32768, "num_predict": 2048}, "truncate": false})
	return b
}

func TestNativeObservationAdmitsAppendOnlyToolResult(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": strings.Repeat("a", 68000)}}
	first := observationBody(messages, "local")
	more := append(messages, map[string]any{"role": "assistant", "thinking": strings.Repeat("b", 2500)},
		map[string]any{"role": "tool", "content": strings.Repeat("c", 3000)})
	next := observationBody(more, "local")
	if admitAssembledContext(next, 32768, 2048, len(more), 0) == nil {
		t.Fatal("fixture did not reproduce whole-request refusal")
	}
	p := &OpenAIProvider{}
	if p.admitObservedContext(next, 32768, 2048, len(more), 0) == nil {
		t.Fatal("unobserved prefix bypassed preflight")
	}
	p.observeNativeContext(first, 18000)
	if err := p.admitObservedContext(next, 32768, 2048, len(more), 0); err != nil {
		t.Fatal("observed prefix and bounded append refused", err)
	}
	for _, mode := range []string{"model", "prefix", "system", "tools", "options", "large-append"} {
		t.Run(mode, func(t *testing.T) {
			var wire map[string]any
			json.Unmarshal(next, &wire)
			switch mode {
			case "model":
				wire["model"] = "different"
			case "prefix":
				wire["messages"].([]any)[0].(map[string]any)["content"] = strings.Repeat("z", 68000)
			case "system":
				wire["messages"].([]any)[0].(map[string]any)["role"] = "system"
			case "tools":
				wire["tools"] = []any{map[string]any{"name": "different"}}
			case "options":
				wire["options"].(map[string]any)["num_ctx"] = 65536
			case "large-append":
				wire["messages"].([]any)[2].(map[string]any)["content"] = strings.Repeat("x", 40000)
			}
			raw, _ := json.Marshal(wire)
			if p.admitObservedContext(raw, 32768, 2048, len(more), 0) == nil {
				t.Fatal("invalidated or oversized append bypassed admission")
			}
		})
	}
}

func TestNativeObservationComesFromSuccessfulProtocolNotEstimate(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var wire map[string]any
		json.NewDecoder(r.Body).Decode(&wire)
		if wire["truncate"] != false || wire["options"].(map[string]any)["num_ctx"] != float64(32768) {
			t.Error("native bounds changed")
		}
		if calls == 1 {
			w.Write([]byte(`{"done":true,"prompt_eval_count":18000,"eval_count":500,"message":{"content":"ok"}}`))
		} else {
			w.Write([]byte(`{"done":true,"prompt_eval_count":19000,"eval_count":200,"message":{"content":"ok"}}`))
		}
	}))
	defer srv.Close()
	p := &OpenAIProvider{ollamaBudgetURL: srv.URL, httpClient: srv.Client()}
	r := Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("a", 68000))}}
	if _, err := p.CompleteBounded(context.Background(), r, 32768, 2048); err != nil {
		t.Fatal(err)
	}
	r.Messages = append(r.Messages, NewTextMessage(RoleAssistant, strings.Repeat("b", 6000)))
	result, err := p.CompleteBounded(context.Background(), r, 32768, 2048)
	if err != nil || calls != 2 || result.Usage.PromptTokens != 19000 {
		t.Fatalf("second request failed or estimate became usage: calls=%d result=%+v err=%v", calls, result, err)
	}
}
