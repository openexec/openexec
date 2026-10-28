package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Models the live comparison: a valid user query followed by oversized tool
// evidence fails inside the compatibility template; native truncate=false
// refuses the context without losing that query. No resource grant is supplied.
func TestUnboundedLocalOllamaCompletePreservesContextAndDefaults(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		t.Run(map[bool]string{false: "fits", true: "overflow"}[overflow], func(t *testing.T) {
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					w.Write([]byte(`{"version":"0.32.13"}`))
					return
				}
				calls++
				if r.URL.Path != "/api/chat" {
					w.WriteHeader(500)
					w.Write([]byte(`{"error":{"message":"No user query found in messages."}}`))
					return
				}
				var body struct {
					Truncate *bool            `json:"truncate"`
					Options  map[string]any   `json:"options"`
					Messages []map[string]any `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Truncate == nil || *body.Truncate {
					t.Error("ungranted continuation permits truncation")
				}
				if len(body.Options) != 0 {
					t.Errorf("zero grant changed model defaults: %v", body.Options)
				}
				if len(body.Messages) != 3 || body.Messages[0]["role"] != "user" || body.Messages[0]["content"] != "keep the actual query" || body.Messages[2]["tool_name"] != "read_file" || body.Messages[2]["content"] != "actual tool evidence" {
					t.Errorf("conversation changed: %v", body.Messages)
				}
				if overflow {
					w.WriteHeader(400)
					w.Write([]byte(`{"error":"request exceeds the available context size"}`))
					return
				}
				w.Write([]byte(`{"done":true,"done_reason":"stop","prompt_eval_count":120000,"eval_count":3000,"message":{"content":"useful answer"}}`))
			}))
			defer srv.Close()
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", Models: []string{"local"}, BaseURL: srv.URL + "/v1"})
			if !p.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("native negotiation failed")
			}
			messages := []Message{NewTextMessage(RoleUser, "keep the actual query"), {Role: RoleAssistant, Content: []ContentBlock{{Type: ContentTypeToolUse, ToolUseID: "call-1", ToolName: "read_file", ToolInput: json.RawMessage(`{"path":"fixture.txt"}`)}}}, NewToolResultMessage("call-1", "actual tool evidence", nil)}
			res, err := p.Complete(context.Background(), Request{Model: "local", Messages: messages})
			if calls != 1 {
				t.Fatalf("inference retried: %d", calls)
			}
			if overflow {
				var contextErr *ContextOverflowError
				if !errors.As(err, &contextErr) || !contextErr.Dispatched {
					t.Fatalf("ungranted context failure lost its typed refusal: %v", err)
				}
			} else if err != nil || res.GetText() != "useful answer" || res.Usage.TotalTokens != 123000 {
				t.Fatalf("ungranted native completion lost actual usage/result: %v %v", res, err)
			}
		})
	}
}

func TestUnboundedLocalOllamaCompletePreservesExplicitOptionsAndFallbacks(t *testing.T) {
	for _, mode := range []string{"options", "forced-tool", "media", "unnegotiated"} {
		t.Run(mode, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					w.Write([]byte(`{"version":"0.32.13"}`))
					return
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if mode == "options" {
					if r.URL.Path != "/api/chat" {
						t.Errorf("wrong route: %s", r.URL.Path)
					}
					o, _ := body["options"].(map[string]any)
					if len(o) != 4 || o["num_predict"] != float64(17) || o["temperature"] != 0.4 || o["top_p"] != 0.7 || len(o["stop"].([]any)) != 1 {
						t.Errorf("explicit generation options changed: %v", o)
					}
					w.Write([]byte(`{"done":true,"prompt_eval_count":10,"eval_count":2,"message":{"content":"ok"}}`))
				} else {
					if r.URL.Path != "/v1/chat/completions" {
						t.Errorf("compatibility fallback lost for %s: %s", mode, r.URL.Path)
					}
					w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
				}
			}))
			defer srv.Close()
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", Models: []string{"local"}, BaseURL: srv.URL + "/v1"})
			if mode != "unnegotiated" && !p.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("negotiation failed")
			}
			temperature, topP := 0.4, 0.7
			req := Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, "query")}, MaxTokens: 17, Temperature: &temperature, TopP: &topP, StopSequences: []string{"END"}}
			if mode == "forced-tool" {
				req.ToolChoice = "required"
			}
			if mode == "media" {
				req.Messages[0].Content = append(req.Messages[0].Content, ContentBlock{Type: ContentTypeImage})
			}
			if _, err := p.Complete(context.Background(), req); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Exercise Complete after real negotiation, so compatibility metadata cannot
// silently disappear when ordinary calls start using the native transport.
func TestUnboundedLocalOllamaCompleteStopReasons(t *testing.T) {
	for _, tc := range []struct {
		name, reason, calls string
		want                StopReason
	}{
		{"end", "stop", "[]", StopReasonEnd},
		{"length", "length", "[]", StopReasonMaxTokens},
		{"tool", "stop", `[{"function":{"name":"read_file","arguments":{"path":"x"}}}]`, StopReasonToolUse},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				if r.URL.Path != "/api/chat" {
					t.Errorf("wrong route: %s", r.URL.Path)
				}
				fmt.Fprintf(w, `{"done":true,"done_reason":%q,"prompt_eval_count":3,"eval_count":2,"message":{"content":"ok","tool_calls":%s}}`, tc.reason, tc.calls)
			}))
			defer srv.Close()
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", Models: []string{"local"}, BaseURL: srv.URL + "/v1"})
			if !p.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("negotiation failed")
			}
			res, err := p.Complete(context.Background(), Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, "query")}})
			if err != nil {
				t.Fatal(err)
			}
			if res.StopReason != tc.want {
				t.Fatalf("native stop reason lost: got %q want %q", res.StopReason, tc.want)
			}
			if res.Usage.TotalTokens != 5 {
				t.Fatalf("actual usage changed: %+v", res.Usage)
			}
			if tc.want == StopReasonToolUse && len(res.GetToolCalls()) != 1 {
				t.Fatal("tool call lost")
			}
		})
	}
}

func TestUnboundedLocalOllamaCompleteErrorContract(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		retry  bool
	}{
		{400, ErrCodeInvalidRequest, false}, {401, ErrCodeAuthentication, false},
		{403, ErrCodePermission, false}, {404, ErrCodeNotFound, false},
		{429, ErrCodeRateLimit, true}, {503, ErrCodeServerError, true},
		{0, ErrCodeServerError, true},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				if r.URL.Path != "/api/chat" {
					t.Errorf("wrong route: %s", r.URL.Path)
				}
				if tc.status == 0 {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					conn.Close()
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, `{"error":"private prompt must not escape"}`)
			}))
			defer srv.Close()
			p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", Models: []string{"local"}, BaseURL: srv.URL + "/v1"})
			if !p.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("negotiation failed")
			}
			p.httpClient.Timeout = 50 * time.Millisecond
			_, err := p.Complete(context.Background(), Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, "query")}})
			var pe *ProviderError
			if !errors.As(err, &pe) {
				t.Fatalf("native error classification lost: %v", err)
			}
			if pe.Code != tc.code || pe.StatusCode != tc.status || pe.Retryable != tc.retry {
				t.Fatalf("native error fields changed: %+v", pe)
			}
			if strings.Contains(err.Error(), "private prompt") {
				t.Fatal("native error leaked provider content")
			}
			if tc.status == 0 && !strings.Contains(pe.Message, "request timeout") {
				t.Fatalf("transport timing lost: %v", pe)
			}
		})
	}
}

func TestLocalOllamaCompletePreservesCallerCancellation(t *testing.T) {
	for _, bounded := range []bool{false, true} {
		for _, deadline := range []bool{false, true} {
			t.Run(fmt.Sprintf("bounded=%t/deadline=%t", bounded, deadline), func(t *testing.T) {
				received := make(chan struct{})
				release := make(chan struct{})
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/version" {
						fmt.Fprint(w, `{"version":"0.32.13"}`)
						return
					}
					close(received)
					<-release
				}))
				defer srv.Close()
				defer close(release)
				p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", Models: []string{"local"}, BaseURL: srv.URL + "/v1"})
				if !p.EnableLocalOllamaBounds(context.Background()) {
					t.Fatal("native negotiation failed")
				}
				ctx, cancel := context.WithCancel(context.Background())
				want := context.Canceled
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 250*time.Millisecond)
					want = context.DeadlineExceeded
				}
				defer cancel()
				if !deadline {
					go func() {
						select {
						case <-received:
							cancel()
						case <-ctx.Done():
						}
					}()
				}
				req := Request{Model: "local", Messages: []Message{NewTextMessage(RoleUser, "query")}}
				var err error
				if bounded {
					_, err = p.CompleteBounded(ctx, req, 2048, 16)
				} else {
					_, err = p.Complete(ctx, req)
				}
				if !errors.Is(err, want) {
					t.Fatalf("caller cancellation identity lost: got %v, want %v", err, want)
				}
				select {
				case <-received:
				default:
					t.Fatal("cancellation fixture never reached native inference")
				}
			})
		}
	}
}
