package planner

import (
	"fmt"
	"slices"
)

// RetainedLedger is the persisted backlog a re-plan is drawn against.
type RetainedLedger struct {
	// Goal returns a persisted goal as plan content.
	Goal func(id string) (Goal, bool)
	// Story returns a persisted story with its tasks in ledger order, each
	// carrying its persisted content (TechnicalStrategy stays empty: the
	// persisted description already holds it).
	Story func(id string) (Story, bool)
	// TaskStory names the persisted story that holds a task.
	TaskStory func(id string) (string, bool)
	// Taken reports whether any persisted goal, story or task holds an ID.
	Taken func(id string) bool
	// Conflicts reports which plan IDs differ from their persisted rows.
	Conflicts func(*ProjectPlan) map[string]bool
}

// ContinueRetainedWork makes a re-plan continue the persisted backlog instead
// of renaming it. Every goal, story and task ID the plan lists that is already
// persisted names that persisted row: the plan carries it with its persisted
// content and ID, never a new ID. New tasks the plan put into a persisted
// story move to a continuation story that depends on it, a new story that
// only regrouped persisted tasks resolves to their story, and genuinely new
// work keeps (or is given) IDs nothing persisted holds. It returns the IDs the
// plan continues; import leaves those rows as they are.
//
// Treating a changed re-listing as new work renamed it: a Goal's owner
// boundary T-US-008-002 came back as T-US-017-002, T-US-022-002 and
// T-US-014-002, and its pending work US-004 as US-021 and US-013, plan review
// refused the renames, and the Goal re-planned nine times without
// converging. A persisted task is never renamed (owner rule, 2026-10-05); a
// re-plan's rewording of one is dropped in favour of what was imported.
func ContinueRetainedWork(plan *ProjectPlan, ledger RetainedLedger) []string {
	if plan == nil {
		return nil
	}
	plan.Continues = nil
	continued := map[string]Story{}
	retainedTasks := map[string]string{}
	keep := func(storyID string) bool {
		if _, ok := continued[storyID]; ok {
			return true
		}
		story, ok := ledger.Story(storyID)
		if !ok {
			return false
		}
		continued[storyID] = story
		for _, t := range story.Tasks {
			retainedTasks[t.ID] = storyID
		}
		return true
	}
	// A re-listing identical to its persisted rows already keeps its IDs.
	conflicts := ledger.Conflicts(plan)
	for _, s := range plan.Stories {
		changed := conflicts[s.ID]
		for _, t := range s.Tasks {
			if storyID, ok := ledger.TaskStory(t.ID); ok && conflicts[t.ID] && storyID != s.ID {
				keep(storyID)
			}
			changed = changed || conflicts[t.ID]
		}
		if changed {
			keep(s.ID)
		}
	}
	goals := []string{}
	for i, g := range plan.Goals {
		if !conflicts[g.ID] {
			continue
		}
		if persisted, ok := ledger.Goal(g.ID); ok {
			plan.Goals[i] = persisted
			goals = append(goals, g.ID)
		}
	}
	if len(continued) == 0 {
		if len(goals) == 0 {
			return nil
		}
		slices.Sort(goals)
		plan.Continues = goals
		return plan.Continues
	}

	used := map[string]bool{}
	for _, g := range plan.Goals {
		used[g.ID] = true
	}
	for _, s := range plan.Stories {
		used[s.ID] = true
		for _, t := range s.Tasks {
			used[t.ID] = true
		}
	}
	taken := func(id string) bool { return used[id] || ledger.Taken(id) }
	refs := map[string]string{}
	emitted := map[string]bool{}
	var stories []Story
	emit := func(id string) {
		if !emitted[id] {
			emitted[id] = true
			stories = append(stories, continued[id])
		}
	}
	for _, s := range plan.Stories {
		var fresh []Task
		var held []string
		for _, t := range s.Tasks {
			if storyID, ok := retainedTasks[t.ID]; ok {
				if !slices.Contains(held, storyID) {
					held = append(held, storyID)
				}
				continue
			}
			fresh = append(fresh, t)
		}
		if _, ok := continued[s.ID]; ok {
			emit(s.ID)
			for _, id := range held {
				emit(id)
			}
			if len(fresh) == 0 {
				continue
			}
			// The plan's own wording describes the new work; the persisted
			// story keeps what was already agreed.
			retained := s.ID
			s.ID = nextFreeID("US", taken)
			used[s.ID] = true
			if !slices.Contains(s.DependsOn, retained) {
				s.DependsOn = append(slices.Clone(s.DependsOn), retained)
			}
		} else {
			for _, id := range held {
				emit(id)
			}
			if len(fresh) == 0 {
				// A story that only regrouped retained tasks is that work.
				if len(held) > 0 {
					refs[s.ID] = held[0]
				}
				continue
			}
		}
		for i := range fresh {
			id := fresh[i].ID
			if want := fmt.Sprintf("T-%s-%03d", s.ID, i+1); want != id && !taken(want) {
				used[want] = true
				refs[id] = want
			}
		}
		s.Tasks = fresh
		stories = append(stories, s)
	}
	plan.Stories = stories

	ids := goals
	for id, story := range continued {
		ids = append(ids, id)
		for _, t := range story.Tasks {
			ids = append(ids, t.ID)
		}
		// A persisted goal the plan restates is the persisted goal, or the
		// remapper would move it and drag the continued story's goal_id along.
		for i := range plan.Goals {
			if plan.Goals[i].ID == story.GoalID {
				if goal, ok := ledger.Goal(story.GoalID); ok {
					plan.Goals[i] = goal
					ids = append(ids, goal.ID)
				}
			}
		}
	}
	slices.Sort(ids)
	plan.Continues = slices.Compact(ids)
	rewritePlanRefs(plan, refs)
	return plan.Continues
}
