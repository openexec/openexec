// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Deploy command message type constants
const (
	// TypeDeployRequest is the type identifier for deploy request messages.
	TypeDeployRequest = "deploy_request"

	// TypeDeployResponse is the type identifier for deploy response messages.
	TypeDeployResponse = "deploy_response"
)

// Errors specific to deploy command handling.
var (
	ErrMissingProjectIDForDeploy = errors.New("message missing project_id field")
	ErrInvalidDeployStatus       = errors.New("invalid deploy status")
)

// DeployRequest represents a request from the Gateway to Openexec to deploy a project.
// It contains the project ID that identifies which project should be deployed.
type DeployRequest struct {
	BaseMessage

	// ProjectID is the unique identifier of the project to deploy.
	ProjectID string `json:"project_id"`

	// Environment is the target deployment environment (e.g., "staging", "production").
	Environment string `json:"environment,omitempty"`
}

// DeployStatus represents the status of a deploy operation.
type DeployStatus string

const (
	// DeployStatusAccepted indicates the deploy request was accepted.
	DeployStatusAccepted DeployStatus = "accepted"

	// DeployStatusRunning indicates the deployment is currently in progress.
	DeployStatusRunning DeployStatus = "running"

	// DeployStatusCompleted indicates the deployment completed successfully.
	DeployStatusCompleted DeployStatus = "completed"

	// DeployStatusFailed indicates the deployment failed.
	DeployStatusFailed DeployStatus = "failed"

	// DeployStatusRejected indicates the deploy request was rejected.
	DeployStatusRejected DeployStatus = "rejected"
)

// DeployResponse represents the Gateway's response to a deploy request.
type DeployResponse struct {
	BaseMessage

	// ProjectID is the unique identifier of the project.
	ProjectID string `json:"project_id"`

	// Status indicates the current status of the deploy operation.
	Status DeployStatus `json:"status"`

	// Message provides a human-readable description of the status.
	Message string `json:"message,omitempty"`

	// Error contains error details if the deploy failed or was rejected.
	Error string `json:"error,omitempty"`

	// Environment is the target deployment environment.
	Environment string `json:"environment,omitempty"`

	// Version is the deployed version identifier (e.g., commit SHA, tag).
	Version string `json:"version,omitempty"`
}

// NewDeployRequest creates a new deploy request message.
func NewDeployRequest(requestID, projectID string) *DeployRequest {
	return &DeployRequest{
		BaseMessage: BaseMessage{
			Type:      TypeDeployRequest,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		ProjectID: projectID,
	}
}

// NewDeployRequestWithEnv creates a new deploy request message with environment.
func NewDeployRequestWithEnv(requestID, projectID, environment string) *DeployRequest {
	req := NewDeployRequest(requestID, projectID)
	req.Environment = environment
	return req
}

// NewDeployResponse creates a new deploy response message.
func NewDeployResponse(requestID, projectID string, status DeployStatus, message string) *DeployResponse {
	return &DeployResponse{
		BaseMessage: BaseMessage{
			Type:      TypeDeployResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		ProjectID: projectID,
		Status:    status,
		Message:   message,
	}
}

// NewDeployErrorResponse creates a new deploy response with an error.
func NewDeployErrorResponse(requestID, projectID string, status DeployStatus, errorMsg string) *DeployResponse {
	return &DeployResponse{
		BaseMessage: BaseMessage{
			Type:      TypeDeployResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		ProjectID: projectID,
		Status:    status,
		Error:     errorMsg,
	}
}

// Validate validates the deploy request message.
func (r *DeployRequest) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeDeployRequest {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.ProjectID == "" {
		return ErrMissingProjectIDForDeploy
	}
	return nil
}

// Validate validates the deploy response message.
func (r *DeployResponse) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeDeployResponse {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.ProjectID == "" {
		return ErrMissingProjectIDForDeploy
	}
	if r.Status == "" {
		return ErrInvalidDeployStatus
	}
	// Validate status is one of the known values
	switch r.Status {
	case DeployStatusAccepted, DeployStatusRunning, DeployStatusCompleted, DeployStatusFailed, DeployStatusRejected:
		// Valid status
	default:
		return ErrInvalidDeployStatus
	}
	return nil
}

// MarshalJSON implements json.Marshaler for DeployRequest.
func (r *DeployRequest) MarshalJSON() ([]byte, error) {
	type Alias DeployRequest
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for DeployRequest.
func (r *DeployRequest) UnmarshalJSON(data []byte) error {
	type Alias DeployRequest
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for DeployResponse.
func (r *DeployResponse) MarshalJSON() ([]byte, error) {
	type Alias DeployResponse
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for DeployResponse.
func (r *DeployResponse) UnmarshalJSON(data []byte) error {
	type Alias DeployResponse
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
