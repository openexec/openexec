package execution

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestContextGrantRefusalOrigin(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			_, _ = w.Write([]byte(`{"version":"0.32.13"}`))
			return
		}
		calls++
		_, _ = w.Write([]byte(`{"done":true,"prompt_eval_count":6000,"eval_count":10,"message":{"content":"done"}}`))
	}))
	defer server.Close()
	a, _ := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{APIKey: "fixture", BaseURL: server.URL + "/v1"})
	if !a.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("native bounds unavailable")
	}
	request := agent.Request{Model: "qwen-27b", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, strings.Repeat("x", 13906)+strings.Repeat("é", 86))}}
	_, baseline := a.CompleteBounded(context.Background(), request, 4096, 2048)
	var overflow *agent.ContextOverflowError
	if !errors.As(baseline, &overflow) || overflow.RequestBytes != 14229 || overflow.EstimatedTokens != 6318 || overflow.Dispatched {
		t.Fatalf("failure-shaped native request: %v", baseline)
	}
	meter := &boundedAPIAdapter{ProviderAdapter: a, remaining: 7001}
	_, err := meter.Complete(context.Background(), request)
	if !errors.Is(err, errHardTokenGrantExhausted) || !strings.Contains(err.Error(), "remaining grant") || calls != 0 || meter.remaining != 7001 {
		t.Fatalf("grant origin lost: %v calls=%d remaining=%d", err, calls, meter.remaining)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000}
	if _, err = meter.Complete(context.Background(), request); err != nil || calls != 1 {
		t.Fatalf("sufficient grant refused: %v calls=%d", err, calls)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000, contextLimit: 4096, providerContext: 131072}
	_, err = meter.Complete(context.Background(), request)
	if !errors.As(err, &overflow) || overflow.ContextTokens != 4096 || overflow.Dispatched || calls != 1 || meter.remaining != 250000 {
		t.Fatalf("explicit ceiling lost: %v calls=%d remaining=%d", err, calls, meter.remaining)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 7001}
	request.Messages = []agent.Message{agent.NewTextMessage(agent.RoleUser, strings.Repeat("x", 200000))}
	_, err = meter.Complete(context.Background(), request)
	if !errors.As(err, &overflow) || errors.Is(err, errHardTokenGrantExhausted) || calls != 1 {
		t.Fatalf("request exceeding even the provider ceiling misclassified: %v", err)
	}
}
