// Package admittedevidence models an external adapter using only public runtime
// APIs. Its silent check must carry evidence solely through the typed error.
package admittedevidence

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/openexec/openexec/pkg/runtime"
)

type Executor struct {
	Dir, Gate  string
	Hash, Path string
	Argv       []string
	Calls      int
	Diagnostic bool
	NilResult  bool
}

func (e *Executor) Execute(ctx context.Context, stage *runtime.Stage, _ *runtime.StageInput) (*runtime.StageResult, error) {
	result := &runtime.StageResult{StageName: stage.Name, Attempt: 1, Status: runtime.StageStatusCompleted}
	if stage.Name != e.Gate {
		return result, nil
	}
	e.Calls++
	e.Argv = []string{"/bin/sh", "-c", "exit 2", "argument with spaces"}
	if e.Diagnostic {
		e.Argv[2] = "printf 'head\\n'; i=0; while [ $i -lt 3000 ]; do printf 'noise line\\n'; printf 'stderr noise\\n' >&2; i=$((i+1)); done; printf 'token=PRIVATE_SENTINEL\\nDIAGNOSTIC_TAIL\\n'; printf 'STDERR_TAIL\\n' >&2; exit 2"
	}
	cmd := exec.CommandContext(ctx, e.Argv[0], e.Argv[1:]...)
	cmd.Dir = e.Dir
	var stdout, stderr runtime.EvidenceBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	execErr := cmd.Run()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 2 {
		return nil, fmt.Errorf("silent fixture did not exit 2: %w", execErr)
	}
	var err error
	e.Hash, e.Path, err = runtime.RetainCommandEvidence(e.Dir, runtime.CommandEvidence{
		Argv: e.Argv, Cwd: cmd.Dir, ExitCode: cmd.ProcessState.ExitCode(),
		Stdout: stdout.String(), Stderr: stderr.String(),
		StdoutTruncated: stdout.Truncated, StderrTruncated: stderr.Truncated,
		Toolchain: map[string]string{"go_version": "fixture-go", "TOKEN": "EXCLUDED_SENTINEL"},
	})
	if err != nil {
		return nil, err
	}
	result.Status = runtime.StageStatusFailed
	// No header, output, diagnostics or result.Artifacts fallback: losing the
	// wrapper attachment must leave a diagnostic-free receipt.
	if e.Diagnostic {
		result.Output = runtime.PublicVerificationStream(&stdout, []string{"PRIVATE_SENTINEL"})
		result.Diagnostics = runtime.PublicVerificationStream(&stderr, nil)
	}
	if e.NilResult {
		result = nil
	}
	return result, runtime.VerificationCommandFailureWithEvidence(ctx, stage.Name, execErr, e.Hash, e.Path)
}
