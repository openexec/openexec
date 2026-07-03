package planner

import "testing"

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
		`go test ./internal/governance/...`,
		`grep -q "MAX_BYTES = 40" route.ts`,
		``,
	}
	for _, s := range sound {
		if issues := LintVerificationScript(s); len(issues) != 0 {
			t.Errorf("sound script %q flagged: %v", s, issues)
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
