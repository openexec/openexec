package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// This is a synthetic control, not a replay of the truncated Solar capture.
// It uses the real workspace search and native admission, with a scripted local
// endpoint and synthetic, distinct files. It intentionally gives the current
// loop no provenance that would allow search output to be omitted safely.
func TestSearchHeavyNativeAdmissionRefusesWithoutSafeCompaction(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Never deploy without review.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		var content strings.Builder
		if i == 0 {
			content.WriteString("needle Instruction: preserve the original authority.\n")
		}
		for line := 0; line < 10; line++ {
			fmt.Fprintf(&content, "needle file-%02d line-%d %s\n", i, line, strings.Repeat(string(rune('a'+i%26)), 580))
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("evidence-%02d.txt", i)), []byte(content.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	server, seen := nativeTemplateServer(t, func(n int) map[string]any {
		calls = n
		var toolCalls []map[string]any
		add := func(name string, args map[string]any) {
			toolCalls = append(toolCalls, map[string]any{"function": map[string]any{"name": name, "arguments": args}})
		}
		if n == 1 {
			add("read_file", map[string]any{"path": "AGENTS.md"})
		}
		for i := 0; i < 4; i++ {
			add("search_files", map[string]any{"path": fmt.Sprintf("evidence-%02d.txt", (n-1)*4+i), "text": "needle"})
		}
		return map[string]any{"done": true, "prompt_eval_count": n * 7000, "eval_count": 100, "message": map[string]any{"tool_calls": toolCalls}}
	})
	p, err := NewAPIProvider(APIProviderConfig{Adapter: newNativeProvider(t, server.URL), ToolExecutor: NewWorkspaceToolExecutor(), Tools: WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), MaxSteps: 6})
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Execute(context.Background(), Request{ID: "synthetic-search", WorkingDir: root, Prompt: "Inspect the evidence and advance the original task.", System: "Owner: never deploy without review.", Model: "local", Sandbox: Sandbox{Mode: SandboxReadOnly}, TokenBudget: 250000, ContextTokenLimit: 32768}, nil)
	var overflow *agent.ContextOverflowError
	if !errors.As(err, &overflow) || overflow.Dispatched || result.Outcome != OutcomeFailed {
		t.Fatalf("expected pre-dispatch context refusal: result=%+v err=%v", result, err)
	}
	if calls < 2 || len(*seen) != calls {
		t.Fatalf("expected multiple admitted rounds: calls=%d seen=%d", calls, len(*seen))
	}
	last, _ := json.Marshal((*seen)[len(*seen)-1])
	if !strings.Contains(string(last), "Never deploy without review") || !strings.Contains(string(last), "needle Instruction: preserve the original authority") || !strings.Contains(string(last), "file-00") {
		t.Fatal("authoritative file or search evidence missing from admitted history")
	}
}
