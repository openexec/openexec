// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Logs message type constants
const (
	// TypeLogsRequest is the type identifier for logs request messages.
	TypeLogsRequest = "logs_request"

	// TypeLogsResponse is the type identifier for logs response messages.
	TypeLogsResponse = "logs_response"
)

// Errors specific to logs handling.
var (
	ErrInvalidLogLevel = errors.New("invalid log level")
	ErrInvalidLimit    = errors.New("invalid limit value")
)

// LogLevel represents the severity level of a log entry.
type LogLevel string

const (
	// LogLevelDebug is for debug-level messages.
	LogLevelDebug LogLevel = "debug"

	// LogLevelInfo is for informational messages.
	LogLevelInfo LogLevel = "info"

	// LogLevelWarn is for warning messages.
	LogLevelWarn LogLevel = "warn"

	// LogLevelError is for error messages.
	LogLevelError LogLevel = "error"
)

// LogsRequest represents a request to retrieve task logs.
// It is used to request logs for a specific task from the Gateway.
type LogsRequest struct {
	BaseMessage

	// TaskID is the unique identifier of the task whose logs are requested.
	TaskID string `json:"task_id"`

	// Level filters logs by minimum severity level (optional).
	// If not specified, all levels are returned.
	Level LogLevel `json:"level,omitempty"`

	// Since filters logs to entries after this timestamp (RFC3339 format, optional).
	Since string `json:"since,omitempty"`

	// Until filters logs to entries before this timestamp (RFC3339 format, optional).
	Until string `json:"until,omitempty"`

	// Limit is the maximum number of log entries to return (optional).
	// If not specified or zero, a default limit is applied by the server.
	Limit int `json:"limit,omitempty"`

	// Follow, if true, requests a streaming response for real-time logs.
	Follow bool `json:"follow,omitempty"`
}

// LogEntry represents a single log entry.
type LogEntry struct {
	// Timestamp is when the log entry was created (RFC3339 format).
	Timestamp string `json:"timestamp"`

	// Level is the severity level of the log entry.
	Level LogLevel `json:"level"`

	// Message is the log message content.
	Message string `json:"message"`

	// Source is the origin of the log entry (e.g., "agent", "system", "user").
	Source string `json:"source,omitempty"`

	// Metadata contains additional structured data associated with the log entry.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// LogsResponse represents the Gateway's response to a logs request.
type LogsResponse struct {
	BaseMessage

	// TaskID is the unique identifier of the task.
	TaskID string `json:"task_id"`

	// Entries contains the requested log entries.
	Entries []LogEntry `json:"entries"`

	// HasMore indicates if there are more log entries available beyond the limit.
	HasMore bool `json:"has_more,omitempty"`

	// NextCursor is a pagination cursor for fetching the next batch of logs.
	NextCursor string `json:"next_cursor,omitempty"`

	// Error contains error details if the request failed.
	Error string `json:"error,omitempty"`
}

// NewLogsRequest creates a new logs request message.
func NewLogsRequest(requestID, taskID string) *LogsRequest {
	return &LogsRequest{
		BaseMessage: BaseMessage{
			Type:      TypeLogsRequest,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID: taskID,
	}
}

// NewLogsRequestWithOptions creates a logs request with filtering options.
func NewLogsRequestWithOptions(requestID, taskID string, level LogLevel, limit int) *LogsRequest {
	req := NewLogsRequest(requestID, taskID)
	req.Level = level
	req.Limit = limit
	return req
}

// NewLogsResponse creates a new logs response message.
func NewLogsResponse(requestID, taskID string, entries []LogEntry) *LogsResponse {
	return &LogsResponse{
		BaseMessage: BaseMessage{
			Type:      TypeLogsResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:  taskID,
		Entries: entries,
	}
}

// NewLogsErrorResponse creates a new logs response with an error.
func NewLogsErrorResponse(requestID, taskID string, errorMsg string) *LogsResponse {
	return &LogsResponse{
		BaseMessage: BaseMessage{
			Type:      TypeLogsResponse,
			RequestID: requestID,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:  taskID,
		Entries: []LogEntry{},
		Error:   errorMsg,
	}
}

// NewLogEntry creates a new log entry with the current timestamp.
func NewLogEntry(level LogLevel, message string) LogEntry {
	return LogEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Level:     level,
		Message:   message,
	}
}

// Validate validates the logs request message.
func (r *LogsRequest) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeLogsRequest {
		return ErrUnknownType
	}
	if r.RequestID == "" {
		return ErrMissingRequestID
	}
	if r.TaskID == "" {
		return ErrMissingTaskID
	}
	// Validate log level if specified
	if r.Level != "" {
		switch r.Level {
		case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
			// Valid level
		default:
			return ErrInvalidLogLevel
		}
	}
	// Validate limit if specified
	if r.Limit < 0 {
		return ErrInvalidLimit
	}
	return nil
}

// Validate validates the logs response message.
func (r *LogsResponse) Validate() error {
	if r.Type == "" {
		return ErrMissingType
	}
	if r.Type != TypeLogsResponse {
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

// MarshalJSON implements json.Marshaler for LogsRequest.
func (r *LogsRequest) MarshalJSON() ([]byte, error) {
	type Alias LogsRequest
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for LogsRequest.
func (r *LogsRequest) UnmarshalJSON(data []byte) error {
	type Alias LogsRequest
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}

// MarshalJSON implements json.Marshaler for LogsResponse.
func (r *LogsResponse) MarshalJSON() ([]byte, error) {
	type Alias LogsResponse
	return json.Marshal((*Alias)(r))
}

// UnmarshalJSON implements json.Unmarshaler for LogsResponse.
func (r *LogsResponse) UnmarshalJSON(data []byte) error {
	type Alias LogsResponse
	aux := (*Alias)(r)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
