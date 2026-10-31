package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type d26ReadTools struct{ recordingToolExecutor }

func (*d26ReadTools) ExecuteTool(context.Context, ToolRequest) (string, error) {
	return strings.Repeat("public repeated evidence ", 4000), nil
}

// A public protocol control, NOT a replay of d26 or a recovery demonstration.
// Drive Execute -> Complete -> native HTTP -> capture/trace, with repeated
// results large enough to expose accidental ordinary-route compaction.
func TestD26DiagnosticNativeAssemblyAndRefusalBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		overflow bool
	}{
		{"classified-refusal", 400, `{"error":"private-provider-text: context size exceeded"}`, true},
		{"different-400", 400, `{"error":"private-provider-text: context field missing"}`, false},
		{"different-status", 503, `{"error":"private-provider-text: context size exceeded"}`, false},
		{"success", 200, `{"done":true,"done_reason":"stop","prompt_eval_count":100,"eval_count":10,"message":{"content":"public answer"}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/version" {
					fmt.Fprint(w, `{"version":"0.32.13"}`)
					return
				}
				calls++
				if r.URL.Path != "/api/chat" || r.Method != "POST" {
					t.Error("native dispatch shape changed")
				}
				var body struct {
					Messages []map[string]any `json:"messages"`
					Options  map[string]any   `json:"options"`
					Tools    []any            `json:"tools"`
					Truncate *bool            `json:"truncate"`
					Stream   bool             `json:"stream"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if len(body.Options) != 0 || body.Truncate == nil || *body.Truncate || body.Stream || len(body.Tools) != 1 {
					t.Error("ordinary native defaults or tool schema changed")
				}
				if len(body.Tools) == 1 {
					function := asMap(asMap(body.Tools[0])["function"])
					if function["name"] != "read_record" || asMap(function["parameters"])["type"] != "object" {
						t.Error("native tool definition changed")
					}
				}
				if calls == 1 {
					fmt.Fprint(w, `{"done":true,"prompt_eval_count":100,"eval_count":10,"message":{"tool_calls":[{"function":{"name":"read_record","arguments":{}}},{"function":{"name":"read_record","arguments":{}}},{"function":{"name":"read_record","arguments":{}}}]}}`)
					return
				}
				if len(body.Messages) != 6 || body.Messages[0]["content"] != "public authority" || body.Messages[1]["content"] != "public task" {
					t.Error("original system/query/history lost")
				}
				results := 0
				for _, m := range body.Messages {
					if m["role"] == "tool" {
						results++
						if m["tool_name"] != "read_record" || m["content"] != strings.Repeat("public repeated evidence ", 4000) {
							t.Error("ordinary route compacted or rewrote tool history")
						}
					}
				}
				if results != 3 {
					t.Error("tool call/result binding lost")
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			adapter := fidelityAdapter(t, server.URL+"/v1")
			if !adapter.EnableLocalOllamaBounds(context.Background()) {
				t.Fatal("native negotiation failed")
			}
			root := t.TempDir()
			tracePath, capturePath := filepath.Join(root, "private", "trace.json"), filepath.Join(root, "private", "capture.json")
			t.Setenv(RequestTraceEnv, tracePath)
			t.Setenv(RequestCaptureEnv, capturePath)
			provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, MaxSteps: 5, ToolExecutor: &d26ReadTools{}, Tools: []agent.ToolDefinition{{Name: "read_record", InputSchema: json.RawMessage(`{"type":"object"}`)}}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := provider.Execute(context.Background(), Request{ID: "public-control", WorkingDir: root, Model: "synthetic-model", Prompt: "public task", System: "public authority", Sandbox: Sandbox{Mode: "read-only"}}, nil)
			if calls != 2 {
				t.Fatalf("unexpected retry or preflight: dispatches=%d", calls)
			}
			var overflow *agent.ContextOverflowError
			if errors.As(err, &overflow) != tc.overflow {
				t.Fatal("native refusal classifier mismatch")
			}
			if tc.overflow && (!overflow.Dispatched || overflow.ContextTokens != 0 || overflow.RequestBytes < 280000 || overflow.EstimatedTokens <= 0) {
				t.Fatalf("ordinary refusal boundary lost: %v", err)
			}
			if tc.status == 200 {
				if err != nil || result.Outcome != OutcomeSucceeded {
					t.Fatal("public success control failed")
				}
				return
			}
			if err == nil || result.Outcome != OutcomeFailed || strings.Contains(err.Error(), "private-provider-text") {
				t.Fatal("failure outcome or privacy boundary changed")
			}
			trace := readTrace(t, tracePath)
			// The failure event's estimated_tokens=... matches the existing
			// token-secret redactor even though the wire has no secret text.
			if trace["incomplete"] != false || trace["droppedEntries"] != float64(0) || trace["redactionChangedContent"] != tc.overflow || trace["imagesOmitted"] != false {
				t.Fatalf("trace flags: incomplete=%v dropped=%v redacted=%v images=%v", trace["incomplete"], trace["droppedEntries"], trace["redactionChangedContent"], trace["imagesOmitted"])
			}
			entries := asList(trace["entries"])
			if len(entries) != 2 || len(asList(asMap(asMap(entries[1])["wire"])["messages"])) != 6 {
				t.Fatal("trace lost refused native wire or earlier request")
			}
			if tc.overflow && !strings.Contains(asString(asMap(entries[1])["error"]), "estimated_tokens=<redacted>") {
				t.Fatal("trace failed to disclose estimate redaction")
			}
			assertDispatch(t, asMap(asMap(entries[1])["dispatch"]), "POST", server.URL+"/api/chat")
			capture := readCapture(t, capturePath)
			if capture["truncated"] != true || capture["textLimit"] != float64(2000) {
				t.Fatal("large tool results unexpectedly captured in full")
			}
		})
	}
}
