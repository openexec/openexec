// Package parallel extends the blueprint engine with parallel stage execution.
// This enables multi-agent coordination for processing large codebases.
package parallel

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/openexec/openexec/internal/agent"
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/types"
)

// ParallelEngine extends the standard blueprint engine with parallel execution capabilities.
type ParallelEngine struct {
	baseEngine    *blueprint.Engine
	agentRegistry *agent.AgentRegistry
	executor      blueprint.StageExecutor
	config        *ParallelConfig
}

// ParallelConfig contains configuration for parallel execution.
type ParallelConfig struct {
	// MaxAgents is the maximum number of agents to spawn per stage.
	MaxAgents int

	// MinFilesForParallel is the minimum number of files to trigger parallelization.
	MinFilesForParallel int

	// DefaultMergeStrategy is the default strategy for merging agent results.
	DefaultMergeStrategy agent.MergeStrategy

	// EnableParallelism enables/disables parallel execution globally.
	EnableParallelism bool
}

// DefaultParallelConfig returns default parallel configuration.
func DefaultParallelConfig() *ParallelConfig {
	return &ParallelConfig{
		MaxAgents:           4,
		MinFilesForParallel: 5,
		DefaultMergeStrategy: agent.MergeReconcile,
		EnableParallelism:   true,
	}
}

// NewParallelEngine creates a new parallel blueprint engine.
func NewParallelEngine(
	bp *blueprint.Blueprint,
	executor blueprint.StageExecutor,
	registry *agent.AgentRegistry,
	config *ParallelConfig,
) (*ParallelEngine, error) {
	if config == nil {
		config = DefaultParallelConfig()
	}

	// Create base engine
	baseConfig := blueprint.DefaultEngineConfig()
	baseEngine, err := blueprint.NewEngine(bp, executor, baseConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create base engine: %w", err)
	}

	return &ParallelEngine{
		baseEngine:    baseEngine,
		agentRegistry: registry,
		executor:      executor,
		config:        config,
	}, nil
}

// ParallelStage extends a blueprint stage with parallel execution options.
type ParallelStage struct {
	*blueprint.Stage

	// EnableParallel enables parallel execution for this stage.
	EnableParallel bool `json:"enable_parallel,omitempty" yaml:"enable_parallel,omitempty"`

	// MaxAgents overrides the global max agents for this stage.
	MaxAgents int `json:"max_agents,omitempty" yaml:"max_agents,omitempty"`

	// BatchStrategy determines how to split work.
	BatchStrategy agent.BatchStrategy `json:"batch_strategy,omitempty" yaml:"batch_strategy,omitempty"`

	// MergeStrategy determines how to combine results.
	MergeStrategy agent.MergeStrategy `json:"merge_strategy,omitempty" yaml:"merge_strategy,omitempty"`
}

// ExecuteParallelStage executes a stage in parallel across multiple agents.
func (pe *ParallelEngine) ExecuteParallelStage(
	ctx context.Context,
	run *blueprint.Run,
	stage *ParallelStage,
	input *blueprint.StageInput,
	files []string,
) (*blueprint.StageResult, error) {
	if !pe.shouldParallelize(stage, files) {
		// Fall back to sequential execution
		return pe.executor.Execute(ctx, stage.Stage, input)
	}

	// Create work batches
	batches := pe.createBatches(stage, files)
	numAgents := min(len(batches), pe.getMaxAgents(stage))

	// Create parallel executor
	parallelExec := agent.NewParallelExecutor(
		pe.agentRegistry,
		pe.executor,
		numAgents,
	)

	// Execute in parallel
	return parallelExec.ExecuteParallel(ctx, &agent.ParallelStage{
		Stage:         stage.Stage,
		Parallel:      true,
		MaxAgents:     numAgents,
		BatchStrategy: stage.BatchStrategy,
		MergeStrategy: stage.MergeStrategy,
	}, input, files)
}

// ExecuteBlueprint executes a blueprint with parallel stage support.
func (pe *ParallelEngine) ExecuteBlueprint(
	ctx context.Context,
	run *blueprint.Run,
	input *blueprint.StageInput,
	files []string,
) error {
	totalRetries := 0

	for run.CurrentStage != "complete" && run.CurrentStage != "" {
		// Check context cancellation
		select {
		case <-ctx.Done():
			run.Cancel()
			return ctx.Err()
		default:
		}

		// Get current stage
		stage, ok := pe.baseEngine.GetBlueprint().GetStage(run.CurrentStage)
		if !ok {
			run.Fail(fmt.Sprintf("stage %q not found", run.CurrentStage))
			return fmt.Errorf("stage %q not found", run.CurrentStage)
		}

		// Check if this stage should run in parallel
		parallelStage := pe.toParallelStage(stage)

		// Execute stage (parallel or sequential)
		var result *blueprint.StageResult
		var err error

		if pe.shouldUseParallelism(parallelStage, files) {
			result, err = pe.ExecuteParallelStage(ctx, run, parallelStage, input, files)
		} else {
			result, err = pe.executor.Execute(ctx, stage, input)
		}

		if err != nil {
			attempt := run.GetRetries(stage.Name) + 1
			result = blueprint.NewStageResult(stage.Name, attempt)
			result.Fail(err.Error())
		}

		run.AddResult(result)
		input.AddPreviousResult(result)

		// Handle result
		switch result.Status {
		case types.StageStatusCompleted:
			// Create checkpoint if configured
			if stage.CreateCheckpoint {
				run.AddCheckpoint()
			}

			// Move to next stage
			run.CurrentStage = stage.OnSuccess

		case types.StageStatusFailed:
			// Check if we can retry
			if stage.OnFailure != "" && run.GetRetries(stage.Name) < stage.MaxRetries {
				run.IncrementRetries(stage.Name)
				totalRetries++

				// Check total retry limit
				if totalRetries > 10 { // Use default max retries
					run.Fail("exceeded maximum total retries")
					return fmt.Errorf("exceeded maximum total retries")
				}

				// Move to failure handler
				run.CurrentStage = stage.OnFailure
			} else {
				run.Fail(result.Error)
				return fmt.Errorf("stage %q failed: %s", stage.Name, result.Error)
			}
		}
	}

	run.Complete()
	return nil
}

// shouldParallelize determines if a stage should run in parallel.
func (pe *ParallelEngine) shouldParallelize(stage *ParallelStage, files []string) bool {
	if !pe.config.EnableParallelism {
		return false
	}
	if !stage.EnableParallel {
		return false
	}
	if len(files) < pe.config.MinFilesForParallel {
		return false
	}
	// Only parallelize agentic stages
	if stage.Type != types.StageTypeAgentic {
		return false
	}
	return true
}

// shouldUseParallelism checks if we should use parallel execution.
func (pe *ParallelEngine) shouldUseParallelism(stage *ParallelStage, files []string) bool {
	return pe.shouldParallelize(stage, files)
}

// toParallelStage converts a regular stage to a parallel stage.
func (pe *ParallelEngine) toParallelStage(stage *blueprint.Stage) *ParallelStage {
	return &ParallelStage{
		Stage:          stage,
		EnableParallel: false, // Default to sequential
		MaxAgents:      0,
		BatchStrategy:  agent.BatchByFiles,
		MergeStrategy:  pe.config.DefaultMergeStrategy,
	}
}

// createBatches creates work batches for parallel execution.
func (pe *ParallelEngine) createBatches(stage *ParallelStage, files []string) []agent.WorkBatch {
	maxAgents := pe.getMaxAgents(stage)
	numBatches := min(len(files), maxAgents)

	if numBatches <= 1 {
		return []agent.WorkBatch{{Index: 0, Files: files}}
	}

	batches := make([]agent.WorkBatch, numBatches)
	filesPerBatch := len(files) / numBatches
	extraFiles := len(files) % numBatches

	fileIndex := 0
	for i := 0; i < numBatches; i++ {
		batchSize := filesPerBatch
		if i < extraFiles {
			batchSize++
		}

		batches[i] = agent.WorkBatch{
			Index: i,
			Files: files[fileIndex : fileIndex+batchSize],
		}

		if stage.BatchStrategy == agent.BatchByDirectory && len(batches[i].Files) > 0 {
			batches[i].Directory = filepath.Dir(batches[i].Files[0])
		}

		fileIndex += batchSize
	}

	return batches
}

// getMaxAgents returns the maximum number of agents for a stage.
func (pe *ParallelEngine) getMaxAgents(stage *ParallelStage) int {
	if stage.MaxAgents > 0 {
		return min(stage.MaxAgents, pe.config.MaxAgents)
	}
	return pe.config.MaxAgents
}

// GetAgentStatus returns the status of all agents for a run.
func (pe *ParallelEngine) GetAgentStatus(blueprintID, runID string) ([]*agent.Agent, error) {
	return pe.agentRegistry.ListByRun(blueprintID, runID)
}

// WaitForAgents waits for all agents to complete for a run.
func (pe *ParallelEngine) WaitForAgents(blueprintID, runID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for agents")
		case <-ticker.C:
			counts, err := pe.agentRegistry.CountByStatus(blueprintID, runID)
			if err != nil {
				return err
			}

			// Check if all agents are done
			running := counts[agent.AgentStatusRunning] + counts[agent.AgentStatusIdle]
			if running == 0 {
				return nil
			}
		}
	}
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParallelBlueprintBuilder helps build blueprints with parallel stages.
type ParallelBlueprintBuilder struct {
	blueprint *blueprint.Blueprint
}

// NewParallelBlueprintBuilder creates a new builder.
func NewParallelBlueprintBuilder(id, name string) *ParallelBlueprintBuilder {
	return &ParallelBlueprintBuilder{
		blueprint: &blueprint.Blueprint{
			ID:           id,
			Name:         name,
			Stages:       make(map[string]*blueprint.Stage),
			InitialStage: "",
			Version:      "1.0",
		},
	}
}

// AddParallelStage adds a stage with parallel execution enabled.
func (b *ParallelBlueprintBuilder) AddParallelStage(
	name string,
	stageType types.StageType,
	toolset string,
	maxAgents int,
	onSuccess string,
) *ParallelBlueprintBuilder {
	b.blueprint.Stages[name] = &blueprint.Stage{
		Name:      name,
		Type:      stageType,
		Toolset:   toolset,
		MaxRetries: 3,
		Timeout:   10 * time.Minute,
		OnSuccess: onSuccess,
	}

	if b.blueprint.InitialStage == "" {
		b.blueprint.InitialStage = name
	}

	return b
}

// AddSequentialStage adds a regular sequential stage.
func (b *ParallelBlueprintBuilder) AddSequentialStage(
	name string,
	stageType types.StageType,
	toolset string,
	onSuccess string,
) *ParallelBlueprintBuilder {
	return b.AddParallelStage(name, stageType, toolset, 0, onSuccess)
}

// Build returns the constructed blueprint.
func (b *ParallelBlueprintBuilder) Build() *blueprint.Blueprint {
	return b.blueprint
}

// CreateDefaultParallelBlueprint creates the default blueprint with parallel support.
func CreateDefaultParallelBlueprint() *blueprint.Blueprint {
	return NewParallelBlueprintBuilder("parallel_task", "Parallel Task").
		AddSequentialStage("gather_context", types.StageTypeDeterministic, "repo_readonly", "implement").
		AddParallelStage("implement", types.StageTypeAgentic, "coding_backend", 4, "lint").
		AddSequentialStage("lint", types.StageTypeDeterministic, "coding_backend", "test").
		AddSequentialStage("test", types.StageTypeDeterministic, "coding_backend", "review").
		AddSequentialStage("review", types.StageTypeAgentic, "repo_readonly", "complete").
		Build()
}
