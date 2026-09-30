package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// Use the Console's public API and argument shape: gate, process error,
// strings.Join(argv, " "), combined output tail. No private-evidence helper.
type admittedAdapterExecutor struct {
	gate, command, output string
	noisy, nilResult      bool
	calls                 int
}

func (a *admittedAdapterExecutor) Execute(ctx context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	result := &runtime.StageResult{StageName: stage.Name, Attempt: 1, Status: runtime.StageStatusCompleted}
	if stage.Name != a.gate {
		return result, nil
	}
	a.calls++
	script := "exit 2"
	if a.noisy {
		script = "i=0; while [ $i -lt 2000 ]; do printf 'adapter noise line\\n'; i=$((i+1)); done; printf 'ADAPTER_DIAGNOSTIC_TAIL\\n' >&2; exit 2"
	}
	argv := []string{"/bin/sh", "-c", script}
	output, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
	a.command, a.output = strings.Join(argv, " "), string(output)
	result.Status = runtime.StageStatusFailed
	result.Output = a.command + "\n" + a.output
	if a.nilResult {
		result = nil
	}
	return result, runtime.VerificationCommandFailureWithOutput(ctx, stage.Name, err, a.command, a.output)
}

func TestAdmittedAdapterFailureReloadAndRepair(t *testing.T) {
	for _, gate := range []string{"lint", "test"} {
		for _, noisy := range []bool{false, true} {
			for _, nilResult := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/noisy=%t/nil=%t", gate, noisy, nilResult), func(t *testing.T) {
					e := newSchedulerTestEnv(t)
					createStory(t, e.rel, "S", nil)
					createQueueTask(t, e, "A", nil)
					task := *e.rel.GetTask("A")
					task.Status, task.AttemptCount = release.TaskStatusInProgress, 1
					if err := e.rel.UpdateTask(&task); err != nil {
						t.Fatal(err)
					}
					adapter := &admittedAdapterExecutor{gate: gate, noisy: noisy, nilResult: nilResult}
					e.mgr.cfg.StageExecutor = adapter
					e.mgr.taskQueueActive = true
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					if err := e.mgr.start(ctx, "A", true, WithBlueprint("standard_task"), WithTaskDescription(task.Description)); err != nil {
						t.Fatal(err)
					}
					if err := e.mgr.waitTaskQueueRun(ctx, "A"); err == nil {
						t.Fatal("exit 2 accepted")
					}
					failed, err := e.rel.TaskSnapshot(ctx, "A")
					if err != nil || failed.Status != release.TaskStatusFailed {
						t.Fatalf("failed task: %+v %v", failed, err)
					}
					id, _ := failed.Metadata["verification_failure_evidence"].(string)
					if id == "" {
						t.Fatal("missing persisted receipt")
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
					step, err := reopened.GetRunStep(ctx, id)
					if err != nil || step == nil {
						t.Fatal("missing reloaded step", err)
					}
					var artifacts map[string]string
					if err := json.Unmarshal([]byte(step.Metadata), &artifacts); err != nil {
						t.Fatal(err)
					}
					if step.Phase != gate || step.Status != "failed" || !gates.ValidateVerificationFailureArtifacts(artifacts) {
						t.Fatal("lost failed stage or receipt")
					}
					var checks []gates.CheckFailure
					if err := json.Unmarshal([]byte(artifacts[gates.VerificationFailureReceiptKey]), &checks); err != nil || len(checks) != 1 {
						t.Fatal("invalid receipt", err)
					}
					check := checks[0]
					if check.Gate != gate || check.ExitCode != 2 || check.Command != adapter.command {
						t.Fatalf("lost command identity: %+v", check)
					}
					if noisy {
						if len(adapter.output) <= 6000 || len(check.Output) > 6000+len("…") || !strings.HasSuffix(check.Output, "ADAPTER_DIAGNOSTIC_TAIL\n") {
							t.Fatal("lost bounded output tail")
						}
					} else if check.Output != "" {
						t.Fatal("silent output invented")
					}
					// Command-bearing receipts count only when their digest validates.
					invalid := map[string]string{
						gates.VerificationFailureReceiptKey: artifacts[gates.VerificationFailureReceiptKey],
						gates.VerificationFailureDigestKey:  "invalid",
					}
					if !fresh.diagnosticFreeReceipt(invalid) {
						t.Fatal("invalid receipt accepted as evidence")
					}
					if fresh.diagnosticFreeReceipt(artifacts) {
						t.Fatal("adapter receipt incorrectly requires recapture")
					}
					if err := fresh.repairTaskFromRetainedFailure(ctx, "A", id); err != nil {
						t.Fatal(err)
					}
					rel, err := fresh.GetInternalReleaseManager()
					if err != nil {
						t.Fatal(err)
					}
					tasks, err := rel.TasksInStories(ctx, []string{"S"})
					if err != nil || len(tasks) != 2 {
						t.Fatal("one repair not created", err)
					}
					repairs := 0
					for _, task := range tasks {
						if task.Metadata["repair_of"] != "A" {
							continue
						}
						repairs++
						repair, err := rel.TaskSnapshot(ctx, task.ID)
						if err != nil || !strings.Contains(repair.Description, id) || !strings.Contains(repair.Description, step.Metadata) {
							t.Fatal("repair context lost receipt", err)
						}
					}
					if repairs != 1 {
						t.Fatal("missing unique repair")
					}
					if adapter.calls != 1 {
						t.Fatal("adapter failure rerun instead of repaired")
					}
					if err := fresh.repairTaskFromRetainedFailure(ctx, "A", "missing-evidence"); err == nil {
						t.Fatal("missing receipt authorized repair")
					}
				})
			}
		}
	}
}
