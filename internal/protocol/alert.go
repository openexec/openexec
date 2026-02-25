// Package protocol defines JSON protocol messages for communication between Gateway and Openexec.
package protocol

import (
	"encoding/json"
	"errors"
	"time"
)

// Alert message type constant
const (
	// TypeAlert is the type identifier for alert event messages.
	TypeAlert = "alert"
)

// Errors specific to alert handling.
var (
	ErrInvalidAlertType     = errors.New("invalid alert type")
	ErrInvalidAlertSeverity = errors.New("invalid alert severity")
)

// AlertType represents the category of alert.
type AlertType string

const (
	// AlertTypeLongRunning indicates a task has been running longer than expected.
	AlertTypeLongRunning AlertType = "long_running"

	// AlertTypeStalled indicates a task appears to be stalled or unresponsive.
	AlertTypeStalled AlertType = "stalled"

	// AlertTypeResourceLimit indicates a resource limit is being approached.
	AlertTypeResourceLimit AlertType = "resource_limit"

	// AlertTypeError indicates an error condition that requires attention.
	AlertTypeError AlertType = "error"
)

// AlertSeverity represents the urgency level of an alert.
type AlertSeverity string

const (
	// AlertSeverityInfo is for informational alerts.
	AlertSeverityInfo AlertSeverity = "info"

	// AlertSeverityWarning is for alerts that may require attention.
	AlertSeverityWarning AlertSeverity = "warning"

	// AlertSeverityCritical is for urgent alerts requiring immediate action.
	AlertSeverityCritical AlertSeverity = "critical"
)

// AlertEvent represents an alert event initiated by OpenExec.
// It is sent to the gateway when conditions requiring user attention are detected.
type AlertEvent struct {
	BaseMessage

	// AlertID is the unique identifier of this alert.
	AlertID string `json:"alert_id"`

	// AlertType categorizes the alert (e.g., "long_running", "stalled").
	AlertType AlertType `json:"alert_type"`

	// Severity indicates the urgency level.
	Severity AlertSeverity `json:"severity"`

	// TaskID is the task associated with this alert (if applicable).
	TaskID string `json:"task_id,omitempty"`

	// ProjectID identifies which project this alert relates to.
	ProjectID string `json:"project_id,omitempty"`

	// Title is a short summary of the alert.
	Title string `json:"title"`

	// Message provides detailed description of the alert.
	Message string `json:"message,omitempty"`

	// Duration is how long the condition has persisted (in seconds).
	Duration float64 `json:"duration,omitempty"`

	// Threshold is the threshold that was exceeded (if applicable).
	Threshold float64 `json:"threshold,omitempty"`

	// CurrentValue is the current value that triggered the alert.
	CurrentValue float64 `json:"current_value,omitempty"`

	// CreatedAt is when the alert was created (RFC3339 format).
	CreatedAt string `json:"created_at,omitempty"`

	// Metadata contains additional structured data about the alert.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewAlertEvent creates a new alert event message.
func NewAlertEvent(alertID string, alertType AlertType, severity AlertSeverity, title string) *AlertEvent {
	return &AlertEvent{
		BaseMessage: BaseMessage{
			Type:      TypeAlert,
			RequestID: alertID, // Use alert ID as request ID for correlation
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		AlertID:   alertID,
		AlertType: alertType,
		Severity:  severity,
		Title:     title,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// NewLongRunningAlertEvent creates an alert for a long-running task.
func NewLongRunningAlertEvent(alertID, taskID, projectID string, duration, threshold float64) *AlertEvent {
	event := NewAlertEvent(alertID, AlertTypeLongRunning, AlertSeverityWarning, "Task running longer than expected")
	event.TaskID = taskID
	event.ProjectID = projectID
	event.Duration = duration
	event.Threshold = threshold
	event.CurrentValue = duration
	event.Message = "Task has exceeded the expected execution time threshold"
	return event
}

// Validate validates the alert event message.
func (e *AlertEvent) Validate() error {
	if e.Type == "" {
		return ErrMissingType
	}
	if e.Type != TypeAlert {
		return ErrUnknownType
	}
	if e.AlertID == "" {
		return ErrMissingRequestID
	}
	if e.AlertType == "" {
		return ErrInvalidAlertType
	}
	// Validate alert type is one of the known values
	switch e.AlertType {
	case AlertTypeLongRunning, AlertTypeStalled, AlertTypeResourceLimit, AlertTypeError:
		// Valid type
	default:
		return ErrInvalidAlertType
	}
	if e.Severity == "" {
		return ErrInvalidAlertSeverity
	}
	// Validate severity is one of the known values
	switch e.Severity {
	case AlertSeverityInfo, AlertSeverityWarning, AlertSeverityCritical:
		// Valid severity
	default:
		return ErrInvalidAlertSeverity
	}
	if e.Title == "" {
		return ErrMissingRequestID // Reuse for missing required field
	}
	return nil
}

// IsLongRunning returns true if this is a long-running task alert.
func (e *AlertEvent) IsLongRunning() bool {
	return e.AlertType == AlertTypeLongRunning
}

// IsCritical returns true if the alert severity is critical.
func (e *AlertEvent) IsCritical() bool {
	return e.Severity == AlertSeverityCritical
}

// IsWarning returns true if the alert severity is warning or higher.
func (e *AlertEvent) IsWarning() bool {
	return e.Severity == AlertSeverityWarning || e.Severity == AlertSeverityCritical
}

// MarshalJSON implements json.Marshaler for AlertEvent.
func (e *AlertEvent) MarshalJSON() ([]byte, error) {
	type Alias AlertEvent
	return json.Marshal((*Alias)(e))
}

// UnmarshalJSON implements json.Unmarshaler for AlertEvent.
func (e *AlertEvent) UnmarshalJSON(data []byte) error {
	type Alias AlertEvent
	aux := (*Alias)(e)
	if err := json.Unmarshal(data, aux); err != nil {
		return ErrInvalidMessage
	}
	return nil
}
