package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestDispatchProvenanceRefusedBeforeAttempt(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"0.32.13"}`)
			return
		}
		calls.Add(1)
		fmt.Fprint(w, `{"done":true,"prompt_eval_count":10,"eval_count":2,"message":{"content":"public ok"}}`)
	}))
	defer server.Close()
	p, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "public-key", Models: []string{"public-model"}, BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if !p.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("negotiation failed")
	}
	valid := Request{Model: "public-model", Messages: []Message{NewTextMessage(RoleUser, "public query")}}
	if _, err := p.Complete(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	body, at, base, dispatch := p.LastRequestWithDispatch()
	for _, reason := range []string{"validation", "floor", "media", "arguments", "unbound", "construction"} {
		t.Run(reason, func(t *testing.T) {
			req := valid
			ctx := context.Background()
			var err error
			switch reason {
			case "validation":
				req.Model = ""
				_, err = p.Complete(ctx, req)
			case "floor":
				_, err = p.CompleteBounded(ctx, req, 1536, 16)
			case "media":
				req.Messages = []Message{{Role: RoleUser, Content: []ContentBlock{{Type: ContentTypeImage}}}}
				_, err = p.CompleteBounded(ctx, req, 2048, 16)
			case "arguments":
				req.Messages = []Message{{Role: RoleAssistant, Content: []ContentBlock{{Type: ContentTypeToolUse, ToolInput: []byte(`"not an object"`)}}}}
				_, err = p.CompleteBounded(ctx, req, 2048, 16)
			case "unbound":
				req.Messages = []Message{NewToolResultMessage("missing", "public", nil)}
				_, err = p.CompleteBounded(ctx, req, 2048, 16)
			case "construction":
				_, err = p.Complete(nil, req) // NewRequestWithContext refuses a nil context before Do.
			}
			if err == nil {
				t.Fatal("refused request was allowed")
			}
			gotBody, gotAt, gotBase, gotDispatch := p.LastRequestWithDispatch()
			if calls.Load() != 1 || !bytes.Equal(gotBody, body) || !gotAt.Equal(at) || gotBase != base || !reflect.DeepEqual(gotDispatch, dispatch) {
				t.Fatal("refused request acquired fresh dispatch evidence")
			}
		})
	}
	// A snapshot is immutable to callers, including the pointer field.
	dispatch.URL = "mutated"
	body[0] = 'x'
	next, _, _, d := p.LastRequestWithDispatch()
	if d.URL == "mutated" || next[0] == 'x' {
		t.Fatal("caller mutated remembered evidence")
	}
}

func TestDispatchProvenanceMalformedCompatibilityConstruction(t *testing.T) {
	for _, stream := range []bool{false, true} {
		p, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "public-key", Models: []string{"public-model"}, BaseURL: "http://%zz"})
		if err != nil {
			t.Fatal(err)
		}
		req := Request{Model: "public-model", Messages: []Message{NewTextMessage(RoleUser, "public query")}}
		if stream {
			_, err = p.Stream(context.Background(), req)
		} else {
			_, err = p.Complete(context.Background(), req)
		}
		if err == nil {
			t.Fatal("malformed target was allowed")
		}
		body, at, _, dispatch := p.LastRequestWithDispatch()
		if len(body) > 0 || !at.IsZero() || dispatch != nil {
			t.Fatal("construction failure invented a transport attempt")
		}
	}
}

func TestDispatchProvenanceRetainsInitialAttemptAcrossRedirect(t *testing.T) {
	paths := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		io.Copy(io.Discard, r.Body)
		if r.URL.Path == "/v1/chat/completions" {
			http.Redirect(w, r, "/redirected", http.StatusTemporaryRedirect)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"public ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	p, err := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "public-key", Models: []string{"public-model"}, BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Complete(context.Background(), Request{Model: "public-model", Messages: []Message{NewTextMessage(RoleUser, "public query")}}); err != nil {
		t.Fatal(err)
	}
	_, _, _, d := p.LastRequestWithDispatch()
	if d == nil || d.URL != server.URL+"/v1/chat/completions" || d.Method != "POST" || d.Provenance != DispatchSourceKnown || len(paths) != 2 || !reflect.DeepEqual([]string{<-paths, <-paths}, []string{"/v1/chat/completions", "/redirected"}) {
		t.Fatal("initial dispatch confused with redirect receipt")
	}
}
