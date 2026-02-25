package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewRunRequest(t *testing.T) {
	req := NewRunRequest("req-123", "task-456")

	if req.Type != TypeRunRequest {
		t.Errorf("expected type %s, got %s", TypeRunRequest, req.Type)
	}
	if req.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", req.RequestID)
	}
	if req.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", req.TaskID)
	}
	if req.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewRunResponse(t *testing.T) {
	resp := NewRunResponse("req-123", "task-456", RunStatusAccepted, "Task accepted")

	if resp.Type != TypeRunResponse {
		t.Errorf("expected type %s, got %s", TypeRunResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", resp.TaskID)
	}
	if resp.Status != RunStatusAccepted {
		t.Errorf("expected status %s, got %s", RunStatusAccepted, resp.Status)
	}
	if resp.Message != "Task accepted" {
		t.Errorf("expected message 'Task accepted', got %s", resp.Message)
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewRunErrorResponse(t *testing.T) {
	resp := NewRunErrorResponse("req-123", "task-456", RunStatusFailed, "Task not found")

	if resp.Type != TypeRunResponse {
		t.Errorf("expected type %s, got %s", TypeRunResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", resp.TaskID)
	}
	if resp.Status != RunStatusFailed {
		t.Errorf("expected status %s, got %s", RunStatusFailed, resp.Status)
	}
	if resp.Error != "Task not found" {
		t.Errorf("expected error 'Task not found', got %s", resp.Error)
	}
	if resp.Message != "" {
		t.Errorf("expected empty message, got %s", resp.Message)
	}
}

func TestRunRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     RunRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: RunRequest{
				BaseMessage: BaseMessage{
					Type:      TypeRunRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			req: RunRequest{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			req: RunRequest{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			req: RunRequest{
				BaseMessage: BaseMessage{
					Type: TypeRunRequest,
				},
				TaskID: "task-001",
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing task_id",
			req: RunRequest{
				BaseMessage: BaseMessage{
					Type:      TypeRunRequest,
					RequestID: "valid-id",
				},
			},
			wantErr: ErrMissingTaskID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunResponseValidation(t *testing.T) {
	tests := []struct {
		name    string
		resp    RunResponse
		wantErr error
	}{
		{
			name: "valid response with accepted status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusAccepted,
			},
			wantErr: nil,
		},
		{
			name: "valid response with running status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusRunning,
			},
			wantErr: nil,
		},
		{
			name: "valid response with completed status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusCompleted,
			},
			wantErr: nil,
		},
		{
			name: "valid response with failed status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusFailed,
			},
			wantErr: nil,
		},
		{
			name: "valid response with rejected status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusRejected,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusAccepted,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatusAccepted,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type: TypeRunResponse,
				},
				TaskID: "task-001",
				Status: RunStatusAccepted,
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing task_id",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				Status: RunStatusAccepted,
			},
			wantErr: ErrMissingTaskID,
		},
		{
			name: "missing status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrInvalidRunStatus,
		},
		{
			name: "invalid status",
			resp: RunResponse{
				BaseMessage: BaseMessage{
					Type:      TypeRunResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: RunStatus("invalid"),
			},
			wantErr: ErrInvalidRunStatus,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.resp.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRunRequestMarshalJSON(t *testing.T) {
	req := NewRunRequest("req-001", "task-001")

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"run_request"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"req-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
}

func TestRunRequestUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "run_request",
		"request_id": "req-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002"
	}`

	var req RunRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != TypeRunRequest {
		t.Errorf("expected type %s, got %s", TypeRunRequest, req.Type)
	}
	if req.RequestID != "req-002" {
		t.Errorf("expected request_id req-002, got %s", req.RequestID)
	}
	if req.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", req.TaskID)
	}
}

func TestRunResponseMarshalJSON(t *testing.T) {
	resp := NewRunResponse("resp-001", "task-001", RunStatusRunning, "Task is running")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"run_response"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"resp-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"running"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"message":"Task is running"`) {
		t.Errorf("expected message field in JSON, got %s", jsonStr)
	}
}

func TestRunResponseMarshalJSONWithError(t *testing.T) {
	resp := NewRunErrorResponse("resp-001", "task-001", RunStatusFailed, "Connection timeout")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"status":"failed"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"error":"Connection timeout"`) {
		t.Errorf("expected error field in JSON, got %s", jsonStr)
	}
}

func TestRunResponseUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "run_response",
		"request_id": "resp-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002",
		"status": "completed",
		"message": "Task completed successfully"
	}`

	var resp RunResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Type != TypeRunResponse {
		t.Errorf("expected type %s, got %s", TypeRunResponse, resp.Type)
	}
	if resp.RequestID != "resp-002" {
		t.Errorf("expected request_id resp-002, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", resp.TaskID)
	}
	if resp.Status != RunStatusCompleted {
		t.Errorf("expected status %s, got %s", RunStatusCompleted, resp.Status)
	}
	if resp.Message != "Task completed successfully" {
		t.Errorf("expected message 'Task completed successfully', got %s", resp.Message)
	}
}

func TestRunResponseUnmarshalJSONWithError(t *testing.T) {
	jsonStr := `{
		"type": "run_response",
		"request_id": "resp-003",
		"task_id": "task-003",
		"status": "rejected",
		"error": "Task already running"
	}`

	var resp RunResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Status != RunStatusRejected {
		t.Errorf("expected status %s, got %s", RunStatusRejected, resp.Status)
	}
	if resp.Error != "Task already running" {
		t.Errorf("expected error 'Task already running', got %s", resp.Error)
	}
}

func TestParseMessageRunRequest(t *testing.T) {
	input := `{"type": "run_request", "request_id": "req-001", "task_id": "task-001"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeRunRequest {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeRunRequest)
	}

	req, ok := result.(*RunRequest)
	if !ok {
		t.Fatal("expected *RunRequest")
	}
	if req.RequestID != "req-001" {
		t.Errorf("expected request_id req-001, got %s", req.RequestID)
	}
	if req.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", req.TaskID)
	}
}

func TestParseMessageRunResponse(t *testing.T) {
	input := `{"type": "run_response", "request_id": "resp-001", "task_id": "task-001", "status": "accepted"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeRunResponse {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeRunResponse)
	}

	resp, ok := result.(*RunResponse)
	if !ok {
		t.Fatal("expected *RunResponse")
	}
	if resp.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", resp.TaskID)
	}
	if resp.Status != RunStatusAccepted {
		t.Errorf("expected status accepted, got %s", resp.Status)
	}
}

func TestRunRequestRoundTrip(t *testing.T) {
	original := NewRunRequest("roundtrip-001", "task-roundtrip")
	original.Timestamp = "2024-01-15T10:00:00Z"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed RunRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Compare
	if parsed.Type != original.Type {
		t.Errorf("Type mismatch: %s != %s", parsed.Type, original.Type)
	}
	if parsed.RequestID != original.RequestID {
		t.Errorf("RequestID mismatch: %s != %s", parsed.RequestID, original.RequestID)
	}
	if parsed.Timestamp != original.Timestamp {
		t.Errorf("Timestamp mismatch: %s != %s", parsed.Timestamp, original.Timestamp)
	}
	if parsed.TaskID != original.TaskID {
		t.Errorf("TaskID mismatch: %s != %s", parsed.TaskID, original.TaskID)
	}
}

func TestRunResponseRoundTrip(t *testing.T) {
	original := NewRunResponse("roundtrip-002", "task-roundtrip", RunStatusCompleted, "Done")
	original.Timestamp = "2024-01-15T11:00:00Z"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed RunResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Compare
	if parsed.Type != original.Type {
		t.Errorf("Type mismatch: %s != %s", parsed.Type, original.Type)
	}
	if parsed.RequestID != original.RequestID {
		t.Errorf("RequestID mismatch: %s != %s", parsed.RequestID, original.RequestID)
	}
	if parsed.TaskID != original.TaskID {
		t.Errorf("TaskID mismatch: %s != %s", parsed.TaskID, original.TaskID)
	}
	if parsed.Status != original.Status {
		t.Errorf("Status mismatch: %s != %s", parsed.Status, original.Status)
	}
	if parsed.Message != original.Message {
		t.Errorf("Message mismatch: %s != %s", parsed.Message, original.Message)
	}
}

func TestRunStatusConstants(t *testing.T) {
	// Ensure run status codes have expected string values
	if RunStatusAccepted != "accepted" {
		t.Errorf("RunStatusAccepted should be 'accepted', got %s", RunStatusAccepted)
	}
	if RunStatusRunning != "running" {
		t.Errorf("RunStatusRunning should be 'running', got %s", RunStatusRunning)
	}
	if RunStatusCompleted != "completed" {
		t.Errorf("RunStatusCompleted should be 'completed', got %s", RunStatusCompleted)
	}
	if RunStatusFailed != "failed" {
		t.Errorf("RunStatusFailed should be 'failed', got %s", RunStatusFailed)
	}
	if RunStatusRejected != "rejected" {
		t.Errorf("RunStatusRejected should be 'rejected', got %s", RunStatusRejected)
	}
}

func TestRunMessageTypeConstants(t *testing.T) {
	// Ensure message type constants have expected values
	if TypeRunRequest != "run_request" {
		t.Errorf("TypeRunRequest should be 'run_request', got %s", TypeRunRequest)
	}
	if TypeRunResponse != "run_response" {
		t.Errorf("TypeRunResponse should be 'run_response', got %s", TypeRunResponse)
	}
}
