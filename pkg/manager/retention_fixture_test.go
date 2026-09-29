package manager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/openexec/openexec/pkg/runtime"
)

const retentionMarker = "RETAINED_EXIT_TWO"
const retentionLimit = 4096

// retentionFixture exercises the admitted StageExecutor seam with a real
// process and an existing content-addressed evidence file, without a provider.
type retentionFixture struct {
	t          *testing.T
	dir        string
	argv       []string
	hash, path string
	last       *runtime.StageResult
}

type retentionEvidence struct {
	Argv           []string
	Cwd            string
	ExitCode       int
	Stdout, Stderr string
}

type retentionBuffer struct{ buffer bytes.Buffer }

func (b *retentionBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := retentionLimit - b.buffer.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buffer.Write(p)
	}
	return n, nil
}

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
	cmd := exec.CommandContext(ctx, f.argv[0], f.argv[1:]...)
	cmd.Dir = f.dir
	var stdout, stderr retentionBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 2 {
		f.t.Fatalf("fixture exit: %v", err)
	}
	raw, marshalErr := json.Marshal(retentionEvidence{f.argv, cmd.Dir, cmd.ProcessState.ExitCode(), stdout.buffer.String(), stderr.buffer.String()})
	if marshalErr != nil {
		f.t.Fatal(marshalErr)
	}
	hash := sha256.Sum256(raw)
	f.hash = hex.EncodeToString(hash[:])
	f.path = filepath.Join(f.dir, f.hash+".log")
	if writeErr := os.WriteFile(f.path, raw, 0600); writeErr != nil {
		f.t.Fatal(writeErr)
	}
	result.Status = runtime.StageStatusFailed
	result.Output, result.Diagnostics = stdout.buffer.String(), stderr.buffer.String()
	result.Error = "verification fixture exited 2"
	result.Artifacts = map[string]string{f.hash: f.path}
	f.last = result
	return result, runtime.VerificationCommandFailure(ctx, stage.Name, err)
}
