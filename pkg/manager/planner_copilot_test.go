package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copilotPlannerFixture(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("OPENEXEC_PLANNER_CLI", "")
	t.Setenv("OPENEXEC_PLANNER_ARGS", "")
	for _, v := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
		t.Setenv(v, "")
	}
	return bin
}

func TestPlannerCopilotScopesCredentialsAndPassesPrompt(t *testing.T) {
	bin := copilotPlannerFixture(t, "printf '%s' \"$*\" >\"$0.args\"\nprintf '%s|%s|%s' \"$OPENAI_API_KEY\" \"$ANTHROPIC_API_KEY\" \"$GEMINI_API_KEY\" >\"$0.keys\"\nprintf '%s' \"$COPILOT_GITHUB_TOKEN\" >\"$0.tok\"\necho plan\n")
	t.Setenv("OPENAI_API_KEY", "sk-o")
	t.Setenv("ANTHROPIC_API_KEY", "sk-a")
	t.Setenv("GEMINI_API_KEY", "g-key")
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_ok")
	out, err := (&cliLLMProvider{model: "copilot"}).Complete(context.Background(), "plan this")
	if err != nil || strings.TrimSpace(out) != "plan" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	bin += "/copilot"
	if a, _ := os.ReadFile(bin + ".args"); !strings.HasPrefix(string(a), "-p plan this") {
		t.Fatalf("args = %q", a)
	}
	if k, _ := os.ReadFile(bin + ".keys"); string(k) != "||" {
		t.Fatalf("vendor keys reached copilot: %q", k)
	}
	if tok, _ := os.ReadFile(bin + ".tok"); string(tok) != "gho_ok" {
		t.Fatalf("copilot's own token dropped: %q", tok)
	}
}

func TestPlannerCopilotRefusesClassicPATBeforeLaunch(t *testing.T) {
	bin := copilotPlannerFixture(t, "touch \"$0.ran\"\necho plan\n")
	t.Setenv("GITHUB_TOKEN", "ghp_classic")
	_, err := (&cliLLMProvider{model: "copilot"}).Complete(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "classic personal access token") || strings.Contains(err.Error(), "ghp_classic") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(bin + "/copilot.ran"); statErr == nil {
		t.Fatal("copilot launched with a classic PAT")
	}
	// Near miss: a fine-grained token passes.
	t.Setenv("GITHUB_TOKEN", "github_pat_ok")
	if _, err := (&cliLLMProvider{model: "copilot"}).Complete(context.Background(), "x"); err != nil {
		t.Fatalf("fine-grained token refused: %v", err)
	}
}
