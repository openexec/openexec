package planner

import (
	"encoding/json"
	"strings"
)

// remapContentIDs reaches a fixed point before review: moving a dependency or
// a prose reference can invalidate an otherwise identical retained row. Every
// iteration renders from the original plan, so substitutions never cascade.
func remapContentIDs(plan *ProjectPlan, look ExistingLookup) int {
	raw, _ := json.Marshal(plan)
	refs := map[string]string{}
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
	taken := func(id string) bool {
		_, g := look.GoalTitle(id)
		_, s := look.StoryTitle(id)
		return used[id] || g || s || look.TaskExists(id)
	}
	for {
		conflicts := look.Conflicts(plan)
		old, next := "", ""
		for _, g := range plan.Goals {
			if conflicts[g.ID] {
				old = g.ID
				next = nextFreeID("G", taken)
				break
			}
		}
		if old == "" {
			for _, s := range plan.Stories {
				if conflicts[s.ID] {
					old = s.ID
					next = nextFreeID("US", taken)
					break
				}
			}
		}
		if old == "" {
			for _, s := range plan.Stories {
				for _, t := range s.Tasks {
					if conflicts[t.ID] {
						old = t.ID
						next = nextFreeTaskID(t.ID, taken)
						break
					}
				}
				if old != "" {
					break
				}
			}
		}
		if old == "" {
			return len(refs)
		}
		used[next] = true
		refs[old] = next
		for _, s := range plan.Stories {
			if s.ID == old {
				for _, t := range s.Tasks {
					id := t.ID
					if strings.HasPrefix(id, "T-"+old+"-") {
						id = "T-" + next + strings.TrimPrefix(id, "T-"+old)
					}
					if (id != t.ID && used[id]) || look.TaskExists(id) {
						id = nextFreeTaskID(id, taken)
					}
					if id == t.ID {
						continue
					}
					used[id] = true
					source := t.ID
					for k, v := range refs {
						if v == t.ID {
							source = k
							break
						}
					}
					refs[source] = id
				}
			}
		}
		_ = json.Unmarshal(raw, plan)
		rewritePlanRefs(plan, refs)
	}
}

func rewritePlanRefs(plan *ProjectPlan, refs map[string]string) {
	id := func(s string) string {
		if n, ok := refs[s]; ok {
			return n
		}
		return s
	}
	rw := func(s string) string { return rewriteIDRefs(s, refs) }
	for i := range plan.Goals {
		g := &plan.Goals[i]
		g.ID = id(g.ID)
		g.Description = rw(g.Description)
		g.SuccessCriteria = rw(g.SuccessCriteria)
		g.VerificationMethod = rw(g.VerificationMethod)
	}
	for i := range plan.Stories {
		s := &plan.Stories[i]
		s.ID = id(s.ID)
		s.GoalID = id(s.GoalID)
		for j := range s.DependsOn {
			s.DependsOn[j] = id(s.DependsOn[j])
		}
		s.Description = rw(s.Description)
		s.Contract = rw(s.Contract)
		s.VerificationScript = rw(s.VerificationScript)
		for j := range s.AcceptanceCriteria {
			s.AcceptanceCriteria[j] = rw(s.AcceptanceCriteria[j])
		}
		for j := range s.Tasks {
			t := &s.Tasks[j]
			t.ID = id(t.ID)
			for k := range t.DependsOn {
				t.DependsOn[k] = id(t.DependsOn[k])
			}
			t.Description = rw(t.Description)
			t.TechnicalStrategy = rw(t.TechnicalStrategy)
			t.VerificationScript = rw(t.VerificationScript)
		}
	}
}
