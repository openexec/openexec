package pipeline

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openexec/openexec/internal/actions"
	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/cache"
	"github.com/openexec/openexec/internal/checkpoint"
	"github.com/openexec/openexec/internal/config"
	ocontext "github.com/openexec/openexec/internal/context"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/memory"
	"github.com/openexec/openexec/internal/parallel"
	"github.com/openexec/openexec/internal/predictive"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/quality"
	"github.com/openexec/openexec/internal/router"
	"github.com/openexec/openexec/internal/skills"
	"github.com/openexec/openexec/internal/types"
	pagent "github.com/openexec/openexec/pkg/agent"
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

	// ReviewEnabled enables the code review stage after testing.
	ReviewEnabled bool

	// ReviewerModel optionally overrides the model used for the review stage.
	ReviewerModel string

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

	// API provider settings for OpenAI-compatible execution
	APIProvider string // "openai_compat"
	APIBaseURL  string // e.g. "https://api.moonshot.cn/v1"
	APIKey      string // API key or "$ENV_VAR" reference
	APIModel    string // e.g. "moonshot-v1-128k"
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

	gateRunner     types.GateRunner
	qualityManager *quality.Manager
	checkpointMgr  *checkpoint.Manager

	intentRouter router.Router

	agentRegistry  *agent.AgentRegistry
	parallelConfig *parallel.ParallelConfig

	skillRegistry *skills.Registry

	memoryManager    *memory.MemoryManager
	knowledgeCache   *cache.KnowledgeCache
	toolResultCache  *cache.ToolResultCache
	predictiveLoader *predictive.Loader
	contextPruner    *ocontext.Pruner

	coordinator *agent.TaskCoordinator

	currentLoop *loop.Loop
	mu          sync.Mutex

	paused  atomic.Bool
	stopped atomic.Bool
}

// New creates a Pipeline and returns it along with a read-only event channel.
// The channel is closed when Run returns.
func New(cfg Config, opts ...Option) (*Pipeline, <-chan loop.Event) {
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
	return NewWithFactory(cfg, factory, opts...)
}

// Option defines a functional option for Pipeline.
type Option func(*Pipeline)

// WithGateRunner sets the quality gate runner for the pipeline.
func WithGateRunner(runner types.GateRunner) Option {
	return func(p *Pipeline) {
		p.gateRunner = runner
	}
}

// WithQualityManager sets the V2 quality gate manager for the pipeline.
func WithQualityManager(qm *quality.Manager) Option {
	return func(p *Pipeline) {
		p.qualityManager = qm
	}
}

// WithCheckpointManager sets the checkpoint manager for crash recovery.
func WithCheckpointManager(m *checkpoint.Manager) Option {
	return func(p *Pipeline) {
		p.checkpointMgr = m
	}
}

// WithKnowledgeCache sets the knowledge cache for the pipeline.
func WithKnowledgeCache(c *cache.KnowledgeCache) Option {
	return func(p *Pipeline) { p.knowledgeCache = c }
}

// WithToolResultCache sets the tool result cache for the pipeline.
func WithToolResultCache(c *cache.ToolResultCache) Option {
	return func(p *Pipeline) { p.toolResultCache = c }
}

// WithPredictiveLoader sets the predictive file loader for the pipeline.
func WithPredictiveLoader(l *predictive.Loader) Option {
	return func(p *Pipeline) { p.predictiveLoader = l }
}

// WithMemoryManager sets the memory manager for learning persistence across sessions.
func WithMemoryManager(m *memory.MemoryManager) Option {
	return func(p *Pipeline) { p.memoryManager = m }
}

// WithContextPruner sets the context pruner for reducing token usage by selecting
// only the most relevant context items for a given task.
func WithContextPruner(cp *ocontext.Pruner) Option {
	return func(p *Pipeline) { p.contextPruner = cp }
}

// WithSkillRegistry sets the skill registry for injecting relevant skills into pipeline context.
func WithSkillRegistry(r *skills.Registry) Option {
	return func(p *Pipeline) { p.skillRegistry = r }
}

// WithRouter sets the intent router for deterministic task routing.
func WithRouter(r router.Router) Option {
	return func(p *Pipeline) { p.intentRouter = r }
}

// WithParallelExecution enables multi-agent parallel execution for agentic stages.
// When worker_count > 1 in config, agentic stages are split across parallel agents.
func WithParallelExecution(registry *agent.AgentRegistry, config *parallel.ParallelConfig) Option {
	return func(p *Pipeline) {
		p.agentRegistry = registry
		p.parallelConfig = config
	}
}

// WithCoordinator sets the coordinator for multi-agent task decomposition.
// When set and API mode is active, the coordinator decomposes tasks into subtasks,
// runs worker agents in parallel, and merges results.
func WithCoordinator(c *agent.TaskCoordinator) Option {
	return func(p *Pipeline) {
		p.coordinator = c
	}
}

// NewWithFactory creates a Pipeline using a pre-configured factory.
func NewWithFactory(cfg Config, factory *LoopFactory, opts ...Option) (*Pipeline, <-chan loop.Event) {
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

	for _, opt := range opts {
		opt(p)
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

// Close releases resources held by the pipeline.
func (p *Pipeline) Close() {
	if p.checkpointMgr != nil {
		p.checkpointMgr.Close()
	}
	if p.memoryManager != nil {
		p.memoryManager.Close()
	}
	if p.predictiveLoader != nil {
		p.predictiveLoader.Close()
	}
	if p.knowledgeCache != nil {
		p.knowledgeCache.Close()
	}
	if p.toolResultCache != nil {
		p.toolResultCache.Close()
	}
	if p.contextPruner != nil {
		p.contextPruner.Close()
	}
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
	// Deterministic routing: classify task and set context parameters
	if p.intentRouter != nil && p.cfg.TaskDescription != "" {
		plan, err := router.Route(ctx, p.intentRouter, p.cfg.TaskDescription, nil)
		if err == nil {
			if len(plan.RepoZones) > 0 && len(p.cfg.RepoZones) == 0 {
				p.cfg.RepoZones = plan.RepoZones
			}
			if len(plan.KnowledgeSources) > 0 && len(p.cfg.KnowledgeSources) == 0 {
				p.cfg.KnowledgeSources = plan.KnowledgeSources
			}
			if plan.Sensitivity != "" && p.cfg.Sensitivity == "" {
				p.cfg.Sensitivity = string(plan.Sensitivity)
			}
		}
	}

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

	// Predictive file pre-loading
	if p.predictiveLoader != nil && p.cfg.TaskDescription != "" {
		if predicted, err := p.predictiveLoader.PredictAndLoad(ctx, p.cfg.TaskDescription, nil); err == nil && predicted != nil {
			for _, pf := range predicted.LoadedFiles {
				if pf.Content != "" {
					if contextPack == nil {
						contextPack = &ocontext.ContextPack{}
					}
					contextPack.Items = append(contextPack.Items, &ocontext.ContextItem{
						Type:    ocontext.ContextTypeRecentFiles,
						Source:  fmt.Sprintf("predicted:%s", pf.Path),
						Content: pf.Content,
					})
				}
			}
		}
	}

	// Prune context to reduce token usage
	if p.contextPruner != nil && contextPack != nil && len(contextPack.Items) > 0 {
		files := make([]ocontext.FileInfo, 0, len(contextPack.Items))
		for _, item := range contextPack.Items {
			files = append(files, ocontext.FileInfo{
				Path:    item.Source,
				Content: item.Content,
			})
		}
		if pruned, err := p.contextPruner.Prune(files, p.cfg.TaskDescription); err == nil && len(pruned.Files) > 0 {
			prunedItems := make([]*ocontext.ContextItem, 0, len(pruned.Files))
			for _, pf := range pruned.Files {
				prunedItems = append(prunedItems, &ocontext.ContextItem{
					Type:    ocontext.ContextTypeRecentFiles,
					Source:  pf.Path,
					Content: pf.Content,
				})
			}
			contextPack.Items = prunedItems
			contextPack.TotalTokens = pruned.TotalTokens
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
		// Try to load from external file in .openexec/blueprints/
		absWorkDir, _ := filepath.Abs(p.cfg.WorkDir)
		bpPath := filepath.Join(absWorkDir, ".openexec", "blueprints", p.cfg.BlueprintID+".yaml")
		log.Printf("[Pipeline] Loading external blueprint from: %s", bpPath)
		if _, err := os.Stat(bpPath); err == nil {
			reg := blueprint.NewRegistry()
			if externalBP, err := reg.LoadFromFile(bpPath); err == nil {
				bp = externalBP
			} else {
				return fmt.Errorf("failed to load blueprint from %s: %w", bpPath, err)
			}
		} else {
			return fmt.Errorf("unknown blueprint: %s", p.cfg.BlueprintID)
		}
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
	executor.ActionRegistry = actions.DefaultRegistry(p.cfg.WorkDir)

	// If a custom gate runner is provided (e.g. by tests), register it as the 'run_gates' action
	if p.gateRunner != nil {
		executor.ActionRegistry.Overwrite(&gateRunnerAction{runner: p.gateRunner})
	}

	// Set up agentic runner that wraps a bounded loop
	executor.AgenticRunner = &blueprint.LoopAgenticRunner{
		MaxIterations: p.cfg.DefaultMaxIterations,
		LoopFactory: func(stageName string, prompt string, workDir string, maxIterations int) (blueprint.AgenticLoop, error) {
			// Model tiering: Use ReviewerModel for review stage if configured
			model := p.cfg.ExecutorModel
			if stageName == "review" && p.cfg.ReviewerModel != "" {
				model = p.cfg.ReviewerModel
			}
			return p.createAgenticLoop(ctx, model, prompt, workDir, maxIterations)
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
		if result.Status == types.StageStatusFailed {
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

		// Run quality gates after successful agentic stages
		if p.qualityManager != nil && result.Status == types.StageStatusCompleted {
			go func() {
				summary, err := p.qualityManager.RunAll(context.Background())
				if err != nil {
					p.emit(loop.Event{Type: loop.EventError, ErrText: fmt.Sprintf("quality gates error: %v", err)})
					return
				}
				if summary.FailedGates > 0 {
					p.emit(loop.Event{
						Type:  loop.EventGatesFailed,
						Text:  fmt.Sprintf("Quality gates: %d passed, %d failed, blocked=%v", summary.PassedGates, summary.FailedGates, summary.Blocked),
						FWUID: p.cfg.FWUID,
					})
				} else {
					p.emit(loop.Event{
						Type:  loop.EventGatesPassed,
						Text:  fmt.Sprintf("Quality gates: %d passed", summary.PassedGates),
						FWUID: p.cfg.FWUID,
					})
				}
			}()
		}

		// Create checkpoint after successful stages for crash recovery
		if p.checkpointMgr != nil && result.Status == types.StageStatusCompleted {
			go func() {
				if _, err := p.checkpointMgr.Create(run, p.cfg.WorkDir); err != nil {
					p.emit(loop.Event{Type: loop.EventError, ErrText: fmt.Sprintf("checkpoint error: %v", err)})
				}
			}()
		}

		// Extract learning patterns from completed stages for memory persistence
		if p.memoryManager != nil && result.Status == types.StageStatusCompleted {
			go p.extractMemory(run, result)
		}
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

	// Inject prior learning context from memory system
	if p.memoryManager != nil {
		memCtx := p.loadMemoryContext(p.cfg.TaskDescription)
		if memCtx != "" {
			input.Briefing += memCtx
		}
	}

	// Inject relevant skills into briefing
	if p.skillRegistry != nil && p.cfg.TaskDescription != "" {
		selected := p.skillRegistry.SelectForTask(p.cfg.TaskDescription)
		for _, skill := range selected {
			input.Briefing += fmt.Sprintf("\n\n--- Skill: %s ---\n%s", skill.Name, skill.Content)
		}
	}

	// Handle conditional review stage
	if !p.cfg.ReviewEnabled {
		if test, ok := bp.Stages["test"]; ok {
			test.OnSuccess = "complete"
		}
	} else if p.cfg.ReviewerModel != "" {
		// Use specialized model for review if configured
		if _, ok := bp.Stages["review"]; ok {
			// Model tiering is handled in the LoopAgenticRunner setup
		}
	}

	// Execute blueprint: use coordinator, parallel engine, or standard
	var execErr error
	if p.coordinator != nil && p.cfg.APIProvider != "" {
		// Coordinator-based multi-agent execution (Phase C)
		// Gather workspace files for task decomposition
		var files []string
		_ = filepath.Walk(p.cfg.WorkDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(p.cfg.WorkDir, path)
			if rel != "" && rel[0] != '.' {
				files = append(files, rel)
			}
			return nil
		})

		// Use coordinator for the implement stage, standard engine for the rest
		parallelEngine, pErr := parallel.NewParallelEngine(bp, executor, p.agentRegistry, p.parallelConfig)
		if pErr != nil {
			log.Printf("[Pipeline] Coordinator: parallel engine init failed, using coordinator directly")
		}

		// Run blueprint normally but intercept the implement stage with coordinator
		if parallelEngine != nil {
			// Wire coordinator into parallel engine for the implement stage
			impl, hasImpl := bp.Stages["implement"]
			if hasImpl && len(files) > 0 {
				coordResult, err := parallelEngine.ExecuteWithCoordinator(ctx, run, impl, input, files, p.coordinator)
				if err != nil {
					log.Printf("[Pipeline] Coordinator execution failed, falling back to standard: %v", err)
					execErr = engine.Execute(ctx, run, input)
				} else {
					run.AddResult(coordResult)
					input.AddPreviousResult(coordResult)
					// Skip to post-implement stages by adjusting the run
					run.CurrentStage = impl.OnSuccess
					execErr = engine.Execute(ctx, run, input)
				}
			} else {
				execErr = engine.Execute(ctx, run, input)
			}
		} else {
			execErr = engine.Execute(ctx, run, input)
		}
	} else if p.agentRegistry != nil && p.parallelConfig != nil && p.parallelConfig.EnableParallelism {
		parallelEngine, err := parallel.NewParallelEngine(bp, executor, p.agentRegistry, p.parallelConfig)
		if err != nil {
			log.Printf("[Pipeline] Parallel engine init failed, falling back to sequential: %v", err)
			execErr = engine.Execute(ctx, run, input)
		} else {
			// Gather workspace files for batch splitting
			var files []string
			_ = filepath.Walk(p.cfg.WorkDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(p.cfg.WorkDir, path)
				if rel != "" && rel[0] != '.' {
					files = append(files, rel)
				}
				return nil
			})
			execErr = parallelEngine.ExecuteBlueprint(ctx, run, input, files)
		}
	} else {
		execErr = engine.Execute(ctx, run, input)
	}

	if execErr != nil {
		p.emit(loop.Event{
			Type:        loop.EventBlueprintFailed,
			FWUID:       p.cfg.FWUID,
			BlueprintID: bp.ID,
			ErrText:     execErr.Error(),
		})
		return execErr
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
// Routes to API-based execution when API provider is configured.
func (p *Pipeline) createAgenticLoop(ctx context.Context, model string, prompt string, workDir string, maxIterations int) (blueprint.AgenticLoop, error) {
	// API mode: use HTTP provider instead of CLI subprocess
	if p.cfg.APIProvider != "" {
		return p.createAPIAgenticLoop(ctx, prompt, workDir, maxIterations)
	}

	// CLI mode: existing subprocess path
	// Build MCP config
	mcpPath, cleanup, err := p.buildMCPConfig()
	if err != nil {
		return nil, fmt.Errorf("build MCP config: %w", err)
	}

	cfg := loop.Config{
		ExecutorModel: model,
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

// createAPIAgenticLoop creates an API-based agentic loop using an OpenAI-compatible provider.
func (p *Pipeline) createAPIAgenticLoop(ctx context.Context, prompt string, workDir string, maxTurns int) (blueprint.AgenticLoop, error) {
	// Resolve API key (support $ENV_VAR references)
	apiKey := p.cfg.APIKey
	if strings.HasPrefix(apiKey, "$") {
		apiKey = os.Getenv(strings.TrimPrefix(apiKey, "$"))
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required: set api_key in config or use $ENV_VAR reference")
	}

	// Create OpenAI provider with custom base URL
	providerCfg := pagent.OpenAIProviderConfig{
		APIKey:  apiKey,
		BaseURL: p.cfg.APIBaseURL,
	}
	provider, err := pagent.NewOpenAIProvider(providerCfg)
	if err != nil {
		return nil, fmt.Errorf("create API provider: %w", err)
	}

	// Build tool definitions for the API
	tools := loop.BuildAPIToolDefinitions()

	// Create event channel and runner
	ch := make(chan loop.Event, 100)
	runner := loop.NewAPIRunner(loop.APIRunnerConfig{
		Provider: provider,
		Model:    p.cfg.APIModel,
		Prompt:   prompt,
		WorkDir:  workDir,
		MaxTurns: maxTurns,
		Tools:    tools,
	}, ch)

	return &apiLoopAdapter{runner: runner, events: ch, emit: p.emit}, nil
}

// apiLoopAdapter wraps an APIRunner to implement blueprint.AgenticLoop.
type apiLoopAdapter struct {
	runner     *loop.APIRunner
	events     chan loop.Event
	emit       func(loop.Event)
	lastOutput string
	artifacts  map[string]string
}

// Run executes the API runner and captures results.
func (a *apiLoopAdapter) Run(ctx context.Context) error {
	a.artifacts = make(map[string]string)

	// Run in background
	runDone := make(chan error, 1)
	go func() { runDone <- a.runner.Run(ctx) }()

	// Consume events and capture output/artifacts
	for event := range a.events {
		a.emit(event)

		if event.Text != "" {
			a.lastOutput = event.Text
		}
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

	return <-runDone
}

// GetResult returns the captured output and artifacts.
func (a *apiLoopAdapter) GetResult() (string, map[string]string, error) {
	return a.lastOutput, a.artifacts, nil
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

// gateRunnerAction adapts a types.GateRunner to the actions.Action interface.
type gateRunnerAction struct {
	runner types.GateRunner
}

func (a *gateRunnerAction) Name() string {
	return "run_gates"
}

func (a *gateRunnerAction) Execute(ctx context.Context, req actions.ActionRequest) (actions.ActionResponse, error) {
	err := a.runner.RunAll(ctx)
	if err != nil {
		return actions.ActionResponse{
			Status: types.StageStatusFailed,
			Output: "Quality gates failed",
			Error:  err.Error(),
		}, nil
	}

	return actions.ActionResponse{
		Status: types.StageStatusCompleted,
		Output: "All quality gates passed",
	}, nil
}
