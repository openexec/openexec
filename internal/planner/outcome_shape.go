package planner

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// realExampleRunes bounds a real_example reference. It names the input; a
// longer value is the input pasted into the plan artifact.
const realExampleRunes = 300

// preOutcomeShapeVersions are the schema versions plans carried before the
// outcome shape existed. Their persisted plans replay as they were judged.
var preOutcomeShapeVersions = []string{"", "legacy", "1.0", "1.0.0", "1.1"}

// LintOutcomeShape checks the structure of a multi-story plan: a cheapest
// baseline it beats, a walking skeleton on the real example before the rest,
// and infrastructure after the skeleton. It reads only the roles and
// references the plan declares, never its wording, so it proves shape, not
// usefulness; whether a role is honest is the independent reviewer's call.
// Plans persisted under an earlier schema, single-story plans and the
// persisted stories a re-plan continues are not checked.
func LintOutcomeShape(plan *ProjectPlan) []string {
	if plan == nil || slices.Contains(preOutcomeShapeVersions, plan.SchemaVersion) {
		return nil
	}
	roles := map[string]string{}
	var implementation []Story
	for _, s := range plan.Stories {
		if slices.Contains(plan.Continues, s.ID) {
			continue
		}
		roles[s.ID] = s.Role
		if s.Role != RoleStudy && s.Role != RoleValidation {
			implementation = append(implementation, s)
		}
	}
	if len(implementation) < 2 {
		return nil
	}

	var issues []string
	switch b := plan.Baseline; {
	case b == nil:
		issues = append(issues, "plan records no baseline: state what the cheapest approach gives on the real example and how this plan beats it")
	default:
		for _, f := range []struct{ name, value string }{{"approach", b.Approach}, {"result", b.Result}, {"bar", b.Bar}, {"verdict", b.Verdict}, {"source", b.Source}} {
			if strings.TrimSpace(f.value) == "" {
				issues = append(issues, "baseline "+f.name+" is empty")
			}
		}
		switch b.Verdict {
		case "", VerdictBuild, VerdictBuildOnBaseline:
		case VerdictDoNotBuild:
			issues = append(issues, "baseline verdict do_not_build contradicts a multi-story build plan")
		default:
			issues = append(issues, "baseline verdict "+b.Verdict+" is not build, build_on_baseline or do_not_build")
		}
	}

	// A stated prerequisite (tenant isolation a private slice needs, an
	// owner-stated order) may come first; nothing else may.
	leading := true
	var skeleton *Story
	for i, s := range implementation {
		switch s.Role {
		case RoleSkeleton, RoleCore:
		case RoleInfrastructure, RolePrerequisite:
			if strings.TrimSpace(s.RoleReason) == "" {
				issues = append(issues, s.ID+": "+s.Role+" story requires a role_reason")
			}
		case "":
			issues = append(issues, s.ID+": story declares no role")
		default:
			issues = append(issues, s.ID+": unknown role "+s.Role)
		}
		if leading && s.Role != RolePrerequisite {
			leading = false
			if s.Role != RoleSkeleton {
				issues = append(issues, s.ID+": first implementation story is "+roleName(s.Role)+", not the walking skeleton; put the thinnest end-to-end slice on the real example before it")
			}
		}
		if skeleton == nil && s.Role == RoleSkeleton {
			skeleton = &implementation[i]
		}
	}
	if skeleton == nil {
		return append(issues, "plan has no walking skeleton story")
	}
	switch ref := strings.TrimSpace(skeleton.RealExample); {
	case ref == "":
		issues = append(issues, skeleton.ID+": skeleton names no real_example input")
	case utf8.RuneCountInString(ref) > realExampleRunes || strings.Contains(ref, "\n"):
		issues = append(issues, skeleton.ID+": real_example must reference the input, not contain it")
	}
	for _, dep := range skeleton.DependsOn {
		if role, planned := roles[dep]; planned && role != RoleStudy && role != RolePrerequisite {
			issues = append(issues, skeleton.ID+": skeleton depends on "+dep+", which is neither a study nor a prerequisite story")
		}
	}
	for _, s := range implementation {
		if s.Role == RoleInfrastructure && !dependsOnStory(plan, s.ID, skeleton.ID) {
			issues = append(issues, s.ID+": infrastructure story does not depend on skeleton "+skeleton.ID)
		}
	}
	return issues
}

func roleName(role string) string {
	if role == "" {
		return "a story with no role"
	}
	return role
}

// dependsOnStory reports whether story from reaches story to through the
// plan's story-level depends_on, directly or through other stories.
func dependsOnStory(plan *ProjectPlan, from, to string) bool {
	deps := map[string][]string{}
	for _, s := range plan.Stories {
		deps[s.ID] = s.DependsOn
	}
	seen := map[string]bool{}
	queue := slices.Clone(deps[from])
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == to {
			return true
		}
		if !seen[id] {
			seen[id] = true
			queue = append(queue, deps[id]...)
		}
	}
	return false
}
