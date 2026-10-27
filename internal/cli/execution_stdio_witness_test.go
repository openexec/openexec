//go:build linux

package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/execution"
)

// The helper runs the real Cobra entry point, config loader, stdio protocol,
// APIProvider, OpenAI HTTP adapter and native162 writers in a separate process.
// No installed executable, owner config, gateway or inference service is used.
func TestExecutionStdioWitnessChild(t *testing.T) {
	if os.Getenv("OPENEXEC_TEST_WITNESS_CHILD") != "1" {
		return
	}
	rootCmd.SetArgs([]string{"execution-stdio", "--provider", "api", "--api-provider", "fixture"})
	Execute()
	os.Exit(0) // keep the Go test runner's PASS line out of the line protocol
}

// Audit the retained, content-free operational extract, not a fabricated
// request. This proves its timeline/counts are internally consistent; it
// cannot supply the missing historical child's identity or environment.
func TestExecutionStdioWitnessHistoricalReceipt(t *testing.T) {
	raw, err := os.ReadFile("testdata/witness_60a3_metadata.json")
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Installation struct{ At time.Time }
		Run          struct {
			ID, Status, Error     string
			StartedAt, FinishedAt time.Time
			LastSequence          int
		}
		Events []struct {
			Sequence  int
			Type      string
			CreatedAt time.Time
			Tool      struct {
				Name  string
				Input struct{ Command []string }
			}
		}
		HistoricalAPIChild map[string]any
		PreInstallRuns     []struct{ StartedAt time.Time }
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Run.ID != "60a3dddb9c387861d369c5a4aba5dea0" || receipt.Run.Status != "inconclusive" ||
		!strings.HasPrefix(receipt.Run.Error, "inconclusive: budget_exhausted\n") || !receipt.Run.StartedAt.After(receipt.Installation.At) ||
		receipt.Run.FinishedAt.Sub(receipt.Run.StartedAt) != 2*time.Hour+3733614594*time.Nanosecond {
		t.Fatal("unexpected original run timeline/ending")
	}
	if len(receipt.Events) != 114 || receipt.Run.LastSequence != len(receipt.Events) {
		t.Fatal("historical event extract is incomplete")
	}
	counts := map[string]int{}
	commands := 0
	previous := receipt.Run.StartedAt
	for i, event := range receipt.Events {
		if event.Sequence != i+1 || event.CreatedAt.Before(previous) || event.CreatedAt.After(receipt.Run.FinishedAt) {
			t.Fatal("historical sequence or chronology changed")
		}
		previous = event.CreatedAt
		if event.Type == "tool.started" {
			counts[event.Tool.Name]++
			if len(event.Tool.Input.Command) > 0 {
				commands++
			}
		}
	}
	if len(counts) != 4 || counts["read_file"] != 16 || counts["search_files"] != 23 || counts["run_command"] != 12 || counts["record_route_judgment"] != 1 || commands != 12 {
		t.Fatalf("historical tool counts = %v; command argv arrays=%d", counts, commands)
	}
	for _, key := range []string{"executableIdentity", "captureEnvironment", "traceEnvironment"} {
		value, exists := receipt.HistoricalAPIChild[key]
		if !exists || value != nil {
			t.Fatal("missing historical provenance must remain explicitly unknown")
		}
	}
	if len(receipt.PreInstallRuns) != 2 {
		t.Fatal("missing pre-install exclusion evidence")
	}
	for _, run := range receipt.PreInstallRuns {
		if !run.StartedAt.Before(receipt.Installation.At) {
			t.Fatal("pre-install run was incorrectly classified")
		}
	}
}

// This models the Linux caller boundary inspected in agent-console's
// OpenExec.call / processtree.Configure: CommandContext, Setpgid, SIGKILL to
// the group. It does NOT establish the historical 60a3 child's provenance or
// execute the sibling repository's caller. SIGTERM is a cooperative control;
// native Execute() handles it through signal.NotifyContext.
func TestExecutionStdioWitnessBoundary(t *testing.T) {
	for _, mode := range []string{"cooperative", "kill", "closed-reader", "success", "refused", "opt-out", "capture-only", "trace-only"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			capture := filepath.Join(dir, "private", "request.json")
			trace := filepath.Join(dir, "private", "trace.json")
			const secret = "sk-abcdefghijklmnopqrstuvwx"
			longText := strings.Repeat("synthetic evidence ", 220)
			arrived := make(chan struct{}, 1)
			release := make(chan struct{})
			endpointDone := make(chan struct{}, 1)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)
					return
				}
				calls.Add(1)
				defer func() {
					select {
					case endpointDone <- struct{}{}:
					default:
					}
				}()
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error("invalid synthetic HTTP request")
					return
				}
				if body["model"] != "fixture-model" {
					t.Error("configured fixture model did not reach endpoint")
				}
				select {
				case arrived <- struct{}{}:
				default:
				}
				select {
				case <-release:
				case <-r.Context().Done():
					return
				case <-time.After(10 * time.Second):
					t.Error("fixture did not terminate its HTTP request")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, `{"id":"fixture","model":"fixture-model","choices":[{"index":0,"message":{"role":"assistant","content":"synthetic done"},"finish_reason":"stop"}]}`)
			}))
			defer server.Close()
			config := map[string]any{"name": "fixture", "execution": map[string]any{"providers": map[string]any{"fixture": map[string]any{
				"base_url": server.URL + "/v1", "model": "fixture-model", "api_key": "fixture-credential-never-in-witness", "max_turns": 2,
			}}}}
			if err := os.Mkdir(filepath.Join(dir, ".openexec"), 0o700); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			req := execution.Request{ID: "finite-witness-fixture", WorkingDir: dir, Prompt: "synthetic " + secret + " " + longText,
				Model: "default", Sandbox: execution.Sandbox{Mode: execution.SandboxReadOnly}}
			if mode == "refused" {
				req.NativeSessionID = "unsupported-api-resume"
			}
			input, err := json.Marshal(executionEnvelope{Version: executionProtocolVersion, Operation: "execute", Request: &req})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestExecutionStdioWitnessChild$")
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
			cmd.WaitDelay = time.Second
			// Only a synthetic environment is passed. Do not inherit capture
			// paths, credentials, live-test flags or owner configuration.
			cmd.Env = []string{"OPENEXEC_TEST_WITNESS_CHILD=1", "HOME=" + dir, "PATH=" + os.Getenv("PATH")}
			if mode != "opt-out" && mode != "trace-only" {
				cmd.Env = append(cmd.Env, execution.RequestCaptureEnv+"="+capture)
			}
			if mode != "opt-out" && mode != "capture-only" {
				cmd.Env = append(cmd.Env, execution.RequestTraceEnv+"="+trace)
			}
			cmd.Stdin = bytes.NewReader(append(input, '\n'))
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Cancel() }()
			decoder := json.NewDecoder(bufio.NewReader(stdout))
			var first executionEnvelope
			if err := decoder.Decode(&first); err != nil {
				t.Fatalf("initial protocol event: %v", err)
			}
			var envelopes []executionEnvelope
			envelopes = append(envelopes, first)
			if mode != "refused" {
				if first.Event == nil || first.Event.Type != execution.EventStarted {
					t.Fatal("missing real API started event")
				}
				select {
				case <-arrived:
				case <-ctx.Done():
					t.Fatal("endpoint never received request")
				}
				switch mode {
				case "kill":
					cancel() // exactly the caller's CommandContext/group-kill boundary
				case "closed-reader":
					if err := stdout.Close(); err != nil {
						t.Fatal(err)
					}
					close(release)
				case "success":
					close(release)
				default:
					if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
						t.Fatal(err)
					}
				}
			}
			for {
				var envelope executionEnvelope
				err := decoder.Decode(&envelope)
				if err != nil {
					if mode != "closed-reader" && !errors.Is(err, io.EOF) {
						t.Fatalf("protocol decoding: %v", err)
					}
					break
				}
				envelopes = append(envelopes, envelope)
			}
			waitErr := cmd.Wait()
			if mode != "kill" && ctx.Err() != nil {
				t.Fatal("finite fixture exceeded deadline")
			}
			if mode == "kill" || mode == "closed-reader" {
				var exit *exec.ExitError
				if !errors.As(waitErr, &exit) {
					t.Fatalf("expected forced process termination: %v", waitErr)
				}
				status := exit.Sys().(syscall.WaitStatus)
				want := syscall.SIGKILL
				if mode == "closed-reader" {
					want = syscall.SIGPIPE
				}
				if !status.Signaled() || status.Signal() != want || len(envelopes) != 1 {
					t.Fatalf("termination=%v envelopes=%d", status, len(envelopes))
				}
				t.Logf("exit signal=%s; no terminal event/result", status.Signal())
			} else {
				if waitErr != nil {
					t.Fatalf("CLI exit: %v (stderr bytes=%d)", waitErr, stderr.Len())
				}
				last := envelopes[len(envelopes)-1]
				want := execution.OutcomeFailed // cancellation during model Complete
				if mode == "success" {
					want = execution.OutcomeSucceeded
				}
				if last.Result == nil || last.Result.Outcome != want {
					t.Fatalf("missing terminal result, want %s", want)
				}
				if mode != "success" && last.Error == "" {
					t.Fatal("failed CLI result omitted error")
				}
				terminal := envelopes[len(envelopes)-2].Event
				wantEvent := execution.EventFailed
				if mode == "success" {
					wantEvent = execution.EventCompleted
				}
				if terminal == nil || terminal.Type != wantEvent {
					t.Fatal("terminal event contradicts result")
				}
				t.Logf("terminal event=%s; result=%s", terminal.Type, last.Result.Outcome)
			}
			if mode == "refused" {
				select {
				case <-arrived:
					t.Fatal("refused request reached model")
				default:
				}
			} else {
				select {
				case <-endpointDone:
				case <-time.After(10 * time.Second):
					t.Fatal("HTTP request outlived child")
				}
			}
			wantCalls := int32(1)
			if mode == "refused" {
				wantCalls = 0
			}
			if calls.Load() != wantCalls {
				t.Fatalf("completion requests=%d, want %d", calls.Load(), wantCalls)
			}
			for _, path := range []string{capture, trace} {
				want := mode == "cooperative" || mode == "capture-only" && path == capture || mode == "trace-only" && path == trace
				info, err := os.Stat(path)
				if !want {
					if !os.IsNotExist(err) {
						t.Fatal("unexpected private witness")
					}
					t.Logf("%s absent", filepath.Base(path))
					continue
				}
				if err != nil {
					t.Fatalf("missing cooperative witness: %v", err)
				}
				if info.Mode().Perm() != 0o600 {
					t.Fatal("witness is not private")
				}
				parent, err := os.Stat(filepath.Dir(path))
				if err != nil || parent.Mode().Perm() != 0o700 {
					t.Fatal("witness directory is not private")
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if bytes.Contains(raw, []byte(secret)) || bytes.Contains(raw, []byte("fixture-credential-never-in-witness")) {
					t.Fatal("synthetic credential reached witness")
				}
				var doc map[string]any
				if err := json.Unmarshal(raw, &doc); err != nil {
					t.Fatal(err)
				}
				if doc["outcome"] != execution.OutcomeFailed || doc["baseURL"] != server.URL+"/v1" || !strings.Contains(doc["error"].(string), "terminated signal received") {
					t.Fatal("read-back did not retain actual endpoint/signal cancellation")
				}
				if path == capture {
					if doc["truncated"] != true || bytes.Contains(raw, []byte(longText)) || len(raw) > 512<<10 {
						t.Fatal("capture bounds not applied")
					}
				} else if doc["redactionChangedContent"] != true || doc["imagesOmitted"] != false || doc["incomplete"] != false || doc["droppedEntries"] != float64(0) || len(doc["entries"].([]any)) != 1 || !bytes.Contains(raw, []byte(longText)) {
					t.Fatal("trace fidelity disclosures or full request lost")
				}
				t.Logf("%s persisted/read back; file=0600 directory=0700; redaction/bounds verified", filepath.Base(path))
			}
		})
	}
}
