package manager

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/db/state"
	"github.com/openexec/openexec/pkg/runtime"
)

// Every invalid retained binding is consumed before dispatch, then survives an
// actual SQLite close/reopen without blocking unrelated native work.
func TestCorrectionPreAdmissionRefusalPersistence(t *testing.T) {
	for _, mode := range []string{"candidate_edit", "candidate_commit", "candidate_output", "actual_branch", "receipt", "missing_receipt", "branch", "state_hash", "current_plan", "newer_plan", "graph", "metadata_array", "metadata_scalar", "metadata_fields", "metadata_types", "plan_metadata"} {
		t.Run(mode, func(t *testing.T) {
			f, c, _ := correctionFixture(t)
			createQueueTask(t, f.env, "Independent", nil)
			ctx := context.Background()
			if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
				t.Fatal(err)
			}
			if mode == "candidate_edit" || mode == "candidate_output" || mode == "candidate_commit" {
				name := "corrected.go"
				if mode == "candidate_output" {
					name = "generated.txt"
				}
				if err := os.WriteFile(filepath.Join(f.env.dir, name), []byte("changed candidate\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if mode == "candidate_commit" {
					for _, args := range [][]string{{"add", "."}, {"-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "independent product change"}} {
						cmd := exec.Command("git", args...)
						cmd.Dir = f.env.dir
						if out, err := cmd.CombinedOutput(); err != nil {
							t.Fatalf("commit: %s %v", out, err)
						}
					}
				}
			}
			if mode == "actual_branch" {
				cmd := exec.Command("git", "switch", "-c", "different-branch")
				cmd.Dir = f.env.dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("branch: %s %v", out, err)
				}
			}
			if mode == "newer_plan" {
				if _, err := f.env.mgr.state.CreateValidationPlanRevision(ctx, state.ValidationPlanRevision{TaskID: "A", GenerationID: "graph", WorktreeStateHash: c.StateHash, Status: "accepted"}); err != nil {
					t.Fatal(err)
				}
			}
			query := map[string]string{
				"receipt":         "UPDATE run_steps SET metadata='{}' WHERE id='legacy'",
				"missing_receipt": "DELETE FROM run_steps WHERE id='legacy'",
				"branch":          "UPDATE tasks SET git_branch='other' WHERE id='A'",
				"state_hash":      "UPDATE tasks SET metadata=json_set(metadata,'$.task_correction.state_hash','other') WHERE id='A'",
				"current_plan":    "UPDATE validation_plan_revisions SET status='proposed'",
				"graph":           "UPDATE graph_generations SET worktree_state_hash='other'",
				"newer_plan":      "SELECT 1",
				"metadata_array":  "UPDATE tasks SET metadata=json_set(metadata,'$.task_correction',json('[]')) WHERE id='A'",
				"metadata_scalar": "UPDATE tasks SET metadata=json_set(metadata,'$.task_correction','broken') WHERE id='A'",
				"metadata_fields": "UPDATE tasks SET metadata=json_remove(metadata,'$.task_correction.decision_ref') WHERE id='A'",
				"metadata_types":  "UPDATE tasks SET metadata=json_set(metadata,'$.task_correction.task_id',7) WHERE id='A'",
				"plan_metadata":   "UPDATE validation_items SET command_argv='broken'",
			}[mode]
			if query == "" {
				query = "SELECT 1"
			}
			if _, err := f.env.mgr.state.GetDB().Exec(query); err != nil {
				t.Fatal(err)
			}
			original, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
			if err != nil {
				t.Fatal(err)
			}
			f.restart(t)
			var boundary *TaskQueueBoundary
			if err := boundaryRun(f); !errors.As(err, &boundary) {
				t.Fatalf("not recorded refusal: %v", err)
			}
			a, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil || a.Status != "failed" || a.AttemptCount != 3 || a.MaxAttempts != 3 || !release.CorrectionRefused(a) || a.Metadata["verification_failure_evidence"] != "legacy" {
				t.Fatalf("refusal: %+v %v", a, err)
			}
			independent, err := f.env.rel.TaskSnapshot(ctx, "Independent")
			if err != nil || independent.Status != "done" || f.calls != 0 {
				t.Fatal("independent work blocked", independent, err, f.calls)
			}
			f.restart(t)
			if err := boundaryRun(f); !errors.As(err, &boundary) {
				t.Fatal(err)
			}
			again, err := f.env.rel.TaskSnapshot(ctx, "A")
			if err != nil || !reflect.DeepEqual(a, again) || f.calls != 0 {
				t.Fatal("refusal not stable across restart", err)
			}
			receipt, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
			if err != nil || !reflect.DeepEqual(original, receipt) {
				t.Fatal("original evidence changed", err)
			}
			if mode == "candidate_edit" || mode == "candidate_commit" || mode == "candidate_output" {
				// Refresh bindings to the changed candidate; never reset its bytes
				// or HEAD merely to make the old authorization usable again.
				if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err == nil {
					t.Fatal("stale decision replay accepted")
				}
				c.CandidatePath, c.Branch, c.CandidateDigest, err = f.env.mgr.CorrectionCandidate(ctx)
				if err != nil {
					t.Fatal(err)
				}
				manifest, err := knowledge.BuildScanManifest(c.CandidatePath)
				if err != nil {
					t.Fatal(err)
				}
				c.StateHash = manifest.WorktreeStateHash
				if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE graph_generations SET worktree_state_hash=?`, c.StateHash); err != nil {
					t.Fatal(err)
				}
				previous, err := f.env.mgr.state.GetValidationPlanRevision(ctx, c.PlanID)
				if err != nil {
					t.Fatal(err)
				}
				for i := range previous.Items {
					previous.Items[i].ID = ""
				}
				next, err := f.env.mgr.state.CreateValidationPlanRevision(ctx, state.ValidationPlanRevision{TaskID: "A", GenerationID: "graph", WorktreeStateHash: c.StateHash, Status: "accepted", Items: previous.Items})
				if err != nil {
					t.Fatal(err)
				}
				c.PlanID, c.DecisionRef = next.ID, "fixture-owner:changed-candidate"
				if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
					t.Fatal("fresh valid candidate refused", err)
				}
				f.restart(t)
				task, err := f.env.rel.TaskSnapshot(ctx, "A")
				if err != nil {
					t.Fatal(err)
				}
				got, err := release.CorrectionForTask(task)
				if err != nil || got != c || task.AttemptCount != 3 || f.calls != 0 {
					t.Fatal("fresh decision not persisted", got, err)
				}
				if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err == nil {
					t.Fatal("fresh decision replay accepted")
				}
			}
		})
	}
}

func TestCorrectionRefusalStoreFailureIsOperational(t *testing.T) {
	f, c, _ := correctionFixture(t)
	ctx := context.Background()
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.env.dir, "corrected.go"), []byte("package changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	db := f.env.mgr.state.GetDB()
	if _, err := db.Exec("CREATE TRIGGER refuse_disposition BEFORE UPDATE ON tasks BEGIN SELECT RAISE(ABORT,'store unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	var boundary *TaskQueueBoundary
	if err := boundaryRun(f); err == nil || errors.As(err, &boundary) {
		t.Fatalf("store failure hidden as boundary: %v", err)
	}
	a, err := f.env.rel.TaskSnapshot(ctx, "A")
	got, e := release.CorrectionForTask(a)
	if err != nil || e != nil || got.Consumed || f.calls != 0 {
		t.Fatal("failed persistence consumed allowance", err, e)
	}
	if _, err := db.Exec("DROP TRIGGER refuse_disposition"); err != nil {
		t.Fatal(err)
	}
	if err := boundaryRun(f); !errors.As(err, &boundary) {
		t.Fatal("retry did not persist refusal", err)
	}
}

// Independent work can alter the candidate while a correction waits for a native
// review prerequisite. On the next selection pass the stale decision is refused.
type correctionIndependentWriter struct {
	fixture *recaptureFixture
	changed bool
}

func (w *correctionIndependentWriter) Execute(ctx context.Context, stage *runtime.Stage, input *runtime.StageInput) (*runtime.StageResult, error) {
	if !w.changed {
		w.changed = true
		if err := os.WriteFile(filepath.Join(w.fixture.env.dir, "corrected.go"), []byte("package independent\n"), 0644); err != nil {
			return nil, err
		}
	}
	return w.fixture.Execute(ctx, stage, input)
}
func TestCorrectionIndependentCandidateChange(t *testing.T) {
	f, c, _ := correctionFixture(t)
	createQueueTask(t, f.env, "Independent", nil)
	ctx := context.Background()
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := f.env.mgr.state.GetDB().Exec("UPDATE tasks SET needs_review=1 WHERE id='A'"); err != nil {
		t.Fatal(err)
	}
	f.env.mgr.cfg.StageExecutor = &correctionIndependentWriter{fixture: f}
	var boundary *TaskQueueBoundary
	if err := boundaryRun(f); !errors.As(err, &boundary) {
		t.Fatal(err)
	}
	a, err := f.env.rel.TaskSnapshot(ctx, "A")
	if err != nil || !release.CorrectionRefused(a) || f.calls != 0 {
		t.Fatal("independent candidate drift not refused", a, err)
	}
	independent, err := f.env.rel.TaskSnapshot(ctx, "Independent")
	if err != nil || independent.Status != release.TaskStatusDone {
		t.Fatal("independent task not complete", err)
	}
	receipt, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	if err := boundaryRun(f); !errors.As(err, &boundary) {
		t.Fatal("restart lost boundary", err)
	}
	after, err := f.env.rel.TaskSnapshot(ctx, "A")
	if err != nil || !reflect.DeepEqual(a, after) || after.AttemptCount != 3 || f.calls != 0 {
		t.Fatal("restart changed refusal", err)
	}
	retained, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
	if err != nil || !reflect.DeepEqual(receipt, retained) {
		t.Fatal("restart changed receipt", err)
	}
	dependent, err := f.env.rel.TaskSnapshot(ctx, "Settings")
	if err != nil || dependent.Status != release.TaskStatusPending || dependent.AttemptCount != 0 {
		t.Fatal("dependent escaped", err)
	}
}
