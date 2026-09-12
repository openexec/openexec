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

func TestMetadataReadinessNativeDefaultTagOnly(t *testing.T) {
	for _, tc := range []struct {
		name, model, catalog string
		native, ready        bool
	}{
		{"native implicit latest", "qwen38-27b-mtp2-128k", "qwen38-27b-mtp2-128k:latest", true, true},
		{"native namespace", "team/model", "team/model:latest", true, true},
		{"native explicit exact", "model:q4", "model:q4", true, true},
		{"native wrong explicit tag", "model:q4", "model:latest", true, false},
		{"native wrong model", "model", "other:latest", true, false},
		{"hosted no alias", "model", "model:latest", false, false},
		{"hosted exact", "model", "model", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: readinessTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
					t.Fatalf("non-metadata request: %s %s", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"` + tc.catalog + `"}]}`)), Header: http.Header{}}, nil
			})}
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{BaseURL: "http://fixture.invalid/v1", APIKey: "fixture", HTTPClient: client})
			if tc.native {
				p.ollamaBudgetURL = "http://127.0.0.1:11436/api/chat"
			}
			err := p.CheckModelMetadata(context.Background(), tc.model)
			if (err == nil) != tc.ready || calls != 1 {
				t.Fatalf("ready=%v calls=%d err=%v", tc.ready, calls, err)
			}
		})
	}
}
