package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewStatusRequest(t *testing.T) {
	req := NewStatusRequest("test-123")

	if req.Type != TypeStatusRequest {
		t.Errorf("expected type %s, got %s", TypeStatusRequest, req.Type)
	}
	if req.RequestID != "test-123" {
		t.Errorf("expected request_id test-123, got %s", req.RequestID)
	}
	if req.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
	if req.IncludeMetrics {
		t.Error("expected IncludeMetrics to be false by default")
	}
	if req.IncludeConnections {
		t.Error("expected IncludeConnections to be false by default")
	}
}

func TestNewStatusRequestWithOptions(t *testing.T) {
	req := NewStatusRequestWithOptions("test-456", true, true)

	if req.Type != TypeStatusRequest {
		t.Errorf("expected type %s, got %s", TypeStatusRequest, req.Type)
	}
	if req.RequestID != "test-456" {
		t.Errorf("expected request_id test-456, got %s", req.RequestID)
	}
	if !req.IncludeMetrics {
		t.Error("expected IncludeMetrics to be true")
	}
	if !req.IncludeConnections {
		t.Error("expected IncludeConnections to be true")
	}
}

func TestNewStatusResponse(t *testing.T) {
	resp := NewStatusResponse("test-789", StatusOK, "Gateway running")

	if resp.Type != TypeStatusResponse {
		t.Errorf("expected type %s, got %s", TypeStatusResponse, resp.Type)
	}
	if resp.RequestID != "test-789" {
		t.Errorf("expected request_id test-789, got %s", resp.RequestID)
	}
	if resp.Status != StatusOK {
		t.Errorf("expected status %s, got %s", StatusOK, resp.Status)
	}
	if resp.Message != "Gateway running" {
		t.Errorf("expected message 'Gateway running', got %s", resp.Message)
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestStatusRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     StatusRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: StatusRequest{
				BaseMessage: BaseMessage{
					Type:      TypeStatusRequest,
					RequestID: "valid-id",
				},
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			req: StatusRequest{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			req: StatusRequest{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			req: StatusRequest{
				BaseMessage: BaseMessage{
					Type: TypeStatusRequest,
				},
			},
			wantErr: ErrMissingRequestID,
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

func TestStatusResponseValidation(t *testing.T) {
	tests := []struct {
		name    string
		resp    StatusResponse
		wantErr error
	}{
		{
			name: "valid response with ok status",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type:      TypeStatusResponse,
					RequestID: "valid-id",
				},
				Status: StatusOK,
			},
			wantErr: nil,
		},
		{
			name: "valid response with degraded status",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type:      TypeStatusResponse,
					RequestID: "valid-id",
				},
				Status: StatusDegraded,
			},
			wantErr: nil,
		},
		{
			name: "valid response with error status",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type:      TypeStatusResponse,
					RequestID: "valid-id",
				},
				Status: StatusError,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				Status: StatusOK,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				Status: StatusOK,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type: TypeStatusResponse,
				},
				Status: StatusOK,
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing status",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type:      TypeStatusResponse,
					RequestID: "valid-id",
				},
			},
			wantErr: ErrInvalidStatusCode,
		},
		{
			name: "invalid status code",
			resp: StatusResponse{
				BaseMessage: BaseMessage{
					Type:      TypeStatusResponse,
					RequestID: "valid-id",
				},
				Status: StatusCode("invalid"),
			},
			wantErr: ErrInvalidStatusCode,
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

func TestStatusRequestMarshalJSON(t *testing.T) {
	req := NewStatusRequestWithOptions("req-001", true, false)

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"status_request"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"req-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"include_metrics":true`) {
		t.Errorf("expected include_metrics field in JSON, got %s", jsonStr)
	}
}

func TestStatusRequestUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "status_request",
		"request_id": "req-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"include_metrics": true,
		"include_connections": true
	}`

	var req StatusRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != TypeStatusRequest {
		t.Errorf("expected type %s, got %s", TypeStatusRequest, req.Type)
	}
	if req.RequestID != "req-002" {
		t.Errorf("expected request_id req-002, got %s", req.RequestID)
	}
	if !req.IncludeMetrics {
		t.Error("expected IncludeMetrics to be true")
	}
	if !req.IncludeConnections {
		t.Error("expected IncludeConnections to be true")
	}
}

func TestStatusResponseMarshalJSON(t *testing.T) {
	resp := NewStatusResponse("resp-001", StatusOK, "All systems operational")
	resp.Version = "1.0.0"
	resp.Connections = &ConnectionInfo{
		TotalConnections:         10,
		AuthenticatedConnections: 8,
		ConnectionsByProject: map[string]int{
			"project-a": 5,
			"project-b": 3,
		},
	}
	resp.Metrics = &Metrics{
		UptimeSeconds:    3600,
		MessagesReceived: 1000,
		MessagesSent:     950,
		MemoryUsageBytes: 1024 * 1024 * 50,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"status_response"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"ok"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"version":"1.0.0"`) {
		t.Errorf("expected version field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"total_connections":10`) {
		t.Errorf("expected total_connections field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"uptime_seconds":3600`) {
		t.Errorf("expected uptime_seconds field in JSON, got %s", jsonStr)
	}
}

func TestStatusResponseUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "status_response",
		"request_id": "resp-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"status": "degraded",
		"message": "High latency detected",
		"version": "1.0.0",
		"connections": {
			"total_connections": 5,
			"authenticated_connections": 3
		},
		"metrics": {
			"uptime_seconds": 7200,
			"messages_received": 500
		}
	}`

	var resp StatusResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Type != TypeStatusResponse {
		t.Errorf("expected type %s, got %s", TypeStatusResponse, resp.Type)
	}
	if resp.RequestID != "resp-002" {
		t.Errorf("expected request_id resp-002, got %s", resp.RequestID)
	}
	if resp.Status != StatusDegraded {
		t.Errorf("expected status %s, got %s", StatusDegraded, resp.Status)
	}
	if resp.Message != "High latency detected" {
		t.Errorf("expected message 'High latency detected', got %s", resp.Message)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", resp.Version)
	}
	if resp.Connections == nil {
		t.Fatal("expected Connections to be set")
	}
	if resp.Connections.TotalConnections != 5 {
		t.Errorf("expected TotalConnections 5, got %d", resp.Connections.TotalConnections)
	}
	if resp.Metrics == nil {
		t.Fatal("expected Metrics to be set")
	}
	if resp.Metrics.UptimeSeconds != 7200 {
		t.Errorf("expected UptimeSeconds 7200, got %d", resp.Metrics.UptimeSeconds)
	}
}

func TestParseMessage(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantType    string
		wantErr     error
		checkResult func(t *testing.T, result interface{})
	}{
		{
			name:     "valid status request",
			input:    `{"type": "status_request", "request_id": "req-001"}`,
			wantType: TypeStatusRequest,
			wantErr:  nil,
			checkResult: func(t *testing.T, result interface{}) {
				req, ok := result.(*StatusRequest)
				if !ok {
					t.Fatal("expected *StatusRequest")
				}
				if req.RequestID != "req-001" {
					t.Errorf("expected request_id req-001, got %s", req.RequestID)
				}
			},
		},
		{
			name:     "valid status response",
			input:    `{"type": "status_response", "request_id": "resp-001", "status": "ok"}`,
			wantType: TypeStatusResponse,
			wantErr:  nil,
			checkResult: func(t *testing.T, result interface{}) {
				resp, ok := result.(*StatusResponse)
				if !ok {
					t.Fatal("expected *StatusResponse")
				}
				if resp.Status != StatusOK {
					t.Errorf("expected status ok, got %s", resp.Status)
				}
			},
		},
		{
			name:     "missing type",
			input:    `{"request_id": "req-001"}`,
			wantType: "",
			wantErr:  ErrMissingType,
		},
		{
			name:     "unknown type",
			input:    `{"type": "unknown_type", "request_id": "req-001"}`,
			wantType: "unknown_type",
			wantErr:  ErrUnknownType,
		},
		{
			name:     "invalid json",
			input:    `{invalid json}`,
			wantType: "",
			wantErr:  ErrInvalidMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, msgType, err := ParseMessage([]byte(tt.input))

			if err != tt.wantErr {
				t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if msgType != tt.wantType {
				t.Errorf("ParseMessage() type = %v, want %v", msgType, tt.wantType)
			}

			if tt.checkResult != nil && err == nil {
				tt.checkResult(t, result)
			}
		})
	}
}

func TestStatusRequestRoundTrip(t *testing.T) {
	original := NewStatusRequestWithOptions("roundtrip-001", true, true)
	original.Timestamp = "2024-01-15T10:00:00Z"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed StatusRequest
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
	if parsed.IncludeMetrics != original.IncludeMetrics {
		t.Errorf("IncludeMetrics mismatch: %v != %v", parsed.IncludeMetrics, original.IncludeMetrics)
	}
	if parsed.IncludeConnections != original.IncludeConnections {
		t.Errorf("IncludeConnections mismatch: %v != %v", parsed.IncludeConnections, original.IncludeConnections)
	}
}

func TestStatusResponseRoundTrip(t *testing.T) {
	original := NewStatusResponse("roundtrip-002", StatusDegraded, "Test message")
	original.Timestamp = "2024-01-15T11:00:00Z"
	original.Version = "2.0.0"
	original.Connections = &ConnectionInfo{
		TotalConnections: 15,
	}
	original.Metrics = &Metrics{
		UptimeSeconds: 9999,
	}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed StatusResponse
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
	if parsed.Status != original.Status {
		t.Errorf("Status mismatch: %s != %s", parsed.Status, original.Status)
	}
	if parsed.Message != original.Message {
		t.Errorf("Message mismatch: %s != %s", parsed.Message, original.Message)
	}
	if parsed.Connections == nil {
		t.Fatal("Connections should not be nil")
	}
	if parsed.Connections.TotalConnections != original.Connections.TotalConnections {
		t.Errorf("TotalConnections mismatch: %d != %d",
			parsed.Connections.TotalConnections, original.Connections.TotalConnections)
	}
	if parsed.Metrics == nil {
		t.Fatal("Metrics should not be nil")
	}
	if parsed.Metrics.UptimeSeconds != original.Metrics.UptimeSeconds {
		t.Errorf("UptimeSeconds mismatch: %d != %d",
			parsed.Metrics.UptimeSeconds, original.Metrics.UptimeSeconds)
	}
}

func TestStatusCodeConstants(t *testing.T) {
	// Ensure status codes have expected string values
	if StatusOK != "ok" {
		t.Errorf("StatusOK should be 'ok', got %s", StatusOK)
	}
	if StatusDegraded != "degraded" {
		t.Errorf("StatusDegraded should be 'degraded', got %s", StatusDegraded)
	}
	if StatusError != "error" {
		t.Errorf("StatusError should be 'error', got %s", StatusError)
	}
}

func TestMessageTypeConstants(t *testing.T) {
	// Ensure message type constants have expected values
	if TypeStatusRequest != "status_request" {
		t.Errorf("TypeStatusRequest should be 'status_request', got %s", TypeStatusRequest)
	}
	if TypeStatusResponse != "status_response" {
		t.Errorf("TypeStatusResponse should be 'status_response', got %s", TypeStatusResponse)
	}
}
