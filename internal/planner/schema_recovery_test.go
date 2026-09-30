package planner

import (
	"context"
	"encoding/json"
	"errors"
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
