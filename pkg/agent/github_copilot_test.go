package agent

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func copilotChat(t *testing.T, p *OpenAIProvider) error {
	t.Helper()
	_, err := p.Complete(context.Background(), Request{
		Model:     "gpt-4.1",
		MaxTokens: 16,
		Messages:  []Message{{Role: RoleUser, Content: []ContentBlock{{Type: ContentTypeText, Text: "hello"}}}},
	})
	return err
}

// The chat endpoint must receive the exchanged session token (never the OAuth
// token) plus the client headers; the session token is cached across requests
// and re-exchanged once it nears expiry.
func TestGitHubCopilotExchangesOAuthTokenForSessionToken(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "sk-openai-must-not-leak")
	var exchanges int
	var exchangeAuth, chatAuth, chatPath, editor, integration string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			exchanges++
			exchangeAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"token":"tid=session%d","expires_at":%d}`, exchanges, time.Now().Add(30*time.Minute).Unix())))
			return
		}
		chatAuth, chatPath = r.Header.Get("Authorization"), r.URL.Path
		editor, integration = r.Header.Get("Editor-Version"), r.Header.Get("Copilot-Integration-Id")
		_, _ = w.Write([]byte(`{"id":"1","model":"gpt-4.1","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer srv.Close()
	old := githubCopilotTokenURL
	githubCopilotTokenURL = srv.URL + "/token"
	defer func() { githubCopilotTokenURL = old }()

	p, err := NewGitHubCopilotProvider(OpenAIProviderConfig{APIKey: "gho_oauth", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := copilotChat(t, p); err != nil {
			t.Fatalf("Complete: %v", err)
		}
	}
	if exchangeAuth != "token gho_oauth" {
		t.Errorf("exchange Authorization = %q", exchangeAuth)
	}
	if chatAuth != "Bearer tid=session1" {
		t.Errorf("chat Authorization = %q (OAuth token must not be sent to the chat API)", chatAuth)
	}
	if chatPath != "/chat/completions" || editor == "" || integration == "" {
		t.Errorf("path=%q editor=%q integration=%q", chatPath, editor, integration)
	}
	if exchanges != 1 {
		t.Errorf("exchanges = %d, want 1 (session token cached)", exchanges)
	}

	// Near expiry the token is exchanged again.
	tr := p.httpClient.Transport.(*copilotTransport)
	tr.now = func() time.Time { return time.Now().Add(29*time.Minute + 30*time.Second) }
	if err := copilotChat(t, p); err != nil {
		t.Fatal(err)
	}
	if exchanges != 2 || chatAuth != "Bearer tid=session2" {
		t.Errorf("exchanges=%d chatAuth=%q, want refresh to session2", exchanges, chatAuth)
	}
}

// A refused exchange (no subscription, wrong token type) fails the request with
// an explanation and never reaches the chat endpoint.
func TestGitHubCopilotRefusedExchangeFailsClosed(t *testing.T) {
	chatCalled := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"no copilot access"}`))
			return
		}
		chatCalled = true
	}))
	defer srv.Close()
	old := githubCopilotTokenURL
	githubCopilotTokenURL = srv.URL + "/token"
	defer func() { githubCopilotTokenURL = old }()

	p, err := NewGitHubCopilotProvider(OpenAIProviderConfig{APIKey: "ghp_classic", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	err = copilotChat(t, p)
	if err == nil || !strings.Contains(err.Error(), "token exchange") {
		t.Fatalf("expected token exchange failure, got %v", err)
	}
	if chatCalled {
		t.Error("chat endpoint must not be called without a session token")
	}
}

// A provider entry that names only the Copilot URL (the config-driven routes
// build the generic adapter directly) gets the exchange; other URLs do not.
func TestGenericAdapterWrapsOnlyCopilotURL(t *testing.T) {
	cop, err := NewOpenAIProvider(OpenAIProviderConfig{Name: "mine", APIKey: "gho_x", BaseURL: "https://api.githubcopilot.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cop.httpClient.Transport.(*copilotTransport); !ok {
		t.Error("copilot URL must use the session-token exchange")
	}
	for _, u := range []string{"https://api.openai.com/v1", "http://localhost:8080/v1", "https://evil.example/api.githubcopilot.com"} {
		p, err := NewOpenAIProvider(OpenAIProviderConfig{Name: "other", APIKey: "k", BaseURL: u})
		if err != nil {
			t.Fatal(err)
		}
		if p.httpClient.Transport != nil {
			t.Errorf("%s must not be wrapped", u)
		}
	}
}
