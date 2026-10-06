package planner

import (
	"reflect"
	"strings"
	"testing"
)

func TestLintVerificationScript(t *testing.T) {
	falseGreen := []string{
		`npx vitest run x 2>/dev/null || npm test -- y`,
		`go test ./... || echo ok`,
		`grep -q "MAX_BYTES" route.ts | head -1`,
		`test -f a && grep foo a || true`,
		`cat x 2>/dev/null`,
		`grep -q x a || { echo missing; false; true; }`,
		`go test ./... || exit 0`,
		`grep -q x a || false || true`,
		`test -f a && grep foo a || { echo no; }`,
	}
	for _, s := range falseGreen {
		if issues := LintVerificationScript(s); len(issues) == 0 {
			t.Errorf("expected a false-green warning for %q", s)
		}
	}

	sound := []string{
		`npx vitest run upload.test.ts`,
		`go test ./internal/foo/...`,
		`grep -q "MAX_BYTES = 40" route.ts`,
		``,
		// A fallback that fails the script reports the failure; it masks nothing.
		// Voice-control Goal 4011a347's plan was refused for the first one.
		`f="$HOME/observe.env"; for k in A B; do grep -q "^$k=" "$f" || { echo "missing $k"; exit 1; }; done`,
		`go test ./internal/x || exit 1`,
		`grep -q foo a || false`,
		"grep -q foo a || exit 2\necho ok",
		`test -f a && grep -q foo a || { exit 1; }`,
		`(grep -q foo a || exit 1)`,
	}
	for _, s := range sound {
		if issues := LintVerificationScript(s); len(issues) != 0 {
			t.Errorf("sound script %q flagged: %v", s, issues)
		}
	}
}

func TestStaleBaseRefIssue(t *testing.T) {
	// Templates use REF for the default-branch name; each runs for main and master.
	stale := []string{
		// Acceptance criteria (US-012).
		`git diff --name-only REF...HEAD -- x | grep -vc y`,
		`git log REF..HEAD`,
		`git diff 'REF' HEAD`,
		`git diff "REF...HEAD"`,
		`git diff 'REF..HEAD'`,
		`git merge-base REF HEAD`,
		`git diff origin/REF...HEAD && git diff REF...HEAD`,
		// Further revision positions and shell contexts.
		`git diff --name-only REF..HEAD`,
		`git log --oneline REF..HEAD | wc -l`,
		`git diff --name-only REF -- internal/`,
		`test "$(git diff --name-only REF...HEAD | wc -l)" -eq 1`,
		"test -z \"`git diff REF...HEAD`\"",
		`(cd sub && git diff REF...HEAD)`,
		`git -C sub -c core.pager=cat diff REF...HEAD`,
		`git --no-pager log -n 5 REF..HEAD`,
		`git rev-list --count ^REF HEAD`,
		`git rev-parse --verify REF~1`,
		`git show REF:README.md`,
		`git diff REF@{u}...HEAD`,
		`git cherry REF`,
		`git range-diff REF...HEAD`,
		`LC_ALL=C git diff REF...HEAD`,
		`! git diff --quiet REF...HEAD`,
		`git diff REF...HEAD 2>&1 | grep -q x`,
		"# a comment\ngit diff REF...HEAD",
		"git diff \\\n  REF...HEAD",
	}
	sound := []string{
		// Acceptance criteria (US-012).
		`git diff --exit-code origin/REF...HEAD -- REF`,
		`grep -Fq 'REF...HEAD' docs/ARCHITECTURE.md`,
		"# never use REF...HEAD\ngit diff --exit-code origin/REF...HEAD -- internal/",
		`git diff upstream/REF...HEAD`,
		`git diff feature-REF..HEAD`,
		// Further non-revision mentions.
		`git diff --name-only origin/REF...HEAD -- x | grep -vc y`,
		`git diff --name-only origin/REF..HEAD`,
		`git merge-base origin/REF HEAD`,
		`git diff HEAD -- REF/`,
		`git log --grep REF origin/REF..HEAD`,
		`git checkout REF`,
		`echo "git diff REF...HEAD"`,
		`echo git diff REF...HEAD`,
		`git diff origin/REF...HEAD # not REF...HEAD`,
		`git diff origin/REF...HEAD > REF`,
		`go test ./cmd/REF/...`,
		`grep -c "func REF" cmd/openexec/REF.go`,
		``,
	}
	for _, ref := range []string{"main", "master"} {
		for _, tmpl := range stale {
			s := strings.ReplaceAll(tmpl, "REF", ref)
			issue := StaleBaseRefIssue(s)
			if issue == "" {
				t.Errorf("expected a stale-base issue for %q", s)
			} else if !strings.Contains(issue, "`"+ref+"`") || !strings.Contains(issue, "`origin/"+ref+"`") {
				t.Errorf("issue for %q must name %s and the origin/%s fix: %s", s, ref, ref, issue)
			}
		}
		for _, tmpl := range sound {
			s := strings.ReplaceAll(tmpl, "REF", ref)
			if issue := StaleBaseRefIssue(s); issue != "" {
				t.Errorf("sound script %q flagged: %s", s, issue)
			}
		}
	}
}

func TestShellCommands(t *testing.T) {
	cases := []struct {
		script string
		want   [][]string
	}{
		{`a 'b c' "d e" f\ g`, [][]string{{"a", "b c", "d e", "f g"}}},
		{`a;b&&c||d|e&f`, [][]string{{"a"}, {"b"}, {"c"}, {"d"}, {"e"}, {"f"}}},
		{"a # b c\nd", [][]string{{"a"}, {"d"}}},
		{`a#b`, [][]string{{"a#b"}}},
		// The quoted word around a substitution splits into empty halves; only
		// the substitution's own argv matters.
		{`x "$(y 'z')" w`, [][]string{{"x", ""}, {"y", "z"}, {"", "w"}}},
		{"x `y z` w", [][]string{{"x"}, {"y", "z"}, {"w"}}},
		{`a '' b`, [][]string{{"a", "", "b"}}},
		{`a 2>&1 > out b`, [][]string{{"a", "b"}}},
		{`"a\"b\$c\d"`, [][]string{{`a"b$c\d`}}},
	}
	for _, c := range cases {
		if got := shellCommands(c.script); !reflect.DeepEqual(got, c.want) {
			t.Errorf("shellCommands(%q) = %q, want %q", c.script, got, c.want)
		}
	}
}

func TestRevisionEndpoints(t *testing.T) {
	cases := map[string][]string{
		"main...HEAD":      {"main", "HEAD"},
		"main..HEAD":       {"main", "HEAD"},
		"^main":            {"main"},
		"main~2":           {"main"},
		"main^{commit}":    {"main"},
		"master@{u}..HEAD": {"master", "HEAD"},
		"main:path/x.go":   {"main"},
		"origin/main":      {"origin/main"},
	}
	for rev, want := range cases {
		if got := revisionEndpoints(rev); !reflect.DeepEqual(got, want) {
			t.Errorf("revisionEndpoints(%q) = %q, want %q", rev, got, want)
		}
	}
}

func TestLintPlanVerification(t *testing.T) {
	plan := &ProjectPlan{
		Stories: []Story{{
			ID: "US-001", VerificationScript: "go test ./... || echo ok",
			Tasks: []Task{{ID: "T-US-001-001", VerificationScript: "npx vitest run ok.test.ts"}},
		}},
	}
	warnings := LintPlanVerification(plan)
	if len(warnings["US-001"]) == 0 {
		t.Fatalf("expected the story script to be flagged")
	}
	if _, ok := warnings["T-US-001-001"]; ok {
		t.Fatalf("the sound task script must not be flagged")
	}
}
