package execution

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestAPIReadinessUsesOnlyMetadata(t *testing.T) {
	for _, tc := range []struct {
		name        string
		code        int
		body, state string
	}{
		{"ready", 200, `{"data":[{"id":"test-model"}]}`, ReadinessReady},
		{"auth", 401, `secret fixture credential`, ReadinessNeedsLogin},
		{"forbidden", 403, `secret fixture credential`, ReadinessNeedsLogin},
		{"missing-model", 200, `{"data":[{"id":"other"}]}`, ReadinessUnhealthy},
		{"server", 503, `secret fixture backend error`, ReadinessUnhealthy},
		{"unsupported", 404, `secret fixture route`, ReadinessUnknown},
		{"method", 405, ``, ReadinessUnknown},
		{"malformed", 200, `secret fixture not-json`, ReadinessUnhealthy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, completions := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
					completions++
					w.WriteHeader(500)
					return
				}
				if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("metadata lost configured authentication")
				}
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			adapter, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{BaseURL: server.URL + "/v1", APIKey: "fixture-key", Models: []string{"test-model"}})
			if err != nil {
				t.Fatal(err)
			}
			provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter})
			if err != nil {
				t.Fatal(err)
			}
			got := provider.Probe(context.Background(), "")
			if got.State != tc.state || got.Check != "model-metadata" || calls != 1 || completions != 0 {
				t.Fatalf("readiness=%+v requests=%d completions=%d", got, calls, completions)
			}
			if strings.Contains(got.Problem, "secret") || strings.Contains(got.Problem, "fixture-key") || strings.Contains(got.Problem, server.URL) {
				t.Fatal("sensitive provider content leaked", got)
			}
		})
	}
}

func TestAPIReadinessUnsupportedAdapterNeverFallsBackToInference(t *testing.T) {
	adapter := &fakeAPIAdapter{}
	provider, _ := NewAPIProvider(APIProviderConfig{Adapter: adapter})
	if !provider.Descriptor().Capabilities.NonInferenceReadiness {
		t.Fatal("safe probe is not negotiated")
	}
	got := provider.Probe(context.Background(), "")
	if got.State != ReadinessUnknown || len(adapter.requests) != 0 {
		t.Fatal("metadata absence triggered inference or false-ready", got)
	}
}

func TestAPIReadinessCancellationAndRedirectNeverInfer(t *testing.T) {
	for _, cancelFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelFirst), func(t *testing.T) {
			followed := 0
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { followed++; w.WriteHeader(500) }))
			defer destination.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, destination.URL, 302) }))
			defer server.Close()
			adapter, _ := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{BaseURL: server.URL, APIKey: "fixture-key", Models: []string{"test-model"}})
			provider, _ := NewAPIProvider(APIProviderConfig{Adapter: adapter})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelFirst {
				cancel()
			}
			got := provider.Probe(ctx, "")
			if got.State != ReadinessUnhealthy || followed != 0 {
				t.Fatal("cancel or redirect boundary bypassed", got, followed)
			}
		})
	}
}
