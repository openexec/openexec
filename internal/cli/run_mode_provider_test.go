package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

// The run routes' execution mode as the provider receives it. These drive the
// real daemon over loopback with the DETERMINISTIC PROVIDER FIXTURE from
// chat_escalation_test.go standing in for the claude CLI: it records the
// OPENEXEC_MODE it was started with and whether the real claude argv carried
// --dangerously-skip-permissions. No model is called.

type modeRouteCall struct {
	name string
	// post sends the request with body extras and returns status and run ID.
	post func(t *testing.T, d *chatDaemon, extra map[string]any) (int, string)
}

func postJSON(t *testing.T, d *chatDaemon, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	resp, err := http.Post(fmt.Sprintf("http://localhost:%d%s", d.port, path), "application/json", strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func merged(base, extra map[string]any) map[string]any {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

var providerModeRoutes = []modeRouteCall{
	{"start", func(t *testing.T, d *chatDaemon, extra map[string]any) (int, string) {
		code, _ := postJSON(t, d, "/api/v1/runs/T-MODE/start", merged(map[string]any{"task_description": chatLine}, extra))
		return code, "T-MODE"
	}},
	{"parallel", func(t *testing.T, d *chatDaemon, extra map[string]any) (int, string) {
		code, _ := postJSON(t, d, "/api/v1/runs/parallel", merged(map[string]any{"run_ids": []string{"T-UNRELATED"}}, extra))
		return code, "T-UNRELATED"
	}},
	{"blueprint", func(t *testing.T, d *chatDaemon, extra map[string]any) (int, string) {
		code, out := postJSON(t, d, "/api/v1/runs:blueprint", merged(map[string]any{"task_description": chatLine}, extra))
		id, _ := out["run_id"].(string)
		return code, id
	}},
	{"session", func(t *testing.T, d *chatDaemon, extra map[string]any) (int, string) {
		code, out := postJSON(t, d, "/api/v1/sessions/"+d.session(t)+"/run", merged(map[string]any{"task_description": chatLine}, extra))
		id, _ := out["run_id"].(string)
		return code, id
	}},
}

// An omitted mode reaches the provider as workspace-write, never
// danger-full-access; explicit read-only stays read-only without the
// permission bypass. The claude argv keeps --dangerously-skip-permissions
// under workspace-write, as before: the default changes the MCP tier, it is
// not a sandbox.
func TestRunRoutesProviderReceivesAdmittedMode(t *testing.T) {
	cases := []struct {
		name     string
		extra    map[string]any
		wantMode string
		wantSkip string
	}{
		{"omitted", map[string]any{}, "workspace-write", "yes"},
		{"read-only", map[string]any{"mode": "read-only"}, "read-only", "no"},
	}
	for _, route := range providerModeRoutes {
		for _, tc := range cases {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				installChatProvider(t)
				// The project is configured broad; the route does not read it.
				dir := chatFixture(t, "danger-full-access")
				d := startChatDaemon(t, dir)
				code, id := route.post(t, d, tc.extra)
				if code != http.StatusCreated && code != http.StatusAccepted {
					t.Fatalf("HTTP %d", code)
				}
				waitRun(t, d.srv, id)
				calls := providerCalls(t, dir)
				if len(calls) == 0 {
					t.Fatal("no provider call recorded")
				}
				for _, c := range calls {
					if c.Mode != tc.wantMode || c.SkipPermissions != tc.wantSkip {
						t.Fatalf("%s stage: OPENEXEC_MODE=%q skip_permissions=%s; want %q/%s", c.Stage, c.Mode, c.SkipPermissions, tc.wantMode, tc.wantSkip)
					}
				}
				if got := d.persistedMode(t, id); got != tc.wantMode {
					t.Fatalf("persisted mode %q; want %q", got, tc.wantMode)
				}
			})
		}
	}
}

// runBlueprintCLI executes `openexec blueprint <args>` through the real Cobra
// root command against the daemon d, which a PID file advertises.
func runBlueprintCLI(t *testing.T, dir string, d *chatDaemon, args ...string) (string, error) {
	t.Helper()
	pid := fmt.Sprintf("%d:%d", os.Getpid(), d.port)
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.pid"), []byte(pid), 0o600); err != nil {
		t.Fatal(err)
	}
	// The fixture daemon skips startup preflight; mark it ready so the
	// command uses it rather than trying to auto-start an engine.
	d.srv.Checker.SetReady(true)
	if !isServerRunning(dir, 0) {
		t.Fatal("blueprint command would not see the fixture daemon")
	}
	t.Chdir(dir)
	reset := func() {
		blueprintCmd.Flags().VisitAll(func(fl *pflag.Flag) {
			_ = fl.Value.Set(fl.DefValue)
			fl.Changed = false
		})
	}
	reset()
	t.Cleanup(reset)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	rootCmd.SetArgs(append([]string{"blueprint", "--port", strconv.Itoa(d.port)}, args...))
	err := rootCmd.Execute()
	return out.String(), err
}

// `openexec blueprint --mode workspace-write` used to send the operational
// mode "run", which is not an execution mode and the route now refuses. The
// requested mode is sent as given; the danger-full-access flag default and an
// invalid flag value behave as the route says.
func TestBlueprintCLISendsRequestedExecMode(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantMode string // "" means refused
	}{
		{"workspace-write", []string{"--mode", "workspace-write"}, "workspace-write"},
		{"read-only", []string{"--mode", "read-only"}, "read-only"},
		{"flag default", nil, "danger-full-access"},
		{"invalid", []string{"--mode", "run"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installChatProvider(t)
			dir := chatFixture(t, "danger-full-access")
			d := startChatDaemon(t, dir)
			out, err := runBlueprintCLI(t, dir, d, append(tc.args, chatLine)...)
			if tc.wantMode == "" {
				if err == nil || !strings.Contains(err.Error(), "invalid mode: run") {
					t.Fatalf("err = %v; want the route's invalid-mode refusal\n%s", err, out)
				}
				if n := d.runRows(t); n != 0 {
					t.Fatalf("%d runs rows for a refused mode", n)
				}
				return
			}
			if err != nil {
				t.Fatalf("blueprint: %v\n%s", err, out)
			}
			m := regexp.MustCompile(`Run ID:\s+(\S+)`).FindStringSubmatch(out)
			if m == nil {
				t.Fatalf("no run ID in output:\n%s", out)
			}
			waitRun(t, d.srv, m[1])
			if got := d.persistedMode(t, m[1]); got != tc.wantMode {
				t.Fatalf("persisted mode %q; want %q", got, tc.wantMode)
			}
			if c := implementCall(t, dir); c.Mode != tc.wantMode {
				t.Fatalf("provider OPENEXEC_MODE=%q; want %q", c.Mode, tc.wantMode)
			}
		})
	}
}

// An unknown mode is refused before a run row or provider call exists.
func TestRunRoutesProviderNeverSeesInvalidMode(t *testing.T) {
	for _, route := range providerModeRoutes {
		t.Run(route.name, func(t *testing.T) {
			installChatProvider(t)
			dir := chatFixture(t, "danger-full-access")
			d := startChatDaemon(t, dir)
			code, _ := route.post(t, d, map[string]any{"mode": "full-auto"})
			if code != http.StatusBadRequest {
				t.Fatalf("HTTP %d; want 400", code)
			}
			if n := d.runRows(t); n != 0 {
				t.Fatalf("%d runs rows for a refused mode", n)
			}
			if calls := providerCalls(t, dir); len(calls) != 0 {
				t.Fatalf("provider called for a refused mode: %+v", calls)
			}
		})
	}
}
