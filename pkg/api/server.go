package api

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/openexec/openexec/internal/approval"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
)

// Server exposes the Manager and Session Repository over HTTP.
type Server struct {
	Mgr             *manager.Manager
	SessionRepo     session.Repository
	AuditLogger     audit.Logger
	StateStore      *state.Store // Unified state store for runs/steps/artifacts
	ApprovalGate    approval.ApprovalGate // Approval gate for tool execution
	UseUnifiedReads bool         // Default ON; set OPENEXEC_USE_UNIFIED_READS=0 to disable
	ProjectsDir     string
	Server          *http.Server
	Mux             *http.ServeMux
	Hub             *Hub
}

// ServerOption configures the Server.
type ServerOption func(*Server)

// WithStateStore sets the unified state store for database reads.
func WithStateStore(store *state.Store) ServerOption {
	return func(s *Server) {
		s.StateStore = store
	}
}

// WithApprovalGate sets the approval gate for tool execution approval.
func WithApprovalGate(gate approval.ApprovalGate) ServerOption {
	return func(s *Server) {
		s.ApprovalGate = gate
	}
}

// New creates an HTTP Server bound to the given address.
func New(mgr *manager.Manager, sessionRepo session.Repository, auditLogger audit.Logger, projectsDir string, addr string, opts ...ServerOption) *Server {
	mux := http.NewServeMux()

	// Unified DB reads are ON by default.
	// Set OPENEXEC_USE_UNIFIED_READS=0 to disable for troubleshooting.
	useUnifiedReads := true
	if v := os.Getenv("OPENEXEC_USE_UNIFIED_READS"); v != "" {
		lower := strings.ToLower(v)
		if lower == "0" || lower == "false" || lower == "no" {
			useUnifiedReads = false
		}
	}

	s := &Server{
		Mgr:             mgr,
		SessionRepo:     sessionRepo,
		AuditLogger:     auditLogger,
		UseUnifiedReads: useUnifiedReads,
		ProjectsDir:     projectsDir,
		Mux:             mux,
		Hub:             NewHub(),
		Server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(s)
	}

	s.registerRoutes()

	// Start the WebSocket hub
	go s.Hub.Run()

	return s
}

func (s *Server) registerRoutes() {
	s.RegisterRoutes(s.Mux)
}

// RegisterRoutes registers all API routes to the given mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// WebSocket route
	mux.HandleFunc("GET /ws", s.handleWS)

	// Legacy FWU routes REMOVED in Phase Four.
	// Use /api/v1/runs endpoints for all orchestration.

    // v1 Runs (deterministic run creation)
    mux.HandleFunc("GET /api/v1/runs", s.handleListRuns)
    mux.HandleFunc("POST /api/v1/runs", s.handleCreateRun)
    mux.HandleFunc("POST /api/v1/runs:plan", s.handlePlan)
    mux.HandleFunc("POST /api/v1/runs:execute", s.handleExecuteRuns)
    mux.HandleFunc("POST /api/v1/runs:blueprint", s.handleStartBlueprintRun)
    mux.HandleFunc("POST /api/v1/runs/{id}/start", s.handleStartRun)
    mux.HandleFunc("POST /api/v1/runs/{id}/resume", s.handleResumeRun)
    mux.HandleFunc("GET /api/v1/runs/{id}", s.handleGetRun)
    mux.HandleFunc("GET /api/v1/runs/{id}/steps", s.handleGetRunSteps)
    mux.HandleFunc("GET /api/v1/runs/{id}/checkpoints", s.handleGetRunCheckpoints)
    mux.HandleFunc("GET /api/v1/runs/{id}/timeline", s.handleGetRunTimeline)
    mux.HandleFunc("POST /api/v1/runs/parallel", s.handleStartParallelRuns)

	// Session routes (v1 API - daemon-owned orchestration)
	mux.HandleFunc("GET /api/v1/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/v1/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/v1/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("PATCH /api/v1/sessions/{id}", s.handleUpdateSession)
	mux.HandleFunc("DELETE /api/v1/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /api/v1/sessions/{id}/fork", s.handleForkSession)
	mux.HandleFunc("POST /api/v1/sessions/{id}/archive", s.handleArchiveSession)
	mux.HandleFunc("POST /api/v1/sessions/{id}/run", s.handleStartRunFromSession)
	mux.HandleFunc("GET /api/v1/sessions/{id}/fork-info", s.handleGetForkInfo)
	mux.HandleFunc("GET /api/v1/sessions/{id}/forks", s.handleListSessionForks)
	mux.HandleFunc("GET /api/v1/sessions/{id}/messages", s.handleListMessages)

	// Legacy session routes (aliases to v1, kept for backward compatibility)
	mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	mux.HandleFunc("PATCH /api/sessions/{id}", s.handleUpdateSession)
	mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	mux.HandleFunc("POST /api/sessions/{id}/fork", s.handleForkSession)
	mux.HandleFunc("POST /api/sessions/{id}/archive", s.handleArchiveSession)
	mux.HandleFunc("GET /api/sessions/{id}/fork-info", s.handleGetForkInfo)
	mux.HandleFunc("GET /api/sessions/{id}/forks", s.handleListSessionForks)
	mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)

	// Project routes
	mux.HandleFunc("GET /api/projects", s.handleListProjects)
	mux.HandleFunc("POST /api/projects/init", s.handleInitProject)
	mux.HandleFunc("POST /api/projects/wizard", s.handleWizard)
	mux.HandleFunc("GET /api/directories", s.handleListDirectories)

	// Model/Provider routes
	mux.HandleFunc("GET /api/providers", s.handleListProviders)
	mux.HandleFunc("GET /api/models", s.handleListModels)

	// Register Usage routes (from usage.go)
	if s.AuditLogger != nil && s.SessionRepo != nil {
		RegisterUsageRoutes(mux, s.AuditLogger, s.SessionRepo)
	}

	// Approval routes (v1 API)
	mux.HandleFunc("GET /api/v1/approvals", s.handleListApprovals)
	mux.HandleFunc("GET /api/v1/approvals/{id}", s.handleGetApproval)
	mux.HandleFunc("POST /api/v1/approvals/{id}/approve", s.handleApproveRequest)
	mux.HandleFunc("POST /api/v1/approvals/{id}/reject", s.handleRejectRequest)
}

// Handler returns the HTTP handler for testing without a listener.
func (s *Server) Handler() http.Handler {
	return s.Mux
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
// On context cancellation, it gracefully shuts down the server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.Server.Shutdown(context.Background())
	}
}
