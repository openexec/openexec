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

// retainedLedger presents the persisted backlog as plan content, so a
// continued story is carried exactly as it was imported.
func retainedLedger(rel *release.Manager) planner.RetainedLedger {
	finished := func(t *release.Task) bool {
		return t.Status == release.TaskStatusDone || t.Status == release.TaskStatusApproved
	}
	return planner.RetainedLedger{
		Goal: func(id string) (planner.Goal, bool) {
			g := rel.GetGoal(id)
			if g == nil {
				return planner.Goal{}, false
			}
			return planner.Goal{ID: g.ID, Title: g.Title, Description: g.Description, SuccessCriteria: g.SuccessCriteria, VerificationMethod: g.VerificationMethod}, true
		},
		Story: func(id string) (planner.Story, bool) {
			s := rel.GetStory(id)
			if s == nil {
				return planner.Story{}, false
			}
			story := planner.Story{ID: s.ID, GoalID: s.GoalID, Title: s.Title, Description: s.Description, AcceptanceCriteria: s.AcceptanceCriteria, VerificationScript: s.VerificationScript, Contract: s.Contract, DependsOn: s.DependsOn}
			for _, taskID := range s.Tasks {
				t := rel.GetTask(taskID)
				if t == nil {
					return planner.Story{}, false
				}
				task := planner.Task{ID: t.ID, Title: t.Title, Description: t.Description, DependsOn: t.DependsOn, VerificationScript: t.VerificationScript}
				task.Mode, _ = t.Metadata["mode"].(string)
				task.DecisionReason, _ = t.Metadata["decision_reason"].(string)
				task.DecisionRef, _ = t.Metadata["decision_ref"].(string)
				if paths, ok := t.Metadata["allowed_paths"].([]interface{}); ok {
					for _, p := range paths {
						if p, ok := p.(string); ok {
							task.AllowedPaths = append(task.AllowedPaths, p)
						}
					}
				}
				story.Tasks = append(story.Tasks, task)
			}
			return story, true
		},
		TaskStory: func(id string) (string, bool) {
			if t := rel.GetTask(id); t != nil {
				return t.StoryID, true
			}
			return "", false
		},
		OpenBoundary: func(storyID string) bool {
			s := rel.GetStory(storyID)
			if s == nil {
				return false
			}
			for _, taskID := range s.Tasks {
				if t := rel.GetTask(taskID); t != nil && t.ExecutionMode() == release.TaskModeHITL && !finished(t) {
					return true
				}
			}
			return false
		},
		Taken: func(id string) bool {
			return rel.GetGoal(id) != nil || rel.GetStory(id) != nil || rel.GetTask(id) != nil
		},
		Conflicts: func(plan *planner.ProjectPlan) map[string]bool { return reviewedIdentityConflicts(rel, plan) },
	}
}
