// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Run command message type constants
const (
	// TypeRunRequest is the type identifier for run request messages.
	TypeRunRequest = "run_request"

	// TypeRunResponse is the type identifier for run response messages.
	TypeRunResponse = "run_response"
)

// Errors specific to run command handling.
var (
	ErrMissingTaskID    = errors.New("message missing task_id field")
	ErrInvalidRunStatus = errors.New("invalid run status")
)

// RunRequest represents a request from Openexec to the Gateway to run a task.
// It contains the task ID that identifies which task should be executed.
type RunRequest struct {
	BaseMessage

	// TaskID is the unique identifier of the task to run.
	TaskID string `json:"task_id"`
}

// RunStatus represents the status of a run operation.
type RunStatus string

const (
	// RunStatusAccepted indicates the run request was accepted.
	RunStatusAccepted RunStatus = "accepted"

	// RunStatusRunning indicates the task is currently running.
	RunStatusRunning RunStatus = "running"

	// RunStatusCompleted indicates the task completed successfully.
	RunStatusCompleted RunStatus = "completed"

	// RunStatusFailed indicates the task failed.
	RunStatusFailed RunStatus = "failed"

	// RunStatusRejected indicates the run request was rejected.
	RunStatusRejected RunStatus = "rejected"
)

// RunResponse represents the Gateway's response to a run request.
type RunResponse struct {
	BaseMessage

	// TaskID is the unique identifier of the task.
	TaskID string `json:"task_id"`

	// Status indicates the current status of the run operation.
	Status RunStatus `json:"status"`

	// Message provides a human-readable description of the status.
	Message string `json:"message,omitempty"`

	// Error contains error details if the run failed or was rejected.
	Error string `json:"error,omitempty"`
}

// NewRunRequest creates a new run request message.
func NewRunRequest(requestID, taskID string) *RunRequest {
	return &RunRequest{
		BaseMessage: BaseMessage{
			Type:      TypeRunRequest,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID: taskID,
	}
}

// NewRunResponse creates a new run response message.
func NewRunResponse(requestID, taskID string, status RunStatus, message string) *RunResponse {
	return &RunResponse{
		BaseMessage: BaseMessage{
			Type:      TypeRunResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:  taskID,
		Status:  status,
		Message: message,
	}
}

// NewRunErrorResponse creates a new run response with an error.
func NewRunErrorResponse(requestID, taskID string, status RunStatus, errorMsg string) *RunResponse {
	return &RunResponse{
		BaseMessage: BaseMessage{
			Type:      TypeRunResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID: taskID,
		Status: status,
		Error:  errorMsg,
	}
}

// Validate validates the run request message.
func (r *RunRequest) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeRunRequest {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.TaskID == "" {
		return ErrMissingTaskID
	}
	return nil
}

// Validate validates the run response message.
func (r *RunResponse) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeRunResponse {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.TaskID == "" {
		return ErrMissingTaskID
	}
	if r.Status == "" {
		return ErrInvalidRunStatus
	}
	// Validate status is one of the known values
	switch r.Status {
	case RunStatusAccepted, RunStatusRunning, RunStatusCompleted, RunStatusFailed, RunStatusRejected:
		// Valid status
	default:
		return ErrInvalidRunStatus
	}
	return nil
}

// MarshalJSON implements json.Marshaler for RunRequest.
func (r *RunRequest) MarshalJSON() ([]byte, error) {
	type Alias RunRequest
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for RunRequest.
func (r *RunRequest) UnmarshalJSON(data []byte) error {
	type Alias RunRequest
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for RunResponse.
func (r *RunResponse) MarshalJSON() ([]byte, error) {
	type Alias RunResponse
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for RunResponse.
func (r *RunResponse) UnmarshalJSON(data []byte) error {
	type Alias RunResponse
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
