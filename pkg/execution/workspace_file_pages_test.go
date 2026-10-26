package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func filePageRequest(root, path string, offset int64, limit *int) ToolRequest {
	input := map[string]any{"path": path, "offset_bytes": offset}
	if limit != nil {
		input["limit_bytes"] = *limit
	}
	raw, _ := json.Marshal(input)
	return ToolRequest{Name: "read_file", Input: raw, WorkingDir: root, Sandbox: Sandbox{Mode: SandboxReadOnly}}
}

func TestReadFilePagesBoundHistoryAndRoundTripSource(t *testing.T) {
	root, _ := workspace(t)
	// A multibyte character crosses the first page boundary. Important source
	// at the end must remain reachable, rather than being permanently cut off.
	source := strings.Repeat("x", 8191) + strings.Repeat("å", 9000) + "\nIMPORTANT-TAIL\n"
	if err := os.WriteFile(filepath.Join(root, "large.go"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	executor := NewWorkspaceToolExecutor()
	var assembled strings.Builder
	var offset int64
	pages := 0
	for {
		out, err := executor.ExecuteTool(context.Background(), filePageRequest(root, "large.go", offset, nil))
		if err != nil {
			t.Fatal(err)
		}
		body, marker, more := strings.Cut(out, "\n\n[more file content; continue with offset_bytes=")
		if len(body) > 8192 {
			t.Fatalf("file read flooded retained history: %d bytes", len(body))
		}
		if !utf8.ValidString(body) {
			t.Fatal("page split a UTF-8 character")
		}
		assembled.WriteString(body)
		pages++
		if !more {
			break
		}
		var next int64
		if _, err := fmt.Sscanf(marker, "%d]", &next); err != nil || next != offset+int64(len(body)) || next <= offset {
			t.Fatalf("incorrect continuation at %d: %q", offset, marker)
		}
		offset = next
		if pages > 10 {
			t.Fatal("continuation did not advance")
		}
	}
	if pages < 2 || assembled.String() != source {
		t.Fatal("paginated source did not round-trip through the original reader")
	}
}

func TestReadFilePagesLimitsOffsetsAndContainment(t *testing.T) {
	root, outside := workspace(t)
	executor := NewWorkspaceToolExecutor()
	if err := os.WriteFile(filepath.Join(root, "exact"), []byte("12345678"), 0600); err != nil {
		t.Fatal(err)
	}
	limit := 4
	for _, tc := range []struct {
		offset int64
		want   string
	}{
		{0, "1234\n\n[more file content; continue with offset_bytes=4]"},
		{4, "5678"}, {8, ""}, {1000000, ""},
	} {
		out, err := executor.ExecuteTool(context.Background(), filePageRequest(root, "exact", tc.offset, &limit))
		if err != nil || out != tc.want {
			t.Fatalf("offset %d: %q, %v; want %q", tc.offset, out, err, tc.want)
		}
	}
	for _, n := range []int{8193, 131072, 0, -1} {
		if _, err := executor.ExecuteTool(context.Background(), filePageRequest(root, "exact", 0, &n)); err == nil {
			t.Fatalf("invalid limit %d bypassed file page bound", n)
		}
	}
	if _, err := executor.ExecuteTool(context.Background(), filePageRequest(root, "exact", -1, nil)); err == nil {
		t.Fatal("negative offset was accepted")
	}
	if err := os.WriteFile(filepath.Join(root, "unicode"), []byte("åå"), 0600); err != nil {
		t.Fatal(err)
	}
	one := 1
	if _, err := executor.ExecuteTool(context.Background(), filePageRequest(root, "unicode", 0, &one)); err == nil {
		t.Fatal("a page too small for one character did not refuse")
	}
	if _, err := executor.ExecuteTool(context.Background(), filePageRequest(root, filepath.Join(outside, "token"), 1, &limit)); err == nil {
		t.Fatal("paginated read escaped authorized root")
	}
}
