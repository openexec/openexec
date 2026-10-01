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
	// Retain the boundary's direct neighbours: the work the owner decides on
	// and the work that waits for the answer. Not the whole connected graph.
	// A plan ending in owner acceptance is connected almost end to end, and
	// freezing all of it refused every review asking to merge an
	// implementation task with its verification ("refinement removed task
	// T-US-013-002"), so the plan could never be repaired at all.
	boundaries := make([]string, 0, len(retained))
	for id := range retained {
		boundaries = append(boundaries, id)
	}
	for _, story := range original.Stories {
		for _, task := range story.Tasks {
			if slices.ContainsFunc(task.DependsOn, func(dep string) bool { return slices.Contains(boundaries, dep) }) {
				retained[task.ID] = true
			}
			if slices.Contains(boundaries, task.ID) {
				for _, dep := range task.DependsOn {
					retained[dep] = true
				}
			}
		}
	}
	stories := map[string]*Story{}
	present := map[string]bool{}
	for i := range refined.Stories {
		stories[refined.Stories[i].ID] = &refined.Stories[i]
		for _, task := range refined.Stories[i].Tasks {
			present[task.ID] = true
		}
	}
	storyIDs, taskIDs := renumbering(original, refined, present)
	storyID := func(id string) string {
		if next, ok := storyIDs[id]; ok {
			return next
		}
		return id
	}
	taskID := func(id string) string {
		if next, ok := taskIDs[id]; ok {
			return next
		}
		return id
	}
	for _, s := range original.Stories {
		if !slices.ContainsFunc(s.Tasks, func(task Task) bool { return retained[task.ID] }) {
			continue
		}
		nextStory, ok := stories[storyID(s.ID)]
		if !ok {
			return fmt.Errorf("refinement removed story %s around a retained human boundary", s.ID)
		}
		for _, dep := range s.DependsOn {
			dep = storyID(dep)
			if _, ok := stories[dep]; ok && !slices.Contains(nextStory.DependsOn, dep) {
				nextStory.DependsOn = append(nextStory.DependsOn, dep)
			}
		}
		for _, t := range s.Tasks {
			if !retained[t.ID] {
				continue
			}
			i := slices.IndexFunc(nextStory.Tasks, func(candidate Task) bool { return candidate.ID == taskID(t.ID) })
			if i < 0 {
				return fmt.Errorf("refinement removed task %s around a retained human boundary", t.ID)
			}
			next := &nextStory.Tasks[i]
			if t.Mode == TaskModeHITL {
				// What the owner is asked stays; how the agent carries it out
				// is the refinement's to correct. Restoring the strategy too
				// returned a refused instruction (safe_commit in an acceptance
				// task, openexec b5b5611a) to every fresh plan, so review
				// could never approve one.
				next.Mode, next.DecisionReason, next.DecisionRef = t.Mode, t.DecisionReason, t.DecisionRef
				next.Description = t.Description
			}
			// An edge to work refinement was free to merge away is not restored;
			// it would point at a task that no longer exists.
			for _, dep := range t.DependsOn {
				dep = taskID(dep)
				if present[dep] && !slices.Contains(next.DependsOn, dep) {
					next.DependsOn = append(next.DependsOn, dep)
				}
			}
		}
	}
	return nil
}

// renumbering finds a retained boundary that refinement only renumbered. A
// review may ask for new ids, for instance when the plan reuses story ids
// already committed on the branch for different work, and renaming the story
// renames every task in it ("refinement removed story US-009" for a plan whose
// owner acceptance came back unchanged as US-019). The boundary is the same
// when exactly one new HITL task carries its decision_reason word for word; the
// other tasks of its story are followed through the same id prefix. Anything
// less certain stays a removal.
func renumbering(original, refined *ProjectPlan, present map[string]bool) (stories, tasks map[string]string) {
	stories, tasks = map[string]string{}, map[string]string{}
	known := map[string]bool{}
	for _, s := range original.Stories {
		for _, t := range s.Tasks {
			known[t.ID] = true
		}
	}
	for _, s := range original.Stories {
		for _, t := range s.Tasks {
			reason := strings.TrimSpace(t.DecisionReason)
			if t.Mode != TaskModeHITL || reason == "" {
				continue
			}
			// The boundary kept its id but its story was renumbered around it
			// ("refinement removed story US-009" for a plan that returned
			// T-US-009-003 unchanged inside US-019).
			if present[t.ID] {
				for _, ns := range refined.Stories {
					if ns.ID != s.ID && slices.ContainsFunc(ns.Tasks, func(nt Task) bool { return nt.ID == t.ID }) {
						stories[s.ID] = ns.ID
					}
				}
				continue
			}
			var matches [][2]string
			for _, ns := range refined.Stories {
				for _, nt := range ns.Tasks {
					if nt.Mode == TaskModeHITL && !known[nt.ID] && strings.TrimSpace(nt.DecisionReason) == reason {
						matches = append(matches, [2]string{ns.ID, nt.ID})
					}
				}
			}
			if len(matches) != 1 {
				continue
			}
			tasks[t.ID] = matches[0][1]
			if matches[0][0] != s.ID {
				stories[s.ID] = matches[0][0]
			}
		}
	}
	for _, s := range original.Stories {
		next, ok := stories[s.ID]
		if !ok {
			continue
		}
		for _, t := range s.Tasks {
			if _, mapped := tasks[t.ID]; mapped || present[t.ID] {
				continue
			}
			candidate := strings.Replace(t.ID, "-"+s.ID+"-", "-"+next+"-", 1)
			if candidate != t.ID && present[candidate] && !known[candidate] {
				tasks[t.ID] = candidate
			}
		}
	}
	return stories, tasks
}
