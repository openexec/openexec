package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// githubCopilotTokenURL exchanges a GitHub OAuth token for a short-lived
// Copilot session token. api.githubcopilot.com rejects the OAuth token itself;
// it only accepts the session token. This endpoint is what Copilot's own
// editor clients use; GitHub does not publish it as a stable public API.
var githubCopilotTokenURL = "https://api.github.com/copilot_internal/v2/token"

// Headers the Copilot chat endpoint requires to identify the client.
const (
	githubCopilotEditorVersion     = "openexec/1.0"
	githubCopilotEditorPluginVer   = "openexec/1.0"
	githubCopilotIntegrationID     = "vscode-chat"
	githubCopilotTokenRefreshSlack = 60 * time.Second
)

// copilotTransport authenticates requests to the Copilot chat API: it swaps the
// configured GitHub OAuth token for a cached Copilot session token and adds the
// client-identification headers.
// withGitHubCopilotAuth routes config's requests through copilotTransport so
// the OAuth token in config.APIKey is exchanged for a session token. It is
// idempotent.
func withGitHubCopilotAuth(config *OpenAIProviderConfig) {
	base := http.DefaultTransport
	if config.HTTPClient != nil && config.HTTPClient.Transport != nil {
		if _, done := config.HTTPClient.Transport.(*copilotTransport); done {
			return
		}
		base = config.HTTPClient.Transport
	}
	timeout := config.Timeout
	if config.HTTPClient != nil && config.HTTPClient.Timeout != 0 {
		timeout = config.HTTPClient.Timeout
	}
	if timeout == 0 {
		timeout = DefaultOpenAITimeout
	}
	config.HTTPClient = &http.Client{
		Transport: &copilotTransport{base: base, oauthToken: config.APIKey, tokenURL: githubCopilotTokenURL, now: time.Now},
		Timeout:   timeout,
	}
}

// isGitHubCopilotBaseURL reports whether baseURL is the Copilot chat API, so a
// provider entry configured by URL alone (rather than by the githubcopilot
// name) still authenticates with a session token.
func isGitHubCopilotBaseURL(baseURL string) bool {
	u, err := url.Parse(baseURL)
	return err == nil && strings.EqualFold(u.Hostname(), "api.githubcopilot.com")
}

type copilotTransport struct {
	base       http.RoundTripper
	oauthToken string
	tokenURL   string
	now        func() time.Time

	mu      sync.Mutex
	session string
	expires time.Time
}

func (t *copilotTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	session, err := t.sessionToken(req)
	if err != nil {
		return nil, err
	}
	out := req.Clone(req.Context())
	out.Header.Set("Authorization", "Bearer "+session)
	out.Header.Set("Editor-Version", githubCopilotEditorVersion)
	out.Header.Set("Editor-Plugin-Version", githubCopilotEditorPluginVer)
	out.Header.Set("Copilot-Integration-Id", githubCopilotIntegrationID)
	return t.base.RoundTrip(out)
}

func (t *copilotTransport) sessionToken(req *http.Request) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.session != "" && t.now().Add(githubCopilotTokenRefreshSlack).Before(t.expires) {
		return t.session, nil
	}
	exchange, err := http.NewRequestWithContext(req.Context(), http.MethodGet, t.tokenURL, nil)
	if err != nil {
		return "", err
	}
	exchange.Header.Set("Authorization", "token "+t.oauthToken)
	exchange.Header.Set("Accept", "application/json")
	exchange.Header.Set("Editor-Version", githubCopilotEditorVersion)
	exchange.Header.Set("Editor-Plugin-Version", githubCopilotEditorPluginVer)
	resp, err := t.base.RoundTrip(exchange)
	if err != nil {
		return "", fmt.Errorf("githubcopilot: token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", &ProviderError{
			Code: ErrCodeAuthentication,
			Message: fmt.Sprintf("githubcopilot: GitHub refused the Copilot token exchange (HTTP %d): %s. %s must be a GitHub OAuth token from a Copilot sign-in on an account with an active Copilot subscription (a classic personal access token is not accepted)",
				resp.StatusCode, truncateForError(string(body)), GitHubCopilotAPIKeyEnv),
		}
	}
	var parsed struct {
		Token     string `json:"token"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.Token == "" {
		return "", &ProviderError{Code: ErrCodeAuthentication, Message: "githubcopilot: token exchange returned no session token"}
	}
	t.session = parsed.Token
	t.expires = time.Unix(parsed.ExpiresAt, 0)
	return t.session, nil
}

func truncateForError(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
