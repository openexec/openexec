package execution

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// These tests pin the composition behind sig-ad3b25c2a04f (see
// docs/QWEN_MAX_TURNS_AFTER_DRAFT_DIAGNOSIS.md): a complete plan is emitted and
// the run still ends inconclusive:max_turns. They use authored responses; they
// show what the tool loop does, not why the configured model needed 16 rounds.

const authoredDraft = "## Contract\n1. Do the thing.\n2. Prove it."

// roundsWithCalls returns tool-using responses carrying total calls (37 is the
// count observed for run b93662ba), spread as evenly as possible.
func roundsWithCalls(rounds, total int) []*agent.Response {
	out := make([]*agent.Response, 0, rounds)
	for i := 0; i < rounds; i++ {
		n := total / rounds
		if i < total%rounds {
			n++
		}
		out = append(out, readCalls(string(rune('A'+i)), n))
	}
	return out
}

func draftResponse() *agent.Response {
	return &agent.Response{
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: authoredDraft}},
		StopReason: agent.StopReasonEnd,
	}
}

func eventTypes(events []Event) []string {
	var types []string
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

// 16 tool rounds then a complete draft: the draft is only requested by the
// tool-less synthesis call, and the run ends inconclusive:max_turns although the
// draft is in FinalText and was streamed as an assistant delta. The inconclusive
// event carries no text.
func TestCompleteDraftAfterRoundLimitStillEndsMaxTurns(t *testing.T) {
	adapter := &fakeAPIAdapter{responses: append(roundsWithCalls(16, 37), draftResponse())}
	tools := &recordingToolExecutor{}

	result, events, err := runBoundary(t, context.Background(), adapter, tools, 0)

	if err != nil {
		t.Fatal(err)
	}
	if len(tools.requests) != 37 || len(adapter.requests) != 17 {
		t.Fatalf("tool calls = %d, model requests = %d", len(tools.requests), len(adapter.requests))
	}
	if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.FinalText, authoredDraft) {
		t.Fatalf("draft missing from FinalText: %q", result.FinalText)
	}
	if last := adapter.requests[16]; last.ToolChoice != "none" {
		t.Fatalf("draft was not the synthesis request: %+v", last)
	}
	last := events[len(events)-1]
	if last.Type != EventInconclusive || last.Reason != ReasonMaxTurns || last.Text != "" {
		t.Fatalf("terminal event = %+v", last)
	}
	// 37 calls give 74 tool events, then the draft delta, then the terminal.
	// Agent Console turns that terminal into turn.error then turn.inconclusive
	// and saves a triage contract only for a completed run.
	delta := events[len(events)-2]
	if delta.Type != EventAssistantDelta || delta.Text != authoredDraft {
		t.Fatalf("event before terminal = %+v (%v)", delta, eventTypes(events))
	}
	completed := 0
	for _, event := range events {
		if event.Type == EventToolCompleted {
			completed++
		}
		if event.Type == EventCompleted {
			t.Fatalf("a completed event was emitted: %v", eventTypes(events))
		}
	}
	if completed != 37 {
		t.Fatalf("tool.completed events = %d", completed)
	}
}

// Smallest changed ordering: the same 37 calls in 15 rounds, then the draft as a
// normal round with tools available, succeed. Nothing else differs.
func TestSameDraftWithinRoundLimitSucceeds(t *testing.T) {
	adapter := &fakeAPIAdapter{responses: append(roundsWithCalls(15, 37), draftResponse())}
	tools := &recordingToolExecutor{}

	result, events, err := runBoundary(t, context.Background(), adapter, tools, 0)

	if err != nil {
		t.Fatal(err)
	}
	if len(tools.requests) != 37 {
		t.Fatalf("tool calls = %d", len(tools.requests))
	}
	if result.Outcome != OutcomeSucceeded || result.Reason != "" {
		t.Fatalf("result = %+v", result)
	}
	if result.FinalText != authoredDraft {
		t.Fatalf("FinalText = %q", result.FinalText)
	}
	if last := events[len(events)-1]; last.Type != EventCompleted {
		t.Fatalf("terminal = %v", eventTypes(events))
	}
	if last := adapter.requests[len(adapter.requests)-1]; last.ToolChoice != "auto" {
		t.Fatalf("draft was requested with tools disabled: %+v", last)
	}
}

// Near miss: when synthesis yields no draft (serialized tool call twice), the
// run is also max_turns, but FinalText holds no draft and the terminal carries
// the failure text.
func TestFailedSynthesisIsMaxTurnsWithoutDraft(t *testing.T) {
	toolCall := &agent.Response{
		Content:    []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "<tool_call>{}</tool_call>"}},
		StopReason: agent.StopReasonEnd,
	}
	adapter := &fakeAPIAdapter{responses: append(roundsWithCalls(16, 37), toolCall, toolCall)}

	result, events, err := runBoundary(t, context.Background(), adapter, &recordingToolExecutor{}, 0)

	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.FinalText, authoredDraft) || events[len(events)-1].Text == "" {
		t.Fatalf("result = %+v last = %+v", result, events[len(events)-1])
	}
}
