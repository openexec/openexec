package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/openexec/openexec/pkg/execution"
	"github.com/spf13/cobra"
)

const executionProtocolVersion = 1

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
)

var executionStdioCmd = &cobra.Command{
	Use:    "execution-stdio",
	Short:  "Run the versioned execution provider protocol over standard I/O",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		provider, err := execution.NewAgentCLIProvider(execution.AgentCLIConfig{
			Kind: executionProviderKind, Binary: executionProviderBin, SearchPath: executionSearchPath,
		})
		if err != nil {
			return err
		}
		return serveExecutionProtocol(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), provider)
	},
}

func serveExecutionProtocol(ctx context.Context, input io.Reader, output io.Writer, provider execution.Provider) error {
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
	if request.Version != executionProtocolVersion {
		return fmt.Errorf("unsupported execution protocol version %d", request.Version)
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
	executionStdioCmd.Flags().StringVar(&executionProviderKind, "provider", "", "provider kind: claude or codex")
	executionStdioCmd.Flags().StringVar(&executionProviderBin, "binary", "", "provider executable")
	executionStdioCmd.Flags().StringVar(&executionSearchPath, "search-path", "", "sanitized provider executable path")
	rootCmd.AddCommand(executionStdioCmd)
}
