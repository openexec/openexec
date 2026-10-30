package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type replyObservationTools struct {
	recordingToolExecutor
	root string
}

func (e *replyObservationTools) ExecuteTool(_ context.Context, r ToolRequest) (string, error) {
	if r.Name == "read_source" {
		return strings.Repeat("source ", 900), nil
	}
	return "saved", os.WriteFile(filepath.Join(e.root, "checkpoint"), []byte("source-grounded draft"), 0600)
}

func TestReplyAllowanceChangePreservesNativeToolProgress(t *testing.T) {
	calls := 0
	var outputs []float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			w.Write([]byte(`{"version":"0.32.13"}`))
			return
		}
		calls++
		var wire map[string]any
		json.NewDecoder(r.Body).Decode(&wire)
		options := wire["options"].(map[string]any)
		outputs = append(outputs, options["num_predict"].(float64))
		if wire["truncate"] != false || options["num_ctx"] != float64(32768) {
			t.Error("context ceiling or no-truncation boundary changed")
		}
		message := map[string]any{"content": "draft saved"}
		if calls <= 2 {
			name := "read_source"
			if calls == 2 {
				name = "save_checkpoint"
			}
			message = map[string]any{"tool_calls": []any{map[string]any{"function": map[string]any{"name": name, "arguments": map[string]any{}}}}}
		}
		json.NewEncoder(w).Encode(map[string]any{"done": true, "prompt_eval_count": 18000, "eval_count": 500, "message": message})
	}))
	defer server.Close()
	root := t.TempDir()
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: newNativeProvider(t, server.URL),
		ToolExecutor: &replyObservationTools{root: root}, MaxSteps: 4,
		Tools: []agent.ToolDefinition{{Name: "read_source", InputSchema: json.RawMessage(`{"type":"object"}`)},
			{Name: "save_checkpoint", InputSchema: json.RawMessage(`{"type":"object"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	var usage Event
	result, err := provider.Execute(context.Background(), Request{ID: "original", WorkingDir: root,
		Prompt: "No deployment authority. Read source and save a draft. " + strings.Repeat("a", 68000), Model: "local",
		Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{root}, TokenBudget: 262144, ContextTokenLimit: 32768}, func(e Event) error {
		if e.Type == EventUsage {
			usage = e
		}
		return nil
	})
	if err != nil || result.Outcome != OutcomeSucceeded || calls != 3 {
		t.Fatalf("native tool journey failed: result=%+v calls=%d outputCaps=%v err=%v", result, calls, outputs, err)
	}
	saved, readErr := os.ReadFile(filepath.Join(root, "checkpoint"))
	if readErr != nil || string(saved) != "source-grounded draft" || outputs[0] == outputs[1] || !usage.UsageFinal || usage.InputTokens != 54000 || usage.OutputTokens != 1500 {
		t.Fatalf("persistence, changed reply reservation or native usage missing: saved=%q caps=%v usage=%+v err=%v", saved, outputs, usage, readErr)
	}
}
