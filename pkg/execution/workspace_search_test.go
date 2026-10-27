package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func searchRequest(root, path, text string) ToolRequest {
	input, _ := json.Marshal(map[string]string{"path": path, "text": text})
	return ToolRequest{Name: "search_files", Input: input, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxReadOnly}}
}

func searchFile(t *testing.T, root, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// The real triage guessed store files for sixteen rounds looking for SplitTask.
// Search must find its definition after the first 8 KiB without paging each file.
func TestWorkspaceSearchFindsDefinitionAndCompletesTriage(t *testing.T) {
	root := t.TempDir()
	searchFile(t, root, "internal/store/blocked.go", strings.Repeat("// padding\n", 1000)+"func (s *Store) SplitTask() {}\n")
	for i := 0; i < 20; i++ {
		searchFile(t, root, fmt.Sprintf("internal/store/other%d.go", i), "package store\n")
	}
	tools := WorkspaceTools(Sandbox{Mode: SandboxReadOnly})
	found := false
	for _, tool := range tools {
		if tool.Name == "search_files" {
			found = true
		}
	}
	if !found {
		t.Fatal("read-only triage was not offered search_files")
	}
	adapter := &fakeAPIAdapter{responses: []*agent.Response{
		{Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: "find", ToolName: "search_files", ToolInput: searchRequest(root, "internal/store", "SplitTask").Input}}},
		{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "Contract grounded in internal/store/blocked.go.\nREPOSITORY: agent-console"}}},
	}}
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, Tools: tools, ToolExecutor: NewWorkspaceToolExecutor(), MaxSteps: 2})
	if err != nil {
		t.Fatal(err)
	}
	var evidence string
	result, err := provider.Execute(context.Background(), Request{ID: "search-triage", WorkingDir: root, Model: "test-model", Prompt: "Locate SplitTask and draft its contract", Sandbox: Sandbox{Mode: SandboxReadOnly}}, func(e Event) error {
		if e.Type == EventToolCompleted {
			evidence = e.Text
		}
		return nil
	})
	if err != nil || result.Outcome != OutcomeSucceeded || !strings.Contains(evidence, "internal/store/blocked.go:1001:func (s *Store) SplitTask() {}") {
		t.Fatalf("search-backed triage failed: outcome=%s evidence=%q err=%v", result.Outcome, evidence, err)
	}
	if len(adapter.requests) != 2 || !strings.Contains(fmt.Sprint(adapter.requests[1].Messages), "blocked.go:1001:") {
		t.Fatal("search evidence did not reach the next native completion")
	}
}

func TestWorkspaceSearchContainsReadsAndSkipsPrivateFiles(t *testing.T) {
	root, outside := workspace(t)
	searchFile(t, root, "visible.go", "Needle ordinary\n")
	searchFile(t, root, ".openexec/config.json", "Needle credential\n")
	searchFile(t, root, ".git/config", "Needle git-secret\n")
	searchFile(t, root, ".uaos/config.json", "Needle legacy-secret\n")
	searchFile(t, root, "binary", "\x00Needle binary\n")
	searchFile(t, outside, "outside", "Needle outside-secret\n")
	for link, target := range map[string]string{"external": outside, "private": ".openexec", "filelink": filepath.Join(outside, "outside")} {
		if err := os.Symlink(target, filepath.Join(root, link)); err != nil {
			t.Fatal(err)
		}
	}
	e := NewWorkspaceToolExecutor()
	got, err := e.ExecuteTool(context.Background(), searchRequest(root, ".", "Needle"))
	if err != nil || got != "visible.go:1:Needle ordinary\n" {
		t.Fatalf("search leaked or missed content: %q %v", got, err)
	}
	for _, path := range []string{outside, "../secrets", ".git", ".openexec", "private/config.json", "external/outside", "filelink"} {
		t.Run(path, func(t *testing.T) {
			got, err := e.ExecuteTool(context.Background(), searchRequest(root, path, "Needle"))
			if err == nil && strings.Contains(got, "Needle") {
				t.Fatalf("search escaped through %s: %q", path, got)
			}
			if path == outside && err == nil {
				t.Fatal("ungranted root was accepted")
			}
		})
	}
	request := searchRequest(root, outside, "Needle")
	request.ReadableRoots = []string{outside}
	got, err = e.ExecuteTool(context.Background(), request)
	if err != nil || !strings.Contains(got, "outside:1:Needle outside-secret") {
		t.Fatalf("explicit readable root was not searched: %q %v", got, err)
	}
	if _, err := e.ExecuteTool(context.Background(), writeRequest(root, "forbidden", "x", nil)); err == nil {
		t.Fatal("search conferred writing authority")
	}
}

func TestWorkspaceSearchLimitsAndCancellation(t *testing.T) {
	for _, test := range []struct{ name, content string }{
		{"output", strings.Repeat("Needle\n", maxReadFileBytes)},
		{"bytes", strings.Repeat("x", maxSearchBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			searchFile(t, root, "a", test.content)
			got, err := NewWorkspaceToolExecutor().ExecuteTool(context.Background(), searchRequest(root, ".", "Needle"))
			if err != nil || !strings.Contains(got, "incomplete search") || len(got) > maxReadFileBytes {
				t.Fatalf("missing bounded incomplete result: bytes=%d suffix=%q err=%v", len(got), got[max(0, len(got)-100):], err)
			}
		})
	}
	t.Run("entries", func(t *testing.T) {
		root := t.TempDir()
		for i := 0; i <= maxSearchEntries; i++ {
			if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d%04d", i)), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		got, err := NewWorkspaceToolExecutor().ExecuteTool(context.Background(), searchRequest(root, ".", "Needle"))
		if err != nil || !strings.Contains(got, "incomplete search") {
			t.Fatalf("unbounded traversal: %q %v", got, err)
		}
	})
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewWorkspaceToolExecutor().ExecuteTool(ctx, searchRequest(root, ".", "Needle")); err == nil {
		t.Fatal("cancelled search ran")
	}
	for _, text := range []string{"", "a\nb"} {
		if _, err := NewWorkspaceToolExecutor().ExecuteTool(context.Background(), searchRequest(root, ".", text)); err == nil {
			t.Fatal("invalid literal was accepted")
		}
	}
	got, err := NewWorkspaceToolExecutor().ExecuteTool(context.Background(), searchRequest(root, ".", "Needle"))
	if err != nil || !strings.HasPrefix(got, "No matches") {
		t.Fatalf("empty search was not explicit: %q %v", got, err)
	}
}
