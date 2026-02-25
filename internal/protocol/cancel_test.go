package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewCancelRequest(t *testing.T) {
	req := NewCancelRequest("req-123", "task-456")

	if req.Type != TypeCancelRequest {
		t.Errorf("expected type %s, got %s", TypeCancelRequest, req.Type)
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
	if req.Reason != "" {
		t.Errorf("expected empty reason, got %s", req.Reason)
	}
	if req.Force {
		t.Error("expected force to be false")
	}
}

func TestNewCancelRequestWithReason(t *testing.T) {
	req := NewCancelRequestWithReason("req-123", "task-456", "User requested cancellation")

	if req.Type != TypeCancelRequest {
		t.Errorf("expected type %s, got %s", TypeCancelRequest, req.Type)
	}
	if req.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", req.TaskID)
	}
	if req.Reason != "User requested cancellation" {
		t.Errorf("expected reason 'User requested cancellation', got %s", req.Reason)
	}
}

func TestNewCancelRequestWithForce(t *testing.T) {
	req := NewCancelRequestWithForce("req-123", "task-456", true)

	if req.Type != TypeCancelRequest {
		t.Errorf("expected type %s, got %s", TypeCancelRequest, req.Type)
	}
	if req.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", req.TaskID)
	}
	if !req.Force {
		t.Error("expected force to be true")
	}
}

func TestNewCancelResponse(t *testing.T) {
	resp := NewCancelResponse("req-123", "task-456", CancelStatusAccepted, "Cancellation initiated")

	if resp.Type != TypeCancelResponse {
		t.Errorf("expected type %s, got %s", TypeCancelResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", resp.TaskID)
	}
	if resp.Status != CancelStatusAccepted {
		t.Errorf("expected status %s, got %s", CancelStatusAccepted, resp.Status)
	}
	if resp.Message != "Cancellation initiated" {
		t.Errorf("expected message 'Cancellation initiated', got %s", resp.Message)
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewCancelErrorResponse(t *testing.T) {
	resp := NewCancelErrorResponse("req-123", "task-456", CancelStatusFailed, "Task not found")

	if resp.Type != TypeCancelResponse {
		t.Errorf("expected type %s, got %s", TypeCancelResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", resp.TaskID)
	}
	if resp.Status != CancelStatusFailed {
		t.Errorf("expected status %s, got %s", CancelStatusFailed, resp.Status)
	}
	if resp.Error != "Task not found" {
		t.Errorf("expected error 'Task not found', got %s", resp.Error)
	}
	if resp.Message != "" {
		t.Errorf("expected empty message, got %s", resp.Message)
	}
}

func TestCancelRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     CancelRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: CancelRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCancelRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: nil,
		},
		{
			name: "valid request with reason",
			req: CancelRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCancelRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Reason: "User requested",
			},
			wantErr: nil,
		},
		{
			name: "valid request with force",
			req: CancelRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCancelRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Force:  true,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			req: CancelRequest{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			req: CancelRequest{
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
			req: CancelRequest{
				BaseMessage: BaseMessage{
					Type: TypeCancelRequest,
				},
				TaskID: "task-001",
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing task_id",
			req: CancelRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCancelRequest,
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

func TestCancelResponseValidation(t *testing.T) {
	tests := []struct {
		name    string
		resp    CancelResponse
		wantErr error
	}{
		{
			name: "valid response with accepted status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusAccepted,
			},
			wantErr: nil,
		},
		{
			name: "valid response with completed status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusCompleted,
			},
			wantErr: nil,
		},
		{
			name: "valid response with failed status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusFailed,
			},
			wantErr: nil,
		},
		{
			name: "valid response with rejected status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusRejected,
			},
			wantErr: nil,
		},
		{
			name: "valid response with not_found status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusNotFound,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusAccepted,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatusAccepted,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type: TypeCancelResponse,
				},
				TaskID: "task-001",
				Status: CancelStatusAccepted,
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing task_id",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				Status: CancelStatusAccepted,
			},
			wantErr: ErrMissingTaskID,
		},
		{
			name: "missing status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrInvalidCancelStatus,
		},
		{
			name: "invalid status",
			resp: CancelResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCancelResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CancelStatus("invalid"),
			},
			wantErr: ErrInvalidCancelStatus,
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

func TestCancelRequestMarshalJSON(t *testing.T) {
	req := NewCancelRequest("req-001", "task-001")

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"cancel_request"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"req-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
}

func TestCancelRequestMarshalJSONWithReason(t *testing.T) {
	req := NewCancelRequestWithReason("req-001", "task-001", "Timeout exceeded")

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"reason":"Timeout exceeded"`) {
		t.Errorf("expected reason field in JSON, got %s", jsonStr)
	}
}

func TestCancelRequestMarshalJSONWithForce(t *testing.T) {
	req := NewCancelRequestWithForce("req-001", "task-001", true)

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"force":true`) {
		t.Errorf("expected force field in JSON, got %s", jsonStr)
	}
}

func TestCancelRequestUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "cancel_request",
		"request_id": "req-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002",
		"reason": "User cancelled",
		"force": true
	}`

	var req CancelRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != TypeCancelRequest {
		t.Errorf("expected type %s, got %s", TypeCancelRequest, req.Type)
	}
	if req.RequestID != "req-002" {
		t.Errorf("expected request_id req-002, got %s", req.RequestID)
	}
	if req.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", req.TaskID)
	}
	if req.Reason != "User cancelled" {
		t.Errorf("expected reason 'User cancelled', got %s", req.Reason)
	}
	if !req.Force {
		t.Error("expected force to be true")
	}
}

func TestCancelResponseMarshalJSON(t *testing.T) {
	resp := NewCancelResponse("resp-001", "task-001", CancelStatusCompleted, "Task cancelled successfully")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"cancel_response"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"resp-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"completed"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"message":"Task cancelled successfully"`) {
		t.Errorf("expected message field in JSON, got %s", jsonStr)
	}
}

func TestCancelResponseMarshalJSONWithError(t *testing.T) {
	resp := NewCancelErrorResponse("resp-001", "task-001", CancelStatusFailed, "Cannot cancel completed task")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"status":"failed"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"error":"Cannot cancel completed task"`) {
		t.Errorf("expected error field in JSON, got %s", jsonStr)
	}
}

func TestCancelResponseUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "cancel_response",
		"request_id": "resp-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002",
		"status": "completed",
		"message": "Task cancelled successfully"
	}`

	var resp CancelResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Type != TypeCancelResponse {
		t.Errorf("expected type %s, got %s", TypeCancelResponse, resp.Type)
	}
	if resp.RequestID != "resp-002" {
		t.Errorf("expected request_id resp-002, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", resp.TaskID)
	}
	if resp.Status != CancelStatusCompleted {
		t.Errorf("expected status %s, got %s", CancelStatusCompleted, resp.Status)
	}
	if resp.Message != "Task cancelled successfully" {
		t.Errorf("expected message 'Task cancelled successfully', got %s", resp.Message)
	}
}

func TestCancelResponseUnmarshalJSONWithError(t *testing.T) {
	jsonStr := `{
		"type": "cancel_response",
		"request_id": "resp-003",
		"task_id": "task-003",
		"status": "rejected",
		"error": "Task not running"
	}`

	var resp CancelResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Status != CancelStatusRejected {
		t.Errorf("expected status %s, got %s", CancelStatusRejected, resp.Status)
	}
	if resp.Error != "Task not running" {
		t.Errorf("expected error 'Task not running', got %s", resp.Error)
	}
}

func TestParseMessageCancelRequest(t *testing.T) {
	input := `{"type": "cancel_request", "request_id": "req-001", "task_id": "task-001"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeCancelRequest {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeCancelRequest)
	}

	req, ok := result.(*CancelRequest)
	if !ok {
		t.Fatal("expected *CancelRequest")
	}
	if req.RequestID != "req-001" {
		t.Errorf("expected request_id req-001, got %s", req.RequestID)
	}
	if req.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", req.TaskID)
	}
}

func TestParseMessageCancelResponse(t *testing.T) {
	input := `{"type": "cancel_response", "request_id": "resp-001", "task_id": "task-001", "status": "accepted"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeCancelResponse {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeCancelResponse)
	}

	resp, ok := result.(*CancelResponse)
	if !ok {
		t.Fatal("expected *CancelResponse")
	}
	if resp.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", resp.TaskID)
	}
	if resp.Status != CancelStatusAccepted {
		t.Errorf("expected status accepted, got %s", resp.Status)
	}
}

func TestCancelRequestRoundTrip(t *testing.T) {
	original := NewCancelRequestWithReason("roundtrip-001", "task-roundtrip", "Test reason")
	original.Timestamp = "2024-01-15T10:00:00Z"
	original.Force = true

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed CancelRequest
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
	if parsed.Reason != original.Reason {
		t.Errorf("Reason mismatch: %s != %s", parsed.Reason, original.Reason)
	}
	if parsed.Force != original.Force {
		t.Errorf("Force mismatch: %v != %v", parsed.Force, original.Force)
	}
}

func TestCancelResponseRoundTrip(t *testing.T) {
	original := NewCancelResponse("roundtrip-002", "task-roundtrip", CancelStatusCompleted, "Done")
	original.Timestamp = "2024-01-15T11:00:00Z"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed CancelResponse
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

func TestCancelStatusConstants(t *testing.T) {
	// Ensure cancel status codes have expected string values
	if CancelStatusAccepted != "accepted" {
		t.Errorf("CancelStatusAccepted should be 'accepted', got %s", CancelStatusAccepted)
	}
	if CancelStatusCompleted != "completed" {
		t.Errorf("CancelStatusCompleted should be 'completed', got %s", CancelStatusCompleted)
	}
	if CancelStatusFailed != "failed" {
		t.Errorf("CancelStatusFailed should be 'failed', got %s", CancelStatusFailed)
	}
	if CancelStatusRejected != "rejected" {
		t.Errorf("CancelStatusRejected should be 'rejected', got %s", CancelStatusRejected)
	}
	if CancelStatusNotFound != "not_found" {
		t.Errorf("CancelStatusNotFound should be 'not_found', got %s", CancelStatusNotFound)
	}
}

func TestCancelMessageTypeConstants(t *testing.T) {
	// Ensure message type constants have expected values
	if TypeCancelRequest != "cancel_request" {
		t.Errorf("TypeCancelRequest should be 'cancel_request', got %s", TypeCancelRequest)
	}
	if TypeCancelResponse != "cancel_response" {
		t.Errorf("TypeCancelResponse should be 'cancel_response', got %s", TypeCancelResponse)
	}
}
