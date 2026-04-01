package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/cache"
	"github.com/openexec/openexec/internal/checkpoint"
	"github.com/openexec/openexec/internal/config"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/memory"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/predictive"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/quality"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)
// PipelineStatus represents the lifecycle state of a managed pipeline.
type PipelineStatus string

const (
	StatusStarting PipelineStatus = "starting"
	StatusRunning  PipelineStatus = "running"
	StatusPaused   PipelineStatus = "paused"
	StatusComplete PipelineStatus = "complete"
	StatusError    PipelineStatus = "error"
	StatusStopped  PipelineStatus = "stopped"
)

// Config holds server-level configuration set once at startup.
type Config struct {
	WorkDir              string
	AgentsFS             fs.FS
	ExecutorModel        string   // model for runner resolution
	RunnerCommand        string   // CLI override
	RunnerArgs           []string // CLI args override
	DefaultMaxIterations int
	MaxRetries           int
	MaxReviewCycles      int
	ThrashThreshold      int
	RetryBackoff         []time.Duration
	CommandName          string   // test override
	CommandArgs          []string // test override
	LogDir               string
	// ExecMode: read-only | workspace-write | danger-full-access
	ExecMode    string
	BlueprintID string
	// ReviewEnabled enables code review after task execution
	ReviewEnabled bool
	// ReviewerModel is the model to use for code review
	ReviewerModel string
	StateStore    *state.Store
	AuditLogger audit.Logger // optional audit logger for run-step events
	// PIIScrubLevel controls PII scrubbing sensitivity for audit logs
	// Valid values: "low", "medium", "high", "" (disabled)
	PIIScrubLevel string
	// TaskTimeout overrides the default implement stage timeout.
	// Read from config.json execution.timeout_seconds.
	TaskTimeout time.Duration
}

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
}

// ErrNoWorkDir is returned when Manager is created without a WorkDir.
var ErrNoWorkDir = fmt.Errorf("CRITICAL: WorkDir not configured; set WorkDir in manager.Config")

// New creates a Manager with the given server-level config.
// Returns error if WorkDir is empty (fail-fast for workspace scoping).
func New(cfg Config) (*Manager, error) {
	// FAIL FAST: WorkDir is mandatory for workspace-scoped execution
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
	// If the server crashed while tasks were running, they are stuck in the DB
	// as 'running' or 'starting'. We must reset them to 'pending' on startup.
	relMgr, err := m.getInternalReleaseManager()
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

	m.watchdog = NewWatchdog(m)
	go m.watchdog.Run(context.Background())
	return m, nil
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

// WithExecMode sets execution mode for this pipeline
func WithExecMode(mode string) StartOption {
    return func(cfg *pipeline.Config) {
        cfg.ExecMode = mode
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

// WithBlueprint enables blueprint mode with the specified blueprint ID.
func WithBlueprint(blueprintID string) StartOption {
    return func(cfg *pipeline.Config) {
        cfg.BlueprintID = blueprintID
    }
}

// WithTaskDescription sets the task description for blueprint runs.
func WithTaskDescription(description string) StartOption {
    return func(cfg *pipeline.Config) {
        cfg.TaskDescription = description
    }
}

// Start launches a new pipeline for the given FWU ID.
// Returns error if the pipeline is already active (non-terminal state).
// Allows re-start after complete/error/stopped.
func (m *Manager) Start(ctx context.Context, fwuID string, opts ...StartOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.pipelines[fwuID]; ok && !isTerminal(e.info.Status) {
		return fmt.Errorf("pipeline %s already active (status: %s)", fwuID, e.info.Status)
	}

	rel, err := m.getInternalReleaseManager()
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
        BlueprintID:          m.cfg.BlueprintID, // Use global default if available
        ReviewEnabled:        m.cfg.ReviewEnabled,
        ReviewerModel:        m.cfg.ReviewerModel,
        TaskDescription:      "",
        TaskTimeout:          m.cfg.TaskTimeout,
    }

	for _, opt := range opts {
		opt(&pCfg)
	}

	// Auto-populate from ReleaseManager if not explicitly set via options
	if pCfg.TaskDescription == "" || pCfg.BlueprintID == "" {
		if task := rel.GetTask(fwuID); task != nil {
			if pCfg.TaskDescription == "" {
				pCfg.TaskDescription = task.Description
			}
			if pCfg.BlueprintID == "" {
				pCfg.BlueprintID = "standard_task" // Default for all tasks
			}
		}
	}

	// Create factory using the same manager
    factory := pipeline.NewLoopFactory(pipeline.LoopFactoryConfig{
        FWUID:                pCfg.FWUID,
        WorkDir:              pCfg.WorkDir,
        AgentsFS:             pCfg.AgentsFS,
        ReleaseManager:       rel,
        DefaultMaxIterations: pCfg.DefaultMaxIterations,
        MaxRetries:           pCfg.MaxRetries,
        RetryBackoff:         pCfg.RetryBackoff,
        ThrashThreshold:      pCfg.ThrashThreshold,
        ExecutorModel:        pCfg.ExecutorModel,
        RunnerCommand:        pCfg.RunnerCommand,
        RunnerArgs:           pCfg.RunnerArgs,
        CommandName:          pCfg.CommandName,
        CommandArgs:          pCfg.CommandArgs,
        LogDir:               pCfg.LogDir,
        ExecMode:             pCfg.ExecMode,
        BlueprintID:          pCfg.BlueprintID,
        TaskDescription:      pCfg.TaskDescription,
    })

	p, events := pipeline.NewWithFactory(pCfg, factory)

	// Initialize quality gate runner and inject into pipeline
	if runner, err := gates.NewRunner(m.cfg.WorkDir, 5*time.Minute); err == nil {
		pipeline.WithGateRunner(&gateRunnerAdapter{runner: runner})(p)
	}

	// Wire quality gates V2 if enabled in project config
	if projCfg, err := project.LoadProjectConfig(m.cfg.WorkDir); err == nil && projCfg.Execution.QualityGatesV2 {
		projectType := quality.DetectProjectType(m.cfg.WorkDir)
		qm := quality.NewManager(m.cfg.WorkDir, quality.DefaultGates(projectType))
		pipeline.WithQualityManager(qm)(p)
	}

	// Wire checkpoint manager for crash recovery if enabled in project config
	if projCfg, err := project.LoadProjectConfig(m.cfg.WorkDir); err == nil && projCfg.Execution.CheckpointEnabled {
		if cm, err := checkpoint.NewManager(m.cfg.WorkDir); err == nil {
			pipeline.WithCheckpointManager(cm)(p)
		}
	}

	// Wire caching and predictive loading if enabled in project config
	if projCfg, err := project.LoadProjectConfig(m.cfg.WorkDir); err == nil {
		if projCfg.Execution.CacheEnabled {
			if kc, err := cache.NewKnowledgeCache(m.cfg.WorkDir, 24*time.Hour); err == nil {
				pipeline.WithKnowledgeCache(kc)(p)
			}
			if trc, err := cache.NewToolResultCache(m.cfg.WorkDir, 1*time.Hour); err == nil {
				pipeline.WithToolResultCache(trc)(p)
			}
		}
		if projCfg.Execution.PredictiveLoad {
			if loader, err := predictive.NewLoader(m.cfg.WorkDir, nil, predictive.DefaultLoaderConfig()); err == nil {
				pipeline.WithPredictiveLoader(loader)(p)
			}
		}
		if projCfg.Execution.MemoryEnabled {
			if mm, err := memory.NewMemoryManager(m.cfg.WorkDir); err == nil {
				pipeline.WithMemoryManager(mm)(p)
			}
		}
	}

	// Deterministic routing is always on
	dr := router.NewDeterministicRouter()
	pipeline.WithRouter(dr)(p)

	// BitNet routing upgrade (optional, feature flag)
	if projCfg, err := project.LoadProjectConfig(m.cfg.WorkDir); err == nil && projCfg.Execution.BitNetRouting {
		modelPath := projCfg.Execution.BitNetModel
		if modelPath == "" {
			modelPath = "/models/bitnet-2b.gguf"
		}
		br := router.NewBitNetRouter(modelPath)
		br.SetSkipAvailabilityCheck(false)
		pipeline.WithRouter(br)(p) // Overrides deterministic
	}

	// Create OTel span for the entire run lifecycle
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

	// Write run to unified DB synchronously so it exists before any
	// run_step or checkpoint writes (which have FK constraints on run_id).
	if m.state != nil {
		if err := m.state.CreateRun(ctx, fwuID, "", "", m.cfg.WorkDir, pCfg.ExecMode); err != nil {
			log.Printf("[Manager] Failed to create run record for %s: %v", fwuID, err)
		}
	}

	// Start event consumer before pipeline run.
	go m.consumeEvents(fwuID, events)

	// Run pipeline in background.
	go func() {
		log.Printf("[Manager] Pipeline %s: running", fwuID)
		err := p.Run(pipeCtx)
		m.mu.Lock()
		defer m.mu.Unlock()
		if e, ok := m.pipelines[fwuID]; ok {
			if err != nil && !isTerminal(e.info.Status) {
				e.info.Status = StatusError
				e.info.Error = err.Error()
				log.Printf("[Manager] Pipeline %s: failed with error: %v", fwuID, err)
				// Record error on run span
				e.runSpan.RecordError(err)
				e.runSpan.SetAttributes(attribute.String("run.status", string(StatusError)))
			} else if err != nil {
				log.Printf("[Manager] Pipeline %s: finished (terminal status=%s) with error: %v", fwuID, e.info.Status, err)
				e.runSpan.SetAttributes(attribute.String("run.status", string(e.info.Status)))
			} else {
				log.Printf("[Manager] Pipeline %s: finished with status=%s", fwuID, e.info.Status)
				e.runSpan.SetAttributes(attribute.String("run.status", string(e.info.Status)))
			}
			// End the run span when pipeline completes
			e.runSpan.End()
		}
	}()

	return nil
}

// Stop terminates the pipeline for the given FWU ID.
func (m *Manager) Stop(fwuID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	e, ok := m.pipelines[fwuID]
	if !ok {
		return fmt.Errorf("pipeline %s not found", fwuID)
	}
	if isTerminal(e.info.Status) {
		return fmt.Errorf("pipeline %s already in terminal state: %s", fwuID, e.info.Status)
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
	if !ok {
		return fmt.Errorf("pipeline %s not found", fwuID)
	}
	if isTerminal(e.info.Status) {
		return fmt.Errorf("pipeline %s already in terminal state: %s", fwuID, e.info.Status)
	}

	e.pipeline.Pause()
	return nil
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

	// Get real-time health from pipeline
	if h, ok := e.pipeline.GetHealth(); ok {
		info.Iteration = h.Iteration
		info.LastActivity = h.LastActivity
		info.CurrentPID = h.CurrentPID
	}

    info.Elapsed = time.Since(info.StartedAt).Truncate(time.Second).String()
    // include drop count
    m.mu.RLock()
    if e, ok := m.pipelines[fwuID]; ok {
        info.DroppedEvents = e.drops
    }
    m.mu.RUnlock()
    return info, nil
}

// List returns info snapshots for all known pipelines.
func (m *Manager) List() []PipelineInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]PipelineInfo, 0, len(m.pipelines))
	for _, e := range m.pipelines {
		info := e.info

		// Get real-time health from pipeline
		if h, ok := e.pipeline.GetHealth(); ok {
			info.Iteration = h.Iteration
			info.LastActivity = h.LastActivity
			info.CurrentPID = h.CurrentPID
		}

        info.Elapsed = time.Since(info.StartedAt).Truncate(time.Second).String()
        info.DroppedEvents = e.drops
        result = append(result, info)
    }
    return result
}

// GetConfig returns the manager's configuration.
func (m *Manager) GetConfig() Config {
	return m.cfg
}

func (m *Manager) getInternalReleaseManager() (*release.Manager, error) {
	if m.state == nil {
		return nil, fmt.Errorf("state store not configured")
	}
	rel, err := release.NewManagerWithDB(m.cfg.WorkDir, release.DefaultConfig(), m.state.GetDB())
	if err != nil {
		return nil, err
	}
	return rel, nil
}

// ExportJSON exports the current release state to JSON files for backward compatibility.
func (m *Manager) ExportJSON(dir string) error {
	rel, err := m.getInternalReleaseManager()
	if err != nil {
		return err
	}

	// Fetch and map goals
	relGoals := rel.GetGoals()
	plannerGoals := make([]planner.Goal, len(relGoals))
	for i, g := range relGoals {
		plannerGoals[i] = planner.Goal{
			ID:                 g.ID,
			Title:              g.Title,
			Description:        g.Description,
			SuccessCriteria:    g.SuccessCriteria,
			VerificationMethod: g.VerificationMethod,
		}
	}

	// Fetch and map stories
	relStories := rel.GetStories()
	plannerStories := make([]planner.Story, len(relStories))
	for i, s := range relStories {
		// Map tasks for this story
		relTasks := rel.GetTasksForStory(s.ID)
		plannerTasks := make([]planner.Task, len(relTasks))
		for j, t := range relTasks {
			plannerTasks[j] = planner.Task{
				ID:                 t.ID,
				Title:              t.Title,
				Description:        t.Description,
				VerificationScript: t.VerificationScript,
				DependsOn:          t.DependsOn,
			}
		}

		plannerStories[i] = planner.Story{
			ID:                 s.ID,
			GoalID:             s.GoalID,
			Title:              s.Title,
			Description:        s.Description,
			AcceptanceCriteria: s.AcceptanceCriteria,
			VerificationScript: s.VerificationScript,
			DependsOn:          s.DependsOn,
			Tasks:              plannerTasks,
		}
	}

	plan := &planner.ProjectPlan{
		Goals:   plannerGoals,
		Stories: plannerStories,
	}

	data, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, "stories.json"), data, 0644)
}

// gateRunnerAdapter adapts gates.Runner to the blueprint.GateRunner interface.
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
