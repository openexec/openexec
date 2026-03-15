package loop

// EventType identifies the kind of loop event.
type EventType string

const (
	EventIterationStart       EventType = "iteration_start"
	EventAssistantText        EventType = "assistant_text"
	EventToolStart            EventType = "tool_start"
	EventToolResult           EventType = "tool_result"
	EventRetrying             EventType = "retrying"
	EventError                EventType = "error"
	EventMaxIterationsReached EventType = "max_iterations_reached"
	EventPaused               EventType = "paused"
	EventComplete             EventType = "complete"
	EventSignalReceived       EventType = "signal_received"
	EventThrashingDetected    EventType = "thrashing_detected"

	// Pipeline event types (V4).
	EventPhaseStart        EventType = "phase_start"
	EventPhaseComplete     EventType = "phase_complete"
	EventRouteDecision     EventType = "route_decision"
	EventPipelineComplete  EventType = "pipeline_complete"
	EventOperatorAttention EventType = "operator_attention"

	// Quality gate event types.
	EventPreflightStart  EventType = "preflight_start"
	EventPreflightPassed EventType = "preflight_passed"
	EventPreflightFailed EventType = "preflight_failed"
	EventGatesStart      EventType = "gates_start"
	EventGatesPassed     EventType = "gates_passed"
	EventGatesFailed     EventType = "gates_failed"
	EventGatesFixing     EventType = "gates_fixing"

	// Specialized conflict events.
	EventPlanningMismatch EventType = "planning_mismatch"
	EventUsageRecorded    EventType = "usage_recorded"

	// Heartbeat and progress events.
	EventHeartbeat EventType = "heartbeat"
	EventProgress  EventType = "progress"

	// Blueprint execution events.
	EventBlueprintStart     EventType = "blueprint_start"
	EventBlueprintComplete  EventType = "blueprint_complete"
	EventBlueprintFailed    EventType = "blueprint_failed"
	EventStageStart         EventType = "stage_start"
	EventStageComplete      EventType = "stage_complete"
	EventStageFailed        EventType = "stage_failed"
	EventStageRetry         EventType = "stage_retry"
	EventCheckpointCreated  EventType = "checkpoint_created"
)

// EventKind identifies the high-level category of an event.
type EventKind string

const (
	EventKindLifecycle EventKind = "lifecycle"
	EventKindText      EventKind = "text"
	EventKindTool      EventKind = "tool"
	EventKindSignal    EventKind = "signal"
	EventKindError     EventKind = "error"
	EventKindCost      EventKind = "cost"
	EventKindIteration EventKind = "iteration"
)

// LoopEvent is an alias for Event used in some packages.
type LoopEvent = Event

// CostInfo holds session and total cost information
type CostInfo struct {
	SessionTotal float64 `json:"session_total"`
	TotalUSD     float64 `json:"total_usd"`
}

// StepResult is the constrained output schema for an execution step.
// This is used to enforce determinism and limit excessive agency.
type StepResult struct {
	Status      string            `json:"status"`       // complete, error, pivot, retry
	Reason      string            `json:"reason"`       // explanation for the status
	NextPhase   string            `json:"next_phase"`   // requested transition
	Artifacts   map[string]string `json:"artifacts"`    // hash-addressed results
	Confidence  float64           `json:"confidence"`   // 0.0 to 1.0
	Diagnostics string            `json:"diagnostics"`  // optional internal reasoning
}

// Event represents a single occurrence in the loop lifecycle.
type Event struct {
    Type         EventType              `json:"type"`
    Kind         EventKind              `json:"kind,omitempty"`
    Iteration    int                    `json:"iteration,omitempty"`
    Text         string                 `json:"text,omitempty"`
    Tool         string                 `json:"tool,omitempty"`
    ToolInput    map[string]interface{} `json:"tool_input,omitempty"`
    Err          error                  `json:"-"`
    ErrText      string                 `json:"error,omitempty"`
    SignalType   string                 `json:"signal_type,omitempty"`
    SignalTarget string                 `json:"signal_target,omitempty"`
    Cost         *CostInfo              `json:"cost,omitempty"`
    SessionID    string                 `json:"session_id,omitempty"`

    // Pipeline context fields (V4). Omitted for standalone Loop usage.
    Phase       string `json:"phase,omitempty"`
    FWUID       string `json:"fwu_id,omitempty"`
    Agent       string `json:"agent,omitempty"`
    ReviewCycle int    `json:"review_cycle,omitempty"`
    RouteTarget string `json:"route_target,omitempty"`

    // Result is the constrained output from the runner (V5).
    Result *StepResult `json:"result,omitempty"`

    // Observability fields
    PromptHash string `json:"prompt_hash,omitempty"`

    // CacheKey is a stable hash of context inputs for deterministic replay.
    // Unlike PromptHash (which may vary with formatting), CacheKey is computed
    // from the semantic content: intent, context files, and model parameters.
    CacheKey string `json:"cache_key,omitempty"`

    // Artifacts extracted from tool results (e.g., patch hash/path)
    Artifacts map[string]string `json:"artifacts,omitempty"`

    // Trace context for replay/observability
    TraceID string `json:"trace_id,omitempty"`
    StepID  int    `json:"step_id,omitempty"`

    // Blueprint execution context
    BlueprintID string `json:"blueprint_id,omitempty"`
    StageName   string `json:"stage_name,omitempty"`
    StageType   string `json:"stage_type,omitempty"` // "deterministic" or "agentic"
    Attempt     int    `json:"attempt,omitempty"`
}
