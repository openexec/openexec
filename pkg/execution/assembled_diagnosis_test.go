package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type assembledDiagnosisTools struct {
	recordingToolExecutor
	reads  int
	mode   string
	cancel context.CancelFunc
}

func (e *assembledDiagnosisTools) ExecuteTool(context.Context, ToolRequest) (string, error) {
	e.reads++
	if e.mode == "cancel" && e.reads == 2 {
		e.cancel()
	}
	output := strings.Repeat("e", 90000)
	if e.mode != "duplicate" && e.reads == 2 {
		output += "different"
	}
	return output, nil
}

// Synthetic protocol controls, NOT an original Solar replay. Drive the native
// API loop through real admission, reservation retry, duplicate compaction,
// capture, trace and terminal accounting. Only the local server is scripted.
func TestAssembledDiagnosisPreflightPersistence(t *testing.T) {
	for _, mode := range []string{"near-duplicate", "duplicate", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				calls++
				var wire map[string]any
				if json.NewDecoder(r.Body).Decode(&wire) != nil || r.Method != "POST" || r.URL.Path != "/api/chat" || wire["truncate"] != false || wire["stream"] != false {
					t.Error("native provider shape changed")
				}
				options := asMap(wire["options"])
				output := options["num_predict"].(float64)
				if options["num_ctx"] != float64(65536) || output < 2048 || output >= 65536 {
					t.Error("reservation adjustment or fixed context boundary lost")
				}
				if calls == 1 {
					fmt.Fprint(w, `{"done":true,"prompt_eval_count":1000,"eval_count":10,"message":{"tool_calls":[{"function":{"name":"read","arguments":{}}},{"function":{"name":"read","arguments":{}}}]}}`)
					return
				}
				if mode != "duplicate" {
					t.Error("non-duplicate overflow or cancelled request dispatched")
				}
				raw, _ := json.Marshal(wire)
				if strings.Count(string(raw), "duplicateOfToolResult") != 1 || !strings.Contains(string(raw), strings.Repeat("e", 90000)) {
					t.Error("exact duplicate compaction lost complete retained result")
				}
				w.WriteHeader(400)
				fmt.Fprint(w, `{"error":"synthetic-private-provider-text: context exceeded"}`)
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			adapter := newNativeProvider(t, server.URL)
			root := t.TempDir()
			capturePath, tracePath := filepath.Join(root, "private", "capture.json"), filepath.Join(root, "private", "trace.json")
			t.Setenv(RequestCaptureEnv, capturePath)
			t.Setenv(RequestTraceEnv, tracePath)
			executor := &assembledDiagnosisTools{mode: mode, cancel: cancel}
			provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, MaxSteps: 4, ToolExecutor: executor,
				Tools: []agent.ToolDefinition{{Name: "read", InputSchema: json.RawMessage(`{"type":"object"}`)}}})
			if err != nil {
				t.Fatal(err)
			}
			var usage Event
			result, err := provider.Execute(ctx, Request{ID: "synthetic-preflight", WorkingDir: root, Model: "local",
				System: "synthetic authority", Prompt: strings.Repeat("p", 6000) + " " + fixtureSecret,
				Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: 250000, ContextTokenLimit: 65536}, func(e Event) error {
				if e.Type == EventUsage {
					usage = e
				}
				return nil
			})
			var overflow *agent.ContextOverflowError
			if mode == "cancel" {
				if !errors.Is(err, context.Canceled) || result.Outcome != OutcomeCancelled || calls != 1 {
					t.Fatal("Stop at the tool boundary was not retained")
				}
			} else if !errors.As(err, &overflow) || overflow.Dispatched != (mode == "duplicate") || overflow.ContextTokens != 65536 || result.Outcome != OutcomeFailed {
				t.Fatal("preflight and dispatched refusal were conflated")
			}
			wantCalls := 1
			if mode == "duplicate" {
				wantCalls = 2
			}
			if calls != wantCalls || executor.reads != 2 || usage.InputTokens != 1000 || usage.OutputTokens != 10 || usage.UsageFinal != (mode != "duplicate") {
				t.Fatal("retry, observed usage or uncertainty changed")
			}
			if err == nil || strings.Contains(err.Error(), fixtureSecret) || strings.Contains(err.Error(), "synthetic-private-provider-text") {
				t.Fatal("private text reached refusal diagnostic")
			}
			// Re-open both persisted files; inspect the actual saved state.
			capture, trace := readCapture(t, capturePath), readTrace(t, tracePath)
			capRaw, _ := os.ReadFile(capturePath)
			traceRaw, _ := os.ReadFile(tracePath)
			if strings.Contains(string(capRaw), fixtureSecret) || strings.Contains(string(traceRaw), fixtureSecret) || capture["truncated"] != true || capture["textLimit"] != float64(2000) {
				t.Fatal("capture privacy or truncation disclosure changed")
			}
			if trace["incomplete"] != false || trace["redactionChangedContent"] != true || trace["imagesOmitted"] != false || trace["droppedEntries"] != float64(0) {
				t.Fatal("trace fidelity disclosure changed")
			}
			for _, path := range []string{capturePath, tracePath, filepath.Dir(tracePath)} {
				info, statErr := os.Stat(path)
				want := os.FileMode(0600)
				if path == filepath.Dir(tracePath) {
					want = 0700
				}
				if statErr != nil || info.Mode().Perm() != want {
					t.Fatal("private persistence permissions changed")
				}
			}
			assertDispatch(t, asMap(capture["dispatch"]), "POST", server.URL+"/api/chat")
			messages := asList(asMap(capture["request"])["messages"])
			wantMessages := 2 // Earlier successful dispatch, not refused assembly.
			if mode == "duplicate" {
				wantMessages = 5
			}
			if len(messages) != wantMessages {
				t.Fatal("capture attached to wrong request")
			}
			entries := asList(trace["entries"])
			if mode == "cancel" {
				if len(entries) != 1 {
					t.Fatal("Stop caused another model call")
				}
				return
			}
			if len(entries) != 2 {
				t.Fatal("trace lost the refused loop request")
			}
			last := asMap(entries[1])
			_, hasWire := last["wire"]
			_, hasDispatch := last["dispatch"]
			if hasWire != (mode == "duplicate") || hasDispatch != hasWire || len(asList(asMap(last["request"])["messages"])) != 4 {
				t.Fatal("trace invented refused wire/dispatch or lost pre-compaction request")
			}
			if !strings.Contains(string(traceRaw), strings.Repeat("e", 90000)) {
				t.Fatal("trace truncated the requested tool result")
			}
		})
	}
}

func TestAssembledDiagnosisFirstRefusalCannotCapturePreviousRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			fmt.Fprint(w, `{"version":"0.32.13"}`)
			return
		}
		fmt.Fprint(w, `{"done":true,"prompt_eval_count":10,"eval_count":2,"message":{"content":"old run"}}`)
	}))
	defer server.Close()
	adapter := newNativeProvider(t, server.URL)
	if _, err := adapter.CompleteBounded(context.Background(), agent.Request{Model: "local"}, 2048, 128); err != nil {
		t.Fatal(err)
	}
	_, at, _, _ := adapter.LastRequestWithDispatch()
	root := t.TempDir()
	path := filepath.Join(root, "private", "capture.json")
	tracePath := filepath.Join(root, "private", "trace.json")
	t.Setenv(RequestCaptureEnv, path)
	t.Setenv(RequestTraceEnv, tracePath)
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, ToolExecutor: &recordingToolExecutor{}, Tools: []agent.ToolDefinition{{Name: "read"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = provider.Execute(context.Background(), Request{ID: "synthetic-first-refusal", WorkingDir: root, Model: "local",
		Prompt: strings.Repeat("x", 180000), Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: 250000, ContextTokenLimit: 65536}, nil)
	var overflow *agent.ContextOverflowError
	if !errors.As(err, &overflow) || overflow.Dispatched {
		t.Fatal("first assembly was not refused before dispatch")
	}
	_, after, _, _ := adapter.LastRequestWithDispatch()
	if !at.Equal(after) {
		t.Fatal("first refusal invented a dispatch")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("capture reused an earlier run")
	}
	trace := readTrace(t, tracePath)
	entries := asList(trace["entries"])
	if len(entries) != 1 || asMap(entries[0])["wire"] != nil || asMap(entries[0])["dispatch"] != nil || asMap(entries[0])["request"] == nil {
		t.Fatal("trace must keep attempted loop request without stale wire provenance")
	}
}
