package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeCopilotOnPath puts a `copilot` fixture first on PATH. It follows GitHub's
// documented command line (-p <prompt> -s --no-ask-user [--model m]
// [--allow-tool=write]) and records its arguments and credential environment.
func fakeCopilotOnPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >\"" + dir + "/args\"\n" +
		"printf '%s|%s|%s|%s\\n' \"$OPENAI_API_KEY\" \"$ANTHROPIC_API_KEY\" \"$COPILOT_GITHUB_TOKEN\" \"$GH_TOKEN\" >\"" + dir + "/env\"\n" +
		"echo done\n"
	if err := os.WriteFile(filepath.Join(dir, "copilot"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestBuildCommandCopilot(t *testing.T) {
	fakeCopilotOnPath(t)
	for _, tc := range []struct {
		name, model, mode string
		want              string
		wantNo            string
	}{
		{"default model, read-only", "copilot", "read-only", "-s --no-ask-user", "--allow-tool"},
		{"named model, workspace-write", "copilot:gpt-4.1", "workspace-write", "--model gpt-4.1 --allow-tool=write", ""},
		{"danger grants writes", "copilot", "danger-full-access", "--allow-tool=write", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name, args := buildCommand(Config{ExecutorModel: tc.model, ExecMode: tc.mode, Prompt: "fix it"})
			joined := strings.Join(args, " ")
			if filepath.Base(name) != "copilot" || args[0] != "-p" || !strings.Contains(args[1], "fix it") {
				t.Fatalf("command = %s %q", name, args)
			}
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("args %q lack %q", joined, tc.want)
			}
			if tc.wantNo != "" && strings.Contains(joined, tc.wantNo) {
				t.Fatalf("args %q must not contain %q", joined, tc.wantNo)
			}
		})
	}
}

// Near miss: a model that merely mentions copilot elsewhere is not routed to it.
func TestBuildCommandNotCopilotForOtherModels(t *testing.T) {
	fakeCopilotOnPath(t)
	for _, m := range []string{"gpt-5.4", "not-copilot", "copilotx"} {
		name, args := buildCommand(Config{ExecutorModel: m, Prompt: "p"})
		if filepath.Base(name) == "copilot" || (len(args) > 0 && args[0] == "-p" && filepath.Base(name) != "claude") {
			t.Fatalf("%s routed to %s %q", m, name, args)
		}
	}
}

func TestStartProcessCopilotScopesCredentials(t *testing.T) {
	dir := fakeCopilotOnPath(t)
	t.Setenv("OPENAI_API_KEY", "sk-must-not-reach-github")
	t.Setenv("ANTHROPIC_API_KEY", "ant-must-not-reach-github")
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
	t.Setenv("GH_TOKEN", "")

	p, err := StartProcess(context.Background(), Config{ExecutorModel: "copilot", ExecMode: "read-only", Prompt: "hi", WorkDir: t.TempDir()}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = p.cmd.Wait()
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if strings.TrimSpace(string(env)) != "||gho_fixture|" {
		t.Fatalf("copilot saw env %q: want its GitHub token only", env)
	}
}

func TestStartProcessCopilotRefusesClassicPAT(t *testing.T) {
	dir := fakeCopilotOnPath(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "ghp_classic")
	_, err := StartProcess(context.Background(), Config{ExecutorModel: "copilot", Prompt: "hi", WorkDir: t.TempDir()}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "GH_TOKEN") || strings.Contains(err.Error(), "ghp_classic") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "args")); statErr == nil {
		t.Fatal("copilot started with a classic PAT")
	}
}

// The init-selected model drives the real Loop: plain `-s` output must become
// assistant text events instead of being dropped by the stream-json parser.
func TestLoopRunCopilotPlainTextReachesEvents(t *testing.T) {
	fakeCopilotOnPath(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
	l, events := New(Config{ExecutorModel: "copilot", ExecMode: "read-only", Prompt: "hi", WorkDir: t.TempDir()})
	if err := l.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for e := range events {
		if e.Type == EventAssistantText {
			texts = append(texts, e.Text)
		}
	}
	if len(texts) != 1 || texts[0] != "done" {
		t.Fatalf("assistant texts = %q, want [done]", texts)
	}
}

// Near miss: the stream-json parser (every other CLI) still skips plain text.
func TestDefaultParserStillSkipsPlainText(t *testing.T) {
	ch := make(chan Event, 4)
	if err := NewParser(ch, 1).Parse(strings.NewReader("done\n")); err != nil {
		t.Fatal(err)
	}
	close(ch)
	for e := range ch {
		t.Fatalf("unexpected event %+v", e)
	}
}

// A missing copilot binary is reported as such, never replaced by claude.
func TestStartProcessCopilotMissingBinaryDoesNotFallBackToClaude(t *testing.T) {
	dir := t.TempDir()
	// A claude on PATH would be the fallback target.
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\necho claude-ran >"+dir+"/ran\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := StartProcess(context.Background(), Config{ExecutorModel: "copilot", Prompt: "hi", WorkDir: t.TempDir()}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "copilot") {
		t.Fatalf("err = %v, want copilot-not-found", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "ran")); statErr == nil {
		t.Fatal("fell back to claude")
	}
}
