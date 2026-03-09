package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openexec/openexec/internal/router"
)

func TestDCPQueryIntegration(t *testing.T) {
	// Initialize a unified server in test mode
	// We mock the dependencies minimally
	cfg := Config{
		Port:        0, // random
		ProjectsDir: t.TempDir(),
		DataDir:     t.TempDir(),
	}

	s, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Bypass availability check for bitnet during tests
	if br, ok := s.Coordinator.GetRouter().(*router.BitNetRouter); ok {
		br.SetSkipAvailabilityCheck(true)
	}

	// Test successful general chat fallback
	payload := map[string]string{"query": "hello"}
	body, _ := json.Marshal(payload)
	
	req := httptest.NewRequest("POST", "/api/v1/dcp/query", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	
	s.Mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Result string `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Error != "" {
		t.Errorf("unexpected error in response: %s", resp.Error)
	}
	if resp.Result == "" {
		t.Error("expected non-empty result (general_chat fallback)")
	}
}
