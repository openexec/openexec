package manager

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/release"
)

// Runs the shipped binary against native SQLite, then the existing scoped queue.
// OPENEXEC_CORRECTION_BINARY lets the verifier run compiled source-overlay mutants.
func TestTaskCorrectCLI(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := os.Getenv("OPENEXEC_CORRECTION_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "openexec")
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/openexec")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %s %v", out, err)
		}
	}
	for _, planned := range []bool{true, false} {
		name := "no-plan"
		if planned {
			name = "planned"
		}
		t.Run(name, func(t *testing.T) {
			f, c, repair := correctionFixture(t)
			if !planned {
				if _, err := f.env.mgr.state.GetDB().Exec(`DELETE FROM validation_items; DELETE FROM validation_plan_revisions`); err != nil {
					t.Fatal(err)
				}
				c.PlanID, c.StateHash = "", ""
			}
			createQueueTask(t, f.env, "Independent", nil)
			receipt, _ := json.Marshal(recaptureReceipt("verify", 124))
			f.originalReceipt = receipt
			if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE run_steps SET metadata=? WHERE id='legacy'`, string(receipt)); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(f.env.dir, ".openexec"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(f.env.dir, "state.db"), filepath.Join(f.env.dir, ".openexec/openexec.db")); err != nil {
				t.Fatal(err)
			}
			run := func(t *testing.T, operator bool, want bool, flags ...string) {
				t.Helper()
				args := append([]string{"task", "correct", "A", "--dir", f.env.dir}, flags...)
				cmd := exec.Command(binary, args...)
				cmd.Dir = f.env.dir
				for _, v := range os.Environ() {
					if !strings.HasPrefix(v, "OPENEXEC_OPERATOR_SESSION=") {
						cmd.Env = append(cmd.Env, v)
					}
				}
				if operator {
					cmd.Env = append(cmd.Env, "OPENEXEC_OPERATOR_SESSION=1")
				}
				out, err := cmd.CombinedOutput()
				t.Logf("PUBLIC_COMMAND operator=%t %s\n%s", operator, strings.Join(args, " "), out)
				if (err == nil) != want {
					t.Fatalf("command success=%t want=%t: %v", err == nil, want, err)
				}
			}
			flags := []string{"--authorize-correction", "--decision-ref", c.DecisionRef}
			t.Run("denials", func(t *testing.T) {
				run(t, false, false, flags...)
				run(t, true, false, "--decision-ref", c.DecisionRef)
				run(t, true, false, "--authorize-correction")
				run(t, true, false, "--authorize-correction", "--decision-ref", "bad reference")
				run(t, true, false, append(flags, "--plan", "stale-plan")...)
				if planned {
					if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE graph_generations SET status='stale'`); err != nil {
						t.Fatal(err)
					}
					run(t, true, false, flags...)
					if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE graph_generations SET status='current'`); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE tasks SET git_branch='other' WHERE id='A'`); err != nil {
					t.Fatal(err)
				}
				run(t, true, false, flags...)
				if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE tasks SET git_branch=? WHERE id='A'`, c.Branch); err != nil {
					t.Fatal(err)
				}
				if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE run_steps SET metadata='{}' WHERE id='legacy'`); err != nil {
					t.Fatal(err)
				}
				run(t, true, false, flags...)
				if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE run_steps SET metadata=? WHERE id='legacy'`, string(receipt)); err != nil {
					t.Fatal(err)
				}
			})
			// With no authorization, independent work drains but A stays failed 3/3.
			var boundary *TaskQueueBoundary
			if err := boundaryRun(f); !errors.As(err, &boundary) {
				t.Fatalf("want TaskQueueBoundary: %v", err)
			}
			a, err := f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil || a.Status != release.TaskStatusFailed || a.AttemptCount != 3 || a.Metadata["task_correction"] != nil {
				t.Fatal("denied authorization changed history", a, err)
			}
			independent, err := f.env.rel.TaskSnapshot(context.Background(), "Independent")
			if err != nil || independent.Status != release.TaskStatusDone {
				t.Fatal("independent work blocked", independent, err)
			}
			run(t, true, true, flags...)
			f.restart(t)
			a, err = f.env.rel.TaskSnapshot(context.Background(), "A")
			if err != nil {
				t.Fatal(err)
			}
			got, err := release.CorrectionForTask(a)
			if err != nil {
				queueErr := boundaryRun(f)
				if !errors.As(queueErr, &boundary) {
					t.Fatalf("missing authorization bypassed boundary: %v", queueErr)
				}
				t.Fatalf("authorization missing; A failed 3/3 with TaskQueueBoundary: %v", err)
			}
			if got != c {
				t.Fatalf("exact decision binding mismatch: got %+v want %+v", got, c)
			}
			run(t, true, false, flags...)
			if err := boundaryRun(f); err != nil {
				t.Fatal(err)
			}
			assertCorrectionReload(t, f, c, repair, true)
			a, _ = f.env.rel.TaskSnapshot(context.Background(), "A")
			got, err = release.CorrectionForTask(a)
			if err != nil || got.DecisionRef != c.DecisionRef || got.EvidenceID != c.EvidenceID || got.CandidatePath != c.CandidatePath || got.PlanID != c.PlanID || got.StateHash != c.StateHash {
				t.Fatal("persisted decision binding mismatch", got, err)
			}
			run(t, true, false, flags...)
		})
	}
}
