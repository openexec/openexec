package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNativeNonThinkingWireAndModelRefusal(t *testing.T) {
	for _, mode := range []string{"default", "off", "gptoss", "unknown", "missing-switch"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/version":
					w.Write([]byte(`{"version":"0.32.13"}`))
					return
				case "/api/show":
					family, template := "qwen35", "enable_thinking is defined and enable_thinking is false"
					if mode == "gptoss" || mode == "unknown" {
						family = mode
					}
					if mode == "missing-switch" {
						template = ""
					}
					json.NewEncoder(w).Encode(map[string]any{"details": map[string]string{"family": family}, "capabilities": []string{"thinking"}, "template": template})
					return
				case "/api/chat":
					calls++
					var body map[string]any
					json.NewDecoder(r.Body).Decode(&body)
					value, exists := body["think"]
					if mode == "default" && exists {
						t.Error("default changed")
					}
					if mode == "off" && (!exists || value != false) {
						t.Error("native false missing or malformed")
					}
					options := body["options"].(map[string]any)
					if options["num_ctx"] != float64(4096) || options["num_predict"] != float64(512) {
						t.Error("caps changed")
					}
					w.Write([]byte(`{"done":true,"prompt_eval_count":10,"eval_count":3,"message":{"content":"ok"}}`))
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{BaseURL: server.URL + "/v1", APIKey: "local"})
			if !p.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("negotiation failed")
			}
			_, err := p.CompleteBounded(context.Background(), Request{Model: "arbitrary-alias", NonThinking: mode != "default"}, 4096, 512)
			if mode == "default" || mode == "off" {
				if err != nil || calls != 1 {
					t.Fatal(err, calls)
				}
			} else if err == nil || calls != 0 {
				t.Fatal("unsupported reached inference", err, calls)
			}
		})
	}
}
