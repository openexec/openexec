package execution

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type retrievalJourneyTools struct {
	recordingToolExecutor
	root  string
	reads int
}

func (e *retrievalJourneyTools) ExecuteTool(_ context.Context, r ToolRequest) (string, error) {
	// Synthetic enlargement preserves one authoritative boundary and exact
	// repeated evidence. It is not claimed to be the historical provider trace.
	if r.Name == "read_record" {
		e.reads++
		return `{"id":"original-task","authority":"no deployment without review","evidence":"` + strings.Repeat("retained evidence ", 1500) + `"}`, nil
	}
	if err := os.WriteFile(filepath.Join(e.root, "original-task-complete"), []byte("verified"), 0600); err != nil {
		return "", err
	}
	return "original task advanced", nil
}

func TestSecondInferenceAfterRetrievalCompactsDuplicatesAndAdvancesOriginalTask(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			io.WriteString(w, `{"version":"0.32.13"}`)
			return
		}
		calls++
		body, _ := io.ReadAll(r.Body)
		if len(body) > 50000 {
			t.Errorf("aggregate known overflow dispatched: %d", len(body))
		}
		message := map[string]any{"content": "Original task verified"}
		call := func(name string) map[string]any {
			return map[string]any{"function": map[string]any{"name": name, "arguments": map[string]any{}}}
		}
		if calls == 1 {
			message = map[string]any{"tool_calls": []any{call("read_record"), call("read_record"), call("read_record")}}
		}
		if calls == 2 {
			if strings.Count(string(body), "duplicateOfToolResult") != 2 || !strings.Contains(string(body), "no deployment without review") {
				t.Error("compaction lost identity/boundary or retained duplicates")
			}
			message = map[string]any{"tool_calls": []any{call("advance_original_task")}}
		}
		json.NewEncoder(w).Encode(map[string]any{"done": true, "prompt_eval_count": 1000, "eval_count": 100, "message": message})
	}))
	defer server.Close()
	adapter, _ := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{APIKey: "local", BaseURL: server.URL + "/v1", Models: []string{"test-model"}})
	if !adapter.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("native protocol unavailable")
	}
	root := t.TempDir()
	executor := &retrievalJourneyTools{root: root}
	p, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, ToolExecutor: executor, Tools: []agent.ToolDefinition{{Name: "read_record", InputSchema: json.RawMessage(`{"type":"object"}`)}, {Name: "advance_original_task", InputSchema: json.RawMessage(`{"type":"object"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Execute(context.Background(), Request{ID: "original-task", WorkingDir: root, Prompt: "Read durable evidence and advance original-task. No deployment without review.", Model: "test-model", Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{root}, TokenBudget: 65536}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(root, "original-task-complete"))
	if err != nil || string(state) != "verified" || executor.reads != 3 || calls != 3 {
		t.Fatalf("journey did not advance: calls%d reads%d err%v", calls, executor.reads, err)
	}
}
