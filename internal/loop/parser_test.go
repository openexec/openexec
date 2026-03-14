package loop

import (
    "bytes"
    "testing"
)

func TestParser_ExtractsPatchArtifact(t *testing.T) {
    ch := make(chan Event, 1)
    p := NewParser(ch, 1)
    // Simulate tool_result content array with a text item containing artifact marker
    payload := []byte(`[{"type":"text","text":"Patch applied successfully\nARTIFACT:patch abc123 /tmp/x.patch"}]`)
    p.parseToolResult(payload)
    select {
    case evt := <-ch:
        if evt.Type != EventToolResult {
            t.Fatalf("expected EventToolResult, got %v", evt.Type)
        }
        if evt.Artifacts["patch_hash"] != "abc123" {
            t.Fatalf("expected patch_hash=abc123, got %q", evt.Artifacts["patch_hash"])
        }
        if evt.Artifacts["patch_path"] != "/tmp/x.patch" {
            t.Fatalf("expected patch_path=/tmp/x.patch, got %q", evt.Artifacts["patch_path"])
        }
        if !bytes.Contains([]byte(evt.Text), []byte("Patch applied successfully")) {
            t.Fatalf("expected text to include success message, got %q", evt.Text)
        }
    default:
        t.Fatal("no event emitted")
    }
}

