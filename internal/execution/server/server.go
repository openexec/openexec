package server

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/openexec/openexec/internal/dcp"
	"github.com/openexec/openexec/internal/execution/health"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/tools"
	"github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/pkg/api"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/util"
	"github.com/openexec/openexec/pkg/db/session"
	openexec_manager "github.com/openexec/openexec/pkg/manager"
)

const (
	maxRequestBodySize = 1 << 20 // 1MB
	version            = "0.1.0"
)

// Server manages execution loops and provides HTTP API
type Server struct {
	auditWriter audit.Logger
	loops       map[string]*LoopInstance
	mu          sync.RWMutex
	checker     *health.Checker
	apiServer   *api.Server
	projectsDir string
	coordinator *dcp.Coordinator
}

// LoopInstance tracks a running execution loop
type LoopInstance struct {
	ID        string
	Config    loop.Config
	Loop      *loop.Loop
	Events    <-chan loop.Event
	Status    string
	Iteration int
	StartedAt time.Time
	cancel    context.CancelFunc

	// SSE broadcasting
	sseClients map[chan LoopEvent]struct{}
	sseMu      sync.RWMutex
}

// LoopEvent is an event sent to SSE clients
type LoopEvent struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	Iteration int    `json:"iteration"`
	Text      string `json:"text,omitempty"`
}

// LoopResponse is the API response for loop operations
type LoopResponse struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Iteration int    `json:"iteration"`
	StartedAt string `json:"started_at,omitempty"`
}

// addSSEClient registers a new SSE client
func (inst *LoopInstance) addSSEClient(ch chan LoopEvent) {
	inst.sseMu.Lock()
	if inst.sseClients == nil {
		inst.sseClients = make(map[chan LoopEvent]struct{})
	}
	inst.sseClients[ch] = struct{}{}
	inst.sseMu.Unlock()
}

// removeSSEClient unregisters an SSE client
func (inst *LoopInstance) removeSSEClient(ch chan LoopEvent) {
	inst.sseMu.Lock()
	delete(inst.sseClients, ch)
	inst.sseMu.Unlock()
}

// broadcast sends an event to all SSE clients
func (inst *LoopInstance) broadcast(event LoopEvent) {
	inst.sseMu.RLock()
	defer inst.sseMu.RUnlock()
	for ch := range inst.sseClients {
		select {
		case ch <- event:
		default:
			// Client too slow, skip
		}
	}
}

// snapshot returns a copy of the loop state (call with s.mu held)
func (inst *LoopInstance) snapshot() LoopResponse {
	return LoopResponse{
		ID:        inst.ID,
		Status:    inst.Status,
		Iteration: inst.Iteration,
		StartedAt: inst.StartedAt.Format(time.RFC3339),
	}
}

func StartServer() {
	var (
		port         = flag.Int("port", 8765, "HTTP server port")
		auditDB      = flag.String("audit-db", "audit.db", "Path to audit database")
		enableDemo   = flag.Bool("demo", false, "Enable demo mode (stops after 2 iterations)")
		dataDir      = flag.String("data-dir", ".openexec/data", "Data directory for audit logs")
		modelsConfig = flag.String("models-config", "", "Path to models configuration JSON file")
	)
	flag.Parse()

	log.Printf("OpenExec Execution Engine v%s starting...", version)

	// Load custom models if provided
	if *modelsConfig != "" {
		log.Printf("Loading custom models from: %s", *modelsConfig)
		if err := agent.DefaultModelCatalog.LoadFromConfig(*modelsConfig); err != nil {
			log.Printf("Warning: failed to load models config: %v", err)
		}
	} else {
		// Try default location (.openexec/models.json in project dir)
		defaultPath := filepath.Join(filepath.Dir(*dataDir), "models.json")
		if _, err := os.Stat(defaultPath); err == nil {
			log.Printf("Loading models from default path: %s", defaultPath)
			_ = agent.DefaultModelCatalog.LoadFromConfig(defaultPath)
		}
	}

	// Create health checker
	checker := health.NewChecker()

	// Register preflight checks
	auditDir := filepath.Dir(*auditDB)
	if auditDir == "." {
		auditDir = *dataDir
	}

	checker.Register(health.Check{
		Name:     "audit_directory",
		Critical: true,
		Run: func(ctx context.Context) (health.Status, string, error) {
			// Ensure directory exists
			if err := os.MkdirAll(auditDir, 0750); err != nil {
				return health.StatusFailed, fmt.Sprintf("cannot create audit directory: %v", err), nil
			}
			// Test write access
			testFile := filepath.Join(auditDir, ".write_test")
			f, err := os.Create(testFile)
			if err != nil {
				return health.StatusFailed, fmt.Sprintf("cannot write to audit directory: %v", err), nil
			}
			_ = f.Close()
			_ = os.Remove(testFile)
			return health.StatusOK, fmt.Sprintf("audit directory %s writable", auditDir), nil
		},
		Remediation: fmt.Sprintf("Ensure directory %s exists and is writable", auditDir),
	})

	// Run preflight checks
	ctx := context.Background()
	if err := checker.RunPreflight(ctx); err != nil {
		log.Fatalf("Preflight checks failed: %v", err)
	}

	// Initialize audit logger
	auditWriter, err := audit.NewLogger(*auditDB)
	if err != nil {
		log.Fatalf("failed to initialize audit logger: %v", err)
	}
	defer func() { _ = auditWriter.Close() }()

	// Verify audit writer works
	checker.Register(health.Check{
		Name:     "audit_writer",
		Critical: true,
		Run: func(ctx context.Context) (health.Status, string, error) {
			// Try a test query instead of Log (signatures differ between openexec and local)
			_, err := auditWriter.Query(ctx, &audit.QueryFilter{Limit: 1})
			if err != nil {
				return health.StatusFailed, fmt.Sprintf("audit query failed: %v", err), nil
			}
			return health.StatusOK, "audit logger functional", nil
		},
		Remediation: "Check audit database file permissions and disk space",
	})

	// Re-run to include audit writer check
	if err := checker.RunPreflight(ctx); err != nil {
		log.Fatalf("Preflight checks failed: %v", err)
	}

	log.Println("All preflight checks passed")

	// Discovery directory for projects
	projectsDir := "/Users/perttu/study/siivous/orchestrator/projects"
	log.Printf("Project discovery root forced to: %s", projectsDir)

	// Create server
	srv := &Server{
		auditWriter: auditWriter,
		loops:       make(map[string]*LoopInstance),
		checker:     checker,
		projectsDir: projectsDir,
	}

	// Initialize DCP
	kStore, _ := knowledge.NewStore(".")
	bRouter := router.NewBitNetRouter("/models/bitnet-2b.gguf")
	srv.coordinator = dcp.NewCoordinator(bRouter, kStore)
	srv.coordinator.RegisterTool(tools.NewSymbolReaderTool(kStore))
	srv.coordinator.RegisterTool(tools.NewDeployTool(kStore))
	srv.coordinator.RegisterTool(tools.NewKnowledgePopulationTool(kStore))
	srv.coordinator.RegisterTool(tools.NewDocsUpdaterTool(kStore, "."))

	// Initial sync
	go srv.coordinator.SyncKnowledge(".")

	// Initialize OpenExec project/session API
	db := auditWriter.GetDB()
	sessionRepo, err := session.NewSQLiteRepository(db)
	if err != nil {
		log.Fatalf("failed to initialize session repository: %v", err)
	}

	// We create a dummy manager for now as the execution server has its own loop management
	// but we need it for the API struct. In the future these should be unified.
	dummyMgr := openexec_manager.New(openexec_manager.Config{})

	srv.apiServer = api.New(dummyMgr, sessionRepo, auditWriter, projectsDir, fmt.Sprintf(":%d", *port))

	// Create HTTP mux
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"ok","version":"%s"}`, version)))
	})
	mux.HandleFunc("/health/details", checker.Handler(true, version))
	mux.HandleFunc("/ready", checker.ReadyHandler())

	// WebSocket endpoint
	mux.HandleFunc("/ws", srv.handleWebSocket)

	// API endpoints
	mux.HandleFunc("/api/v1/loops", srv.handleLoops)
	mux.HandleFunc("/api/v1/loops/", srv.handleLoop)
	mux.HandleFunc("/api/v1/audit", srv.handleAudit)
	mux.HandleFunc("/api/v1/evidence", srv.handleTaskEvidence)
	mux.HandleFunc("/api/v1/logs", srv.handleSessionLog)
	mux.HandleFunc("/api/v1/dcp/query", srv.handleDCPQuery)

	// Integrate the new API server's routes
	srv.apiServer.RegisterRoutes(mux)

	// Demo mode endpoint (if enabled)
	if *enableDemo {
		mux.HandleFunc("/api/v1/demo", srv.handleDemo)
		log.Println("Demo mode enabled at /api/v1/demo")
	}

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server
	go func() {
		log.Printf("HTTP server listening on :%d", *port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Start periodic health checks
	go srv.runPeriodicHealthChecks(ctx)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")

	// Mark as not ready
	checker.SetReady(false)

	// Stop all running loops
	srv.mu.Lock()
	for _, inst := range srv.loops {
		if inst.cancel != nil {
			inst.cancel()
		}
		inst.Loop.Stop()
	}
	srv.mu.Unlock()

	// Graceful HTTP shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

func (s *Server) runPeriodicHealthChecks(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Check audit writer health
			err := s.auditWriter.LogEvent(ctx, audit.EventSystemStartup, "health-checker", "system")
			if err != nil {
				s.checker.UpdateCheck("audit_writer", health.StatusFailed, fmt.Sprintf("write failed: %v", err))
			} else {
				s.checker.UpdateCheck("audit_writer", health.StatusOK, "periodic check passed")
			}
		case <-ctx.Done():
			return
		}
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

// handleLoops handles GET /api/v1/loops (list) and POST /api/v1/loops (create)
func (s *Server) handleLoops(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listLoops(w, r)
	case http.MethodPost:
		s.createLoop(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLoop handles individual loop operations
func (s *Server) handleLoop(w http.ResponseWriter, r *http.Request) {
	// Extract loop ID from path: /api/v1/loops/{id}[/action]
	path := r.URL.Path[len("/api/v1/loops/"):]
	if path == "" {
		http.Error(w, "Loop ID required", http.StatusBadRequest)
		return
	}

	// Check for action
	var loopID, action string
	for i, c := range path {
		if c == '/' {
			loopID = path[:i]
			action = path[i+1:]
			break
		}
	}
	if loopID == "" {
		loopID = path
	}

	switch {
	case action == "" && r.Method == http.MethodGet:
		s.getLoop(w, r, loopID)
	case action == "pause" && r.Method == http.MethodPost:
		s.pauseLoop(w, r, loopID)
	case action == "stop" && r.Method == http.MethodPost:
		s.stopLoop(w, r, loopID)
	case action == "events" && r.Method == http.MethodGet:
		s.streamEvents(w, r, loopID)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// CreateLoopRequest is the request body for creating a loop
type CreateLoopRequest struct {
	Prompt        string `json:"prompt"`
	WorkDir       string `json:"work_dir"`
	MaxIterations int    `json:"max_iterations,omitempty"`
	ReviewerModel string `json:"reviewer_model,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	MCPConfigPath string `json:"mcp_config_path,omitempty"`
}

func (s *Server) listLoops(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	loops := make([]LoopResponse, 0, len(s.loops))
	for _, inst := range s.loops {
		loops = append(loops, inst.snapshot())
	}
	s.mu.RUnlock()

	respondJSON(w, http.StatusOK, loops)
}

func (s *Server) createLoop(w http.ResponseWriter, r *http.Request) {
	// Limit request body size
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req CreateLoopRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Sanitize input prompt from UI
	req.Prompt = util.SanitizeInput(req.Prompt)

	if req.Prompt == "" {
		http.Error(w, "Prompt is required", http.StatusBadRequest)
		return
	}
	if req.WorkDir == "" {
		req.WorkDir = "."
	}

	// Create loop config
	cfg := loop.DefaultConfig()
	cfg.Prompt = req.Prompt
	cfg.WorkDir = req.WorkDir
	if req.MaxIterations > 0 {
		cfg.MaxIterations = req.MaxIterations
	}
	if req.ReviewerModel != "" {
		cfg.ReviewerModel = req.ReviewerModel
	}
	if req.TaskID != "" {
		cfg.TaskID = req.TaskID
	}
	if req.MCPConfigPath != "" {
		cfg.MCPConfigPath = req.MCPConfigPath
	}

	// Create loop instance
	l, events := loop.New(cfg)
	loopID := fmt.Sprintf("loop-%d", time.Now().UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	inst := &LoopInstance{
		ID:        loopID,
		Config:    cfg,
		Loop:      l,
		Events:    events,
		Status:    "running",
		StartedAt: time.Now(),
		cancel:    cancel,
	}

	s.mu.Lock()
	s.loops[loopID] = inst
	s.mu.Unlock()

	// Start loop in goroutine
	go s.runLoop(ctx, inst)

	respondJSON(w, http.StatusCreated, LoopResponse{
		ID:        loopID,
		Status:    "running",
		Iteration: 0,
		StartedAt: inst.StartedAt.Format(time.RFC3339),
	})
}

func (s *Server) runLoop(ctx context.Context, inst *LoopInstance) {
	// Process events and update instance state
	go func() {
		for event := range inst.Events {
			s.mu.Lock()
			inst.Iteration = event.Iteration
			switch event.Type {
			case loop.EventComplete:
				inst.Status = "complete"
			case loop.EventPaused:
				inst.Status = "paused"
			case loop.EventError:
				inst.Status = "error"
			case loop.EventMaxIterationsReached:
				inst.Status = "max_iterations"
			}
			// Capture state for broadcast while holding lock
			sseEvent := LoopEvent{
				Type:      string(event.Type),
				ID:        inst.ID,
				Status:    inst.Status,
				Iteration: inst.Iteration,
				Text:      event.Text,
			}
			s.mu.Unlock()

			// Broadcast to SSE clients
			inst.broadcast(sseEvent)

			// Log to audit
			_ = s.auditWriter.LogEvent(ctx, audit.EventType(event.Type), inst.ID, "loop")
		}

		// Broadcast completion
		s.mu.RLock()
		finalEvent := LoopEvent{
			Type:      "stream_end",
			ID:        inst.ID,
			Status:    inst.Status,
			Iteration: inst.Iteration,
		}
		s.mu.RUnlock()
		inst.broadcast(finalEvent)
	}()

	// Run the loop
	if err := inst.Loop.Run(ctx); err != nil && ctx.Err() == nil {
		s.mu.Lock()
		inst.Status = "error"
		s.mu.Unlock()
	}
}

func (s *Server) getLoop(w http.ResponseWriter, r *http.Request, loopID string) {
	s.mu.RLock()
	inst, ok := s.loops[loopID]
	var resp LoopResponse
	if ok {
		resp = inst.snapshot()
	}
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Loop not found", http.StatusNotFound)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func (s *Server) pauseLoop(w http.ResponseWriter, r *http.Request, loopID string) {
	s.mu.RLock()
	inst, ok := s.loops[loopID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Loop not found", http.StatusNotFound)
		return
	}

	inst.Loop.Pause()
	respondJSON(w, http.StatusOK, map[string]string{"status": "pausing"})
}

func (s *Server) stopLoop(w http.ResponseWriter, r *http.Request, loopID string) {
	s.mu.RLock()
	inst, ok := s.loops[loopID]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Loop not found", http.StatusNotFound)
		return
	}

	if inst.cancel != nil {
		inst.cancel()
	}
	inst.Loop.Stop()
	respondJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) streamEvents(w http.ResponseWriter, r *http.Request, loopID string) {
	s.mu.RLock()
	inst, ok := s.loops[loopID]
	var initialStatus LoopResponse
	if ok {
		initialStatus = inst.snapshot()
	}
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "Loop not found", http.StatusNotFound)
		return
	}

	// Set up SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Create client channel and register
	clientCh := make(chan LoopEvent, 16)
	inst.addSSEClient(clientCh)
	defer func() {
		inst.removeSSEClient(clientCh)
		close(clientCh)
	}()

	// Send initial status
	data, _ := json.Marshal(LoopEvent{
		Type:      "initial",
		ID:        initialStatus.ID,
		Status:    initialStatus.Status,
		Iteration: initialStatus.Iteration,
	})
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	// Stream events until client disconnects or loop ends
	for {
		select {
		case event, ok := <-clientCh:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()

			// End stream on completion
			if event.Type == "stream_end" {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleDCPQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	result, err := s.coordinator.ProcessQuery(r.Context(), req.Query)
	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"result": result,
	})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Default to last hour
	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Minute)

	result, err := s.auditWriter.Query(r.Context(), &audit.QueryFilter{
		Since: since,
		Until: until,
		Limit: 100,
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to query audit logs: %v", err), http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, result.Entries)
}

// handleDemo runs a demo execution (limited iterations for testing)
func (s *Server) handleDemo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	// Create loop with demo limits
	cfg := loop.DefaultConfig()
	cfg.Prompt = "Execute the demo task"
	cfg.WorkDir = "."
	cfg.MaxIterations = 2 // Demo limit

	l, events := loop.New(cfg)

	// Process events
	go func() {
		for event := range events {
			fmt.Printf("[DEMO] %s iteration=%d text=%s\n", event.Type, event.Iteration, event.Text)
			_ = s.auditWriter.LogEvent(ctx, audit.EventType(event.Type), "demo", "loop")
		}
	}()

	// Run loop
	if err := l.Run(ctx); err != nil {
		log.Printf("Demo loop failed: %v", err)
	}

	// Query audit logs
	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Minute)
	result, _ := s.auditWriter.Query(ctx, &audit.QueryFilter{Since: since, Until: until})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "demo_complete",
		"logs":   len(result.Entries),
	})
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // For development
	},
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("WebSocket client connected")

	// Subscription state
	var currentLoopID string
	clientCh := make(chan LoopEvent, 32)

	// Clean up on disconnect
	defer func() {
		if currentLoopID != "" {
			s.mu.RLock()
			inst, ok := s.loops[currentLoopID]
			s.mu.RUnlock()
			if ok {
				inst.removeSSEClient(clientCh)
			}
		}
		close(clientCh)
		log.Printf("WebSocket client disconnected")
	}()

	// Send initial connection acknowledgement
	_ = conn.WriteJSON(map[string]interface{}{
		"type": "connect",
	})

	// Event forwarder
	go func() {
		for event := range clientCh {
			err := conn.WriteJSON(map[string]interface{}{
				"type":      "event",
				"payload":   event,
				"sessionId": event.ID,
			})
			if err != nil {
				return
			}
		}
	}()

	// Command listener
	for {
		var msg struct {
			Type      string `json:"type"`
			SessionID string `json:"sessionId"`
			Content   string `json:"content"`
		}
		if err := conn.ReadJSON(&msg); err != nil {
			break
		}

		switch msg.Type {
		case "subscribe":
			if currentLoopID != "" {
				s.mu.RLock()
				if inst, ok := s.loops[currentLoopID]; ok {
					inst.removeSSEClient(clientCh)
				}
				s.mu.RUnlock()
			}
			
			currentLoopID = msg.SessionID
			s.mu.RLock()
			inst, ok := s.loops[currentLoopID]
			s.mu.RUnlock()
			
			if ok {
				inst.addSSEClient(clientCh)
				_ = conn.WriteJSON(map[string]interface{}{
					"type":      "subscribe",
					"sessionId": currentLoopID,
				})
			}
		case "ping":
			_ = conn.WriteJSON(map[string]interface{}{
				"type":      "pong",
				"timestamp": time.Now().Format(time.RFC3339),
			})
		}
	}
}
