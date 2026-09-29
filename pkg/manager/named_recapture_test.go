package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// This admitted boundary deliberately resolves only stage.Name, just as Console
// does. It does not delegate to DefaultExecutor or consume stage.Commands.
type namedRecaptureExecutor struct {
	dir   string
	gate  string
	calls int
}

func (f *namedRecaptureExecutor) Execute(ctx context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	if stage.Name != f.gate {
		return nil, fmt.Errorf("fixture stops at repair execution")
	}
	f.calls++
	command := "printf 'NAMED_CHECK_DIAGNOSTIC\\n' >&2; exit 2"
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = f.dir
	var stdout, stderr runtime.EvidenceBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if cmd.ProcessState == nil {
		return nil, err
	}
	hash, path, captureErr := runtime.RetainCommandEvidence(f.dir, runtime.CommandEvidence{
		Argv: []string{"sh", "-c", command}, Cwd: f.dir, ExitCode: cmd.ProcessState.ExitCode(),
		Stdout: stdout.String(), Stderr: stderr.String(),
	})
	if captureErr != nil {
		return nil, captureErr
	}
	return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusFailed,
			Output: runtime.PublicVerificationStream(&stderr, nil)},
		runtime.VerificationCommandFailureWithEvidence(ctx, stage.Name, err, hash, path)
}

func reopenNamedRecapture(t *testing.T, e *schedulerTestEnv) {
	t.Helper()
	cfg := e.mgr.cfg
	e.mgr.Close()
	e.closeState()
	store, err := state.NewStore(filepath.Join(e.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	closeState := sync.OnceFunc(func() { store.Close() })
	t.Cleanup(closeState)
	cfg.StateStore = store
	e.mgr, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.mgr.Close() })
	e.closeState = closeState
	e.rel, err = e.mgr.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
}

func TestNamedRecaptureIncident(t *testing.T) {
	for _, gate := range []string{"lint", "test"} {
		for _, phase := range []string{"", gate} {
			label := "empty"
			if phase != "" {
				label = "matching"
			}
			t.Run(gate+"/"+label, func(t *testing.T) {
				e := newSchedulerTestEnv(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				createStory(t, e.rel, "S", nil)
				createQueueTask(t, e, "A", nil)
				createQueueTask(t, e, "Settings", []string{"A"})
				task, err := e.rel.TaskSnapshot(ctx, "A")
				if err != nil {
					t.Fatal(err)
				}
				task.Status, task.AttemptCount, task.VerificationScript = release.TaskStatusInProgress, 1, ""
				if err := e.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
				if err := e.mgr.state.CreateRun(ctx, "A", "", "", e.dir, "workspace-write"); err != nil {
					t.Fatal(err)
				}
				data, _ := json.Marshal(recaptureReceipt(gate, 2))
				if err := e.mgr.state.RecordTaskFailureStep(ctx, state.RunStepData{ID: "incident", RunID: "A", Phase: phase, Agent: "deterministic-verification", Status: "failed", Metadata: string(data)}, 1); err != nil {
					t.Fatal(err)
				}
				fixture := &namedRecaptureExecutor{dir: e.dir, gate: gate}
				e.mgr.cfg.StageExecutor = fixture
				reopenNamedRecapture(t, e)
				queueErr := e.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
				reopenNamedRecapture(t, e)
				task, err = e.rel.TaskSnapshot(ctx, "A")
				if err != nil {
					t.Fatal(err)
				}
				if fixture.calls != 1 || task.Metadata["recapture_outcome"] != "failed" || task.Status != release.TaskStatusPending {
					t.Fatalf("named recapture missing: calls=%d status=%s recapture_outcome=%v queue=%v", fixture.calls, task.Status, task.Metadata["recapture_outcome"], queueErr)
				}
				if task.AttemptCount != 2 || task.VerificationScript != "" {
					t.Fatalf("unexpected task: %+v", task)
				}
				id, _ := task.Metadata["verification_failure_evidence"].(string)
				if id == "" || id == "incident" {
					t.Fatal("fresh receipt missing")
				}
				step, err := e.mgr.state.GetRunStep(ctx, id)
				if err != nil || step == nil || step.Phase != gate {
					t.Fatalf("fresh phase lost: %+v %v", step, err)
				}
				var artifacts map[string]string
				if err := json.Unmarshal([]byte(step.Metadata), &artifacts); err != nil {
					t.Fatal(err)
				}
				if artifacts["recapture_definition"] != "current-check-definition" || !strings.Contains(artifacts["stage_output"], "NAMED_CHECK_DIAGNOSTIC") || e.mgr.diagnosticFreeReceipt(artifacts) {
					t.Fatal("actionable current-definition evidence missing", artifacts)
				}
				usable := false
				for hash, path := range artifacts {
					if len(hash) != 64 {
						continue
					}
					ref, err := e.mgr.state.GetArtifact(ctx, hash)
					if err != nil || ref == nil || ref.Path != path {
						t.Fatal("unregistered evidence")
					}
					ev, err := runtime.ReadCommandEvidence(e.dir, hash)
					if err != nil {
						t.Fatal(err)
					}
					usable = ev.Cwd == e.dir && ev.ExitCode == 2 && len(ev.Argv) == 3 && ev.Argv[0] == "sh" && strings.Contains(ev.Argv[2], "exit 2") && strings.Contains(ev.Stderr, "NAMED_CHECK_DIAGNOSTIC")
				}
				if !usable {
					t.Fatal("actual command identity lost")
				}
				tasks, err := e.rel.TasksInStories(ctx, []string{"S"})
				if err != nil {
					t.Fatal(err)
				}
				repairs := 0
				for _, repair := range tasks {
					if repair.Metadata["repair_of"] == "A" {
						repairs++
						if !strings.Contains(repair.Description, "NAMED_CHECK_DIAGNOSTIC") || !strings.Contains(repair.Description, id) {
							t.Fatal("repair lost diagnostic/reference")
						}
					}
				}
				if repairs != 1 || len(tasks) != 3 {
					t.Fatal("expected exactly one repair")
				}
				settings, err := e.rel.TaskSnapshot(ctx, "Settings")
				if err != nil || settings.Status != release.TaskStatusPending || settings.AttemptCount != 0 {
					t.Fatalf("Settings advanced: %+v %v", settings, err)
				}
			})
		}
	}
}
