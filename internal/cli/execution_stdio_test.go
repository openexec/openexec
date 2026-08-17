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

func staticProvider(p execution.Provider) func(context.Context, string, execution.Sandbox, toolGateway) (execution.Provider, error) {
	return func(context.Context, string, execution.Sandbox, toolGateway) (execution.Provider, error) {
		return p, nil
	}
}

func TestExecutionProtocolExecute(t *testing.T) {
	request := executionEnvelope{
		Version: executionProtocolVersion, Operation: "execute",
		Request: &execution.Request{ID: "one", WorkingDir: t.TempDir(), Prompt: "answer", Sandbox: execution.Sandbox{Mode: "read-only"}},
	}
	input, _ := json.Marshal(request)
	var output bytes.Buffer
	if err := serveExecutionProtocol(context.Background(), bytes.NewReader(input), &output, staticProvider(protocolProvider{})); err != nil {
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
	err := serveExecutionProtocol(context.Background(), strings.NewReader(`{"version":99,"operation":"describe"}`), &output, staticProvider(protocolProvider{}))
	if err == nil || !strings.Contains(err.Error(), "unsupported execution protocol version") {
		t.Fatalf("error = %v", err)
	}
}

// The previous version is still spoken, because a console that sends no
// history has not changed and should not be broken by a binary upgrade.
func TestExecutionProtocolStillAnswersTheOlderVersion(t *testing.T) {
	var output bytes.Buffer
	if err := serveExecutionProtocol(context.Background(),
		strings.NewReader(`{"version":1,"operation":"describe"}`), &output,
		staticProvider(protocolProvider{})); err != nil {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(output.String(), `"operation":"describe"`) {
		t.Fatalf("output = %s", output.String())
	}
}

// What must never happen quietly: a caller sends a conversation, the binary
// does not understand the field, and the model answers as though the
// conversation had just begun.
func TestExecutionProtocolRefusesReplayOnTheOlderVersion(t *testing.T) {
	request := executionEnvelope{
		Version: 1, Operation: "execute",
		Request: &execution.Request{
			ID: "one", WorkingDir: t.TempDir(), Prompt: "and then?",
			Sandbox: execution.Sandbox{Mode: execution.SandboxReadOnly},
			History: []execution.HistoryMessage{{Role: "user", Content: "remember 41"}},
		},
	}
	input, _ := json.Marshal(request)
	var output bytes.Buffer
	err := serveExecutionProtocol(context.Background(), bytes.NewReader(input), &output, staticProvider(protocolProvider{}))
	if err == nil || !strings.Contains(err.Error(), "carries no replay") {
		t.Fatalf("error = %v, want a refusal naming the version", err)
	}
}
