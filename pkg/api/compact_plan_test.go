package api

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/manager"
	"github.com/openexec/openexec/pkg/runtime"
)

type compactCompletion func(context.Context, string) (string, error)

func (f compactCompletion) Complete(c context.Context, p string) (string, error) { return f(c, p) }

type compactStage func(context.Context, *runtime.Stage, *runtime.StageInput) (*runtime.StageResult, error)

func (f compactStage) Execute(c context.Context, s *runtime.Stage, i *runtime.StageInput) (*runtime.StageResult, error) {
	return f(c, s, i)
}

func TestCompactPlanAndNativeExecutionThroughHTTP(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".openexec"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := filepath.Join(dir, "label.txt")
	if err := os.WriteFile(path, []byte("Old label"), 0600); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(manager.Config{WorkDir: dir, StateStore: db,
		PlanGenerator: compactCompletion(func(_ context.Context, p string) (string, error) {
			if !strings.Contains(p, "SMALL, well-scoped change") {
				t.Error("HTTP compact selection lost")
			}
			return `{"goals":[{"id":"G","title":"Rename"}],"stories":[{"id":"S","goal_id":"G","title":"Rename","tasks":[{"id":"T","title":"Rename and verify","description":"Replace Old label with New label","verification_script":"test -f label.txt"}]}]}`, nil
		}),
		PlanReviewer: compactCompletion(func(context.Context, string) (string, error) {
			return `{"approved":true,"assessment":"One independently reviewed vertical task"}`, nil
		}),
		StageExecutor: compactStage(func(_ context.Context, s *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
			if s.Name == "implement" {
				if err := os.WriteFile(path, []byte("New label"), 0600); err != nil {
					return nil, err
				}
			}
			if s.Name == "test" {
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != "New label" {
					return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusFailed, Error: "label did not persist"}, nil
				}
			}
			return &runtime.StageResult{StageName: s.Name, Status: runtime.StageStatusCompleted, Attempt: 1}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	s := New(m, nil, nil, "", ":0")
	call := func(route, body string) *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		s.Handler().ServeHTTP(r, httptest.NewRequest("POST", route, strings.NewReader(body)))
		return r
	}
	request := `{"request_id":"compact-label","intent":"Existing label.txt contains Old label. Replace with New label and verify persistence.","compact":true,"review":true,"auto_import":true,"no_validate":true}`
	r := call("/api/v1/runs:plan", request)
	if r.Code != 200 {
		t.Fatal(r.Code, r.Body.String())
	}
	r = call("/api/v1/runs:execute", `{"task_oriented":true,"story_ids":["S"],"max_parallel":1}`)
	if r.Code != 202 {
		t.Fatal(r.Code, r.Body.String())
	}
	backlog, err := m.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.After(5 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-deadline:
			t.Fatal("native task did not converge")
		case <-tick.C:
			task, err := backlog.TaskSnapshot(context.Background(), "T")
			if err != nil {
				t.Fatal(err)
			}
			if task.Status == "done" {
				raw, err := os.ReadFile(path)
				if err != nil || string(raw) != "New label" {
					t.Fatal("durable product absent", err)
				}
				return
			}
		}
	}
}
