package ai

import (
	"context"
	"testing"
)

func TestAnalyzeImpact_ParsesAndNormalizes(t *testing.T) {
	const reply = "```yaml\n" + `files:
  - path: src/components/LoginForm.tsx
    action: MODIFY
    reason: add empty-password validation
  - path: src/api/auth.ts
    action: create
    reason: new validation helper
  - path: ""
    action: modify
    reason: dropped (empty path)
  - path: src/x.ts
    action: nonsense
    reason: action normalized to modify
notes: backend schema path still unknown
` + "```"
	rep, err := AnalyzeImpact(context.Background(), fixedCompleter{reply}, "add validation", "excerpt")
	if err != nil {
		t.Fatalf("AnalyzeImpact: %v", err)
	}
	if len(rep.Files) != 3 {
		t.Fatalf("expected 3 files (empty-path dropped), got %d: %+v", len(rep.Files), rep.Files)
	}
	if rep.Files[0].Action != "modify" || rep.Files[1].Action != "create" {
		t.Fatalf("action normalization wrong: %+v", rep.Files)
	}
	if rep.Files[2].Action != "modify" {
		t.Fatalf("unknown action should normalize to modify, got %q", rep.Files[2].Action)
	}
	if rep.Notes == "" {
		t.Fatalf("expected notes preserved")
	}
}

// fixedCompleter returns a canned reply for any prompt.
type fixedCompleter struct{ reply string }

func (f fixedCompleter) Complete(_ context.Context, _ string) (string, error) { return f.reply, nil }
