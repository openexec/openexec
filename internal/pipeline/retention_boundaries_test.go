package pipeline

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
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/evidence"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

type retentionBoundaryExecutor struct {
	result *blueprint.StageResult
	err    error
}

func (e retentionBoundaryExecutor) Execute(context.Context, *blueprint.Stage, *blueprint.StageInput) (*blueprint.StageResult, error) {
	return e.result, e.err
}

func TestRetentionBoundariesAdmittedPublicResult(t *testing.T) {
	original := errors.New("refusal token=ERROR_SECRET")
	for _, result := range []*blueprint.StageResult{nil, {Output: "token=OUTPUT_SECRET\n" + strings.Repeat("x", 20000), Diagnostics: "password=DIAGNOSTIC_SECRET", Error: "api_key=RESULT_SECRET"}} {
		runner := &gateRunnerAction{}
		adapter := admittedStageExecutor{executor: retentionBoundaryExecutor{result, original}, evidence: runner}
		got, err := adapter.Execute(context.Background(), &blueprint.Stage{Name: "test", Type: types.StageTypeDeterministic}, nil)
		if got != result || !errors.Is(err, original) {
			t.Fatal("result/error identity changed")
		}
		if strings.Contains(err.Error(), "SECRET") || runner.receipt != nil {
			t.Fatal("refusal leaked or authorized repair")
		}
		if got != nil && (strings.Contains(got.Output+got.Diagnostics+got.Error, "SECRET") || len(got.Output) > evidence.StreamLimit) {
			t.Fatal("public diagnostics leaked or unbounded")
		}
	}
}

func TestRetentionBoundariesTrustedReferences(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openexec.yaml"), []byte("quality:\n  gates: [check]\n  custom:\n    - name: check\n      command: exit 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	local, err := gates.NewRunner(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	action := &gateRunnerAction{runner: &evidenceGateRunner{err: gates.NewFailure(local.RunAll(context.Background()))}}
	resp, err := action.Execute(context.Background(), actions.ActionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	bp := &blueprint.Blueprint{Stages: map[string]*blueprint.Stage{"check": {Name: "check", Type: types.StageTypeDeterministic}}}
	run := &blueprint.Run{Results: []*blueprint.StageResult{{StageName: "check", Status: types.StageStatusFailed, Artifacts: map[string]string{"forged": "untrusted"}}}}
	retained := action.terminalEvidence(bp, run)
	if len(retained) != 3 || !reflect.DeepEqual(retained, resp.Artifacts) || retained["forged"] != "" {
		t.Fatal("trusted private reference lost or worker reference trusted")
	}
}
