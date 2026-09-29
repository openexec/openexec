package planner

import (
	"regexp"
	"strings"
)

// falseGreenPatterns are verification-script shapes that report success even
// when the real check failed. Detecting them deterministically complements the
// plan prompt (which asks the model to avoid them) and the AI reviewers (which
// only sometimes catch them) — the reviewer recommended a deterministic linter.
var falseGreenPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"masks failure: a test/assert command followed by `|| <fallback>` (the fallback can pass while the real check failed)",
		regexp.MustCompile(`(?i)(vitest|jest|pytest|\bnpm test\b|\bgo test\b|\bgrep\b)[^\n|]*\|\|`)},
	{"hides errors: `2>/dev/null` on the checked command",
		regexp.MustCompile(`2>\s*/dev/null`)},
	{"discards exit status: a quiet grep (`grep -q`) piped into another command",
		regexp.MustCompile(`grep\s+-\w*q\w*\b[^\n|]*\|`)},
	{"masks failure: assertions chained as `A && B || C` (C passing hides an A/B failure)",
		regexp.MustCompile(`&&[^\n]*\|\|`)},
}

// staleBaseRefPatterns match a bare default-branch name used as a git base ref:
// the range form (`main...HEAD`, `master..HEAD`) and a positional argument to
// git diff/log/merge-base/rev-list. RE2 has no lookbehind, so the ref's
// preceding context is checked by hand in StaleBaseRefIssue (group 1 is the ref).
var staleBaseRefPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(main|master)\.\.`),
	regexp.MustCompile(`\bgit\s+(?:diff|log|merge-base|rev-list)\b[^\n|;&]*?\s(main|master)(?:\s|$|;|\||&|\))`),
}

// StaleBaseRefIssue reports a verification script that diffs against a bare
// local `main`/`master`. OpenExec runs tasks in linked candidate worktrees
// synced to origin/<default>, where the local branch of that name is stale or
// missing, so such a script fails for correct work or passes for wrong work.
// Unlike LintVerificationScript this is a hard gate, not a warning. Returns ""
// when the script is acceptable.
func StaleBaseRefIssue(script string) string {
	for _, re := range staleBaseRefPatterns {
		for _, m := range re.FindAllStringSubmatchIndex(script, -1) {
			start, ref := m[2], script[m[2]:m[3]]
			prefix := script[:start]
			if strings.HasSuffix(prefix, "origin/") {
				continue
			}
			if start > 0 {
				// Part of a longer ref (upstream/main, feature-main, v1.main): not bare.
				if c := prefix[start-1]; c == '/' || c == '-' || c == '.' || c == '_' ||
					(c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
					continue
				}
			}
			return "verification script diffs against the bare local `" + ref + "` ref; use `origin/" + ref +
				"` — candidate worktrees are synced to origin/<default>, where the local `" + ref + "` is stale or missing"
		}
	}
	return ""
}

// LintVerificationScript returns human-readable descriptions of false-green
// anti-patterns found in a verification script. An empty result means none of
// the known anti-patterns matched — not a guarantee of soundness.
func LintVerificationScript(script string) []string {
	if strings.TrimSpace(script) == "" {
		return nil
	}
	var issues []string
	for _, p := range falseGreenPatterns {
		if p.re.MatchString(script) {
			issues = append(issues, p.name)
		}
	}
	return issues
}

// LintPlanVerification lints every verification script in a plan and returns
// warnings keyed by the owning story/task id, so a caller can surface them for
// human review before approval.
func LintPlanVerification(plan *ProjectPlan) map[string][]string {
	if plan == nil {
		return nil
	}
	out := map[string][]string{}
	for _, st := range plan.Stories {
		if issues := LintVerificationScript(st.VerificationScript); len(issues) > 0 {
			out[st.ID] = issues
		}
		for _, tk := range st.Tasks {
			if issues := LintVerificationScript(tk.VerificationScript); len(issues) > 0 {
				out[tk.ID] = issues
			}
		}
	}
	return out
}
