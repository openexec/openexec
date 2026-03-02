package loop

import (
	"encoding/json"
	"testing"
	"time"
)

// --- LoopEventType Tests ---

func TestLoopEventType_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		eventType LoopEventType
		want     bool
	}{
		// Lifecycle events
		{"valid loop.start", LoopEventStart, true},
		{"valid loop.pause", LoopEventPause, true},
		{"valid loop.resume", LoopEventResume, true},
		{"valid loop.stop", LoopEventStop, true},
		{"valid loop.complete", LoopEventComplete, true},
		{"valid loop.error", LoopEventError, true},
		{"valid loop.timeout", LoopEventTimeout, true},
		{"valid loop.max_reached", LoopEventMaxReached, true},

		// Iteration events
		{"valid iteration.start", IterationStart, true},
		{"valid iteration.complete", IterationComplete, true},
		{"valid iteration.retry", IterationRetry, true},
		{"valid iteration.skip", IterationSkip, true},

		// LLM events
		{"valid llm.request_start", LLMRequestStart, true},
		{"valid llm.request_end", LLMRequestEnd, true},
		{"valid llm.stream_start", LLMStreamStart, true},
		{"valid llm.stream_chunk", LLMStreamChunk, true},
		{"valid llm.stream_end", LLMStreamEnd, true},
		{"valid llm.error", LLMError, true},
		{"valid llm.rate_limit", LLMRateLimit, true},
		{"valid llm.context_window", LLMContextWindow, true},

		// Tool events
		{"valid tool.call_requested", ToolCallRequested, true},
		{"valid tool.call_queued", ToolCallQueued, true},
		{"valid tool.call_approved", ToolCallApproved, true},
		{"valid tool.call_rejected", ToolCallRejected, true},
		{"valid tool.call_start", ToolCallStart, true},
		{"valid tool.call_progress", ToolCallProgress, true},
		{"valid tool.call_complete", ToolCallComplete, true},
		{"valid tool.call_error", ToolCallError, true},
		{"valid tool.call_timeout", ToolCallTimeout, true},
		{"valid tool.call_cancelled", ToolCallCancelled, true},
		{"valid tool.result_sent", ToolResultSent, true},
		{"valid tool.auto_approved", ToolAutoApproved, true},

		// Context events
		{"valid context.injected", ContextInjected, true},
		{"valid context.truncated", ContextTruncated, true},
		{"valid context.summarized", ContextSummarized, true},
		{"valid context.refreshed", ContextRefreshed, true},

		// Message events
		{"valid message.user", MessageUser, true},
		{"valid message.assistant", MessageAssistant, true},
		{"valid message.system", MessageSystem, true},

		// Gate events
		{"valid gate.check_start", GateCheckStart, true},
		{"valid gate.check_pass", GateCheckPass, true},
		{"valid gate.check_fail", GateCheckFail, true},
		{"valid gate.fix_start", GateFixStart, true},
		{"valid gate.fix_success", GateFixSuccess, true},
		{"valid gate.fix_fail", GateFixFail, true},

		// Signal events
		{"valid signal.received", SignalReceived, true},
		{"valid signal.sent", SignalSent, true},
		{"valid signal.phase_complete", SignalPhaseComplete, true},

		// Cost events
		{"valid cost.updated", CostUpdated, true},
		{"valid cost.budget_warn", CostBudgetWarn, true},
		{"valid cost.budget_exceeded", CostBudgetExceeded, true},

		// Session events
		{"valid session.created", SessionCreated, true},
		{"valid session.restored", SessionRestored, true},
		{"valid session.persisted", SessionPersisted, true},
		{"valid session.forked", SessionForked, true},

		// Thrashing events
		{"valid thrashing.detected", ThrashingDetected, true},
		{"valid thrashing.resolved", ThrashingResolved, true},

		// Invalid events
		{"invalid empty", LoopEventType(""), false},
		{"invalid unknown", LoopEventType("unknown.event"), false},
		{"invalid malformed", LoopEventType("notanevent"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.eventType.IsValid(); got != tt.want {
				t.Errorf("LoopEventType(%q).IsValid() = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestLoopEventType_String(t *testing.T) {
	tests := []struct {
		eventType LoopEventType
		want      string
	}{
		{LoopEventStart, "loop.start"},
		{ToolCallComplete, "tool.call_complete"},
		{LLMRequestEnd, "llm.request_end"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.eventType.String(); got != tt.want {
				t.Errorf("LoopEventType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoopEventType_Category(t *testing.T) {
	tests := []struct {
		eventType LoopEventType
		want      string
	}{
		{LoopEventStart, "loop"},
		{IterationStart, "iteration"},
		{LLMRequestStart, "llm"},
		{ToolCallComplete, "tool"},
		{ContextInjected, "context"},
		{MessageUser, "message"},
		{GateCheckStart, "gate"},
		{SignalReceived, "signal"},
		{CostUpdated, "cost"},
		{SessionCreated, "session"},
		{ThrashingDetected, "thrashing"},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if got := tt.eventType.Category(); got != tt.want {
				t.Errorf("LoopEventType(%q).Category() = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}

func TestLoopEventType_IsTerminal(t *testing.T) {
	terminalEvents := []LoopEventType{
		LoopEventComplete,
		LoopEventError,
		LoopEventTimeout,
		LoopEventMaxReached,
		LoopEventStop,
	}

	nonTerminalEvents := []LoopEventType{
		LoopEventStart,
		LoopEventPause,
		LoopEventResume,
		IterationStart,
		ToolCallStart,
		LLMRequestStart,
	}

	for _, et := range terminalEvents {
		if !et.IsTerminal() {
			t.Errorf("LoopEventType(%q).IsTerminal() = false, want true", et)
		}
	}

	for _, et := range nonTerminalEvents {
		if et.IsTerminal() {
			t.Errorf("LoopEventType(%q).IsTerminal() = true, want false", et)
		}
	}
}

func TestLoopEventType_RequiresAction(t *testing.T) {
	actionEvents := []LoopEventType{
		ToolCallQueued,
		GateCheckFail,
		CostBudgetExceeded,
		ThrashingDetected,
	}

	noActionEvents := []LoopEventType{
		LoopEventStart,
		ToolCallComplete,
		LLMRequestEnd,
		IterationStart,
	}

	for _, et := range actionEvents {
		if !et.RequiresAction() {
			t.Errorf("LoopEventType(%q).RequiresAction() = false, want true", et)
		}
	}

	for _, et := range noActionEvents {
		if et.RequiresAction() {
			t.Errorf("LoopEventType(%q).RequiresAction() = true, want false", et)
		}
	}
}

func TestLoopEventType_GetKind(t *testing.T) {
	tests := []struct {
		eventType LoopEventType
		want      EventKind
	}{
		{LoopEventStart, EventKindLifecycle},
		{LoopEventComplete, EventKindLifecycle},
		{IterationStart, EventKindIteration},
		{LLMRequestStart, EventKindLLM},
		{ToolCallComplete, EventKindTool},
		{ContextInjected, EventKindContext},
		{MessageUser, EventKindMessage},
		{GateCheckStart, EventKindGate},
		{SignalReceived, EventKindSignal},
		{CostUpdated, EventKindCost},
		{SessionCreated, EventKindSession},
		{ThrashingDetected, EventKindThrashing},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			if got := tt.eventType.GetKind(); got != tt.want {
				t.Errorf("LoopEventType(%q).GetKind() = %v, want %v", tt.eventType, got, tt.want)
			}
		})
	}
}

// --- EventKind Tests ---

func TestEventKind_IsValid(t *testing.T) {
	validKinds := []EventKind{
		EventKindLifecycle,
		EventKindIteration,
		EventKindLLM,
		EventKindTool,
		EventKindContext,
		EventKindMessage,
		EventKindGate,
		EventKindSignal,
		EventKindCost,
		EventKindSession,
		EventKindThrashing,
	}

	for _, k := range validKinds {
		if !k.IsValid() {
			t.Errorf("EventKind(%q).IsValid() = false, want true", k)
		}
	}

	invalidKinds := []EventKind{
		EventKind(""),
		EventKind("unknown"),
		EventKind("invalid"),
	}

	for _, k := range invalidKinds {
		if k.IsValid() {
			t.Errorf("EventKind(%q).IsValid() = true, want false", k)
		}
	}
}

func TestEventKind_String(t *testing.T) {
	tests := []struct {
		kind EventKind
		want string
	}{
		{EventKindLifecycle, "lifecycle"},
		{EventKindTool, "tool"},
		{EventKindLLM, "llm"},
	}

	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("EventKind.String() = %v, want %v", got, tt.want)
		}
	}
}

// --- ToolCallStatus Tests ---

func TestToolCallStatus_IsValid(t *testing.T) {
	validStatuses := []ToolCallStatus{
		ToolCallStatusPending,
		ToolCallStatusApproved,
		ToolCallStatusRejected,
		ToolCallStatusRunning,
		ToolCallStatusCompleted,
		ToolCallStatusFailed,
		ToolCallStatusTimeout,
		ToolCallStatusCancelled,
		ToolCallStatusAutoApproved,
	}

	for _, s := range validStatuses {
		if !s.IsValid() {
			t.Errorf("ToolCallStatus(%q).IsValid() = false, want true", s)
		}
	}

	invalidStatuses := []ToolCallStatus{
		ToolCallStatus(""),
		ToolCallStatus("unknown"),
	}

	for _, s := range invalidStatuses {
		if s.IsValid() {
			t.Errorf("ToolCallStatus(%q).IsValid() = true, want false", s)
		}
	}
}

func TestToolCallStatus_IsFinal(t *testing.T) {
	finalStatuses := []ToolCallStatus{
		ToolCallStatusCompleted,
		ToolCallStatusFailed,
		ToolCallStatusTimeout,
		ToolCallStatusCancelled,
		ToolCallStatusRejected,
	}

	nonFinalStatuses := []ToolCallStatus{
		ToolCallStatusPending,
		ToolCallStatusApproved,
		ToolCallStatusRunning,
		ToolCallStatusAutoApproved,
	}

	for _, s := range finalStatuses {
		if !s.IsFinal() {
			t.Errorf("ToolCallStatus(%q).IsFinal() = false, want true", s)
		}
	}

	for _, s := range nonFinalStatuses {
		if s.IsFinal() {
			t.Errorf("ToolCallStatus(%q).IsFinal() = true, want false", s)
		}
	}
}

func TestToolCallStatus_IsSuccess(t *testing.T) {
	if !ToolCallStatusCompleted.IsSuccess() {
		t.Error("ToolCallStatusCompleted.IsSuccess() = false, want true")
	}

	nonSuccessStatuses := []ToolCallStatus{
		ToolCallStatusPending,
		ToolCallStatusFailed,
		ToolCallStatusTimeout,
		ToolCallStatusCancelled,
		ToolCallStatusRejected,
	}

	for _, s := range nonSuccessStatuses {
		if s.IsSuccess() {
			t.Errorf("ToolCallStatus(%q).IsSuccess() = true, want false", s)
		}
	}
}

// --- ToolCallInfo Tests ---

func TestNewToolCallInfo(t *testing.T) {
	// With provided ID
	info := NewToolCallInfo("test-id", "read_file")
	if info.ID != "test-id" {
		t.Errorf("ToolCallInfo.ID = %q, want %q", info.ID, "test-id")
	}
	if info.Name != "read_file" {
		t.Errorf("ToolCallInfo.Name = %q, want %q", info.Name, "read_file")
	}
	if info.Status != ToolCallStatusPending {
		t.Errorf("ToolCallInfo.Status = %q, want %q", info.Status, ToolCallStatusPending)
	}

	// With empty ID (should generate UUID)
	info2 := NewToolCallInfo("", "write_file")
	if info2.ID == "" {
		t.Error("ToolCallInfo.ID should not be empty when no ID provided")
	}
	if len(info2.ID) != 36 { // UUID format
		t.Errorf("ToolCallInfo.ID should be UUID format, got %q", info2.ID)
	}
}

func TestToolCallInfo_SetInput(t *testing.T) {
	info := NewToolCallInfo("test-id", "test_tool")

	input := map[string]string{"path": "/test/file.txt"}
	if err := info.SetInput(input); err != nil {
		t.Fatalf("SetInput() error = %v", err)
	}

	if info.Input == nil {
		t.Fatal("Input should not be nil after SetInput")
	}

	var decoded map[string]string
	if err := json.Unmarshal(info.Input, &decoded); err != nil {
		t.Fatalf("Unmarshal input error = %v", err)
	}

	if decoded["path"] != "/test/file.txt" {
		t.Errorf("Input path = %q, want %q", decoded["path"], "/test/file.txt")
	}
}

func TestToolCallInfo_GetInput(t *testing.T) {
	info := NewToolCallInfo("test-id", "test_tool")

	input := map[string]interface{}{"path": "/test/file.txt", "count": 42.0}
	if err := info.SetInput(input); err != nil {
		t.Fatalf("SetInput() error = %v", err)
	}

	var decoded map[string]interface{}
	if err := info.GetInput(&decoded); err != nil {
		t.Fatalf("GetInput() error = %v", err)
	}

	if decoded["path"] != "/test/file.txt" {
		t.Errorf("Decoded path = %v, want %q", decoded["path"], "/test/file.txt")
	}
	if decoded["count"] != 42.0 {
		t.Errorf("Decoded count = %v, want 42.0", decoded["count"])
	}

	// Test GetInput with nil input
	info2 := NewToolCallInfo("test-id-2", "test_tool")
	var decoded2 map[string]interface{}
	if err := info2.GetInput(&decoded2); err != nil {
		t.Errorf("GetInput() on nil input should not error, got %v", err)
	}
}

// --- LoopEventBuilder Tests ---

func TestNewLoopEvent(t *testing.T) {
	// Valid event type
	builder, err := NewLoopEvent(ToolCallStart)
	if err != nil {
		t.Fatalf("NewLoopEvent() error = %v", err)
	}
	if builder == nil {
		t.Fatal("NewLoopEvent() returned nil builder")
	}

	// Invalid event type
	_, err = NewLoopEvent(LoopEventType("invalid"))
	if err == nil {
		t.Error("NewLoopEvent() should error on invalid event type")
	}
}

func TestLoopEventBuilder_Build(t *testing.T) {
	builder, err := NewLoopEvent(ToolCallComplete)
	if err != nil {
		t.Fatalf("NewLoopEvent() error = %v", err)
	}

	toolInfo := NewToolCallInfo("tool-123", "read_file")
	toolInfo.Status = ToolCallStatusCompleted
	toolInfo.Output = "file contents"

	event, err := builder.
		WithSession("session-123").
		WithIteration(5).
		WithMessage("Tool execution completed").
		WithToolCall(toolInfo).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Type != ToolCallComplete {
		t.Errorf("Event.Type = %v, want %v", event.Type, ToolCallComplete)
	}
	if event.Kind != EventKindTool {
		t.Errorf("Event.Kind = %v, want %v", event.Kind, EventKindTool)
	}
	if event.SessionID != "session-123" {
		t.Errorf("Event.SessionID = %q, want %q", event.SessionID, "session-123")
	}
	if event.Iteration != 5 {
		t.Errorf("Event.Iteration = %d, want 5", event.Iteration)
	}
	if event.Message != "Tool execution completed" {
		t.Errorf("Event.Message = %q, want %q", event.Message, "Tool execution completed")
	}
	if event.ToolCall == nil {
		t.Fatal("Event.ToolCall should not be nil")
	}
	if event.ToolCall.ID != "tool-123" {
		t.Errorf("Event.ToolCall.ID = %q, want %q", event.ToolCall.ID, "tool-123")
	}
	if event.ID == "" {
		t.Error("Event.ID should not be empty")
	}
	if event.Timestamp.IsZero() {
		t.Error("Event.Timestamp should not be zero")
	}
}

func TestLoopEventBuilder_WithError(t *testing.T) {
	builder, _ := NewLoopEvent(ToolCallError)

	testErr := &testError{msg: "test error"}
	event, err := builder.
		WithError(testErr).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Error != "test error" {
		t.Errorf("Event.Error = %q, want %q", event.Error, "test error")
	}

	// Test with nil error
	builder2, _ := NewLoopEvent(ToolCallComplete)
	event2, err := builder2.WithError(nil).Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if event2.Error != "" {
		t.Errorf("Event.Error with nil should be empty, got %q", event2.Error)
	}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}

func TestLoopEventBuilder_WithLLMRequest(t *testing.T) {
	builder, _ := NewLoopEvent(LLMRequestEnd)

	llmInfo := &LLMRequestInfo{
		Provider:     "openai",
		Model:        "gpt-4",
		InputTokens:  1000,
		OutputTokens: 500,
		TotalTokens:  1500,
		CostUSD:      0.05,
		DurationMs:   2500,
		StopReason:   "end_turn",
	}

	event, err := builder.
		WithLLMRequest(llmInfo).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.LLMRequest == nil {
		t.Fatal("Event.LLMRequest should not be nil")
	}
	if event.LLMRequest.Provider != "openai" {
		t.Errorf("LLMRequest.Provider = %q, want %q", event.LLMRequest.Provider, "openai")
	}
	if event.LLMRequest.InputTokens != 1000 {
		t.Errorf("LLMRequest.InputTokens = %d, want 1000", event.LLMRequest.InputTokens)
	}
}

func TestLoopEventBuilder_WithCost(t *testing.T) {
	builder, _ := NewLoopEvent(CostUpdated)

	costInfo := &CostInfo{
		SessionTotal:      0.25,
		IterationCost:     0.05,
		BudgetLimit:       1.00,
		BudgetRemaining:   0.75,
		BudgetPercent:     25.0,
		TotalTokensInput:  5000,
		TotalTokensOutput: 2500,
	}

	event, err := builder.
		WithCost(costInfo).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Cost == nil {
		t.Fatal("Event.Cost should not be nil")
	}
	if event.Cost.SessionTotal != 0.25 {
		t.Errorf("Cost.SessionTotal = %f, want 0.25", event.Cost.SessionTotal)
	}
}

func TestLoopEventBuilder_WithContext(t *testing.T) {
	builder, _ := NewLoopEvent(ContextTruncated)

	ctxInfo := &ContextInfo{
		TokenCount:      8000,
		MaxTokens:       10000,
		UsagePercent:    80.0,
		WasTruncated:    true,
		TruncatedTokens: 2000,
		SourceFiles:     []string{"file1.go", "file2.go"},
	}

	event, err := builder.
		WithContext(ctxInfo).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Context == nil {
		t.Fatal("Event.Context should not be nil")
	}
	if !event.Context.WasTruncated {
		t.Error("Context.WasTruncated should be true")
	}
}

func TestLoopEventBuilder_WithGate(t *testing.T) {
	builder, _ := NewLoopEvent(GateCheckFail)

	gateInfo := &GateInfo{
		GateName:       "lint",
		Passed:         false,
		Message:        "3 lint errors found",
		FixAttempt:     1,
		MaxFixAttempts: 3,
		Details:        map[string]interface{}{"errors": 3},
	}

	event, err := builder.
		WithGate(gateInfo).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Gate == nil {
		t.Fatal("Event.Gate should not be nil")
	}
	if event.Gate.GateName != "lint" {
		t.Errorf("Gate.GateName = %q, want %q", event.Gate.GateName, "lint")
	}
}

func TestLoopEventBuilder_WithSignal(t *testing.T) {
	builder, _ := NewLoopEvent(SignalReceived)

	signalInfo := &SignalInfo{
		SignalType: "phase-complete",
		Reason:     "Task completed successfully",
		Metadata:   map[string]interface{}{"duration": 120},
	}

	event, err := builder.
		WithSignal(signalInfo).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Signal == nil {
		t.Fatal("Event.Signal should not be nil")
	}
	if event.Signal.SignalType != "phase-complete" {
		t.Errorf("Signal.SignalType = %q, want %q", event.Signal.SignalType, "phase-complete")
	}
}

func TestLoopEventBuilder_WithMetadata(t *testing.T) {
	builder, _ := NewLoopEvent(ToolCallComplete)

	metadata := map[string]interface{}{
		"custom_field": "custom_value",
		"count":        42,
	}

	event, err := builder.
		WithMetadata(metadata).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if event.Metadata == nil {
		t.Fatal("Event.Metadata should not be nil")
	}
	if event.Metadata["custom_field"] != "custom_value" {
		t.Errorf("Metadata[custom_field] = %v, want %q", event.Metadata["custom_field"], "custom_value")
	}
}

func TestLoopEventBuilder_WithTimestamp(t *testing.T) {
	builder, _ := NewLoopEvent(ToolCallStart)

	customTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	event, err := builder.
		WithTimestamp(customTime).
		Build()

	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if !event.Timestamp.Equal(customTime) {
		t.Errorf("Event.Timestamp = %v, want %v", event.Timestamp, customTime)
	}
}

// --- LoopEvent Tests ---

func TestLoopEvent_Validate(t *testing.T) {
	// Valid event
	event := &LoopEvent{
		ID:        "test-id",
		Type:      ToolCallComplete,
		Kind:      EventKindTool,
		Timestamp: time.Now(),
	}
	if err := event.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}

	// Missing ID
	event2 := &LoopEvent{
		Type:      ToolCallComplete,
		Kind:      EventKindTool,
		Timestamp: time.Now(),
	}
	if err := event2.Validate(); err == nil {
		t.Error("Validate() should error on missing ID")
	}

	// Invalid type
	event3 := &LoopEvent{
		ID:        "test-id",
		Type:      LoopEventType("invalid"),
		Kind:      EventKindTool,
		Timestamp: time.Now(),
	}
	if err := event3.Validate(); err == nil {
		t.Error("Validate() should error on invalid type")
	}

	// Invalid kind
	event4 := &LoopEvent{
		ID:        "test-id",
		Type:      ToolCallComplete,
		Kind:      EventKind("invalid"),
		Timestamp: time.Now(),
	}
	if err := event4.Validate(); err == nil {
		t.Error("Validate() should error on invalid kind")
	}

	// Missing timestamp
	event5 := &LoopEvent{
		ID:   "test-id",
		Type: ToolCallComplete,
		Kind: EventKindTool,
	}
	if err := event5.Validate(); err == nil {
		t.Error("Validate() should error on missing timestamp")
	}
}

func TestLoopEvent_IsTerminal(t *testing.T) {
	terminalEvent := &LoopEvent{Type: LoopEventComplete}
	if !terminalEvent.IsTerminal() {
		t.Error("Event with LoopEventComplete should be terminal")
	}

	nonTerminalEvent := &LoopEvent{Type: ToolCallStart}
	if nonTerminalEvent.IsTerminal() {
		t.Error("Event with ToolCallStart should not be terminal")
	}
}

func TestLoopEvent_RequiresAction(t *testing.T) {
	actionEvent := &LoopEvent{Type: ToolCallQueued}
	if !actionEvent.RequiresAction() {
		t.Error("Event with ToolCallQueued should require action")
	}

	noActionEvent := &LoopEvent{Type: ToolCallComplete}
	if noActionEvent.RequiresAction() {
		t.Error("Event with ToolCallComplete should not require action")
	}
}

// --- EventFilter Tests ---

func TestEventFilter_Matches(t *testing.T) {
	now := time.Now()
	event := &LoopEvent{
		ID:        "test-id",
		Type:      ToolCallComplete,
		Kind:      EventKindTool,
		SessionID: "session-123",
		Iteration: 5,
		Timestamp: now,
		Error:     "test error",
	}

	// Empty filter matches everything
	emptyFilter := &EventFilter{}
	if !emptyFilter.Matches(event) {
		t.Error("Empty filter should match any event")
	}

	// Type filter
	typeFilter := &EventFilter{Types: []LoopEventType{ToolCallComplete}}
	if !typeFilter.Matches(event) {
		t.Error("Type filter should match ToolCallComplete")
	}

	wrongTypeFilter := &EventFilter{Types: []LoopEventType{ToolCallStart}}
	if wrongTypeFilter.Matches(event) {
		t.Error("Type filter should not match ToolCallStart")
	}

	// Kind filter
	kindFilter := &EventFilter{Kinds: []EventKind{EventKindTool}}
	if !kindFilter.Matches(event) {
		t.Error("Kind filter should match EventKindTool")
	}

	wrongKindFilter := &EventFilter{Kinds: []EventKind{EventKindLLM}}
	if wrongKindFilter.Matches(event) {
		t.Error("Kind filter should not match EventKindLLM")
	}

	// Session filter
	sessionFilter := &EventFilter{SessionID: "session-123"}
	if !sessionFilter.Matches(event) {
		t.Error("Session filter should match session-123")
	}

	wrongSessionFilter := &EventFilter{SessionID: "other-session"}
	if wrongSessionFilter.Matches(event) {
		t.Error("Session filter should not match other-session")
	}

	// Time range filter
	timeFilter := &EventFilter{
		Since: now.Add(-1 * time.Hour),
		Until: now.Add(1 * time.Hour),
	}
	if !timeFilter.Matches(event) {
		t.Error("Time filter should match event within range")
	}

	earlyFilter := &EventFilter{Since: now.Add(1 * time.Hour)}
	if earlyFilter.Matches(event) {
		t.Error("Since filter should not match event before timestamp")
	}

	lateFilter := &EventFilter{Until: now.Add(-1 * time.Hour)}
	if lateFilter.Matches(event) {
		t.Error("Until filter should not match event after timestamp")
	}

	// Iteration range filter
	iterFilter := &EventFilter{IterationMin: 3, IterationMax: 10}
	if !iterFilter.Matches(event) {
		t.Error("Iteration filter should match iteration 5")
	}

	iterMinFilter := &EventFilter{IterationMin: 10}
	if iterMinFilter.Matches(event) {
		t.Error("IterationMin filter should not match iteration 5 when min is 10")
	}

	iterMaxFilter := &EventFilter{IterationMax: 3}
	if iterMaxFilter.Matches(event) {
		t.Error("IterationMax filter should not match iteration 5 when max is 3")
	}

	// Include errors filter
	errorFilter := &EventFilter{IncludeErrors: true}
	if !errorFilter.Matches(event) {
		t.Error("IncludeErrors filter should match event with error")
	}

	noErrorEvent := &LoopEvent{
		ID:        "test-id-2",
		Type:      ToolCallComplete,
		Kind:      EventKindTool,
		Timestamp: now,
	}
	if errorFilter.Matches(noErrorEvent) {
		t.Error("IncludeErrors filter should not match event without error")
	}

	// Exclude types filter
	excludeFilter := &EventFilter{ExcludeTypes: []LoopEventType{ToolCallComplete}}
	if excludeFilter.Matches(event) {
		t.Error("ExcludeTypes filter should not match excluded type")
	}

	excludeOtherFilter := &EventFilter{ExcludeTypes: []LoopEventType{ToolCallStart}}
	if !excludeOtherFilter.Matches(event) {
		t.Error("ExcludeTypes filter should match non-excluded type")
	}
}

// --- EventDispatcher Tests ---

func TestEventDispatcher_Subscribe(t *testing.T) {
	dispatcher := NewEventDispatcher()

	var receivedEvents []*LoopEvent

	dispatcher.Subscribe(EventKindTool, func(e *LoopEvent) {
		receivedEvents = append(receivedEvents, e)
	})

	toolEvent := &LoopEvent{Type: ToolCallComplete, Kind: EventKindTool}
	llmEvent := &LoopEvent{Type: LLMRequestEnd, Kind: EventKindLLM}

	dispatcher.Dispatch(toolEvent)
	dispatcher.Dispatch(llmEvent)

	if len(receivedEvents) != 1 {
		t.Errorf("Expected 1 tool event, got %d", len(receivedEvents))
	}

	if receivedEvents[0].Type != ToolCallComplete {
		t.Errorf("Expected ToolCallComplete event, got %v", receivedEvents[0].Type)
	}
}

func TestEventDispatcher_SubscribeAll(t *testing.T) {
	dispatcher := NewEventDispatcher()

	var receivedEvents []*LoopEvent

	dispatcher.SubscribeAll(func(e *LoopEvent) {
		receivedEvents = append(receivedEvents, e)
	})

	toolEvent := &LoopEvent{Type: ToolCallComplete, Kind: EventKindTool}
	llmEvent := &LoopEvent{Type: LLMRequestEnd, Kind: EventKindLLM}

	dispatcher.Dispatch(toolEvent)
	dispatcher.Dispatch(llmEvent)

	if len(receivedEvents) != 2 {
		t.Errorf("Expected 2 events, got %d", len(receivedEvents))
	}
}

func TestEventDispatcher_MultipleHandlers(t *testing.T) {
	dispatcher := NewEventDispatcher()

	var handler1Count, handler2Count int

	dispatcher.Subscribe(EventKindTool, func(e *LoopEvent) {
		handler1Count++
	})

	dispatcher.Subscribe(EventKindTool, func(e *LoopEvent) {
		handler2Count++
	})

	toolEvent := &LoopEvent{Type: ToolCallComplete, Kind: EventKindTool}
	dispatcher.Dispatch(toolEvent)

	if handler1Count != 1 {
		t.Errorf("Handler1 count = %d, want 1", handler1Count)
	}
	if handler2Count != 1 {
		t.Errorf("Handler2 count = %d, want 1", handler2Count)
	}
}

// --- ValidLoopEventTypes Coverage Test ---

func TestValidLoopEventTypes_Coverage(t *testing.T) {
	// Ensure all defined constants are in the ValidLoopEventTypes slice
	expectedCount := 57 // Total number of event types defined

	if len(ValidLoopEventTypes) != expectedCount {
		t.Errorf("ValidLoopEventTypes has %d entries, expected %d", len(ValidLoopEventTypes), expectedCount)
	}

	// Ensure all are valid
	for _, et := range ValidLoopEventTypes {
		if !et.IsValid() {
			t.Errorf("Event type %q in ValidLoopEventTypes is not valid", et)
		}
	}
}

// --- JSON Serialization Tests ---

func TestLoopEvent_JSONSerialization(t *testing.T) {
	toolInfo := NewToolCallInfo("tool-123", "read_file")
	toolInfo.Status = ToolCallStatusCompleted
	toolInfo.Output = "file contents"

	builder, _ := NewLoopEvent(ToolCallComplete)
	event, _ := builder.
		WithSession("session-123").
		WithIteration(5).
		WithMessage("Tool completed").
		WithToolCall(toolInfo).
		Build()

	// Marshal
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal
	var decoded LoopEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify
	if decoded.Type != event.Type {
		t.Errorf("Decoded Type = %v, want %v", decoded.Type, event.Type)
	}
	if decoded.SessionID != event.SessionID {
		t.Errorf("Decoded SessionID = %q, want %q", decoded.SessionID, event.SessionID)
	}
	if decoded.Iteration != event.Iteration {
		t.Errorf("Decoded Iteration = %d, want %d", decoded.Iteration, event.Iteration)
	}
	if decoded.ToolCall == nil {
		t.Fatal("Decoded ToolCall should not be nil")
	}
	if decoded.ToolCall.ID != toolInfo.ID {
		t.Errorf("Decoded ToolCall.ID = %q, want %q", decoded.ToolCall.ID, toolInfo.ID)
	}
}
