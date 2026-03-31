// Package harness provides the integrated OpenExec execution harness.
// It combines caching, memory, multi-agent coordination, and blueprint execution.
package harness

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/cache"
	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/memory"
	"github.com/openexec/openexec/internal/parallel"
	"github.com/openexec/openexec/internal/types"
)

// Harness is the integrated OpenExec execution harness.
// It provides caching, memory, and multi-agent coordination.
type Harness struct {
	// Configuration
	config *HarnessConfig

	// Caching
	knowledgeCache   *cache.KnowledgeCache
	toolResultCache  *cache.ToolResultCache

	// Memory
	memorySystem *memory.MemorySystem
	memoryManager *memory.MemoryManager

	// Multi-Agent
	agentRegistry   *agent.AgentRegistry
	parallelEngine  *parallel.ParallelEngine

	// Blueprint execution
	blueprintEngine *blueprint.Engine
	loop            *loop.Loop

	// Project context
	projectDir string
}

// HarnessConfig contains configuration for the harness.
type HarnessConfig struct {
	// Project directory
	ProjectDir string

	// Cache configuration
	CacheEnabled    bool
	CacheTTL        time.Duration
	KnowledgeCache  *cache.KnowledgeCache
	ToolResultCache *cache.ToolResultCache

	// Memory configuration
	MemoryEnabled bool
	MemorySystem  *memory.MemorySystem
	MemoryManager *memory.MemoryManager

	// Multi-agent configuration
	MultiAgentEnabled bool
	MaxAgents         int
	AgentRegistry     *agent.AgentRegistry

	// Blueprint configuration
	Blueprint        *blueprint.Blueprint
	BlueprintEnabled bool

	// Execution callbacks
	OnStageStart    func(run *blueprint.Run, stage *blueprint.Stage)
	OnStageComplete func(run *blueprint.Run, result *blueprint.StageResult)
	OnCheckpoint    func(run *blueprint.Run, stageName string)
	OnRunComplete   func(run *blueprint.Run)
}

// DefaultHarnessConfig returns default harness configuration.
func DefaultHarnessConfig(projectDir string) *HarnessConfig {
	return &HarnessConfig{
		ProjectDir:        projectDir,
		CacheEnabled:      true,
		CacheTTL:          1 * time.Hour,
		MemoryEnabled:     true,
		MultiAgentEnabled: true,
		MaxAgents:         4,
		Blueprint:         blueprint.DefaultBlueprint,
		BlueprintEnabled:  true,
	}
}

// NewHarness creates a new integrated harness.
func NewHarness(config *HarnessConfig) (*Harness, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	h := &Harness{
		config:     config,
		projectDir: config.ProjectDir,
	}

	// Initialize caches
	if err := h.initCaches(); err != nil {
		return nil, fmt.Errorf("failed to initialize caches: %w", err)
	}

	// Initialize memory
	if err := h.initMemory(); err != nil {
		return nil, fmt.Errorf("failed to initialize memory: %w", err)
	}

	// Initialize multi-agent
	if err := h.initMultiAgent(); err != nil {
		return nil, fmt.Errorf("failed to initialize multi-agent: %w", err)
	}

	// Initialize blueprint engine
	if err := h.initBlueprintEngine(); err != nil {
		return nil, fmt.Errorf("failed to initialize blueprint engine: %w", err)
	}

	return h, nil
}

// initCaches initializes the caching subsystem.
func (h *Harness) initCaches() error {
	if !h.config.CacheEnabled {
		return nil
	}

	// Use provided caches or create new ones
	if h.config.KnowledgeCache != nil {
		h.knowledgeCache = h.config.KnowledgeCache
	} else {
		cache, err := cache.NewKnowledgeCache(h.projectDir, h.config.CacheTTL)
		if err != nil {
			return fmt.Errorf("failed to create knowledge cache: %w", err)
		}
		h.knowledgeCache = cache
	}

	if h.config.ToolResultCache != nil {
		h.toolResultCache = h.config.ToolResultCache
	} else {
		cache, err := cache.NewToolResultCache(h.projectDir, h.config.CacheTTL)
		if err != nil {
			return fmt.Errorf("failed to create tool result cache: %w", err)
		}
		h.toolResultCache = cache
	}

	return nil
}

// initMemory initializes the memory subsystem.
func (h *Harness) initMemory() error {
	if !h.config.MemoryEnabled {
		return nil
	}

	// Use provided memory or create new
	if h.config.MemorySystem != nil {
		h.memorySystem = h.config.MemorySystem
	} else {
		h.memorySystem = memory.NewMemorySystem(h.projectDir)
	}

	if h.config.MemoryManager != nil {
		h.memoryManager = h.config.MemoryManager
	} else {
		manager, err := memory.NewMemoryManager(h.projectDir)
		if err != nil {
			return fmt.Errorf("failed to create memory manager: %w", err)
		}
		h.memoryManager = manager
	}

	return nil
}

// initMultiAgent initializes the multi-agent subsystem.
func (h *Harness) initMultiAgent() error {
	if !h.config.MultiAgentEnabled {
		return nil
	}

	// Use provided registry or create new
	if h.config.AgentRegistry != nil {
		h.agentRegistry = h.config.AgentRegistry
	} else {
		registry, err := agent.NewAgentRegistry(h.projectDir)
		if err != nil {
			return fmt.Errorf("failed to create agent registry: %w", err)
		}
		h.agentRegistry = registry
	}

	return nil
}

// initBlueprintEngine initializes the blueprint execution engine.
func (h *Harness) initBlueprintEngine() error {
	if !h.config.BlueprintEnabled {
		return nil
	}

	bp := h.config.Blueprint
	if bp == nil {
		bp = blueprint.DefaultBlueprint
	}

	// Create stage executor that uses our caches
	executor := &HarnessStageExecutor{
		harness: h,
	}

	// Create parallel engine if multi-agent is enabled
	if h.config.MultiAgentEnabled && h.agentRegistry != nil {
		parallelConfig := parallel.DefaultParallelConfig()
		parallelConfig.MaxAgents = h.config.MaxAgents

		parallelEngine, err := parallel.NewParallelEngine(bp, executor, h.agentRegistry, parallelConfig)
		if err != nil {
			return fmt.Errorf("failed to create parallel engine: %w", err)
		}
		h.parallelEngine = parallelEngine
	}

	// Create base blueprint engine
	engineConfig := blueprint.DefaultEngineConfig()
	engineConfig.OnStageStart = h.wrapStageStartCallback(h.config.OnStageStart)
	engineConfig.OnStageComplete = h.wrapStageCompleteCallback(h.config.OnStageComplete)
	engineConfig.OnCheckpoint = h.config.OnCheckpoint
	engineConfig.OnRunComplete = h.config.OnRunComplete

	engine, err := blueprint.NewEngine(bp, executor, engineConfig)
	if err != nil {
		return fmt.Errorf("failed to create blueprint engine: %w", err)
	}

	h.blueprintEngine = engine

	return nil
}

// wrapStageStartCallback wraps the user's callback to add harness functionality.
func (h *Harness) wrapStageStartCallback(userCallback func(*blueprint.Run, *blueprint.Stage)) func(*blueprint.Run, string) {
	return func(run *blueprint.Run, stageName string) {
		// Load memory context before stage execution
		if h.memoryManager != nil {
			context, err := h.memoryManager.LoadContext()
			if err == nil && context != "" {
				// Memory context is available for the stage
				_ = context // Could be passed to the stage input
			}
		}

		if userCallback != nil {
			stage, _ := h.blueprintEngine.GetBlueprint().GetStage(stageName)
			userCallback(run, stage)
		}
	}
}

// wrapStageCompleteCallback wraps the user's callback to add harness functionality.
func (h *Harness) wrapStageCompleteCallback(userCallback func(*blueprint.Run, *blueprint.StageResult)) func(*blueprint.Run, *blueprint.StageResult) {
	return func(run *blueprint.Run, result *blueprint.StageResult) {
		// Extract memories from completed stage
		if h.memoryManager != nil && result.Status == types.StageStatusCompleted {
			h.extractAndStoreMemories(run, result)
		}

		if userCallback != nil {
			userCallback(run, result)
		}
	}
}

// extractAndStoreMemories extracts memories from a completed stage.
func (h *Harness) extractAndStoreMemories(run *blueprint.Run, result *blueprint.StageResult) {
	// Create a session from the stage result
	session := memory.Session{
		ID:        fmt.Sprintf("%s-%s", run.ID, result.StageName),
		StartTime: result.StartedAt,
		EndTime:   result.CompletedAt,
	}

	// Extract patterns from output
	if patterns := h.extractPatterns(result.Output); len(patterns) > 0 {
		session.Patterns = patterns
	}

	// Auto-extract and store
	entries, err := h.memorySystem.AutoExtract(session)
	if err == nil && len(entries) > 0 {
		_ = h.memoryManager.StoreEntries(entries)
	}
}

// extractPatterns extracts patterns from stage output.
func (h *Harness) extractPatterns(output string) []memory.Pattern {
	// Simple pattern extraction - could be enhanced with NLP
	var patterns []memory.Pattern

	// Look for common pattern indicators
	if contains(output, "pattern", "convention", "standard") {
		patterns = append(patterns, memory.Pattern{
			Name:        "Extracted Pattern",
			Description: output[:min(len(output), 200)],
		})
	}

	return patterns
}

// contains checks if any keywords exist in the text.
func contains(text string, keywords ...string) bool {
	lower := strings.ToLower(text)
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Execute runs a task using the harness.
func (h *Harness) Execute(ctx context.Context, task string, files []string) (*blueprint.Run, error) {
	if h.blueprintEngine == nil {
		return nil, fmt.Errorf("blueprint engine not initialized")
	}

	// Start run
	input := blueprint.NewStageInput("harness-run", task, h.projectDir)
	
	// Add memory context
	if h.memoryManager != nil {
		context, err := h.memoryManager.LoadContext()
		if err == nil {
			input.Briefing = context
		}
	}

	run, err := h.blueprintEngine.StartRun(ctx, "harness-run", input)
	if err != nil {
		return nil, fmt.Errorf("failed to start run: %w", err)
	}

	// Execute with parallel support if available
	if h.parallelEngine != nil && len(files) > 0 {
		err = h.parallelEngine.ExecuteBlueprint(ctx, run, input, files)
	} else {
		err = h.blueprintEngine.Execute(ctx, run, input)
	}

	if err != nil {
		return run, err
	}

	return run, nil
}

// GetKnowledgeCache returns the knowledge cache.
func (h *Harness) GetKnowledgeCache() *cache.KnowledgeCache {
	return h.knowledgeCache
}

// GetToolResultCache returns the tool result cache.
func (h *Harness) GetToolResultCache() *cache.ToolResultCache {
	return h.toolResultCache
}

// GetMemoryManager returns the memory manager.
func (h *Harness) GetMemoryManager() *memory.MemoryManager {
	return h.memoryManager
}

// GetAgentRegistry returns the agent registry.
func (h *Harness) GetAgentRegistry() *agent.AgentRegistry {
	return h.agentRegistry
}

// GetBlueprintEngine returns the blueprint engine.
func (h *Harness) GetBlueprintEngine() *blueprint.Engine {
	return h.blueprintEngine
}

// Close cleans up the harness resources.
func (h *Harness) Close() error {
	var errs []error

	if h.knowledgeCache != nil {
		if err := h.knowledgeCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if h.toolResultCache != nil {
		if err := h.toolResultCache.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if h.memoryManager != nil {
		if err := h.memoryManager.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if h.agentRegistry != nil {
		if err := h.agentRegistry.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing harness: %v", errs)
	}

	return nil
}

// HarnessStageExecutor wraps stage execution with caching.
type HarnessStageExecutor struct {
	harness *Harness
}

// Execute executes a stage with caching support.
func (e *HarnessStageExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	// Check tool result cache for deterministic stages
	if e.harness.toolResultCache != nil && stage.Type == types.StageTypeDeterministic {
		// Create cache key from stage and input
		cacheKey := map[string]interface{}{
			"stage": stage.Name,
			"type":  string(stage.Type),
		}

		if cached, err := e.harness.toolResultCache.Get(stage.Name, cacheKey); err == nil && cached != nil {
			// Return cached result
			result := blueprint.NewStageResult(stage.Name, 1)
			result.Complete(string(cached))
			return result, nil
		}
	}

	// Execute the stage (this would call the actual executor)
	// For now, return a placeholder
	result := blueprint.NewStageResult(stage.Name, 1)
	result.Complete("executed")

	// Cache the result if deterministic
	if e.harness.toolResultCache != nil && stage.Type == types.StageTypeDeterministic {
		cacheKey := map[string]interface{}{
			"stage": stage.Name,
			"type":  string(stage.Type),
		}
		_ = e.harness.toolResultCache.Set(stage.Name, cacheKey, []byte(result.Output), 0)
	}

	return result, nil
}


