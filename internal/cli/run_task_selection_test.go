package cli

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/api"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/runtime"
)

// cliSelectionRecorder is a deterministic stage-executor fixture, not a
// provider: it completes every stage and records the task each stage ran for.
type cliSelectionRecorder struct {
	mu   sync.Mutex
	ran  []string
	seen map[string]bool
}

func (r *cliSelectionRecorder) Execute(_ context.Context, s *runtime.Stage, in *runtime.StageInput) (*runtime.StageResult, error) {
	r.mu.Lock()
	if id := strings.TrimPrefix(in.TaskDescription, "work "); !r.seen[id] {
		r.seen[id] = true
		r.ran = append(r.ran, id)
	}
	r.mu.Unlock()
	return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
}

func (r *cliSelectionRecorder) tasks() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.ran...)
}

// realEngine serves the real API handler, manager and SQLite ledger of the
// run fixture's project on the port registered in its PID file, so
// `openexec run` drives the actual selection path end to end.
type realEngine struct {
	rec   *cliSelectionRecorder
	mu    sync.Mutex
	paths []string
	m     *manager.Manager
	db    *state.Store
}

func startRealEngine(t *testing.T, dir string) *realEngine {
	t.Helper()
	// The run fixture seeds T-001 and T-UNRELATED with no description;
	// give each one so the recorder can name it.
	rel, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"T-001", "T-UNRELATED"} {
		task := rel.GetTask(id)
		task.Description = "work " + id
		if err := rel.UpdateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	rel.Close()

	db, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	e := &realEngine{rec: &cliSelectionRecorder{seen: map[string]bool{}}, db: db}
	e.m, err = manager.New(manager.Config{WorkDir: dir, StateStore: db, StageExecutor: e.rec})
	if err != nil {
		t.Fatal(err)
	}
	h := api.New(e.m, nil, nil, "", ":0").Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		e.paths = append(e.paths, r.URL.Path)
		e.mu.Unlock()
		if r.URL.Path == "/api/ready" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(func() { srv.Close(); e.close() })
	u, _ := url.Parse(srv.URL)
	pid := fmt.Sprintf("%d:%s", os.Getpid(), u.Port())
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.pid"), []byte(pid), 0o600); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *realEngine) close() {
	if e.m != nil {
		e.m.Close()
		e.m = nil
	}
	if e.db != nil {
		e.db.Close()
		e.db = nil
	}
}

func (e *realEngine) posted() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []string
	for _, p := range e.paths {
		if p != "/api/ready" {
			out = append(out, p)
		}
	}
	return out
}

func (e *realEngine) status(t *testing.T, id string) string {
	t.Helper()
	rel, err := e.m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	snap, err := rel.TaskSnapshot(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return snap.Status
}

func reloadStatus(t *testing.T, dir, id string) string {
	t.Helper()
	rel, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel.Close()
	return rel.GetTask(id).Status
}

// `openexec run T-001` against the real engine executes T-001 only; the
// unrelated pending task is still pending after the ledger is reopened.
func TestRunTaskIDExecutesOnlyThatTaskThroughRealEngine(t *testing.T) {
	f := newRunFixture(t, false)
	e := startRealEngine(t, f.dir)
	if out, err := f.run(t, "T-001"); err != nil {
		t.Fatalf("run T-001: %v\n%s", err, out)
	}
	if got := e.posted(); !reflect.DeepEqual(got, []string{"/api/v1/runs:execute"}) {
		t.Fatalf("posted %v; want execute only (no plan/import)", got)
	}
	deadline := time.Now().Add(30 * time.Second)
	for e.status(t, "T-001") != "done" {
		if time.Now().After(deadline) {
			t.Fatalf("T-001 did not finish; recorder %v", e.rec.tasks())
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond) // the old scheduler dispatched T-UNRELATED next
	if got := e.rec.tasks(); !reflect.DeepEqual(got, []string{"T-001"}) {
		t.Fatalf("recorder ran %v; want exactly [T-001]", got)
	}
	e.close()
	if st := reloadStatus(t, f.dir, "T-UNRELATED"); st != "pending" {
		t.Fatalf("T-UNRELATED = %q after reload; want pending", st)
	}
	if st := reloadStatus(t, f.dir, "T-001"); st != "done" {
		t.Fatalf("T-001 = %q after reload; want done", st)
	}
}

// An unknown task is refused by the engine before anything runs, and the
// engine's reason reaches the user instead of a bare status code.
func TestRunUnknownTaskReportsEngineRefusal(t *testing.T) {
	f := newRunFixture(t, false)
	e := startRealEngine(t, f.dir)
	out, err := f.run(t, "T-404")
	if err == nil {
		t.Fatalf("run T-404 succeeded; want a refusal\n%s", out)
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "unknown task T-404") {
		t.Fatalf("error = %q; want the engine's 400 reason naming T-404", err)
	}
	if got := e.posted(); !reflect.DeepEqual(got, []string{"/api/v1/runs:execute"}) {
		t.Fatalf("posted %v; want execute only (no plan/import)", got)
	}
	time.Sleep(300 * time.Millisecond)
	if got := e.rec.tasks(); len(got) != 0 {
		t.Fatalf("refused run executed %v", got)
	}
	for _, id := range []string{"T-001", "T-UNRELATED"} {
		if st := e.status(t, id); st != "pending" {
			t.Fatalf("%s = %q; want pending", id, st)
		}
	}
}

// A rejection without a JSON error body still reports the status code.
func TestRunRejectionWithoutReasonKeepsStatus(t *testing.T) {
	f := newRunFixture(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ready" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("not json"))
	}))
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	if err := os.WriteFile(filepath.Join(f.dir, ".openexec", "openexec.pid"), []byte(fmt.Sprintf("%d:%s", os.Getpid(), u.Port())), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := f.run(t, "T-001")
	if err == nil || err.Error() != "server rejected execution request: 500" {
		t.Fatalf("error = %v; want the bare status", err)
	}
}

func TestRunHelpDescribesTaskSelection(t *testing.T) {
	f := newRunFixture(t, true)
	out, err := f.run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Execute only existing pending task T-001 (no planning)") {
		t.Fatalf("help does not describe the task selection:\n%s", out)
	}
}
