package planner

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ExistingLookup supplies retained occupancy and optionally full canonical content
// comparison. Native reviewed planning always supplies Conflicts. Title-only
// callers retain the legacy policy: matching titles reuse goals/stories while
// occupied tasks move. Allocation reserves incoming IDs and rewrites references.
type ExistingLookup struct {
	// Conflicts compares the complete, rewritten importer content against retained rows.
	// When provided, exact task content is reusable as well as goals and stories.
	Conflicts func(*ProjectPlan) map[string]bool
	// GoalTitle returns the title of an existing goal and whether it exists.
	GoalTitle func(id string) (string, bool)
	// StoryTitle returns the title of an existing story and whether it exists.
	StoryTitle func(id string) (string, bool)
	// TaskExists reports whether a task ID is already taken.
	TaskExists func(id string) bool
}

// RemapPlanIDs resolves identity and reference changes to a fixed point.
// It returns the number of changed Goal, story and task identities.
func RemapPlanIDs(plan *ProjectPlan, look ExistingLookup) int {
	if plan == nil {
		return 0
	}
	if look.Conflicts == nil {
		// Preserve the exported title-only lookup contract for legacy callers.
		look.Conflicts = func(p *ProjectPlan) map[string]bool {
			conflicts := map[string]bool{}
			for _, g := range p.Goals {
				title, exists := look.GoalTitle(g.ID)
				conflicts[g.ID] = exists && title != g.Title
			}
			for _, s := range p.Stories {
				title, exists := look.StoryTitle(s.ID)
				conflicts[s.ID] = exists && title != s.Title
				for _, t := range s.Tasks {
					conflicts[t.ID] = look.TaskExists(t.ID)
				}
			}
			return conflicts
		}
	}
	return remapContentIDs(plan, look)
}

// idRefTokenRe matches a WHOLE planner ID token (goal G-NNN, story US-NNN, or
// task T-US-NNN-NNN). The task alternative is listed first and the digit runs
// are greedy, so "T-US-001-002" matches as one task token (not an inner US-001)
// and "US-0010" matches wholly (so a US-001 remap never corrupts it).
var idRefTokenRe = regexp.MustCompile(`T-US-[0-9]+-[0-9]+|US-[0-9]+|G-[0-9]+`)

// rewriteIDRefs replaces whole ID tokens in s according to refs (old->new),
// leaving any token not in refs untouched.
func rewriteIDRefs(s string, refs map[string]string) string {
	if s == "" {
		return s
	}
	return idRefTokenRe.ReplaceAllStringFunc(s, func(tok string) string {
		if nid, ok := refs[tok]; ok {
			return nid
		}
		return tok
	})
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
