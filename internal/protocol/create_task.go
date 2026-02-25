// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// CreateTask message type constants
const (
	// TypeCreateTaskRequest is the type identifier for create task request messages.
	TypeCreateTaskRequest = "create_task_request"

	// TypeCreateTaskResponse is the type identifier for create task response messages.
	TypeCreateTaskResponse = "create_task_response"
)

// Errors specific to create task handling.
var (
	ErrMissingTitle            = errors.New("message missing title field")
	ErrMissingDescription      = errors.New("message missing description field")
	ErrInvalidPriority         = errors.New("invalid priority value")
	ErrInvalidCreateTaskStatus = errors.New("invalid create task status")
)

// TaskPriority represents the priority level of a task.
type TaskPriority string

const (
	// TaskPriorityLow is for low priority tasks.
	TaskPriorityLow TaskPriority = "low"

	// TaskPriorityNormal is for normal priority tasks.
	TaskPriorityNormal TaskPriority = "normal"

	// TaskPriorityHigh is for high priority tasks.
	TaskPriorityHigh TaskPriority = "high"

	// TaskPriorityCritical is for critical priority tasks.
	TaskPriorityCritical TaskPriority = "critical"
)

// CreateTaskRequest represents a request to create a new task in the queue.
// It is used to submit a new task for execution by the Gateway.
type CreateTaskRequest struct {
	BaseMessage

	// Title is a short, descriptive title for the task.
	Title string `json:"title"`

	// Description provides detailed information about what the task should do.
	Description string `json:"description"`

	// Priority indicates the task priority level (optional, defaults to normal).
	Priority TaskPriority `json:"priority,omitempty"`

	// ProjectID is the identifier of the project this task belongs to (optional).
	ProjectID string `json:"project_id,omitempty"`

	// Tags are optional labels for categorizing the task.
	Tags []string `json:"tags,omitempty"`

	// Metadata contains additional structured data associated with the task.
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// DependsOn is a list of task IDs that must complete before this task can run.
	DependsOn []string `json:"depends_on,omitempty"`

	// TimeoutSeconds specifies the maximum execution time in seconds (optional).
	// If not specified or zero, a default timeout is applied by the server.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

// CreateTaskStatus represents the status of a create task operation.
type CreateTaskStatus string

const (
	// CreateTaskStatusCreated indicates the task was successfully created.
	CreateTaskStatusCreated CreateTaskStatus = "created"

	// CreateTaskStatusQueued indicates the task was created and queued for execution.
	CreateTaskStatusQueued CreateTaskStatus = "queued"

	// CreateTaskStatusRejected indicates the create request was rejected.
	CreateTaskStatusRejected CreateTaskStatus = "rejected"

	// CreateTaskStatusFailed indicates the task creation failed.
	CreateTaskStatusFailed CreateTaskStatus = "failed"
)

// CreateTaskResponse represents the Gateway's response to a create task request.
type CreateTaskResponse struct {
	BaseMessage

	// TaskID is the unique identifier assigned to the newly created task.
	TaskID string `json:"task_id,omitempty"`

	// Status indicates the current status of the create task operation.
	Status CreateTaskStatus `json:"status"`

	// Message provides a human-readable description of the status.
	Message string `json:"message,omitempty"`

	// QueuePosition indicates the task's position in the queue (if queued).
	QueuePosition int `json:"queue_position,omitempty"`

	// Error contains error details if the creation failed or was rejected.
	Error string `json:"error,omitempty"`
}

// NewCreateTaskRequest creates a new create task request message.
func NewCreateTaskRequest(requestID, title, description string) *CreateTaskRequest {
	return &CreateTaskRequest{
		BaseMessage: BaseMessage{
			Type:      TypeCreateTaskRequest,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Title:       title,
		Description: description,
		Priority:    TaskPriorityNormal,
	}
}

// NewCreateTaskRequestWithOptions creates a create task request with additional options.
func NewCreateTaskRequestWithOptions(requestID, title, description string, priority TaskPriority, projectID string) *CreateTaskRequest {
	req := NewCreateTaskRequest(requestID, title, description)
	if priority != "" {
		req.Priority = priority
	}
	req.ProjectID = projectID
	return req
}

// NewCreateTaskResponse creates a new create task response message.
func NewCreateTaskResponse(requestID, taskID string, status CreateTaskStatus, message string) *CreateTaskResponse {
	return &CreateTaskResponse{
		BaseMessage: BaseMessage{
			Type:      TypeCreateTaskResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:  taskID,
		Status:  status,
		Message: message,
	}
}

// NewCreateTaskErrorResponse creates a new create task response with an error.
func NewCreateTaskErrorResponse(requestID string, status CreateTaskStatus, errorMsg string) *CreateTaskResponse {
	return &CreateTaskResponse{
		BaseMessage: BaseMessage{
			Type:      TypeCreateTaskResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Status: status,
		Error:  errorMsg,
	}
}

// Validate validates the create task request message.
func (r *CreateTaskRequest) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeCreateTaskRequest {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.Title == "" {
		return ErrMissingTitle
	}
	if r.Description == "" {
		return ErrMissingDescription
	}
	// Validate priority if specified
	if r.Priority != "" {
		switch r.Priority {
		case TaskPriorityLow, TaskPriorityNormal, TaskPriorityHigh, TaskPriorityCritical:
			// Valid priority
		default:
			return ErrInvalidPriority
		}
	}
	// Validate timeout if specified
	if r.TimeoutSeconds < 0 {
		return errors.New("timeout_seconds must be non-negative")
	}
	return nil
}

// Validate validates the create task response message.
func (r *CreateTaskResponse) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeCreateTaskResponse {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.Status == "" {
		return ErrInvalidCreateTaskStatus
	}
	// Validate status is one of the known values
	switch r.Status {
	case CreateTaskStatusCreated, CreateTaskStatusQueued, CreateTaskStatusRejected, CreateTaskStatusFailed:
		// Valid status
	default:
		return ErrInvalidCreateTaskStatus
	}
	// TaskID should be present for successful creation
	if (r.Status == CreateTaskStatusCreated || r.Status == CreateTaskStatusQueued) && r.TaskID == "" {
		return ErrMissingTaskID
	}
	return nil
}

// MarshalJSON implements json.Marshaler for CreateTaskRequest.
func (r *CreateTaskRequest) MarshalJSON() ([]byte, error) {
	type Alias CreateTaskRequest
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for CreateTaskRequest.
func (r *CreateTaskRequest) UnmarshalJSON(data []byte) error {
	type Alias CreateTaskRequest
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for CreateTaskResponse.
func (r *CreateTaskResponse) MarshalJSON() ([]byte, error) {
	type Alias CreateTaskResponse
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for CreateTaskResponse.
func (r *CreateTaskResponse) UnmarshalJSON(data []byte) error {
	type Alias CreateTaskResponse
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
