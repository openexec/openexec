package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubCopilotProvider(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-must-not-leak")
	t.Setenv(GitHubCopilotAPIKeyEnv, "")

	// Near miss: an OpenAI key alone must not satisfy Copilot.
	if _, err := NewGitHubCopilotProvider(OpenAIProviderConfig{}); err == nil {
		t.Fatal("expected auth error without GITHUB_COPILOT_API_KEY")
	}
	if CheckEnvVars()[ProviderGitHubCopilot] {
		t.Error("copilot must not be configured by OPENAI_API_KEY")
	}

	t.Setenv(GitHubCopilotAPIKeyEnv, "ghu_copilot")
	if !CheckEnvVars()[ProviderGitHubCopilot] {
		t.Error("copilot should be configured via env")
	}
	if got := RequiredEnvVars()[ProviderGitHubCopilot]; len(got) != 1 || got[0] != GitHubCopilotAPIKeyEnv {
		t.Errorf("unexpected env vars %v", got)
	}

	reg := NewProviderRegistry()
	RegisterDefaultFactories(reg)
	p, err := reg.Get(ProviderGitHubCopilot)
	if err != nil {
		t.Fatal(err)
	}
	if p.GetName() != "githubcopilot" {
		t.Errorf("name = %s", p.GetName())
	}
	info, err := p.GetModelInfo("gpt-4.1")
	if err != nil || info.Provider != "githubcopilot" {
		t.Errorf("model info = %+v, %v", info, err)
	}
	if got, err := reg.GetForModel("claude-sonnet-4"); err != nil || got.GetName() != "githubcopilot" {
		t.Errorf("GetForModel: %v %v", got, err)
	}
}

func TestGitHubCopilotSendsCopilotTokenToCopilotEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-must-not-leak")
	var auth, path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()

	p, err := NewGitHubCopilotProvider(OpenAIProviderConfig{APIKey: "ghu_copilot", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Complete(context.Background(), Request{
		Model:     "gpt-4.1",
		MaxTokens: 16,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{{Type: ContentTypeText, Text: "hello"}}}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if auth != "Bearer ghu_copilot" {
		t.Errorf("Authorization = %q", auth)
	}
	if path != "/chat/completions" {
		t.Errorf("path = %q", path)
	}
}
