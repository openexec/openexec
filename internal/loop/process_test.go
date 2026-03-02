package loop

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestProcessLifecycle(t *testing.T) {
	cfg := Config{
		CommandName: "echo",
		CommandArgs: []string{`{"type":"result"}`},
		WorkDir:     t.TempDir(),
	}

	proc, err := StartProcess(context.Background(), cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	out, err := io.ReadAll(proc.Stdout)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}

	if err := proc.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	if !strings.Contains(string(out), `{"type":"result"}`) {
		t.Errorf("stdout = %q, want JSON output", string(out))
	}
}

func TestProcessNonZeroExit(t *testing.T) {
	cfg := Config{
		CommandName: "sh",
		CommandArgs: []string{"-c", "echo fail && exit 1"},
		WorkDir:     t.TempDir(),
	}

	proc, err := StartProcess(context.Background(), cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	io.ReadAll(proc.Stdout) // drain
	err = proc.Wait()
	if err == nil {
		t.Fatal("expected non-zero exit error, got nil")
	}
}

func TestProcessKill(t *testing.T) {
	cfg := Config{
		CommandName: "sleep",
		CommandArgs: []string{"30"},
		WorkDir:     t.TempDir(),
	}

	proc, err := StartProcess(context.Background(), cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	if err := proc.Kill(); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	err = proc.Wait()
	if err == nil {
		t.Fatal("expected error after kill, got nil")
	}
}

func TestProcessContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	cfg := Config{
		CommandName: "sleep",
		CommandArgs: []string{"30"},
		WorkDir:     t.TempDir(),
	}

	proc, err := StartProcess(ctx, cfg, nil, nil, nil)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}

	err = proc.Wait()
	if err == nil {
		t.Fatal("expected error after context cancel, got nil")
	}
}

func TestBuildCommandDefault(t *testing.T) {
	cfg := Config{Prompt: "do stuff"}
	name, args := buildCommand(cfg)
	if name != "claude" {
		t.Errorf("name = %q, want claude", name)
	}
	if len(args) < 3 {
		t.Fatalf("expected args, got %v", args)
	}

	joined := strings.Join(args, " ")

	// Prompt should contain both the user prompt and the autonomous preamble.
	if !strings.Contains(joined, "do stuff") {
		t.Error("args missing user prompt")
	}
	if !strings.Contains(joined, "non-interactive pipeline") {
		t.Error("args missing autonomous preamble")
	}
	if !strings.Contains(joined, "--max-turns") {
		t.Error("args missing --max-turns")
	}
	// Interactive tools should be disallowed.
	if !strings.Contains(joined, "--disallowedTools") {
		t.Error("args missing --disallowedTools")
	}
	if !strings.Contains(joined, "EnterPlanMode") {
		t.Error("disallowedTools missing EnterPlanMode")
	}
	if !strings.Contains(joined, "AskUserQuestion") {
		t.Error("disallowedTools missing AskUserQuestion")
	}
}

func TestBuildCommandOverride(t *testing.T) {
	cfg := Config{
		CommandName: "my-mock",
		CommandArgs: []string{"--flag"},
	}
	name, args := buildCommand(cfg)
	if name != "my-mock" {
		t.Errorf("name = %q", name)
	}
	if len(args) != 1 || args[0] != "--flag" {
		t.Errorf("args = %v", args)
	}
}
