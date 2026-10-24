package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/server"
	"github.com/spf13/pflag"
)

// The native review choice saved by `openexec init` must reach the daemon's
// task route. Every test here drives the real init command, the real daemon
// constructor (server.New), its HTTP run route and the native standard_task
// blueprint. Only the provider CLI is replaced: DETERMINISTIC PROVIDER FIXTURE
// scripts named claude/codex on PATH record which stage invoked them and
// answer with a fixed stream-json result. No model is called.

const fakeProviderScript = `#!/bin/sh
# DETERMINISTIC PROVIDER FIXTURE (openexec tests): no model is called.
stage=$(printf '%s\n' "$*" | sed -n 's/.*## Stage: \([a-z_]*\).*/\1/p' | head -n 1)
printf '%s %s\n' "$(basename "$0")" "$stage" >> "$(pwd -P)/.openexec/provider-receipts.log"
if [ "$stage" = "implement" ] && [ -f "$(pwd -P)/.openexec/fixture-block" ]; then
	exec sleep 60
fi
printf '%s\n' '{"type":"result","subtype":"success","result":"fixture done"}'
`

// installFakeProviders puts the fixture claude and codex CLIs first on PATH.
func installFakeProviders(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	for _, name := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(fakeProviderScript), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// resetInitFlags restores init's package-level flags so one in-process init
// cannot leak --review/--no-review into the next.
func resetInitFlags(t *testing.T) {
	t.Helper()
	initCmd.Flags().VisitAll(func(f *pflag.Flag) {
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

// initReviewFixture creates a disposable git project and runs the real
// `openexec init <name> -y <flags...>` in it.
func initReviewFixture(t *testing.T, flags ...string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "Initial")
	runGit(t, dir, "branch", "-M", "main")

	oldCwd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldCwd) }()

	resetInitFlags(t)
	defer resetInitFlags(t)
	var out strings.Builder
	rootCmd.SetOut(&out)
	rootCmd.SetArgs(append([]string{"init", "review-fixture", "-y"}, flags...))
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init %v failed: %v\n%s", flags, err, out.String())
	}

	// The native lint and test stages refuse to run without commands.
	cfg, err := project.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.LintCommands = []string{"true"}
	cfg.Execution.TestCommands = []string{"true"}
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	return dir
}

// startDaemon builds the daemon the way `openexec start` does, rereading the
// project's saved configuration.
func startReviewDaemon(t *testing.T, dir string) *server.Server {
	t.Helper()
	srv, err := server.New(server.Config{
		ProjectsDir:   dir,
		DataDir:       filepath.Join(dir, ".openexec", "data"),
		UnifiedDB:     filepath.Join(dir, ".openexec", "openexec.db"),
		SkipPreflight: true,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return srv
}

func closeReviewDaemon(srv *server.Server) {
	srv.Mgr.Close()
	_ = srv.StateStore.Close()
}

// postRunStart starts one native task through the daemon's HTTP run route.
func postRunStart(t *testing.T, srv *server.Server, id string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs/"+id+"/start",
		strings.NewReader(`{"task_description":"Add a greeting to README.md"}`))
	rec := httptest.NewRecorder()
	srv.Mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("start %s: HTTP %d: %s", id, rec.Code, rec.Body.String())
	}
}

func waitRun(t *testing.T, srv *server.Server, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := srv.Mgr.Wait(ctx, id); err != nil {
		t.Fatalf("wait %s: %v", id, err)
	}
}

// runNativeTask runs one task through a fresh daemon on dir and returns the
// provider receipts ("<cli> <stage>" per invocation) that task produced.
func runNativeTask(t *testing.T, dir, id string) []string {
	t.Helper()
	receipts := filepath.Join(dir, ".openexec", "provider-receipts.log")
	_ = os.Remove(receipts)
	srv := startReviewDaemon(t, dir)
	defer closeReviewDaemon(srv)
	postRunStart(t, srv, id)
	waitRun(t, srv, id)
	info, err := srv.Mgr.Status(id)
	if err != nil {
		t.Fatalf("status %s: %v", id, err)
	}
	if string(info.Status) != "complete" {
		t.Fatalf("task %s ended %q, want complete; info=%+v receipts=%q", id, info.Status, info, readReceipts(t, dir))
	}
	return readReceipts(t, dir)
}

func readReceipts(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".openexec", "provider-receipts.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var receipts []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		receipts = append(receipts, strings.Replace(line, " ", "|", 1))
	}
	return receipts
}

func setSavedReview(t *testing.T, dir string, enabled bool) {
	t.Helper()
	cfg, err := project.LoadProjectConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.ReviewEnabled = enabled
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func assertReceipts(t *testing.T, label string, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s: provider receipts = %q, want %q", label, got, want)
	}
}

// A1 + A2: the saved choice decides whether the native review stage runs
// after test, and a reloaded daemon follows a changed choice.
func TestInitReviewChoiceDrivesDaemonReviewStage(t *testing.T) {
	installFakeProviders(t)

	on := initReviewFixture(t, "--review")
	assertReceipts(t, "review on", runNativeTask(t, on, "RUN-ON-1"),
		"claude|implement", "claude|review")

	off := initReviewFixture(t, "--no-review")
	assertReceipts(t, "review off", runNativeTask(t, off, "RUN-OFF-1"),
		"claude|implement")

	// Reload: flip the saved value; a new daemon on the same project rereads it.
	setSavedReview(t, on, false)
	assertReceipts(t, "on project reloaded off", runNativeTask(t, on, "RUN-ON-2"),
		"claude|implement")
	setSavedReview(t, on, true)
	assertReceipts(t, "on project restored", runNativeTask(t, on, "RUN-ON-3"),
		"claude|implement", "claude|review")

	setSavedReview(t, off, true)
	assertReceipts(t, "off project reloaded on", runNativeTask(t, off, "RUN-OFF-2"),
		"claude|implement", "claude|review")
}

// A1: the saved reviewer model selects the review stage's CLI runner. Codex
// receives no stage prompt on stdin today, so its receipt carries no stage
// name; the claude executor never sees the review stage.
func TestInitReviewerModelSelectsDaemonReviewRunner(t *testing.T) {
	installFakeProviders(t)
	dir := initReviewFixture(t, "--review", "--reviewer", "gpt-5-codex")
	assertReceipts(t, "codex reviewer", runNativeTask(t, dir, "RUN-MODEL-1"),
		"claude|implement", "codex|")
}

// A4: a config written before review_enabled existed keeps today's behaviour.
func TestDaemonReviewMissingKeySkipsReview(t *testing.T) {
	installFakeProviders(t)
	dir := initReviewFixture(t)
	path := filepath.Join(dir, ".openexec", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stripped := strings.Replace(string(data), `"review_enabled": true,`, "", 1)
	if stripped == string(data) {
		t.Fatalf("fixture config has no review_enabled key to remove:\n%s", data)
	}
	if err := os.WriteFile(path, []byte(stripped), 0o644); err != nil {
		t.Fatal(err)
	}
	assertReceipts(t, "missing key", runNativeTask(t, dir, "RUN-LEGACY-1"), "claude|implement")
}

// A4: a malformed config is not silently read as a review choice; the daemon
// still starts exactly as before, with review off.
func TestDaemonReviewMalformedConfigKeepsReviewOff(t *testing.T) {
	installFakeProviders(t)
	dir := initReviewFixture(t, "--review")
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "config.json"), []byte(`{"execution": {"review_enabled": true`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := project.LoadProjectConfig(dir); err == nil {
		t.Fatal("fixture config should be malformed")
	}
	srv := startReviewDaemon(t, dir)
	defer closeReviewDaemon(srv)
	if cfg := srv.Mgr.GetConfig(); cfg.ReviewEnabled || cfg.ReviewerModel != "" {
		t.Fatalf("malformed config produced review=%v reviewer=%q", cfg.ReviewEnabled, cfg.ReviewerModel)
	}
}

// A4: a task stopped before review never reaches the review stage.
func TestDaemonReviewStoppedTaskRunsNoReview(t *testing.T) {
	installFakeProviders(t)
	dir := initReviewFixture(t, "--review")
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "fixture-block"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	srv := startReviewDaemon(t, dir)
	defer closeReviewDaemon(srv)
	if !srv.Mgr.GetConfig().ReviewEnabled {
		t.Fatal("daemon did not load the saved review choice")
	}
	postRunStart(t, srv, "RUN-STOP-1")
	deadline := time.Now().Add(30 * time.Second)
	for len(readReceipts(t, dir)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("implement stage never reached the provider")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := srv.Mgr.Stop("RUN-STOP-1"); err != nil {
		t.Fatal(err)
	}
	waitRun(t, srv, "RUN-STOP-1")
	info, err := srv.Mgr.Status("RUN-STOP-1")
	if err != nil {
		t.Fatal(err)
	}
	if string(info.Status) == "complete" {
		t.Fatal("stopped task reported complete")
	}
	assertReceipts(t, "stopped", readReceipts(t, dir), "claude|implement")
}

// A4: two daemons with opposite saved choices run side by side without
// leaking either choice into the other project.
func TestDaemonReviewChoiceDoesNotLeakAcrossProjects(t *testing.T) {
	installFakeProviders(t)
	on := initReviewFixture(t, "--review")
	off := initReviewFixture(t, "--no-review")
	srvOn, srvOff := startReviewDaemon(t, on), startReviewDaemon(t, off)
	defer closeReviewDaemon(srvOn)
	defer closeReviewDaemon(srvOff)
	postRunStart(t, srvOn, "RUN-A")
	postRunStart(t, srvOff, "RUN-B")
	waitRun(t, srvOn, "RUN-A")
	waitRun(t, srvOff, "RUN-B")
	assertReceipts(t, "review-on project", readReceipts(t, on), "claude|implement", "claude|review")
	assertReceipts(t, "review-off project", readReceipts(t, off), "claude|implement")
}
