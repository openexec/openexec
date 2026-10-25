package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/health"
	"github.com/openexec/openexec/internal/project"
	"github.com/openexec/openexec/pkg/audit"
	"github.com/openexec/openexec/pkg/db/sqlitecfg"
	"github.com/openexec/openexec/pkg/db/state"
)

// daemonApplyPatchRefusal is the refusal text the apply_patch action returns.
// It is spelled out so this test fails on its assertions, not on compilation.
const daemonApplyPatchRefusal = "action apply_patch is not implemented and applied no changes to the workspace; " +
	"apply the patch from an agentic stage with the git_apply_patch tool, or remove this stage from the blueprint"

// failedStageDaemonChildEnv names the initialized project a child test process
// serves through the production daemon; the parent reads what it left behind.
const failedStageDaemonChildEnv = "OPENEXEC_FAILED_STAGE_DAEMON_CHILD"

// failedStageBlueprints are two external custom blueprints dispatching the
// registered apply_patch action with a valid diff: one without a failure route
// (the run must end in error) and one recovering through a true command.
var failedStageBlueprints = map[string]string{
	"apply_nohandler": "",
	"apply_recovered": "    on_failure: recover\n    max_retries: 1\n",
}

// plainSuccessBlueprint is an ordinary deterministic blueprint whose only
// stage succeeds; the daemon must record it as completed, never failed.
const plainSuccessBlueprint = "plain_success"

func plainSuccessYAML() string {
	return "id: " + plainSuccessBlueprint + "\nname: " + plainSuccessBlueprint + "\ninitial_stage: recover\nstages:\n" +
		"  recover:\n    name: recover\n    type: deterministic\n    toolset: coding_backend\n    commands: [\"true\"]\n    on_success: complete\n"
}

func failedStageBlueprint(id, handler string) string {
	return "id: " + id + "\nname: " + id + "\ninitial_stage: apply\nstages:\n" +
		"  apply:\n    name: apply\n    type: deterministic\n    toolset: coding_backend\n    action: apply_patch\n" +
		"    inputs:\n      implementation: |\n        --- a/probe.txt\n        +++ b/probe.txt\n        @@ -1 +1 @@\n        -before\n        +after\n" +
		"    on_success: complete\n" + handler +
		"  recover:\n    name: recover\n    type: deterministic\n    toolset: coding_backend\n    commands: [\"true\"]\n    on_success: complete\n"
}

// initFailedStageProject initializes a disposable project the way
// `openexec init` does, with the deterministic `true` runner (no model).
func initFailedStageProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg, err := project.Initialize("failed-stage-probe", dir)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Execution.RunnerCommand = "true"
	if err := project.SaveProjectConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe.txt"), []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bpDir := filepath.Join(dir, ".openexec", "blueprints")
	if err := os.MkdirAll(bpDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for id, handler := range failedStageBlueprints {
		if err := os.WriteFile(filepath.Join(bpDir, id+".yaml"), []byte(failedStageBlueprint(id, handler)), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(bpDir, plainSuccessBlueprint+".yaml"), []byte(plainSuccessYAML()), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func sharedStateDB(dir string) string { return filepath.Join(dir, ".openexec", "openexec.db") }

// daemonConfig is the server.Config `openexec start` builds for a project.
func daemonConfig(dir string, port int) Config {
	return Config{
		Port:          port,
		ListenAddress: "127.0.0.1",
		UnifiedDB:     sharedStateDB(dir),
		DataDir:       filepath.Join(dir, ".openexec", "data"),
		ProjectsDir:   dir,
	}
}

// TestNativeDaemonFailedStageChild is the child half of
// TestNativeDaemonPersistsFailedStageEvidence: it serves the project through
// the production daemon (server.New + Start with preflight) on a loopback
// port, starts both blueprints over the public API, waits for each run to end,
// shuts the daemon down, and exits.
func TestNativeDaemonFailedStageChild(t *testing.T) {
	dir := os.Getenv(failedStageDaemonChildEnv)
	if dir == "" {
		t.Skip("child process of TestNativeDaemonPersistsFailedStageEvidence")
	}
	// `openexec start` runs from the project directory.
	t.Chdir(dir)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	s, err := New(daemonConfig(dir, port))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Start(ctx) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	deadline := time.Now().Add(time.Minute)
	for {
		resp, err := http.Get(base + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		select {
		case err := <-done:
			t.Fatalf("daemon exited before serving: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("daemon did not serve /api/health")
		}
		time.Sleep(50 * time.Millisecond)
	}

	runIDs := map[string]string{}
	ids := []string{plainSuccessBlueprint}
	for id := range failedStageBlueprints {
		ids = append(ids, id)
	}
	for _, id := range ids {
		body, _ := json.Marshal(map[string]string{"blueprint_id": id, "task_description": "apply the probe patch"})
		resp, err := http.Post(base+"/api/v1/runs:blueprint", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		var created struct {
			RunID string `json:"run_id"`
		}
		err = json.NewDecoder(resp.Body).Decode(&created)
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusCreated || created.RunID == "" {
			t.Fatalf("start %s: status %d, %v", id, resp.StatusCode, err)
		}
		runIDs[id] = created.RunID
	}
	for id, runID := range runIDs {
		for {
			resp, err := http.Get(base + "/api/v1/runs/" + runID)
			if err != nil {
				t.Fatal(err)
			}
			var run struct {
				Status string `json:"status"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&run)
			_ = resp.Body.Close()
			if run.Status == "error" || run.Status == "complete" {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("run %s (%s) did not finish: %q", runID, id, run.Status)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("daemon shutdown: %v", err)
	}
	raw, _ := json.Marshal(runIDs)
	if err := os.WriteFile(filepath.Join(dir, "run-ids.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// seedPreFixSharedDB writes the shared state DB as a daemon built before the
// audit column migration left it: the unified schema without migrations, with
// existing run, step, audit entry and legacy audit log rows.
func seedPreFixSharedDB(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", sqlitecfg.DSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	stmts := []string{
		state.UnifiedSchema,
		`INSERT INTO runs (id, project_path, mode, status) VALUES ('legacy-run', '/legacy', 'workspace-write', 'complete')`,
		`INSERT INTO run_steps (id, run_id, phase, iteration, status, metadata) VALUES ('legacy-run-1', 'legacy-run', 'implement', 1, 'completed', '{"type":"stage_complete"}')`,
		`INSERT INTO audit_entries (id, run_id, timestamp, event_type, severity, actor_id, actor_type, metadata)
		 VALUES ('legacy-audit', 'legacy-run', '2026-01-01T00:00:00Z', 'run.step', 'info', 'openexec', 'system', '{"type":"stage_complete"}')`,
		`CREATE TABLE IF NOT EXISTS audit_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, timestamp TEXT NOT NULL, event_type TEXT NOT NULL, iteration INTEGER, data TEXT, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO audit_logs (timestamp, event_type, iteration, data) VALUES ('2026-01-01T00:00:00Z', 'legacy', 1, '{}')`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed pre-fix DB: %v\n%s", err, stmt)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('audit_entries') WHERE name = 'message_id'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("seeded DB must lack audit_entries.message_id (count %d, %v)", n, err)
	}
}

// TestNativeDaemonPersistsFailedStageEvidence runs both apply_patch blueprints
// through the production daemon in a separate process and, after that process
// exits, reopens the SHARED .openexec/openexec.db it used: the refusal must be
// a failed stage step and a stage_failed audit record, the recovery a
// completed step, the target bytes unchanged, and existing rows preserved.
func TestNativeDaemonPersistsFailedStageEvidence(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "freshly-initialized"
		if existing {
			name = "existing-pre-fix-db"
		}
		t.Run(name, func(t *testing.T) {
			dir := initFailedStageProject(t)
			dbPath := sharedStateDB(dir)
			if existing {
				seedPreFixSharedDB(t, dbPath)
			}

			child := exec.Command(os.Args[0], "-test.run=^TestNativeDaemonFailedStageChild$", "-test.count=1", "-test.v")
			child.Env = append(os.Environ(), failedStageDaemonChildEnv+"="+dir)
			out, err := child.CombinedOutput()
			if err != nil {
				t.Fatalf("daemon child failed: %v\n%s", err, out)
			}
			if !strings.Contains(string(out), "--- PASS: TestNativeDaemonFailedStageChild") {
				t.Fatalf("daemon child did not run:\n%s", out)
			}
			raw, err := os.ReadFile(filepath.Join(dir, "run-ids.json"))
			if err != nil {
				t.Fatalf("daemon child left no run IDs: %v\n%s", err, out)
			}
			runIDs := map[string]string{}
			if err := json.Unmarshal(raw, &runIDs); err != nil {
				t.Fatal(err)
			}

			if b, _ := os.ReadFile(filepath.Join(dir, "probe.txt")); string(b) != "before\n" {
				t.Fatalf("probe.txt changed: %q", b)
			}

			// Reopen the exact file the daemon used, without migrating it.
			db, err := sql.Open("sqlite", sqlitecfg.DSN(dbPath))
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			assertFailedStageEvidence(t, db, runIDs)
			if existing {
				assertPreFixRowsPreserved(t, db)
			}

			// Reopening through the store keeps the projection failed and
			// ordinary audit writes working on the shared DB.
			store, err := state.NewStore(dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = store.Close() }()
			ctx := context.Background()
			for id, runID := range runIDs {
				steps, err := store.ListRunSteps(ctx, runID, 100, 0)
				if err != nil {
					t.Fatal(err)
				}
				// The projection readers use: the latest step per stage.
				last := map[string]string{}
				for _, step := range steps {
					last[step.Phase] = step.Status
				}
				want := map[string]string{"apply": "failed"}
				switch {
				case id == plainSuccessBlueprint:
					want = map[string]string{"recover": "completed"}
				case failedStageBlueprints[id] != "":
					want["recover"] = "completed"
				}
				for phase, status := range want {
					if last[phase] != status {
						t.Fatalf("%s: reopened %s projection = %q, want %s (%+v)", id, phase, last[phase], status, steps)
					}
				}
			}
			logger, err := audit.NewLoggerWithDB(store.GetDB())
			if err != nil {
				t.Fatalf("audit logger on reopened shared DB: %v", err)
			}
			entry, err := audit.NewEntry(audit.EventRunStep, "openexec", "system")
			if err != nil {
				t.Fatal(err)
			}
			e, err := entry.WithMetadata(map[string]interface{}{"type": "reopen_probe"}).Build()
			if err != nil {
				t.Fatal(err)
			}
			e.MessageID = sql.NullString{String: "reopen-message", Valid: true}
			if err := logger.Log(ctx, e); err != nil {
				t.Fatalf("ordinary audit write after reopen: %v", err)
			}
			var messageID string
			if err := store.GetDB().QueryRow(`SELECT message_id FROM audit_entries WHERE id = ?`, e.ID).Scan(&messageID); err != nil || messageID != "reopen-message" {
				t.Fatalf("reopened audit write not read back: %q, %v", messageID, err)
			}
			if existing {
				assertPreFixRowsPreserved(t, store.GetDB())
			}
		})
	}
}

func assertFailedStageEvidence(t *testing.T, db *sql.DB, runIDs map[string]string) {
	t.Helper()
	if len(runIDs) != len(failedStageBlueprints)+1 {
		t.Fatalf("run IDs = %v", runIDs)
	}
	assertPlainSuccess(t, db, runIDs[plainSuccessBlueprint])
	for id, runID := range runIDs {
		if id == plainSuccessBlueprint {
			continue
		}
		recovered := failedStageBlueprints[id] != ""

		var status string
		var errMsg sql.NullString
		if err := db.QueryRow(`SELECT status, error_message FROM runs WHERE id = ?`, runID).Scan(&status, &errMsg); err != nil {
			t.Fatalf("%s: persisted run %s missing: %v", id, runID, err)
		}
		wantStatus := "error"
		if recovered {
			wantStatus = "complete"
		}
		if status != wantStatus {
			t.Fatalf("%s: persisted run status = %q (error %q), want %q", id, status, errMsg.String, wantStatus)
		}
		if !recovered && !strings.Contains(errMsg.String, daemonApplyPatchRefusal) {
			t.Fatalf("%s: persisted run error = %q, want the refusal", id, errMsg.String)
		}

		// Durable stage steps: apply has an explicit failed result carrying
		// the refusal and never a completed one; recover records success.
		rows, err := db.Query(`SELECT phase, status, COALESCE(metadata, '{}') FROM run_steps WHERE run_id = ?`, runID)
		if err != nil {
			t.Fatal(err)
		}
		applyFailed, recoverCompleted := false, false
		for rows.Next() {
			var phase, stepStatus, raw string
			if err := rows.Scan(&phase, &stepStatus, &raw); err != nil {
				t.Fatal(err)
			}
			var md map[string]interface{}
			_ = json.Unmarshal([]byte(raw), &md)
			switch {
			case phase == "apply" && stepStatus == "completed":
				t.Fatalf("%s: apply step recorded completed: %s", id, raw)
			case phase == "apply" && stepStatus == "failed":
				if md["error"] != daemonApplyPatchRefusal {
					t.Fatalf("%s: failed apply step error = %v, want the refusal", id, md["error"])
				}
				applyFailed = true
			case phase == "recover" && stepStatus == "completed":
				recoverCompleted = true
			}
		}
		_ = rows.Close()
		if !applyFailed {
			t.Fatalf("%s: no failed run step for the apply stage in the shared DB", id)
		}
		if recoverCompleted != recovered {
			t.Fatalf("%s: recover step completed = %v, want %v", id, recoverCompleted, recovered)
		}

		// Durable audit in the same shared DB.
		arows, err := db.Query(`SELECT event_type, COALESCE(metadata, '{}') FROM audit_entries WHERE metadata LIKE ?`, "%"+runID+"%")
		if err != nil {
			t.Fatal(err)
		}
		created, stageFailed, recoverComplete := false, false, false
		for arows.Next() {
			var eventType, raw string
			if err := arows.Scan(&eventType, &raw); err != nil {
				t.Fatal(err)
			}
			var md map[string]interface{}
			_ = json.Unmarshal([]byte(raw), &md)
			if md["run_id"] != runID {
				continue
			}
			if eventType == string(audit.EventRunCreated) && md["event"] == "run.blueprint_created" {
				created = true
			}
			if eventType != string(audit.EventRunStep) {
				continue
			}
			switch {
			case md["stage"] == "apply" && md["type"] == "stage_complete":
				t.Fatalf("%s: audit records apply as complete: %s", id, raw)
			case md["stage"] == "apply" && md["type"] == "stage_failed":
				if md["error"] != daemonApplyPatchRefusal {
					t.Fatalf("%s: audit stage_failed error = %v, want the refusal", id, md["error"])
				}
				stageFailed = true
			case md["stage"] == "recover" && md["type"] == "stage_complete":
				recoverComplete = true
			}
		}
		_ = arows.Close()
		if !created {
			t.Fatalf("%s: shared DB has no run.blueprint_created audit record", id)
		}
		if !stageFailed {
			t.Fatalf("%s: shared DB has no stage_failed audit record for apply", id)
		}
		if recoverComplete != recovered {
			t.Fatalf("%s: audit recover stage_complete = %v, want %v", id, recoverComplete, recovered)
		}
	}
}

// assertPlainSuccess: an ordinary deterministic success persists as a
// completed run, a completed step and a stage_complete audit record, with no
// failed step or stage_failed record.
func assertPlainSuccess(t *testing.T, db *sql.DB, runID string) {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM runs WHERE id = ?`, runID).Scan(&status); err != nil || status != "complete" {
		t.Fatalf("plain success run %s status = %q, %v", runID, status, err)
	}
	var completed, failed int
	if err := db.QueryRow(`SELECT COUNT(*) FILTER (WHERE status = 'completed'), COUNT(*) FILTER (WHERE status = 'failed')
		FROM run_steps WHERE run_id = ? AND phase = 'recover'`, runID).Scan(&completed, &failed); err != nil {
		t.Fatal(err)
	}
	if completed != 1 || failed != 0 {
		t.Fatalf("plain success steps: completed=%d failed=%d, want 1/0", completed, failed)
	}
	var stageComplete, stageFailed int
	if err := db.QueryRow(`SELECT COUNT(*) FILTER (WHERE json_extract(metadata, '$.type') = 'stage_complete'),
		COUNT(*) FILTER (WHERE json_extract(metadata, '$.type') = 'stage_failed')
		FROM audit_entries WHERE event_type = ? AND json_extract(metadata, '$.run_id') = ?`, string(audit.EventRunStep), runID).Scan(&stageComplete, &stageFailed); err != nil {
		t.Fatal(err)
	}
	if stageComplete != 1 || stageFailed != 0 {
		t.Fatalf("plain success audit: stage_complete=%d stage_failed=%d, want 1/0", stageComplete, stageFailed)
	}
}

func assertPreFixRowsPreserved(t *testing.T, db *sql.DB) {
	t.Helper()
	var runStatus, stepStatus, auditMD, logData string
	var messageID sql.NullString
	if err := db.QueryRow(`SELECT status FROM runs WHERE id = 'legacy-run'`).Scan(&runStatus); err != nil || runStatus != "complete" {
		t.Fatalf("existing run not preserved: %q, %v", runStatus, err)
	}
	if err := db.QueryRow(`SELECT status FROM run_steps WHERE id = 'legacy-run-1'`).Scan(&stepStatus); err != nil || stepStatus != "completed" {
		t.Fatalf("existing run step not preserved: %q, %v", stepStatus, err)
	}
	if err := db.QueryRow(`SELECT metadata, message_id FROM audit_entries WHERE id = 'legacy-audit'`).Scan(&auditMD, &messageID); err != nil ||
		auditMD != `{"type":"stage_complete"}` || messageID.Valid {
		t.Fatalf("existing audit entry not preserved: %q %v, %v", auditMD, messageID, err)
	}
	if err := db.QueryRow(`SELECT data FROM audit_logs WHERE event_type = 'legacy'`).Scan(&logData); err != nil || logData != "{}" {
		t.Fatalf("existing audit log not preserved: %q, %v", logData, err)
	}
}

// TestNativeDaemonAuditInitFailureIsReportedDegraded: when the shared DB holds
// an audit table the logger cannot use, the daemon still starts (audit is
// non-critical) but reports the degradation through the audit_db preflight
// instead of presenting a typed-nil logger as active.
func TestNativeDaemonAuditInitFailureIsReportedDegraded(t *testing.T) {
	dir := initFailedStageProject(t)
	db, err := sql.Open("sqlite", sqlitecfg.DSN(sharedStateDB(dir)))
	if err != nil {
		t.Fatal(err)
	}
	// The logger indexes audit_logs(iteration); a table without it refuses.
	if _, err := db.Exec(`CREATE TABLE audit_logs (id INTEGER PRIMARY KEY, timestamp TEXT NOT NULL, event_type TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	t.Chdir(dir)

	s, err := New(daemonConfig(dir, 0))
	if err != nil {
		t.Fatalf("audit is non-critical; daemon must still construct: %v", err)
	}
	defer func() { _ = s.StateStore.Close() }()
	if s.AuditLogger != nil {
		t.Fatalf("failed audit logger presented as %T", s.AuditLogger)
	}
	s.registerPreflightChecks()
	_ = s.Checker.RunPreflight(context.Background())
	_, results := s.Checker.GetStatus()
	if got := results["audit_db"]; got.Status != health.StatusDegraded {
		t.Fatalf("audit_db preflight = %+v, want degraded", got)
	}
}
