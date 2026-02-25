package api

import (
	"bufio"
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

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/manager"
	"github.com/openexec/openexec/internal/pipeline"
)

func buildMockClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "mock_claude")
	src := filepath.Join("..", "loop", "testdata", "mock_claude.go")

	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build mock_claude: %v", err)
	}
	return bin
}

func mockBriefing() pipeline.BriefingFunc {
	return func(ctx context.Context, fwuID string) (string, error) {
		return "## FWU Briefing: " + fwuID + "\n\n**Status:** in_progress\n**Intent:** Test intent", nil
	}
}

func allPhasesConfig(scenario string) ([]pipeline.Phase, map[pipeline.Phase]pipeline.PhaseConfig) {
	order := pipeline.DefaultPhaseOrder()
	phases := map[pipeline.Phase]pipeline.PhaseConfig{
		pipeline.PhaseTD: {Agent: "test-agent", Workflow: "technical-design", CommandArgs: []string{scenario}},
		pipeline.PhaseIM: {Agent: "test-agent", Workflow: "implement", CommandArgs: []string{scenario}},
		pipeline.PhaseRV: {Agent: "test-agent", Workflow: "review", CommandArgs: []string{scenario}, Routes: map[string]pipeline.Phase{"spark": pipeline.PhaseIM, "hon": pipeline.PhaseRF}},
		pipeline.PhaseRF: {Agent: "test-agent", Workflow: "refactor", CommandArgs: []string{scenario}},
		pipeline.PhaseFL: {Agent: "test-agent", Workflow: "feedback-loop", CommandArgs: []string{scenario}},
	}
	return order, phases
}

func testManager(t *testing.T, bin string) *manager.Manager {
	t.Helper()
	order, phases := allPhasesConfig("signal-complete")
	return manager.New(manager.Config{
		WorkDir:              t.TempDir(),
		AgentsDir:            filepath.Join("..", "pipeline", "testdata"),
		Order:                order,
		Phases:               phases,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		MaxReviewCycles:      3,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		BriefingFunc:         mockBriefing(),
	})
}

func TestHandleStartSuccess(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, ":0")

	req := httptest.NewRequest("POST", "/api/fwu/FWU-01/start", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if body["fwu_id"] != "FWU-01" {
		t.Errorf("fwu_id = %s, want FWU-01", body["fwu_id"])
	}

	// Clean up: wait for completion.
	waitForTerminal(t, mgr, "FWU-01")
}

func TestHandleStartDuplicate(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("slow")
	mgr := manager.New(manager.Config{
		WorkDir:              t.TempDir(),
		AgentsDir:            filepath.Join("..", "pipeline", "testdata"),
		Order:                order,
		Phases:               phases,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		MaxReviewCycles:      3,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		BriefingFunc:         mockBriefing(),
	})
	srv := New(mgr, ":0")

	// Start pipeline with slow scenario to keep it running.
	mgr.Start(context.Background(), "FWU-01")
	time.Sleep(100 * time.Millisecond)

	req := httptest.NewRequest("POST", "/api/fwu/FWU-01/start", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusConflict)
	}

	mgr.Stop("FWU-01")
}

func TestHandleStatusFound(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, ":0")

	mgr.Start(context.Background(), "FWU-01")
	time.Sleep(50 * time.Millisecond)

	req := httptest.NewRequest("GET", "/api/fwu/FWU-01/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var info manager.PipelineInfo
	json.NewDecoder(rec.Body).Decode(&info)
	if info.FWUID != "FWU-01" {
		t.Errorf("fwu_id = %s, want FWU-01", info.FWUID)
	}

	waitForTerminal(t, mgr, "FWU-01")
}

func TestHandleStatusNotFound(t *testing.T) {
	mgr := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, ":0")

	req := httptest.NewRequest("GET", "/api/fwu/nonexistent/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleListEmpty(t *testing.T) {
	mgr := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, ":0")

	req := httptest.NewRequest("GET", "/api/fwus", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var list []manager.PipelineInfo
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 0 {
		t.Errorf("list length = %d, want 0", len(list))
	}
}

func TestHandleListWithPipelines(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, ":0")

	mgr.Start(context.Background(), "FWU-01")
	mgr.Start(context.Background(), "FWU-02")

	waitForTerminal(t, mgr, "FWU-01")
	waitForTerminal(t, mgr, "FWU-02")

	req := httptest.NewRequest("GET", "/api/fwus", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var list []manager.PipelineInfo
	json.NewDecoder(rec.Body).Decode(&list)
	if len(list) != 2 {
		t.Errorf("list length = %d, want 2", len(list))
	}
}

func TestHandlePauseFound(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("slow")
	mgr := manager.New(manager.Config{
		WorkDir:              t.TempDir(),
		AgentsDir:            filepath.Join("..", "pipeline", "testdata"),
		Order:                order,
		Phases:               phases,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		MaxReviewCycles:      3,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		BriefingFunc:         mockBriefing(),
	})
	srv := New(mgr, ":0")

	mgr.Start(context.Background(), "FWU-01")
	time.Sleep(100 * time.Millisecond)

	req := httptest.NewRequest("POST", "/api/fwu/FWU-01/pause", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	// Stop to clean up.
	time.Sleep(100 * time.Millisecond)
	mgr.Stop("FWU-01")
}

func TestHandlePauseNotFound(t *testing.T) {
	mgr := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, ":0")

	req := httptest.NewRequest("POST", "/api/fwu/nonexistent/pause", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleStopFound(t *testing.T) {
	bin := buildMockClaude(t)
	order, phases := allPhasesConfig("slow")
	mgr := manager.New(manager.Config{
		WorkDir:              t.TempDir(),
		AgentsDir:            filepath.Join("..", "pipeline", "testdata"),
		Order:                order,
		Phases:               phases,
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		MaxReviewCycles:      3,
		ThrashThreshold:      0,
		RetryBackoff:         []time.Duration{0},
		CommandName:          bin,
		BriefingFunc:         mockBriefing(),
	})
	srv := New(mgr, ":0")

	mgr.Start(context.Background(), "FWU-01")
	time.Sleep(100 * time.Millisecond)

	req := httptest.NewRequest("POST", "/api/fwu/FWU-01/stop", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHandleStopNotFound(t *testing.T) {
	mgr := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, ":0")

	req := httptest.NewRequest("POST", "/api/fwu/nonexistent/stop", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleEventsSSE(t *testing.T) {
	bin := buildMockClaude(t)
	mgr := testManager(t, bin)
	srv := New(mgr, ":0")

	// Use a real HTTP server for SSE (needs flusher).
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// Start a pipeline.
	mgr.Start(context.Background(), "FWU-01")

	// Connect SSE.
	resp, err := http.Get(ts.URL + "/api/fwu/FWU-01/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %s, want text/event-stream", ct)
	}

	// Read at least one SSE event.
	scanner := bufio.NewScanner(resp.Body)
	got := false
	deadline := time.After(30 * time.Second)
	done := make(chan struct{})

	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				var event loop.Event
				if json.Unmarshal([]byte(data), &event) == nil && event.Type != "" {
					got = true
					close(done)
					return
				}
			}
		}
	}()

	select {
	case <-done:
		if !got {
			t.Error("expected at least one SSE event")
		}
	case <-deadline:
		t.Fatal("timeout waiting for SSE event")
	}
}

func TestHandleEventsNotFound(t *testing.T) {
	mgr := manager.New(manager.Config{WorkDir: "/tmp"})
	srv := New(mgr, ":0")

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/fwu/nonexistent/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

// waitForTerminal polls until the pipeline reaches a terminal state.
func waitForTerminal(t *testing.T, mgr *manager.Manager, fwuID string) {
	t.Helper()
	deadline := time.After(30 * time.Second)
	for {
		info, err := mgr.Status(fwuID)
		if err != nil {
			t.Fatalf("Status(%s): %v", fwuID, err)
		}
		switch info.Status {
		case manager.StatusComplete, manager.StatusError, manager.StatusStopped:
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for %s to reach terminal state (current: %s)", fwuID, info.Status)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
