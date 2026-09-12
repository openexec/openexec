package execution

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type nonThinkingFake struct {
	boundedFake
	unsupported bool
}

func (a *nonThinkingFake) SupportsNativeNonThinking() bool { return true }
func (a *nonThinkingFake) ValidateNativeNonThinking(context.Context, string) error {
	if a.unsupported {
		return fmt.Errorf("unsupported model")
	}
	return nil
}

func TestNativeNonThinkingRetryAndAccounting(t *testing.T) {
	for _, mode := range []string{"retry", "ignored", "unsupported"} {
		t.Run(mode, func(t *testing.T) {
			first := &agent.Response{}
			if mode == "ignored" {
				first.Metadata = map[string]any{"thinking": "private fixture"}
			}
			a := &nonThinkingFake{boundedFake: boundedFake{fakeAPIAdapter: fakeAPIAdapter{responses: []*agent.Response{first, {Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "done"}}}}}}, unsupported: mode == "unsupported"}
			p, _ := NewAPIProvider(APIProviderConfig{Adapter: a, Tools: []agent.ToolDefinition{{Name: "read"}}, ToolExecutor: &recordingToolExecutor{}})
			var final Event
			_, err := p.Execute(context.Background(), Request{ID: "bounded", Prompt: "one bounded observation", Model: "alias", WorkingDir: t.TempDir(), Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: 65536, ContextTokenLimit: 16384, NonThinking: true}, func(e Event) error {
				if e.Type == EventUsage {
					final = e
				}
				return nil
			})
			if mode == "unsupported" {
				if err == nil || a.calls != 0 {
					t.Fatal("unsupported inferred")
				}
				return
			}
			if mode == "ignored" {
				if err == nil || !strings.Contains(err.Error(), "ignored non-thinking") || a.calls != 1 || !final.UsageFinal || final.InputTokens+final.OutputTokens != 18432 {
					t.Fatal("ignored mode was accepted or usage lost", err, final)
				}
				return
			}
			if err != nil || a.calls != 2 || !final.UsageFinal || final.InputTokens+final.OutputTokens != 36864 {
				t.Fatal(err, a.calls, final)
			}
			for _, req := range a.requests {
				if !req.NonThinking {
					t.Fatal("retry lost native mode")
				}
			}
			for _, caps := range a.limits {
				if caps != [2]int{16384, 2048} {
					t.Fatal("mode changed caps")
				}
			}
		})
	}
}
