package manager

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/config"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/quality/checklist"
	"github.com/openexec/openexec/internal/quality/nostubs"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/skills"
	pagent "github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/telemetry"
	"go.opentelemetry.io/otel/trace"

	_ "modernc.org/sqlite"
)

// PipelineStatus represents the current state of a pipeline.
type PipelineStatus string

const (
	StatusCreated  PipelineStatus = "created"
	StatusStarting PipelineStatus = "starting"
	StatusRunning  PipelineStatus = "running"
	StatusPaused   PipelineStatus = "paused"
	StatusComplete PipelineStatus = "complete"
	StatusError    PipelineStatus = "error"
	StatusStopped  PipelineStatus = "stopped"
)

// PipelineInfo is the external status snapshot of a managed pipeline.
type PipelineInfo struct {
	FWUID         string         `json:"fwu_id"`
	Status        PipelineStatus `json:"status"`
	Stage         string         `json:"stage,omitempty"` // current blueprint stage
	Agent         string         `json:"agent,omitempty"`
	Iteration     int            `json:"iteration,omitempty"`
	ReviewCycles  int            `json:"review_cycles,omitempty"`
	StartedAt     time.Time      `json:"started_at"`
	Elapsed       string         `json:"elapsed"`
	Error         string         `json:"error,omitempty"`
	LastActivity  time.Time      `json:"last_activity"`
	CurrentPID    int            `json:"current_pid,omitempty"`
	DroppedEvents int            `json:"dropped_events,omitempty"`
	DoneTasks     int            `json:"done_tasks"`
	TotalTasks    int            `json:"total_tasks"`
}

type entry struct {
	pipeline *pipeline.Pipeline
	info     PipelineInfo
	cancel   context.CancelFunc
	subs     []chan loop.Event
	subsMu   sync.Mutex
	drops    int
	stepSeq  int
	traceID  string
	runSpan  trace.Span // OTel span for the entire run lifecycle
}

// Manager orchestrates multiple concurrent FWU pipelines.
type Manager struct {
	cfg       Config
	pipelines map[string]*entry
	mu        sync.RWMutex
	watchdog  *Watchdog
	state     *state.Store
	cancel    context.CancelFunc // Cancels watchdog goroutine
	relMu     sync.Mutex         // Serializes release manager creation
	rel       *release.Manager   // Cached release manager
}

// Config defines settings for the manager.
type Config struct {
	WorkDir              string
	AgentsFS             fs.FS
	LogDir               string
	ExecutorModel        string
	RunnerCommand        string
	RunnerArgs           []string
	CommandName          string
	CommandArgs          []string
	StateStore           *state.Store
	DefaultMaxIterations int
	MaxRetries           int
	MaxReviewCycles      int
	ThrashThreshold      int
	RetryBackoff         []time.Duration
	TaskTimeout          time.Duration
	ExecMode             string
	BlueprintID          string
	ReviewEnabled        bool
	ReviewerModel        string
	AuditLogger          audit.Logger
	PIIScrubLevel        string
}

// ErrNoWorkDir is returned when Manager is created without a WorkDir.
var ErrNoWorkDir = fmt.Errorf("CRITICAL: WorkDir not configured; set WorkDir in manager.Config")

// New creates a Manager with the given server-level config.
func New(cfg Config) (*Manager, error) {
	if cfg.WorkDir == "" {
		return nil, ErrNoWorkDir
	}

	if cfg.DefaultMaxIterations == 0 {
		cfg.DefaultMaxIterations = config.DefaultMaxIterations
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = config.DefaultMaxRetries
	}
	if cfg.MaxReviewCycles == 0 {
		cfg.MaxReviewCycles = config.DefaultMaxReviewCycles
	}
	if cfg.ThrashThreshold == 0 {
		cfg.ThrashThreshold = config.DefaultThrashThreshold
	}
	if cfg.RetryBackoff == nil {
		cfg.RetryBackoff = config.DefaultRetryBackoff
	}
	m := &Manager{
		cfg:       cfg,
		pipelines: make(map[string]*entry),
		state:     cfg.StateStore,
	}

	// SELF-HEALING: Ghost State Cleanup
	relMgr, err := m.GetInternalReleaseManager()
	if err == nil {
		tasks := relMgr.GetTasks()
		resetCount := 0
		for _, t := range tasks {
			if t.Status == "running" || t.Status == "starting" {
				t.Status = "pending"
				_ = relMgr.UpdateTask(t)
				resetCount++
			}
		}
		if resetCount > 0 {
			log.Printf("[Manager] ✨ Self-Healed: Reset %d ghost tasks to pending", resetCount)
		}
	}

	// SELF-HEALING: Orphan run cleanup
	if m.state != nil {
		if n, err := m.state.CleanupOrphanRuns(context.Background(), m.cfg.WorkDir); err != nil {
			log.Printf("[Manager] Orphan run cleanup failed: %v", err)
		} else if n > 0 {
			log.Printf("[Manager] ✨ Self-Healed: Marked %d orphan runs as stopped", n)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.watchdog = NewWatchdog(m)
	go m.watchdog.Run(ctx)
	return m, nil
}

// Close shuts down the manager.
func (m *Manager) Close() {
	if m.cancel != nil {
		m.cancel()
	}
}

// EnsureReady performs pre-flight checks and setup.
func (m *Manager) EnsureReady(ctx context.Context) error {
	if projCfg, err := project.LoadProjectConfig(m.cfg.WorkDir); err == nil && projCfg.Execution.BitNetRouting {
		mm := router.NewModelManager(router.DefaultModelName)
		if !mm.CheckModelExists() {
			log.Printf("[Manager] 🚨 BitNet model missing. Starting blocking download (2GB)...")
			if _, err := mm.Download(); err != nil {
				return fmt.Errorf("bitnet model download failed: %w", err)
			}
			log.Printf("[Manager] ✓ BitNet model ready.")
		}
	}
	m.StartIndexing()
	return nil
}

// StartIndexing triggers the project indexing process.
func (m *Manager) StartIndexing() {
	db := m.state.GetDB()
	store, err := knowledge.NewStoreWithDB(db)
	if err != nil {
		log.Printf("[Indexer] knowledge store init failed: %v", err)
		return
	}

	var lastIndexedStr sql.NullString
	if err := db.QueryRow(`SELECT MAX(updated_at) FROM symbols`).Scan(&lastIndexedStr); err == nil && lastIndexedStr.Valid {
		if t, perr := time.Parse("2006-01-02 15:04:05", lastIndexedStr.String); perr == nil {
			if time.Since(t) < time.Hour {
				log.Printf("[Indexer] Skipping: symbols table indexed %s ago", time.Since(t).Truncate(time.Second))
				return
			}
		}
	}

	indexer := knowledge.NewIndexer(store)
	go func() {
		time.Sleep(5 * time.Second)
		start := time.Now()
		if err := indexer.IndexProject(m.cfg.WorkDir); err != nil {
			log.Printf("[Indexer] IndexProject failed: %v", err)
			return
		}
		stats := indexer.GetStats()
		log.Printf("[Indexer] Indexed %d symbols across %d files in %s",
			stats.SymbolsExtracted, stats.FilesProcessed, time.Since(start).Truncate(time.Millisecond))
	}()
}

// isTerminal returns true if the status represents a finished pipeline.
func isTerminal(s PipelineStatus) bool {
	return s == StatusComplete || s == StatusError || s == StatusStopped
}

// StartOption defines functional options for Start.
type StartOption func(*pipeline.Config)

// WithIsStudy flags the pipeline as documentation/analysis only.
func WithIsStudy(isStudy bool) StartOption {
	return func(cfg *pipeline.Config) {
		cfg.IsStudy = isStudy
	}
}

// WithResumeCheckpoint enables resuming from a checkpoint.
func WithResumeCheckpoint(checkpoint *state.CheckpointData, appliedToolCalls []string) StartOption {
	return func(cfg *pipeline.Config) {
		if checkpoint == nil {
			return
		}
		cfg.ResumeFrom = &pipeline.ResumeConfig{
			CheckpointID:     checkpoint.ID,
			Phase:            checkpoint.Phase,
			Iteration:        checkpoint.Iteration,
			MessageHistory:   checkpoint.MessageHistory,
			AppliedToolCalls: appliedToolCalls,
		}
	}
}

// WithExecMode sets execution mode for this pipeline
func WithExecMode(mode string) StartOption {
	return func(cfg *pipeline.Config) {
		cfg.ExecMode = mode
	}
}

// WithBlueprint enables blueprint mode.
func WithBlueprint(blueprintID string) StartOption {
	return func(cfg *pipeline.Config) {
		cfg.BlueprintID = blueprintID
	}
}

// WithTaskDescription sets the task description.
func WithTaskDescription(description string) StartOption {
	return func(cfg *pipeline.Config) {
		cfg.TaskDescription = description
	}
}

// Start launches a new pipeline.
func (m *Manager) Start(ctx context.Context, fwuID string, opts ...StartOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.pipelines[fwuID]; ok && !isTerminal(e.info.Status) {
		return fmt.Errorf("pipeline %s already active (status: %s)", fwuID, e.info.Status)
	}

	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return fmt.Errorf("load release manager: %w", err)
	}

	pCfg := pipeline.Config{
		FWUID:                fwuID,
		WorkDir:              m.cfg.WorkDir,
		AgentsFS:             m.cfg.AgentsFS,
		ExecutorModel:        m.cfg.ExecutorModel,
		RunnerCommand:        m.cfg.RunnerCommand,
		RunnerArgs:           m.cfg.RunnerArgs,
		DefaultMaxIterations: m.cfg.DefaultMaxIterations,
		MaxRetries:           m.cfg.MaxRetries,
		ThrashThreshold:      m.cfg.ThrashThreshold,
		RetryBackoff:         m.cfg.RetryBackoff,
		CommandName:          m.cfg.CommandName,
		CommandArgs:          m.cfg.CommandArgs,
		LogDir:               m.cfg.LogDir,
		ExecMode:             m.cfg.ExecMode,
		BlueprintID:          m.cfg.BlueprintID,
		ReviewEnabled:        m.cfg.ReviewEnabled,
		ReviewerModel:        m.cfg.ReviewerModel,
		TaskTimeout:          m.cfg.TaskTimeout,
	}

	projCfg, _ := project.LoadProjectConfig(m.cfg.WorkDir)
	if projCfg != nil {
		if name, baseURL, key, model := projCfg.Execution.ActiveAPI(); name != "" {
			pCfg.APIProvider = name
			pCfg.APIBaseURL = baseURL
			pCfg.APIKey = key
			pCfg.APIModel = model
		}
		pCfg.ToolsetFiltering = projCfg.Execution.ToolsetFiltering
		pCfg.LocalPreResolveEnabled = projCfg.Execution.IsLocalPreResolveEnabled()
	}

	// Pass the state store's DB handle so the pre-resolver can query the
	// symbols table without opening a separate connection (Layer 2).
	if m.state != nil {
		pCfg.StateDB = m.state.GetDB()
	}

	for _, opt := range opts {
		opt(&pCfg)
	}

	if pCfg.TaskDescription == "" {
		if task := rel.GetTask(fwuID); task != nil {
			pCfg.TaskDescription = task.Description
		}
	}
	if pCfg.BlueprintID == "" {
		pCfg.BlueprintID = "standard_task"
	}

	factory := pipeline.NewLoopFactory(pipeline.LoopFactoryConfig{
		FWUID:                pCfg.FWUID,
		WorkDir:              pCfg.WorkDir,
		AgentsFS:             pCfg.AgentsFS,
		ReleaseManager:       rel,
		DefaultMaxIterations: pCfg.DefaultMaxIterations,
		MaxRetries:           pCfg.MaxRetries,
		ExecutorModel:        pCfg.ExecutorModel,
		LogDir:               pCfg.LogDir,
		ExecMode:             pCfg.ExecMode,
		BlueprintID:          pCfg.BlueprintID,
		TaskDescription:      pCfg.TaskDescription,
	})

	p, events := pipeline.NewWithFactory(pCfg, factory)

	var gateRunners []ptypesGateRunner
	if runner, err := gates.NewRunner(m.cfg.WorkDir, 5*time.Minute); err == nil {
		gateRunners = append(gateRunners, &gateRunnerAdapter{runner: runner})
	}
	if projCfg == nil || projCfg.QualityGates.IsNoStubsEnabled() {
		var overrides map[string]nostubs.Severity
		if projCfg != nil && len(projCfg.QualityGates.NoStubsRules) > 0 {
			overrides = make(map[string]nostubs.Severity, len(projCfg.QualityGates.NoStubsRules))
			for k, v := range projCfg.QualityGates.NoStubsRules {
				overrides[k] = nostubs.Severity(v)
			}
		}
		nsGate := nostubs.NewGate(m.cfg.WorkDir, nostubs.Config{RuleOverrides: overrides})
		gateRunners = append(gateRunners, nsGate)
	}
	if projCfg == nil || projCfg.QualityGates.IsProductionReadyEnabled() {
		var skip []string
		if projCfg != nil && len(projCfg.QualityGates.ProductionReadySkip) > 0 {
			skip = append(skip, projCfg.QualityGates.ProductionReadySkip...)
		}
		prGate := checklist.NewGate(m.cfg.WorkDir, checklist.Config{Skip: skip})
		gateRunners = append(gateRunners, prGate)
	}
	if len(gateRunners) > 0 {
		pipeline.WithGateRunner(&compositeGateRunner{runners: gateRunners})(p)
	}

	dr := router.NewDeterministicRouter()
	pipeline.WithRouter(dr)(p)

	if projCfg != nil && projCfg.Execution.BitNetRouting {
		br := router.NewBitNetRouter(projCfg.Execution.BitNetModel)
		br.SetProjectDir(m.cfg.WorkDir)
		pipeline.WithRouter(br)(p)
	}

	sr := skills.NewRegistry()
	_ = sr.LoadAll(m.cfg.WorkDir)
	pipeline.WithSkillRegistry(sr)(p)

	runCtx, runSpan := telemetry.StartRunSpan(ctx, fwuID, m.cfg.WorkDir, pCfg.ExecMode)
	pipeCtx, cancel := context.WithCancel(runCtx)

	e := &entry{
		pipeline: p,
		info: PipelineInfo{
			FWUID:     fwuID,
			Status:    StatusStarting,
			StartedAt: time.Now(),
		},
		cancel:  cancel,
		runSpan: runSpan,
	}
	m.pipelines[fwuID] = e

	if m.state != nil {
		_ = m.state.CreateRun(ctx, fwuID, "", "", m.cfg.WorkDir, pCfg.ExecMode)
	}

	go m.consumeEvents(fwuID, events)
	go func() {
		log.Printf("[Manager] Pipeline %s: running", fwuID)
		err := p.Run(pipeCtx)
		m.mu.Lock()
		if e, ok := m.pipelines[fwuID]; ok {
			if err != nil && !isTerminal(e.info.Status) {
				e.info.Status = StatusError
				e.info.Error = err.Error()
				log.Printf("[Manager] Pipeline %s: failed: %v", fwuID, err)
			} else {
				log.Printf("[Manager] Pipeline %s: finished status=%s", fwuID, e.info.Status)
			}
			e.runSpan.End()
		}
		m.mu.Unlock()
	}()
	return nil
}

// Stop terminates the pipeline.
func (m *Manager) Stop(fwuID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.pipelines[fwuID]
	if !ok || isTerminal(e.info.Status) {
		return nil
	}
	e.info.Status = StatusStopped
	e.pipeline.Stop()
	return nil
}

// Pause signals the pipeline for the given FWU ID to pause after the current iteration.
func (m *Manager) Pause(fwuID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.pipelines[fwuID]
	if !ok || isTerminal(e.info.Status) {
		return nil
	}
	e.info.Status = StatusPaused
	e.pipeline.Pause()
	return nil
}

func (m *Manager) GetInternalReleaseManager() (*release.Manager, error) {
	m.relMu.Lock()
	defer m.relMu.Unlock()
	if m.rel != nil {
		return m.rel, nil
	}
	rel, err := release.NewManagerWithDB(m.cfg.WorkDir, release.DefaultConfig(), m.state.GetDB())
	if err != nil {
		return nil, err
	}
	m.rel = rel
	return rel, nil
}

// Status returns the current info snapshot for a pipeline.
func (m *Manager) Status(fwuID string) (PipelineInfo, error) {
	m.mu.RLock()
	e, ok := m.pipelines[fwuID]
	m.mu.RUnlock()

	if !ok {
		return PipelineInfo{}, fmt.Errorf("pipeline %s not found", fwuID)
	}

	info := e.info
	info.Elapsed = time.Since(info.StartedAt).Truncate(time.Second).String()

	if rel, err := m.GetInternalReleaseManager(); err == nil {
		tasks := rel.GetTasks()
		info.TotalTasks = len(tasks)
		doneCount := 0
		for _, t := range tasks {
			if t.Status == release.TaskStatusDone || t.Status == release.TaskStatusApproved {
				doneCount++
			}
		}
		info.DoneTasks = doneCount
	}

	return info, nil
}

// List returns info snapshots for all known pipelines.
func (m *Manager) List() []PipelineInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total, done int
	if rel, err := m.GetInternalReleaseManager(); err == nil {
		tasks := rel.GetTasks()
		total = len(tasks)
		for _, t := range tasks {
			if t.Status == release.TaskStatusDone || t.Status == release.TaskStatusApproved {
				done++
			}
		}
	}

	result := make([]PipelineInfo, 0, len(m.pipelines))
	for _, e := range m.pipelines {
		info := e.info
		info.Elapsed = time.Since(info.StartedAt).Truncate(time.Second).String()
		info.TotalTasks = total
		info.DoneTasks = done
		result = append(result, info)
	}
	return result
}

func (m *Manager) GetConfig() Config {
	return m.cfg
}

// ExportJSON exports the project state to JSON files in the specified directory.
func (m *Manager) ExportJSON(exportDir string) error {
	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		return err
	}
	return rel.ExportJSON(exportDir)
}

// gateRunnerAdapter adapts the gates.Runner to the pipeline.GateRunner interface.
type gateRunnerAdapter struct {
	runner *gates.Runner
}

func (a *gateRunnerAdapter) RunAll(ctx context.Context) error {
	report := a.runner.RunAll(ctx)
	if !report.Passed {
		return fmt.Errorf("%s", report.Summary)
	}
	return nil
}

// ptypesGateRunner is a narrow local alias matching internal/types.GateRunner
// so we can chain multiple runners without importing the types package here.
type ptypesGateRunner interface {
	RunAll(ctx context.Context) error
}

// compositeGateRunner runs a sequence of gate runners and returns a combined
// error. All runners are executed even if an earlier one fails so the user
// sees every failure at once.
type compositeGateRunner struct {
	runners []ptypesGateRunner
}

func (c *compositeGateRunner) RunAll(ctx context.Context) error {
	var errs []string
	for _, r := range c.runners {
		if err := r.RunAll(ctx); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("quality gates failed: %s", strings.Join(errs, "; "))
}

func (m *Manager) createAPIProvider(projCfg *project.ProjectConfig) pagent.ProviderAdapter {
	return nil // Implementation stub
}

func (m *Manager) createWorkerProvider(projCfg *project.ProjectConfig) pagent.ProviderAdapter {
	return nil // Implementation stub
}
