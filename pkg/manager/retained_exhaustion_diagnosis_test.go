package manager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

const diagnosisDocumentCheck = `set -euo pipefail
test -f docs/ARCHITECTURE.md
grep -q '## Module Map' docs/ARCHITECTURE.md
test $(wc -l < docs/ARCHITECTURE.md) -gt 20`

func diagnosisDocument(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0700); err != nil {
		t.Fatal(err)
	}
	data := "# Synthetic architecture\n\n## Module Map\n" + strings.Repeat("Generic module: reads a generic document.\n", 22)
	if err := os.WriteFile(filepath.Join(dir, "docs", "ARCHITECTURE.md"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", diagnosisDocumentCheck)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	t.Logf("artifact argv=%q output=%q result=%v", cmd.Args, out, err)
	if err != nil {
		t.Fatal(err)
	}
}

// Only disposable native stores are constructed. Failures come from actual
// processes; the later passing document check does not rewrite those receipts.
func diagnosisChain(t *testing.T, fixes int) (*schedulerTestEnv, string) {
	t.Helper()
	e := newSchedulerTestEnv(t)
	createStory(t, e.rel, "S", nil)
	createQueueTask(t, e, "A", nil)
	createQueueTask(t, e, "dependent", []string{"A"})
	createQueueTask(t, e, "independent", nil)
	id := "A"
	for i := 0; i <= fixes; i++ {
		task, err := e.rel.TaskSnapshot(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		task.VerificationScript = diagnosisDocumentCheck
		task.Status, task.AttemptCount = release.TaskStatusInProgress, 2
		if id == "A" {
			task.AttemptCount = 1
		}
		if err := e.rel.UpdateTask(task); err != nil {
			t.Fatal(err)
		}
		if err := e.mgr.state.CreateRun(context.Background(), id, "", "", e.dir, "workspace-write"); err != nil {
			t.Fatal(err)
		}
		const command = "printf 'synthetic configured gate failed\\n'; exit 2"
		cmd := exec.Command("sh", "-c", command)
		cmd.Dir = e.dir
		out, failed := cmd.CombinedOutput()
		if failed == nil {
			t.Fatal("synthetic failure did not fail")
		}
		t.Logf("historical gate argv=%q output=%q result=%v", cmd.Args, out, failed)
		artifacts := gates.VerificationFailureArtifacts(gates.NewCommandFailureWithOutput(context.Background(), "test", failed, command, string(out)))
		data, err := json.Marshal(artifacts)
		if err != nil || len(artifacts) == 0 {
			t.Fatal("no typed failure", err)
		}
		evidenceID := fmt.Sprintf("synthetic-check-%d", i)
		if err := e.mgr.state.RecordTaskFailureStep(context.Background(), state.RunStepData{ID: evidenceID, RunID: id, Phase: "test", Agent: "deterministic-verification", Status: "failed", Metadata: string(data)}, task.AttemptCount); err != nil {
			t.Fatal(err)
		}
		if err := e.mgr.state.UpdateRunStatus(context.Background(), id, "error", failed.Error()); err != nil {
			t.Fatal(err)
		}
		if i < fixes {
			if err := e.mgr.repairTaskFromRetainedFailure(context.Background(), id, evidenceID); err != nil {
				t.Fatal(err)
			}
			closed, err := e.rel.TaskSnapshot(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			id = closed.Metadata["fixed_by"].(string)
		}
	}
	diagnosisDocument(t, e.dir)
	return e, id
}

func diagnosisLedger(t *testing.T, e *schedulerTestEnv) []*release.Task {
	t.Helper()
	tasks, err := e.rel.TasksInStories(context.Background(), []string{"S"})
	if err != nil {
		t.Fatal(err)
	}
	return tasks
}

func diagnosisHistory(t *testing.T, e *schedulerTestEnv) string {
	t.Helper()
	history := make(map[string]any)
	for _, task := range diagnosisLedger(t, e) {
		id, _ := task.Metadata["verification_failure_evidence"].(string)
		if id == "" {
			continue
		}
		run, err := e.mgr.state.GetRun(context.Background(), task.ID)
		if err != nil || run == nil {
			t.Fatal("historical run missing", err)
		}
		step, err := e.mgr.state.GetRunStep(context.Background(), id)
		if err != nil || step == nil {
			t.Fatal("historical failure receipt missing", err)
		}
		history[id] = []any{run, step}
	}
	data, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestRetainedExhaustionDiagnosisQueue(t *testing.T) {
	for _, stop := range []string{"none", "cancelled", "yield"} {
		t.Run(stop, func(t *testing.T) {
			e, last := diagnosisChain(t, 3)
			before := diagnosisLedger(t, e)
			history := diagnosisHistory(t, e)
			ready, err := e.rel.RunnableTasks(context.Background(), []string{"S"})
			if err != nil || len(ready) != 1 || ready[0].ID != "independent" {
				t.Fatal("final fix/dependent selected or independent work lost", ready, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := RunOptions{TaskOriented: true, StoryIDs: []string{"S"}}
			if stop == "cancelled" {
				cancel()
			}
			if stop == "yield" {
				opts.Yield = func() bool { return true }
			}
			err = e.mgr.ExecuteTasks(ctx, opts)
			t.Logf("ExecuteTasks stop=%s result=%v", stop, err)
			switch stop {
			case "none":
				if err == nil || err.Error() != "task repair attempt limit reached" {
					t.Fatal("exact retained boundary not reproduced", err)
				}
			case "cancelled":
				if !errors.Is(err, context.Canceled) {
					t.Fatal("Stop/cancellation lost precedence", err)
				}
			case "yield":
				if !errors.Is(err, ErrQueueYielded) {
					t.Fatal("caller boundary ignored", err)
				}
			}
			for _, task := range before {
				if _, err := e.mgr.Status(task.ID); err == nil {
					t.Fatal("retained task dispatched", task.ID)
				}
			}
			// Close and reopen both native managers and the database, not just
			// their caches. The shared helper preserves the same disposable path.
			(&recaptureFixture{env: e}).restart(t)
			if after := diagnosisLedger(t, e); !reflect.DeepEqual(before, after) {
				t.Fatal("boundary changed task history/accounting", before, after)
			}
			if diagnosisHistory(t, e) != history {
				t.Fatal("boundary rewrote historical runs/receipts")
			}
			final, err := e.rel.TaskSnapshot(context.Background(), last)
			if err != nil || final.AttemptCount != 2 || final.MaxAttempts != 2 || final.Status != release.TaskStatusFailed {
				t.Fatal("spent final fix changed", final, err)
			}
			var plans int
			if err := e.mgr.state.GetDB().QueryRow(`SELECT COUNT(*) FROM validation_plan_revisions`).Scan(&plans); err != nil || plans != 0 {
				t.Fatal("fixture no longer represents legacy validation", plans, err)
			}
		})
	}
}

func TestRetainedExhaustionDiagnosisNearMiss(t *testing.T) {
	e, last := diagnosisChain(t, 2)
	// An exhausted individual fix is not the chain limit: with only two
	// repairs against root max=3, native repair creation still permits one.
	task, err := e.rel.TaskSnapshot(context.Background(), last)
	if err != nil || task.AttemptCount != task.MaxAttempts {
		t.Fatal(task, err)
	}
	if err := e.mgr.repairTaskFromRetainedFailure(context.Background(), last, "synthetic-check-2"); err != nil {
		t.Fatal("individual exhaustion confused with root chain exhaustion", err)
	}
	ready, err := e.rel.RunnableTasks(context.Background(), []string{"S"})
	if err != nil || len(ready) != 2 || ready[0].Metadata["repair_of"] != "A" || ready[0].AttemptCount != 0 {
		t.Fatal("remaining root budget did not produce priority repair", ready, err)
	}
	if err := e.mgr.repairTaskFromRetainedFailure(context.Background(), ready[0].ID, "missing-receipt"); err == nil {
		t.Fatal("missing proof authorized repair")
	}
}

func TestRetainedExhaustionDiagnosisRecaptureIsNotReconciliation(t *testing.T) {
	for _, mode := range []string{"spent", "refused", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureFixture(t, "printf 'synthetic recheck\\n'")
			f.mode = mode
			if mode == "refused" {
				f.mode = "refusal"
			}
			if mode == "spent" {
				task, err := f.env.rel.TaskSnapshot(context.Background(), "A")
				if err != nil {
					t.Fatal(err)
				}
				task.AttemptCount = task.MaxAttempts
				if err := f.env.rel.UpdateTask(task); err != nil {
					t.Fatal(err)
				}
			}
			err := boundaryRun(f)
			t.Logf("diagnostic-free recapture mode=%s result=%v", mode, err)
			if err == nil {
				t.Fatal("recapture bypassed boundary")
			}
			f.restart(t)
			outcome, attempts, calls := "refused", 2, 1
			if mode == "spent" {
				outcome, attempts, calls = "exhausted", 3, 0
			}
			if mode == "cancel" {
				outcome = "cancelled"
			}
			f.assertTerminal(t, outcome, attempts)
			if f.calls != calls || len(diagnosisLedger(t, f.env)) != 2 {
				t.Fatal("recapture changed budget/created replacement work", f.calls)
			}
		})
	}
}

// Exercise all release-level completion routes. CLI task complete calls
// CompleteTask; MCP backlog_complete_task calls SetTaskStatus; normal queue
// completion and UpdateTask share the accepted-plan guard.
func diagnosisComplete(m *release.Manager, route, id string) error {
	switch route {
	case "CompleteTask":
		_, err := m.CompleteTask(id)
		return err
	case "SetTaskStatus":
		return m.SetTaskStatus(id, release.TaskStatusDone)
	default:
		task, err := m.TaskSnapshot(context.Background(), id)
		if err != nil {
			return err
		}
		task.Status = release.TaskStatusDone
		return m.UpdateTask(task)
	}
}

func TestRetainedExhaustionDiagnosisLegacyCompletionLimitation(t *testing.T) {
	for _, route := range []string{"CompleteTask", "SetTaskStatus", "UpdateTask"} {
		t.Run(route, func(t *testing.T) {
			e, last := diagnosisChain(t, 3)
			task, err := e.rel.TaskSnapshot(context.Background(), last)
			if err != nil {
				t.Fatal(err)
			}
			// A failing task script and an unfulfilled human/review boundary
			// make this an unsafe completion candidate. These APIs accept it:
			// this test characterizes a gap, never recommends using the bypass.
			task.VerificationScript, task.NeedsReview = "exit 9", true
			task.Metadata["mode"] = release.TaskModeHITL
			if err := e.rel.UpdateTask(task); err != nil {
				t.Fatal(err)
			}
			if err := e.rel.Load(); err != nil {
				t.Fatal(err)
			}
			before := diagnosisLedger(t, e)
			history := diagnosisHistory(t, e)
			cmd := exec.Command("bash", "-c", task.VerificationScript)
			cmd.Dir = e.dir
			out, failed := cmd.CombinedOutput()
			if failed == nil {
				t.Fatal("bad proof passed")
			}
			t.Logf("bad proof argv=%q output=%q result=%v", cmd.Args, out, failed)
			if err := diagnosisComplete(e.rel, route, last); err != nil {
				t.Fatal("legacy completion behavior changed; reassess diagnosis", err)
			}
			(&recaptureFixture{env: e}).restart(t)
			if diagnosisHistory(t, e) != history {
				t.Fatal("legacy completion rewrote historical runs/receipts")
			}
			after := diagnosisLedger(t, e)
			for i, old := range before {
				current := after[i]
				if old.ID == last {
					if current.Status != release.TaskStatusDone {
						t.Fatal("completion was not persisted")
					}
					// Only status and completion timestamp may differ. Counters,
					// failures, links and unfulfilled review authority survive.
					current.Status, current.CompletedAt = old.Status, old.CompletedAt
				}
				if !reflect.DeepEqual(old, current) {
					t.Fatal("completion rewrote history/accounting", old, current)
				}
			}
			t.Logf("%s accepted legacy completion despite failed proof and hitl/needs_review; unsafe as reconciliation", route)
		})
	}
}

// Seed only repository identity; use supported native APIs for plans, explicit
// acceptance, real check results, evidence links and final completion claims.
func diagnosisValidation(t *testing.T, e *schedulerTestEnv, last, proof string) string {
	t.Helper()
	ctx := context.Background()
	s := e.mgr.state
	if _, err := s.GetDB().Exec(`
INSERT INTO repositories (id,persisted_uuid) VALUES ('repo','synthetic');
INSERT INTO checkouts (id,repository_id,root_path) VALUES ('checkout','repo','/synthetic');
INSERT INTO worktrees (id,repository_id,checkout_id,root_path) VALUES ('worktree','repo','checkout','/synthetic');
INSERT INTO graph_generations (id,schema_version,repository_id,checkout_id,worktree_id,worktree_state_hash,configuration_digest,extractor_version,manifest_hash,status) VALUES ('graph',1,'repo','checkout','worktree','state','config','extractor','manifest','current');
`); err != nil {
		t.Fatal(err)
	}
	proposal, err := s.CreateValidationPlanRevision(ctx, state.ValidationPlanRevision{
		TaskID: last, GenerationID: "graph", WorktreeStateHash: "state", Status: "proposed",
		Items: []state.ValidationItem{
			{Source: "policy", Disposition: "suggested", Requirement: "optional", Criterion: "Synthetic document check passes", CommandArgv: []string{"bash", "-c", diagnosisDocumentCheck}},
			{Source: "policy", Disposition: "suggested", Requirement: "optional", Criterion: "Synthetic required gate passes", CommandArgv: []string{"bash", "-c", "test -s docs/ARCHITECTURE.md"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := s.AcceptValidationPlanRevision(ctx, proposal.ID, "", []state.ValidationItemDecision{
		{ID: proposal.Items[0].ID, Disposition: "accepted", Requirement: "required"},
		{ID: proposal.Items[1].ID, Disposition: "accepted", Requirement: "blocking"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinalizeEvidenceCoverage(ctx, plan.ID); !errors.Is(err, state.ErrCompletionEvidenceEmpty) {
		t.Fatal("completion report accepted absent proof", err)
	}
	if err := s.CreateRun(ctx, "synthetic-recheck", "", "", e.dir, "read-only"); err != nil {
		t.Fatal(err)
	}
	for i, item := range plan.Items {
		if proof == "missing" && item.Requirement == "blocking" {
			continue
		}
		argv := item.CommandArgv
		stepStatus, linkStatus := "completed", "passed"
		if proof == "unsupported" {
			argv = []string{"bash", "-c", "exit 9"}
			stepStatus, linkStatus = "failed", "failed"
		}
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = e.dir
		out, checkErr := cmd.CombinedOutput()
		t.Logf("validation argv=%q output=%q result=%v", argv, out, checkErr)
		if (checkErr == nil) != (linkStatus == "passed") {
			t.Fatal("unexpected actual validation result", checkErr)
		}
		stepID := fmt.Sprintf("synthetic-recheck-%d", i)
		if err := s.AddRunStep(ctx, stepID, "synthetic-recheck", "", "verify", i+1, stepStatus); err != nil {
			t.Fatal(err)
		}
		link := state.ValidationEvidenceLink{ValidationItemID: item.ID, RunID: "synthetic-recheck", RunStepID: stepID, WorktreeStateHash: "state", Status: linkStatus}
		if proof == "wrong_state" {
			link.WorktreeStateHash = "old-state"
		}
		err := s.LinkValidationEvidence(ctx, link)
		if proof == "wrong_state" {
			if !errors.Is(err, state.ErrEvidenceStateMismatch) {
				t.Fatal("wrong-state evidence was not refused", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if proof == "stale" {
		// Simulate graph invalidation, without modifying task state/accounting.
		if _, err := s.GetDB().Exec(`UPDATE graph_generations SET status='stale' WHERE id='graph'`); err != nil {
			t.Fatal(err)
		}
	}
	report, err := s.FinalizeEvidenceCoverage(ctx, plan.ID)
	switch proof {
	case "stale":
		if !errors.Is(err, state.ErrCompletionStateMoved) {
			t.Fatal("stale evidence was finalized", err)
		}
	case "wrong_state":
		if !errors.Is(err, state.ErrCompletionEvidenceEmpty) {
			t.Fatal("rejected evidence produced completion", err)
		}
	default:
		if err != nil || report.CanComplete != (proof == "supported") {
			t.Fatal("completion coverage misreported proof", report, err)
		}
	}
	return plan.ID
}

func TestRetainedExhaustionDiagnosisAcceptedCompletion(t *testing.T) {
	for _, route := range []string{"CompleteTask", "SetTaskStatus", "UpdateTask"} {
		for _, proof := range []string{"missing", "wrong_state", "unsupported", "stale", "supported"} {
			t.Run(route+"/"+proof, func(t *testing.T) {
				e, last := diagnosisChain(t, 3)
				planID := diagnosisValidation(t, e, last, proof)
				if err := e.rel.Load(); err != nil {
					t.Fatal(err)
				}
				before := diagnosisLedger(t, e)
				history := diagnosisHistory(t, e)
				err := diagnosisComplete(e.rel, route, last)
				t.Logf("%s proof=%s result=%v", route, proof, err)
				if proof == "supported" {
					if err != nil {
						t.Fatal(err)
					}
				} else if err == nil || (!strings.Contains(err.Error(), "without supported evidence") && !strings.Contains(err.Error(), "evidence is stale")) {
					t.Fatal("missing/stale proof was not refused", err)
				}
				(&recaptureFixture{env: e}).restart(t)
				if diagnosisHistory(t, e) != history {
					t.Fatal("completion rewrote historical runs/receipts")
				}
				if proof == "supported" {
					report, err := e.mgr.state.ReadCompletionReport(context.Background(), planID)
					if err != nil || !report.CanComplete || len(report.Verified) != 2 {
						t.Fatal("supported completion report did not round trip", report, err)
					}
				}
				after := diagnosisLedger(t, e)
				if proof != "supported" {
					if !reflect.DeepEqual(before, after) {
						t.Fatal("refusal modified accounting/status")
					}
					return
				}
				for i, old := range before {
					current := after[i]
					if old.ID == last {
						if current.Status != release.TaskStatusDone {
							t.Fatal("supported completion not persisted")
						}
						current.Status, current.CompletedAt = old.Status, old.CompletedAt
					}
					if !reflect.DeepEqual(old, current) {
						t.Fatal("supported completion rewrote history/accounting")
					}
				}
				ready, err := e.rel.RunnableTasks(context.Background(), []string{"S"})
				if err != nil || len(ready) != 2 {
					t.Fatal("passed chain did not release existing dependent", ready, err)
				}
			})
		}
	}
}

// Substitute only the unrelated implementation/action stages. The subject of
// this test is the real config loader -> blueprint -> deterministic command ->
// retained receipt -> native repair boundary path, without a model or network.
type diagnosisGateExecutor struct {
	dir      string
	commands []string
}

func (e *diagnosisGateExecutor) Execute(ctx context.Context, stage *blueprint.Stage, input *blueprint.StageInput) (*blueprint.StageResult, error) {
	if stage.Name != "lint" && stage.Name != "test" {
		return &blueprint.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
	}
	e.commands = append(e.commands, stage.Commands...)
	executor := blueprint.NewDefaultExecutor(e.dir)
	executor.VerificationStages = map[*blueprint.Stage]bool{stage: true}
	var failure error
	executor.OnVerificationFailure = func(_ *blueprint.Stage, err error) { failure = err }
	result, err := executor.Execute(ctx, stage, input)
	if failure != nil {
		return result, failure
	}
	return result, err
}

func TestRetainedExhaustionDiagnosisConfiguredGateAuthority(t *testing.T) {
	for _, configuredFailure := range []bool{true, false} {
		t.Run(fmt.Sprintf("configured_failure=%t", configuredFailure), func(t *testing.T) {
			e := newSchedulerTestEnv(t)
			createStory(t, e.rel, "S", nil)
			createQueueTask(t, e, "A", nil)
			diagnosisDocument(t, e.dir)
			task, err := e.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			task.MaxAttempts, task.VerificationScript = 1, diagnosisDocumentCheck
			command := "printf 'synthetic configured gate failed\\n'; exit 2"
			if !configuredFailure {
				command = "printf 'synthetic configured gate passed\\n'"
				// Near miss: a failing task script does not replace the configured
				// blueprint test command in this entry point either.
				task.VerificationScript = "exit 9"
			}
			if err := e.rel.UpdateTask(task); err != nil {
				t.Fatal(err)
			}
			config, err := json.Marshal(map[string]any{"execution": map[string]any{"lint_commands": []string{"true"}, "test_commands": []string{command}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(e.dir, ".openexec", "config.json"), config, 0600); err != nil {
				t.Fatal(err)
			}
			executor := &diagnosisGateExecutor{dir: e.dir}
			e.mgr.cfg.StageExecutor = executor
			err = e.mgr.ExecuteTasks(context.Background(), RunOptions{TaskOriented: true, StoryIDs: []string{"S"}})
			t.Logf("configured commands=%q queue result=%v", executor.commands, err)
			if configuredFailure {
				if err == nil || !strings.Contains(err.Error(), "task repair attempt limit reached") {
					t.Fatal("passing task artifact bypassed configured gate", err)
				}
				(&recaptureFixture{env: e}).restart(t)
				for _, retained := range diagnosisLedger(t, e) {
					id, _ := retained.Metadata["verification_failure_evidence"].(string)
					step, err := e.mgr.state.GetRunStep(context.Background(), id)
					if err != nil || step == nil || step.Phase != "test" || step.Status != "failed" || !strings.Contains(step.Metadata, "synthetic configured gate failed") {
						t.Fatal("configured gate failure was not retained", step, err)
					}
				}
			} else if err != nil {
				t.Fatal("task script incorrectly replaced configured check", err)
			}
			want := []string{"true", command}
			if configuredFailure {
				want = append(want, want...)
			}
			if !reflect.DeepEqual(executor.commands, want) {
				t.Fatal("did not execute exactly the configured checks", executor.commands)
			}
		})
	}
}
