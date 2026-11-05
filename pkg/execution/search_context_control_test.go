package execution

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Reuse each call position's ID on later rounds, as compatible endpoints may.
// Calls within the same round still have distinct IDs and can be associated.
type reusedIDNativeAdapter struct{ *agent.OpenAIProvider }

func (a *reusedIDNativeAdapter) CompleteBounded(ctx context.Context, req agent.Request, input, output int) (*agent.Response, error) {
	response, err := a.OpenAIProvider.CompleteBounded(ctx, req, input, output)
	if err == nil && response != nil {
		index := 0
		for i := range response.Content {
			if response.Content[i].Type == agent.ContentTypeToolUse {
				response.Content[i].ToolUseID = fmt.Sprintf("reused-%d", index)
				index++
			}
		}
	}
	return response, err
}

// A synthetic native run, not a replay of the truncated Solar capture. It uses
// the real workspace search and native admission with a scripted local endpoint.
// lineWidth selects whether every scan completes (580) or truncates lines (1100).
func searchHeavyRun(t *testing.T, lineWidth int, script func(n int, body string, recalled *bool) map[string]any) (Result, error, []nativeWireRequest, map[string]int) {
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
			fmt.Fprintf(&content, "needle file-%02d line-%d %s\n", i, line, strings.Repeat(string(rune('a'+i%26)), lineWidth))
		}
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("evidence-%02d.txt", i)), []byte(content.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var seen *[]nativeWireRequest
	recalled := false
	server, seen := nativeTemplateServer(t, func(n int) map[string]any {
		body, _ := json.Marshal((*seen)[n-1])
		return script(n, string(body), &recalled)
	})
	p, err := NewAPIProvider(APIProviderConfig{Adapter: &reusedIDNativeAdapter{newNativeProvider(t, server.URL)}, ToolExecutor: NewWorkspaceToolExecutor(), Tools: WorkspaceTools(Sandbox{Mode: SandboxReadOnly}), MaxSteps: 10})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int{}
	result, err := p.Execute(context.Background(), Request{ID: "synthetic-search", WorkingDir: root, Prompt: "Inspect the evidence and advance the original task.", System: "Owner: never deploy without review.", Model: "local", Sandbox: Sandbox{Mode: SandboxReadOnly}, TokenBudget: 250000, ContextTokenLimit: 32768}, func(e Event) error {
		if e.Type == EventToolCompleted {
			ids[e.CallID]++
		}
		return nil
	})
	return result, err, *seen, ids
}

func searchRound(n int) map[string]any {
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
	return map[string]any{"done": true, "prompt_eval_count": n * 2000, "eval_count": 100, "message": map[string]any{"tool_calls": toolCalls}}
}

// Complete scans carry position-bound provenance from the real search, so the
// old ones are omitted only at admission, recalled by page, and the task ends.
func TestSearchHeavyNativeSequenceCompactsCompleteScansRecallsAndAdvances(t *testing.T) {
	pointer := regexp.MustCompile(`Prior complete search result retained as ([0-9a-f]{64})`)
	var recalledText string
	result, err, seen, ids := searchHeavyRun(t, 580, func(n int, body string, recalled *bool) map[string]any {
		match := pointer.FindStringSubmatch(body)
		switch {
		case *recalled:
			recalledText = body
			return map[string]any{"done": true, "prompt_eval_count": 3000, "eval_count": 50, "message": map[string]any{"content": "Original task advanced"}}
		case match != nil:
			*recalled = true
			return map[string]any{"done": true, "prompt_eval_count": 3000, "eval_count": 50, "message": map[string]any{"tool_calls": []map[string]any{{"function": map[string]any{"name": recallTool, "arguments": map[string]any{"id": match[1], "offset": 0}}}}}}
		case n > 6:
			t.Errorf("no compaction after %d rounds", n)
			return map[string]any{"done": true, "message": map[string]any{"content": "stop"}}
		}
		return searchRound(n)
	})
	if err != nil || result.Outcome != OutcomeSucceeded || !strings.Contains(result.FinalText, "Original task advanced") {
		t.Fatalf("original task did not advance: result=%+v err=%v", result, err)
	}
	if ids["reused-0"] < 2 {
		t.Fatalf("expected a tool-call ID reused across rounds: %v", ids)
	}
	if !strings.Contains(recalledText, "file-") || !strings.Contains(recalledText, "needle file-0") {
		t.Fatal("paged recall did not return the omitted search evidence")
	}
	for _, request := range seen[1:] {
		body, _ := json.Marshal(request)
		if !strings.Contains(string(body), "Never deploy without review") || !strings.Contains(string(body), "Owner: never deploy without review") {
			t.Fatal("governing instruction missing or altered in a later request")
		}
	}
	last, _ := json.Marshal(seen[len(seen)-1])
	if !strings.Contains(string(last), "needle Instruction: preserve the original authority") && !strings.Contains(recalledText, "needle Instruction: preserve the original authority") {
		t.Fatal("authoritative search evidence lost")
	}
}

// Scans that truncate lines are incomplete: no provenance, so no omission, and
// the original pre-dispatch refusal is unchanged.
func TestSearchHeavyNativeAdmissionRefusesIncompleteScans(t *testing.T) {
	result, err, seen, ids := searchHeavyRun(t, 1100, func(n int, body string, _ *bool) map[string]any {
		if strings.Contains(body, "retained as") {
			t.Error("an incomplete scan was omitted")
		}
		r := searchRound(n)
		r["prompt_eval_count"] = n * 7000
		return r
	})
	var overflow *agent.ContextOverflowError
	if !errors.As(err, &overflow) || overflow.Dispatched || result.Outcome != OutcomeFailed {
		t.Fatalf("expected pre-dispatch context refusal: result=%+v err=%v", result, err)
	}
	if len(seen) < 2 || ids["reused-0"] < 2 {
		t.Fatalf("expected multiple admitted rounds with reused IDs: seen=%d ids=%v", len(seen), ids)
	}
	last, _ := json.Marshal(seen[len(seen)-1])
	if !strings.Contains(string(last), "Never deploy without review") || !strings.Contains(string(last), "needle Instruction: preserve the original authority") || !strings.Contains(string(last), "file-00") {
		t.Fatal("authoritative file or search evidence missing from admitted history")
	}
}
