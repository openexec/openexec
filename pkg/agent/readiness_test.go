package agent

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type readinessTransport func(*http.Request) (*http.Response, error)

func (f readinessTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestMetadataReadinessBoundsTimeoutBodyAndConfiguredModel(t *testing.T) {
	for _, oversized := range []bool{false, true} {
		client := &http.Client{Transport: readinessTransport(func(r *http.Request) (*http.Response, error) {
			deadline, ok := r.Context().Deadline()
			if !ok || time.Until(deadline) > 5*time.Second {
				t.Fatal("metadata request has no bounded deadline")
			}
			body := `{"data":[{"id":"configured"}]}`
			if oversized {
				body = strings.Repeat("x", (1<<20)+1)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}
		p, _ := NewOpenAIProvider(OpenAIProviderConfig{BaseURL: "http://fixture.invalid/v1", APIKey: "fixture", HTTPClient: client})
		err := p.CheckModelMetadata(context.Background(), "configured")
		if (err != nil) != oversized {
			t.Fatalf("body admission mismatch: %v", err)
		}
		if err := p.CheckModelMetadata(context.Background(), ""); err == nil {
			t.Fatal("empty configuration treated as ready")
		}
	}
}
