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

func TestContextGrantRefusalOrigin(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			_, _ = w.Write([]byte(`{"version":"0.32.13"}`))
			return
		}
		if r.URL.Path == "/api/show" {
			_, _ = w.Write([]byte(`{"model_info":{"qwen35.context_length":131072}}`))
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
	// The next power of two does not fit the grant, but the actual prompt and
	// output do. The original power-of-two-only selection refused this request.
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 11000, providerContext: 131072}
	if _, err = meter.Complete(context.Background(), request); err != nil || calls != 1 || meter.contextCap <= 8192 || meter.contextCap+2048 > 11000 {
		t.Fatalf("usable grant stranded: %v calls=%d context=%d", err, calls, meter.contextCap)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000}
	if _, err = meter.Complete(context.Background(), request); err != nil || calls != 2 {
		t.Fatalf("sufficient grant refused: %v calls=%d", err, calls)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000, contextLimit: 4096, providerContext: 131072}
	_, err = meter.Complete(context.Background(), request)
	if !errors.As(err, &overflow) || overflow.ContextTokens != 4096 || overflow.Dispatched || calls != 2 || meter.remaining != 250000 {
		t.Fatalf("explicit ceiling lost: %v calls=%d remaining=%d", err, calls, meter.remaining)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000, providerContext: 4096}
	_, err = meter.Complete(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "context_misconfiguration: provider context_tokens=4096") || !strings.Contains(err.Error(), "window=131072") || calls != 2 || meter.remaining != 250000 {
		t.Fatalf("provider configuration origin lost: %v calls=%d remaining=%d", err, calls, meter.remaining)
	}
	p, _ := NewAPIProvider(APIProviderConfig{Adapter: a, Context: (ContextPolicy{ContextTokens: 4096}).WithDefaults(true)})
	_, err = p.Execute(context.Background(), Request{ID: "fixture", Model: "qwen-27b", Prompt: strings.Repeat("x", 14000), WorkingDir: t.TempDir(), Sandbox: Sandbox{Mode: SandboxReadOnly}, TokenBudget: 250000}, nil)
	if err == nil || !strings.Contains(err.Error(), "context_misconfiguration: provider context_tokens=4096") {
		t.Fatalf("Execute lost provider configuration diagnosis: %v", err)
	}
	meter = &boundedAPIAdapter{ProviderAdapter: a, remaining: 7001}
	request.Messages = []agent.Message{agent.NewTextMessage(agent.RoleUser, strings.Repeat("x", 200000))}
	_, err = meter.Complete(context.Background(), request)
	if !errors.As(err, &overflow) || errors.Is(err, errHardTokenGrantExhausted) || calls != 2 {
		t.Fatalf("request exceeding even the provider ceiling misclassified: %v", err)
	}
}

func TestContextMetadataAndDispatchedRefusal(t *testing.T) {
	modelInfo := `{"model_info":{"qwen35.context_length":131072}}`
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.32.13"}`))
		case "/api/show":
			_ = json.NewDecoder(r.Body).Decode(&struct {
				Model *string `json:"model"`
			}{Model: &gotModel})
			_, _ = w.Write([]byte(modelInfo))
		case "/api/chat":
			_ = json.NewDecoder(r.Body).Decode(&struct {
				Model *string `json:"model"`
			}{Model: &gotModel})
			http.Error(w, "context exceeds capacity", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	a, _ := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{APIKey: "fixture", BaseURL: server.URL + "/v1"})
	if !a.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("native bounds unavailable")
	}
	if window, err := a.NativeContextWindow(context.Background(), "qwen-27b"); err != nil || window != 131072 || gotModel != "qwen-27b" {
		t.Fatalf("metadata: %d %s %v", window, gotModel, err)
	}
	for _, invalid := range []string{`{"model_info":{"context_length":131072}}`, `{"model_info":{"a.context_length":131072,"b.context_length":4096}}`, `{"model_info":{"a.context_length":-1}}`} {
		modelInfo = invalid
		if _, err := a.NativeContextWindow(context.Background(), "qwen-27b"); err == nil {
			t.Fatalf("accepted metadata %s", invalid)
		}
	}
	meter := &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000, providerContext: 131072}
	_, err := meter.Complete(context.Background(), agent.Request{Model: "qwen-27b", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "hello")}})
	var overflow *agent.ContextOverflowError
	if !errors.As(err, &overflow) || !overflow.Dispatched || !meter.unknown || meter.remaining >= 250000 || gotModel != "qwen-27b" {
		t.Fatalf("dispatched accounting: %v %+v model=%s", err, meter, gotModel)
	}
}
