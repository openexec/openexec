package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestSchemaDecodeDiagnostic(t *testing.T) {
	for _, response := range []string{`[{"id":"S","requirement_id":[]}]`, `{"stories":[{"id":"S","requirement_id":["R1","R2"]}]}`, `{"stories":[`, `[{"id":"valid","requirement_id":"R1"},{"id":"invalid","requirement_id":42}]`} {
		plan, err := New(nil).parseResponse(response)
		var decode *ResponseDecodeError
		if plan != nil || !errors.As(err, &decode) || decode.Response != response {
			t.Fatalf("lost decode evidence: %+v %v", plan, err)
		}
		if json.Valid([]byte(response)) {
			var mismatch *json.UnmarshalTypeError
			if !errors.As(err, &mismatch) || !strings.Contains(mismatch.Field, "requirement_id") {
				t.Fatalf("lost concrete field error: %v", err)
			}
		} else {
			var syntax *json.SyntaxError
			if !errors.As(err, &syntax) {
				t.Fatalf("lost concrete syntax error: %v", err)
			}
		}
	}
	for _, response := range []string{`[]`, `{"stories":[]}`} {
		_, err := New(nil).parseResponse(response)
		var decode *ResponseDecodeError
		if err == nil || errors.As(err, &decode) || !strings.Contains(err.Error(), "no stories found") {
			t.Fatalf("empty plan confused with malformed schema: %v", err)
		}
	}
}

func TestSchemaCorrectionAdmissionFailurePreservesDiagnostic(t *testing.T) {
	calls := 0
	response := "```json\n[{\"requirement_id\":[\"R1\",\"R2\"]}]\n```"
	p := New(planProviderFunc(func(context.Context, string) (string, error) {
		calls++
		return response, nil
	}))
	refused := errors.New("reservation could not be persisted")
	original := &ProjectPlan{Stories: []Story{{ID: "S", Title: "Original"}}}
	plan, err := p.RefinePlanWithSchemaCorrection(context.Background(), "intent", original, &PlanReview{}, func(decode *ResponseDecodeError) error {
		if decode.Response != response || !strings.Contains(decode.Diagnostic.Error(), "requirement_id of type string") {
			t.Fatalf("lost fenced response diagnostic: %v", decode)
		}
		return refused
	})
	if plan != nil || !errors.Is(err, refused) || calls != 1 || !strings.Contains(err.Error(), response) {
		t.Fatalf("correction dispatched without durable admission: %+v %v calls=%d", plan, err, calls)
	}
}

func TestSchemaCorrectionIsBoundedAndOptional(t *testing.T) {
	original := &ProjectPlan{Stories: []Story{{ID: "S", Title: "Original"}}}
	review := &PlanReview{Assessment: "Retained findings"}
	for _, response := range []string{`[{"id":"S","requirement_id":[]}]`, `[]`} {
		calls := 0
		p := New(planProviderFunc(func(context.Context, string) (string, error) { calls++; return response, nil }))
		plan, err := p.RefinePlan(context.Background(), "intent", original, review)
		want := 2
		if response == `[]` {
			want = 1
		}
		if err == nil || plan != nil || calls != want {
			t.Fatalf("unbounded/partial correction: %d %+v %v", calls, plan, err)
		}
	}
}

type planProviderFunc func(context.Context, string) (string, error)

func (f planProviderFunc) Complete(ctx context.Context, prompt string) (string, error) {
	return f(ctx, prompt)
}

func TestSchemaSupportedEnvelopes(t *testing.T) {
	for _, body := range []string{
		`[{"id":"S","requirement_id":"REQ-1"}]`,
		`{"schema_version":"1.0.0","stories":[{"id":"S","requirement_id":"REQ-1"}]}`,
	} {
		for _, wrap := range []string{"%s", "```json\n%s\n```", "Plan follows:\n%s\nEnd."} {
			response := fmt.Sprintf(wrap, body)
			plan, err := New(nil).parseResponse(response)
			if err != nil || len(plan.Stories) != 1 || plan.Stories[0].RequirementID != "REQ-1" {
				t.Fatalf("envelope %q: %+v %v", response, plan, err)
			}
		}
	}
	for _, response := range []string{`null`, `{}`, `{"stories":null}`, `[]`} {
		plan, err := New(nil).parseResponse(response)
		var decode *ResponseDecodeError
		if plan != nil || err == nil || errors.As(err, &decode) || !strings.Contains(err.Error(), "no stories found") {
			t.Fatalf("empty response %q: %+v %v", response, plan, err)
		}
	}
	for _, response := range []string{"not JSON", "[", "{", `{"stories":[{"id":"good"},{"requirement_id":[]}]}`} {
		plan, err := New(nil).parseResponse(response)
		var decode *ResponseDecodeError
		if plan != nil || !errors.As(err, &decode) || !strings.Contains(err.Error(), response) {
			t.Fatalf("partial or malformed response escaped: %+v %v", plan, err)
		}
	}
}

func TestSchemaScalarPromptRequirements(t *testing.T) {
	for name, prompt := range map[string]string{"generate": StoryGenerationPrompt, "compact": CompactStoryGenerationPrompt, "review": StoryReviewPrompt, "refine": StoryFixPrompt} {
		t.Run(name, func(t *testing.T) {
			for _, rule := range []string{"requirement_id MUST be a scalar JSON string, never an array", "do not drop mappings or concatenate IDs", "Split stories where necessary"} {
				if !strings.Contains(prompt, rule) {
					t.Fatalf("missing scalar requirement: %q", rule)
				}
			}
		})
	}
}

func TestSchemaCorrectionFailurePaths(t *testing.T) {
	original := &ProjectPlan{Stories: []Story{{ID: "S", Title: "Original"}}}
	failure := errors.New("provider failed")
	for _, stage := range []string{"nil-review", "approved-review", "invalid-plan", "invalid-evidence", "initial-provider", "correction-provider"} {
		t.Run(stage, func(t *testing.T) {
			plan := original
			review := &PlanReview{Assessment: "retained"}
			switch stage {
			case "nil-review":
				review = nil
			case "approved-review":
				review.Approved = true
			case "invalid-plan":
				plan = nil
			case "invalid-evidence":
				review.KeyIssues = json.RawMessage(`{`)
			}
			calls := 0
			p := New(planProviderFunc(func(context.Context, string) (string, error) {
				calls++
				if stage == "correction-provider" && calls == 1 {
					return `[{"requirement_id":[]}]`, nil
				}
				return "", failure
			}))
			got, err := p.RefinePlan(context.Background(), "intent", plan, review)
			wantCalls := 0
			if stage == "initial-provider" {
				wantCalls = 1
			}
			if stage == "correction-provider" {
				wantCalls = 2
			}
			if got != nil || err == nil || calls != wantCalls {
				t.Fatalf("%+v %v calls=%d", got, err, calls)
			}
			if wantCalls > 0 && !errors.Is(err, failure) {
				t.Fatalf("lost provider cause: %v", err)
			}
		})
	}
}
