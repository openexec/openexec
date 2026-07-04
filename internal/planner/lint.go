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
