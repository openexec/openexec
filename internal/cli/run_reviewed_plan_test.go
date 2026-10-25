package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/pflag"

	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/internal/server"
	"github.com/openexec/openexec/pkg/db/state"
)

// Native CLI planning must go through the reviewed, retained plan route. These
// tests drive the real Cobra commands, the real daemon constructor
// (server.New) and its runs:plan handler, or the real `openexec plan`
// composition. Only the configured planner CLI is replaced: a DETERMINISTIC
// PLANNER/REVIEWER FIXTURE selected through OPENEXEC_PLANNER_CLI, which the
// native provider honours. It labels each call by the prompt it received
// (generate, review or fix), logs it, and answers from a file. No model is
// called.

const plannerFixtureScript = `#!/bin/sh
# DETERMINISTIC PLANNER/REVIEWER FIXTURE (openexec tests): no model is called.
d=$(dirname "$0")
in=$(mktemp)
cat > "$in"
case "$(head -n 1 "$in")" in
*"reviewing generated user stories"*) role=review ;;
*"fixing user stories"*) role=fix ;;
*"generating user stories"*) role=generate ;;
*) role=unknown ;;
esac
rm -f "$in"
printf '%s\n' "$role" >> "$d/calls.log"
[ -f "$d/$role.json" ] || { echo "fixture: no $role answer" >&2; exit 7; }
cat "$d/$role.json"
`

// The generated plan lacks the reload check the reviewer asks for; the fix
// adds it. Same IDs, so the fix is a changed plan for the same rows.
const (
	incompletePlan  = `{"goals":[{"id":"G-1","title":"Edit","description":"Validated edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Implement edit","description":"Implement and verify editing","mode":"afk"},{"id":"T-2","title":"Verify edit","description":"Verify the running editing journey","mode":"afk","depends_on":["T-1"]}]}]}`
	refinedPlan     = `{"goals":[{"id":"G-1","title":"Edit","description":"Validated edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Implement edit","description":"Implement and verify editing","mode":"afk"},{"id":"T-2","title":"Verify edit","description":"Verify the running editing journey and persistence after reload","mode":"afk","depends_on":["T-1"]}]}]}`
	historyPlan     = `{"goals":[{"id":"G-1","title":"Edit","description":"Validated edit"}],"stories":[{"id":"US-1","title":"Edit","goal_id":"G-1","verification_script":"go test ./...","tasks":[{"id":"T-1","title":"Implement edit","description":"Reworded: implement editing","mode":"afk"},{"id":"T-2","title":"Verify edit","description":"Verify the running editing journey and persistence after reload","mode":"afk","depends_on":["T-1"]},{"id":"T-3","title":"Record history","description":"Record append-only history","mode":"afk","depends_on":["T-2"]}]}]}`
	rejectingReview = `{"approved":false,"assessment":"Missing reload evidence","key_issues":["Verify persistence after reload"]}`
	approvingReview = `{"approved":true,"assessment":"Required verification represented"}`
)

const reviewedIntent = `# Editing

## Goals
- [ ] Goal 1: Edit documents

## Requirements
- [ ] US-001: As a user, I want to edit a document so that my change is kept.
  - Acceptance Criteria: Given a document, when I edit and reload it, then the edit persists.

## Constraints
- Platform: Linux
- Shape: CLI
- Data Source: Local SQLite
`

type plannerFixture struct{ dir, script string }

// installPlannerFixture selects the fixture as the configured planner CLI and
// as the project's runner, so the daemon composition resolves.
func installPlannerFixture(t *testing.T, projectDir string) *plannerFixture {
	t.Helper()
	p := &plannerFixture{dir: t.TempDir()}
	p.script = filepath.Join(p.dir, "planner-fixture")
	if err := os.WriteFile(p.script, []byte(plannerFixtureScript), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENEXEC_PLANNER_CLI", p.script)
	t.Setenv("OPENEXEC_PLANNER_ARGS", "")
	cfg, err := project.LoadProjectConfig(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.RunnerCommand = p.script
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "INTENT.md"), []byte(reviewedIntent), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (p *plannerFixture) answer(t *testing.T, role, body string) {
	t.Helper()
	path := filepath.Join(p.dir, role+".json")
	if body == "" {
		_ = os.Remove(path)
		return
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (p *plannerFixture) calls(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(p.dir, "calls.log"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(raw))
}

// reviewedEngine is the real daemon composition behind the project's PID
// file. Only runs:execute is answered here (and the readiness probe): it
// records what the ledger held when execution was asked for and starts
// nothing, because execution routing is not under test.
type reviewedEngine struct {
	srv       *server.Server
	mu        sync.Mutex
	requests  []runRequest
	atExecute []string // ledger tasks when each runs:execute arrived
}

func startReviewedEngine(t *testing.T, dir string) *reviewedEngine {
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
	e := &reviewedEngine{srv: srv}
	hs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		e.mu.Lock()
		e.requests = append(e.requests, runRequest{Path: r.URL.Path, Body: body})
		e.mu.Unlock()
		switch r.URL.Path {
		case "/api/ready":
			w.WriteHeader(http.StatusOK)
		case "/api/v1/runs:execute":
			tasks := ledgerTasks(t, srv.StateStore.GetDB())
			e.mu.Lock()
			e.atExecute = append(e.atExecute, tasks)
			e.mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		default:
			r.Body = io.NopCloser(bytes.NewReader(raw))
			srv.Mux.ServeHTTP(w, r)
		}
	}))
	t.Cleanup(func() { hs.Close(); srv.Mgr.Close(); _ = srv.StateStore.Close() })
	u, _ := url.Parse(hs.URL)
	if err := os.WriteFile(filepath.Join(dir, ".openexec", "openexec.pid"), []byte(fmt.Sprintf("%d:%s", os.Getpid(), u.Port())), 0o600); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *reviewedEngine) recorded() []runRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]runRequest(nil), e.requests...)
}

func (e *reviewedEngine) executeSnapshots() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.atExecute...)
}

// ledgerTasks lists the ledger's tasks as "ID=description" in ID order.
func ledgerTasks(t *testing.T, db *sql.DB) string {
	t.Helper()
	rows, err := db.Query(`SELECT id, description FROM tasks ORDER BY id`)
	if err != nil {
		t.Error(err)
		return ""
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, description string
		if err := rows.Scan(&id, &description); err != nil {
			t.Error(err)
			return ""
		}
		out = append(out, id+"="+description)
	}
	return strings.Join(out, "\n")
}

// ledgerDump renders every goal, story and task row, all columns, so a
// refusal can be required to leave the pending ledger byte-identical.
func ledgerDump(t *testing.T, db *sql.DB) string {
	t.Helper()
	var b strings.Builder
	for _, table := range []string{"goals", "stories", "tasks"} {
		rows, err := db.Query(`SELECT * FROM ` + table + ` ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&b, "%s %q\n", table, vals)
		}
		rows.Close()
	}
	return b.String()
}

func planReceipts(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT id || ':' || status FROM run_steps WHERE run_id LIKE 'plan-request-%' AND agent='reviewed-planner' ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

func planArtifacts(t *testing.T, dir string) (plans, reviews []string) {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(dir, ".openexec", "artifacts", "plans"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".review.json") {
			reviews = append(reviews, e.Name())
		} else {
			plans = append(plans, e.Name())
		}
	}
	return plans, reviews
}

// assertRetainedEvidence requires the output to name the retained artifact of
// the fixed plan and a retained approving review record of exactly that plan,
// and returns the review's path.
func assertRetainedEvidence(t *testing.T, out string) string {
	t.Helper()
	var planPath, reviewPath string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "Plan: "); ok {
			planPath = v
		}
		if v, ok := strings.CutPrefix(line, "Review: "); ok {
			reviewPath = v
		}
	}
	plan, err := os.ReadFile(planPath)
	if err != nil || !strings.Contains(string(plan), "persistence after reload") {
		t.Fatalf("output names no retained artifact of the fixed plan (%q: %v):\n%s", planPath, err, out)
	}
	review, err := os.ReadFile(reviewPath)
	digest := strings.TrimSuffix(filepath.Base(planPath), ".json")
	if err != nil || !strings.Contains(string(review), `"PlanDigest":"`+digest+`"`) || !strings.Contains(string(review), `"approved":true`) {
		t.Fatalf("output names no retained approving review of plan %s (%q: %v): %s", digest, reviewPath, err, review)
	}
	return reviewPath
}

func joinCalls(calls []string) string { return strings.Join(calls, ",") }

// fixedPlanImported is the ledger listing once the fixed plan's tasks are
// added to the rows in before, in the ledger's ID order.
func fixedPlanImported(before string) string {
	rows := append(strings.Split(before, "\n"),
		"T-1=Implement and verify editing",
		"T-2=Verify the running editing journey and persistence after reload")
	sort.Strings(rows)
	return strings.Join(rows, "\n")
}

// A bare run plans through the reviewed route before it asks for execution:
// generate, independent review, one fix, import, then execute. Repeating it
// replays the retained result; a changed INTENT.md is a new request.
func TestRunReviewsPlanBeforeImportAndExecute(t *testing.T) {
	f := newRunFixture(t, false)
	p := installPlannerFixture(t, f.dir)
	p.answer(t, "generate", incompletePlan)
	p.answer(t, "review", rejectingReview)
	p.answer(t, "fix", refinedPlan)
	e := startReviewedEngine(t, f.dir)
	db := e.srv.StateStore.GetDB()
	before := ledgerTasks(t, db)

	out, err := f.run(t)
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if got := joinCalls(p.calls(t)); got != "generate,review,fix" {
		t.Fatalf("planner calls = %q; want generate,review,fix (the plan must be reviewed and fixed before import)", got)
	}
	reqs := e.recorded()
	if countPath(reqs, "/api/v1/runs:plan") != 1 || countPath(reqs, "/api/v1/runs:execute") != 1 {
		t.Fatalf("requests = %s; want one plan then one execute", describeRequests(reqs))
	}
	var planBody map[string]any
	for i, r := range reqs {
		if r.Path == "/api/v1/runs:plan" {
			planBody = r.Body
			if i > len(reqs)-1 || reqs[len(reqs)-1].Path != "/api/v1/runs:execute" {
				t.Fatalf("execute did not follow the plan: %s", describeRequests(reqs))
			}
		}
	}
	if planBody["review"] != true || planBody["auto_import"] != true || !strings.HasPrefix(fmt.Sprint(planBody["request_id"]), "cli-intent-") {
		t.Fatalf("plan request = %v; want a reviewed, imported request with a stable cli-intent ID", planBody)
	}
	snaps := e.executeSnapshots()
	wantImported := fixedPlanImported(before)
	if len(snaps) != 1 || snaps[0] != wantImported {
		t.Fatalf("ledger when execute arrived =\n%v\nwant the fixed plan imported first:\n%s", snaps, wantImported)
	}
	receipts := planReceipts(t, db)
	if len(receipts) != 1 || !strings.HasSuffix(receipts[0], ":completed") {
		t.Fatalf("plan receipts = %v; want one completed plan-request receipt", receipts)
	}
	plans, reviews := planArtifacts(t, f.dir)
	if len(plans) != 2 || len(reviews) != 2 {
		t.Fatalf("artifacts plans=%v reviews=%v; want the generated and fixed plans, the rejecting review and the one-fix import record", plans, reviews)
	}
	reviewPath := assertRetainedEvidence(t, out)

	t.Run("unchanged intent replays without a model call or import", func(t *testing.T) {
		out, err := f.run(t)
		if err != nil {
			t.Fatalf("rerun: %v\n%s", err, out)
		}
		if got := joinCalls(p.calls(t)); got != "generate,review,fix" {
			t.Fatalf("planner calls after replay = %q; want no new call", got)
		}
		if got := ledgerTasks(t, db); got != wantImported {
			t.Fatalf("ledger after replay =\n%s\nwant unchanged (no duplicate import)", got)
		}
		if got := planReceipts(t, db); strings.Join(got, ",") != strings.Join(receipts, ",") {
			t.Fatalf("receipts after replay = %v; want %v", got, receipts)
		}
		if got := assertRetainedEvidence(t, out); got != reviewPath {
			t.Fatalf("replay named review %s; want the retained %s", got, reviewPath)
		}
		if n := countPath(e.recorded(), "/api/v1/runs:execute"); n != 2 {
			t.Fatalf("replayed run sent %d execute requests in total; want 2", n)
		}
	})

	t.Run("changed intent is a new reviewed request", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(f.dir, "INTENT.md"), []byte(reviewedIntent+"- Storage: append-only history\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		// The generator re-lists the imported story (rewording T-1) and adds
		// one task. The existing re-plan identity rules keep the imported rows
		// as they are and append only the new work.
		p.answer(t, "generate", historyPlan)
		p.answer(t, "review", approvingReview)
		out, err := f.run(t)
		if err != nil {
			t.Fatalf("run after intent change: %v\n%s", err, out)
		}
		if got := joinCalls(p.calls(t)); got != "generate,review,fix,generate,review" {
			t.Fatalf("planner calls = %q; want one new generate and review", got)
		}
		if got := planReceipts(t, db); len(got) != 2 {
			t.Fatalf("receipts = %v; want a second plan request", got)
		}
		got := ledgerTasks(t, db)
		kept := true
		for _, row := range strings.Split(wantImported, "\n") {
			kept = kept && slices.Contains(strings.Split(got, "\n"), row)
		}
		if !kept || strings.Count(got, "\n") != strings.Count(wantImported, "\n")+1 ||
			!strings.Contains(got, "=Record append-only history") || strings.Contains(got, "Reworded") {
			t.Fatalf("ledger =\n%s\nwant the imported rows kept unchanged and only the new task appended", got)
		}
		if snaps := e.executeSnapshots(); len(snaps) != 3 || snaps[2] != got {
			t.Fatalf("the changed plan was not imported before execute: %v", snaps)
		}
	})
}

// A plan the review rejects and the fix cannot repair, a reviewer that fails
// or cannot be reached, and a reviewer answer without a verdict import
// nothing and execute nothing.
func TestRunRefusesUnreviewedPlanBeforeExecute(t *testing.T) {
	cases := []struct {
		name      string
		review    string
		fix       string
		wantCalls string
		wantErr   string
	}{
		{"rejected and the fix changes nothing", rejectingReview, incompletePlan, "generate,review,fix", "unchanged review retry refused"},
		{"rejected and the fixer is unavailable", rejectingReview, "", "generate,review,fix", "fixture: no fix answer"},
		{"reviewer unavailable", "", "", "generate,review", "fixture: no review answer"},
		{"reviewer answers without a verdict", `{"assessment":"looks fine"}`, "", "generate,review", "requires explicit approved"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunFixture(t, false)
			p := installPlannerFixture(t, f.dir)
			p.answer(t, "generate", incompletePlan)
			p.answer(t, "review", tc.review)
			p.answer(t, "fix", tc.fix)
			e := startReviewedEngine(t, f.dir)
			db := e.srv.StateStore.GetDB()
			before := ledgerDump(t, db)

			out, err := f.run(t)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "nothing was executed") {
				t.Fatalf("error = %v; want %q and nothing executed\n%s", err, tc.wantErr, out)
			}
			if got := joinCalls(p.calls(t)); got != tc.wantCalls {
				t.Fatalf("planner calls = %q; want %q", got, tc.wantCalls)
			}
			reqs := e.recorded()
			if countPath(reqs, "/api/v1/runs:execute") != 0 || countPath(reqs, "/api/v1/runs:plan") != 1 {
				t.Fatalf("requests = %s; want one plan request and no execute", describeRequests(reqs))
			}
			if after := ledgerDump(t, db); after != before {
				t.Fatalf("refused plan changed the ledger:\nbefore\n%s\nafter\n%s", before, after)
			}
			assertPlanGateBacklogUntouched(t, f.dir)
		})
	}
}

// Named runs and --no-auto-plan never plan, so they never call the planner
// or the reviewer.
func TestRunWithoutAutoPlanNeverReviews(t *testing.T) {
	for _, args := range [][]string{{"--no-auto-plan"}, {"T-001"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newRunFixture(t, false)
			p := installPlannerFixture(t, f.dir)
			p.answer(t, "generate", incompletePlan)
			p.answer(t, "review", approvingReview)
			e := startReviewedEngine(t, f.dir)
			out, err := f.run(t, args...)
			if err != nil {
				t.Fatalf("run %v: %v\n%s", args, err, out)
			}
			reqs := e.recorded()
			if countPath(reqs, "/api/v1/runs:plan") != 0 || countPath(reqs, "/api/v1/runs:execute") != 1 {
				t.Fatalf("requests = %s; want execute only", describeRequests(reqs))
			}
			if calls := p.calls(t); len(calls) != 0 {
				t.Fatalf("planner was called: %v", calls)
			}
		})
	}
}

// runPlanCommand executes `openexec plan <args>` through the real root command.
func runPlanCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	reset := func(fs *pflag.FlagSet) {
		fs.VisitAll(func(fl *pflag.Flag) {
			_ = fl.Value.Set(fl.DefValue)
			fl.Changed = false
		})
	}
	reset(planCmd.Flags())
	t.Cleanup(func() { reset(planCmd.Flags()) })
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	rootCmd.SetArgs(append([]string{"plan"}, args...))
	err := rootCmd.Execute()
	return out.String(), err
}

func withLedger(t *testing.T, dir string, read func(*sql.DB)) {
	t.Helper()
	store, err := state.NewStore(filepath.Join(dir, ".openexec", "openexec.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	read(store.GetDB())
}

// The request identity is the intent file, its content and the validation
// choice: equal inputs replay, any change is a new request.
func TestReviewedPlanRequestIdentity(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("INTENT.md", reviewedIntent)
	write("OTHER.md", reviewedIntent)
	base := reviewedPlanRequest(dir, "INTENT.md", false)
	if !base.Review || !base.AutoImport || base.IntentFile != "INTENT.md" || base.NoValidate || !strings.HasPrefix(base.RequestID, "cli-intent-") {
		t.Fatalf("request = %+v; want a reviewed, imported request for INTENT.md", base)
	}
	if again := reviewedPlanRequest(dir, "INTENT.md", false); again != base {
		t.Fatalf("same input gave %+v, then %+v", base, again)
	}
	differs := map[string]string{
		"other file, same content": reviewedPlanRequest(dir, "OTHER.md", false).RequestID,
		"validation skipped":       reviewedPlanRequest(dir, "INTENT.md", true).RequestID,
		"missing file":             reviewedPlanRequest(dir, "ABSENT.md", false).RequestID,
	}
	write("INTENT.md", reviewedIntent+"\n")
	differs["one byte appended"] = reviewedPlanRequest(dir, "INTENT.md", false).RequestID
	for name, id := range differs {
		if id == base.RequestID {
			t.Fatalf("%s kept request ID %s", name, id)
		}
	}
}

// `openexec plan` composes the same reviewed route in-process.
func TestPlanCommandReviewsBeforeImport(t *testing.T) {
	t.Run("invalid intent is refused before any model call", func(t *testing.T) {
		f := newRunFixture(t, false)
		p := installPlannerFixture(t, f.dir)
		p.answer(t, "generate", incompletePlan)
		p.answer(t, "review", approvingReview)
		if err := os.WriteFile(filepath.Join(f.dir, "INTENT.md"), []byte("no heading, no goals, no requirements\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var before string
		withLedger(t, f.dir, func(db *sql.DB) { before = ledgerDump(t, db) })
		out, err := runPlanCommand(t)
		if err == nil || !strings.Contains(err.Error(), "intent validation failed") || !strings.Contains(err.Error(), "Document title not found") {
			t.Fatalf("error = %v; want the intent's validation issues\n%s", err, out)
		}
		if calls := p.calls(t); len(calls) != 0 {
			t.Fatalf("planner or reviewer was called: %v", calls)
		}
		withLedger(t, f.dir, func(db *sql.DB) {
			if after := ledgerDump(t, db); after != before {
				t.Fatalf("refused plan changed the ledger:\nbefore\n%s\nafter\n%s", before, after)
			}
		})
	})

	t.Run("accepted, then replayed", func(t *testing.T) {
		f := newRunFixture(t, false)
		p := installPlannerFixture(t, f.dir)
		p.answer(t, "generate", incompletePlan)
		p.answer(t, "review", rejectingReview)
		p.answer(t, "fix", refinedPlan)
		var before string
		withLedger(t, f.dir, func(db *sql.DB) { before = ledgerTasks(t, db) })

		out, err := runPlanCommand(t)
		if err != nil {
			t.Fatalf("plan: %v\n%s", err, out)
		}
		if got := joinCalls(p.calls(t)); got != "generate,review,fix" {
			t.Fatalf("planner calls = %q; want generate,review,fix", got)
		}
		want := fixedPlanImported(before)
		var receipts []string
		withLedger(t, f.dir, func(db *sql.DB) {
			if got := ledgerTasks(t, db); got != want {
				t.Fatalf("ledger =\n%s\nwant the fixed plan imported:\n%s", got, want)
			}
			receipts = planReceipts(t, db)
		})
		if len(receipts) != 1 || !strings.HasSuffix(receipts[0], ":completed") {
			t.Fatalf("receipts = %v", receipts)
		}
		reviewPath := assertRetainedEvidence(t, out)

		out, err = runPlanCommand(t)
		if err != nil {
			t.Fatalf("plan replay: %v\n%s", err, out)
		}
		if got := joinCalls(p.calls(t)); got != "generate,review,fix" {
			t.Fatalf("planner calls after replay = %q; want no new call", got)
		}
		if got := assertRetainedEvidence(t, out); got != reviewPath {
			t.Fatalf("replay named review %s; want the retained %s", got, reviewPath)
		}
		withLedger(t, f.dir, func(db *sql.DB) {
			if got := ledgerTasks(t, db); got != want {
				t.Fatalf("replay changed the ledger:\n%s", got)
			}
			if got := planReceipts(t, db); strings.Join(got, ",") != strings.Join(receipts, ",") {
				t.Fatalf("receipts after replay = %v", got)
			}
		})

		// The same intent through a bare run replays the plan `plan` retained.
		e := startReviewedEngine(t, f.dir)
		if out, err := f.run(t); err != nil {
			t.Fatalf("run after plan: %v\n%s", err, out)
		}
		if got := joinCalls(p.calls(t)); got != "generate,review,fix" {
			t.Fatalf("run after plan called the planner: %q", got)
		}
		if snaps := e.executeSnapshots(); len(snaps) != 1 || snaps[0] != want {
			t.Fatalf("ledger at execute = %v; want %q", snaps, want)
		}
	})

	for _, tc := range []struct {
		name, review, fix, wantCalls, wantErr string
	}{
		{"rejected and the fix changes nothing", rejectingReview, incompletePlan, "generate,review,fix", "unchanged review retry refused"},
		{"reviewer unavailable", "", "", "generate,review", "fixture: no review answer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRunFixture(t, false)
			p := installPlannerFixture(t, f.dir)
			p.answer(t, "generate", incompletePlan)
			p.answer(t, "review", tc.review)
			p.answer(t, "fix", tc.fix)
			var before string
			withLedger(t, f.dir, func(db *sql.DB) { before = ledgerDump(t, db) })

			out, err := runPlanCommand(t)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v; want %q\n%s", err, tc.wantErr, out)
			}
			if got := joinCalls(p.calls(t)); got != tc.wantCalls {
				t.Fatalf("planner calls = %q; want %q", got, tc.wantCalls)
			}
			withLedger(t, f.dir, func(db *sql.DB) {
				if after := ledgerDump(t, db); after != before {
					t.Fatalf("refused plan changed the ledger:\nbefore\n%s\nafter\n%s", before, after)
				}
			})
		})
	}
}
