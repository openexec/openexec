package manager

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/pkg/runtime"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
)

func correctionFixture(t *testing.T, checkCommands ...string) (*recaptureFixture, release.TaskCorrection, *release.Task) {
	t.Helper()
	f := newRecaptureFixture(t, "test -f corrected.go")
	f.mode = "success"
	e := f.env
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(e.dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(".gitignore", "state.db*\n.openexec/*\n!.openexec/config.json\n")
	write("corrected.go", "package corrected\n")
	for _, args := range [][]string{{"init", "-b", "retained"}, {"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "retained candidate"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = e.dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s %v", args, out, err)
		}
	}
	task, err := e.rel.TaskSnapshot(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	// Create a real native prerequisite before exhausting the original.
	task.Git = &release.TaskGitInfo{Branch: "retained", Commits: []string{"retained-history"}}
	if err = e.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	repair, err := e.rel.CreateFailureRepair(context.Background(), "A", "earlier-receipt", "repair prerequisite")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.rel.SetTaskStatus(repair.ID, release.TaskStatusDone); err != nil {
		t.Fatal(err)
	}
	repair, err = e.rel.TaskSnapshot(context.Background(), repair.ID)
	if err != nil {
		t.Fatal(err)
	}
	task, err = e.rel.TaskSnapshot(context.Background(), "A")
	if err != nil {
		t.Fatal(err)
	}
	task.Status = release.TaskStatusFailed
	task.AttemptCount = 3
	if err = e.rel.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	manifest, err := knowledge.BuildScanManifest(e.dir)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.mgr.state.GetDB().Exec(`INSERT INTO repositories(id,persisted_uuid) VALUES('repo','repo');
 INSERT INTO checkouts(id,repository_id,root_path) VALUES('checkout','repo','/candidate');
 INSERT INTO worktrees(id,repository_id,checkout_id,root_path) VALUES('worktree','repo','checkout','/candidate');
 INSERT INTO graph_generations(id,schema_version,repository_id,checkout_id,worktree_id,worktree_state_hash,configuration_digest,extractor_version,manifest_hash,status) VALUES('graph',1,'repo','checkout','worktree',?,'config','extractor','manifest','current')`, manifest.WorktreeStateHash)
	if err != nil {
		t.Fatal(err)
	}
	command := "test -f corrected.go"
	if len(checkCommands) > 0 {
		command = checkCommands[0]
	}
	plan, err := e.mgr.state.CreateValidationPlanRevision(context.Background(), state.ValidationPlanRevision{TaskID: "A", GenerationID: "graph", WorktreeStateHash: manifest.WorktreeStateHash, Status: "accepted", Items: []state.ValidationItem{
		{ID: "check", Source: "policy", Disposition: "accepted", Requirement: "blocking", Criterion: "Corrected candidate passes", CommandArgv: []string{"sh", "-c", command}},
		{ID: "second", Source: "policy", Disposition: "accepted", Requirement: "required", Criterion: "Fresh second check", CommandArgv: []string{"sh", "-c", "test -s corrected.go"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path, branch, digest, err := e.mgr.CorrectionCandidate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return f, release.TaskCorrection{DecisionRef: "fixture-owner:explicit-correction", TaskID: "A", EvidenceID: "legacy", CandidatePath: path, Branch: branch, CandidateDigest: digest, PlanID: plan.ID, StateHash: manifest.WorktreeStateHash}, repair
}

func assertCorrectionReload(t *testing.T, f *recaptureFixture, c release.TaskCorrection, repair *release.Task, done bool) {
	t.Helper()
	f.restart(t)
	ctx := context.Background()
	a, err := f.env.rel.TaskSnapshot(ctx, "A")
	if err != nil {
		t.Fatal(err)
	}
	got, err := release.CorrectionForTask(a)
	if err != nil {
		t.Fatal(err)
	}
	if a.AttemptCount != 3 || a.MaxAttempts != 3 || a.Git.Branch != c.Branch || !reflect.DeepEqual(a.Git.Commits, []string{"retained-history"}) || got.CandidateDigest != c.CandidateDigest || !got.Consumed {
		t.Fatalf("history/candidate changed: %+v %+v", a, got)
	}
	again, err := f.env.rel.TaskSnapshot(ctx, repair.ID)
	if err != nil || !reflect.DeepEqual(again, repair) {
		t.Fatalf("completed repair changed: %+v %v", again, err)
	}
	step, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
	originalReceipt, _ := json.Marshal(recaptureReceipt("verify", 2))
	if f.originalReceipt != nil {
		originalReceipt = f.originalReceipt
	}
	if err != nil || step == nil || step.Status != "failed" || step.RunID != "A" || step.Agent.String != "deterministic-verification" || step.Metadata != string(originalReceipt) {
		t.Fatal("historical receipt changed", err)
	}
	dep, err := f.env.rel.TaskSnapshot(ctx, "Settings")
	if err != nil {
		t.Fatal(err)
	}
	if done {
		if a.Status != release.TaskStatusDone || dep.Status != release.TaskStatusDone || got.Outcome != "completed" {
			t.Fatalf("queue incomplete: %+v %+v", a, dep)
		}
	} else if a.Status == release.TaskStatusDone || dep.Status != release.TaskStatusPending || dep.AttemptCount != 0 {
		t.Fatal("unverified dependency escaped")
	}
	data, _ := json.Marshal(map[string]interface{}{"task": a, "repair": again, "dependent": dep})
	t.Logf("RELOADED_CORRECTION %s", data)
}

func TestCorrectionNativeQueueSuccess(t *testing.T) {
	testCorrectionQueueSuccess(t, false)
}

func TestCorrectionDiagnosticQueueSuccess(t *testing.T) {
	testCorrectionQueueSuccess(t, true)
}

func testCorrectionQueueSuccess(t *testing.T, diagnostic bool) {
	f, c, repair := correctionFixture(t)
	ctx := context.Background()
	if diagnostic {
		refs := recaptureReceipt("verify", 124)
		payload := `[{"gate":"verify","exit_code":124,"command":"sh check.sh","output":"verification timeout: deadline exceeded\n"}]`
		refs[gates.VerificationFailureReceiptKey] = payload
		refs[gates.VerificationFailureDigestKey] = fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
		data, err := json.Marshal(refs)
		f.originalReceipt = data
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE run_steps SET metadata=? WHERE id='legacy'`, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := boundaryRun(f); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Fatalf("fresh required checks=%d", f.calls)
	}
	assertCorrectionReload(t, f, c, repair, true)
	if err := boundaryRun(f); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Fatal("completion restart repeated checks")
	}
	var links int
	if err := f.env.mgr.state.GetDB().QueryRow(`SELECT count(*) FROM validation_evidence_links WHERE status='passed'`).Scan(&links); err != nil || links != 2 {
		t.Fatal("fresh evidence absent", links, err)
	}
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err == nil {
		t.Fatal("completion replay granted more allowance")
	}
}

func TestCorrectionNativeQueueRefusals(t *testing.T) {
	for _, mode := range []string{"absent", "candidate", "task_binding", "review", "dependency", "failed_check", "cancel", "admitted_restart", "admitted_failure_restart", "stale_plan"} {
		t.Run(mode, func(t *testing.T) {
			f, c, repair := correctionFixture(t)
			ctx := context.Background()
			if mode != "absent" {
				if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "candidate":
				if err := os.WriteFile(filepath.Join(f.env.dir, "corrected.go"), []byte("package changed\n"), 0644); err != nil {
					t.Fatal(err)
				}
			case "task_binding":
				_, err := f.env.mgr.state.GetDB().Exec(`UPDATE tasks SET metadata=json_set(metadata,'$.task_correction.task_id','Settings') WHERE id='A'`)
				if err != nil {
					t.Fatal(err)
				}
			case "review":
				_, err := f.env.mgr.state.GetDB().Exec(`UPDATE tasks SET needs_review=1 WHERE id='A'`)
				if err != nil {
					t.Fatal(err)
				}
			case "dependency":
				_, err := f.env.mgr.state.GetDB().Exec(`UPDATE tasks SET status='pending' WHERE id=?`, repair.ID)
				if err != nil {
					t.Fatal(err)
				}
				f.mode = "refusal"
			case "failed_check":
				f.mode = "refusal"
			case "cancel":
				f.mode = "cancel"
			case "admitted_restart", "admitted_failure_restart":
				s, err := release.NewSQLiteStore(f.env.mgr.state.GetDB())
				if err != nil {
					t.Fatal(err)
				}
				if err = s.AdmitTaskCorrection(ctx, c); err != nil {
					t.Fatal(err)
				}
				if mode == "admitted_failure_restart" {
					data, _ := json.Marshal(recaptureReceipt("verify", 7))
					if err := f.env.mgr.state.RecordTaskFailureStep(ctx, state.RunStepData{ID: "interrupted-fresh", RunID: "A", Status: "failed", Agent: "deterministic-verification", Metadata: string(data)}, 3); err != nil {
						t.Fatal(err)
					}
				}
			case "stale_plan":
				_, err := f.env.mgr.state.GetDB().Exec(`UPDATE graph_generations SET status='stale'`)
				if err != nil {
					t.Fatal(err)
				}
			}
			f.restart(t)
			err := boundaryRun(f)
			if err == nil {
				t.Fatal("refusal accepted")
			}
			if mode == "failed_check" || mode == "cancel" || mode == "admitted_restart" || mode == "admitted_failure_restart" {
				assertCorrectionReload(t, f, c, repair, false)
				calls := f.calls
				if err := boundaryRun(f); err == nil {
					t.Fatal("spent restart accepted")
				}
				if f.calls != calls {
					t.Fatal("restart renewed allowance")
				}
			} else if mode != "dependency" && f.calls != 0 {
				t.Fatalf("refusal dispatched %d checks: %v", f.calls, err)
			}
		})
	}
}

func TestCorrectionCompletionObligations(t *testing.T) {
	f, c, repair := correctionFixture(t)
	ctx := context.Background()
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	s, err := release.NewSQLiteStore(f.env.mgr.state.GetDB())
	if err != nil {
		t.Fatal(err)
	}
	if err = s.AdmitTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishTaskCorrection(ctx, c, true); err == nil || !strings.Contains(err.Error(), "without supported evidence") {
		t.Fatalf("completion bypass: %v", err)
	}

	_, err = f.env.mgr.state.GetDB().Exec(`INSERT INTO completion_claims(id,validation_item_id,predicate,scope,status,repository_state_hash)
    SELECT 'old-'||id,id,'validation_item_passed','tests','supported',? FROM validation_items WHERE plan_revision_id=?`, c.StateHash, c.PlanID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishTaskCorrection(ctx, c, true); err == nil || !strings.Contains(err.Error(), "fresh evidence") {
		t.Fatalf("old claims bypass fresh correction: %v", err)
	}
	assertCorrectionReload(t, f, c, repair, false)
	if err := boundaryRun(f); err == nil {
		t.Fatal("restart repeated incomplete admission")
	}
}

// The executor still uses the native pipeline's cancellation and trusted check
// adapter, even when stopped between admission and a subprocess result.
type correctionStopExecutor struct {
	fixture *recaptureFixture
	started chan struct{}
}

func (e *correctionStopExecutor) Execute(ctx context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
	close(e.started)
	<-ctx.Done()
	return nil, ctx.Err()
}
func TestCorrectionNativeStopAndFailure(t *testing.T) {
	for _, mode := range []string{"stop", "check_failure", "drift"} {
		t.Run(mode, func(t *testing.T) {
			command := "test -f corrected.go"
			if mode == "check_failure" {
				command = "printf 'fresh failure\\n' >&2; exit 2"
			}
			if mode == "drift" {
				command = "printf 'package moved\\n' > corrected.go"
			}
			f, c, repair := correctionFixture(t, command)
			ctx := context.Background()
			if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
				t.Fatal(err)
			}
			if mode == "stop" {
				executor := &correctionStopExecutor{fixture: f, started: make(chan struct{})}
				f.env.mgr.cfg.StageExecutor = executor
				result := make(chan error, 1)
				go func() { result <- boundaryRun(f) }()
				select {
				case <-executor.started:
				case <-time.After(5 * time.Second):
					t.Fatal("check never started")
				}
				if err := f.env.mgr.Stop("A"); err != nil {
					t.Fatal(err)
				}
				select {
				case err := <-result:
					if err == nil {
						t.Fatal("Stop completed task")
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Stop failed to drain")
				}
			} else if err := boundaryRun(f); err == nil {
				t.Fatal("failed/drifting candidate completed")
			}
			assertCorrectionReload(t, f, c, repair, false)
			calls := f.calls
			if err := boundaryRun(f); err == nil {
				t.Fatal("terminal correction resumed")
			}
			if f.calls != calls {
				t.Fatal("terminal correction redispatched")
			}
		})
	}
}
