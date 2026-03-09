package tools

import (
	"context"
	"strings"
	"testing"
)

func TestGeneralChatTool(t *testing.T) {
	tool := NewGeneralChatTool()
	ctx := context.Background()

	tests := []struct {
		name    string
		query   string
		contain string
	}{
		{
			name:    "help command",
			query:   "help",
			contain: "ask me to",
		},
		{
			name:    "list command",
			query:   "list",
			contain: "list files",
		},
		{
			name:    "wizard command",
			query:   "wizard",
			contain: "openexec wizard",
		},
		{
			name:    "init command",
			query:   "init",
			contain: "openexec init",
		},
		{
			name:    "default fallback",
			query:   "hello",
			contain: "received your query",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := map[string]interface{}{"query": tt.query}
			resp, err := tool.Execute(ctx, args)
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			str, ok := resp.(string)
			if !ok {
				t.Fatalf("expected string response, got %T", resp)
			}

			if !strings.Contains(strings.ToLower(str), strings.ToLower(tt.contain)) {
				t.Errorf("expected response to contain %q, got %q", tt.contain, str)
			}
		})
	}
}
