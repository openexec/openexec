// Package schema provides JSON Schema validation for typed agent outputs.
// This replaces text-based heuristic detection with structured validation.
package schema

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ActionType represents the type of agent action.
type ActionType string

const (
	ActionComplete   ActionType = "complete"
	ActionPivot      ActionType = "pivot"
	ActionRetry      ActionType = "retry"
	ActionError      ActionType = "error"
	ActionProgress   ActionType = "progress"
	ActionToolCall   ActionType = "tool_call"
	ActionPlanUpdate ActionType = "plan_update"
)

// StepResult is the typed output schema for agent step completion.
// Agents MUST emit this as structured JSON via the openexec_result tool.
type StepResult struct {
	Status      string            `json:"status"`                // complete, error, pivot, retry
	Reason      string            `json:"reason"`                // explanation for the status
	NextPhase   string            `json:"next_phase,omitempty"`  // requested transition
	Artifacts   map[string]string `json:"artifacts,omitempty"`   // hash-addressed results
	Confidence  float64           `json:"confidence,omitempty"`  // 0.0 to 1.0
	Diagnostics string            `json:"diagnostics,omitempty"` // optional internal reasoning
}

// Validate checks the StepResult against the schema constraints.
func (s *StepResult) Validate() error {
	validStatuses := map[string]bool{
		"complete": true,
		"error":    true,
		"pivot":    true,
		"retry":    true,
	}
	if !validStatuses[s.Status] {
		return fmt.Errorf("invalid status %q; must be one of: complete, error, pivot, retry", s.Status)
	}
	if s.Reason == "" {
		return fmt.Errorf("reason is required")
	}
	if s.Confidence < 0 || s.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got %f", s.Confidence)
	}
	return nil
}

// ToolCallRequest is the schema for requesting a tool execution.
type ToolCallRequest struct {
	Tool       string                 `json:"tool"`                  // tool name
	Input      map[string]interface{} `json:"input"`                 // tool parameters
	Idempotent bool                   `json:"idempotent,omitempty"`  // safe to retry
	Priority   string                 `json:"priority,omitempty"`    // low, normal, high
	Timeout    int                    `json:"timeout_ms,omitempty"`  // max execution time
}

// Validate checks the ToolCallRequest against schema constraints.
func (t *ToolCallRequest) Validate() error {
	if t.Tool == "" {
		return fmt.Errorf("tool name is required")
	}
	if t.Input == nil {
		t.Input = make(map[string]interface{})
	}
	validPriorities := map[string]bool{"": true, "low": true, "normal": true, "high": true}
	if !validPriorities[t.Priority] {
		return fmt.Errorf("invalid priority %q; must be one of: low, normal, high", t.Priority)
	}
	return nil
}

// PlanUpdate is the schema for agent plan modifications.
type PlanUpdate struct {
	Action      string   `json:"action"`                 // add, remove, reorder, complete
	TaskIDs     []string `json:"task_ids,omitempty"`     // affected task IDs
	Reason      string   `json:"reason"`                 // explanation for the change
	NewPriority int      `json:"new_priority,omitempty"` // for reorder action
}

// Validate checks the PlanUpdate against schema constraints.
func (p *PlanUpdate) Validate() error {
	validActions := map[string]bool{
		"add":      true,
		"remove":   true,
		"reorder":  true,
		"complete": true,
	}
	if !validActions[p.Action] {
		return fmt.Errorf("invalid action %q; must be one of: add, remove, reorder, complete", p.Action)
	}
	if p.Reason == "" {
		return fmt.Errorf("reason is required")
	}
	return nil
}

// AgentAction is the unified envelope for all typed agent outputs.
type AgentAction struct {
	Type       ActionType       `json:"type"`                  // action type
	StepResult *StepResult      `json:"step_result,omitempty"` // for complete/error/pivot/retry
	ToolCall   *ToolCallRequest `json:"tool_call,omitempty"`   // for tool_call
	PlanUpdate *PlanUpdate      `json:"plan_update,omitempty"` // for plan_update
	Text       string           `json:"text,omitempty"`        // for progress (human-readable)
}

// Validate checks the AgentAction and its nested types.
func (a *AgentAction) Validate() error {
	switch a.Type {
	case ActionComplete, ActionPivot, ActionRetry, ActionError:
		if a.StepResult == nil {
			return fmt.Errorf("step_result is required for action type %q", a.Type)
		}
		return a.StepResult.Validate()
	case ActionToolCall:
		if a.ToolCall == nil {
			return fmt.Errorf("tool_call is required for action type %q", a.Type)
		}
		return a.ToolCall.Validate()
	case ActionPlanUpdate:
		if a.PlanUpdate == nil {
			return fmt.Errorf("plan_update is required for action type %q", a.Type)
		}
		return a.PlanUpdate.Validate()
	case ActionProgress:
		// Progress can be text-only
		return nil
	default:
		return fmt.Errorf("unknown action type %q", a.Type)
	}
}

// ParseAgentAction parses and validates a JSON agent action.
func ParseAgentAction(data []byte) (*AgentAction, error) {
	var action AgentAction
	if err := json.Unmarshal(data, &action); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := action.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	return &action, nil
}

// ParseStepResult parses and validates a JSON step result.
// This is a convenience wrapper for backward compatibility.
func ParseStepResult(data []byte) (*StepResult, error) {
	var result StepResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	return &result, nil
}

// DetectLegacyStepResult attempts to extract a StepResult from legacy text format.
// Returns nil if no STEP_RESULT: prefix is found.
// This is a bridge for backward compatibility during migration.
func DetectLegacyStepResult(text string) (*StepResult, error) {
	if !strings.Contains(text, "STEP_RESULT:") {
		return nil, nil
	}
	parts := strings.SplitN(text, "STEP_RESULT:", 2)
	if len(parts) < 2 {
		return nil, nil
	}
	jsonPart := strings.TrimSpace(parts[1])
	// Handle case where there's trailing text after the JSON
	if idx := strings.Index(jsonPart, "\n"); idx > 0 {
		jsonPart = jsonPart[:idx]
	}
	return ParseStepResult([]byte(jsonPart))
}

// DetectCompletionSignal checks text for legacy completion patterns.
// Returns a descriptive reason if detected, empty string otherwise.
// This is deprecated; agents should use typed StepResult instead.
func DetectCompletionSignal(text string) string {
	lower := strings.ToLower(text)
	patterns := []struct {
		pattern string
		reason  string
	}{
		{"already completed", "Agent verified implementation already exists"},
		{"already done", "Agent verified task already done"},
		{"implementation is complete", "Agent confirmed implementation complete"},
		{"criteria appear to be met", "Agent verified acceptance criteria met"},
		{"task accomplished", "Agent confirmed task accomplished"},
		{"successfully implemented", "Agent confirmed successful implementation"},
	}
	for _, p := range patterns {
		if strings.Contains(lower, p.pattern) {
			return p.reason
		}
	}
	return ""
}

// SchemaJSON returns the JSON Schema definition for AgentAction.
// This can be embedded in agent prompts for strict output enforcement.
func SchemaJSON() string {
	return `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "AgentAction",
  "type": "object",
  "required": ["type"],
  "properties": {
    "type": {
      "type": "string",
      "enum": ["complete", "pivot", "retry", "error", "progress", "tool_call", "plan_update"]
    },
    "step_result": {
      "type": "object",
      "properties": {
        "status": { "type": "string", "enum": ["complete", "error", "pivot", "retry"] },
        "reason": { "type": "string", "minLength": 1 },
        "next_phase": { "type": "string" },
        "artifacts": { "type": "object", "additionalProperties": { "type": "string" } },
        "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
        "diagnostics": { "type": "string" }
      },
      "required": ["status", "reason"]
    },
    "tool_call": {
      "type": "object",
      "properties": {
        "tool": { "type": "string", "minLength": 1 },
        "input": { "type": "object" },
        "idempotent": { "type": "boolean" },
        "priority": { "type": "string", "enum": ["low", "normal", "high"] },
        "timeout_ms": { "type": "integer", "minimum": 0 }
      },
      "required": ["tool", "input"]
    },
    "plan_update": {
      "type": "object",
      "properties": {
        "action": { "type": "string", "enum": ["add", "remove", "reorder", "complete"] },
        "task_ids": { "type": "array", "items": { "type": "string" } },
        "reason": { "type": "string", "minLength": 1 },
        "new_priority": { "type": "integer" }
      },
      "required": ["action", "reason"]
    },
    "text": { "type": "string" }
  }
}`
}
