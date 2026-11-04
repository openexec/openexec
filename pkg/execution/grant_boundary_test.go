package execution

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/agent"
)

// boundaryStub mirrors the real adapter's preflight rule
// (estimate+output > input => non-dispatched overflow) with a fixed estimate,
// so the grant decision can be pinned without a real request.
type boundaryStub struct {
	agent.ProviderAdapter
	estimate   int
	dispatched int
	limits     [][2]int
}

func (*boundaryStub) SupportsHardTokenBudget() bool { return true }

func (s *boundaryStub) CompleteBounded(_ context.Context, _ agent.Request, input, output int) (*agent.Response, error) {
	s.limits = append(s.limits, [2]int{input, output})
	if s.estimate+output > input {
		return nil, &agent.ContextOverflowError{EstimatedTokens: s.estimate, ContextTokens: input, RequestBytes: 20000}
	}
	s.dispatched++
	return &agent.Response{Usage: agent.Usage{PromptTokens: s.estimate, CompletionTokens: 1}}, nil
}

func runGrantBoundary(estimate int) (*boundaryStub, *boundedAPIAdapter, error) {
	stub := &boundaryStub{estimate: estimate}
	meter := &boundedAPIAdapter{ProviderAdapter: stub, remaining: 9556, providerContext: 65536}
	_, err := meter.Complete(context.Background(), agent.Request{Model: "qwen-27b", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "x")}})
	return stub, meter, err
}

func TestGrantBoundary9556(t *testing.T) {
	const prefix = "hard token grant cannot admit inference: remaining grant=9556 selected context=4096 provider/request ceiling=65536"

	t.Run("5445 refused with no dispatch", func(t *testing.T) {
		stub, meter, err := runGrantBoundary(5445)
		var grant *contextGrantError
		if err == nil || !strings.HasPrefix(err.Error(), prefix) || !errors.Is(err, errHardTokenGrantExhausted) || !errors.As(err, &grant) {
			t.Fatalf("expected grant refusal, got %v", err)
		}
		if grant.requiredContext != 7509 || grant.requiredContext+grant.outputReserve != 9557 || grant.estimatedTokens != 5445 {
			t.Fatalf("arithmetic: %+v", grant)
		}
		if stub.dispatched != 0 || meter.remaining != 9556 || meter.input != 0 || meter.output != 0 {
			t.Fatalf("dispatched=%d remaining=%d", stub.dispatched, meter.remaining)
		}
		if len(stub.limits) != 1 || stub.limits[0] != [2]int{4096, 4096} {
			t.Fatalf("no resize attempt expected: %v", stub.limits)
		}
	})

	t.Run("5444 resizes within the grant", func(t *testing.T) {
		stub, meter, err := runGrantBoundary(5444)
		if err != nil {
			t.Fatalf("5444 must fit: %v", err)
		}
		if meter.contextCap != 7508 || meter.contextCap+2048 != 9556 {
			t.Fatalf("contextCap=%d", meter.contextCap)
		}
		last := stub.limits[len(stub.limits)-1]
		if last != [2]int{7508, 2048} || stub.dispatched != 1 {
			t.Fatalf("limits=%v dispatched=%d", stub.limits, stub.dispatched)
		}
		if meter.remaining < 0 || meter.remaining != 9556-5444-1 {
			t.Fatalf("remaining=%d", meter.remaining)
		}
	})

	// Initial fit, derived from the stub's admission rule: the first attempt
	// reserves output=4096 (always overflows), the output-room retry then
	// succeeds at 4096 only when room=4096-estimate >= 2048, i.e. estimate<=2048.
	// Estimates 2049..5444 do not fit 4096 and must resize.
	for _, tc := range []struct {
		estimate int
		resized  bool
	}{{100, false}, {2048, false}, {2049, true}, {4096, true}, {5444, true}} {
		stub, meter, err := runGrantBoundary(tc.estimate)
		if err != nil {
			t.Fatalf("estimate %d: %v", tc.estimate, err)
		}
		if resized := meter.contextCap != 4096; resized != tc.resized || stub.dispatched != 1 {
			t.Fatalf("estimate %d: contextCap=%d dispatched=%d", tc.estimate, meter.contextCap, stub.dispatched)
		}
		if !tc.resized {
			if last := stub.limits[len(stub.limits)-1]; last != [2]int{4096, 4096 - tc.estimate} {
				t.Fatalf("estimate %d: limits=%v", tc.estimate, stub.limits)
			}
		}
	}
}

// The two admission guards ahead of the context choice are not reached with a
// 9556 grant, so pin them at the grants where they decide.
func TestGrantBoundaryAdmissionGuards(t *testing.T) {
	complete := func(meter *boundedAPIAdapter) error {
		_, err := meter.Complete(context.Background(), agent.Request{Model: "qwen-27b", Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "x")}})
		return err
	}
	// remaining below 4096: refused before any context is selected.
	stub := &boundaryStub{estimate: 10}
	meter := &boundedAPIAdapter{ProviderAdapter: stub, remaining: 4095, providerContext: 65536}
	if err := complete(meter); err != errHardTokenGrantExhausted || len(stub.limits) != 0 || meter.remaining != 4095 {
		t.Fatalf("remaining 4095: err=%v limits=%v remaining=%d", err, stub.limits, meter.remaining)
	}
	// A stable 4096 context admitted earlier cannot be served by a grant
	// below input+2048, even though remaining is at least 4096.
	stub = &boundaryStub{estimate: 10}
	meter = &boundedAPIAdapter{ProviderAdapter: stub, remaining: 6143, providerContext: 65536, contextCap: 4096, contextTarget: 65536}
	if err := complete(meter); err != errHardTokenGrantExhausted || len(stub.limits) != 0 || meter.remaining != 6143 {
		t.Fatalf("remaining 6143: err=%v limits=%v remaining=%d", err, stub.limits, meter.remaining)
	}
	// Near miss: exactly input+2048 is admitted.
	stub = &boundaryStub{estimate: 10}
	meter = &boundedAPIAdapter{ProviderAdapter: stub, remaining: 6144, providerContext: 65536, contextCap: 4096, contextTarget: 65536}
	if err := complete(meter); err != nil || stub.dispatched != 1 {
		t.Fatalf("remaining 6144 must admit: err=%v dispatched=%d", err, stub.dispatched)
	}
}
