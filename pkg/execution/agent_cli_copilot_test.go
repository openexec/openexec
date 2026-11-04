package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures below follow GitHub's documented Copilot CLI contract (non
// interactive `copilot -p <prompt> -s --no-ask-user`, tool grants through
// --allow-tool, credentials through COPILOT_GITHUB_TOKEN / GH_TOKEN /
// GITHUB_TOKEN with gho_, ghu_ and github_pat_ accepted and ghp_ refused).
// They stand in for the binary, not for any private GitHub endpoint.

func clearCopilotEnv(t *testing.T) {
	t.Helper()
	for _, name := range copilotTokenEnv {
		t.Setenv(name, "")
	}
}

func copilotFixture(t *testing.T, body string) (*AgentCLIProvider, string) {
	t.Helper()
	dir := t.TempDir()
	script := writeProviderScript(t, dir, "copilot", body)
	provider, err := NewAgentCLIProvider(AgentCLIConfig{Kind: CopilotKind, Binary: script, SearchPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	return provider, dir
}

const copilotRecorder = `printf '%s\n' "$*" >"${0%/*}/args"
printf '%s|%s|%s|%s|%s\n' "${COPILOT_GITHUB_TOKEN:-}" "${GH_TOKEN:-}" "${OPENAI_API_KEY:-}" "${ANTHROPIC_API_KEY:-}" "${GITHUB_TOKEN:-}" >"${0%/*}/env"
printf '%s\n' 'first line'
printf '%s\n' 'second line'`

func TestCopilotCLIContractAndCredentialScoping(t *testing.T) {
	clearCopilotEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
	t.Setenv("OPENAI_API_KEY", "sk-must-not-reach-copilot")
	t.Setenv("ANTHROPIC_API_KEY", "ant-must-not-reach-copilot")

	for _, tc := range []struct {
		name    string
		sandbox string
		want    string
	}{
		{"read-only grants no tool", "read-only", "-p do the work -s --no-ask-user --model gpt-4.1"},
		{"workspace-write grants only file writes", "workspace-write", "-p do the work -s --no-ask-user --model gpt-4.1 --allow-tool=write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider, dir := copilotFixture(t, copilotRecorder)
			var events []Event
			result, err := provider.Execute(context.Background(), Request{
				ID: "r1", WorkingDir: dir, Prompt: "do the work", Model: "gpt-4.1", Sandbox: Sandbox{Mode: tc.sandbox},
			}, func(e Event) error { events = append(events, e); return nil })
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != OutcomeSucceeded || result.FinalText != "first line\nsecond line\n" {
				t.Fatalf("result = %+v", result)
			}
			if len(events) != 4 || events[0].Type != EventStarted || events[3].Type != EventCompleted {
				t.Fatalf("events = %#v", events)
			}
			args, _ := os.ReadFile(filepath.Join(dir, "args"))
			if strings.TrimSpace(string(args)) != tc.want {
				t.Fatalf("args = %q, want %q", args, tc.want)
			}
			env, _ := os.ReadFile(filepath.Join(dir, "env"))
			if strings.TrimSpace(string(env)) != "gho_fixture||||" {
				t.Fatalf("copilot saw env %q: want only its own token, never an OpenAI/Anthropic key", env)
			}
		})
	}
}

func TestCopilotTokenNotForwardedToOtherCLIs(t *testing.T) {
	clearCopilotEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
	t.Setenv("GH_TOKEN", "gho_other")
	dir := t.TempDir()
	script := writeProviderScript(t, dir, "codex", `printf '%s|%s' "${COPILOT_GITHUB_TOKEN:-}" "${GH_TOKEN:-}" >"${0%/*}/env"`)
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: "codex", Binary: script, SearchPath: dir})
	_, err := provider.Execute(context.Background(), Request{ID: "r", WorkingDir: dir, Prompt: "p", Sandbox: Sandbox{Mode: "read-only"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if env, _ := os.ReadFile(filepath.Join(dir, "env")); string(env) != "|" {
		t.Fatalf("codex received GitHub credentials: %q", env)
	}
}

func TestCopilotCLIRefusesWhatItCannotEnforce(t *testing.T) {
	clearCopilotEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_fixture")
	for name, req := range map[string]Request{
		"writable roots": {Sandbox: Sandbox{Mode: "workspace-write"}, WritableRoots: []string{"/tmp"}},
		"network access": {Sandbox: Sandbox{Mode: "workspace-write"}, NetworkAccess: true},
		"resume":         {Sandbox: Sandbox{Mode: "read-only"}, NativeSessionID: "x"},
	} {
		t.Run(name, func(t *testing.T) {
			provider, dir := copilotFixture(t, `touch "${0%/*}/ran"`)
			req.ID, req.WorkingDir, req.Prompt = "r", dir, "p"
			if _, err := provider.Execute(context.Background(), req, nil); err == nil {
				t.Fatal("request the Copilot CLI cannot enforce was accepted")
			}
			if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
				t.Fatal("binary was started despite the refusal")
			}
		})
	}
}

func TestCopilotCLIRefusesClassicPAT(t *testing.T) {
	clearCopilotEnv(t)
	t.Setenv("GH_TOKEN", "ghp_classic")
	provider, dir := copilotFixture(t, `touch "${0%/*}/ran"`)
	_, err := provider.Execute(context.Background(), Request{ID: "r", WorkingDir: dir, Prompt: "p", Sandbox: Sandbox{Mode: "read-only"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "GH_TOKEN") || !strings.Contains(err.Error(), "classic personal access tokens") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "ran")); statErr == nil {
		t.Fatal("binary ran with an unsupported token")
	}
	if strings.Contains(err.Error(), "ghp_classic") {
		t.Fatal("token value leaked into the error")
	}
}

func TestCopilotCLIAuthFailureSurfacesCLIMessage(t *testing.T) {
	clearCopilotEnv(t)
	t.Setenv("COPILOT_GITHUB_TOKEN", "gho_expired")
	provider, dir := copilotFixture(t, `echo "Authentication failed: token expired, run /login" >&2; exit 1`)
	var last Event
	result, err := provider.Execute(context.Background(), Request{ID: "r", WorkingDir: dir, Prompt: "p", Sandbox: Sandbox{Mode: "read-only"}},
		func(e Event) error { last = e; return nil })
	if err == nil || !strings.Contains(err.Error(), "token expired") || result.Outcome != OutcomeFailed || last.Type != EventFailed {
		t.Fatalf("result=%+v err=%v last=%+v", result, err, last)
	}
}

// Refresh is delegated: with no token variable, the CLI uses its own stored
// sign-in under COPILOT_HOME. The fixture models the CLI renewing an expired
// session from its stored refresh credential; OpenExec must pass COPILOT_HOME
// through, never touch the credential files, and report a successful result.
const copilotStoredSignIn = `home="${COPILOT_HOME:?}"
read -r state <"$home/session"
if [ "$state" = "expired" ]; then
  if [ -f "$home/refresh" ]; then echo fresh >"$home/session"; else
    echo "Session expired and could not be refreshed; run /login" >&2; exit 1; fi
fi
echo ok`

func TestCopilotCLIDelegatesRefreshToStoredSignIn(t *testing.T) {
	clearCopilotEnv(t)
	home := t.TempDir()
	t.Setenv("COPILOT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "session"), []byte("expired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "refresh"), []byte("r"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, dir := copilotFixture(t, copilotStoredSignIn)
	result, err := provider.Execute(context.Background(), Request{ID: "r", WorkingDir: dir, Prompt: "p", Sandbox: Sandbox{Mode: "read-only"}}, nil)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, "session")); strings.TrimSpace(string(got)) != "fresh" {
		t.Fatalf("session = %q: the CLI did not get to refresh its own sign-in", got)
	}
	// The probe cannot see inside the CLI's sign-in: it must not claim ready.
	if got := provider.Probe(context.Background(), dir); got.State != ReadinessUnknown {
		t.Fatalf("readiness with stored sign-in only = %+v", got)
	}
}

func TestCopilotCLIRefreshFailureReachesOpenExec(t *testing.T) {
	clearCopilotEnv(t)
	home := t.TempDir()
	t.Setenv("COPILOT_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "session"), []byte("expired\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, dir := copilotFixture(t, copilotStoredSignIn)
	result, err := provider.Execute(context.Background(), Request{ID: "r", WorkingDir: dir, Prompt: "p", Sandbox: Sandbox{Mode: "read-only"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "run /login") || result.Outcome != OutcomeFailed {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if got, _ := os.ReadFile(filepath.Join(home, "session")); strings.TrimSpace(string(got)) != "expired" {
		t.Fatalf("OpenExec changed the CLI's credential state: %q", got)
	}
}

func TestCopilotCLIReadinessFromDocumentedTokenVariables(t *testing.T) {
	for _, tc := range []struct {
		name  string
		env   map[string]string
		state string
	}{
		{"nothing set", nil, ReadinessUnknown},
		{"oauth user token", map[string]string{"COPILOT_GITHUB_TOKEN": "gho_x"}, ReadinessReady},
		{"github app user token", map[string]string{"GH_TOKEN": "ghu_x"}, ReadinessReady},
		{"fine-grained pat", map[string]string{"GITHUB_TOKEN": "github_pat_x"}, ReadinessReady},
		{"classic pat", map[string]string{"GITHUB_TOKEN": "ghp_x"}, ReadinessNeedsLogin},
		{"first documented variable wins", map[string]string{"COPILOT_GITHUB_TOKEN": "gho_x", "GH_TOKEN": "ghp_x"}, ReadinessReady},
		{"near-miss prefix without underscore", map[string]string{"GH_TOKEN": "gho"}, ReadinessUnknown},
		{"unrelated key", map[string]string{"GH_TOKEN": "sk-openai"}, ReadinessUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearCopilotEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			provider, dir := copilotFixture(t, `touch "${0%/*}/ran"`)
			got := provider.Probe(context.Background(), dir)
			if got.State != tc.state {
				t.Fatalf("readiness = %+v, want %s", got, tc.state)
			}
			if got.State != ReadinessReady && got.Problem == "" {
				t.Fatal("a non-ready state must say why")
			}
			if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
				t.Fatal("readiness probe ran the binary (no documented status command; must not infer)")
			}
		})
	}

	clearCopilotEnv(t)
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: CopilotKind, Binary: "copilot", SearchPath: t.TempDir()})
	if got := provider.Probe(context.Background(), t.TempDir()); got.State != ReadinessNotInstalled {
		t.Fatalf("missing binary readiness = %+v", got)
	}
}

func TestCopilotCLIDescriptorDoesNotClaimResumeOrNetwork(t *testing.T) {
	provider, _ := NewAgentCLIProvider(AgentCLIConfig{Kind: CopilotKind})
	caps := provider.Descriptor().Capabilities
	if caps.Resume || caps.CommandNetwork || !caps.ReadOnly || !caps.WorkspaceWrite {
		t.Fatalf("capabilities = %+v", caps)
	}
}
