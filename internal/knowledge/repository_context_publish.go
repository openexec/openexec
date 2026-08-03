package knowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RepositoryContextPublisher publishes only the versioned presentation model.
// It first observes Agent Console's cached version and uses If-Match so a
// delayed process cannot overwrite a context published after it started.
type RepositoryContextPublisher struct {
	Client  *http.Client
	Timeout time.Duration
}

func (p RepositoryContextPublisher) Publish(ctx context.Context, consoleURL, projectID, token string, projection RepositoryContextProjection) error {
	base, err := url.Parse(strings.TrimRight(consoleURL, "/"))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return fmt.Errorf("agent console URL must be an absolute http(s) URL")
	}
	if strings.TrimSpace(projectID) == "" || strings.Contains(projectID, "/") {
		return fmt.Errorf("agent console project ID is required")
	}
	client := p.httpClient()
	endpoint := strings.TrimRight(base.String(), "/") + "/api/projects/" + url.PathEscape(projectID) + "/repository-context"
	current, err := p.currentVersion(ctx, client, endpoint, token)
	if err != nil {
		return err
	}
	body, err := json.Marshal(projection)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if current != "" {
		request.Header.Set("If-Match", `"`+strings.ReplaceAll(current, `"`, "")+`"`)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("publish repository context: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return repositoryContextHTTPError("publish repository context", response)
	}
	return nil
}

func (p RepositoryContextPublisher) httpClient() *http.Client {
	if p.Client != nil {
		return p.Client
	}
	// Never the zero-timeout default: a hung Agent Console must fail the
	// publish, not park it forever.
	timeout := p.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &http.Client{Timeout: timeout}
}

func (p RepositoryContextPublisher) currentVersion(ctx context.Context, client *http.Client, endpoint, token string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("read current repository context: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", repositoryContextHTTPError("read current repository context", response)
	}
	return strings.Trim(response.Header.Get("ETag"), `"`), nil
}

func repositoryContextHTTPError(action string, response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	var payload struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &payload)
	message := strings.TrimSpace(payload.Error)
	if message == "" {
		message = http.StatusText(response.StatusCode)
	}
	return fmt.Errorf("%s: %s (%d)", action, message, response.StatusCode)
}
