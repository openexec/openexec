package api

import (
	"context"
	"net/http"

	"github.com/openexec/openexec/internal/manager"
)

// Server exposes the Manager over HTTP.
type Server struct {
	mgr    *manager.Manager
	server *http.Server
	mux    *http.ServeMux
}

// New creates an HTTP Server bound to the given address.
func New(mgr *manager.Manager, addr string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mgr: mgr,
		mux: mux,
		server: &http.Server{
			Addr:    addr,
			Handler: mux,
		},
	}
	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /api/fwu/{id}/start", s.handleStart)
	s.mux.HandleFunc("GET /api/fwu/{id}/status", s.handleStatus)
	s.mux.HandleFunc("GET /api/fwus", s.handleList)
	s.mux.HandleFunc("POST /api/fwu/{id}/pause", s.handlePause)
	s.mux.HandleFunc("POST /api/fwu/{id}/stop", s.handleStop)
	s.mux.HandleFunc("GET /api/fwu/{id}/events", s.handleEvents)
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
