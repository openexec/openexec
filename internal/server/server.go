package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/openexec/openexec"
	"github.com/openexec/openexec/internal/dcp"
	"github.com/openexec/openexec/internal/execution/health"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/policy"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
	"github.com/openexec/openexec/pkg/api"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/version"
)

// Server is the unified OpenExec API and UI host.
type Server struct {
	Mgr         *manager.Manager
	SessionRepo session.Repository
	AuditLogger audit.Logger
	Coordinator *dcp.Coordinator
	Checker     *health.Checker
	ProjectsDir string
	Mux         *http.ServeMux
	HttpServer  *http.Server
	mu          sync.RWMutex
	axonBridge  *api.Server
}

// Config defines settings for the unified server
type Config struct {
	Port        int
	DataDir     string
	AuditDB     string
	ModelsPath  string
	ProjectsDir string
}

// New creates a new unified OpenExec server
func New(cfg Config) (*Server, error) {
	// 1. Initialize Storage
	auditLogger, err := audit.NewLogger(cfg.AuditDB)
	if err != nil {
		return nil, fmt.Errorf("failed to init audit logger: %w", err)
	}

	db := auditLogger.GetDB()
	sessionRepo, err := session.NewSQLiteRepository(db)
	if err != nil {
		return nil, fmt.Errorf("failed to init session repo: %w", err)
	}

	// 2. Initialize Core Engine
	// Resolve to absolute path — ProjectsDir may be "." from config.
	projectsAbs, _ := filepath.Abs(cfg.ProjectsDir)

	// Agents ship alongside the binary, not inside the user's project.
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	agentsDir := filepath.Join(execDir, "..", "agents")
	// If not found next to binary (e.g. `go run`), fall back to cwd.
	if _, err := os.Stat(agentsDir); err != nil {
		agentsDir = filepath.Join(".", "agents")
	}
	agentsDir, _ = filepath.Abs(agentsDir)

	logDir := filepath.Join(projectsAbs, ".openexec", "logs")
	_ = os.MkdirAll(logDir, 0750)

	mgr := manager.New(manager.Config{
		WorkDir:    projectsAbs,
		TractStore: cfg.DataDir,
		AgentsDir:  agentsDir,
		LogDir:     logDir,
	})
	
	// 3. Initialize Deterministic Control Plane (DCP)
	kStore, _ := knowledge.NewStore(".")
	bRouter := router.NewBitNetRouter("/models/bitnet-2b.gguf")
	bRouter.SetSkipAvailabilityCheck(true) // Default to skip for easy startup
	
	pEngine := policy.NewEngine(kStore)
	coordinator := dcp.NewCoordinator(bRouter, kStore)
	coordinator.RegisterTool(tools.NewSymbolReaderTool(kStore))
	coordinator.RegisterTool(tools.NewDeployTool(kStore))
	coordinator.RegisterTool(tools.NewSafeCommitTool(pEngine, coordinator))
	
	// 4. Initialize API Layer
	mux := http.NewServeMux()
	s := &Server{
		Mgr:         mgr,
		SessionRepo: sessionRepo,
		AuditLogger: auditLogger,
		Coordinator: coordinator,
		Checker:     health.NewChecker(),
		ProjectsDir: cfg.ProjectsDir,
		Mux:         mux,
		axonBridge:  api.New(mgr, sessionRepo, auditLogger, cfg.ProjectsDir, ""),
		HttpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Port),
			Handler: mux,
		},
	}

	s.registerRoutes()
	
	// Start background indexing
	go coordinator.SyncKnowledge(".")

	return s, nil
}

func (s *Server) registerRoutes() {
	// --- Legacy/High-Level OpenExec Routes (pkg/api bridge) ---
	s.axonBridge.RegisterRoutes(s.Mux)

	// --- DCP Surgical Routes ---
	s.Mux.HandleFunc("POST /api/v1/dcp/query", s.handleDCPQuery)
	s.Mux.HandleFunc("GET /api/v1/knowledge/symbols", s.handleKnowledgeSymbols)
	s.Mux.HandleFunc("GET /api/v1/knowledge/envs", s.handleKnowledgeEnvs)
	
	// --- Health & System Routes ---
	s.Mux.HandleFunc("GET /api/health", s.handleHealth)

	// --- Catch-all 404 handler for unknown API routes ---
	s.Mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[API] 404 Not Found: %s %s", r.Method, r.URL.Path)
		s.respondJSON(w, http.StatusNotFound, map[string]string{
			"error":      "Endpoint not found",
			"path":       r.URL.Path,
			"suggestion": "Verify the URL prefix and version (e.g., /api/v1/). If using 'openexec run', ensure the server is updated to v0.1.7+.",
		})
	})

	// --- Embedded UI ---
	uiFS := openexec.GetUIFS()
	s.Mux.Handle("/", http.FileServer(http.FS(uiFS)))
}

func (s *Server) handleDCPQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	result, err := s.Coordinator.ProcessQuery(r.Context(), req.Query)
	if err != nil {
		s.respondJSON(w, http.StatusOK, map[string]interface{}{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}

func (s *Server) handleKnowledgeSymbols(w http.ResponseWriter, r *http.Request) {
	store, _ := knowledge.NewStore(".")
	defer store.Close()
	list, _ := store.ListSymbols()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"symbols": list})
}

func (s *Server) handleKnowledgeEnvs(w http.ResponseWriter, r *http.Request) {
	store, _ := knowledge.NewStore(".")
	defer store.Close()
	list, _ := store.ListEnvironments()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"environments": list})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"status": "ok", "version": version.Version})
}

func (s *Server) respondJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// loggingMiddleware logs details about every incoming request
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		
		next.ServeHTTP(wrapped, r)
		
		log.Printf("[API] %s %s %d (%v)", r.Method, r.URL.Path, wrapped.status, time.Since(start))
	})
}

// Start runs the server and blocks
func (s *Server) Start(ctx context.Context) error {
	log.Printf("[Server] Unified OpenExec API listening on %s", s.HttpServer.Addr)
	
	// Wrap the mux with logging middleware
	s.HttpServer.Handler = loggingMiddleware(s.Mux)
	
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.HttpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return s.HttpServer.Shutdown(context.Background())
	}
}
