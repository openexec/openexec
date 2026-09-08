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
