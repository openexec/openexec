package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/agent"
	"github.com/openexec/openexec/pkg/execution"
	"github.com/spf13/cobra"
)

// executionProtocolVersion is what this binary speaks. Version 2 added typed
// replay. Version 3 adds the negotiated Outcome Navigator capability envelope.
//
// Both versions are accepted, because the alternative is worse in the
// direction that matters: a console that sends history to a v1 binary must be
// refused loudly, and a console that sends none to a v2 binary must keep
// working. What must never happen is a binary silently dropping history and
// answering as though the conversation had just begun.
const (
	executionProtocolVersion       = 3
	executionProtocolVersionReplay = 2
	executionProtocolVersionLegacy = 1
)

type executionEnvelope struct {
	Version   int                           `json:"version"`
	Operation string                        `json:"operation,omitempty"`
	Directory string                        `json:"directory,omitempty"`
	Request   *execution.Request            `json:"request,omitempty"`
	Event     *execution.Event              `json:"event,omitempty"`
	Result    *execution.Result             `json:"result,omitempty"`
	Readiness *execution.Readiness          `json:"readiness,omitempty"`
	Provider  *execution.ProviderDescriptor `json:"provider,omitempty"`
	Error     string                        `json:"error,omitempty"`
}

var (
	executionProviderKind string
	executionProviderBin  string
	executionSearchPath   string
	executionAPIProvider  string
)

var executionStdioCmd = &cobra.Command{
	Use:    "execution-stdio",
	Short:  "Run the versioned execution provider protocol over standard I/O",
	Hidden: true,
	Args:   cobra.NoArgs,
	// The caller is a program reading a line protocol, not a person: a
	// misconfigured provider should hand it one error line, not a usage dump
	// to parse around.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return serveExecutionProtocol(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), executionProviderFor)
	},
}

// executionProviderFor builds the provider named by the flags.
//
// Called per request rather than once at startup because an API provider is
// resolved from the *project's* configuration and offers a tool set that
// depends on the sandbox, neither of which is known until the request
// arrives. CLI providers ignore both arguments.
func executionProviderFor(ctx context.Context, directory string, sandbox execution.Sandbox, gateway toolGateway) (execution.Provider, error) {
	if executionProviderKind == "api" {
		return newConfiguredAPIProvider(ctx, directory, executionAPIProvider, sandbox, gateway)
	}
	return execution.NewAgentCLIProvider(execution.AgentCLIConfig{
		Kind: executionProviderKind, Binary: executionProviderBin, SearchPath: executionSearchPath,
	})
}

// toolGateway is the caller-owned endpoint for one run, and what it is for.
//
// Carried together rather than as two adjacent strings: the endpoint decides
// where authority is asked and the scope decides what may run beside it, and a
// pair of bare strings in that order is a swap away from granting the wrong
// combination silently.
type toolGateway struct {
	Endpoint string
	Scope    string
}

// newConfiguredAPIProvider builds an OpenAI-compatible provider from one named
// entry in the project's execution config. The endpoint lives there and
// nowhere else — callers name a provider, never a URL.
func newConfiguredAPIProvider(ctx context.Context, directory, name string, sandbox execution.Sandbox, gateway toolGateway) (execution.Provider, error) {
	if name == "" {
		return nil, fmt.Errorf("--api-provider is required with --provider api")
	}
	if directory == "" {
		working, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		directory = working
	}
	config, err := project.LoadProjectConfig(directory)
	if err != nil {
		return nil, err
	}
	entry, ok := config.Execution.APIByName(name)
	if !ok {
		configured := config.Execution.ProviderNames()
		if len(configured) == 0 {
			return nil, fmt.Errorf("no API providers configured in %s", directory)
		}
		return nil, fmt.Errorf("no API provider named %q in %s (configured: %s)",
			name, directory, strings.Join(configured, ", "))
	}
	if entry.Model == "" {
		return nil, fmt.Errorf("API provider %q has no model configured", name)
	}
	apiKey := resolveAPIKeyReference(entry.APIKey)
	if apiKey == "" {
		// Local endpoints ignore the key but the client still requires one, so
		// this is reachable with a perfectly working Ollama. Say which of the
		// two causes it is: a console-spawned run gets a scrubbed environment
		// (agent-console internal/providers/process.go), so a "$VAR" reference
		// that resolves in a terminal resolves to nothing there.
		return nil, fmt.Errorf("API provider %q has no usable api_key (a literal is required; %q resolved to empty)",
			name, entry.APIKey)
	}
	adapter, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
		Name: name, BaseURL: entry.BaseURL, APIKey: apiKey,
		ReplayReasoningContent: requiresReasoningContentReplay(entry.Model),
		// Exactly the configured model, never appended to DefaultOpenAIModels()
		// the way the pipeline does it. Probe sends its readiness prompt to
		// models[0] and consumers list these in a picker, so an inherited
		// OpenAI catalogue would probe a local endpoint with gpt-4o and
		// advertise models it cannot serve.
		Models: []string{entry.Model},
		ModelInfo: map[string]*agent.ModelInfo{entry.Model: {
			ID: entry.Model, Name: entry.Model, Provider: name, Enabled: true,
			Capabilities: agent.ProviderCapabilities{
				Streaming: true, ToolUse: true, SystemPrompt: true, MultiTurn: true,
			},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("create API provider %q: %w", name, err)
	}
	// A gateway that stands alone still stands alone. Console state and the
	// repository in one turn is the pairing with no defensible story, and the
	// reason that boundary holds is that it is the only one in play — so this
	// stays refused here, before the model is contacted, rather than by
	// whichever executor happened to be asked first.
	//
	// The caller says which kind of gateway it is. Silence means the strict
	// one: a caller that names an endpoint without saying why gets the
	// standalone rule, and this runtime paired with a caller that knows about
	// the other scope refuses rather than combining anything by accident.
	if gateway.Endpoint != "" && gateway.Scope != execution.GatewayScopeWithWorkspace {
		if sandbox.Mode != execution.SandboxReadOnly {
			return nil, fmt.Errorf("a tool gateway run is read-only; %q was requested", sandbox.Mode)
		}
		executor, err := execution.NewGatewayToolExecutor(ctx, gateway.Endpoint)
		if err != nil {
			return nil, err
		}
		return execution.NewAPIProvider(execution.APIProviderConfig{
			Adapter: adapter, Tools: executor.Tools(), ToolExecutor: executor,
		})
	}
	// The caller's execution verbs beside the workspace tools: an agent that
	// edits a repository and can also build, test and push it. OpenExec owns
	// the file tools and bounds them by sandbox and roots; it does not own a
	// shell, so those verbs stay the caller's, executed under the caller's
	// approval and never approximated here.
	if gateway.Endpoint != "" {
		// This scope is for editing work, and the mode is checked here rather
		// than trusted from the caller — the same reason the standalone rule
		// above is enforced on this side at all.
		//
		// Without it a read-only run could be handed forwarded run_command and
		// git_push: the workspace executor validates paths, not modes, so
		// nothing downstream would have objected. A read-only conversation that
		// can push is not read-only, whatever the field says.
		//
		// Refused rather than filtered down to the harmless verbs. Which of a
		// caller's verbs mutate is the caller's knowledge, not OpenExec's, and
		// guessing it from a name is how a boundary becomes decorative.
		if sandbox.Mode != execution.SandboxWorkspaceWrite {
			return nil, fmt.Errorf(
				"a %s tool gateway requires workspace-write; %q was requested",
				execution.GatewayScopeWithWorkspace, sandbox.Mode)
		}
		forwarded, err := execution.NewGatewayToolExecutor(ctx, gateway.Endpoint)
		if err != nil {
			return nil, err
		}
		executor, err := execution.NewCompositeToolExecutor(
			execution.NewWorkspaceToolExecutor(), execution.WorkspaceTools(sandbox), forwarded)
		if err != nil {
			return nil, err
		}
		return execution.NewAPIProvider(execution.APIProviderConfig{
			Adapter: adapter, Tools: executor.Tools(), ToolExecutor: executor,
		})
	}
	// Tools in both modes, filtered by the sandbox: a read-only reviewer that
	// cannot open a file is useless, and the executor re-checks the mode on
	// every call, so the filtering is convenience and the enforcement is
	// separate.
	return execution.NewAPIProvider(execution.APIProviderConfig{
		Adapter:      adapter,
		Tools:        execution.WorkspaceTools(sandbox),
		ToolExecutor: execution.NewWorkspaceToolExecutor(),
	})
}

func requiresReasoningContentReplay(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.Contains(model, "kimi-k3")
}

// resolveAPIKeyReference expands a "$VAR" indirection, matching the pipeline's
// handling of the same field. A literal is returned unchanged.
func resolveAPIKeyReference(key string) string {
	if strings.HasPrefix(key, "$") {
		return os.Getenv(strings.TrimPrefix(key, "$"))
	}
	return key
}

func serveExecutionProtocol(ctx context.Context, input io.Reader, output io.Writer, providerFor func(ctx context.Context, directory string, sandbox execution.Sandbox, gateway toolGateway) (execution.Provider, error)) error {
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	writer := bufio.NewWriter(output)
	defer writer.Flush()
	responseVersion := executionProtocolVersion
	write := func(value executionEnvelope) error {
		value.Version = responseVersion
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err := writer.Write(append(data, '\n')); err != nil {
			return err
		}
		return writer.Flush()
	}
	var request executionEnvelope
	if err := decoder.Decode(&request); err != nil {
		return fmt.Errorf("decode execution request: %w", err)
	}
	switch request.Version {
	case executionProtocolVersion, executionProtocolVersionReplay, executionProtocolVersionLegacy:
	default:
		return fmt.Errorf("unsupported execution protocol version %d", request.Version)
	}
	// Reply in the requester's version. This lets v1/v2 callers keep working
	// while a v3 caller can require the new capability contract. Unknown fields
	// are additive and ignored by older callers.
	responseVersion = request.Version
	// A v1 caller cannot have meant to send history — the field did not exist —
	// so anything in it came from a confusion about who is speaking.
	if request.Version == executionProtocolVersionLegacy && request.Request != nil {
		if len(request.Request.History) > 0 || request.Request.System != "" {
			return fmt.Errorf("protocol version 1 carries no replay; send version %d", executionProtocolVersion)
		}
	}
	// The execute path carries its directory on the request; describe and
	// probe carry it on the envelope. An API provider is configured per
	// project, so it cannot be built before this point.
	directory := request.Directory
	// describe and probe answer for the provider's full capability, so they
	// ask for the widest tool set; execute asks for exactly what the request
	// was authorized to do.
	sandbox := execution.Sandbox{Mode: execution.SandboxWorkspaceWrite}
	if request.Request != nil {
		// Configuration comes from the declared config directory, which is the
		// registered checkout — not the working directory, which for
		// unattended work is a throwaway worktree with no `.openexec/` in it
		// (it is git-ignored, and a worktree carries tracked files only).
		if resolved := request.Request.ConfigDirectory(); resolved != "" {
			directory = resolved
		}
		sandbox = request.Request.Sandbox
	}
	var gateway toolGateway
	if request.Request != nil {
		gateway = toolGateway{Endpoint: request.Request.ToolGateway, Scope: request.Request.ToolGatewayScope}
	}
	provider, err := providerFor(ctx, directory, sandbox, gateway)
	if err != nil {
		return err
	}
	descriptor := provider.Descriptor()
	switch request.Operation {
	case "describe":
		// The terminal contract belongs to this v3 transport, not to a provider
		// that may also be called directly. Advertise only the two boundaries
		// enforced here; authoritative budget reservations, child accounting,
		// challenge composition, effect fencing, and remote containment remain
		// false until their own authorities can prove them.
		if request.Version == executionProtocolVersion {
			descriptor.Capabilities.OutcomeNavigator = &execution.OutcomeNavigatorCapability{
				Version:              1,
				TerminalInconclusive: true,
				TerminalReducer:      true,
			}
		}
		// describe answers for the runtime, execute answers for one provider
		// instance. A provider built without a gateway cannot serve one and
		// says so; this binary can build one when asked, and the caller needs
		// that fact before it has anything to ask with.
		if executionProviderKind == "api" {
			descriptor.Capabilities.ToolGateway = true
			// This binary can also run those verbs beside the workspace tools.
			// Declared separately from the line above so a caller can tell the
			// two apart: builds that predate the composite scope answer true to
			// ToolGateway and nothing at all to this, and a caller that
			// conflated them would send them a request they refuse whole.
			descriptor.Capabilities.ToolGatewayWithWorkspace = true
		}
		return write(executionEnvelope{Operation: "describe", Provider: &descriptor})
	case "probe":
		readiness := provider.Probe(ctx, request.Directory)
		return write(executionEnvelope{Operation: "probe", Readiness: &readiness})
	case "execute":
		if request.Request == nil {
			return fmt.Errorf("execute request is required")
		}
		// "default" is the console's sentinel for "whatever this provider
		// runs". A CLI provider answers it by omitting --model; an API
		// provider must name one, because the OpenAI client validates the
		// model against its configured list. Resolved here so no caller has
		// to learn the endpoint's model name.
		if descriptor.Runtime == "api" && len(descriptor.Models) > 0 {
			if request.Request.Model == "" || request.Request.Model == "default" {
				request.Request.Model = descriptor.Models[0]
			}
		}
		reducer := executionTerminalReducer{}
		result, err := provider.Execute(ctx, *request.Request, func(event execution.Event) error {
			if isExecutionTerminal(event.Type) {
				reducer.terminals = append(reducer.terminals, event)
				return nil
			}
			return write(executionEnvelope{Operation: "event", Event: &event})
		})
		terminal := reducer.reduce(&result, err)
		if writeErr := write(executionEnvelope{Operation: "event", Event: &terminal}); writeErr != nil {
			return writeErr
		}
		response := executionEnvelope{Operation: "result", Result: &result}
		if err != nil && result.Outcome == execution.OutcomeFailed {
			response.Error = err.Error()
		}
		return write(response)
	default:
		return fmt.Errorf("unsupported execution operation %q", request.Operation)
	}
}

type executionTerminalReducer struct {
	terminals []execution.Event
}

func isExecutionTerminal(eventType string) bool {
	switch eventType {
	case execution.EventCompleted, execution.EventFailed, execution.EventCancelled, execution.EventInconclusive:
		return true
	default:
		return false
	}
}

func (r executionTerminalReducer) reduce(result *execution.Result, executeErr error) execution.Event {
	// A provider can fail before it has a stream to emit into: invalid gateway
	// configuration, a missing executable, or cmd.Start itself. That is a real
	// failed execution, not malformed terminal data. Preserve both its type and
	// its cause; protocol_error is reserved for a provider that did speak a
	// terminal contract and contradicted it (or claimed success without one).
	if len(r.terminals) == 0 && executeErr != nil {
		result.Outcome = execution.OutcomeFailed
		result.Reason = ""
		return execution.Event{Type: execution.EventFailed, Text: executeErr.Error()}
	}
	if len(r.terminals) == 1 && terminalMatchesResult(r.terminals[0], *result, executeErr) {
		return r.terminals[0]
	}
	result.Outcome = execution.OutcomeInconclusive
	result.Reason = execution.ReasonProtocolError
	return execution.Event{Type: execution.EventInconclusive, Reason: execution.ReasonProtocolError}
}

func terminalMatchesResult(event execution.Event, result execution.Result, executeErr error) bool {
	if executeErr != nil && result.Outcome != execution.OutcomeFailed && result.Outcome != execution.OutcomeCancelled {
		return false
	}
	switch result.Outcome {
	case execution.OutcomeSucceeded:
		return event.Type == execution.EventCompleted && result.Reason == ""
	case execution.OutcomeFailed:
		return event.Type == execution.EventFailed && result.Reason == ""
	case execution.OutcomeCancelled:
		return event.Type == execution.EventCancelled && result.Reason == ""
	case execution.OutcomeInconclusive:
		return event.Type == execution.EventInconclusive && event.Reason == result.Reason && validInconclusiveReason(result.Reason)
	default:
		return false
	}
}

func validInconclusiveReason(reason string) bool {
	switch reason {
	case execution.ReasonMaxTurns, execution.ReasonBudgetExhausted,
		execution.ReasonRouteFalsified, execution.ReasonProtocolError:
		return true
	default:
		return false
	}
}

func init() {
	executionStdioCmd.Flags().StringVar(&executionProviderKind, "provider", "", "provider kind: claude, codex, or api")
	executionStdioCmd.Flags().StringVar(&executionProviderBin, "binary", "", "provider executable")
	executionStdioCmd.Flags().StringVar(&executionSearchPath, "search-path", "", "sanitized provider executable path")
	executionStdioCmd.Flags().StringVar(&executionAPIProvider, "api-provider", "",
		"named entry under execution.providers to run, with --provider api")
	rootCmd.AddCommand(executionStdioCmd)
}
