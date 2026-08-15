// Package execution is the public boundary for invoking OpenExec execution
// engines. Callers decide whether a request may run; an Executor performs the
// work described by an already-authorized request.
package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
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
	ID         string
	WorkingDir string
	// ConfigDir is where this run's provider configuration is read from, when
	// that is not the directory the work happens in.
	//
	// The two diverge for unattended work: an isolated worktree is prepared
	// per task and `.openexec/` is not in it, because it is git-ignored and a
	// worktree carries tracked files only. Reading configuration from the
	// working directory therefore failed exactly where nobody was watching.
	// Empty means "the same place as the work".
	ConfigDir string
	Prompt    string
	Model     string
	Sandbox   Sandbox
	// WritableRoots may be edited. Never set for read-only execution.
	WritableRoots []string
	// ReadableRoots may be opened but not changed, in every mode. A project
	// spans several checkouts and a reviewer asked about one of them from a
	// session attached to another must be able to open it — Agent Console
	// already tells the agent it can, so a contract that drops these turns
	// that statement into a false one.
	ReadableRoots []string
	// System is the standing context for this turn: who the agent is, what it
	// may touch, what the project is for. Sent whole every turn for providers
	// replayed from the console, because a replayed conversation has no memory
	// of a brief delivered once.
	System string
	// History is the conversation so far, oldest first, as the console
	// persisted it. Empty for a provider that resumes its own native session:
	// that history lives inside the CLI and must not be duplicated here.
	History []HistoryMessage
	// ToolGateway is a per-run loopback endpoint offering tools that belong to
	// the caller rather than to OpenExec — console state a model may read,
	// which OpenExec has no business implementing and must not approximate
	// with a shell. Mutually exclusive with the workspace tools: a run either
	// touches files or asks the console questions, never both.
	ToolGateway string
	// NetworkAccess and NativeSessionID keep their meaning.
	NetworkAccess   bool
	NativeSessionID string
}

// HistoryMessage is one prior turn, replayed.
//
// Roles are limited to user and assistant on purpose. A `system` entry here
// would be a second, unvalidated way to set standing instructions, and a
// `tool` entry would replay tool results the console deliberately does not
// keep — both would smuggle content past the boundary that decides what a
// model is allowed to be told.
type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	HistoryRoleUser      = "user"
	HistoryRoleAssistant = "assistant"
)

// Replay budgets, in bytes.
//
// Sized for a 32K-token local context: history and standing context together
// leave room for the current request and a tool loop. Changing either is a
// deliberate context-policy decision, which is why they are named here rather
// than appearing as numbers at the point of use.
const (
	MaxHistoryBytes = 64 << 10
	MaxSystemBytes  = 16 << 10
)

// ValidateReplay checks what a provider is about to be told, before any of it
// reaches a model. Fails closed: a malformed history is a bug in the caller,
// and guessing what it meant would send the model something nobody wrote.
func (r Request) ValidateReplay() error {
	if len(r.System) > MaxSystemBytes {
		return fmt.Errorf("system context is %d bytes, over the %d byte budget", len(r.System), MaxSystemBytes)
	}
	total := 0
	for index, message := range r.History {
		switch message.Role {
		case HistoryRoleUser, HistoryRoleAssistant:
		default:
			return fmt.Errorf("history[%d] has role %q; only %q and %q may be replayed",
				index, message.Role, HistoryRoleUser, HistoryRoleAssistant)
		}
		if strings.TrimSpace(message.Content) == "" {
			return fmt.Errorf("history[%d] is empty", index)
		}
		total += len(message.Content)
	}
	if total > MaxHistoryBytes {
		return fmt.Errorf("history is %d bytes, over the %d byte budget", total, MaxHistoryBytes)
	}
	return nil
}

// ConfigDirectory is where provider configuration is read from: the declared
// directory, or the working directory when none was declared.
func (r Request) ConfigDirectory() string {
	if r.ConfigDir != "" {
		return r.ConfigDir
	}
	return r.WorkingDir
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

// Sandbox modes. The strings are the wire values and predate these names;
// existing comparisons against the literals remain correct.
const (
	SandboxReadOnly       = "read-only"
	SandboxWorkspaceWrite = "workspace-write"
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
	Streaming bool `json:"streaming"`
	// Resume is the provider continuing its own native session. Replay is the
	// caller resending the conversation. They are different mechanisms with
	// different owners, and a provider may have either, both, or neither — so
	// they are two bits rather than one "multi-turn".
	Resume bool `json:"resume"`
	Replay bool `json:"replay"`
	// ToolGateway is this runtime's ability to forward tools to a caller-owned
	// loopback endpoint. Declared because the alternative is what it replaced:
	// an older binary ignores the field, runs the workspace tools instead, and
	// the caller learns that the "administrative" turn read its repository
	// only by reading the transcript afterwards.
	ToolGateway    bool `json:"tool_gateway"`
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
