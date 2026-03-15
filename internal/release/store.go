package release

import (
	"context"
	"errors"
)

// Common errors for store operations.
var (
	ErrReleaseNotFound        = errors.New("release not found")
	ErrReleaseAlreadyExist    = errors.New("release already exists")
	ErrGoalNotFound           = errors.New("goal not found")
	ErrGoalAlreadyExist       = errors.New("goal already exists")
	ErrStoryNotFound          = errors.New("story not found")
	ErrStoryAlreadyExist      = errors.New("story already exists")
	ErrTaskNotFound           = errors.New("task not found")
	ErrTaskAlreadyExist       = errors.New("task already exists")
	ErrCheckpointNotFound     = errors.New("checkpoint not found")
	ErrCheckpointAlreadyExist = errors.New("checkpoint already exists")
	ErrInvalidData            = errors.New("invalid data")
)

// Store defines the interface for release state persistence.
// Implementations must be safe for concurrent access.
type Store interface {
	// Release operations
	CreateRelease(ctx context.Context, r *Release) error
	GetRelease(ctx context.Context) (*Release, error)
	UpdateRelease(ctx context.Context, r *Release) error
	DeleteRelease(ctx context.Context) error

	// Goal operations
	CreateGoal(ctx context.Context, g *Goal) error
	GetGoal(ctx context.Context, id string) (*Goal, error)
	ListGoals(ctx context.Context) ([]*Goal, error)
	UpdateGoal(ctx context.Context, g *Goal) error
	DeleteGoal(ctx context.Context, id string) error

	// Story operations
	CreateStory(ctx context.Context, s *Story) error
	GetStory(ctx context.Context, id string) (*Story, error)
	ListStories(ctx context.Context) ([]*Story, error)
	ListStoriesByGoal(ctx context.Context, goalID string) ([]*Story, error)
	ListStoriesByStatus(ctx context.Context, status string) ([]*Story, error)
	UpdateStory(ctx context.Context, s *Story) error
	DeleteStory(ctx context.Context, id string) error

	// Task operations
	CreateTask(ctx context.Context, t *Task) error
	GetTask(ctx context.Context, id string) (*Task, error)
	ListTasks(ctx context.Context) ([]*Task, error)
	ListTasksByStory(ctx context.Context, storyID string) ([]*Task, error)
	ListTasksByStatus(ctx context.Context, status string) ([]*Task, error)
	UpdateTask(ctx context.Context, t *Task) error
	DeleteTask(ctx context.Context, id string) error

	// Bulk operations for bootstrap
	BulkCreateGoals(ctx context.Context, goals []*Goal) error
	BulkCreateStories(ctx context.Context, stories []*Story) error
	BulkCreateTasks(ctx context.Context, tasks []*Task) error

	// Count operations for bootstrap check
	CountStories(ctx context.Context) (int, error)
	CountGoals(ctx context.Context) (int, error)
	CountTasks(ctx context.Context) (int, error)

	// Checkpoint operations (for blueprint resumability)
	CreateCheckpoint(ctx context.Context, cp *Checkpoint) error
	GetCheckpoint(ctx context.Context, id string) (*Checkpoint, error)
	ListCheckpointsForRun(ctx context.Context, runID string) ([]*Checkpoint, error)
	GetLatestCheckpoint(ctx context.Context, runID string) (*Checkpoint, error)
	DeleteCheckpoint(ctx context.Context, id string) error
	DeleteCheckpointsForRun(ctx context.Context, runID string) error

	// Lifecycle
	Close() error
}
