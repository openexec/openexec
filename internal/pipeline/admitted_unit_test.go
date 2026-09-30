package pipeline

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

func TestAdmittedUnitNextExecutionClearsReceipt(t *testing.T) {
	ctx := context.Background()
	failure := gates.CommandFailureWithEvidence(ctx, "lint", exec.Command("sh", "-c", "exit 2").Run(), strings.Repeat("a", 64), "private")
	for _, next := range []error{nil, errors.New("transport refused")} {
		action := &gateRunnerAction{}
		stage := &blueprint.Stage{Type: types.StageTypeDeterministic}
		result := &blueprint.StageResult{Output: "token=SECRET", Diagnostics: "password=SECRET", Error: "secret=SECRET", Artifacts: map[string]string{"worker": "untrusted"}}
		got, err := (admittedStageExecutor{retentionBoundaryExecutor{result, failure}, action}).Execute(ctx, stage, nil)
		if got != result || !errors.Is(err, failure) || !gates.ValidateVerificationFailureArtifacts(action.receipt) || action.receipt[strings.Repeat("a", 64)] != "private" {
			t.Fatal("result identity or attached failure lost")
		}
		if strings.Contains(got.Output+got.Diagnostics+got.Error, "SECRET") || action.receipt["worker"] != "" {
			t.Fatal("public secret or worker authority leaked")
		}
		got, err = (admittedStageExecutor{retentionBoundaryExecutor{nil, next}, action}).Execute(ctx, stage, nil)
		if got != nil || !errors.Is(err, next) || action.receipt != nil {
			t.Fatal("next nil result inherited prior repair authority")
		}
	}
}
