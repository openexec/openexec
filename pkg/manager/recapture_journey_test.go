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

func TestLegacyRecaptureFailureReloadRepair(t *testing.T) {
	f := newRecaptureFixture(t, "printf 'RECAPTURE_DIAGNOSTIC\\n' >&2; exit 2")
	f.restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Stops only when the queue reaches the generated repair's ordinary work.
	if err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("fixture boundary not reached")
	}
	if f.calls != 1 {
		t.Fatalf("recaptures = %d", f.calls)
	}
	f.restart(t)
	task, err := f.env.rel.TaskSnapshot(ctx, "A")
	if err != nil || task.AttemptCount != 2 || task.Status != release.TaskStatusFailed || task.Metadata["fixed_by"] == nil {
		t.Fatalf("original disposition: %+v %v", task, err)
	}
	id, _ := task.Metadata["verification_failure_evidence"].(string)
	if id == "" || id == "legacy" {
		t.Fatal("fresh failure binding absent")
	}
	step, err := f.env.mgr.state.GetRunStep(ctx, id)
	if err != nil || step == nil || !strings.Contains(step.Metadata, "RECAPTURE_DIAGNOSTIC") {
		t.Fatal("recaptured diagnostics lost", err)
	}
	var refs map[string]string
	if err := json.Unmarshal([]byte(step.Metadata), &refs); err != nil {
		t.Fatal(err)
	}
	usable := false
	for hash := range refs {
		if ev, err := runtime.ReadCommandEvidence(f.env.dir, hash); err == nil {
			usable = ev.ExitCode == 2 && strings.Contains(ev.Stderr, "RECAPTURE_DIAGNOSTIC") && len(ev.Argv) == 3 && ev.Argv[2] == task.VerificationScript
		}
	}
	if !usable {
		t.Fatal("private repair context unavailable after reload")
	}
	tasks, err := f.env.rel.TasksInStories(ctx, []string{"S"})
	if err != nil || len(checkLedger(tasks)) != 3 {
		t.Fatal("expected one repair", err)
	}
	for _, repair := range checkLedger(tasks) {
		if repair.Metadata["repair_of"] == "A" && !strings.Contains(repair.Description, id) {
			t.Fatal("repair lacks fresh context")
		}
	}
	settings, _ := f.env.rel.TaskSnapshot(ctx, "Settings")
	if settings.AttemptCount != 0 {
		t.Fatal("Settings executed before dependency")
	}
}

func TestLegacyRecaptureSuccessReload(t *testing.T) {
	f := newRecaptureFixture(t, "printf 'CHECK_PASSED\\n'")
	f.restart(t)
	f.env.mgr.taskQueueActive = true
	err := f.env.mgr.repairTaskFromRetainedFailure(context.Background(), "A", "legacy")
	f.env.mgr.taskQueueActive = false
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
	if err != nil || task.Status != release.TaskStatusPending || task.AttemptCount != 2 || task.Metadata["recapture_outcome"] != "success" || task.Metadata["verification_failure_evidence"] != nil {
		t.Fatalf("successful recapture: %+v %v", task, err)
	}
	tasks, _ := f.env.rel.TasksInStories(context.Background(), []string{"S"})
	if len(tasks) != 2 || f.calls != 1 {
		t.Fatal("successful check invented repair")
	}
	ready, err := f.env.rel.RunnableTasks(context.Background(), []string{"S"})
	if err != nil || len(ready) != 1 || ready[0].ID != "A" {
		t.Fatal("original resume/Settings waiting lost", err)
	}
}

func TestLegacyRecaptureTerminalReload(t *testing.T) {
	for _, mode := range []string{"exhausted", "unresolved", "refusal", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			command := "exit 2"
			if mode == "unresolved" {
				command = ""
			}
			f := newRecaptureFixture(t, command)
			f.mode = mode
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			f.cancel = cancel
			if err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
				t.Fatal("terminal branch accepted")
			}
			outcome, attempts, calls := mode, 2, 1
			switch mode {
			case "exhausted":
				attempts, calls = 3, 2
			case "unresolved":
				attempts, calls = 1, 0
			case "refusal":
				outcome = "refused"
			case "cancel":
				outcome = "cancelled"
			}
			f.restart(t)
			f.assertTerminal(t, outcome, attempts)
			if err := f.env.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
				t.Fatal("terminal restart accepted")
			}
			if f.calls != calls {
				t.Fatalf("unexpected dispatch count %d want %d", f.calls, calls)
			}
			f.assertTerminal(t, outcome, attempts)
		})
	}
}

func TestLegacyRecaptureSuccessQueueConverges(t *testing.T) {
	f := newRecaptureFixture(t, "printf 'CHECK_PASSED\\n'")
	f.mode = "success"
	f.restart(t)
	if err := f.env.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	for _, id := range []string{"A", "Settings"} {
		task, err := f.env.rel.TaskSnapshot(context.Background(), id)
		if err != nil || task.Status != release.TaskStatusDone {
			t.Fatalf("queue did not converge: %+v %v", task, err)
		}
	}
	if f.calls != 1 {
		t.Fatal("successful check repeated")
	}
	tasks, _ := f.env.rel.TasksInStories(context.Background(), []string{"S"})
	if len(tasks) != 2 {
		t.Fatal("success generated a repair")
	}
}

func TestLegacyRecaptureInterruptedBudgetReload(t *testing.T) {
	f := newRecaptureFixture(t, "exit 2")
	task, _ := f.env.rel.TaskSnapshot(context.Background(), "A")
	task.Status = release.TaskStatusInProgress
	task.AttemptCount = task.MaxAttempts
	task.Metadata["recapture_outcome"] = "running"
	if err := f.env.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := f.env.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("interrupted exhausted attempt accepted")
	}
	f.restart(t)
	f.assertTerminal(t, "exhausted", 3)
	if f.calls != 0 {
		t.Fatal("restart refunded a spent attempt")
	}
}

func TestLegacyRecaptureAuthoritativeResolution(t *testing.T) {
	f := newRecaptureFixture(t, "exit 0")
	ctx := context.Background()
	task, _ := f.env.rel.TaskSnapshot(ctx, "A")
	refs := recaptureReceipt("lint", 2)
	if command, err := f.env.mgr.resolveRecaptureCommand(task, "lint", refs); err != nil || command != "" {
		t.Fatal("named current check was not deferred to execution boundary")
	}
	hash, path, err := runtime.RetainCommandEvidence(f.env.dir, runtime.CommandEvidence{Argv: []string{"sh", "-c", "printf original; exit 2"}, Cwd: f.env.dir, ExitCode: 2})
	if err != nil {
		t.Fatal(err)
	}
	refs[hash] = path
	if _, err := f.env.mgr.resolveRecaptureCommand(task, "lint", refs); err == nil {
		t.Fatal("unregistered reference authorized replay")
	}
	if err := f.env.mgr.state.RecordArtifact(ctx, hash, "test_log", path, 0); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	command, err := f.env.mgr.resolveRecaptureCommand(task, "lint", refs)
	if err != nil || command != "printf original; exit 2" {
		t.Fatal("original identity unavailable after restart", err)
	}
	if _, err := f.env.mgr.resolveRecaptureCommand(task, "test", refs); err == nil {
		t.Fatal("mismatched check accepted")
	}
}

func TestLegacyRecaptureInterruptedRemainingBudgetReload(t *testing.T) {
	f := newRecaptureFixture(t, "exit 2")
	f.mode = "exhausted"
	task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	task.Status = release.TaskStatusInProgress
	task.AttemptCount = task.MaxAttempts - 1
	task.Metadata["recapture_outcome"] = "running"
	if err := f.env.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("remaining attempt did not reach exhaustion")
	}
	if f.calls != 1 {
		t.Fatalf("remaining budget dispatched %d checks, want 1", f.calls)
	}
	f.restart(t)
	f.assertTerminal(t, "exhausted", task.MaxAttempts)
	tasks, err := f.env.rel.TasksInStories(context.Background(), []string{"S"})
	if err != nil || len(tasks) != 2 {
		t.Fatalf("diagnostic-free exhaustion invented repair: %+v %v", tasks, err)
	}
	if err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
		t.Fatal("exhausted restart accepted")
	}
	f.restart(t)
	f.assertTerminal(t, "exhausted", task.MaxAttempts)
	if f.calls != 1 {
		t.Fatal("restart refunded an interrupted or exhausted attempt")
	}
}
