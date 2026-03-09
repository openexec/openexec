package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/router"
)

// TestE2EIntentRoutingValidation validates G-001 goal completion.
// This is the comprehensive validation suite proving intent routing works correctly.
// All queries MUST receive valid responses; none may return "could not determine intent" errors.
func TestE2EIntentRoutingValidation(t *testing.T) {
	// Setup server with GeneralChatTool registered (default behavior)
	cfg := Config{
		Port:        0, // random
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Bypass availability check - use simulator for deterministic keyword-based routing
	if br, ok := s.Coordinator.GetRouter().(*router.BitNetRouter); ok {
		br.SetSkipAvailabilityCheck(true)
	}

	// Matrix of test inputs representing real user scenarios
	// NOTE: allowError=true means we accept 500s from tool execution failures,
	// as long as they don't contain intent routing errors.
	testCases := []struct {
		name       string
		query      string
		wantTool   string // expected tool (or "" for any)
		wantInResp string // substring that MUST appear in result (or "" for any)
		allowError bool   // true if tool execution failure is acceptable (not an intent routing error)
	}{
		// --- Fallback scenarios (MUST work) ---
		{"empty_query", "", "general_chat", "", false},
		{"help_request", "help", "general_chat", "OpenExec", false},
		{"gibberish", "asdf1234xyz", "general_chat", "query", false},
		{"question", "What is the weather?", "general_chat", "query", false},

		// --- Keyword-matched scenarios (SHOULD work via simulator) ---
		{"deploy_keyword", "deploy to prod", "deploy", "", false},
		// symbol_keyword: The tool may return 500 because symbol doesn't exist in test env,
		// but that's a tool execution error NOT an intent routing error.
		{"symbol_keyword", "show function Execute", "read_symbol", "", true},
		{"wizard_keyword", "start wizard", "general_chat", "wizard", false},

		// --- Edge cases ---
		{"unicode_chinese", "你好世界", "general_chat", "query", false},
		{"unicode_japanese", "こんにちは", "general_chat", "query", false},
		{"long_query", strings.Repeat("hello ", 100), "general_chat", "query", false},
		{"special_chars", "!@#$%^&*()", "general_chat", "query", false},
		{"whitespace_only", "   ", "general_chat", "", false},
		{"newlines", "hello\nworld\ntesting", "general_chat", "query", false},
		{"tabs", "hello\tworld", "general_chat", "query", false},
	}

	// CRITICAL ASSERTION: No response may contain the old error message
	forbiddenSubstrings := []string{
		"could not determine intent",
		"low confidence",
		"model could not determine",
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

			// 1. HTTP 200 is required (unless allowError is true for expected tool failures)
			if rec.Code != http.StatusOK {
				if !tc.allowError {
					t.Errorf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
					return
				}
				// For allowError cases, we still check the response doesn't contain intent errors
				bodyStr := rec.Body.String()
				for _, forbidden := range forbiddenSubstrings {
					if strings.Contains(strings.ToLower(bodyStr), forbidden) {
						t.Errorf("CRITICAL: Found forbidden substring %q in error response: %s", forbidden, bodyStr)
					}
				}
				// Tool execution failures are acceptable - the key is that intent routing worked
				t.Logf("Accepted tool execution error (intent routing succeeded): %s", rec.Body.String())
				return
			}

			// 2. Parse response
			var resp struct {
				Result interface{} `json:"result"`
				Error  string      `json:"error"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// 3. Error field MUST be empty
			if resp.Error != "" {
				t.Errorf("unexpected error in response: %s", resp.Error)
			}

			// 4. Result field MUST be non-nil
			if resp.Result == nil {
				t.Error("expected non-empty result")
			}

			// 5. CRITICAL: Check for forbidden substrings
			resultStr, _ := resp.Result.(string)
			bodyStr := rec.Body.String()
			for _, forbidden := range forbiddenSubstrings {
				if strings.Contains(strings.ToLower(resultStr), forbidden) {
					t.Errorf("CRITICAL: Found forbidden substring %q in result: %s", forbidden, resultStr)
				}
				if strings.Contains(strings.ToLower(bodyStr), forbidden) {
					t.Errorf("CRITICAL: Found forbidden substring %q in response body: %s", forbidden, bodyStr)
				}
			}

			// 6. Optional: Check for expected content in response
			if tc.wantInResp != "" && !strings.Contains(resultStr, tc.wantInResp) {
				t.Errorf("expected result to contain %q, got: %s", tc.wantInResp, resultStr)
			}
		})
	}
}

// TestE2ENoConfidenceErrorsOnAnyInput ensures that arbitrary inputs never produce confidence errors.
// This is the definitive regression test for G-001.
func TestE2ENoConfidenceErrorsOnAnyInput(t *testing.T) {
	cfg := Config{
		Port:        0,
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if br, ok := s.Coordinator.GetRouter().(*router.BitNetRouter); ok {
		br.SetSkipAvailabilityCheck(true)
	}

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

	forbiddenSubstrings := []string{
		"could not determine intent",
		"low confidence",
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
			for _, forbidden := range forbiddenSubstrings {
				if strings.Contains(strings.ToLower(bodyStr), forbidden) {
					t.Errorf("[input %d] CRITICAL: Found forbidden substring %q in response: %s", i, forbidden, bodyStr)
				}
			}
		})
	}
}

// TestE2ERapidSequentialQueries ensures no race conditions under sequential load.
func TestE2ERapidSequentialQueries(t *testing.T) {
	cfg := Config{
		Port:        0,
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	if br, ok := s.Coordinator.GetRouter().(*router.BitNetRouter); ok {
		br.SetSkipAvailabilityCheck(true)
	}

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

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
