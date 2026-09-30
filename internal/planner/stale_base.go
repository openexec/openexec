package planner

import (
	"fmt"
	"sort"
	"strings"
)

// PlanStaleBaseRefIssues applies StaleBaseRefIssue to every story and task
// verification script in plan. It is the single stale-base rule shared by every
// import path, so it looks at scripts only — never at goals — and a goal-less or
// legacy plan is checked exactly like a full one.
//
// The result maps an owner label ("story US-001" or "story US-001 task
// T-US-001-002") to its diagnostic; it is empty when every script is acceptable.
// Use StaleBaseRefOwners for a deterministic order.
func PlanStaleBaseRefIssues(plan *ProjectPlan) map[string]string {
	issues := map[string]string{}
	if plan == nil {
		return issues
	}
	for _, s := range plan.Stories {
		if issue := StaleBaseRefIssue(s.VerificationScript); issue != "" {
			issues["story "+s.ID] = issue
		}
		for _, t := range s.Tasks {
			if issue := StaleBaseRefIssue(t.VerificationScript); issue != "" {
				issues["story "+s.ID+" task "+t.ID] = issue
			}
		}
	}
	return issues
}

// StaleBaseRefOwners returns the owner labels of issues in sorted order.
func StaleBaseRefOwners(issues map[string]string) []string {
	owners := make([]string, 0, len(issues))
	for owner := range issues {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	return owners
}

// PlanStaleBaseRefError is PlanStaleBaseRefIssues as a refusal: nil when every
// script is acceptable, else one error naming every owner and its origin/<ref>
// fix, in StaleBaseRefOwners order. Import paths call it at the last step
// before persistence, so an approved or retained plan cannot bypass it.
func PlanStaleBaseRefError(plan *ProjectPlan) error {
	issues := PlanStaleBaseRefIssues(plan)
	if len(issues) == 0 {
		return nil
	}
	owners := StaleBaseRefOwners(issues)
	parts := make([]string, len(owners))
	for i, owner := range owners {
		parts[i] = owner + ": " + issues[owner]
	}
	return fmt.Errorf("stale base ref refused: %s", strings.Join(parts, "; "))
}
