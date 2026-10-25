package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/api"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/runtime"
)

// nativeRunFixture is a disposable initialized project for `openexec run`
// journeys against the real API handler, manager and SQLite ledger:
//
//	S: A, B (depends on A)    U: UNREL (later priority)
//
// The project's test gate is `test -f feature.txt`, so A's first attempt
// fails its check until a fix writes feature.txt.
type nativeRunFixture struct {
	*runFixture
	m  *manager.Manager
	db *state.Store
}

func newNativeRunFixture(t *testing.T, mcfg manager.Config) *nativeRunFixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	cfg, err := project.Initialize("native-run-fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	f := &runFixture{dir: dir, updates: make(chan struct{}, 8)}
	previous := runUpdateCheck
	runUpdateCheck = func() {}
	t.Cleanup(func() { runUpdateCheck = previous })
	t.Chdir(dir)
	cfg.Execution.WorkerCount = 4 // the README example; the queue still runs one at a time
	cfg.Execution.LintCommands = []string{"true"}
	cfg.Execution.TestCommands = []string{"test -f feature.txt"}
	if mcfg.RunnerCommand != "" {
		cfg.Execution.RunnerCommand = mcfg.RunnerCommand
	}
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	rel, err := release.NewManager(f.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"S", "U"} {
		if err := rel.CreateStory(&release.Story{ID: s, Title: "Story " + s, Status: "pending"}); err != nil {
			t.Fatal(err)
		}
	}
	for _, task := range []*release.Task{
		{ID: "A", StoryID: "S", Description: "Implement feature A"},
		{ID: "B", StoryID: "S", DependsOn: []string{"A"}, Description: "Finish remaining original task B by persisting remaining.txt"},
		{ID: "UNREL", StoryID: "U", Priority: 5, Description: "Unrelated work in another story"},
	} {
		task.Title, task.Status = task.ID, release.TaskStatusPending
		if err := rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	rel.Close()

	db, err := state.NewStore(filepath.Join(f.dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	mcfg.WorkDir, mcfg.StateStore = f.dir, db
	m, err := manager.New(mcfg)
	if err != nil {
		t.Fatal(err)
	}
	n := &nativeRunFixture{runFixture: f, m: m, db: db}
	h := api.New(m, nil, nil, "", ":0").Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ready" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { srv.Close(); n.close() })
	u, _ := url.Parse(srv.URL)
	if err := os.WriteFile(filepath.Join(f.dir, ".openexec", "openexec.pid"), []byte(fmt.Sprintf("%d:%s", os.Getpid(), u.Port())), 0o600); err != nil {
		t.Fatal(err)
	}
	return n
}

func (n *nativeRunFixture) close() {
	if n.m != nil {
		_ = n.m.Wait(context.Background(), "A")
		n.m.Close()
		n.m = nil
	}
	if n.db != nil {
		n.db.Close()
		n.db = nil
	}
}

func (n *nativeRunFixture) snapshot(t *testing.T) map[string]*release.Task {
	t.Helper()
	rel, err := n.m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := rel.TasksInStories(context.Background(), []string{"S", "U"})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*release.Task{}
	for _, task := range tasks {
		out[task.ID] = task
	}
	return out
}

// await polls the ledger until settled reports true, then lets the engine's
// queue drain so the last pipeline's writes are complete.
func (n *nativeRunFixture) await(t *testing.T, what string, settled func(map[string]*release.Task) bool) map[string]*release.Task {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for {
		tasks := n.snapshot(t)
		if settled(tasks) {
			time.Sleep(500 * time.Millisecond)
			return n.snapshot(t)
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: ledger never settled: %s", what, describeTasks(tasks))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func describeTasks(tasks map[string]*release.Task) string {
	var b strings.Builder
	for id, task := range tasks {
		fmt.Fprintf(&b, "\n  %s status=%s attempts=%d meta=%v", id, task.Status, task.AttemptCount, task.Metadata)
	}
	return b.String()
}

func terminal(task *release.Task) bool {
	return task != nil && (task.Status == release.TaskStatusDone || task.Status == "error" || task.Status == release.TaskStatusFailed)
}

// newRepairJourney runs tasks through the real pipeline, gates and the
// labeled test runner (internal/loop/testdata/mock_claude.go). The runner
// only acts on task prompts inside an opted-in workspace: "Fix task for A"
// writes feature.txt, "Finish remaining original task B" writes
// remaining.txt; every other task does nothing useful.
func newRepairJourney(t *testing.T) *nativeRunFixture {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mock_claude")
	build := exec.Command("go", "build", "-o", bin, filepath.Join("..", "loop", "testdata", "mock_claude.go"))
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("build mock_claude: %v", err)
	}
	n := newNativeRunFixture(t, manager.Config{
		RunnerCommand:        bin,
		AgentsFS:             os.DirFS(filepath.Join("..", "pipeline", "testdata")),
		DefaultMaxIterations: 10,
		MaxRetries:           1,
		RetryBackoff:         []time.Duration{0},
	})
	if err := os.WriteFile(filepath.Join(n.dir, ".task-loop-fixture"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return n
}

// fixOf returns the one task created to repair id, checking its linkage.
func fixOf(t *testing.T, tasks map[string]*release.Task, id string) *release.Task {
	t.Helper()
	var fixes []*release.Task
	for _, task := range tasks {
		if task.Metadata["repair_of"] == id {
			fixes = append(fixes, task)
		}
	}
	if len(fixes) != 1 {
		t.Fatalf("want exactly one fix of %s, got %d:%s", id, len(fixes), describeTasks(tasks))
	}
	fix := fixes[0]
	if ev, _ := fix.Metadata["failure_evidence"].(string); !strings.HasPrefix(ev, "verification-failure-") {
		t.Fatalf("fix %s does not carry the failed check's receipt: %v", fix.ID, fix.Metadata)
	}
	if tasks[id].Metadata["fixed_by"] != fix.ID {
		t.Fatalf("%s is not closed with fix %s: %v", id, fix.ID, tasks[id].Metadata)
	}
	return fix
}

// assertRepaired checks A was closed with a passed fix after one attempt,
// in the live ledger and after it is reopened from disk.
func assertRepaired(t *testing.T, tasks map[string]*release.Task) *release.Task {
	t.Helper()
	a := tasks["A"]
	if a.Status != release.TaskStatusFailed || a.AttemptCount != 1 || !release.ClosedWithFix(a) {
		t.Fatalf("A was not closed with its fix after one attempt:%s", describeTasks(tasks))
	}
	fix := fixOf(t, tasks, "A")
	if fix.Status != release.TaskStatusDone || fix.StoryID != "S" {
		t.Fatalf("fix %s did not pass:%s", fix.ID, describeTasks(tasks))
	}
	return fix
}

func reopenLedger(t *testing.T, dir string) map[string]*release.Task {
	t.Helper()
	rel, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel.Close()
	out := map[string]*release.Task{}
	for _, task := range rel.GetTasks() {
		out[task.ID] = task
	}
	return out
}

// Public journey: bare `openexec run --no-auto-plan` on a project whose task A
// fails its check. Reproduced first on the old route, where A and B ended
// "error" with no fix. Now A is closed with a fix that runs next and passes,
// B (waiting on A) and the unrelated story's task run after it, and the whole
// ledger — identities, statuses and repair linkage — survives a reopen.
func TestRunRepairsFailedCheckAndContinuesRemainingWork(t *testing.T) {
	n := newRepairJourney(t)
	out, err := n.run(t, "--no-auto-plan")
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "worker_count is 4; the task queue runs one task at a time") {
		t.Fatalf("run did not disclose the sequential queue:\n%s", out)
	}
	tasks := n.await(t, "bare run", func(tasks map[string]*release.Task) bool {
		return terminal(tasks["B"]) && terminal(tasks["UNREL"])
	})
	fix := assertRepaired(t, tasks)
	// B and UNREL each passed `test -f feature.txt` on their first attempt,
	// so both ran after the fix wrote it: the fix ran first.
	for _, id := range []string{"B", "UNREL"} {
		if task := tasks[id]; task.Status != release.TaskStatusDone || task.AttemptCount != 1 || task.Metadata["fixed_by"] != nil {
			t.Fatalf("%s did not converge after the fix:%s", id, describeTasks(tasks))
		}
	}
	if len(tasks) != 4 {
		t.Fatalf("want A, its fix, B and UNREL, got:%s", describeTasks(tasks))
	}
	for _, name := range []string{"feature.txt", "remaining.txt"} {
		if data, err := os.ReadFile(filepath.Join(n.dir, name)); err != nil || len(data) == 0 {
			t.Fatalf("useful work %s absent: %v", name, err)
		}
	}
	evidence, _ := fix.Metadata["failure_evidence"].(string)

	n.close()
	reopened := reopenLedger(t, n.dir)
	if len(reopened) != 4 {
		t.Fatalf("reopened ledger lost or gained tasks:%s", describeTasks(reopened))
	}
	assertRepaired(t, reopened)
	if reopened[fix.ID].Metadata["failure_evidence"] != evidence {
		t.Fatalf("fix linkage changed across reopen")
	}
	for _, id := range []string{"B", "UNREL"} {
		if reopened[id].Status != release.TaskStatusDone {
			t.Fatalf("%s = %s after reopen", id, reopened[id].Status)
		}
	}
	store, err := state.NewStore(filepath.Join(n.dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	step, err := store.GetRunStep(context.Background(), evidence)
	if err != nil || step == nil || step.RunID != "A" || step.Status != "failed" {
		t.Fatalf("A's failed-check receipt did not survive reopen: %+v %v", step, err)
	}
}

// `openexec run A` runs A and the fix made for it, nothing else: B waits on
// A but was not selected, and the other story's task is never touched.
func TestRunTaskIDRepairsOnlyTheSelectedTask(t *testing.T) {
	n := newRepairJourney(t)
	out, err := n.run(t, "A")
	if err != nil {
		t.Fatalf("run A: %v\n%s", err, out)
	}
	tasks := n.await(t, "run A", func(tasks map[string]*release.Task) bool {
		fixed, _ := tasks["A"].Metadata["fixed_by"].(string)
		return (fixed != "" && terminal(tasks[fixed])) || tasks["A"].Status == "error"
	})
	time.Sleep(1500 * time.Millisecond) // time in which an unscoped queue would start B
	tasks = n.snapshot(t)
	assertRepaired(t, tasks)
	for _, id := range []string{"B", "UNREL"} {
		if task := tasks[id]; task.Status != release.TaskStatusPending || task.AttemptCount != 0 {
			t.Fatalf("unselected %s was touched:%s", id, describeTasks(tasks))
		}
	}
	if _, err := os.Stat(filepath.Join(n.dir, "remaining.txt")); !os.IsNotExist(err) {
		t.Fatalf("unselected B ran: %v", err)
	}
	n.close()
	reopened := reopenLedger(t, n.dir)
	assertRepaired(t, reopened)
	for _, id := range []string{"B", "UNREL"} {
		if reopened[id].Status != release.TaskStatusPending {
			t.Fatalf("%s = %s after reopen; want pending", id, reopened[id].Status)
		}
	}
}

// stoppableExecutor blocks A's first attempt until it is cancelled and
// completes every other stage; it records each task attempt it saw.
type stoppableExecutor struct {
	mu      sync.Mutex
	ran     []string
	started chan struct{}
	blocked bool
}

func (s *stoppableExecutor) Execute(ctx context.Context, st *runtime.Stage, in *runtime.StageInput) (*runtime.StageResult, error) {
	s.mu.Lock()
	id := strings.TrimPrefix(in.TaskDescription, "work ")
	if id != "" && (len(s.ran) == 0 || s.ran[len(s.ran)-1] != id) {
		s.ran = append(s.ran, id)
	}
	block := id == "A" && !s.blocked
	if block {
		s.blocked = true
	}
	s.mu.Unlock()
	if block {
		close(s.started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return &runtime.StageResult{StageName: st.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
}

func (s *stoppableExecutor) attempts() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ran...)
}

// An owner Stop is not an ordinary failure: the stopped task is retained, not
// failed or "error", gets no fix, and nothing runs after it — not B, not the
// other story. Only the owner's next run resumes it, with a new counted
// attempt and no release reset.
func TestRunOwnerStopStaysRestrictive(t *testing.T) {
	executor := &stoppableExecutor{started: make(chan struct{})}
	n := newNativeRunFixture(t, manager.Config{StageExecutor: executor})
	rel, err := n.m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"A", "B", "UNREL"} {
		task, err := rel.TaskSnapshot(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		task.Description = "work " + id
		if err := rel.UpdateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := n.run(t, "--no-auto-plan"); err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	select {
	case <-executor.started:
	case <-time.After(30 * time.Second):
		t.Fatal("A never started")
	}
	if err := n.m.Stop("A"); err != nil {
		t.Fatal(err)
	}
	if err := n.m.Wait(context.Background(), "A"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond) // time in which a queue would continue
	tasks := n.snapshot(t)
	if got := executor.attempts(); !reflect.DeepEqual(got, []string{"A"}) {
		t.Fatalf("work ran after the owner stop: %v", got)
	}
	if a := tasks["A"]; a.Status == "error" || a.Status == release.TaskStatusFailed || a.Status == release.TaskStatusDone || a.Metadata["fixed_by"] != nil {
		t.Fatalf("stopped A was failed, fixed or finished:%s", describeTasks(tasks))
	}
	if len(tasks) != 3 {
		t.Fatalf("the owner stop created a fix:%s", describeTasks(tasks))
	}
	for _, id := range []string{"B", "UNREL"} {
		if task := tasks[id]; task.Status != release.TaskStatusPending || task.AttemptCount != 0 {
			t.Fatalf("%s moved after the stop:%s", id, describeTasks(tasks))
		}
	}

	// The owner runs again; the stopped task resumes, the rest follows.
	if out, err := n.run(t, "--no-auto-plan"); err != nil {
		t.Fatalf("second run: %v\n%s", err, out)
	}
	tasks = n.await(t, "resumed run", func(tasks map[string]*release.Task) bool {
		return terminal(tasks["B"]) && terminal(tasks["UNREL"])
	})
	if a := tasks["A"]; a.Status != release.TaskStatusDone || a.AttemptCount != 2 {
		t.Fatalf("A did not resume with a new counted attempt:%s", describeTasks(tasks))
	}
	if got := executor.attempts(); !reflect.DeepEqual(got, []string{"A", "B", "UNREL"}) {
		t.Fatalf("resumed run order %v; want [A B UNREL]", got)
	}
}
