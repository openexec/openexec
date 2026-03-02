package api

import (
	"context"
	"net/http"

	"github.com/openexec/openexec/internal/audit"
	"github.com/openexec/openexec/internal/db/session"
	"github.com/openexec/openexec/internal/manager"
)

// Server exposes the Manager and Session Repository over HTTP.
type Server struct {
	mgr         *manager.Manager
	sessionRepo session.Repository
	auditLogger audit.Logger
	projectsDir string
	server      *http.Server
	mux         *http.ServeMux
}

// New creates an HTTP Server bound to the given address.
func New(mgr *manager.Manager, sessionRepo session.Repository, auditLogger audit.Logger, projectsDir string, addr string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mgr:         mgr,
		sessionRepo: sessionRepo,
		auditLogger: auditLogger,
		projectsDir: projectsDir,
		mux:         mux,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	// Task Execution (FWU) routes
	s.mux.HandleFunc("POST /api/fwu/{id}/start", s.handleStart)
	s.mux.HandleFunc("GET /api/fwu/{id}/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/fwus", s.handleList)
	s.mux.HandleFunc("POST /api/fwu/{id}/pause", s.handlePause)
	s.mux.HandleFunc("POST /api/fwu/{id}/stop", s.handleStop)
	s.mux.HandleFunc("GET /api/fwu/{id}/events", s.handleEvents)

	// Session routes
	s.mux.HandleFunc("GET /api/sessions", s.handleListSessions)
	s.mux.HandleFunc("POST /api/sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /api/sessions/{id}", s.handleGetSession)
	s.mux.HandleFunc("PATCH /api/sessions/{id}", s.handleUpdateSession)
	s.mux.HandleFunc("DELETE /api/sessions/{id}", s.handleDeleteSession)
	s.mux.HandleFunc("POST /api/sessions/{id}/fork", s.handleForkSession)
	s.mux.HandleFunc("POST /api/sessions/{id}/archive", s.handleArchiveSession)
	s.mux.HandleFunc("GET /api/sessions/{id}/fork-info", s.handleGetForkInfo)
	s.mux.HandleFunc("GET /api/sessions/{id}/forks", s.handleListSessionForks)
	s.mux.HandleFunc("GET /api/sessions/{id}/messages", s.handleListMessages)

	// Project routes
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)

	// Register Usage routes (from usage.go)
	if s.auditLogger != nil && s.sessionRepo != nil {
		RegisterUsageRoutes(s.mux, s.auditLogger, s.sessionRepo)
	}
}

// Handler returns the HTTP handler for testing without a listener.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the HTTP server and blocks until the context is cancelled.
// On context cancellation, it gracefully shuts down the server.
func (s *Server) ListenAndServe(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.server.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.server.Shutdown(context.Background())
	}
}
