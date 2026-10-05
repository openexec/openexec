package manager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/release"
	"github.com/openexec/openexec/pkg/runtime"
)

// Resolve the admitted current check by name; explicit historical commands
// remain authoritative. Ordinary work is supported only in the success journey.
type variantRecaptureExecutor struct {
	fixture             *recaptureFixture
	gate, mode, command string
}

func (v *variantRecaptureExecutor) Execute(ctx context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	if stage.Name != v.gate {
		if v.mode == "success" {
			return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted}, nil
		}
		return nil, fmt.Errorf("fixture stops at ordinary task execution")
	}
	v.fixture.calls++
	if v.mode == "launch_refusal" {
		return nil, fmt.Errorf("admission refused")
	}
	if v.mode == "cancellation" {
		v.fixture.cancel()
		return nil, ctx.Err()
	}
	command := namedRecaptureCommand
	if v.mode == "success" {
		command = "printf 'NAMED_SUCCESS\\n'"
	}
	if len(stage.Commands) != 0 {
		command = stage.Commands[0]
	}
	v.command = command
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = v.fixture.env.dir
	var stdout, stderr runtime.EvidenceBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if cmd.ProcessState == nil {
		return nil, err
	}
	if strings.HasPrefix(v.mode, "budget_") {
		// Simulate a legacy adapter which still returns classification only.
		return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusFailed}, runtime.VerificationCommandFailure(ctx, stage.Name, err)
	}
	hash, path, captureErr := runtime.RetainCommandEvidence(v.fixture.env.dir, runtime.CommandEvidence{
		Argv: []string{"sh", "-c", command}, Cwd: v.fixture.env.dir, ExitCode: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String(),
	})
	if captureErr != nil {
		return nil, captureErr
	}
	if err == nil {
		return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusCompleted, Artifacts: map[string]string{hash: path}}, nil
	}
	return &runtime.StageResult{StageName: stage.Name, Status: runtime.StageStatusFailed, Output: runtime.PublicVerificationStream(&stderr, nil)}, runtime.VerificationCommandFailureWithEvidence(ctx, stage.Name, err, hash, path)
}

func TestNamedRecaptureVariants(t *testing.T) {
	for _, gate := range []string{"lint", "test"} {
		for _, phaseLabel := range []string{"empty", "matching"} {
			for _, mode := range []string{"success", "existing_evidence", "historical_command", "budget_fresh", "budget_remaining", "budget_spent", "ambiguous", "mismatch", "hitl", "review", "cancellation", "launch_refusal"} {
				t.Run(gate+"/"+phaseLabel+"/"+mode, func(t *testing.T) {
					f := newRecaptureFixture(t, "")
					ctx := context.Background()
					refs := recaptureReceipt(gate, 2)
					phase := ""
					if phaseLabel == "matching" {
						phase = gate
					}
					task, err := f.env.rel.TaskSnapshot(ctx, "A")
					if err != nil {
						t.Fatal(err)
					}
					wantCalls, wantAttempts, wantRepairs, outcome := 0, 1, 0, "refused"
					switch mode {
					case "success":
						// Recapture, resumed A, then Settings each execute the check once.
						wantCalls, wantAttempts, outcome = 3, 3, "success"
					case "existing_evidence", "historical_command":
						exit := 2
						if mode == "historical_command" {
							exit = 0
							wantCalls, wantAttempts, outcome = 1, 2, "failed"
						} else {
							outcome = ""
						}
						hash, path, err := runtime.RetainCommandEvidence(f.env.dir, runtime.CommandEvidence{Argv: []string{"sh", "-c", "printf 'HISTORICAL_COMMAND\\n' >&2; exit 2"}, Cwd: f.env.dir, ExitCode: exit})
						if err != nil {
							t.Fatal(err)
						}
						if err := f.env.mgr.state.RecordArtifact(ctx, hash, "test_log", path, 0); err != nil {
							t.Fatal(err)
						}
						refs[hash] = path
						wantRepairs = 1
					case "budget_fresh", "budget_remaining", "budget_spent":
						spent := 1
						if mode == "budget_remaining" {
							spent = 2
						}
						if mode == "budget_spent" {
							spent = 3
						}
						task.Status, task.AttemptCount = release.TaskStatusInProgress, spent
						task.Metadata["recapture_outcome"] = "running"
						wantCalls, wantAttempts, outcome = 3-spent, 3, "exhausted"
					case "ambiguous":
						raw, _ := json.Marshal([]gates.CheckFailure{{Gate: "lint", ExitCode: 2}, {Gate: "test", ExitCode: 2}})
						digest := sha256.Sum256(raw)
						refs[gates.VerificationFailureReceiptKey], refs[gates.VerificationFailureDigestKey] = string(raw), hex.EncodeToString(digest[:])
						outcome = "unresolved"
					case "mismatch":
						phase, outcome = "verify", "unresolved"
					case "hitl":
						task.Metadata["mode"] = release.TaskModeHITL
					case "review":
						task.NeedsReview = true
					case "cancellation":
						wantCalls, wantAttempts, outcome = 1, 2, "cancelled"
					case "launch_refusal":
						wantCalls, wantAttempts = 1, 2
					}
					if err := f.env.rel.UpdateTask(task); err != nil {
						t.Fatal(err)
					}
					data, err := json.Marshal(refs)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := f.env.mgr.state.GetDB().Exec(`UPDATE run_steps SET phase=?, metadata=? WHERE id='legacy'`, phase, string(data)); err != nil {
						t.Fatal(err)
					}
					executor := &variantRecaptureExecutor{fixture: f, gate: gate, mode: mode}
					f.env.mgr.cfg.StageExecutor = executor
					f.restart(t)
					for resume := 0; resume < 3; resume++ {
						queueErr := boundaryRun(f)
						if (queueErr == nil) != (mode == "success") {
							t.Fatalf("queue outcome: %v", queueErr)
						}
						f.restart(t)
						current, err := f.env.rel.TaskSnapshot(ctx, "A")
						if err != nil {
							t.Fatal(err)
						}
						gotOutcome, _ := current.Metadata["recapture_outcome"].(string)
						if current.AttemptCount != wantAttempts || f.calls != wantCalls || gotOutcome != outcome {
							t.Fatalf("resume=%d calls=%d want=%d attempts=%d want=%d outcome=%q want=%q", resume, f.calls, wantCalls, current.AttemptCount, wantAttempts, gotOutcome, outcome)
						}
						if mode == "success" {
							if current.Status != release.TaskStatusDone || current.Metadata["verification_failure_evidence"] != nil {
								t.Fatalf("success retained failure: %+v", current)
							}
						} else if wantRepairs == 0 && current.Status != release.TaskStatusNeedsReview {
							t.Fatalf("boundary lost: %+v", current)
						}
						if mode == "hitl" && current.ExecutionMode() != release.TaskModeHITL || mode == "review" && !current.NeedsReview {
							t.Fatal("human boundary removed")
						}
						tasks, err := f.env.rel.TasksInStories(ctx, []string{"S"})
						if err != nil {
							t.Fatal(err)
						}
						// The check's fix, plus at most one provider fix of it:
						// the fixture stops inside the fix it runs.
						repairs, stops := 0, 0
						for _, candidate := range tasks {
							switch {
							case candidate.Metadata["repair_of"] == "A" && candidate.Metadata["failure_kind"] == "check":
								repairs++
							case candidate.Metadata["repair_of"] == "A" && candidate.Metadata["failure_kind"] == "provider" && candidate.Metadata["fix_of"] != "A":
								stops++
							}
						}
						if repairs != wantRepairs || stops > wantRepairs || len(tasks) != 2+wantRepairs+stops {
							t.Fatalf("duplicate or missing repair: %+v", tasks)
						}
						settings, err := f.env.rel.TaskSnapshot(ctx, "Settings")
						if err != nil {
							t.Fatal(err)
						}
						if mode == "success" {
							if settings.Status != release.TaskStatusDone || settings.AttemptCount != 1 {
								t.Fatalf("Settings not completed once: %+v", settings)
							}
						} else if settings.Status != release.TaskStatusPending || settings.AttemptCount != 0 {
							t.Fatalf("Settings escaped dependency: %+v", settings)
						}
						original, err := f.env.mgr.state.GetRunStep(ctx, "legacy")
						if err != nil || original == nil || original.Phase != phase || original.Metadata != string(data) {
							t.Fatalf("legacy receipt overwritten: %+v %v", original, err)
						}
					}
					if mode == "historical_command" && executor.command != "printf 'HISTORICAL_COMMAND\\n' >&2; exit 2" {
						t.Fatalf("historical command replaced: %q", executor.command)
					}
				})
			}
		}
	}
}
