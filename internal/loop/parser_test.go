package loop

import (
	"bytes"
	"testing"
)

func TestParser_Parse(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedEvents int
		checkEvent     func(*testing.T, Event)
	}{
		{
			name: "assistant text event",
			input: "{\"type\": \"assistant\", \"message\": {\"content\": [{\"type\": \"text\", \"text\": \"hello\"}]}}\n",
			expectedEvents: 1,
			checkEvent: func(t *testing.T, e Event) {
				if e.Type != EventAssistantText {
					t.Errorf("expected EventAssistantText, got %v", e.Type)
				}
				if e.Text != "hello" {
					t.Errorf("expected 'hello', got %q", e.Text)
				}
			},
		},
		{
			name: "tool use event",
			input: "{\"type\": \"assistant\", \"message\": {\"content\": [{\"type\": \"tool_use\", \"name\": \"write_file\", \"input\": {\"path\": \"test.txt\"}}]}}\n",
			expectedEvents: 1,
			checkEvent: func(t *testing.T, e Event) {
				if e.Type != EventToolStart {
					t.Errorf("expected EventToolStart, got %v", e.Type)
				}
				if e.Tool != "write_file" {
					t.Errorf("expected 'write_file', got %q", e.Tool)
				}
			},
		},
		{
			name: "axon signal event",
			input: "{\"type\": \"assistant\", \"message\": {\"content\": [{\"type\": \"tool_use\", \"name\": \"axon_signal\", \"input\": {\"type\": \"complete\", \"reason\": \"done\"}}]}}\n",
			expectedEvents: 1,
			checkEvent: func(t *testing.T, e Event) {
				if e.Type != EventSignalReceived {
					t.Errorf("expected EventSignalReceived, got %v", e.Type)
				}
				if e.SignalType != "complete" {
					t.Errorf("expected 'complete', got %q", e.SignalType)
				}
			},
		},
		{
			name: "robust parsing of messy json line",
			input: "{\"type\": \"assistant\", \"message\": {\"content\": [{\"type\": \"text\", \"text\": \"messy\"}]}, \"extra\": \"garbage\",}\n",
			expectedEvents: 1,
			checkEvent: func(t *testing.T, e Event) {
				if e.Text != "messy" {
					t.Errorf("expected 'messy', got %q", e.Text)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := make(chan Event, 10)
			p := NewParser(events, 1)
			
			err := p.Parse(bytes.NewBufferString(tt.input))
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			
			close(events)
			
			count := 0
			for e := range events {
				count++
				if tt.checkEvent != nil {
					tt.checkEvent(t, e)
				}
			}
			
			if count != tt.expectedEvents {
				t.Errorf("expected %d events, got %d", tt.expectedEvents, count)
			}
		})
	}
}
