package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

// The Go build cache a caller configured must reach the agent CLI. These run
// the real go command inside a provider-launched helper, so the evidence is
// what Go itself resolves and writes, not the string the filter emitted.
func TestAgentCLIProviderCarriesConfiguredGoCache(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		t.Run(kind, func(t *testing.T) {
			fixture := newGoCacheFixture(t, kind, true)
			cache := filepath.Join(fixture.repo, ".cache", "go-build")
			t.Setenv("GOCACHE", cache)
			fixture.execute(t, Request{
				ID: "gocache-" + kind, WorkingDir: fixture.repo, Prompt: "build", Model: "default",
				Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{fixture.repo},
			})
			if got := fixture.read(t, "gocache"); got != cache {
				t.Fatalf("agent go env GOCACHE = %q, want configured %q", got, cache)
			}
			if _, err := os.Stat(filepath.Join(cache, "README")); err != nil {
				t.Fatalf("agent go build did not use the configured cache: %v", err)
			}
			if rel, err := filepath.Rel(fixture.repo, cache); err != nil || strings.HasPrefix(rel, "..") {
				t.Fatalf("configured cache %q is outside the granted writable root %q", cache, fixture.repo)
			}
			if _, err := os.Stat(fixture.defaultCache); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("agent wrote the default cache %q: %v", fixture.defaultCache, err)
			}
			args := fixture.read(t, "args")
			if kind == "claude" && (strings.Count(args, "--add-dir") != 1 || !strings.Contains(args, "--add-dir "+fixture.repo+" ")) ||
				kind == "codex" && !strings.Contains(args, `sandbox_workspace_write.writable_roots=["`+fixture.repo+`"]`) {
				t.Fatalf("writable roots changed: %s", args)
			}
			fixture.assertFiltered(t, "GOCACHE="+cache)
		})
	}
}

// Without a usable configured cache the agent keeps Go's documented default.
// A relative GOCACHE is one Go itself refuses, so it is dropped, not passed on.
func TestAgentCLIProviderKeepsGoDefaultWithoutConfiguredCache(t *testing.T) {
	for _, kind := range []string{"claude", "codex"} {
		for _, configured := range []string{"", "relative/go-build"} {
			name := "absent"
			if configured != "" {
				name = "relative"
			}
			t.Run(kind+"/"+name, func(t *testing.T) {
				fixture := newGoCacheFixture(t, kind, false)
				t.Setenv("GOCACHE", configured)
				if configured == "" {
					if err := os.Unsetenv("GOCACHE"); err != nil {
						t.Fatal(err)
					}
				}
				fixture.execute(t, Request{
					ID: "gocache-default-" + kind, WorkingDir: fixture.repo, Prompt: "inspect", Model: "default",
					Sandbox: Sandbox{Mode: "read-only"},
				})
				if got := fixture.read(t, "gocache"); got != fixture.defaultCache {
					t.Fatalf("agent go env GOCACHE = %q, want Go's default %q", got, fixture.defaultCache)
				}
				if args := fixture.read(t, "args"); strings.Contains(args, "--add-dir") || strings.Contains(args, "writable_roots") {
					t.Fatalf("read-only request gained writable roots: %s", args)
				}
				fixture.assertFiltered(t, "")
				if strings.Contains(fixture.read(t, "env"), "GOCACHE=") {
					t.Fatal("unconfigured GOCACHE reached the agent")
				}
			})
		}
	}
}

type goCacheFixture struct {
	kind, dir, repo, home, searchPath, defaultCache string
}

const goCacheFixtureSecret = "fixture-credential-sentinel"

// newGoCacheFixture isolates everything Go reads: HOME and XDG_CONFIG_HOME
// point into a temp tree, so neither the owner's caches nor their go env file
// can leak in. A credential-like sentinel is planted to prove it stays out.
func newGoCacheFixture(t *testing.T, kind string, build bool) goCacheFixture {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		if _, statErr := os.Stat(goBin); statErr != nil {
			t.Skipf("go command unavailable: %v", err)
		}
	}
	root := t.TempDir()
	f := goCacheFixture{kind: kind, dir: filepath.Join(root, "bin"), repo: filepath.Join(root, "repo"), home: filepath.Join(root, "home")}
	for _, dir := range []string{f.dir, filepath.Join(f.repo, "lib"), f.home} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		filepath.Join(f.repo, "go.mod"):        "module fixture\n\ngo 1.21\n",
		filepath.Join(f.repo, "lib", "lib.go"): "package lib\n\nfunc Answer() int { return 42 }\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", f.home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(f.home, ".config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "outer-cache"))
	t.Setenv("OPENEXEC_FIXTURE_TOKEN", goCacheFixtureSecret)
	t.Setenv("GITHUB_TOKEN", goCacheFixtureSecret)
	f.searchPath = f.dir + string(os.PathListSeparator) + filepath.Dir(goBin)

	// Go's documented default, as Go computes it for the agent's environment.
	probe := exec.Command(goBin, "env", "GOCACHE")
	probe.Dir, probe.Env = f.repo, []string{"PATH=" + f.searchPath, "HOME=" + f.home, "XDG_CONFIG_HOME=" + filepath.Join(f.home, ".config")}
	out, err := probe.Output()
	if err != nil {
		t.Fatalf("go env GOCACHE: %v", err)
	}
	f.defaultCache = strings.TrimSpace(string(out))
	if !strings.HasPrefix(f.defaultCache, f.home+string(filepath.Separator)) {
		t.Fatalf("Go's default cache %q is not isolated under %q", f.defaultCache, f.home)
	}

	event := `{"type":"assistant","session_id":"native-go","message":{"content":[{"type":"text","text":"ok"}]}}`
	if kind == "codex" {
		event = `{"type":"item.completed","thread_id":"native-go","item":{"type":"agent_message","text":"ok"}}`
	}
	body := `out="${0%/*}"
printf '%s\n' "$*" >"$out/args"
/usr/bin/env >"$out/env"
go env GOCACHE >"$out/gocache"
`
	if build {
		body += "go build ./lib\n"
	}
	writeProviderScript(t, f.dir, kind, body+"printf '%s\\n' '"+event+"'")
	return f
}

func (f goCacheFixture) execute(t *testing.T, req Request) {
	t.Helper()
	provider, err := NewAgentCLIProvider(AgentCLIConfig{Kind: f.kind, Binary: filepath.Join(f.dir, f.kind), SearchPath: f.searchPath})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Execute(context.Background(), req, nil)
	if err != nil || result.Outcome != OutcomeSucceeded || result.FinalText != "ok" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func (f goCacheFixture) read(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(f.dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// assertFiltered checks PATH is the provider search path, HOME still reaches
// the agent, want (when set) is present, and no credential or unlisted
// variable came along.
func (f goCacheFixture) assertFiltered(t *testing.T, want string) {
	t.Helper()
	env := f.read(t, "env")
	if strings.Contains(env, goCacheFixtureSecret) || strings.Contains(env, "XDG_CACHE_HOME=") {
		t.Fatalf("unlisted environment reached the agent:\n%s", env)
	}
	lines := strings.Split(env, "\n")
	if !slices.Contains(lines, "PATH="+f.searchPath) || !slices.Contains(lines, "HOME="+f.home) || want != "" && !slices.Contains(lines, want) {
		t.Fatalf("agent environment missing PATH/HOME/%q:\n%s", want, env)
	}
}
