package pipeline

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openexec/openexec/internal/config"
	"github.com/openexec/openexec/internal/loop"
)

// Config controls pipeline behavior.
type Config struct {
	FWUID                string
	WorkDir              string
	TractStore           string
	AgentsFS             fs.FS
	ExecutorModel        string   // model for runner resolution
	RunnerCommand        string   // CLI override
	RunnerArgs           []string // CLI args override
	Pipeline             *PipelineDef          // if set, overrides Phases/Order
	Phases               map[Phase]PhaseConfig // defaults to DefaultPhaseConfigs()
	Order                []Phase               // defaults to DefaultPhaseOrder()
	MaxReviewCycles      int                   // default 3
	DefaultMaxIterations int                   // default 10
	MaxRetries           int                   // default 3
	RetryBackoff         []time.Duration
	ThrashThreshold      int          // default 3
	BriefingFunc         BriefingFunc // nil = TractBriefingFunc(TractStore)
	CommandName          string       // test override
	CommandArgs          []string     // test override

	// IsStudy flags the task as documentation/analysis only (collapses phases).
	IsStudy bool

	// Log configuration
	LogDir string

	// Evidence configuration
	EvidenceDir      string
	EvidenceBucket   string
	EvidenceRegion   string
    EvidenceEndpoint string
    EvidencePrefix   string

    // ExecMode: read-only | workspace-write | danger-full-access
    ExecMode string
}

// Pipeline drives an FWU through TD → IM → RV → RF → FL phases.
type Pipeline struct {
	cfg     Config
	sm      *StateMachine
	factory *LoopFactory
	events  chan loop.Event

	briefingCache string

	currentLoop *loop.Loop
	mu          sync.Mutex

	paused  atomic.Bool
	stopped atomic.Bool
}

// New creates a Pipeline and returns it along with a read-only event channel.
// The channel is closed when Run returns.
func New(cfg Config) (*Pipeline, <-chan loop.Event) {
    factory := NewLoopFactory(LoopFactoryConfig{
        FWUID:                cfg.FWUID,
        WorkDir:              cfg.WorkDir,
        TractStore:           cfg.TractStore,
        AgentsFS:             cfg.AgentsFS,
        DefaultMaxIterations: cfg.DefaultMaxIterations,
        MaxRetries:           cfg.MaxRetries,
        MaxReviewCycles:      cfg.MaxReviewCycles,
        RetryBackoff:         cfg.RetryBackoff,
        ThrashThreshold:      cfg.ThrashThreshold,
        ExecutorModel:        cfg.ExecutorModel,
        RunnerCommand:        cfg.RunnerCommand,
        RunnerArgs:           cfg.RunnerArgs,
        CommandName:          cfg.CommandName,
        CommandArgs:          cfg.CommandArgs,
        LogDir:               cfg.LogDir,
        EvidenceDir:          cfg.EvidenceDir,
        EvidenceBucket:       cfg.EvidenceBucket,
        EvidenceRegion:       cfg.EvidenceRegion,
        EvidenceEndpoint:     cfg.EvidenceEndpoint,
        EvidencePrefix:       cfg.EvidencePrefix,
        ExecMode:             cfg.ExecMode,
    })
    return NewWithFactory(cfg, factory)
}

// NewWithFactory creates a Pipeline using a pre-configured factory.
func NewWithFactory(cfg Config, factory *LoopFactory) (*Pipeline, <-chan loop.Event) {
    // Apply defaults. Pipeline takes precedence over Phases/Order.
    if cfg.Pipeline != nil {
        cfg.Order = cfg.Pipeline.PhaseOrder()
        cfg.Phases = cfg.Pipeline.PhaseConfigs()
    }
    if cfg.Order == nil {
        cfg.Order = DefaultPhaseOrder()
    }
    if cfg.Phases == nil {
        cfg.Phases = DefaultPhaseConfigs()
    }
    // Collapse Study tasks to TD -> FL only
    if cfg.IsStudy {
        cfg.Order = []Phase{PhaseTD, PhaseFL}
        // Ensure phases exist for TD and FL
        ph := DefaultPhaseConfigs()
        cfg.Phases = map[Phase]PhaseConfig{
            PhaseTD: ph[PhaseTD],
            PhaseFL: ph[PhaseFL],
        }
    }
	if cfg.MaxReviewCycles == 0 {
		cfg.MaxReviewCycles = config.DefaultMaxReviewCycles
	}
	if cfg.DefaultMaxIterations == 0 {
		cfg.DefaultMaxIterations = config.DefaultMaxIterations
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = config.DefaultMaxRetries
	}
	if cfg.RetryBackoff == nil {
		cfg.RetryBackoff = config.DefaultRetryBackoff
	}
	if cfg.ThrashThreshold == 0 {
		cfg.ThrashThreshold = config.DefaultThrashThreshold
	}

	ch := make(chan loop.Event, 64)

	p := &Pipeline{
		cfg:     cfg,
		sm:      NewStateMachine(cfg.Order, cfg.Phases, cfg.MaxReviewCycles),
		factory: factory,
		events:  ch,
	}

	return p, ch
}

// Run executes the pipeline until all phases complete, a phase is paused/blocked,
// or the context is cancelled. It closes the event channel when it returns.
func (p *Pipeline) Run(ctx context.Context) error {
	defer close(p.events)

	// Build MCP config once, shared across all phases.
	mcpPath, cleanup, err := p.buildMCPConfig()
	if err != nil {
		return fmt.Errorf("build MCP config: %w", err)
	}
	defer cleanup()

	// Resolve briefing function.
	briefingFn := p.cfg.BriefingFunc
	if briefingFn == nil {
		briefingFn = TractBriefingFunc(p.factory.cfg.ReleaseManager)
	}

	// Build factory.
    p.factory = NewLoopFactory(LoopFactoryConfig{
        FWUID:                p.cfg.FWUID,
        WorkDir:              p.cfg.WorkDir,
        TractStore:           p.cfg.TractStore,
        AgentsFS:             p.cfg.AgentsFS,
        MCPConfigPath:        mcpPath,
        DefaultMaxIterations: p.cfg.DefaultMaxIterations,
        MaxRetries:           p.cfg.MaxRetries,
        RetryBackoff:         p.cfg.RetryBackoff,
        ThrashThreshold:      p.cfg.ThrashThreshold,
        ExecutorModel:        p.cfg.ExecutorModel,
        RunnerCommand:        p.cfg.RunnerCommand,
        RunnerArgs:           p.cfg.RunnerArgs,
        CommandName:          p.cfg.CommandName,
        CommandArgs:          p.cfg.CommandArgs,
        LogDir:               p.cfg.LogDir,
        EvidenceDir:          p.cfg.EvidenceDir,
        EvidenceBucket:       p.cfg.EvidenceBucket,
        EvidenceRegion:       p.cfg.EvidenceRegion,
        EvidenceEndpoint:     p.cfg.EvidenceEndpoint,
        EvidencePrefix:       p.cfg.EvidencePrefix,
        ExecMode:             p.cfg.ExecMode,
    })

	// Phase loop.
	for p.sm.Current() != PhaseDone {
		if p.stopped.Load() {
			return nil
		}
		if p.paused.Load() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		phase := p.sm.Current()
		phaseCfg, ok := p.sm.CurrentConfig()
		if !ok {
			return fmt.Errorf("no config for phase %s", phase)
		}

		// Emit phase_start.
        p.emit(loop.Event{
            Type:        loop.EventPhaseStart,
            Phase:       string(phase),
            FWUID:       p.cfg.FWUID,
            Agent:       phaseCfg.Agent,
            ReviewCycle: p.sm.ReviewCycles(),
        })

		// Fetch fresh briefing once per phase.
		briefing, err := briefingFn(ctx, p.cfg.FWUID)
		if err != nil {
			return fmt.Errorf("briefing for phase %s: %w", phase, err)
		}

		// Doc-only fast-path: if this is a Study/Mapping task and we just finished TD,
		// jump straight to Feedback Loop (FL) to finalize.
		isStudy := p.cfg.IsStudy
		
		// Run Loop, consume events (with retries for transient orchestrator errors)
		var phaseCompleted, routed, blocked bool
		var runErr error
		
		for retry := 0; retry <= p.cfg.MaxRetries; retry++ {
			if retry > 0 {
				p.emit(loop.Event{
					Type: loop.EventError,
					Text: fmt.Sprintf("phase %s failed (attempt %d/%d), retrying in %v...", phase, retry, p.cfg.MaxRetries+1, p.cfg.RetryBackoff[retry-1]),
				})
				time.Sleep(p.cfg.RetryBackoff[retry-1])
				
				// Re-fetch on retry if requested by user for extra "active healing"
				if b, err := briefingFn(ctx, p.cfg.FWUID); err == nil {
					briefing = b
				}
			}

			// Create Loop for this phase.
			l, loopCh, err := p.factory.Create(briefing, phaseCfg)
			if err != nil {
				runErr = fmt.Errorf("create loop for phase %s: %w", phase, err)
				continue
			}

			p.mu.Lock()
			p.currentLoop = l
			p.mu.Unlock()

            phaseCompleted, routed, blocked, runErr = p.runPhase(ctx, l, loopCh, phase, phaseCfg)
            if runErr == nil {
                break
            }
			
			// If we reached here, it's a phase execution error.
			// Only retry transient-looking errors. 
			// (Future: improve classification)
		}

		if runErr != nil {
			return runErr
		}

		p.mu.Lock()
		p.currentLoop = nil
		p.mu.Unlock()

		// Check if we were stopped or paused during the phase.
		if p.stopped.Load() {
			return nil
		}
		if p.paused.Load() {
			return nil
		}

		if blocked {
			// Pipeline pauses for operator attention.
			return nil
		}

		// Emit phase_complete.
        p.emit(loop.Event{
            Type:        loop.EventPhaseComplete,
            Phase:       string(phase),
            FWUID:       p.cfg.FWUID,
            Agent:       phaseCfg.Agent,
            ReviewCycle: p.sm.ReviewCycles(),
            PromptHash:  func() string { if p.currentLoop != nil { return p.currentLoop.PromptHash() }; return "" }(),
        })

		// Advance state machine.
		if routed {
			// Already handled by Route() during runPhase.
		} else if len(phaseCfg.Routes) > 0 {
			// Routing phase completed without explicit route signal.
			if !phaseCompleted {
				return fmt.Errorf("phase %s ended without route or phase-complete signal", phase)
			}
			// phase-complete without route is abnormal, but not fatal.
			// Advance linearly (skip routing).
			if _, err := p.sm.advanceLinear(); err != nil {
				return fmt.Errorf("advance from %s: %w", phase, err)
			}
		} else {
			// Linear advancement.
			if isStudy && phase == PhaseTD {
				// Fast-track: Study tasks are done after Technical Design.
				// No need for IM (Implement), RV (Review), RF (Refactor), or FL (Feedback Loop).
				if err := p.sm.JumpTo(PhaseDone); err != nil {
					return fmt.Errorf("complete study task: %w", err)
				}
			} else {
				if _, err := p.sm.Advance(); err != nil {
					return fmt.Errorf("advance from %s: %w", phase, err)
				}
			}
		}
	}

	p.emit(loop.Event{
		Type:  loop.EventPipelineComplete,
		FWUID: p.cfg.FWUID,
	})

	return nil
}

// Pause signals the pipeline to exit after the current loop iteration.
func (p *Pipeline) Pause() {
	p.paused.Store(true)
	p.mu.Lock()
	if p.currentLoop != nil {
		p.currentLoop.Pause()
	}
	p.mu.Unlock()
}

// Stop signals the pipeline to kill the current process and exit immediately.
func (p *Pipeline) Stop() {
	p.stopped.Store(true)
	p.mu.Lock()
	if p.currentLoop != nil {
		p.currentLoop.Stop()
	}
	p.mu.Unlock()
}

// runPhase runs the Loop for one phase, consuming and forwarding events.
// Returns (phaseCompleted, routed, blocked, error).
func (p *Pipeline) runPhase(ctx context.Context, l *loop.Loop, loopCh <-chan loop.Event, phase Phase, phaseCfg PhaseConfig) (bool, bool, bool, error) {
	var phaseCompleted, routed, blocked bool

	// Consume events in the main goroutine, run Loop in a goroutine.
	loopDone := make(chan error, 1)
	go func() {
		loopDone <- l.Run(ctx)
	}()

	for event := range loopCh {
		// Enrich with pipeline context.
		event.Phase = string(phase)
		event.FWUID = p.cfg.FWUID
		event.Agent = phaseCfg.Agent
		event.ReviewCycle = p.sm.ReviewCycles()

		// Handle signals.
		sr := HandleSignal(event)
		switch sr.Action {
		case ActionPhaseComplete:
			phaseCompleted = true

		case ActionRoute:
			routed = true
			// Apply route to state machine.
			next, err := p.sm.Route(sr.RouteTarget)
			if err != nil {
				// Route error (e.g., max review cycles exceeded).
				p.emit(loop.Event{
					Type:        loop.EventOperatorAttention,
					Phase:       string(phase),
					FWUID:       p.cfg.FWUID,
					Agent:       phaseCfg.Agent,
					ReviewCycle: p.sm.ReviewCycles(),
					Text:        fmt.Sprintf("route error: %v", err),
				})
				blocked = true
			} else {
				// Emit route decision.
				p.emit(loop.Event{
					Type:        loop.EventRouteDecision,
					Phase:       string(phase),
					FWUID:       p.cfg.FWUID,
					Agent:       phaseCfg.Agent,
					RouteTarget: sr.RouteTarget,
					ReviewCycle: p.sm.ReviewCycles(),
					Text:        fmt.Sprintf("routed to %s (next phase: %s)", sr.RouteTarget, next),
				})
			}

		case ActionPause:
			blocked = true
			l.Pause()
			p.emit(loop.Event{
				Type:        loop.EventOperatorAttention,
				Phase:       string(phase),
				FWUID:       p.cfg.FWUID,
				Agent:       phaseCfg.Agent,
				ReviewCycle: p.sm.ReviewCycles(),
				Text:        sr.Reason,
			})

		case ActionReplan:
			blocked = true
			l.Pause()
			p.emit(loop.Event{
				Type:        loop.EventPlanningMismatch,
				Phase:       string(phase),
				FWUID:       p.cfg.FWUID,
				Agent:       phaseCfg.Agent,
				ReviewCycle: p.sm.ReviewCycles(),
				Text:        sr.Reason,
			})
		}

		// Forward event to pipeline channel.
		p.emit(event)
	}

	// Wait for Loop.Run() to return.
	err := <-loopDone
	return phaseCompleted, routed, blocked, err
}

func (p *Pipeline) emit(e loop.Event) {
	p.events <- e
}

func (p *Pipeline) buildMCPConfig() (string, func(), error) {
	if p.cfg.CommandName != "" {
		// Test mode — no real MCP config needed.
		return "", func() {}, nil
	}

	axonBin, _ := os.Executable()
	servers := loop.BuildMCPServers(axonBin, p.cfg.TractStore)
	path, err := loop.WriteMCPConfig(servers)
	if err != nil {
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}

// GetHealth returns health information about the current phase loop.
func (p *Pipeline) GetHealth() (loop.LoopHealth, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.currentLoop == nil {
		return loop.LoopHealth{}, false
	}
	return p.currentLoop.GetHealth(), true
}
