package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLogsRequest(t *testing.T) {
	req := NewLogsRequest("req-123", "task-456")

	if req.Type != TypeLogsRequest {
		t.Errorf("expected type %s, got %s", TypeLogsRequest, req.Type)
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
	if req.Level != "" {
		t.Error("expected Level to be empty by default")
	}
	if req.Limit != 0 {
		t.Error("expected Limit to be 0 by default")
	}
	if req.Follow {
		t.Error("expected Follow to be false by default")
	}
}

func TestNewLogsRequestWithOptions(t *testing.T) {
	req := NewLogsRequestWithOptions("req-789", "task-abc", LogLevelError, 100)

	if req.Type != TypeLogsRequest {
		t.Errorf("expected type %s, got %s", TypeLogsRequest, req.Type)
	}
	if req.RequestID != "req-789" {
		t.Errorf("expected request_id req-789, got %s", req.RequestID)
	}
	if req.TaskID != "task-abc" {
		t.Errorf("expected task_id task-abc, got %s", req.TaskID)
	}
	if req.Level != LogLevelError {
		t.Errorf("expected level %s, got %s", LogLevelError, req.Level)
	}
	if req.Limit != 100 {
		t.Errorf("expected limit 100, got %d", req.Limit)
	}
}

func TestNewLogsResponse(t *testing.T) {
	entries := []LogEntry{
		{Timestamp: "2024-01-15T10:00:00Z", Level: LogLevelInfo, Message: "Test message"},
	}
	resp := NewLogsResponse("req-123", "task-456", entries)

	if resp.Type != TypeLogsResponse {
		t.Errorf("expected type %s, got %s", TypeLogsResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", resp.TaskID)
	}
	if len(resp.Entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(resp.Entries))
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewLogsErrorResponse(t *testing.T) {
	resp := NewLogsErrorResponse("req-123", "task-456", "Task not found")

	if resp.Type != TypeLogsResponse {
		t.Errorf("expected type %s, got %s", TypeLogsResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-456" {
		t.Errorf("expected task_id task-456, got %s", resp.TaskID)
	}
	if resp.Error != "Task not found" {
		t.Errorf("expected error 'Task not found', got %s", resp.Error)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(resp.Entries))
	}
}

func TestNewLogEntry(t *testing.T) {
	entry := NewLogEntry(LogLevelWarn, "Warning message")

	if entry.Level != LogLevelWarn {
		t.Errorf("expected level %s, got %s", LogLevelWarn, entry.Level)
	}
	if entry.Message != "Warning message" {
		t.Errorf("expected message 'Warning message', got %s", entry.Message)
	}
	if entry.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestLogsRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     LogsRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type:      TypeLogsRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: nil,
		},
		{
			name: "valid request with level",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type:      TypeLogsRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Level:  LogLevelInfo,
			},
			wantErr: nil,
		},
		{
			name: "valid request with all levels",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type:      TypeLogsRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Level:  LogLevelDebug,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			req: LogsRequest{
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
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type: TypeLogsRequest,
				},
				TaskID: "task-001",
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing task_id",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type:      TypeLogsRequest,
					RequestID: "valid-id",
				},
			},
			wantErr: ErrMissingTaskID,
		},
		{
			name: "invalid log level",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type:      TypeLogsRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Level:  LogLevel("invalid"),
			},
			wantErr: ErrInvalidLogLevel,
		},
		{
			name: "negative limit",
			req: LogsRequest{
				BaseMessage: BaseMessage{
					Type:      TypeLogsRequest,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Limit:  -1,
			},
			wantErr: ErrInvalidLimit,
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

func TestLogsResponseValidation(t *testing.T) {
	tests := []struct {
		name    string
		resp    LogsResponse
		wantErr error
	}{
		{
			name: "valid response",
			resp: LogsResponse{
				BaseMessage: BaseMessage{
					Type:      TypeLogsResponse,
					RequestID: "valid-id",
				},
				TaskID:  "task-001",
				Entries: []LogEntry{},
			},
			wantErr: nil,
		},
		{
			name: "valid response with entries",
			resp: LogsResponse{
				BaseMessage: BaseMessage{
					Type:      TypeLogsResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Entries: []LogEntry{
					{Timestamp: "2024-01-15T10:00:00Z", Level: LogLevelInfo, Message: "Test"},
				},
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			resp: LogsResponse{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID:  "task-001",
				Entries: []LogEntry{},
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			resp: LogsResponse{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				TaskID:  "task-001",
				Entries: []LogEntry{},
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			resp: LogsResponse{
				BaseMessage: BaseMessage{
					Type: TypeLogsResponse,
				},
				TaskID:  "task-001",
				Entries: []LogEntry{},
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing task_id",
			resp: LogsResponse{
				BaseMessage: BaseMessage{
					Type:      TypeLogsResponse,
					RequestID: "valid-id",
				},
				Entries: []LogEntry{},
			},
			wantErr: ErrMissingTaskID,
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

func TestLogsRequestMarshalJSON(t *testing.T) {
	req := NewLogsRequestWithOptions("req-001", "task-001", LogLevelError, 50)
	req.Follow = true
	req.Since = "2024-01-15T00:00:00Z"

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"logs_request"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"req-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"level":"error"`) {
		t.Errorf("expected level field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"limit":50`) {
		t.Errorf("expected limit field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"follow":true`) {
		t.Errorf("expected follow field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"since":"2024-01-15T00:00:00Z"`) {
		t.Errorf("expected since field in JSON, got %s", jsonStr)
	}
}

func TestLogsRequestUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "logs_request",
		"request_id": "req-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002",
		"level": "warn",
		"limit": 100,
		"follow": true,
		"since": "2024-01-14T00:00:00Z",
		"until": "2024-01-15T00:00:00Z"
	}`

	var req LogsRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != TypeLogsRequest {
		t.Errorf("expected type %s, got %s", TypeLogsRequest, req.Type)
	}
	if req.RequestID != "req-002" {
		t.Errorf("expected request_id req-002, got %s", req.RequestID)
	}
	if req.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", req.TaskID)
	}
	if req.Level != LogLevelWarn {
		t.Errorf("expected level %s, got %s", LogLevelWarn, req.Level)
	}
	if req.Limit != 100 {
		t.Errorf("expected limit 100, got %d", req.Limit)
	}
	if !req.Follow {
		t.Error("expected Follow to be true")
	}
	if req.Since != "2024-01-14T00:00:00Z" {
		t.Errorf("expected since 2024-01-14T00:00:00Z, got %s", req.Since)
	}
	if req.Until != "2024-01-15T00:00:00Z" {
		t.Errorf("expected until 2024-01-15T00:00:00Z, got %s", req.Until)
	}
}

func TestLogsResponseMarshalJSON(t *testing.T) {
	entries := []LogEntry{
		{
			Timestamp: "2024-01-15T10:00:00Z",
			Level:     LogLevelInfo,
			Message:   "Task started",
			Source:    "agent",
		},
		{
			Timestamp: "2024-01-15T10:01:00Z",
			Level:     LogLevelError,
			Message:   "Connection failed",
			Source:    "system",
			Metadata: map[string]interface{}{
				"retry_count": 3,
				"error_code":  "CONN_TIMEOUT",
			},
		},
	}
	resp := NewLogsResponse("resp-001", "task-001", entries)
	resp.HasMore = true
	resp.NextCursor = "cursor-abc123"

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"logs_response"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"has_more":true`) {
		t.Errorf("expected has_more field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"next_cursor":"cursor-abc123"`) {
		t.Errorf("expected next_cursor field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"level":"info"`) {
		t.Errorf("expected level:info in entries, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"level":"error"`) {
		t.Errorf("expected level:error in entries, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"source":"agent"`) {
		t.Errorf("expected source:agent in entries, got %s", jsonStr)
	}
}

func TestLogsResponseUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "logs_response",
		"request_id": "resp-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002",
		"entries": [
			{
				"timestamp": "2024-01-15T10:00:00Z",
				"level": "debug",
				"message": "Debug info",
				"source": "agent"
			},
			{
				"timestamp": "2024-01-15T10:01:00Z",
				"level": "info",
				"message": "Processing complete",
				"metadata": {"duration_ms": 150}
			}
		],
		"has_more": false
	}`

	var resp LogsResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Type != TypeLogsResponse {
		t.Errorf("expected type %s, got %s", TypeLogsResponse, resp.Type)
	}
	if resp.RequestID != "resp-002" {
		t.Errorf("expected request_id resp-002, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", resp.TaskID)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(resp.Entries))
	}
	if resp.Entries[0].Level != LogLevelDebug {
		t.Errorf("expected first entry level debug, got %s", resp.Entries[0].Level)
	}
	if resp.Entries[0].Source != "agent" {
		t.Errorf("expected first entry source agent, got %s", resp.Entries[0].Source)
	}
	if resp.Entries[1].Message != "Processing complete" {
		t.Errorf("expected second entry message 'Processing complete', got %s", resp.Entries[1].Message)
	}
	if resp.Entries[1].Metadata == nil {
		t.Error("expected second entry to have metadata")
	}
	if resp.HasMore {
		t.Error("expected HasMore to be false")
	}
}

func TestLogsResponseUnmarshalJSONWithError(t *testing.T) {
	jsonStr := `{
		"type": "logs_response",
		"request_id": "resp-003",
		"task_id": "task-003",
		"entries": [],
		"error": "Task not found"
	}`

	var resp LogsResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Error != "Task not found" {
		t.Errorf("expected error 'Task not found', got %s", resp.Error)
	}
	if len(resp.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(resp.Entries))
	}
}

func TestParseMessageLogsRequest(t *testing.T) {
	input := `{"type": "logs_request", "request_id": "req-001", "task_id": "task-001", "level": "info"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeLogsRequest {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeLogsRequest)
	}

	req, ok := result.(*LogsRequest)
	if !ok {
		t.Fatal("expected *LogsRequest")
	}
	if req.RequestID != "req-001" {
		t.Errorf("expected request_id req-001, got %s", req.RequestID)
	}
	if req.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", req.TaskID)
	}
	if req.Level != LogLevelInfo {
		t.Errorf("expected level info, got %s", req.Level)
	}
}

func TestParseMessageLogsResponse(t *testing.T) {
	input := `{"type": "logs_response", "request_id": "resp-001", "task_id": "task-001", "entries": []}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeLogsResponse {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeLogsResponse)
	}

	resp, ok := result.(*LogsResponse)
	if !ok {
		t.Fatal("expected *LogsResponse")
	}
	if resp.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", resp.TaskID)
	}
}

func TestLogsRequestRoundTrip(t *testing.T) {
	original := NewLogsRequestWithOptions("roundtrip-001", "task-roundtrip", LogLevelWarn, 50)
	original.Timestamp = "2024-01-15T10:00:00Z"
	original.Follow = true
	original.Since = "2024-01-14T00:00:00Z"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed LogsRequest
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
	if parsed.Level != original.Level {
		t.Errorf("Level mismatch: %s != %s", parsed.Level, original.Level)
	}
	if parsed.Limit != original.Limit {
		t.Errorf("Limit mismatch: %d != %d", parsed.Limit, original.Limit)
	}
	if parsed.Follow != original.Follow {
		t.Errorf("Follow mismatch: %v != %v", parsed.Follow, original.Follow)
	}
	if parsed.Since != original.Since {
		t.Errorf("Since mismatch: %s != %s", parsed.Since, original.Since)
	}
}

func TestLogsResponseRoundTrip(t *testing.T) {
	entries := []LogEntry{
		{
			Timestamp: "2024-01-15T10:00:00Z",
			Level:     LogLevelInfo,
			Message:   "Test entry",
			Source:    "test",
			Metadata:  map[string]interface{}{"key": "value"},
		},
	}
	original := NewLogsResponse("roundtrip-002", "task-roundtrip", entries)
	original.Timestamp = "2024-01-15T11:00:00Z"
	original.HasMore = true
	original.NextCursor = "cursor-xyz"

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed LogsResponse
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
	if len(parsed.Entries) != len(original.Entries) {
		t.Fatalf("Entries count mismatch: %d != %d", len(parsed.Entries), len(original.Entries))
	}
	if parsed.Entries[0].Message != original.Entries[0].Message {
		t.Errorf("Entry message mismatch: %s != %s", parsed.Entries[0].Message, original.Entries[0].Message)
	}
	if parsed.HasMore != original.HasMore {
		t.Errorf("HasMore mismatch: %v != %v", parsed.HasMore, original.HasMore)
	}
	if parsed.NextCursor != original.NextCursor {
		t.Errorf("NextCursor mismatch: %s != %s", parsed.NextCursor, original.NextCursor)
	}
}

func TestLogLevelConstants(t *testing.T) {
	// Ensure log level constants have expected string values
	if LogLevelDebug != "debug" {
		t.Errorf("LogLevelDebug should be 'debug', got %s", LogLevelDebug)
	}
	if LogLevelInfo != "info" {
		t.Errorf("LogLevelInfo should be 'info', got %s", LogLevelInfo)
	}
	if LogLevelWarn != "warn" {
		t.Errorf("LogLevelWarn should be 'warn', got %s", LogLevelWarn)
	}
	if LogLevelError != "error" {
		t.Errorf("LogLevelError should be 'error', got %s", LogLevelError)
	}
}

func TestLogsMessageTypeConstants(t *testing.T) {
	// Ensure message type constants have expected values
	if TypeLogsRequest != "logs_request" {
		t.Errorf("TypeLogsRequest should be 'logs_request', got %s", TypeLogsRequest)
	}
	if TypeLogsResponse != "logs_response" {
		t.Errorf("TypeLogsResponse should be 'logs_response', got %s", TypeLogsResponse)
	}
}

func TestLogEntryWithMetadata(t *testing.T) {
	entry := LogEntry{
		Timestamp: "2024-01-15T10:00:00Z",
		Level:     LogLevelInfo,
		Message:   "Task completed",
		Source:    "agent",
		Metadata: map[string]interface{}{
			"duration_ms": 1500,
			"exit_code":   0,
			"output_size": 1024,
		},
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"duration_ms"`) {
		t.Errorf("expected duration_ms in metadata, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"exit_code"`) {
		t.Errorf("expected exit_code in metadata, got %s", jsonStr)
	}

	var parsed LogEntry
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Metadata == nil {
		t.Fatal("expected Metadata to be set")
	}
	// JSON numbers are parsed as float64
	if durationMS, ok := parsed.Metadata["duration_ms"].(float64); !ok || durationMS != 1500 {
		t.Errorf("expected duration_ms to be 1500, got %v", parsed.Metadata["duration_ms"])
	}
}

func TestLogsValidLevels(t *testing.T) {
	levels := []LogLevel{LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError}

	for _, level := range levels {
		req := LogsRequest{
			BaseMessage: BaseMessage{
				Type:      TypeLogsRequest,
				RequestID: "test-id",
			},
			TaskID: "task-id",
			Level:  level,
		}

		if err := req.Validate(); err != nil {
			t.Errorf("expected level %s to be valid, got error: %v", level, err)
		}
	}
}
