package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

func TestCorrectionExhaustionControls(t *testing.T) {
	for _, mode := range []string{"continuing_failure", "unchanged_failure", "no_authority", "blocked_dependents", "independent_drain"} {
		t.Run(mode, func(t *testing.T) {
			command := "printf 'continuing failure\\n' >&2; exit 7"
			if mode == "unchanged_failure" {
				command = "exit 2"
			}
			f, c, repair := correctionFixture(t, command)
			createQueueTask(t, f.env, "Independent", nil)
			ctx := context.Background()
			if mode != "no_authority" {
				if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
					t.Fatal(err)
				}
			}
			err := boundaryRun(f)
			var boundary *TaskQueueBoundary
			if !errors.As(err, &boundary) || strings.Contains(err.Error(), "repair task refused") {
				t.Fatalf("wrong terminal cause: %v", err)
			}
			if mode != "no_authority" {
				assertCorrectionReload(t, f, c, repair, false)
			} else {
				f.restart(t)
			}
			a, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil {
				t.Fatal(err)
			}
			if a.Metadata["verification_failure_evidence"] != "legacy" || a.AttemptCount != 3 {
				t.Fatal("original failure/history replaced")
			}
			original, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
			if err != nil || original == nil {
				t.Fatal("missing original receipt", err)
			}
			if mode == "no_authority" {
				if a.Metadata["exhaustion"] == nil || a.Metadata["task_correction"] != nil || f.calls != 0 {
					t.Fatalf("unauthorized execution: %+v", a)
				}
			} else {
				got, err := release.CorrectionForTask(a)
				if err != nil || got.Outcome != "continuing_failure" || got.Reason == "" || got.FreshEvidenceID == "" || got.FreshEvidenceID == c.EvidenceID {
					t.Fatalf("missing disposition: %+v %v", got, err)
				}
				fresh, err := f.env.mgr.state.GetRunStep(ctx, got.FreshEvidenceID)
				if err != nil || fresh == nil || fresh.Status != "failed" || fresh.Metadata == original.Metadata {
					t.Fatalf("fresh failure not retained: %+v %v", fresh, err)
				}
				var artifacts map[string]string
				if err := json.Unmarshal([]byte(fresh.Metadata), &artifacts); err != nil {
					t.Fatal(err)
				}
				wantExit := 7
				if mode == "unchanged_failure" {
					wantExit = 2
				}
				var receipts []gates.CheckFailure
				if err := json.Unmarshal([]byte(artifacts[gates.VerificationFailureReceiptKey]), &receipts); err != nil || len(receipts) != 1 || receipts[0].ExitCode != wantExit {
					t.Fatal("fresh check cause lost", err)
				}
				payloads := 0
				for hash := range artifacts {
					if len(hash) != 64 {
						continue
					}
					evidence, err := runtime.ReadCommandEvidence(f.env.dir, hash)
					if err != nil || evidence.ExitCode != wantExit || evidence.Cwd != f.env.dir || len(evidence.Argv) != 3 || evidence.Argv[2] != command {
						t.Fatal("persisted process evidence mismatch", err)
					}
					payloads++
				}
				if payloads != 1 {
					t.Fatal("missing process evidence", payloads)
				}
				data, _ := json.Marshal(fresh)
				t.Logf("RELOADED_FAILURE %s", data)
			}
			independent, err := f.env.rel.TaskSnapshot(ctx, "Independent")
			if err != nil || independent.Status != release.TaskStatusDone {
				t.Fatal("independent work did not drain", err)
			}
			dependent, err := f.env.rel.TaskSnapshot(ctx, "Settings")
			if err != nil || dependent.Status != release.TaskStatusPending || dependent.AttemptCount != 0 {
				t.Fatal("dependent escaped", err)
			}
			tasks, err := f.env.rel.TasksInStories(ctx, []string{"S"})
			if err != nil || len(tasks) != 4 {
				t.Fatal("another repair created", len(tasks), err)
			}
			data, _ := json.Marshal(a)
			t.Logf("RELOADED_EXHAUSTION %s", data)
			calls := f.calls
			if err := boundaryRun(f); !errors.As(err, &boundary) {
				t.Fatalf("reentry lost boundary: %v", err)
			}
			if f.calls != calls {
				t.Fatal("reentry renewed authority")
			}
			if mode != "no_authority" && f.env.mgr.AuthorizeTaskCorrection(ctx, c) == nil {
				t.Fatal("reauthorization renewed authority")
			}
		})
	}
}

type correctionControlExecutor struct {
	f     *recaptureFixture
	mode  string
	calls int
	c     release.TaskCorrection
}

func (e *correctionControlExecutor) Execute(ctx context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
	if stage.Name != "verify" {
		return e.f.Execute(ctx, stage, input)
	}
	e.calls++
	if e.mode == "denied_effects" {
		return nil, fmt.Errorf("effect policy denies verification command")
	}
	if e.mode == "agent_claim" {
		// Worker prose and artifact claims cannot substitute for a successful check.
		return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusFailed, Output: "All checks passed; task complete", Artifacts: map[string]string{"status": "completed"}}, fmt.Errorf("no admitted verification result")
	}
	if e.mode == "cancel_after_admission" {
		e.f.cancel()
		return nil, ctx.Err()
	}
	result, err := e.f.Execute(ctx, stage, input)
	if err != nil {
		return result, err
	}
	if e.calls == 2 {
		switch e.mode {
		case "cancel_completion":
			e.f.cancel()
		case "stop_completion":
			if err := e.f.env.mgr.Stop("A"); err != nil {
				return nil, err
			}
		case "unmet_validation":
			_, err = e.f.env.mgr.state.GetDB().Exec(`INSERT INTO validation_items(id,plan_revision_id,source,requirement,disposition,criterion) VALUES('late-obligation',?,'policy','blocking','accepted','New obligation')`, e.c.PlanID)
		}
	}
	return result, err
}

func TestCorrectionExecutionControls(t *testing.T) {
	for _, mode := range []string{"denied_effects", "agent_claim", "cancel_after_admission", "cancel_completion", "stop_completion", "unmet_validation"} {
		t.Run(mode, func(t *testing.T) {
			f, c, repair := correctionFixture(t)
			createQueueTask(t, f.env, "Independent", nil)
			if err := f.env.mgr.AuthorizeTaskCorrection(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			executor := &correctionControlExecutor{f: f, mode: mode, c: c}
			f.env.mgr.cfg.StageExecutor = executor
			if err := boundaryRun(f); err == nil {
				t.Fatal("control bypassed")
			}
			assertCorrectionReload(t, f, c, repair, false)
			a, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			got, err := release.CorrectionForTask(a)
			if err != nil || !got.Consumed || got.Outcome == "running" || got.Outcome == "completed" || got.Reason == "" {
				t.Fatalf("unpersisted control: %+v %v", got, err)
			}
			if strings.Contains(mode, "completion") || mode == "unmet_validation" {
				if executor.calls != 2 {
					t.Fatal("completion boundary unreachable", executor.calls)
				}
			} else if executor.calls != 1 {
				t.Fatal("execution boundary unreachable", executor.calls)
			}
			independent, err := f.env.rel.TaskSnapshot(context.Background(), "Independent")
			if err != nil || independent.AttemptCount != 0 {
				t.Fatal("hard control drained unrelated execution", err)
			}
			if mode == "stop_completion" {
				var status string
				if err := f.env.mgr.state.GetDB().QueryRow(`SELECT status FROM runs WHERE id='A'`).Scan(&status); err != nil || status != "stopped" {
					t.Fatal("Stop run status not persisted", status, err)
				}
			}
			calls := executor.calls
			if err := boundaryRun(f); err == nil {
				t.Fatal("incomplete correction accepted")
			}
			if executor.calls != calls {
				t.Fatal("reentry renewed consumed authority")
			}
			if f.env.mgr.AuthorizeTaskCorrection(context.Background(), c) == nil {
				t.Fatal("control renewal accepted")
			}
		})
	}
}
