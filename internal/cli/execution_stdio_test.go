package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/execution"
)

type protocolProvider struct{}

func (protocolProvider) Descriptor() execution.ProviderDescriptor {
	return execution.ProviderDescriptor{ID: "fake", Runtime: "test"}
}
func (protocolProvider) Probe(context.Context, string) execution.Readiness {
	return execution.Readiness{State: execution.ReadinessReady}
}
func (protocolProvider) Execute(_ context.Context, request execution.Request, sink execution.EventSink) (execution.Result, error) {
	_ = sink(execution.Event{Type: execution.EventStarted})
	_ = sink(execution.Event{Type: execution.EventAssistantDelta, Text: request.Prompt})
	_ = sink(execution.Event{Type: execution.EventCompleted})
	return execution.Result{Outcome: execution.OutcomeSucceeded, FinalText: request.Prompt}, nil
}

func TestExecutionProtocolExecute(t *testing.T) {
	request := executionEnvelope{
		Version: executionProtocolVersion, Operation: "execute",
		Request: &execution.Request{ID: "one", WorkingDir: t.TempDir(), Prompt: "answer", Sandbox: execution.Sandbox{Mode: "read-only"}},
	}
	input, _ := json.Marshal(request)
	var output bytes.Buffer
	if err := serveExecutionProtocol(context.Background(), bytes.NewReader(input), &output, protocolProvider{}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("protocol lines = %q", lines)
	}
	for _, line := range lines {
		var envelope executionEnvelope
		if err := json.Unmarshal([]byte(line), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Version != executionProtocolVersion {
			t.Fatalf("version = %d", envelope.Version)
		}
	}
}

func TestExecutionProtocolRejectsVersionDrift(t *testing.T) {
	var output bytes.Buffer
	err := serveExecutionProtocol(context.Background(), strings.NewReader(`{"version":2,"operation":"describe"}`), &output, protocolProvider{})
	if err == nil || !strings.Contains(err.Error(), "unsupported execution protocol version") {
		t.Fatalf("error = %v", err)
	}
}
