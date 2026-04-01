package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// forbiddenIntentErrorStrings defines error messages that must never appear in responses.
// These indicate intent routing failures that the G-001 fix should prevent.
var forbiddenIntentErrorStrings = []string{
	"could not determine intent",
	"low confidence",
	"model could not determine",
}

// TestE2EIntentRoutingValidation validates G-001 goal completion.
// This is the comprehensive validation suite proving intent routing works correctly.
// All queries MUST receive valid responses; none may return "could not determine intent" errors.
func TestE2EIntentRoutingValidation(t *testing.T) {
	// Setup server with BitNetRouter in skip mode (uses NewTestServer helper)
	ts := NewTestServer(t)
	s := ts.Server

	// Matrix of test inputs representing real user scenarios.
	// In suggest-only mode, DCP returns IntentSuggestion structs, not execution results.
	testCases := []struct {
		name     string
		query    string
		wantTool string // expected tool_name (or "" for any)
	}{
		// --- Fallback scenarios (MUST work) ---
		{"empty_query", "", "general_chat"},
		{"help_request", "help", "general_chat"},
		{"gibberish", "asdf1234xyz", "general_chat"},
		{"question", "What is the weather?", "general_chat"},

		// --- Keyword-matched scenarios (SHOULD work via simulator) ---
		{"deploy_keyword", "deploy to prod", "deploy"},
		{"symbol_keyword", "show function Execute", ""}, // may route to read_symbol or general_chat
		{"wizard_keyword", "start wizard", ""},

		// --- Edge cases ---
		{"unicode_chinese", "你好世界", "general_chat"},
		{"unicode_japanese", "こんにちは", "general_chat"},
		{"long_query", strings.Repeat("hello ", 100), "general_chat"},
		{"special_chars", "!@#$%^&*()", "general_chat"},
		{"whitespace_only", "   ", "general_chat"},
		{"newlines", "hello\nworld\ntesting", "general_chat"},
		{"tabs", "hello\tworld", "general_chat"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Prepare request
			payload := map[string]string{"query": tc.query}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			// Execute
			s.Mux.ServeHTTP(rec, req)

			// --- Contract Validation ---

			// 1. HTTP 200 is required (suggest-only mode never returns 500 for routing)
			if rec.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
				return
			}

			// 2. Parse response
			var resp struct {
				Result map[string]interface{} `json:"result"`
				Error  string                 `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// 3. Error field MUST be empty
			if resp.Error != "" {
				t.Errorf("unexpected error in response: %s", resp.Error)
			}

			// 4. Result field MUST be non-nil with valid IntentSuggestion fields
			if resp.Result == nil {
				t.Fatal("expected non-nil result")
			}

			toolName, _ := resp.Result["tool_name"].(string)
			if toolName == "" {
				t.Error("expected non-empty tool_name in suggestion")
			}

			if _, ok := resp.Result["confidence"].(float64); !ok {
				t.Errorf("expected confidence to be a number, got: %T", resp.Result["confidence"])
			}

			// 5. Verify expected tool_name if specified
			if tc.wantTool != "" && toolName != tc.wantTool {
				t.Errorf("expected tool_name %q, got: %s", tc.wantTool, toolName)
			}

			// 6. CRITICAL: Check for forbidden substrings in full response
			bodyStr := strings.ToLower(rec.Body.String())
			for _, forbidden := range forbiddenIntentErrorStrings {
				if strings.Contains(bodyStr, forbidden) {
					t.Errorf("CRITICAL: Found forbidden substring %q in response body", forbidden)
				}
			}
		})
	}
}

// TestE2ENoConfidenceErrorsOnAnyInput ensures that arbitrary inputs never produce confidence errors.
// This is the definitive regression test for G-001.
func TestE2ENoConfidenceErrorsOnAnyInput(t *testing.T) {
	ts := NewTestServer(t)
	s := ts.Server

	// Fuzz-like inputs designed to potentially trigger edge cases
	fuzzInputs := []string{
		"",                                       // empty
		"a",                                      // single char
		strings.Repeat("x", 10000),               // very long
		"SELECT * FROM users; DROP TABLE users;", // SQL injection attempt
		"<script>alert('xss')</script>",          // XSS attempt
		"../../etc/passwd",                       // path traversal
		"null",                                   // JSON null keyword
		"undefined",                              // JS undefined
		"true",                                   // boolean
		"12345",                                  // numeric
		`{"nested": "json"}`,                     // JSON object
		"🚀🔥💻",                                    // emoji
		"\x00\x01\x02",                           // control characters
		"hello\nworld",                           // newlines
		"foo\tbar\tbaz",                          // tabs
	}

	for i, input := range fuzzInputs {
		t.Run(strings.ReplaceAll(input[:min(len(input), 20)], "\n", "\\n"), func(t *testing.T) {
			payload := map[string]string{"query": input}
			body, _ := json.Marshal(payload)

			req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			s.Mux.ServeHTTP(rec, req)

			// Must return 200
			if rec.Code != http.StatusOK {
				t.Errorf("[input %d] expected status 200, got %d. Body: %s", i, rec.Code, rec.Body.String())
				return
			}

			// Must not contain forbidden substrings
			bodyStr := rec.Body.String()
			for _, forbidden := range forbiddenIntentErrorStrings {
				if strings.Contains(strings.ToLower(bodyStr), forbidden) {
					t.Errorf("[input %d] CRITICAL: Found forbidden substring %q in response: %s", i, forbidden, bodyStr)
				}
			}
		})
	}
}

// TestE2ERapidSequentialQueries ensures no race conditions under sequential load.
func TestE2ERapidSequentialQueries(t *testing.T) {
	ts := NewTestServer(t)
	s := ts.Server

	queries := []string{"help", "deploy", "wizard", "asdf", "test", "hello"}

	// Run 50 rapid requests
	for i := 0; i < 50; i++ {
		query := queries[i%len(queries)]
		payload := map[string]string{"query": query}
		body, _ := json.Marshal(payload)

		req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		s.Mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("[iteration %d] expected status 200, got %d. Body: %s", i, rec.Code, rec.Body.String())
		}

		// Check no error in response
		var resp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp.Error != "" {
			t.Errorf("[iteration %d] unexpected error: %s", i, resp.Error)
		}
	}
}
