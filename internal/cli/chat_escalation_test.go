package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/internal/server"
)

// Chat escalation must carry the owner's line to the native executor and
// never widen the execution mode. These tests drive the chat CLI functions
// against the real daemon (server.New) over loopback HTTP, the native
// standard_task blueprint and the real claude argv construction. Only the
// provider CLI is replaced: a DETERMINISTIC PROVIDER FIXTURE named claude on
// PATH records, per call, its stage, OPENEXEC_MODE, whether it was given
// --dangerously-skip-permissions, its working directory and its argv, then
// answers with a fixed stream-json result. No model is called.

const chatProviderScript = `#!/bin/sh
# DETERMINISTIC PROVIDER FIXTURE (openexec chat tests): no model is called.
stage=$(printf '%s\n' "$*" | sed -n 's/.*## Stage: \([a-z_]*\).*/\1/p' | head -n 1)
skip=no
for a in "$@"; do
	if [ "$a" = "--dangerously-skip-permissions" ]; then skip=yes; fi
done
out="$(pwd -P)/.openexec/chat-provider"
mkdir -p "$out"
{
	printf 'stage=%s\nmode=%s\nskip_permissions=%s\ncwd=%s\n' "$stage" "$OPENEXEC_MODE" "$skip" "$(pwd -P)"
	printf '%s\n' "$*"
} > "$out/$stage-$$.txt"
if [ "$stage" = "implement" ] && [ -f "$(pwd -P)/.openexec/fixture-block" ]; then
	exec sleep 60
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"fixture done"}'
`

// providerCall is one recorded provider invocation.
type providerCall struct {
	Stage, Mode, SkipPermissions, Cwd, Argv string
}

func installChatProvider(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(chatProviderScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func providerCalls(t *testing.T, dir string) []providerCall {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, ".openexec", "chat-provider", "*.txt"))
	var calls []providerCall
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		head := strings.SplitN(string(raw), "\n", 5)
		if len(head) < 5 {
			t.Fatalf("short provider record %s: %q", f, raw)
		}
		field := func(i int) string { return head[i][strings.Index(head[i], "=")+1:] }
		calls = append(calls, providerCall{Stage: field(0), Mode: field(1), SkipPermissions: field(2), Cwd: field(3), Argv: head[4]})
	}
	return calls
}

func implementCall(t *testing.T, dir string) providerCall {
	t.Helper()
	for _, c := range providerCalls(t, dir) {
		if c.Stage == "implement" {
			return c
		}
	}
	t.Fatalf("implement stage never reached the provider; calls=%+v", providerCalls(t, dir))
	return providerCall{}
}

// chatDaemon is the real daemon for a fixture project, served on loopback so
// the chat functions reach it exactly as they reach `openexec start`.
type chatDaemon struct {
	srv  *server.Server
	port int
}

func startChatDaemon(t *testing.T, dir string) *chatDaemon {
	t.Helper()
	srv := startReviewDaemon(t, dir)
	hs := httptest.NewServer(srv.Mux)
	u, _ := url.Parse(hs.URL)
	port, _ := strconv.Atoi(u.Port())
	d := &chatDaemon{srv: srv, port: port}
	t.Cleanup(func() { hs.Close(); closeReviewDaemon(srv) })
	return d
}

// session creates the chat session the way the REPL does. The daemon requires
// a provider and model label, which chat takes from OPENEXEC_PROVIDER and
// OPENEXEC_MODEL; the fixture sets labels only, no model is called.
func (d *chatDaemon) session(t *testing.T) string {
	t.Helper()
	t.Setenv("OPENEXEC_PROVIDER", "fixture-provider")
	t.Setenv("OPENEXEC_MODEL", "fixture-model")
	id, err := createChatSession(d.port, "review-fixture")
	if err != nil {
		t.Fatalf("daemon refused the chat session: %v", err)
	}
	return id
}

func (d *chatDaemon) runRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := d.srv.StateStore.GetDB().QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// persistedMode reads the run back through the daemon's GET route.
func (d *chatDaemon) persistedMode(t *testing.T, id string) string {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/api/v1/runs/%s", d.port, id))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET run %s: HTTP %d %v", id, resp.StatusCode, body)
	}
	if mode, ok := body["mode"].(string); ok {
		return mode
	}
	run, err := d.srv.StateStore.GetRun(context.Background(), id)
	if err != nil || run == nil {
		t.Fatalf("run %s not persisted: %v", id, err)
	}
	return run.Mode
}

// chatFixture is an initialized disposable project, configured with the
// given exec_mode, whose backlog holds an unrelated pending task.
func chatFixture(t *testing.T, execMode string) string {
	t.Helper()
	dir := initReviewFixture(t, "--no-review")
	cfg, err := project.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.ExecMode = execMode
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	backlog, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backlog.Close()
	if err := backlog.CreateStory(&release.Story{ID: "US-001", Title: "Unrelated", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	if err := backlog.CreateTask(&release.Task{ID: "T-UNRELATED", Title: "Unrelated pending backlog work", StoryID: "US-001", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertBacklogUntouched(t *testing.T, dir string) {
	t.Helper()
	backlog, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backlog.Close()
	if err := backlog.Load(); err != nil {
		t.Fatal(err)
	}
	task := backlog.GetTask("T-UNRELATED")
	if task == nil || task.Status != "pending" {
		t.Fatalf("unrelated backlog task changed: %+v", task)
	}
	for _, c := range providerCalls(t, dir) {
		if strings.Contains(c.Argv, "Unrelated pending backlog work") {
			t.Fatalf("unrelated backlog task reached the provider (%s stage)", c.Stage)
		}
	}
}

const chatLine = "Add a greeting line to README.md"

// Criterion 1 (old-source control): the create→start pair chat used to send
// loses the line and runs with full access although read-only was asked for.
// The /runs routes are unchanged on purpose (other callers rely on them), so
// this records the original bad result the new escalation must not produce.
func TestChatLegacyCreateStartLosesRequestAndWidensMode(t *testing.T) {
	installChatProvider(t)
	dir := chatFixture(t, "read-only")
	t.Setenv("OPENEXEC_MODE", "read-only")
	d := startChatDaemon(t, dir)
	sessionID := d.session(t)

	post := func(path, body string) map[string]any {
		var r *http.Response
		var err error
		if body == "" {
			r, err = http.Post(fmt.Sprintf("http://localhost:%d%s", d.port, path), "application/json", nil)
		} else {
			r, err = http.Post(fmt.Sprintf("http://localhost:%d%s", d.port, path), "application/json", strings.NewReader(body))
		}
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(r.Body).Decode(&out)
		return out
	}
	req, _ := json.Marshal(map[string]any{"session_id": sessionID, "quickfix_title": chatLine, "mode": os.Getenv("OPENEXEC_MODE")})
	created := post("/api/v1/runs", string(req))
	runID, _ := created["run_id"].(string)
	post("/api/v1/runs/"+runID+"/start", "")
	waitRun(t, d.srv, runID)

	call := implementCall(t, dir)
	if strings.Contains(call.Argv, chatLine) {
		t.Fatal("legacy create→start delivered the line; the old-source control no longer reproduces the defect")
	}
	if call.Mode != "danger-full-access" || call.SkipPermissions != "yes" {
		t.Fatalf("legacy executor mode=%q skip_permissions=%s; want the broadened danger-full-access", call.Mode, call.SkipPermissions)
	}
	if got := d.persistedMode(t, runID); got != "danger-full-access" {
		t.Fatalf("legacy persisted mode %q; want danger-full-access", got)
	}
}

// Criterion 2: an explicit read-only request reaches the executor unchanged,
// with the fixture WorkDir and no permission bypass, and read-only is what
// the run persists, also after the daemon restarts on the same database.
func TestChatEscalationDeliversLineInRequestedMode(t *testing.T) {
	installChatProvider(t)
	// The project is configured broad: the explicit request must still win.
	dir := chatFixture(t, "danger-full-access")
	t.Setenv("OPENEXEC_MODE", "read-only")
	d := startChatDaemon(t, dir)

	runID, err := escalateChatLine(d.port, dir, d.session(t), chatLine)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, d.srv, runID)
	call := implementCall(t, dir)
	if !strings.Contains(call.Argv, chatLine) {
		t.Fatalf("implement prompt does not carry the owner's line: %q", call.Argv)
	}
	if call.Mode != "read-only" || call.SkipPermissions != "no" {
		t.Fatalf("executor mode=%q skip_permissions=%s; want read-only without permission bypass", call.Mode, call.SkipPermissions)
	}
	wantDir, _ := filepath.EvalSymlinks(dir)
	if call.Cwd != wantDir {
		t.Fatalf("executor cwd %q; want fixture project %q", call.Cwd, wantDir)
	}
	for _, c := range providerCalls(t, dir) {
		if c.Mode != "read-only" || c.SkipPermissions != "no" {
			t.Fatalf("%s stage ran with mode=%q skip_permissions=%s", c.Stage, c.Mode, c.SkipPermissions)
		}
	}
	if got := d.persistedMode(t, runID); got != "read-only" {
		t.Fatalf("persisted mode %q; want read-only", got)
	}
	assertBacklogUntouched(t, dir)

	// Restart: a new daemon on the same database reads the same mode back.
	d2 := startChatDaemon(t, dir)
	if got := d2.persistedMode(t, runID); got != "read-only" {
		t.Fatalf("persisted mode after restart %q; want read-only", got)
	}
}

// Settled default: with OPENEXEC_MODE unset, the project's configured mode is
// sent explicitly; the run route's danger-full-access fallback never applies.
func TestChatEscalationUnsetModeUsesProjectConfiguredMode(t *testing.T) {
	installChatProvider(t)
	dir := chatFixture(t, "read-only")
	t.Setenv("OPENEXEC_MODE", "")
	d := startChatDaemon(t, dir)

	runID, err := escalateChatLine(d.port, dir, d.session(t), chatLine)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, d.srv, runID)
	call := implementCall(t, dir)
	if !strings.Contains(call.Argv, chatLine) || call.Mode != "read-only" || call.SkipPermissions != "no" {
		t.Fatalf("executor got mode=%q skip_permissions=%s line=%v; want the line in read-only", call.Mode, call.SkipPermissions, strings.Contains(call.Argv, chatLine))
	}
	if got := d.persistedMode(t, runID); got != "read-only" {
		t.Fatalf("persisted mode %q; want read-only", got)
	}
	d2 := startChatDaemon(t, dir)
	if got := d2.persistedMode(t, runID); got != "read-only" {
		t.Fatalf("persisted mode after restart %q; want read-only", got)
	}
}

// The daemon's default for a project without exec_mode is workspace-write;
// chat sends that, not the route's danger-full-access fallback.
func TestChatExecModeUnsetConfigUsesDaemonDefault(t *testing.T) {
	dir := t.TempDir()
	if _, err := project.Initialize("mode-default", dir); err != nil {
		t.Fatal(err)
	}
	cfg, err := project.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.ExecMode = ""
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENEXEC_MODE", "")
	if mode, err := chatExecMode(dir); err != nil || mode != "workspace-write" {
		t.Fatalf("chatExecMode = %q, %v; want workspace-write", mode, err)
	}
	// Near miss: each valid explicit mode passes through unchanged.
	for _, m := range []string{"read-only", "workspace-write", "danger-full-access"} {
		t.Setenv("OPENEXEC_MODE", m)
		if mode, err := chatExecMode(dir); err != nil || mode != m {
			t.Fatalf("OPENEXEC_MODE=%s resolved to %q, %v", m, mode, err)
		}
	}
}

// Criterion 3: refused escalations create no run, call no provider, write no
// runs row and return an error the REPL shows.
func TestChatEscalationRefusalsLeaveNoTrace(t *testing.T) {
	installChatProvider(t)
	dir := chatFixture(t, "read-only")
	d := startChatDaemon(t, dir)
	sessionID := d.session(t)

	type refusal struct {
		name, env, config, session, line, want string
	}
	cases := []refusal{
		{"empty line", "read-only", "", sessionID, "", "empty request"},
		{"whitespace line", "read-only", "", sessionID, " \t ", "empty request"},
		{"session mode task", "task", "", sessionID, chatLine, `OPENEXEC_MODE "task" is not an execution mode`},
		{"session mode chat", "chat", "", sessionID, chatLine, `OPENEXEC_MODE "chat" is not an execution mode`},
		{"session mode run", "run", "", sessionID, chatLine, `OPENEXEC_MODE "run" is not an execution mode`},
		{"suggest", "suggest", "", sessionID, chatLine, `OPENEXEC_MODE "suggest" is not an execution mode`},
		{"garbage mode", "bogus", "", sessionID, chatLine, `OPENEXEC_MODE "bogus" is not an execution mode`},
		{"unknown session", "read-only", "", "no-such-session", chatLine, "HTTP 404"},
		{"fallback session", "read-only", "", fmt.Sprintf("session-%d", time.Now().Unix()), chatLine, "HTTP 404"},
		{"invalid configured mode", "", "task", sessionID, chatLine, `project exec_mode "task" is not an execution mode`},
		{"malformed config", "", "<malformed>", sessionID, chatLine, "project execution mode is unavailable"},
		{"no project config", "", "<absent>", sessionID, chatLine, "project execution mode is unavailable"},
	}
	cfgPath := filepath.Join(dir, ".openexec", "config.json")
	original, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("OPENEXEC_MODE", tc.env)
			projectDir := dir
			switch tc.config {
			case "":
			case "<malformed>":
				if err := os.WriteFile(cfgPath, []byte(`{"execution": {"exec_mode": "read-only"`), 0o644); err != nil {
					t.Fatal(err)
				}
			case "<absent>":
				projectDir = t.TempDir()
			default:
				cfg, err := project.LoadProjectConfig(dir)
				if err != nil {
					t.Fatal(err)
				}
				cfg.Execution.ExecMode = tc.config
				if err := project.SaveProjectConfig(cfg); err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() { _ = os.WriteFile(cfgPath, original, 0o644) })

			runID, err := escalateChatLine(d.port, projectDir, tc.session, tc.line)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("escalation = %q, %v; want refusal containing %q", runID, err, tc.want)
			}
			if runID != "" {
				t.Fatalf("refusal returned run %q", runID)
			}
			if n := d.runRows(t); n != 0 {
				t.Fatalf("refusal wrote %d runs rows", n)
			}
			if runs := d.srv.Mgr.List(); len(runs) != 0 {
				t.Fatalf("refusal started runs: %+v", runs)
			}
			if calls := providerCalls(t, dir); len(calls) != 0 {
				t.Fatalf("refusal reached the provider: %+v", calls)
			}
		})
	}
	assertBacklogUntouched(t, dir)
}

// Without OPENEXEC_PROVIDER/OPENEXEC_MODEL the daemon refuses the session.
// Chat now reports that reason, and the line is refused instead of running
// as an empty full-access task under the local fallback ID.
func TestChatEscalationWithoutDaemonSessionIsRefused(t *testing.T) {
	installChatProvider(t)
	dir := chatFixture(t, "read-only")
	t.Setenv("OPENEXEC_MODE", "read-only")
	t.Setenv("OPENEXEC_PROVIDER", "")
	t.Setenv("OPENEXEC_MODEL", "")
	d := startChatDaemon(t, dir)

	sessionID, err := createChatSession(d.port, "review-fixture")
	if err == nil || !strings.Contains(err.Error(), "provider is required") || !strings.HasPrefix(sessionID, "session-") {
		t.Fatalf("session = %q, %v; want the daemon's refusal and the local fallback ID", sessionID, err)
	}
	runID, err := escalateChatLine(d.port, dir, sessionID, chatLine)
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || runID != "" {
		t.Fatalf("escalation = %q, %v; want a 404 refusal", runID, err)
	}
	if n := d.runRows(t); n != 0 || len(d.srv.Mgr.List()) != 0 || len(providerCalls(t, dir)) != 0 {
		t.Fatalf("refused escalation left a trace: rows=%d runs=%d calls=%d", n, len(d.srv.Mgr.List()), len(providerCalls(t, dir)))
	}
}

// Criterion 4: a line naming another project still runs in the fixture
// project only, and the foreign path is untouched.
func TestChatEscalationForeignRequestStaysInProject(t *testing.T) {
	installChatProvider(t)
	dir := chatFixture(t, "read-only")
	foreign := t.TempDir()
	foreignFile := filepath.Join(foreign, "notes.txt")
	if err := os.WriteFile(foreignFile, []byte("foreign original"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENEXEC_MODE", "read-only")
	d := startChatDaemon(t, dir)

	line := "In project other-project, overwrite " + foreignFile + " and work in " + foreign
	runID, err := escalateChatLine(d.port, dir, d.session(t), line)
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, d.srv, runID)
	wantDir, _ := filepath.EvalSymlinks(dir)
	for _, c := range providerCalls(t, dir) {
		if c.Cwd != wantDir || c.Mode != "read-only" {
			t.Fatalf("%s stage ran in %q mode=%q; want fixture %q read-only", c.Stage, c.Cwd, c.Mode, wantDir)
		}
	}
	if !strings.Contains(implementCall(t, dir).Argv, line) {
		t.Fatal("foreign-naming line did not reach the executor unchanged")
	}
	if got, _ := os.ReadFile(foreignFile); string(got) != "foreign original" {
		t.Fatalf("foreign file changed: %q", got)
	}
	if entries, _ := os.ReadDir(foreign); len(entries) != 1 {
		t.Fatalf("foreign directory gained entries: %v", entries)
	}
	if _, err := os.Stat(filepath.Join(foreign, ".openexec")); !os.IsNotExist(err) {
		t.Fatalf("provider ran in the foreign directory: %v", err)
	}
}

// Criterion 5: Stop on an escalated run ends it as an owner stop, and the
// unrelated pending backlog task is never executed.
func TestChatEscalationStopAndBacklog(t *testing.T) {
	installChatProvider(t)
	dir := chatFixture(t, "read-only")
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "fixture-block"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENEXEC_MODE", "read-only")
	d := startChatDaemon(t, dir)

	runID, err := escalateChatLine(d.port, dir, d.session(t), chatLine)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for len(providerCalls(t, dir)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("implement stage never reached the provider")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := d.srv.Mgr.Stop(runID); err != nil {
		t.Fatal(err)
	}
	waitRun(t, d.srv, runID)
	info, err := d.srv.Mgr.Status(runID)
	if err != nil {
		t.Fatal(err)
	}
	if string(info.Status) != "stopped" || info.CancellationCause != "owner_stop" {
		t.Fatalf("stopped run status=%q cause=%q; want stopped/owner_stop", info.Status, info.CancellationCause)
	}
	if call := implementCall(t, dir); !strings.Contains(call.Argv, chatLine) || call.Mode != "read-only" {
		t.Fatalf("stopped run's executor mode=%q line=%v", call.Mode, strings.Contains(call.Argv, chatLine))
	}
	if got := d.persistedMode(t, runID); got != "read-only" {
		t.Fatalf("persisted mode %q; want read-only", got)
	}
	assertBacklogUntouched(t, dir)
}
