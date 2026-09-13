package pipeline

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/openexec/openexec/internal/actions"
	"github.com/openexec/openexec/internal/blueprint"
	"github.com/openexec/openexec/internal/execution/gates"
	"github.com/openexec/openexec/internal/types"
)

type evidenceGateRunner struct{ err error }

func (r *evidenceGateRunner) RunAll(context.Context) error { return r.err }

func TestTerminalVerificationEvidenceRequiresCurrentDeterministicRunner(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openexec.yaml"), []byte("quality:\n  gates: [check]\n  custom:\n    - name: check\n      command: exit 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	local, err := gates.NewRunner(dir, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	runner := &evidenceGateRunner{err: gates.NewFailure(local.RunAll(context.Background()))}
	action := &gateRunnerAction{runner: runner}
	resp, err := action.Execute(context.Background(), actions.ActionRequest{})
	if err != nil || !gates.ValidateVerificationFailureArtifacts(resp.Artifacts) {
		t.Fatalf("missing real evidence: %+v %v", resp, err)
	}
	bp := &blueprint.Blueprint{Stages: map[string]*blueprint.Stage{"check": {Name: "check", Type: types.StageTypeDeterministic}}}
	run := &blueprint.Run{Results: []*blueprint.StageResult{{StageName: "check", Status: types.StageStatusFailed}}}
	if action.terminalEvidence(bp, run) == nil {
		t.Fatal("terminal failed deterministic receipt lost")
	}
	bp.Stages["check"].Type = types.StageTypeAgentic
	run.Results[0].Artifacts = resp.Artifacts
	if action.terminalEvidence(bp, run) != nil {
		t.Fatal("worker artifacts authorized verification failure")
	}
	bp.Stages["check"].Type = types.StageTypeDeterministic
	runner.err = nil
	action.Execute(context.Background(), actions.ActionRequest{})
	if action.terminalEvidence(bp, run) != nil {
		t.Fatal("prior failed retry reused after success")
	}
	runner.err = errors.New("Quality gates failed: check")
	action.Execute(context.Background(), actions.ActionRequest{})
	if action.terminalEvidence(bp, run) != nil {
		t.Fatal("prose classified as evidence")
	}
}
