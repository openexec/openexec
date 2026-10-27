package execution

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// These tests pin the boundary behind the two "inconclusive:budget_exhausted"
// implementation runs of task e5de (see docs/QWEN_BUDGET_EXHAUSTION_DIAGNOSIS.md).
// They use authored responses only: they show what the tool loop does when its
// caller's deadline fires or when its own round limit is reached, not why the
// original model kept re-reading references.

var errAbsoluteRunBudget = errors.New("caller absolute run budget")

// deadlineAdapter answers the scripted responses, then behaves like a slow
// endpoint: the caller's timer fires mid-request and the call returns the
// context's error.
type deadlineAdapter struct {
	fakeAPIAdapter
	cancel context.CancelCauseFunc
}

func (a *deadlineAdapter) Complete(ctx context.Context, request agent.Request) (*agent.Response, error) {
	if len(a.responses) == 0 {
		a.requests = append(a.requests, request)
		a.cancel(errAbsoluteRunBudget)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return a.fakeAPIAdapter.Complete(ctx, request)
}

type cancellingToolExecutor struct {
	recordingToolExecutor
	cancel func()
}

func (e *cancellingToolExecutor) ExecuteTool(ctx context.Context, request ToolRequest) (string, error) {
	e.cancel()
	return e.recordingToolExecutor.ExecuteTool(ctx, request)
}

func readCalls(prefix string, n int) *agent.Response {
	response := &agent.Response{StopReason: agent.StopReasonToolUse}
	for i := 0; i < n; i++ {
		response.Content = append(response.Content, agent.ContentBlock{
			Type: agent.ContentTypeToolUse, ToolUseID: prefix + string(rune('a'+i)), ToolName: "read_file",
			ToolInput: json.RawMessage(`{"path":"evidence.txt"}`),
		})
	}
	return response
}

func runBoundary(t *testing.T, ctx context.Context, adapter agent.ProviderAdapter, tools ToolExecutor, maxSteps int) (Result, []Event, error) {
	t.Helper()
	provider, err := NewAPIProvider(APIProviderConfig{
		Adapter: adapter, ToolExecutor: tools, MaxSteps: maxSteps,
		Tools: []agent.ToolDefinition{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, execErr := provider.Execute(ctx, Request{
		ID: "boundary", WorkingDir: t.TempDir(), Prompt: "edit docs", Model: "test-model",
		Sandbox: Sandbox{Mode: "read-only"},
	}, func(event Event) error { events = append(events, event); return nil })
	return result, events, execErr
}

// A caller deadline that fires while a model request is outstanding ends the
// run as an error with no synthesis request, no inconclusive event and no
// reason. The "budget_exhausted" label is applied by the caller from the
// context cause, which the provider leaves intact.
func TestCallerDeadlineDuringModelRequestEndsWithoutSynthesis(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	adapter := &deadlineAdapter{cancel: cancel}
	adapter.responses = []*agent.Response{readCalls("r1", 3), readCalls("r2", 3)}
	tools := &recordingToolExecutor{}

	result, events, err := runBoundary(t, ctx, adapter, tools, 16)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
	if !errors.Is(context.Cause(ctx), errAbsoluteRunBudget) {
		t.Fatalf("cause lost: %v", context.Cause(ctx))
	}
	if result.Outcome != OutcomeFailed || result.Reason != "" || result.FinalText != "" {
		t.Fatalf("result = %+v", result)
	}
	if len(tools.requests) != 6 || len(adapter.requests) != 3 {
		t.Fatalf("tool calls = %d, model requests = %d", len(tools.requests), len(adapter.requests))
	}
	for _, request := range adapter.requests {
		if request.ToolChoice == "none" {
			t.Fatalf("a synthesis request was sent after the deadline: %+v", request)
		}
	}
	if last := events[len(events)-1]; last.Type != EventFailed {
		t.Fatalf("terminal event = %#v", last)
	}
	for _, event := range events {
		if event.Type == EventInconclusive || event.Type == EventCompleted || event.Reason == ReasonBudgetExhausted {
			t.Fatalf("provider produced a result of its own: %#v", event)
		}
	}
}

// A caller deadline that fires while a tool runs is seen at the next round
// boundary as a cancellation; still no synthesis is requested.
func TestCallerDeadlineDuringToolEndsCancelledWithoutSynthesis(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	adapter := &fakeAPIAdapter{responses: []*agent.Response{readCalls("r1", 1), readCalls("r2", 1)}}
	tools := &cancellingToolExecutor{cancel: func() { cancel(errAbsoluteRunBudget) }}

	result, events, err := runBoundary(t, ctx, adapter, tools, 16)

	if !errors.Is(err, context.Canceled) || result.Outcome != OutcomeCancelled || result.Reason != "" {
		t.Fatalf("result = %+v err = %v", result, err)
	}
	if len(adapter.requests) != 1 {
		t.Fatalf("model requests after the deadline = %d", len(adapter.requests))
	}
	if events[len(events)-1].Type != EventCancelled {
		t.Fatalf("terminal event = %#v", events[len(events)-1])
	}
}

// Near miss: the provider's own limit counts model rounds, not tool calls, and
// ends as max_turns (never budget_exhausted). 16 rounds of 7 calls is 112 tool
// calls, the scale of the observed 107 and 123, so those counts alone do not
// show that the round limit was or was not the ending.
func TestRoundLimitCountsRoundsNotToolCallsAndIsNotBudgetExhausted(t *testing.T) {
	responses := make([]*agent.Response, 0, 17)
	for i := 0; i < 16; i++ {
		responses = append(responses, readCalls(string(rune('A'+i)), 7))
	}
	responses = append(responses, &agent.Response{
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "summary"}},
		StopReason: agent.StopReasonEnd,
	})
	adapter := &fakeAPIAdapter{responses: responses}
	tools := &recordingToolExecutor{}

	result, _, err := runBoundary(t, context.Background(), adapter, tools, 16) // observed historical limit

	if err != nil {
		t.Fatal(err)
	}
	if len(tools.requests) != 112 {
		t.Fatalf("tool calls = %d", len(tools.requests))
	}
	if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("result = %+v", result)
	}
	if last := adapter.requests[len(adapter.requests)-1]; last.ToolChoice != "none" {
		t.Fatalf("synthesis was not requested at the round limit: %+v", last)
	}
}
