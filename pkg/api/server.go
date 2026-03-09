package api

import (
	"context"
	"net/http"

	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"
	"github.com/openexec/openexec/pkg/manager"
)

// Server exposes the Manager and Session Repository over HTTP.
type Server struct {
	Mgr         *manager.Manager
	SessionRepo session.Repository
	AuditLogger audit.Logger
	ProjectsDir string
	Server      *http.Server
	Mux         *http.ServeMux
	Hub         *Hub
}

// New creates an HTTP Server bound to the given address.
func New(mgr *manager.Manager, sessionRepo session.Repository, auditLogger audit.Logger, projectsDir string, addr string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		Mgr:         mgr,
		SessionRepo: sessionRepo,
		AuditLogger: auditLogger,
		ProjectsDir: projectsDir,
		Mux:         mux,
		Hub:         NewHub(),
		Server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
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

	// Task Execution (FWU) routes
	mux.HandleFunc("POST /api/fwu/{id}/start", s.handleStart)
	mux.HandleFunc("GET /api/fwu/{id}/status", s.handleStatus)
	mux.HandleFunc("GET /api/fwus", s.handleList)
	mux.HandleFunc("POST /api/fwu/{id}/pause", s.handlePause)
	mux.HandleFunc("POST /api/fwu/{id}/stop", s.handleStop)
	mux.HandleFunc("GET /api/fwu/{id}/events", s.handleEvents)

	// v1 Loops compatibility routes (for openexec run)
	mux.HandleFunc("POST /api/v1/loops", s.handleCreateLoop)
	mux.HandleFunc("GET /api/v1/loops/{id}", s.handleGetLoop)

	// Session routes
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
