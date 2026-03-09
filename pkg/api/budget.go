package api

import (
	"encoding/json"
	"net/http"

	"github.com/openexec/openexec/internal/budget"
)

// BudgetHandler provides HTTP endpoints for budget monitoring.
type BudgetHandler struct {
	monitor *budget.Monitor
}

// NewBudgetHandler creates a new budget handler.
func NewBudgetHandler(monitor *budget.Monitor) *BudgetHandler {
	return &BudgetHandler{
		monitor: monitor,
	}
}

// RegisterBudgetRoutes adds budget API routes to an existing server mux.
func RegisterBudgetRoutes(mux *http.ServeMux, monitor *budget.Monitor) {
	handler := NewBudgetHandler(monitor)

	mux.HandleFunc("GET /api/budget/status", handler.handleGetStatus)
	mux.HandleFunc("GET /api/budget/config", handler.handleGetConfig)
	mux.HandleFunc("PUT /api/budget/config", handler.handleUpdateConfig)
	mux.HandleFunc("POST /api/budget/reset-alerts", handler.handleResetAlerts)
}

// BudgetStatusResponse represents the budget status API response.
type BudgetStatusResponse struct {
	Status *budget.Status        `json:"status"`
	Config *BudgetConfigResponse `json:"config,omitempty"`
}

// BudgetConfigResponse represents the budget configuration for API responses.
type BudgetConfigResponse struct {
	Enabled           bool     `json:"enabled"`
	TotalBudgetUSD    float64  `json:"total_budget_usd"`
	SessionBudgetUSD  float64  `json:"session_budget_usd"`
	DailyBudgetUSD    float64  `json:"daily_budget_usd"`
	WarningThreshold  float64  `json:"warning_threshold"`
	CriticalThreshold float64  `json:"critical_threshold"`
	BlockOnExceed     bool     `json:"block_on_exceed"`
	AlertChannels     []string `json:"alert_channels"`
}

// handleGetStatus returns the current budget status.
func (h *BudgetHandler) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.monitor == nil {
		WriteError(w, http.StatusServiceUnavailable, "budget monitoring not configured")
		return
	}

	status, err := h.monitor.GetStatus(ctx)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to get budget status: "+err.Error())
		return
	}

	config := h.monitor.Config()
	response := BudgetStatusResponse{
		Status: status,
		Config: &BudgetConfigResponse{
			Enabled:           config.Enabled,
			TotalBudgetUSD:    config.TotalBudgetUSD,
			SessionBudgetUSD:  config.SessionBudgetUSD,
			DailyBudgetUSD:    config.DailyBudgetUSD,
			WarningThreshold:  config.WarningThreshold,
			CriticalThreshold: config.CriticalThreshold,
			BlockOnExceed:     config.BlockOnExceed,
			AlertChannels:     config.AlertChannels,
		},
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleGetConfig returns the current budget configuration.
func (h *BudgetHandler) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		WriteError(w, http.StatusServiceUnavailable, "budget monitoring not configured")
		return
	}

	config := h.monitor.Config()
	response := BudgetConfigResponse{
		Enabled:           config.Enabled,
		TotalBudgetUSD:    config.TotalBudgetUSD,
		SessionBudgetUSD:  config.SessionBudgetUSD,
		DailyBudgetUSD:    config.DailyBudgetUSD,
		WarningThreshold:  config.WarningThreshold,
		CriticalThreshold: config.CriticalThreshold,
		BlockOnExceed:     config.BlockOnExceed,
		AlertChannels:     config.AlertChannels,
	}

	WriteJSON(w, http.StatusOK, response)
}

// BudgetConfigUpdateRequest represents a request to update budget configuration.
type BudgetConfigUpdateRequest struct {
	Enabled           *bool    `json:"enabled,omitempty"`
	TotalBudgetUSD    *float64 `json:"total_budget_usd,omitempty"`
	SessionBudgetUSD  *float64 `json:"session_budget_usd,omitempty"`
	DailyBudgetUSD    *float64 `json:"daily_budget_usd,omitempty"`
	WarningThreshold  *float64 `json:"warning_threshold,omitempty"`
	CriticalThreshold *float64 `json:"critical_threshold,omitempty"`
	BlockOnExceed     *bool    `json:"block_on_exceed,omitempty"`
	AlertChannels     []string `json:"alert_channels,omitempty"`
}

// handleUpdateConfig updates the budget configuration.
func (h *BudgetHandler) handleUpdateConfig(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		WriteError(w, http.StatusServiceUnavailable, "budget monitoring not configured")
		return
	}

	var req BudgetConfigUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	// Get current config and apply updates
	currentConfig := h.monitor.Config()
	newConfig := &budget.Config{
		Enabled:           currentConfig.Enabled,
		TotalBudgetUSD:    currentConfig.TotalBudgetUSD,
		SessionBudgetUSD:  currentConfig.SessionBudgetUSD,
		DailyBudgetUSD:    currentConfig.DailyBudgetUSD,
		WarningThreshold:  currentConfig.WarningThreshold,
		CriticalThreshold: currentConfig.CriticalThreshold,
		BlockOnExceed:     currentConfig.BlockOnExceed,
		AlertCooldown:     currentConfig.AlertCooldown,
		AlertChannels:     currentConfig.AlertChannels,
	}

	// Apply partial updates
	if req.Enabled != nil {
		newConfig.Enabled = *req.Enabled
	}
	if req.TotalBudgetUSD != nil {
		newConfig.TotalBudgetUSD = *req.TotalBudgetUSD
	}
	if req.SessionBudgetUSD != nil {
		newConfig.SessionBudgetUSD = *req.SessionBudgetUSD
	}
	if req.DailyBudgetUSD != nil {
		newConfig.DailyBudgetUSD = *req.DailyBudgetUSD
	}
	if req.WarningThreshold != nil {
		newConfig.WarningThreshold = *req.WarningThreshold
	}
	if req.CriticalThreshold != nil {
		newConfig.CriticalThreshold = *req.CriticalThreshold
	}
	if req.BlockOnExceed != nil {
		newConfig.BlockOnExceed = *req.BlockOnExceed
	}
	if req.AlertChannels != nil {
		newConfig.AlertChannels = req.AlertChannels
	}

	// Update the configuration
	if err := h.monitor.UpdateConfig(newConfig); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid configuration: "+err.Error())
		return
	}

	response := BudgetConfigResponse{
		Enabled:           newConfig.Enabled,
		TotalBudgetUSD:    newConfig.TotalBudgetUSD,
		SessionBudgetUSD:  newConfig.SessionBudgetUSD,
		DailyBudgetUSD:    newConfig.DailyBudgetUSD,
		WarningThreshold:  newConfig.WarningThreshold,
		CriticalThreshold: newConfig.CriticalThreshold,
		BlockOnExceed:     newConfig.BlockOnExceed,
		AlertChannels:     newConfig.AlertChannels,
	}

	WriteJSON(w, http.StatusOK, response)
}

// handleResetAlerts resets alert cooldowns so alerts can be triggered again.
func (h *BudgetHandler) handleResetAlerts(w http.ResponseWriter, r *http.Request) {
	if h.monitor == nil {
		WriteError(w, http.StatusServiceUnavailable, "budget monitoring not configured")
		return
	}

	h.monitor.ResetCooldowns()

	WriteJSON(w, http.StatusOK, map[string]string{
		"status": "alert cooldowns reset",
	})
}
