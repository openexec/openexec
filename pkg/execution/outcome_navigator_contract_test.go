//go:build outcome_navigator_contract

package execution

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

func TestOutcomeNavigatorContractTerminalProtocol(t *testing.T) {
	toolCall := &agent.Response{Content: []agent.ContentBlock{{
		Type: agent.ContentTypeToolUse, ToolUseID: "call", ToolName: "read_file",
		ToolInput: json.RawMessage(`{"path":"evidence.txt"}`),
	}}, StopReason: agent.StopReasonToolUse}
	tests := []struct {
		name          string
		finalResponse *agent.Response
		finalError    error
		textContains  string
	}{
		{name: "successful synthesis", finalResponse: &agent.Response{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "bounded evidence"}}}, textContains: "bounded evidence"},
		{name: "empty synthesis", finalResponse: &agent.Response{}, textContains: "returned no text"},
		{name: "failed synthesis", finalError: errors.New("provider unavailable"), textContains: "final synthesis failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			adapter := &fakeAPIAdapter{responses: []*agent.Response{toolCall}}
			adapter.completeErrors = []error{nil, test.finalError}
			if test.finalResponse != nil {
				adapter.responses = append(adapter.responses, test.finalResponse)
			}
			provider, err := NewAPIProvider(APIProviderConfig{
				Adapter: adapter, ToolExecutor: &recordingToolExecutor{}, MaxSteps: 1,
				Tools: []agent.ToolDefinition{{Name: "read_file", InputSchema: json.RawMessage(`{"type":"object"}`)}},
			})
			if err != nil {
				t.Fatal(err)
			}
			var events []Event
			result, err := provider.Execute(context.Background(), Request{
				ID: "bounded", WorkingDir: t.TempDir(), Prompt: "inspect", Model: "test-model",
				Sandbox: Sandbox{Mode: SandboxReadOnly},
			}, func(event Event) error {
				events = append(events, event)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != OutcomeInconclusive || result.Reason != ReasonMaxTurns ||
				!strings.Contains(result.FinalText, test.textContains) {
				t.Fatalf("result = %+v", result)
			}
			terminals := 0
			for _, event := range events {
				if event.Type == EventCompleted || event.Type == EventFailed || event.Type == EventCancelled || event.Type == EventInconclusive {
					terminals++
					if event.Type != EventInconclusive || event.Reason != ReasonMaxTurns {
						t.Fatalf("terminal = %+v", event)
					}
				}
			}
			if terminals != 1 {
				t.Fatalf("terminal count = %d, events=%+v", terminals, events)
			}
		})
	}
}
