package protocol

import (
	"encoding/json"
	"testing"
)

func TestNewAlertEvent(t *testing.T) {
	event := NewAlertEvent("alert-123", AlertTypeLongRunning, AlertSeverityWarning, "Task running long")

	if event.Type != TypeAlert {
		t.Errorf("Expected type '%s', got '%s'", TypeAlert, event.Type)
	}

	if event.AlertID != "alert-123" {
		t.Errorf("Expected alert ID 'alert-123', got '%s'", event.AlertID)
	}

	if event.AlertType != AlertTypeLongRunning {
		t.Errorf("Expected alert type '%s', got '%s'", AlertTypeLongRunning, event.AlertType)
	}

	if event.Severity != AlertSeverityWarning {
		t.Errorf("Expected severity '%s', got '%s'", AlertSeverityWarning, event.Severity)
	}

	if event.Title != "Task running long" {
		t.Errorf("Expected title 'Task running long', got '%s'", event.Title)
	}

	if event.RequestID != "alert-123" {
		t.Errorf("Expected request ID to match alert ID 'alert-123', got '%s'", event.RequestID)
	}

	if event.Timestamp == "" {
		t.Error("Expected timestamp to be set")
	}

	if event.CreatedAt == "" {
		t.Error("Expected created_at to be set")
	}
}

func TestNewLongRunningAlertEvent(t *testing.T) {
	event := NewLongRunningAlertEvent("alert-456", "task-789", "project-abc", 300.0, 120.0)

	if event.AlertID != "alert-456" {
		t.Errorf("Expected alert ID 'alert-456', got '%s'", event.AlertID)
	}

	if event.AlertType != AlertTypeLongRunning {
		t.Errorf("Expected alert type '%s', got '%s'", AlertTypeLongRunning, event.AlertType)
	}

	if event.Severity != AlertSeverityWarning {
		t.Errorf("Expected severity '%s', got '%s'", AlertSeverityWarning, event.Severity)
	}

	if event.TaskID != "task-789" {
		t.Errorf("Expected task ID 'task-789', got '%s'", event.TaskID)
	}

	if event.ProjectID != "project-abc" {
		t.Errorf("Expected project ID 'project-abc', got '%s'", event.ProjectID)
	}

	if event.Duration != 300.0 {
		t.Errorf("Expected duration 300.0, got %f", event.Duration)
	}

	if event.Threshold != 120.0 {
		t.Errorf("Expected threshold 120.0, got %f", event.Threshold)
	}

	if event.CurrentValue != 300.0 {
		t.Errorf("Expected current value 300.0, got %f", event.CurrentValue)
	}

	if event.Message == "" {
		t.Error("Expected message to be set")
	}
}

func TestAlertEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		event   *AlertEvent
		wantErr error
	}{
		{
			name:    "valid long running alert",
			event:   NewAlertEvent("alert-1", AlertTypeLongRunning, AlertSeverityWarning, "Task slow"),
			wantErr: nil,
		},
		{
			name:    "valid stalled alert",
			event:   NewAlertEvent("alert-2", AlertTypeStalled, AlertSeverityCritical, "Task stalled"),
			wantErr: nil,
		},
		{
			name:    "valid resource limit alert",
			event:   NewAlertEvent("alert-3", AlertTypeResourceLimit, AlertSeverityInfo, "Memory high"),
			wantErr: nil,
		},
		{
			name:    "valid error alert",
			event:   NewAlertEvent("alert-4", AlertTypeError, AlertSeverityCritical, "Error occurred"),
			wantErr: nil,
		},
		{
			name: "missing type",
			event: &AlertEvent{
				BaseMessage: BaseMessage{RequestID: "req-1"},
				AlertID:     "alert-1",
				AlertType:   AlertTypeLongRunning,
				Severity:    AlertSeverityWarning,
				Title:       "Test",
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: "wrong_type", RequestID: "req-1"},
				AlertID:     "alert-1",
				AlertType:   AlertTypeLongRunning,
				Severity:    AlertSeverityWarning,
				Title:       "Test",
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing alert ID",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: TypeAlert, RequestID: "req-1"},
				AlertType:   AlertTypeLongRunning,
				Severity:    AlertSeverityWarning,
				Title:       "Test",
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing alert type",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: TypeAlert, RequestID: "req-1"},
				AlertID:     "alert-1",
				Severity:    AlertSeverityWarning,
				Title:       "Test",
			},
			wantErr: ErrInvalidAlertType,
		},
		{
			name: "invalid alert type",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: TypeAlert, RequestID: "req-1"},
				AlertID:     "alert-1",
				AlertType:   AlertType("invalid"),
				Severity:    AlertSeverityWarning,
				Title:       "Test",
			},
			wantErr: ErrInvalidAlertType,
		},
		{
			name: "missing severity",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: TypeAlert, RequestID: "req-1"},
				AlertID:     "alert-1",
				AlertType:   AlertTypeLongRunning,
				Title:       "Test",
			},
			wantErr: ErrInvalidAlertSeverity,
		},
		{
			name: "invalid severity",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: TypeAlert, RequestID: "req-1"},
				AlertID:     "alert-1",
				AlertType:   AlertTypeLongRunning,
				Severity:    AlertSeverity("invalid"),
				Title:       "Test",
			},
			wantErr: ErrInvalidAlertSeverity,
		},
		{
			name: "missing title",
			event: &AlertEvent{
				BaseMessage: BaseMessage{Type: TypeAlert, RequestID: "req-1"},
				AlertID:     "alert-1",
				AlertType:   AlertTypeLongRunning,
				Severity:    AlertSeverityWarning,
			},
			wantErr: ErrMissingRequestID, // Reused for missing required field
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAlertEventIsLongRunning(t *testing.T) {
	tests := []struct {
		alertType AlertType
		want      bool
	}{
		{AlertTypeLongRunning, true},
		{AlertTypeStalled, false},
		{AlertTypeResourceLimit, false},
		{AlertTypeError, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.alertType), func(t *testing.T) {
			event := NewAlertEvent("alert-1", tt.alertType, AlertSeverityWarning, "test")
			if got := event.IsLongRunning(); got != tt.want {
				t.Errorf("IsLongRunning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertEventIsCritical(t *testing.T) {
	tests := []struct {
		severity AlertSeverity
		want     bool
	}{
		{AlertSeverityInfo, false},
		{AlertSeverityWarning, false},
		{AlertSeverityCritical, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			event := NewAlertEvent("alert-1", AlertTypeLongRunning, tt.severity, "test")
			if got := event.IsCritical(); got != tt.want {
				t.Errorf("IsCritical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertEventIsWarning(t *testing.T) {
	tests := []struct {
		severity AlertSeverity
		want     bool
	}{
		{AlertSeverityInfo, false},
		{AlertSeverityWarning, true},
		{AlertSeverityCritical, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			event := NewAlertEvent("alert-1", AlertTypeLongRunning, tt.severity, "test")
			if got := event.IsWarning(); got != tt.want {
				t.Errorf("IsWarning() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAlertEventMarshalJSON(t *testing.T) {
	event := &AlertEvent{
		BaseMessage: BaseMessage{
			Type:      TypeAlert,
			RequestID: "alert-789",
			Timestamp: "2024-01-15T10:00:00Z",
		},
		AlertID:      "alert-789",
		AlertType:    AlertTypeLongRunning,
		Severity:     AlertSeverityWarning,
		TaskID:       "task-abc",
		ProjectID:    "proj-xyz",
		Title:        "Long running task",
		Message:      "Task exceeded threshold",
		Duration:     300.5,
		Threshold:    120.0,
		CurrentValue: 300.5,
		CreatedAt:    "2024-01-15T10:00:00Z",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal and verify
	var decoded AlertEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != TypeAlert {
		t.Errorf("Type mismatch: got %s", decoded.Type)
	}

	if decoded.AlertID != "alert-789" {
		t.Errorf("AlertID mismatch: got %s", decoded.AlertID)
	}

	if decoded.AlertType != AlertTypeLongRunning {
		t.Errorf("AlertType mismatch: got %s", decoded.AlertType)
	}

	if decoded.Severity != AlertSeverityWarning {
		t.Errorf("Severity mismatch: got %s", decoded.Severity)
	}

	if decoded.TaskID != "task-abc" {
		t.Errorf("TaskID mismatch: got %s", decoded.TaskID)
	}

	if decoded.ProjectID != "proj-xyz" {
		t.Errorf("ProjectID mismatch: got %s", decoded.ProjectID)
	}

	if decoded.Duration != 300.5 {
		t.Errorf("Duration mismatch: got %f", decoded.Duration)
	}

	if decoded.Threshold != 120.0 {
		t.Errorf("Threshold mismatch: got %f", decoded.Threshold)
	}
}

func TestAlertEventUnmarshalJSON(t *testing.T) {
	jsonData := `{
		"type": "alert",
		"request_id": "alert-abc",
		"timestamp": "2024-01-15T10:00:00Z",
		"alert_id": "alert-abc",
		"alert_type": "long_running",
		"severity": "warning",
		"task_id": "task-xyz",
		"project_id": "proj-123",
		"title": "Task slow",
		"message": "Running too long",
		"duration": 250.5,
		"threshold": 120.0,
		"metadata": {"key": "value"}
	}`

	var event AlertEvent
	if err := json.Unmarshal([]byte(jsonData), &event); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if event.Type != TypeAlert {
		t.Errorf("Expected type '%s', got '%s'", TypeAlert, event.Type)
	}

	if event.AlertID != "alert-abc" {
		t.Errorf("Expected alert ID 'alert-abc', got '%s'", event.AlertID)
	}

	if event.AlertType != AlertTypeLongRunning {
		t.Errorf("Expected alert type '%s', got '%s'", AlertTypeLongRunning, event.AlertType)
	}

	if event.Severity != AlertSeverityWarning {
		t.Errorf("Expected severity '%s', got '%s'", AlertSeverityWarning, event.Severity)
	}

	if event.TaskID != "task-xyz" {
		t.Errorf("Expected task ID 'task-xyz', got '%s'", event.TaskID)
	}

	if event.ProjectID != "proj-123" {
		t.Errorf("Expected project ID 'proj-123', got '%s'", event.ProjectID)
	}

	if event.Duration != 250.5 {
		t.Errorf("Expected duration 250.5, got %f", event.Duration)
	}

	if event.Metadata == nil {
		t.Error("Expected metadata to be set")
	} else if event.Metadata["key"] != "value" {
		t.Errorf("Expected metadata key 'key' to have value 'value', got %v", event.Metadata["key"])
	}
}

func TestAlertEventUnmarshalInvalidJSON(t *testing.T) {
	var event AlertEvent
	err := json.Unmarshal([]byte("invalid json"), &event)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParseMessageAlert(t *testing.T) {
	jsonData := `{
		"type": "alert",
		"request_id": "alert-parse-test",
		"alert_id": "alert-parse-test",
		"alert_type": "long_running",
		"severity": "warning",
		"title": "Test alert"
	}`

	parsed, msgType, err := ParseMessage([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}

	if msgType != TypeAlert {
		t.Errorf("Expected message type '%s', got '%s'", TypeAlert, msgType)
	}

	event, ok := parsed.(*AlertEvent)
	if !ok {
		t.Fatalf("Expected *AlertEvent, got %T", parsed)
	}

	if event.AlertID != "alert-parse-test" {
		t.Errorf("Expected alert ID 'alert-parse-test', got '%s'", event.AlertID)
	}

	if event.AlertType != AlertTypeLongRunning {
		t.Errorf("Expected alert type '%s', got '%s'", AlertTypeLongRunning, event.AlertType)
	}
}
