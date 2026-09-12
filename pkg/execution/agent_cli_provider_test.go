package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgentCLIProviderCodexContract(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	script := writeProviderScript(t, dir, "codex", `printf '%s\n' "$*" >"${0%/*}/args"
printf '%s\n' '{"type":"thread.started","thread_id":"native-1"}'
printf '%s\n' '{"type":"item.completed","thread_id":"native-1","item":{"type":"agent_message","text":"done"}}'`)
	provider, err := NewAgentCLIProvider(AgentCLIConfig{Kind: "codex", Binary: script, SearchPath: dir, Models: []string{"default"}})
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	result, err := provider.Execute(context.Background(), Request{
		ID: "request-1", WorkingDir: dir, Prompt: "work", Model: "default",
		Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{dir}, NetworkAccess: true,
	}, func(event Event) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSucceeded || result.NativeSessionID != "native-1" || result.FinalText != "done" {
		t.Fatalf("result = %+v", result)
	}
	args, _ := os.ReadFile(capture)
	if !strings.Contains(string(args), "--sandbox workspace-write") ||
		!strings.Contains(string(args), "sandbox_workspace_write.writable_roots") ||
		!strings.Contains(string(args), "sandbox_workspace_write.network_access=true") {
		t.Fatalf("arguments do not enforce workspace-write roots and network policy: %s", args)
	}
	if len(events) != 3 || events[0].Type != EventStarted || events[1].Type != EventAssistantDelta || events[2].Type != EventCompleted {
		t.Fatalf("events = %#v", events)
	}
}

func TestAgentCLIProviderAdvertisesOnlyEnforceableCommandNetworking(t *testing.T) {
	for _, test := range []struct {
		kind string
		want bool
	}{
		{kind: "codex", want: true},
		{kind: "claude", want: false},
	} {
		provider, err := NewAgentCLIProvider(AgentCLIConfig{Kind: test.kind})
		if err != nil {
			t.Fatal(err)
		}
		if got := provider.Descriptor().Capabilities.CommandNetwork; got != test.want {
			t.Errorf("%s command network capability = %v, want %v", test.kind, got, test.want)
		}
	}
}

func TestAgentCLIProviderClaudeResumeIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	capture := filepath.Join(dir, "args")
	script := writeProviderScript(t, dir, "claude", `printf '%s\n' "$*" >"${0%/*}/args"
printf '%s\n' '{"type":"assistant","session_id":"native-2","message":{"content":[{"type":"text","text":"answer"}]}}'`)
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: "claude", Binary: script, SearchPath: dir})
	result, err := provider.Execute(context.Background(), Request{
		ID: "request-2", WorkingDir: dir, Prompt: "question", Model: "default",
		NativeSessionID: "existing", Sandbox: Sandbox{Mode: "read-only"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.NativeSessionID != "native-2" || result.FinalText != "answer" {
		t.Fatalf("result = %+v", result)
	}
	args, _ := os.ReadFile(capture)
	if !strings.Contains(string(args), "--resume existing") || !strings.Contains(string(args), "--permission-mode plan") ||
		strings.Contains(string(args), "--dangerously-skip-permissions") {
		t.Fatalf("resume arguments violate read-only contract: %s", args)
	}
}

func TestAgentCLIProviderRejectsUnenforceableRequests(t *testing.T) {
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: "codex", Binary: "codex"})
	for _, request := range []Request{
		{ID: "1", WorkingDir: t.TempDir(), Prompt: "x", Sandbox: Sandbox{Mode: "danger-full-access"}},
		{ID: "2", WorkingDir: t.TempDir(), Prompt: "x", Sandbox: Sandbox{Mode: "read-only"}, WritableRoots: []string{"/tmp"}},
		{ID: "3", WorkingDir: t.TempDir(), Prompt: "x", Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{"relative"}},
	} {
		if _, err := provider.Execute(context.Background(), request, nil); err == nil {
			t.Fatalf("request was accepted: %+v", request)
		}
	}
}

func TestAgentCLIProviderCancellation(t *testing.T) {
	dir := t.TempDir()
	script := writeProviderScript(t, dir, "codex", "exec /bin/sleep 30")
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: "codex", Binary: script, SearchPath: dir})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	var cancelled bool
	result, err := provider.Execute(ctx, Request{
		ID: "request-4", WorkingDir: dir, Prompt: "wait",
		Sandbox: Sandbox{Mode: "read-only"},
	}, func(event Event) error {
		cancelled = cancelled || event.Type == EventCancelled
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) || result.Outcome != OutcomeCancelled || !cancelled {
		t.Fatalf("result=%+v err=%v cancelled=%v", result, err, cancelled)
	}
}

func TestAgentCLIProviderProbeClassifiesLogin(t *testing.T) {
	dir := t.TempDir()
	script := writeProviderScript(t, dir, "codex", `echo "please log in" >&2; exit 1`)
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: "codex", Binary: script, SearchPath: dir})
	readiness := provider.Probe(context.Background(), dir)
	if readiness.State != ReadinessNeedsLogin {
		t.Fatalf("readiness = %+v", readiness)
	}
}

func TestAgentCLIReadinessUsesStatusNotInference(t *testing.T) {
	for kind, arguments := range map[string]string{"claude": "auth status --json", "codex": "login status"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			script := writeProviderScript(t, dir, kind, `printf '%s' "$*" >"${0%/*}/arguments"; printf '%s' '{"loggedIn":true}'`)
			provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: kind, Binary: script, SearchPath: dir})
			got := provider.Probe(context.Background(), dir)
			raw, _ := os.ReadFile(filepath.Join(dir, "arguments"))
			if string(raw) != arguments || got.State != ReadinessReady || got.Check != "authentication-status" || !provider.Descriptor().Capabilities.NonInferenceReadiness {
				t.Fatalf("unsafe readiness: args=%q state=%+v", raw, got)
			}
		})
	}
}

func TestAgentCLIReadinessUnknownAndSignedOut(t *testing.T) {
	for _, tc := range []struct {
		output, state string
		exit          int
	}{
		{`{"loggedIn":false}`, ReadinessNeedsLogin, 0},
		{"unknown command auth: fixture-secret", ReadinessUnknown, 1},
		{"unexpected fixture-secret output", ReadinessUnknown, 0},
	} {
		dir := t.TempDir()
		script := writeProviderScript(t, dir, "claude", fmt.Sprintf("printf '%%s' '%s'; exit %d", tc.output, tc.exit))
		provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: "claude", Binary: script, SearchPath: dir})
		got := provider.Probe(context.Background(), dir)
		if got.State != tc.state || strings.Contains(got.Problem, "fixture-secret") {
			t.Fatalf("unsafe readiness: %+v", got)
		}
	}
}

func writeProviderScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
