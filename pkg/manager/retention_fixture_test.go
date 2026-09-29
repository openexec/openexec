package manager

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openexec/openexec/pkg/runtime"
)

const retentionMarker = "RETAINED_EXIT_TWO"
const retentionLimit = runtime.EvidenceStreamLimit

// retentionFixture exercises the admitted StageExecutor seam with a real
// process and an existing content-addressed evidence file, without a provider.
type retentionFixture struct {
	t          *testing.T
	dir        string
	argv       []string
	hash, path string
	last       *runtime.StageResult
	secrets    []string
	toolchain  map[string]string
	mode       string
}

type retentionEvidence = runtime.CommandEvidence

func newRetentionFixture(t *testing.T, dir string) *retentionFixture {
	t.Helper()
	script := filepath.Join(dir, "verify fixture.sh")
	if err := os.WriteFile(script, []byte("printf 'RETAINED_EXIT_TWO stdout\\n'; printf 'RETAINED_EXIT_TWO stderr\\n' >&2\ni=0; while [ $i -lt 6000 ]; do printf x; printf y >&2; i=$((i+1)); done\nexit 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return &retentionFixture{t: t, dir: dir, argv: []string{"/bin/sh", script, "argument with spaces"}}
}

func (f *retentionFixture) Execute(ctx context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	result := &runtime.StageResult{StageName: stage.Name, Attempt: 1, Status: runtime.StageStatusCompleted}
	if stage.Name != "test" {
		return result, nil
	}
	switch f.mode {
	case "nil-error":
		return nil, errors.New("transport token=NIL_ERROR_SENTINEL")
	case "refusal":
		return result, errors.New("admission refused token=REFUSAL_SENTINEL")
	case "cancel":
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		cmd := exec.CommandContext(cancelled, f.argv[0], f.argv[1:]...)
		err := cmd.Run()
		result.Status = runtime.StageStatusFailed
		return result, runtime.VerificationCommandFailure(cancelled, stage.Name, err)
	}
	cmd := exec.CommandContext(ctx, f.argv[0], f.argv[1:]...)
	cmd.Dir = f.dir
	var stdout, stderr runtime.EvidenceBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	execErr := cmd.Run()
	err := execErr
	expected := 2
	if f.mode == "success" {
		expected = 0
	}
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != expected {
		f.t.Fatalf("fixture exit: %v", err)
	}
	f.hash, f.path, err = runtime.RetainCommandEvidence(f.dir, retentionEvidence{
		Argv: f.argv, Cwd: cmd.Dir, ExitCode: cmd.ProcessState.ExitCode(), Stdout: stdout.String(), Stderr: stderr.String(),
		StdoutTruncated: stdout.Truncated, StderrTruncated: stderr.Truncated, Toolchain: f.toolchain,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	result.Status = runtime.StageStatusFailed
	if f.mode == "success" {
		result.Status = runtime.StageStatusCompleted
	}
	result.Output, result.Diagnostics = runtime.PublicVerificationStream(&stdout, f.secrets), runtime.PublicVerificationStream(&stderr, f.secrets)
	if execErr != nil {
		result.Error = "verification fixture exited 2"
	}
	result.Artifacts = map[string]string{f.hash: f.path}
	f.last = result
	return result, runtime.VerificationCommandFailure(ctx, stage.Name, execErr)
}

// Shared boundary inputs are consumed independently by coverage and mutation tests.
func newRetentionBoundaryFixture(t *testing.T, dir, mode string) *retentionFixture {
	f := newRetentionFixture(t, dir)
	f.mode = mode
	f.secrets = []string{"COMMAND_VALUE_SENTINEL", "DIAGNOSTIC_VALUE_SENTINEL"}
	f.argv = append(f.argv, "--token=COMMAND_VALUE_SENTINEL")
	f.toolchain = map[string]string{"go_version": strings.Repeat("v", 200), "PATH": "ENVIRONMENT_SENTINEL", "unlisted_version": "UNLISTED_SENTINEL"}
	exit := "2"
	if mode == "success" {
		exit = "0"
	}
	script := "printf 'RETAINED_EXIT_TWO token=COMMAND_VALUE_SENTINEL\\n'; printf 'RETAINED_EXIT_TWO DIAGNOSTIC_VALUE_SENTINEL\\n' >&2\n" +
		"i=0; while [ $i -lt 6000 ]; do printf x; printf y >&2; i=$((i+1)); done\nexit " + exit + "\n"
	if err := os.WriteFile(f.argv[1], []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	return f
}
