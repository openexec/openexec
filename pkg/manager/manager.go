package manager

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/config"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/pipeline"
	"github.com/openexec/openexec/internal/release"
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
	TractStore           string
	AgentsFS             fs.FS
	ExecutorModel        string   // model for runner resolution
	RunnerCommand        string   // CLI override
	RunnerArgs           []string // CLI args override
	Pipeline             *pipeline.PipelineDef                   // pipeline config (nil = default)
	Phases               map[pipeline.Phase]pipeline.PhaseConfig // test override (nil = DefaultPhaseConfigs)
	Order                []pipeline.Phase                        // test override (nil = DefaultPhaseOrder)
	DefaultMaxIterations int
	MaxRetries           int
	MaxReviewCycles      int
	ThrashThreshold      int
	RetryBackoff         []time.Duration
	CommandName          string   // test override
	CommandArgs          []string // test override
	LogDir               string
	BriefingFunc         pipeline.BriefingFunc // test override (nil = TractBriefingFunc)
}

// PipelineInfo is the external status snapshot of a managed pipeline.
type PipelineInfo struct {
	FWUID        string         `json:"fwu_id"`
	Status       PipelineStatus `json:"status"`
	Phase        string         `json:"phase,omitempty"`
	Agent        string         `json:"agent,omitempty"`
	Iteration    int            `json:"iteration,omitempty"`
	ReviewCycles int            `json:"review_cycles,omitempty"`
	StartedAt    time.Time      `json:"started_at"`
	Elapsed      string         `json:"elapsed"`
	Error        string         `json:"error,omitempty"`
	LastActivity time.Time      `json:"last_activity"`
	CurrentPID   int            `json:"current_pid,omitempty"`
}

type entry struct {
	pipeline *pipeline.Pipeline
	info     PipelineInfo
	cancel   context.CancelFunc
	subs     []chan loop.Event
	subsMu   sync.Mutex
}

// Manager orchestrates multiple concurrent FWU pipelines.
type Manager struct {
	cfg       Config
	pipelines map[string]*entry
	mu        sync.RWMutex
	watchdog  *Watchdog
}

// New creates a Manager with the given server-level config.
func New(cfg Config) *Manager {
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
	}

	// SELF-HEALING: Ghost State Cleanup
	// If the server crashed while tasks were running, they are stuck in the DB
	// as 'running' or 'starting'. We must reset them to 'pending' on startup.
	if cfg.WorkDir != "" {
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
	}

	m.watchdog = NewWatchdog(m)
	go m.watchdog.Run(context.Background())
	return m
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

// Start launches a new pipeline for the given FWU ID.
// Returns error if the pipeline is already active (non-terminal state).
// Allows re-start after complete/error/stopped.
func (m *Manager) Start(ctx context.Context, fwuID string, opts ...StartOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if e, ok := m.pipelines[fwuID]; ok && !isTerminal(e.info.Status) {
		return fmt.Errorf("pipeline %s already active (status: %s)", fwuID, e.info.Status)
	}

	pCfg := pipeline.Config{
		FWUID:                fwuID,
		WorkDir:              m.cfg.WorkDir,
		TractStore:           m.cfg.TractStore,
		AgentsFS:             m.cfg.AgentsFS,
		ExecutorModel:        m.cfg.ExecutorModel,
		RunnerCommand:        m.cfg.RunnerCommand,
		RunnerArgs:           m.cfg.RunnerArgs,
		Pipeline:             m.cfg.Pipeline,
		Phases:               m.cfg.Phases,
		Order:                m.cfg.Order,
		DefaultMaxIterations: m.cfg.DefaultMaxIterations,
		MaxRetries:           m.cfg.MaxRetries,
		MaxReviewCycles:      m.cfg.MaxReviewCycles,
		ThrashThreshold:      m.cfg.ThrashThreshold,
		RetryBackoff:         m.cfg.RetryBackoff,
		CommandName:          m.cfg.CommandName,
		CommandArgs:          m.cfg.CommandArgs,
		LogDir:               m.cfg.LogDir,
		BriefingFunc:         m.cfg.BriefingFunc,
	}

	for _, opt := range opts {
		opt(&pCfg)
	}

	p, events := pipeline.New(pCfg)

	pipeCtx, cancel := context.WithCancel(ctx)

	e := &entry{
		pipeline: p,
		info: PipelineInfo{
			FWUID:     fwuID,
			Status:    StatusStarting,
			StartedAt: time.Now(),
		},
		cancel: cancel,
	}
	m.pipelines[fwuID] = e

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
			} else if err != nil {
				log.Printf("[Manager] Pipeline %s: finished (terminal status=%s) with error: %v", fwuID, e.info.Status, err)
			} else {
				log.Printf("[Manager] Pipeline %s: finished with status=%s", fwuID, e.info.Status)
			}
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
		result = append(result, info)
	}
	return result
}

// GetConfig returns the manager's configuration.
func (m *Manager) GetConfig() Config {
	return m.cfg
}

func (m *Manager) getInternalReleaseManager() (*release.Manager, error) {
	rel, err := release.NewManager(m.cfg.WorkDir, release.DefaultConfig())
	if err != nil {
		return nil, err
	}
	if err := rel.Load(); err != nil {
		return nil, err
	}
	return rel, nil
}
