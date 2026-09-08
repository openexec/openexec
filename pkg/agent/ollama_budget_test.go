package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaBudgetNegotiatesAndSendsNativeHardBounds(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.Write([]byte(`{"version":"0.32.13"}`))
			return
		}
		if r.URL.Path != "/api/chat" {
			t.Errorf("wrong native path %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		calls++
		var body struct {
			Stream   bool           `json:"stream"`
			Truncate *bool          `json:"truncate"`
			Options  map[string]int `json:"options"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Options["num_ctx"] != 4096 || body.Options["num_predict"] != 512 || body.Truncate == nil || *body.Truncate || body.Stream {
			t.Errorf("unbounded native request %+v", body)
		}
		w.Write([]byte(`{"done":true,"prompt_eval_count":101,"eval_count":7,"message":{"content":"ok"}}`))
	}))
	defer server.Close()
	p, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if p.SupportsHardTokenBudget() {
		t.Fatal("unnegotiated endpoint claimed admission")
	}
	if !p.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("supported endpoint not negotiated")
	}
	r, err := p.CompleteBounded(context.Background(), Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, "hello")}}, 4096, 512)
	if err != nil || r.Usage.TotalTokens != 108 || calls != 1 {
		t.Fatal(r, err, calls)
	}
	if _, err = p.CompleteBounded(context.Background(), Request{}, 0, 512); err == nil || calls != 1 {
		t.Fatal("invalid bound reached inference")
	}
}
func TestOllamaBudgetRefusesUnknownProtocolAndMalformedUsage(t *testing.T) {
	for _, mode := range []string{"unknown", "missing", "overrun", "negative", "incomplete"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					v := "0.32.13"
					if mode == "unknown" {
						v = "99.0.0"
					}
					json.NewEncoder(w).Encode(map[string]string{"version": v})
					return
				}
				value := map[string]any{"done": true, "prompt_eval_count": 10, "eval_count": 3, "message": map[string]any{"content": "ok"}}
				switch mode {
				case "missing":
					delete(value, "prompt_eval_count")
				case "overrun":
					value["eval_count"] = 1000
				case "negative":
					value["eval_count"] = -1
				case "incomplete":
					value["done"] = false
				}
				json.NewEncoder(w).Encode(value)
			}))
			defer server.Close()
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", BaseURL: server.URL + "/v1"})
			yes := p.EnableLocalOllamaBounds(context.Background())
			if mode == "unknown" {
				if yes {
					t.Fatal("unknown protocol admitted")
				}
				return
			}
			if _, err := p.CompleteBounded(context.Background(), Request{Model: "local"}, 4096, 512); err == nil {
				t.Fatal("invalid accounting released reservation")
			}
		})
	}
}

func TestOllamaBudgetRejectsContextBelowNativeFloor(t *testing.T) {
	p := &OpenAIProvider{ollamaBudgetURL: "http://127.0.0.1:1/api/chat"}
	_, err := p.CompleteBounded(context.Background(), Request{}, 1536, 512)
	if err == nil || err.Error() != "hard token admission unavailable" {
		t.Fatalf("below-floor request reached transport: %v", err)
	}
}
