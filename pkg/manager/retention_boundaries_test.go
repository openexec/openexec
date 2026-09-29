package manager

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

func assertRetentionPrivacy(t *testing.T, f *retentionFixture, public string) {
	t.Helper()
	for _, secret := range append(append([]string{}, f.secrets...), "ENVIRONMENT_SENTINEL", "UNLISTED_SENTINEL") {
		if strings.Contains(public, secret) {
			t.Fatalf("secret exposed in public state: %s", secret)
		}
	}
	command, err := runtime.ReadCommandEvidence(f.dir, f.hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(command.Stdout) != retentionLimit || len(command.Stderr) != retentionLimit || !command.StdoutTruncated || !command.StderrTruncated {
		t.Fatal("streams not bounded with explicit truncation")
	}
	if len(command.Toolchain) != 1 || len(command.Toolchain["go_version"]) != 128 {
		t.Fatalf("toolchain allowlist/bounds lost: %+v", command.Toolchain)
	}
	if !strings.Contains(strings.Join(command.Argv, " "), f.secrets[0]) || !strings.Contains(command.Stderr, f.secrets[1]) {
		t.Fatal("exact private evidence unavailable")
	}
}

func TestRetentionBoundariesReload(t *testing.T) {
	for _, mode := range []string{"failure", "nil-error", "refusal", "cancel", "success"} {
		t.Run(mode, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			task := *e.rel.GetTask("A")
			task.Status, task.AttemptCount = release.TaskStatusInProgress, 1
			if err := e.rel.UpdateTask(&task); err != nil {
				t.Fatal(err)
			}
			f := newRetentionBoundaryFixture(t, e.dir, mode)
			e.mgr.cfg.StageExecutor = f
			e.mgr.mu.Lock()
			e.mgr.taskQueueActive = true
			e.mgr.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if err := e.mgr.start(ctx, "A", true, WithBlueprint("standard_task"), WithTaskDescription(task.Description)); err != nil {
				t.Fatal(err)
			}
			err := e.mgr.waitTaskQueueRun(ctx, "A")
			if (err == nil) != (mode == "success") {
				t.Fatalf("unexpected execution outcome: %v", err)
			}
			cfg := e.mgr.cfg
			e.mgr.Close()
			e.closeState()
			reopened, err := state.NewStore(filepath.Join(e.dir, "state.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			cfg.StateStore = reopened
			fresh, err := New(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Close()
			rel, err := fresh.GetInternalReleaseManager()
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			id, _ := persisted.Metadata["verification_failure_evidence"].(string)
			steps, err := reopened.ListRunSteps(ctx, "A", 1000, 0)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(steps)
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"COMMAND_VALUE_SENTINEL", "DIAGNOSTIC_VALUE_SENTINEL", "NIL_ERROR_SENTINEL", "REFUSAL_SENTINEL"} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("secret exposed in persisted event history")
				}
			}

			if mode != "failure" {
				if mode == "success" {
					assertRetentionPrivacy(t, f, string(encoded))
					command, err := runtime.ReadCommandEvidence(f.dir, f.hash)
					if err != nil || command.ExitCode != 0 || f.last.Status != runtime.StageStatusCompleted {
						t.Fatal("success semantics lost", err)
					}
				} else if f.hash != "" {
					t.Fatal("unexecuted command manufactured evidence")
				}

				if id != "" {
					t.Fatal("non-verification outcome authorized repair")
				}
				if err := fresh.repairTaskFromRetainedFailure(ctx, "A", id); err == nil {
					t.Fatal("receipt-free repair accepted")
				}
				tasks, err := rel.TasksInStories(ctx, []string{"S"})
				if err != nil || len(tasks) != 1 {
					t.Fatal("spurious repair persisted", err)
				}
				return
			}
			if id == "" {
				t.Fatal("missing failure binding")
			}
			step, err := reopened.GetRunStep(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			assertRetentionPrivacy(t, f, step.Metadata)
			var refs map[string]string
			if err := json.Unmarshal([]byte(step.Metadata), &refs); err != nil {
				t.Fatal(err)
			}
			if !gates.ValidateVerificationFailureArtifacts(refs) {
				t.Fatal("invalid classification")
			}
			if refs[gates.VerificationFailureReceiptKey] != `[{"gate":"test","exit_code":2}]` {
				t.Fatal("diagnostics contaminated classification")
			}
			if !strings.Contains(refs["stage_diagnostics"], retentionMarker) || !strings.Contains(refs["stage_diagnostics"], "[REDACTED]") {
				t.Fatal("useful redacted diagnostic missing")
			}
			artifact, err := reopened.GetArtifact(ctx, f.hash)
			if err != nil || artifact == nil || artifact.Path != f.path {
				t.Fatal("private reference not persisted", err)
			}
			if err := fresh.repairTaskFromRetainedFailure(ctx, "A", id); err != nil {
				t.Fatal(err)
			}
			tasks, err := rel.TasksInStories(ctx, []string{"S"})
			if err != nil || len(tasks) != 2 {
				t.Fatal("repair not persisted", err)
			}
			for _, task := range tasks {
				snapshot, err := rel.TaskSnapshot(ctx, task.ID)
				if err != nil {
					t.Fatal(err)
				}
				assertRetentionPrivacy(t, f, snapshot.Description)
			}
		})
	}
}

func TestRetentionBoundariesPrivateAccess(t *testing.T) {
	f := newRetentionBoundaryFixture(t, t.TempDir(), "failure")
	_, err := f.Execute(context.Background(), &runtime.Stage{Name: "test"}, nil)
	if err == nil {
		t.Fatal("fixture did not fail")
	}
	assertRetentionPrivacy(t, f, f.last.Output+f.last.Diagnostics)
	if _, err := runtime.ReadCommandEvidence(f.dir, "../escape"); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := os.Chmod(f.path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReadCommandEvidence(f.dir, f.hash); err == nil {
		t.Fatal("public file accepted")
	}
	if err := os.Chmod(f.path, 0600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReadCommandEvidence(f.dir, f.hash); err == nil {
		t.Fatal("tampering accepted")
	}
	if err := os.Remove(f.path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.json")
	if err := os.WriteFile(outside, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, f.path); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ReadCommandEvidence(f.dir, f.hash); err == nil {
		t.Fatal("symlink accepted")
	}
}
