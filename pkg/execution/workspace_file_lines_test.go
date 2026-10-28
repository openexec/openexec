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
	"unicode/utf8"
)

func lineReadRequest(root string, input map[string]any) ToolRequest {
	raw, _ := json.Marshal(input)
	return ToolRequest{Name: "read_file", Input: raw, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxReadOnly}}
}

// The natural 59614721 failure found OpenAPI line 6591, then guessed byte
// offsets for half an hour and never reached the definition. Exercise the
// actual search coordinates and reader together, without a shell or helpers.
func TestReadFileLineFromSearchReachesDefinition(t *testing.T) {
	root, _ := workspace(t)
	prefix := strings.Repeat("# variable-width å source\r\n", 6590)
	tail := "    Metrics:\r\n" + strings.Repeat("  property: å\r\n", 900)
	source := prefix + tail
	if err := os.WriteFile(filepath.Join(root, "openapi.yaml"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	executor := NewWorkspaceToolExecutor()
	match, err := executor.ExecuteTool(context.Background(), searchRequest(root, "openapi.yaml", "    Metrics:"))
	if err != nil {
		t.Fatal(err)
	}
	var line int64
	if _, err := fmt.Sscanf(match, "openapi.yaml:%d:", &line); err != nil || line != 6591 {
		t.Fatalf("search location = %q, %v", match, err)
	}
	out, err := executor.ExecuteTool(context.Background(), lineReadRequest(root, map[string]any{"path": "openapi.yaml", "start_line": line}))
	if err != nil || !strings.HasPrefix(out, "    Metrics:\r\n") {
		t.Fatalf("search line did not reach definition: %.100q, %v", out, err)
	}
	body, marker, more := strings.Cut(out, "\n\n[more file content; continue with offset_bytes=")
	if !more || len(body) > maxReadFileBytes || !utf8.ValidString(body) {
		t.Fatal("line read lost bounded UTF-8 pagination")
	}
	var next int64
	if _, err := fmt.Sscanf(marker, "%d]", &next); err != nil || next != int64(len(prefix)+len(body)) {
		t.Fatalf("line continuation is not an absolute byte offset: %q", marker)
	}
	rest, err := executor.ExecuteTool(context.Background(), filePageRequest(root, "openapi.yaml", next, nil))
	if err != nil || body+rest != tail {
		t.Fatal("search line and byte continuation did not round-trip source")
	}
	var offered bool
	for _, tool := range WorkspaceTools(Sandbox{Mode: SandboxReadOnly}) {
		if tool.Name == "read_file" {
			offered = strings.Contains(string(tool.InputSchema), `"start_line"`) && strings.Contains(tool.Description, "start_line")
		}
	}
	if !offered {
		t.Fatal("line coordinate is not advertised to the native provider")
	}
}

func TestReadFileLineRefusalsAndBounds(t *testing.T) {
	root, outside := workspace(t)
	if err := os.WriteFile(filepath.Join(root, "source"), []byte("one\r\nå\nlast"), 0600); err != nil {
		t.Fatal(err)
	}
	searchFile(t, root, ".openexec/config.json", "private provider configuration\n")
	executor := NewWorkspaceToolExecutor()
	for _, input := range []map[string]any{
		{"path": "source", "start_line": 0},
		{"path": "source", "start_line": -1},
		{"path": "source", "start_line": 2, "offset_bytes": 0},
		{"path": "source", "start_line": 2, "limit_bytes": 8193},
		{"path": "source", "start_line": 2, "limit_bytes": 1},
		{"path": filepath.Join(outside, "token"), "start_line": 1},
		{"path": ".openexec/config.json", "start_line": 1},
	} {
		if _, err := executor.ExecuteTool(context.Background(), lineReadRequest(root, input)); err == nil {
			t.Fatalf("invalid line read accepted: %v", input)
		}
	}
	for _, tc := range []struct {
		line int64
		want string
	}{{1, "one\r\nå\nlast"}, {2, "å\nlast"}, {3, "last"}, {4, ""}, {1000000000, ""}} {
		out, err := executor.ExecuteTool(context.Background(), lineReadRequest(root, map[string]any{"path": "source", "start_line": tc.line}))
		if err != nil || out != tc.want {
			t.Fatalf("line %d = %q, %v; want %q", tc.line, out, err, tc.want)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "long"), []byte(strings.Repeat("x", maxSearchBytes+1)+"\nsecret-tail"), 0600); err != nil {
		t.Fatal(err)
	}
	request := lineReadRequest(root, map[string]any{"path": "long", "start_line": 2})
	if _, err := executor.ExecuteTool(context.Background(), request); err == nil || !strings.Contains(err.Error(), "scan bound") {
		t.Fatalf("line scan exceeded bound or falsely reported EOF: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := executor.ExecuteTool(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled line scan continued: %v", err)
	}
}

func TestReadFileLineNativeTransport(t *testing.T) {
	root := t.TempDir()
	searchFile(t, root, "openapi.yaml", strings.Repeat("# padding\n", 6590)+"    Metrics:\n      useful: true\n")
	server, _ := frontTruncatingServer(t, 200000, func(n int) string {
		switch n {
		case 1:
			return strings.Replace(readFileCall("find", `{"path":"openapi.yaml","text":"    Metrics:"}`), `"read_file"`, `"search_files"`, 1)
		case 2:
			return readFileCall("read", `{"path":"openapi.yaml","start_line":6591}`)
		}
		return textReply("CONTRACT: definition reached")
	})
	var completed []string
	result, err := workspaceProvider(t, server.URL, 4).Execute(context.Background(), Request{
		ID: "line-read", WorkingDir: root, Prompt: "Read the Metrics definition", Model: "test-model", Sandbox: Sandbox{Mode: SandboxReadOnly},
	}, func(e Event) error {
		if e.Type == EventToolCompleted {
			completed = append(completed, e.Text)
		}
		return nil
	})
	if err != nil || result.Outcome != OutcomeSucceeded || result.FinalText != "CONTRACT: definition reached" || len(completed) != 2 {
		t.Fatalf("native search/read journey failed: %+v, %v, %q", result, err, completed)
	}
	if completed[0] != "openapi.yaml:6591:    Metrics:\n" || completed[1] != "    Metrics:\n      useful: true\n" {
		t.Fatalf("line coordinate lost at native transport: %q", completed)
	}
}
