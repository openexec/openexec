package execution

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

type synthesisOverflow struct {
	fakeAPIAdapter
	calls      int
	dispatched bool
}

type retainedSynthesisAdapter struct {
	fakeAPIAdapter
	seen   agent.Request
	refuse bool
}

func (*retainedSynthesisAdapter) SupportsHardTokenBudget() bool { return true }
func (a *retainedSynthesisAdapter) CompleteBounded(_ context.Context, r agent.Request, input, output int) (*agent.Response, error) {
	a.seen = r
	if a.refuse {
		return nil, &agent.ContextOverflowError{EstimatedTokens: input + 1, ContextTokens: input}
	}
	return &agent.Response{Usage: agent.Usage{PromptTokens: 100, CompletionTokens: 20}}, nil
}

func TestFinalSynthesisRetainsOriginalEvidenceAfterContextRecovery(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(fmt.Sprint(refuse), func(t *testing.T) {
			a := &retainedSynthesisAdapter{refuse: refuse}
			meter := &boundedAPIAdapter{ProviderAdapter: a, remaining: 250000, prior: priorResults{"old": "saved"}}
			r := agent.Request{System: "original authority", ToolChoice: "none", Tools: []agent.ToolDefinition{{Name: "list_directory"}}}
			for i := 0; i < 3; i++ {
				id := fmt.Sprintf("id-%d", i)
				r.Messages = append(r.Messages, agent.Message{Role: agent.RoleAssistant, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: id, ToolName: "list_directory"}}}, agent.NewToolResultMessage(id, fmt.Sprintf("evidence-%d ", i)+strings.Repeat("a", 2048), nil))
			}
			_, err := meter.Complete(context.Background(), r)
			if refuse {
				var overflow *agent.ContextOverflowError
				if !errors.As(err, &overflow) || overflow.Dispatched {
					t.Fatalf("expected pre-dispatch refusal: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if !strings.Contains(a.seen.Messages[2*i+1].Content[0].ToolOutput, fmt.Sprintf("evidence-%d", i)) {
					t.Fatal("final synthesis lost original evidence")
				}
			}
			if len(a.seen.Tools) != 2 || a.seen.Tools[1].Name != recallTool {
				t.Fatal("prior recall tool declaration missing from final history")
			}
		})
	}
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
