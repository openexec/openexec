package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewDeployRequest(t *testing.T) {
	req := NewDeployRequest("req-123", "project-456")

	if req.Type != TypeDeployRequest {
		t.Errorf("expected type %s, got %s", TypeDeployRequest, req.Type)
	}
	if req.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", req.RequestID)
	}
	if req.ProjectID != "project-456" {
		t.Errorf("expected project_id project-456, got %s", req.ProjectID)
	}
	if req.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewDeployRequestWithEnv(t *testing.T) {
	req := NewDeployRequestWithEnv("req-123", "project-456", "production")

	if req.Type != TypeDeployRequest {
		t.Errorf("expected type %s, got %s", TypeDeployRequest, req.Type)
	}
	if req.ProjectID != "project-456" {
		t.Errorf("expected project_id project-456, got %s", req.ProjectID)
	}
	if req.Environment != "production" {
		t.Errorf("expected environment production, got %s", req.Environment)
	}
}

func TestNewDeployResponse(t *testing.T) {
	resp := NewDeployResponse("req-123", "project-456", DeployStatusAccepted, "Deployment accepted")

	if resp.Type != TypeDeployResponse {
		t.Errorf("expected type %s, got %s", TypeDeployResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.ProjectID != "project-456" {
		t.Errorf("expected project_id project-456, got %s", resp.ProjectID)
	}
	if resp.Status != DeployStatusAccepted {
		t.Errorf("expected status %s, got %s", DeployStatusAccepted, resp.Status)
	}
	if resp.Message != "Deployment accepted" {
		t.Errorf("expected message 'Deployment accepted', got %s", resp.Message)
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewDeployErrorResponse(t *testing.T) {
	resp := NewDeployErrorResponse("req-123", "project-456", DeployStatusFailed, "Deployment failed")

	if resp.Type != TypeDeployResponse {
		t.Errorf("expected type %s, got %s", TypeDeployResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.ProjectID != "project-456" {
		t.Errorf("expected project_id project-456, got %s", resp.ProjectID)
	}
	if resp.Status != DeployStatusFailed {
		t.Errorf("expected status %s, got %s", DeployStatusFailed, resp.Status)
	}
	if resp.Error != "Deployment failed" {
		t.Errorf("expected error 'Deployment failed', got %s", resp.Error)
	}
	if resp.Message != "" {
		t.Errorf("expected empty message, got %s", resp.Message)
	}
}

func TestDeployRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     DeployRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: DeployRequest{
				BaseMessage: BaseMessage{
					Type:      TypeDeployRequest,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
			},
			wantErr: nil,
		},
		{
			name: "valid request with environment",
			req: DeployRequest{
				BaseMessage: BaseMessage{
					Type:      TypeDeployRequest,
					RequestID: "valid-id",
				},
				ProjectID:   "project-001",
				Environment: "staging",
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			req: DeployRequest{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			req: DeployRequest{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			req: DeployRequest{
				BaseMessage: BaseMessage{
					Type: TypeDeployRequest,
				},
				ProjectID: "project-001",
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing project_id",
			req: DeployRequest{
				BaseMessage: BaseMessage{
					Type:      TypeDeployRequest,
					RequestID: "valid-id",
				},
			},
			wantErr: ErrMissingProjectIDForDeploy,
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

func TestDeployResponseValidation(t *testing.T) {
	tests := []struct {
		name    string
		resp    DeployResponse
		wantErr error
	}{
		{
			name: "valid response with accepted status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusAccepted,
			},
			wantErr: nil,
		},
		{
			name: "valid response with running status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusRunning,
			},
			wantErr: nil,
		},
		{
			name: "valid response with completed status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusCompleted,
			},
			wantErr: nil,
		},
		{
			name: "valid response with failed status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusFailed,
			},
			wantErr: nil,
		},
		{
			name: "valid response with rejected status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusRejected,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusAccepted,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatusAccepted,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type: TypeDeployResponse,
				},
				ProjectID: "project-001",
				Status:    DeployStatusAccepted,
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing project_id",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				Status: DeployStatusAccepted,
			},
			wantErr: ErrMissingProjectIDForDeploy,
		},
		{
			name: "missing status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
			},
			wantErr: ErrInvalidDeployStatus,
		},
		{
			name: "invalid status",
			resp: DeployResponse{
				BaseMessage: BaseMessage{
					Type:      TypeDeployResponse,
					RequestID: "valid-id",
				},
				ProjectID: "project-001",
				Status:    DeployStatus("invalid"),
			},
			wantErr: ErrInvalidDeployStatus,
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

func TestDeployRequestMarshalJSON(t *testing.T) {
	req := NewDeployRequestWithEnv("req-001", "project-001", "production")

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"deploy_request"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"req-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"project_id":"project-001"`) {
		t.Errorf("expected project_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"environment":"production"`) {
		t.Errorf("expected environment field in JSON, got %s", jsonStr)
	}
}

func TestDeployRequestUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "deploy_request",
		"request_id": "req-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"project_id": "project-002",
		"environment": "staging"
	}`

	var req DeployRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != TypeDeployRequest {
		t.Errorf("expected type %s, got %s", TypeDeployRequest, req.Type)
	}
	if req.RequestID != "req-002" {
		t.Errorf("expected request_id req-002, got %s", req.RequestID)
	}
	if req.ProjectID != "project-002" {
		t.Errorf("expected project_id project-002, got %s", req.ProjectID)
	}
	if req.Environment != "staging" {
		t.Errorf("expected environment staging, got %s", req.Environment)
	}
}

func TestDeployResponseMarshalJSON(t *testing.T) {
	resp := NewDeployResponse("resp-001", "project-001", DeployStatusRunning, "Deployment in progress")
	resp.Environment = "production"
	resp.Version = "v1.2.3"

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"deploy_response"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"resp-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"project_id":"project-001"`) {
		t.Errorf("expected project_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"running"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"message":"Deployment in progress"`) {
		t.Errorf("expected message field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"environment":"production"`) {
		t.Errorf("expected environment field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"version":"v1.2.3"`) {
		t.Errorf("expected version field in JSON, got %s", jsonStr)
	}
}

func TestDeployResponseMarshalJSONWithError(t *testing.T) {
	resp := NewDeployErrorResponse("resp-001", "project-001", DeployStatusFailed, "Connection timeout")

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

func TestDeployResponseUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "deploy_response",
		"request_id": "resp-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"project_id": "project-002",
		"status": "completed",
		"message": "Deployment completed successfully",
		"environment": "production",
		"version": "abc123"
	}`

	var resp DeployResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Type != TypeDeployResponse {
		t.Errorf("expected type %s, got %s", TypeDeployResponse, resp.Type)
	}
	if resp.RequestID != "resp-002" {
		t.Errorf("expected request_id resp-002, got %s", resp.RequestID)
	}
	if resp.ProjectID != "project-002" {
		t.Errorf("expected project_id project-002, got %s", resp.ProjectID)
	}
	if resp.Status != DeployStatusCompleted {
		t.Errorf("expected status %s, got %s", DeployStatusCompleted, resp.Status)
	}
	if resp.Message != "Deployment completed successfully" {
		t.Errorf("expected message 'Deployment completed successfully', got %s", resp.Message)
	}
	if resp.Environment != "production" {
		t.Errorf("expected environment production, got %s", resp.Environment)
	}
	if resp.Version != "abc123" {
		t.Errorf("expected version abc123, got %s", resp.Version)
	}
}

func TestDeployResponseUnmarshalJSONWithError(t *testing.T) {
	jsonStr := `{
		"type": "deploy_response",
		"request_id": "resp-003",
		"project_id": "project-003",
		"status": "rejected",
		"error": "Deployment already in progress"
	}`

	var resp DeployResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Status != DeployStatusRejected {
		t.Errorf("expected status %s, got %s", DeployStatusRejected, resp.Status)
	}
	if resp.Error != "Deployment already in progress" {
		t.Errorf("expected error 'Deployment already in progress', got %s", resp.Error)
	}
}

func TestParseMessageDeployRequest(t *testing.T) {
	input := `{"type": "deploy_request", "request_id": "req-001", "project_id": "project-001"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeDeployRequest {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeDeployRequest)
	}

	req, ok := result.(*DeployRequest)
	if !ok {
		t.Fatal("expected *DeployRequest")
	}
	if req.RequestID != "req-001" {
		t.Errorf("expected request_id req-001, got %s", req.RequestID)
	}
	if req.ProjectID != "project-001" {
		t.Errorf("expected project_id project-001, got %s", req.ProjectID)
	}
}

func TestParseMessageDeployResponse(t *testing.T) {
	input := `{"type": "deploy_response", "request_id": "resp-001", "project_id": "project-001", "status": "accepted"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeDeployResponse {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeDeployResponse)
	}

	resp, ok := result.(*DeployResponse)
	if !ok {
		t.Fatal("expected *DeployResponse")
	}
	if resp.ProjectID != "project-001" {
		t.Errorf("expected project_id project-001, got %s", resp.ProjectID)
	}
	if resp.Status != DeployStatusAccepted {
		t.Errorf("expected status accepted, got %s", resp.Status)
	}
}

func TestDeployRequestRoundTrip(t *testing.T) {
	original := NewDeployRequestWithEnv("roundtrip-001", "project-roundtrip", "staging")
	original.Timestamp = "2024-01-15T10:00:00Z"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed DeployRequest
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
	if parsed.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: %s != %s", parsed.ProjectID, original.ProjectID)
	}
	if parsed.Environment != original.Environment {
		t.Errorf("Environment mismatch: %s != %s", parsed.Environment, original.Environment)
	}
}

func TestDeployResponseRoundTrip(t *testing.T) {
	original := NewDeployResponse("roundtrip-002", "project-roundtrip", DeployStatusCompleted, "Done")
	original.Timestamp = "2024-01-15T11:00:00Z"
	original.Environment = "production"
	original.Version = "v1.0.0"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed DeployResponse
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
	if parsed.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: %s != %s", parsed.ProjectID, original.ProjectID)
	}
	if parsed.Status != original.Status {
		t.Errorf("Status mismatch: %s != %s", parsed.Status, original.Status)
	}
	if parsed.Message != original.Message {
		t.Errorf("Message mismatch: %s != %s", parsed.Message, original.Message)
	}
	if parsed.Environment != original.Environment {
		t.Errorf("Environment mismatch: %s != %s", parsed.Environment, original.Environment)
	}
	if parsed.Version != original.Version {
		t.Errorf("Version mismatch: %s != %s", parsed.Version, original.Version)
	}
}

func TestDeployStatusConstants(t *testing.T) {
	// Ensure deploy status codes have expected string values
	if DeployStatusAccepted != "accepted" {
		t.Errorf("DeployStatusAccepted should be 'accepted', got %s", DeployStatusAccepted)
	}
	if DeployStatusRunning != "running" {
		t.Errorf("DeployStatusRunning should be 'running', got %s", DeployStatusRunning)
	}
	if DeployStatusCompleted != "completed" {
		t.Errorf("DeployStatusCompleted should be 'completed', got %s", DeployStatusCompleted)
	}
	if DeployStatusFailed != "failed" {
		t.Errorf("DeployStatusFailed should be 'failed', got %s", DeployStatusFailed)
	}
	if DeployStatusRejected != "rejected" {
		t.Errorf("DeployStatusRejected should be 'rejected', got %s", DeployStatusRejected)
	}
}

func TestDeployMessageTypeConstants(t *testing.T) {
	// Ensure message type constants have expected values
	if TypeDeployRequest != "deploy_request" {
		t.Errorf("TypeDeployRequest should be 'deploy_request', got %s", TypeDeployRequest)
	}
	if TypeDeployResponse != "deploy_response" {
		t.Errorf("TypeDeployResponse should be 'deploy_response', got %s", TypeDeployResponse)
	}
}
