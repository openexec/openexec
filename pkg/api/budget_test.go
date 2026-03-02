package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openexec/openexec/internal/budget"
)

func TestBudgetHandlerGetStatus(t *testing.T) {
	cfg := budget.DefaultConfig()
	cfg.Enabled = true

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	req := httptest.NewRequest("GET", "/api/budget/status", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", rec.Code, http.StatusOK)
	}

	var response BudgetStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status == nil {
		t.Error("Status should not be nil")
	}

	if response.Config == nil {
		t.Error("Config should not be nil")
	}

	if !response.Config.Enabled {
		t.Error("Config.Enabled should be true")
	}
}

func TestBudgetHandlerGetConfig(t *testing.T) {
	cfg := &budget.Config{
		Enabled:           true,
		TotalBudgetUSD:    200,
		SessionBudgetUSD:  20,
		DailyBudgetUSD:    50,
		WarningThreshold:  0.75,
		CriticalThreshold: 0.90,
		BlockOnExceed:     true,
		AlertChannels:     []string{"telegram", "audit"},
	}

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	req := httptest.NewRequest("GET", "/api/budget/config", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", rec.Code, http.StatusOK)
	}

	var response BudgetConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.TotalBudgetUSD != 200 {
		t.Errorf("TotalBudgetUSD = %v, want 200", response.TotalBudgetUSD)
	}

	if response.SessionBudgetUSD != 20 {
		t.Errorf("SessionBudgetUSD = %v, want 20", response.SessionBudgetUSD)
	}

	if response.WarningThreshold != 0.75 {
		t.Errorf("WarningThreshold = %v, want 0.75", response.WarningThreshold)
	}

	if len(response.AlertChannels) != 2 {
		t.Errorf("AlertChannels count = %v, want 2", len(response.AlertChannels))
	}
}

func TestBudgetHandlerUpdateConfig(t *testing.T) {
	cfg := budget.DefaultConfig()
	cfg.Enabled = true

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	// Prepare update request
	totalBudget := 300.0
	updateReq := BudgetConfigUpdateRequest{
		TotalBudgetUSD: &totalBudget,
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("PUT", "/api/budget/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var response BudgetConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.TotalBudgetUSD != 300 {
		t.Errorf("TotalBudgetUSD = %v, want 300", response.TotalBudgetUSD)
	}

	// Verify the monitor's config was actually updated
	monitorCfg := monitor.Config()
	if monitorCfg.TotalBudgetUSD != 300 {
		t.Errorf("Monitor TotalBudgetUSD = %v, want 300", monitorCfg.TotalBudgetUSD)
	}
}

func TestBudgetHandlerUpdateConfigPartial(t *testing.T) {
	cfg := &budget.Config{
		Enabled:           true,
		TotalBudgetUSD:    100,
		SessionBudgetUSD:  10,
		DailyBudgetUSD:    25,
		WarningThreshold:  0.8,
		CriticalThreshold: 0.95,
	}

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	// Only update session budget
	sessionBudget := 15.0
	updateReq := BudgetConfigUpdateRequest{
		SessionBudgetUSD: &sessionBudget,
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("PUT", "/api/budget/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", rec.Code, http.StatusOK)
	}

	var response BudgetConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Session budget should be updated
	if response.SessionBudgetUSD != 15 {
		t.Errorf("SessionBudgetUSD = %v, want 15", response.SessionBudgetUSD)
	}

	// Other fields should remain unchanged
	if response.TotalBudgetUSD != 100 {
		t.Errorf("TotalBudgetUSD = %v, want 100 (unchanged)", response.TotalBudgetUSD)
	}
	if response.DailyBudgetUSD != 25 {
		t.Errorf("DailyBudgetUSD = %v, want 25 (unchanged)", response.DailyBudgetUSD)
	}
}

func TestBudgetHandlerUpdateConfigInvalid(t *testing.T) {
	cfg := budget.DefaultConfig()
	cfg.Enabled = true

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	// Try to set warning > critical
	warning := 0.99
	critical := 0.5
	updateReq := BudgetConfigUpdateRequest{
		WarningThreshold:  &warning,
		CriticalThreshold: &critical,
	}
	body, _ := json.Marshal(updateReq)

	req := httptest.NewRequest("PUT", "/api/budget/config", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %v, want %v", rec.Code, http.StatusBadRequest)
	}
}

func TestBudgetHandlerResetAlerts(t *testing.T) {
	cfg := budget.DefaultConfig()
	cfg.Enabled = true

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	req := httptest.NewRequest("POST", "/api/budget/reset-alerts", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %v, want %v", rec.Code, http.StatusOK)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "alert cooldowns reset" {
		t.Errorf("status = %q, want 'alert cooldowns reset'", response["status"])
	}
}

func TestBudgetHandlerNoMonitor(t *testing.T) {
	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, nil)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/budget/status"},
		{"GET", "/api/budget/config"},
		{"PUT", "/api/budget/config"},
		{"POST", "/api/budget/reset-alerts"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			var req *http.Request
			if ep.method == "PUT" {
				req = httptest.NewRequest(ep.method, ep.path, bytes.NewReader([]byte("{}")))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(ep.method, ep.path, nil)
			}
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusServiceUnavailable {
				t.Errorf("Status code = %v, want %v", rec.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestBudgetHandlerInvalidJSON(t *testing.T) {
	cfg := budget.DefaultConfig()
	cfg.Enabled = true

	monitor, err := budget.NewMonitor(cfg, nil)
	if err != nil {
		t.Fatalf("NewMonitor() error = %v", err)
	}

	mux := http.NewServeMux()
	RegisterBudgetRoutes(mux, monitor)

	req := httptest.NewRequest("PUT", "/api/budget/config", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("Status code = %v, want %v", rec.Code, http.StatusBadRequest)
	}
}
