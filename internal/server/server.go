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
	mgr := manager.New(auditLogger)
	
	// 3. Initialize Deterministic Control Plane (DCP)
	kStore, _ := knowledge.NewStore(".")
	bRouter := router.NewBitNetRouter("/models/bitnet-2b.gguf")
	bRouter.SetSkipAvailabilityCheck(true) // Default to skip for easy startup
	
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
	// --- Legacy/High-Level Axon Routes (pkg/api bridge) ---
	axonBridge := api.New(s.Mgr, s.SessionRepo, s.AuditLogger, s.ProjectsDir, "")
	axonBridge.RegisterRoutes(s.Mux)

	// --- DCP Surgical Routes ---
	s.Mux.HandleFunc("POST /api/v1/dcp/query", s.handleDCPQuery)
	s.Mux.HandleFunc("GET /api/v1/knowledge/symbols", s.handleKnowledgeSymbols)
	s.Mux.HandleFunc("GET /api/v1/knowledge/envs", s.handleKnowledgeEnvs)
	
	// --- Health & System Routes ---
	s.Mux.HandleFunc("GET /api/health", s.handleHealth)

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

// Start runs the server and blocks
func (s *Server) Start(ctx context.Context) error {
	log.Printf("[Server] Unified OpenExec API listening on %s", s.HttpServer.Addr)
	
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
