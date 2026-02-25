package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNewCreateTaskRequest(t *testing.T) {
	req := NewCreateTaskRequest("req-123", "Test Task", "This is a test task description")

	if req.Type != TypeCreateTaskRequest {
		t.Errorf("expected type %s, got %s", TypeCreateTaskRequest, req.Type)
	}
	if req.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", req.RequestID)
	}
	if req.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", req.Title)
	}
	if req.Description != "This is a test task description" {
		t.Errorf("expected description 'This is a test task description', got %s", req.Description)
	}
	if req.Priority != TaskPriorityNormal {
		t.Errorf("expected default priority %s, got %s", TaskPriorityNormal, req.Priority)
	}
	if req.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewCreateTaskRequestWithOptions(t *testing.T) {
	req := NewCreateTaskRequestWithOptions("req-456", "High Priority Task", "Critical work", TaskPriorityHigh, "project-001")

	if req.Type != TypeCreateTaskRequest {
		t.Errorf("expected type %s, got %s", TypeCreateTaskRequest, req.Type)
	}
	if req.RequestID != "req-456" {
		t.Errorf("expected request_id req-456, got %s", req.RequestID)
	}
	if req.Title != "High Priority Task" {
		t.Errorf("expected title 'High Priority Task', got %s", req.Title)
	}
	if req.Priority != TaskPriorityHigh {
		t.Errorf("expected priority %s, got %s", TaskPriorityHigh, req.Priority)
	}
	if req.ProjectID != "project-001" {
		t.Errorf("expected project_id project-001, got %s", req.ProjectID)
	}
}

func TestNewCreateTaskResponse(t *testing.T) {
	resp := NewCreateTaskResponse("req-123", "task-789", CreateTaskStatusCreated, "Task created successfully")

	if resp.Type != TypeCreateTaskResponse {
		t.Errorf("expected type %s, got %s", TypeCreateTaskResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-789" {
		t.Errorf("expected task_id task-789, got %s", resp.TaskID)
	}
	if resp.Status != CreateTaskStatusCreated {
		t.Errorf("expected status %s, got %s", CreateTaskStatusCreated, resp.Status)
	}
	if resp.Message != "Task created successfully" {
		t.Errorf("expected message 'Task created successfully', got %s", resp.Message)
	}
	if resp.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
}

func TestNewCreateTaskErrorResponse(t *testing.T) {
	resp := NewCreateTaskErrorResponse("req-123", CreateTaskStatusFailed, "Queue is full")

	if resp.Type != TypeCreateTaskResponse {
		t.Errorf("expected type %s, got %s", TypeCreateTaskResponse, resp.Type)
	}
	if resp.RequestID != "req-123" {
		t.Errorf("expected request_id req-123, got %s", resp.RequestID)
	}
	if resp.Status != CreateTaskStatusFailed {
		t.Errorf("expected status %s, got %s", CreateTaskStatusFailed, resp.Status)
	}
	if resp.Error != "Queue is full" {
		t.Errorf("expected error 'Queue is full', got %s", resp.Error)
	}
	if resp.TaskID != "" {
		t.Errorf("expected empty task_id, got %s", resp.TaskID)
	}
}

func TestCreateTaskRequestValidation(t *testing.T) {
	tests := []struct {
		name    string
		req     CreateTaskRequest
		wantErr error
	}{
		{
			name: "valid request",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskRequest,
					RequestID: "valid-id",
				},
				Title:       "Test Task",
				Description: "A test task",
			},
			wantErr: nil,
		},
		{
			name: "valid request with priority",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskRequest,
					RequestID: "valid-id",
				},
				Title:       "Test Task",
				Description: "A test task",
				Priority:    TaskPriorityHigh,
			},
			wantErr: nil,
		},
		{
			name: "valid request with all fields",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskRequest,
					RequestID: "valid-id",
				},
				Title:          "Test Task",
				Description:    "A test task",
				Priority:       TaskPriorityCritical,
				ProjectID:      "proj-001",
				Tags:           []string{"test", "important"},
				DependsOn:      []string{"task-001", "task-002"},
				TimeoutSeconds: 3600,
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				Title:       "Test Task",
				Description: "A test task",
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				Title:       "Test Task",
				Description: "A test task",
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type: TypeCreateTaskRequest,
				},
				Title:       "Test Task",
				Description: "A test task",
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing title",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskRequest,
					RequestID: "valid-id",
				},
				Description: "A test task",
			},
			wantErr: ErrMissingTitle,
		},
		{
			name: "missing description",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskRequest,
					RequestID: "valid-id",
				},
				Title: "Test Task",
			},
			wantErr: ErrMissingDescription,
		},
		{
			name: "invalid priority",
			req: CreateTaskRequest{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskRequest,
					RequestID: "valid-id",
				},
				Title:       "Test Task",
				Description: "A test task",
				Priority:    TaskPriority("invalid"),
			},
			wantErr: ErrInvalidPriority,
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

func TestCreateTaskResponseValidation(t *testing.T) {
	tests := []struct {
		name    string
		resp    CreateTaskResponse
		wantErr error
	}{
		{
			name: "valid response with created status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CreateTaskStatusCreated,
			},
			wantErr: nil,
		},
		{
			name: "valid response with queued status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				TaskID:        "task-001",
				Status:        CreateTaskStatusQueued,
				QueuePosition: 5,
			},
			wantErr: nil,
		},
		{
			name: "valid response with rejected status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				Status: CreateTaskStatusRejected,
				Error:  "Validation failed",
			},
			wantErr: nil,
		},
		{
			name: "valid response with failed status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				Status: CreateTaskStatusFailed,
				Error:  "Internal error",
			},
			wantErr: nil,
		},
		{
			name: "missing type",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CreateTaskStatusCreated,
			},
			wantErr: ErrMissingType,
		},
		{
			name: "wrong type",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      "wrong_type",
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CreateTaskStatusCreated,
			},
			wantErr: ErrUnknownType,
		},
		{
			name: "missing request_id",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type: TypeCreateTaskResponse,
				},
				TaskID: "task-001",
				Status: CreateTaskStatusCreated,
			},
			wantErr: ErrMissingRequestID,
		},
		{
			name: "missing status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
			},
			wantErr: ErrInvalidCreateTaskStatus,
		},
		{
			name: "invalid status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				TaskID: "task-001",
				Status: CreateTaskStatus("invalid"),
			},
			wantErr: ErrInvalidCreateTaskStatus,
		},
		{
			name: "missing task_id for created status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				Status: CreateTaskStatusCreated,
			},
			wantErr: ErrMissingTaskID,
		},
		{
			name: "missing task_id for queued status",
			resp: CreateTaskResponse{
				BaseMessage: BaseMessage{
					Type:      TypeCreateTaskResponse,
					RequestID: "valid-id",
				},
				Status: CreateTaskStatusQueued,
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

func TestCreateTaskRequestMarshalJSON(t *testing.T) {
	req := NewCreateTaskRequest("req-001", "My Task", "Task description")
	req.Priority = TaskPriorityHigh
	req.ProjectID = "proj-001"
	req.Tags = []string{"urgent", "backend"}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"create_task_request"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"req-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"title":"My Task"`) {
		t.Errorf("expected title field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"description":"Task description"`) {
		t.Errorf("expected description field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"priority":"high"`) {
		t.Errorf("expected priority field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"project_id":"proj-001"`) {
		t.Errorf("expected project_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"tags":["urgent","backend"]`) {
		t.Errorf("expected tags field in JSON, got %s", jsonStr)
	}
}

func TestCreateTaskRequestUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "create_task_request",
		"request_id": "req-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"title": "New Task",
		"description": "Create something new",
		"priority": "critical",
		"project_id": "proj-002",
		"tags": ["feature", "v2"],
		"depends_on": ["task-001"],
		"timeout_seconds": 7200
	}`

	var req CreateTaskRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if req.Type != TypeCreateTaskRequest {
		t.Errorf("expected type %s, got %s", TypeCreateTaskRequest, req.Type)
	}
	if req.RequestID != "req-002" {
		t.Errorf("expected request_id req-002, got %s", req.RequestID)
	}
	if req.Title != "New Task" {
		t.Errorf("expected title 'New Task', got %s", req.Title)
	}
	if req.Description != "Create something new" {
		t.Errorf("expected description 'Create something new', got %s", req.Description)
	}
	if req.Priority != TaskPriorityCritical {
		t.Errorf("expected priority %s, got %s", TaskPriorityCritical, req.Priority)
	}
	if req.ProjectID != "proj-002" {
		t.Errorf("expected project_id proj-002, got %s", req.ProjectID)
	}
	if len(req.Tags) != 2 || req.Tags[0] != "feature" || req.Tags[1] != "v2" {
		t.Errorf("expected tags [feature, v2], got %v", req.Tags)
	}
	if len(req.DependsOn) != 1 || req.DependsOn[0] != "task-001" {
		t.Errorf("expected depends_on [task-001], got %v", req.DependsOn)
	}
	if req.TimeoutSeconds != 7200 {
		t.Errorf("expected timeout_seconds 7200, got %d", req.TimeoutSeconds)
	}
}

func TestCreateTaskResponseMarshalJSON(t *testing.T) {
	resp := NewCreateTaskResponse("resp-001", "task-new-001", CreateTaskStatusQueued, "Task queued")
	resp.QueuePosition = 3

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"type":"create_task_response"`) {
		t.Errorf("expected type field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"request_id":"resp-001"`) {
		t.Errorf("expected request_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"task_id":"task-new-001"`) {
		t.Errorf("expected task_id field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"status":"queued"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"message":"Task queued"`) {
		t.Errorf("expected message field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"queue_position":3`) {
		t.Errorf("expected queue_position field in JSON, got %s", jsonStr)
	}
}

func TestCreateTaskResponseMarshalJSONWithError(t *testing.T) {
	resp := NewCreateTaskErrorResponse("resp-001", CreateTaskStatusRejected, "Invalid task configuration")

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"status":"rejected"`) {
		t.Errorf("expected status field in JSON, got %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"error":"Invalid task configuration"`) {
		t.Errorf("expected error field in JSON, got %s", jsonStr)
	}
}

func TestCreateTaskResponseUnmarshalJSON(t *testing.T) {
	jsonStr := `{
		"type": "create_task_response",
		"request_id": "resp-002",
		"timestamp": "2024-01-15T10:30:00Z",
		"task_id": "task-002",
		"status": "created",
		"message": "Task created successfully"
	}`

	var resp CreateTaskResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Type != TypeCreateTaskResponse {
		t.Errorf("expected type %s, got %s", TypeCreateTaskResponse, resp.Type)
	}
	if resp.RequestID != "resp-002" {
		t.Errorf("expected request_id resp-002, got %s", resp.RequestID)
	}
	if resp.TaskID != "task-002" {
		t.Errorf("expected task_id task-002, got %s", resp.TaskID)
	}
	if resp.Status != CreateTaskStatusCreated {
		t.Errorf("expected status %s, got %s", CreateTaskStatusCreated, resp.Status)
	}
	if resp.Message != "Task created successfully" {
		t.Errorf("expected message 'Task created successfully', got %s", resp.Message)
	}
}

func TestCreateTaskResponseUnmarshalJSONWithError(t *testing.T) {
	jsonStr := `{
		"type": "create_task_response",
		"request_id": "resp-003",
		"status": "failed",
		"error": "Database connection failed"
	}`

	var resp CreateTaskResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if resp.Status != CreateTaskStatusFailed {
		t.Errorf("expected status %s, got %s", CreateTaskStatusFailed, resp.Status)
	}
	if resp.Error != "Database connection failed" {
		t.Errorf("expected error 'Database connection failed', got %s", resp.Error)
	}
}

func TestCreateTaskRequestRoundTrip(t *testing.T) {
	original := NewCreateTaskRequest("roundtrip-001", "Round Trip Task", "Testing round trip")
	original.Timestamp = "2024-01-15T10:00:00Z"
	original.Priority = TaskPriorityHigh
	original.ProjectID = "proj-rt"
	original.Tags = []string{"test", "roundtrip"}
	original.DependsOn = []string{"dep-001"}
	original.TimeoutSeconds = 1800
	original.Metadata = map[string]interface{}{"key": "value"}

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed CreateTaskRequest
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
	if parsed.Title != original.Title {
		t.Errorf("Title mismatch: %s != %s", parsed.Title, original.Title)
	}
	if parsed.Description != original.Description {
		t.Errorf("Description mismatch: %s != %s", parsed.Description, original.Description)
	}
	if parsed.Priority != original.Priority {
		t.Errorf("Priority mismatch: %s != %s", parsed.Priority, original.Priority)
	}
	if parsed.ProjectID != original.ProjectID {
		t.Errorf("ProjectID mismatch: %s != %s", parsed.ProjectID, original.ProjectID)
	}
	if parsed.TimeoutSeconds != original.TimeoutSeconds {
		t.Errorf("TimeoutSeconds mismatch: %d != %d", parsed.TimeoutSeconds, original.TimeoutSeconds)
	}
}

func TestCreateTaskResponseRoundTrip(t *testing.T) {
	original := NewCreateTaskResponse("roundtrip-002", "task-rt", CreateTaskStatusQueued, "Queued for processing")
	original.Timestamp = "2024-01-15T11:00:00Z"
	original.QueuePosition = 7

	// Marshal
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	// Unmarshal
	var parsed CreateTaskResponse
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
	if parsed.QueuePosition != original.QueuePosition {
		t.Errorf("QueuePosition mismatch: %d != %d", parsed.QueuePosition, original.QueuePosition)
	}
}

func TestTaskPriorityConstants(t *testing.T) {
	// Ensure priority constants have expected string values
	if TaskPriorityLow != "low" {
		t.Errorf("TaskPriorityLow should be 'low', got %s", TaskPriorityLow)
	}
	if TaskPriorityNormal != "normal" {
		t.Errorf("TaskPriorityNormal should be 'normal', got %s", TaskPriorityNormal)
	}
	if TaskPriorityHigh != "high" {
		t.Errorf("TaskPriorityHigh should be 'high', got %s", TaskPriorityHigh)
	}
	if TaskPriorityCritical != "critical" {
		t.Errorf("TaskPriorityCritical should be 'critical', got %s", TaskPriorityCritical)
	}
}

func TestCreateTaskStatusConstants(t *testing.T) {
	// Ensure status constants have expected string values
	if CreateTaskStatusCreated != "created" {
		t.Errorf("CreateTaskStatusCreated should be 'created', got %s", CreateTaskStatusCreated)
	}
	if CreateTaskStatusQueued != "queued" {
		t.Errorf("CreateTaskStatusQueued should be 'queued', got %s", CreateTaskStatusQueued)
	}
	if CreateTaskStatusRejected != "rejected" {
		t.Errorf("CreateTaskStatusRejected should be 'rejected', got %s", CreateTaskStatusRejected)
	}
	if CreateTaskStatusFailed != "failed" {
		t.Errorf("CreateTaskStatusFailed should be 'failed', got %s", CreateTaskStatusFailed)
	}
}

func TestCreateTaskMessageTypeConstants(t *testing.T) {
	// Ensure message type constants have expected values
	if TypeCreateTaskRequest != "create_task_request" {
		t.Errorf("TypeCreateTaskRequest should be 'create_task_request', got %s", TypeCreateTaskRequest)
	}
	if TypeCreateTaskResponse != "create_task_response" {
		t.Errorf("TypeCreateTaskResponse should be 'create_task_response', got %s", TypeCreateTaskResponse)
	}
}

func TestCreateTaskRequestWithMetadata(t *testing.T) {
	req := NewCreateTaskRequest("req-meta", "Metadata Task", "Task with metadata")
	req.Metadata = map[string]interface{}{
		"source":   "api",
		"priority": 10,
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed CreateTaskRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Metadata == nil {
		t.Fatal("expected metadata to be present")
	}
	if parsed.Metadata["source"] != "api" {
		t.Errorf("expected metadata source 'api', got %v", parsed.Metadata["source"])
	}
}

func TestCreateTaskRequestTimeoutValidation(t *testing.T) {
	req := CreateTaskRequest{
		BaseMessage: BaseMessage{
			Type:      TypeCreateTaskRequest,
			RequestID: "valid-id",
		},
		Title:          "Test Task",
		Description:    "A test task",
		TimeoutSeconds: -1,
	}

	err := req.Validate()
	if err == nil {
		t.Error("expected error for negative timeout")
	}
}

func TestParseMessageCreateTaskRequest(t *testing.T) {
	input := `{"type": "create_task_request", "request_id": "req-001", "title": "Test Task", "description": "Do something"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeCreateTaskRequest {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeCreateTaskRequest)
	}

	req, ok := result.(*CreateTaskRequest)
	if !ok {
		t.Fatal("expected *CreateTaskRequest")
	}
	if req.RequestID != "req-001" {
		t.Errorf("expected request_id req-001, got %s", req.RequestID)
	}
	if req.Title != "Test Task" {
		t.Errorf("expected title 'Test Task', got %s", req.Title)
	}
	if req.Description != "Do something" {
		t.Errorf("expected description 'Do something', got %s", req.Description)
	}
}

func TestParseMessageCreateTaskResponse(t *testing.T) {
	input := `{"type": "create_task_response", "request_id": "resp-001", "task_id": "task-001", "status": "created"}`

	result, msgType, err := ParseMessage([]byte(input))
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	if msgType != TypeCreateTaskResponse {
		t.Errorf("ParseMessage() type = %v, want %v", msgType, TypeCreateTaskResponse)
	}

	resp, ok := result.(*CreateTaskResponse)
	if !ok {
		t.Fatal("expected *CreateTaskResponse")
	}
	if resp.TaskID != "task-001" {
		t.Errorf("expected task_id task-001, got %s", resp.TaskID)
	}
	if resp.Status != CreateTaskStatusCreated {
		t.Errorf("expected status created, got %s", resp.Status)
	}
}
