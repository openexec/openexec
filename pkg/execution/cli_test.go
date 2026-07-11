package execution

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestAgentCLIExecutorRunsThroughCoreBoundary(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "prompt.txt")
	script := filepath.Join(dir, "fake-codex")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > \"$CAPTURE\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAPTURE", capture)
	executor := &AgentCLIExecutor{OverrideCommand: script}
	result, err := executor.Execute(context.Background(), Request{
		ID: "C-1", WorkingDir: dir, Prompt: "authorized work", Model: "gpt-test",
		Sandbox: Sandbox{Mode: "danger-full-access", Isolation: "git-worktree"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSucceeded || result.Executor != script {
		t.Fatalf("result = %+v", result)
	}
	b, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "authorized work" {
		t.Fatalf("captured prompt = %q", b)
	}
}

func TestAgentCLIExecutorRejectsUnsupportedSandbox(t *testing.T) {
	executor := &AgentCLIExecutor{}
	result, err := executor.Execute(context.Background(), Request{WorkingDir: t.TempDir(), Prompt: "x", Sandbox: Sandbox{Mode: "workspace-write"}})
	if err == nil {
		t.Fatal("expected unsupported sandbox refusal")
	}
	if result.Outcome != OutcomeFailed {
		t.Fatalf("result = %+v", result)
	}
}
