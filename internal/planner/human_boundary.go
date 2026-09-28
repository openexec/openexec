package planner

import (
	"fmt"
	"slices"
	"strings"
)

// ExecutionMetadata uses the existing native task metadata in both import paths.
// Missing reasons never convert retained legacy HITL work into automatic work.
func (t Task) ExecutionMetadata() map[string]interface{} {
	if t.Mode == "" && t.DecisionReason == "" && t.DecisionRef == "" {
		return nil
	}
	m := map[string]interface{}{"mode": t.Mode}
	if t.DecisionReason != "" {
		m["decision_reason"] = t.DecisionReason
	}
	if t.DecisionRef != "" {
		m["decision_ref"] = t.DecisionRef
	}
	return m
}

// LintHumanBoundaries checks newly reviewed plans, not retained task ledgers.
// Whether the reason is justified still requires independent plan review.
func LintHumanBoundaries(plan *ProjectPlan) []string {
	var issues []string
	for _, s := range plan.Stories {
		for _, t := range s.Tasks {
			switch t.Mode {
			case TaskModeHITL:
				if strings.TrimSpace(t.DecisionReason) == "" {
					issues = append(issues, t.ID+": HITL requires a concrete decision_reason")
				}
			case "", TaskModeAFK:
				if t.DecisionReason != "" || t.DecisionRef != "" {
					issues = append(issues, t.ID+": decision metadata requires HITL mode")
				}
			default:
				issues = append(issues, t.ID+": unknown execution mode")
			}
		}
	}
	return issues
}

// Refinement is not an owner answer. Keep retained boundary identities and the
// graph around them; a rejected refinement cannot replace the original plan.
//
// A refinement that rewords a retained boundary, or drops an edge around it,
// gets the original back rather than refusing the whole plan: the model was
// asked to fix review findings, not to decide what the owner is asked, and
// refusing stopped an otherwise sound repair on a changed sentence
// ("refinement changed retained human boundary T-US-025-002"). Only removing
// retained work is refused, since restoring it would mean guessing where it
// now belongs.
func preserveHumanBoundaries(original, refined *ProjectPlan) error {
	retained := map[string]bool{}
	for _, story := range original.Stories {
		for _, task := range story.Tasks {
			if task.Mode == TaskModeHITL {
				retained[task.ID] = true
			}
		}
	}
	if len(retained) == 0 {
		return nil
	}
	// Retain the connected task graph around a boundary, while allowing
	// unrelated work to be decomposed or removed by ordinary refinement.
	for changed := true; changed; {
		changed = false
		for _, story := range original.Stories {
			for _, task := range story.Tasks {
				connected := retained[task.ID]
				for _, dep := range task.DependsOn {
					connected = connected || retained[dep]
				}
				if !connected {
					continue
				}
				for _, id := range append([]string{task.ID}, task.DependsOn...) {
					if !retained[id] {
						retained[id] = true
						changed = true
					}
				}
			}
		}
	}
	stories := map[string]*Story{}
	for i := range refined.Stories {
		stories[refined.Stories[i].ID] = &refined.Stories[i]
	}
	for _, s := range original.Stories {
		if !slices.ContainsFunc(s.Tasks, func(task Task) bool { return retained[task.ID] }) {
			continue
		}
		nextStory, ok := stories[s.ID]
		if !ok {
			return fmt.Errorf("refinement removed story %s around a retained human boundary", s.ID)
		}
		for _, dep := range s.DependsOn {
			if !slices.Contains(nextStory.DependsOn, dep) {
				nextStory.DependsOn = append(nextStory.DependsOn, dep)
			}
		}
		for _, t := range s.Tasks {
			if !retained[t.ID] {
				continue
			}
			i := slices.IndexFunc(nextStory.Tasks, func(candidate Task) bool { return candidate.ID == t.ID })
			if i < 0 {
				return fmt.Errorf("refinement removed task %s around a retained human boundary", t.ID)
			}
			next := &nextStory.Tasks[i]
			if t.Mode == TaskModeHITL {
				next.Mode, next.DecisionReason, next.DecisionRef = t.Mode, t.DecisionReason, t.DecisionRef
				next.Description, next.TechnicalStrategy = t.Description, t.TechnicalStrategy
			}
			for _, dep := range t.DependsOn {
				if !slices.Contains(next.DependsOn, dep) {
					next.DependsOn = append(next.DependsOn, dep)
				}
			}
		}
	}
	return nil
}
