package loop

import (
    "bytes"
    "strings"
    "testing"
)

func TestParser_ExtractsPatchArtifact(t *testing.T) {
    ch := make(chan Event, 2) // Buffer must hold heartbeat + tool result
    p := NewParser(ch, 1)
    // Simulate tool_result content array with a text item containing artifact marker
    payload := []byte(`[{"type":"text","text":"Patch applied successfully\nARTIFACT:patch abc123 /tmp/x.patch"}]`)
    p.parseToolResult(payload)
    // Drain the heartbeat event emitted first
    <-ch
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


func TestParser_TracksPeakContextTokens(t *testing.T) {
	ch := make(chan Event, 10)
	p := NewParser(ch, 1)

	// Two assistant turns: the second has the larger context (peak, not sum).
	p.parseLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"turn 1"}],"usage":{"input_tokens":1000,"cache_creation_input_tokens":200,"cache_read_input_tokens":300,"output_tokens":50}}}`))
	p.parseLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"turn 2"}],"usage":{"input_tokens":2000,"cache_creation_input_tokens":0,"cache_read_input_tokens":1500,"output_tokens":80}}}`))
	p.parseLine([]byte(`{"type":"result","subtype":"success"}`))

	var peak string
	for {
		select {
		case evt := <-ch:
			if v, ok := evt.Artifacts["peak_context_tokens"]; ok {
				peak = v
			}
			continue
		default:
		}
		break
	}

	// Peak = max(1000+200+300, 2000+0+1500) = 3500, NOT the 5000 sum.
	if peak != "3500" {
		t.Fatalf("expected peak_context_tokens=3500, got %q", peak)
	}
}

func TestParser_NoUsage_NoPeakArtifact(t *testing.T) {
	ch := make(chan Event, 10)
	p := NewParser(ch, 1)

	p.parseLine([]byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"no usage here"}]}}`))
	p.parseLine([]byte(`{"type":"result","subtype":"success"}`))

	for {
		select {
		case evt := <-ch:
			if _, ok := evt.Artifacts["peak_context_tokens"]; ok {
				t.Fatal("expected no peak_context_tokens artifact when usage is absent")
			}
			continue
		default:
		}
		break
	}
}

func TestParser_ThrashDetection(t *testing.T) {
	ch := make(chan Event, 30)
	p := NewParser(ch, 1)

	failLine := `{"type":"tool_result","content":[{"type":"text","text":"validation error: invalid patch: old line count mismatch"}]}`
	toolUse := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"git_apply_patch","input":{}}]}}`

	// Three identical failures of the same tool → exactly one thrash event.
	for i := 0; i < 3; i++ {
		p.parseLine([]byte(toolUse))
		p.parseLine([]byte(failLine))
	}

	thrashCount := 0
	var thrashText string
	for {
		select {
		case evt := <-ch:
			if evt.Type == EventThrashingDetected {
				thrashCount++
				thrashText = evt.Text
			}
			continue
		default:
		}
		break
	}
	if thrashCount != 1 {
		t.Fatalf("expected exactly 1 thrash event after 3 identical failures, got %d", thrashCount)
	}
	if !strings.Contains(thrashText, "git_apply_patch") {
		t.Fatalf("thrash event should name the failing tool, got %q", thrashText)
	}
}

func TestParser_ThrashResetOnSuccess(t *testing.T) {
	ch := make(chan Event, 30)
	p := NewParser(ch, 1)

	failLine := `{"type":"tool_result","content":[{"type":"text","text":"validation error: bad patch"}]}`
	okLine := `{"type":"tool_result","content":[{"type":"text","text":"Patch applied successfully (1 file(s), +1/-0 lines)"}]}`
	toolUse := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"git_apply_patch","input":{}}]}}`

	// Two failures, a success, two more failures: never reaches threshold.
	for _, line := range []string{failLine, failLine, okLine, failLine, failLine} {
		p.parseLine([]byte(toolUse))
		p.parseLine([]byte(line))
	}

	for {
		select {
		case evt := <-ch:
			if evt.Type == EventThrashingDetected {
				t.Fatal("no thrash event expected when successes interleave failures")
			}
			continue
		default:
		}
		break
	}
}
