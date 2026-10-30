package execution

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// A native planner had an accepted Goal but returned only private
// reasoning at its 2K output boundary, twice. No plan reached task import.
// Exercise that same native wire boundary, including the empty-answer retry.
func TestBoundedNativePlanningReturnsTasksAfterReasoning(t *testing.T) {
	const plan = `{"tasks":[{"id":"architecture"},{"id":"lab-installation"},{"id":"takeover-plan"}]}`
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/version" {
			_, _ = w.Write([]byte(`{"version":"0.32.13"}`))
			return
		}
		calls++
		var wire struct {
			Truncate *bool          `json:"truncate"`
			Options  map[string]int `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
			return
		}
		if wire.Truncate == nil || *wire.Truncate || wire.Options["num_ctx"] != 65536 {
			t.Error("planning lost its stable, non-truncating context")
		}
		output := wire.Options["num_predict"]
		if output <= 2048 {
			_ = json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "length", "prompt_eval_count": 12000, "eval_count": output,
				"message": map[string]any{"thinking": "PRIVATE_REASONING", "content": ""}})
			return
		}
		if output > 65536 || output < 32000 {
			t.Errorf("generous grant did not leave bounded output room after framing: %d", output)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "stop", "prompt_eval_count": 12000, "eval_count": 3072,
			"message": map[string]any{"thinking": "PRIVATE_REASONING", "content": plan}})
	}))
	defer server.Close()
	provider, err := NewAPIProvider(APIProviderConfig{Adapter: newNativeProvider(t, server.URL)})
	if err != nil {
		t.Fatal(err)
	}
	var final Event
	result, err := provider.Execute(context.Background(), Request{ID: "native-plan", WorkingDir: t.TempDir(), Model: "local", Prompt: "Return the bounded task plan as JSON.", Sandbox: Sandbox{Mode: SandboxReadOnly}, TokenBudget: 250000}, func(e Event) error {
		if e.UsageFinal {
			final = e
		}
		return nil
	})
	if err != nil || result.Outcome != OutcomeSucceeded || result.FinalText != plan || calls != 1 {
		t.Fatalf("native task plan did not survive reasoning: outcome=%s calls=%d error=%v", result.Outcome, calls, err)
	}
	var parsed struct{ Tasks []struct{ ID string } }
	if err := json.Unmarshal([]byte(result.FinalText), &parsed); err != nil || len(parsed.Tasks) != 3 {
		t.Fatal("native task plan was incomplete", err)
	}
	if !final.UsageFinal || final.InputTokens != 12000 || final.OutputTokens != 3072 {
		t.Fatalf("lost cumulative input/reasoning/output usage: %+v", final)
	}
}

func TestOutputReservationStaysInsideExistingGrant(t *testing.T) {
	for _, tc := range []struct {
		name       string
		grant      int64
		requested  int
		input, out int
	}{
		{"minimum", 4096, 0, 2048, 2048},
		{"small", 7001, 0, 4096, 2905},
		{"shared", 65536, 0, 32768, 32768},
		{"remaining", 100000, 0, 65536, 34464},
		{"generous", 250000, 0, 65536, 65536},
		{"caller ceiling", 250000, 1024, 65536, 1024},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &boundedFake{fakeAPIAdapter: fakeAPIAdapter{responses: []*agent.Response{{Content: []agent.ContentBlock{{Type: agent.ContentTypeText, Text: "done"}}}}}}
			meter := &boundedAPIAdapter{ProviderAdapter: adapter, remaining: tc.grant}
			_, err := meter.Complete(context.Background(), agent.Request{MaxTokens: tc.requested})
			if err != nil || len(adapter.limits) != 1 || adapter.limits[0] != [2]int{tc.input, tc.out} {
				t.Fatalf("wrong bounded allocation: caps=%v error=%v", adapter.limits, err)
			}
			if meter.input+meter.output > tc.grant || meter.remaining != tc.grant-meter.input-meter.output {
				t.Fatalf("allocation increased or lost the accepted grant: %+v", meter)
			}
		})
	}
}

type outputOverflowAdapter struct {
	boundedFake
	dispatched bool
	room       int
}

func (a *outputOverflowAdapter) CompleteBounded(_ context.Context, _ agent.Request, input, _ int) (*agent.Response, error) {
	a.calls++
	return nil, &agent.ContextOverflowError{EstimatedTokens: input - a.room, ContextTokens: input, Dispatched: a.dispatched}
}

func TestOutputPreflightDoesNotRepeatDispatchedInference(t *testing.T) {
	adapter := &outputOverflowAdapter{dispatched: true, room: 32000}
	meter := &boundedAPIAdapter{ProviderAdapter: adapter, remaining: 65536}
	_, err := meter.Complete(context.Background(), agent.Request{})
	if err == nil || adapter.calls != 1 || !meter.unknown || meter.remaining != 0 {
		t.Fatalf("dispatched failure was retried or refunded: calls=%d unknown=%t remaining=%d error=%v", adapter.calls, meter.unknown, meter.remaining, err)
	}
}

func TestOutputPreflightPreservesMinimumReplyRoom(t *testing.T) {
	adapter := &outputOverflowAdapter{room: 1024}
	meter := &boundedAPIAdapter{ProviderAdapter: adapter, remaining: 65536}
	_, err := meter.Complete(context.Background(), agent.Request{})
	if err == nil || adapter.calls != 1 || meter.unknown || meter.remaining != 65536 {
		t.Fatalf("unique prompt admitted with insufficient reply room: calls=%d unknown=%t remaining=%d error=%v", adapter.calls, meter.unknown, meter.remaining, err)
	}
}

type outputResizeAdapter struct{ boundedFake }

func (a *outputResizeAdapter) CompleteBounded(_ context.Context, _ agent.Request, input, output int) (*agent.Response, error) {
	a.calls++
	if output > 8192 {
		return nil, &agent.ContextOverflowError{EstimatedTokens: input - 8192, ContextTokens: input}
	}
	return &agent.Response{Usage: agent.Usage{PromptTokens: 100, CompletionTokens: 50}}, nil
}

func TestOutputPreflightRefundsOnlyUnsubmittedCapacity(t *testing.T) {
	adapter := &outputResizeAdapter{}
	meter := &boundedAPIAdapter{ProviderAdapter: adapter, remaining: 250000}
	_, err := meter.Complete(context.Background(), agent.Request{})
	if err != nil || adapter.calls != 2 || meter.input != 100 || meter.output != 50 || meter.remaining != 249850 || meter.unknown {
		t.Fatalf("output preparation lost or enlarged the original grant: calls=%d input=%d output=%d remaining=%d error=%v", adapter.calls, meter.input, meter.output, meter.remaining, err)
	}
}
