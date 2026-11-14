package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/loop"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/runner"
)

func TestLoopRunnerCopilotLeavesInvocationToLoop(t *testing.T) {
	for _, tc := range []struct {
		name, model, override, cmd string
		wantEmpty                  bool
	}{
		{"default copilot", "copilot", "", "/bin/copilot", true},
		{"named copilot model", "copilot:gpt-4.1", "", "/bin/copilot", true},
		{"claude", "sonnet", "", "/bin/claude", true},
		{"explicit override wins", "copilot", "/opt/my-runner", "/opt/my-runner", false},
		{"near miss copilotx", "copilotx", "", "/bin/codex", false},
		{"gemini passes through", "gemini-pro", "", "/bin/gemini", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, args := loopRunner(tc.model, tc.override, tc.cmd, []string{"-s"})
			if tc.wantEmpty != (got == "" && args == nil) {
				t.Fatalf("loopRunner = %q %q", got, args)
			}
		})
	}
}

// Configuration as `openexec init --executor copilot` stores it travels the
// server's runner resolution into the real loop: the fixture copilot must get
// the prompt via -p, and its plain text must arrive as an assistant event.
func TestServerCopilotConfigRoundTripReachesLoop(t *testing.T) {
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >\"" + bin + "/args\"\n" +
		"printf '%s' \"$OPENAI_API_KEY\" >\"" + bin + "/openai\"\necho done\n"
	if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("OPENAI_API_KEY", "sk-must-not-reach-github")
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")

	proj := t.TempDir()
	cfg := project.ProjectConfig{Execution: project.ExecutionConfig{ExecutorModel: "copilot"}}
	data, _ := json.Marshal(cfg)
	if err := os.MkdirAll(filepath.Join(proj, ".openexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".openexec", "config.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	pc, err := project.LoadProjectConfig(proj)
	if err != nil {
		t.Fatal(err)
	}
	rc, ra, err := runner.Resolve(pc.Execution.ExecutorModel, pc.Execution.RunnerCommand, pc.Execution.RunnerArgs)
	if err != nil {
		t.Fatal(err)
	}
	cmd, cargs := loopRunner(pc.Execution.ExecutorModel, pc.Execution.RunnerCommand, rc, ra)

	l, events := loop.New(loop.Config{
		ExecutorModel: pc.Execution.ExecutorModel, CommandName: cmd, CommandArgs: cargs,
		ExecMode: "read-only", Prompt: "do the thing", WorkDir: proj,
	})
	if err := l.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var texts []string
	for e := range events {
		if e.Type == loop.EventAssistantText {
			texts = append(texts, e.Text)
		}
	}
	if len(texts) != 1 || texts[0] != "done" {
		t.Fatalf("assistant texts = %q, want [done]", texts)
	}
	args, _ := os.ReadFile(filepath.Join(bin, "args"))
	if !strings.HasPrefix(string(args), "-p ") || !strings.Contains(string(args), "do the thing") {
		t.Fatalf("copilot args = %q, want -p <prompt>", args)
	}
	if k, _ := os.ReadFile(filepath.Join(bin, "openai")); len(k) != 0 {
		t.Fatalf("OPENAI_API_KEY leaked to copilot: %q", k)
	}
}

// A missing executable surfaces through the same route, with no claude fallback.
func TestServerCopilotRouteMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, _, err := runner.Resolve("copilot", "", nil); err == nil {
		t.Fatal("missing copilot must be an error")
	}
	l, events := loop.New(loop.Config{ExecutorModel: "copilot", Prompt: "x", WorkDir: t.TempDir()})
	if err := l.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "copilot") {
		t.Fatalf("err = %v", err)
	}
	for range events {
	}
}
