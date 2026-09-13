package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

func TestTaskFailureReceiptAndDispositionAreAtomic(t *testing.T) {
	for _, scenario := range []string{"commit", "receipt_write_fails", "stale_attempt", "owner_stopped", "completed"} {
		t.Run(scenario, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			task := *e.rel.GetTask("A")
			task.Status, task.AttemptCount = release.TaskStatusInProgress, 2
			// Legacy nil metadata serializes as JSON null, not SQL NULL.
			task.Metadata = nil
			if scenario == "owner_stopped" {
				task.Status = "cancelled"
			}
			if scenario == "completed" {
				task.Status = release.TaskStatusDone
			}
			if err := e.rel.UpdateTask(&task); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if err := e.mgr.state.CreateRun(ctx, "A", "", "", e.dir, "workspace-write"); err != nil {
				t.Fatal(err)
			}
			if scenario == "receipt_write_fails" {
				_, err := e.mgr.state.GetDB().Exec(`CREATE TRIGGER refuse_failure_receipt BEFORE INSERT ON run_steps BEGIN SELECT RAISE(ABORT,'injected storage failure'); END`)
				if err != nil {
					t.Fatal(err)
				}
			}
			attempt := 2
			if scenario == "stale_attempt" {
				attempt = 1
			}
			err := e.mgr.state.RecordTaskFailureStep(ctx, state.RunStepData{
				ID: "failure", RunID: "A", Agent: "deterministic-verification", Status: "failed", Metadata: `{"receipt":"fixture"}`,
			}, attempt)
			retained, readErr := e.rel.TaskSnapshot(ctx, "A")
			if readErr != nil {
				t.Fatal(readErr)
			}
			receipt, _ := e.mgr.state.GetRunStep(ctx, "failure")
			if scenario == "commit" {
				if err != nil || receipt == nil || retained.Status != release.TaskStatusFailed || retained.Metadata["verification_failure_evidence"] != "failure" {
					t.Fatalf("receipt/task did not commit together: %v %#v %#v", err, receipt, retained)
				}
			} else if err == nil || receipt != nil || retained.Status != task.Status || retained.Metadata["verification_failure_evidence"] != nil {
				t.Fatalf("refusal left partial state: %v %#v %#v", err, receipt, retained)
			}
			if retained.AttemptCount != 2 {
				t.Fatal("attempt accounting changed")
			}
			if scenario == "receipt_write_fails" && !strings.Contains(err.Error(), "injected storage failure") {
				t.Fatal("test did not reach receipt write", err)
			}
		})
	}
}
