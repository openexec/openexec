package manager

import (
	"fmt"
	"github.com/openexec/openexec/internal/planner"
	"github.com/openexec/openexec/internal/release"
)

func reviewedIdentityConflicts(retained *release.Manager, plan *planner.ProjectPlan) map[string]bool {
	conflicts := map[string]bool{}
	goals, stories, tasks := reviewedPlanRows(plan)
	for _, g := range goals {
		if old := retained.GetGoal(g.ID); old != nil {
			conflicts[g.ID] = !release.ReviewedGoalEqual(old, g)
		}
	}
	for _, s := range stories {
		if old := retained.GetStory(s.ID); old != nil {
			conflicts[s.ID] = !release.ReviewedStoryEqual(old, s)
		}
	}
	for _, t := range tasks {
		if old := retained.GetTask(t.ID); old != nil {
			conflicts[t.ID] = !release.ReviewedTaskEqual(old, t)
		}
	}
	return conflicts
}

// Ambiguous incoming sources cannot be rewritten consistently.
func uniquePlanIDs(plan *planner.ProjectPlan) error {
	if plan == nil {
		return fmt.Errorf("plan is nil")
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, g := range plan.Goals {
		ids = append(ids, g.ID)
	}
	for _, s := range plan.Stories {
		ids = append(ids, s.ID)
		for _, t := range s.Tasks {
			ids = append(ids, t.ID)
		}
	}
	for _, id := range ids {
		if id == "" || seen[id] {
			return fmt.Errorf("empty or duplicate plan ID %q", id)
		}
		seen[id] = true
	}
	return nil
}
