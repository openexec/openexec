package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type synthesisOverflow struct {
	fakeAPIAdapter
	calls      int
	dispatched bool
}

func (a *synthesisOverflow) SupportsHardTokenBudget() bool { return true }
func (a *synthesisOverflow) CompleteBounded(ctx context.Context, r agent.Request, i, o int) (*agent.Response, error) {
	a.calls++
	if a.calls == 1 {
		return &agent.Response{Usage: agent.Usage{PromptTokens: 1000, CompletionTokens: 100}, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: "read-1", ToolName: "read", ToolInput: []byte(`{}`)}}}, nil
	}
	return nil, &agent.ContextOverflowError{EstimatedTokens: 40000, ContextTokens: i, RequestBytes: 100000, Dispatched: a.dispatched}
}
func TestFinalSynthesisPreservesContextBoundaryAndUncertainUsage(t *testing.T) {
	for _, dispatched := range []bool{false, true} {
		t.Run(map[bool]string{false: "preflight", true: "native-refusal"}[dispatched], func(t *testing.T) {
			a := &synthesisOverflow{dispatched: dispatched}
			p, err := NewAPIProvider(APIProviderConfig{Adapter: a, MaxSteps: 1, Tools: []agent.ToolDefinition{{Name: "read"}}, ToolExecutor: &recordingToolExecutor{}})
			if err != nil {
				t.Fatal(err)
			}
			var final Event
			result, err := p.Execute(context.Background(), Request{ID: "original", WorkingDir: t.TempDir(), Prompt: "Continue", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: 65536}, func(e Event) error {
				if e.Type == EventUsage {
					final = e
				}
				return nil
			})
			var overflow *agent.ContextOverflowError
			if !errors.As(err, &overflow) || overflow.Dispatched != dispatched || result.Outcome != OutcomeFailed || result.Reason == ReasonMaxTurns {
				t.Fatalf("context boundary lost: calls%d err%v result%+v", a.calls, err, result)
			}
			if a.calls != 2 || final.UsageFinal == dispatched || final.InputTokens+final.OutputTokens != 1100 {
				t.Fatalf("incorrect accounting or retry: %+v calls%d", final, a.calls)
			}
		})
	}
}
