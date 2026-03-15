// Package blueprint provides stage-based execution orchestration.
// Blueprints define sequences of deterministic and agentic stages that
// execute to complete a task (e.g., gather_context → implement → lint → test → review).
package blueprint

import (
	"context"
	"time"
)

// StageType indicates whether a stage is deterministic or agentic.
type StageType string

const (
	// StageTypeDeterministic indicates a stage with fixed, predictable behavior.
	// Deterministic stages run predefined commands (lint, test, etc.).
	StageTypeDeterministic StageType = "deterministic"

	// StageTypeAgentic indicates a stage requiring LLM reasoning.
	// Agentic stages involve code generation, debugging, or analysis.
	StageTypeAgentic StageType = "agentic"
)

// StageStatus represents the current status of a stage.
type StageStatus string

const (
	// StageStatusPending indicates the stage has not started.
	StageStatusPending StageStatus = "pending"

	// StageStatusRunning indicates the stage is currently executing.
	StageStatusRunning StageStatus = "running"

	// StageStatusCompleted indicates the stage completed successfully.
	StageStatusCompleted StageStatus = "completed"

	// StageStatusFailed indicates the stage failed.
	StageStatusFailed StageStatus = "failed"

	// StageStatusSkipped indicates the stage was skipped.
	StageStatusSkipped StageStatus = "skipped"
)

// Stage defines a single step in a blueprint.
type Stage struct {
	// Name is the unique identifier for this stage.
	Name string `json:"name"`

	// Description explains what this stage does.
	Description string `json:"description,omitempty"`

	// Type indicates whether this is deterministic or agentic.
	Type StageType `json:"type"`

	// Toolset specifies which toolset to use for this stage.
	Toolset string `json:"toolset"`

	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int `json:"max_retries,omitempty"`

	// Timeout is the maximum duration for this stage.
	Timeout time.Duration `json:"timeout,omitempty"`

	// OnSuccess is the next stage to execute on success.
	OnSuccess string `json:"on_success,omitempty"`

	// OnFailure is the stage to execute on failure (for retry or fallback).
	OnFailure string `json:"on_failure,omitempty"`

	// Commands lists shell commands for deterministic stages.
	Commands []string `json:"commands,omitempty"`

	// Prompt is the LLM prompt for agentic stages.
	Prompt string `json:"prompt,omitempty"`

	// CreateCheckpoint indicates whether to create a checkpoint after this stage.
	CreateCheckpoint bool `json:"create_checkpoint,omitempty"`
}

// IsTerminal returns true if this stage has no successor.
func (s *Stage) IsTerminal() bool {
	return s.OnSuccess == "" || s.OnSuccess == "complete"
}

// StageResult captures the outcome of a stage execution.
type StageResult struct {
	// StageName is the name of the executed stage.
	StageName string `json:"stage_name"`

	// Status is the outcome status.
	Status StageStatus `json:"status"`

	// StartedAt is when the stage started.
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the stage completed.
	CompletedAt time.Time `json:"completed_at"`

	// Duration is how long the stage took.
	Duration time.Duration `json:"duration"`

	// Attempt is the current retry attempt (1-based).
	Attempt int `json:"attempt"`

	// Output is the stage output (command output or LLM response).
	Output string `json:"output,omitempty"`

	// Error is the error message if the stage failed.
	Error string `json:"error,omitempty"`

	// Artifacts are files or data produced by this stage.
	Artifacts map[string]string `json:"artifacts,omitempty"`

	// Diagnostics contains additional debugging information.
	Diagnostics string `json:"diagnostics,omitempty"`
}

// NewStageResult creates a new stage result in running state.
func NewStageResult(stageName string, attempt int) *StageResult {
	return &StageResult{
		StageName: stageName,
		Status:    StageStatusRunning,
		StartedAt: time.Now().UTC(),
		Attempt:   attempt,
		Artifacts: make(map[string]string),
	}
}

// Complete marks the stage as completed successfully.
func (r *StageResult) Complete(output string) {
	r.Status = StageStatusCompleted
	r.Output = output
	r.CompletedAt = time.Now().UTC()
	r.Duration = r.CompletedAt.Sub(r.StartedAt)
}

// Fail marks the stage as failed.
func (r *StageResult) Fail(err string) {
	r.Status = StageStatusFailed
	r.Error = err
	r.CompletedAt = time.Now().UTC()
	r.Duration = r.CompletedAt.Sub(r.StartedAt)
}

// Skip marks the stage as skipped.
func (r *StageResult) Skip(reason string) {
	r.Status = StageStatusSkipped
	r.Output = reason
	r.CompletedAt = time.Now().UTC()
	r.Duration = r.CompletedAt.Sub(r.StartedAt)
}

// AddArtifact adds an artifact to the result.
func (r *StageResult) AddArtifact(name, value string) {
	r.Artifacts[name] = value
}

// StageExecutor is the interface for executing stages.
type StageExecutor interface {
	// Execute runs the stage and returns the result.
	Execute(ctx context.Context, stage *Stage, input *StageInput) (*StageResult, error)
}

// StageInput contains the input data for a stage execution.
type StageInput struct {
	// RunID is the ID of the current run.
	RunID string `json:"run_id"`

	// TaskDescription is the original user task.
	TaskDescription string `json:"task_description"`

	// PreviousStages contains results from previous stages.
	PreviousStages []*StageResult `json:"previous_stages,omitempty"`

	// ContextPack contains gathered context files.
	ContextPack map[string]string `json:"context_pack,omitempty"`

	// WorkingDirectory is the directory for execution.
	WorkingDirectory string `json:"working_directory"`

	// Variables are stage-specific variables.
	Variables map[string]string `json:"variables,omitempty"`
}

// NewStageInput creates a new stage input.
func NewStageInput(runID, taskDescription, workingDir string) *StageInput {
	return &StageInput{
		RunID:            runID,
		TaskDescription:  taskDescription,
		WorkingDirectory: workingDir,
		PreviousStages:   make([]*StageResult, 0),
		ContextPack:      make(map[string]string),
		Variables:        make(map[string]string),
	}
}

// AddPreviousResult adds a previous stage result.
func (i *StageInput) AddPreviousResult(result *StageResult) {
	i.PreviousStages = append(i.PreviousStages, result)
}

// GetLastResult returns the result of the last executed stage.
func (i *StageInput) GetLastResult() *StageResult {
	if len(i.PreviousStages) == 0 {
		return nil
	}
	return i.PreviousStages[len(i.PreviousStages)-1]
}

// HasFailedStage returns true if any previous stage failed.
func (i *StageInput) HasFailedStage() bool {
	for _, r := range i.PreviousStages {
		if r.Status == StageStatusFailed {
			return true
		}
	}
	return false
}
