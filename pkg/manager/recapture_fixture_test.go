package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// Shared receipt, command resolution, restart and terminal assertions for the
// independently owned recapture boundary/coverage/compatibility scenarios.
type recaptureFixture struct {
	originalReceipt []byte
	env             *schedulerTestEnv
	calls           int
	commands        []string
	mode            string
	cancel          context.CancelFunc
}

func newRecaptureFixture(t *testing.T, command string) *recaptureFixture {
	t.Helper()
	return newRecaptureFixtureOrigin(t, command, false)
}

func newRecaptureFixtureOrigin(t *testing.T, command string, imported bool) *recaptureFixture {
	t.Helper()
	e := newSchedulerTestEnv(t)
	if imported {
		e.mgr.cfg.PlanGenerator = fixedPlanCompletion(`{"goals":[{"id":"G-1","title":"Correct","description":"Correct retained work"}],"stories":[{"id":"S","title":"Correct","goal_id":"G-1","tasks":[{"id":"A","title":"Correct candidate","description":"Correct and verify","verification_script":` + fmt.Sprintf("%q", command) + `,"mode":"afk","max_attempts":3},{"id":"Settings","title":"Dependent","description":"Execute after verified correction","mode":"afk","depends_on":["A"]}]}]}`)
		e.mgr.cfg.PlanReviewer = fixedPlanCompletion(replayReviewFixture)
		if _, err := e.mgr.Plan(context.Background(), replayRequest()); err != nil {
			t.Fatal(err)
		}
	} else {
		createStory(t, e.rel, "S", nil)
		createQueueTask(t, e, "A", nil)
		createQueueTask(t, e, "Settings", []string{"A"})
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	if imported && task.VerificationScript != command {
		t.Fatalf("planner import lost task verification script: %q", task.VerificationScript)
	}
	task.Status, task.AttemptCount, task.VerificationScript = release.TaskStatusInProgress, 1, command
	if err := e.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.state.CreateRun(context.Background(), "A", "", "", e.dir, "workspace-write"); err != nil {
		t.Fatal(err)
	}
	artifacts := recaptureReceipt("verify", 2)
	data, _ := json.Marshal(artifacts)
	if err := e.mgr.state.RecordTaskFailureStep(context.Background(), state.RunStepData{ID: "legacy", RunID: "A", Phase: "verify", Agent: "deterministic-verification", Status: "failed", Metadata: string(data)}, 1); err != nil {
		t.Fatal(err)
	}
	f := &recaptureFixture{env: e}
	e.mgr.cfg.StageExecutor = f
	return f
}

func recaptureReceipt(gate string, exit int) map[string]string {
	raw, _ := json.Marshal([]gates.CheckFailure{{Gate: gate, ExitCode: exit}})
	hash := sha256.Sum256(raw)
	return map[string]string{gates.VerificationFailureReceiptKey: string(raw), gates.VerificationFailureDigestKey: hex.EncodeToString(hash[:])}
}

func (f *recaptureFixture) Execute(ctx context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
	if stage.Name != "verify" {
		if f.mode == "success" {
			return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
		}
		return nil, fmt.Errorf("fixture stops at ordinary task execution")
	}
	f.calls++
	f.commands = append(f.commands, stage.Commands...)
	if f.mode == "exhausted" {
		// A legacy adapter still returning only a normal-exit receipt must not
		// mint repeated diagnostic-free repair tasks.
		err := exec.CommandContext(ctx, "sh", "-c", stage.Commands[0]).Run()
		return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusFailed}, runtime.VerificationCommandFailure(ctx, stage.Name, err)
	}
	if f.mode == "refusal" {
		return nil, fmt.Errorf("admission refused")
	}
	if f.mode == "cancel" {
		f.cancel()
		return nil, ctx.Err()
	}
	executor := blueprint.NewDefaultExecutor(f.env.dir)
	executor.VerificationStages = map[*blueprint.Stage]bool{stage: true}
	var observed error
	executor.OnVerificationFailure = func(_ *blueprint.Stage, err error) { observed = err }
	result, err := executor.Execute(ctx, stage, input)
	if observed != nil {
		return result, observed
	}
	return result, err
}

func (f *recaptureFixture) restart(t *testing.T) {
	t.Helper()
	cfg := f.env.mgr.cfg
	f.env.mgr.Close()
	f.env.closeState()
	reopened, err := state.NewStore(filepath.Join(f.env.dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	closeState := sync.OnceFunc(func() { reopened.Close() })
	t.Cleanup(closeState)
	cfg.StateStore = reopened
	fresh, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fresh.Close() })
	f.env.mgr = fresh
	f.env.closeState = closeState
	f.env.rel, err = fresh.GetInternalReleaseManager()
	if err != nil {
		t.Fatal(err)
	}
}

func (f *recaptureFixture) assertTerminal(t *testing.T, outcome string, attempts int) {
	t.Helper()
	task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
	if err != nil || task.Metadata["recapture_outcome"] != outcome || task.AttemptCount != attempts || task.Status != release.TaskStatusNeedsReview {
		t.Fatalf("terminal state: %+v %v", task, err)
	}
	settings, err := f.env.rel.TaskSnapshot(context.Background(), "Settings")
	if err != nil || settings.Status != release.TaskStatusPending || settings.AttemptCount != 0 {
		t.Fatalf("Settings advanced: %+v %v", settings, err)
	}
}
