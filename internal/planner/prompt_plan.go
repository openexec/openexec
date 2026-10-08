package planner

import (
	"encoding/json"
	"slices"
	"unicode/utf8"
)

// continuedDescriptionBytes bounds a continued task's description in a review
// or fix prompt.
const continuedDescriptionBytes = 1500

// promptPlan is the plan as a reviewer or fixer reads it. A task the plan
// continues is persisted work that import keeps exactly as it was
// (ContinueRetainedWork), so the prompt names it and shortens its description:
// a model's rewording of it is dropped anyway. Sent whole, the SRE Goal
// 0ca8ccef's eighteen fix tasks, each carrying a full review, made a 407 KB
// fix prompt that the provider refused before inference (run 4107961d, 10-07).
func promptPlan(plan *ProjectPlan) ([]byte, error) {
	shown := *plan
	if len(plan.Continues) > 0 {
		shown.Stories = make([]Story, len(plan.Stories))
		for i, story := range plan.Stories {
			story.Tasks = slices.Clone(story.Tasks)
			for j, task := range story.Tasks {
				if slices.Contains(plan.Continues, task.ID) && len(task.Description) > continuedDescriptionBytes {
					cut := continuedDescriptionBytes
					for cut > 0 && !utf8.RuneStart(task.Description[cut]) {
						cut--
					}
					story.Tasks[j].Description = task.Description[:cut] + " … [persisted task, shortened here; it is kept as imported]"
				}
			}
			shown.Stories[i] = story
		}
	}
	// A re-plan lists only the goals it touches. Shown no others, the SRE
	// Goal 0ca8ccef's reviewer called the persisted G-000 undefined and asked
	// for a new goal G-SR6 where G-009 already served SR6; the fixer cited it
	// and import refused the plan (run d1687257, 10-08).
	listed := map[string]bool{}
	for _, g := range plan.Goals {
		listed[g.ID] = true
	}
	var persisted []Goal
	for _, g := range plan.PersistedGoals {
		if !listed[g.ID] {
			persisted = append(persisted, g)
		}
	}
	return json.Marshal(struct {
		*ProjectPlan
		PersistedGoals []Goal `json:"persisted_goals,omitempty"`
	}{&shown, persisted})
}
