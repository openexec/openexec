// Package execution is the public boundary for invoking OpenExec execution
// engines. Callers decide whether a request may run; an Executor performs the
// work described by an already-authorized request.
package execution

import (
	"context"
	"encoding/json"
	"io"
	"time"
)

// Sandbox describes the containment promised for an execution. It is evidence,
// not a permission request: an Executor must reject modes it cannot enforce.
type Sandbox struct {
	Mode      string `json:"mode"`
	Isolation string `json:"isolation"`
}

// Request is one authorized execution unit. Prompt is intentionally opaque to
// the engine so callers can supply their own task instructions.
type Request struct {
	ID              string
	WorkingDir      string
	Prompt          string
	Model           string
	Sandbox         Sandbox
	WritableRoots   []string
	NetworkAccess   bool
	NativeSessionID string
}

// Result records what the executor actually used and how it terminated.
type Result struct {
	Executor        string    `json:"executor"`
	Model           string    `json:"model"`
	Sandbox         Sandbox   `json:"sandbox"`
	NativeSessionID string    `json:"native_session_id,omitempty"`
	FinalText       string    `json:"final_text,omitempty"`
	StartedAt       time.Time `json:"started_at"`
	EndedAt         time.Time `json:"ended_at"`
	Outcome         string    `json:"outcome"`
}

const (
	OutcomeSucceeded = "succeeded"
	OutcomeFailed    = "failed"
	OutcomeCancelled = "cancelled"
)

// Executor performs an authorized request. Implementations must return a
// Result even on execution failure so callers can preserve provenance.
type Executor interface {
	Execute(context.Context, Request) (Result, error)
}

// Streams carries optional process output destinations.
type Streams struct {
	Stdout io.Writer
	Stderr io.Writer
}

type Capability struct {
	Streaming      bool `json:"streaming"`
	Resume         bool `json:"resume"`
	Cancellation   bool `json:"cancellation"`
	ReadOnly       bool `json:"read_only"`
	WorkspaceWrite bool `json:"workspace_write"`
	CommandNetwork bool `json:"command_network"`
	ToolCalling    bool `json:"tool_calling"`
}

type ProviderDescriptor struct {
	ID           string     `json:"id"`
	Runtime      string     `json:"runtime"`
	Models       []string   `json:"models"`
	Capabilities Capability `json:"capabilities"`
}

type Readiness struct {
	State   string `json:"state"`
	Problem string `json:"problem,omitempty"`
}

const (
	ReadinessReady        = "ready"
	ReadinessNotInstalled = "not-installed"
	ReadinessNeedsLogin   = "needs-login"
	ReadinessUnhealthy    = "unhealthy"
)

type Event struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	CallID   string          `json:"call_id,omitempty"`
	ToolName string          `json:"tool_name,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
}

const (
	EventStarted        = "execution.started"
	EventAssistantDelta = "assistant.delta"
	EventToolProposed   = "tool.proposed"
	EventToolStarted    = "tool.started"
	EventToolCompleted  = "tool.completed"
	EventFailed         = "execution.failed"
	EventCancelled      = "execution.cancelled"
	EventCompleted      = "execution.completed"
)

type EventSink func(Event) error

// Provider is the public runtime boundary. Authorization remains the caller's
// responsibility; implementations must reject containment they cannot enforce.
type Provider interface {
	Descriptor() ProviderDescriptor
	Probe(context.Context, string) Readiness
	Execute(context.Context, Request, EventSink) (Result, error)
}
