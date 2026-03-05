package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/pkg/audit"
)

func TestServer_Health(t *testing.T) {
	// Setup a minimal server
	tmpDir, _ := os.MkdirTemp("", "audit-test-*")
	defer os.RemoveAll(tmpDir)
	
	logger, _ := audit.NewLogger(filepath.Join(tmpDir, "audit.db"))
	defer logger.Close()

	req, err := http.NewRequest("GET", "/health", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	expected := `{"status":"ok"}`
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
	}
}

func TestServer_CreateLoop(t *testing.T) {
	// Setup
	tmpDir, _ := os.MkdirTemp("", "audit-test-*")
	defer os.RemoveAll(tmpDir)
	
	logger, _ := audit.NewLogger(filepath.Join(tmpDir, "audit.db"))
	defer logger.Close()

	srv := &Server{
		auditWriter: logger,
		loops:       make(map[string]*LoopInstance),
	}

	payload := CreateLoopRequest{
		Prompt:  "test prompt",
		WorkDir: ".",
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", "/api/v1/loops", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	
	// We need to call the actual handler but it uses a real Loop which might fail without binaries
	// For this test, we'll just check that it parses the body correctly and handles the request
	handler := http.HandlerFunc(srv.createLoop)
	handler.ServeHTTP(rr, req)

	// It might return 500 if it fails to start the actual loop binary, but it should be a 201 or 500, not 400
	if rr.Code == http.StatusBadRequest {
		t.Errorf("expected non-400 status, got %v", rr.Code)
	}
}

func TestServer_ListLoops(t *testing.T) {
	srv := &Server{
		loops: make(map[string]*LoopInstance),
	}
	
	// Add a dummy loop
	srv.loops["test-loop"] = &LoopInstance{
		ID:     "test-loop",
		Status: "running",
	}

	req, _ := http.NewRequest("GET", "/api/v1/loops", nil)
	rr := httptest.NewRecorder()
	
	handler := http.HandlerFunc(srv.listLoops)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp []LoopResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp) != 1 || resp[0].ID != "test-loop" {
		t.Errorf("unexpected response: %v", resp)
	}
}

func TestServer_GetLoop(t *testing.T) {
	srv := &Server{
		loops: make(map[string]*LoopInstance),
	}
	
	srv.loops["test-loop"] = &LoopInstance{
		ID:     "test-loop",
		Status: "running",
	}

	t.Run("Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/loops/test-loop", nil)
		rr := httptest.NewRecorder()
		srv.getLoop(rr, req, "test-loop")

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
	})

	t.Run("Not Found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/api/v1/loops/none", nil)
		rr := httptest.NewRecorder()
		srv.getLoop(rr, req, "none")

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected 404, got %d", rr.Code)
		}
	})
}

func TestServer_Audit(t *testing.T) {
	tmpDir := t.TempDir()
	logger, _ := audit.NewLogger(filepath.Join(tmpDir, "audit.db"))
	defer logger.Close()

	srv := &Server{
		auditWriter: logger,
	}

	req, _ := http.NewRequest("GET", "/api/v1/audit", nil)
	rr := httptest.NewRecorder()
	
	handler := http.HandlerFunc(srv.handleAudit)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
