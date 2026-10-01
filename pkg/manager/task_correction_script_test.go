package manager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/knowledge"
	"github.com/openexec/openexec/internal/release"
)

// Each journey executes the actual shell command through the native deterministic
// executor, then closes and reopens SQLite and resumes the dependent queue.
func TestCorrectionTaskScriptJourneys(t *testing.T) {
	for _, origin := range []string{"planned", "planner_imported", "legacy_no_plan"} {
		for _, mode := range []string{"pass", "missing_file", "exit_3", "plan_failure", "quoted", "unsupported", "effect_denial", "no_authority"} {
			t.Run(origin+"/"+mode, func(t *testing.T) {
				f, c, repair := correctionFixtureOrigin(t, origin == "planner_imported", "true")
				db := f.env.mgr.state.GetDB()
				if origin != "planned" {
					if _, err := db.Exec(`DELETE FROM validation_items; DELETE FROM validation_plan_revisions`); err != nil {
						t.Fatal(err)
					}
					c.PlanID, c.StateHash = "", ""
				}
				script := "test -f corrected.go"
				wantDone := mode == "pass" || mode == "quoted"
				switch mode {
				case "missing_file":
					if err := os.Remove(filepath.Join(f.env.dir, "corrected.go")); err != nil {
						t.Fatal(err)
					}
					// Bind the missing-file candidate before authorization, so the command
					// must fail rather than merely hitting the candidate-drift refusal.
					var err error
					c.CandidatePath, c.Branch, c.CandidateDigest, err = f.env.mgr.CorrectionCandidate(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if c.PlanID != "" {
						manifest, err := knowledge.BuildScanManifest(f.env.dir)
						if err != nil {
							t.Fatal(err)
						}
						c.StateHash = manifest.WorktreeStateHash
						if _, err := db.Exec(`UPDATE graph_generations SET worktree_state_hash=?; UPDATE validation_plan_revisions SET worktree_state_hash=?`, c.StateHash, c.StateHash); err != nil {
							t.Fatal(err)
						}
					}
				case "exit_3":
					script = "exit 3"
				case "quoted":
					script = `test 'a b' = "a b" && test '$HOME' = '$HOME'`
				case "plan_failure":
					if c.PlanID == "" {
						t.Skip("accepted-plan obligation requires a planned task")
					}
					if _, err := db.Exec(`UPDATE validation_items SET command_argv='["/bin/sh","-c","exit 3"]' WHERE id='check'`); err != nil {
						t.Fatal(err)
					}
				case "unsupported":
					if c.PlanID == "" {
						t.Skip("unsupported plan command requires a plan")
					}
					if _, err := db.Exec(`UPDATE validation_items SET command_argv='["sh","-c","true","extra-argv"]' WHERE id='check'`); err != nil {
						t.Fatal(err)
					}
				case "effect_denial":
					f.mode = "refusal"
				}
				// Keep both plan checks passing even when the task script fails.
				if mode != "plan_failure" && mode != "unsupported" {
					if _, err := db.Exec(`UPDATE validation_items SET command_argv='["sh","-c","true"]'`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.Exec(`UPDATE tasks SET verification_script=? WHERE id='A'`, script); err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if mode != "no_authority" {
					err := f.env.mgr.AuthorizeTaskCorrection(ctx, c)
					if mode == "unsupported" {
						if err == nil || !strings.Contains(err.Error(), "no supported deterministic check") {
							t.Fatalf("unsupported required command not refused: %v", err)
						}
						if f.calls != 0 {
							t.Fatal("unsupported command executed")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				f.restart(t)
				err := boundaryRun(f)
				if wantDone && err != nil {
					t.Fatalf("script journey failed: %v", err)
				}
				if !wantDone && err == nil {
					t.Fatal("required task/plan failure completed")
				}
				if mode == "no_authority" {
					f.restart(t)
					task, err := f.env.rel.TaskSnapshot(ctx, "A")
					if err != nil || task.Status != release.TaskStatusFailed || task.AttemptCount != 3 || f.calls != 0 {
						t.Fatalf("missing authority dispatched: %+v %v", task, err)
					}
					return
				}
				assertCorrectionReload(t, f, c, repair, wantDone)
				var proofs int
				if err := f.env.mgr.state.GetDB().QueryRow(`SELECT count(*) FROM run_steps WHERE run_id='A' AND phase='verify' AND json_extract(metadata,'$.correction_decision')=? AND json_extract(metadata,'$.task_verification_script')=?`, c.DecisionRef, script).Scan(&proofs); err != nil || proofs != 1 {
					t.Fatalf("task script proof count=%d: %v", proofs, err)
				}
				if mode != "effect_denial" {
					invocations := 0
					for _, command := range f.commands {
						if command == script {
							invocations++
						}
					}
					if invocations != 1 {
						t.Fatalf("task script invocations=%d, want exactly one", invocations)
					}
				}
				if origin != "planned" && f.calls != 1 {
					t.Fatalf("task verify invocations=%d, want exactly one", f.calls)
				}
				task, err := f.env.rel.TaskSnapshot(ctx, "A")
				if err != nil {
					t.Fatal(err)
				}
				decision, err := release.CorrectionForTask(task)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "missing_file" || mode == "exit_3" || mode == "plan_failure" {
					if decision.Outcome != "continuing_failure" || decision.FreshEvidenceID == "" {
						t.Fatalf("failure disposition: %+v", decision)
					}
				}
				if mode == "effect_denial" && decision.Outcome != "refused" {
					t.Fatalf("effect denial: %+v", decision)
				}
				calls := f.calls
				_ = boundaryRun(f)
				if f.calls != calls {
					t.Fatal("restart repeated verification")
				}
			})
		}
	}
}

func TestCorrectionNoPlanCannotHideAcceptedPlan(t *testing.T) {
	f, c, _ := correctionFixture(t)
	c.PlanID, c.StateHash = "", ""
	if err := f.env.mgr.AuthorizeTaskCorrection(context.Background(), c); err == nil {
		t.Fatal("omitted applicable plan accepted")
	}
}

func TestCorrectionNoPlanCompletionRequiresScriptProof(t *testing.T) {
	f, c, _ := correctionFixture(t)
	db := f.env.mgr.state.GetDB()
	if _, err := db.Exec(`DELETE FROM validation_items; DELETE FROM validation_plan_revisions`); err != nil {
		t.Fatal(err)
	}
	c.PlanID, c.StateHash = "", ""
	ctx := context.Background()
	if err := f.env.mgr.AuthorizeTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	store, err := release.NewSQLiteStore(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AdmitTaskCorrection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := store.FinishTaskCorrection(ctx, c, true); err == nil || !strings.Contains(err.Error(), "task verification script") {
		t.Fatalf("missing task script proof accepted: %v", err)
	}
	f.restart(t)
	task, err := f.env.rel.TaskSnapshot(ctx, "A")
	if err != nil || task.Status == release.TaskStatusDone || task.AttemptCount != 3 {
		t.Fatalf("unverified completion persisted: %+v %v", task, err)
	}
}

func TestCorrectionWithoutChecksUsesNativeCompletion(t *testing.T) {
	for _, plan := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_plan", true: "optional_plan"}[plan], func(t *testing.T) {
			f, c, repair := correctionFixture(t)
			db := f.env.mgr.state.GetDB()
			if _, err := db.Exec(`UPDATE tasks SET verification_script='' WHERE id='A'; UPDATE validation_items SET requirement='optional'`); err != nil {
				t.Fatal(err)
			}
			if !plan {
				if _, err := db.Exec(`DELETE FROM validation_items; DELETE FROM validation_plan_revisions`); err != nil {
					t.Fatal(err)
				}
				c.PlanID, c.StateHash = "", ""
			}
			if err := f.env.mgr.AuthorizeTaskCorrection(context.Background(), c); err != nil {
				t.Fatal(err)
			}
			if err := boundaryRun(f); err != nil {
				t.Fatal(err)
			}
			assertCorrectionReload(t, f, c, repair, true)
			if f.calls != 0 {
				t.Fatal("invented a verification obligation")
			}
		})
	}
}
