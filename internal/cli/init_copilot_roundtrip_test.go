package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/agent"
)

// One journey: `openexec init --executor copilot` persists the config, the
// persisted model is resolved through the registered provider factory and
// model lookup, and the adapter drives a fixture copilot binary. The fixture
// follows GitHub's documented command line (-p <prompt> -s); it does not
// reproduce the real CLI's output.
func TestInitCopilotConfigThroughFactoryToCLI(t *testing.T) {
	t.Cleanup(func() {
		initExecutorModel, initNonInteractive = "sonnet", false
		for _, n := range []string{"executor", "yes"} {
			initCmd.Flags().Lookup(n).Changed = false
		}
	})
	proj := t.TempDir()
	runGit(t, proj, "init")
	runGit(t, proj, "config", "user.name", "Test User")
	runGit(t, proj, "config", "user.email", "test@example.com")
	runGit(t, proj, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(proj, "README.md"), []byte("# t"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, proj, "add", "README.md")
	runGit(t, proj, "commit", "-m", "Initial")
	oldCwd, _ := os.Getwd()
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCwd)

	out := bytes.NewBufferString("")
	rootCmd.SetOut(out)
	rootCmd.SetArgs([]string{"init", "p", "-y", "--executor", "copilot:gpt-4.1"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	cfg, err := project.LoadProjectConfig(proj)
	if err != nil {
		t.Fatal(err)
	}
	model := cfg.Execution.ExecutorModel
	if model != "copilot:gpt-4.1" {
		t.Fatalf("persisted executor model = %q", model)
	}

	bin := t.TempDir()
	install := func(body string) {
		if err := os.WriteFile(filepath.Join(bin, "copilot"), []byte("#!/bin/sh\n"+body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	complete := func(t *testing.T) (*agent.Response, error) {
		reg := agent.NewProviderRegistry()
		agent.RegisterDefaultFactories(reg)
		p, err := reg.GetForModel(model)
		if err != nil {
			return nil, err
		}
		if p.GetName() != "github-copilot" {
			t.Fatalf("model %q resolved to %s", model, p.GetName())
		}
		return p.Complete(context.Background(), agent.Request{
			Model:    model,
			Messages: []agent.Message{agent.NewTextMessage(agent.RoleUser, "say hi")},
		})
	}
	clean := func(t *testing.T) {
		t.Setenv("PATH", bin+string(os.PathListSeparator)+"/usr/bin:/bin")
		for _, v := range []string{"COPILOT_GITHUB_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"} {
			t.Setenv(v, "")
		}
	}

	t.Run("happy path", func(t *testing.T) {
		clean(t)
		t.Setenv("OPENAI_API_KEY", "sk-must-not-reach-github")
		t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
		install("printf '%s' \"$*\" >\"" + bin + "/args\"\nprintf '%s' \"$OPENAI_API_KEY\" >\"" + bin + "/openai\"\necho hello\n")
		resp, err := complete(t)
		if err != nil {
			t.Fatal(err)
		}
		if got := resp.Content[0].Text; got != "hello" {
			t.Fatalf("answer = %q", got)
		}
		args, _ := os.ReadFile(filepath.Join(bin, "args"))
		if !strings.HasPrefix(string(args), "-p ") || !strings.Contains(string(args), "say hi") || !strings.Contains(string(args), "--model gpt-4.1") {
			t.Fatalf("args = %q", args)
		}
		if k, _ := os.ReadFile(filepath.Join(bin, "openai")); len(k) != 0 {
			t.Fatalf("OPENAI_API_KEY reached the CLI: %q", k)
		}
	})

	t.Run("missing executable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if _, err := complete(t); err == nil || !strings.Contains(err.Error(), "not found on PATH") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("classic PAT refused before launch", func(t *testing.T) {
		clean(t)
		t.Setenv("GH_TOKEN", "ghp_classic")
		install("touch \"" + bin + "/ran\"\necho hello\n")
		_, err := complete(t)
		if err == nil || !strings.Contains(err.Error(), "classic personal access token") || strings.Contains(err.Error(), "ghp_classic") {
			t.Fatalf("err = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(bin, "ran")); statErr == nil {
			t.Fatal("CLI launched with a classic PAT")
		}
	})

	t.Run("nonzero exit carries stderr and re-login hint, ignores OpenAI key", func(t *testing.T) {
		clean(t)
		t.Setenv("OPENAI_API_KEY", "sk-must-not-rescue-auth")
		install("echo 'fixture-cli-refused' >&2\nexit 1\n")
		_, err := complete(t)
		if err == nil || !strings.Contains(err.Error(), "fixture-cli-refused") || !strings.Contains(err.Error(), "/login") {
			t.Fatalf("err = %v", err)
		}
	})
}
