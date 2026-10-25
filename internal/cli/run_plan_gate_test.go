package cli

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
)

func countPath(reqs []runRequest, path string) int {
	n := 0
	for _, r := range reqs {
		if r.Path == path {
			n++
		}
	}
	return n
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// assertPlanGateBacklogUntouched reopens the fixture's ledger and requires every
// pre-existing task to still be pending.
func assertPlanGateBacklogUntouched(t *testing.T, dir string) {
	t.Helper()
	backlog, err := release.NewManager(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backlog.Close()
	if err := backlog.Load(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"T-001", "T-UNRELATED"} {
		task := backlog.GetTask(id)
		if task == nil {
			t.Fatalf("task %s disappeared from the ledger", id)
		}
		if task.Status != release.TaskStatusPending {
			t.Fatalf("task %s status = %q after a refused run; want pending", id, task.Status)
		}
	}
}

func jsonReply(status int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

// Fixture daemon answers to runs:plan that are not an accepted plan. Each
// must stop the invocation before runs:execute, so the unrelated pending
// backlog never starts.
func TestRunStopsWhenAutoPlanningFailsOrIsRejected(t *testing.T) {
	cases := []struct {
		name    string
		plan    http.HandlerFunc
		wantErr []string
		wantOut []string
	}{
		{
			name:    "valid false with issues",
			plan:    jsonReply(http.StatusOK, `{"plan":null,"valid":false,"issues":["Document title not found","Goals section not found"]}`),
			wantErr: []string{"intent validation failed", "nothing was executed"},
			wantOut: []string{"PLANNING GATE FAILED", "- Document title not found", "- Goals section not found"},
		},
		{
			// Reviewed route: a plan exists but the reviewer rejected it.
			name:    "valid false with a rejected plan present",
			plan:    jsonReply(http.StatusOK, `{"plan":{"goals":[{"id":"G-001","title":"x"}],"stories":[]},"valid":false,"issues":["reviewer: not a vertical slice"]}`),
			wantErr: []string{"intent validation failed"},
			wantOut: []string{"PLANNING GATE FAILED", "- reviewer: not a vertical slice"},
		},
		{
			name:    "400 input error",
			plan:    jsonReply(http.StatusBadRequest, `{"error":"intent file not found: INTENT.md"}`),
			wantErr: []string{"server returned 400", "intent file not found: INTENT.md", "nothing was executed"},
		},
		{
			name:    "500 planner error",
			plan:    jsonReply(http.StatusInternalServerError, `{"error":"planner failed: provider unavailable"}`),
			wantErr: []string{"server returned 500", "planner failed: provider unavailable"},
		},
		{
			name:    "non-JSON error body",
			plan:    jsonReply(http.StatusBadGateway, "upstream down"),
			wantErr: []string{"server returned 502", "upstream down"},
		},
		{
			name:    "200 with a non-JSON body",
			plan:    jsonReply(http.StatusOK, "<html>not a plan</html>"),
			wantErr: []string{"invalid plan response"},
		},
		{
			name:    "200 with an empty body",
			plan:    jsonReply(http.StatusOK, ""),
			wantErr: []string{"invalid plan response"},
		},
		{
			name:    "valid true without a plan",
			plan:    jsonReply(http.StatusOK, `{"plan":null,"valid":true}`),
			wantErr: []string{"server returned no plan"},
		},
		{
			name: "connection dropped",
			plan: func(w http.ResponseWriter, _ *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err == nil {
					_ = conn.Close()
				}
			},
			wantErr: []string{"auto-planning failed"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunFixture(t, true)
			f.daemon.plan = tc.plan
			configPath := filepath.Join(f.dir, ".openexec", "config.json")
			configBefore := readFile(t, configPath)

			out, err := f.run(t)
			reqs := f.daemon.recorded()
			if n := countPath(reqs, "/api/v1/runs:execute"); n != 0 {
				t.Fatalf("failed plan was followed by %d execute request(s):%s", n, describeRequests(reqs))
			}
			if countPath(reqs, "/api/v1/runs:plan") != 1 {
				t.Fatalf("requests = %s; want exactly one plan request", describeRequests(reqs))
			}
			if err == nil {
				t.Fatalf("run succeeded after a failed plan\n%s", out)
			}
			for _, want := range tc.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error = %q; want it to contain %q", err, want)
				}
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(out, want) {
					t.Fatalf("output missing %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "Execution orchestrated by daemon") {
				t.Fatalf("refused run reported orchestration:\n%s", out)
			}
			if readFile(t, configPath) != configBefore {
				t.Fatal("refused run changed .openexec/config.json")
			}
			assertPlanGateBacklogUntouched(t, f.dir)
		})
	}
}

// The accepted-plan and --no-auto-plan controls keep today's requests.
func TestRunAcceptedPlanAndNoAutoPlanStillExecute(t *testing.T) {
	t.Run("valid plan executes once with the existing body", func(t *testing.T) {
		f := newRunFixture(t, true)
		out, err := f.run(t)
		if err != nil {
			t.Fatalf("run: %v\n%s", err, out)
		}
		reqs := f.daemon.recorded()
		if countPath(reqs, "/api/v1/runs:plan") != 1 || countPath(reqs, "/api/v1/runs:execute") != 1 {
			t.Fatalf("requests = %s; want one plan and one execute", describeRequests(reqs))
		}
		exec := reqs[len(reqs)-1]
		if exec.Path != "/api/v1/runs:execute" || exec.Body["mode"] != "danger-full-access" || len(exec.Body) != 3 {
			t.Fatalf("execute = %s; want the {task_oriented, worker_count, mode} body", describeRequests([]runRequest{exec}))
		}
		// The task queue is the repairing route and runs one task at a time.
		if exec.Body["task_oriented"] != true || exec.Body["worker_count"] != float64(1) {
			t.Fatalf("execute body does not ask for the task queue: %v", exec.Body)
		}
		if !strings.Contains(out, "Execution orchestrated by daemon") {
			t.Fatalf("output = %s", out)
		}
	})
	t.Run("no-auto-plan skips planning even when planning would fail", func(t *testing.T) {
		f := newRunFixture(t, true)
		f.daemon.plan = jsonReply(http.StatusOK, `{"valid":false,"issues":["must not be asked"]}`)
		out, err := f.run(t, "--no-auto-plan")
		if err != nil {
			t.Fatalf("run --no-auto-plan: %v\n%s", err, out)
		}
		reqs := f.daemon.recorded()
		if countPath(reqs, "/api/v1/runs:plan") != 0 || countPath(reqs, "/api/v1/runs:execute") != 1 {
			t.Fatalf("requests = %s; want execute only", describeRequests(reqs))
		}
	})
}

// The real daemon composition (server.New) and its runs:plan handler, with an
// invalid INTENT.md or a missing one, behind the project's PID file. The
// reviewed route refuses either before any planner or reviewer call (HTTP
// 400 naming what is wrong), and the unrelated pending backlog must not start.
func TestRunRealHandlerRefusesFailedPlanBeforeExecute(t *testing.T) {
	cases := []struct {
		name    string
		intent  *string
		wantErr []string
	}{
		{"invalid intent is refused with its issues", strPtr("no heading, no goals, no requirements\n"), []string{"server returned 400", "intent validation failed", "Document title not found", "Goals section not found"}},
		{"missing intent returns 400", nil, []string{"server returned 400", "INTENT.md"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunFixture(t, false)
			p := installPlannerFixture(t, f.dir)
			p.answer(t, "generate", incompletePlan)
			p.answer(t, "review", approvingReview)
			intentPath := filepath.Join(f.dir, "INTENT.md")
			_ = os.Remove(intentPath)
			if tc.intent != nil {
				if err := os.WriteFile(intentPath, []byte(*tc.intent), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			configPath := filepath.Join(f.dir, ".openexec", "config.json")
			configBefore := readFile(t, configPath)
			e := startReviewedEngine(t, f.dir)

			out, runErr := f.run(t)

			reqs := e.recorded()
			if n := countPath(reqs, "/api/v1/runs:execute"); n != 0 {
				t.Fatalf("real handler path sent %d execute request(s) after a failed plan:%s", n, describeRequests(reqs))
			}
			if countPath(reqs, "/api/v1/runs:plan") != 1 {
				t.Fatalf("requests = %s; want one plan request (err=%v)\n%s", describeRequests(reqs), runErr, out)
			}
			for _, want := range append(tc.wantErr, "nothing was executed") {
				if runErr == nil || !strings.Contains(runErr.Error(), want) {
					t.Fatalf("error = %v; want %q\n%s", runErr, want, out)
				}
			}
			if calls := p.calls(t); len(calls) != 0 {
				t.Fatalf("planner or reviewer was called for a refused intent: %v", calls)
			}
			if receipts := planReceipts(t, e.srv.StateStore.GetDB()); len(receipts) != 0 {
				t.Fatalf("a refused intent left a plan receipt: %v", receipts)
			}
			if _, err := os.Stat(filepath.Join(f.dir, ".openexec", "artifacts", "plans")); !os.IsNotExist(err) {
				t.Fatalf("a plan artifact directory appeared: %v", err)
			}
			if readFile(t, configPath) != configBefore {
				t.Fatal("refused run changed .openexec/config.json")
			}
			assertPlanGateBacklogUntouched(t, f.dir)
		})
	}
}

func strPtr(s string) *string { return &s }
