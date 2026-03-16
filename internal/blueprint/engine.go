// Package blueprint provides stage-based execution orchestration for AI-driven tasks.
//
// Blueprints define sequences of deterministic and agentic stages that execute
// to complete a task. The standard flow is:
//
//	gather_context -> implement -> lint -> test -> review
//
// Key types:
//   - Blueprint: Defines the complete execution plan with stages and transitions
//   - Engine: Executes blueprints, managing state and callbacks
//   - Stage: A single step that is either deterministic (shell commands) or agentic (LLM)
//   - Run: Tracks the state of a single blueprint execution
//
// Stages support retry logic, checkpointing, and can route to different stages
// based on success or failure.
package blueprint

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/openexec/openexec/internal/types"
)

// Blueprint defines a complete execution plan with stages.
type Blueprint struct {
	// ID is the unique identifier for this blueprint.
	ID string `json:"id" yaml:"id"`

	// Name is the human-readable name.
	Name string `json:"name" yaml:"name"`

	// Description explains what this blueprint does.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`

	// Stages is the map of stage name to stage definition.
	Stages map[string]*Stage `json:"stages" yaml:"stages"`

	// InitialStage is the first stage to execute.
	InitialStage string `json:"initial_stage" yaml:"initial_stage"`

	// Version is the blueprint version.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

// GetStage retrieves a stage by name.
func (b *Blueprint) GetStage(name string) (*Stage, bool) {
	stage, ok := b.Stages[name]
	return stage, ok
}

// Validate checks if the blueprint is valid.
func (b *Blueprint) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("blueprint ID is required")
	}
	if b.Name == "" {
		return fmt.Errorf("blueprint name is required")
	}
	if b.InitialStage == "" {
		return fmt.Errorf("initial stage is required")
	}
	if _, ok := b.Stages[b.InitialStage]; !ok {
		return fmt.Errorf("initial stage %q not found in stages", b.InitialStage)
	}

	// Validate all stage references
	for name, stage := range b.Stages {
		if stage.OnSuccess != "" && stage.OnSuccess != "complete" {
			if _, ok := b.Stages[stage.OnSuccess]; !ok {
				return fmt.Errorf("stage %q references unknown OnSuccess stage %q", name, stage.OnSuccess)
			}
		}
		if stage.OnFailure != "" {
			if _, ok := b.Stages[stage.OnFailure]; !ok {
				return fmt.Errorf("stage %q references unknown OnFailure stage %q", name, stage.OnFailure)
			}
		}
	}

	return nil
}

// DefaultBlueprint is the standard task blueprint.
var DefaultBlueprint = &Blueprint{
	ID:          "standard_task",
	Name:        "Standard Task",
	Description: "Default blueprint for implementing tasks with lint/test validation",
	Version:     "1.0",
	Stages: map[string]*Stage{
		"gather_context": {
			Name:             "gather_context",
			Description:      "Gather relevant files and context for the task",
			Type:             types.StageTypeDeterministic,
			Toolset:          "repo_readonly",
			Action:           "build_context",
			OnSuccess:        "implement",
			CreateCheckpoint: true,
		},
		"implement": {
			Name:        "implement",
			Description: "Implement the requested changes",
			Type:        types.StageTypeAgentic,
			Toolset:     "coding_backend",
			MaxRetries:  3,
			Timeout:     10 * time.Minute,
			OnSuccess:   "lint",
			OnFailure:   "implement",
		},
		"lint": {
			Name:        "lint",
			Description: "Run linting checks",
			Type:        types.StageTypeDeterministic,
			Toolset:     "coding_backend",
			Commands:    nil, // Set from project config; empty = auto-pass
			OnSuccess:   "test",
			OnFailure:   "fix_lint",
		},
		"fix_lint": {
			Name:        "fix_lint",
			Description: "Fix linting errors",
			Type:        types.StageTypeAgentic,
			Toolset:     "coding_backend",
			MaxRetries:  2,
			OnSuccess:   "lint",
		},
		"test": {
			Name:        "test",
			Description: "Run tests",
			Type:        types.StageTypeDeterministic,
			Toolset:     "coding_backend",
			Commands:    nil, // Set from project config; empty = auto-pass
			OnSuccess:   "review",
			OnFailure:   "fix_tests",
		},
		"fix_tests": {
			Name:        "fix_tests",
			Description: "Fix failing tests",
			Type:        types.StageTypeAgentic,
			Toolset:     "coding_backend",
			MaxRetries:  2,
			OnSuccess:   "test",
		},
		"review": {
			Name:             "review",
			Description:      "Review changes and generate summary",
			Type:             types.StageTypeAgentic,
			Toolset:          "repo_readonly",
			OnSuccess:        "complete",
			CreateCheckpoint: true,
		},
	},
	InitialStage: "gather_context",
}

// QuickFixBlueprint is a simplified blueprint for small fixes.
var QuickFixBlueprint = &Blueprint{
	ID:          "quick_fix",
	Name:        "Quick Fix",
	Description: "Simplified blueprint for small, targeted fixes",
	Version:     "1.0",
	Stages: map[string]*Stage{
		"implement": {
			Name:       "implement",
			Type:       types.StageTypeAgentic,
			Toolset:    "coding_backend",
			MaxRetries: 2,
			OnSuccess:  "verify",
		},
		"verify": {
			Name:      "verify",
			Type:      types.StageTypeDeterministic,
			Toolset:   "coding_backend",
			Action:    "run_gates",
			OnSuccess: "complete",
		},

	},
	InitialStage: "implement",
}

// RunStatus represents the status of a blueprint run.
type RunStatus string

const (
	// RunStatusPending indicates the run has not started.
	RunStatusPending RunStatus = "pending"

	// RunStatusRunning indicates the run is in progress.
	RunStatusRunning RunStatus = "running"

	// RunStatusPaused indicates the run is paused (e.g., awaiting approval).
	RunStatusPaused RunStatus = "paused"

	// RunStatusCompleted indicates the run completed successfully.
	RunStatusCompleted RunStatus = "completed"

	// RunStatusFailed indicates the run failed.
	RunStatusFailed RunStatus = "failed"

	// RunStatusCancelled indicates the run was cancelled.
	RunStatusCancelled RunStatus = "cancelled"
)

// Run represents a single execution of a blueprint.
type Run struct {
	// ID is the unique identifier for this run.
	ID string `json:"id"`

	// BlueprintID is the ID of the blueprint being executed.
	BlueprintID string `json:"blueprint_id"`

	// Status is the current run status.
	Status RunStatus `json:"status"`

	// CurrentStage is the name of the currently executing stage.
	CurrentStage string `json:"current_stage"`

	// StageRetries tracks retry attempts per stage.
	StageRetries map[string]int `json:"stage_retries"`

	// Results contains the results of completed stages.
	Results []*StageResult `json:"results"`

	// StartedAt is when the run started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the run completed.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Error is the error message if the run failed.
	Error string `json:"error,omitempty"`

	// Checkpoints are stage names where checkpoints were created.
	Checkpoints []string `json:"checkpoints,omitempty"`
}

// NewRun creates a new run for a blueprint.
func NewRun(id, blueprintID, initialStage string) *Run {
	return &Run{
		ID:           id,
		BlueprintID:  blueprintID,
		Status:       RunStatusPending,
		CurrentStage: initialStage,
		StageRetries: make(map[string]int),
		Results:      make([]*StageResult, 0),
		StartedAt:    time.Now().UTC(),
		Checkpoints:  make([]string, 0),
	}
}

// AddResult adds a stage result to the run.
func (r *Run) AddResult(result *StageResult) {
	r.Results = append(r.Results, result)
}

// GetLastResult returns the last stage result.
func (r *Run) GetLastResult() *StageResult {
	if len(r.Results) == 0 {
		return nil
	}
	return r.Results[len(r.Results)-1]
}

// IncrementRetries increments the retry counter for a stage.
func (r *Run) IncrementRetries(stageName string) int {
	r.StageRetries[stageName]++
	return r.StageRetries[stageName]
}

// GetRetries returns the current retry count for a stage.
func (r *Run) GetRetries(stageName string) int {
	return r.StageRetries[stageName]
}

// AddCheckpoint records a checkpoint at the current stage.
func (r *Run) AddCheckpoint() {
	r.Checkpoints = append(r.Checkpoints, r.CurrentStage)
}

// Complete marks the run as completed.
func (r *Run) Complete() {
	r.Status = RunStatusCompleted
	now := time.Now().UTC()
	r.CompletedAt = &now
}

// Fail marks the run as failed.
func (r *Run) Fail(err string) {
	r.Status = RunStatusFailed
	r.Error = err
	now := time.Now().UTC()
	r.CompletedAt = &now
}

// Cancel marks the run as cancelled.
func (r *Run) Cancel() {
	r.Status = RunStatusCancelled
	now := time.Now().UTC()
	r.CompletedAt = &now
}

// EngineConfig contains configuration for the blueprint engine.
type EngineConfig struct {
	// MaxTotalRetries is the maximum total retries across all stages.
	MaxTotalRetries int

	// DefaultTimeout is the default stage timeout.
	DefaultTimeout time.Duration

	// OnStageStart is called when a stage begins execution.
	OnStageStart func(run *Run, stageName string)

	// OnStageComplete is called when a stage completes.
	OnStageComplete func(run *Run, result *StageResult)

	// OnCheckpoint is called when a checkpoint is created.
	OnCheckpoint func(run *Run, stageName string)

	// OnRunComplete is called when the run completes.
	OnRunComplete func(run *Run)
}

// DefaultEngineConfig returns default engine configuration.
func DefaultEngineConfig() *EngineConfig {
	return &EngineConfig{
		MaxTotalRetries: 10,
		DefaultTimeout:  5 * time.Minute,
	}
}

// Engine executes blueprints.
type Engine struct {
	mu        sync.RWMutex
	blueprint *Blueprint
	executor  StageExecutor
	config    *EngineConfig
	runs      map[string]*Run
}

// NewEngine creates a new blueprint engine.
func NewEngine(blueprint *Blueprint, executor StageExecutor, config *EngineConfig) (*Engine, error) {
	if err := blueprint.Validate(); err != nil {
		return nil, fmt.Errorf("invalid blueprint: %w", err)
	}
	if config == nil {
		config = DefaultEngineConfig()
	}

	return &Engine{
		blueprint: blueprint,
		executor:  executor,
		config:    config,
		runs:      make(map[string]*Run),
	}, nil
}

// GetBlueprint returns the engine's blueprint.
func (e *Engine) GetBlueprint() *Blueprint {
	return e.blueprint
}

// StartRun begins a new blueprint execution.
func (e *Engine) StartRun(ctx context.Context, runID string, input *StageInput) (*Run, error) {
	e.mu.Lock()
	run := NewRun(runID, e.blueprint.ID, e.blueprint.InitialStage)
	run.Status = RunStatusRunning
	e.runs[runID] = run
	e.mu.Unlock()

	return run, nil
}

// GetRun retrieves a run by ID.
func (e *Engine) GetRun(runID string) (*Run, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	run, ok := e.runs[runID]
	return run, ok
}

// Execute runs the full blueprint for a given run.
func (e *Engine) Execute(ctx context.Context, run *Run, input *StageInput) error {
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
		stage, ok := e.blueprint.GetStage(run.CurrentStage)
		if !ok {
			run.Fail(fmt.Sprintf("stage %q not found", run.CurrentStage))
			return fmt.Errorf("stage %q not found", run.CurrentStage)
		}

		// Notify stage start
		if e.config.OnStageStart != nil {
			e.config.OnStageStart(run, stage.Name)
		}

		// Execute stage
		attempt := run.GetRetries(stage.Name) + 1
		result, err := e.executor.Execute(ctx, stage, input)
		if err != nil {
			result = NewStageResult(stage.Name, attempt)
			result.Fail(err.Error())
		}

		run.AddResult(result)
		input.AddPreviousResult(result)

		// Call callback if configured
		if e.config.OnStageComplete != nil {
			e.config.OnStageComplete(run, result)
		}

		// Handle result
		if result.Status == types.StageStatusCompleted {
			// Create checkpoint if configured
			if stage.CreateCheckpoint {
				run.AddCheckpoint()
				if e.config.OnCheckpoint != nil {
					e.config.OnCheckpoint(run, stage.Name)
				}
			}

			// Move to next stage
			run.CurrentStage = stage.OnSuccess
		} else if result.Status == types.StageStatusFailed {
			// Check if we can retry
			if stage.OnFailure != "" && run.GetRetries(stage.Name) < stage.MaxRetries {
				run.IncrementRetries(stage.Name)
				totalRetries++

				// Check total retry limit
				if totalRetries > e.config.MaxTotalRetries {
					run.Fail("exceeded maximum total retries")
					return fmt.Errorf("exceeded maximum total retries")
				}

				// Move to failure handler (which might be same stage for retry)
				run.CurrentStage = stage.OnFailure
			} else {
				run.Fail(result.Error)
				return fmt.Errorf("stage %q failed: %s", stage.Name, result.Error)
			}
		}
	}

	run.Complete()
	if e.config.OnRunComplete != nil {
		e.config.OnRunComplete(run)
	}

	return nil
}

// ExecuteStage executes a single stage and returns the result.
func (e *Engine) ExecuteStage(ctx context.Context, run *Run, stageName string, input *StageInput) (*StageResult, error) {
	stage, ok := e.blueprint.GetStage(stageName)
	if !ok {
		return nil, fmt.Errorf("stage %q not found", stageName)
	}

	run.CurrentStage = stageName
	result, err := e.executor.Execute(ctx, stage, input)
	if err != nil {
		return nil, err
	}

	run.AddResult(result)
	return result, nil
}

// Pause pauses the run execution.
func (e *Engine) Pause(runID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, ok := e.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}

	if run.Status != RunStatusRunning {
		return fmt.Errorf("run is not running")
	}

	run.Status = RunStatusPaused
	return nil
}

// Resume resumes a paused run.
func (e *Engine) Resume(runID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, ok := e.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}

	if run.Status != RunStatusPaused {
		return fmt.Errorf("run is not paused")
	}

	run.Status = RunStatusRunning
	return nil
}

// Cancel cancels a running or paused run.
func (e *Engine) Cancel(runID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	run, ok := e.runs[runID]
	if !ok {
		return fmt.Errorf("run %q not found", runID)
	}

	if run.Status != RunStatusRunning && run.Status != RunStatusPaused {
		return fmt.Errorf("run cannot be cancelled")
	}

	run.Cancel()
	return nil
}
