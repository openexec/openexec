package audit

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		want      bool
	}{
		{"valid session.created", EventSessionCreated, true},
		{"valid tool_call.requested", EventToolCallRequested, true},
		{"valid llm.request_sent", EventLLMRequestSent, true},
		{"valid security.path_violation", EventSecurityPathViolation, true},
		{"invalid empty", EventType(""), false},
		{"invalid unknown", EventType("unknown.event"), false},
		{"invalid typo", EventType("session.createddd"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.eventType.IsValid(); got != tt.want {
				t.Errorf("EventType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventType_Category(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		want      string
	}{
		{"session category", EventSessionCreated, "session"},
		{"tool_call category", EventToolCallRequested, "tool_call"},
		{"llm category", EventLLMRequestSent, "llm"},
		{"usage category", EventUsageRecorded, "usage"},
		{"security category", EventSecurityPathViolation, "security"},
		{"system category", EventSystemStartup, "system"},
		{"context category", EventContextInjected, "context"},
		{"message category", EventMessageSent, "message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.eventType.Category(); got != tt.want {
				t.Errorf("EventType.Category() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventType_String(t *testing.T) {
	if got := EventSessionCreated.String(); got != "session.created" {
		t.Errorf("EventType.String() = %v, want %v", got, "session.created")
	}
}

func TestSeverity_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     bool
	}{
		{"valid debug", SeverityDebug, true},
		{"valid info", SeverityInfo, true},
		{"valid warning", SeverityWarning, true},
		{"valid error", SeverityError, true},
		{"valid critical", SeverityCritical, true},
		{"invalid empty", Severity(""), false},
		{"invalid unknown", Severity("fatal"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.severity.IsValid(); got != tt.want {
				t.Errorf("Severity.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewEntry(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		actorID   string
		actorType string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid entry",
			eventType: EventSessionCreated,
			actorID:   "user-123",
			actorType: "user",
			wantErr:   false,
		},
		{
			name:      "invalid event type",
			eventType: EventType("invalid"),
			actorID:   "user-123",
			actorType: "user",
			wantErr:   true,
			errMsg:    "invalid event type",
		},
		{
			name:      "empty actor ID",
			eventType: EventSessionCreated,
			actorID:   "",
			actorType: "user",
			wantErr:   true,
			errMsg:    "actor_id is required",
		},
		{
			name:      "empty actor type",
			eventType: EventSessionCreated,
			actorID:   "user-123",
			actorType: "",
			wantErr:   true,
			errMsg:    "actor_type is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder, err := NewEntry(tt.eventType, tt.actorID, tt.actorType)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewEntry() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewEntry() unexpected error: %v", err)
				return
			}
			if builder == nil {
				t.Errorf("NewEntry() returned nil builder")
				return
			}

			entry, err := builder.Build()
			if err != nil {
				t.Errorf("Build() unexpected error: %v", err)
				return
			}
			if entry.ID == "" {
				t.Errorf("Entry.ID should be set")
			}
			if entry.EventType != tt.eventType {
				t.Errorf("Entry.EventType = %v, want %v", entry.EventType, tt.eventType)
			}
			if entry.ActorID != tt.actorID {
				t.Errorf("Entry.ActorID = %v, want %v", entry.ActorID, tt.actorID)
			}
			if entry.ActorType != tt.actorType {
				t.Errorf("Entry.ActorType = %v, want %v", entry.ActorType, tt.actorType)
			}
			if entry.Severity != SeverityInfo {
				t.Errorf("Entry.Severity = %v, want %v (default)", entry.Severity, SeverityInfo)
			}
		})
	}
}

func TestEntryBuilder_WithMethods(t *testing.T) {
	builder, err := NewEntry(EventToolCallCompleted, "user-123", "user")
	if err != nil {
		t.Fatalf("NewEntry() error: %v", err)
	}

	metadata := map[string]interface{}{
		"tool_name": "read_file",
		"file_path": "/path/to/file.txt",
	}

	entry, err := builder.
		WithSeverity(SeverityDebug).
		WithSession("session-456").
		WithMessage("message-789").
		WithToolCall("toolcall-012").
		WithProject("/path/to/project").
		WithProvider("openai", "gpt-4").
		WithTokens(100, 50).
		WithCost(0.005).
		WithDuration(150).
		WithSuccess(true).
		WithIteration(3).
		WithClientInfo("127.0.0.1", "Mozilla/5.0").
		WithMetadata(metadata).
		Build()

	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Verify all fields
	if entry.Severity != SeverityDebug {
		t.Errorf("Entry.Severity = %v, want %v", entry.Severity, SeverityDebug)
	}
	if !entry.SessionID.Valid || entry.SessionID.String != "session-456" {
		t.Errorf("Entry.SessionID = %v, want session-456", entry.SessionID)
	}
	if !entry.MessageID.Valid || entry.MessageID.String != "message-789" {
		t.Errorf("Entry.MessageID = %v, want message-789", entry.MessageID)
	}
	if !entry.ToolCallID.Valid || entry.ToolCallID.String != "toolcall-012" {
		t.Errorf("Entry.ToolCallID = %v, want toolcall-012", entry.ToolCallID)
	}
	if !entry.ProjectPath.Valid || entry.ProjectPath.String != "/path/to/project" {
		t.Errorf("Entry.ProjectPath = %v, want /path/to/project", entry.ProjectPath)
	}
	if !entry.Provider.Valid || entry.Provider.String != "openai" {
		t.Errorf("Entry.Provider = %v, want openai", entry.Provider)
	}
	if !entry.Model.Valid || entry.Model.String != "gpt-4" {
		t.Errorf("Entry.Model = %v, want gpt-4", entry.Model)
	}
	if !entry.TokensInput.Valid || entry.TokensInput.Int64 != 100 {
		t.Errorf("Entry.TokensInput = %v, want 100", entry.TokensInput)
	}
	if !entry.TokensOutput.Valid || entry.TokensOutput.Int64 != 50 {
		t.Errorf("Entry.TokensOutput = %v, want 50", entry.TokensOutput)
	}
	if !entry.CostUSD.Valid || entry.CostUSD.Float64 != 0.005 {
		t.Errorf("Entry.CostUSD = %v, want 0.005", entry.CostUSD)
	}
	if !entry.DurationMs.Valid || entry.DurationMs.Int64 != 150 {
		t.Errorf("Entry.DurationMs = %v, want 150", entry.DurationMs)
	}
	if !entry.Success.Valid || entry.Success.Bool != true {
		t.Errorf("Entry.Success = %v, want true", entry.Success)
	}
	if !entry.Iteration.Valid || entry.Iteration.Int64 != 3 {
		t.Errorf("Entry.Iteration = %v, want 3", entry.Iteration)
	}
	if !entry.IPAddress.Valid || entry.IPAddress.String != "127.0.0.1" {
		t.Errorf("Entry.IPAddress = %v, want 127.0.0.1", entry.IPAddress)
	}
	if !entry.UserAgent.Valid || entry.UserAgent.String != "Mozilla/5.0" {
		t.Errorf("Entry.UserAgent = %v, want Mozilla/5.0", entry.UserAgent)
	}
	if !entry.Metadata.Valid {
		t.Errorf("Entry.Metadata should be valid")
	}

	// Verify metadata can be parsed back
	var parsedMetadata map[string]interface{}
	if err := entry.GetMetadata(&parsedMetadata); err != nil {
		t.Errorf("GetMetadata() error: %v", err)
	}
	if parsedMetadata["tool_name"] != "read_file" {
		t.Errorf("Metadata tool_name = %v, want read_file", parsedMetadata["tool_name"])
	}
}

func TestEntryBuilder_WithError(t *testing.T) {
	builder, err := NewEntry(EventToolCallFailed, "system", "system")
	if err != nil {
		t.Fatalf("NewEntry() error: %v", err)
	}

	entry, err := builder.
		WithError("file not found").
		Build()

	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if !entry.ErrorMessage.Valid || entry.ErrorMessage.String != "file not found" {
		t.Errorf("Entry.ErrorMessage = %v, want 'file not found'", entry.ErrorMessage)
	}
	// WithError should also set Success to false
	if !entry.Success.Valid || entry.Success.Bool != false {
		t.Errorf("Entry.Success = %v, want false", entry.Success)
	}
}

func TestEntryBuilder_WithTimestamp(t *testing.T) {
	builder, err := NewEntry(EventSessionCreated, "user-123", "user")
	if err != nil {
		t.Fatalf("NewEntry() error: %v", err)
	}

	customTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	entry, err := builder.
		WithTimestamp(customTime).
		Build()

	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	if !entry.Timestamp.Equal(customTime) {
		t.Errorf("Entry.Timestamp = %v, want %v", entry.Timestamp, customTime)
	}
}

func TestEntry_Validate(t *testing.T) {
	tests := []struct {
		name    string
		entry   *Entry
		wantErr bool
	}{
		{
			name: "valid entry",
			entry: &Entry{
				ID:        "test-id",
				Timestamp: time.Now(),
				EventType: EventSessionCreated,
				Severity:  SeverityInfo,
				ActorID:   "user-123",
				ActorType: "user",
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			entry: &Entry{
				Timestamp: time.Now(),
				EventType: EventSessionCreated,
				Severity:  SeverityInfo,
				ActorID:   "user-123",
				ActorType: "user",
			},
			wantErr: true,
		},
		{
			name: "invalid event type",
			entry: &Entry{
				ID:        "test-id",
				Timestamp: time.Now(),
				EventType: EventType("invalid"),
				Severity:  SeverityInfo,
				ActorID:   "user-123",
				ActorType: "user",
			},
			wantErr: true,
		},
		{
			name: "invalid severity",
			entry: &Entry{
				ID:        "test-id",
				Timestamp: time.Now(),
				EventType: EventSessionCreated,
				Severity:  Severity("invalid"),
				ActorID:   "user-123",
				ActorType: "user",
			},
			wantErr: true,
		},
		{
			name: "missing actor ID",
			entry: &Entry{
				ID:        "test-id",
				Timestamp: time.Now(),
				EventType: EventSessionCreated,
				Severity:  SeverityInfo,
				ActorID:   "",
				ActorType: "user",
			},
			wantErr: true,
		},
		{
			name: "missing actor type",
			entry: &Entry{
				ID:        "test-id",
				Timestamp: time.Now(),
				EventType: EventSessionCreated,
				Severity:  SeverityInfo,
				ActorID:   "user-123",
				ActorType: "",
			},
			wantErr: true,
		},
		{
			name: "zero timestamp",
			entry: &Entry{
				ID:        "test-id",
				EventType: EventSessionCreated,
				Severity:  SeverityInfo,
				ActorID:   "user-123",
				ActorType: "user",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Entry.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEntry_GetMetadata_Empty(t *testing.T) {
	entry := &Entry{
		ID:        "test-id",
		Timestamp: time.Now(),
		EventType: EventSessionCreated,
		Severity:  SeverityInfo,
		ActorID:   "user-123",
		ActorType: "user",
		// Metadata not set
	}

	var metadata map[string]interface{}
	err := entry.GetMetadata(&metadata)
	if err != nil {
		t.Errorf("GetMetadata() error: %v", err)
	}
	// Should return nil without error when metadata is not set
}

func TestEntry_JSONSerialization(t *testing.T) {
	builder, err := NewEntry(EventLLMResponseReceived, "llm", "system")
	if err != nil {
		t.Fatalf("NewEntry() error: %v", err)
	}

	entry, err := builder.
		WithSession("session-123").
		WithProvider("anthropic", "claude-3-opus").
		WithTokens(500, 1000).
		WithCost(0.03).
		WithDuration(2500).
		WithSuccess(true).
		Build()

	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}

	// Serialize to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	// Verify JSON contains expected fields
	var jsonMap map[string]interface{}
	if err := json.Unmarshal(data, &jsonMap); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if jsonMap["event_type"] != "llm.response_received" {
		t.Errorf("JSON event_type = %v, want llm.response_received", jsonMap["event_type"])
	}
	if jsonMap["actor_id"] != "llm" {
		t.Errorf("JSON actor_id = %v, want llm", jsonMap["actor_id"])
	}
}

func TestValidEventTypes_Coverage(t *testing.T) {
	// Ensure all declared event type constants are in ValidEventTypes
	allEvents := []EventType{
		EventSessionCreated, EventSessionForked, EventSessionPaused,
		EventSessionResumed, EventSessionArchived, EventSessionDeleted,
		EventMessageSent, EventMessageReceived,
		EventToolCallRequested, EventToolCallApproved, EventToolCallRejected,
		EventToolCallAutoApproved, EventToolCallStarted, EventToolCallCompleted,
		EventToolCallFailed, EventToolCallCancelled,
		EventLLMRequestSent, EventLLMResponseReceived, EventLLMStreamChunk, EventLLMError,
		EventUsageRecorded, EventBudgetWarning, EventBudgetExceeded,
		EventContextInjected, EventContextSummarized,
		EventSecurityPathViolation, EventSecurityRateLimit, EventSecurityAuthFailure,
		EventSystemStartup, EventSystemShutdown, EventSystemError,
		EventRunCreated, EventRunStep,
	}

	for _, event := range allEvents {
		if !event.IsValid() {
			t.Errorf("Event %s should be valid but IsValid() returned false", event)
		}
	}

	// Verify count matches
	if len(ValidEventTypes) != len(allEvents) {
		t.Errorf("ValidEventTypes has %d events, expected %d", len(ValidEventTypes), len(allEvents))
	}
}

func TestQueryFilter_Defaults(t *testing.T) {
	filter := QueryFilter{}

	// Verify zero values
	if len(filter.EventTypes) != 0 {
		t.Errorf("QueryFilter.EventTypes should be empty by default")
	}
	if filter.Limit != 0 {
		t.Errorf("QueryFilter.Limit should be 0 by default")
	}
	if !filter.Since.IsZero() {
		t.Errorf("QueryFilter.Since should be zero by default")
	}
}

func TestUsageStats_Structure(t *testing.T) {
	stats := UsageStats{
		TotalTokensInput:   10000,
		TotalTokensOutput:  5000,
		TotalCostUSD:       0.25,
		TotalRequests:      100,
		SuccessfulRequests: 95,
		FailedRequests:     5,
		AverageDurationMs:  250.5,
		ByProvider: map[string]*ProviderStats{
			"openai": {
				Provider:          "openai",
				TotalTokensInput:  6000,
				TotalTokensOutput: 3000,
				TotalCostUSD:      0.15,
				TotalRequests:     60,
			},
			"anthropic": {
				Provider:          "anthropic",
				TotalTokensInput:  4000,
				TotalTokensOutput: 2000,
				TotalCostUSD:      0.10,
				TotalRequests:     40,
			},
		},
	}

	// Verify JSON serialization
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var parsed UsageStats
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if parsed.TotalTokensInput != 10000 {
		t.Errorf("UsageStats.TotalTokensInput = %d, want 10000", parsed.TotalTokensInput)
	}
	if len(parsed.ByProvider) != 2 {
		t.Errorf("UsageStats.ByProvider should have 2 entries")
	}
}

func TestToolCallStats_Structure(t *testing.T) {
	stats := ToolCallStats{
		TotalRequested:    100,
		TotalApproved:     80,
		TotalRejected:     10,
		TotalAutoApproved: 5,
		TotalCompleted:    75,
		TotalFailed:       5,
		ByTool: map[string]int64{
			"read_file":   40,
			"write_file":  30,
			"run_command": 30,
		},
	}

	// Verify JSON serialization
	data, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var parsed ToolCallStats
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if parsed.TotalRequested != 100 {
		t.Errorf("ToolCallStats.TotalRequested = %d, want 100", parsed.TotalRequested)
	}
	if parsed.ByTool["read_file"] != 40 {
		t.Errorf("ToolCallStats.ByTool[read_file] = %d, want 40", parsed.ByTool["read_file"])
	}
}
