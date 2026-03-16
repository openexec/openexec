package types

import (
	"context"
)

// StageType indicates whether a stage is deterministic or agentic.
type StageType string

const (
	StageTypeDeterministic StageType = "deterministic"
	StageTypeAgentic       StageType = "agentic"
)

// StageStatus represents the current status of a stage.
type StageStatus string

const (
	StageStatusPending   StageStatus = "pending"
	StageStatusRunning   StageStatus = "running"
	StageStatusCompleted StageStatus = "completed"
	StageStatusFailed    StageStatus = "failed"
	StageStatusSkipped   StageStatus = "skipped"
)

// ActionRequest defines the input for a deterministic tool action.
type ActionRequest struct {
	RunID        string
	WorkspaceDir string
	Inputs       map[string]any
	Metadata     map[string]any
}

// ActionResponse defines the result of a tool action.
type ActionResponse struct {
	Status    StageStatus
	Output    string
	Error     string
	Artifacts map[string]string // Name -> Hash/Path
}

// Action is the interface for all deterministic runtime actions.
type Action interface {
	// Name returns the unique identifier for the action.
	Name() string

	// Execute runs the action logic.
	Execute(ctx context.Context, req ActionRequest) (ActionResponse, error)
}

// GateRunner defines the interface for executing quality gates.
type GateRunner interface {
	RunAll(ctx context.Context) error
}
