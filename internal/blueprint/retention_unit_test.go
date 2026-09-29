package blueprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/actions"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

type retentionUnitExecutor struct {
	result *StageResult
	err    error
}

func (e retentionUnitExecutor) Execute(context.Context, *Stage, *StageInput) (*StageResult, error) {
	return e.result, e.err
}

func TestRetentionUnitEngineResultErrorMatrix(t *testing.T) {
	for _, single := range []bool{false, true} {
		for _, kind := range []string{"success", "failed-result", "result-error", "existing-error", "nil-error"} {
			t.Run(map[bool]string{true: "single", false: "full"}[single]+"/"+kind, func(t *testing.T) {
				original := errors.New("executor failed")
				result := NewStageResult("verify", 3)
				result.Complete("output")
				result.Diagnostics = "diagnostics"
				result.Artifacts = map[string]string{"hash": "path"}
				var execErr error
				switch kind {
				case "failed-result":
					result.Fail("check failed")
				case "result-error":
					execErr = original
				case "existing-error":
					execErr = original
					result.Error = "more specific"
				case "nil-error":
					result = nil
					execErr = original
				}
				var before StageResult
				if result != nil {
					before = *result
				}
				stage := &Stage{Name: "verify", Type: types.StageTypeDeterministic, OnSuccess: "complete"}
				bp := &Blueprint{ID: "matrix", Name: "matrix", InitialStage: "verify", Stages: map[string]*Stage{"verify": stage}}
				engine, err := NewEngine(bp, retentionUnitExecutor{result, execErr}, nil)
				if err != nil {
					t.Fatal(err)
				}
				input := NewStageInput("run", "", t.TempDir())
				run, _ := engine.StartRun(context.Background(), "run", input)
				if single {
					got, err := engine.ExecuteStage(context.Background(), run, "verify", input)
					if got != result || !errors.Is(err, execErr) {
						t.Fatalf("pair lost: %p %p %v", got, result, err)
					}
					if result == nil {
						if len(run.Results) != 0 {
							t.Fatal("nil result recorded")
						}
						return
					}
				} else {
					err := engine.Execute(context.Background(), run, input)
					if (err == nil) != (kind == "success") {
						t.Fatal("wrong disposition", err)
					}
				}
				got := run.GetLastResult()
				if result == nil {
					if got == nil || got.StageName != "verify" || got.Attempt != 1 || got.Error != original.Error() {
						t.Fatal("missing synthesized failure")
					}
					return
				}
				if got != result || got.Output != before.Output || got.Diagnostics != before.Diagnostics || !reflect.DeepEqual(got.Artifacts, before.Artifacts) || got.StartedAt != before.StartedAt || got.CompletedAt != before.CompletedAt || got.Attempt != 3 {
					t.Fatal("executor evidence changed")
				}
				if execErr != nil && (got.Status != types.StageStatusFailed || got.Error == "") {
					t.Fatal("error not applied")
				}
				if before.Error != "" && got.Error != before.Error {
					t.Fatal("specific error replaced")
				}
			})
		}
	}
}

func TestRetentionUnitEngineRefusesUnknownStageAndRetryLimit(t *testing.T) {
	bp := &Blueprint{ID: "limits", Name: "limits", InitialStage: "verify", Stages: map[string]*Stage{"verify": {Name: "verify", OnFailure: "verify", MaxRetries: 2}}}
	cfg := DefaultEngineConfig()
	cfg.MaxTotalRetries = 0
	engine, err := NewEngine(bp, retentionUnitExecutor{nil, errors.New("failed")}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	input := NewStageInput("run", "", t.TempDir())
	run, _ := engine.StartRun(context.Background(), "run", input)
	if _, err := engine.ExecuteStage(context.Background(), run, "missing", input); err == nil {
		t.Fatal("unknown single stage")
	}
	run.CurrentStage = "missing"
	if err := engine.Execute(context.Background(), run, input); err == nil {
		t.Fatal("unknown full stage")
	}
	run.CurrentStage = "verify"
	if err := engine.Execute(context.Background(), run, input); err == nil || !strings.Contains(err.Error(), "maximum total retries") {
		t.Fatal("retry bound", err)
	}
}

func TestRetentionUnitNativeCallbacksAndStorageRefusal(t *testing.T) {
	dir := t.TempDir()
	executor := NewDefaultExecutor(dir)
	stage := &Stage{Name: "verify", Type: types.StageTypeDeterministic, Commands: []string{`TOKEN=PRIVATE_SENTINEL; printf '%s' "$TOKEN"; exit 2`}, Timeout: time.Second}
	executor.VerificationStages = map[*Stage]bool{stage: true}
	calls := 0
	executor.OnCommandStart = func(_ *Stage, command string) {
		calls++
		if strings.Contains(command, "PRIVATE_SENTINEL") {
			t.Fatal("start leaked")
		}
	}
	executor.OnCommandComplete = func(_ *Stage, command, output string, err error) {
		calls++
		if err == nil || strings.Contains(command+output, "PRIVATE_SENTINEL") {
			t.Fatal("complete leaked")
		}
	}
	executor.OnVerificationFailure = func(_ *Stage, err error) {
		calls++
		if !gates.ValidateVerificationFailureArtifacts(gates.VerificationFailureArtifacts(err)) {
			t.Fatal("receipt missing")
		}
	}
	result, err := executor.Execute(context.Background(), stage, NewStageInput("run", "", dir))
	if err != nil || result.Status != types.StageStatusFailed || calls != 3 {
		t.Fatal("callbacks", calls, err)
	}
	if err := os.Chmod(filepath.Join(dir, ".openexec-verification"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.runCommandWithCheck(context.Background(), "exit 2", "verify"); err == nil || gates.VerificationFailureArtifacts(err) != nil {
		t.Fatal("capture refusal classified")
	}
	executor.WorkDir = ""
	if _, err := executor.runCommandWithCheck(context.Background(), "printf ok", ""); err != nil {
		t.Fatal(err)
	}
}

type retentionUnitAction struct {
	fail bool
	t    *testing.T
}

func (a retentionUnitAction) Name() string { return "retain" }
func (a retentionUnitAction) Execute(_ context.Context, request actions.ActionRequest) (actions.ActionResponse, error) {
	if request.Inputs["value"] != "expanded" {
		a.t.Fatal("input expansion lost", request.Inputs)
	}
	if a.fail {
		return actions.ActionResponse{}, errors.New("action failed")
	}
	return actions.ActionResponse{Status: types.StageStatusCompleted, Output: "action output", Artifacts: map[string]string{"hash": "path"}}, nil
}
func TestRetentionUnitNativeActionEvidence(t *testing.T) {
	executor := NewDefaultExecutor(t.TempDir())
	executor.ActionRegistry = actions.NewRegistry()
	stage := &Stage{Name: "verify", Type: types.StageTypeDeterministic, Action: "retain", Inputs: map[string]string{"value": "${key}"}}
	input := NewStageInput("run", "description", executor.WorkDir)
	input.Variables = map[string]string{"key": "expanded"}
	for _, fail := range []bool{false, true} {
		executor.ActionRegistry.Overwrite(retentionUnitAction{fail, t})
		result, err := executor.Execute(context.Background(), stage, input)
		if err != nil {
			t.Fatal(err)
		}
		if fail {
			if result.Status != types.StageStatusFailed || result.Error != "action failed" {
				t.Fatal("action refusal lost")
			}
		} else if result.Output != "action output" || result.Artifacts["hash"] != "path" || result.Status != types.StageStatusCompleted {
			t.Fatal("action evidence lost")
		}
	}
	stage.Action = "missing"
	if result, err := executor.Execute(context.Background(), stage, input); err != nil || result.Status != types.StageStatusFailed {
		t.Fatal("missing action accepted")
	}
}
