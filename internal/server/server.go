package server

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/openexec/openexec"
	"github.com/openexec/openexec/internal/execution/health"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/runner"
	"github.com/openexec/openexec/pkg/api"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/telemetry"
	"github.com/openexec/openexec/pkg/version"
)

// Server is the unified OpenExec API and UI host.
type Server struct {
	Mgr         *manager.Manager
	SessionRepo session.Repository
	AuditLogger audit.Logger
	Coordinator DCPCoordinator
	Checker     *health.Checker
	ProjectsDir string
	Mux         *http.ServeMux
	HttpServer  *http.Server
	axonBridge  *api.Server
	StateStore  *state.Store
	// KnowledgeStore is process-scoped so freshness checks and graph writers
	// share the same single-writer lock across concurrent HTTP requests.
	KnowledgeStore *knowledge.Store
	// repositoryEvidenceToken protects the narrow server-to-server read profile
	// used by Agent Console. It grants no validation or execution authority.
	repositoryEvidenceToken string
	// Observability
	runnerCommand string
	runnerArgs    []string
	runnerModel   string
	skipPreflight bool // For testing: skip preflight checks
}

// DCPCoordinator is the core seam for the optional Deterministic Control Plane.
// The concrete implementation lives in internal/dcp (a routing module); core
// holds only this interface so the daemon never imports it. Nil means DCP is off.
type DCPCoordinator interface {
	// AllowsExecution reports whether the coordinator executes tools directly
	// (must be false in production — the HTTP layer refuses otherwise).
	AllowsExecution() bool
	// SyncKnowledge performs a best-effort full re-index of the project.
	SyncKnowledge(projectDir string)
	// ProcessQuery parses intent and returns a suggestion (or result).
	ProcessQuery(ctx context.Context, query string) (any, error)
}

// CoordinatorFactory builds a DCPCoordinator from the daemon-owned database and
// resolved project directory. Supplied by the composition root (internal/cli),
// which is the only layer allowed to import internal/dcp and the DCP tools.
type CoordinatorFactory func(db *sql.DB, projectsDir string) (DCPCoordinator, error)

// Config defines settings for the unified server
type Config struct {
	Port                    int
	DataDir                 string
	UnifiedDB               string
	ModelsPath              string
	ProjectsDir             string
	SkipPreflight           bool // For testing: skip preflight checks that require real runner
	EnableDCP               bool // Feature flag: enable Deterministic Control Plane (default: false)
	RepositoryEvidenceToken string
	// NewCoordinator, when set and EnableDCP resolves true, builds the DCP
	// coordinator. Left nil by default so core needs no DCP dependency.
	NewCoordinator CoordinatorFactory
}

// New creates a new unified OpenExec server
func New(cfg Config) (*Server, error) {
	// 1. Initialize Storage
	dbPath := cfg.UnifiedDB
	if dbPath == "" {
		dbPath = filepath.Join(cfg.DataDir, "openexec.db")
	}
	stateStore, err := state.NewStore(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init unified state store: %w", err)
	}
	knowledgeStore, err := knowledge.NewStoreWithDB(stateStore.GetDB())
	if err != nil {
		_ = stateStore.Close()
		return nil, fmt.Errorf("failed to init repository graph store: %w", err)
	}

	// Legacy adapters for backward compatibility during transition
	db := stateStore.GetDB()
	auditLogger, _ := audit.NewLoggerWithDB(db)
	sessionRepo, _ := session.NewSQLiteRepository(db)

	// 2. Initialize Core Engine
	// Resolve to absolute path — ProjectsDir may be "." from config.
	// Error ignored: filepath.Abs only fails on invalid path syntax; cfg.ProjectsDir
	// is validated before reaching here, and "." is always valid.
	projectsAbs, _ := filepath.Abs(cfg.ProjectsDir)

	// Agents are embedded by default, but can be overridden by env for dev.
	var agentsFS fs.FS = openexec.GetAgentsFS()
	if envDir := os.Getenv("OPENEXEC_AGENTS_DIR"); envDir != "" {
		agentsFS = os.DirFS(envDir)
	}

	logDir := filepath.Join(projectsAbs, ".openexec", "logs")
	// Error ignored: log directory is best-effort. If it fails (permissions, disk full),
	// the manager will fallback to stderr logging.
	_ = os.MkdirAll(logDir, 0750)

	// Resolve runner command from project execution model.
	runnerCmd := ""
	var runnerArgs []string
	var modelUsed string
	if projCfg, err := project.LoadProjectConfig(cfg.ProjectsDir); err == nil {
		modelUsed = projCfg.Execution.ExecutorModel
		rc, ra, err := runner.Resolve(
			modelUsed,
			projCfg.Execution.RunnerCommand,
			projCfg.Execution.RunnerArgs,
		)
		if err != nil {
			// FAIL FAST: Abort startup if runner is missing (unless explicitly forced)
			return nil, fmt.Errorf("CRITICAL: runner resolution failed: %w. Install the CLI or check your config", err)
		}
		runnerCmd = rc
		runnerArgs = ra
		log.Printf("[Server] Runner: model=%s command=%s args=%v",
			modelUsed, runnerCmd, runnerArgs)
	}

	// For Claude, keep CommandName empty to use internal buildClaudeArgs defaults.
	loopCmd := runnerCmd
	loopArgs := runnerArgs
	if strings.Contains(strings.ToLower(runnerCmd), "claude") {
		loopCmd = ""
		loopArgs = nil
	}

	mgr, err := manager.New(manager.Config{
		WorkDir:       projectsAbs,
		AgentsFS:      agentsFS,
		LogDir:        logDir,
		ExecutorModel: modelUsed,
		RunnerCommand: func() string {
			if pc, _ := project.LoadProjectConfig(cfg.ProjectsDir); pc != nil {
				return pc.Execution.RunnerCommand
			}
			return ""
		}(),
		RunnerArgs: func() []string {
			if pc, _ := project.LoadProjectConfig(cfg.ProjectsDir); pc != nil {
				return pc.Execution.RunnerArgs
			}
			return nil
		}(),
		CommandName: loopCmd,
		CommandArgs: loopArgs,
		StateStore:  stateStore,
		TaskTimeout: func() time.Duration {
			if pc, _ := project.LoadProjectConfig(cfg.ProjectsDir); pc != nil && pc.Execution.TimeoutSeconds > 0 {
				return time.Duration(pc.Execution.TimeoutSeconds) * time.Second
			}
			return 0
		}(),
		ExecMode: func() string {
			if pc, _ := project.LoadProjectConfig(cfg.ProjectsDir); pc != nil && pc.Execution.ExecMode != "" {
				return pc.Execution.ExecMode
			}
			return "workspace-write"
		}(),
	})
	if err != nil {
		return nil, fmt.Errorf("manager initialization failed: %w", err)
	}
	// 3. Initialize Deterministic Control Plane (DCP) — optional
	// Controlled via Config.EnableDCP or OPENEXEC_ENABLE_DCP env var.
	enableDCP := cfg.EnableDCP
	if !enableDCP {
		if v := os.Getenv("OPENEXEC_ENABLE_DCP"); v != "" {
			lower := strings.ToLower(v)
			enableDCP = (lower == "1" || lower == "true" || lower == "yes")
		}
	}
	// The coordinator (internal/dcp) is a routing module; core holds only the
	// DCPCoordinator seam and never imports it. When enabled, the composition
	// root's factory (cfg.NewCoordinator) builds and wires it, sharing the
	// daemon-owned db and resolved project dir.
	var coordinator DCPCoordinator
	if enableDCP && cfg.NewCoordinator != nil {
		c, cerr := cfg.NewCoordinator(db, projectsAbs)
		if cerr != nil {
			return nil, fmt.Errorf("DCP coordinator init failed: %w", cerr)
		}
		coordinator = c
		log.Printf("[Server] DCP enabled: coordinator wired by composition root")
	} else {
		log.Printf("[Server] DCP disabled: using Pipeline/Manager only")
	}

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
		axonBridge:  api.New(mgr, sessionRepo, auditLogger, cfg.ProjectsDir, "", api.WithStateStore(stateStore)),
		HttpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", cfg.Port),
			Handler: mux,
		},
		runnerCommand:           runnerCmd,
		runnerArgs:              runnerArgs,
		runnerModel:             modelUsed,
		skipPreflight:           cfg.SkipPreflight,
		StateStore:              stateStore,
		KnowledgeStore:          knowledgeStore,
		repositoryEvidenceToken: cfg.RepositoryEvidenceToken,
	}

	s.registerRoutes()

	if s.Coordinator != nil {
		// Start background indexing only when DCP is enabled
		go s.Coordinator.SyncKnowledge(".")
	}
	s.Mux.HandleFunc("POST /api/v1/repository-graph/scan", s.handleRepositoryGraphScan)
	s.Mux.Handle("GET /api/v1/repository-context", s.repositoryGraphReadAuth(http.HandlerFunc(s.handleRepositoryContext)))
	s.registerRepositoryGraphQueryRoutes()
	if s.repositoryEvidenceToken != "" {
		s.registerRepositoryEvidenceRoutes()
	}

	// Log active feature flags for operators
	logFeatureFlags()

	return s, nil
}

func (s *Server) handleRepositoryGraphScan(w http.ResponseWriter, r *http.Request) {
	store, err := s.repositoryGraphStore()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	result, err := store.RefreshRepository(r.Context(), s.ProjectsDir)
	if err != nil {
		s.respondJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleRepositoryContext(w http.ResponseWriter, r *http.Request) {
	store, err := s.repositoryGraphStore()
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	identity, err := store.EnsureRepositoryIdentity(r.Context(), s.ProjectsDir, "")
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var names []string
	for _, name := range strings.Split(r.URL.Query().Get("symbols"), ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	planID := r.URL.Query().Get("plan_revision_id")
	var report *state.CompletionReport
	if planID != "" {
		coverage, coverageErr := s.StateStore.EvidenceCoverage(r.Context(), planID)
		if coverageErr != nil {
			s.respondJSON(w, http.StatusConflict, map[string]string{"error": coverageErr.Error()})
			return
		}
		report = &coverage
	}
	projection, err := store.BuildRepositoryContext(r.Context(), identity, names, r.URL.Query().Get("task_id"), r.URL.Query().Get("run_id"), planID, report)
	if err != nil {
		s.respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.respondJSON(w, http.StatusOK, projection)
}

// logFeatureFlags logs the state of all feature flags on startup.
func logFeatureFlags() {
	flags := []struct {
		name    string
		envVar  string
		enabled bool
	}{
		{"DCP", "OPENEXEC_ENABLE_DCP", isEnvTrue("OPENEXEC_ENABLE_DCP")},
		{"UnifiedReads", "OPENEXEC_USE_UNIFIED_READS", !isEnvFalse("OPENEXEC_USE_UNIFIED_READS")}, // default true
		{"LegacyFWU", "OPENEXEC_ENABLE_LEGACY_FWU", isEnvTrue("OPENEXEC_ENABLE_LEGACY_FWU")},
	}

	var enabled, disabled []string
	for _, f := range flags {
		if f.enabled {
			enabled = append(enabled, f.name)
		} else {
			disabled = append(disabled, f.name)
		}
	}

	if len(enabled) > 0 {
		log.Printf("[Server] Feature flags enabled: %s", strings.Join(enabled, ", "))
	}
	if len(disabled) > 0 {
		log.Printf("[Server] Feature flags disabled: %s", strings.Join(disabled, ", "))
	}
}

// isEnvTrue returns true if the environment variable is set to a truthy value.
func isEnvTrue(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "1" || v == "true" || v == "yes"
}

// isEnvFalse returns true if the environment variable is explicitly set to a falsy value.
func isEnvFalse(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "0" || v == "false" || v == "no"
}

func (s *Server) registerRoutes() {
	// --- Legacy/High-Level OpenExec Routes (pkg/api bridge) ---
	s.axonBridge.RegisterRoutes(s.Mux)

	// --- DCP Surgical Routes ---
	if s.Coordinator != nil {
		s.Mux.HandleFunc("POST /api/v1/dcp/query", s.handleDCPQuery)
		s.Mux.HandleFunc("GET /api/v1/knowledge/symbols", s.handleKnowledgeSymbols)
		s.Mux.HandleFunc("GET /api/v1/knowledge/envs", s.handleKnowledgeEnvs)
	}

	// --- Health & System Routes ---
	s.Mux.HandleFunc("GET /api/health", s.handleHealth)
	s.Mux.HandleFunc("GET /api/ready", s.Checker.ReadyHandler())

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
		Query   string `json:"query"`
		Execute bool   `json:"execute,omitempty"` // Defense in depth: always rejected
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Defense in depth: DCP is suggest-only. Execution must go through MCP.
	if req.Execute {
		log.Printf("[DCP] BLOCKED: execute=true rejected (DCP is suggest-only)")
		s.respondJSON(w, http.StatusForbidden, map[string]interface{}{
			"error":      "DCP is suggest-only; execution must go through MCP",
			"suggestion": "Use MCP tools directly or pass the suggestion to the MCP execution layer",
		})
		return
	}

	// Extra guard: ensure coordinator is in suggest-only mode
	if s.Coordinator.AllowsExecution() {
		log.Printf("[DCP] BLOCKED: coordinator has AllowExecution=true (should never happen in production)")
		s.respondJSON(w, http.StatusForbidden, map[string]interface{}{
			"error": "DCP is misconfigured; execution is disabled at the HTTP layer",
		})
		return
	}

	log.Printf("[DCP] Received query: %q", req.Query)
	result, err := s.Coordinator.ProcessQuery(r.Context(), req.Query)
	if err != nil {
		log.Printf("[DCP] Query error: %v", err)
		s.respondJSON(w, http.StatusInternalServerError, map[string]interface{}{"error": err.Error()})
		return
	}

	log.Printf("[DCP] Query result: %v", result)
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"result": result})
}

func (s *Server) handleKnowledgeSymbols(w http.ResponseWriter, r *http.Request) {
	// Errors ignored: these are informational endpoints that return empty lists on failure.
	// Creating a new store per request is intentional to pick up index changes.
	store, _ := knowledge.NewStore(".")
	defer store.Close()
	list, _ := store.ListSymbols()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"symbols": list})
}

func (s *Server) handleKnowledgeEnvs(w http.ResponseWriter, r *http.Request) {
	// Errors ignored: these are informational endpoints that return empty lists on failure.
	store, _ := knowledge.NewStore(".")
	defer store.Close()
	list, _ := store.ListEnvironments()
	s.respondJSON(w, http.StatusOK, map[string]interface{}{"environments": list})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	runnerCmd := "claude"
	runnerArgs := runner.ClaudeDefaultArgs
	modelName := ""

	if s.Mgr != nil {
		cfg := s.Mgr.GetConfig()
		modelName = cfg.ExecutorModel

		// If custom runner was configured, use those
		if cfg.RunnerCommand != "" {
			runnerCmd = cfg.RunnerCommand
			runnerArgs = cfg.RunnerArgs
		} else if cfg.CommandName != "" {
			// Resolved from model
			runnerCmd = filepath.Base(cfg.CommandName)
			_, ra, _ := runner.Resolve(modelName, "", nil)
			runnerArgs = ra
		}
	}

	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": version.Version,
		"runner": map[string]interface{}{
			"command": runnerCmd,
			"args":    runnerArgs,
			"model":   modelName,
		},
	})
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

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not implement http.Hijacker")
	}
	return h.Hijack()
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

// registerPreflightChecks registers startup validation checks.
// CONTRACT:
// 1. Must be called before Start()
// 2. Registers checks for: runner_available, knowledge_db, audit_db
// 3. runner_available is CRITICAL (blocks startup)
// 4. knowledge_db and audit_db are NON-CRITICAL (degraded mode OK)
func (s *Server) registerPreflightChecks() {
	// Critical: Runner must be available
	s.Checker.Register(health.Check{
		Name:     "runner_available",
		Critical: true,
		Run: func(ctx context.Context) (health.Status, string, error) {
			// Get execution config from manager
			cfg := s.Mgr.GetConfig()
			runnerCmd, _, err := runner.Resolve(
				cfg.ExecutorModel,
				cfg.RunnerCommand,
				cfg.RunnerArgs,
			)
			if err != nil {
				return health.StatusFailed, fmt.Sprintf("runner not available: %v", err), nil
			}
			return health.StatusOK, fmt.Sprintf("runner '%s' available", filepath.Base(runnerCmd)), nil
		},
		Remediation: "Install Claude CLI: npm install -g @anthropic/claude-code, or configure a custom runner in openexec.yaml",
	})

	// Critical: Runner must respond to a simple command (auth/config check)
	s.Checker.Register(health.Check{
		Name:     "runner_noop",
		Critical: true,
		Run: func(ctx context.Context) (health.Status, string, error) {
			cfg := s.Mgr.GetConfig()
			runnerCmd, _, err := runner.Resolve(
				cfg.ExecutorModel,
				cfg.RunnerCommand,
				cfg.RunnerArgs,
			)
			if err != nil {
				return health.StatusFailed, fmt.Sprintf("runner resolution failed: %v", err), nil
			}

			// For internal providers, no-op is always OK if resolution passed
			if runnerCmd == "gemini" || runnerCmd == "openai" {
				return health.StatusOK, "internal provider ready", nil
			}

			// Try a simple version check or similar no-op
			// (Best effort: don't fail if command doesn't support --version, but most do)
			check := exec.CommandContext(ctx, runnerCmd, "--version")
			// If it's Claude, we might need a more specific check, but --version is safe
			if strings.Contains(strings.ToLower(runnerCmd), "claude") {
				check = exec.CommandContext(ctx, runnerCmd, "--version")
			}

			if err := check.Run(); err != nil {
				return health.StatusFailed, fmt.Sprintf("runner binary exists but failed to execute (auth or config issue): %v", err), nil
			}

			return health.StatusOK, "runner responded to noop check", nil
		},
		Remediation: "Verify the runner is authenticated (e.g. 'claude login' or 'gemini auth') and that all required flags are valid.",
	})

	// Non-critical: Knowledge database
	s.Checker.Register(health.Check{
		Name:     "knowledge_db",
		Critical: false,
		Run: func(ctx context.Context) (health.Status, string, error) {
			kStore, err := knowledge.NewStore(".")
			if err != nil {
				return health.StatusDegraded, fmt.Sprintf("knowledge store unavailable: %v", err), nil
			}
			_ = kStore.Close()
			return health.StatusOK, "knowledge store initialized", nil
		},
		Remediation: "Run 'openexec index' to initialize the knowledge database",
	})

	// Non-critical: Audit database (already initialized in New())
	s.Checker.Register(health.Check{
		Name:     "audit_db",
		Critical: false,
		Run: func(ctx context.Context) (health.Status, string, error) {
			if s.AuditLogger == nil {
				return health.StatusDegraded, "audit logger not initialized", nil
			}
			return health.StatusOK, "audit logger active", nil
		},
		Remediation: "Check data directory permissions",
	})
}

// Start runs the server and blocks
func (s *Server) Start(ctx context.Context) error {
	// Enable global panic recovery for the daemon
	defer func() {
		if r := recover(); r != nil {
			logDir := filepath.Join(s.ProjectsDir, ".openexec")
			_ = os.MkdirAll(logDir, 0750)
			crashLog := filepath.Join(logDir, "crash.log")

			f, err := os.OpenFile(crashLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				fmt.Fprintf(f, "\n=== DAEMON CRASH AT %s ===\n", time.Now().Format(time.RFC3339))
				fmt.Fprintf(f, "Panic: %v\n", r)
				// Write stack trace
				buf := make([]byte, 64<<10) // 64KB
				n := runtime.Stack(buf, true)
				f.Write(buf[:n])
				fmt.Fprintf(f, "\n=== END CRASH LOG ===\n")
				f.Close()
			}

			log.Printf("[Server] 🚨 CRITICAL ERROR: Daemon panicked! Details written to %s", crashLog)
			// Re-panic to ensure process exits (we want it to restart, not stay in broken state)
			panic(r)
		}
	}()

	// Perform pre-flight setup (model downloads, indexing)
	if err := s.Mgr.EnsureReady(ctx); err != nil {
		return fmt.Errorf("failed pre-flight readiness: %w", err)
	}

	// Initialize OpenTelemetry
	shutdown, err := telemetry.InitOTel(ctx, "openexec-daemon", os.Stdout)
	if err != nil {
		log.Printf("[Server] ⚠ Warning: failed to initialize OTel: %v", err)
	} else {
		defer func() { _ = shutdown(context.Background()) }()
	}

	// Register and run preflight checks before starting (unless skipped for testing)
	if !s.skipPreflight {
		s.registerPreflightChecks()
		if err := s.Checker.RunPreflight(ctx); err != nil {
			return fmt.Errorf("preflight failed: %w", err)
		}
	}

	log.Printf("[Server] Unified OpenExec API listening on %s", s.HttpServer.Addr)

	// Wrap the mux with logging middleware
	s.HttpServer.Handler = loggingMiddleware(s.Mux)

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.HttpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		// Server terminated (error or closed). Flush state store and return.
		if s.StateStore != nil {
			_ = s.StateStore.Close()
		}
		return err
	case <-ctx.Done():
		// Graceful shutdown path
		_ = s.HttpServer.Shutdown(context.Background())
		if s.StateStore != nil {
			_ = s.StateStore.Close()
		}
		return nil
	}
}
