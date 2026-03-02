package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/openexec/openexec/internal/audit"
	"github.com/openexec/openexec/internal/db/session"
)

// UsageServer provides HTTP endpoints for cost and token usage tracking.
type UsageServer struct {
	auditLogger audit.Logger
	sessionRepo session.Repository
	mux         *http.ServeMux
	server      *http.Server
}

// UsageServerConfig configures the usage API server.
type UsageServerConfig struct {
	AuditLogger audit.Logger
	SessionRepo session.Repository
	Addr        string
}

// NewUsageServer creates a new Usage API server.
func NewUsageServer(cfg UsageServerConfig) *UsageServer {
	mux := http.NewServeMux()
	s := &UsageServer{
		auditLogger: cfg.AuditLogger,
		sessionRepo: cfg.SessionRepo,
		mux:         mux,
		server: &http.Server{
			Addr:    cfg.Addr,
			Handler: mux,
		},
	}
	s.registerRoutes()
	return s
}

func (s *UsageServer) registerRoutes() {
	// Usage summary endpoint - overall platform usage
	s.mux.HandleFunc("GET /api/usage/summary", s.handleGetUsageSummary)

	// Provider usage breakdown
	s.mux.HandleFunc("GET /api/usage/providers", s.handleGetProviderUsage)

	// Session-specific usage
	s.mux.HandleFunc("GET /api/usage/sessions/{sessionID}", s.handleGetSessionUsage)

	// Tool call statistics
	s.mux.HandleFunc("GET /api/usage/tools", s.handleGetToolCallStats)

	// Audit log entries (raw usage events)
	s.mux.HandleFunc("GET /api/usage/audit-logs", s.handleGetAuditLogs)

	// Cost breakdown by model
	s.mux.HandleFunc("GET /api/usage/cost-by-model", s.handleGetCostByModel)
}

// Handler returns the HTTP handler for testing without a listener.
func (s *UsageServer) Handler() http.Handler {
	return s.mux
}

// UsageSummaryResponse represents the overall usage summary.
type UsageSummaryResponse struct {
	TotalTokensInput    int64              `json:"total_tokens_input"`
	TotalTokensOutput   int64              `json:"total_tokens_output"`
	TotalTokens         int64              `json:"total_tokens"`
	TotalCostUSD        float64            `json:"total_cost_usd"`
	TotalRequests       int64              `json:"total_requests"`
	SuccessfulRequests  int64              `json:"successful_requests"`
	FailedRequests      int64              `json:"failed_requests"`
	AverageDurationMs   float64            `json:"average_duration_ms"`
	ByProvider          map[string]*ProviderStatsResponse `json:"by_provider,omitempty"`
	Period              *TimePeriod        `json:"period,omitempty"`
}

// ProviderStatsResponse represents usage stats for a provider.
type ProviderStatsResponse struct {
	Provider          string  `json:"provider"`
	TotalTokensInput  int64   `json:"total_tokens_input"`
	TotalTokensOutput int64   `json:"total_tokens_output"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	TotalRequests     int64   `json:"total_requests"`
}

// TimePeriod represents a time range filter.
type TimePeriod struct {
	Since time.Time `json:"since,omitempty"`
	Until time.Time `json:"until,omitempty"`
}

// handleGetUsageSummary returns the overall platform usage statistics.
func (s *UsageServer) handleGetUsageSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse time range filters
	filter := parseTimeFilter(r)

	stats, err := s.auditLogger.GetUsageStats(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get usage stats: "+err.Error())
		return
	}

	response := UsageSummaryResponse{
		TotalTokensInput:   stats.TotalTokensInput,
		TotalTokensOutput:  stats.TotalTokensOutput,
		TotalTokens:        stats.TotalTokensInput + stats.TotalTokensOutput,
		TotalCostUSD:       stats.TotalCostUSD,
		TotalRequests:      stats.TotalRequests,
		SuccessfulRequests: stats.SuccessfulRequests,
		FailedRequests:     stats.FailedRequests,
		AverageDurationMs:  stats.AverageDurationMs,
	}

	// Convert provider stats
	if len(stats.ByProvider) > 0 {
		response.ByProvider = make(map[string]*ProviderStatsResponse)
		for name, ps := range stats.ByProvider {
			response.ByProvider[name] = &ProviderStatsResponse{
				Provider:          ps.Provider,
				TotalTokensInput:  ps.TotalTokensInput,
				TotalTokensOutput: ps.TotalTokensOutput,
				TotalCostUSD:      ps.TotalCostUSD,
				TotalRequests:     ps.TotalRequests,
			}
		}
	}

	// Add time period info if filters were applied
	if !filter.Since.IsZero() || !filter.Until.IsZero() {
		response.Period = &TimePeriod{
			Since: filter.Since,
			Until: filter.Until,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// ProviderUsageResponse represents provider-level usage data.
type ProviderUsageResponse struct {
	Providers []*session.ProviderUsage `json:"providers"`
	Total     *TotalUsage              `json:"total"`
}

// TotalUsage represents aggregated totals across all providers.
type TotalUsage struct {
	SessionCount      int     `json:"session_count"`
	MessageCount      int     `json:"message_count"`
	TotalTokensInput  int     `json:"total_tokens_input"`
	TotalTokensOutput int     `json:"total_tokens_output"`
	TotalTokens       int     `json:"total_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
}

// handleGetProviderUsage returns usage statistics grouped by provider.
func (s *UsageServer) handleGetProviderUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providers, err := s.sessionRepo.GetUsageByProvider(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get provider usage: "+err.Error())
		return
	}

	// Calculate totals
	total := &TotalUsage{}
	for _, p := range providers {
		total.SessionCount += p.SessionCount
		total.MessageCount += p.MessageCount
		total.TotalTokensInput += p.TotalTokensInput
		total.TotalTokensOutput += p.TotalTokensOutput
		total.TotalCostUSD += p.TotalCostUSD
	}
	total.TotalTokens = total.TotalTokensInput + total.TotalTokensOutput

	response := ProviderUsageResponse{
		Providers: providers,
		Total:     total,
	}

	writeJSON(w, http.StatusOK, response)
}

// SessionUsageResponse represents usage for a specific session.
type SessionUsageResponse struct {
	SessionID         string  `json:"session_id"`
	MessageCount      int     `json:"message_count"`
	ToolCallCount     int     `json:"tool_call_count"`
	TotalTokensInput  int     `json:"total_tokens_input"`
	TotalTokensOutput int     `json:"total_tokens_output"`
	TotalTokens       int     `json:"total_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	SummaryCount      int     `json:"summary_count"`
	TokensSaved       int     `json:"tokens_saved"`
}

// handleGetSessionUsage returns usage statistics for a specific session.
func (s *UsageServer) handleGetSessionUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("sessionID")

	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "missing session_id")
		return
	}

	stats, err := s.sessionRepo.GetSessionStats(ctx, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get session stats: "+err.Error())
		return
	}

	response := SessionUsageResponse{
		SessionID:         stats.SessionID,
		MessageCount:      stats.MessageCount,
		ToolCallCount:     stats.ToolCallCount,
		TotalTokensInput:  stats.TotalTokensInput,
		TotalTokensOutput: stats.TotalTokensOutput,
		TotalTokens:       stats.TotalTokensInput + stats.TotalTokensOutput,
		TotalCostUSD:      stats.TotalCostUSD,
		SummaryCount:      stats.SummaryCount,
		TokensSaved:       stats.TokensSaved,
	}

	writeJSON(w, http.StatusOK, response)
}

// ToolCallStatsResponse represents aggregated tool call statistics.
type ToolCallStatsResponse struct {
	TotalRequested    int64            `json:"total_requested"`
	TotalApproved     int64            `json:"total_approved"`
	TotalRejected     int64            `json:"total_rejected"`
	TotalAutoApproved int64            `json:"total_auto_approved"`
	TotalCompleted    int64            `json:"total_completed"`
	TotalFailed       int64            `json:"total_failed"`
	ByTool            map[string]int64 `json:"by_tool,omitempty"`
	Period            *TimePeriod      `json:"period,omitempty"`
}

// handleGetToolCallStats returns aggregated tool call statistics.
func (s *UsageServer) handleGetToolCallStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse time range filters
	filter := parseTimeFilter(r)

	stats, err := s.auditLogger.GetToolCallStats(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tool call stats: "+err.Error())
		return
	}

	response := ToolCallStatsResponse{
		TotalRequested:    stats.TotalRequested,
		TotalApproved:     stats.TotalApproved,
		TotalRejected:     stats.TotalRejected,
		TotalAutoApproved: stats.TotalAutoApproved,
		TotalCompleted:    stats.TotalCompleted,
		TotalFailed:       stats.TotalFailed,
		ByTool:            stats.ByTool,
	}

	// Add time period info if filters were applied
	if !filter.Since.IsZero() || !filter.Until.IsZero() {
		response.Period = &TimePeriod{
			Since: filter.Since,
			Until: filter.Until,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

// AuditLogEntry represents a simplified audit log entry for the API response.
type AuditLogEntry struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	EventType    string    `json:"event_type"`
	Severity     string    `json:"severity"`
	SessionID    string    `json:"session_id,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	Model        string    `json:"model,omitempty"`
	TokensInput  int64     `json:"tokens_input,omitempty"`
	TokensOutput int64     `json:"tokens_output,omitempty"`
	CostUSD      float64   `json:"cost_usd,omitempty"`
	DurationMs   int64     `json:"duration_ms,omitempty"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
}

// AuditLogResponse represents paginated audit log entries.
type AuditLogResponse struct {
	Entries    []*AuditLogEntry `json:"entries"`
	TotalCount int64            `json:"total_count"`
	HasMore    bool             `json:"has_more"`
	Limit      int              `json:"limit"`
	Offset     int              `json:"offset"`
}

// handleGetAuditLogs returns paginated audit log entries.
func (s *UsageServer) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	filter := parseAuditFilter(r)

	result, err := s.auditLogger.Query(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query audit logs: "+err.Error())
		return
	}

	// Convert to API response format
	entries := make([]*AuditLogEntry, 0, len(result.Entries))
	for _, e := range result.Entries {
		entry := &AuditLogEntry{
			ID:        e.ID,
			Timestamp: e.Timestamp,
			EventType: string(e.EventType),
			Severity:  string(e.Severity),
		}

		if e.SessionID.Valid {
			entry.SessionID = e.SessionID.String
		}
		if e.Provider.Valid {
			entry.Provider = e.Provider.String
		}
		if e.Model.Valid {
			entry.Model = e.Model.String
		}
		if e.TokensInput.Valid {
			entry.TokensInput = e.TokensInput.Int64
		}
		if e.TokensOutput.Valid {
			entry.TokensOutput = e.TokensOutput.Int64
		}
		if e.CostUSD.Valid {
			entry.CostUSD = e.CostUSD.Float64
		}
		if e.DurationMs.Valid {
			entry.DurationMs = e.DurationMs.Int64
		}
		if e.Success.Valid {
			entry.Success = e.Success.Bool
		}
		if e.ErrorMessage.Valid {
			entry.ErrorMessage = e.ErrorMessage.String
		}

		entries = append(entries, entry)
	}

	response := AuditLogResponse{
		Entries:    entries,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	}

	writeJSON(w, http.StatusOK, response)
}

// ModelCostEntry represents cost breakdown for a specific model.
type ModelCostEntry struct {
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	SessionCount      int     `json:"session_count"`
	MessageCount      int     `json:"message_count"`
	TotalTokensInput  int     `json:"total_tokens_input"`
	TotalTokensOutput int     `json:"total_tokens_output"`
	TotalTokens       int     `json:"total_tokens"`
	TotalCostUSD      float64 `json:"total_cost_usd"`
	PercentageOfTotal float64 `json:"percentage_of_total"`
}

// CostByModelResponse represents cost breakdown by model.
type CostByModelResponse struct {
	Models       []*ModelCostEntry `json:"models"`
	TotalCostUSD float64           `json:"total_cost_usd"`
}

// handleGetCostByModel returns cost breakdown by model.
func (s *UsageServer) handleGetCostByModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providers, err := s.sessionRepo.GetUsageByProvider(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get cost by model: "+err.Error())
		return
	}

	// Calculate total cost
	var totalCost float64
	for _, p := range providers {
		totalCost += p.TotalCostUSD
	}

	// Build model entries with percentages
	models := make([]*ModelCostEntry, 0, len(providers))
	for _, p := range providers {
		percentage := 0.0
		if totalCost > 0 {
			percentage = (p.TotalCostUSD / totalCost) * 100
		}

		models = append(models, &ModelCostEntry{
			Provider:          p.Provider,
			Model:             p.Model,
			SessionCount:      p.SessionCount,
			MessageCount:      p.MessageCount,
			TotalTokensInput:  p.TotalTokensInput,
			TotalTokensOutput: p.TotalTokensOutput,
			TotalTokens:       p.TotalTokensInput + p.TotalTokensOutput,
			TotalCostUSD:      p.TotalCostUSD,
			PercentageOfTotal: percentage,
		})
	}

	response := CostByModelResponse{
		Models:       models,
		TotalCostUSD: totalCost,
	}

	writeJSON(w, http.StatusOK, response)
}

// Helper functions

// parseTimeFilter extracts time range parameters from the request.
func parseTimeFilter(r *http.Request) *audit.QueryFilter {
	filter := &audit.QueryFilter{}

	// Parse since/until timestamps
	if since := r.URL.Query().Get("since"); since != "" {
		if t, err := time.Parse(time.RFC3339, since); err == nil {
			filter.Since = t
		}
	}

	if until := r.URL.Query().Get("until"); until != "" {
		if t, err := time.Parse(time.RFC3339, until); err == nil {
			filter.Until = t
		}
	}

	// Parse session filter
	if sessionID := r.URL.Query().Get("session_id"); sessionID != "" {
		filter.SessionID = sessionID
	}

	return filter
}

// parseAuditFilter extracts all query parameters for audit log queries.
func parseAuditFilter(r *http.Request) *audit.QueryFilter {
	filter := parseTimeFilter(r)

	// Parse event types
	if eventTypes := r.URL.Query()["event_type"]; len(eventTypes) > 0 {
		for _, et := range eventTypes {
			filter.EventTypes = append(filter.EventTypes, audit.EventType(et))
		}
	}

	// Parse pagination
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
			filter.Limit = limit
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 100 // Default limit
	}
	if filter.Limit > 1000 {
		filter.Limit = 1000 // Max limit
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	return filter
}

// RegisterUsageRoutes adds usage API routes to an existing server mux.
// This allows integration with the main API server.
func RegisterUsageRoutes(mux *http.ServeMux, auditLogger audit.Logger, sessionRepo session.Repository) {
	handler := &usageHandler{
		auditLogger: auditLogger,
		sessionRepo: sessionRepo,
	}

	mux.HandleFunc("GET /api/usage/summary", handler.handleGetUsageSummary)
	mux.HandleFunc("GET /api/usage/providers", handler.handleGetProviderUsage)
	mux.HandleFunc("GET /api/usage/sessions/{sessionID}", handler.handleGetSessionUsage)
	mux.HandleFunc("GET /api/usage/tools", handler.handleGetToolCallStats)
	mux.HandleFunc("GET /api/usage/audit-logs", handler.handleGetAuditLogs)
	mux.HandleFunc("GET /api/usage/cost-by-model", handler.handleGetCostByModel)
}

// usageHandler is the internal handler for usage routes when integrated with the main server.
type usageHandler struct {
	auditLogger audit.Logger
	sessionRepo session.Repository
}

func (h *usageHandler) handleGetUsageSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter := parseTimeFilter(r)

	stats, err := h.auditLogger.GetUsageStats(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get usage stats: "+err.Error())
		return
	}

	response := UsageSummaryResponse{
		TotalTokensInput:   stats.TotalTokensInput,
		TotalTokensOutput:  stats.TotalTokensOutput,
		TotalTokens:        stats.TotalTokensInput + stats.TotalTokensOutput,
		TotalCostUSD:       stats.TotalCostUSD,
		TotalRequests:      stats.TotalRequests,
		SuccessfulRequests: stats.SuccessfulRequests,
		FailedRequests:     stats.FailedRequests,
		AverageDurationMs:  stats.AverageDurationMs,
	}

	if len(stats.ByProvider) > 0 {
		response.ByProvider = make(map[string]*ProviderStatsResponse)
		for name, ps := range stats.ByProvider {
			response.ByProvider[name] = &ProviderStatsResponse{
				Provider:          ps.Provider,
				TotalTokensInput:  ps.TotalTokensInput,
				TotalTokensOutput: ps.TotalTokensOutput,
				TotalCostUSD:      ps.TotalCostUSD,
				TotalRequests:     ps.TotalRequests,
			}
		}
	}

	if !filter.Since.IsZero() || !filter.Until.IsZero() {
		response.Period = &TimePeriod{
			Since: filter.Since,
			Until: filter.Until,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *usageHandler) handleGetProviderUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providers, err := h.sessionRepo.GetUsageByProvider(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get provider usage: "+err.Error())
		return
	}

	total := &TotalUsage{}
	for _, p := range providers {
		total.SessionCount += p.SessionCount
		total.MessageCount += p.MessageCount
		total.TotalTokensInput += p.TotalTokensInput
		total.TotalTokensOutput += p.TotalTokensOutput
		total.TotalCostUSD += p.TotalCostUSD
	}
	total.TotalTokens = total.TotalTokensInput + total.TotalTokensOutput

	response := ProviderUsageResponse{
		Providers: providers,
		Total:     total,
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *usageHandler) handleGetSessionUsage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessionID := r.PathValue("sessionID")

	if sessionID == "" {
		writeError(w, http.StatusBadRequest, "missing session_id")
		return
	}

	stats, err := h.sessionRepo.GetSessionStats(ctx, sessionID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get session stats: "+err.Error())
		return
	}

	response := SessionUsageResponse{
		SessionID:         stats.SessionID,
		MessageCount:      stats.MessageCount,
		ToolCallCount:     stats.ToolCallCount,
		TotalTokensInput:  stats.TotalTokensInput,
		TotalTokensOutput: stats.TotalTokensOutput,
		TotalTokens:       stats.TotalTokensInput + stats.TotalTokensOutput,
		TotalCostUSD:      stats.TotalCostUSD,
		SummaryCount:      stats.SummaryCount,
		TokensSaved:       stats.TokensSaved,
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *usageHandler) handleGetToolCallStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter := parseTimeFilter(r)

	stats, err := h.auditLogger.GetToolCallStats(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get tool call stats: "+err.Error())
		return
	}

	response := ToolCallStatsResponse{
		TotalRequested:    stats.TotalRequested,
		TotalApproved:     stats.TotalApproved,
		TotalRejected:     stats.TotalRejected,
		TotalAutoApproved: stats.TotalAutoApproved,
		TotalCompleted:    stats.TotalCompleted,
		TotalFailed:       stats.TotalFailed,
		ByTool:            stats.ByTool,
	}

	if !filter.Since.IsZero() || !filter.Until.IsZero() {
		response.Period = &TimePeriod{
			Since: filter.Since,
			Until: filter.Until,
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *usageHandler) handleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	filter := parseAuditFilter(r)

	result, err := h.auditLogger.Query(ctx, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to query audit logs: "+err.Error())
		return
	}

	entries := make([]*AuditLogEntry, 0, len(result.Entries))
	for _, e := range result.Entries {
		entry := &AuditLogEntry{
			ID:        e.ID,
			Timestamp: e.Timestamp,
			EventType: string(e.EventType),
			Severity:  string(e.Severity),
		}

		if e.SessionID.Valid {
			entry.SessionID = e.SessionID.String
		}
		if e.Provider.Valid {
			entry.Provider = e.Provider.String
		}
		if e.Model.Valid {
			entry.Model = e.Model.String
		}
		if e.TokensInput.Valid {
			entry.TokensInput = e.TokensInput.Int64
		}
		if e.TokensOutput.Valid {
			entry.TokensOutput = e.TokensOutput.Int64
		}
		if e.CostUSD.Valid {
			entry.CostUSD = e.CostUSD.Float64
		}
		if e.DurationMs.Valid {
			entry.DurationMs = e.DurationMs.Int64
		}
		if e.Success.Valid {
			entry.Success = e.Success.Bool
		}
		if e.ErrorMessage.Valid {
			entry.ErrorMessage = e.ErrorMessage.String
		}

		entries = append(entries, entry)
	}

	response := AuditLogResponse{
		Entries:    entries,
		TotalCount: result.TotalCount,
		HasMore:    result.HasMore,
		Limit:      filter.Limit,
		Offset:     filter.Offset,
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *usageHandler) handleGetCostByModel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	providers, err := h.sessionRepo.GetUsageByProvider(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get cost by model: "+err.Error())
		return
	}

	var totalCost float64
	for _, p := range providers {
		totalCost += p.TotalCostUSD
	}

	models := make([]*ModelCostEntry, 0, len(providers))
	for _, p := range providers {
		percentage := 0.0
		if totalCost > 0 {
			percentage = (p.TotalCostUSD / totalCost) * 100
		}

		models = append(models, &ModelCostEntry{
			Provider:          p.Provider,
			Model:             p.Model,
			SessionCount:      p.SessionCount,
			MessageCount:      p.MessageCount,
			TotalTokensInput:  p.TotalTokensInput,
			TotalTokensOutput: p.TotalTokensOutput,
			TotalTokens:       p.TotalTokensInput + p.TotalTokensOutput,
			TotalCostUSD:      p.TotalCostUSD,
			PercentageOfTotal: percentage,
		})
	}

	response := CostByModelResponse{
		Models:       models,
		TotalCostUSD: totalCost,
	}

	writeJSON(w, http.StatusOK, response)
}

// Utility function for encoding JSON responses (reuses existing from handlers.go)
func usageWriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
