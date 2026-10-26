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
	"strings"
	"sync"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// frontTruncatingServer reproduces the downstream mechanism recorded for the
// post-#150 failure of task c6b359c3 (turn.error sequence 36 after 17 completed
// read-only tool calls): a local Ollama endpoint drops the oldest messages of an
// oversized history before rendering the Qwen template, so the user turn is lost
// and the template answers HTTP 500 "No user query found in messages". The
// original serialized request was not retained; only this mechanism, with the
// observed 131072-byte read_file responses, was reproduced. Only role, order
// and byte sizes are recorded.
func frontTruncatingServer(t *testing.T, budget int, reply func(n int) string) (*httptest.Server, func() [][]int) {
	t.Helper()
	var mu sync.Mutex
	var sizes [][]int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var req wireRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Errorf("decode: %v", err)
		}
		shape := make([]int, len(req.Messages))
		total := 0
		for i, m := range req.Messages {
			shape[i] = len(m.Content)
			total += len(m.Content)
		}
		mu.Lock()
		sizes = append(sizes, shape)
		n := len(sizes)
		mu.Unlock()
		kept := req.Messages
		for total > budget && len(kept) > 1 {
			total -= len(kept[0].Content)
			kept = kept[1:]
		}
		hasQuery := false
		for _, m := range kept {
			if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
				hasQuery = true
			}
		}
		if !hasQuery {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"error":{"message":"Jinja Exception: No user query found in messages."}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, reply(n))
	}))
	t.Cleanup(server.Close)
	return server, func() [][]int {
		mu.Lock()
		defer mu.Unlock()
		return append([][]int(nil), sizes...)
	}
}

func readFileCall(id, args string) string {
	b, _ := json.Marshal(args)
	return fmt.Sprintf(`{"id":"x","model":"test-model","choices":[{"index":0,"finish_reason":"tool_calls","message":{"role":"assistant","tool_calls":[{"id":%q,"type":"function","function":{"name":"read_file","arguments":%s}}]}}]}`, id, b)
}

func workspaceProvider(t *testing.T, url string, steps int) *APIProvider {
	t.Helper()
	openAI, err := agent.NewOpenAIProvider(agent.OpenAIProviderConfig{
		Name: "compatible", APIKey: "k", BaseURL: url, Models: []string{"test-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: openAI, ToolExecutor: NewWorkspaceToolExecutor(), MaxSteps: steps,
		Tools: WorkspaceTools(Sandbox{Mode: SandboxReadOnly}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

// Two reads of a file larger than the old 131072-byte cap, then synthesis, over
// the real OpenAI-compatible transport and the real workspace executor. Before
// the 8192-byte page bound the second request carried >260000 bytes of tool
// history, the endpoint front-truncated the user turn away and the run failed
// with the observed HTTP 500; with it the user turn survives every request.
func TestLargeFileReadsKeepUserTurnWithinFrontTruncatingEndpoint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "big.go"), []byte(strings.Repeat("// line of source\n", 20000)), 0600); err != nil {
		t.Fatal(err)
	}
	server, shapes := frontTruncatingServer(t, 200000, func(n int) string {
		switch n {
		case 1:
			return readFileCall("c1", `{"path":"big.go"}`)
		case 2:
			return readFileCall("c2", `{"path":"big.go","offset_bytes":8192}`)
		case 3:
			// Unsupported input: the tool refuses it safely and the model
			// still gets a usable answer afterwards.
			return readFileCall("c3", `{"path":"big.go","limit_bytes":131072}`)
		}
		return textReply("CONTRACT: usable")
	})
	provider := workspaceProvider(t, server.URL, 8)
	var tools []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "pages", WorkingDir: root, Prompt: "ORIGINAL-QUERY", Model: "test-model",
		Sandbox: Sandbox{Mode: SandboxReadOnly},
	}, func(e Event) error {
		if e.Type == EventToolCompleted {
			tools = append(tools, e)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run failed: %v (request shapes %v)", err, shapes())
	}
	if result.Outcome != OutcomeSucceeded || result.FinalText != "CONTRACT: usable" {
		t.Fatalf("result = %+v", result)
	}
	if len(tools) != 3 {
		t.Fatalf("tool results = %d", len(tools))
	}
	for i, e := range tools[:2] {
		if len(e.Text) > 8192+128 {
			t.Fatalf("read %d returned %d bytes", i, len(e.Text))
		}
	}
	if !strings.Contains(tools[1].Text, "continue with offset_bytes=") {
		t.Fatalf("continuation missing: %q", tools[1].Text[len(tools[1].Text)-80:])
	}
	if !strings.Contains(tools[2].Text, "limit_bytes must be between") {
		t.Fatalf("unsupported input not refused: %q", tools[2].Text)
	}
}
