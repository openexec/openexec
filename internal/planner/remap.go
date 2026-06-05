package planner

import (
	"fmt"
	"strconv"
	"strings"
)

// RemapPlanIDs makes re-planning possible on a project that already has a
// backlog. The story-generation prompt always numbers from US-001/G-001, so a
// second plan (a refactor epic, a new feature wave after the initial build)
// collides with the existing backlog — and plan import skips colliding IDs
// silently, importing nothing.
//
// Collision policy:
//   - Same ID, same title: treated as the same item (idempotent re-import of
//     an identical plan) — left untouched so the importer's skip applies.
//   - Same ID, different title: a genuinely new item — remapped to the next
//     free ID. Story remaps cascade into task IDs (T-US-001-002 follows its
//     story), story/task depends_on references, and story goal_id references.
//
// Returns the number of remapped IDs (goals + stories + tasks).
type ExistingLookup struct {
	// GoalTitle returns the title of an existing goal and whether it exists.
	GoalTitle func(id string) (string, bool)
	// StoryTitle returns the title of an existing story and whether it exists.
	StoryTitle func(id string) (string, bool)
	// TaskExists reports whether a task ID is already taken.
	TaskExists func(id string) bool
}

func RemapPlanIDs(plan *ProjectPlan, look ExistingLookup) int {
	if plan == nil {
		return 0
	}
	remapped := 0

	// IDs already used by the incoming plan, so newly assigned IDs cannot
	// collide within the plan itself.
	usedGoal := map[string]bool{}
	for _, g := range plan.Goals {
		usedGoal[g.ID] = true
	}
	usedStory := map[string]bool{}
	usedTask := map[string]bool{}
	for _, s := range plan.Stories {
		usedStory[s.ID] = true
		for _, t := range s.Tasks {
			usedTask[t.ID] = true
		}
	}

	// 1. Goals
	goalMap := map[string]string{}
	for i := range plan.Goals {
		g := &plan.Goals[i]
		title, exists := look.GoalTitle(g.ID)
		if !exists || title == g.Title {
			continue
		}
		newID := nextFreeID("G", func(id string) bool {
			_, ex := look.GoalTitle(id)
			return ex || usedGoal[id]
		})
		usedGoal[newID] = true
		goalMap[g.ID] = newID
		g.ID = newID
		remapped++
	}

	// 2. Stories (compute map first; apply in a second pass so depends_on
	// references between incoming stories resolve consistently)
	storyMap := map[string]string{}
	for i := range plan.Stories {
		s := &plan.Stories[i]
		title, exists := look.StoryTitle(s.ID)
		if !exists || title == s.Title {
			continue
		}
		newID := nextFreeID("US", func(id string) bool {
			_, ex := look.StoryTitle(id)
			return ex || usedStory[id]
		})
		usedStory[newID] = true
		storyMap[s.ID] = newID
		remapped++
	}

	// 3. Apply story remaps: IDs, goal/dependency references, task IDs.
	taskMap := map[string]string{}
	for i := range plan.Stories {
		s := &plan.Stories[i]
		oldStoryID := s.ID
		if nid, ok := storyMap[s.ID]; ok {
			s.ID = nid
		}
		if nid, ok := goalMap[s.GoalID]; ok {
			s.GoalID = nid
		}
		for j, dep := range s.DependsOn {
			if nid, ok := storyMap[dep]; ok {
				s.DependsOn[j] = nid
			}
		}

		for j := range s.Tasks {
			t := &s.Tasks[j]
			oldTaskID := t.ID
			newTaskID := t.ID
			// Task IDs embed their story ID (T-US-001-002); follow the story.
			if nid, ok := storyMap[oldStoryID]; ok && strings.Contains(newTaskID, oldStoryID) {
				newTaskID = strings.Replace(newTaskID, oldStoryID, nid, 1)
			}
			// Resolve any remaining collision with existing tasks.
			if look.TaskExists(newTaskID) {
				newTaskID = nextFreeTaskID(newTaskID, func(id string) bool {
					return look.TaskExists(id) || usedTask[id]
				})
			}
			if newTaskID != oldTaskID {
				usedTask[newTaskID] = true
				taskMap[oldTaskID] = newTaskID
				t.ID = newTaskID
				remapped++
			}
		}
	}

	// 4. Task dependency references.
	for i := range plan.Stories {
		for j := range plan.Stories[i].Tasks {
			t := &plan.Stories[i].Tasks[j]
			for k, dep := range t.DependsOn {
				if nid, ok := taskMap[dep]; ok {
					t.DependsOn[k] = nid
				}
			}
		}
	}

	return remapped
}

// nextFreeID returns the first "<prefix>-NNN" not reported taken.
func nextFreeID(prefix string, taken func(string) bool) string {
	for n := 1; ; n++ {
		id := fmt.Sprintf("%s-%03d", prefix, n)
		if !taken(id) {
			return id
		}
	}
}

// nextFreeTaskID bumps the trailing numeric suffix of a task ID
// (T-US-007-001 → T-US-007-002, …) until it is free. IDs without a numeric
// suffix get one appended.
func nextFreeTaskID(id string, taken func(string) bool) string {
	base := id
	if idx := strings.LastIndex(id, "-"); idx > 0 {
		if _, err := strconv.Atoi(id[idx+1:]); err == nil {
			base = id[:idx]
		}
	}
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s-%03d", base, n)
		if !taken(candidate) {
			return candidate
		}
	}
}
