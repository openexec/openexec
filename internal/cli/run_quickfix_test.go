package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
)

// runRequest is one request the run command sent to the engine.
type runRequest struct {
	Path string
	Body map[string]any
}

// runDaemon is a deterministic test fixture, not a live engine: an httptest
// server registered in .openexec/openexec.pid so the run command treats it as
// the project's running daemon and records every request it sends.
type runDaemon struct {
	mu       sync.Mutex
	requests []runRequest
}

func (d *runDaemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	d.mu.Lock()
	d.requests = append(d.requests, runRequest{Path: r.URL.Path, Body: body})
	d.mu.Unlock()
	switch r.URL.Path {
	case "/api/ready", "/api/v1/runs:plan":
		w.WriteHeader(http.StatusOK)
	case "/api/v1/runs:execute":
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (d *runDaemon) recorded() []runRequest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]runRequest(nil), d.requests...)
}

// runFixture is a disposable initialized project whose backlog already holds
// an unrelated pending task. With daemon set, a recording engine is registered;
// without it, no engine runs and the configured runner does not exist, so the
// old code path stops at the auto-start preflight instead of spawning anything.
type runFixture struct {
	dir     string
	daemon  *runDaemon
	updates chan struct{}
}

func newRunFixture(t *testing.T, daemon bool) *runFixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	cfg, err := project.Initialize("run-quickfix-fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.RunnerCommand = "openexec-test-runner-that-does-not-exist"
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	backlog, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := backlog.CreateStory(&release.Story{ID: "US-001", Title: "Unrelated", Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"T-001", "T-UNRELATED"} {
		if err := backlog.CreateTask(&release.Task{ID: id, Title: "Unrelated pending " + id, StoryID: "US-001", Status: "pending"}); err != nil {
			t.Fatal(err)
		}
	}
	backlog.Close()

	f := &runFixture{dir: dir, updates: make(chan struct{}, 8)}
	if daemon {
		f.daemon = &runDaemon{}
		srv := httptest.NewServer(f.daemon)
		t.Cleanup(srv.Close)
		u, _ := url.Parse(srv.URL)
		pid := fmt.Sprintf("%d:%s", os.Getpid(), u.Port())
		if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.pid"), []byte(pid), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	previous := runUpdateCheck
	runUpdateCheck = func() { f.updates <- struct{}{} }
	t.Cleanup(func() { runUpdateCheck = previous })
	t.Chdir(dir)
	return f
}

// run executes `openexec run <args>` through the real Cobra root command.
func (f *runFixture) run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetFlags := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(fl *pflag.Flag) {
			_ = fl.Value.Set(fl.DefValue)
			fl.Changed = false
		})
	}
	resetFlags(runCmd.Flags())
	t.Cleanup(func() { resetFlags(runCmd.Flags()) })
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	rootCmd.SetArgs(append([]string{"run"}, args...))
	err := rootCmd.Execute()
	return out.String(), err
}

func describeRequests(reqs []runRequest) string {
	var b strings.Builder
	for _, r := range reqs {
		raw, _ := json.Marshal(r.Body)
		fmt.Fprintf(&b, "\n  %s %s", r.Path, raw)
	}
	return b.String()
}

func TestRunRefusesQuickfixAndVerifyBeforeAnySideEffect(t *testing.T) {
	cases := []struct {
		name string
		args []string
		flag string
	}{
		{"quickfix and verify", []string{"--quickfix", "Fix typo", "--verify", "make test"}, "--quickfix"},
		{"quickfix alone", []string{"--quickfix", "Fix typo"}, "--quickfix"},
		{"verify alone", []string{"--verify", "make test"}, "--verify"},
		{"empty values", []string{"--quickfix", "", "--verify", ""}, "--quickfix"},
		{"malformed verify", []string{"--verify=;rm -rf /"}, "--verify"},
		{"with task id", []string{"T-001", "--quickfix", "Fix typo"}, "--quickfix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunFixture(t, true)
			out, err := f.run(t, tc.args...)
			if reqs := f.daemon.recorded(); len(reqs) != 0 {
				t.Fatalf("refused invocation reached the engine (%d requests):%s", len(reqs), describeRequests(reqs))
			}
			if err == nil {
				t.Fatalf("run %v succeeded; want an unsupported-flag refusal\n%s", tc.args, out)
			}
			want := "run " + tc.flag + " is not supported"
			if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "blueprint --blueprint-id quick_fix") {
				t.Fatalf("error = %q; want it to contain %q and name the quick_fix blueprint", err, want)
			}
			if len(f.updates) != 0 {
				t.Fatal("refused invocation started the update check")
			}
		})
	}
}

// Without a running engine the refusal must also precede the auto-start
// path: the old code reached the runner preflight and, with a runner present,
// would have spawned `openexec start --daemon`.
func TestRunQuickfixRefusalPrecedesDaemonAutoStart(t *testing.T) {
	f := newRunFixture(t, false)
	_, err := f.run(t, "--quickfix", "Fix typo", "--verify", "make test")
	if err == nil || !strings.Contains(err.Error(), "run --quickfix is not supported") {
		t.Fatalf("error = %v; want the unsupported refusal before the auto-start preflight", err)
	}
	if strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("refusal came after the auto-start preflight: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(f.dir, ".openexec", "openexec.pid")); !os.IsNotExist(statErr) {
		t.Fatalf("a daemon PID file appeared: %v", statErr)
	}
	if len(f.updates) != 0 {
		t.Fatal("refused invocation started the update check")
	}

	// Near miss: plain run in the same project does reach the auto-start
	// preflight, so the assertion above is not satisfied by accident.
	_, err = f.run(t)
	if err == nil || !strings.Contains(err.Error(), "not found on PATH") {
		t.Fatalf("plain run error = %v; want the auto-start runner preflight", err)
	}
}

func waitUpdateCheck(t *testing.T, f *runFixture) {
	t.Helper()
	select {
	case <-f.updates:
	case <-time.After(5 * time.Second):
		t.Fatal("normal run did not start the update check")
	}
}

func TestRunNormalInvocationsUnchanged(t *testing.T) {
	t.Run("plain run plans then executes every pending task", func(t *testing.T) {
		f := newRunFixture(t, true)
		if out, err := f.run(t); err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		waitUpdateCheck(t, f)
		reqs := f.daemon.recorded()
		var posts []runRequest
		for _, r := range reqs {
			if r.Path != "/api/ready" {
				posts = append(posts, r)
			}
		}
		if len(posts) != 2 || posts[0].Path != "/api/v1/runs:plan" || posts[1].Path != "/api/v1/runs:execute" {
			t.Fatalf("requests = %s; want plan then execute", describeRequests(reqs))
		}
		if posts[0].Body["intent_file"] != "INTENT.md" || posts[0].Body["auto_import"] != true {
			t.Fatalf("plan body = %v", posts[0].Body)
		}
		if _, ok := posts[1].Body["task_ids"]; ok {
			t.Fatalf("plain run execute body = %v; want no task_ids (all pending)", posts[1].Body)
		}
		if posts[1].Body["mode"] != "danger-full-access" {
			t.Fatalf("execute mode = %v", posts[1].Body["mode"])
		}
	})

	t.Run("task id skips auto-plan and scopes execute", func(t *testing.T) {
		f := newRunFixture(t, true)
		if out, err := f.run(t, "T-001"); err != nil {
			t.Fatalf("run T-001: %v\n%s", err, out)
		}
		waitUpdateCheck(t, f)
		reqs := f.daemon.recorded()
		var paths []string
		var exec runRequest
		for _, r := range reqs {
			if r.Path == "/api/ready" {
				continue
			}
			paths = append(paths, r.Path)
			exec = r
		}
		// A named task must already exist; planning first would import
		// before the engine can refuse the selection.
		if strings.Join(paths, ",") != "/api/v1/runs:execute" {
			t.Fatalf("requests = %s; want execute only", describeRequests(reqs))
		}
		ids, _ := exec.Body["task_ids"].([]any)
		if len(ids) != 1 || ids[0] != "T-001" {
			t.Fatalf("execute task_ids = %v; want [T-001]", exec.Body["task_ids"])
		}
	})

	t.Run("no-auto-plan executes without planning", func(t *testing.T) {
		f := newRunFixture(t, true)
		if out, err := f.run(t, "--no-auto-plan", "T-001"); err != nil {
			t.Fatalf("run --no-auto-plan T-001: %v\n%s", err, out)
		}
		reqs := f.daemon.recorded()
		var paths []string
		for _, r := range reqs {
			if r.Path != "/api/ready" {
				paths = append(paths, r.Path)
			}
		}
		if strings.Join(paths, ",") != "/api/v1/runs:execute" {
			t.Fatalf("requests = %s; want execute only", describeRequests(reqs))
		}
	})
}

func TestRunHelpNoLongerAdvertisesQuickfix(t *testing.T) {
	f := newRunFixture(t, true)
	out, err := f.run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if reqs := f.daemon.recorded(); len(reqs) != 0 {
		t.Fatalf("help reached the engine:%s", describeRequests(reqs))
	}
	if !strings.Contains(out, "openexec run T-001") {
		t.Fatalf("help output missing the run examples:\n%s", out)
	}
	if strings.Contains(out, `--quickfix "Fix typo"`) {
		t.Fatalf("run help still advertises the dead --quickfix example:\n%s", out)
	}
	if !strings.Contains(out, "blueprint --blueprint-id quick_fix") {
		t.Fatalf("run help does not point to the quick_fix blueprint:\n%s", out)
	}
	for _, name := range []string{"quickfix", "verify"} {
		// The flags stay parseable so they are refused explicitly rather
		// than rejected as unknown, and their usage says so.
		if !strings.Contains(out, "--"+name+" string") {
			t.Fatalf("--%s is no longer listed:\n%s", name, out)
		}
	}
	if strings.Count(out, "Unsupported; refused.") != 2 {
		t.Fatalf("--quickfix/--verify usage not marked unsupported:\n%s", out)
	}
}
