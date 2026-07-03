// Package validation implements the central status machine and structural
// validation rules for OpenExec release governance. It is a pure package: it
// has no database, file, or network access. Every function operates on
// already-loaded governance structs so that the CLI, MCP server, and future UI
// can all share one authoritative validator.
//
// This file encodes the legal status transitions for releases and change
// records (Phase 2 transition tables). Risk-tier *authority* rules (who may
// approve which risk tier, required reviews) deliberately live in a separate
// policy package and are NOT implemented here.
package validation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/openexec/openexec/internal/governance"
)

// ErrInvalidTransition is the sentinel wrapped by every illegal transition
// error. Callers can test with errors.Is for branching; the wrapped message
// names the offending from/to pair.
var ErrInvalidTransition = errors.New("governance: invalid status transition")

// releaseActiveStates are the non-terminal, non-blocked release states. "Any
// active state -> blocked" and "blocked -> previous active state" are expressed
// against this set.
var releaseActiveStates = []string{
	governance.ReleaseStatusDraft,
	governance.ReleaseStatusPlanned,
	governance.ReleaseStatusApproved,
	governance.ReleaseStatusImplementing,
	governance.ReleaseStatusReadyForTest,
	governance.ReleaseStatusTesting,
	governance.ReleaseStatusReadyToDeploy,
	governance.ReleaseStatusDeployed,
}

// releaseTransitions maps each release status to the set of statuses it may
// legally move to. Terminal states (closed, cancelled) have no outgoing edges.
//
// "blocked -> previous active state": a static from/to table cannot know which
// active state a release was blocked from, so blocked is permitted to return to
// ANY active state. The caller is responsible for restoring the actual recorded
// previous state; this validator only guarantees the destination is an active
// (resumable) state and never a terminal one.
var releaseTransitions = buildReleaseTransitions()

func buildReleaseTransitions() map[string]map[string]bool {
	m := map[string]map[string]bool{
		governance.ReleaseStatusDraft: {
			governance.ReleaseStatusPlanned:   true,
			governance.ReleaseStatusCancelled: true,
		},
		governance.ReleaseStatusPlanned: {
			governance.ReleaseStatusApproved:  true,
			governance.ReleaseStatusCancelled: true,
		},
		governance.ReleaseStatusApproved: {
			governance.ReleaseStatusImplementing: true,
			// Scope-add invalidation: attaching new scope to an approved release
			// resets it to planned for re-approval (see Service.AttachChange).
			governance.ReleaseStatusPlanned: true,
		},
		governance.ReleaseStatusImplementing: {
			governance.ReleaseStatusReadyForTest: true,
		},
		governance.ReleaseStatusReadyForTest: {
			governance.ReleaseStatusTesting: true,
		},
		governance.ReleaseStatusTesting: {
			governance.ReleaseStatusReadyToDeploy: true,
		},
		governance.ReleaseStatusReadyToDeploy: {
			governance.ReleaseStatusDeployed: true,
		},
		governance.ReleaseStatusDeployed: {
			governance.ReleaseStatusClosed: true,
		},
		// Terminal states.
		governance.ReleaseStatusClosed:    {},
		governance.ReleaseStatusCancelled: {},
		// blocked may resume into any active state (see doc comment above).
		governance.ReleaseStatusBlocked: {},
	}

	// Any active state may move to blocked.
	for _, s := range releaseActiveStates {
		m[s][governance.ReleaseStatusBlocked] = true
	}
	// blocked may return to any active state.
	for _, s := range releaseActiveStates {
		m[governance.ReleaseStatusBlocked][s] = true
	}
	return m
}

// changeTransitions maps each change-record status to its legal next statuses.
var changeTransitions = map[string]map[string]bool{
	governance.ChangeStatusCandidate: {
		governance.ChangeStatusPlanned:  true,
		governance.ChangeStatusRejected: true,
		governance.ChangeStatusDeferred: true,
	},
	governance.ChangeStatusPlanned: {
		governance.ChangeStatusPlanReady: true,
		governance.ChangeStatusRejected:  true,
		governance.ChangeStatusDeferred:  true,
	},
	governance.ChangeStatusPlanReady: {
		governance.ChangeStatusChangesRequested: true,
		governance.ChangeStatusApprovedForAI:    true,
		governance.ChangeStatusRejected:         true,
		governance.ChangeStatusDeferred:         true,
	},
	governance.ChangeStatusChangesRequested: {
		governance.ChangeStatusPlanReady: true,
	},
	governance.ChangeStatusApprovedForAI: {
		governance.ChangeStatusImplementing: true,
	},
	governance.ChangeStatusImplementing: {
		governance.ChangeStatusPROpen: true,
		// Manual-evidence path: a change verified without a PR (manual/test/CI
		// evidence) may go straight to ready_for_test. The evidence requirement
		// is enforced by ValidateReadyForTest; this edge makes that path legal so
		// the manual exception is not dead (a PR-based change instead routes
		// implementing -> pr_open -> ready_for_test).
		governance.ChangeStatusReadyForTest: true,
	},
	governance.ChangeStatusPROpen: {
		governance.ChangeStatusReadyForTest: true,
	},
	governance.ChangeStatusReadyForTest: {
		governance.ChangeStatusDone: true,
	},
	// Terminal states.
	governance.ChangeStatusDone:     {},
	governance.ChangeStatusRejected: {},
	governance.ChangeStatusDeferred: {},
}

// ValidateReleaseTransition reports whether a release may move from one status
// to another. A nil return means the transition is legal; otherwise the error
// wraps ErrInvalidTransition and names the from/to pair and the legal options.
func ValidateReleaseTransition(from, to string) error {
	return validateTransition("release", releaseTransitions, from, to)
}

// ValidateChangeTransition reports whether a change record may move from one
// status to another. A nil return means the transition is legal; otherwise the
// error wraps ErrInvalidTransition.
func ValidateChangeTransition(from, to string) error {
	return validateTransition("change record", changeTransitions, from, to)
}

func validateTransition(kind string, table map[string]map[string]bool, from, to string) error {
	if from == to {
		return fmt.Errorf("%w: %s cannot transition from %q to itself", ErrInvalidTransition, kind, from)
	}
	allowed, known := table[from]
	if !known {
		return fmt.Errorf("%w: %s has no known status %q", ErrInvalidTransition, kind, from)
	}
	if allowed[to] {
		return nil
	}
	return fmt.Errorf(
		"%w: %s may not move from %q to %q (allowed: %s)",
		ErrInvalidTransition, kind, from, to, allowedList(allowed),
	)
}

// allowedList renders the legal destinations for an error message, sorted for
// deterministic output.
func allowedList(allowed map[string]bool) string {
	if len(allowed) == 0 {
		return "<none, terminal state>"
	}
	out := make([]string, 0, len(allowed))
	for s := range allowed {
		out = append(out, s)
	}
	sort.Strings(out)
	return strings.Join(out, ", ")
}
