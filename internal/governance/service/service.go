// Package service is the integration keystone for OpenExec release governance.
//
// It is the SINGLE place that sequences validation + policy + store writes +
// decision-event recording for each workflow operation. The CLI and MCP layers
// are intended to be thin adapters over this package: they parse input, call one
// Service method, and render the result. Both surfaces therefore traverse the
// IDENTICAL path — there is no second copy of the gate logic.
//
// The Service composes the already-built governance subsystems and never
// reimplements them:
//   - internal/governance            (Store: persistence + claim primitives)
//   - internal/governance/validation (status machine + structural rules)
//   - internal/governance/policy     (risk-tier approval authority)
//   - internal/governance/comms      (deterministic communication generators)
//   - internal/governance/ai         (planner/reviewer actors, injected Completer)
//   - internal/governance/connectors/github (issue import, injected Runner)
//
// Dependencies are injected so the package is testable and so the CLI/MCP layers
// can wire real implementations. The Completer and Runner are optional: an
// operation that needs one returns a clear, actionable error when it is absent,
// so a chat-only (no-AI, no-gh) context degrades gracefully instead of panicking.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/openexec/openexec/internal/governance"
	"github.com/openexec/openexec/internal/governance/ai"
	"github.com/openexec/openexec/internal/governance/connectors/github"
	"github.com/openexec/openexec/internal/governance/policy"
	"github.com/openexec/openexec/internal/release"
)

// Service sequences every governance workflow operation. It holds injected
// dependencies and no mutable state of its own, so it is safe to share.
type Service struct {
	store           governance.Store
	evaluator       *policy.PolicyEvaluator
	completer       ai.Completer  // optional; nil in a chat-only context
	runner          github.Runner // optional; nil when gh is unavailable
	planStore       PlanStore     // optional; nil disables deep triage
	operatorSession bool          // true only in a human operator session
}

// PlanStore is the subset of the release manager the deep-triage bridge needs to
// persist planner-generated goals/stories/tasks. *release.Manager satisfies it.
// Injecting a narrow interface keeps the service testable and avoids the release
// layer depending on governance.
type PlanStore interface {
	GetGoal(id string) *release.Goal
	GetStory(id string) *release.Story
	GetTask(id string) *release.Task
	CreateGoal(*release.Goal) error
	CreateStory(*release.Story) error
	CreateTask(*release.Task) error
}

// Options carries the optional dependencies for NewService. A nil Completer or
// Runner is allowed; the operations that require them fail with a clear error. A
// nil Policy falls back to policy.DefaultPolicy().
type Options struct {
	Completer ai.Completer
	Runner    github.Runner
	Policy    *policy.Policy
	// PlanStore persists planner-generated stories/tasks for deep triage. Nil
	// disables TriageDeep (the operation returns a clear error).
	PlanStore PlanStore
	// OperatorSession marks this Service as running under a human operator
	// (e.g. OPENEXEC_OPERATOR_SESSION=1). Approval operations (ApproveChange,
	// ApproveRelease) refuse when it is false, so an unattended agent session —
	// which never sets this — cannot self-approve, independent of which surface
	// (CLI/MCP) it uses. Mirrors the infra approval plane's operator gate.
	OperatorSession bool
}

// NewService wires a Service over a store and the given options. The store is
// required. If opts.Policy is nil the conservative DefaultPolicy is used.
func NewService(store governance.Store, opts Options) *Service {
	p := opts.Policy
	if p == nil {
		p = policy.DefaultPolicy()
	}
	return &Service{
		store:           store,
		evaluator:       policy.NewEvaluator(p),
		completer:       opts.Completer,
		runner:          opts.Runner,
		planStore:       opts.PlanStore,
		operatorSession: opts.OperatorSession,
	}
}

// errNotOperator is returned by approval operations invoked outside a human
// operator session.
var errNotOperator = errors.New("governance/service: approval requires a human operator session (set OPENEXEC_OPERATOR_SESSION=1); an agent session cannot approve its own work")

// errMissingCompleter / errMissingRunner are returned when an operation needs an
// optional dependency that was not injected.
var (
	errMissingCompleter = errors.New("governance/service: AI completer not configured (this operation needs an AI provider)")
	errMissingRunner    = errors.New("governance/service: GitHub runner not configured (this operation needs the gh CLI)")
	errMissingPlanStore = errors.New("governance/service: plan store not configured (deep triage needs the release store)")
)

// newID returns a fresh identifier for store records the service creates
// (decision events, evidence, communication artifacts).
func newID() string { return uuid.New().String() }

// getRelease loads a release, translating the store's ErrNotFound sentinel into
// an actionable message naming the missing id.
func (s *Service) getRelease(ctx context.Context, id string) (*governance.GovernanceRelease, error) {
	rel, err := s.store.GetRelease(ctx, id)
	if err != nil {
		if errors.Is(err, governance.ErrNotFound) {
			return nil, fmt.Errorf("release %q not found", id)
		}
		return nil, fmt.Errorf("load release %q: %w", id, err)
	}
	return rel, nil
}

// getChange loads a change record, translating ErrNotFound into an actionable
// message naming the missing id.
func (s *Service) getChange(ctx context.Context, id string) (*governance.ChangeRecord, error) {
	ch, err := s.store.GetChangeRecord(ctx, id)
	if err != nil {
		if errors.Is(err, governance.ErrNotFound) {
			return nil, fmt.Errorf("change %q not found", id)
		}
		return nil, fmt.Errorf("load change %q: %w", id, err)
	}
	return ch, nil
}

// getAuthority loads a review authority, translating ErrNotFound into an
// actionable message naming the missing id.
func (s *Service) getAuthority(ctx context.Context, id string) (*governance.ReviewAuthority, error) {
	a, err := s.store.GetReviewAuthority(ctx, id)
	if err != nil {
		if errors.Is(err, governance.ErrNotFound) {
			return nil, fmt.Errorf("review authority %q not found (seeded authorities: pm, developer, bugbot, tester_ai, security_ai, ci_verifier)", id)
		}
		return nil, fmt.Errorf("load review authority %q: %w", id, err)
	}
	return a, nil
}
