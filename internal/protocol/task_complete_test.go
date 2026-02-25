package protocol

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewTaskCompleteEvent(t *testing.T) {
	event := NewTaskCompleteEvent("task-123", TaskCompleteStatusSuccess, "Task completed successfully")

	if event.Type != TypeTaskComplete {
		t.Errorf("Expected type '%s', got '%s'", TypeTaskComplete, event.Type)
	}

	if event.TaskID != "task-123" {
		t.Errorf("Expected task ID 'task-123', got '%s'", event.TaskID)
	}

	if event.Status != TaskCompleteStatusSuccess {
		t.Errorf("Expected status '%s', got '%s'", TaskCompleteStatusSuccess, event.Status)
	}

	if event.Message != "Task completed successfully" {
		t.Errorf("Expected message 'Task completed successfully', got '%s'", event.Message)
	}

	if event.RequestID != "task-123" {
		t.Errorf("Expected request ID to match task ID 'task-123', got '%s'", event.RequestID)
	}

	if event.Timestamp == "" {
		t.Error("Expected timestamp to be set")
	}

	if event.CompletedAt == "" {
		t.Error("Expected completed_at to be set")
	}
}

func TestNewTaskCompleteEventWithDetails(t *testing.T) {
	event := NewTaskCompleteEventWithDetails("task-456", TaskCompleteStatusFailure, "Task failed", "project-abc", 120.5)

	if event.TaskID != "task-456" {
		t.Errorf("Expected task ID 'task-456', got '%s'", event.TaskID)
	}

	if event.Status != TaskCompleteStatusFailure {
		t.Errorf("Expected status '%s', got '%s'", TaskCompleteStatusFailure, event.Status)
	}

	if event.ProjectID != "project-abc" {
		t.Errorf("Expected project ID 'project-abc', got '%s'", event.ProjectID)
	}

	if event.Duration != 120.5 {
		t.Errorf("Expected duration 120.5, got %f", event.Duration)
	}
}

func TestTaskCompleteEventValidate(t *testing.T) {
	tests := []struct {
		name    string
		event   *TaskCompleteEvent
		wantErr error
	}{
		{
			name:    "valid success event",
			event:   NewTaskCompleteEvent("task-1", TaskCompleteStatusSuccess, "Done"),
			wantErr: nil,
		},
		{
			name:    "valid failure event",
			event:   NewTaskCompleteEvent("task-2", TaskCompleteStatusFailure, "Failed"),
			wantErr: nil,
		},
		{
			name:    "valid cancelled event",
			event:   NewTaskCompleteEvent("task-3", TaskCompleteStatusCancelled, "Cancelled"),
			wantErr: nil,
		},
		{
			name:    "valid timeout event",
			event:   NewTaskCompleteEvent("task-4", TaskCompleteStatusTimeout, "Timed out"),
			wantErr: nil,
		},
		{
			name: "missing type",
			event: &TaskCompleteEvent{
				BaseMessage: BaseMessage{RequestID: "req-1"},
				TaskID:      "task-1",
				Status:      TaskCompleteStatusSuccess,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			event: &TaskCompleteEvent{
				BaseMessage: BaseMessage{Type: "wrong_type", RequestID: "req-1"},
				TaskID:      "task-1",
				Status:      TaskCompleteStatusSuccess,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing task ID",
			event: &TaskCompleteEvent{
				BaseMessage: BaseMessage{Type: TypeTaskComplete, RequestID: "req-1"},
				Status:      TaskCompleteStatusSuccess,
			},
			wantErr: ErrMissingTaskID,
		},
		{
			name: "missing status",
			event: &TaskCompleteEvent{
				BaseMessage: BaseMessage{Type: TypeTaskComplete, RequestID: "req-1"},
				TaskID:      "task-1",
			},
			wantErr: ErrInvalidTaskCompleteStatus,
		},
		{
			name: "invalid status",
			event: &TaskCompleteEvent{
				BaseMessage: BaseMessage{Type: TypeTaskComplete, RequestID: "req-1"},
				TaskID:      "task-1",
				Status:      TaskCompleteStatus("invalid"),
			},
			wantErr: ErrInvalidTaskCompleteStatus,
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

func TestTaskCompleteEventIsSuccess(t *testing.T) {
	tests := []struct {
		status TaskCompleteStatus
		want   bool
	}{
		{TaskCompleteStatusSuccess, true},
		{TaskCompleteStatusFailure, false},
		{TaskCompleteStatusCancelled, false},
		{TaskCompleteStatusTimeout, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			event := NewTaskCompleteEvent("task-1", tt.status, "test")
			if got := event.IsSuccess(); got != tt.want {
				t.Errorf("IsSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskCompleteEventIsFailure(t *testing.T) {
	tests := []struct {
		status TaskCompleteStatus
		want   bool
	}{
		{TaskCompleteStatusSuccess, false},
		{TaskCompleteStatusFailure, true},
		{TaskCompleteStatusCancelled, true},
		{TaskCompleteStatusTimeout, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			event := NewTaskCompleteEvent("task-1", tt.status, "test")
			if got := event.IsFailure(); got != tt.want {
				t.Errorf("IsFailure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTaskCompleteEventMarshalJSON(t *testing.T) {
	exitCode := 1
	event := &TaskCompleteEvent{
		BaseMessage: BaseMessage{
			Type:      TypeTaskComplete,
			RequestID: "task-789",
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
		TaskID:      "task-789",
		Status:      TaskCompleteStatusFailure,
		Message:     "Command failed",
		ProjectID:   "proj-abc",
		ExitCode:    &exitCode,
		Error:       "exit status 1",
		Duration:    45.2,
		StartedAt:   "2024-01-15T10:00:00Z",
		CompletedAt: "2024-01-15T10:00:45Z",
		Output:      "Some output text",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	// Unmarshal and verify
	var decoded TaskCompleteEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Type != TypeTaskComplete {
		t.Errorf("Type mismatch: got %s", decoded.Type)
	}

	if decoded.TaskID != "task-789" {
		t.Errorf("TaskID mismatch: got %s", decoded.TaskID)
	}

	if decoded.Status != TaskCompleteStatusFailure {
		t.Errorf("Status mismatch: got %s", decoded.Status)
	}

	if decoded.ProjectID != "proj-abc" {
		t.Errorf("ProjectID mismatch: got %s", decoded.ProjectID)
	}

	if decoded.ExitCode == nil || *decoded.ExitCode != 1 {
		t.Error("ExitCode mismatch")
	}

	if decoded.Duration != 45.2 {
		t.Errorf("Duration mismatch: got %f", decoded.Duration)
	}

	if decoded.Output != "Some output text" {
		t.Errorf("Output mismatch: got %s", decoded.Output)
	}
}

func TestTaskCompleteEventUnmarshalJSON(t *testing.T) {
	jsonData := `{
		"type": "task_complete",
		"request_id": "task-abc",
		"timestamp": "2024-01-15T10:00:00Z",
		"task_id": "task-abc",
		"status": "success",
		"message": "Task completed",
		"project_id": "proj-xyz",
		"duration": 30.5,
		"metadata": {"key": "value"}
	}`

	var event TaskCompleteEvent
	if err := json.Unmarshal([]byte(jsonData), &event); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if event.Type != TypeTaskComplete {
		t.Errorf("Expected type '%s', got '%s'", TypeTaskComplete, event.Type)
	}

	if event.TaskID != "task-abc" {
		t.Errorf("Expected task ID 'task-abc', got '%s'", event.TaskID)
	}

	if event.Status != TaskCompleteStatusSuccess {
		t.Errorf("Expected status '%s', got '%s'", TaskCompleteStatusSuccess, event.Status)
	}

	if event.ProjectID != "proj-xyz" {
		t.Errorf("Expected project ID 'proj-xyz', got '%s'", event.ProjectID)
	}

	if event.Duration != 30.5 {
		t.Errorf("Expected duration 30.5, got %f", event.Duration)
	}

	if event.Metadata == nil {
		t.Error("Expected metadata to be set")
	} else if event.Metadata["key"] != "value" {
		t.Errorf("Expected metadata key 'key' to have value 'value', got %v", event.Metadata["key"])
	}
}

func TestTaskCompleteEventUnmarshalInvalidJSON(t *testing.T) {
	var event TaskCompleteEvent
	err := json.Unmarshal([]byte("invalid json"), &event)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestParseMessageTaskComplete(t *testing.T) {
	jsonData := `{
		"type": "task_complete",
		"request_id": "task-parse-test",
		"task_id": "task-parse-test",
		"status": "success",
		"message": "Done"
	}`

	parsed, msgType, err := ParseMessage([]byte(jsonData))
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}

	if msgType != TypeTaskComplete {
		t.Errorf("Expected message type '%s', got '%s'", TypeTaskComplete, msgType)
	}

	event, ok := parsed.(*TaskCompleteEvent)
	if !ok {
		t.Fatalf("Expected *TaskCompleteEvent, got %T", parsed)
	}

	if event.TaskID != "task-parse-test" {
		t.Errorf("Expected task ID 'task-parse-test', got '%s'", event.TaskID)
	}

	if event.Status != TaskCompleteStatusSuccess {
		t.Errorf("Expected status '%s', got '%s'", TaskCompleteStatusSuccess, event.Status)
	}
}
