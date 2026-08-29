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
	// the caller rather than to OpenExec — console state a model may read, or
	// commands the caller executes under its own approval, neither of which
	// OpenExec has any business implementing or approximating with a shell.
	ToolGateway string
	// ToolGatewayScope says what the endpoint above is being used for, and so
	// what may run beside it.
	//
	// Empty — the original meaning, and still the default — is a gateway that
	// stands alone: console state, read-only, no workspace tools. A run either
	// touches files or asks the console about itself, never both, because
	// reaching console state and the repository in one turn is the pairing
	// that has no defensible story.
	//
	// GatewayScopeWithWorkspace is a different endpoint doing a different job:
	// the caller's own execution verbs — run a command, push — offered beside
	// the workspace tools. That combination is not new authority. It is what
	// every agent CLI on this contract already has, and withholding it from an
	// API provider was an artifact of the gateway having been built for the
	// console-state case first, not a decision about local models.
	//
	// Named on the wire rather than inferred from the sandbox mode so the
	// default is the strict one: a caller that sets a gateway without saying
	// why gets the standalone rule, and an older runtime paired with a newer
	// caller refuses instead of silently combining the two.
	ToolGatewayScope string
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
	Reason          string    `json:"reason,omitempty"`
}

const (
	OutcomeSucceeded    = "succeeded"
	OutcomeFailed       = "failed"
	OutcomeCancelled    = "cancelled"
	OutcomeInconclusive = "inconclusive"

	ReasonMaxTurns        = "max_turns"
	ReasonBudgetExhausted = "budget_exhausted"
	ReasonRouteFalsified  = "route_falsified"
	ReasonProtocolError   = "protocol_error"
)

// Sandbox modes. The strings are the wire values and predate these names;
// existing comparisons against the literals remain correct.
const (
	SandboxReadOnly       = "read-only"
	SandboxWorkspaceWrite = "workspace-write"
	// GatewayScopeWithWorkspace lets the caller's execution verbs run beside
	// the workspace tools. See Request.ToolGatewayScope.
	GatewayScopeWithWorkspace = "with-workspace"
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
	ToolGateway bool `json:"tool_gateway"`
	// ToolGatewayWithWorkspace is the ability to run a gateway's verbs *beside*
	// the workspace tools. A separate bit from ToolGateway, and it has to be:
	// every runtime that forwards console state already answers true to that
	// one, including every build made before this scope existed. A caller that
	// read the general bit as consent to the new scope would send it to a
	// runtime that ignores the unknown field, sees a gateway on a
	// workspace-write run, and refuses the turn outright — so the feature would
	// not degrade, it would break every Build-mode turn on a local endpoint.
	//
	// Absent means false, which is exactly right for those older builds: the
	// caller withholds the composite endpoint and the run proceeds with the
	// workspace tools alone, which is what it had before.
	ToolGatewayWithWorkspace bool `json:"tool_gateway_with_workspace"`
	Cancellation             bool `json:"cancellation"`
	ReadOnly                 bool `json:"read_only"`
	WorkspaceWrite           bool `json:"workspace_write"`
	CommandNetwork           bool `json:"command_network"`
	ToolCalling              bool `json:"tool_calling"`
	// OutcomeNavigator is absent until the provider/runtime enforces every
	// capability it claims. A transport upgrade alone must never enable
	// autonomous navigation.
	OutcomeNavigator *OutcomeNavigatorCapability `json:"outcome_navigator,omitempty"`
}

// OutcomeNavigatorCapability is the versioned, fail-closed runtime contract
// negotiated before an Outcome Navigator provider call. Each field names an
// independently enforced boundary; missing is false.
type OutcomeNavigatorCapability struct {
	Version                int  `json:"version"`
	TerminalInconclusive   bool `json:"terminal_inconclusive_v1"`
	UsageReservations      bool `json:"usage_reservations_v1"`
	ChildAccounting        bool `json:"child_accounting_v1"`
	ChallengeWithWorkspace bool `json:"challenge_with_workspace_v1"`
	EffectFencing          bool `json:"effect_fencing_v1"`
	RemoteHardContainment  bool `json:"remote_hard_containment_v1"`
	TerminalReducer        bool `json:"terminal_reducer_v1"`
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
	Reason   string          `json:"reason,omitempty"`
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
	EventInconclusive   = "execution.inconclusive"
)

type EventSink func(Event) error

// Provider is the public runtime boundary. Authorization remains the caller's
// responsibility; implementations must reject containment they cannot enforce.
type Provider interface {
	Descriptor() ProviderDescriptor
	Probe(context.Context, string) Readiness
	Execute(context.Context, Request, EventSink) (Result, error)
}
