package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
)

// A server that accepts the request but never sends headers: the failure must
// keep its original text, code and retryability, and add the wait, the
// effective timeout and the body size, for both Complete and Stream.
func TestOpenAIHeaderTimeoutReportsWaitAndBudget(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	p, err := NewOpenAIProvider(OpenAIProviderConfig{
		Name: "fixture", BaseURL: srv.URL, APIKey: "k", Timeout: 150 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Model: "gpt-4o", Messages: []Message{NewTextMessage(RoleUser, "hello")}}

	wait := regexp.MustCompile(`waited (1[5-9][0-9]|[2-9][0-9]{2})ms of 150ms request timeout; request body [0-9]+ bytes`)
	check := func(name string, err error) {
		t.Helper()
		var pe *ProviderError
		if !errors.As(err, &pe) {
			t.Fatalf("%s: want ProviderError, got %v", name, err)
		}
		if pe.Code != ErrCodeServerError || !pe.Retryable {
			t.Errorf("%s: code/retryable changed: %+v", name, pe)
		}
		if !strings.HasPrefix(pe.Message, "HTTP error: ") ||
			!strings.Contains(pe.Message, "Client.Timeout exceeded while awaiting headers") {
			t.Errorf("%s: original failure text lost: %q", name, pe.Message)
		}
		if !wait.MatchString(pe.Message) {
			t.Errorf("%s: no wait/timeout/body size in %q", name, pe.Message)
		}
	}
	_, err = p.Complete(context.Background(), req)
	check("complete", err)
	_, err = p.Stream(context.Background(), req)
	check("stream", err)
}

// Near miss: a non-200 reply is not a transport failure and keeps its own
// path; the default request timeout is untouched.
func TestOpenAIHTTPStatusErrorHasNoTransportSuffix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	p, err := NewOpenAIProvider(OpenAIProviderConfig{Name: "fixture", BaseURL: srv.URL, APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if got := p.RequestTimeout(); got != DefaultOpenAITimeout {
		t.Errorf("default timeout changed: %v", got)
	}
	_, err = p.Complete(context.Background(), Request{Model: "gpt-4o", Messages: []Message{NewTextMessage(RoleUser, "hi")}})
	if err == nil || strings.Contains(err.Error(), "request timeout") {
		t.Fatalf("unexpected error %v", err)
	}
}
