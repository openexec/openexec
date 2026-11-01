package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// Synthetic controls, not the Solar request. Exercise the real serializer and
// admission caller at the observed bounds; do not infer original eligibility.
func TestAssembledDiagnosisObservationEligibility(t *testing.T) {
	for _, mode := range []string{"append-output-change", "same-count", "edited-prefix", "model", "tools", "context", "large-append"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil || r.Method != "POST" || r.URL.Path != "/api/chat" || body["truncate"] != false {
					t.Error("native request shape changed")
				}
				fmt.Fprint(w, `{"done":true,"prompt_eval_count":18000,"eval_count":500,"message":{"content":"synthetic answer"}}`)
			}))
			defer server.Close()
			p := &OpenAIProvider{ollamaBudgetURL: server.URL + "/api/chat", httpClient: server.Client()}
			request := Request{Model: "synthetic-model", System: "synthetic authority",
				Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("a", 120000))},
				Tools:    []ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}}}
			if _, err := p.CompleteBounded(context.Background(), request, 65536, 2048); err != nil {
				t.Fatal(err)
			}
			prior, at, base, dispatch := p.LastRequestWithDispatch()
			request.Messages = append(request.Messages, NewTextMessage(RoleAssistant, strings.Repeat("b", 8000)))
			cap := 65536
			switch mode {
			case "same-count":
				request.Messages = request.Messages[:1]
			case "edited-prefix":
				request.Messages[0] = NewTextMessage(RoleUser, strings.Repeat("z", 120000))
			case "model":
				request.Model = "other-model"
			case "tools":
				request.Tools[0].Name = "other-tool"
			case "context":
				cap = 32768
			case "large-append":
				request.Messages[1] = NewTextMessage(RoleAssistant, strings.Repeat("b", 80000))
			}
			response, err := p.CompleteBounded(context.Background(), request, cap, 21998)
			if mode == "append-output-change" {
				if err != nil || calls != 2 || response.Usage.PromptTokens != 18000 {
					t.Fatal("eligible observation failed or estimate became accounting")
				}
				return
			}
			var overflow *ContextOverflowError
			if !errors.As(err, &overflow) || overflow.Dispatched || overflow.ContextTokens != cap || calls != 1 {
				t.Fatal("ineligible/oversized request escaped native admission")
			}
			body, gotAt, gotBase, gotDispatch := p.LastRequestWithDispatch()
			if !bytes.Equal(body, prior) || !at.Equal(gotAt) || base != gotBase || !reflect.DeepEqual(dispatch, gotDispatch) {
				t.Fatal("preflight refusal replaced earlier dispatch evidence")
			}
		})
	}
}

func TestAssembledDiagnosisObservationRequiresValidNativeUsage(t *testing.T) {
	for _, response := range []string{
		`{"done":true,"eval_count":500,"message":{"content":"ok"}}`,
		`{"done":false,"prompt_eval_count":18000,"eval_count":500}`,
		`{"done":true,"prompt_eval_count":-1,"eval_count":500}`,
		`{"done":true,"prompt_eval_count":18000,"eval_count":2049}`,
	} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			fmt.Fprint(w, response)
		}))
		p := &OpenAIProvider{ollamaBudgetURL: server.URL, httpClient: server.Client()}
		request := Request{Model: "synthetic", Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("a", 120000))}}
		if _, err := p.CompleteBounded(context.Background(), request, 65536, 2048); err == nil {
			t.Fatal("invalid native usage accepted")
		}
		request.Messages = append(request.Messages, NewTextMessage(RoleAssistant, strings.Repeat("b", 8000)))
		_, err := p.CompleteBounded(context.Background(), request, 65536, 21998)
		var overflow *ContextOverflowError
		if !errors.As(err, &overflow) || overflow.Dispatched || calls != 1 || p.contextObservation != nil {
			t.Fatal("invalid response seeded an observation")
		}
		server.Close()
	}
}
