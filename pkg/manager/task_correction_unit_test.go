package manager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

// The unit executor never invokes a shell or provider. Native command execution
// is independently verified by TestCorrectionTaskScriptJourneys.
type correctionUnitExecutor struct {
	calls  int
	fail   bool
	before func()
}

func (e *correctionUnitExecutor) Execute(_ context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	e.calls++
	if e.before != nil {
		e.before()
	}
	if e.fail {
		return nil, fmt.Errorf("unit executor refused")
	}
	return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
}
func TestCorrectionUnitReconcile(t *testing.T) {
	for _, mode := range []string{"planned", "no_plan", "refused", "optional", "deduplicated"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			db := f.env.mgr.state.GetDB()
			switch mode {
			case "no_plan":
				if _, err := db.Exec(`DELETE FROM validation_items; DELETE FROM validation_plan_revisions`); err != nil {
					t.Fatal(err)
				}
				c.PlanID, c.StateHash = "", ""
			case "optional":
				if _, err := db.Exec(`UPDATE validation_items SET requirement='optional'; UPDATE tasks SET verification_script='' WHERE id='A'`); err != nil {
					t.Fatal(err)
				}
			case "deduplicated":
				if _, err := db.Exec(`UPDATE validation_items SET command_argv='["sh","-c","test -f corrected.go"]'`); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
				t.Fatal(err)
			}
			executor := &correctionUnitExecutor{fail: mode == "refused"}
			f.env.mgr.cfg.StageExecutor = executor
			task, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			err = f.env.mgr.reconcileTaskCorrection(ctx, task)
			if (err != nil) != (mode == "refused") {
				t.Fatalf("reconcile %v", err)
			}
			f.restart(t)
			task, err = f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			got, err := release.CorrectionForTask(task)
			if err != nil || !got.Consumed || task.AttemptCount != 3 || task.MaxAttempts != 3 {
				t.Fatalf("lost admission: %+v %v", task, err)
			}
			if (task.Status == release.TaskStatusDone) != (mode != "refused") {
				t.Fatalf("wrong completion: %+v", task)
			}
			want := 2
			if mode == "no_plan" || mode == "refused" || mode == "deduplicated" {
				want = 1
			}
			if mode == "optional" {
				want = 0
			}
			if executor.calls != want {
				t.Fatalf("checks=%d want %d", executor.calls, want)
			}
			if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err == nil {
				t.Fatal("spent allowance renewed")
			}
		})
	}
}

func TestCorrectionUnitQueueAndPersistenceErrors(t *testing.T) {
	for _, mode := range []string{"success", "step", "link", "disposition", "no_authority", "refused", "waiting", "study"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			ctx := context.Background()
			if mode != "no_authority" {
				if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
					t.Fatal(err)
				}
			}
			executor := &correctionUnitExecutor{fail: mode == "refused"}
			f.env.mgr.cfg.StageExecutor = executor
			query := map[string]string{
				"step":        `CREATE TRIGGER refuse_proof BEFORE INSERT ON run_steps WHEN NEW.id LIKE 'correction-%' BEGIN SELECT RAISE(ABORT,'proof refused'); END`,
				"link":        `CREATE TRIGGER refuse_proof BEFORE INSERT ON validation_evidence_links BEGIN SELECT RAISE(ABORT,'link refused'); END`,
				"disposition": `CREATE TRIGGER refuse_proof BEFORE UPDATE ON tasks WHEN NEW.status='done' BEGIN SELECT RAISE(ABORT,'completion refused'); END`,
				"waiting":     `UPDATE tasks SET needs_review=1 WHERE id='A'`,
			}[mode]
			if query != "" {
				if _, err := f.env.mgr.state.GetDB().Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}, IsStudy: mode == "study", Mode: "afk"})
			success := mode == "success" || mode == "study"
			if (err == nil) != success {
				t.Fatalf("queue success=%t: %v", success, err)
			}
			f.restart(t)
			task, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			if (task.Status == release.TaskStatusDone) != success || task.AttemptCount != 3 {
				t.Fatalf("persisted task: %+v", task)
			}
			if !success {
				if err := f.env.mgr.ExecuteTasks(ctx, RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}); err == nil {
					t.Fatal("failed proof renewed")
				}
			}
		})
	}
}

func TestCorrectionUnitQueueStoreGuards(t *testing.T) {
	for _, mode := range []string{"attempt", "exhaustion", "empty", "start", "complete", "scope"} {
		t.Run(mode, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			if mode != "empty" {
				createQueueTask(t, e, "A", nil)
			}
			e.mgr.cfg.StageExecutor = &correctionUnitExecutor{}
			query := map[string]string{
				"attempt":    `CREATE TRIGGER refuse BEFORE UPDATE ON tasks WHEN NEW.status='in_progress' BEGIN SELECT RAISE(ABORT,'attempt refused'); END`,
				"exhaustion": `UPDATE tasks SET status='failed',attempt_count=max_attempts; CREATE TRIGGER refuse BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'exhaustion refused'); END`,
				"complete":   `CREATE TRIGGER refuse BEFORE UPDATE ON tasks WHEN NEW.status='done' BEGIN SELECT RAISE(ABORT,'completion refused'); END`,
			}[mode]
			if query != "" {
				if _, err := e.mgr.state.GetDB().Exec(query); err != nil {
					t.Fatal(err)
				}
			}
			opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
			if mode == "start" {
				e.mgr.cfg.AgentsFS = nil
				e.mgr.cfg.WorkDir = "/nonexistent-unit-workdir"
			}
			if mode == "scope" {
				opts.StoryIDs = []string{"absent"}
			}
			if err := e.mgr.ExecuteTasks(context.Background(), opts); err == nil {
				t.Fatal("invalid queue/store operation accepted")
			}
		})
	}
}

// Shared start/legacy scheduler functions remain in the complete denominator.
// Invalid blueprints fail before any provider call, even with provider options.
func TestCorrectionUnitSharedStart(t *testing.T) {
	e := newSchedulerTestEnv(t)
	config := `{"execution":{"api_provider":"openai_compat","api_base_url":"http://127.0.0.1:1","api_key":"unit-placeholder","api_model":"unit","bitnet_routing":true,"bitnet_model":"unit"},"quality_gates":{"no_stubs_rules":{"TODO":"low"},"production_ready_skip":["readme"]}}`
	if err := os.WriteFile(filepath.Join(e.dir, ".openexec/config.json"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Start(context.Background(), "missing-blueprint", WithBlueprint("missing-unit-blueprint")); err != nil {
		t.Fatal(err)
	}
	if err := e.mgr.Wait(context.Background(), "missing-blueprint"); err != nil {
		t.Fatal(err)
	}
	info, err := e.mgr.Status("missing-blueprint")
	if err != nil || info.Status != StatusError || !strings.Contains(info.Error, "unknown blueprint") {
		t.Fatalf("invalid blueprint not refused before execution: %+v %v", info, err)
	}
}

func TestCorrectionUnitLegacyScheduler(t *testing.T) {
	for _, mode := range []string{"done", "options", "owned"} {
		t.Run(mode, func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			e.mgr.cfg.StageExecutor = &correctionUnitExecutor{}
			if mode == "done" {
				if err := e.rel.SetTaskStatus("A", release.TaskStatusDone); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "owned" {
				e.mgr.taskQueueActive = true
				defer func() { e.mgr.taskQueueActive = false }()
			}
			err := e.mgr.ExecuteTasks(context.Background(), RunOptions{IsStudy: true, Mode: "afk"})
			if (err != nil) != (mode == "owned") {
				t.Fatalf("legacy scheduler %s: %v", mode, err)
			}
			task, err := e.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			if mode != "owned" && task.Status != release.TaskStatusDone {
				t.Fatal("legacy task did not complete")
			}
		})
	}
}

func TestCorrectionUnitInterruptAndAdmissionErrors(t *testing.T) {
	for _, mode := range []string{"cancel", "stop", "admission", "active", "receipt_read", "failure_write", "malformed_outcome"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
				t.Fatal(err)
			}
			task, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			executor := &correctionUnitExecutor{}
			f.env.mgr.cfg.StageExecutor = executor
			f.env.mgr.cfg.TaskTimeout = time.Second
			switch mode {
			case "cancel":
				executor.before = cancel
			case "stop":
				executor.before = func() {
					if err := f.env.mgr.Stop("A"); err != nil {
						panic(err)
					}
				}
			case "active":
				f.env.mgr.pipelines["A"] = &entry{info: PipelineInfo{Status: StatusRunning}}
				defer func() { delete(f.env.mgr.pipelines, "A") }()
			case "receipt_read":
				if _, err := f.env.mgr.state.GetDB().Exec(`DROP TABLE run_steps`); err != nil {
					t.Fatal(err)
				}
			case "admission":
				if _, err := f.env.mgr.state.GetDB().Exec(`CREATE TRIGGER refuse BEFORE UPDATE ON tasks WHEN NEW.status='in_progress' BEGIN SELECT RAISE(ABORT,'admission refused'); END`); err != nil {
					t.Fatal(err)
				}
			case "failure_write":
				executor.fail = true
				if _, err := f.env.mgr.state.GetDB().Exec(`CREATE TRIGGER refuse BEFORE UPDATE ON tasks WHEN NEW.status='needs_review' BEGIN SELECT RAISE(ABORT,'disposition unavailable'); END`); err != nil {
					t.Fatal(err)
				}
			case "malformed_outcome":
				if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE tasks SET metadata=json_set(metadata,'$.task_correction.reason','unconsumed invalid reason') WHERE id='A'`); err != nil {
					t.Fatal(err)
				}
				task, err = f.env.rel.TaskSnapshot(ctx, "A")
				if err != nil {
					t.Fatal(err)
				}
			}
			err = f.env.mgr.reconcileTaskCorrection(ctx, task)
			if mode == "malformed_outcome" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("interruption/storage error accepted")
			}
			after, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil || after.Status == release.TaskStatusDone || after.AttemptCount != 3 {
				t.Fatalf("interrupted task completed: %+v %v", after, err)
			}
			if mode == "cancel" || mode == "stop" {
				correction, err := release.CorrectionForTask(after)
				want := map[string]string{"cancel": "cancelled", "stop": "stopped"}[mode]
				if err != nil || !correction.Consumed || correction.Outcome != want {
					t.Fatalf("lost terminal cause: %+v %v", correction, err)
				}
			}
		})
	}
}

type correctionBlockingUnitExecutor struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
	checks  atomic.Int32
}

func (e *correctionBlockingUnitExecutor) Execute(ctx context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	if stage.Name == "verify" {
		e.checks.Add(1)
		e.once.Do(func() { close(e.entered) })
		select {
		case <-e.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
}
func TestCorrectionUnitConcurrentQueues(t *testing.T) {
	f, c, repair := correctionFixture(t)
	createQueueTask(t, f.env, "Independent", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	executor := &correctionBlockingUnitExecutor{entered: make(chan struct{}), release: make(chan struct{})}
	f.env.mgr.cfg.StageExecutor = executor
	unlock := sync.OnceFunc(func() { close(executor.release) })
	defer unlock()
	done := make(chan error, 1)
	opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
	go func() { done <- f.env.mgr.ExecuteTasks(ctx, opts) }()
	select {
	case <-executor.entered:
	case <-ctx.Done():
		t.Fatal("first queue never admitted")
	}
	if err := f.env.mgr.ExecuteTasks(ctx, opts); err == nil {
		t.Fatal("competing queue admitted")
	}
	if executor.checks.Load() != 1 {
		t.Fatal("duplicate verification before release")
	}
	unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if executor.checks.Load() != 2 {
		t.Fatal("duplicate required checks")
	}
	assertCorrectionReload(t, f, c, repair, true)
	independent, err := f.env.rel.TaskSnapshot(context.Background(), "Independent")
	if err != nil || independent.Status != release.TaskStatusDone {
		t.Fatal("independent work failed to drain", err)
	}
	if err := f.env.mgr.ExecuteTasks(ctx, opts); err != nil {
		t.Fatal(err)
	}
	if executor.checks.Load() != 2 {
		t.Fatal("restart repeated checks")
	}
}
