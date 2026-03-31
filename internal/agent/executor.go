package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/blueprint"
)

// ParallelStage represents a stage that can be executed by multiple agents in parallel.
type ParallelStage struct {
	*blueprint.Stage

	// Parallel indicates if this stage supports parallel execution.
	Parallel bool `json:"parallel,omitempty" yaml:"parallel,omitempty"`

	// MaxAgents is the maximum number of agents to spawn.
	MaxAgents int `json:"max_agents,omitempty" yaml:"max_agents,omitempty"`

	// BatchStrategy determines how to split work among agents.
	BatchStrategy BatchStrategy `json:"batch_strategy,omitempty" yaml:"batch_strategy,omitempty"`

	// MergeStrategy determines how to combine agent results.
	MergeStrategy MergeStrategy `json:"merge_strategy,omitempty" yaml:"merge_strategy,omitempty"`
}

// BatchStrategy defines how work is distributed among agents.
type BatchStrategy string

const (
	// BatchByFiles splits work by file paths.
	BatchByFiles BatchStrategy = "files"
	// BatchByDirectory splits work by directory.
	BatchByDirectory BatchStrategy = "directory"
	// BatchBySymbol splits work by code symbols (functions, structs).
	BatchBySymbol BatchStrategy = "symbol"
)

// MergeStrategy defines how agent results are combined.
type MergeStrategy string

const (
	// MergeReconcile attempts to merge changes automatically.
	MergeReconcile MergeStrategy = "reconcile"
	// MergeSequential applies changes in agent order.
	MergeSequential MergeStrategy = "sequential"
	// MergeManual requires manual review of conflicts.
	MergeManual MergeStrategy = "manual"
)

// WorkBatch represents a unit of work assigned to an agent.
type WorkBatch struct {
	Index     int      `json:"index"`
	Files     []string `json:"files"`
	Symbols   []string `json:"symbols,omitempty"`
	Directory string   `json:"directory,omitempty"`
}

// ParallelExecutor manages parallel stage execution.
type ParallelExecutor struct {
	registry  *AgentRegistry
	executor  blueprint.StageExecutor
	maxAgents int
}

// NewParallelExecutor creates a new parallel executor.
func NewParallelExecutor(registry *AgentRegistry, executor blueprint.StageExecutor, maxAgents int) *ParallelExecutor {
	if maxAgents <= 0 {
		maxAgents = 4 // Default
	}
	return &ParallelExecutor{
		registry:  registry,
		executor:  executor,
		maxAgents: maxAgents,
	}
}

// ExecuteParallel runs a stage in parallel across multiple agents.
func (pe *ParallelExecutor) ExecuteParallel(
	ctx context.Context,
	stage *ParallelStage,
	input *blueprint.StageInput,
	files []string,
) (*blueprint.StageResult, error) {
	if !stage.Parallel || len(files) <= 1 {
		// Fall back to sequential execution
		return pe.executor.Execute(ctx, stage.Stage, input)
	}

	// Create work batches
	batches := pe.createBatches(stage, files)
	if len(batches) == 1 {
		// Only one batch, no need for parallelization
		return pe.executor.Execute(ctx, stage.Stage, input)
	}

	// Limit number of agents
	numAgents := min(len(batches), stage.MaxAgents)
	if numAgents <= 0 {
		numAgents = min(len(batches), pe.maxAgents)
	}

	// Spawn agents
	agents := make([]*Agent, numAgents)
	results := make(chan *agentResult, numAgents)
	var wg sync.WaitGroup

	for i := 0; i < numAgents; i++ {
		agentID := fmt.Sprintf("%s-agent-%d", input.RunID, i)
		agent := &Agent{
			ID:          agentID,
			Type:        AgentTypeWorker,
			Status:      AgentStatusIdle,
			BlueprintID: stage.Name,
			RunID:       input.RunID,
			StageName:   stage.Name,
			BatchIndex:  i,
			BatchSize:   len(batches[i].Files),
			StartedAt:   time.Now().UTC(),
		}

		// Register agent
		if err := pe.registry.Register(agent); err != nil {
			return nil, fmt.Errorf("failed to register agent %s: %w", agentID, err)
		}
		agents[i] = agent

		// Start agent goroutine
		wg.Add(1)
		go pe.runAgent(ctx, agent, stage, input, batches[i], &wg, results)
	}

	// Wait for all agents to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	var agentResults []*agentResult
	for result := range results {
		agentResults = append(agentResults, result)
	}

	// Merge results
	mergedResult, err := pe.mergeResults(stage, agentResults)
	if err != nil {
		return nil, fmt.Errorf("failed to merge agent results: %w", err)
	}

	return mergedResult, nil
}

// agentResult wraps an agent execution result.
type agentResult struct {
	AgentID string
	Result  *blueprint.StageResult
	Error   error
}

func (pe *ParallelExecutor) runAgent(
	ctx context.Context,
	agent *Agent,
	stage *ParallelStage,
	input *blueprint.StageInput,
	batch WorkBatch,
	wg *sync.WaitGroup,
	results chan<- *agentResult,
) {
	defer wg.Done()

	// Update status to running
	if err := pe.registry.UpdateStatus(agent.ID, AgentStatusRunning); err != nil {
		results <- &agentResult{AgentID: agent.ID, Error: err}
		return
	}

	// Create batch-specific input
	batchInput := pe.createBatchInput(input, batch)

	// Execute stage
	result, err := pe.executor.Execute(ctx, stage.Stage, batchInput)
	if err != nil {
		pe.registry.Fail(agent.ID, err.Error())
		results <- &agentResult{AgentID: agent.ID, Error: err}
		return
	}

	// Complete agent
	ar := &AgentResult{
		FilesProcessed: batch.Files,
		Summary:        result.Output,
	}
	if err := pe.registry.Complete(agent.ID, ar); err != nil {
		results <- &agentResult{AgentID: agent.ID, Error: err}
		return
	}

	results <- &agentResult{AgentID: agent.ID, Result: result}
}

func (pe *ParallelExecutor) createBatches(stage *ParallelStage, files []string) []WorkBatch {
	maxAgents := stage.MaxAgents
	if maxAgents <= 0 {
		maxAgents = pe.maxAgents
	}

	// Limit batches to available files and max agents
	numBatches := min(len(files), maxAgents)
	if numBatches <= 1 {
		return []WorkBatch{{Index: 0, Files: files}}
	}

	batches := make([]WorkBatch, numBatches)
	filesPerBatch := len(files) / numBatches
	extraFiles := len(files) % numBatches

	fileIndex := 0
	for i := 0; i < numBatches; i++ {
		batchSize := filesPerBatch
		if i < extraFiles {
			batchSize++ // Distribute extra files
		}

		batches[i] = WorkBatch{
			Index: i,
			Files: files[fileIndex : fileIndex+batchSize],
		}

		// Set directory for directory-based batching
		if stage.BatchStrategy == BatchByDirectory && len(batches[i].Files) > 0 {
			batches[i].Directory = filepath.Dir(batches[i].Files[0])
		}

		fileIndex += batchSize
	}

	return batches
}

func (pe *ParallelExecutor) createBatchInput(base *blueprint.StageInput, batch WorkBatch) *blueprint.StageInput {
	// Create a copy with batch-specific context
	batchInput := blueprint.NewStageInput(
		base.RunID+fmt.Sprintf("-batch-%d", batch.Index),
		base.TaskDescription,
		base.WorkingDirectory,
	)

	// Copy previous results
	for _, result := range base.PreviousStages {
		batchInput.AddPreviousResult(result)
	}

	// Copy original variables
	for k, v := range base.Variables {
		batchInput.Variables[k] = v
	}

	// Add batch-specific variables
	batchInput.Variables["batch_index"] = fmt.Sprintf("%d", batch.Index)
	batchInput.Variables["batch_files"] = strings.Join(batch.Files, ",")
	if batch.Directory != "" {
		batchInput.Variables["batch_directory"] = batch.Directory
	}

	return batchInput
}

func (pe *ParallelExecutor) mergeResults(
	stage *ParallelStage,
	agentResults []*agentResult,
) (*blueprint.StageResult, error) {
	// Check for errors
	var errors []string
	var successfulResults []*blueprint.StageResult

	for _, ar := range agentResults {
		if ar.Error != nil {
			errors = append(errors, fmt.Sprintf("Agent %s: %v", ar.AgentID, ar.Error))
		} else if ar.Result != nil {
			successfulResults = append(successfulResults, ar.Result)
		}
	}

	if len(successfulResults) == 0 {
		return nil, fmt.Errorf("all agents failed: %s", strings.Join(errors, "; "))
	}

	// Create merged result
	merged := blueprint.NewStageResult(stage.Name, 1)

	// Merge outputs
	var outputs []string
	for _, r := range successfulResults {
		outputs = append(outputs, r.Output)
	}
	merged.Complete(pe.formatMergedOutput(stage, outputs, errors))

	return merged, nil
}

func (pe *ParallelExecutor) formatMergedOutput(
	stage *ParallelStage,
	outputs []string,
	errors []string,
) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Parallel Execution Results (%d agents)\n\n", len(outputs)))

	if len(errors) > 0 {
		sb.WriteString("## Errors\n")
		for _, err := range errors {
			sb.WriteString(fmt.Sprintf("- %s\n", err))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Agent Outputs\n\n")
	for i, output := range outputs {
		sb.WriteString(fmt.Sprintf("### Agent %d\n%s\n\n", i+1, output))
	}

	return sb.String()
}

// min returns the minimum of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ParallelBlueprint creates a blueprint with parallel stage support.
func ParallelBlueprint(base *blueprint.Blueprint) *blueprint.Blueprint {
	// Clone the blueprint
	parallelBlueprint := &blueprint.Blueprint{
		ID:           base.ID + "-parallel",
		Name:         base.Name + " (Parallel)",
		Description:  base.Description + " with parallel execution support",
		Stages:       make(map[string]*blueprint.Stage),
		InitialStage: base.InitialStage,
		Version:      base.Version,
	}

	// Copy stages and add parallel support
	for name, stage := range base.Stages {
		parallelBlueprint.Stages[name] = stage

		// Enable parallel execution for implementation stages
		if name == "implement" {
			// This would need type assertion in real implementation
			// For now, we document the pattern
		}
	}

	return parallelBlueprint
}

// AgentCoordinator manages the overall multi-agent workflow.
type AgentCoordinator struct {
	registry *AgentRegistry
	executor *ParallelExecutor
}

// NewAgentCoordinator creates a new coordinator.
func NewAgentCoordinator(registry *AgentRegistry, executor blueprint.StageExecutor, maxAgents int) *AgentCoordinator {
	return &AgentCoordinator{
		registry: registry,
		executor: NewParallelExecutor(registry, executor, maxAgents),
	}
}

// CoordinateRun manages a complete multi-agent blueprint run.
func (ac *AgentCoordinator) CoordinateRun(
	ctx context.Context,
	bp *blueprint.Blueprint,
	input *blueprint.StageInput,
	files []string,
) (*blueprint.StageResult, error) {
	// Check if we should use parallel execution
	implementStage, ok := bp.Stages["implement"]
	if !ok || len(files) < 5 {
		// Not enough files or no implement stage, use sequential
		return ac.runSequential(ctx, bp, input)
	}

	// Create parallel stage
	parallelStage := &ParallelStage{
		Stage:         implementStage,
		Parallel:      true,
		MaxAgents:     4,
		BatchStrategy: BatchByFiles,
		MergeStrategy: MergeReconcile,
	}

	// Execute in parallel
	return ac.executor.ExecuteParallel(ctx, parallelStage, input, files)
}

func (ac *AgentCoordinator) runSequential(
	ctx context.Context,
	bp *blueprint.Blueprint,
	input *blueprint.StageInput,
) (*blueprint.StageResult, error) {
	// Fall back to standard sequential execution
	// This would use the existing blueprint engine
	return nil, fmt.Errorf("sequential execution not implemented in coordinator")
}
