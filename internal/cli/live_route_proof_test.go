package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/execution"
)

// The environment cannot be prefixed onto a command in the console's host
// route, so the same explicit opt-in as TestAPIProviderLiveOllama is also
// accepted as a test flag: go test ./internal/cli -run LiveRoute -args -live-ollama=true
var liveOllamaFlag = flag.Bool("live-ollama", false, "explicitly authorize live inference against the configured local endpoint")

func liveOllamaAuthorized() bool {
	return *liveOllamaFlag || os.Getenv("OPENEXEC_LIVE_OLLAMA_TEST") == "1"
}

// Route proof for the post-#150 query-template failure: the configured
// local-API provider (the same entry the console routes to), driven through the
// real stdio execution protocol, makes several real read_file calls against a
// workspace, synthesizes a usable contract, and the terminal result is
// persisted to disk and re-read from the persisted bytes. No stub endpoint
// stands in for the model. Raw prompts and file contents are not logged; the
// checks compare only fixed marker values.
func TestLiveRouteMultiToolContractPersistsAndRereads(t *testing.T) {
	if !liveOllamaAuthorized() {
		t.Skip("live inference requires explicit opt-in (-live-ollama or OPENEXEC_LIVE_OLLAMA_TEST=1) and separate resource authorization")
	}
	connection, err := liveOllamaDial("tcp", liveOllamaAddress, 300*time.Millisecond)
	if err != nil {
		t.Skipf("no local model endpoint on %s: %v", liveOllamaAddress, err)
	}
	_ = connection.Close()

	directory := writeProjectConfig(t, "qwen-gpu0")
	markers := map[string]string{"alpha.txt": "ALPHA-7431", "beta.txt": "BETA-2956", "gamma.txt": "GAMMA-8120"}
	for name, marker := range markers {
		body := "marker: " + marker + "\n" + strings.Repeat("padding line\n", 400)
		if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A canary outside the workspace the model is invited to read.
	outside := t.TempDir()
	const canary = "CANARY-SECRET-5512"
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte(canary), 0o600); err != nil {
		t.Fatal(err)
	}

	prompt := "Use the read_file tool once for each of alpha.txt, beta.txt and gamma.txt in the workspace. " +
		"Then also try read_file on " + filepath.Join(outside, "secret.txt") + " and report whether it was refused. " +
		"Finish with a short contract, one line per file, in the form `<file>: <marker value>`, " +
		"plus a final line `outside: refused` or `outside: read`."
	request := executionEnvelope{
		Version: executionProtocolVersion, Operation: "execute", Directory: directory,
		Request: &execution.Request{
			ID: "live-route", WorkingDir: directory, Prompt: prompt, Model: "default",
			Sandbox: execution.Sandbox{Mode: execution.SandboxReadOnly},
		},
	}
	input, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	providerFor := func(ctx context.Context, dir string, sandbox execution.Sandbox, gateway toolGateway) (execution.Provider, error) {
		return newConfiguredAPIProvider(ctx, dir, "qwen-gpu0", sandbox, gateway)
	}
	if err := serveExecutionProtocol(ctx, bytes.NewReader(input), &output, providerFor); err != nil {
		t.Fatalf("protocol: %v", err)
	}

	// Persist the terminal stream, then judge only what is read back.
	saved := filepath.Join(t.TempDir(), "terminal.jsonl")
	if err := os.WriteFile(saved, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var result *execution.Result
	var terminal *execution.Event
	reads := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1<<20), 1<<24)
	for scanner.Scan() {
		var envelope executionEnvelope
		if err := json.Unmarshal(scanner.Bytes(), &envelope); err != nil {
			t.Fatalf("persisted line is not an envelope: %v", err)
		}
		if envelope.Error != "" {
			t.Fatalf("persisted error: %s", envelope.Error)
		}
		if event := envelope.Event; event != nil {
			if event.Type == execution.EventToolCompleted && event.ToolName == "read_file" {
				reads++
				if strings.Contains(event.Text, canary) {
					t.Fatal("tool result leaked the outside canary")
				}
			}
			if isExecutionTerminal(event.Type) {
				terminal = event
			}
		}
		if envelope.Result != nil {
			result = envelope.Result
		}
	}
	if result == nil || terminal == nil {
		t.Fatalf("persisted stream lacks a result (%v) or terminal event (%v)", result != nil, terminal != nil)
	}
	if result.Outcome != execution.OutcomeSucceeded || terminal.Type != execution.EventCompleted {
		t.Fatalf("outcome = %s reason = %q terminal = %s", result.Outcome, result.Reason, terminal.Type)
	}
	if result.Model != liveOllamaModel {
		t.Fatalf("model = %q, want the configured %q", result.Model, liveOllamaModel)
	}
	if reads < 3 {
		t.Fatalf("only %d completed read_file calls", reads)
	}
	for name, marker := range markers {
		if !strings.Contains(result.FinalText, marker) {
			t.Errorf("contract lacks the value read from %s", name)
		}
	}
	if strings.Contains(result.FinalText, canary) {
		t.Error("contract leaked the outside canary")
	}
	t.Logf("saved terminal result re-read: outcome=%s reads=%d final_text_bytes=%d", result.Outcome, reads, len(result.FinalText))
}

func TestLiveRouteRequiresOptInBeforeNetwork(t *testing.T) {
	if liveOllamaAuthorized() {
		t.Skip("opt-in present; the guard is only meaningful without it")
	}
	old := liveOllamaDial
	t.Cleanup(func() { liveOllamaDial = old })
	calls := 0
	liveOllamaDial = func(string, string, time.Duration) (net.Conn, error) {
		calls++
		return nil, errors.New("test stub: no network or inference allowed")
	}
	t.Run("live_route_without_authorization", TestLiveRouteMultiToolContractPersistsAndRereads)
	if calls != 0 {
		t.Fatal("live route test reached the network without explicit opt-in")
	}
}
