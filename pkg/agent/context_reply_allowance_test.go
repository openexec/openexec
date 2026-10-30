package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Bounded planning changes num_predict to leave room for the full prompt.
// Reply capacity does not change the tokenization of an unchanged prefix.
func TestNativeObservationSurvivesReplyAllowanceChange(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var wire map[string]any
		json.NewDecoder(r.Body).Decode(&wire)
		opts := wire["options"].(map[string]any)
		want := 2048
		if calls == 2 {
			want = 6000
		}
		if opts["num_predict"] != float64(want) || opts["num_ctx"] != float64(32768) || wire["truncate"] != false {
			t.Errorf("native limits changed: %v", wire["options"])
		}
		w.Write([]byte(`{"done":true,"prompt_eval_count":18000,"eval_count":500,"message":{"content":"ok"}}`))
	}))
	defer srv.Close()
	p := &OpenAIProvider{ollamaBudgetURL: srv.URL, httpClient: srv.Client()}
	r := Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("a", 68000))}}
	if _, err := p.CompleteBounded(context.Background(), r, 32768, 2048); err != nil {
		t.Fatal(err)
	}
	r.Messages = append(r.Messages, NewTextMessage(RoleAssistant, strings.Repeat("b", 6000)))
	res, err := p.CompleteBounded(context.Background(), r, 32768, 6000)
	if err != nil || calls != 2 || res == nil || res.Usage.PromptTokens != 18000 || res.Usage.CompletionTokens != 500 {
		t.Fatalf("reply-only change discarded native count: calls=%d result=%+v err=%v", calls, res, err)
	}
	// A reply that actually cannot fit still fails before dispatch.
	if _, err := p.CompleteBounded(context.Background(), r, 32768, 16000); err == nil || calls != 2 {
		t.Fatalf("oversized reply admitted: calls=%d err=%v", calls, err)
	}
}
