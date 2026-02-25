// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// TaskComplete message type constant
const (
	// TypeTaskComplete is the type identifier for task complete event messages.
	TypeTaskComplete = "task_complete"
)

// Errors specific to task complete handling.
var (
	ErrInvalidTaskCompleteStatus = errors.New("invalid task complete status")
)

// TaskCompleteStatus represents the final status of a completed task.
type TaskCompleteStatus string

const (
	// TaskCompleteStatusSuccess indicates the task completed successfully.
	TaskCompleteStatusSuccess TaskCompleteStatus = "success"

	// TaskCompleteStatusFailure indicates the task failed during execution.
	TaskCompleteStatusFailure TaskCompleteStatus = "failure"

	// TaskCompleteStatusCancelled indicates the task was cancelled.
	TaskCompleteStatusCancelled TaskCompleteStatus = "cancelled"

	// TaskCompleteStatusTimeout indicates the task timed out.
	TaskCompleteStatusTimeout TaskCompleteStatus = "timeout"
)

// TaskCompleteEvent represents a task completion event initiated by OpenExec.
// It is sent to the gateway when a task finishes execution, regardless of outcome.
type TaskCompleteEvent struct {
	BaseMessage

	// TaskID is the unique identifier of the completed task.
	TaskID string `json:"task_id"`

	// Status indicates the final outcome of the task.
	Status TaskCompleteStatus `json:"status"`

	// Message provides a human-readable description of the completion.
	Message string `json:"message,omitempty"`

	// ProjectID identifies which project this task belongs to.
	ProjectID string `json:"project_id,omitempty"`

	// ExitCode is the numeric exit code if applicable (e.g., for command execution).
	ExitCode *int `json:"exit_code,omitempty"`

	// Error contains error details if the task failed.
	Error string `json:"error,omitempty"`

	// Duration is the task execution duration in seconds.
	Duration float64 `json:"duration,omitempty"`

	// StartedAt is when the task started (RFC3339 format).
	StartedAt string `json:"started_at,omitempty"`

	// CompletedAt is when the task completed (RFC3339 format).
	CompletedAt string `json:"completed_at,omitempty"`

	// Output contains task output or result data (optional, truncated if large).
	Output string `json:"output,omitempty"`

	// Metadata contains additional structured data about the task completion.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewTaskCompleteEvent creates a new task complete event message.
func NewTaskCompleteEvent(taskID string, status TaskCompleteStatus, message string) *TaskCompleteEvent {
	return &TaskCompleteEvent{
		BaseMessage: BaseMessage{
			Type:      TypeTaskComplete,
			RequestID: taskID, // Use task ID as request ID for correlation
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:      taskID,
		Status:      status,
		Message:     message,
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// NewTaskCompleteEventWithDetails creates a task complete event with additional details.
func NewTaskCompleteEventWithDetails(taskID string, status TaskCompleteStatus, message, projectID string, duration float64) *TaskCompleteEvent {
	event := NewTaskCompleteEvent(taskID, status, message)
	event.ProjectID = projectID
	event.Duration = duration
	return event
}

// Validate validates the task complete event message.
func (e *TaskCompleteEvent) Validate() error {
	if e.Type == "" {
		return ErrMissingType
	}
	if e.Type != TypeTaskComplete {
		return ErrUnknownType
	}
	if e.TaskID == "" {
		return ErrMissingTaskID
	}
	if e.Status == "" {
		return ErrInvalidTaskCompleteStatus
	}
	// Validate status is one of the known values
	switch e.Status {
	case TaskCompleteStatusSuccess, TaskCompleteStatusFailure, TaskCompleteStatusCancelled, TaskCompleteStatusTimeout:
		// Valid status
	default:
		return ErrInvalidTaskCompleteStatus
	}
	return nil
}

// IsSuccess returns true if the task completed successfully.
func (e *TaskCompleteEvent) IsSuccess() bool {
	return e.Status == TaskCompleteStatusSuccess
}

// IsFailure returns true if the task failed (failure, cancelled, or timeout).
func (e *TaskCompleteEvent) IsFailure() bool {
	return e.Status == TaskCompleteStatusFailure ||
		e.Status == TaskCompleteStatusCancelled ||
		e.Status == TaskCompleteStatusTimeout
}

// MarshalJSON implements json.Marshaler for TaskCompleteEvent.
func (e *TaskCompleteEvent) MarshalJSON() ([]byte, error) {
	type Alias TaskCompleteEvent
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler for TaskCompleteEvent.
func (e *TaskCompleteEvent) UnmarshalJSON(data []byte) error {
	type Alias TaskCompleteEvent
	aux := (*Alias)(e)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
