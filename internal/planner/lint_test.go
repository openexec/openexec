package planner

import (
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
	}
	for _, s := range sound {
		if issues := LintVerificationScript(s); len(issues) != 0 {
			t.Errorf("sound script %q flagged: %v", s, issues)
		}
	}
}

func TestStaleBaseRefIssue(t *testing.T) {
	stale := []string{
		`git diff --name-only main...HEAD -- x | grep -vc y`,
		`git diff --name-only master..HEAD`,
		`git log --oneline main..HEAD | wc -l`,
		`git diff --name-only main -- internal/`,
		`git merge-base main HEAD`,
		`test "$(git diff --name-only main...HEAD | wc -l)" -eq 1`,
	}
	for _, s := range stale {
		issue := StaleBaseRefIssue(s)
		if issue == "" {
			t.Errorf("expected a stale-base issue for %q", s)
		} else if !strings.Contains(issue, "origin/") {
			t.Errorf("issue for %q must name the origin/ fix: %s", s, issue)
		}
	}

	sound := []string{
		`git diff --name-only origin/main...HEAD -- x | grep -vc y`,
		`git diff --name-only origin/master..HEAD`,
		`git merge-base origin/main HEAD`,
		`git diff --name-only upstream/main...HEAD`,
		`git diff --name-only feature-main...HEAD`,
		`go test ./cmd/main/...`,
		`grep -c "func main" cmd/openexec/main.go`,
		``,
	}
	for _, s := range sound {
		if issue := StaleBaseRefIssue(s); issue != "" {
			t.Errorf("sound script %q flagged: %s", s, issue)
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
