package execution

import (
	"context"
	"errors"
	"github.com/openexec/openexec/pkg/agent"
	"testing"
)

type boundedFake struct {
	fakeAPIAdapter
	calls  int
	limits [][2]int
	fail   bool
}

func (a *boundedFake) SupportsHardTokenBudget() bool { return true }
func (a *boundedFake) CompleteBounded(ctx context.Context, r agent.Request, i, o int) (*agent.Response, error) {
	a.calls++
	a.limits = append(a.limits, [2]int{i, o})
	if a.fail {
		return nil, errors.New("response lost")
	}
	response, err := a.fakeAPIAdapter.Complete(ctx, r)
	if response != nil {
		response.Usage = agent.Usage{PromptTokens: i, CompletionTokens: o, TotalTokens: i + o}
	}
	return response, err
}
func TestHardBudgetCoversEmptyRetriesBeforeInference(t *testing.T) {
	a := &boundedFake{fakeAPIAdapter: fakeAPIAdapter{responses: []*agent.Response{{}, {Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "must not be reached"}}}}}}
	p, _ := NewAPIProvider(APIProviderConfig{Adapter: a, Tools: []agent.ToolDefinition{{Name: "read"}}, ToolExecutor: &recordingToolExecutor{}})
	var terminal Event
	_, err := p.Execute(context.Background(), Request{ID: "bounded", WorkingDir: t.TempDir(), Prompt: "work", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: 4096}, func(e Event) error {
		if e.UsageFinal {
			terminal = e
		}
		return nil
	})
	if err == nil || a.calls != 1 {
		t.Fatalf("retry exceeded grant: calls%d err%v", a.calls, err)
	}
	if a.limits[0][0]+a.limits[0][1] > 4096 || terminal.InputTokens+terminal.OutputTokens != 4096 {
		t.Fatalf("bad reservation/final %+v %+v", a.limits, terminal)
	}
}
func TestHardBudgetRetainsUnknownUsageAndRejectsUnsupported(t *testing.T) {
	a := &boundedFake{fail: true}
	meter := &boundedAPIAdapter{ProviderAdapter: a, remaining: 4096}
	if _, err := meter.Complete(context.Background(), agent.Request{}); err == nil {
		t.Fatal("lost response treated as success")
	}
	if _, err := meter.Complete(context.Background(), agent.Request{}); err == nil || a.calls != 1 {
		t.Fatal("unknown usage returned capacity")
	}
	if !meter.unknown || meter.input != 0 || meter.output != 0 {
		t.Fatal("unknown reservation reported as observed token usage")
	}
	plain := &fakeAPIAdapter{}
	p, _ := NewAPIProvider(APIProviderConfig{Adapter: plain})
	if _, err := p.Execute(context.Background(), Request{TokenBudget: 4096}, nil); err == nil || len(plain.requests) != 0 {
		t.Fatal("unsupported provider executed")
	}
}

func TestCLITokenGrantRefusedBeforeExecutableResolution(t *testing.T) {
	p, err := NewAgentCLIProvider(AgentCLIConfig{Kind: "codex", Binary: "/missing-cli"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Execute(context.Background(), Request{TokenBudget: 100}, nil)
	if err == nil || err.Error() != "CLI cannot enforce a hard total-token grant" {
		t.Fatalf("unsupported grant reached CLI path: %v", err)
	}
}

func TestHardBudgetLowRemainingRefusesBeforeInference(t *testing.T) {
	for _, remaining := range []int64{1, 2048, 2730, 4095} {
		a := &boundedFake{}
		meter := &boundedAPIAdapter{ProviderAdapter: a, remaining: remaining}
		if _, err := meter.Complete(context.Background(), agent.Request{}); err == nil || a.calls != 0 {
			t.Fatalf("remaining %d admitted inference below native context floor", remaining)
		}
	}
}

// historicalContextAdapter reproduces the provider's reported usage from the
// 2026-09-08 failure. The third prepared prompt was 23,462 tokens, while the
// shrinking limiter requested only 16,026 context tokens and received HTTP 400.
type historicalContextAdapter struct {
	fakeAPIAdapter
	calls  int
	limits [][2]int
}

func (*historicalContextAdapter) SupportsHardTokenBudget() bool { return true }
func (a *historicalContextAdapter) CompleteBounded(_ context.Context, _ agent.Request, input, output int) (*agent.Response, error) {
	a.calls++
	a.limits = append(a.limits, [2]int{input, output})
	switch a.calls {
	case 1:
		return &agent.Response{Usage: agent.Usage{PromptTokens: 20700, CompletionTokens: 1298}, Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: "read-1", ToolName: "read", ToolInput: []byte(`{}`)}}}, nil
	case 2:
		return &agent.Response{Usage: agent.Usage{PromptTokens: 23416, CompletionTokens: 2048}, Metadata: map[string]any{"thinking": "fixture"}}, nil
	default:
		if input < 23462 {
			return nil, errors.New("bounded local inference HTTP 400: fixture prompt 23462 exceeds context")
		}
		return nil, errors.New("third inference must not be dispatched")
	}
}

func TestHardBudgetHistoricalContextExhaustionYieldsBeforeHTTP(t *testing.T) {
	a := &historicalContextAdapter{}
	p, err := NewAPIProvider(APIProviderConfig{Adapter: a, Tools: []agent.ToolDefinition{{Name: "read"}}, ToolExecutor: &recordingToolExecutor{}})
	if err != nil {
		t.Fatal(err)
	}
	var final Event
	_, err = p.Execute(context.Background(), Request{ID: "historical", WorkingDir: t.TempDir(), Prompt: "Continue retained work", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: 65536}, func(e Event) error {
		if e.Type == EventUsage && e.UsageFinal {
			final = e
		}
		return nil
	})
	if err != errHardTokenGrantExhausted || err.Error() != "hard token grant cannot admit another inference" {
		t.Fatalf("wrapped recovery failed to preserve typed grant yield: %v", err)
	}
	if a.calls != 2 || len(a.limits) != 2 || a.limits[0] != [2]int{32768, 2048} || a.limits[1] != a.limits[0] {
		t.Fatalf("inference context shrank or a third HTTP request escaped: %+v", a.limits)
	}
	if !final.UsageFinal || final.InputTokens != 44116 || final.OutputTokens != 3346 {
		t.Fatalf("known consumption lost: %+v", final)
	}
}

func TestHardBudgetKeepsAdmittedContextForSmallUnevenGrant(t *testing.T) {
	a := &boundedFake{fakeAPIAdapter: fakeAPIAdapter{responses: []*agent.Response{{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "ok"}}}}}}
	meter := &boundedAPIAdapter{ProviderAdapter: a, remaining: 7001}
	if _, err := meter.Complete(context.Background(), agent.Request{}); err != nil {
		t.Fatal(err)
	}
	if a.limits[0] != [2]int{4096, 2048} || meter.remaining != 857 {
		t.Fatalf("unsafe native context or accounting: limits%v remaining%d", a.limits, meter.remaining)
	}
	if _, err := meter.Complete(context.Background(), agent.Request{}); err != errHardTokenGrantExhausted || a.calls != 1 || meter.unknown {
		t.Fatal("small known remainder did not yield without inference", err)
	}
}

func TestHardBudgetFinalSynthesisPreservesGrantYield(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		t.Run(map[bool]string{false: "initial-synthesis", true: "synthesis-recovery"}[wrapped], func(t *testing.T) {
			first := &boundedFake{fakeAPIAdapter: fakeAPIAdapter{responses: []*agent.Response{{Content: []agent.ContentBlock{{Type: agent.ContentTypeToolUse, ToolUseID: "read-1", ToolName: "read", ToolInput: []byte(`{}`)}}}}}}
			historical := &historicalContextAdapter{}
			var adapter agent.ProviderAdapter = first
			grant := int64(4096)
			if wrapped {
				adapter = historical
				grant = 65536
			}
			p, err := NewAPIProvider(APIProviderConfig{Adapter: adapter, MaxSteps: 1, Tools: []agent.ToolDefinition{{Name: "read"}}, ToolExecutor: &recordingToolExecutor{}})
			if err != nil {
				t.Fatal(err)
			}
			var receipt Event
			result, err := p.Execute(context.Background(), Request{ID: "synthesis", WorkingDir: t.TempDir(), Prompt: "Continue", Model: "test-model", Sandbox: Sandbox{Mode: "read-only"}, TokenBudget: grant}, func(e Event) error {
				if e.Type == EventUsage && e.UsageFinal {
					receipt = e
				}
				return nil
			})
			if err != errHardTokenGrantExhausted || result.Outcome != OutcomeFailed || result.Reason == ReasonMaxTurns {
				t.Fatalf("capacity yield swallowed as round limit: result%+v err%v", result, err)
			}
			used := int64(4096)
			if wrapped {
				used = 47462
				if historical.calls != 2 {
					t.Fatalf("extra synthesis request: %d", historical.calls)
				}
			} else if first.calls != 1 {
				t.Fatalf("extra synthesis request: %d", first.calls)
			}
			if !receipt.UsageFinal || receipt.InputTokens+receipt.OutputTokens != used {
				t.Fatalf("lost finalized receipt: %+v", receipt)
			}
		})
	}
}
