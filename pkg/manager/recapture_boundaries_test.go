package manager

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

// These independent boundary journeys use the implementation-owned admitted
// adapter, real verification subprocesses and the public native queue entry.
func boundaryRun(f *recaptureFixture) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	f.cancel = cancel
	return f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
}

func boundaryEvidence(t *testing.T, f *recaptureFixture, checkpoint string, repairs int, waiting bool) {
	t.Helper()
	f.restart(t)
	tasks, err := f.env.rel.TasksInStories(context.Background(), []string{"S"})
	if err != nil {
		t.Fatal(err)
	}
	tasks = checkLedger(tasks)
	count := 0
	for _, task := range tasks {
		if task.Metadata["repair_of"] == "A" {
			count++
		}
	}
	if count != repairs || len(tasks) != 2+repairs {
		t.Fatalf("unexpected repair ledger: %+v", tasks)
	}
	a, err := f.env.rel.TaskSnapshot(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := f.env.rel.TaskSnapshot(context.Background(), "Settings")
	if err != nil {
		t.Fatal(err)
	}
	if waiting && (settings.Status != release.TaskStatusPending || settings.AttemptCount != 0) {
		t.Fatalf("Settings escaped dependency: %+v", settings)
	}
	if a.AttemptCount > a.MaxAttempts {
		t.Fatalf("retry limit exceeded: %+v", a)
	}
	data, err := json.Marshal(map[string]interface{}{"checkpoint": checkpoint, "task": a, "settings": settings, "repairs": count, "dispatches": f.calls})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RECAPTURE_BOUNDARY_STATE %s", data)
}

func TestRecaptureBoundariesTerminal(t *testing.T) {
	for _, tc := range []struct {
		name, mode, outcome    string
		spent, attempts, calls int
	}{
		{"exhausted", "exhausted", "exhausted", 1, 3, 2},
		{"restart_remaining", "exhausted", "exhausted", 2, 3, 1},
		{"restart_spent", "exhausted", "exhausted", 3, 3, 0},
		{"unresolved", "unresolved", "unresolved", 1, 1, 0},
		{"launch_refusal", "refusal", "refused", 1, 2, 1},
		{"cancellation", "cancel", "cancelled", 1, 2, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := "exit 2"
			if tc.mode == "unresolved" {
				command = ""
			}
			f := newRecaptureFixture(t, command)
			f.mode = tc.mode
			if tc.spent > 1 {
				task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
				if err != nil {
					t.Fatal(err)
				}
				task.Status, task.AttemptCount = release.TaskStatusInProgress, tc.spent
				task.Metadata["recapture_outcome"] = "running"
				if err := f.env.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
			}
			f.restart(t)
			err := boundaryRun(f)
			if err == nil || !strings.Contains(err.Error(), "recapture "+tc.outcome) {
				t.Fatalf("missing explicit terminal reason: %v", err)
			}
			for _, checkpoint := range []string{"terminal", "terminal_restart"} {
				boundaryEvidence(t, f, checkpoint, 0, true)
				f.assertTerminal(t, tc.outcome, tc.attempts)
				if f.calls != tc.calls {
					t.Fatalf("dispatches=%d want=%d", f.calls, tc.calls)
				}
				if err := boundaryRun(f); err == nil {
					t.Fatal("terminal restart accepted")
				}
			}
			boundaryEvidence(t, f, "terminal_stable", 0, true)
			f.assertTerminal(t, tc.outcome, tc.attempts)
			if f.calls != tc.calls {
				t.Fatal("restart refunded attempts")
			}
		})
	}
}

func TestRecaptureBoundariesFailure(t *testing.T) {
	f := newRecaptureFixture(t, "printf 'BOUNDARY_DIAGNOSTIC\\n' >&2; exit 2")
	f.restart(t)
	if err := boundaryRun(f); err == nil {
		t.Fatal("ordinary repair fixture must stop")
	}
	boundaryEvidence(t, f, "failed_repair", 1, true)
	task, _ := f.env.rel.TaskSnapshot(context.Background(), "A")
	if task.AttemptCount != 2 || task.Metadata["recapture_outcome"] != "failed" {
		t.Fatalf("failure disposition: %+v", task)
	}
	id, _ := task.Metadata["verification_failure_evidence"].(string)
	if id == "" || id == "legacy" {
		t.Fatal("fresh receipt not bound")
	}
	step, err := f.env.mgr.state.GetRunStep(context.Background(), id)
	if err != nil || step == nil {
		t.Fatalf("missing durable receipt: %v", err)
	}
	var refs map[string]string
	if err := json.Unmarshal([]byte(step.Metadata), &refs); err != nil {
		t.Fatal(err)
	}
	usable := false
	for hash := range refs {
		ev, err := runtime.ReadCommandEvidence(f.env.dir, hash)
		if err == nil && ev.ExitCode == 2 && strings.Contains(ev.Stderr, "BOUNDARY_DIAGNOSTIC") && ev.Cwd == f.env.dir && len(ev.Argv) == 3 && ev.Argv[2] == task.VerificationScript {
			usable = true
		}
	}
	if !usable {
		t.Fatal("fresh private diagnostics unavailable")
	}
	tasks, _ := f.env.rel.TasksInStories(context.Background(), []string{"S"})
	for _, repair := range checkLedger(tasks) {
		if repair.Metadata["repair_of"] == "A" && !strings.Contains(repair.Description, id) {
			t.Fatal("repair missing fresh evidence")
		}
	}
	if err := boundaryRun(f); err == nil {
		t.Fatal("unfinished repair accepted")
	}
	boundaryEvidence(t, f, "failed_restart", 1, true)
	if f.calls != 1 {
		t.Fatal("repeated empty recapture/repair")
	}
}

func TestRecaptureBoundariesSuccessCompletionGuard(t *testing.T) {
	f := newRecaptureFixture(t, "printf 'BOUNDARY_SUCCESS\\n'")
	f.mode = "success"
	// Use existing accepted-plan completion obligations, not a test-only gate.
	_, err := f.env.mgr.state.GetDB().Exec(`
 INSERT INTO repositories (id,persisted_uuid) VALUES ('repo','persisted');
 INSERT INTO checkouts (id,repository_id,root_path) VALUES ('checkout','repo','/project');
 INSERT INTO worktrees (id,repository_id,checkout_id,root_path) VALUES ('worktree','repo','checkout','/project');
 INSERT INTO graph_generations (id,schema_version,repository_id,checkout_id,worktree_id,worktree_state_hash,configuration_digest,extractor_version,manifest_hash,status) VALUES ('graph',1,'repo','checkout','worktree','state','config','extractor','manifest','current');
 INSERT INTO validation_plan_revisions (id,task_id,revision,generation_id,worktree_state_hash,status) VALUES ('plan','A',1,'graph','state','accepted');
 INSERT INTO validation_items (id,plan_revision_id,source,disposition,requirement,criterion) VALUES ('item','plan','policy','accepted','blocking','Tests pass');`)
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := boundaryRun(f); err == nil || !strings.Contains(err.Error(), "without supported evidence") {
		t.Fatalf("completion obligation bypassed: %v", err)
	}
	boundaryEvidence(t, f, "success_waiting", 0, true)
	task, _ := f.env.rel.TaskSnapshot(context.Background(), "A")
	if task.Metadata["recapture_outcome"] != "success" || task.Metadata["verification_failure_evidence"] != nil || task.Status == release.TaskStatusDone || f.calls != 1 {
		t.Fatalf("success skipped normal completion: %+v calls=%d", task, f.calls)
	}
	// Supply the accepted obligation and exercise its ordinary completion path.
	if _, err := f.env.mgr.state.GetDB().Exec(`INSERT INTO completion_claims (id,validation_item_id,predicate,scope,status,repository_state_hash) VALUES ('claim','item','validation_item_passed','tests','supported','state')`); err != nil {
		t.Fatal(err)
	}
	if err := f.env.rel.SetTaskStatus("A", release.TaskStatusDone); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := boundaryRun(f); err != nil {
		t.Fatal(err)
	}
	boundaryEvidence(t, f, "success_completed", 0, false)
	for _, id := range []string{"A", "Settings"} {
		task, err := f.env.rel.TaskSnapshot(context.Background(), id)
		if err != nil || task.Status != release.TaskStatusDone {
			t.Fatalf("dependency did not complete: %+v %v", task, err)
		}
	}
	if f.calls != 1 {
		t.Fatal("success recaptured again")
	}
}

// checkLedger leaves out the provider fix of a fix: recapture fixtures stop
// the queue inside the fix they run, and that stop is closed with a fix of
// its own. What these tests count is the failed check's own fix.
func checkLedger(tasks []*release.Task) []*release.Task {
	var out []*release.Task
	for _, task := range tasks {
		of, _ := task.Metadata["fix_of"].(string)
		root, _ := task.Metadata["repair_of"].(string)
		if task.Metadata["failure_kind"] == "provider" && of != root {
			continue
		}
		out = append(out, task)
	}
	return out
}
