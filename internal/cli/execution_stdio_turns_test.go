package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/execution"
)

// Exercise disk config -> stdio -> the actual API tool loop, including the
// Console's ConfigDir/WorkingDir split. HTTP fixtures perform no inference.
func TestConfiguredAPIMaxTurnsThroughProtocol(t *testing.T) {
	for _, tc := range []struct {
		name       string
		configured any
		wantCalls  int
		finishAt   int
		gateway    bool
		workspace  bool
	}{
		{name: "default_160", wantCalls: 160},
		{name: "zero_uses_default", configured: 0, wantCalls: 160},
		{name: "trial_100", configured: 100, wantCalls: 100},
		{name: "normal_160", configured: 160, wantCalls: 160},
		{name: "hard_900", configured: 900, wantCalls: 900},
		{name: "small_override", configured: 1, wantCalls: 1},
		{name: "finishes_after_old_limit", configured: 100, wantCalls: 20, finishAt: 21},
		{name: "standalone_gateway", configured: 100, wantCalls: 100, gateway: true},
		{name: "workspace_gateway", configured: 100, wantCalls: 100, gateway: true, workspace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var completions atomic.Int64
			toolName := "read_file"
			if tc.gateway {
				toolName = "fixture_status"
			}
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/chat/completions" {
					http.NotFound(w, r)
					return
				}
				var request struct {
					ToolChoice string `json:"tool_choice"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					http.Error(w, "invalid request", http.StatusBadRequest)
					return
				}
				n := int(completions.Add(1))
				message := map[string]any{"role": "assistant", "content": "finished"}
				stop := "stop"
				if request.ToolChoice != "none" && (tc.finishAt == 0 || n < tc.finishAt) {
					message["content"] = nil
					message["tool_calls"] = []any{map[string]any{
						"id": fmt.Sprintf("call-%d", n), "type": "function",
						"function": map[string]string{"name": toolName, "arguments": `{"path":"evidence.txt"}`},
					}}
					stop = "tool_calls"
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"index": 0, "message": message, "finish_reason": stop,
				}}})
			}))
			defer endpoint.Close()
			entry := map[string]any{"base_url": endpoint.URL + "/v1", "api_key": "local", "model": "fixture-model"}
			if tc.configured != nil {
				entry["max_turns"] = tc.configured
			}
			configRoot := writeTurnsConfig(t, entry)
			working := t.TempDir()
			if err := os.WriteFile(filepath.Join(working, "evidence.txt"), []byte("evidence"), 0o600); err != nil {
				t.Fatal(err)
			}
			request := execution.Request{
				ID: "turns", ConfigDir: configRoot, WorkingDir: working, Prompt: "inspect evidence", Model: "default",
				Sandbox: execution.Sandbox{Mode: execution.SandboxReadOnly},
			}
			if tc.gateway {
				gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var rpc struct{ Method string }
					_ = json.NewDecoder(r.Body).Decode(&rpc)
					result := map[string]any{"content": []any{map[string]string{"type": "text", "text": "evidence"}}}
					if rpc.Method == "tools/list" {
						result = map[string]any{"tools": []any{map[string]any{
							"name": toolName, "inputSchema": map[string]string{"type": "object"},
						}}}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": 1, "result": result})
				}))
				defer gateway.Close()
				request.ToolGateway = gateway.URL
			}
			if tc.workspace {
				request.Sandbox.Mode = execution.SandboxWorkspaceWrite
				request.ToolGatewayScope = execution.GatewayScopeWithWorkspace
				request.WritableRoots = []string{working}
			}
			input, err := json.Marshal(executionEnvelope{Version: 3, Operation: "execute", Request: &request})
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			factory := func(ctx context.Context, dir string, sandbox execution.Sandbox, gateway toolGateway) (execution.Provider, error) {
				return newConfiguredAPIProvider(ctx, dir, "chosen", sandbox, gateway)
			}
			if err := serveExecutionProtocol(context.Background(), bytes.NewReader(input), &output, factory); err != nil {
				t.Fatal(err)
			}
			calls := 0
			var result *execution.Result
			decoder := json.NewDecoder(&output)
			for decoder.More() {
				var envelope executionEnvelope
				if err := decoder.Decode(&envelope); err != nil {
					t.Fatal(err)
				}
				if envelope.Error != "" {
					t.Fatal(envelope.Error)
				}
				if envelope.Event != nil && envelope.Event.Type == execution.EventToolCompleted {
					calls++
					if !strings.Contains(envelope.Event.Text, "evidence") {
						t.Fatalf("tool did not execute successfully: %+v", envelope.Event)
					}
				}
				if envelope.Result != nil {
					result = envelope.Result
				}
			}
			if calls != tc.wantCalls || completions.Load() != int64(tc.wantCalls+1) {
				t.Fatalf("tool rounds = %d, completions = %d; want %d rounds plus final answer", calls, completions.Load(), tc.wantCalls)
			}
			if result == nil || result.FinalText != "finished" {
				t.Fatalf("missing terminal answer: %+v", result)
			}
			if tc.finishAt > 0 {
				if result.Outcome != execution.OutcomeSucceeded || result.Reason != "" {
					t.Fatalf("useful work past 16 rounds did not finish: %+v", result)
				}
			} else if result.Outcome != execution.OutcomeInconclusive || result.Reason != execution.ReasonMaxTurns {
				t.Fatalf("configured cap was not enforced: %+v", result)
			}
			// Loading/saving through the existing project path retains the override.
			config, err := project.LoadProjectConfig(configRoot)
			if err != nil {
				t.Fatal(err)
			}
			if err := project.SaveProjectConfig(config); err != nil {
				t.Fatal(err)
			}
			loaded, err := project.LoadProjectConfig(configRoot)
			if err != nil || loaded.Execution.Providers["chosen"].MaxTurns != config.Execution.Providers["chosen"].MaxTurns {
				t.Fatalf("max_turns did not persist: %v", err)
			}
		})
	}
}

func TestConfiguredAPIMaxTurnsRejectsInvalidValues(t *testing.T) {
	for _, value := range []any{-1, 1.5, "100"} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			root := writeTurnsConfig(t, map[string]any{
				"base_url": "http://127.0.0.1:1/v1", "api_key": "local", "model": "fixture-model", "max_turns": value,
			})
			_, err := newConfiguredAPIProvider(context.Background(), root, "chosen", readOnly, toolGateway{})
			if err == nil || !strings.Contains(err.Error(), "max_turns") {
				t.Fatalf("invalid max_turns %v was not refused: %v", value, err)
			}
		})
	}
}

func writeTurnsConfig(t *testing.T, entry map[string]any) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".openexec"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]any{"name": "fixture", "execution": map[string]any{
		"active_provider": "other", "providers": map[string]any{"chosen": entry, "other": map[string]any{
			"base_url": "http://127.0.0.1:1/v1", "api_key": "local", "model": "other-model", "max_turns": 1,
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".openexec", "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}
