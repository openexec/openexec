package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type retrievalJourneyTools struct {
	recordingToolExecutor
	root  string
	reads int
}

type distinctEvidenceTools struct {
	recordingToolExecutor
	root  string
	reads int
}

func (e *distinctEvidenceTools) ExecuteTool(_ context.Context, r ToolRequest) (string, error) {
	if r.Name == "read_record" {
		e.reads++
		return fmt.Sprintf("record-%d ", e.reads) + strings.Repeat("distinct evidence ", 850), nil
	}
	if r.Name == "finish" {
		return "original task advanced", os.WriteFile(filepath.Join(e.root, "done"), []byte("verified"), 0600)
	}
	return "", fmt.Errorf("unexpected tool %s", r.Name)
}

func TestDistinctPriorEvidenceCanBeRecalledAndOriginalTaskContinues(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			io.WriteString(w, `{"version":"0.32.13"}`)
			return
		}
		calls++
		body, _ := io.ReadAll(r.Body)
		message := map[string]any{"content": "Original task verified"}
		call := func(name string, args map[string]any) map[string]any {
			return map[string]any{"function": map[string]any{"name": name, "arguments": args}}
		}
		switch calls {
		case 1:
			list := []any{}
			for i := 0; i < 6; i++ {
				list = append(list, call("read_record", map[string]any{"id": i}))
			}
			message = map[string]any{"tool_calls": list}
		case 2:
			match := regexp.MustCompile(`Prior tool result retained as ([0-9a-f]{64})`).FindSubmatch(body)
			if len(match) != 2 || !strings.Contains(string(body), "no deployment without review") {
				t.Error("compaction lost original authority or prior result reference")
				return
			}
			message = map[string]any{"tool_calls": []any{call(recallTool, map[string]any{"id": string(match[1]), "offset": 0})}}
		case 3:
			if !strings.Contains(string(body), "record-1") {
				t.Error("recalled evidence missing")
			}
			message = map[string]any{"tool_calls": []any{call("finish", map[string]any{})}}
		}
		json.NewEncoder(w).Encode(map[string]any{"done": true, "prompt_eval_count": 1000, "eval_count": 100, "message": message})
	}))
	defer server.Close()
	adapter, _ := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{APIKey: "local", BaseURL: server.URL + "/v1", Models: []string{"test-model"}})
	if !adapter.EnableLocalOllamaBounds(context.Background()) {
		t.Fatal("native protocol unavailable")
	}
	root := t.TempDir()
	executor := &distinctEvidenceTools{root: root}
	p, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, ToolExecutor: executor, Tools: []agent.ToolDefinition{{Name: "read_record", InputSchema: json.RawMessage(`{"type":"object"}`)}, {Name: "finish", InputSchema: json.RawMessage(`{"type":"object"}`)}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Execute(context.Background(), Request{ID: "original", WorkingDir: root, Prompt: "Advance original task; no deployment without review", Model: "test-model", Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{root}, TokenBudget: 250000, ContextTokenLimit: 32768}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(root, "done"))
	if err != nil || string(state) != "verified" || executor.reads != 6 || calls != 4 {
		t.Fatalf("not advanced: calls=%d reads=%d err=%v", calls, executor.reads, err)
	}
}

func TestPriorResultRecallKeepsOriginalAndRejectsInvalidRequests(t *testing.T) {
	results := priorResults{}
	request := agent.Request{System: "authority", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "original request")}}
	for i := 0; i < 6; i++ {
		request.Messages = append(request.Messages, agent.NewToolResultMessage(fmt.Sprintf("call-%d", i), fmt.Sprintf("item-%d", i)+strings.Repeat("ä", 2500), nil))
	}
	original, _ := json.Marshal(request)
	compacted, changed := results.compact(request)
	after, _ := json.Marshal(request)
	if !changed || string(original) != string(after) || compacted.System != request.System || len(results) != 5 {
		t.Fatal("compaction lost history or authority")
	}
	var id string
	for key := range results {
		id = key
		break
	}
	page, err := results.read(json.RawMessage(fmt.Sprintf(`{"id":%q,"offset":0}`, id)))
	if err != nil || !strings.Contains(page, "item-") {
		t.Fatalf("first page: %s %v", page, err)
	}
	for _, bad := range []string{`{}`, `{"id":"foreign","offset":0}`, fmt.Sprintf(`{"id":%q,"offset":-1}`, id), fmt.Sprintf(`{"id":%q,"offset":999999}`, id)} {
		if _, err := results.read(json.RawMessage(bad)); err == nil {
			t.Fatalf("accepted invalid request %s", bad)
		}
	}
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
