package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAssembledContextRejectsSystemToolsResultsAndUnicodeBeforeHTTP(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; t.Error("oversized request reached inference") }))
	defer server.Close()
	p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", BaseURL: server.URL + "/v1"})
	p.ollamaBudgetURL = server.URL + "/api/chat"
	for _, req := range []Request{
		{System: strings.Repeat("system code ", 20000)},
		{Tools: []ToolDefinition{{Name: "read", Description: strings.Repeat("nested schema ", 20000)}}},
		{Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("界", 30000))}},
		{Messages: []Message{NewTextMessage(RoleUser, strings.Repeat("\\\"12345", 30000))}},
	} {
		_, err := p.CompleteBounded(context.Background(), req, 32768, 2048)
		var overflow *ContextOverflowError
		if !errors.As(err, &overflow) || overflow.Dispatched || overflow.RequestBytes == 0 {
			t.Fatalf("missing typed preflight %v", err)
		}
	}
	if calls != 0 {
		t.Fatal("inference escaped")
	}
}

func TestNativeContextRefusalDoesNotExposePromptOrClaimNoDispatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "private owner content: request exceeds available context size"})
	}))
	defer server.Close()
	p, _ := NewOpenAIProvider(OpenAIProviderConfig{APIKey: "local", BaseURL: server.URL + "/v1"})
	p.ollamaBudgetURL = server.URL + "/api/chat"
	_, err := p.CompleteBounded(context.Background(), Request{}, 32768, 2048)
	var overflow *ContextOverflowError
	if !errors.As(err, &overflow) || !overflow.Dispatched || strings.Contains(err.Error(), "private owner") {
		t.Fatalf("unsafe diagnostic %v", err)
	}
}
