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
// replay (system text and conversation history) to the execute request.
//
// Both versions are accepted, because the alternative is worse in the
// direction that matters: a console that sends history to a v1 binary must be
// refused loudly, and a console that sends none to a v2 binary must keep
// working. What must never happen is a binary silently dropping history and
// answering as though the conversation had just begun.
const (
	executionProtocolVersion       = 2
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
func executionProviderFor(ctx context.Context, directory string, sandbox execution.Sandbox, gateway string) (execution.Provider, error) {
	if executionProviderKind == "api" {
		return newConfiguredAPIProvider(ctx, directory, executionAPIProvider, sandbox, gateway)
	}
	return execution.NewAgentCLIProvider(execution.AgentCLIConfig{
		Kind: executionProviderKind, Binary: executionProviderBin, SearchPath: executionSearchPath,
	})
}

// newConfiguredAPIProvider builds an OpenAI-compatible provider from one named
// entry in the project's execution config. The endpoint lives there and
// nowhere else — callers name a provider, never a URL.
func newConfiguredAPIProvider(ctx context.Context, directory, name string, sandbox execution.Sandbox, gateway string) (execution.Provider, error) {
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
	// A run either touches files or asks the caller questions. Both at once
	// would let a model reach console state and the repository in one turn,
	// and the reason each boundary is defensible is that it is the only one in
	// play. Refused here, before the model is contacted, rather than by
	// whichever executor happened to be asked first.
	if gateway != "" {
		if sandbox.Mode != execution.SandboxReadOnly {
			return nil, fmt.Errorf("a tool gateway run is read-only; %q was requested", sandbox.Mode)
		}
		executor, err := execution.NewGatewayToolExecutor(ctx, gateway)
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

// resolveAPIKeyReference expands a "$VAR" indirection, matching the pipeline's
// handling of the same field. A literal is returned unchanged.
func resolveAPIKeyReference(key string) string {
	if strings.HasPrefix(key, "$") {
		return os.Getenv(strings.TrimPrefix(key, "$"))
	}
	return key
}

func serveExecutionProtocol(ctx context.Context, input io.Reader, output io.Writer, providerFor func(ctx context.Context, directory string, sandbox execution.Sandbox, gateway string) (execution.Provider, error)) error {
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	writer := bufio.NewWriter(output)
	defer writer.Flush()
	write := func(value executionEnvelope) error {
		value.Version = executionProtocolVersion
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
	case executionProtocolVersion, executionProtocolVersionLegacy:
	default:
		return fmt.Errorf("unsupported execution protocol version %d", request.Version)
	}
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
	gateway := ""
	if request.Request != nil {
		gateway = request.Request.ToolGateway
	}
	provider, err := providerFor(ctx, directory, sandbox, gateway)
	if err != nil {
		return err
	}
	descriptor := provider.Descriptor()
	switch request.Operation {
	case "describe":
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
		result, err := provider.Execute(ctx, *request.Request, func(event execution.Event) error {
			return write(executionEnvelope{Operation: "event", Event: &event})
		})
		response := executionEnvelope{Operation: "result", Result: &result}
		if err != nil {
			response.Error = err.Error()
		}
		return write(response)
	default:
		return fmt.Errorf("unsupported execution operation %q", request.Operation)
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
