// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Message type constants
const (
	// TypeStatusRequest is the type identifier for status request messages.
	TypeStatusRequest = "status_request"

	// TypeStatusResponse is the type identifier for status response messages.
	TypeStatusResponse = "status_response"
)

// Errors for protocol message handling.
var (
	ErrInvalidMessage    = errors.New("invalid message format")
	ErrUnknownType       = errors.New("unknown message type")
	ErrMissingType       = errors.New("message missing type field")
	ErrMissingRequestID  = errors.New("message missing request_id field")
	ErrInvalidStatusCode = errors.New("invalid status code")
)

// BaseMessage contains fields common to all protocol messages.
type BaseMessage struct {
	// Type identifies the message type (e.g., "status_request", "status_response").
	Type string `json:"type"`

	// RequestID is a unique identifier for request-response correlation.
	RequestID string `json:"request_id"`

	// Timestamp is when the message was created (RFC3339 format).
	Timestamp string `json:"timestamp,omitempty"`
}

// StatusRequest represents a status inquiry from Openexec to Gateway.
// It is used to request the current operational status of the Gateway.
type StatusRequest struct {
	BaseMessage

	// IncludeMetrics, if true, requests additional performance metrics.
	IncludeMetrics bool `json:"include_metrics,omitempty"`

	// IncludeConnections, if true, requests connection details.
	IncludeConnections bool `json:"include_connections,omitempty"`
}

// StatusCode represents the operational status of the Gateway.
type StatusCode string

const (
	// StatusOK indicates the gateway is operating normally.
	StatusOK StatusCode = "ok"

	// StatusDegraded indicates reduced functionality.
	StatusDegraded StatusCode = "degraded"

	// StatusError indicates an error state.
	StatusError StatusCode = "error"
)

// ConnectionInfo holds information about connected clients.
type ConnectionInfo struct {
	// TotalConnections is the number of active WebSocket connections.
	TotalConnections int `json:"total_connections"`

	// AuthenticatedConnections is the number of authenticated connections.
	AuthenticatedConnections int `json:"authenticated_connections,omitempty"`

	// ConnectionsByProject maps project IDs to connection counts.
	ConnectionsByProject map[string]int `json:"connections_by_project,omitempty"`
}

// Metrics holds performance metrics for the Gateway.
type Metrics struct {
	// UptimeSeconds is how long the gateway has been running.
	UptimeSeconds int64 `json:"uptime_seconds"`

	// MessagesReceived is the total count of messages received.
	MessagesReceived int64 `json:"messages_received,omitempty"`

	// MessagesSent is the total count of messages sent.
	MessagesSent int64 `json:"messages_sent,omitempty"`

	// MemoryUsageBytes is the current memory usage in bytes.
	MemoryUsageBytes int64 `json:"memory_usage_bytes,omitempty"`
}

// StatusResponse represents the Gateway's response to a status request.
type StatusResponse struct {
	BaseMessage

	// Status indicates the operational status (ok, degraded, error).
	Status StatusCode `json:"status"`

	// Message provides a human-readable status description.
	Message string `json:"message,omitempty"`

	// Version is the Gateway software version.
	Version string `json:"version,omitempty"`

	// Connections contains connection information if requested.
	Connections *ConnectionInfo `json:"connections,omitempty"`

	// Metrics contains performance metrics if requested.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// NewStatusRequest creates a new status request message.
func NewStatusRequest(requestID string) *StatusRequest {
	return &StatusRequest{
		BaseMessage: BaseMessage{
			Type:      TypeStatusRequest,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// NewStatusRequestWithOptions creates a status request with optional data flags.
func NewStatusRequestWithOptions(requestID string, includeMetrics, includeConnections bool) *StatusRequest {
	req := NewStatusRequest(requestID)
	req.IncludeMetrics = includeMetrics
	req.IncludeConnections = includeConnections
	return req
}

// NewStatusResponse creates a new status response message.
func NewStatusResponse(requestID string, status StatusCode, message string) *StatusResponse {
	return &StatusResponse{
		BaseMessage: BaseMessage{
			Type:      TypeStatusResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		Status:  status,
		Message: message,
	}
}

// Validate validates the status request message.
func (r *StatusRequest) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeStatusRequest {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	return nil
}

// Validate validates the status response message.
func (r *StatusResponse) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeStatusResponse {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.Status == "" {
		return ErrInvalidStatusCode
	}
	// Validate status code is one of the known values
	switch r.Status {
	case StatusOK, StatusDegraded, StatusError:
		// Valid status
	default:
		return ErrInvalidStatusCode
	}
	return nil
}

// MarshalJSON implements json.Marshaler for StatusRequest.
func (r *StatusRequest) MarshalJSON() ([]byte, error) {
	type Alias StatusRequest
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for StatusRequest.
func (r *StatusRequest) UnmarshalJSON(data []byte) error {
	type Alias StatusRequest
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for StatusResponse.
func (r *StatusResponse) MarshalJSON() ([]byte, error) {
	type Alias StatusResponse
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for StatusResponse.
func (r *StatusResponse) UnmarshalJSON(data []byte) error {
	type Alias StatusResponse
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// ParseMessage parses a raw JSON message and returns the appropriate type.
// It returns the parsed message and the message type string.
func ParseMessage(data []byte) (interface{}, string, error) {
	// First, extract just the type field
	var base BaseMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return nil, "", ErrInvalidMessage
	}

	if base.Type == "" {
		return nil, "", ErrMissingType
	}

	switch base.Type {
	case TypeStatusRequest:
		var req StatusRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &req, base.Type, nil

	case TypeStatusResponse:
		var resp StatusResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &resp, base.Type, nil

	case TypeRunRequest:
		var req RunRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &req, base.Type, nil

	case TypeRunResponse:
		var resp RunResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &resp, base.Type, nil

	case TypeLogsRequest:
		var req LogsRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &req, base.Type, nil

	case TypeLogsResponse:
		var resp LogsResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &resp, base.Type, nil

	case TypeCancelRequest:
		var req CancelRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &req, base.Type, nil

	case TypeCancelResponse:
		var resp CancelResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &resp, base.Type, nil

	case TypeCreateTaskRequest:
		var req CreateTaskRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &req, base.Type, nil

	case TypeCreateTaskResponse:
		var resp CreateTaskResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &resp, base.Type, nil

	case TypeTaskComplete:
		var event TaskCompleteEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &event, base.Type, nil

	case TypeDeployRequest:
		var req DeployRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &req, base.Type, nil

	case TypeDeployResponse:
		var resp DeployResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &resp, base.Type, nil

	case TypeAlert:
		var event AlertEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &event, base.Type, nil

	case TypeAudioStreamStart:
		var msg AudioStreamStart
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &msg, base.Type, nil

	case TypeAudioStreamStartAck:
		var msg AudioStreamStartAck
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &msg, base.Type, nil

	case TypeAudioStreamChunk:
		var msg AudioStreamChunk
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &msg, base.Type, nil

	case TypeAudioStreamEnd:
		var msg AudioStreamEnd
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &msg, base.Type, nil

	case TypeAudioStreamEndAck:
		var msg AudioStreamEndAck
		if err := json.Unmarshal(data, &msg); err != nil {
			return nil, base.Type, ErrInvalidMessage
		}
		return &msg, base.Type, nil

	default:
		return nil, base.Type, ErrUnknownType
	}
}
