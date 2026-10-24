package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/runtime"
)

// selectionRecorder is a deterministic stage-executor fixture, not a
// provider: it completes every stage and records which task each stage ran
// for, in first-seen order. Task descriptions are "work <task id>".
type selectionRecorder struct {
	mu    sync.Mutex
	order []string
	seen  map[string]bool
}

func (r *selectionRecorder) Execute(_ context.Context, s *runtime.Stage, in *runtime.StageInput) (*runtime.StageResult, error) {
	id := strings.TrimPrefix(in.TaskDescription, "work ")
	r.mu.Lock()
	if !r.seen[id] {
		r.seen[id] = true
		r.order = append(r.order, id)
	}
	r.mu.Unlock()
	return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
}

func (r *selectionRecorder) ran() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.order...)
}

type selectionFixture struct {
	dir string
	db  *state.Store
	m   *manager.Manager
	rec *selectionRecorder
	srv *Server
}

// newSelectionFixture seeds a real SQLite ledger with:
//
//	US-001: T-001, T-002 (unrelated), T-003 (depends on T-001),
//	        T-004 (depends on T-002), T-DONE (done), T-PROG (in_progress),
//	        T-HITL (hitl)
//	US-002 (depends on story US-001): T-005
//	US-003: T-007;  US-004 (depends on story US-003): T-008
func newSelectionFixture(t *testing.T) *selectionFixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	rec := &selectionRecorder{seen: map[string]bool{}}
	m, err := manager.New(manager.Config{WorkDir: dir, StateStore: db, StageExecutor: rec})
	if err != nil {
		t.Fatal(err)
	}
	f := &selectionFixture{dir: dir, db: db, m: m, rec: rec, srv: New(m, nil, nil, "", ":0")}
	t.Cleanup(f.close)

	rel, err := m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []*release.Story{
		{ID: "US-001", Title: "Selected story", Status: "pending"},
		{ID: "US-002", Title: "Later story", Status: "pending", DependsOn: []string{"US-001"}},
		{ID: "US-003", Title: "Prerequisite story", Status: "pending"},
		{ID: "US-004", Title: "Dependent story", Status: "pending", DependsOn: []string{"US-003"}},
	} {
		if err := rel.CreateStory(s); err != nil {
			t.Fatal(err)
		}
	}
	tasks := []*release.Task{
		{ID: "T-001", StoryID: "US-001", Status: "pending"},
		{ID: "T-002", StoryID: "US-001", Status: "pending"},
		{ID: "T-003", StoryID: "US-001", Status: "pending", DependsOn: []string{"T-001"}},
		{ID: "T-004", StoryID: "US-001", Status: "pending", DependsOn: []string{"T-002"}},
		{ID: "T-DONE", StoryID: "US-001", Status: "done"},
		{ID: "T-PROG", StoryID: "US-001", Status: "in_progress"},
		{ID: "T-HITL", StoryID: "US-001", Status: "pending", Metadata: map[string]any{"mode": release.TaskModeHITL}},
		{ID: "T-005", StoryID: "US-002", Status: "pending"},
		{ID: "T-007", StoryID: "US-003", Status: "pending"},
		{ID: "T-008", StoryID: "US-004", Status: "pending"},
	}
	for _, task := range tasks {
		task.Title = "Task " + task.ID
		task.Description = "work " + task.ID
		if err := rel.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *selectionFixture) close() {
	if f.m != nil {
		f.m.Close()
		f.m = nil
	}
	if f.db != nil {
		f.db.Close()
		f.db = nil
	}
}

func (f *selectionFixture) execute(body string) *httptest.ResponseRecorder {
	r := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(r, httptest.NewRequest("POST", "/api/v1/runs:execute", strings.NewReader(body)))
	return r
}

func (f *selectionFixture) statuses(t *testing.T) map[string]string {
	t.Helper()
	rel, err := f.m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, task := range rel.GetTasks() {
		snap, err := rel.TaskSnapshot(context.Background(), task.ID)
		if err != nil {
			t.Fatal(err)
		}
		out[task.ID] = snap.Status
	}
	return out
}

// awaitDone waits until every id is done in the ledger, then gives the
// scheduler a grace period in which the old code dispatched the next
// unrelated pending task.
func (f *selectionFixture) awaitDone(t *testing.T, ids ...string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		st := f.statuses(t)
		all := true
		for _, id := range ids {
			if st[id] != "done" {
				all = false
			}
		}
		if all {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("selected tasks %v did not finish; statuses %v, recorder %v", ids, st, f.rec.ran())
		}
		time.Sleep(20 * time.Millisecond)
	}
	time.Sleep(1500 * time.Millisecond)
}

// reopenStatuses closes the manager and store and reads the ledger again
// from the same database file, as a restarted daemon would.
func (f *selectionFixture) reopenStatuses(t *testing.T) map[string]string {
	t.Helper()
	f.close()
	rel, err := release.NewManager(f.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel.Close()
	out := map[string]string{}
	for _, task := range rel.GetTasks() {
		out[task.ID] = task.Status
	}
	return out
}

// Reproduction and fix: the old handler decoded task_ids into a RunOptions
// without that field and the scheduler ran the whole pending backlog, so the
// recorder saw T-002 (and the others). With the selection honoured only
// T-001 runs, and every unrelated task is still pending after a reload.
func TestRunsExecuteRunsOnlySelectedTask(t *testing.T) {
	f := newSelectionFixture(t)
	r := f.execute(`{"task_ids":["T-001"],"worker_count":1}`)
	if r.Code != 202 {
		t.Fatalf("status %d: %s", r.Code, r.Body.String())
	}
	f.awaitDone(t, "T-001")
	if got := f.rec.ran(); !reflect.DeepEqual(got, []string{"T-001"}) {
		t.Fatalf("recorder ran %v; want exactly [T-001] (unrelated pending tasks were executed)", got)
	}
	st := f.reopenStatuses(t)
	if st["T-001"] != "done" {
		t.Fatalf("T-001 = %q after reload; want done", st["T-001"])
	}
	for _, id := range []string{"T-002", "T-003", "T-004", "T-HITL", "T-005", "T-007", "T-008"} {
		if st[id] != "pending" {
			t.Fatalf("%s = %q after reload; want pending (unselected)", id, st[id])
		}
	}
}

func TestRunsExecuteRunsSelectedSetInDependencyOrder(t *testing.T) {
	f := newSelectionFixture(t)
	// T-003 is listed first but depends on T-001, also selected.
	r := f.execute(`{"task_ids":["T-003","T-001"],"worker_count":2}`)
	if r.Code != 202 {
		t.Fatalf("status %d: %s", r.Code, r.Body.String())
	}
	f.awaitDone(t, "T-001", "T-003")
	if got := f.rec.ran(); !reflect.DeepEqual(got, []string{"T-001", "T-003"}) {
		t.Fatalf("recorder ran %v; want [T-001 T-003] in dependency order", got)
	}
	st := f.reopenStatuses(t)
	for _, id := range []string{"T-002", "T-004", "T-005", "T-007", "T-008"} {
		if st[id] != "pending" {
			t.Fatalf("%s = %q after reload; want pending", id, st[id])
		}
	}
}

// Every selection the scheduler cannot honour is refused with 400 before any
// task runs or any status changes.
func TestRunsExecuteRefusesUnusableSelection(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"unknown id", `{"task_ids":["T-404"]}`, "unknown task T-404"},
		{"one unknown among known", `{"task_ids":["T-001","T-404"]}`, "unknown task T-404"},
		{"explicit empty", `{"task_ids":[]}`, "task_ids is empty"},
		{"explicit null", `{"task_ids":null}`, "task_ids is empty"},
		{"blank id", `{"task_ids":["  "]}`, "blank task id"},
		{"duplicate id", `{"task_ids":["T-001","T-001"]}`, "duplicate task id T-001"},
		{"not an array", `{"task_ids":"T-001"}`, "invalid request body"},
		{"not strings", `{"task_ids":[1]}`, "invalid request body"},
		{"done task", `{"task_ids":["T-DONE"]}`, "task T-DONE is done, not pending"},
		{"in progress task", `{"task_ids":["T-PROG"]}`, "task T-PROG is in_progress, not pending"},
		{"hitl task", `{"task_ids":["T-HITL"]}`, "task T-HITL requires a human in the loop"},
		{"unmet task dependency", `{"task_ids":["T-004"]}`, "task T-004 depends on T-002"},
		{"depends on hitl via story", `{"task_ids":["T-005"]}`, "task T-005 depends on a human-in-the-loop task"},
		{"unmet story dependency", `{"task_ids":["T-008"]}`, "story US-004 depends on story US-003, whose task T-007 is pending and not selected"},
		{"one refused among valid", `{"task_ids":["T-001","T-004"]}`, "task T-004 depends on T-002"},
		{"with task_oriented", `{"task_ids":["T-001"],"task_oriented":true}`, "task_ids cannot be combined"},
		{"with story_ids", `{"task_ids":["T-001"],"story_ids":["US-001"]}`, "task_ids cannot be combined"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSelectionFixture(t)
			before := f.statuses(t)
			r := f.execute(tc.body)
			if r.Code != 400 {
				t.Fatalf("status %d (%s); want 400", r.Code, r.Body.String())
			}
			if !strings.Contains(r.Body.String(), tc.want) {
				t.Fatalf("body %s; want it to contain %q", r.Body.String(), tc.want)
			}
			time.Sleep(300 * time.Millisecond)
			if got := f.rec.ran(); len(got) != 0 {
				t.Fatalf("refused selection executed %v", got)
			}
			if after := f.statuses(t); !reflect.DeepEqual(before, after) {
				t.Fatalf("refused selection changed statuses:\nbefore %v\nafter  %v", before, after)
			}
		})
	}
}

// Near miss for the task-dependency rule: a dependency that is already done
// is satisfied without being selected.
func TestRunsExecuteAcceptsDependencyAlreadyDone(t *testing.T) {
	f := newSelectionFixture(t)
	rel, _ := f.m.GetInternalReleaseManager()
	if err := rel.CreateTask(&release.Task{ID: "T-006", Title: "Task T-006", Description: "work T-006", StoryID: "US-001", Status: "pending", DependsOn: []string{"T-DONE"}}); err != nil {
		t.Fatal(err)
	}
	r := f.execute(`{"task_ids":["T-006"]}`)
	if r.Code != 202 {
		t.Fatalf("status %d: %s", r.Code, r.Body.String())
	}
	f.awaitDone(t, "T-006")
	if got := f.rec.ran(); !reflect.DeepEqual(got, []string{"T-006"}) {
		t.Fatalf("recorder ran %v; want [T-006]", got)
	}
}

// Unscoped execution is unchanged: every ready auto-dispatchable pending task
// runs and the hitl task is still held back.
func TestRunsExecuteUnscopedRunsWholeBacklog(t *testing.T) {
	f := newSelectionFixture(t)
	r := f.execute(`{"worker_count":1}`)
	if r.Code != 202 {
		t.Fatalf("status %d: %s", r.Code, r.Body.String())
	}
	f.awaitDone(t, "T-001", "T-002", "T-003", "T-004", "T-007", "T-008")
	got := f.rec.ran()
	// T-005's story depends on US-001, which holds a hitl task, so the
	// existing scheduler holds it back too.
	want := map[string]bool{"T-001": true, "T-002": true, "T-003": true, "T-004": true, "T-007": true, "T-008": true}
	if len(got) != len(want) {
		t.Fatalf("recorder ran %v; want the six afk pending tasks", got)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("recorder ran %v; %s should not run", got, id)
		}
	}
	st := f.reopenStatuses(t)
	for _, id := range []string{"T-HITL", "T-005"} {
		if st[id] != "pending" {
			t.Fatalf("%s = %q; want pending (held back)", id, st[id])
		}
	}
}

// Near miss for the story-dependency rule: selecting the prerequisite story's
// unfinished task together with the dependent task is accepted and runs in
// story order.
func TestRunsExecuteAcceptsStoryDependencySelectedTogether(t *testing.T) {
	f := newSelectionFixture(t)
	r := f.execute(`{"task_ids":["T-008","T-007"],"worker_count":2}`)
	if r.Code != 202 {
		t.Fatalf("status %d: %s", r.Code, r.Body.String())
	}
	f.awaitDone(t, "T-007", "T-008")
	if got := f.rec.ran(); !reflect.DeepEqual(got, []string{"T-007", "T-008"}) {
		t.Fatalf("recorder ran %v; want [T-007 T-008]", got)
	}
}
