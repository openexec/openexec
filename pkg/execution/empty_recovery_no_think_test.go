package execution

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// Solar's T-US-CS29AB35C-001-013 on 11-12: qwen38 spent its whole output
// allowance on private reasoning, was asked again the same way, and spent it
// again. The retry after a reasoning-only completion turns Qwen's thinking off,
// as the final-synthesis recovery already does; any other empty completion, or
// another model, keeps the plain instruction.
func TestReasoningOnlyRecoveryTurnsQwenThinkingOff(t *testing.T) {
	reasoningOnly := &agent.Response{StopReason: agent.StopReasonMaxTokens, Metadata: map[string]interface{}{"thinking": "PRIVATE_REASONING"}}
	empty := &agent.Response{StopReason: agent.StopReasonEnd}
	answer := &agent.Response{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "visible answer"}}, StopReason: agent.StopReasonEnd}
	for _, tc := range []struct {
		name, model string
		first       *agent.Response
		noThink     bool
	}{
		{"qwen reasoning-only", "qwen38-27b-mtp2-128k", reasoningOnly, true},
		{"qwen plain empty", "qwen38-27b-mtp2-128k", empty, false},
		{"other model reasoning-only", "local", reasoningOnly, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &fakeAPIAdapter{responses: []*agent.Response{tc.first, answer}}
			provider := &APIProvider{config: APIProviderConfig{Adapter: adapter}}
			response, err := provider.completeWithEmptyRecovery(context.Background(), agent.Request{
				Model: tc.model, Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "review")},
			})
			if err != nil || response.GetText() != "visible answer" || len(adapter.requests) != 2 {
				t.Fatalf("recovery: response=%v err=%v requests=%d", response, err, len(adapter.requests))
			}
			retry := adapter.requests[1]
			last := retry.Messages[len(retry.Messages)-1]
			if last.Role != agent.RoleUser || !strings.HasSuffix(last.GetText(), emptyCompletionRecoveryInstruction) {
				t.Fatalf("recovery instruction missing: %q", last.GetText())
			}
			if got := strings.HasPrefix(last.GetText(), "/no_think\n"); got != tc.noThink {
				t.Fatalf("retry after %s on %s: /no_think = %v, want %v", tc.name, tc.model, got, tc.noThink)
			}
		})
	}
}
