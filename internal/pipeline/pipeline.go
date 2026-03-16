package pipeline

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/config"
	ocontext "github.com/openexec/openexec/internal/context"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Config controls pipeline behavior.
type Config struct {
	FWUID                string
	WorkDir              string
	AgentsFS             fs.FS
	ExecutorModel        string   // model for runner resolution
	RunnerCommand        string   // CLI override
	RunnerArgs           []string // CLI args override
	DefaultMaxIterations int      // default 10
	MaxRetries           int      // default 3
	RetryBackoff         []time.Duration
	ThrashThreshold      int      // default 3
	CommandName          string   // test override
	CommandArgs          []string // test override

	// IsStudy flags the task as documentation/analysis only.
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

	// ResumeFrom enables resuming from a checkpoint.
	ResumeFrom *ResumeConfig

	// BlueprintID enables blueprint mode with the specified blueprint.
	// When set, the pipeline uses blueprint-based execution instead of phases.
	BlueprintID string

	// TaskDescription is the user's task description for blueprint runs.
	TaskDescription string

	// ContextTokenBudget is the token budget for context assembly.
	// If > 0, context is gathered using two-stage assembly.
	ContextTokenBudget int

	// RepoZones filters context to specific directories.
	RepoZones []string

	// KnowledgeSources ranks context items by source relevance.
	KnowledgeSources []string

	// Sensitivity determines redaction level ("low", "medium", "high").
	Sensitivity string

	// TaskTimeout overrides the default implement stage timeout.
	// If zero, the blueprint default (10 minutes) is used.
	TaskTimeout time.Duration
}

// ResumeConfig holds configuration for resuming from a checkpoint.
type ResumeConfig struct {
	CheckpointID     string   // ID of the checkpoint to resume from
	Phase            string   // Phase to resume from
	Iteration        int      // Iteration to resume from
	MessageHistory   []byte   // JSON-encoded message history
	AppliedToolCalls []string // Idempotency keys of already-applied tool calls
}

// Pipeline drives an FWU through blueprint-based stage execution.
// It coordinates with the blueprint engine to execute stages like
// gather_context → implement → lint → test → review.
type Pipeline struct {
	cfg     Config
	factory *LoopFactory
	events  chan loop.Event

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
		AgentsFS:             cfg.AgentsFS,
		DefaultMaxIterations: cfg.DefaultMaxIterations,
		MaxRetries:           cfg.MaxRetries,
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
	// Apply defaults for blueprint-based execution.
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
		factory: factory,
		events:  ch,
	}

	return p, ch
}

// Run executes the pipeline using blueprint-based stage orchestration.
// It closes the event channel when it returns.
//
// Note: Legacy phase-based execution has been removed. All pipeline execution
// now uses blueprint mode. If BlueprintID is not set, "standard_task" is used.
func (p *Pipeline) Run(ctx context.Context) error {
	ctx, span := telemetry.StartSpan(ctx, "Pipeline.Run", trace.WithAttributes(
		attribute.String("fwu_id", p.cfg.FWUID),
		attribute.String("project_path", p.cfg.WorkDir),
	))
	defer span.End()

	defer close(p.events)

	// Always use blueprint mode - default to standard_task if not specified
	return p.runBlueprintMode(ctx)
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

func (p *Pipeline) emit(e loop.Event) {
	p.events <- e
}

func (p *Pipeline) buildMCPConfig() (string, func(), error) {
	if p.cfg.CommandName != "" {
		// Test mode — no real MCP config needed.
		return "", func() {}, nil
	}

	axonBin, _ := os.Executable()
	servers := loop.BuildMCPServers(axonBin, "")
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

// runBlueprintMode executes the pipeline using blueprint-based stage orchestration.
// This is the sole execution path - all pipeline runs use blueprint mode.
// If BlueprintID is empty, it defaults to "standard_task".
func (p *Pipeline) runBlueprintMode(ctx context.Context) error {
	// Build context using two-stage assembly
	var contextPack *ocontext.ContextPack
	if p.cfg.ContextTokenBudget > 0 {
		pack, err := ocontext.BuildContextWithRouting(
			ctx,
			p.cfg.WorkDir,
			p.cfg.ContextTokenBudget,
			p.cfg.RepoZones,
			p.cfg.KnowledgeSources,
			p.cfg.Sensitivity,
		)
		if err != nil {
			// Log warning but continue - context is optional
			p.emit(loop.Event{
				Type:    loop.EventError,
				FWUID:   p.cfg.FWUID,
				ErrText: fmt.Sprintf("context assembly warning: %v", err),
			})
		} else {
			contextPack = pack
		}
	}

	// Lookup blueprint by ID
	var bp *blueprint.Blueprint
	switch p.cfg.BlueprintID {
	case "standard_task", "":
		bp = blueprint.DefaultBlueprint
	case "quick_fix":
		bp = blueprint.QuickFixBlueprint
	default:
		return fmt.Errorf("unknown blueprint: %s", p.cfg.BlueprintID)
	}

	// Override implement stage timeout if configured
	if p.cfg.TaskTimeout > 0 {
		if impl, ok := bp.Stages["implement"]; ok {
			impl.Timeout = p.cfg.TaskTimeout
		}
	}

	// Override lint/test commands from project config
	if projCfg, err := project.LoadProjectConfig(p.cfg.WorkDir); err == nil {
		if lint, ok := bp.Stages["lint"]; ok && len(projCfg.Execution.LintCommands) > 0 {
			lint.Commands = projCfg.Execution.LintCommands
		}
		if test, ok := bp.Stages["test"]; ok && len(projCfg.Execution.TestCommands) > 0 {
			test.Commands = projCfg.Execution.TestCommands
		}
	}

	// Create executor with agentic runner
	executor := blueprint.NewDefaultExecutor(p.cfg.WorkDir)

	// Set up agentic runner that wraps a bounded loop
	executor.AgenticRunner = &blueprint.LoopAgenticRunner{
		MaxIterations: p.cfg.DefaultMaxIterations,
		LoopFactory: func(prompt string, workDir string, maxIterations int) (blueprint.AgenticLoop, error) {
			return p.createAgenticLoop(ctx, prompt, workDir, maxIterations)
		},
	}

	// Create engine with callbacks that emit events
	engineConfig := blueprint.DefaultEngineConfig()
	engineConfig.OnStageStart = func(run *blueprint.Run, stageName string) {
		p.emit(loop.Event{
			Type:      loop.EventStageStart,
			FWUID:     p.cfg.FWUID,
			StageName: stageName,
		})
	}
	engineConfig.OnStageComplete = func(run *blueprint.Run, result *blueprint.StageResult) {
		eventType := loop.EventStageComplete
		if result.Status == blueprint.StageStatusFailed {
			eventType = loop.EventStageFailed
		}
		p.emit(loop.Event{
			Type:      eventType,
			FWUID:     p.cfg.FWUID,
			StageName: result.StageName,
			StageType: string(result.Status),
			Attempt:   result.Attempt,
			Text:      result.Output,
			ErrText:   result.Error,
			Artifacts: result.Artifacts,
		})
	}
	engineConfig.OnCheckpoint = func(run *blueprint.Run, stageName string) {
		p.emit(loop.Event{
			Type:      loop.EventCheckpointCreated,
			FWUID:     p.cfg.FWUID,
			StageName: stageName,
			Artifacts: map[string]string{
				"checkpoint_stage": stageName,
				"run_id":           run.ID,
			},
		})
	}
	engineConfig.OnRunComplete = func(run *blueprint.Run) {
		// Collect all artifacts from stage results
		allArtifacts := make(map[string]string)
		for _, result := range run.Results {
			for k, v := range result.Artifacts {
				allArtifacts[k] = v
			}
		}
		p.emit(loop.Event{
			Type:  loop.EventBlueprintComplete,
			FWUID: p.cfg.FWUID,
			Result: &loop.StepResult{
				Status:     "complete",
				Reason:     "blueprint_complete",
				NextPhase:  "done",
				Artifacts:  allArtifacts,
				Confidence: 1.0,
			},
		})
	}

	engine, err := blueprint.NewEngine(bp, executor, engineConfig)
	if err != nil {
		return fmt.Errorf("create blueprint engine: %w", err)
	}

	// Emit blueprint start event
	p.emit(loop.Event{
		Type:        loop.EventBlueprintStart,
		FWUID:       p.cfg.FWUID,
		BlueprintID: bp.ID,
	})

	// Create run and input
	run, err := engine.StartRun(ctx, p.cfg.FWUID, nil)
	if err != nil {
		return fmt.Errorf("start blueprint run: %w", err)
	}

	input := blueprint.NewStageInput(p.cfg.FWUID, p.cfg.TaskDescription, p.cfg.WorkDir)

	// Inject rich context from ReleaseManager briefing if available
	if p.factory != nil && p.factory.cfg.ReleaseManager != nil {
		if brief, err := p.factory.cfg.ReleaseManager.Brief(p.cfg.FWUID); err == nil && brief != nil {
			// Format using prompt assembler style
			input.Briefing = p.factory.assembler.FormatBriefing(brief)
		}
	}

	// Inject gathered context into stage input
	if contextPack != nil {
		items := make([]blueprint.ContextPackItem, 0, len(contextPack.Items))
		for _, item := range contextPack.Items {
			items = append(items, blueprint.ContextPackItem{
				Type:    string(item.Type),
				Source:  item.Source,
				Content: item.Content,
			})
		}
		input.SetContextFromPack(items)
	}

	// Execute blueprint
	if err := engine.Execute(ctx, run, input); err != nil {
		p.emit(loop.Event{
			Type:        loop.EventBlueprintFailed,
			FWUID:       p.cfg.FWUID,
			BlueprintID: bp.ID,
			ErrText:     err.Error(),
		})
		return err
	}

	// Emit pipeline complete for compatibility
	p.emit(loop.Event{
		Type:  loop.EventPipelineComplete,
		FWUID: p.cfg.FWUID,
	})

	return nil
}

// createAgenticLoop creates a bounded loop for agentic stage execution.
// Returns an AgenticLoop that wraps the internal loop infrastructure.
func (p *Pipeline) createAgenticLoop(ctx context.Context, prompt string, workDir string, maxIterations int) (blueprint.AgenticLoop, error) {
	// Build MCP config
	mcpPath, cleanup, err := p.buildMCPConfig()
	if err != nil {
		return nil, fmt.Errorf("build MCP config: %w", err)
	}

	cfg := loop.Config{
		Prompt:        prompt,
		WorkDir:       workDir,
		MaxIterations: maxIterations,
		MaxRetries:    p.cfg.MaxRetries,
		RetryBackoff:  p.cfg.RetryBackoff,
		MCPConfigPath: mcpPath,
		FwuID:         p.cfg.FWUID,
		ExecMode:      p.cfg.ExecMode,
		RunnerCommand: p.cfg.RunnerCommand,
		RunnerArgs:    p.cfg.RunnerArgs,
		CommandName:   p.cfg.CommandName,
		CommandArgs:   p.cfg.CommandArgs,
	}

	l, ch := loop.New(cfg)

	return &agenticLoopAdapter{
		loop:    l,
		events:  ch,
		cleanup: cleanup,
		emit:    p.emit,
	}, nil
}

// agenticLoopAdapter wraps a loop.Loop to implement blueprint.AgenticLoop.
type agenticLoopAdapter struct {
	loop       *loop.Loop
	events     <-chan loop.Event
	cleanup    func()
	emit       func(loop.Event)
	lastOutput string
	artifacts  map[string]string
}

// Run executes the loop and captures results.
func (a *agenticLoopAdapter) Run(ctx context.Context) error {
	defer a.cleanup()

	a.artifacts = make(map[string]string)

	// Run loop in background
	loopDone := make(chan error, 1)
	go func() { loopDone <- a.loop.Run(ctx) }()

	// Consume events and capture output/artifacts
	for event := range a.events {
		// Forward event to pipeline
		a.emit(event)

		// Capture text output
		if event.Text != "" {
			a.lastOutput = event.Text
		}

		// Capture artifacts
		if event.Result != nil && event.Result.Artifacts != nil {
			for k, v := range event.Result.Artifacts {
				a.artifacts[k] = v
			}
		}
		if event.Artifacts != nil {
			for k, v := range event.Artifacts {
				a.artifacts[k] = v
			}
		}
	}

	return <-loopDone
}

// GetResult returns the captured output and artifacts.
func (a *agenticLoopAdapter) GetResult() (string, map[string]string, error) {
	return a.lastOutput, a.artifacts, nil
}
