package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/execution"
)

const (
	liveOllamaAddress = "127.0.0.1:11434"
	liveOllamaBaseURL = "http://127.0.0.1:11434/v1"
	liveOllamaModel   = "qwen3:8b"
)

// Most cases here only need a provider built; read-only is the narrower mode.
var readOnly = execution.Sandbox{Mode: execution.SandboxReadOnly}

func TestReasoningContentReplayIsScopedToKimiK3(t *testing.T) {
	for model, want := range map[string]bool{
		"kimi-k3": true, "kimi-k3-preview": true, "moonshotai/kimi-k3": true,
		"kimi-k3:latest": true, "gpt-4o": false, "qwen3:8b": false,
	} {
		if got := requiresReasoningContentReplay(model); got != want {
			t.Errorf("requiresReasoningContentReplay(%q) = %v, want %v", model, got, want)
		}
	}
}

// writeProjectConfig lays down a project whose execution config names two
// endpoints, mirroring the two-GPU layout the console routes across.
func writeProjectConfig(t *testing.T, active string) string {
	t.Helper()
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{
		"name": "fixture",
		"execution": map[string]any{
			"active_provider": active,
			"providers": map[string]any{
				"qwen-gpu0": map[string]string{
					"base_url": liveOllamaBaseURL, "api_key": "local", "model": liveOllamaModel,
				},
				"qwen-coder-gpu1": map[string]string{
					"base_url": "http://127.0.0.1:11435/v1", "api_key": "local", "model": "qwen3-coder:30b",
				},
				"needs-env": map[string]string{
					"base_url": liveOllamaBaseURL, "api_key": "$OPENEXEC_TEST_ABSENT_KEY", "model": liveOllamaModel,
				},
			},
		},
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, ".openexec", "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return directory
}

// The descriptor must advertise the configured model and nothing else. Left to
// its default the OpenAI client claims the whole hosted catalogue, which sends
// Probe's readiness prompt to gpt-4o against an endpoint serving Qwen.
func TestAPIProviderAdvertisesOnlyConfiguredModel(t *testing.T) {
	provider, err := newConfiguredAPIProvider(context.Background(), writeProjectConfig(t, "qwen-gpu0"), "qwen-gpu0", readOnly, toolGateway{})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := provider.Descriptor()
	if descriptor.Runtime != "api" {
		t.Errorf("runtime = %q, want api", descriptor.Runtime)
	}
	if descriptor.ID != "qwen-gpu0" {
		t.Errorf("id = %q", descriptor.ID)
	}
	if len(descriptor.Models) != 1 || descriptor.Models[0] != liveOllamaModel {
		t.Fatalf("models = %v, want exactly [%s]", descriptor.Models, liveOllamaModel)
	}
}

// Selection is per invocation. Naming one endpoint must not consult the
// project's active_provider, and must not rewrite it — two runs on two GPUs
// would otherwise race over one field on disk.
func TestAPIProviderSelectionIgnoresActiveProvider(t *testing.T) {
	directory := writeProjectConfig(t, "qwen-gpu0")
	before, err := os.ReadFile(filepath.Join(directory, ".openexec", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := newConfiguredAPIProvider(context.Background(), directory, "qwen-coder-gpu1", readOnly, toolGateway{})
	if err != nil {
		t.Fatal(err)
	}
	if models := provider.Descriptor().Models; len(models) != 1 || models[0] != "qwen3-coder:30b" {
		t.Fatalf("models = %v, want the named entry's model, not the active one", models)
	}
	after, err := os.ReadFile(filepath.Join(directory, ".openexec", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("selecting a provider rewrote the project config")
	}
}

func TestAPIProviderUnknownNameListsConfigured(t *testing.T) {
	_, err := newConfiguredAPIProvider(context.Background(), writeProjectConfig(t, "qwen-gpu0"), "qwen-gpu7", readOnly, toolGateway{})
	if err == nil {
		t.Fatal("expected an error for an unconfigured provider name")
	}
	for _, want := range []string{"qwen-gpu7", "qwen-gpu0", "qwen-coder-gpu1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// A "$VAR" api_key resolves in a terminal and to nothing under the console's
// scrubbed environment. The failure has to name the field, not surface as an
// authentication problem against a local endpoint that has no auth.
func TestAPIProviderRejectsUnresolvedKeyReference(t *testing.T) {
	_, err := newConfiguredAPIProvider(context.Background(), writeProjectConfig(t, "qwen-gpu0"), "needs-env", readOnly, toolGateway{})
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("error = %v, want one naming api_key", err)
	}
}

func TestAPIProviderRequiresName(t *testing.T) {
	if _, err := newConfiguredAPIProvider(context.Background(), writeProjectConfig(t, ""), "", readOnly, toolGateway{}); err == nil {
		t.Fatal("expected an error when --api-provider is omitted")
	}
}

// recordingProvider captures the request an API-runtime provider receives.
type recordingProvider struct{ got execution.Request }

func (p *recordingProvider) Descriptor() execution.ProviderDescriptor {
	return execution.ProviderDescriptor{ID: "qwen-gpu0", Runtime: "api", Models: []string{liveOllamaModel}}
}
func (p *recordingProvider) Probe(context.Context, string) execution.Readiness {
	return execution.Readiness{State: execution.ReadinessReady}
}
func (p *recordingProvider) Execute(_ context.Context, request execution.Request, _ execution.EventSink) (execution.Result, error) {
	p.got = request
	return execution.Result{Outcome: execution.OutcomeSucceeded}, nil
}

// The console sends "default" to mean "whatever this provider runs". A CLI
// provider answers that by omitting --model; an API provider must name one,
// because the client validates the model against its configured list.
func TestExecutionProtocolResolvesDefaultModelForAPI(t *testing.T) {
	for _, sent := range []string{"default", ""} {
		provider := &recordingProvider{}
		request := executionEnvelope{
			Version: executionProtocolVersion, Operation: "execute",
			Request: &execution.Request{
				ID: "one", WorkingDir: t.TempDir(), Prompt: "answer", Model: sent,
				Sandbox: execution.Sandbox{Mode: "read-only"},
			},
		}
		input, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		if err := serveExecutionProtocol(context.Background(), bytes.NewReader(input), &output, staticProvider(provider)); err != nil {
			t.Fatal(err)
		}
		if provider.got.Model != liveOllamaModel {
			t.Errorf("model %q was sent as %q, want %q", sent, provider.got.Model, liveOllamaModel)
		}
	}
}

// TestAPIProviderLiveOllama drives the protocol end to end against a real
// local endpoint. Skipped when nothing is listening, like the other
// environment-gated tests here.
func TestAPIProviderLiveOllama(t *testing.T) {
	connection, err := net.DialTimeout("tcp", liveOllamaAddress, 300*time.Millisecond)
	if err != nil {
		t.Skipf("no local model endpoint on %s: %v", liveOllamaAddress, err)
	}
	_ = connection.Close()

	directory := writeProjectConfig(t, "qwen-gpu0")
	provider, err := newConfiguredAPIProvider(context.Background(), directory, "qwen-gpu0", readOnly, toolGateway{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if readiness := provider.Probe(ctx, directory); readiness.State != execution.ReadinessReady {
		t.Fatalf("probe = %s: %s", readiness.State, readiness.Problem)
	}

	var text strings.Builder
	result, err := provider.Execute(ctx, execution.Request{
		ID: "live", WorkingDir: directory, Prompt: "Reply with exactly: ok",
		Model: liveOllamaModel, Sandbox: execution.Sandbox{Mode: "read-only"},
	}, func(event execution.Event) error {
		if event.Type == execution.EventAssistantDelta {
			text.WriteString(event.Text)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Outcome != execution.OutcomeSucceeded {
		t.Fatalf("outcome = %s", result.Outcome)
	}
	if strings.TrimSpace(text.String()) == "" {
		t.Fatal("no assistant text streamed from the local model")
	}
	t.Logf("local model replied: %.120q", strings.TrimSpace(text.String()))
}
