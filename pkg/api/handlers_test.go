package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
)

// Legacy FWU endpoint tests removed in Phase Four.
// Tests below use /api/v1/runs endpoints.

func buildMockClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mock_claude")
	src := filepath.Join("..", "..", "internal", "loop", "testdata", "mock_claude.go")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build mock_claude: %v", err)
	}
	return bin
}


func testManager(t *testing.T, bin string) *manager.Manager {
	t.Helper()
	workDir := t.TempDir()
	dbPath := filepath.Join(workDir, "test.db")
	stateStore, err := state.NewStore(dbPath)
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	t.Cleanup(func() { stateStore.Close() })

	mgr, err := manager.New(manager.Config{
		WorkDir:              workDir,
		AgentsFS:             os.DirFS(filepath.Join("..", "..", "internal", "pipeline", "testdata")),
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		StateStore:           stateStore,
	})
	if err != nil {
		t.Fatalf("manager.New: %v", err)
	}
	return mgr
}

func TestHandleStartRunSuccess(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, nil, nil, "", ":0")

	// Use v1 runs endpoint
	req := httptest.NewRequest("POST", "/api/v1/runs/RUN-01/start", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&body)
	if body["run_id"] != "RUN-01" {
		t.Errorf("run_id = %v, want RUN-01", body["run_id"])
	}

	// Clean up: wait for completion.
	waitForTerminal(t, mgr, "RUN-01")
}

func TestHandleStartRunDuplicate(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, nil, nil, "", ":0")

	// Start pipeline with slow scenario to keep it running.
	mgr.Start(context.Background(), "RUN-01")
	time.Sleep(100 * time.Millisecond)

	req := httptest.NewRequest("POST", "/api/v1/runs/RUN-01/start", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	mgr.Stop("RUN-01")
}

func TestHandleGetRunFound(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, nil, nil, "", ":0")

	mgr.Start(context.Background(), "RUN-01")
	time.Sleep(50 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/v1/runs/RUN-01", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var info map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&info)
	if info["run_id"] != "RUN-01" {
		t.Errorf("run_id = %v, want RUN-01", info["run_id"])
	}

	waitForTerminal(t, mgr, "RUN-01")
}

func TestHandleGetRunNotFound(t *testing.T) {
	mgr, _ := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, nil, nil, "", ":0")

	req := httptest.NewRequest("GET", "/api/v1/runs/nonexistent", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListRunsEmpty(t *testing.T) {
	mgr, _ := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, nil, nil, "", ":0")

	req := httptest.NewRequest("GET", "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	runs, ok := resp["runs"].([]interface{})
	if !ok {
		t.Errorf("expected runs array")
		return
	}
	if len(runs) != 0 {
		t.Errorf("list length = %d, want 0", len(runs))
	}
}

func TestHandleListRunsWithPipelines(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, nil, nil, "", ":0")

	mgr.Start(context.Background(), "RUN-01")
	mgr.Start(context.Background(), "RUN-02")

	waitForTerminal(t, mgr, "RUN-01")
	waitForTerminal(t, mgr, "RUN-02")

	req := httptest.NewRequest("GET", "/api/v1/runs", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	runs, ok := resp["runs"].([]interface{})
	if !ok {
		t.Errorf("expected runs array")
		return
	}
	if len(runs) != 2 {
		t.Errorf("list length = %d, want 2", len(runs))
	}
}

// waitForTerminal polls until the pipeline reaches a terminal state.
func waitForTerminal(t *testing.T, mgr *manager.Manager, runID string) {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		info, err := mgr.Status(runID)
		if err != nil {
			t.Fatalf("Status(%s): %v", runID, err)
		}
		switch info.Status {
		case manager.StatusComplete, manager.StatusError, manager.StatusStopped:
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s to reach terminal state (current: %s)", runID, info.Status)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
