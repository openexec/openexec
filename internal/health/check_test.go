package health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChecker(t *testing.T) {
	c := NewChecker()

	// 1. Initial state
	if c.IsReady() {
		t.Error("expected initial ready state to be false")
	}

	// 2. Register and run checks
	c.Register(Check{
		Name:     "good",
		Critical: true,
		Run: func(ctx context.Context) (Status, string, error) {
			return StatusOK, "all good", nil
		},
	})

	c.Register(Check{
		Name:     "degraded",
		Critical: false,
		Run: func(ctx context.Context) (Status, string, error) {
			return StatusDegraded, "minor issue", nil
		},
	})

	ctx := context.Background()
	err := c.RunPreflight(ctx)
	if err != nil {
		t.Fatalf("RunPreflight failed: %v", err)
	}

	if !c.IsReady() {
		t.Error("expected ready state after successful critical checks")
	}

	overall, results := c.GetStatus()
	if overall != StatusDegraded {
		t.Errorf("got status %q, want %q", overall, StatusDegraded)
	}

	if len(results) != 2 {
		t.Errorf("got %d results, want 2", len(results))
	}

	// 3. Critical failure
	c.Register(Check{
		Name:     "failed_critical",
		Critical: true,
		Run: func(ctx context.Context) (Status, string, error) {
			return StatusFailed, "critical failure", nil
		},
	})

	err = c.RunPreflight(ctx)
	if err == nil {
		t.Error("expected error from failed critical check")
	}

	if c.IsReady() {
		t.Error("expected ready state to be false after critical failure")
	}

	overall, _ = c.GetStatus()
	if overall != StatusFailed {
		t.Errorf("got status %q, want %q", overall, StatusFailed)
	}
}

func TestCheckerUpdate(t *testing.T) {
	c := NewChecker()
	c.Register(Check{
		Name:     "dynamic",
		Critical: true,
		Run: func(ctx context.Context) (Status, string, error) {
			return StatusOK, "ok", nil
		},
	})

	_ = c.RunPreflight(context.Background())
	if !c.IsReady() {
		t.Fatal("should be ready")
	}

	c.UpdateCheck("dynamic", StatusFailed, "something broke")
	if c.IsReady() {
		t.Error("expected not ready after updating critical check to failed")
	}
}

func TestHealthHandlers(t *testing.T) {
	c := NewChecker()
	c.Register(Check{
		Name: "test",
		Run: func(ctx context.Context) (Status, string, error) {
			return StatusOK, "ok", nil
		},
	})
	_ = c.RunPreflight(context.Background())

	// Test ready handler
	req := httptest.NewRequest("GET", "/ready", nil)
	rr := httptest.NewRecorder()
	c.ReadyHandler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}

	// Test health handler
	req = httptest.NewRequest("GET", "/health?detailed=true", nil)
	rr = httptest.NewRecorder()
	c.Handler(false, "1.0.0").ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
	if !strings.Contains(rr.Body.String(), `"status":"ok"`) {
		t.Errorf("body missing status ok: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"test"`) {
		t.Errorf("body missing check result: %s", rr.Body.String())
	}
}

func TestCheckHTTPEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	check := CheckHTTPEndpoint("server", server.URL, time.Second, true)
	status, msg, err := check.Run(context.Background())

	if err != nil {
		t.Fatalf("check failed with error: %v", err)
	}
	if status != StatusOK {
		t.Errorf("got status %q, want %q", status, StatusOK)
	}
	if !strings.Contains(msg, "reachable") {
		t.Errorf("unexpected message: %q", msg)
	}

	// Error case
	checkErr := CheckHTTPEndpoint("bad", "http://localhost:12345", 100*time.Millisecond, true)
	status, _, _ = checkErr.Run(context.Background())
	if status != StatusFailed {
		t.Errorf("expected failed status for unreachable endpoint, got %q", status)
	}
}

func TestPreflightError(t *testing.T) {
	c := NewChecker()
	c.Register(Check{
		Name:     "error_run",
		Critical: true,
		Run: func(ctx context.Context) (Status, string, error) {
			return "", "", errors.New("execution error")
		},
	})

	err := c.RunPreflight(context.Background())
	if err == nil {
		t.Fatal("expected error from failed preflight execution")
	}

	_, results := c.GetStatus()
	if results["error_run"].Status != StatusFailed {
		t.Errorf("got status %q, want %q", results["error_run"].Status, StatusFailed)
	}
}
