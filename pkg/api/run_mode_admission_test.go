package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/session"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/runtime"
)

// modeTestRunner is a deterministic stage-executor test fixture, not a
// provider: it completes every stage without calling a model and records
// which runs reached execution.
type modeTestRunner struct {
	mu   sync.Mutex
	runs map[string]bool
}

func (r *modeTestRunner) Execute(_ context.Context, s *runtime.Stage, in *runtime.StageInput) (*runtime.StageResult, error) {
	r.mu.Lock()
	r.runs[in.RunID] = true
	r.mu.Unlock()
	return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
}

func (r *modeTestRunner) executed() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.runs)
}

// modeFixture is a disposable project behind the real Server mux: a real
// manager and SQLite state store, an audit logger and the labelled runner.
type modeFixture struct {
	dbPath string
	db     *state.Store
	audit  *audit.AuditLogger
	m      *manager.Manager
	runner *modeTestRunner
	srv    *Server
}

func newModeFixture(t *testing.T) *modeFixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0700); err != nil {
		t.Fatal(err)
	}
	f := &modeFixture{dbPath: filepath.Join(dir, ".openexec", "openexec.db"), runner: &modeTestRunner{runs: map[string]bool{}}}
	var err error
	if f.db, err = state.NewStore(f.dbPath); err != nil {
		t.Fatal(err)
	}
	// A RunSpec belongs to a session in the state database.
	if _, err := f.db.GetDB().Exec(`INSERT INTO sessions (id, project_path, provider, model) VALUES ('sess-1', ?, 'labelled-test', 'labelled-test')`, dir); err != nil {
		t.Fatal(err)
	}
	if f.audit, err = audit.NewLogger(filepath.Join(dir, ".openexec", "audit.db")); err != nil {
		t.Fatal(err)
	}
	if f.m, err = manager.New(manager.Config{WorkDir: dir, StateStore: f.db, StageExecutor: f.runner}); err != nil {
		t.Fatal(err)
	}
	repo := newMockSessionRepo()
	repo.sessions["sess-1"] = &session.Session{ID: "sess-1", Status: session.StatusActive, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.srv = New(f.m, repo, f.audit, dir, ":0", WithStateStore(f.db))
	t.Cleanup(func() {
		f.m.Close()
		_ = f.audit.Close()
		_ = f.db.Close()
	})
	return f
}

func (f *modeFixture) post(t *testing.T, path string, body map[string]any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	w := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(string(raw))))
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// persistedRunMode reopens the state database and returns the mode the
// manager recorded for the run, which is the mode its pipeline ran with.
func (f *modeFixture) persistedRunMode(t *testing.T, runID string) string {
	t.Helper()
	f.waitTerminal(t, runID)
	reopened, err := state.NewStore(f.dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	run, err := reopened.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if run == nil {
		t.Fatalf("run %s was not persisted", runID)
	}
	return run.Mode
}

// waitTerminal lets the started pipeline finish under the labelled runner
// so the fixture can be torn down without a run still in flight.
func (f *modeFixture) waitTerminal(t *testing.T, runID string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := f.m.Status(runID); err == nil {
			switch info.Status {
			case manager.StatusComplete, manager.StatusError, manager.StatusStopped:
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish", runID)
}

func (f *modeFixture) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := f.db.GetDB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *modeFixture) auditModes(t *testing.T) []string {
	t.Helper()
	res, err := f.audit.Query(context.Background(), &audit.QueryFilter{EventTypes: []audit.EventType{audit.EventRunCreated}})
	if err != nil {
		t.Fatal(err)
	}
	var modes []string
	for _, e := range res.Entries {
		var md struct {
			Mode string `json:"mode"`
		}
		if err := e.GetMetadata(&md); err != nil {
			t.Fatal(err)
		}
		modes = append(modes, md.Mode)
	}
	return modes
}

// modeRoute drives one public run route and returns the effective mode it
// persisted. An empty mode argument omits the field from the request.
type modeRoute struct {
	name string
	// call returns the HTTP status and the route's run identifier.
	call func(t *testing.T, f *modeFixture, mode *string) (int, string)
	// effective re-reads the persisted mode for a successful call.
	effective func(f *modeFixture, t *testing.T, id string) string
	// audited reports whether the route writes a run.created audit entry.
	audited bool
}

func withMode(body map[string]any, mode *string) map[string]any {
	if mode != nil {
		body["mode"] = *mode
	}
	return body
}

func runIDFrom(out map[string]any, key string) string {
	s, _ := out[key].(string)
	return s
}

var modeRoutes = []modeRoute{
	{
		name: "POST /api/v1/runs",
		call: func(t *testing.T, f *modeFixture, mode *string) (int, string) {
			code, out := f.post(t, "/api/v1/runs", withMode(map[string]any{"intent": "labelled test intent", "session_id": "sess-1"}, mode))
			return code, runIDFrom(out, "spec_id")
		},
		effective: func(f *modeFixture, t *testing.T, specID string) string {
			reopened, err := state.NewStore(f.dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			spec, err := reopened.GetRunSpec(context.Background(), specID)
			if err != nil || spec == nil {
				t.Fatalf("RunSpec %q not persisted: %v", specID, err)
			}
			return spec.Mode
		},
		audited: true,
	},
	{
		name: "POST /api/v1/runs/{id}/start",
		call: func(t *testing.T, f *modeFixture, mode *string) (int, string) {
			code, out := f.post(t, "/api/v1/runs/T-MODE/start", withMode(map[string]any{"task_description": "labelled test task"}, mode))
			return code, runIDFrom(out, "run_id")
		},
		effective: (*modeFixture).persistedRunMode,
	},
	{
		name: "POST /api/v1/runs/parallel",
		call: func(t *testing.T, f *modeFixture, mode *string) (int, string) {
			code, out := f.post(t, "/api/v1/runs/parallel", withMode(map[string]any{"run_ids": []string{"T-PAR"}}, mode))
			if code == http.StatusAccepted {
				runs, _ := out["runs"].([]any)
				if len(runs) != 1 || runs[0].(map[string]any)["status"] != "starting" {
					t.Fatalf("parallel run did not start: %v", out)
				}
				return code, "T-PAR"
			}
			return code, ""
		},
		effective: (*modeFixture).persistedRunMode,
	},
	{
		name: "POST /api/v1/runs:blueprint",
		call: func(t *testing.T, f *modeFixture, mode *string) (int, string) {
			code, out := f.post(t, "/api/v1/runs:blueprint", withMode(map[string]any{"task_description": "labelled test task"}, mode))
			return code, runIDFrom(out, "run_id")
		},
		effective: (*modeFixture).persistedRunMode,
		audited:   true,
	},
	{
		name: "POST /api/v1/sessions/{id}/run",
		call: func(t *testing.T, f *modeFixture, mode *string) (int, string) {
			code, out := f.post(t, "/api/v1/sessions/sess-1/run", withMode(map[string]any{"task_description": "labelled test task"}, mode))
			return code, runIDFrom(out, "run_id")
		},
		effective: (*modeFixture).persistedRunMode,
	},
}

func strp(s string) *string { return &s }

// TestRunRoutesAdmitExecMode drives every public run route through the real
// mux and re-reads the mode the run was persisted with. Omission must take
// the documented workspace-write default, never danger-full-access.
func TestRunRoutesAdmitExecMode(t *testing.T) {
	accepted := []struct {
		name string
		mode *string
		want string
	}{
		{"omitted", nil, "workspace-write"},
		{"empty", strp(""), "workspace-write"},
		{"read-only", strp("read-only"), "read-only"},
		{"workspace-write", strp("workspace-write"), "workspace-write"},
		{"explicit danger-full-access", strp("danger-full-access"), "danger-full-access"},
	}
	for _, route := range modeRoutes {
		for _, tc := range accepted {
			t.Run(route.name+"/"+tc.name, func(t *testing.T) {
				f := newModeFixture(t)
				code, id := route.call(t, f, tc.mode)
				if code != http.StatusCreated && code != http.StatusAccepted {
					t.Fatalf("status = %d, want success", code)
				}
				if id == "" {
					t.Fatal("route returned no run identifier")
				}
				if got := route.effective(f, t, id); got != tc.want {
					t.Fatalf("effective persisted mode = %q, want %q", got, tc.want)
				}
				if route.audited {
					if got := f.auditModes(t); len(got) != 1 || got[0] != tc.want {
						t.Fatalf("audited mode = %v, want [%q]", got, tc.want)
					}
				}
			})
		}
	}
}

// TestRunRoutesRefuseInvalidExecMode: anything that is not an execution mode
// is refused with 400 before a RunSpec, audit entry, run record or runner
// execution exists. Near misses of the valid names must not match.
func TestRunRoutesRefuseInvalidExecMode(t *testing.T) {
	for _, route := range modeRoutes {
		for _, bad := range []string{"full-access", "Danger-Full-Access", "suggest", "read-only "} {
			t.Run(route.name+"/"+bad, func(t *testing.T) {
				f := newModeFixture(t)
				code, _ := route.call(t, f, strp(bad))
				if code != http.StatusBadRequest {
					t.Fatalf("status = %d, want 400", code)
				}
				if n := f.count(t, "runs"); n != 0 {
					t.Fatalf("%d run records persisted for a refused mode", n)
				}
				if n := f.count(t, "run_specs"); n != 0 {
					t.Fatalf("%d RunSpecs persisted for a refused mode", n)
				}
				if got := f.auditModes(t); len(got) != 0 {
					t.Fatalf("audit entries written for a refused mode: %v", got)
				}
				if n := f.runner.executed(); n != 0 {
					t.Fatalf("labelled runner executed %d runs for a refused mode", n)
				}
			})
		}
	}
}

func TestAdmitExecMode(t *testing.T) {
	for in, want := range map[string]string{"": "workspace-write", "read-only": "read-only", "workspace-write": "workspace-write", "danger-full-access": "danger-full-access"} {
		if got, err := admitExecMode(in); err != nil || got != want {
			t.Errorf("admitExecMode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"suggest", "auto-edit", "full", "READ-ONLY"} {
		if got, err := admitExecMode(bad); err == nil {
			t.Errorf("admitExecMode(%q) = %q, want refusal", bad, got)
		}
	}
}
