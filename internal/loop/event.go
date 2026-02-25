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
	EventPreflightStart   EventType = "preflight_start"
	EventPreflightPassed  EventType = "preflight_passed"
	EventPreflightFailed  EventType = "preflight_failed"
	EventGatesStart       EventType = "gates_start"
	EventGatesPassed      EventType = "gates_passed"
	EventGatesFailed      EventType = "gates_failed"
	EventGatesFixing      EventType = "gates_fixing"

	// Specialized conflict events.
	EventPlanningMismatch EventType = "planning_mismatch"
)

// Event represents a single occurrence in the loop lifecycle.
type Event struct {
	Type         EventType              `json:"type"`
	Iteration    int                    `json:"iteration,omitempty"`
	Text         string                 `json:"text,omitempty"`
	Tool         string                 `json:"tool,omitempty"`
	ToolInput    map[string]interface{} `json:"tool_input,omitempty"`
	Err          error                  `json:"-"`
	ErrText      string                 `json:"error,omitempty"`
	SignalType   string                 `json:"signal_type,omitempty"`
	SignalTarget string                 `json:"signal_target,omitempty"`

	// Pipeline context fields (V4). Omitted for standalone Loop usage.
	Phase       string `json:"phase,omitempty"`
	FWUID       string `json:"fwu_id,omitempty"`
	Agent       string `json:"agent,omitempty"`
	ReviewCycle int    `json:"review_cycle,omitempty"`
	RouteTarget string `json:"route_target,omitempty"`
}
