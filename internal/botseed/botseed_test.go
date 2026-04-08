package botseed

import (
	"context"
	"encoding/json"
	mathrand "math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/contracts"
)

func TestGeneratePersonas_DeterministicWithSeed(t *testing.T) {
	a := GeneratePersonas(25, 42)
	b := GeneratePersonas(25, 42)
	if len(a) != len(b) {
		t.Fatalf("length mismatch: %d vs %d", len(a), len(b))
	}
	// Passwords use crypto/rand so they legitimately differ; we only
	// assert the deterministic fields.
	for i := range a {
		if a[i].Email != b[i].Email {
			t.Errorf("email mismatch at %d: %q vs %q", i, a[i].Email, b[i].Email)
		}
		if a[i].Name != b[i].Name {
			t.Errorf("name mismatch at %d: %q vs %q", i, a[i].Name, b[i].Name)
		}
		if a[i].Locale != b[i].Locale {
			t.Errorf("locale mismatch at %d", i)
		}
		if strings.Join(a[i].Traits, ",") != strings.Join(b[i].Traits, ",") {
			t.Errorf("traits mismatch at %d", i)
		}
	}
}

func TestGeneratePersonas_UniqueEmails(t *testing.T) {
	personas := GeneratePersonas(100, 7)
	seen := make(map[string]struct{}, len(personas))
	for _, p := range personas {
		if _, dup := seen[p.Email]; dup {
			t.Fatalf("duplicate email: %s", p.Email)
		}
		seen[p.Email] = struct{}{}
		if !strings.HasPrefix(p.Email, "bot_") {
			t.Errorf("email %q missing bot_ prefix", p.Email)
		}
		if len(p.Password) < 24 {
			t.Errorf("password too short: %d chars", len(p.Password))
		}
	}
}

func TestExecutorResolvesPathParams(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	bot := &Bot{ID: "t1", Persona: Persona{Email: "bot_x@botseed.local"}}
	op := contracts.Operation{
		ID:     "listing_bid",
		Method: "POST",
		Path:   "/api/listings/:id/bids",
	}
	_, err := client.Execute(context.Background(), bot, op, map[string]any{
		"id":     "abc",
		"amount": 42,
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if gotPath != "/api/listings/abc/bids" {
		t.Errorf("unexpected path: %q", gotPath)
	}
}

func TestExecutorGETEncodesQueryString(t *testing.T) {
	var gotRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	bot := &Bot{ID: "t2"}
	op := contracts.Operation{Method: "GET", Path: "/api/items"}
	_, err := client.Execute(context.Background(), bot, op, map[string]any{"q": "hello"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(gotRawQuery, "q=hello") {
		t.Errorf("expected q=hello in query, got %q", gotRawQuery)
	}
}

func TestSignupFallsBackToLogin(t *testing.T) {
	var (
		signupCalls int
		loginCalls  int
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/signup":
			signupCalls++
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"exists"}`))
		case "/api/auth/login":
			loginCalls++
			// Decode body to make sure the client sent JSON credentials.
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["email"] == "" || body["password"] == "" {
				t.Errorf("login missing credentials: %+v", body)
			}
			http.SetCookie(w, &http.Cookie{Name: "auth_session", Value: "sess-123"})
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client := NewClient(srv.URL)
	bot, err := client.Signup(context.Background(), Persona{
		Email:    "bot_fallback@botseed.local",
		Password: "pw",
	})
	if err != nil {
		t.Fatalf("signup: %v", err)
	}
	if signupCalls != 1 {
		t.Errorf("expected 1 signup call, got %d", signupCalls)
	}
	if loginCalls != 1 {
		t.Errorf("expected 1 login call, got %d", loginCalls)
	}
	if bot.SessionID != "sess-123" {
		t.Errorf("expected session id to be captured, got %q", bot.SessionID)
	}
	if len(bot.Cookies) == 0 {
		t.Errorf("expected cookies to be captured")
	}
}

func TestRunTraffic_ErrorsOnEmptyContract(t *testing.T) {
	_, err := RunTraffic(context.Background(), TrafficConfig{
		Bots:     1,
		Rate:     1,
		BaseURL:  "http://example.invalid",
		Contract: &contracts.ContractFile{},
	})
	if err == nil {
		t.Fatalf("expected error for empty contract")
	}
}

func TestPickOperationPrefersBidderPaths(t *testing.T) {
	ops := []contracts.Operation{
		{ID: "list", Method: "GET", Path: "/api/listings"},
		{ID: "bid", Method: "POST", Path: "/api/listings/:id/bids"},
	}
	bidder := []string{"bidder"}
	hits := 0
	rng := mathrand.New(mathrand.NewSource(1))
	for i := 0; i < 500; i++ {
		op := pickOperation(rng, bidder, ops)
		if op.ID == "bid" {
			hits++
		}
	}
	if hits < 300 {
		t.Errorf("expected bidder bias (>=300/500), got %d", hits)
	}
}
