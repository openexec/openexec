// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Cancel message type constants
const (
	// TypeCancelRequest is the type identifier for cancel request messages.
	TypeCancelRequest = "cancel_request"

	// TypeCancelResponse is the type identifier for cancel response messages.
	TypeCancelResponse = "cancel_response"
)

// Errors specific to cancel handling.
var (
	ErrInvalidCancelStatus = errors.New("invalid cancel status")
)

// CancelRequest represents a request to cancel a running task.
// It is used to request cancellation of a specific task from the Gateway.
type CancelRequest struct {
	BaseMessage

	// TaskID is the unique identifier of the task to cancel.
	TaskID string `json:"task_id"`

	// Reason is an optional human-readable reason for the cancellation.
	Reason string `json:"reason,omitempty"`

	// Force, if true, requests immediate termination without graceful shutdown.
	Force bool `json:"force,omitempty"`
}

// CancelStatus represents the status of a cancel operation.
type CancelStatus string

const (
	// CancelStatusAccepted indicates the cancel request was accepted and cancellation is in progress.
	CancelStatusAccepted CancelStatus = "accepted"

	// CancelStatusCompleted indicates the task was successfully cancelled.
	CancelStatusCompleted CancelStatus = "completed"

	// CancelStatusFailed indicates the cancellation failed.
	CancelStatusFailed CancelStatus = "failed"

	// CancelStatusRejected indicates the cancel request was rejected (e.g., task not found or not running).
	CancelStatusRejected CancelStatus = "rejected"

	// CancelStatusNotFound indicates the task was not found.
	CancelStatusNotFound CancelStatus = "not_found"
)

// CancelResponse represents the Gateway's response to a cancel request.
type CancelResponse struct {
	BaseMessage

	// TaskID is the unique identifier of the task.
	TaskID string `json:"task_id"`

	// Status indicates the current status of the cancel operation.
	Status CancelStatus `json:"status"`

	// Message provides a human-readable description of the status.
	Message string `json:"message,omitempty"`

	// Error contains error details if the cancellation failed or was rejected.
	Error string `json:"error,omitempty"`
}

// NewCancelRequest creates a new cancel request message.
func NewCancelRequest(requestID, taskID string) *CancelRequest {
	return &CancelRequest{
		BaseMessage: BaseMessage{
			Type:      TypeCancelRequest,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID: taskID,
	}
}

// NewCancelRequestWithReason creates a cancel request with a reason.
func NewCancelRequestWithReason(requestID, taskID, reason string) *CancelRequest {
	req := NewCancelRequest(requestID, taskID)
	req.Reason = reason
	return req
}

// NewCancelRequestWithForce creates a cancel request with force flag.
func NewCancelRequestWithForce(requestID, taskID string, force bool) *CancelRequest {
	req := NewCancelRequest(requestID, taskID)
	req.Force = force
	return req
}

// NewCancelResponse creates a new cancel response message.
func NewCancelResponse(requestID, taskID string, status CancelStatus, message string) *CancelResponse {
	return &CancelResponse{
		BaseMessage: BaseMessage{
			Type:      TypeCancelResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:  taskID,
		Status:  status,
		Message: message,
	}
}

// NewCancelErrorResponse creates a new cancel response with an error.
func NewCancelErrorResponse(requestID, taskID string, status CancelStatus, errorMsg string) *CancelResponse {
	return &CancelResponse{
		BaseMessage: BaseMessage{
			Type:      TypeCancelResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID: taskID,
		Status: status,
		Error:  errorMsg,
	}
}

// Validate validates the cancel request message.
func (r *CancelRequest) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeCancelRequest {
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

// Validate validates the cancel response message.
func (r *CancelResponse) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeCancelResponse {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.TaskID == "" {
		return ErrMissingTaskID
	}
	if r.Status == "" {
		return ErrInvalidCancelStatus
	}
	// Validate status is one of the known values
	switch r.Status {
	case CancelStatusAccepted, CancelStatusCompleted, CancelStatusFailed, CancelStatusRejected, CancelStatusNotFound:
		// Valid status
	default:
		return ErrInvalidCancelStatus
	}
	return nil
}

// MarshalJSON implements json.Marshaler for CancelRequest.
func (r *CancelRequest) MarshalJSON() ([]byte, error) {
	type Alias CancelRequest
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for CancelRequest.
func (r *CancelRequest) UnmarshalJSON(data []byte) error {
	type Alias CancelRequest
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for CancelResponse.
func (r *CancelResponse) MarshalJSON() ([]byte, error) {
	type Alias CancelResponse
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for CancelResponse.
func (r *CancelResponse) UnmarshalJSON(data []byte) error {
	type Alias CancelResponse
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
